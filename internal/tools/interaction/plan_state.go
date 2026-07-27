package interaction

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/agent-dance/luban/i18n"
	storepaths "github.com/agent-dance/luban/internal/store/paths"
	"github.com/agent-dance/luban/internal/store/secureio"
)

type PlanAllowedPrompt struct {
	Tool   string `json:"tool"`
	Prompt string `json:"prompt"`
}

const planStateSchemaVersion = 2

type persistedPlanState struct {
	SchemaVersion  int                 `json:"schema_version,omitempty"`
	Active         bool                `json:"active"`
	PlanFile       string              `json:"plan_file,omitempty"`
	AllowedPrompts []PlanAllowedPrompt `json:"allowed_prompts,omitempty"`
	EnteredAt      *time.Time          `json:"entered_at,omitempty"`
	// PrePlanModeSnapshot captures the permission state before plan-mode
	// transition so EnterPlanMode/ExitPlanMode can restore it cleanly. The
	// field is opaque (the runtime owns the schema); we keep it in the
	// persisted state so cross-process resumes don't lose context.
	PrePlanModeSnapshot map[string]any `json:"pre_plan_mode_snapshot,omitempty"`
}

// PlanState holds shared state for plan mode: whether it is active and the
// path to the plan file that was created when plan mode was entered.
// All fields are unexported; callers use the accessor methods.
type PlanState struct {
	mu                sync.RWMutex
	transitionMu      sync.Mutex
	active            bool
	planFile          string
	allowedPrompts    []PlanAllowedPrompt
	enteredAt         *time.Time
	projectRoot       string
	stateFile         string
	prePlanState      map[string]any
	permissionRuntime planModePermissionRuntime
}

// PreparedPlanState is an immutable, validated project-root snapshot. Session
// switching prepares it before any runtime consumer advances, then applies it
// infallibly inside the registry publication barrier.
type PreparedPlanState struct {
	projectRoot    string
	stateFile      string
	active         bool
	planFile       string
	allowedPrompts []PlanAllowedPrompt
	enteredAt      *time.Time
	prePlanState   map[string]any
}

func (s *PlanState) bindPermissionRuntime(runtime planModePermissionRuntime) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.permissionRuntime = runtime
	s.mu.Unlock()
}

// NewPlanState constructs project-root-anchored state and reports malformed or
// unreadable persisted state. A missing state file is a clean inactive state.
func NewPlanState(projectRoot string) (*PlanState, error) {
	prepared, err := preparePlanState(projectRoot)
	state := &PlanState{}
	if prepared != nil {
		state.ApplyPreparedProject(prepared)
	}
	return state, err
}

func planStateFile(projectRoot string) string {
	return filepath.Join(storepaths.RuntimeServiceDir(projectRoot, "plan"), "plan-mode.json")
}

func planArtifactsDir(projectRoot string) string {
	return filepath.Join(storepaths.RuntimeServiceDir(projectRoot, "plan"), "plans")
}

func preparePlanState(projectRoot string) (*PreparedPlanState, error) {
	root := strings.TrimSpace(projectRoot)
	if root == "" {
		return nil, i18n.NewError(i18n.KeyToolIndirectPlanStateProjectRootRequired)
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, i18n.WrapError(i18n.KeyToolIndirectPlanStateResolveProjectRoot, err, projectRoot)
	}
	root = filepath.Clean(root)
	info, err := os.Stat(root)
	if err != nil {
		return nil, i18n.WrapError(i18n.KeyToolIndirectPlanStateResolveProjectRoot, err, projectRoot)
	}
	if !info.IsDir() {
		return nil, i18n.WrapError(i18n.KeyToolIndirectPlanStateResolveProjectRoot, fs.ErrInvalid, projectRoot)
	}
	prepared := &PreparedPlanState{projectRoot: root, stateFile: planStateFile(root)}
	data, err := secureio.ReadPrivateRuntimeRegularFile(prepared.stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return prepared, nil
		}
		return prepared, i18n.WrapError(i18n.KeyToolIndirectPlanStateLoad, err, prepared.stateFile)
	}
	var persisted persistedPlanState
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&persisted); err != nil {
		return prepared, i18n.WrapError(i18n.KeyToolIndirectPlanStateDecode, err, prepared.stateFile)
	}
	if persisted.SchemaVersion != planStateSchemaVersion {
		return prepared, i18n.NewError(i18n.KeyToolIndirectPlanStateSchemaVersion, persisted.SchemaVersion, planStateSchemaVersion)
	}
	planFileExists := false
	if strings.TrimSpace(persisted.PlanFile) != "" {
		_, statErr := os.Stat(persisted.PlanFile)
		planFileExists = statErr == nil
	}
	if persisted.Active && (planFileExists || len(persisted.PrePlanModeSnapshot) > 0) {
		// EnterPlanMode intentionally reserves but does not materialize the plan
		// file. Active permission state therefore cannot be inferred from file
		// existence: doing so drops plan mode on every pre-write resume.
		prepared.active = true
		prepared.planFile = persisted.PlanFile
		prepared.allowedPrompts = append([]PlanAllowedPrompt(nil), persisted.AllowedPrompts...)
		prepared.enteredAt = cloneTimePointer(persisted.EnteredAt)
		prepared.prePlanState = clonePlanSnapshot(persisted.PrePlanModeSnapshot)
	}
	return prepared, nil
}

func cloneTimePointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

// PrepareProjectRoot validates and loads another project's state without
// mutating the live session.
func (s *PlanState) PrepareProjectRoot(projectRoot string) (*PreparedPlanState, error) {
	return preparePlanState(projectRoot)
}

// ApplyPreparedProject publishes a previously validated project snapshot.
func (s *PlanState) ApplyPreparedProject(prepared *PreparedPlanState) {
	if s == nil || prepared == nil {
		return
	}
	s.transitionMu.Lock()
	defer s.transitionMu.Unlock()
	s.mu.Lock()
	s.projectRoot = prepared.projectRoot
	s.stateFile = prepared.stateFile
	s.active = prepared.active
	s.planFile = prepared.planFile
	s.allowedPrompts = append([]PlanAllowedPrompt(nil), prepared.allowedPrompts...)
	s.enteredAt = cloneTimePointer(prepared.enteredAt)
	s.prePlanState = clonePlanSnapshot(prepared.prePlanState)
	s.mu.Unlock()
}

func (s *PlanState) persistLocked() error {
	if strings.TrimSpace(s.stateFile) == "" {
		return i18n.NewError(i18n.KeyToolIndirectPlanStateProjectRootRequired)
	}
	if err := secureio.EnsurePrivateRuntimeDirectory(filepath.Dir(s.stateFile)); err != nil {
		return err
	}
	body := persistedPlanState{
		SchemaVersion:       planStateSchemaVersion,
		Active:              s.active,
		PlanFile:            s.planFile,
		AllowedPrompts:      append([]PlanAllowedPrompt{}, s.allowedPrompts...),
		EnteredAt:           s.enteredAt,
		PrePlanModeSnapshot: s.prePlanState,
	}
	data, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		return err
	}
	return secureio.AtomicWritePrivateRuntimeFile(s.stateFile, append(data, '\n'))
}

// IsActive reports whether plan mode is currently active.
func (s *PlanState) IsActive() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.active
}

// PlanFile returns the path to the current plan file, or "" if plan mode is
// not active or no file has been created yet.
func (s *PlanState) PlanFile() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.planFile
}

func (s *PlanState) projectRootPath() string {
	if s == nil {
		return ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.projectRoot
}

// Fork snapshots the plan gate for an isolated agent filesystem scope. Agent
// registries do not expose the interactive plan-transition tools, so the fork
// deliberately carries no permission runtime and cannot mutate the foreground
// session's transition state.
func (s *PlanState) Fork(projectRoot string) *PlanState {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	root := strings.TrimSpace(projectRoot)
	if root == "" {
		root = s.projectRoot
	}
	var enteredAt *time.Time
	if s.enteredAt != nil {
		value := *s.enteredAt
		enteredAt = &value
	}
	return &PlanState{
		active:         s.active,
		planFile:       s.planFile,
		allowedPrompts: append([]PlanAllowedPrompt(nil), s.allowedPrompts...),
		enteredAt:      enteredAt,
		projectRoot:    root,
		stateFile:      planStateFile(root),
		prePlanState:   clonePlanSnapshot(s.prePlanState),
	}
}

func (s *PlanState) allowedPromptSnapshot() []PlanAllowedPrompt {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]PlanAllowedPrompt{}, s.allowedPrompts...)
}

