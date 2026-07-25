package file

import (
	"github.com/agent-dance/luban/types"
)

const (
	fileErrorReadRequired      = "file.edit.read_required"
	fileErrorViewTransformed   = "file.edit.view_transformed"
	fileErrorAnchorUnobserved  = "file.edit.anchor_unobserved"
	fileErrorSnapshotStale     = "file.edit.snapshot_stale"
	fileErrorAnchorMissing     = "file.edit.anchor_missing"
	fileErrorAnchorAmbiguous   = "file.edit.anchor_ambiguous"
	fileErrorReadTokenLimit    = "file.read.token_limit"
	fileErrorReadSizeLimit     = "file.read.size_limit"
	fileErrorWriteReadRequired = "file.write.read_required"
	fileErrorWriteFullRead     = "file.write.full_read_required"
)

func structuredReadSizeError(path string, size, maximum int64) types.ToolResult {
	base := errorResponse(fileTooLargeRuntimeError(size, maximum))
	// Size alone cannot prove how many lines fit the token budget. Suggest the
	// smallest syntactically useful range instead of repeating the known-risky
	// fixed 2000-line recommendation.
	result := structuredFileError(base.Content, fileErrorReadSizeLimit, path, true, nil, readFileRetry(path, 1, 1))
	result.Completeness = types.ToolResultCompleteness{Source: types.ToolResultCompletenessCaptureDropped}
	return result
}

func structuredFileError(content, code, path string, retryable bool, coverage *types.ToolErrorCoverage, retry *types.ToolErrorRetry) types.ToolResult {
	return types.ToolResult{
		Content: content,
		Data: types.ToolErrorData{
			Schema: "tool_error/v1", Code: code, Retryable: retryable,
			Path: path, Coverage: coverage, Retry: retry,
		},
		IsError:      true,
		Outcome:      types.ToolOutcomeFailed,
		Completeness: types.ToolResultCompleteness{Source: types.ToolResultCompletenessComplete},
	}
}

func readEntryToolCoverage(entry ReadFileEntry, required []types.ToolErrorRange) *types.ToolErrorCoverage {
	observed := make([]types.ToolErrorRange, 0, len(entry.Coverage))
	for _, value := range entry.Coverage {
		if value.EndLine <= value.StartLine {
			continue
		}
		observed = append(observed, types.ToolErrorRange{StartLine: value.StartLine, EndLine: value.EndLine - 1})
	}
	return &types.ToolErrorCoverage{
		Complete: readEntryCoverageComplete(entry), TotalLines: entry.TotalLines,
		Observed: observed, Required: required,
	}
}

func readFileRetry(path string, offset, limit int) *types.ToolErrorRetry {
	action := "read_file"
	if offset > 0 && limit > 0 {
		action = "read_range"
	}
	return &types.ToolErrorRetry{Action: action, Tool: "Read", FilePath: path, Offset: offset, Limit: limit}
}
