package sdk

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"

	"github.com/agent-dance/luban/engine"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/prompt"
)

// SDKServer reads NDJSON from an io.Reader (stdin), drives an engine.Engine,
// and writes NDJSON responses to an io.Writer (stdout).
type SDKServer struct {
	eng engine.Engine
	in  io.Reader
	out io.Writer

	mu sync.Mutex // protects writes to out

	bridge           *permissionBridge
	counter          uint64 // atomic request-ID counter
	permissionModeMu sync.RWMutex
	permissionMode   string // "default", "plan", "auto-edit", "full-auto"

	toolApprovalMu sync.RWMutex
	toolApproval   ToolApprovalFunc

	systemPromptMu sync.RWMutex
	systemPrompt   prompt.SystemPrompt
}

// NewSDKServer creates an SDKServer backed by eng, reading from in and writing
// to out (typically os.Stdin / os.Stdout).
func NewSDKServer(eng engine.Engine, in io.Reader, out io.Writer) *SDKServer {
	return &SDKServer{
		eng:    eng,
		in:     in,
		out:    out,
		bridge: newPermissionBridge(),
	}
}

// Serve reads NDJSON lines from s.in and dispatches them until ctx is cancelled
// or in reaches EOF. It blocks the caller.
func (s *SDKServer) Serve(ctx context.Context) error {
	// Wire the permission bridge BEFORE any queries start so that every
	// can_use_tool challenge round-trips through the SDK client.
	s.eng.SetPermission(s.permissionHandler())

	// Emit system/init so the client knows the server is ready.
	_ = s.writeMsg(SDKSystemMessage{
		Type:    "system",
		Subtype: "init",
		Message: i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeySDKReady),
	})

	scanner := bufio.NewScanner(s.in)
	scanner.Buffer(make([]byte, 4*1024*1024), 4*1024*1024) // 4 MiB max line

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		// Capture the display language at the request boundary so every response
		// produced for this input line uses one coherent language, even if the
		// process-wide preference changes while the request is running.
		lang := i18n.DetectOrLoadLanguage()
		if err := s.dispatch(ctx, line, lang); err != nil {
			// Log non-fatal dispatch errors as system error messages.
			_ = s.writeMsg(SDKSystemMessage{
				Type:    "system",
				Subtype: "error",
				Message: engine.UserFacingError(lang, err),
			})
		}
	}

	if err := scanner.Err(); err != nil {
		return i18n.WrapError(i18n.KeySDKStdinReadError, err)
	}
	return nil
}

// envelope is used to peek at the "type" field of an incoming JSON line.
type envelope struct {
	Type string `json:"type"`
}

// requestSubtype is used to peek at the "subtype" field inside Request bodies.
type requestSubtype struct {
	Subtype string `json:"subtype"`
}

// dispatch routes a single JSON line to the appropriate handler.
func (s *SDKServer) dispatch(ctx context.Context, line []byte, lang i18n.Language) error {
	var env envelope
	if err := json.Unmarshal(line, &env); err != nil {
		return i18n.WrapError(i18n.KeySDKInvalidJSONEnvelope, err)
	}

	switch env.Type {
	case "user":
		return s.handleUser(ctx, line, lang)
	case "control_request":
		return s.handleControlRequest(ctx, line, lang)
	case "control_response":
		return s.handleControlResponse(line, lang)
	case "keep_alive":
		return nil // heartbeat — no-op
	default:
		return i18n.NewError(i18n.KeySDKUnknownMessageType, env.Type)
	}
}

// handleUser extracts the text from a user message and runs a query.
func (s *SDKServer) handleUser(ctx context.Context, line []byte, lang i18n.Language) error {
	var msg SDKUserMessage
	if err := json.Unmarshal(line, &msg); err != nil {
		return i18n.WrapError(i18n.KeySDKParseUserMessage, err)
	}

	// The Message field is an API-format user message; extract text content.
	text, err := extractText(msg.Message)
	if err != nil {
		return i18n.WrapError(i18n.KeySDKExtractMessageText, err)
	}

	sessionID := msg.SessionID
	queryUUID := msg.UUID
	if queryUUID == "" {
		queryUUID = uuid.New().String()
	}

	req := engine.QueryRequest{
		SessionID: sessionID,
		Message:   text,
	}
	if systemPrompt := s.currentSystemPrompt(); len(systemPrompt) > 0 {
		req.SystemPromptOverride = systemPrompt.String()
	}

	ch, err := s.eng.Query(ctx, req)
	if err != nil {
		return s.writeMsg(SDKResultMessage{
			Type:      "result",
			Subtype:   "error_during_execution",
			SessionID: sessionID,
			UUID:      queryUUID,
			IsError:   true,
			Errors:    []string{engine.UserFacingError(lang, err)},
		})
	}

	adapter := newEventAdapter(sessionID)
	gotFinal := false

	for evt := range ch {
		if evt.Final {
			gotFinal = true
			result := adapter.resultMessage(lang, evt.SessionID, queryUUID, evt.Error)
			return s.writeMsg(result)
		}
		msgs := adapter.process(evt.Inner)
		for _, m := range msgs {
			if werr := s.writeMsg(m); werr != nil {
				return werr
			}
		}
	}

	// SP4: channel closed without a Final event — send an error result so the
	// client always receives a terminal message.
	if !gotFinal {
		return s.writeMsg(adapter.resultMessage(lang, sessionID, queryUUID, i18n.NewError(i18n.KeySDKStreamEndedWithoutFinalEvent)))
	}
	return nil
}

