// Package skills provides skill loading, parsing, and management.
//
// Architecture: this package is the skill ENGINE (loading, parsing, discovery).
// The Skill tool in internal/tools/skill consumes this package to expose skills to
// the model via the Tool interface.
package skills

// SkillSource indicates where a skill was loaded from.
// Matches the TS SettingSource | 'builtin' | 'mcp' | 'plugin' | 'bundled'
// and CommandBase.loadedFrom.
type SkillSource string

const (
	SourceProject SkillSource = "project" // .luban-code/skills/
	SourceUser    SkillSource = "user"    // ~/.luban-code/skills/
	SourceManaged SkillSource = "managed" // policy settings (enterprise)
	SourcePlugin  SkillSource = "plugin"  // plugin system
	SourceMCP     SkillSource = "mcp"     // MCP server
	SourceBundled SkillSource = "bundled" // built-in skills
)

// SkillContext controls how the skill is executed.
// Matches TS PromptCommand.context.
type SkillContext string

const (
	ContextInline SkillContext = "inline" // default: expand into current conversation
	ContextFork   SkillContext = "fork"   // run as sub-agent with isolated context
)

// Skill represents a fully parsed skill definition.
// This aligns with the TS Command (CommandBase + PromptCommand) type.
type Skill struct {
	// --- Identity ---
	Name        string `json:"name"`        // skill name (directory or file name)
	Description string `json:"description"` // human-readable description (from frontmatter or fallback)

	// --- Source tracking ---
	Source   SkillSource `json:"source"`    // where this skill was loaded from
	FilePath string      `json:"file_path"` // absolute path to the skill .md file
	SkillDir string      `json:"skill_dir"` // directory containing the skill file

	// --- Content ---
	RawContent string `json:"-"` // original file content including frontmatter
	Content    string `json:"-"` // content with frontmatter stripped

	// --- Frontmatter fields (from YAML) ---

	// AllowedTools restricts which tools the model can use while this skill
	// is active. Matches TS PromptCommand.allowedTools.
	AllowedTools []string `json:"allowed_tools,omitempty"`

	// ArgumentHint is displayed in the UI after the command name.
	// Matches TS CommandBase.argumentHint.
	ArgumentHint string `json:"argument_hint,omitempty"`

	// ArgNames are the named parameters this skill accepts (e.g., ["file", "mode"]).
	// When present, $arg_name substitution is performed on the prompt.
	ArgNames []string `json:"arg_names,omitempty"`

	// WhenToUse provides detailed usage scenarios for the model.
	// Matches TS CommandBase.whenToUse.
	WhenToUse string `json:"when_to_use,omitempty"`

	// Version is the skill version string.
	// Matches TS CommandBase.version.
	Version string `json:"version,omitempty"`

	// Model overrides the model used when executing this skill.
	// Matches TS PromptCommand.model.
	Model string `json:"model,omitempty"`

	// DisableModelInvocation prevents the model from calling this skill.
	// Matches TS CommandBase.disableModelInvocation.
	DisableModelInvocation bool `json:"disable_model_invocation,omitempty"`

	// UserInvocable controls whether users can type /skill-name.
	// Default depends on source: commands/ = true, skills/ = false.
	// Matches TS CommandBase.userInvocable.
	UserInvocable *bool `json:"user_invocable,omitempty"`

	// Context controls execution mode: inline (default) or fork.
	// Matches TS PromptCommand.context.
	Context SkillContext `json:"context,omitempty"`

	// Agent specifies the agent type when Context == "fork".
	// Matches TS PromptCommand.agent.
	Agent string `json:"agent,omitempty"`

	// Effort overrides the reasoning effort level.
	// Matches TS PromptCommand.effort.
	Effort string `json:"effort,omitempty"`

	// Paths are gitignore-style glob patterns for conditional activation.
	// When set, the skill only activates when matching files are touched.
	// Matches TS PromptCommand.paths.
	Paths []string `json:"paths,omitempty"`

	// Shell specifies the shell for !`cmd` blocks: "bash" or "powershell".
	// Matches TS FrontmatterData.shell.
	Shell string `json:"shell,omitempty"`

	// ContentLength is the character length of the skill content (used for
	// token estimation in prompt budget). Matches TS PromptCommand.contentLength.
	ContentLength int `json:"content_length"`

	// --- Computed fields ---

	// IsHidden means this skill should not appear in typeahead/help.
	// Matches TS CommandBase.isHidden.
	IsHidden bool `json:"is_hidden,omitempty"`

	// HasUserSpecifiedDescription is true when the description came from
	// frontmatter (explicitly set by the user). Skills from plugin/MCP
	// sources without this flag (and without WhenToUse) are excluded from
	// model-invocable listings. Matches TS CommandBase.hasUserSpecifiedDescription.
	HasUserSpecifiedDescription bool `json:"has_user_specified_description,omitempty"`

	// HasGeneratedDescription identifies the first-party fallback description.
	// Presentation surfaces use it to re-render that copy in the active
	// language; authored descriptions remain untouched.
	HasGeneratedDescription bool `json:"has_generated_description,omitempty"`
}

// effectiveDescription returns the description including whenToUse for catalog projection.
func (s *Skill) effectiveDescription() string {
	if s.WhenToUse != "" {
		return s.Description + " - " + s.WhenToUse
	}
	return s.Description
}
