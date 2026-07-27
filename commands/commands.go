package commands

import (
	"errors"
	"strings"
	"time"

	"github.com/agent-dance/luban/buildinfo"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/runtime/compact"
	"github.com/agent-dance/luban/internal/runtime/goal"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/types"
)

// SessionListEntry holds session metadata for display and filtering.
type SessionListEntry struct {
	ID           string
	ProjectDir   string
	Title        string
	UpdatedAt    time.Time
	CreatedAt    time.Time
	MessageCount int
	CWD          string
	GitBranch    string
	PreviewText  string
	Provider     string // Phase 5: provider used in session
	Model        string // Phase 5: model used in session
}

// ErrExit is returned by the /exit command to signal the REPL should exit.
var ErrExit = errors.New("exit")

// Command is the interface for slash commands.
type Command interface {
	Name() string
	Aliases() []string
	Description() string
	Execute(ctx *Context, args string) error
}

// DescriptionKeyProvider lets extension commands opt into semantic i18n while
// preserving the long-standing Command interface.
type DescriptionKeyProvider interface {
	DescriptionKey() i18n.Key
}

// CommandDescriptionKey returns the semantic description key for a command.
// Built-ins are resolved by canonical command name; extensions may implement
// DescriptionKeyProvider. Unknown extensions retain their original copy.
func CommandDescriptionKey(cmd Command) (i18n.Key, bool) {
	if cmd == nil {
		return "", false
	}
	if provider, ok := cmd.(DescriptionKeyProvider); ok {
		if key := provider.DescriptionKey(); key != "" {
			return key, true
		}
	}
	return i18n.CommandDescriptionKey(cmd.Name())
}

// LocalizedCommandDescription resolves semantic copy using the active runtime
// language and falls back to extension-provided Description text.
func LocalizedCommandDescription(lang i18n.Language, cmd Command) string {
	if key, ok := CommandDescriptionKey(cmd); ok {
		return i18n.Text(lang, key)
	}
	if cmd == nil {
		return ""
	}
	return cmd.Description()
}

func builtinCommandDescription(name string) string {
	if key, ok := i18n.CommandDescriptionKey(name); ok {
		return i18n.Text(i18n.DetectOrLoadLanguage(), key)
	}
	return name
}

// QueryLooper is the subset of loop.QueryLoop needed by commands.
type QueryLooper interface {
	SetMessagesPreservingToolUseLedger([]types.Message)
	Messages() []types.Message
	Model() string
	SetModel(string)
	ContextUsage() (maxTokens, usedTokens int)
	// SetProvider replaces the active provider for future queries.
	// The implementation should atomically swap the underlying ProviderRef
	// so that in-flight queries are not affected.
	SetProvider(p provider.Provider)
}

// SessionStore is the subset of session.Store needed by slash commands.
type SessionStore interface {
	Save(sessionID string, messages []types.Message) error
	Load(sessionID string) ([]types.Message, error)
	List() ([]SessionListEntry, error)
	Search(query, currentCWD string, allProjects bool) ([]SessionListEntry, error)
	Rename(sessionID, title string) error
}

// GoalRuntime persists goal state without exposing session storage to commands.
type GoalRuntime interface {
	LoadGoal() (*goal.Goal, error)
	SaveGoal(goal.Goal) error
}

