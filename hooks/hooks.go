package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/agent-dance/luban/i18n"
)

// HookType represents when a hook fires
type HookType string

const (
	HookPreToolUse         HookType = "PreToolUse"
	HookPostToolUse        HookType = "PostToolUse"
	HookPostToolUseFailure HookType = "PostToolUseFailure"
	HookSessionStart       HookType = "SessionStart"
	HookSessionEnd         HookType = "SessionEnd"
	HookUserPromptSubmit   HookType = "UserPromptSubmit"
	HookStop               HookType = "Stop"         // model finishes generating
	HookPreQuery           HookType = "PreQuery"     // before sending to model
	HookPostQuery          HookType = "PostQuery"    // after model responds
	HookPostSampling       HookType = "PostSampling" // after model sampling, before tool execution
	HookStopFailure        HookType = "StopFailure"  // unrecoverable query failure before loop exits
	HookNotification       HookType = "Notification" // user-facing event notifications
	HookPreCompact         HookType = "PreCompact"
	HookPostCompact        HookType = "PostCompact"
	HookSubagentStart      HookType = "SubagentStart"
	HookSubagentStop       HookType = "SubagentStop"
	HookTeammateIdle       HookType = "TeammateIdle"
	HookTaskCreated        HookType = "TaskCreated"
	HookTaskCompleted      HookType = "TaskCompleted"
)

// HookKind discriminates the execution strategy for a hook.
type HookKind string

const (
	HookKindCommand      HookKind = "command"      // run a shell command (default)
	HookKindHTTP         HookKind = "http"         // POST JSON to a URL
	HookKindNotification HookKind = "notification" // send a system notification
)

// Hook defines a single hook execution
type Hook struct {
	Type HookType `json:"type"`
	Kind HookKind `json:"kind,omitempty"` // defaults to "command" if empty

	// command hook fields
	Command string `json:"command,omitempty"`
	Timeout int    `json:"timeout,omitempty"` // seconds, default 10

	// http hook fields
	URL        string            `json:"url,omitempty"`
	Headers    map[string]string `json:"headers,omitempty"`
	RetryCount int               `json:"retry_count,omitempty"` // number of attempts (default 1)

	// matcher (used by config loading to filter by tool name)
	Matcher string `json:"matcher,omitempty"`
}

// HookInput is the data passed to a hook via stdin (command) or POST body (http)
type HookInput struct {
	Type                 HookType       `json:"type"`
	HookEventName        HookType       `json:"hook_event_name,omitempty"`
	ToolName             string         `json:"tool_name,omitempty"`
	ToolUseID            string         `json:"tool_use_id,omitempty"`
	SessionID            string         `json:"session_id,omitempty"`
	ProjectRoot          string         `json:"project_root,omitempty"`
	TurnID               string         `json:"turn_id,omitempty"`
	WorkUnitID           string         `json:"work_unit_id,omitempty"`
	HookConfigID         string         `json:"hook_config_id,omitempty"`
	HookExecutionID      string         `json:"hook_execution_id,omitempty"`
	ToolInput            map[string]any `json:"tool_input,omitempty"`
	Result               string         `json:"result,omitempty"`
	Messages             []any          `json:"messages,omitempty"`
	UserInput            string         `json:"user_input,omitempty"`
	AgentID              string         `json:"agent_id,omitempty"`
	AgentType            string         `json:"agent_type,omitempty"`
	AgentTranscriptPath  string         `json:"agent_transcript_path,omitempty"`
	LastAssistantMessage string         `json:"last_assistant_message,omitempty"`
	StopHookActive       bool           `json:"stop_hook_active,omitempty"`
	TeammateName         string         `json:"teammate_name,omitempty"`
	TeamName             string         `json:"team_name,omitempty"`
	TaskID               string         `json:"task_id,omitempty"`
	TaskSubject          string         `json:"task_subject,omitempty"`
	TaskDescription      string         `json:"task_description,omitempty"`
	TaskOwner            string         `json:"task_owner,omitempty"`
	Owner                string         `json:"owner,omitempty"`
	Trigger              string         `json:"trigger,omitempty"`
	CustomInstructions   *string        `json:"custom_instructions,omitempty"`
	CompactSummary       string         `json:"compact_summary,omitempty"`
	// Notification-specific fields
	Message string `json:"message,omitempty"`
	Title   string `json:"title,omitempty"`
}

