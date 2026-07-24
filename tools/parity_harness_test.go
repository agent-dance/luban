package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/agent-dance/luban/internal/approvalcommit"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

type toolParityFixture struct {
	Name                          string              `json:"name"`
	Tool                          string              `json:"tool"`
	Category                      string              `json:"category"`
	TSReference                   string              `json:"ts_reference"`
	TSBehavior                    string              `json:"ts_behavior"`
	Runtime                       parityRuntime       `json:"runtime"`
	Setup                         paritySetup         `json:"setup"`
	Input                         map[string]any      `json:"input"`
	ExpectedEnabled               *bool               `json:"expected_enabled"`
	Permission                    *parityPermission   `json:"permission"`
	SkipExecution                 bool                `json:"skip_execution"`
	ExecuteWithApprovalCommit     bool                `json:"execute_with_permission_prepared"`
	ExpectedValidationError       string              `json:"expected_validation_error"`
	ExpectedModelText             *string             `json:"expected_model_text"`
	ExpectedModelTextContains     []string            `json:"expected_model_text_contains"`
	ExpectedBlockTextContains     []string            `json:"expected_content_block_text_contains"`
	ExpectedTypedDataJSON         json.RawMessage     `json:"expected_typed_data_json"`
	ExpectedTypedDataJSONContains []string            `json:"expected_typed_data_json_contains"`
	ExpectedContentJSON           map[string]any      `json:"expected_content_json"`
	ExpectedState                 parityExpectedState `json:"expected_state"`
}

type parityRuntime struct {
	Interactive *bool `json:"interactive"`
}

type paritySetup struct {
	Files        []parityFile        `json:"files"`
	Todos        []TodoItem          `json:"todos"`
	HTTPResponse *parityHTTPResponse `json:"http_response"`
}

type parityFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type parityHTTPResponse struct {
	ContentType string `json:"content_type"`
	Body        string `json:"body"`
}

type parityPermission struct {
	Behavior                string   `json:"behavior"`
	MessageContains         []string `json:"message_contains"`
	SuggestionsJSONContains []string `json:"suggestions_json_contains"`
}

type parityExpectedState struct {
	Files              []parityFile `json:"files"`
	ReadStatePaths     []string     `json:"read_state_paths"`
	TodoCount          *int         `json:"todo_count"`
	WebCacheEntriesMin *int         `json:"web_cache_entries_min"`
}

type parityHarness struct {
	workspace string
	readState *ReadFileState
	todoStore *TodoStore
	webCache  *searchCache
	registry  *registry.Registry
	serverURL string
	cleanup   []func()
}

func TestToolGoldenParityFixtures(t *testing.T) {
	fixtures := loadToolParityFixtures(t)
	seen := map[string]bool{}
	for _, fixture := range fixtures {
		fixture := fixture
		if fixture.Category != "" {
			seen[fixture.Category] = true
		}
		t.Run(fixture.Name, func(t *testing.T) {
			h := newParityHarness(t, fixture)
			defer h.close()
			h.runFixture(t, fixture)
		})
	}

	for _, category := range []string{"read-only", "mutating", "stateful", "web"} {
		if !seen[category] {
			t.Fatalf("golden parity fixtures did not cover category %q", category)
		}
	}
}

func loadToolParityFixtures(t *testing.T) []toolParityFixture {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("testdata", "parity", "*.json"))
	if err != nil {
		t.Fatalf("glob parity fixtures: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no tool parity fixtures found")
	}
	fixtures := make([]toolParityFixture, 0, len(paths))
	names := make(map[string]string, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		var fixture toolParityFixture
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&fixture); err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		if err := ensureJSONEOF(decoder); err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		if fixture.Name == "" || fixture.Tool == "" || fixture.Category == "" || fixture.TSReference == "" || fixture.TSBehavior == "" {
			t.Fatalf("%s must include name, tool, category, ts_reference, and ts_behavior", path)
		}
		if previous, exists := names[fixture.Name]; exists {
			t.Fatalf("%s duplicates fixture name %q from %s", path, fixture.Name, previous)
		}
		names[fixture.Name] = path
		if fixture.Input == nil {
			t.Fatalf("%s must include an input object", path)
		}
		if !strings.Contains(fixture.TSReference, ".ts:") {
			t.Fatalf("%s ts_reference must cite a TypeScript source line: %q", path, fixture.TSReference)
		}
		if fixture.SkipExecution && fixture.Permission == nil && fixture.ExpectedEnabled == nil {
			t.Fatalf("%s skip_execution requires an enabled or permission assertion", path)
		}
		if fixture.SkipExecution && fixture.ExecuteWithApprovalCommit {
			t.Fatalf("%s cannot combine skip_execution with execute_with_permission_prepared", path)
		}
		if !fixtureHasAssertion(fixture) {
			t.Fatalf("%s must contain at least one enabled, permission, result, or state assertion", path)
		}
		fixtures = append(fixtures, fixture)
	}
	return fixtures
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values are not allowed")
		}
		return err
	}
	return nil
}

