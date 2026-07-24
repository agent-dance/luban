package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

func (t *TodoWriteTool) CheckPermissions(_ context.Context, input map[string]any, _ types.ToolPermissionRequest) (types.ToolPermissionResult, error) {
	return types.ToolPermissionResult{Behavior: types.PermissionBehaviorAllow, UpdatedInput: input}, nil
}

// RuntimeScope centralizes session/team/task resolution so task/todo/cron
// stores don't each invent their own environment fallback chain.
type RuntimeScope struct {
	mu                     sync.RWMutex
	barrierMu              sync.RWMutex
	sessionBarrier         *sync.RWMutex
	permissionTransitionMu sync.Mutex
	projectRoot            string
	allowedDirs            []string
	interactive            bool
	permissionMode         string
	permissionModeSetter   func(string) error
	permissionModeObserver func(string)
	provider               string
	model                  string
	features               map[string]bool
	allowedTools           map[string]bool
	deniedTools            map[string]bool
	allowedRules           []types.PermissionRuleValue
	deniedRules            []types.PermissionRuleValue
	askRules               []types.PermissionRuleValue
	sessionIDFunc          func() string
	teammateTeamNameFunc   func() string
	teamNameFunc           func() string
	agentIDFunc            func() string
}

func (s *RuntimeScope) SetSessionBarrier(barrier *sync.RWMutex) {
	if s == nil {
		return
	}
	s.barrierMu.Lock()
	s.sessionBarrier = barrier
	s.barrierMu.Unlock()
}

func (s *RuntimeScope) lockSessionSnapshot() func() {
	s.barrierMu.RLock()
	barrier := s.sessionBarrier
	s.barrierMu.RUnlock()
	if barrier == nil {
		return func() {}
	}
	barrier.RLock()
	return barrier.RUnlock
}

func NewRuntimeScope(projectRoot string, interactive bool) *RuntimeScope {
	return &RuntimeScope{
		projectRoot:    filepath.Clean(projectRoot),
		interactive:    interactive,
		permissionMode: "default",
		features:       make(map[string]bool),
		deniedTools:    make(map[string]bool),
	}
}

func (s *RuntimeScope) SetSessionIDFunc(fn func() string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessionIDFunc = fn
}

func (s *RuntimeScope) SetTeamNameFunc(fn func() string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.teamNameFunc = fn
}

// SetTeammateTeamNameFunc registers the request-local/in-process teammate
// resolver. It has higher task-list precedence than process and leader state.
func (s *RuntimeScope) SetTeammateTeamNameFunc(fn func() string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.teammateTeamNameFunc = fn
}

// SetAgentIDFunc registers the resolver used by TD-01 to isolate todo lists
// per subagent. When the func returns a non-empty string, TodoKey/TodoPath
// scope the file to that agent so a subagent cannot overwrite its parent's
// list.
func (s *RuntimeScope) SetAgentIDFunc(fn func() string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.agentIDFunc = fn
}

func (s *RuntimeScope) SetProjectRoot(root string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.projectRoot = filepath.Clean(strings.TrimSpace(root))
}

func (s *RuntimeScope) SetAllowedDirs(dirs []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.allowedDirs = append([]string(nil), dirs...)
}

func (s *RuntimeScope) SetProviderInfo(provider, model string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.provider = strings.TrimSpace(provider)
	s.model = strings.TrimSpace(model)
}

// SetPermissionModeDispatcher connects plan-mode transitions to the runtime's
// real permission controller. The getter is sampled once to initialize the
// session-facing mode; subsequent changes must go through TransitionPermissionMode
// so registry policy and UI observers see the same state.
func (s *RuntimeScope) SetPermissionModeDispatcher(getter func() string, setter func(string) error) {
	s.permissionTransitionMu.Lock()
	defer s.permissionTransitionMu.Unlock()
	mode := "default"
	if getter != nil {
		if current := strings.TrimSpace(getter()); current != "" {
			mode = current
		}
	}
	s.mu.Lock()
	s.permissionMode = mode
	s.permissionModeSetter = setter
	observer := s.permissionModeObserver
	s.mu.Unlock()
	if observer != nil {
		observer(mode)
	}
}

// SetPermissionModeObserver registers the runtime/UI state sink. Registering an
// observer immediately publishes the current mode so resumed plan sessions do
// not render a stale default-mode badge.
func (s *RuntimeScope) SetPermissionModeObserver(observer func(string)) {
	s.permissionTransitionMu.Lock()
	defer s.permissionTransitionMu.Unlock()
	s.mu.Lock()
	s.permissionModeObserver = observer
	mode := s.permissionMode
	s.mu.Unlock()
	if observer != nil {
		observer(mode)
	}
}

