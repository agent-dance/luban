package sdk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/agent-dance/luban/engine"
	"github.com/agent-dance/luban/i18n"
)

func localizedSDKError(t *testing.T, err error, lang i18n.Language) string {
	t.Helper()
	var localized interface {
		Localized(i18n.Language) string
	}
	if !errors.As(err, &localized) {
		t.Fatalf("error %T does not support explicit-language rendering: %v", err, err)
	}
	return localized.Localized(lang)
}

func TestSessionAPIErrorsAreSemanticAndPreserveOSCause(t *testing.T) {
	err := validateSessionID("bad/id")
	if got, want := localizedSDKError(t, err, i18n.LangEN), `sdk/sessions: session ID "bad/id" contains invalid characters (only alphanumeric, hyphen, underscore allowed)`; got != want {
		t.Fatalf("English validation error = %q, want %q", got, want)
	}
	zh := localizedSDKError(t, err, i18n.LangZH)
	if !strings.Contains(zh, "bad/id") || strings.Contains(zh, "contains invalid characters") {
		t.Fatalf("Chinese validation error did not preserve the ID or localize its copy: %q", zh)
	}

	fileInsteadOfDir := t.TempDir() + "/sessions-file"
	if err := os.WriteFile(fileInsteadOfDir, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	overrideSessionsDir(t, fileInsteadOfDir)
	_, err = ListSessions()
	if err == nil {
		t.Fatal("ListSessions unexpectedly succeeded for a non-directory path")
	}
	var pathErr *os.PathError
	if !errors.As(err, &pathErr) {
		t.Fatalf("ListSessions no longer preserves its OS cause: %T: %v", err, err)
	}
	zh = localizedSDKError(t, err, i18n.LangZH)
	if strings.Contains(zh, "list sessions") || !strings.Contains(zh, pathErr.Error()) {
		t.Fatalf("localized list error did not preserve the raw OS cause: %q", zh)
	}
}

func TestPermissionBridgeErrorsAreSemanticAndPreserveCauses(t *testing.T) {
	marshalHandler := &SDKPermissionHandler{
		bridge:   newPermissionBridge(),
		newReqID: func() string { return "req-marshal" },
		sendFn:   func(any) error { return nil },
	}
	_, err := marshalHandler.Check(context.Background(), engine.PermissionRequest{
		ToolName: "CustomTool",
		Input:    map[string]any{"unsupported": make(chan int)},
	})
	if err == nil {
		t.Fatal("permission request with unsupported input unexpectedly marshaled")
	}
	var unsupported *json.UnsupportedTypeError
	if !errors.As(err, &unsupported) {
		t.Fatalf("marshal error no longer preserves json.UnsupportedTypeError: %T: %v", err, err)
	}
	if got := localizedSDKError(t, err, i18n.LangZH); strings.Contains(got, "marshal permission request") || !strings.Contains(got, unsupported.Error()) {
		t.Fatalf("localized marshal error did not retain the raw JSON cause: %q", got)
	}

	sendCause := errors.New("raw-transport-cause")
	sendHandler := &SDKPermissionHandler{
		bridge:   newPermissionBridge(),
		newReqID: func() string { return "req-send" },
		sendFn:   func(any) error { return sendCause },
	}
	_, err = sendHandler.Check(context.Background(), engine.PermissionRequest{ToolName: "Write"})
	if !errors.Is(err, sendCause) {
		t.Fatalf("send error no longer preserves errors.Is: %T: %v", err, err)
	}
	if got := localizedSDKError(t, err, i18n.LangZH); strings.Contains(got, "send permission request") || !strings.Contains(got, sendCause.Error()) {
		t.Fatalf("localized send error did not retain the raw transport cause: %q", got)
	}
}

type sdkBoundaryEngine struct {
	engine.Engine
	queryErr    error
	setModelErr error
}

func (e *sdkBoundaryEngine) Query(context.Context, engine.QueryRequest) (<-chan engine.Event, error) {
	return nil, e.queryErr
}

func (e *sdkBoundaryEngine) SetModel(string, string) error {
	return e.setModelErr
}

func TestSDKResultUsesCapturedLanguageAndHidesInternalCause(t *testing.T) {
	internal := errors.New("internal English diagnostic")
	semantic := i18n.WrapInternalError(i18n.KeyEngineSessionLoadFailed, internal)
	result := newEventAdapter("session-7").resultMessage(i18n.LangZH, "session-7", "query-7", semantic)
	if len(result.Errors) != 1 || result.Errors[0] != i18n.Text(i18n.LangZH, i18n.KeyEngineSessionLoadFailed) {
		t.Fatalf("query result did not use its captured language: %#v", result.Errors)
	}
	if strings.Contains(result.Errors[0], internal.Error()) {
		t.Fatalf("query result leaked an internal cause: %q", result.Errors[0])
	}

	rawExternal := errors.New("raw provider detail")
	result = newEventAdapter("session-8").resultMessage(i18n.LangZH, "session-8", "query-8", rawExternal)
	if len(result.Errors) != 1 || result.Errors[0] != rawExternal.Error() {
		t.Fatalf("query result did not preserve an unknown external diagnostic: %#v", result.Errors)
	}
}

func TestSDKControlAndInitialQueryErrorsUseCapturedLanguage(t *testing.T) {
	internal := errors.New("internal English diagnostic")
	semantic := i18n.WrapInternalError(i18n.KeyEngineSessionLoadFailed, internal)

	var controlOut bytes.Buffer
	controlServer := NewSDKServer(&sdkBoundaryEngine{setModelErr: semantic}, bytes.NewReader(nil), &controlOut)
	payload, err := json.Marshal(SetModelRequest{Subtype: "set_model", SessionID: "session-7", Model: "model-id"})
	if err != nil {
		t.Fatal(err)
	}
	if err := controlServer.handleSetModel(SDKControlRequest{RequestID: "req-7", Request: payload}, i18n.LangZH); err != nil {
		t.Fatalf("handleSetModel: %v", err)
	}
	var envelope SDKControlResponse
	if err := json.NewDecoder(&controlOut).Decode(&envelope); err != nil {
		t.Fatalf("decode control response: %v", err)
	}
	var controlErr ControlError
	if err := json.Unmarshal(envelope.Response, &controlErr); err != nil {
		t.Fatalf("decode control error: %v", err)
	}
	if controlErr.Error != i18n.Text(i18n.LangZH, i18n.KeyEngineSessionLoadFailed) || strings.Contains(controlErr.Error, internal.Error()) {
		t.Fatalf("control error ignored its captured language or leaked its cause: %q", controlErr.Error)
	}

	var queryOut bytes.Buffer
	queryServer := NewSDKServer(&sdkBoundaryEngine{queryErr: semantic}, bytes.NewReader(nil), &queryOut)
	userLine, err := json.Marshal(SDKUserMessage{Type: "user", SessionID: "session-7", UUID: "query-7", Message: json.RawMessage(`"hello"`)})
	if err != nil {
		t.Fatal(err)
	}
	if err := queryServer.handleUser(context.Background(), userLine, i18n.LangZH); err != nil {
		t.Fatalf("handleUser: %v", err)
	}
	var queryResult SDKResultMessage
	if err := json.NewDecoder(&queryOut).Decode(&queryResult); err != nil {
		t.Fatalf("decode query result: %v", err)
	}
	if len(queryResult.Errors) != 1 || queryResult.Errors[0] != i18n.Text(i18n.LangZH, i18n.KeyEngineSessionLoadFailed) {
		t.Fatalf("initial query error ignored its captured language: %#v", queryResult.Errors)
	}
}
