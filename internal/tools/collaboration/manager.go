package collaboration

import (
	"path/filepath"
	"strings"
	"sync"

	runtimestore "github.com/agent-dance/luban/internal/store/runtime"
	"github.com/agent-dance/luban/skills"
	"github.com/agent-dance/luban/swarm"
)

// RuntimeIdentity is the immutable session identity published by the app.
// Team tools deliberately retain only the fields they need to select durable
// team state; permission policy and mutable registries stay in their owners.
type RuntimeIdentity struct {
	SessionID   string
	ProjectRoot string
	AgentID     string
	Model       string
}

type teamOwnerKey struct {
	SessionID   string
	ProjectRoot string
}

type teamInfo struct {
	ID               string
	Name             string
	StorageName      string
	OwnerSessionID   string
	OwnerProjectRoot string
}

type teamSnapshot struct {
	ID          string
	Name        string
	StorageName string
	Owner       teamOwnerKey
}

// TeamManager owns the in-process projection of durable team state.
type TeamManager struct {
	mu            sync.Mutex
	mutationMu    sync.Mutex
	teams         map[string]*teamInfo
	activeByOwner map[teamOwnerKey]string
	nextTeamID    int
	runtime       RuntimeIdentity
	lifecycle     *runtimestore.RuntimeLifecycle
	skills        *skills.Manager
	taskChanged   func()
}

func NewTeamManager(skillManager *skills.Manager) *TeamManager {
	return &TeamManager{
		teams:         make(map[string]*teamInfo),
		activeByOwner: make(map[teamOwnerKey]string),
		skills:        skillManager,
	}
}

// PublishRuntimeIdentity atomically retargets the manager to the active
// session and restores only a strictly matching durable team projection.
func (m *TeamManager) PublishRuntimeIdentity(identity RuntimeIdentity) {
	if m == nil {
		return
	}
	m.mutationMu.Lock()
	defer m.mutationMu.Unlock()
	identity.SessionID = strings.TrimSpace(identity.SessionID)
	identity.ProjectRoot = canonicalTeamOwnerRoot(identity.ProjectRoot)
	identity.AgentID = strings.TrimSpace(identity.AgentID)
	identity.Model = strings.TrimSpace(identity.Model)

	m.mu.Lock()
	before := m.currentTeamNameLocked()
	m.runtime = identity
	m.lifecycle = nil
	if identity.ProjectRoot != "" {
		m.lifecycle = runtimestore.NewRuntimeLifecycle(identity.ProjectRoot)
	}
	m.restoreLifecycleTeamLocked()
	after := m.currentTeamNameLocked()
	m.mu.Unlock()
	if before != after {
		m.notifyTaskListChanged()
	}
}

func (m *TeamManager) CurrentTeamName() string {
	snapshot, ok := m.currentTeamSnapshot()
	if !ok {
		return ""
	}
	return snapshot.Name
}

