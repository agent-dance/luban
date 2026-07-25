package sdk

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/agent-dance/luban/i18n"
)

// SDKServer reads NDJSON from an io.Reader (stdin), drives a Runtime,
// and writes NDJSON responses to an io.Writer (stdout).
type SDKServer struct {
	runtime Runtime
	in      io.Reader
	out     io.Writer

	initialPermissionMode InitialPermissionMode

	directWriteMu sync.Mutex
	writerMu      sync.RWMutex
	writer        *sdkMessageWriter
	serveStarted  atomic.Bool
	inputClose    sync.Once

	queryMu     sync.Mutex
	activeQuery *sdkActiveQuery
	queryWG     sync.WaitGroup
	asyncErrMu  sync.Mutex
	asyncErr    error
	controlMu   sync.Mutex
	controls    map[string]sdkControlHistory

	bridge         *permissionBridge
	counter        uint64 // atomic request-ID counter
	toolApprovalMu sync.RWMutex
	toolApproval   ToolApprovalFunc
}

// InitialPermissionMode selects the permission handler installed before the
// SDK server accepts its first query. It has exactly two valid states.
type InitialPermissionMode bool

const (
	// InitialPermissionBridge requires the SDK client to answer permission
	// challenges.
	InitialPermissionBridge InitialPermissionMode = false
	// InitialPermissionFullAuto approves permission requests without emitting a
	// client challenge.
	InitialPermissionFullAuto InitialPermissionMode = true
)

type sdkUserQuery struct {
	sessionID string
	uuid      string
	text      string
	language  i18n.Language
}

type sdkActiveQuery struct {
	sessionID     string
	uuid          string
	cancel        context.CancelFunc
	rejected      []sdkUserQuery
	rejectedUUIDs map[string]struct{}
}

type sdkControlHistory struct {
	request  string
	response SDKControlResponse
}

// NewSDKServer creates an SDKServer backed by runtime, reading from in and
// writing to out (typically os.Stdin / os.Stdout). initialPermissionMode is
// installed before the first query. To interrupt a blocked read when Serve is
// cancelled, in must implement io.Closer. The output writer must return from
// Write; Go's io.Writer contract has no cancellation operation.
func NewSDKServer(runtime Runtime, in io.Reader, out io.Writer, initialPermissionMode InitialPermissionMode) *SDKServer {
	return &SDKServer{
		runtime:               runtime,
		in:                    in,
		out:                   out,
		initialPermissionMode: initialPermissionMode,
		bridge:                newPermissionBridge(),
		controls:              make(map[string]sdkControlHistory),
	}
}

