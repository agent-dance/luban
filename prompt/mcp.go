package prompt

import (
	"sort"
	"strings"

	"github.com/agent-dance/luban/i18n"
)

// MCPServerInstruction is the prompt-facing view of instructions advertised
// by a connected MCP server.
type MCPServerInstruction struct {
	Name         string
	Instructions string
}

// MCPInstructionBlocks returns the rendered per-server instruction blocks.
// Empty server names or instructions are ignored.
func MCPInstructionBlocks(servers []MCPServerInstruction) []string {
	filtered := make([]MCPServerInstruction, 0, len(servers))
	for _, server := range servers {
		name := strings.TrimSpace(server.Name)
		instructions := strings.TrimSpace(server.Instructions)
		if name == "" || instructions == "" {
			continue
		}
		filtered = append(filtered, MCPServerInstruction{Name: name, Instructions: instructions})
	}
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Name < filtered[j].Name
	})

	blocks := make([]string, 0, len(filtered))
	for _, server := range filtered {
		blocks = append(blocks, "## "+server.Name+"\n"+server.Instructions)
	}
	return blocks
}

// MCPInstructionsSectionForLanguage renders instructions that may be retained
// in visible conversation history using the active runtime language.
func MCPInstructionsSectionForLanguage(lang i18n.Language, servers []MCPServerInstruction) string {
	blocks := MCPInstructionBlocks(servers)
	if len(blocks) == 0 {
		return ""
	}
	return i18n.Format(lang, i18n.KeyLoopVisibleMCPInstructions, strings.Join(blocks, "\n\n"))
}
