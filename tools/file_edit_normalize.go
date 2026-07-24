package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/agent-dance/luban/types"
)

var fileEditDesanitizations = []struct{ from, to string }{
	{"<fnr>", "<function_results>"},
	{"<n>", "<name>"}, {"</n>", "</name>"},
	{"<o>", "<output>"}, {"</o>", "</output>"},
	{"<e>", "<error>"}, {"</e>", "</error>"},
	{"<s>", "<system>"}, {"</s>", "</system>"},
	{"<r>", "<result>"}, {"</r>", "</result>"},
	{"< META_START >", "<META_START>"}, {"< META_END >", "<META_END>"},
	{"< EOT >", "<EOT>"}, {"< META >", "<META>"}, {"< SOS >", "<SOS>"},
	{"\n\nH:", "\n\nHuman:"}, {"\n\nA:", "\n\nAssistant:"},
}

func (t *FileEditTool) runtimeSnapshot() types.ToolRuntimeContext {
	if t != nil && t.Runtime != nil {
		return t.Runtime.ToolRuntimeContext()
	}
	return types.ToolRuntimeContext{}
}

func (t *FileEditTool) editBaseDir() string {
	if root := strings.TrimSpace(t.runtimeSnapshot().ProjectRoot); root != "" {
		return root
	}
	cwd, err := os.Getwd()
	if err != nil || strings.TrimSpace(cwd) == "" {
		return "."
	}
	return cwd
}

func (t *FileEditTool) expandPath(raw string) (string, error) {
	return expandReadPath(raw, t.editBaseDir())
}

// BackfillObservableInput makes the normalized path and whitespace-safe input
// visible to hooks, permissions, and direct Registry callers. File-dependent
// desanitization is deliberately deferred until Execute has authorization.
func (t *FileEditTool) BackfillObservableInput(input map[string]any) (map[string]any, error) {
	return t.normalizeFileEditInput(input)
}

func (t *FileEditTool) NormalizeToolInput(_ context.Context, input map[string]any) (map[string]any, error) {
	return t.normalizeFileEditInput(input)
}

func (t *FileEditTool) normalizeFileEditInput(input map[string]any) (map[string]any, error) {
	updated := cloneToolInput(input)
	rawPath, ok := updated["file_path"].(string)
	if !ok || strings.TrimSpace(rawPath) == "" {
		return updated, nil
	}
	expanded, err := t.expandPath(rawPath)
	if err != nil {
		return nil, err
	}
	updated["file_path"] = expanded

	_, oldOK := updated["old_string"].(string)
	newString, newOK := updated["new_string"].(string)
	if !oldOK || !newOK {
		return updated, nil
	}
	if !isMarkdownFile(expanded) {
		newString = stripFileEditTrailingWhitespace(newString)
		updated["new_string"] = newString
	}
	return updated, nil
}

// desanitizeFileEditPair runs only after Execute has completed permission and
// allowed-directory checks and read the target under the edit lock. Keeping
// file-dependent normalization here prevents pre-permission file disclosure.
func desanitizeFileEditPair(fileContent, oldString, newString string) (string, string) {
	if strings.Contains(fileContent, oldString) {
		return oldString, newString
	}
	desanitizedOld := oldString
	applied := make([]struct{ from, to string }, 0)
	for _, replacement := range fileEditDesanitizations {
		next := strings.ReplaceAll(desanitizedOld, replacement.from, replacement.to)
		if next != desanitizedOld {
			applied = append(applied, replacement)
			desanitizedOld = next
		}
	}
	if !strings.Contains(fileContent, desanitizedOld) {
		return oldString, newString
	}
	for _, replacement := range applied {
		newString = strings.ReplaceAll(newString, replacement.from, replacement.to)
	}
	return desanitizedOld, newString
}

func isMarkdownFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".md" || ext == ".mdx"
}

func stripFileEditTrailingWhitespace(value string) string {
	var out strings.Builder
	for start := 0; start < len(value); {
		end := start
		for end < len(value) && value[end] != '\n' && value[end] != '\r' {
			end++
		}
		out.WriteString(strings.TrimRightFunc(value[start:end], unicode.IsSpace))
		if end == len(value) {
			break
		}
		if value[end] == '\r' && end+1 < len(value) && value[end+1] == '\n' {
			out.WriteString("\r\n")
			start = end + 2
		} else {
			out.WriteByte(value[end])
			start = end + 1
		}
	}
	return out.String()
}