// Serve reads NDJSON lines from s.in and dispatches them until ctx is cancelled
// or in reaches EOF. It blocks the caller.
func (s *SDKServer) Serve(ctx context.Context) (returnErr error) {
	if !s.serveStarted.CompareAndSwap(false, true) {
		return i18n.NewError(i18n.KeySDKServeAlreadyStarted)
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	writer := newSDKMessageWriter(s.out, func() {
		s.cancelActiveQuery()
		s.bridge.close()
		s.interruptInput()
	})
	s.writerMu.Lock()
	s.writer = writer
	s.writerMu.Unlock()

	stopInputInterrupt := context.AfterFunc(ctx, func() {
		s.cancelActiveQuery()
		s.bridge.close()
		s.interruptInput()
	})
	defer func() {
		stopInputInterrupt()
		s.cancelActiveQuery()
		s.bridge.close()
		s.queryWG.Wait()
		writerErr := writer.Close()
		s.writerMu.Lock()
		s.writer = nil
		s.writerMu.Unlock()
		if returnErr == nil && writerErr != nil {
			returnErr = i18n.WrapError(i18n.KeySDKWriteOutput, writerErr)
		}
	}()

	// Install the selected permission handler BEFORE any queries start.
	s.runtime.SetPermission(s.initialPermissionHandler())

	// Emit system/init so the client knows the server is ready.
	if err := s.writeMsg(SDKSystemMessage{
		Type:    "system",
		Subtype: "init",
		Message: i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeySDKReady),
	}); err != nil {
		return err
	}

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
		if err := s.dispatch(ctx, append([]byte(nil), line...), lang); err != nil {
			// Log non-fatal dispatch errors as system error messages.
			if writeErr := s.writeMsg(SDKSystemMessage{
				Type:    "system",
				Subtype: "error",
				Message: userFacingError(lang, err),
			}); writeErr != nil {
				return writeErr
			}
		}
	}

	if err := writer.Err(); err != nil {
		return i18n.WrapError(i18n.KeySDKWriteOutput, err)
	}
	if err := s.getAsyncError(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
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
		return s.startUser(ctx, line, lang)
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

func parseSDKUserQuery(line []byte, lang i18n.Language) (sdkUserQuery, error) {
	var msg SDKUserMessage
	if err := json.Unmarshal(line, &msg); err != nil {
		return sdkUserQuery{}, i18n.WrapError(i18n.KeySDKParseUserMessage, err)
	}

	// The Message field is an API-format user message; extract text content.
	text, err := extractText(msg.Message)
	if err != nil {
		return sdkUserQuery{}, i18n.WrapError(i18n.KeySDKExtractMessageText, err)
	}

	if msg.UUID == "" {
		return sdkUserQuery{}, i18n.NewError(i18n.KeySDKUserUUIDRequired)
	}
	return sdkUserQuery{sessionID: msg.SessionID, uuid: msg.UUID, text: text, language: lang}, nil
}

func (s *SDKServer) startUser(ctx context.Context, line []byte, lang i18n.Language) error {
	query, err := parseSDKUserQuery(line, lang)
	if err != nil {
		return err
	}

	queryCtx, cancel := context.WithCancel(ctx)
	active := &sdkActiveQuery{
		sessionID: query.sessionID, uuid: query.uuid, cancel: cancel,
		rejectedUUIDs: make(map[string]struct{}),
	}
	s.queryMu.Lock()
	if current := s.activeQuery; current != nil {
		if current.uuid != query.uuid {
			if _, duplicate := current.rejectedUUIDs[query.uuid]; !duplicate {
				current.rejectedUUIDs[query.uuid] = struct{}{}
				current.rejected = append(current.rejected, query)
			}
		}
		s.queryMu.Unlock()
		cancel()
		return nil
	}
	s.activeQuery = active
	s.queryWG.Add(1)
	s.queryMu.Unlock()

	go func() {
		defer s.queryWG.Done()
		if err := s.runUserQuery(queryCtx, query); err != nil {
			s.failAsync(err)
			s.abandonActiveQuery(active)
			return
		}
		s.finishActiveQuery(active)
	}()
	return nil
}

func (s *SDKServer) runUserQuery(ctx context.Context, query sdkUserQuery) error {
	req := QueryRequest{SessionID: query.sessionID, Message: query.text}

	ch, err := s.runtime.Query(ctx, req)
	if ctx.Err() != nil {
		return s.writeCancelledResult(query)
	}
	if err != nil {
		return s.writeMsg(SDKResultMessage{
			Type:      "result",
			Subtype:   "error_during_execution",
			SessionID: query.sessionID,
			UUID:      query.uuid,
			IsError:   true,
			Errors:    []string{userFacingError(query.language, err)},
		})
	}

	adapter := newEventAdapter(query.sessionID, query.language)
	for {
		if ctx.Err() != nil {
			return s.writeCancelledResultWithAdapter(query, adapter)
		}
		select {
		case <-ctx.Done():
			return s.writeCancelledResultWithAdapter(query, adapter)
		case evt, ok := <-ch:
			if !ok {
				return s.writeMsg(adapter.resultMessage(query.language, query.sessionID, query.uuid, i18n.NewError(i18n.KeySDKStreamEndedWithoutFinalEvent)))
			}
			if ctx.Err() != nil {
				return s.writeCancelledResultWithAdapter(query, adapter)
			}
			if evt.Final {
				sessionID := evt.SessionID
				if sessionID == "" {
					sessionID = query.sessionID
				}
				return s.writeMsg(adapter.resultMessage(query.language, sessionID, query.uuid, evt.Error))
			}
			for _, message := range adapter.process(evt.Event) {
				if err := s.writeMsg(message); err != nil {
					return err
				}
			}
		}
	}
}

func (s *SDKServer) writeCancelledResult(query sdkUserQuery) error {
	return s.writeCancelledResultWithAdapter(query, newEventAdapter(query.sessionID, query.language))
}

func (s *SDKServer) writeCancelledResultWithAdapter(query sdkUserQuery, adapter *eventAdapter) error {
	return s.writeMsg(adapter.resultMessage(
		query.language, query.sessionID, query.uuid, i18n.NewError(i18n.KeySDKQueryCancelled),
	))
}

func (s *SDKServer) finishActiveQuery(active *sdkActiveQuery) {
	active.cancel()
	for {
		s.queryMu.Lock()
		if s.activeQuery != active {
			s.queryMu.Unlock()
			return
		}
		if len(active.rejected) == 0 {
			s.activeQuery = nil
			s.queryMu.Unlock()
			return
		}
		rejected := append([]sdkUserQuery(nil), active.rejected...)
		active.rejected = nil
		s.queryMu.Unlock()

		for _, query := range rejected {
			if err := s.writeMsg(SDKResultMessage{
				Type: "result", Subtype: "error_during_execution",
				SessionID: query.sessionID, UUID: query.uuid, IsError: true,
				Errors: []string{i18n.Text(query.language, i18n.KeySDKQueryAlreadyActive)},
			}); err != nil {
				s.queryMu.Lock()
				if s.activeQuery == active {
					s.activeQuery = nil
				}
				s.queryMu.Unlock()
				s.failAsync(err)
				return
			}
		}
	}
}

// handleControlRequest processes a control_request message.
func (s *SDKServer) handleControlRequest(ctx context.Context, line []byte, lang i18n.Language) error {
	var req SDKControlRequest
	if err := json.Unmarshal(line, &req); err != nil {
		return i18n.WrapError(i18n.KeySDKParseControlRequest, err)
	}
	if req.RequestID == "" {
		return s.sendUncachedControlError("", i18n.Text(lang, i18n.KeySDKControlRequestIDRequired))
	}
	replay, conflict := s.beginControlRequest(req.RequestID, req.Request)
	if conflict {
		return s.sendUncachedControlError(req.RequestID,
			i18n.Format(lang, i18n.KeySDKControlRequestIDConflict, req.RequestID))
	}
	if replay != nil {
		return s.writeMsg(*replay)
	}

	// Peek at subtype.
	var sub requestSubtype
	if err := json.Unmarshal(req.Request, &sub); err != nil {
		return s.sendControlErrorKey(lang, req.RequestID, i18n.KeySDKParseRequestSubtype, err)
	}
	if sub.Subtype != "interrupt" && s.hasActiveQuery() {
		return s.sendControlErrorKey(lang, req.RequestID, i18n.KeySDKControlUnavailableDuringQuery, sub.Subtype)
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
		s.runtime.SetSystemPrompt(initReq.SystemPrompt)
		if err := s.writeMsg(SDKSystemMessage{
			Type:    "system",
			Subtype: "status",
			Message: i18n.Text(lang, i18n.KeySDKSystemPromptReceived),
		}); err != nil {
			return err
		}
	}

	modelID := s.runtime.ModelID()
	respPayload, err := json.Marshal(InitializeResponse{
		Tools:           s.runtime.Tools(),
		Model:           modelID,
		Models:          []string{modelID},
		OutputStyle:     "streamlined",
		AvailableStyles: []string{"streamlined"},
		ProtocolVersion: "1.0",
	})
	if err != nil {
		return s.sendControlErrorKey(lang, req.RequestID, i18n.KeySDKMarshalInitializeResponse, err)
	}

	return s.sendControlSuccess(req.RequestID, json.RawMessage(respPayload))
}

func (s *SDKServer) handleInterrupt(_ context.Context, req SDKControlRequest, lang i18n.Language) error {
	var intReq InterruptRequest
	if err := json.Unmarshal(req.Request, &intReq); err != nil {
		return s.sendControlErrorKey(lang, req.RequestID, i18n.KeySDKParseControlPayload, "interrupt", err)
	}
	s.runtime.Interrupt(intReq.SessionID)
	s.cancelActiveSession(intReq.SessionID)
	return s.sendControlSuccess(req.RequestID, nil)
}

func (s *SDKServer) handleSetModel(req SDKControlRequest, lang i18n.Language) error {
	var smReq SetModelRequest
	if err := json.Unmarshal(req.Request, &smReq); err != nil {
		return s.sendControlErrorKey(lang, req.RequestID, i18n.KeySDKParseControlPayload, "set_model", err)
	}
	if err := s.runtime.SetModel(smReq.SessionID, smReq.Model); err != nil {
		return s.sendControlEngineError(lang, req.RequestID, err)
	}
	return s.sendControlSuccess(req.RequestID, nil)
}

func (s *SDKServer) handleSetPermissionMode(req SDKControlRequest, lang i18n.Language) error {
	var pmReq SetPermissionModeRequest
	if err := json.Unmarshal(req.Request, &pmReq); err != nil {
		return s.sendControlErrorKey(lang, req.RequestID, i18n.KeySDKParseControlPayload, "set_permission_mode", err)
	}

	log.Printf("%s: %q", i18n.Text(lang, i18n.KeyLogSDKPermissionMode), pmReq.Mode)

	// "full-auto" maps to AllowAllHandler; anything else uses the SDK bridge handler.
	if pmReq.Mode == "full-auto" {
		s.runtime.SetPermission(allowAllPermissionHandler{})
	} else {
		s.runtime.SetPermission(s.permissionHandler())
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

	if err := s.runtime.SetThinkingConfig(mttReq.SessionID, enabled, budget); err != nil {
		return s.sendControlEngineError(lang, req.RequestID, err)
	}
	return s.sendControlSuccess(req.RequestID, nil)
}

func (s *SDKServer) handleResume(ctx context.Context, req SDKControlRequest, lang i18n.Language) error {
	var resumeReq ResumeRequest
	if err := json.Unmarshal(req.Request, &resumeReq); err != nil {
		return s.sendControlErrorKey(lang, req.RequestID, i18n.KeySDKParseControlPayload, "resume", err)
	}

	count, err := s.runtime.Resume(ctx, resumeReq.SessionID)
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

	result, err := s.runtime.Compact(ctx, compactReq.SessionID)
	if err != nil {
		return s.sendControlEngineError(lang, req.RequestID, err)
	}
	respPayload, err := json.Marshal(CompactResponse{
		SessionID: compactReq.SessionID, Compacted: result.Compacted,
		BeforeMessageCount: result.BeforeMessageCount, AfterMessageCount: result.AfterMessageCount,
		ContextGeneration: result.ContextGeneration,
	})
	if err != nil {
		return s.sendControlErrorKey(lang, req.RequestID, i18n.KeySDKMarshalCompactResponse, err)
	}
	return s.sendControlSuccess(req.RequestID, json.RawMessage(respPayload))
}

func (s *SDKServer) handleGetContextUsage(req SDKControlRequest, lang i18n.Language) error {
	var cuReq GetContextUsageRequest
	if err := json.Unmarshal(req.Request, &cuReq); err != nil {
		return s.sendControlErrorKey(lang, req.RequestID, i18n.KeySDKParseControlPayload, "get_context_usage", err)
	}

	info, err := s.runtime.ContextUsage(cuReq.SessionID)
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

	// Permission results use the control protocol's success envelope. The outer
	// request ID is the sole correlation authority; the result only carries the
	// decision.
	var cs ControlSuccess
	if err := json.Unmarshal(resp.Response, &cs); err == nil && cs.Subtype == "success" && cs.RequestID != "" {
		var permission PermissionResultMsg
		if err2 := json.Unmarshal(cs.Response, &permission); err2 == nil && validPermissionBehavior(permission.Behavior) {
			s.bridge.deliver(cs.RequestID, permissionResult{
				behavior: permission.Behavior,
			})
			return nil
		}
		// A syntactically recognizable success envelope belongs to this waiter.
		// Resolve it as deny before reporting the protocol violation so malformed
		// input cannot strand a permission Check forever.
		s.bridge.deliver(cs.RequestID, permissionResult{behavior: "deny"})
		return s.writeUnrecognizedControlResponse(lang, resp.Response)
	}

	// An explicit control error is also correlatable and must release the
	// permission waiter as a denial.
	var controlError ControlError
	if err := json.Unmarshal(resp.Response, &controlError); err == nil &&
		controlError.Subtype == "error" && controlError.RequestID != "" {
		s.bridge.deliver(controlError.RequestID, permissionResult{behavior: "deny"})
		return nil
	}
	return s.writeUnrecognizedControlResponse(lang, resp.Response)
}

func validPermissionBehavior(behavior string) bool {
	return behavior == "allow" || behavior == "deny"
}

func (s *SDKServer) writeUnrecognizedControlResponse(lang i18n.Language, payload json.RawMessage) error {
	return s.writeMsg(SDKSystemMessage{
		Type:    "system",
		Subtype: "error",
		Message: i18n.Format(lang, i18n.KeySDKUnrecognizedControlResponsePayload, string(payload)),
	})
}

// ─── helpers ─────────────────────────────────────────────────────────────────

// writeMsg JSON-encodes v and writes it as a single NDJSON line to s.out.
func (s *SDKServer) writeMsg(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return i18n.WrapError(i18n.KeySDKMarshalOutput, err)
	}
	line := append(b, '\n')
	s.writerMu.RLock()
	writer := s.writer
	s.writerMu.RUnlock()
	if writer != nil {
		if err := writer.submit(line); err != nil {
			return i18n.WrapError(i18n.KeySDKWriteOutput, err)
		}
		return nil
	}
	s.directWriteMu.Lock()
	defer s.directWriteMu.Unlock()
	if err := writeSDKLine(s.out, line); err != nil {
		return i18n.WrapError(i18n.KeySDKWriteOutput, err)
	}
	return nil
}

func (s *SDKServer) hasActiveQuery() bool {
	s.queryMu.Lock()
	defer s.queryMu.Unlock()
	return s.activeQuery != nil
}

func (s *SDKServer) cancelActiveSession(sessionID string) {
	s.queryMu.Lock()
	active := s.activeQuery
	if active != nil && (sessionID == "" || active.sessionID == sessionID) {
		active.cancel()
	}
	s.queryMu.Unlock()
}

func (s *SDKServer) cancelActiveQuery() {
	s.queryMu.Lock()
	if s.activeQuery != nil {
		s.activeQuery.cancel()
	}
	s.queryMu.Unlock()
}

func (s *SDKServer) abandonActiveQuery(active *sdkActiveQuery) {
	active.cancel()
	s.queryMu.Lock()
	if s.activeQuery == active {
		s.activeQuery = nil
	}
	s.queryMu.Unlock()
}

func (s *SDKServer) failAsync(err error) {
	if err == nil {
		return
	}
	s.asyncErrMu.Lock()
	if s.asyncErr == nil {
		s.asyncErr = err
	}
	s.asyncErrMu.Unlock()
	s.cancelActiveQuery()
	s.bridge.close()
	s.interruptInput()
}

func (s *SDKServer) getAsyncError() error {
	s.asyncErrMu.Lock()
	defer s.asyncErrMu.Unlock()
	return s.asyncErr
}

func (s *SDKServer) interruptInput() {
	closer, ok := s.in.(io.Closer)
	if !ok {
		return
	}
	s.inputClose.Do(func() { _ = closer.Close() })
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
	return s.writeAndCacheControlResponse(reqID, SDKControlResponse{
		Type:     "control_response",
		Response: json.RawMessage(payload),
	})
}

func (s *SDKServer) sendControlError(reqID, msg string) error {
	return s.sendControlErrorMessage(reqID, msg, true)
}

func (s *SDKServer) sendUncachedControlError(reqID, msg string) error {
	return s.sendControlErrorMessage(reqID, msg, false)
}

func (s *SDKServer) sendControlErrorMessage(reqID, msg string, cache bool) error {
	payload, err := json.Marshal(ControlError{
		Subtype:   "error",
		RequestID: reqID,
		Error:     msg,
	})
	if err != nil {
		return err
	}
	response := SDKControlResponse{
		Type:     "control_response",
		Response: json.RawMessage(payload),
	}
	if !cache {
		return s.writeMsg(response)
	}
	return s.writeAndCacheControlResponse(reqID, response)
}

func (s *SDKServer) beginControlRequest(requestID string, request json.RawMessage) (*SDKControlResponse, bool) {
	fingerprint := canonicalControlFingerprint(request)
	s.controlMu.Lock()
	defer s.controlMu.Unlock()
	if previous, ok := s.controls[requestID]; ok {
		if previous.request != fingerprint {
			return nil, true
		}
		response := previous.response
		response.Response = append(json.RawMessage(nil), previous.response.Response...)
		return &response, false
	}
	s.controls[requestID] = sdkControlHistory{request: fingerprint}
	return nil, false
}

func canonicalControlFingerprint(request json.RawMessage) string {
	decoder := json.NewDecoder(bytes.NewReader(request))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return string(request)
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return string(request)
	}
	return string(canonical)
}

