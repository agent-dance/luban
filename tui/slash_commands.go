package tui

import (
	"sort"
	"strings"

	"github.com/agent-dance/luban/i18n"
)

// SlashCommandEntry is the TUI-facing metadata for a slash command.
// It is intentionally decoupled from the commands package so the TUI can
// render and rank suggestions without depending on command execution details.
type SlashCommandEntry struct {
	Name           string
	Aliases        []string
	Description    string
	DescriptionKey i18n.Key
	// DisplayLabel and Input override the normal /command label and inserted
	// text. They let commands expose localized argument choices as a submenu.
	DisplayLabel string
	Input        string
	OpensSubmenu bool
}

// MCPManagementSlashCommandEntry returns the TUI metadata for /mcp. The command
// itself lives in commands/mcp.go; this helper lets app wiring add the entry
// without duplicating copy.
func MCPManagementSlashCommandEntry() SlashCommandEntry {
	return SlashCommandEntry{
		Name:           "mcp",
		DescriptionKey: i18n.KeyCommandMCPDescription,
	}
}

// slashSuggestionsState holds the current interactive slash-command overlay.
type slashSuggestionsState struct {
	Items    []SlashCommandEntry
	Selected int
}

const maxVisibleSlashSuggestions = 5

func computeSlashSuggestions(input string, commands []SlashCommandEntry, lang i18n.Language) *slashSuggestionsState {
	if !strings.HasPrefix(input, "/") {
		return nil
	}
	if strings.ContainsRune(input, '\n') {
		return nil
	}
	query := strings.TrimSpace(strings.TrimPrefix(input, "/"))
	if languageQuery, ok := languageSubmenuQuery(query, strings.HasSuffix(input, " ")); ok {
		return computeLanguageSuggestions(languageQuery)
	}
	if len(input) > 1 && strings.HasSuffix(input, " ") {
		return nil
	}
	if query == "" {
		items := cloneSlashCommands(commands)
		if len(items) == 0 {
			return nil
		}
		return &slashSuggestionsState{
			Items:    items,
			Selected: 0,
		}
	}

	// Once the user has moved on to arguments, the command list should close.
	if strings.ContainsAny(query, " \t") {
		return nil
	}

	lowerQuery := strings.ToLower(query)
	type ranked struct {
		cmd      SlashCommandEntry
		category int
		length   int
	}

	matches := make([]ranked, 0, len(commands))
	for _, cmd := range commands {
		cmd = withSlashCommandDescriptionKey(cmd)
		name := strings.ToLower(cmd.Name)
		category := matchSlashCommand(lowerQuery, name, cmd.Aliases, cmd.Description)
		if category < 0 {
			continue
		}
		matches = append(matches, ranked{
			cmd:      cmd,
			category: category,
			length:   len(cmd.Name),
		})
	}
	if len(matches) == 0 {
		return nil
	}

	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].category != matches[j].category {
			return matches[i].category < matches[j].category
		}
		if matches[i].length != matches[j].length {
			return matches[i].length < matches[j].length
		}
		return matches[i].cmd.Name < matches[j].cmd.Name
	})

	items := make([]SlashCommandEntry, len(matches))
	for i, m := range matches {
		items[i] = m.cmd
	}
	return &slashSuggestionsState{
		Items:    items,
		Selected: 0,
	}
}

func languageSubmenuQuery(query string, hasTrailingSpace bool) (string, bool) {
	fields := strings.Fields(query)
	if len(fields) == 0 || fields[0] != "language" {
		return "", false
	}
	if len(fields) > 1 {
		return strings.Join(fields[1:], " "), true
	}
	// An exact command match stays on the command list so selecting it opens
	// the submenu instead of silently changing to the first language.
	if query == "language" && !hasTrailingSpace {
		return "", false
	}
	return "", true
}

func computeLanguageSuggestions(query string) *slashSuggestionsState {
	query = strings.ToLower(strings.TrimSpace(query))
	items := make([]SlashCommandEntry, 0, len(i18n.AllLanguages()))
	for _, language := range i18n.AllLanguages() {
		label := language.String()
		if query != "" && !strings.Contains(strings.ToLower(label+" "+language.Code()), query) {
			continue
		}
		items = append(items, SlashCommandEntry{
			Name:         "language",
			Description:  language.Code(),
			DisplayLabel: label,
			Input:        "/language " + language.Code(),
		})
	}
	if len(items) == 0 {
		return nil
	}
	return &slashSuggestionsState{Items: items}
}

func matchSlashCommand(query, name string, aliases []string, desc string) int {
	switch {
	case name == query:
		return 0
	case hasAliasMatch(query, aliases, func(alias string) bool { return alias == query }):
		return 1
	case strings.HasPrefix(name, query):
		return 2
	case hasAliasMatch(query, aliases, func(alias string) bool { return strings.HasPrefix(alias, query) }):
		return 3
	case strings.Contains(name, query):
		return 4
	case hasAliasMatch(query, aliases, func(alias string) bool { return strings.Contains(alias, query) }):
		return 5
	case strings.Contains(strings.ToLower(desc), query):
		return 6
	default:
		return -1
	}
}

func hasAliasMatch(query string, aliases []string, pred func(string) bool) bool {
	for _, alias := range aliases {
		if pred(strings.ToLower(alias)) {
			return true
		}
	}
	return false
}

func cloneSlashCommands(commands []SlashCommandEntry) []SlashCommandEntry {
	if len(commands) == 0 {
		return nil
	}
	items := make([]SlashCommandEntry, len(commands))
	for i, command := range commands {
		items[i] = withSlashCommandDescriptionKey(command)
	}
	return items
}

func withSlashCommandDescriptionKey(command SlashCommandEntry) SlashCommandEntry {
	if command.DescriptionKey == "" {
		command.DescriptionKey, _ = i18n.CommandDescriptionKey(command.Name)
	}
	return command
}

func localizedSlashCommandDescription(lang i18n.Language, command SlashCommandEntry) string {
	command = withSlashCommandDescriptionKey(command)
	if command.DescriptionKey != "" {
		return i18n.Text(lang, command.DescriptionKey)
	}
	return command.Description
}

func visibleSlashSuggestions(state *slashSuggestionsState) []SlashCommandEntry {
	_, visible := slashSuggestionsWindow(state)
	return visible
}

func slashSuggestionsWindow(state *slashSuggestionsState) (int, []SlashCommandEntry) {
	if state == nil || len(state.Items) == 0 {
		return 0, nil
	}
	if len(state.Items) <= maxVisibleSlashSuggestions {
		return 0, state.Items
	}

	selected := state.Selected
	if selected < 0 {
		selected = 0
	}
	if selected >= len(state.Items) {
		selected = len(state.Items) - 1
	}

	start := selected - maxVisibleSlashSuggestions/2
	if start < 0 {
		start = 0
	}
	end := start + maxVisibleSlashSuggestions
	if end > len(state.Items) {
		end = len(state.Items)
		start = end - maxVisibleSlashSuggestions
	}
	return start, state.Items[start:end]
}

func formatSlashCommandInput(name string) string {
	return "/" + name + " "
}
