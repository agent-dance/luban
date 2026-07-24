package tools

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/agent-dance/luban/i18n"
)

type PlanAllowedPrompt struct {
	Tool   string `json:"tool"`
	Prompt string `json:"prompt"`
}

// planStateSchemaVersion is incremented whenever persistedPlanState gains
// non-additive fields. PM-06: forward-compatible loaders use this to migrate
// older payloads (e.g., when a TS leader writes a newer schema and a Go
// follower reads it). The version is intentionally simple — semantic
// versioning is overkill for an internal cache file.
const planStateSchemaVersion = 1

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
	// InterviewPhase is set non-empty while a multi-turn interview is in
	// flight. PM-04: EnterPlanMode refuses re-entry until the interview
	// concludes so the pre-plan permission snapshot is not corrupted by a
	// nested transition.
	InterviewPhase              string `json:"interview_phase,omitempty"`
	HasExitedPlanMode           bool   `json:"has_exited_plan_mode,omitempty"`
	NeedsPlanModeExitAttachment bool   `json:"needs_plan_mode_exit_attachment,omitempty"`
	NeedsAutoModeExitAttachment bool   `json:"needs_auto_mode_exit_attachment,omitempty"`
}

// PlanState holds shared state for plan mode: whether it is active and the
// path to the plan file that was created when plan mode was entered.
// All fields are unexported; callers use the accessor methods.
type PlanState struct {
	mu                          sync.RWMutex
	transitionMu                sync.Mutex
	active                      bool
	planFile                    string
	allowedPrompts              []PlanAllowedPrompt
	enteredAt                   *time.Time
	projectRoot                 string
	stateFile                   string
	interviewPhase              string
	prePlanState                map[string]any
	permissionRuntime           planModePermissionRuntime
	hasExitedPlanMode           bool
	needsPlanModeExitAttachment bool
	needsAutoModeExitAttachment bool
}

func (s *PlanState) bindPermissionRuntime(runtime planModePermissionRuntime) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.permissionRuntime = runtime
	s.mu.Unlock()
}

// NewPlanState returns an initialised PlanState (plan mode off).
func NewPlanState(projectRootOpt ...string) *PlanState {
	root := ""
	if len(projectRootOpt) > 0 {
		root = strings.TrimSpace(projectRootOpt[0])
	}
	if root == "" {
		if cwd, err := os.Getwd(); err == nil {
			root = cwd
		}
	}
	state := &PlanState{
		projectRoot: root,
		stateFile:   planStateFile(root),
	}
	_ = state.loadFromDisk()
	return state
}

func planStateFile(projectRoot string) string {
	root := strings.TrimSpace(projectRoot)
	if root == "" {
		root = "."
	}
	return filepath.Join(root, ".claude", "plan-mode.json")
}

func (s *PlanState) loadFromDisk() error {
	if strings.TrimSpace(s.stateFile) == "" {
		return nil
	}
	data, err := os.ReadFile(s.stateFile)
	if err != nil {
		return err
	}
	var persisted persistedPlanState
	if err := json.Unmarshal(data, &persisted); err != nil {
		return err
	}
	// PM-06: forward-compatible migration. Older payloads (no schema_version
	// field) are treated as v0 and silently upgraded; future schema bumps
	// can branch here. Unknown extra fields in newer payloads are ignored
	// thanks to encoding/json semantics.
	if persisted.SchemaVersion > planStateSchemaVersion {
		// Newer file than this binary understands — keep what we can but
		// proceed cautiously: ignore fields that may have changed semantics.
		s.active = persisted.Active
		s.planFile = persisted.PlanFile
		s.allowedPrompts = append([]PlanAllowedPrompt{}, persisted.AllowedPrompts...)
		s.enteredAt = persisted.EnteredAt
		s.interviewPhase = persisted.InterviewPhase
		s.prePlanState = clonePlanSnapshot(persisted.PrePlanModeSnapshot)
		s.hasExitedPlanMode = persisted.HasExitedPlanMode
		s.needsPlanModeExitAttachment = persisted.NeedsPlanModeExitAttachment
		s.needsAutoModeExitAttachment = persisted.NeedsAutoModeExitAttachment
		return nil
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
		s.active = true
		s.planFile = persisted.PlanFile
		s.allowedPrompts = append([]PlanAllowedPrompt{}, persisted.AllowedPrompts...)
		s.enteredAt = persisted.EnteredAt
		s.interviewPhase = persisted.InterviewPhase
		s.prePlanState = clonePlanSnapshot(persisted.PrePlanModeSnapshot)
		s.hasExitedPlanMode = persisted.HasExitedPlanMode
		s.needsPlanModeExitAttachment = persisted.NeedsPlanModeExitAttachment
		s.needsAutoModeExitAttachment = persisted.NeedsAutoModeExitAttachment
		if s.prePlanState == nil {
			// Legacy active state predates prePlanMode capture. A valid plan file
			// proves the state is live; default is the only non-escalating restore.
			s.prePlanState = map[string]any{"permission_mode": permissionModeDefault, "auto_mode": false}
		}
		return nil
	}
	s.active = false
	s.planFile = ""
	s.allowedPrompts = nil
	s.enteredAt = nil
	s.interviewPhase = persisted.InterviewPhase
	s.prePlanState = clonePlanSnapshot(persisted.PrePlanModeSnapshot)
	s.hasExitedPlanMode = persisted.HasExitedPlanMode
	s.needsPlanModeExitAttachment = persisted.NeedsPlanModeExitAttachment
	s.needsAutoModeExitAttachment = persisted.NeedsAutoModeExitAttachment
	return nil
}