func (s *SDKServer) writeAndCacheControlResponse(requestID string, response SDKControlResponse) error {
	if err := s.writeMsg(response); err != nil {
		return err
	}
	s.controlMu.Lock()
	history := s.controls[requestID]
	history.response = response
	history.response.Response = append(json.RawMessage(nil), response.Response...)
	s.controls[requestID] = history
	s.controlMu.Unlock()
	return nil
}

func (s *SDKServer) sendControlErrorKey(lang i18n.Language, reqID string, key i18n.Key, args ...any) error {
	return s.sendControlError(reqID, i18n.Format(lang, key, args...))
}

func (s *SDKServer) sendControlEngineError(lang i18n.Language, reqID string, err error) error {
	return s.sendControlError(reqID, userFacingError(lang, err))
}

// newRequestID returns a unique request ID string.
func (s *SDKServer) newRequestID() string {
	id := atomic.AddUint64(&s.counter, 1)
	return fmt.Sprintf("req-%d", id)
}

// permissionHandler returns a PermissionHandler wired to s.bridge.
// Use this when constructing a query that should honour SDK-side permission.
func (s *SDKServer) permissionHandler() PermissionHandler {
	return &sdkPermissionHandler{
		bridge:      s.bridge,
		sendFn:      s.writeMsg,
		newReqID:    s.newRequestID,
		getApproval: s.getToolApproval,
	}
}

func (s *SDKServer) initialPermissionHandler() PermissionHandler {
	if s.initialPermissionMode == InitialPermissionFullAuto {
		return allowAllPermissionHandler{}
	}
	return s.permissionHandler()
}

// ─── text extraction helper ───────────────────────────────────────────────────

// extractText pulls plain text out of the canonical API-format user message.
func extractText(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "null" {
		return "", i18n.NewError(i18n.KeySDKUnsupportedMessageContent)
	}
	var msg struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &msg); err != nil {
		return "", err
	}
	if msg.Role != "user" || len(msg.Content) == 0 || string(msg.Content) == "null" {
		return "", i18n.NewError(i18n.KeySDKUnsupportedMessageContent)
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
			if b.Type != "text" {
				return "", i18n.NewError(i18n.KeySDKUnsupportedMessageContent)
			}
			out += b.Text
		}
		return out, nil
	}
	return "", i18n.NewError(i18n.KeySDKUnsupportedMessageContent)
}
