package inspect

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/agent-dance/luban/i18n"
	executioncontract "github.com/agent-dance/luban/internal/contracts/execution"
	toolfile "github.com/agent-dance/luban/internal/tools/file"
	toolsearch "github.com/agent-dance/luban/internal/tools/search"
	"github.com/agent-dance/luban/internal/tools/toolbase"
	"github.com/agent-dance/luban/types"
)

type limits struct {
	maxChars   int
	maxFiles   int
	maxMatches int
}

type normalizedRange struct {
	start int
	end   int
}

type normalizedRequest struct {
	id         string
	kind       string
	path       string
	pattern    string
	ranges     []normalizedRange
	context    int
	maxResults int
}

type validatedInput struct {
	requests []normalizedRequest
	cursor   string
	limits   limits
}

type workspaceSnapshot struct {
	root                   string
	sessionID              string
	runID                  string
	cacheLineageID         string
	historyEpoch           string
	historyBound           bool
	workspaceRevision      uint64
	workspaceRevisionBound bool
	deferEvidenceCommit    bool
	runtime                types.ToolRuntimeContext
}

type immutableRuntimeProvider struct {
	runtime types.ToolRuntimeContext
}

func (p immutableRuntimeProvider) ToolRuntimeContext() types.ToolRuntimeContext {
	return cloneRuntimeContext(p.runtime)
}

func cloneRuntimeContext(runtime types.ToolRuntimeContext) types.ToolRuntimeContext {
	out := runtime
	out.AllowedDirs = append([]string(nil), runtime.AllowedDirs...)
	out.AllowedRules = append([]types.PermissionRuleValue(nil), runtime.AllowedRules...)
	out.DeniedRules = append([]types.PermissionRuleValue(nil), runtime.DeniedRules...)
	out.AskRules = append([]types.PermissionRuleValue(nil), runtime.AskRules...)
	out.AllowedTools = cloneBoolMap(runtime.AllowedTools)
	out.DeniedTools = cloneBoolMap(runtime.DeniedTools)
	out.Features = cloneBoolMap(runtime.Features)
	return out
}