// HookOutput is the result from a hook
type HookOutput struct {
	// If non-empty, injected as system-reminder into conversation
	SystemReminder string
	// If true, block the tool execution (PreToolUse only)
	Block bool
	// Modified tool input (PreToolUse only)
	ModifiedInput map[string]any
	// If true, stop the loop after this hook's tool result is recorded.
	PreventContinuation bool
	// Optional human-readable reason for PreventContinuation.
	StopReason string
	// Additional context to inject as a system-reminder.
	AdditionalContext  string
	AdditionalContexts []string
	// PreToolUse permission behavior: allow, deny/block, ask, or passthrough.
	PermissionBehavior string
	// Compact hook fields.
	NewCustomInstructions string
	UserDisplayMessage    string
	// Exit code
	ExitCode int
	// ExecutionError preserves a process/transport failure separately from the
	// raw stderr stream so neither source of evidence overwrites the other.
	ExecutionError string
	// Stdout preserves the exact captured command/HTTP response bytes. Output is
	// bounded; the observed byte count and truncation flag make any omitted tail
	// explicit. For a truncated HTTP stream, the observed count is limit+1.
	Stdout          string
	StdoutBytes     int64
	StdoutTruncated bool
	// Stderr captures error output from the hook process
	Stderr          string
	StderrBytes     int64
	StderrTruncated bool
}

// HookExecution preserves the identity and configuration of one concrete hook
// invocation. RunDetailed is the evidence-producing counterpart to Run; Run
// remains available for callers that only need aggregate control-flow output.
type HookExecution struct {
	ExecutionID string
	ConfigID    string
	ConfigIndex int
	Hook        Hook
	Input       HookInput
	Output      HookOutput
}

// Snapshot returns an ownership-independent copy suitable for durable
// evidence. Hook payloads are JSON-shaped, so maps, slices, interfaces, and
// pointers must not remain shared with control-flow consumers.
func (e HookExecution) Snapshot() HookExecution {
	e.Hook = e.Hook.evidenceSnapshot()
	e.Input = e.Input.Snapshot()
	e.Output = e.Output.Snapshot()
	return e
}

// Snapshot returns an ownership-independent hook configuration.
func (h Hook) Snapshot() Hook {
	h.Headers = cloneStringMap(h.Headers)
	return h
}

const redactedHookHeaderValue = "[REDACTED]"

// evidenceSnapshot preserves configuration identity while preventing default
// evidence consumers from receiving credentials embedded in HTTP headers.
func (h Hook) evidenceSnapshot() Hook {
	h = h.Snapshot()
	for name := range h.Headers {
		if sensitiveHookHeader(name) {
			h.Headers[name] = redactedHookHeaderValue
		}
	}
	return h
}

func sensitiveHookHeader(name string) bool {
	normalized := strings.ToLower(strings.TrimSpace(name))
	switch normalized {
	case "authorization", "proxy-authorization", "cookie", "set-cookie", "api-key", "x-api-key":
		return true
	}
	return strings.Contains(normalized, "api-key") ||
		strings.Contains(normalized, "token") ||
		strings.Contains(normalized, "secret") ||
		strings.Contains(normalized, "password")
}

// Snapshot returns an ownership-independent hook input.
func (in HookInput) Snapshot() HookInput {
	in.ToolInput = cloneStringAnyMap(in.ToolInput)
	if in.Messages != nil {
		in.Messages = cloneMutableValue(reflect.ValueOf(in.Messages)).Interface().([]any)
	}
	if in.CustomInstructions != nil {
		value := *in.CustomInstructions
		in.CustomInstructions = &value
	}
	return in
}

// Snapshot returns an ownership-independent hook output.
func (out HookOutput) Snapshot() HookOutput {
	out.ModifiedInput = cloneStringAnyMap(out.ModifiedInput)
	out.AdditionalContexts = append([]string(nil), out.AdditionalContexts...)
	return out
}

func cloneHooks(source []Hook) []Hook {
	if source == nil {
		return nil
	}
	out := make([]Hook, len(source))
	for i := range source {
		out[i] = source[i].Snapshot()
	}
	return out
}

