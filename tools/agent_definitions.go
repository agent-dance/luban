package tools

// agent_definitions.go exposes a public API for the agent profile registry
// described by tasks/agent.json subtask agent-02 and the
// AutoClassifierInput/UserFacingName surface from subtask agent-10. The
// underlying implementation continues to live in agent.go (builtinAgentProfiles,
// loadCustomAgentProfiles, parseCustomAgentProfileFile); this file is the
// stable facade that downstream packages can depend on without reaching into
// private types.
//
// AGENT_SOURCE_GROUPS captures the explicit project > user > plugin >
// builtin priority that the TS resolveAgentOverrides applies when the same
// agent name is declared in multiple sources. Project beats user, user
// beats plugin, plugin beats builtin. Without this explicit ordering the
// effective agent picked across loaders is non-deterministic.

import (
	"path"
	"strings"

	"github.com/agent-dance/luban/compact"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

// AgentSourceGroup labels the origin tier of an AgentDefinition for the
// purpose of override resolution.
type AgentSourceGroup string

const (
	AgentSourceProject AgentSourceGroup = "project"
	AgentSourceUser    AgentSourceGroup = "user"
	AgentSourcePlugin  AgentSourceGroup = "plugin"
	AgentSourceManaged AgentSourceGroup = "managed"
	AgentSourceBuiltin AgentSourceGroup = "builtin"
)

// AgentSourceGroups is the priority-ordered tuple consulted by
// ResolveAgentOverrides. Earlier entries win when names collide.
var AgentSourceGroups = []AgentSourceGroup{
	AgentSourceProject,
	AgentSourceUser,
	AgentSourcePlugin,
	AgentSourceManaged,
	AgentSourceBuiltin,
}

// agentSourceRank gives the sort key for a given source. Lower is higher
// priority. Unknown sources sort after the explicit list so they never
// silently shadow a known tier.
func agentSourceRank(source string) int {
	for i, g := range AgentSourceGroups {
		if string(g) == source {
			return i
		}
	}
	return len(AgentSourceGroups)
}

// ResolveAgentOverrides dedupes a flat AgentDefinition list by (Name,
// Source). When the same name appears at multiple priorities, the highest
// (project > user > plugin > managed > builtin) wins. Definitions whose
// Source is empty are treated as builtin to match historical behaviour.
//
// Mirrors TS resolveAgentOverrides from src/tools/AgentTool/agentDisplay.ts.
func ResolveAgentOverrides(defs []AgentDefinition) []AgentDefinition {
	if len(defs) == 0 {
		return defs
	}
	winner := map[string]AgentDefinition{}
	winnerSource := map[string]int{}
	order := []string{}
	for _, d := range defs {
		key := strings.ToLower(strings.TrimSpace(d.Name))
		if key == "" {
			continue
		}
		source := d.Source
		if source == "" {
			source = string(AgentSourceBuiltin)
		}
		rank := agentSourceRank(source)
		if existing, ok := winnerSource[key]; ok {
			if rank >= existing {
				continue
			}
		} else {
			order = append(order, key)
		}
		winner[key] = d
		winnerSource[key] = rank
	}
	out := make([]AgentDefinition, 0, len(order))
	for _, key := range order {
		out = append(out, winner[key])
	}
	return out
}

// AgentDisplayGroup groups agent definitions for the /agents UI by
// source tier. The groups are returned in priority order (project >
// user > plugin > managed > builtin); each group's Definitions slice
// preserves the input ordering. Empty groups are omitted.
type AgentDisplayGroup struct {
	Source      AgentSourceGroup
	Label       string
	Definitions []AgentDefinition
	// Agents is an alias of Definitions retained for callers using the
	// /agents-style field name.
	Agents []AgentDefinition
}

// GroupAgentDefinitionsBySource bins definitions into the priority
// order returned by AgentSourceGroups. Use it after ResolveAgentOverrides
// when rendering /agents so users can see at a glance which tier the
// effective definition came from. Mirrors TS AGENT_SOURCE_GROUPS render
// in agentDisplay.ts.
func GroupAgentDefinitionsBySource(defs []AgentDefinition) []AgentDisplayGroup {
	bySource := map[AgentSourceGroup][]AgentDefinition{}
	for _, d := range defs {
		source := AgentSourceGroup(d.Source)
		if source == "" {
			source = AgentSourceBuiltin
		}
		bySource[source] = append(bySource[source], d)
	}
	out := make([]AgentDisplayGroup, 0, len(AgentSourceGroups))
	for _, src := range AgentSourceGroups {
		entries := bySource[src]
		if len(entries) == 0 {
			continue
		}
		out = append(out, AgentDisplayGroup{Source: src, Definitions: entries})
	}
	return out
}

// AgentDefinition is the public, copy-friendly representation of an agent
// profile loaded from built-ins or .claude/agents/*.md frontmatter. It mirrors
// the relevant subset of agentProfile so external callers can introspect the
// registry without depending on internal types.
type AgentDefinition struct {
	Name            string
	WhenToUse       string
	SystemPrefix    string
	Model           string
	Isolation       string
	Skills          []string
	MCPServers      []string
	AllowedTools    []string
	DisallowedTools []string
	MaxTurns        int
	Background      bool
	Color           string
	Source          string // "builtin" | "project" | "user" | "managed"
}

// LoadAgentDefinitions returns the merged set of agent definitions visible
// from the given working directory. Built-ins come first; custom profiles in
// .claude/agents/*.md and any managed directories override built-ins by name.
//
// The cwd argument may be empty; in that case loadCustomAgentProfiles falls
// back to the process working directory.
func LoadAgentDefinitions(cwd string) ([]AgentDefinition, error) {
	defs := []AgentDefinition{}
	for _, p := range builtinAgentProfiles() {
		def := agentProfileToDefinition(p)
		def.Source = "builtin"
		defs = append(defs, def)
	}
	custom, err := loadCustomAgentProfiles(cwd)
	if err != nil {
		return defs, err
	}
	byName := map[string]int{}
	for i, def := range defs {
		byName[strings.ToLower(strings.TrimSpace(def.Name))] = i
	}
	for _, p := range custom {
		def := agentProfileToDefinition(p)
		def.Source = "project"
		key := strings.ToLower(strings.TrimSpace(def.Name))
		if idx, ok := byName[key]; ok {
			defs[idx] = def
			continue
		}
		byName[key] = len(defs)
		defs = append(defs, def)
	}
	return defs, nil
}

// LoadAgentDefinitionsForRuntime applies any AgentTool-level overrides (e.g.
// disabling built-ins via the SDK opt-out env) on top of LoadAgentDefinitions.
func (t *AgentTool) LoadAgentDefinitionsForRuntime(cwd string) ([]AgentDefinition, error) {
	defs := []AgentDefinition{}
	for _, p := range t.builtinAgentProfilesForRuntime() {
		def := agentProfileToDefinition(p)
		def.Source = "builtin"
		defs = append(defs, def)
	}
	custom, err := loadCustomAgentProfiles(cwd)
	if err != nil {
		return defs, err
	}
	byName := map[string]int{}
	for i, def := range defs {
		byName[strings.ToLower(strings.TrimSpace(def.Name))] = i
	}
	for _, p := range custom {
		def := agentProfileToDefinition(p)
		def.Source = "project"
		key := strings.ToLower(strings.TrimSpace(def.Name))
		if idx, ok := byName[key]; ok {
			defs[idx] = def
			continue
		}
		byName[key] = len(defs)
		defs = append(defs, def)
	}
	if t != nil && t.Registry != nil {
		if err := validateAgentDefinitionToolAllowLists(defs, t.Registry); err != nil {
			return nil, err
		}
	}
	return defs, nil
}

func validateAgentDefinitionToolAllowLists(defs []AgentDefinition, reg *registry.Registry) error {
	if reg == nil {
		return nil
	}
	available := reg.Names()
	for _, def := range defs {
		if strings.EqualFold(strings.TrimSpace(def.Source), string(AgentSourceBuiltin)) {
			continue
		}
		for _, spec := range def.AllowedTools {
			raw := strings.TrimSpace(spec)
			if raw == "" || raw == "*" {
				continue
			}
			name := strings.TrimSpace(raw)
			if parsed := normalizedToolNameFromPermissionSpec(raw); parsed != "" {
				name = parsed
			}
			matched := false
			for _, candidate := range available {
				candidateNames := []string{candidate}
				if tool := reg.Get(candidate); tool != nil {
					if aliases, ok := tool.(types.AliasedTool); ok {
						candidateNames = append(candidateNames, aliases.Aliases()...)
					}
				}
				for _, candidateName := range candidateNames {
					if strings.EqualFold(candidateName, name) {
						matched = true
						break
					}
					if strings.ContainsAny(name, "*?[") {
						if ok, _ := path.Match(strings.ToLower(name), strings.ToLower(candidateName)); ok {
							matched = true
							break
						}
					}
				}
				if matched {
					break
				}
			}
			if !matched {
				return i18n.NewError(i18n.KeyToolAgentDefinitionUnknownTool, def.Name, raw)
			}
		}
	}
	return nil
}

func (t *AgentTool) PostCompactAgentDefinitions(cwd string) []compact.AgentDefinitionSnapshot {
	if t == nil {
		return nil
	}
	defs, err := t.LoadAgentDefinitionsForRuntime(cwd)
	if err != nil {
		return nil
	}
	resolved := ResolveAgentOverrides(defs)
	out := make([]compact.AgentDefinitionSnapshot, 0, len(resolved))
	for _, def := range resolved {
		out = append(out, compact.AgentDefinitionSnapshot{
			Name:      def.Name,
			WhenToUse: def.WhenToUse,
			Source:    def.Source,
		})
	}
	return out
}

func agentProfileToDefinition(p agentProfile) AgentDefinition {
	allowed := make([]string, 0, len(p.AllowedToolSpecs))
	allowed = append(allowed, p.AllowedToolSpecs...)
	disallowed := make([]string, 0, len(p.DisallowedToolSpecs))
	disallowed = append(disallowed, p.DisallowedToolSpecs...)
	skills := make([]string, 0, len(p.Skills))
	skills = append(skills, p.Skills...)
	mcp := make([]string, 0, len(p.MCPServers))
	mcp = append(mcp, p.MCPServers...)
	return AgentDefinition{
		Name:            p.Name,
		WhenToUse:       p.WhenToUse,
		SystemPrefix:    p.SystemPrefix,
		Model:           p.Model,
		Isolation:       p.Isolation,
		Skills:          skills,
		MCPServers:      mcp,
		AllowedTools:    allowed,
		DisallowedTools: disallowed,
		MaxTurns:        p.MaxTurns,
		Background:      p.Background,
		Color:           p.Color,
	}
}

// AutoClassifierInput mirrors the TS AgentTool.toAutoClassifierInput contract
// from agent-10. It returns a compact tagline used by the auto-permission
// classifier when deciding whether to ask the user.
//
//	"(<subagent_type>): <prompt>"
//
// When subagent_type is not set, the leading tag block is omitted.
func (t *AgentTool) AutoClassifierInput(in AgentInput) string {
	tags := make([]string, 0, 1)
	if v := strings.TrimSpace(in.SubagentType); v != "" {
		tags = append(tags, v)
	}
	prefix := ": "
	if len(tags) > 0 {
		prefix = "(" + strings.Join(tags, ", ") + "): "
	}
	return prefix + in.Prompt
}

// UserFacingName returns the label rendered above an agent run in the UI.
// Mirrors src/tools/AgentTool/UI.tsx::userFacingName: shows the subagent_type
// for non-default agents, "Agent" for the general-purpose default and the
// special "worker" type, and "Agent" when the input is empty.
func (t *AgentTool) UserFacingName(in *AgentInput) string {
	if in == nil {
		return "Agent"
	}
	st := strings.TrimSpace(in.SubagentType)
	if st == "" || strings.EqualFold(st, "general-purpose") || strings.EqualFold(st, "worker") {
		return "Agent"
	}
	return st
}

// UserFacingNameBackgroundColor mirrors UI.tsx::userFacingNameBackgroundColor.
// Returns "" when no theme color should be applied (the TS default).
func (t *AgentTool) UserFacingNameBackgroundColor(in *AgentInput) string {
	if in == nil {
		return ""
	}
	return strings.TrimSpace(in.Color)
}

// AgentSourceGroupLabel returns the human-readable header for the /agents UI
// matching src/tools/AgentTool/agentDisplay.ts AGENT_SOURCE_GROUPS labels.
func AgentSourceGroupLabel(source AgentSourceGroup) string {
	switch source {
	case AgentSourceProject:
		return toolRuntimeText(i18n.KeyToolRuntimeAgentSourceProjectLabel)
	case AgentSourceUser:
		return toolRuntimeText(i18n.KeyToolRuntimeAgentSourceUserLabel)
	case AgentSourcePlugin:
		return toolRuntimeText(i18n.KeyToolRuntimeAgentSourcePluginLabel)
	case AgentSourceManaged:
		return toolRuntimeText(i18n.KeyToolRuntimeAgentSourceManagedLabel)
	case AgentSourceBuiltin:
		return toolRuntimeText(i18n.KeyToolRuntimeAgentSourceBuiltinLabel)
	default:
		return toolRuntimeFormat(i18n.KeyToolRuntimeAgentSourceOtherLabel, source)
	}
}

// AgentDisplayGroup type is declared once above. The second declaration
// previously here has been merged into the canonical type.

// GroupAgentsBySource buckets a flat list of definitions by Source in the
// canonical AgentSourceGroups order. Within each group, agents are sorted by
// Name (case-insensitive). Empty groups are omitted.
//
// Mirrors the rendering pass behind the TS /agents view: project → user →
// plugin → managed → builtin, so users can see at a glance which tier wins
// when names collide. This is the convenience wrapper around
// GroupAgentDefinitionsBySource that also sorts within-group entries.
func GroupAgentsBySource(defs []AgentDefinition) []AgentDisplayGroup {
	if len(defs) == 0 {
		return nil
	}
	buckets := map[AgentSourceGroup][]AgentDefinition{}
	for _, d := range defs {
		source := AgentSourceGroup(d.Source)
		if d.Source == "" {
			source = AgentSourceBuiltin
		}
		// Normalise unknown sources to builtin so they still surface.
		if agentSourceRank(string(source)) == len(AgentSourceGroups) {
			source = AgentSourceBuiltin
		}
		buckets[source] = append(buckets[source], d)
	}
	out := make([]AgentDisplayGroup, 0, len(AgentSourceGroups))
	for _, src := range AgentSourceGroups {
		group, ok := buckets[src]
		if !ok || len(group) == 0 {
			continue
		}
		sortAgentsByName(group)
		out = append(out, AgentDisplayGroup{
			Source:      src,
			Definitions: group,
		})
	}
	return out
}

func sortAgentsByName(defs []AgentDefinition) {
	// Simple insertion sort: agent counts are tiny (dozens) and we want a
	// stable, dependency-free comparator.
	for i := 1; i < len(defs); i++ {
		for j := i; j > 0; j-- {
			if strings.ToLower(defs[j].Name) < strings.ToLower(defs[j-1].Name) {
				defs[j], defs[j-1] = defs[j-1], defs[j]
				continue
			}
			break
		}
	}
}