func fixtureHasAssertion(fixture toolParityFixture) bool {
	state := fixture.ExpectedState
	return fixture.ExpectedEnabled != nil || fixture.Permission != nil ||
		fixture.ExpectedValidationError != "" || fixture.ExpectedModelText != nil ||
		len(fixture.ExpectedModelTextContains) > 0 || len(fixture.ExpectedBlockTextContains) > 0 ||
		len(fixture.ExpectedTypedDataJSON) > 0 || len(fixture.ExpectedTypedDataJSONContains) > 0 ||
		len(fixture.ExpectedContentJSON) > 0 || len(state.Files) > 0 || len(state.ReadStatePaths) > 0 ||
		state.TodoCount != nil || state.WebCacheEntriesMin != nil
}

func newParityHarness(t *testing.T, fixture toolParityFixture) *parityHarness {
	t.Helper()
	workspace := t.TempDir()
	t.Setenv("CLAUDE_HOME", filepath.Join(workspace, ".claude-home"))
	t.Setenv("CLAUDE_CODE_ENABLE_TASKS", "")
	interactive := true
	if fixture.Runtime.Interactive != nil {
		interactive = *fixture.Runtime.Interactive
	}
	scope := NewRuntimeScope(workspace, interactive)
	scope.SetAllowedDirs([]string{workspace})

	todoStore := NewTodoStore(workspace)
	todoStore.SetScopeResolver(scope)
	taskStore := NewTaskStore()
	taskStore.SetScopeResolver(scope)
	h := &parityHarness{
		workspace: workspace,
		readState: NewReadFileState(),
		todoStore: todoStore,
		webCache:  NewSearchCache(),
		registry:  registry.New(),
	}
	h.registry.SetRuntimeContextProvider(scope)
	h.registry.Register(&FileReadTool{AllowedDirs: []string{workspace}, ReadState: h.readState})
	h.registry.Register(&FileWriteTool{AllowedDirs: []string{workspace}, ReadState: h.readState})
	h.registry.Register(NewTaskListTool(taskStore))
	h.registry.Register(NewTodoWriteTool(h.todoStore))
	webFetch := NewWebFetchTool(h.webCache)
	webFetch.skipSSRFCheck = true
	h.registry.Register(webFetch)

	if fixture.Setup.HTTPResponse != nil {
		resp := fixture.Setup.HTTPResponse
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if resp.ContentType != "" {
				w.Header().Set("Content-Type", resp.ContentType)
			}
			_, _ = w.Write([]byte(resp.Body))
		}))
		h.serverURL = server.URL
		h.cleanup = append(h.cleanup, server.Close)
	}

	for _, file := range fixture.Setup.Files {
		path := h.expand(file.Path)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir fixture path %s: %v", path, err)
		}
		if err := os.WriteFile(path, []byte(file.Content), 0o644); err != nil {
			t.Fatalf("write fixture file %s: %v", path, err)
		}
	}
	if fixture.Setup.Todos != nil {
		if err := h.todoStore.Save(fixture.Setup.Todos); err != nil {
			t.Fatalf("seed todo fixture: %v", err)
		}
	}
	return h
}

func (h *parityHarness) close() {
	for i := len(h.cleanup) - 1; i >= 0; i-- {
		h.cleanup[i]()
	}
}