func (s *PlanState) prePlanSnapshot() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.prePlanState == nil {
		return nil
	}
	out := make(map[string]any, len(s.prePlanState))
	for k, v := range s.prePlanState {
		out[k] = v
	}
	return out
}

func clonePlanSnapshot(snapshot map[string]any) map[string]any {
	if snapshot == nil {
		return nil
	}
	clone := make(map[string]any, len(snapshot))
	for key, value := range snapshot {
		clone[key] = value
	}
	return clone
}

// AllowedPromptMatches reports whether the given (tool, prompt) pair has been
// approved during plan mode. PM-05: TS supports `glob:` and `regex:` prefixes
// plus case-insensitive matching; previously Go used literal string equality
// only, silently dropping persisted prompts that used pattern matchers.
//
// Plain entries match case-insensitively as a literal prefix; this mirrors
// the TS behavior where Bash command rules typically match the leading
// command segment ("npm test" matches "npm test --watch").
func (s *PlanState) AllowedPromptMatches(tool, prompt string) bool {
	tool = strings.TrimSpace(tool)
	prompt = strings.TrimSpace(prompt)
	if tool == "" || prompt == "" {
		return false
	}
	prompts := s.allowedPromptSnapshot()
	for _, allowed := range prompts {
		if !strings.EqualFold(allowed.Tool, tool) {
			continue
		}
		if matchAllowedPrompt(allowed.Prompt, prompt) {
			return true
		}
	}
	return false
}

func matchAllowedPrompt(rule, candidate string) bool {
	rule = strings.TrimSpace(rule)
	candidate = strings.TrimSpace(candidate)
	if rule == "" {
		return false
	}
	switch {
	case strings.HasPrefix(rule, "regex:"):
		pattern := strings.TrimPrefix(rule, "regex:")
		// Add (?i) to honour TS case-insensitivity unless the rule explicitly
		// embeds its own flags.
		if !strings.HasPrefix(pattern, "(?") {
			pattern = "(?i)" + pattern
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			return false
		}
		return re.MatchString(candidate)
	case strings.HasPrefix(rule, "glob:"):
		return globMatch(strings.TrimPrefix(rule, "glob:"), candidate)
	default:
		// Literal prefix match (case-insensitive). Mirrors TS where Bash
		// allowlist rules approve commands sharing the rule's exact prefix.
		return strings.HasPrefix(strings.ToLower(candidate), strings.ToLower(rule))
	}
}

// globMatch implements a small case-insensitive glob matcher supporting
// `*` (any run of chars including spaces) and `?` (any single char). It is
// purposely simple: PlanAllowedPrompts use plain shell-style globs, not full
// path-glob semantics.
func globMatch(pattern, candidate string) bool {
	pattern = strings.ToLower(pattern)
	candidate = strings.ToLower(candidate)
	return globMatchRec(pattern, candidate)
}

func globMatchRec(pattern, s string) bool {
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '*':
			// Try matching zero or more characters.
			rest := pattern[i+1:]
			for j := 0; j <= len(s); j++ {
				if globMatchRec(rest, s[j:]) {
					return true
				}
			}
			return false
		case '?':
			if len(s) == 0 {
				return false
			}
			s = s[1:]
		default:
			if len(s) == 0 || s[0] != pattern[i] {
				return false
			}
			s = s[1:]
		}
	}
	return len(s) == 0
}

func (s *PlanState) enterWithSnapshot(planFile string, snapshot map[string]any) error {
	s.transitionMu.Lock()
	defer s.transitionMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	previousActive := s.active
	previousPlanFile := s.planFile
	previousAllowedPrompts := append([]PlanAllowedPrompt(nil), s.allowedPrompts...)
	previousEnteredAt := s.enteredAt
	previousPrePlanState := clonePlanSnapshot(s.prePlanState)
	s.active = true
	s.planFile = planFile
	s.allowedPrompts = nil
	s.prePlanState = clonePlanSnapshot(snapshot)
	now := time.Now().UTC()
	s.enteredAt = &now
	if err := s.persistLocked(); err != nil {
		s.active = previousActive
		s.planFile = previousPlanFile
		s.allowedPrompts = previousAllowedPrompts
		s.enteredAt = previousEnteredAt
		s.prePlanState = previousPrePlanState
		return err
	}
	return nil
}