func cloneBoolMap(values map[string]bool) map[string]bool {
	if values == nil {
		return nil
	}
	out := make(map[string]bool, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func (t *Tool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	validated, err := t.validateInput(input)
	if err != nil {
		return resultError(err), nil
	}
	workspace, err := t.workspaceSnapshot(ctx, types.ToolRuntimeContext{})
	if err != nil {
		return resultError(err), nil
	}
	if validated.cursor != "" {
		return t.continuePage(ctx, validated.cursor, workspace)
	}

	generation := t.cursorStore().generation()
	batch := t.executeBatch(ctx, workspace, generation, validated.requests)
	state := newPaginationState(batch, validated.limits)
	return t.renderPage(ctx, workspace, generation, state)
}

func (t *Tool) continuePage(ctx context.Context, cursor string, workspace workspaceSnapshot) (types.ToolResult, error) {
	entry, ok := t.cursorStore().consume(cursor, workspace)
	if !ok {
		return resultError(localizedError(i18n.KeyToolInspectCursorInvalid)), nil
	}
	return t.renderPage(ctx, workspace, entry.generation, entry.state)
}

func (t *Tool) cursorStore() *cursorStore {
	if t.cursors == nil {
		t.cursors = newCursorStore()
	}
	return t.cursors
}

func (t *Tool) validateInput(input map[string]any) (validatedInput, error) {
	if err := types.ValidateToolInput(t, input); err != nil {
		return validatedInput{}, i18n.WrapError(i18n.KeyToolInspectInvalidInput, err)
	}
	decoded, err := types.DecodeStrictToolInput[Input](input)
	if err != nil {
		return validatedInput{}, i18n.WrapInternalError(i18n.KeyToolInspectMalformedInput, err)
	}
	operation := decoded.Operation
	switch operation.Mode {
	case ModeContinue:
		return validatedInput{cursor: strings.TrimSpace(operation.Cursor)}, nil
	case ModeNew:
		// Continue below. JSON Schema admission already guarantees that requests
		// and page belong only to this branch.
	default:
		return validatedInput{}, localizedError(i18n.KeyToolInspectMalformedInput)
	}
	if len(operation.Requests) == 0 {
		return validatedInput{}, localizedError(i18n.KeyToolInspectRequestsRequired)
	}
	if len(operation.Requests) > maximumRequests {
		return validatedInput{}, localizedError(i18n.KeyToolInspectTooManyRequests, maximumRequests)
	}

	pageLimits := limits{maxChars: defaultMaxChars, maxFiles: defaultMaxFiles, maxMatches: defaultMaxMatches}
	if operation.Page != nil && operation.Page.MaxChars != nil {
		pageLimits.maxChars, err = validateIntegerLimit("max_chars", *operation.Page.MaxChars, minimumMaxChars, maximumMaxChars)
		if err != nil {
			return validatedInput{}, err
		}
	}
	if pageLimits.maxChars > maximumModelVisibleChars {
		pageLimits.maxChars = maximumModelVisibleChars
	}
	if operation.Page != nil && operation.Page.MaxFiles != nil {
		pageLimits.maxFiles, err = validateIntegerLimit("max_files", *operation.Page.MaxFiles, 1, maximumMaxFiles)
		if err != nil {
			return validatedInput{}, err
		}
	}
	if operation.Page != nil && operation.Page.MaxMatches != nil {
		pageLimits.maxMatches, err = validateIntegerLimit("max_matches", *operation.Page.MaxMatches, 1, maximumMaxMatches)
		if err != nil {
			return validatedInput{}, err
		}
	}

	requests := make([]normalizedRequest, 0, len(operation.Requests))
	seenIDs := make(map[string]struct{}, len(operation.Requests))
	for _, request := range operation.Requests {
		normalized, normalizeErr := normalizeRequest(request)
		if normalizeErr != nil {
			return validatedInput{}, normalizeErr
		}
		if _, exists := seenIDs[normalized.id]; exists {
			return validatedInput{}, localizedError(i18n.KeyToolInspectDuplicateRequestID, normalized.id)
		}
		seenIDs[normalized.id] = struct{}{}
		requests = append(requests, normalized)
	}
	return validatedInput{requests: requests, limits: pageLimits}, nil
}

func normalizeRequest(request Request) (normalizedRequest, error) {
	id := strings.TrimSpace(request.ID)
	if id == "" {
		return normalizedRequest{}, localizedError(i18n.KeyToolInspectRequestIDRequired)
	}
	if len(id) > maximumRequestID {
		return normalizedRequest{}, localizedError(i18n.KeyToolInspectValueTooLong, "id", maximumRequestID)
	}
	kind := strings.ToLower(strings.TrimSpace(request.Kind))
	if kind != KindRead && kind != KindSearch && kind != KindGlob {
		return normalizedRequest{}, localizedError(i18n.KeyToolInspectUnsupportedKind, request.Kind)
	}
	path := strings.TrimSpace(request.Path)
	if path == "" {
		path = "."
	}
	if len(path) > maximumPath {
		return normalizedRequest{}, localizedError(i18n.KeyToolInspectValueTooLong, "path", maximumPath)
	}
	pattern := strings.TrimSpace(request.Pattern)
	if len(pattern) > maximumPattern {
		return normalizedRequest{}, localizedError(i18n.KeyToolInspectValueTooLong, "pattern", maximumPattern)
	}
	if (kind == KindSearch || kind == KindGlob) && pattern == "" {
		return normalizedRequest{}, localizedError(i18n.KeyToolSearchPatternRequired)
	}
	if len(request.Ranges) > maximumRanges {
		return normalizedRequest{}, localizedError(i18n.KeyToolInspectTooManyRanges, maximumRanges)
	}
	ranges := make([]normalizedRange, 0, len(request.Ranges))
	for _, lineRange := range request.Ranges {
		start, startOK := exactInteger(lineRange.Start)
		end, endOK := exactInteger(lineRange.End)
		if !startOK || !endOK || start < 1 || end < start || end > maximumLineNumber || end-start+1 > maximumRangeLineSpan {
			return normalizedRequest{}, localizedError(i18n.KeyToolInspectInvalidRange, start, end)
		}
		ranges = append(ranges, normalizedRange{start: start, end: end})
	}
	ranges = mergeRanges(ranges)

	contextLines := 0
	if request.Context != nil {
		var ok bool
		contextLines, ok = exactInteger(*request.Context)
		if !ok || contextLines < 0 || contextLines > maximumContext {
			return normalizedRequest{}, localizedError(i18n.KeyToolInspectContextOutOfRange, maximumContext)
		}
	}
	maxResults := defaultMaxResults
	if request.MaxResults != nil {
		var ok bool
		maxResults, ok = exactInteger(*request.MaxResults)
		if !ok || maxResults < 1 || maxResults > maximumMaxResults {
			return normalizedRequest{}, localizedError(i18n.KeyToolInspectMaxResultsOutOfRange, maximumMaxResults)
		}
	}
	return normalizedRequest{
		id: id, kind: kind, path: path, pattern: pattern, ranges: ranges,
		context: contextLines, maxResults: maxResults,
	}, nil
}

func validateIntegerLimit(field string, value float64, minimum, maximum int) (int, error) {
	integer, ok := exactInteger(value)
	if !ok || integer < minimum || integer > maximum {
		return 0, localizedError(i18n.KeyToolInspectLimitOutOfRange, field, minimum, maximum)
	}
	return integer, nil
}

func exactInteger(value float64) (int, bool) {
	if math.IsNaN(value) || math.IsInf(value, 0) || math.Trunc(value) != value {
		return 0, false
	}
	converted := int(value)
	if float64(converted) != value {
		return 0, false
	}
	return converted, true
}

func (t *Tool) workspaceSnapshot(ctx context.Context, supplied types.ToolRuntimeContext) (workspaceSnapshot, error) {
	runtime := cloneRuntimeContext(supplied)
	var execution executioncontract.ToolExecutionContext
	hasExecution := false
	if ctx != nil {
		execution, hasExecution = executioncontract.ToolExecutionContextFromContext(ctx)
	}
	if strings.TrimSpace(runtime.ProjectRoot) == "" && t != nil && t.runtime != nil {
		runtime = cloneRuntimeContext(t.runtime.ToolRuntimeContext())
	}
	root := strings.TrimSpace(runtime.ProjectRoot)
	if hasExecution {
		if root == "" {
			root = strings.TrimSpace(execution.ProjectRoot)
		}
		if runtime.SessionID == "" {
			runtime.SessionID = execution.SessionID
		}
		if runtime.SessionID != execution.SessionID && execution.SessionID != "" {
			return workspaceSnapshot{}, localizedError(i18n.KeyToolInspectProjectRootUnavailable)
		}
	}
	if root == "" {
		var err error
		root, err = os.Getwd()
		if err != nil {
			return workspaceSnapshot{}, localizedError(i18n.KeyToolInspectProjectRootUnavailable)
		}
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return workspaceSnapshot{}, localizedError(i18n.KeyToolInspectProjectRootUnavailable)
	}
	absRoot = filepath.Clean(absRoot)
	if resolved, resolveErr := filepath.EvalSymlinks(absRoot); resolveErr == nil {
		absRoot = filepath.Clean(resolved)
	}
	info, err := os.Stat(absRoot)
	if err != nil || !info.IsDir() {
		return workspaceSnapshot{}, localizedError(i18n.KeyToolInspectProjectRootUnavailable)
	}
	runtime.ProjectRoot = absRoot
	// Inspect deliberately narrows broader session permissions to the exact
	// repository root. The underlying Read/Grep/Glob implementations perform a
	// second descriptor/resolved-path check against this immutable slice.
	runtime.AllowedDirs = []string{absRoot}
	historyEpoch, historyBound := "local", true
	deferEvidenceCommit := false
	if hasExecution {
		historyEpoch, historyBound = execution.ActiveReadEvidenceScope()
		deferEvidenceCommit = historyBound
	}
	workspaceRevision, workspaceRevisionBound := uint64(0), false
	if t != nil && t.WorkspaceRevisions != nil {
		if epoch, ok := t.WorkspaceRevisions.CurrentEpoch(absRoot); ok {
			workspaceRevision, workspaceRevisionBound = uint64(epoch), true
		}
	}
	return workspaceSnapshot{
		root: absRoot, sessionID: runtime.SessionID,
		runID: execution.RunID, cacheLineageID: execution.CacheLineageID,
		historyEpoch: historyEpoch, historyBound: historyBound,
		workspaceRevision: workspaceRevision, workspaceRevisionBound: workspaceRevisionBound,
		deferEvidenceCommit: deferEvidenceCommit,
		runtime:             runtime,
	}, nil
}

func resolveRepositoryPath(root, raw string) (string, error) {
	path := strings.TrimSpace(raw)
	if path == "" || path == "." {
		return root, nil
	}
	if strings.ContainsRune(path, '\x00') || isUNCPath(path) {
		return "", localizedError(i18n.KeyToolInspectPathOutsideRepository, raw)
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", localizedError(i18n.KeyToolInspectPathOutsideRepository, raw)
	}
	absPath = filepath.Clean(absPath)
	if !toolbase.PathWithinAllowedDirs(absPath, []string{root}) {
		return "", localizedError(i18n.KeyToolInspectPathOutsideRepository, raw)
	}
	if resolved, resolveErr := filepath.EvalSymlinks(absPath); resolveErr == nil {
		resolved = filepath.Clean(resolved)
		if !toolbase.PathWithinAllowedDirs(resolved, []string{root}) {
			return "", localizedError(i18n.KeyToolInspectPathOutsideRepository, raw)
		}
		absPath = resolved
	}
	return absPath, nil
}

func isUNCPath(path string) bool {
	return len(path) >= 2 && ((path[0] == '\\' && path[1] == '\\') || (path[0] == '/' && path[1] == '/'))
}

func (t *Tool) CheckPermissions(ctx context.Context, input map[string]any, request types.ToolPermissionRequest) (types.ToolPermissionResult, error) {
	validated, err := t.validateInput(input)
	if err != nil {
		return types.ToolPermissionResult{Behavior: types.PermissionBehaviorDeny, Message: err.Error(), Required: true}, nil
	}
	if validated.cursor != "" {
		return types.ToolPermissionResult{Behavior: types.PermissionBehaviorAllow, UpdatedInput: input}, nil
	}
	workspace, err := t.workspaceSnapshot(ctx, request.Runtime)
	if err != nil {
		return types.ToolPermissionResult{Behavior: types.PermissionBehaviorDeny, Message: err.Error(), Required: true}, nil
	}
	permissionRequest := request
	permissionRequest.Runtime = workspace.runtime
	provider := immutableRuntimeProvider{runtime: workspace.runtime}
	for _, inspectRequest := range validated.requests {
		if _, pathErr := resolveRepositoryPath(workspace.root, inspectRequest.path); pathErr != nil {
			return types.ToolPermissionResult{Behavior: types.PermissionBehaviorDeny, Message: pathErr.Error(), Required: true}, nil
		}
		var underlying types.Tool
		var underlyingInput map[string]any
		switch inspectRequest.kind {
		case KindRead:
			underlying = &toolfile.FileReadTool{Runtime: provider, ReadState: t.readState}
			underlyingInput = map[string]any{"file_path": inspectRequest.path}
		case KindSearch:
			underlying = toolsearch.NewGrep(provider)
			underlyingInput = map[string]any{"path": inspectRequest.path, "pattern": inspectRequest.pattern}
		case KindGlob:
			underlying = toolsearch.NewGlob(provider)
			underlyingInput = map[string]any{"path": inspectRequest.path, "pattern": inspectRequest.pattern}
		}
		checker, ok := underlying.(types.ToolPermissionChecker)
		if !ok {
			continue
		}
		decision, checkErr := checker.CheckPermissions(ctx, underlyingInput, permissionRequest)
		if checkErr != nil {
			return types.ToolPermissionResult{}, checkErr
		}
		if decision.Behavior == types.PermissionBehaviorDeny || decision.Behavior == types.PermissionBehaviorAsk {
			return decision, nil
		}
	}
	return types.ToolPermissionResult{Behavior: types.PermissionBehaviorAllow, UpdatedInput: input}, nil
}
