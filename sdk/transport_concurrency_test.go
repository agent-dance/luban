package sdk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agent-dance/luban/i18n"
)

type concurrencyRuntime struct {
	Runtime

	mu           sync.Mutex
	permission   PermissionHandler
	queryCalls   int
	queryFn      func(context.Context, QueryRequest, PermissionHandler) (<-chan QueryEvent, error)
	compactFn    func(context.Context, string) (CompactResult, error)
	interrupts   chan string
	compactCalls int
}

func (r *concurrencyRuntime) SetPermission(handler PermissionHandler) {
	r.mu.Lock()
	r.permission = handler
	r.mu.Unlock()
}

func (r *concurrencyRuntime) Query(ctx context.Context, request QueryRequest) (<-chan QueryEvent, error) {
	r.mu.Lock()
	r.queryCalls++
	permission := r.permission
	queryFn := r.queryFn
	r.mu.Unlock()
	return queryFn(ctx, request, permission)
}

func (r *concurrencyRuntime) Interrupt(sessionID string) {
	if r.interrupts != nil {
		r.interrupts <- sessionID
	}
}

func (r *concurrencyRuntime) Compact(ctx context.Context, sessionID string) (CompactResult, error) {
	r.mu.Lock()
	r.compactCalls++
	compactFn := r.compactFn
	r.mu.Unlock()
	if compactFn != nil {
		return compactFn(ctx, sessionID)
	}
	return CompactResult{
		Compacted: true, BeforeMessageCount: 40, AfterMessageCount: 8, ContextGeneration: 3,
	}, nil
}

func startSDKServe(t *testing.T, runtime Runtime) (*io.PipeWriter, chan map[string]any, <-chan error) {
	return startSDKServeWithPermissionMode(t, runtime, InitialPermissionBridge)
}

func startSDKServeWithPermissionMode(t *testing.T, runtime Runtime, mode InitialPermissionMode) (*io.PipeWriter, chan map[string]any, <-chan error) {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	server := NewSDKServer(runtime, inR, outW, mode)
	done := make(chan error, 1)
	go func() {
		err := server.Serve(context.Background())
		_ = outW.Close()
		done <- err
	}()
	output := startScanner(outR)
	drainInit(t, output)
	return inW, output, done
}

func TestServeInitialPermissionModeControlsFirstToolCall(t *testing.T) {
	tests := []struct {
		name          string
		mode          InitialPermissionMode
		wantChallenge bool
	}{
		{name: "bridge", mode: InitialPermissionBridge, wantChallenge: true},
		{name: "full auto", mode: InitialPermissionFullAuto, wantChallenge: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime := &concurrencyRuntime{}
			runtime.queryFn = func(ctx context.Context, request QueryRequest, permission PermissionHandler) (<-chan QueryEvent, error) {
				out := make(chan QueryEvent)
				go func() {
					defer close(out)
					decision, err := permission.Check(ctx, PermissionRequest{
						SessionID: request.SessionID,
						ToolName:  "Write",
						Input:     map[string]any{"path": "game.js"},
					})
					if err != nil || decision != PermissionAllow {
						return
					}
					select {
					case out <- QueryEvent{SessionID: request.SessionID, Final: true}:
					case <-ctx.Done():
					}
				}()
				return out, nil
			}

			inW, output, done := startSDKServeWithPermissionMode(t, runtime, test.mode)
			sendLine(t, inW, SDKUserMessage{
				Type: "user", SessionID: "initial-permission-session", UUID: "initial-permission-query",
				Message: textUserMessage("write the game"),
			})

			first := receiveWithTimeout(t, output, testTimeout)
			if test.wantChallenge {
				if first["type"] != "control_request" {
					t.Fatalf("first tool call output = %#v, want permission challenge", first)
				}
				requestID, _ := first["request_id"].(string)
				permission, err := json.Marshal(PermissionResultMsg{Behavior: "allow"})
				if err != nil {
					t.Fatal(err)
				}
				response, err := json.Marshal(ControlSuccess{
					Subtype: "success", RequestID: requestID, Response: permission,
				})
				if err != nil {
					t.Fatal(err)
				}
				sendLine(t, inW, SDKControlResponse{Type: "control_response", Response: response})
				first = receiveWithTimeout(t, output, testTimeout)
			}
			if first["type"] != "result" || first["uuid"] != "initial-permission-query" || first["is_error"] != false {
				t.Fatalf("terminal result = %#v", first)
			}

			if err := inW.Close(); err != nil {
				t.Fatal(err)
			}
			waitServe(t, done, nil)
		})
	}
}