// Enter is the exported version of enter — sets plan mode active with the
// given plan file path. Used by the TUI Shift+Tab mode switching.
func (s *PlanState) Enter(planFile string) error {
	var snapshot map[string]any
	s.mu.RLock()
	runtime := s.permissionRuntime
	s.mu.RUnlock()
	if runtime != nil {
		mode := strings.TrimSpace(runtime.ToolRuntimeContext().PermissionMode)
		if mode != "" && mode != permissionModePlan {
			snapshot = map[string]any{
				"permission_mode": mode,
			}
		}
	}
	return s.enterWithSnapshot(planFile, snapshot)
}

// commitExit is the single plan-exit commit point. It restores the runtime
// permission mode first, then persists the inactive state. Any persistence
// failure rolls both memory and the runtime dispatcher back to plan mode.
func (s *PlanState) commitExit(allowed []PlanAllowedPrompt, restoreMode string) error {
	return s.commitExitWithOptions(allowed, restoreMode, false)
}

// ExitForSessionRestore makes an explicit session snapshot authoritative over
// any project-scoped pre-plan mode. It updates the permission dispatcher
// without publishing an intermediate UI mode and does not manufacture a model
// attachment for a user approval that did not occur.
func (s *PlanState) ExitForSessionRestore(restoreMode string) error {
	s.mu.RLock()
	allowed := append([]PlanAllowedPrompt(nil), s.allowedPrompts...)
	s.mu.RUnlock()
	return s.commitExitWithOptions(allowed, restoreMode, true)
}

// ExitForModeSwitch commits an interactive plan exit while deferring UI
// publication to the mode-switch owner that already updated the presentation.
func (s *PlanState) ExitForModeSwitch(restoreMode string) error {
	s.mu.RLock()
	allowed := append([]PlanAllowedPrompt(nil), s.allowedPrompts...)
	s.mu.RUnlock()
	return s.commitExitWithOptions(allowed, restoreMode, true)
}

func (s *PlanState) commitExitWithOptions(allowed []PlanAllowedPrompt, restoreMode string, quiet bool) error {
	if s == nil {
		return i18n.NewError(i18n.KeyToolIndirectPlanStateRequired)
	}
	s.transitionMu.Lock()
	defer s.transitionMu.Unlock()

	s.mu.RLock()
	if !s.active {
		s.mu.RUnlock()
		return i18n.NewError(i18n.KeyToolIndirectPlanStateNotActive)
	}
	runtime := s.permissionRuntime
	previousActive := s.active
	previousPlanFile := s.planFile
	previousAllowed := append([]PlanAllowedPrompt(nil), s.allowedPrompts...)
	previousEnteredAt := s.enteredAt
	previousPrePlan := clonePlanSnapshot(s.prePlanState)
	s.mu.RUnlock()

	restoreMode = strings.TrimSpace(restoreMode)
	if restoreMode == "" {
		restoreMode = permissionModeDefault
	}
	transition := func(mode string) error {
		if runtime == nil {
			return nil
		}
		if quiet {
			return runtime.RestorePermissionMode(mode)
		}
		return runtime.TransitionPermissionMode(mode)
	}
	if runtime != nil {
		if err := transition(restoreMode); err != nil {
			return i18n.WrapError(i18n.KeyToolIndirectPlanStateRestoreMode, err, restoreMode)
		}
	}

	s.mu.Lock()
	if !s.active || s.planFile != previousPlanFile {
		s.mu.Unlock()
		var rollbackErr error
		if runtime != nil {
			rollbackErr = transition(permissionModePlan)
		}
		return errors.Join(i18n.NewError(i18n.KeyToolIndirectPlanStateChangedDuringExit), rollbackErr)
	}
	s.active = false
	s.planFile = ""
	s.allowedPrompts = append([]PlanAllowedPrompt(nil), allowed...)
	s.enteredAt = nil
	s.prePlanState = nil
	if err := s.persistLocked(); err != nil {
		s.active = previousActive
		s.planFile = previousPlanFile
		s.allowedPrompts = previousAllowed
		s.enteredAt = previousEnteredAt
		s.prePlanState = previousPrePlan
		s.mu.Unlock()
		var rollbackErr error
		if runtime != nil {
			rollbackErr = transition(permissionModePlan)
		}
		return errors.Join(i18n.WrapError(i18n.KeyToolIndirectPlanStatePersistExitedState, err), rollbackErr)
	}
	s.mu.Unlock()
	return nil
}