// handleControlRequest processes a control_request message.
func (s *SDKServer) handleControlRequest(ctx context.Context, line []byte, lang i18n.Language) error {
	var req SDKControlRequest
	if err := json.Unmarshal(line, &req); err != nil {
		return i18n.WrapError(i18n.KeySDKParseControlRequest, err)
	}

	// Peek at subtype.
	var sub requestSubtype
	if err := json.Unmarshal(req.Request, &sub); err != nil {
		return s.sendControlErrorKey(lang, req.RequestID, i18n.KeySDKParseRequestSubtype, err)
	}

	switch sub.Subtype {
	case "initialize":
		return s.handleInitialize(req, lang)
	case "interrupt":
		return s.handleInterrupt(ctx, req, lang)
	case "set_model":
		return s.handleSetModel(req, lang)
	case "set_permission_mode":
		return s.handleSetPermissionMode(req, lang)
	case "set_max_thinking_tokens":
		return s.handleSetMaxThinkingTokens(req, lang)
	case "resume":
		return s.handleResume(ctx, req, lang)
	case "compact":
		return s.handleCompact(ctx, req, lang)
	case "get_context_usage":
		return s.handleGetContextUsage(req, lang)
	default:
		return s.sendControlErrorKey(lang, req.RequestID, i18n.KeySDKUnsupportedControlSubtype, sub.Subtype)
	}
}

func (s *SDKServer) handleInitialize(req SDKControlRequest, lang i18n.Language) error {
	var initReq InitializeRequest
	if err := json.Unmarshal(req.Request, &initReq); err != nil {
		return s.sendControlErrorKey(lang, req.RequestID, i18n.KeySDKParseControlPayload, "initialize", err)
	}

	if strings.TrimSpace(initReq.SystemPrompt) != "" {
		systemPrompt := prompt.BuildEffectiveSystemPrompt(prompt.EffectiveSystemPromptInput{
			Custom: initReq.SystemPrompt,
		})
		if configurable, ok := s.eng.(interface{ SetSystemPrompt(prompt.SystemPrompt) }); ok {
			configurable.SetSystemPrompt(systemPrompt)
			s.setSystemPrompt(nil)
		} else {
			s.setSystemPrompt(systemPrompt)
		}
		_ = s.writeMsg(SDKSystemMessage{
			Type:    "system",
			Subtype: "status",
			Message: i18n.Text(lang, i18n.KeySDKSystemPromptReceived),
		})
	}

	modelID := s.eng.Provider().ModelID()
	respPayload, err := json.Marshal(InitializeResponse{
		Tools:           s.eng.Tools(),
		Model:           modelID,
		Models:          []string{modelID},
		OutputStyle:     "streamlined",
		AvailableStyles: []string{"streamlined", "full"},
		ProtocolVersion: "1.0",
	})
	if err != nil {
		return s.sendControlErrorKey(lang, req.RequestID, i18n.KeySDKMarshalInitializeResponse, err)
	}

	return s.sendControlSuccess(req.RequestID, json.RawMessage(respPayload))
}

func (s *SDKServer) setSystemPrompt(systemPrompt prompt.SystemPrompt) {
	s.systemPromptMu.Lock()
	defer s.systemPromptMu.Unlock()
	s.systemPrompt = append(prompt.SystemPrompt(nil), systemPrompt...)
}

func (s *SDKServer) currentSystemPrompt() prompt.SystemPrompt {
	s.systemPromptMu.RLock()
	defer s.systemPromptMu.RUnlock()
	return append(prompt.SystemPrompt(nil), s.systemPrompt...)
}