// TransitionPermissionMode updates the underlying permission dispatcher first,
// then atomically publishes the same mode to registry visibility and UI state.
func (s *RuntimeScope) TransitionPermissionMode(mode string) error {
	return s.transitionPermissionMode(mode, true)
}

// RestorePermissionMode updates the runtime permission controller without
// notifying the UI observer. Session lifecycle code uses this before publishing
// the matching presentation snapshot so a new mode can never appear beside the
// previous session's transcript.
func (s *RuntimeScope) RestorePermissionMode(mode string) error {
	return s.transitionPermissionMode(mode, false)
}

func (s *RuntimeScope) transitionPermissionMode(mode string, notifyObserver bool) error {
	mode = strings.TrimSpace(mode)
	if mode == "" {
		return i18n.NewError(i18n.KeyToolPermissionModeRequired)
	}
	s.permissionTransitionMu.Lock()
	defer s.permissionTransitionMu.Unlock()
	s.mu.RLock()
	setter := s.permissionModeSetter
	s.mu.RUnlock()
	if setter != nil {
		if err := setter(mode); err != nil {
			return err
		}
	}
	s.mu.Lock()
	s.permissionMode = mode
	observer := s.permissionModeObserver
	s.mu.Unlock()
	if notifyObserver && observer != nil {
		observer(mode)
	}
	return nil
}

func (s *RuntimeScope) PermissionMode() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.permissionMode
}

func (s *RuntimeScope) SetFeatureGate(name string, enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.features == nil {
		s.features = make(map[string]bool)
	}
	s.features[name] = enabled
}

func (s *RuntimeScope) SetDeniedTools(names []string) {
	_, rules := permissionRuleSet(names)
	s.mu.Lock()
	s.deniedTools = permissionBlanketToolMap(rules)
	s.deniedRules = rules
	s.mu.Unlock()
}

func (s *RuntimeScope) SetAllowedTools(names []string) {
	if len(names) == 0 {
		s.mu.Lock()
		s.allowedTools = nil
		s.mu.Unlock()
		return
	}
	allowed, rules := permissionRuleSet(names)
	s.mu.Lock()
	if len(allowed) == 0 {
		s.allowedTools = nil
	} else {
		s.allowedTools = allowed
	}
	s.allowedRules = rules
	s.mu.Unlock()
}

func (s *RuntimeScope) SetAskTools(names []string) {
	_, rules := permissionRuleSet(names)
	s.mu.Lock()
	s.askRules = rules
	s.mu.Unlock()
}

func (s *RuntimeScope) ApplyPermissionUpdates(updates []types.PermissionUpdate) {
	if len(updates) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, update := range updates {
		target := permissionRulesForBehavior(s, update.Behavior)
		if target == nil {
			continue
		}
		switch update.Type {
		case types.PermissionUpdateAddRules:
			*target = appendPermissionRules(*target, update.Rules...)
		case types.PermissionUpdateReplaceRules:
			*target = appendPermissionRules(nil, update.Rules...)
		case types.PermissionUpdateRemoveRules:
			*target = removePermissionRules(*target, update.Rules...)
		}
		s.rebuildPermissionToolMapsLocked()
	}
}

func permissionRulesForBehavior(s *RuntimeScope, behavior types.PermissionBehavior) *[]types.PermissionRuleValue {
	switch behavior {
	case types.PermissionBehaviorAllow:
		return &s.allowedRules
	case types.PermissionBehaviorDeny:
		return &s.deniedRules
	case types.PermissionBehaviorAsk:
		return &s.askRules
	default:
		return nil
	}
}

func permissionRuleSet(specs []string) (map[string]bool, []types.PermissionRuleValue) {
	rules := make([]types.PermissionRuleValue, 0, len(specs))
	for _, spec := range specs {
		rule := parsePermissionRuleSpec(spec)
		if rule.ToolName != "" {
			rules = appendPermissionRules(rules, rule)
		}
	}
	if len(rules) == 0 {
		return nil, nil
	}
	tools := make(map[string]bool, len(rules))
	for _, rule := range rules {
		// Content-specific rules still make the base tool visible. The
		// per-tool CheckPermissions path owns matching ruleContent.
		tools[rule.ToolName] = true
	}
	return tools, rules
}