func waitServe(t *testing.T, done <-chan error, want error) {
	t.Helper()
	select {
	case err := <-done:
		if !errors.Is(err, want) {
			t.Fatalf("Serve error = %v, want %v", err, want)
		}
	case <-time.After(testTimeout):
		t.Fatal("timed out waiting for Serve")
	}
}

func TestServePermissionResponseIsReadWhileQueryIsActive(t *testing.T) {
	permissionDone := make(chan struct{})
	runtime := &concurrencyRuntime{}
	runtime.queryFn = func(ctx context.Context, request QueryRequest, permission PermissionHandler) (<-chan QueryEvent, error) {
		out := make(chan QueryEvent)
		go func() {
			defer close(out)
			defer close(permissionDone)
			decision, err := permission.Check(ctx, PermissionRequest{
				SessionID: request.SessionID, ToolName: "Write", Input: map[string]any{"path": "safe.txt"},
			})
			if err != nil || decision != PermissionAllow {
				return
			}
			select {
			case out <- QueryEvent{SessionID: request.SessionID, Event: Event{Type: EventText, Text: "allowed"}}:
			case <-ctx.Done():
				return
			}
			select {
			case out <- QueryEvent{SessionID: request.SessionID, Final: true}:
			case <-ctx.Done():
			}
		}()
		return out, nil
	}

	inW, output, done := startSDKServe(t, runtime)
	sendLine(t, inW, SDKUserMessage{
		Type: "user", SessionID: "permission-session", UUID: "permission-query", Message: textUserMessage("write"),
	})

	challenge := receiveWithTimeout(t, output, testTimeout)
	if challenge["type"] != "control_request" {
		t.Fatalf("challenge type = %v, want control_request", challenge["type"])
	}
	requestID, _ := challenge["request_id"].(string)
	if requestID == "" {
		t.Fatal("permission challenge omitted request_id")
	}
	permission, err := json.Marshal(PermissionResultMsg{Behavior: "allow"})
	if err != nil {
		t.Fatal(err)
	}
	response, err := json.Marshal(ControlSuccess{
		Subtype: "success", RequestID: requestID, Response: permission,
	})
	if err != nil {
		t.Fatal(err)
	}
	sendLine(t, inW, SDKControlResponse{Type: "control_response", Response: response})

	text := receiveWithTimeout(t, output, testTimeout)
	result := receiveWithTimeout(t, output, testTimeout)
	if text["type"] != "streamlined_text" || text["text"] != "allowed" {
		t.Fatalf("text output = %#v", text)
	}
	if result["type"] != "result" || result["uuid"] != "permission-query" || result["is_error"] != false {
		t.Fatalf("terminal result = %#v", result)
	}
	select {
	case <-permissionDone:
	case <-time.After(testTimeout):
		t.Fatal("permission waiter did not exit")
	}
	if err := inW.Close(); err != nil {
		t.Fatal(err)
	}
	waitServe(t, done, nil)
}