func (m *TeamManager) runtimeIdentitySnapshot() RuntimeIdentity {
	if m == nil {
		return RuntimeIdentity{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.runtime
}

func (m *TeamManager) currentTeamSnapshot() (teamSnapshot, bool) {
	if m == nil {
		return teamSnapshot{}, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	owner := m.currentTeamOwnerLocked()
	id := m.activeByOwner[owner]
	info := m.teams[id]
	if info == nil || !teamInfoOwnedBy(info, owner) {
		return teamSnapshot{}, false
	}
	return snapshotTeamInfo(info), true
}

func snapshotTeamInfo(info *teamInfo) teamSnapshot {
	if info == nil {
		return teamSnapshot{}
	}
	return teamSnapshot{
		ID:          info.ID,
		Name:        info.Name,
		StorageName: info.StorageName,
		Owner: teamOwnerKey{
			SessionID:   info.OwnerSessionID,
			ProjectRoot: info.OwnerProjectRoot,
		},
	}
}

func (m *TeamManager) currentTeamNameLocked() string {
	owner := m.currentTeamOwnerLocked()
	info := m.teams[m.activeByOwner[owner]]
	if info == nil || !teamInfoOwnedBy(info, owner) {
		return ""
	}
	return info.Name
}

func (m *TeamManager) currentTeamOwnerLocked() teamOwnerKey {
	if m == nil {
		return teamOwnerKey{}
	}
	return teamOwnerKey{
		SessionID:   strings.TrimSpace(m.runtime.SessionID),
		ProjectRoot: canonicalTeamOwnerRoot(m.runtime.ProjectRoot),
	}
}

func (m *TeamManager) withMutation(commit func() error) error {
	if m == nil || commit == nil {
		return nil
	}
	m.mutationMu.Lock()
	defer m.mutationMu.Unlock()
	return commit()
}

// SetTaskListChangeNotifier publishes team-scope changes to task watchers.
func (m *TeamManager) SetTaskListChangeNotifier(notifier func()) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.taskChanged = notifier
	m.mu.Unlock()
}

func (m *TeamManager) notifyTaskListChanged() {
	if m == nil {
		return
	}
	m.mu.Lock()
	notify := m.taskChanged
	m.mu.Unlock()
	if notify == nil {
		return
	}
	func() {
		defer func() { _ = recover() }()
		notify()
	}()
}

func (m *TeamManager) lifecycleForOwner(owner teamOwnerKey) *runtimestore.RuntimeLifecycle {
	if m == nil || owner.SessionID == "" || owner.ProjectRoot == "" {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.currentTeamOwnerLocked() != owner {
		return runtimestore.NewRuntimeLifecycle(owner.ProjectRoot)
	}
	if m.lifecycle == nil {
		m.lifecycle = runtimestore.NewRuntimeLifecycle(owner.ProjectRoot)
	}
	return m.lifecycle
}

func (m *TeamManager) restoreLifecycleTeamLocked() {
	owner := m.currentTeamOwnerLocked()
	if owner.SessionID == "" || owner.ProjectRoot == "" || m.lifecycle == nil {
		return
	}
	delete(m.activeByOwner, owner)
	active, err := m.lifecycle.ActiveState()
	if err != nil {
		return
	}
	for i := len(active) - 1; i >= 0; i-- {
		event := active[i]
		if event.Type != runtimestore.LifecycleTeamCreate ||
			strings.TrimSpace(event.SessionID) != owner.SessionID {
			continue
		}
		payloadRoot := canonicalTeamOwnerRoot(lifecyclePayloadString(event.Payload, "owner_project_root"))
		storageName := strings.TrimSpace(lifecyclePayloadString(event.Payload, "storage_name"))
		name := strings.TrimSpace(lifecyclePayloadString(event.Payload, "name"))
		if payloadRoot == "" || payloadRoot != owner.ProjectRoot || storageName == "" || name == "" {
			continue
		}
		config, loadErr := swarm.LoadTeamConfig(storageName)
		if loadErr != nil || strings.TrimSpace(config.Name) != name || strings.TrimSpace(config.LeadSessionID) != owner.SessionID {
			continue
		}
		info := &teamInfo{
			ID:               event.EntityID,
			Name:             name,
			StorageName:      storageName,
			OwnerSessionID:   owner.SessionID,
			OwnerProjectRoot: owner.ProjectRoot,
		}
		if strings.TrimSpace(info.ID) == "" {
			continue
		}
		m.teams[info.ID] = info
		m.activeByOwner[owner] = info.ID
		return
	}
}

func teamInfoOwnedBy(info *teamInfo, owner teamOwnerKey) bool {
	return info != nil &&
		strings.TrimSpace(info.OwnerSessionID) == owner.SessionID &&
		canonicalTeamOwnerRoot(info.OwnerProjectRoot) == owner.ProjectRoot
}

func canonicalTeamOwnerRoot(root string) string {
	root = strings.TrimSpace(root)
	if root == "" {
		return ""
	}
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	root = filepath.Clean(root)
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = filepath.Clean(resolved)
	}
	return root
}

func lifecyclePayloadString(payload map[string]any, key string) string {
	value, _ := payload[key].(string)
	return strings.TrimSpace(value)
}
