package prompt

import (
	"fmt"
	"os"
	"strings"

	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/types"
)

// Config holds system prompt assembly configuration
type Config struct {
	// CustomInstructions is additional instructions (for example, from LUBAN.md).
	CustomInstructions string
	// ToolDescriptions is optional late-bound tool guidance. Provider-facing
	// definitions remain the authoritative catalog.
	ToolDescriptions string
	// CWD is the current working directory shown to the model
	CWD string
	// AdditionalDirs are extra working directories available to the model.
	AdditionalDirs []string
	// ModelID is the exact model identifier, if known.
	ModelID string
	// ModelDescription is a human-readable model name/description, if known.
	ModelDescription string
	// KnowledgeCutoff is the assistant knowledge cutoff, if known.
	KnowledgeCutoff string
	// Language is the preferred natural language for assistant responses.
	Language string
	// OutputStyle names the runtime-selected response style.
	OutputStyle string
}

// SystemPromptParts holds the static and dynamic portions of the system prompt.
// Static content (role and invariant workflow) is stable across sessions and
// suitable for long-term prompt caching. Dynamic content (environment, runtime
// response settings, and optional late tool notes) must not be cached.
type SystemPromptParts struct {
	Static  string // role and invariant workflow – stable, cache-eligible
	Dynamic string // environment and runtime settings – session-specific
}

// SystemPrompt is the ordered system prompt representation used by the request
// pipeline. It mirrors the TypeScript path's string-array model while allowing
// block metadata to be carried through to provider-specific serializers.
type SystemPrompt []SystemPromptBlock

// SystemPromptBlock is a single text block in a system prompt.
type SystemPromptBlock struct {
	Text       string `json:"text"`
	Source     string `json:"source,omitempty"`
	Name       string `json:"name,omitempty"`
	Cache      bool   `json:"cache,omitempty"`
	CacheScope string `json:"cache_scope,omitempty"`
}

// Texts returns the block text values in order, omitting empty text blocks.
func (p SystemPrompt) Texts() []string {
	texts := make([]string, 0, len(p))
	for _, block := range p {
		if block.Text == "" {
			continue
		}
		texts = append(texts, block.Text)
	}
	return texts
}

// JoinedText joins system prompt blocks for providers that require one string.
func (p SystemPrompt) JoinedText() string {
	return strings.Join(p.Texts(), "\n\n")
}

// BuildSystemPromptParts constructs the system prompt split into static and dynamic
// sections. Use the Static part as the first element of provider.Params.SystemParts
// so it receives a cache_control breakpoint; append Dynamic as the second element.
func BuildSystemPromptParts(tools []types.Tool, cfg Config) SystemPromptParts {
	return buildSystemPromptParts(buildStaticPrompt(tools, cfg), cfg)
}

// BuildSystemPromptPartsForDefinitions constructs a prompt from the exact
// provider-facing catalog. It prevents prompt guidance from being derived from
// a broader execution registry than the schemas sent in the same envelope.
func BuildSystemPromptPartsForDefinitions(tools []types.ToolDefinition, cfg Config) SystemPromptParts {
	return buildSystemPromptParts(buildStaticPromptForDefinitions(tools, cfg), cfg)
}

func buildSystemPromptParts(static string, cfg Config) SystemPromptParts {
	// Simple mode is a legacy diagnostics surface. It must never erase the
	// Agentic V2 correctness kernel while the exact coding catalog is active.
	if isTruthyPromptEnv(os.Getenv("LUBAN_CODE_SIMPLE")) && !strings.Contains(static, "# Coding contract") {
		return SystemPromptParts{
			Dynamic: buildSimpleSystemPrompt(cfg),
		}
	}

	cwd := cfg.CWD
	if cwd == "" {
		var err error
		if cwd, err = os.Getwd(); err != nil {
			cwd = "."
		}
	}
	dynamic := []string{EnvironmentContextBuilder{
		PrimaryCWD:       cwd,
		AdditionalDirs:   cfg.AdditionalDirs,
		ModelID:          cfg.ModelID,
		ModelDescription: cfg.ModelDescription,
		KnowledgeCutoff:  cfg.KnowledgeCutoff,
	}.Build()}
	if runtimeSettings := runtimeSettingsSection(cfg); runtimeSettings != "" {
		dynamic = append(dynamic, runtimeSettings)
	}
	if toolDescriptions := strings.TrimSpace(cfg.ToolDescriptions); toolDescriptions != "" {
		dynamic = append(dynamic, toolDescriptions)
	}

	return SystemPromptParts{
		Static:  static,
		Dynamic: strings.Join(dynamic, "\n\n"),
	}
}

// BuildSystemPromptBlocks constructs the system prompt as ordered text blocks.
// This is the preferred builder for provider callers.
func BuildSystemPromptBlocks(tools []types.Tool, cfg Config) SystemPrompt {
	p := BuildSystemPromptParts(tools, cfg)
	return systemPromptBlocks(p)
}

// BuildSystemPromptBlocksForDefinitions is the block-form counterpart of
// BuildSystemPromptPartsForDefinitions.
func BuildSystemPromptBlocksForDefinitions(tools []types.ToolDefinition, cfg Config) SystemPrompt {
	return systemPromptBlocks(BuildSystemPromptPartsForDefinitions(tools, cfg))
}

func systemPromptBlocks(p SystemPromptParts) SystemPrompt {
	blocks := make(SystemPrompt, 0, 2)
	if p.Static != "" {
		blocks = append(blocks, SystemPromptBlock{
			Text:       p.Static,
			Source:     "built_in",
			Name:       "static",
			Cache:      true,
			CacheScope: "ephemeral",
		})
	}
	if p.Dynamic != "" {
		blocks = append(blocks, SystemPromptBlock{
			Text:   p.Dynamic,
			Source: "runtime",
			Name:   "dynamic",
		})
	}
	return blocks
}

// BuildSystemPrompt constructs the full system prompt as plain text.
func BuildSystemPrompt(tools []types.Tool, cfg Config) string {
	return BuildSystemPromptBlocks(tools, cfg).JoinedText()
}

func buildSimpleSystemPrompt(cfg Config) string {
	cwd := cfg.CWD
	if cwd == "" {
		var err error
		if cwd, err = os.Getwd(); err != nil {
			cwd = "."
		}
	}
	return fmt.Sprintf("You are %s, an agentic coding CLI.\n\nCWD: %s", brand.DisplayName, cwd)
}

func isTruthyPromptEnv(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}