func (h *parityHarness) runFixture(t *testing.T, fixture toolParityFixture) {
	t.Helper()
	input := h.expandValue(fixture.Input).(map[string]any)
	tool := h.registry.Get(fixture.Tool)
	if tool == nil {
		t.Fatalf("fixture tool %q is not registered in the parity dispatcher", fixture.Tool)
	}
	if fixture.ExpectedEnabled != nil {
		if got := h.registry.IsToolEnabled(tool); got != *fixture.ExpectedEnabled {
			t.Fatalf("tool enabled = %t, want %t", got, *fixture.ExpectedEnabled)
		}
	}

	if fixture.Permission != nil {
		decision, err := h.registry.CheckToolPermissions(context.Background(), fixture.Tool, input, types.ToolPermissionRequest{})
		if err != nil {
			t.Fatalf("permission check: %v", err)
		}
		if got := string(decision.Behavior); got != fixture.Permission.Behavior {
			t.Fatalf("permission behavior = %q, want %q (message=%q)", got, fixture.Permission.Behavior, decision.Message)
		}
		for _, want := range fixture.Permission.MessageContains {
			if !strings.Contains(decision.Message, want) {
				t.Fatalf("permission message %q missing %q", decision.Message, want)
			}
		}
		if len(fixture.Permission.SuggestionsJSONContains) > 0 {
			suggestions, err := json.Marshal(decision.Suggestions)
			if err != nil {
				t.Fatalf("marshal permission suggestions: %v", err)
			}
			for _, want := range fixture.Permission.SuggestionsJSONContains {
				if !strings.Contains(string(suggestions), want) {
					t.Fatalf("permission suggestions JSON missing %q:\n%s", want, suggestions)
				}
			}
		}
	}
	if fixture.SkipExecution {
		h.assertState(t, fixture.ExpectedState)
		return
	}

	ctx := context.Background()
	if fixture.ExecuteWithApprovalCommit {
		request := types.ToolPermissionRequest{SessionID: "parity", TurnID: "turn", ToolUseID: "fixture", ApprovalEpoch: "parity-epoch"}
		permission, permissionErr := h.registry.CheckToolPermissions(ctx, fixture.Tool, input, request)
		if permissionErr != nil || permission.PermissionGrant == "" {
			t.Fatalf("prepare registry permission: result=%#v err=%v", permission, permissionErr)
		}
		policyCode := permission.ExecutionPolicyCode
		if permission.PolicyDecision != nil {
			if policyCode == "" {
				policyCode = permission.PolicyDecision.Code
			}
		}
		if permission.UpdatedInput != nil {
			input = permission.UpdatedInput
		}
		executionGrant := h.registry.AuthorizePermissionGrant(
			permission.PermissionGrant, fixture.Tool, input, permission.PermissionBinding, policyCode,
		)
		ctx = approvalcommit.WithPending(ctx, approvalcommit.Pending{
			Token: executionGrant, Binding: permission.PermissionBinding, PolicyCode: policyCode,
		})
	}
	result, err := h.registry.ExecuteToolWithError(ctx, fixture.Tool, input)
	if err != nil {
		t.Fatalf("registry dispatch: %v", err)
	}

	if fixture.ExpectedValidationError != "" {
		if !result.IsError {
			t.Fatalf("expected validation error %q, got success: %#v", fixture.ExpectedValidationError, result)
		}
		if !strings.Contains(result.Content, fixture.ExpectedValidationError) {
			t.Fatalf("validation text %q missing %q", result.Content, fixture.ExpectedValidationError)
		}
	} else if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}

	if fixture.ExpectedModelText != nil && result.TextContent() != *fixture.ExpectedModelText {
		t.Fatalf("model-visible text = %q, want %q", result.TextContent(), *fixture.ExpectedModelText)
	}
	for _, want := range fixture.ExpectedModelTextContains {
		if !strings.Contains(result.TextContent(), want) {
			t.Fatalf("model-visible text missing %q:\n%s", want, result.TextContent())
		}
	}
	for _, want := range fixture.ExpectedBlockTextContains {
		if !strings.Contains(contentBlocksText(result.ContentBlocks), want) {
			t.Fatalf("content block text missing %q:\n%s", want, contentBlocksText(result.ContentBlocks))
		}
	}
	if len(fixture.ExpectedTypedDataJSON) > 0 {
		assertJSONEqual(t, "typed data", result.Data, fixture.ExpectedTypedDataJSON)
	}
	if len(fixture.ExpectedTypedDataJSONContains) > 0 {
		data, err := json.Marshal(result.Data)
		if err != nil {
			t.Fatalf("marshal typed data: %v", err)
		}
		for _, want := range fixture.ExpectedTypedDataJSONContains {
			if !strings.Contains(string(data), want) {
				t.Fatalf("typed data JSON missing %q:\n%s", want, data)
			}
		}
	}
	if len(fixture.ExpectedContentJSON) > 0 {
		var got map[string]any
		if err := json.Unmarshal([]byte(result.Content), &got); err != nil {
			t.Fatalf("model content is not JSON: %v\n%s", err, result.Content)
		}
		for key, want := range fixture.ExpectedContentJSON {
			gotValue, ok := got[key]
			if !ok {
				t.Fatalf("content JSON missing key %q in %#v", key, got)
			}
			if !jsonScalarEqual(gotValue, want) {
				t.Fatalf("content JSON %s = %#v, want %#v", key, gotValue, want)
			}
		}
	}

	h.assertState(t, fixture.ExpectedState)
}

