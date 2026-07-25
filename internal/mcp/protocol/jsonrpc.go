// Package protocol owns MCP's transport-independent JSON-RPC wire contract.
package protocol

import (
	"bytes"
	"encoding/json"

	"github.com/agent-dance/luban/i18n"
)

// JSONRPCVersion is the protocol version marker used by MCP transports.
const JSONRPCVersion = "2.0"

// JSONRPCMessage is the transport-neutral JSON-RPC 2.0 envelope used by MCP.
// Result and Params stay raw so protocol result envelopes are not flattened or
// schema-stripped by a transport.
type JSONRPCMessage struct {
	JSONRPC string          `json:"jsonrpc,omitempty"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// RPCError is a JSON-RPC error object returned by an MCP server.
type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// Error implements error.
func (e *RPCError) Error() string {
	if e == nil {
		return i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyServicesMCPJSONRPCError)
	}
	if e.Message == "" {
		return i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyServicesMCPJSONRPCErrorCode, e.Code)
	}
	return i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyServicesMCPJSONRPCErrorDetail, e.Code, e.Message)
}

// NewRequestMessage builds a JSON-RPC request envelope.
func NewRequestMessage(id int64, method string, params any) (JSONRPCMessage, error) {
	if method == "" {
		return JSONRPCMessage{}, i18n.NewError(i18n.KeyServicesMCPJSONRPCRequestMethodMissing)
	}
	idRaw, err := json.Marshal(id)
	if err != nil {
		return JSONRPCMessage{}, err
	}
	msg := JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		ID:      idRaw,
		Method:  method,
	}
	if params != nil {
		raw, err := json.Marshal(params)
		if err != nil {
			return JSONRPCMessage{}, i18n.WrapInternalError(i18n.KeyServicesMCPJSONRPCEncodeRequestParams, err)
		}
		msg.Params = raw
	}
	return msg, nil
}

// NewNotificationMessage builds a JSON-RPC notification envelope.
func NewNotificationMessage(method string, params any) (JSONRPCMessage, error) {
	if method == "" {
		return JSONRPCMessage{}, i18n.NewError(i18n.KeyServicesMCPJSONRPCNotifyMethodMissing)
	}
	msg := JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		Method:  method,
	}
	if params != nil {
		raw, err := json.Marshal(params)
		if err != nil {
			return JSONRPCMessage{}, i18n.WrapInternalError(i18n.KeyServicesMCPJSONRPCEncodeNotifyParams, err)
		}
		msg.Params = raw
	}
	return msg, nil
}

// NewResultMessage builds a JSON-RPC response with a result payload.
func NewResultMessage(id json.RawMessage, result any) (JSONRPCMessage, error) {
	if len(bytes.TrimSpace(id)) == 0 {
		return JSONRPCMessage{}, i18n.NewError(i18n.KeyServicesMCPJSONRPCResultIDMissing)
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return JSONRPCMessage{}, i18n.WrapInternalError(i18n.KeyServicesMCPJSONRPCEncodeResult, err)
	}
	return JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		ID:      CloneRawMessage(id),
		Result:  raw,
	}, nil
}

// NewErrorMessage builds a JSON-RPC error response.
func NewErrorMessage(id json.RawMessage, code int, message string, data any) (JSONRPCMessage, error) {
	if len(bytes.TrimSpace(id)) == 0 {
		return JSONRPCMessage{}, i18n.NewError(i18n.KeyServicesMCPJSONRPCErrorIDMissing)
	}
	rpcErr := &RPCError{Code: code, Message: message}
	if data != nil {
		raw, err := json.Marshal(data)
		if err != nil {
			return JSONRPCMessage{}, i18n.WrapInternalError(i18n.KeyServicesMCPJSONRPCEncodeErrorData, err)
		}
		rpcErr.Data = raw
	}
	return JSONRPCMessage{
		JSONRPC: JSONRPCVersion,
		ID:      CloneRawMessage(id),
		Error:   rpcErr,
	}, nil
}

// CloneRawMessage returns an independent copy of a JSON wire fragment.
func CloneRawMessage(raw json.RawMessage) json.RawMessage {
	if raw == nil {
		return nil
	}
	cp := make(json.RawMessage, len(raw))
	copy(cp, raw)
	return cp
}