func TestServeSingleActiveQueryDeduplicatesReplaysAndRejectsAnotherUUID(t *testing.T) {
	queryStarted := make(chan struct{})
	events := make(chan QueryEvent)
	runtime := &concurrencyRuntime{}
	runtime.queryFn = func(context.Context, QueryRequest, PermissionHandler) (<-chan QueryEvent, error) {
		close(queryStarted)
		return events, nil
	}

	inW, output, done := startSDKServe(t, runtime)
	first := SDKUserMessage{Type: "user", SessionID: "session-1", UUID: "query-1", Message: textUserMessage("one")}
	second := SDKUserMessage{Type: "user", SessionID: "session-2", UUID: "query-2", Message: textUserMessage("two")}
	sendLine(t, inW, first)
	select {
	case <-queryStarted:
	case <-time.After(testTimeout):
		t.Fatal("first query did not start")
	}
	sendLine(t, inW, first)
	sendLine(t, inW, second)
	sendLine(t, inW, second)

	events <- QueryEvent{SessionID: "session-1", Event: Event{Type: EventText, Text: "one"}}
	events <- QueryEvent{SessionID: "session-1", Final: true}

	text := receiveWithTimeout(t, output, testTimeout)
	firstResult := receiveWithTimeout(t, output, testTimeout)
	busyResult := receiveWithTimeout(t, output, testTimeout)
	if text["type"] != "streamlined_text" || text["text"] != "one" {
		t.Fatalf("stream output = %#v", text)
	}
	if firstResult["type"] != "result" || firstResult["uuid"] != "query-1" || firstResult["is_error"] != false {
		t.Fatalf("first terminal = %#v", firstResult)
	}
	if busyResult["type"] != "result" || busyResult["uuid"] != "query-2" || busyResult["is_error"] != true {
		t.Fatalf("busy terminal = %#v", busyResult)
	}

	runtime.mu.Lock()
	queryCalls := runtime.queryCalls
	runtime.mu.Unlock()
	if queryCalls != 1 {
		t.Fatalf("Runtime.Query calls = %d, want 1", queryCalls)
	}
	select {
	case extra := <-output:
		t.Fatalf("duplicate user UUID produced extra output: %#v", extra)
	case <-time.After(50 * time.Millisecond):
	}
	close(events)
	if err := inW.Close(); err != nil {
		t.Fatal(err)
	}
	waitServe(t, done, nil)
}

func TestServeClosedQueryChannelEmitsOneErrorTerminal(t *testing.T) {
	runtime := &concurrencyRuntime{}
	runtime.queryFn = func(context.Context, QueryRequest, PermissionHandler) (<-chan QueryEvent, error) {
		closed := make(chan QueryEvent)
		close(closed)
		return closed, nil
	}
	inW, output, done := startSDKServe(t, runtime)
	sendLine(t, inW, SDKUserMessage{
		Type: "user", SessionID: "closed-session", UUID: "closed-query", Message: textUserMessage("wait"),
	})
	result := receiveWithTimeout(t, output, testTimeout)
	if result["type"] != "result" || result["uuid"] != "closed-query" || result["is_error"] != true {
		t.Fatalf("closed-channel terminal = %#v", result)
	}
	if errorsList, ok := result["errors"].([]any); !ok || len(errorsList) != 1 {
		t.Fatalf("closed-channel errors = %#v", result["errors"])
	}
	select {
	case extra := <-output:
		t.Fatalf("closed query channel produced extra terminal/output: %#v", extra)
	case <-time.After(50 * time.Millisecond):
	}
	if err := inW.Close(); err != nil {
		t.Fatal(err)
	}
	waitServe(t, done, nil)
}

func TestServeAsyncQueryOutputFailureTerminatesInsteadOfDroppingTerminal(t *testing.T) {
	runtime := &concurrencyRuntime{}
	runtime.queryFn = func(context.Context, QueryRequest, PermissionHandler) (<-chan QueryEvent, error) {
		events := make(chan QueryEvent, 1)
		events <- QueryEvent{Event: Event{
			Type: EventProgress, Metadata: map[string]any{"unsupported": make(chan struct{})},
		}}
		close(events)
		return events, nil
	}
	inW, _, done := startSDKServe(t, runtime)
	sendLine(t, inW, SDKUserMessage{Type: "user", UUID: "marshal-query", Message: textUserMessage("wait")})
	select {
	case err := <-done:
		var unsupported *json.UnsupportedTypeError
		if !errors.As(err, &unsupported) {
			t.Fatalf("Serve error = %T %v, want json.UnsupportedTypeError", err, err)
		}
	case <-time.After(testTimeout):
		t.Fatal("async query output error did not terminate Serve")
	}
	_ = inW.Close()
}

