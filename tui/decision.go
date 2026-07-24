package tui

import (
	"context"
	"reflect"
	"sync"
	"time"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/permissions"
)

type DecisionRequest = permissions.PromptRequest

type DecisionRecord struct {
	Prompt     permissions.PromptRequest
	Response   permissions.PromptResponse
	ResolvedAt time.Time
}

// decisionWaiter owns exactly one terminal response. The response channel is
// buffered so the broker never holds its mutex while waking a requester.
type decisionWaiter struct {
	request   permissions.PromptRequest
	sessionID string
	epoch     uint64
	response  chan permissions.PromptResponse
	claimed   bool
}

type decisionBrokerTransition struct {
	wasActive bool
	next      *decisionWaiter
}

// decisionBroker separates concurrent registration from serialized overlay
// presentation. All waiters are registered immediately; active identifies the
// one request currently shown by the fullscreen decision dialog.
type decisionBroker struct {
	mu       sync.Mutex
	waiters  []*decisionWaiter
	active   *decisionWaiter
	pumping  bool
	wakePump chan struct{}
}

func newDecisionBroker() *decisionBroker {
	return &decisionBroker{wakePump: make(chan struct{}, 1)}
}

func (b *decisionBroker) register(request permissions.PromptRequest, sessionID string, epoch uint64) (*decisionWaiter, bool, bool) {
	b.mu.Lock()
	waiter := &decisionWaiter{
		request: request, sessionID: sessionID, epoch: epoch,
		response: make(chan permissions.PromptResponse, 1),
	}
	b.waiters = append(b.waiters, waiter)
	becameActive := false
	if b.active == nil {
		b.active = waiter
		becameActive = true
	}
	startPump := !b.pumping
	if startPump {
		b.pumping = true
	}
	b.mu.Unlock()
	return waiter, becameActive, startPump
}

// deliver routes a UI response to the oldest unresolved waiter with the same
// public decision identity. Duplicate IDs remain deterministic: the active
// overlay wins, followed by registration order.
func (b *decisionBroker) deliver(response permissions.PromptResponse) bool {
	b.mu.Lock()
	var target *decisionWaiter
	if b.active != nil && !b.active.claimed && (response.DecisionID == "" || b.active.request.DecisionID == response.DecisionID) {
		target = b.active
	} else if response.DecisionID != "" {
		for _, waiter := range b.waiters {
			if !waiter.claimed && waiter.request.DecisionID == response.DecisionID {
				target = waiter
				break
			}
		}
	}
	if target == nil {
		b.mu.Unlock()
		return false
	}
	target.claimed = true
	b.mu.Unlock()
	response.DecisionID = target.request.DecisionID
	target.response <- response
	return true
}

// resolve claims one exact waiter for cancellation, timeout, or shutdown. If
// another producer already claimed it, the caller waits for that producer's
// response instead of manufacturing a second terminal outcome.
func (b *decisionBroker) resolve(waiter *decisionWaiter, response permissions.PromptResponse) bool {
	b.mu.Lock()
	if waiter == nil || waiter.claimed {
		b.mu.Unlock()
		return false
	}
	waiter.claimed = true
	b.mu.Unlock()
	response.DecisionID = waiter.request.DecisionID
	waiter.response <- response
	return true
}

func (b *decisionBroker) resolveAll(outcome permissions.PromptOutcome) {
	b.mu.Lock()
	claimed := make([]*decisionWaiter, 0, len(b.waiters))
	for _, waiter := range b.waiters {
		if waiter.claimed {
			continue
		}
		waiter.claimed = true
		claimed = append(claimed, waiter)
	}
	b.mu.Unlock()
	for _, waiter := range claimed {
		waiter.response <- decisionResponse(waiter.request.DecisionID, outcome, "")
	}
}

// complete removes a terminal waiter and chooses the next unresolved overlay.
// A queued waiter that already received a keyed response is skipped rather
// than flashing a dialog while its audit commit is pending.
func (b *decisionBroker) complete(waiter *decisionWaiter) decisionBrokerTransition {
	b.mu.Lock()
	transition := decisionBrokerTransition{wasActive: b.active == waiter}
	for index, candidate := range b.waiters {
		if candidate != waiter {
			continue
		}
		copy(b.waiters[index:], b.waiters[index+1:])
		b.waiters[len(b.waiters)-1] = nil
		b.waiters = b.waiters[:len(b.waiters)-1]
		break
	}
	if transition.wasActive {
		b.active = nil
		for _, candidate := range b.waiters {
			if !candidate.claimed {
				b.active = candidate
				transition.next = candidate
				break
			}
		}
	}
	b.mu.Unlock()
	b.wake()
	return transition
}