func cloneStringMap(source map[string]string) map[string]string {
	if source == nil {
		return nil
	}
	out := make(map[string]string, len(source))
	for key, value := range source {
		out[key] = value
	}
	return out
}

func cloneStringAnyMap(source map[string]any) map[string]any {
	if source == nil {
		return nil
	}
	return cloneMutableValue(reflect.ValueOf(source)).Interface().(map[string]any)
}

// cloneMutableValue preserves concrete JSON-compatible Go types while
// recursively detaching mutable containers. Hook inputs are required to be
// JSON-serializable, so cyclic maps/pointers are outside the contract.
func cloneMutableValue(value reflect.Value) reflect.Value {
	if !value.IsValid() {
		return value
	}
	switch value.Kind() {
	case reflect.Interface:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		cloned := cloneMutableValue(value.Elem())
		out := reflect.New(value.Type()).Elem()
		out.Set(cloned)
		return out
	case reflect.Map:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		out := reflect.MakeMapWithSize(value.Type(), value.Len())
		iterator := value.MapRange()
		for iterator.Next() {
			out.SetMapIndex(cloneMutableValue(iterator.Key()), cloneMutableValue(iterator.Value()))
		}
		return out
	case reflect.Slice:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		out := reflect.MakeSlice(value.Type(), value.Len(), value.Len())
		for i := 0; i < value.Len(); i++ {
			out.Index(i).Set(cloneMutableValue(value.Index(i)))
		}
		return out
	case reflect.Array:
		out := reflect.New(value.Type()).Elem()
		for i := 0; i < value.Len(); i++ {
			out.Index(i).Set(cloneMutableValue(value.Index(i)))
		}
		return out
	case reflect.Pointer:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		out := reflect.New(value.Type().Elem())
		out.Elem().Set(cloneMutableValue(value.Elem()))
		return out
	case reflect.Struct:
		out := reflect.New(value.Type()).Elem()
		out.Set(value)
		for i := 0; i < value.NumField(); i++ {
			if out.Field(i).CanSet() && value.Field(i).CanInterface() {
				out.Field(i).Set(cloneMutableValue(value.Field(i)))
			}
		}
		return out
	default:
		return value
	}
}

// Runner manages and executes hooks.
// D2: mu guards hooks to allow concurrent Run calls alongside Merge.
type Runner struct {
	mu              sync.RWMutex
	hooks           []Hook
	executionCounts map[string]uint64
}

// NewRunner creates a hook runner from a list of hooks
func NewRunner(hooks []Hook) *Runner {
	return &Runner{hooks: cloneHooks(hooks)}
}

// LoadFromSettings loads hooks from a settings.json file
func LoadFromSettings(settingsPath string) (*Runner, error) {
	return LoadConfig(settingsPath)
}

// safeFilenameRe matches hook script filenames that are safe to execute
// directly. Characters outside this set (semicolons, pipes, backticks, spaces,
// etc.) could be used for shell injection when the path is passed to bash -c.
// C4: Reject filenames that don't match this pattern in LoadFromDir.
var safeFilenameRe = regexp.MustCompile(`^[a-zA-Z0-9._-]+\.sh$`)