func TestServeRejectsCompactButInterruptsActiveQuery(t *testing.T) {
	queryStarted := make(chan struct{})
	runtime := &concurrencyRuntime{interrupts: make(chan string, 1)}
	runtime.queryFn = func(ctx context.Context, _ QueryRequest, _ PermissionHandler) (<-chan QueryEvent, error) {
		close(queryStarted)
		return make(chan QueryEvent), nil
	}

	inW, output, done := startSDKServe(t, runtime)
	sendLine(t, inW, SDKUserMessage{
		Type: "user", SessionID: "active-session", UUID: "active-query", Message: textUserMessage("wait"),
	})
	select {
	case <-queryStarted:
	case <-time.After(testTimeout):
		t.Fatal("query did not start")
	}

	compact, _ := json.Marshal(CompactRequest{Subtype: "compact", SessionID: "active-session"})
	sendLine(t, inW, SDKControlRequest{Type: "control_request", RequestID: "compact-1", Request: compact})
	compactResponse := receiveWithTimeout(t, output, testTimeout)
	responseBytes, _ := json.Marshal(compactResponse["response"])
	var controlError ControlError
	if err := json.Unmarshal(responseBytes, &controlError); err != nil {
		t.Fatal(err)
	}
	if controlError.Subtype != "error" || controlError.RequestID != "compact-1" {
		t.Fatalf("compact response = %#v", compactResponse)
	}
	runtime.mu.Lock()
	compactCalls := runtime.compactCalls
	runtime.mu.Unlock()
	if compactCalls != 0 {
		t.Fatalf("Compact ran concurrently %d time(s)", compactCalls)
	}

	interrupt, _ := json.Marshal(InterruptRequest{Subtype: "interrupt"})
	sendLine(t, inW, SDKControlRequest{Type: "control_request", RequestID: "interrupt-1", Request: interrupt})
	select {
	case sessionID := <-runtime.interrupts:
		if sessionID != "" {
			t.Fatalf("interrupt session = %q, want empty protocol value", sessionID)
		}
	case <-time.After(testTimeout):
		t.Fatal("Runtime.Interrupt was not called")
	}

	var resultCount, responseCount int
	for resultCount == 0 || responseCount == 0 {
		message := receiveWithTimeout(t, output, testTimeout)
		switch message["type"] {
		case "result":
			resultCount++
			if message["uuid"] != "active-query" || message["is_error"] != true {
				t.Fatalf("cancel result = %#v", message)
			}
		case "control_response":
			responseCount++
		default:
			t.Fatalf("unexpected message after interrupt: %#v", message)
		}
	}
	if resultCount != 1 || responseCount != 1 {
		t.Fatalf("interrupt outputs: result=%d response=%d", resultCount, responseCount)
	}
	if err := inW.Close(); err != nil {
		t.Fatal(err)
	}
	waitServe(t, done, nil)
}

