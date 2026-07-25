package prompt

import "strings"

const (
	// SystemPromptDynamicBoundary separates stable system prompt text from
	// per-request dynamic text when both appear in the same source block.
	SystemPromptDynamicBoundary = "__SYSTEM_PROMPT_DYNAMIC_BOUNDARY__"

	CacheScopeEphemeral = "ephemeral"
	CacheScopeGlobal    = "global"
	CacheScopeOrg       = "org"
)

// CacheScopeOptions controls how cache-eligible system blocks are scoped.
type CacheScopeOptions struct {
	// GlobalSafe allows cache-eligible static prompt text to use the global
	// Anthropic cache scope. When false, eligible text falls back to org scope.
	GlobalSafe bool
	// ToolCacheMarker indicates that tool definitions carry their own cache
	// marker, so system prompt cache scopes should avoid global sharing.
	ToolCacheMarker bool
}

// ApplyCacheScopes returns a copy of blocks with static/dynamic cache metadata.
// Cache-eligible static blocks become global when safe and org
// otherwise. Dynamic blocks remain uncached. If a block contains the dynamic
// boundary marker, it is split into a cache-eligible static block followed by an
// uncached dynamic block.
func ApplyCacheScopes(blocks []SystemPromptBlock, opts CacheScopeOptions) []SystemPromptBlock {
	if len(blocks) == 0 {
		return nil
	}
	scope := CacheScopeOrg
	if opts.GlobalSafe && !opts.ToolCacheMarker {
		scope = CacheScopeGlobal
	}

	out := make([]SystemPromptBlock, 0, len(blocks)+1)
	for _, block := range blocks {
		if block.Text == "" {
			continue
		}
		if strings.Contains(block.Text, SystemPromptDynamicBoundary) {
			parts := strings.SplitN(block.Text, SystemPromptDynamicBoundary, 2)
			if staticText := strings.TrimSpace(parts[0]); staticText != "" {
				staticBlock := block
				staticBlock.Text = staticText
				markStaticCacheScope(&staticBlock, scope)
				out = append(out, staticBlock)
			}
			if len(parts) > 1 {
				if dynamicText := strings.TrimSpace(parts[1]); dynamicText != "" {
					dynamicBlock := block
					dynamicBlock.Text = dynamicText
					dynamicBlock.Cache = false
					dynamicBlock.CacheScope = ""
					if dynamicBlock.Name == "" || dynamicBlock.Name == block.Name {
						dynamicBlock.Name = "dynamic"
					}
					out = append(out, dynamicBlock)
				}
			}
			continue
		}
		if block.Cache {
			markStaticCacheScope(&block, scope)
		} else {
			block.CacheScope = ""
		}
		out = append(out, block)
	}
	return out
}

func markStaticCacheScope(block *SystemPromptBlock, scope string) {
	block.Cache = true
	if scope == "" {
		scope = CacheScopeOrg
	}
	block.CacheScope = scope
}