// hookTypeFromFilename infers hook type from a filename convention.
// Expected format: "pre-tool-use-*.sh", "post-tool-use-*.sh",
// "session-start-*.sh", "session-end-*.sh", "user-prompt-*.sh",
// "stop-*.sh", "notification-*.sh"
// Falls back to empty string if no pattern matches.
func hookTypeFromFilename(name string) HookType {
	lower := strings.ToLower(name)
	switch {
	case strings.HasPrefix(lower, "pre-tool-use") || strings.HasPrefix(lower, "pretooluse"):
		return HookPreToolUse
	case strings.HasPrefix(lower, "post-tool-use-failure") || strings.HasPrefix(lower, "posttoolusefailure"):
		return HookPostToolUseFailure
	case strings.HasPrefix(lower, "post-tool-use") || strings.HasPrefix(lower, "posttooluse"):
		return HookPostToolUse
	case strings.HasPrefix(lower, "session-start") || strings.HasPrefix(lower, "sessionstart"):
		return HookSessionStart
	case strings.HasPrefix(lower, "session-end") || strings.HasPrefix(lower, "sessionend"):
		return HookSessionEnd
	case strings.HasPrefix(lower, "user-prompt") || strings.HasPrefix(lower, "userprompt"):
		return HookUserPromptSubmit
	case strings.HasPrefix(lower, "stop"):
		return HookStop
	case strings.HasPrefix(lower, "pre-query") || strings.HasPrefix(lower, "prequery"):
		return HookPreQuery
	case strings.HasPrefix(lower, "post-query") || strings.HasPrefix(lower, "postquery"):
		return HookPostQuery
	case strings.HasPrefix(lower, "post-sampling") || strings.HasPrefix(lower, "postsampling"):
		return HookPostSampling
	case strings.HasPrefix(lower, "stop-failure") || strings.HasPrefix(lower, "stopfailure"):
		return HookStopFailure
	case strings.HasPrefix(lower, "notification"):
		return HookNotification
	case strings.HasPrefix(lower, "pre-compact") || strings.HasPrefix(lower, "precompact"):
		return HookPreCompact
	case strings.HasPrefix(lower, "post-compact") || strings.HasPrefix(lower, "postcompact"):
		return HookPostCompact
	case strings.HasPrefix(lower, "subagent-start") || strings.HasPrefix(lower, "subagentstart"):
		return HookSubagentStart
	case strings.HasPrefix(lower, "subagent-stop") || strings.HasPrefix(lower, "subagentstop"):
		return HookSubagentStop
	case strings.HasPrefix(lower, "teammate-idle") || strings.HasPrefix(lower, "teammateidle"):
		return HookTeammateIdle
	case strings.HasPrefix(lower, "task-created") || strings.HasPrefix(lower, "taskcreated"):
		return HookTaskCreated
	case strings.HasPrefix(lower, "task-completed") || strings.HasPrefix(lower, "taskcompleted"):
		return HookTaskCompleted
	default:
		return ""
	}
}

// LoadFromDir loads hooks from shell scripts in a directory.
// Hook type is inferred from the filename prefix (e.g. "pre-tool-use-check.sh").
func LoadFromDir(dir string) (*Runner, error) {
	var hooks []Hook
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return &Runner{}, nil
		}
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".sh" {
			continue
		}
		// C4: Reject filenames with shell metacharacters to prevent command injection.
		if !safeFilenameRe.MatchString(entry.Name()) {
			continue
		}
		hookType := hookTypeFromFilename(strings.TrimSuffix(entry.Name(), ".sh"))
		if hookType == "" {
			continue // skip files that don't match any convention
		}
		hooks = append(hooks, Hook{
			Type:    hookType,
			Command: filepath.Join(dir, entry.Name()),
			Timeout: 10,
		})
	}

	return NewRunner(hooks), nil
}

// Run executes all hooks of the given type.
// The provided context is used as the parent for hook timeouts,
// ensuring hooks are cancelled when the parent context is cancelled.
func (r *Runner) Run(ctx context.Context, hookType HookType, input HookInput) []HookOutput {
	executions := r.RunDetailed(ctx, hookType, input)
	outputs := make([]HookOutput, 0, len(executions))
	for _, execution := range executions {
		outputs = append(outputs, execution.Output.Snapshot())
	}
	return outputs
}

// RunDetailed executes all matching hooks and returns one identity-bearing
// record per actual configuration execution. ConfigIndex is one-based and
// refers to the hook's stable position in this Runner.
func (r *Runner) RunDetailed(ctx context.Context, hookType HookType, input HookInput) []HookExecution {
	// D2: Read-lock while iterating so concurrent Merge calls don't race.
	r.mu.RLock()
	hooksCopy := cloneHooks(r.hooks)
	r.mu.RUnlock()

	var executions []HookExecution
	input = input.Snapshot()
	input.Type = hookType
	input.HookEventName = hookType

	for index, hook := range hooksCopy {
		if hook.Type != hookType {
			continue
		}
		matchQuery := input.ToolName
		if hookType == HookSubagentStart || hookType == HookSubagentStop {
			matchQuery = input.AgentType
		}
		// C1: If a matcher is set, only fire when the event's match field matches.
		if hook.Matcher != "" && hook.Matcher != matchQuery {
			continue
		}

		configID := fmt.Sprintf("config-%d", index+1)
		executionInput := input.Snapshot()
		executionInput.HookConfigID = configID
		executionInput.HookExecutionID = r.uniqueHookExecutionID(hookExecutionID(hookType, executionInput, configID))
		output := r.executeHook(ctx, hook, executionInput)
		executions = append(executions, HookExecution{
			ExecutionID: executionInput.HookExecutionID,
			ConfigID:    configID,
			ConfigIndex: index + 1,
			Hook:        hook.evidenceSnapshot(),
			Input:       executionInput,
			Output:      output.Snapshot(),
		})
	}

	return executions
}