func TestServeControlRequestIDExecutesOnceAndReplaysResponse(t *testing.T) {
	runtime := &concurrencyRuntime{}
	runtime.queryFn = func(context.Context, QueryRequest, PermissionHandler) (<-chan QueryEvent, error) {
		return nil, nil
	}
	inW, output, done := startSDKServe(t, runtime)
	compactOne, err := json.Marshal(CompactRequest{Subtype: "compact", SessionID: "session-1"})
	if err != nil {
		t.Fatal(err)
	}
	request := SDKControlRequest{Type: "control_request", RequestID: "compact-once", Request: compactOne}
	sendLine(t, inW, request)
	first := receiveWithTimeout(t, output, testTimeout)
	sendLine(t, inW, SDKControlRequest{
		Type: "control_request", RequestID: "compact-once",
		Request: json.RawMessage(`{"session_id":"session-1","subtype":"compact"}`),
	})
	second := receiveWithTimeout(t, output, testTimeout)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("replayed control response changed:\nfirst=%#v\nsecond=%#v", first, second)
	}
	responseBytes, err := json.Marshal(first["response"])
	if err != nil {
		t.Fatal(err)
	}
	var success ControlSuccess
	if err := json.Unmarshal(responseBytes, &success); err != nil {
		t.Fatal(err)
	}
	var compactResponse CompactResponse
	if err := json.Unmarshal(success.Response, &compactResponse); err != nil {
		t.Fatal(err)
	}
	if !compactResponse.Compacted || compactResponse.BeforeMessageCount != 40 ||
		compactResponse.AfterMessageCount != 8 || compactResponse.ContextGeneration != 3 {
		t.Fatalf("replayed compact response = %+v", compactResponse)
	}

	compactTwo, err := json.Marshal(CompactRequest{Subtype: "compact", SessionID: "session-2"})
	if err != nil {
		t.Fatal(err)
	}
	sendLine(t, inW, SDKControlRequest{
		Type: "control_request", RequestID: "compact-once", Request: compactTwo,
	})
	conflict := receiveWithTimeout(t, output, testTimeout)
	responseBytes, err = json.Marshal(conflict["response"])
	if err != nil {
		t.Fatal(err)
	}
	var conflictError ControlError
	if err := json.Unmarshal(responseBytes, &conflictError); err != nil {
		t.Fatal(err)
	}
	if conflictError.Subtype != "error" || conflictError.RequestID != "compact-once" || conflictError.Error == "" {
		t.Fatalf("conflicting control response = %#v", conflict)
	}
	runtime.mu.Lock()
	compactCalls := runtime.compactCalls
	runtime.mu.Unlock()
	if compactCalls != 1 {
		t.Fatalf("Compact calls = %d, want exactly 1", compactCalls)
	}
	sendLine(t, inW, SDKControlRequest{Type: "control_request", Request: compactOne})
	missingID := receiveWithTimeout(t, output, testTimeout)
	missingBytes, err := json.Marshal(missingID["response"])
	if err != nil {
		t.Fatal(err)
	}
	var missingError ControlError
	if err := json.Unmarshal(missingBytes, &missingError); err != nil {
		t.Fatal(err)
	}
	if missingError.Subtype != "error" || missingError.RequestID != "" || missingError.Error == "" {
		t.Fatalf("missing request_id response = %#v", missingID)
	}
	runtime.mu.Lock()
	compactCalls = runtime.compactCalls
	runtime.mu.Unlock()
	if compactCalls != 1 {
		t.Fatalf("missing request_id executed Compact; calls = %d", compactCalls)
	}
	if err := inW.Close(); err != nil {
		t.Fatal(err)
	}
	waitServe(t, done, nil)
}

