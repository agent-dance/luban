package agent

// agent_runtime_hooks.go contains the small helpers wired into AgentTool.Execute
// to satisfy the alignment audit contract:
//
//   - agent-05: checkIsolationSupported() converts an isolation mode + the
//     locally-available providers into a definitive yes/no decision before any
//     provider call is made. The resulting error message always mentions
//     "isolation" or "worktree" so the caller can surface a precise diagnostic.
//   - agent-09: openAgentTranscriptWriter() / writeAgentTranscriptEvent()
//     append one JSON object per line to the file pointed to by the
//     LUBAN_AGENT_TRANSCRIPT environment variable. The writer is best-effort:
//     errors opening the file are swallowed so a missing parent directory or a
//     read-only filesystem never prevents an Execute call from running.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	executioncontract "github.com/agent-dance/luban/internal/contracts/execution"

	"github.com/agent-dance/luban/i18n"
	storepaths "github.com/agent-dance/luban/internal/store/paths"
	runtimestore "github.com/agent-dance/luban/internal/store/runtime"
	"github.com/agent-dance/luban/internal/store/secureio"
	"github.com/agent-dance/luban/types"
)

// checkIsolationSupported validates an isolation mode against the locally
// configured providers. Returns ok=true when the mode is supported, in which
// case the returned ToolResult is the zero value. Returns ok=false with an
// IsError ToolResult when the mode is rejected.
func (t *AgentTool) checkIsolationSupported(_ context.Context, isolation string) (types.ToolResult, bool) {
	mode := strings.ToLower(strings.TrimSpace(isolation))
	switch mode {
	case "", "none":
		return types.ToolResult{}, true
	case "worktree":
		// A local provider is required to run the child in a worktree.
		if t == nil || t.Provider == nil {
			return types.ToolResult{
				Content: toolRuntimeText(i18n.KeyToolAgentWorktreeUnavailable),
				IsError: true,
			}, false
		}
		return types.ToolResult{}, true
	default:
		return types.ToolResult{
			Content: toolRuntimeFormat(i18n.KeyToolAgentIsolationUnsupported, isolation),
			IsError: true,
		}, false
	}
}

// agentTranscriptMu guards concurrent appends to the JSONL transcript file.
// Multiple parallel Agent calls share a single LUBAN_AGENT_TRANSCRIPT path in
// practice; serialising the writes keeps each emitted line intact.
var agentTranscriptMu sync.Mutex

var (
	agentTranscriptProcessNamespace = newAgentTranscriptProcessNamespace()
	agentTranscriptRunSequence      atomic.Uint64
)

func newAgentTranscriptProcessNamespace() string {
	var random [8]byte
	if _, err := rand.Read(random[:]); err == nil {
		return fmt.Sprintf("%d-%s", os.Getpid(), hex.EncodeToString(random[:]))
	}
	return fmt.Sprintf("%d-%d", os.Getpid(), time.Now().UnixNano())
}

func nextAgentTranscriptRunSuffix() string {
	return fmt.Sprintf("%s-%d", agentTranscriptProcessNamespace, agentTranscriptRunSequence.Add(1))
}

type agentTranscriptWriterContextKey struct{}

type agentTranscriptIdentity struct {
	SessionID    string
	ProjectRoot  string
	ContextEpoch string
	ActorID      string
	ActorType    string
	RunID        string
}

type agentTranscriptFileWriter struct {
	fileMu sync.Mutex
	file   *os.File
	mu     sync.RWMutex
	id     agentTranscriptIdentity
}

func (w *agentTranscriptFileWriter) Write(data []byte) (int, error) {
	if w == nil {
		return 0, io.ErrClosedPipe
	}
	w.fileMu.Lock()
	defer w.fileMu.Unlock()
	if w.file == nil {
		return 0, io.ErrClosedPipe
	}
	return w.file.Write(data)
}

func (w *agentTranscriptFileWriter) Name() string {
	if w == nil {
		return ""
	}
	w.fileMu.Lock()
	defer w.fileMu.Unlock()
	if w.file == nil {
		return ""
	}
	return w.file.Name()
}

func (w *agentTranscriptFileWriter) Sync() error {
	if w == nil {
		return nil
	}
	w.fileMu.Lock()
	defer w.fileMu.Unlock()
	if w.file == nil {
		return nil
	}
	return w.file.Sync()
}