// uniqueHookExecutionID preserves the historical first-occurrence ID while
// disambiguating retries/repeated lifecycle invocations that share the same
// causal fields and configuration. ConfigID remains stable across executions.
func (r *Runner) uniqueHookExecutionID(base string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.executionCounts == nil {
		r.executionCounts = make(map[string]uint64)
	}
	r.executionCounts[base]++
	if r.executionCounts[base] == 1 {
		return base
	}
	return fmt.Sprintf("%s:occurrence-%d", base, r.executionCounts[base])
}

func hookExecutionID(hookType HookType, input HookInput, configID string) string {
	scope := strings.TrimSpace(input.TurnID)
	if scope == "" {
		scope = strings.TrimSpace(input.SessionID)
	}
	if scope == "" {
		scope = strings.TrimSpace(input.WorkUnitID)
	}
	if scope == "" {
		scope = strings.TrimSpace(input.AgentID)
	}
	if scope == "" {
		scope = "unscoped"
	}
	id := "hook:" + scope + ":" + string(hookType)
	if toolUseID := strings.TrimSpace(input.ToolUseID); toolUseID != "" {
		id += ":tool-" + toolUseID
	}
	if taskID := strings.TrimSpace(input.TaskID); taskID != "" {
		id += ":task-" + taskID
	}
	return id + ":" + configID
}

// BlockingError reports a hook refusal from a lifecycle event whose TS
// semantics allow hooks to veto the state transition.
type BlockingError struct {
	HookType HookType
	Reason   string
	Output   HookOutput
}

func (e *BlockingError) Error() string {
	if e == nil {
		return i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyHookBlockedDefault)
	}
	reason := strings.TrimSpace(e.Reason)
	if reason == "" {
		reason = i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyHookBlockedDefault)
	}
	return i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyHookLifecycleBlocked, e.HookType, reason)
}

// RunBlocking executes a hook event and turns the first blocking output into
// a typed error. Cancellation wins over a hook output so callers can reliably
// distinguish an aborted transition from a policy refusal.
func (r *Runner) RunBlocking(ctx context.Context, hookType HookType, input HookInput) ([]HookOutput, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r == nil {
		return nil, nil
	}
	outputs := r.Run(ctx, hookType, input)
	if err := ctx.Err(); err != nil {
		return outputs, err
	}
	for _, output := range outputs {
		if !output.Block {
			continue
		}
		reason := firstHookReason(output)
		return outputs, &BlockingError{HookType: hookType, Reason: reason, Output: output}
	}
	return outputs, nil
}

// RunBlockingTransition applies a state change and then runs a blocking
// lifecycle hook. Any apply failure, hook refusal, or cancellation rolls the
// change back before the error is returned, preventing partially persisted
// tool/task/worktree state.
func (r *Runner) RunBlockingTransition(
	ctx context.Context,
	hookType HookType,
	input HookInput,
	apply func() error,
	rollback func() error,
) error {
	if apply == nil {
		return fmt.Errorf("%s", i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyHookLifecycleApplyMissing, hookType))
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := apply(); err != nil {
		return joinRollbackError(err, rollback)
	}
	if err := ctx.Err(); err != nil {
		return joinRollbackError(err, rollback)
	}
	if _, err := r.RunBlocking(ctx, hookType, input); err != nil {
		return joinRollbackError(err, rollback)
	}
	return nil
}

func firstHookReason(output HookOutput) string {
	for _, value := range []string{
		output.SystemReminder,
		output.StopReason,
		output.Stderr,
	} {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyHookBlockedDefault)
}

func joinRollbackError(cause error, rollback func() error) error {
	if rollback == nil {
		return cause
	}
	if err := rollback(); err != nil {
		return errors.Join(cause, fmt.Errorf("%s: %w", i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyHookLifecycleRollback), err))
	}
	return cause
}