// Context holds runtime state available to commands during execution.
type Context struct {
	// Language is the active runtime language for user-visible command output.
	// Its zero value is English, matching i18n.LangEN for standalone callers.
	Language  i18n.Language
	QueryLoop QueryLooper
	OnEvent   func(string)
	// OnCommandPresentation receives renderer-neutral command lifecycle events.
	OnCommandPresentation func(CommandPresentation)
	// OnCommandDomainResult carries the command's explicit business outcome.
	// It is separate from OnEvent so presentation code never has
	// to infer success or failure from localized prose.
	OnCommandDomainResult func(CommandDomainResult)
	CWD                   string
	CurrentProjectDir     string
	SessionID             string
	SessionStore          SessionStore
	GoalRuntime           GoalRuntime
	// OnGoalActivated is notified after a command successfully persists an
	// active goal. REPL surfaces use it to start the first model turn.
	OnGoalActivated          func(string)
	ResumeSession            func(SessionListEntry) error
	TotalInputTokens         int
	TotalOutputTokens        int
	TotalCacheReadTokens     int
	TotalCacheCreationTokens int
	TotalWebSearchRequests   int
	SessionUniqueInputTokens int
	TotalCostUSD             float64
	CostCurrency             string
	CostUnknown              bool
	CurrentModel             string
	AppVersion               string
	BuildDiagnostic          buildinfo.Diagnostic
	// MCPBackend is the live runtime manager for interactive commands. A nil
	// backend lets the command construct one from the canonical settings files.
	MCPBackend MCPBackend
	// SkillManager is the live catalog shared with the query loop and Skill
	// tool. A nil backend makes /skills fail closed instead of editing a
	// disconnected manager.
	SkillManager SkillsBackend
	// SkillInvoker is the origin-aware explicit invocation boundary shared by
	// terminal surfaces. Runtime composition adapts the concrete SkillTool.
	SkillInvoker SkillInvoker
	// ClearView clears only the visible transcript projection.
	ClearView func() error
	// ClearConversation atomically starts an empty conversation while preserving
	// the previous session for later recovery and audit.
	ClearConversation    func() error
	SearchTranscript     func(string) (string, error)
	ExportTranscript     func(string) (string, error)
	OpenTranscriptEditor func(string) error
	OpenDetailEditor     func(string) error
	// OpenModelPicker lets interactive surfaces replace the textual model list
	// while still executing through the command presentation lifecycle.
	OpenModelPicker   func() error
	SetMouseCapture   func(string) (bool, error)
	OpenActivityView  func() string
	CloseActivityView func() string
	ActivityAction    func(string, string) (string, error)
	SetDisclosure     func(string, string) (string, error)
	// OpenForkPicker opens the interactive conversation-history selector used
	// by /fork. The runtime owns the picker because terminal surfaces differ.
	OpenForkPicker func() error
	DeleteHistory  func(string) error
	// CompactFunc, when non-nil, runs structured context compaction.
	// The argument is the custom instruction text from "/compact <args>".
	CompactFunc func(customInstructions string) error

	// Provider-related fields (Phase 4: multi-provider support)
	CurrentProvider          string                     // canonical name of the active provider (e.g. "anthropic")
	ProviderRegistry         *provider.ProviderRegistry // registry of all known providers
	CredentialStore          *provider.CredentialStore  // persistent credential store
	ProviderRuntimeOverrides provider.RuntimeOverrides  // invocation-scoped wire choices retained across provider reconstruction

	// SwitchLanguage switches the display language and persists the preference.
	// Receives an ISO 639-1 code or "next" to cycle.
	// Returns a user-facing message describing the new language.
	SwitchLanguage func(code string) string
}

func estimateUniqueInputTokens(messages []types.Message) int {
	var inputMessages []types.Message
	for _, msg := range messages {
		if msg.Role == types.RoleUser {
			inputMessages = append(inputMessages, msg)
		}
	}
	if len(inputMessages) == 0 {
		return 0
	}
	return compact.NewContextWindow(1).EstimateMessages(inputMessages)
}

// Registry holds all registered commands and supports fast lookup by name or alias.
type Registry struct {
	commands map[string]Command
	ordered  []Command
}

// NewRegistry creates an empty command registry.
func NewRegistry() *Registry {
	return &Registry{commands: make(map[string]Command)}
}

// Register adds a command to the registry. Panics on name/alias collision.
func (r *Registry) Register(cmd Command) {
	cmd = wrapCommandPresentation(cmd)
	r.ordered = append(r.ordered, cmd)
	register := func(key string) {
		if _, exists := r.commands[key]; exists {
			panic("commands: duplicate registration for '" + key + "'")
		}
		r.commands[key] = cmd
	}
	register(cmd.Name())
	for _, alias := range cmd.Aliases() {
		register(alias)
	}
}

// PresentationContract returns the display contract for a registered command
// or alias. exact is false when the conservative extension fallback is used.
func (r *Registry) PresentationContract(name string) (contract CommandPresentationContract, exact bool) {
	cmd := r.Find(name)
	if cmd == nil {
		return fallbackCommandPresentationContract(strings.TrimPrefix(name, "/")), false
	}
	return commandPresentationContract(cmd)
}

// Find looks up a command by name or alias. Returns nil if not found.
func (r *Registry) Find(name string) Command {
	name = strings.TrimPrefix(name, "/")
	return r.commands[name]
}

// All returns all registered commands in registration order.
func (r *Registry) All() []Command {
	out := make([]Command, len(r.ordered))
	copy(out, r.ordered)
	return out
}

// IsCommand reports whether input begins with '/'.
func (r *Registry) IsCommand(input string) bool {
	return strings.HasPrefix(input, "/")
}

// Parse splits a slash-command input into the matching Command and argument string.
func (r *Registry) Parse(input string) (Command, string) {
	if !r.IsCommand(input) {
		return nil, ""
	}
	parts := strings.SplitN(input, " ", 2)
	name := strings.TrimPrefix(parts[0], "/")
	args := ""
	if len(parts) == 2 {
		args = strings.TrimSpace(parts[1])
	}
	cmd := r.commands[name]
	return cmd, args
}
