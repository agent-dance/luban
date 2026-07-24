package tools

// agent_color.go mirrors src/tools/AgentTool/agentColorManager.ts.
//
// getAgentColor / setAgentColor maintain a process-wide mapping from agent
// type to a stable color slot (one of 8). The general-purpose agent always
// returns "" (no color override) so the parent harness renders it with the
// default appearance. Persisting the mapping in-process means the same agent
// type always renders the same color across parallel async runs.

import (
	"strings"
	"sync"
)

// AgentPaletteColors lists the 8 palette colors assignable to non-default
// agent types. Order is significant: getAgentColor assigns by round-robin
// over this slice when an agent type has not been seen before.
var AgentPaletteColors = []string{
	"red",
	"blue",
	"green",
	"yellow",
	"purple",
	"orange",
	"pink",
	"cyan",
}

var (
	agentColorMu  sync.Mutex
	agentColorMap = map[string]string{}
)

// GetAgentColor returns the persistent color for an agent type. Returns "" for
// "general-purpose" (matching TS behaviour: no theme override) and "" when no
// color has been assigned yet — callers should call SetAgentColor or
// AssignAgentColor to install one.
//
// The lookup is case-sensitive on the agent type to match TS Map<string,...>.
func GetAgentColor(agentType string) string {
	t := strings.TrimSpace(agentType)
	if t == "" || t == "general-purpose" {
		return ""
	}
	agentColorMu.Lock()
	defer agentColorMu.Unlock()
	color := agentColorMap[t]
	if color == "" {
		return ""
	}
	if !isPaletteColor(color) {
		return ""
	}
	return color
}

// SetAgentColor installs an explicit color for an agent type. Passing "" or an
// unknown color removes any prior mapping. Mirrors TS setAgentColor.
func SetAgentColor(agentType, color string) {
	t := strings.TrimSpace(agentType)
	if t == "" {
		return
	}
	agentColorMu.Lock()
	defer agentColorMu.Unlock()
	if color == "" {
		delete(agentColorMap, t)
		return
	}
	if !isPaletteColor(color) {
		return
	}
	agentColorMap[t] = color
}

// AssignAgentColor returns the existing color for an agent type or, if none is
// set, picks the next palette slot in stable round-robin order and persists
// it. Returns "" for "general-purpose" (no override). Matches the practical
// TS bootstrap behaviour where colors are assigned in first-spawn order.
func AssignAgentColor(agentType string) string {
	t := strings.TrimSpace(agentType)
	if t == "" || t == "general-purpose" {
		return ""
	}
	agentColorMu.Lock()
	defer agentColorMu.Unlock()
	if existing, ok := agentColorMap[t]; ok && isPaletteColor(existing) {
		return existing
	}
	idx := len(agentColorMap) % len(AgentPaletteColors)
	color := AgentPaletteColors[idx]
	agentColorMap[t] = color
	return color
}

// ResetAgentColorMap clears the in-process mapping. Tests should call this in
// setup so assignments are deterministic.
func ResetAgentColorMap() {
	agentColorMu.Lock()
	defer agentColorMu.Unlock()
	agentColorMap = map[string]string{}
}

// AgentColorMapSnapshot returns a copy of the current mapping. Useful for the
// /agents UI and tests.
func AgentColorMapSnapshot() map[string]string {
	agentColorMu.Lock()
	defer agentColorMu.Unlock()
	out := make(map[string]string, len(agentColorMap))
	for k, v := range agentColorMap {
		out[k] = v
	}
	return out
}

func isPaletteColor(color string) bool {
	for _, c := range AgentPaletteColors {
		if c == color {
			return true
		}
	}
	return false
}