func (s *PlanState) persistLocked() error {
	if strings.TrimSpace(s.stateFile) == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.stateFile), 0755); err != nil {
		return err
	}
	body := persistedPlanState{
		SchemaVersion:               planStateSchemaVersion,
		Active:                      s.active,
		PlanFile:                    s.planFile,
		AllowedPrompts:              append([]PlanAllowedPrompt{}, s.allowedPrompts...),
		EnteredAt:                   s.enteredAt,
		InterviewPhase:              s.interviewPhase,
		PrePlanModeSnapshot:         s.prePlanState,
		HasExitedPlanMode:           s.hasExitedPlanMode,
		NeedsPlanModeExitAttachment: s.needsPlanModeExitAttachment,
		NeedsAutoModeExitAttachment: s.needsAutoModeExitAttachment,
	}
	data, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(s.stateFile, append(data, '\n'), 0644)
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

func (s *PlanState) AllowedPrompts() []PlanAllowedPrompt {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]PlanAllowedPrompt{}, s.allowedPrompts...)
}

// InterviewPhase returns the current interview-phase tag, or "" when no
// interview is in flight. PM-04 callers consult this before a plan-mode
// transition to refuse re-entry.
func (s *PlanState) InterviewPhase() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.interviewPhase
}

// SetInterviewPhase records that an interview phase is in progress. Pass ""
// to clear the phase. The state is persisted so a process restart still
// observes the in-flight interview.
func (s *PlanState) SetInterviewPhase(phase string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.interviewPhase = strings.TrimSpace(phase)
	_ = s.persistLocked()
}

// PrePlanState exposes the opaque permission snapshot stored alongside plan
// state. Returns nil when no snapshot has been recorded.
func (s *PlanState) PrePlanState() map[string]any {
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

// SetPrePlanState records the opaque permission snapshot the runtime wants to
// restore on plan-mode exit. Cross-process resumes pick this back up.
func (s *PlanState) SetPrePlanState(snapshot map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if snapshot == nil {
		s.prePlanState = nil
	} else {
		clone := make(map[string]any, len(snapshot))
		for k, v := range snapshot {
			clone[k] = v
		}
		s.prePlanState = clone
	}
	_ = s.persistLocked()
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
	prompts := s.AllowedPrompts()
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

// enter atomically sets plan mode active and records the plan file path.
func (s *PlanState) enter(planFile string) {
	_ = s.enterWithSnapshot(planFile, nil)
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
	s.needsPlanModeExitAttachment = false
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
				"auto_mode":       isAutoLikePermissionMode(mode),
			}
		}
	}
	return s.enterWithSnapshot(planFile, snapshot)
}

// exit atomically clears plan mode but persists allowedPrompts so a later
// EnterPlanMode (or downstream Bash call) can replay the user's incremental
// approvals. TS reference: src/tools/EnterPlanModeTool/ExitPlanModeTool.ts
// at line 131 keeps the prompt list around after exit.
func (s *PlanState) exit() {
	s.mu.RLock()
	restoreMode, _ := s.prePlanState["permission_mode"].(string)
	allowed := append([]PlanAllowedPrompt(nil), s.allowedPrompts...)
	s.mu.RUnlock()
	if strings.TrimSpace(restoreMode) == "" {
		restoreMode = permissionModeDefault
	}
	_ = s.commitExit(allowed, restoreMode, false)
}