func (s *SDKServer) handleInterrupt(_ context.Context, req SDKControlRequest, lang i18n.Language) error {
	var intReq InterruptRequest
	if err := json.Unmarshal(req.Request, &intReq); err != nil {
		return s.sendControlErrorKey(lang, req.RequestID, i18n.KeySDKParseControlPayload, "interrupt", err)
	}
	s.eng.Interrupt(intReq.SessionID)
	return s.sendControlSuccess(req.RequestID, nil)
}

func (s *SDKServer) handleSetModel(req SDKControlRequest, lang i18n.Language) error {
	var smReq SetModelRequest
	if err := json.Unmarshal(req.Request, &smReq); err != nil {
		return s.sendControlErrorKey(lang, req.RequestID, i18n.KeySDKParseControlPayload, "set_model", err)
	}
	if err := s.eng.SetModel(smReq.SessionID, smReq.Model); err != nil {
		return s.sendControlEngineError(lang, req.RequestID, err)
	}
	return s.sendControlSuccess(req.RequestID, nil)
}

func (s *SDKServer) handleSetPermissionMode(req SDKControlRequest, lang i18n.Language) error {
	var pmReq SetPermissionModeRequest
	if err := json.Unmarshal(req.Request, &pmReq); err != nil {
		return s.sendControlErrorKey(lang, req.RequestID, i18n.KeySDKParseControlPayload, "set_permission_mode", err)
	}

	s.permissionModeMu.Lock()
	s.permissionMode = pmReq.Mode
	s.permissionModeMu.Unlock()

	log.Printf("%s: %q", i18n.Text(lang, i18n.KeyLogSDKPermissionMode), pmReq.Mode)

	// "full-auto" maps to AllowAllHandler; anything else uses the SDK bridge handler.
	if pmReq.Mode == "full-auto" {
		s.eng.SetPermission(engine.AllowAllHandler{})
	} else {
		s.eng.SetPermission(s.permissionHandler())
	}

	return s.sendControlSuccess(req.RequestID, nil)
}

func (s *SDKServer) handleSetMaxThinkingTokens(req SDKControlRequest, lang i18n.Language) error {
	var mttReq SetMaxThinkingTokensRequest
	if err := json.Unmarshal(req.Request, &mttReq); err != nil {
		return s.sendControlErrorKey(lang, req.RequestID, i18n.KeySDKParseControlPayload, "set_max_thinking_tokens", err)
	}

	enabled := mttReq.MaxThinkingTokens != nil
	budget := 0
	if enabled {
		budget = *mttReq.MaxThinkingTokens
	}

	if err := s.eng.SetThinkingConfig(mttReq.SessionID, enabled, budget); err != nil {
		return s.sendControlEngineError(lang, req.RequestID, err)
	}
	return s.sendControlSuccess(req.RequestID, nil)
}

func (s *SDKServer) handleResume(ctx context.Context, req SDKControlRequest, lang i18n.Language) error {
	var resumeReq ResumeRequest
	if err := json.Unmarshal(req.Request, &resumeReq); err != nil {
		return s.sendControlErrorKey(lang, req.RequestID, i18n.KeySDKParseControlPayload, "resume", err)
	}

	count, err := s.eng.Resume(ctx, resumeReq.SessionID)
	if err != nil {
		return s.sendControlEngineError(lang, req.RequestID, err)
	}

	respPayload, err := json.Marshal(ResumeResponse{
		SessionID:    resumeReq.SessionID,
		MessageCount: count,
	})
	if err != nil {
		return s.sendControlErrorKey(lang, req.RequestID, i18n.KeySDKMarshalResumeResponse, err)
	}
	return s.sendControlSuccess(req.RequestID, json.RawMessage(respPayload))
}

func (s *SDKServer) handleCompact(ctx context.Context, req SDKControlRequest, lang i18n.Language) error {
	var compactReq CompactRequest
	if err := json.Unmarshal(req.Request, &compactReq); err != nil {
		return s.sendControlErrorKey(lang, req.RequestID, i18n.KeySDKParseControlPayload, "compact", err)
	}

	if err := s.eng.Compact(ctx, compactReq.SessionID); err != nil {
		return s.sendControlEngineError(lang, req.RequestID, err)
	}
	return s.sendControlSuccess(req.RequestID, nil)
}

func (s *SDKServer) handleGetContextUsage(req SDKControlRequest, lang i18n.Language) error {
	var cuReq GetContextUsageRequest
	if err := json.Unmarshal(req.Request, &cuReq); err != nil {
		return s.sendControlErrorKey(lang, req.RequestID, i18n.KeySDKParseControlPayload, "get_context_usage", err)
	}

	info, err := s.eng.ContextUsage(cuReq.SessionID)
	if err != nil {
		return s.sendControlEngineError(lang, req.RequestID, err)
	}

	respPayload, err := json.Marshal(info)
	if err != nil {
		return s.sendControlErrorKey(lang, req.RequestID, i18n.KeySDKMarshalContextUsage, err)
	}
	return s.sendControlSuccess(req.RequestID, json.RawMessage(respPayload))
}

