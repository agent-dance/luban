package prompt

import "testing"

func TestApplyCacheScopes_DynamicTextDoesNotChangeStaticCacheMetadata(t *testing.T) {
	base := []SystemPromptBlock{
		{Text: "stable static prompt", Source: "built_in", Name: "static", Cache: true},
		{Text: "cwd: /one", Source: "runtime", Name: "dynamic"},
	}
	changed := []SystemPromptBlock{
		{Text: "stable static prompt", Source: "built_in", Name: "static", Cache: true},
		{Text: "cwd: /two", Source: "runtime", Name: "dynamic"},
	}

	first := ApplyCacheScopes(base, CacheScopeOptions{GlobalSafe: true})
	second := ApplyCacheScopes(changed, CacheScopeOptions{GlobalSafe: true})

	if len(first) != 2 || len(second) != 2 {
		t.Fatalf("expected two blocks in each prompt, got %d and %d", len(first), len(second))
	}
	if first[0].Text != second[0].Text ||
		first[0].Source != second[0].Source ||
		first[0].Name != second[0].Name ||
		first[0].Cache != second[0].Cache ||
		first[0].CacheScope != second[0].CacheScope {
		t.Fatalf("static cache metadata changed:\nfirst:  %#v\nsecond: %#v", first[0], second[0])
	}
	if !first[0].Cache || first[0].CacheScope != CacheScopeGlobal {
		t.Fatalf("expected global cached static block, got %#v", first[0])
	}
	if first[1].Cache || first[1].CacheScope != "" || second[1].Cache || second[1].CacheScope != "" {
		t.Fatalf("dynamic blocks should remain uncached, got %#v and %#v", first[1], second[1])
	}
}

func TestApplyCacheScopes_BoundarySplitsDynamicText(t *testing.T) {
	blocks := ApplyCacheScopes([]SystemPromptBlock{{
		Text:   "static text\n\n" + SystemPromptDynamicBoundary + "\n\ndynamic text",
		Source: "built_in",
		Name:   "combined",
		Cache:  true,
	}}, CacheScopeOptions{GlobalSafe: true})

	if len(blocks) != 2 {
		t.Fatalf("expected boundary to split into two blocks, got %#v", blocks)
	}
	if blocks[0].Text != "static text" || !blocks[0].Cache || blocks[0].CacheScope != CacheScopeGlobal {
		t.Fatalf("unexpected static block after split: %#v", blocks[0])
	}
	if blocks[1].Text != "dynamic text" || blocks[1].Cache || blocks[1].CacheScope != "" {
		t.Fatalf("unexpected dynamic block after split: %#v", blocks[1])
	}
}

func TestApplyCacheScopes_FallsBackToOrgWhenGlobalUnsafeOrToolMarker(t *testing.T) {
	for name, opts := range map[string]CacheScopeOptions{
		"global unsafe": {GlobalSafe: false},
		"tool marker":   {GlobalSafe: true, ToolCacheMarker: true},
	} {
		t.Run(name, func(t *testing.T) {
			blocks := ApplyCacheScopes([]SystemPromptBlock{{
				Text:  "stable static prompt",
				Cache: true,
			}}, opts)
			if len(blocks) != 1 || !blocks[0].Cache || blocks[0].CacheScope != CacheScopeOrg {
				t.Fatalf("expected org cached static block, got %#v", blocks)
			}
		})
	}
}