func (b *decisionBroker) stopPumpIfIdle() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.waiters) != 0 {
		return false
	}
	b.pumping = false
	return true
}

func (b *decisionBroker) markPumpStopped() {
	b.mu.Lock()
	b.pumping = false
	b.mu.Unlock()
}

func (b *decisionBroker) wake() {
	select {
	case b.wakePump <- struct{}{}:
	default:
	}
}

func clonePromptRequest(request permissions.PromptRequest) permissions.PromptRequest {
	request.Input = cloneDecisionMap(request.Input)
	request.Choices = append([]string(nil), request.Choices...)
	request.ReviewDetails = append([]string(nil), request.ReviewDetails...)
	request.Questionnaire = cloneAskUserQuestionnaire(request.Questionnaire)
	return request
}

func cloneDecisionMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	cloned := make(map[string]any, len(input))
	for key, value := range input {
		cloned[key] = cloneDecisionValue(value)
	}
	return cloned
}

func cloneDecisionValue(value any) any {
	reflected := cloneDecisionReflectValue(reflect.ValueOf(value))
	if !reflected.IsValid() {
		return nil
	}
	return reflected.Interface()
}

func cloneDecisionReflectValue(value reflect.Value) reflect.Value {
	if !value.IsValid() {
		return reflect.Value{}
	}
	switch value.Kind() {
	case reflect.Interface:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		cloned := cloneDecisionReflectValue(value.Elem())
		wrapped := reflect.New(value.Type()).Elem()
		wrapped.Set(cloned)
		return wrapped
	case reflect.Map:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		cloned := reflect.MakeMapWithSize(value.Type(), value.Len())
		iterator := value.MapRange()
		for iterator.Next() {
			cloned.SetMapIndex(iterator.Key(), cloneDecisionReflectValue(iterator.Value()))
		}
		return cloned
	case reflect.Slice:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		cloned := reflect.MakeSlice(value.Type(), value.Len(), value.Len())
		for index := 0; index < value.Len(); index++ {
			cloned.Index(index).Set(cloneDecisionReflectValue(value.Index(index)))
		}
		return cloned
	case reflect.Array:
		cloned := reflect.New(value.Type()).Elem()
		for index := 0; index < value.Len(); index++ {
			cloned.Index(index).Set(cloneDecisionReflectValue(value.Index(index)))
		}
		return cloned
	case reflect.Pointer:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		cloned := reflect.New(value.Type().Elem())
		cloned.Elem().Set(cloneDecisionReflectValue(value.Elem()))
		return cloned
	default:
		return value
	}
}

func decisionResponse(decisionID string, outcome permissions.PromptOutcome, choice string) permissions.PromptResponse {
	response := permissions.PromptResponse{DecisionID: decisionID, Choice: choice, Outcome: outcome, Decision: permissions.DecisionDeny}
	if outcome != permissions.PromptOutcomeApproved {
		return response
	}
	switch choice {
	case "always_allow":
		response.Decision = permissions.DecisionAllow
	case "allow_once", "execute":
		response.Decision = permissions.DecisionAllowOnce
	}
	return response
}

func decisionResponseForContext(ctx context.Context) permissions.PromptResponse {
	if ctx != nil && ctx.Err() == context.DeadlineExceeded {
		return decisionResponse("", permissions.PromptOutcomeTimedOut, "")
	}
	return decisionResponse("", permissions.PromptOutcomeCancelled, "")
}

func formatDecisionReceipt(request permissions.PromptRequest, response permissions.PromptResponse) string {
	return formatDecisionReceiptInLanguage(i18n.DetectOrLoadLanguage(), request, response)
}

func formatDecisionReceiptInLanguage(lang i18n.Language, request permissions.PromptRequest, response permissions.PromptResponse) string {
	action := request.Action
	if action == "" {
		action = request.ToolName
	}
	result := i18n.RuntimeDecisionOutcomeLabel(lang, string(response.Outcome))
	if response.Choice != "" {
		result += " (" + i18n.RuntimeDecisionChoiceLabel(lang, response.Choice) + ")"
	}
	return i18n.Format(lang, i18n.KeyRuntimeDecisionReceiptLine, result, action)
}
