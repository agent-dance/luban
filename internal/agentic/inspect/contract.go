// Package inspect implements the repository-scoped composite Inspect tool.
package inspect

import (
	"sort"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/contracts/compactproof"
	"github.com/agent-dance/luban/internal/contracts/toolmeta"
	"github.com/agent-dance/luban/internal/contracts/workspacerevision"
	toolfile "github.com/agent-dance/luban/internal/tools/file"
	"github.com/agent-dance/luban/internal/tools/toolbase"
	"github.com/agent-dance/luban/types"
)

const (
	KindRead     = "read"
	KindSearch   = "search"
	KindGlob     = "glob"
	ModeNew      = "new"
	ModeContinue = "continue"
)

const (
	defaultMaxChars             = 12_000
	defaultMaxFiles             = 64
	defaultMaxMatches           = 200
	defaultMaxResults           = 50
	minimumMaxChars             = 8_192
	maximumMaxChars             = 100_000
	maximumMaxFiles             = 500
	maximumMaxMatches           = 1_000
	maximumMaxResults           = 500
	maximumRequests             = 16
	maximumRanges               = 64
	maximumContext              = 20
	maximumRequestID            = 128
	maximumPath                 = 512
	maximumPattern              = 4_096
	maximumLineNumber           = 10_000_000
	maximumRangeLineSpan        = 10_000
	maximumErrorBytes           = 512
	maximumSnippetBytes         = 1_024
	maximumAtomicSearchSnippets = 4
	// Keep every Inspect page below the query loop's generic 15K tool-result
	// budget. Otherwise that later budget cuts the JSON tail, including the
	// cursor, and turns a deliberately paginated result into an invalid prefix.
	maximumModelVisibleChars = 14_000
)

type Input struct {
	Operation Operation `json:"operation"`
}

// Operation is an explicitly discriminated input branch. New inspections and
// cursor continuations deliberately do not share a flat bag of optional
// properties: providers can project either branch without inventing inert
// placeholders for the other branch.
type Operation struct {
	Mode     string      `json:"mode"`
	Requests []Request   `json:"requests,omitempty"`
	Page     *PageLimits `json:"page,omitempty"`
	Cursor   string      `json:"cursor,omitempty"`
}

type PageLimits struct {
	MaxChars   *float64 `json:"max_chars,omitempty"`
	MaxFiles   *float64 `json:"max_files,omitempty"`
	MaxMatches *float64 `json:"max_matches,omitempty"`
}

type Request struct {
	ID         string      `json:"id"`
	Kind       string      `json:"kind"`
	Path       string      `json:"path,omitempty"`
	Pattern    string      `json:"pattern,omitempty"`
	Ranges     []LineRange `json:"ranges,omitempty"`
	Context    *float64    `json:"context,omitempty"`
	MaxResults *float64    `json:"max_results,omitempty"`
}

type LineRange struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

type Result struct {
	Generation        string          `json:"generation"`
	Requests          []RequestResult `json:"requests"`
	Snippets          []Snippet       `json:"snippets,omitempty"`
	HasMoreView       bool            `json:"has_more_view"`
	SourceTruncated   bool            `json:"source_truncated"`
	OmittedRequestIDs []string        `json:"omitted_request_ids,omitempty"`
	Cursor            string          `json:"cursor,omitempty"`
	Stats             ResultStats     `json:"stats"`

	// modelContent is the exact compact wire view already accounted for by the
	// evidence ledger. Re-encoding Result in the mapper would incorrectly turn
	// evidence first exposed by this result into a same-result reference.
	modelContent   string
	modelStats     modelResultStats
	visibleReceipt *visibleEvidenceReceipt
}