func parsePermissionRuleSpec(spec string) types.PermissionRuleValue {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return types.PermissionRuleValue{}
	}
	open := strings.Index(spec, "(")
	if open <= 0 || !strings.HasSuffix(spec, ")") {
		return types.PermissionRuleValue{ToolName: spec}
	}
	toolName := strings.TrimSpace(spec[:open])
	ruleContent := strings.TrimSpace(spec[open+1 : len(spec)-1])
	if toolName == "" {
		return types.PermissionRuleValue{}
	}
	return types.PermissionRuleValue{ToolName: toolName, RuleContent: ruleContent}
}

func appendPermissionRules(existing []types.PermissionRuleValue, rules ...types.PermissionRuleValue) []types.PermissionRuleValue {
	out := append([]types.PermissionRuleValue(nil), existing...)
	seen := make(map[types.PermissionRuleValue]bool, len(out)+len(rules))
	for _, rule := range out {
		seen[rule] = true
	}
	for _, rule := range rules {
		rule.ToolName = strings.TrimSpace(rule.ToolName)
		rule.RuleContent = strings.TrimSpace(rule.RuleContent)
		if rule.ToolName == "" || seen[rule] {
			continue
		}
		seen[rule] = true
		out = append(out, rule)
	}
	return out
}

func removePermissionRules(existing []types.PermissionRuleValue, rules ...types.PermissionRuleValue) []types.PermissionRuleValue {
	if len(existing) == 0 || len(rules) == 0 {
		return append([]types.PermissionRuleValue(nil), existing...)
	}
	remove := make(map[types.PermissionRuleValue]bool, len(rules))
	for _, rule := range rules {
		rule.ToolName = strings.TrimSpace(rule.ToolName)
		rule.RuleContent = strings.TrimSpace(rule.RuleContent)
		if rule.ToolName != "" {
			remove[rule] = true
		}
	}
	out := make([]types.PermissionRuleValue, 0, len(existing))
	for _, rule := range existing {
		if !remove[rule] {
			out = append(out, rule)
		}
	}
	return out
}

func (s *RuntimeScope) rebuildPermissionToolMapsLocked() {
	s.allowedTools = permissionToolMap(s.allowedRules)
	s.deniedTools = permissionBlanketToolMap(s.deniedRules)
}