func assertJSONEqual(t *testing.T, label string, got any, expected json.RawMessage) {
	t.Helper()
	gotJSON, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal %s: %v", label, err)
	}
	var gotValue any
	if err := json.Unmarshal(gotJSON, &gotValue); err != nil {
		t.Fatalf("decode actual %s JSON: %v", label, err)
	}
	var expectedValue any
	if err := json.Unmarshal(expected, &expectedValue); err != nil {
		t.Fatalf("decode expected %s JSON: %v", label, err)
	}
	if !reflect.DeepEqual(gotValue, expectedValue) {
		t.Fatalf("%s JSON = %s, want %s", label, gotJSON, expected)
	}
}

func (h *parityHarness) assertState(t *testing.T, expected parityExpectedState) {
	t.Helper()
	for _, file := range expected.Files {
		path := h.expand(file.Path)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read expected state file %s: %v", path, err)
		}
		if string(data) != file.Content {
			t.Fatalf("state file %s = %q, want %q", path, string(data), file.Content)
		}
	}
	for _, rawPath := range expected.ReadStatePaths {
		path := h.expand(rawPath)
		abs, err := filepath.Abs(path)
		if err != nil {
			t.Fatalf("abs read-state path %s: %v", path, err)
		}
		if _, ok := h.readState.Get(filepath.Clean(abs)); !ok {
			t.Fatalf("read state missing %s", filepath.Clean(abs))
		}
	}
	if expected.TodoCount != nil {
		if got := len(h.todoStore.Load()); got != *expected.TodoCount {
			t.Fatalf("todo store count = %d, want %d", got, *expected.TodoCount)
		}
	}
	if expected.WebCacheEntriesMin != nil {
		h.webCache.mu.Lock()
		got := len(h.webCache.entries)
		h.webCache.mu.Unlock()
		if got < *expected.WebCacheEntriesMin {
			t.Fatalf("web cache entries = %d, want at least %d", got, *expected.WebCacheEntriesMin)
		}
	}
}

func (h *parityHarness) expandValue(value any) any {
	switch v := value.(type) {
	case string:
		return h.expand(v)
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = h.expandValue(item)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			out[key] = h.expandValue(item)
		}
		return out
	default:
		return value
	}
}

func (h *parityHarness) expand(value string) string {
	value = strings.ReplaceAll(value, "${workspace}", h.workspace)
	value = strings.ReplaceAll(value, "${server_url}", h.serverURL)
	return value
}

func contentBlocksText(blocks []types.ContentBlock) string {
	var parts []string
	for _, block := range blocks {
		if text, ok := block.(types.TextBlock); ok {
			parts = append(parts, text.Text)
			continue
		}
		parts = append(parts, fmt.Sprintf("%v", block))
	}
	return strings.Join(parts, "\n")
}

func jsonScalarEqual(got, want any) bool {
	if reflect.DeepEqual(got, want) {
		return true
	}
	switch wantValue := want.(type) {
	case bool:
		gotValue, ok := got.(bool)
		return ok && gotValue == wantValue
	case string:
		gotValue, ok := got.(string)
		return ok && gotValue == wantValue
	case float64:
		gotValue, ok := got.(float64)
		return ok && gotValue == wantValue
	default:
		return false
	}
}
