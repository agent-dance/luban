package tools

func (m *TeamManager) currentTeamNameLocked() string {
	if m == nil {
		return ""
	}
	info := m.teams[m.currentTeamIDLocked()]
	if info == nil {
		return ""
	}
	return info.Name
}

// SetTaskListChangeNotifier connects leader team lifecycle changes to task
// watchers. The callback runs outside the manager lock and must be non-blocking.
func (m *TeamManager) SetTaskListChangeNotifier(notifier func()) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.taskListChanged = notifier
	m.mu.Unlock()
}

func (m *TeamManager) SetTaskStore(store *TaskStore) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.taskStore = store
	m.mu.Unlock()
}

func (m *TeamManager) notifyTaskListChanged() {
	if m == nil {
		return
	}
	m.mu.Lock()
	notify := m.taskListChanged
	m.mu.Unlock()
	if notify != nil {
		func() {
			defer func() { _ = recover() }()
			notify()
		}()
	}
}