// CompactionProof exposes bounded repository-observation facts without source
// snippets, match text, paths, cursor authority, or raw errors.
func (r Result) CompactionProof() compactproof.Proof {
	errorCodes := make(map[string]struct{})
	partialReasons := make(map[string]struct{})
	for _, request := range r.Requests {
		for _, requestErr := range request.Errors {
			if requestErr.Code != "" {
				errorCodes[requestErr.Code] = struct{}{}
			}
		}
		if request.PartialReason != "" {
			partialReasons[request.PartialReason] = struct{}{}
		}
	}
	proof := compactproof.Proof{Inspect: &compactproof.InspectProof{
		Requests: r.Stats.Requests, Files: r.Stats.Files, Matches: r.Stats.Matches,
		Snippets: r.Stats.Snippets, Items: r.Stats.Items,
		HasMoreView: r.HasMoreView, SourceTruncated: r.SourceTruncated,
		OmittedRequests: len(r.OmittedRequestIDs),
		ErrorCodes:      mapKeysSorted(errorCodes), PartialReasonCodes: mapKeysSorted(partialReasons),
	}}
	if r.Generation != "" {
		proof.Revision = &compactproof.RevisionProof{Status: "observed", Generation: r.Generation}
	}
	return proof
}

func mapKeysSorted(values map[string]struct{}) []string {
	if len(values) == 0 {
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// CommitVisibleReadEvidence is consumed only by the runtime after the exact
// rendered result has entered model-visible history. Direct Execute callers
// are committed synchronously before return.
func (r Result) CommitVisibleReadEvidence(visibleContent string) bool {
	return r.visibleReceipt != nil && r.visibleReceipt.commit(visibleContent)
}

type RequestResult struct {
	ID            string         `json:"id"`
	Kind          string         `json:"kind"`
	Path          string         `json:"path,omitempty"`
	Files         []string       `json:"files,omitempty"`
	Matches       []Match        `json:"matches,omitempty"`
	SnippetIDs    []string       `json:"snippet_ids,omitempty"`
	Errors        []RequestError `json:"errors,omitempty"`
	SourcePartial bool           `json:"source_partial,omitempty"`
	PartialReason string         `json:"partial_reason,omitempty"`
}

type Match struct {
	Path       string   `json:"path"`
	Line       int      `json:"line"`
	Text       string   `json:"text,omitempty"`
	SnippetIDs []string `json:"snippet_ids,omitempty"`
}

// Snippet carries line coordinates once for the whole content fragment. When
// a single source line must be split, StartColumn and EndColumn are 1-based
// byte columns within that line; ordinary multi-line snippets omit them.
type Snippet struct {
	ID          string `json:"id"`
	Path        string `json:"path"`
	StartLine   int    `json:"start_line"`
	EndLine     int    `json:"end_line"`
	StartColumn int    `json:"start_column,omitempty"`
	EndColumn   int    `json:"end_column,omitempty"`
	Content     string `json:"content"`
}

type RequestError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ResultStats struct {
	Requests int `json:"requests"`
	Files    int `json:"files"`
	Matches  int `json:"matches"`
	Snippets int `json:"snippets"`
	Items    int `json:"items"`
}

type Tool struct {
	runtime            types.ToolRuntimeContextProvider
	readState          *toolfile.ReadFileState
	cursors            *cursorStore
	evidence           *evidenceStore
	WorkspaceRevisions *workspacerevision.Ledger
}

func New(runtime types.ToolRuntimeContextProvider, readState *toolfile.ReadFileState) *Tool {
	if readState == nil {
		readState = toolfile.NewReadFileState()
	}
	return &Tool{
		runtime: runtime, readState: readState,
		cursors: newCursorStore(), evidence: newEvidenceStore(),
	}
}

// WithRuntime returns a repository-scoped clone while preserving the shared
// read-evidence ledger and cursor snapshots.
func (t *Tool) WithRuntime(runtime types.ToolRuntimeContextProvider) types.Tool {
	if t == nil {
		return New(runtime, nil)
	}
	cursors := t.cursors
	if cursors == nil {
		cursors = newCursorStore()
	}
	state := t.readState
	if state == nil {
		state = toolfile.NewReadFileState()
	}
	evidence := t.evidence
	if evidence == nil {
		evidence = newEvidenceStore()
	}
	return &Tool{
		runtime: runtime, readState: state, cursors: cursors, evidence: evidence,
		WorkspaceRevisions: t.WorkspaceRevisions,
	}
}

// WithRuntimeAndReadState returns an actor-local clone. Child agents must use
// this form so Inspect and their mutation tools share the child's evidence
// ledger without authorizing writes from reads performed by the parent. Cursor
// snapshots are intentionally actor-local even when both actors use the same
// workspace and session identifiers.
func (t *Tool) WithRuntimeAndReadState(runtime types.ToolRuntimeContextProvider, readState *toolfile.ReadFileState) types.Tool {
	if readState == nil {
		readState = toolfile.NewReadFileState()
	}
	var revisions *workspacerevision.Ledger
	if t != nil {
		revisions = t.WorkspaceRevisions
	}
	return &Tool{
		runtime: runtime, readState: readState,
		cursors: newCursorStore(), evidence: newEvidenceStore(),
		WorkspaceRevisions: revisions,
	}
}

func (t *Tool) Name() string { return "Inspect" }

func (t *Tool) Description() string {
	return i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolInspectDescription)
}

func (t *Tool) ToolMetadata(map[string]any) types.ToolMetadata {
	return types.ToolMetadata{
		ReadOnly: true, Search: true, ConcurrencySafe: true,
		MaxResultSizeChars: types.UnlimitedToolResultSize,
	}
}

func (t *Tool) ToolDiscoveryMetadata() toolmeta.Metadata {
	return toolmeta.Metadata{
		AlwaysLoad: true,
		SearchHint: i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolInspectSearchHint),
	}
}