func TestCompactCancellationCannotReturnTypedSuccess(t *testing.T) {
	started := make(chan struct{})
	runtime := &concurrencyRuntime{
		compactFn: func(ctx context.Context, _ string) (CompactResult, error) {
			close(started)
			<-ctx.Done()
			return CompactResult{}, ctx.Err()
		},
	}
	var output bytes.Buffer
	server := NewSDKServer(runtime, strings.NewReader(""), &output, InitialPermissionBridge)
	payload, err := json.Marshal(CompactRequest{Subtype: "compact", SessionID: "cancel-session"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- server.handleCompact(ctx, SDKControlRequest{
			Type: "control_request", RequestID: "compact-cancel", Request: payload,
		}, i18n.LangEN)
	}()
	select {
	case <-started:
	case <-time.After(testTimeout):
		t.Fatal("compact did not start")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(testTimeout):
		t.Fatal("cancelled compact did not return")
	}

	var response SDKControlResponse
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	var controlError ControlError
	if err := json.Unmarshal(response.Response, &controlError); err != nil {
		t.Fatal(err)
	}
	if controlError.Subtype != "error" || controlError.RequestID != "compact-cancel" {
		t.Fatalf("cancelled compact response = %+v", controlError)
	}
}

func TestServeEOFAndContextCancellationTerminateActiveQuery(t *testing.T) {
	t.Run("eof", func(t *testing.T) {
		queryStarted := make(chan struct{})
		queryCancelled := make(chan struct{})
		runtime := &concurrencyRuntime{}
		runtime.queryFn = func(ctx context.Context, _ QueryRequest, _ PermissionHandler) (<-chan QueryEvent, error) {
			close(queryStarted)
			go func() { <-ctx.Done(); close(queryCancelled) }()
			return make(chan QueryEvent), nil
		}
		inW, output, done := startSDKServe(t, runtime)
		sendLine(t, inW, SDKUserMessage{Type: "user", UUID: "eof-query", Message: textUserMessage("wait")})
		select {
		case <-queryStarted:
		case <-time.After(testTimeout):
			t.Fatal("query did not start")
		}
		if err := inW.Close(); err != nil {
			t.Fatal(err)
		}
		result := receiveWithTimeout(t, output, testTimeout)
		if result["type"] != "result" || result["uuid"] != "eof-query" || result["is_error"] != true {
			t.Fatalf("EOF terminal = %#v", result)
		}
		select {
		case <-queryCancelled:
		case <-time.After(testTimeout):
			t.Fatal("EOF did not cancel runtime query context")
		}
		waitServe(t, done, nil)
	})

	t.Run("context", func(t *testing.T) {
		queryStarted := make(chan struct{})
		runtime := &concurrencyRuntime{}
		runtime.queryFn = func(context.Context, QueryRequest, PermissionHandler) (<-chan QueryEvent, error) {
			close(queryStarted)
			return make(chan QueryEvent), nil
		}
		inR, inW := io.Pipe()
		outR, outW := io.Pipe()
		server := NewSDKServer(runtime, inR, outW, InitialPermissionBridge)
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() {
			err := server.Serve(ctx)
			_ = outW.Close()
			done <- err
		}()
		output := startScanner(outR)
		drainInit(t, output)
		sendLine(t, inW, SDKUserMessage{Type: "user", UUID: "cancel-query", Message: textUserMessage("wait")})
		select {
		case <-queryStarted:
		case <-time.After(testTimeout):
			t.Fatal("query did not start")
		}
		cancel()
		result := receiveWithTimeout(t, output, testTimeout)
		if result["type"] != "result" || result["uuid"] != "cancel-query" || result["is_error"] != true {
			t.Fatalf("cancel terminal = %#v", result)
		}
		waitServe(t, done, context.Canceled)
		_ = inW.Close()
	})
}

func TestMalformedControlSuccessFailsPermissionClosed(t *testing.T) {
	bridge := newPermissionBridge()
	requestSent := make(chan string, 1)
	handler := &sdkPermissionHandler{
		bridge:   bridge,
		newReqID: func() string { return "permission-malformed" },
		sendFn: func(message any) error {
			requestSent <- message.(SDKControlRequest).RequestID
			return nil
		},
	}
	decision := make(chan PermissionDecision, 1)
	errCh := make(chan error, 1)
	go func() {
		got, err := handler.Check(context.Background(), PermissionRequest{ToolName: "Write"})
		decision <- got
		errCh <- err
	}()
	requestID := <-requestSent

	var output strings.Builder
	server := NewSDKServer(&concurrencyRuntime{}, strings.NewReader(""), &output, InitialPermissionBridge)
	server.bridge = bridge
	malformedInner := json.RawMessage(`{}`)
	success, err := json.Marshal(ControlSuccess{
		Subtype: "success", RequestID: requestID, Response: malformedInner,
	})
	if err != nil {
		t.Fatal(err)
	}
	line, err := json.Marshal(SDKControlResponse{Type: "control_response", Response: success})
	if err != nil {
		t.Fatal(err)
	}
	if err := server.handleControlResponse(line, i18n.LangEN); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-decision:
		if got != PermissionDeny {
			t.Fatalf("decision = %v, want deny", got)
		}
	case <-time.After(testTimeout):
		t.Fatal("malformed ControlSuccess stranded permission waiter")
	}
	if err := <-errCh; err != nil {
		t.Fatalf("fail-closed denial returned error: %v", err)
	}
	if !strings.Contains(output.String(), `"subtype":"error"`) {
		t.Fatalf("protocol error was not surfaced: %s", output.String())
	}
	bridge.mu.Lock()
	pending := len(bridge.pending)
	bridge.mu.Unlock()
	if pending != 0 {
		t.Fatalf("permission waiters remaining = %d", pending)
	}
}

