package mcp

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/agent-dance/luban/i18n"
)

const (
	// ErrorCodeURLElicitationRequired is the MCP JSON-RPC code used when a
	// tool requires the user to open a URL before it can be retried.
	ErrorCodeURLElicitationRequired = -32042
)

// ToolTimeoutError marks a tool-call timeout owned by the client runtime.
type ToolTimeoutError struct {
	ServerName string
	ToolName   string
	Timeout    time.Duration
}

func (e *ToolTimeoutError) Error() string {
	if e == nil {
		return i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyServicesMCPToolTimedOut)
	}
	seconds := int(e.Timeout / time.Second)
	if seconds <= 0 && e.Timeout > 0 {
		seconds = 1
	}
	if e.ServerName == "" {
		return i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyServicesMCPToolTimedOutNamed, e.ToolName, seconds)
	}
	return i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyServicesMCPToolTimedOutOnServer, e.ServerName, e.ToolName, seconds)
}

// ToolCallError carries an MCP isError:true result as a typed error while
// preserving the protocol _meta envelope for downstream consumers.
type ToolCallError struct {
	ServerName string
	ToolName   string
	Message    string
	Meta       map[string]any
	Err        error
}

func (e *ToolCallError) Error() string {
	if e == nil {
		return i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyServicesMCPToolReturnedError)
	}
	msg := strings.TrimSpace(e.Message)
	if msg == "" {
		msg = i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyServicesMCPToolReturnedErrorMessage)
	}
	if e.ServerName == "" && e.ToolName == "" {
		return i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyServicesMCPToolReturnedMessage, msg)
	}
	return i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyServicesMCPToolReturnedServerMessage, e.ServerName, e.ToolName, msg)
}

func (e *ToolCallError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// URLElicitation is the validated URL-mode elicitation payload carried in
// JSON-RPC -32042 error data.
type URLElicitation struct {
	Mode          string          `json:"mode"`
	URL           string          `json:"url"`
	ElicitationID string          `json:"elicitationId"`
	Message       string          `json:"message"`
	Raw           json.RawMessage `json:"-"`
}

// ElicitationResult is the action returned by a hook, SDK host, or UI.
type ElicitationResult struct {
	Action  string         `json:"action"`
	Content map[string]any `json:"content,omitempty"`
}

// IsAuthRequiredError reports whether err should transition a server into the
// needs-auth state.
func IsAuthRequiredError(err error) bool {
	if err == nil {
		return false
	}
	var unauthorized *UnauthorizedError
	if errors.As(err, &unauthorized) {
		return true
	}
	var remote *RemoteHTTPError
	if errors.As(err, &remote) {
		if remote.StatusCode == http.StatusUnauthorized {
			return true
		}
		return remote.StatusCode == http.StatusForbidden &&
			remote.Challenge != nil &&
			strings.EqualFold(remote.Challenge.ErrorCode, "insufficient_scope")
	}
	return false
}

// IsURLElicitationRequiredError reports whether err is JSON-RPC -32042.
func IsURLElicitationRequiredError(err error) bool {
	_, ok := ExtractURLElicitations(err)
	return ok
}

// ExtractURLElicitations returns the validated URL elicitations from a
// JSON-RPC -32042 error. The boolean is true whenever the error code matches,
// even if the data contains no valid URL elicitation entries.
func ExtractURLElicitations(err error) ([]URLElicitation, bool) {
	if err == nil {
		return nil, false
	}
	var rpc *RPCError
	if !errors.As(err, &rpc) || rpc.Code != ErrorCodeURLElicitationRequired {
		return nil, false
	}
	var payload struct {
		Elicitations []json.RawMessage `json:"elicitations"`
	}
	if len(bytes.TrimSpace(rpc.Data)) == 0 {
		return nil, true
	}
	if err := json.Unmarshal(rpc.Data, &payload); err != nil {
		return nil, true
	}
	out := make([]URLElicitation, 0, len(payload.Elicitations))
	for _, raw := range payload.Elicitations {
		var item struct {
			Mode          string `json:"mode"`
			URL           string `json:"url"`
			ElicitationID string `json:"elicitationId"`
			Message       string `json:"message"`
		}
		if err := json.Unmarshal(raw, &item); err != nil {
			continue
		}
		if item.Mode != "url" ||
			strings.TrimSpace(item.URL) == "" ||
			strings.TrimSpace(item.ElicitationID) == "" ||
			strings.TrimSpace(item.Message) == "" {
			continue
		}
		out = append(out, URLElicitation{
			Mode:          item.Mode,
			URL:           item.URL,
			ElicitationID: item.ElicitationID,
			Message:       item.Message,
			Raw:           cloneRawMessage(raw),
		})
	}
	return out, true
}