// commitExit is the single plan-exit commit point. It restores the runtime
// permission mode first, then persists the inactive state. Any persistence
// failure rolls both memory and the runtime dispatcher back to plan mode.
func (s *PlanState) commitExit(allowed []PlanAllowedPrompt, restoreMode string, needsAutoAttachment bool) error {
	return s.commitExitWithOptions(allowed, restoreMode, needsAutoAttachment, false, true)
}

// ExitForSessionRestore makes an explicit session snapshot authoritative over
// any project-scoped pre-plan mode. It updates the permission dispatcher
// without publishing an intermediate UI mode and does not manufacture a model
// attachment for a user approval that did not occur.
func (s *PlanState) ExitForSessionRestore(restoreMode string) error {
	s.mu.RLock()
	allowed := append([]PlanAllowedPrompt(nil), s.allowedPrompts...)
	s.mu.RUnlock()
	return s.commitExitWithOptions(allowed, restoreMode, false, true, false)
}

// ExitForModeSwitch records a real interactive plan exit while deferring UI
// publication to the mode-switch owner that already updated the presentation.
func (s *PlanState) ExitForModeSwitch(restoreMode string) error {
	s.mu.RLock()
	allowed := append([]PlanAllowedPrompt(nil), s.allowedPrompts...)
	s.mu.RUnlock()
	return s.commitExitWithOptions(allowed, restoreMode, false, true, true)
}

func (s *PlanState) commitExitWithOptions(allowed []PlanAllowedPrompt, restoreMode string, needsAutoAttachment, quiet, recordExit bool) error {
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
	previousHasExited := s.hasExitedPlanMode
	previousNeedsExit := s.needsPlanModeExitAttachment
	previousNeedsAuto := s.needsAutoModeExitAttachment
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
	s.hasExitedPlanMode = recordExit
	s.needsPlanModeExitAttachment = recordExit
	s.needsAutoModeExitAttachment = recordExit && needsAutoAttachment
	if err := s.persistLocked(); err != nil {
		s.active = previousActive
		s.planFile = previousPlanFile
		s.allowedPrompts = previousAllowed
		s.enteredAt = previousEnteredAt
		s.prePlanState = previousPrePlan
		s.hasExitedPlanMode = previousHasExited
		s.needsPlanModeExitAttachment = previousNeedsExit
		s.needsAutoModeExitAttachment = previousNeedsAuto
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

func (s *PlanState) HasExitedPlanMode() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.hasExitedPlanMode
}

// ConsumePlanModeExitAttachments returns and clears the one-shot lifecycle
// flags consumed by prompt/compaction attachment assembly.
func (s *PlanState) ConsumePlanModeExitAttachments() (planExit, autoExit bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	planExit = s.needsPlanModeExitAttachment
	autoExit = s.needsAutoModeExitAttachment
	s.needsPlanModeExitAttachment = false
	s.needsAutoModeExitAttachment = false
	_ = s.persistLocked()
	return planExit, autoExit
}

// Exit is the exported version of exit — clears plan mode.
// Used by the TUI Shift+Tab mode switching.
func (s *PlanState) Exit() {
	s.exit()
}

func (s *PlanState) SetAllowedPrompts(prompts []PlanAllowedPrompt) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.allowedPrompts = append([]PlanAllowedPrompt{}, prompts...)
	_ = s.persistLocked()
}

// SetProjectRoot retargets the persisted plan-mode file to a new project root.
// Existing in-memory state is replaced with the on-disk state for that project.
func (s *PlanState) SetProjectRoot(root string) {
	s.mu.Lock()
	s.projectRoot = strings.TrimSpace(root)
	s.stateFile = planStateFile(s.projectRoot)
	s.active = false
	s.planFile = ""
	s.allowedPrompts = nil
	s.enteredAt = nil
	s.interviewPhase = ""
	s.prePlanState = nil
	s.hasExitedPlanMode = false
	s.needsPlanModeExitAttachment = false
	s.needsAutoModeExitAttachment = false
	s.mu.Unlock()
	_ = s.loadFromDisk()
}