func TestPermissionBridgeCloseAndDuplicateRequestIDFailClosed(t *testing.T) {
	bridge := newPermissionBridge()
	requestSent := make(chan struct{}, 1)
	newHandler := func() *sdkPermissionHandler {
		return &sdkPermissionHandler{
			bridge: bridge, newReqID: func() string { return "duplicate" },
			sendFn: func(any) error { requestSent <- struct{}{}; return nil },
		}
	}
	firstErr := make(chan error, 1)
	go func() {
		_, err := newHandler().Check(context.Background(), PermissionRequest{ToolName: "Read"})
		firstErr <- err
	}()
	<-requestSent
	if decision, err := newHandler().Check(context.Background(), PermissionRequest{ToolName: "Write"}); decision != PermissionDeny || err == nil {
		t.Fatalf("duplicate Check = (%v, %v), want deny and error", decision, err)
	}
	bridge.close()
	select {
	case err := <-firstErr:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("close error = %v, want context.Canceled", err)
		}
	case <-time.After(testTimeout):
		t.Fatal("bridge close did not release waiter")
	}
}

func TestExtractTextRejectsUnsupportedOrMalformedContent(t *testing.T) {
	valid := map[string]string{
		`{"role":"user","content":"body"}`:                       "body",
		`{"role":"user","content":[{"type":"text","text":"a"}]}`: "a",
	}
	for raw, want := range valid {
		got, err := extractText(json.RawMessage(raw))
		if err != nil || got != want {
			t.Errorf("extractText(%s) = (%q, %v), want (%q, nil)", raw, got, err, want)
		}
	}
	for _, raw := range []string{
		"", `null`, `17`, `"plain"`, `{}`, `{"content":"missing role"}`,
		`{"role":"assistant","content":"wrong role"}`,
		`{"role":"user","content":{"text":"unsupported"}}`,
		`{"role":"user","content":[{"type":"image","text":"x"}]}`,
	} {
		if got, err := extractText(json.RawMessage(raw)); err == nil {
			t.Errorf("extractText(%s) = %q without error", raw, got)
		}
	}
}