func (w *agentTranscriptFileWriter) Close() error {
	if w == nil {
		return nil
	}
	agentTranscriptMu.Lock()
	defer agentTranscriptMu.Unlock()
	w.fileMu.Lock()
	defer w.fileMu.Unlock()
	if w.file == nil {
		return nil
	}
	f := w.file
	w.file = nil
	syncErr := f.Sync()
	closeErr := f.Close()
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}

func (w *agentTranscriptFileWriter) bindContext(ctx context.Context) {
	if w == nil || ctx == nil {
		return
	}
	execContext, ok := executioncontract.ToolExecutionContextFromContext(ctx)
	if !ok {
		return
	}
	w.mu.Lock()
	if strings.TrimSpace(w.id.SessionID) == "" || w.id.SessionID == "unscoped" {
		w.id.SessionID = strings.TrimSpace(execContext.SessionID)
	}
	if strings.TrimSpace(w.id.ContextEpoch) == "" || strings.HasPrefix(w.id.ContextEpoch, "run:") {
		if epoch := agentTranscriptContextEpoch(ctx); epoch != "" {
			w.id.ContextEpoch = epoch
		}
	}
	if strings.TrimSpace(w.id.RunID) == "" {
		w.id.RunID = strings.TrimSpace(execContext.RunID)
	}
	if strings.TrimSpace(w.id.ActorID) == "" {
		w.id.ActorID = strings.TrimSpace(execContext.ActorID)
	}
	if strings.TrimSpace(w.id.ActorType) == "" {
		w.id.ActorType = strings.TrimSpace(execContext.ActorType)
	}
	w.normalizeIdentityLocked()
	w.mu.Unlock()
}

func agentTranscriptContextEpoch(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	execContext, ok := executioncontract.ToolExecutionContextFromContext(ctx)
	if !ok {
		return ""
	}
	scope, active := execContext.ActiveReadEvidenceScope()
	if !active {
		return ""
	}
	separator := strings.LastIndexByte(scope, '\x1f')
	if separator < 0 || separator+1 >= len(scope) {
		return ""
	}
	return scope[separator+1:]
}

func (w *agentTranscriptFileWriter) normalizeIdentityLocked() {
	if strings.TrimSpace(w.id.SessionID) == "" {
		w.id.SessionID = "unscoped"
	}
	if strings.TrimSpace(w.id.RunID) == "" {
		w.id.RunID = "unscoped"
	}
	if strings.TrimSpace(w.id.ContextEpoch) == "" {
		w.id.ContextEpoch = "run:" + w.id.RunID
	}
	if strings.TrimSpace(w.id.ActorID) == "" {
		w.id.ActorID = w.id.RunID
	}
	if strings.TrimSpace(w.id.ActorType) == "" {
		w.id.ActorType = "agent"
	}
}

func (w *agentTranscriptFileWriter) identity() agentTranscriptIdentity {
	if w == nil {
		return agentTranscriptIdentity{}
	}
	w.mu.Lock()
	w.normalizeIdentityLocked()
	id := w.id
	w.mu.Unlock()
	return id
}

func withAgentTranscriptWriter(ctx context.Context, writer io.Writer) context.Context {
	if writer == nil {
		return ctx
	}
	if transcript, ok := writer.(*agentTranscriptFileWriter); ok {
		transcript.bindContext(ctx)
	}
	return context.WithValue(ctx, agentTranscriptWriterContextKey{}, writer)
}

func agentTranscriptWriterFromContext(ctx context.Context) io.Writer {
	if ctx == nil {
		return nil
	}
	writer, _ := ctx.Value(agentTranscriptWriterContextKey{}).(io.Writer)
	return writer
}

// openAgentTranscriptWriterForRun allocates the persistent JSONL sidechain for
// one agent. An explicit LUBAN_AGENT_TRANSCRIPT path remains supported for
// harnesses; normal runs use external private runtime storage so read-only
// project work does not dirty the repository.
func openAgentTranscriptWriterForRun(agentID string) (io.Writer, func()) {
	return openAgentTranscriptWriterForRunIdentity(agentID, agentTranscriptIdentity{
		ActorID: agentID, ActorType: "agent", RunID: agentID,
	})
}