func (t *Tool) Schema() types.JSONSchema {
	lang := i18n.DetectOrLoadLanguage()
	rangeSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"start": toolbase.SemanticNumber(i18n.Text(lang, i18n.KeyToolInspectRequestRangeStartDescription), 1, true),
			"end":   toolbase.SemanticNumber(i18n.Text(lang, i18n.KeyToolInspectRequestRangeEndDescription), 1, true),
		},
		"required":             []string{"start", "end"},
		"additionalProperties": false,
	}
	requestIDSchema := map[string]any{
		"type": "string", "maxLength": maximumRequestID,
		"description": i18n.Text(lang, i18n.KeyToolInspectRequestIDDescription),
	}
	pathSchema := map[string]any{
		"type": "string", "maxLength": maximumPath,
		"description": i18n.Text(lang, i18n.KeyToolInspectRequestPathDescription),
	}
	patternSchema := map[string]any{
		"type": "string", "maxLength": maximumPattern,
		"description": i18n.Text(lang, i18n.KeyToolInspectRequestPatternDescription),
	}
	kindSchema := func(kind string) map[string]any {
		return map[string]any{
			"type": "string", "enum": []string{kind},
			"description": i18n.Text(lang, i18n.KeyToolInspectRequestKindDescription),
		}
	}
	readRequestSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id":   requestIDSchema,
			"kind": kindSchema(KindRead),
			"path": nullableInspectSchema(pathSchema),
			"ranges": nullableInspectSchema(map[string]any{
				"type": "array", "items": rangeSchema, "maxItems": maximumRanges,
				"description": i18n.Text(lang, i18n.KeyToolInspectRequestRangesDescription),
			}),
		},
		"required":             []string{"id", "kind"},
		"additionalProperties": false,
	}
	searchRequestSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id":          requestIDSchema,
			"kind":        kindSchema(KindSearch),
			"path":        nullableInspectSchema(pathSchema),
			"pattern":     patternSchema,
			"context":     nullableInspectSchema(toolbase.SemanticNumber(i18n.Text(lang, i18n.KeyToolInspectRequestContextDescription), 0, true)),
			"max_results": nullableInspectSchema(toolbase.SemanticNumber(i18n.Text(lang, i18n.KeyToolInspectRequestMaxResultsDescription), 1, true)),
		},
		"required":             []string{"id", "kind", "pattern"},
		"additionalProperties": false,
	}
	globRequestSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id":          requestIDSchema,
			"kind":        kindSchema(KindGlob),
			"path":        nullableInspectSchema(pathSchema),
			"pattern":     patternSchema,
			"max_results": nullableInspectSchema(toolbase.SemanticNumber(i18n.Text(lang, i18n.KeyToolInspectRequestMaxResultsDescription), 1, true)),
		},
		"required":             []string{"id", "kind", "pattern"},
		"additionalProperties": false,
	}
	requestsSchema := map[string]any{
		"type": "array", "minItems": 1, "maxItems": maximumRequests,
		"items":       map[string]any{"oneOf": []any{readRequestSchema, searchRequestSchema, globRequestSchema}},
		"description": i18n.Text(lang, i18n.KeyToolInspectInputRequestsDescription),
	}
	pageSchema := map[string]any{
		"type":        "object",
		"description": i18n.Text(lang, i18n.KeyToolInspectInputPageDescription),
		"properties": map[string]any{
			"max_chars":   nullableInspectSchema(toolbase.SemanticNumber(i18n.Text(lang, i18n.KeyToolInspectInputMaxCharsDescription), minimumMaxChars, true)),
			"max_files":   nullableInspectSchema(toolbase.SemanticNumber(i18n.Text(lang, i18n.KeyToolInspectInputMaxFilesDescription), 1, true)),
			"max_matches": nullableInspectSchema(toolbase.SemanticNumber(i18n.Text(lang, i18n.KeyToolInspectInputMaxMatchesDescription), 1, true)),
		},
		"additionalProperties": false,
	}
	modeDescription := i18n.Text(lang, i18n.KeyToolInspectInputModeDescription)
	operationSchema := map[string]any{
		"description": i18n.Text(lang, i18n.KeyToolInspectInputOperationDescription),
		"oneOf": []any{
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"mode":     map[string]any{"type": "string", "enum": []string{ModeNew}, "description": modeDescription},
					"requests": requestsSchema,
					"page":     nullableInspectSchema(pageSchema),
				},
				"required":             []string{"mode", "requests"},
				"additionalProperties": false,
			},
			map[string]any{
				"type": "object",
				"properties": map[string]any{
					"mode": map[string]any{"type": "string", "enum": []string{ModeContinue}, "description": modeDescription},
					"cursor": map[string]any{
						"type": "string", "minLength": 1,
						"description": i18n.Text(lang, i18n.KeyToolInspectInputCursorDescription),
					},
				},
				"required":             []string{"mode", "cursor"},
				"additionalProperties": false,
			},
		},
	}
	return types.StrictObjectSchema(map[string]any{"operation": operationSchema}, "operation")
}