// handleControlResponse routes a permission response back to the waiting Check.
func (s *SDKServer) handleControlResponse(line []byte, lang i18n.Language) error {
	var resp SDKControlResponse
	if err := json.Unmarshal(line, &resp); err != nil {
		return i18n.WrapError(i18n.KeySDKParseControlResponse, err)
	}

	// Try to decode as PermissionResultMsg (can_use_tool reply).
	var perm PermissionResultMsg
	if err := json.Unmarshal(resp.Response, &perm); err == nil && perm.RequestID != "" {
		s.bridge.deliver(perm.RequestID, permissionResult{
			behavior: perm.Behavior,
			message:  perm.Message,
		})
		return nil
	}

	// Also handle the ControlSuccess envelope format.
	var cs ControlSuccess
	if err := json.Unmarshal(resp.Response, &cs); err == nil && cs.RequestID != "" {
		var perm2 PermissionResultMsg
		if cs.Response != nil {
			if err2 := json.Unmarshal(cs.Response, &perm2); err2 == nil {
				s.bridge.deliver(cs.RequestID, permissionResult{
					behavior: perm2.Behavior,
					message:  perm2.Message,
				})
			}
		}
		return nil
	}

	// SP5: unrecognized control_response format — surface as a system error so
	// the client can observe it rather than silently dropping it.
	_ = s.writeMsg(SDKSystemMessage{
		Type:    "system",
		Subtype: "error",
		Message: i18n.Format(lang, i18n.KeySDKUnrecognizedControlResponsePayload, string(resp.Response)),
	})
	return nil
}

// ─── helpers ─────────────────────────────────────────────────────────────────

// writeMsg JSON-encodes v and writes it as a single NDJSON line to s.out.
func (s *SDKServer) writeMsg(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return i18n.WrapError(i18n.KeySDKMarshalOutput, err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err = fmt.Fprintf(s.out, "%s\n", b)
	return err
}

func (s *SDKServer) sendControlSuccess(reqID string, resp json.RawMessage) error {
	payload, err := json.Marshal(ControlSuccess{
		Subtype:   "success",
		RequestID: reqID,
		Response:  resp,
	})
	if err != nil {
		return err
	}
	return s.writeMsg(SDKControlResponse{
		Type:     "control_response",
		Response: json.RawMessage(payload),
	})
}

func (s *SDKServer) sendControlError(reqID, msg string) error {
	payload, err := json.Marshal(ControlError{
		Subtype:   "error",
		RequestID: reqID,
		Error:     msg,
	})
	if err != nil {
		return err
	}
	return s.writeMsg(SDKControlResponse{
		Type:     "control_response",
		Response: json.RawMessage(payload),
	})
}

func (s *SDKServer) sendControlErrorKey(lang i18n.Language, reqID string, key i18n.Key, args ...any) error {
	return s.sendControlError(reqID, i18n.Format(lang, key, args...))
}

func (s *SDKServer) sendControlEngineError(lang i18n.Language, reqID string, err error) error {
	return s.sendControlError(reqID, engine.UserFacingError(lang, err))
}

// newRequestID returns a unique request ID string.
func (s *SDKServer) newRequestID() string {
	id := atomic.AddUint64(&s.counter, 1)
	return fmt.Sprintf("req-%d", id)
}

// permissionHandler returns an engine.PermissionHandler wired to s.bridge.
// Use this when constructing a query that should honour SDK-side permission.
func (s *SDKServer) permissionHandler() engine.PermissionHandler {
	return &SDKPermissionHandler{
		bridge:      s.bridge,
		sendFn:      s.writeMsg,
		newReqID:    s.newRequestID,
		getApproval: s.getToolApproval,
	}
}

// ─── text extraction helper ───────────────────────────────────────────────────

// extractText pulls plain text out of an API-format user message.
// Handles both {"type":"text","text":"..."} content blocks and bare strings.
func extractText(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}
	// Try plain string first.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s, nil
	}
	// Try {"content": [...]} message shape.
	var msg struct {
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &msg); err != nil {
		// Fall back: return the raw value as a string.
		return string(raw), nil
	}
	// Try content as a bare string.
	var cs string
	if err := json.Unmarshal(msg.Content, &cs); err == nil {
		return cs, nil
	}
	// Try content as array of blocks.
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(msg.Content, &blocks); err == nil {
		var out string
		for _, b := range blocks {
			if b.Type == "text" {
				out += b.Text
			}
		}
		return out, nil
	}
	return string(raw), nil
}