func openAgentTranscriptWriterForRunIdentity(runID string, identity agentTranscriptIdentity) (io.Writer, func()) {
	path := agentTranscriptPathForRunIdentity(runID, identity)
	if path == "" {
		return nil, nil
	}
	var (
		f   *os.File
		err error
	)
	if strings.TrimSpace(os.Getenv("LUBAN_AGENT_TRANSCRIPT")) != "" {
		// An explicit override may point at a user-managed harness directory.
		// Validate it but never take ownership by changing its mode.
		f, err = secureio.OpenPrivateRuntimeAppendFileWithoutDirectoryMutation(path)
	} else {
		managedRoot := defaultAgentTranscriptRoot(identity.ProjectRoot, identity.SessionID)
		if err = secureio.EnsurePrivateRuntimeDirectory(managedRoot); err != nil {
			return nil, nil
		}
		f, err = secureio.OpenPrivateRuntimeAppendFile(path)
	}
	if err != nil {
		return nil, nil
	}
	identity.RunID = firstNonEmpty(identity.RunID, runID)
	identity.ActorID = firstNonEmpty(identity.ActorID, runID)
	w := &agentTranscriptFileWriter{file: f, id: identity}
	w.mu.Lock()
	w.normalizeIdentityLocked()
	w.mu.Unlock()
	return w, func() {
		_ = w.Close()
	}
}

func agentTranscriptPathForRun(runID string) string {
	return agentTranscriptPathForRunIdentity(runID, agentTranscriptIdentity{})
}

func agentTranscriptPathForRunIdentity(runID string, identity agentTranscriptIdentity) string {
	if path := strings.TrimSpace(os.Getenv("LUBAN_AGENT_TRANSCRIPT")); path != "" {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path || filepath.Base(path) == "." || filepath.Base(path) == string(os.PathSeparator) {
			return ""
		}
		id := runtimestore.SafeTaskPathComponent(strings.TrimSpace(runID))
		if id == "" {
			return path
		}
		extension := filepath.Ext(path)
		stem := strings.TrimSuffix(path, extension)
		return stem + "." + id + extension
	}
	id := runtimestore.SafeTaskPathComponent(strings.TrimSpace(runID))
	if id == "" {
		id = fmt.Sprintf("agent-%d", time.Now().UnixNano())
	}
	return filepath.Join(defaultAgentTranscriptRoot(identity.ProjectRoot, identity.SessionID), id+".jsonl")
}

func defaultAgentTranscriptRoot(projectRoot, sessionID string) string {
	root := strings.TrimSpace(projectRoot)
	if root == "" {
		root = "."
		if current, err := os.Getwd(); err == nil && strings.TrimSpace(current) != "" {
			root = current
		}
	}
	if strings.TrimSpace(sessionID) == "" {
		sessionID = strings.TrimSpace(os.Getenv("LUBAN_SESSION_ID"))
	}
	return filepath.Join(
		storepaths.RuntimeSessionDir(root, sessionID),
		"agent-transcripts",
		agentTranscriptProcessNamespace,
	)
}

func writeAgentTranscriptRecord(w io.Writer, record any) error {
	if w == nil {
		return nil
	}
	data, err := marshalAgentTranscriptRecord(w, record)
	if err != nil {
		return err
	}
	line := make([]byte, 0, len(data)+1)
	line = append(line, data...)
	line = append(line, '\n')
	agentTranscriptMu.Lock()
	defer agentTranscriptMu.Unlock()
	if err := secureio.WriteAll(w, line); err != nil {
		return err
	}
	if syncer, ok := w.(interface{ Sync() error }); ok {
		return syncer.Sync()
	}
	return nil
}

func marshalAgentTranscriptRecord(w io.Writer, record any) ([]byte, error) {
	data, err := json.Marshal(record)
	if err != nil {
		return nil, err
	}
	transcript, ok := w.(*agentTranscriptFileWriter)
	if !ok {
		return data, nil
	}
	var envelope map[string]any
	if err := json.Unmarshal(data, &envelope); err != nil || envelope == nil {
		return data, err
	}
	identity := transcript.identity()
	envelope["sessionId"] = identity.SessionID
	envelope["contextEpoch"] = identity.ContextEpoch
	envelope["actorId"] = identity.ActorID
	envelope["actorType"] = identity.ActorType
	envelope["runId"] = identity.RunID
	return json.Marshal(envelope)
}