// WithHookTypeMapped returns a new Runner with hooks of one event type mapped
// to another event type. Agent frontmatter uses this to mirror TS behavior:
// Stop hooks declared on an agent fire as SubagentStop hooks.
func (r *Runner) WithHookTypeMapped(from, to HookType) *Runner {
	if r == nil {
		return &Runner{}
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	mapped := cloneHooks(r.hooks)
	for i := range mapped {
		if mapped[i].Type == from {
			mapped[i].Type = to
		}
	}
	return &Runner{hooks: mapped}
}

// Merge returns a new Runner combining hooks from r and other.
// A nil receiver or nil other is handled gracefully.
// D2: Both runners are read-locked while copying to prevent data races.
func (r *Runner) Merge(other *Runner) *Runner {
	if r == nil && other == nil {
		return &Runner{}
	}
	if r == nil {
		other.mu.RLock()
		defer other.mu.RUnlock()
		return NewRunner(other.hooks)
	}
	if other == nil {
		r.mu.RLock()
		defer r.mu.RUnlock()
		return NewRunner(r.hooks)
	}
	r.mu.RLock()
	rHooks := cloneHooks(r.hooks)
	r.mu.RUnlock()

	other.mu.RLock()
	oHooks := cloneHooks(other.hooks)
	other.mu.RUnlock()

	combined := make([]Hook, 0, len(rHooks)+len(oHooks))
	combined = append(combined, rHooks...)
	combined = append(combined, oHooks...)
	return NewRunner(combined)
}

// HasHooks returns true if any hooks of the given type are registered.
// D2: Read-locked to be safe against concurrent Merge calls.
func (r *Runner) HasHooks(hookType HookType) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, h := range r.hooks {
		if h.Type == hookType {
			return true
		}
	}
	return false
}

// limitedBuffer caps how much output a hook can produce to prevent OOM.
type limitedBuffer struct {
	buf       bytes.Buffer
	max       int
	total     int64
	truncated bool
}

func (lb *limitedBuffer) Write(p []byte) (int, error) {
	lb.total += int64(len(p))
	if lb.buf.Len()+len(p) > lb.max {
		lb.truncated = true
		remaining := lb.max - lb.buf.Len()
		if remaining > 0 {
			lb.buf.Write(p[:remaining])
		}
		return len(p), nil // pretend success to not kill the process
	}
	return lb.buf.Write(p)
}

func (lb *limitedBuffer) Bytes() []byte  { return lb.buf.Bytes() }
func (lb *limitedBuffer) String() string { return lb.buf.String() }
func (lb *limitedBuffer) Total() int64   { return lb.total }
func (lb *limitedBuffer) Truncated() bool {
	return lb.truncated
}

const hookOutputLimit = 1 << 20 // 1 MB

func (r *Runner) executeHook(parentCtx context.Context, hook Hook, input HookInput) HookOutput {
	// D1: Explicit case for every known kind; unknown kinds return an error
	// with Block=true so callers are never silently routed to the wrong handler.
	switch hook.Kind {
	case HookKindHTTP:
		return executeHTTPHook(parentCtx, hook, input)
	case HookKindNotification:
		return executeNotificationHook(parentCtx, hook, input)
	case HookKindCommand, "":
		return executeCommandHook(parentCtx, hook, input)
	default:
		return hookErrorOutput(i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyHookConfigKindUnknown, hook.Kind), true)
	}
}

func hookErrorOutput(message string, block bool) HookOutput {
	return HookOutput{
		ExitCode:       -1,
		ExecutionError: message,
		Stderr:         message,
		StderrBytes:    int64(len(message)),
		Block:          block,
	}
}