func permissionToolMap(rules []types.PermissionRuleValue) map[string]bool {
	if len(rules) == 0 {
		return nil
	}
	out := make(map[string]bool, len(rules))
	for _, rule := range rules {
		if rule.ToolName != "" {
			out[rule.ToolName] = true
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func permissionBlanketToolMap(rules []types.PermissionRuleValue) map[string]bool {
	if len(rules) == 0 {
		return nil
	}
	out := make(map[string]bool, len(rules))
	for _, rule := range rules {
		if rule.ToolName != "" && rule.RuleContent == "" {
			out[rule.ToolName] = true
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (s *RuntimeScope) SessionID() string {
	unlock := s.lockSessionSnapshot()
	defer unlock()
	s.mu.RLock()
	fn := s.sessionIDFunc
	s.mu.RUnlock()
	if fn != nil {
		if value := strings.TrimSpace(fn()); value != "" {
			return value
		}
	}
	return strings.TrimSpace(os.Getenv("CLAUDE_SESSION_ID"))
}

func (s *RuntimeScope) TeamName() string {
	if processTeam := strings.TrimSpace(os.Getenv("CLAUDE_CODE_TEAM_NAME")); processTeam != "" {
		return processTeam
	}
	return s.LeaderTeamName()
}

func (s *RuntimeScope) LeaderTeamName() string {
	s.mu.RLock()
	fn := s.teamNameFunc
	s.mu.RUnlock()
	if fn != nil {
		if value := strings.TrimSpace(fn()); value != "" {
			return value
		}
	}
	return ""
}

func (s *RuntimeScope) TeammateTeamName() string {
	s.mu.RLock()
	fn := s.teammateTeamNameFunc
	s.mu.RUnlock()
	if fn != nil {
		return strings.TrimSpace(fn())
	}
	return ""
}

func (s *RuntimeScope) TaskListID() string {
	if explicit := strings.TrimSpace(os.Getenv("CLAUDE_CODE_TASK_LIST_ID")); explicit != "" {
		return explicit
	}
	if teammateTeam := s.TeammateTeamName(); teammateTeam != "" {
		return teammateTeam
	}
	if processTeam := strings.TrimSpace(os.Getenv("CLAUDE_CODE_TEAM_NAME")); processTeam != "" {
		return processTeam
	}
	if leaderTeam := s.LeaderTeamName(); leaderTeam != "" {
		return leaderTeam
	}
	if sessionID := s.SessionID(); sessionID != "" {
		return sessionID
	}
	return "default"
}

// AgentID returns the resolved subagent identifier, or "" when the runtime is
// running on the parent context. TD-01: a non-empty AgentID makes TodoKey
// scope todo files to the subagent so it can't overwrite its parent.
func (s *RuntimeScope) AgentID() string {
	s.mu.RLock()
	fn := s.agentIDFunc
	s.mu.RUnlock()
	if fn != nil {
		if value := strings.TrimSpace(fn()); value != "" {
			return value
		}
	}
	return strings.TrimSpace(os.Getenv("CLAUDE_CODE_AGENT_ID"))
}

func (s *RuntimeScope) TodoKey() string {
	// TD-01: agent-id takes precedence so a subagent has its own todo list.
	if agentID := s.AgentID(); agentID != "" {
		return agentID
	}
	if sessionID := s.SessionID(); sessionID != "" {
		return sessionID
	}
	return "default"
}

func (s *RuntimeScope) TodoPath(projectRoot string) string {
	root := strings.TrimSpace(projectRoot)
	if root == "" {
		root = s.ProjectRoot()
	}
	if root == "" {
		root = "."
	}
	return filepath.Join(root, ".claude", "todos", sanitizeTaskPathComponent(s.TodoKey())+".json")
}

func (s *RuntimeScope) CronFilePath(projectRoot string) string {
	root := strings.TrimSpace(projectRoot)
	if root == "" {
		root = s.ProjectRoot()
	}
	if root == "" {
		root = "."
	}
	return filepath.Join(root, ".claude", "scheduled_tasks.json")
}

func (s *RuntimeScope) ProjectRoot() string {
	unlock := s.lockSessionSnapshot()
	defer unlock()
	s.mu.RLock()
	defer s.mu.RUnlock()
	if strings.TrimSpace(s.projectRoot) == "" {
		return "."
	}
	return s.projectRoot
}

func (s *RuntimeScope) IsTodoV2Enabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("CLAUDE_CODE_ENABLE_TASKS"))) {
	case "1", "true", "yes", "on":
		return true
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.interactive
}

// ToolRuntimeContext returns the shared session snapshot consumed by registry
// visibility and tool-specific pre-execution permission checks.
func (s *RuntimeScope) ToolRuntimeContext() types.ToolRuntimeContext {
	unlock := s.lockSessionSnapshot()
	defer unlock()
	return s.toolRuntimeContextUnbarriered()
}

// ToolRuntimeContextUnbarriered is for callers that already own the shared
// session publication lock (read or write).
func (s *RuntimeScope) ToolRuntimeContextUnbarriered() types.ToolRuntimeContext {
	return s.toolRuntimeContextUnbarriered()
}

func (s *RuntimeScope) toolRuntimeContextUnbarriered() types.ToolRuntimeContext {
	s.mu.RLock()
	features := make(map[string]bool, len(s.features)+1)
	for name, enabled := range s.features {
		features[name] = enabled
	}
	denied := make(map[string]bool, len(s.deniedTools))
	for name, value := range s.deniedTools {
		denied[name] = value
	}
	var allowed map[string]bool
	if s.allowedTools != nil {
		allowed = make(map[string]bool, len(s.allowedTools))
		for name, value := range s.allowedTools {
			allowed[name] = value
		}
	}
	ctx := types.ToolRuntimeContext{
		ProjectRoot:    s.projectRoot,
		AllowedDirs:    append([]string(nil), s.allowedDirs...),
		Interactive:    s.interactive,
		PermissionMode: s.permissionMode,
		Provider:       s.provider,
		Model:          s.model,
		Features:       features,
		AllowedTools:   allowed,
		DeniedTools:    denied,
		AllowedRules:   append([]types.PermissionRuleValue(nil), s.allowedRules...),
		DeniedRules:    append([]types.PermissionRuleValue(nil), s.deniedRules...),
		AskRules:       append([]types.PermissionRuleValue(nil), s.askRules...),
	}
	sessionIDFunc := s.sessionIDFunc
	s.mu.RUnlock()
	if sessionIDFunc != nil {
		ctx.SessionID = strings.TrimSpace(sessionIDFunc())
	}
	if ctx.SessionID == "" {
		ctx.SessionID = strings.TrimSpace(os.Getenv("CLAUDE_SESSION_ID"))
	}
	ctx.AgentID = s.AgentID()
	ctx.ChannelsActive = AskUserChannelsActive()
	ctx.Features[types.ToolFeatureTaskV2] = s.IsTodoV2Enabled()
	return ctx
}