func TestParseSDKUserQueryRequiresStableUUID(t *testing.T) {
	line, err := json.Marshal(SDKUserMessage{Type: "user", Message: textUserMessage("hello")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parseSDKUserQuery(line, i18n.LangEN); err == nil {
		t.Fatal("user message without uuid was accepted")
	}
}

type blockingSDKOutput struct {
	mu        sync.Mutex
	writes    chan []byte
	entered   chan struct{}
	release   chan struct{}
	enterOnce sync.Once
	count     int
	active    int
	maxActive int
}

func (w *blockingSDKOutput) Write(data []byte) (int, error) {
	w.mu.Lock()
	w.count++
	count := w.count
	w.active++
	if w.active > w.maxActive {
		w.maxActive = w.active
	}
	w.mu.Unlock()
	defer func() {
		w.mu.Lock()
		w.active--
		w.mu.Unlock()
	}()
	if count == 2 {
		w.enterOnce.Do(func() { close(w.entered) })
		<-w.release
	}
	copyOfData := append([]byte(nil), data...)
	w.writes <- copyOfData
	return len(data), nil
}

func TestServeProcessesInterruptBeforeBlockedOutputIsReleased(t *testing.T) {
	events := make(chan QueryEvent, 1)
	events <- QueryEvent{SessionID: "backpressure-session", Event: Event{Type: EventText, Text: "blocked"}}
	runtime := &concurrencyRuntime{interrupts: make(chan string, 1)}
	runtime.queryFn = func(context.Context, QueryRequest, PermissionHandler) (<-chan QueryEvent, error) {
		return events, nil
	}
	output := &blockingSDKOutput{
		writes: make(chan []byte, 8), entered: make(chan struct{}), release: make(chan struct{}),
	}
	inR, inW := io.Pipe()
	server := NewSDKServer(runtime, inR, output, InitialPermissionBridge)
	done := make(chan error, 1)
	go func() { done <- server.Serve(context.Background()) }()
	select {
	case <-output.writes: // init
	case <-time.After(testTimeout):
		t.Fatal("init was not written")
	}
	sendLine(t, inW, SDKUserMessage{
		Type: "user", SessionID: "backpressure-session", UUID: "backpressure-query", Message: textUserMessage("wait"),
	})
	select {
	case <-output.entered:
	case <-time.After(testTimeout):
		t.Fatal("query output did not block")
	}
	interrupt, _ := json.Marshal(InterruptRequest{Subtype: "interrupt", SessionID: "backpressure-session"})
	sendLine(t, inW, SDKControlRequest{Type: "control_request", RequestID: "interrupt-backpressure", Request: interrupt})
	select {
	case sessionID := <-runtime.interrupts:
		if sessionID != "backpressure-session" {
			t.Fatalf("interrupt session = %q", sessionID)
		}
	case <-time.After(testTimeout):
		t.Fatal("blocked stdout prevented interrupt processing")
	}
	close(output.release)

	seenControl, seenResult := false, false
	for !seenControl || !seenResult {
		select {
		case line := <-output.writes:
			if !strings.HasSuffix(string(line), "\n") || strings.Count(string(line), "\n") != 1 {
				t.Fatalf("stdout write was not one NDJSON line: %q", line)
			}
			var message map[string]any
			if err := json.Unmarshal(line, &message); err != nil {
				t.Fatalf("output is not atomic NDJSON: %q: %v", line, err)
			}
			switch message["type"] {
			case "control_response":
				seenControl = true
			case "result":
				seenResult = true
			}
		case <-time.After(testTimeout):
			t.Fatal("timed out waiting for output after releasing backpressure")
		}
	}
	if err := inW.Close(); err != nil {
		t.Fatal(err)
	}
	waitServe(t, done, nil)
	output.mu.Lock()
	maxActive := output.maxActive
	output.mu.Unlock()
	if maxActive != 1 {
		t.Fatalf("concurrent stdout writes = %d, want 1", maxActive)
	}
}

type failNthSDKOutput struct {
	mu      sync.Mutex
	count   int
	failAt  int
	failure error
}

func (w *failNthSDKOutput) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.count++
	if w.count == w.failAt {
		return 0, w.failure
	}
	return len(data), nil
}

func TestServePropagatesInitAndSystemWriteErrorsAndIsOneShot(t *testing.T) {
	t.Run("init", func(t *testing.T) {
		cause := errors.New("init output failed")
		server := NewSDKServer(&concurrencyRuntime{}, strings.NewReader(""), &failNthSDKOutput{failAt: 1, failure: cause}, InitialPermissionBridge)
		if err := server.Serve(context.Background()); !errors.Is(err, cause) {
			t.Fatalf("Serve error = %v, want init write cause", err)
		}
	})

	t.Run("dispatch system error", func(t *testing.T) {
		cause := errors.New("system output failed")
		server := NewSDKServer(&concurrencyRuntime{}, strings.NewReader("not-json\n"), &failNthSDKOutput{failAt: 2, failure: cause}, InitialPermissionBridge)
		if err := server.Serve(context.Background()); !errors.Is(err, cause) {
			t.Fatalf("Serve error = %v, want system write cause", err)
		}
	})

	t.Run("one shot", func(t *testing.T) {
		server := NewSDKServer(&concurrencyRuntime{}, strings.NewReader(""), io.Discard, InitialPermissionBridge)
		if err := server.Serve(context.Background()); err != nil {
			t.Fatal(err)
		}
		if err := server.Serve(context.Background()); err == nil {
			t.Fatal("second Serve unexpectedly succeeded")
		}
	})
}

func TestSDKMessageWriterRejectsSubmitAfterClose(t *testing.T) {
	writer := newSDKMessageWriter(io.Discard, nil)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := writer.submit([]byte("{}\n")); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("submit after Close = %v, want io.ErrClosedPipe", err)
	}
}
