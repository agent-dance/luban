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
	// ToolDescriptions will be auto-generated from tools if empty
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
// Static content (role definition, tool descriptions) is stable across sessions and
// suitable for long-term prompt caching. Dynamic content (working directory)
// changes per session and should not be cached.
type SystemPromptParts struct {
	Static  string // tool descriptions, role definition – stable, cache-eligible
	Dynamic string // environment info – session-specific
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
	if isTruthyPromptEnv(os.Getenv("LUBAN_CODE_SIMPLE")) {
		return SystemPromptParts{
			Dynamic: buildSimpleSystemPrompt(cfg),
		}
	}

	// --- Static section: role identity + global tool-use guidance ---
	static := buildStaticPrompt(tools, cfg)

	cwd := cfg.CWD
	if cwd == "" {
		var err error
		if cwd, err = os.Getwd(); err != nil {
			cwd = "."
		}
	}
	environment := EnvironmentContextBuilder{
		PrimaryCWD:       cwd,
		AdditionalDirs:   cfg.AdditionalDirs,
		ModelID:          cfg.ModelID,
		ModelDescription: cfg.ModelDescription,
		KnowledgeCutoff:  cfg.KnowledgeCutoff,
	}.Build()

	return SystemPromptParts{
		Static:  static,
		Dynamic: environment,
	}
}

// BuildSystemPromptBlocks constructs the system prompt as ordered text blocks.
// This is the preferred builder for provider callers.
func BuildSystemPromptBlocks(tools []types.Tool, cfg Config) SystemPrompt {
	p := BuildSystemPromptParts(tools, cfg)
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
