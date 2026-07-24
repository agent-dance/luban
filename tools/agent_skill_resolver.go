package tools

// agent_skill_resolver.go mirrors the TS resolveSkillName three-strategy
// helper from src/tools/AgentTool/runAgent.ts. When an agent's frontmatter
// declares preloadSkills:['foo'], the runtime tries to resolve 'foo' to
// a real skill via three strategies in order:
//
//   1. direct name match — find a skill whose Name == 'foo'
//   2. plugin-prefix match — find a skill whose Name == '<plugin>:foo'
//      for any plugin namespace
//   3. suffix match — find a skill whose Name ends with ':foo' or '/foo'
//
// Without all three strategies, agents that reference a skill loaded
// under a plugin namespace (or a renamed install) silently fail to
// preload it; the agent is missing capabilities it thinks it has.

import (
	"strings"

	"github.com/agent-dance/luban/skills"
)

// ResolveSkillName picks the best-matching loaded skill for the
// requested name. Returns the resolved skill and true on hit; nil and
// false when no candidate matches any strategy.
//
// Lookup is case-insensitive on the user-visible portion of the name;
// the plugin namespace prefix preserves case.
func ResolveSkillName(manager *skills.Manager, requested string) (*skills.Skill, bool) {
	requested = strings.TrimSpace(requested)
	if manager == nil || requested == "" {
		return nil, false
	}

	// Strategy 1: direct lookup. Manager.Get is case-sensitive but we
	// retry on the requested string verbatim for parity with TS.
	if hit := manager.Get(requested); hit != nil {
		return hit, true
	}

	// Strategy 2 + 3 require the full skill list.
	all := manager.All()
	if len(all) == 0 {
		return nil, false
	}

	loweredRequested := strings.ToLower(requested)

	// Strategy 2: plugin-prefix match. Try '<any-plugin>:requested'.
	for _, sk := range all {
		if sk == nil {
			continue
		}
		name := strings.TrimSpace(sk.Name)
		if !strings.Contains(name, ":") {
			continue
		}
		_, after, _ := strings.Cut(name, ":")
		if strings.EqualFold(after, requested) {
			return sk, true
		}
	}

	// Strategy 3: suffix match across all loaded skills. Treat both ':'
	// and '/' as namespace separators since plugins occasionally render
	// as 'foo/bar'.
	for _, sk := range all {
		if sk == nil {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(sk.Name))
		if name == "" {
			continue
		}
		if strings.HasSuffix(name, ":"+loweredRequested) || strings.HasSuffix(name, "/"+loweredRequested) {
			return sk, true
		}
	}
	return nil, false
}