func executeCommandHook(parentCtx context.Context, hook Hook, input HookInput) HookOutput {
	timeout := time.Duration(hook.Timeout) * time.Second
	// H30: Treat negative timeouts the same as zero (use default).
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	ctx, cancel := context.WithTimeout(parentCtx, timeout)
	defer cancel()

	inputJSON, _ := json.Marshal(input)

	cmd := shellCommandContext(ctx, hook.Command)
	cmd.Stdin = bytes.NewReader(inputJSON)
	// C11: Use limited buffers to cap hook output at 1 MB.
	stdout := &limitedBuffer{max: hookOutputLimit}
	stderr := &limitedBuffer{max: hookOutputLimit}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	err := cmd.Run()

	output := HookOutput{}
	if err != nil {
		output.ExecutionError = err.Error()
		if exitErr, ok := err.(*exec.ExitError); ok {
			output.ExitCode = exitErr.ExitCode()
			// Exit code 2 = intentional block; other non-zero = hook error (passthrough)
			output.Block = output.ExitCode == 2
		} else {
			// H32: Non-ExitError (e.g. binary not found, permission denied)
			// must be treated as failure, not silently allowed.
			output.ExitCode = -1
			output.Block = true
		}
	}
	output.Stdout = stdout.String()
	output.StdoutBytes = stdout.Total()
	output.StdoutTruncated = stdout.Truncated()
	output.Stderr = stderr.String()
	output.StderrBytes = stderr.Total()
	output.StderrTruncated = stderr.Truncated()
	if output.Stderr == "" && output.ExecutionError != "" {
		// Preserve the historical Stderr-facing error contract for callers while
		// retaining the distinct execution error above for exact evidence.
		output.Stderr = output.ExecutionError
		output.StderrBytes = int64(len(output.Stderr))
	}

	// Try to parse stdout as JSON for structured output
	var structured struct {
		SystemReminder      string         `json:"system_reminder"`
		Block               bool           `json:"block"`
		ModifiedInput       map[string]any `json:"modified_input"`
		UpdatedInput        map[string]any `json:"updated_input"`
		UpdatedInputCamel   map[string]any `json:"updatedInput"`
		PreventContinuation bool           `json:"prevent_continuation"`
		PreventContCamel    bool           `json:"preventContinuation"`
		StopReason          string         `json:"stop_reason"`
		StopReasonCamel     string         `json:"stopReason"`
		AdditionalContext   string         `json:"additional_context"`
		AdditionalContexts  []string       `json:"additional_contexts"`
		AdditionalCtxCamel  []string       `json:"additionalContexts"`
		PermissionBehavior  string         `json:"permission_behavior"`
		PermissionCamel     string         `json:"permissionBehavior"`
		NewCustomInstr      string         `json:"new_custom_instructions"`
		NewCustomInstrCamel string         `json:"newCustomInstructions"`
		UserDisplayMessage  string         `json:"user_display_message"`
		UserDisplayCamel    string         `json:"userDisplayMessage"`
	}
	if json.Unmarshal(stdout.Bytes(), &structured) == nil {
		output.SystemReminder = structured.SystemReminder
		if structured.Block {
			output.Block = true
		}
		output.ModifiedInput = structured.ModifiedInput
		if output.ModifiedInput == nil {
			output.ModifiedInput = structured.UpdatedInput
		}
		if output.ModifiedInput == nil {
			output.ModifiedInput = structured.UpdatedInputCamel
		}
		output.PreventContinuation = structured.PreventContinuation
		if structured.PreventContCamel {
			output.PreventContinuation = true
		}
		output.StopReason = structured.StopReason
		if output.StopReason == "" {
			output.StopReason = structured.StopReasonCamel
		}
		output.AdditionalContext = structured.AdditionalContext
		output.AdditionalContexts = structured.AdditionalContexts
		if len(output.AdditionalContexts) == 0 {
			output.AdditionalContexts = structured.AdditionalCtxCamel
		}
		output.PermissionBehavior = structured.PermissionBehavior
		if output.PermissionBehavior == "" {
			output.PermissionBehavior = structured.PermissionCamel
		}
		output.NewCustomInstructions = structured.NewCustomInstr
		if output.NewCustomInstructions == "" {
			output.NewCustomInstructions = structured.NewCustomInstrCamel
		}
		output.UserDisplayMessage = structured.UserDisplayMessage
		if output.UserDisplayMessage == "" {
			output.UserDisplayMessage = structured.UserDisplayCamel
		}
	} else {
		// Plain text output becomes system reminder
		text := stdout.String()
		if text != "" {
			output.SystemReminder = text
		}
	}

	return output
}

func shellCommandContext(ctx context.Context, command string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", command)
	}
	return exec.CommandContext(ctx, "bash", "-c", command)
}
