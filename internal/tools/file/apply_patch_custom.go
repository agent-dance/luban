package file

import (
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

// applyPatchCustomGrammar is intentionally a single bounded envelope grammar.
// The JSON-function fallback accepts unified diff for compatibility, but the
// freeform wire surface does not: one canonical syntax reduces malformed
// generations and keeps the provider grammar aligned with the local parser.
const applyPatchCustomGrammar = `start: begin section+ end

begin: BEGIN NEWLINE
end: END NEWLINE?
?section: add_file | update_file | delete_file

add_file: add_header added_line+ no_newline?
update_file: update_header update_hunk+
delete_file: delete_header
update_hunk: hunk_header hunk_line+ end_of_file?
?hunk_line: context_line | added_line | deleted_line | no_newline

add_header: ADD_HEADER NEWLINE
update_header: UPDATE_HEADER NEWLINE
delete_header: DELETE_HEADER NEWLINE
hunk_header: HUNK_HEADER NEWLINE
context_line: CONTEXT_LINE NEWLINE
added_line: ADDED_LINE NEWLINE
deleted_line: DELETED_LINE NEWLINE
no_newline: NO_NEWLINE NEWLINE
end_of_file: END_OF_FILE NEWLINE

BEGIN: "*** Begin Patch"
END: "*** End Patch"
ADD_HEADER: /\*\*\* Add File: [^\r\n]{1,512}/
UPDATE_HEADER: /\*\*\* Update File: [^\r\n]{1,512}/
DELETE_HEADER: /\*\*\* Delete File: [^\r\n]{1,512}/
HUNK_HEADER: /@@[^\r\n]{0,512}/
CONTEXT_LINE: / [^\r\n]{0,16384}/
ADDED_LINE: /\+[^\r\n]{0,16384}/
DELETED_LINE: /-[^\r\n]{0,16384}/
NO_NEWLINE: /\\ No newline at end of file/
END_OF_FILE: "*** End of File"
NEWLINE: /\r?\n/`

func (t *ApplyPatchTool) CustomToolInputFormat() (types.ToolInputFormat, bool) {
	if t == nil || !t.CustomToolInput {
		return types.ToolInputFormat{}, false
	}
	return types.ToolInputFormat{
		Type:       "grammar",
		Syntax:     "lark",
		Definition: applyPatchCustomGrammar,
	}, true
}

func (t *ApplyPatchTool) DecodeCustomToolInput(raw string) (map[string]any, error) {
	if t == nil || !t.CustomToolInput {
		return nil, i18n.NewError(i18n.KeyRuntimeToolSkippedMalformed, "ApplyPatch")
	}
	return map[string]any{"patch": raw}, nil
}

var (
	_ types.CustomToolDefinitionProvider = (*ApplyPatchTool)(nil)
	_ types.CustomToolInputDecoder       = (*ApplyPatchTool)(nil)
)
