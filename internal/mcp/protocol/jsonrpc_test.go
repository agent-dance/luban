package protocol

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/agent-dance/luban/i18n"
)

func TestJSONRPCMessageConstructorsPreserveWireValues(t *testing.T) {
	request, err := NewRequestMessage(17, "tools/list", map[string]any{"cursor": "next"})
	if err != nil {
		t.Fatal(err)
	}
	if request.JSONRPC != JSONRPCVersion || string(request.ID) != "17" || request.Method != "tools/list" || string(request.Params) != `{"cursor":"next"}` {
		t.Fatalf("request = %#v", request)
	}

	notification, err := NewNotificationMessage("notifications/initialized", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if notification.JSONRPC != JSONRPCVersion || notification.Method != "notifications/initialized" || len(notification.ID) != 0 {
		t.Fatalf("notification = %#v", notification)
	}

	id := json.RawMessage(`"request-1"`)
	result, err := NewResultMessage(id, map[string]any{"ok": true})
	if err != nil {
		t.Fatal(err)
	}
	id[1] = 'X'
	if string(result.ID) != `"request-1"` || string(result.Result) != `{"ok":true}` {
		t.Fatalf("result = %#v", result)
	}

	rpcFailure, err := NewErrorMessage(json.RawMessage(`19`), -32601, "Method not found", map[string]any{"method": "unknown"})
	if err != nil {
		t.Fatal(err)
	}
	if rpcFailure.Error == nil || rpcFailure.Error.Code != -32601 || rpcFailure.Error.Message != "Method not found" || string(rpcFailure.Error.Data) != `{"method":"unknown"}` {
		t.Fatalf("error response = %#v", rpcFailure)
	}
}

func TestJSONRPCMessageConstructorErrorsAreSemantic(t *testing.T) {
	tests := []struct {
		name string
		key  i18n.Key
		err  error
	}{
		{name: "request method", key: i18n.KeyServicesMCPJSONRPCRequestMethodMissing, err: errorFromRequest("", nil)},
		{name: "notification method", key: i18n.KeyServicesMCPJSONRPCNotifyMethodMissing, err: errorFromNotification("", nil)},
		{name: "result id", key: i18n.KeyServicesMCPJSONRPCResultIDMissing, err: errorFromResult(nil, nil)},
		{name: "error id", key: i18n.KeyServicesMCPJSONRPCErrorIDMissing, err: errorFromError(nil, nil)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			info, ok := i18n.DescribeSemanticError(test.err)
			if !ok || info.Key != test.key {
				t.Fatalf("error = %T %[1]v, semantic info = %#v", test.err, info)
			}
		})
	}
}

func TestJSONRPCEncodingErrorsPreserveButHideInternalCause(t *testing.T) {
	causeValue := func() any { return make(chan int) }
	tests := []struct {
		name string
		key  i18n.Key
		run  func(any) error
	}{
		{name: "request params", key: i18n.KeyServicesMCPJSONRPCEncodeRequestParams, run: func(value any) error { return errorFromRequest("tools/list", value) }},
		{name: "notification params", key: i18n.KeyServicesMCPJSONRPCEncodeNotifyParams, run: func(value any) error { return errorFromNotification("notifications/test", value) }},
		{name: "result", key: i18n.KeyServicesMCPJSONRPCEncodeResult, run: func(value any) error { return errorFromResult(json.RawMessage(`1`), value) }},
		{name: "error data", key: i18n.KeyServicesMCPJSONRPCEncodeErrorData, run: func(value any) error { return errorFromError(json.RawMessage(`1`), value) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := causeValue()
			err := test.run(value)
			info, ok := i18n.DescribeSemanticError(err)
			if !ok || info.Key != test.key || info.Cause == nil || info.IncludeCause {
				t.Fatalf("error = %T %[1]v, semantic info = %#v", err, info)
			}
			if !errors.Is(err, info.Cause) {
				t.Fatal("semantic error did not preserve its encoding cause")
			}
			if strings.Contains(err.Error(), info.Cause.Error()) {
				t.Fatalf("internal encoding cause leaked: %q", err)
			}
		})
	}
}

func errorFromRequest(method string, value any) error {
	_, err := NewRequestMessage(1, method, value)
	return err
}

func errorFromNotification(method string, value any) error {
	_, err := NewNotificationMessage(method, value)
	return err
}

func errorFromResult(id json.RawMessage, value any) error {
	_, err := NewResultMessage(id, value)
	return err
}

func errorFromError(id json.RawMessage, value any) error {
	_, err := NewErrorMessage(id, -32603, "Internal error", value)
	return err
}