func nullableInspectSchema(schema map[string]any) map[string]any {
	return map[string]any{"anyOf": []any{schema, map[string]any{"type": "null"}}}
}

func (t *Tool) MapToolResultToToolResultBlock(data any, toolUseID string) types.ToolResultBlock {
	result, ok := data.(Result)
	if !ok {
		return types.ToolResultBlock{
			Type: types.ContentTypeToolResult, ToolUseID: toolUseID,
			Content: i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolInspectResultEncodingFailed),
			IsError: true, Outcome: types.ToolOutcomeFailed,
		}
	}
	content := []byte(result.modelContent)
	if len(content) == 0 {
		var err error
		content, _, err = marshalModelResult(result, nil)
		if err != nil {
			return types.ToolResultBlock{
				Type: types.ContentTypeToolResult, ToolUseID: toolUseID,
				Content: i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolInspectResultEncodingFailed),
				IsError: true, Outcome: types.ToolOutcomeFailed,
			}
		}
	}
	if len(content) == 0 {
		return types.ToolResultBlock{
			Type: types.ContentTypeToolResult, ToolUseID: toolUseID,
			Content: i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolInspectResultEncodingFailed),
			IsError: true, Outcome: types.ToolOutcomeFailed,
		}
	}
	outcome := types.ToolOutcomeSucceeded
	if result.HasMoreView || result.SourceTruncated {
		outcome = types.ToolOutcomePartial
	}
	return types.ToolResultBlock{
		Type: types.ContentTypeToolResult, ToolUseID: toolUseID,
		Content: string(content), Data: result, Outcome: outcome,
	}
}

func resultError(err error) types.ToolResult {
	return types.ToolResult{Content: err.Error(), IsError: true, Outcome: types.ToolOutcomeFailed}
}

func localizedError(key i18n.Key, args ...any) error {
	return i18n.NewError(key, args...)
}

var _ types.Tool = (*Tool)(nil)
var _ types.ToolMetadataProvider = (*Tool)(nil)
var _ types.ToolResultMapper = (*Tool)(nil)
var _ types.ToolPermissionChecker = (*Tool)(nil)
var _ interface {
	WithRuntime(types.ToolRuntimeContextProvider) types.Tool
} = (*Tool)(nil)
