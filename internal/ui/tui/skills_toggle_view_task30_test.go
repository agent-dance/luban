package tui

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/skills"
	gtui "github.com/grindlemire/go-tui"
)

func TestSkillsMenuDirectListLoadsImmediatelyAndEnterSelectsStableSkillID(t *testing.T) {
	backend := newTask30Backend(t, task30Snapshot(t, 1, task30Skill("skill:project:alpha", "alpha", skills.SourceProject)))
	state, root := task30Root(backend)
	var submitted string
	root.onSubmit = func(input string) { submitted = input }
	request := SkillsMenuOpenRequest{
		SessionID: func() string { return state.SessionID.Get() },
		Language:  func() i18n.Language { return state.Language.Get() },
		Backend:   backend,
	}
	menu := newSkillsMenuState(request)
	state.SkillsMenu.Set(menu)
	root.openSkillsMenu(menu)
	task30WaitFor(t, func() bool {
		menu := state.SkillsMenu.Get()
		return menu != nil && menu.Toggle.HasSnapshot && !menu.Toggle.Loading
	})

	direct := collectElementText(renderSkillsMenuForTest(root, state.SkillsMenu.Get()))
	if !strings.Contains(direct, "[x] alpha") {
		t.Fatalf("direct menu omitted checked skill row: %q", direct)
	}
	for _, obsolete := range []string{"List Skills", "Enable/Disable Skills"} {
		if strings.Contains(direct, obsolete) {
			t.Fatalf("direct menu retained obsolete primary entry %q: %q", obsolete, direct)
		}
	}
	if stopped := dispatchRootKeyForTest(t, root, gtui.KeyEvent{Key: gtui.KeyEnter}); !stopped {
		t.Fatal("direct-list Enter leaked to input")
	}
	if submitted != "/skill:project:alpha " {
		t.Fatalf("Enter submitted %q, want stable skill ID", submitted)
	}
	if backend.toggleCallCount() != 0 || state.SkillsMenu.Get() != nil {
		t.Fatalf("Enter selection state: toggles=%d menu=%#v", backend.toggleCallCount(), state.SkillsMenu.Get())
	}
	if backend.snapshotCallCount() != 1 {
		t.Fatalf("direct open snapshot calls=%d, want one", backend.snapshotCallCount())
	}
	if got := root.inputText.Get(); got != "" {
		t.Fatalf("input after selection=%q, want cleared", got)
	}
}

func TestSkillsMenuEmptyBackspaceStaysOpenAndEscapeCloses(t *testing.T) {
	backend := newTask30Backend(t, task30Snapshot(t, 1, task30Skill("skill:project:alpha", "alpha", skills.SourceProject)))
	state, root := task30Root(backend)
	task30OpenSkills(t, state, root, backend)

	dispatchRootKeyForTest(t, root, gtui.KeyEvent{Key: gtui.KeyBackspace})
	if state.SkillsMenu.Get() == nil {
		t.Fatal("Backspace with an empty filter closed the direct skills menu")
	}

	dispatchRootKeyForTest(t, root, gtui.KeyEvent{Key: gtui.KeyEscape})
	if state.SkillsMenu.Get() != nil {
		t.Fatal("single Esc did not close direct skills menu")
	}
}

func TestSkillsMenuDownThenEnterSelectsHighlightedSkill(t *testing.T) {
	alpha := task30Skill("skill:project:alpha", "alpha", skills.SourceProject)
	beta := task30Skill("skill:project:beta", "beta", skills.SourceProject)
	backend := newTask30Backend(t, task30Snapshot(t, 1, beta, alpha))
	state, root := task30Root(backend)
	var submitted string
	root.onSubmit = func(input string) { submitted = input }
	task30OpenSkills(t, state, root, backend)

	dispatchRootKeyForTest(t, root, gtui.KeyEvent{Key: gtui.KeyDown})
	dispatchRootKeyForTest(t, root, gtui.KeyEvent{Key: gtui.KeyEnter})

	if submitted != "/skill:project:beta " {
		t.Fatalf("Down then Enter submitted %q, want highlighted beta skill", submitted)
	}
	if state.SkillsMenu.Get() != nil {
		t.Fatal("successful skill selection did not close the skills menu")
	}
}

func TestSkillsMenuCheckboxReflectsEveryEffectiveVisibility(t *testing.T) {
	auto := task30Skill("skill:project:auto", "auto", skills.SourceProject)
	nameOnly := task30WithVisibility(task30Skill("skill:project:name", "name-only", skills.SourceProject), skills.VisibilityNameOnly, skills.SkillScopeProject)
	manualOnly := task30WithVisibility(task30Skill("skill:project:manual", "manual-only", skills.SourceProject), skills.VisibilityManualOnly, skills.SkillScopeProject)
	off := task30WithVisibility(task30Skill("skill:project:off", "off", skills.SourceProject), skills.VisibilityOff, skills.SkillScopeProject)
	backend := newTask30Backend(t, task30Snapshot(t, 4, auto, nameOnly, manualOnly, off))
	state, root := task30Root(backend)
	root.termWidth, root.termHeight = 120, 40
	task30OpenSkills(t, state, root, backend)

	rendered := collectElementText(renderSkillsMenuForTest(root, state.SkillsMenu.Get()))
	for _, checked := range []string{"[x] auto", "[x] name-only", "[x] manual-only"} {
		if !strings.Contains(rendered, checked) {
			t.Errorf("effective enabled row omitted %q checkbox: %q", checked, rendered)
		}
	}
	if !strings.Contains(rendered, "[ ] off") {
		t.Errorf("effective off row omitted unchecked checkbox: %q", rendered)
	}
}

func TestSkillsMenuShadowedConfigurationLifecycleKeepsCheckboxAndActivityDistinct(t *testing.T) {
	winner := task30Skill("skill:project:review", "review", skills.SourceProject)
	shadowed := task30Skill("skill:user:review", "review", skills.SourceUser)
	shadowed.ShadowedBy = winner.ID
	shadowed.ModelVisible = false
	shadowed.DescriptionVisible = false
	shadowed.UserInvocable = false
	shadowed.Executable = false

	initial := task30Snapshot(t, 1, winner, shadowed)
	off := task30WithVisibility(shadowed, skills.VisibilityOff, skills.SkillScopeProject)
	offSnapshot := task30Snapshot(t, 2, winner, off)
	reenabled := shadowed
	reenabled.VisibilitySource = skills.SkillScopeProject
	reenabledSnapshot := task30Snapshot(t, 3, winner, reenabled)

	backend := newTask30Backend(t, initial)
	calls := make(chan task30ToggleCall, 2)
	backend.toggle = func(call task30ToggleCall) (skills.ProjectVisibilityToggleResult, error) {
		calls <- call
		switch call.revision {
		case initial.Revision:
			return task30ToggleResult(t, call.id, call.revision, offSnapshot, skills.ProjectVisibilityToggleCommitted, skills.ProjectVisibilityToggleReasonNone), nil
		case offSnapshot.Revision:
			return task30ToggleResult(t, call.id, call.revision, reenabledSnapshot, skills.ProjectVisibilityToggleCommitted, skills.ProjectVisibilityToggleReasonNone), nil
		default:
			return skills.ProjectVisibilityToggleResult{}, fmt.Errorf("unexpected observed revision %d", call.revision)
		}
	}

	state, root := task30Root(backend)
	task30OpenSkills(t, state, root, backend)
	next := state.SkillsMenu.Get().clone()
	for visibleIndex, rowIndex := range next.Toggle.Filtered {
		if next.Toggle.Snapshot.Skills[rowIndex].ID == shadowed.ID {
			next.Toggle.Selected = visibleIndex
			break
		}
	}
	state.SkillsMenu.Set(next)

	for _, lang := range i18n.AllLanguages() {
		state.Language.Set(lang)
		rendered := collectElementText(renderSkillsMenuForTest(root, state.SkillsMenu.Get()))
		want := i18n.Format(lang, i18n.KeySkillsMenuDetailShadowed, winner.ID)
		if !strings.Contains(rendered, "→ [x] review") || !strings.Contains(rendered, want) {
			t.Fatalf("%s shadowed enabled detail=%q, want checked configuration and %q", lang.Code(), rendered, want)
		}
	}
	state.Language.Set(i18n.LangEN)

	dispatchRootKeyForTest(t, root, gtui.KeyEvent{Key: gtui.KeyRune, Rune: ' '})
	first := task30ReceiveToggle(t, calls)
	if first.id != shadowed.ID || first.revision != initial.Revision {
		t.Fatalf("disable call=%#v, want shadowed stable ID at revision %d", first, initial.Revision)
	}
	task30WaitFor(t, func() bool {
		menu := state.SkillsMenu.Get()
		return menu != nil && !menu.Toggle.Loading && menu.Toggle.Snapshot.Revision == offSnapshot.Revision
	})
	offRow, found := state.SkillsMenu.Get().Toggle.Snapshot.Find(shadowed.ID)
	if !found || offRow.Visibility != skills.VisibilityOff {
		t.Fatalf("shadowed row did not authoritatively become off: %#v", offRow)
	}
	rendered := collectElementText(renderSkillsMenuForTest(root, state.SkillsMenu.Get()))
	if !strings.Contains(rendered, "→ [ ] review") || strings.Contains(rendered, i18n.Format(i18n.LangEN, i18n.KeySkillsMenuDetailShadowed, winner.ID)) {
		t.Fatalf("disabled shadowed row did not show configured-off state: %q", rendered)
	}

	dispatchRootKeyForTest(t, root, gtui.KeyEvent{Key: gtui.KeyRune, Rune: ' '})
	second := task30ReceiveToggle(t, calls)
	if second.id != shadowed.ID || second.revision != offSnapshot.Revision {
		t.Fatalf("re-enable call=%#v, want shadowed stable ID at revision %d", second, offSnapshot.Revision)
	}
	task30WaitFor(t, func() bool {
		menu := state.SkillsMenu.Get()
		return menu != nil && !menu.Toggle.Loading && menu.Toggle.Snapshot.Revision == reenabledSnapshot.Revision
	})
	reenabledRow, found := state.SkillsMenu.Get().Toggle.Snapshot.Find(shadowed.ID)
	if !found || reenabledRow.Visibility != skills.VisibilityAuto || reenabledRow.ShadowedBy != winner.ID || reenabledRow.Executable {
		t.Fatalf("shadowed row did not restore auto while remaining inactive: %#v", reenabledRow)
	}
	rendered = collectElementText(renderSkillsMenuForTest(root, state.SkillsMenu.Get()))
	shadowedNotice := i18n.Format(i18n.LangEN, i18n.KeySkillsMenuDetailShadowed, winner.ID)
	if !strings.Contains(rendered, "→ [x] review") || !strings.Contains(rendered, shadowedNotice) {
		t.Fatalf("re-enabled shadowed row lost configuration/activity distinction: %q", rendered)
	}

	active := reenabled
	active.ShadowedBy = ""
	active.ModelVisible = true
	active.DescriptionVisible = true
	active.UserInvocable = true
	active.Executable = true
	activeSnapshot := task30Snapshot(t, 4, active)
	backend.setSnapshot(activeSnapshot)
	dispatchRootKeyForTest(t, root, gtui.KeyEvent{Key: gtui.KeyEscape})
	task30OpenSkills(t, state, root, backend)
	rendered = collectElementText(renderSkillsMenuForTest(root, state.SkillsMenu.Get()))
	row, found := state.SkillsMenu.Get().Toggle.Snapshot.Find(active.ID)
	if !found || !row.Executable || !strings.Contains(rendered, "→ [x] review") || strings.Contains(rendered, shadowedNotice) {
		t.Fatalf("winner removal did not activate row and remove shadow notice: row=%#v rendered=%q", row, rendered)
	}
	if backend.toggleCallCount() != 2 {
		t.Fatalf("shadowed lifecycle issued %d mutations, want exactly two Space calls", backend.toggleCallCount())
	}
}

func TestSpaceSkillToggleUsesStableIDObservedRevisionAndNeverOptimistic(t *testing.T) {
	alpha := task30Skill("skill:project:alpha", "alpha", skills.SourceProject)
	beta := task30Skill("skill:user:beta", "beta", skills.SourceUser)
	initial := task30Snapshot(t, 7, alpha, beta)
	backend := newTask30Backend(t, initial)
	started := make(chan task30ToggleCall, 1)
	release := make(chan struct{})
	committedRow := task30WithVisibility(beta, skills.VisibilityOff, skills.SkillScopeProject)
	committed := task30ToggleResult(t, beta.ID, initial.Revision, task30Snapshot(t, 8, alpha, committedRow), skills.ProjectVisibilityToggleCommitted, skills.ProjectVisibilityToggleReasonNone)
	backend.toggle = func(call task30ToggleCall) (skills.ProjectVisibilityToggleResult, error) {
		started <- call
		<-release
		return committed, nil
	}

	state, root := task30Root(backend)
	task30OpenSkills(t, state, root, backend)
	// Search uses name/summary/source/path/state text and keeps a stable row
	// selection rather than filtering a second catalog implementation.
	dispatchRootKeyForTest(t, root, gtui.KeyEvent{Key: gtui.KeyRune, Rune: 'b'})
	menu := state.SkillsMenu.Get()
	if len(menu.Toggle.Filtered) != 1 || menu.Toggle.selectedRow().ID != beta.ID {
		t.Fatalf("filtered selection=%#v row=%#v", menu.Toggle.Filtered, menu.Toggle.selectedRow())
	}

	if stopped := dispatchRootKeyForTest(t, root, gtui.KeyEvent{Key: gtui.KeyRune, Rune: ' '}); !stopped {
		t.Fatal("Space leaked to input")
	}
	call := task30ReceiveToggle(t, started)
	if call.sessionID != "session-a" || call.id != beta.ID || call.revision != 7 {
		t.Fatalf("toggle call=%#v, want session/stable ID/observed catalog revision", call)
	}
	menu = state.SkillsMenu.Get()
	row, _ := menu.Toggle.Snapshot.Find(beta.ID)
	if !menu.Toggle.Loading || row.Visibility != skills.VisibilityAuto {
		t.Fatalf("optimistic state leaked before receipt: loading=%t row=%#v", menu.Toggle.Loading, row)
	}
	close(release)
	task30WaitFor(t, func() bool {
		menu := state.SkillsMenu.Get()
		return menu != nil && !menu.Toggle.Loading && menu.Toggle.Snapshot.Revision == 8
	})
	menu = state.SkillsMenu.Get()
	row, _ = menu.Toggle.Snapshot.Find(beta.ID)
	if row.Visibility != skills.VisibilityOff || menu.Toggle.Notice.Kind != skillsToggleNoticeNone {
		t.Fatalf("authoritative committed redraw row=%#v notice=%#v", row, menu.Toggle.Notice)
	}
	if notice := formatSkillsToggleNotice(i18n.LangEN, menu.Toggle.Notice); notice != "" {
		t.Fatalf("authoritative checkbox redraw should be the only success feedback, got notice %q", notice)
	}
	if got := backend.toggleCallCount(); got != 1 {
		t.Fatalf("Space called ToggleProjectVisibility %d times, want exactly once", got)
	}
}

func TestSkillsToggleAuthoritativeRejectedAndDegradedResultsRedraw(t *testing.T) {
	base := task30Skill("skill:project:alpha", "alpha", skills.SourceProject)
	tests := []struct {
		name       string
		outcome    skills.ProjectVisibilityToggleOutcome
		reason     skills.ProjectVisibilityToggleReason
		visibility skills.Visibility
		scope      skills.SkillScope
		wantNotice SkillsToggleNoticeKind
		wantText   string
	}{
		{"stale", skills.ProjectVisibilityToggleRejected, skills.ProjectVisibilityToggleReasonStaleRevision, skills.VisibilityManualOnly, skills.SkillScopeProject, skillsToggleNoticeStale, "press Space again"},
		{"session override", skills.ProjectVisibilityToggleRejected, skills.ProjectVisibilityToggleReasonSessionOverride, skills.VisibilityNameOnly, skills.SkillScopeSession, skillsToggleNoticeSessionOverride, "/skills reset skill:project:alpha --scope session"},
		{"persistence failure", skills.ProjectVisibilityToggleRejected, skills.ProjectVisibilityToggleReasonPersistenceFailed, skills.VisibilityAuto, skills.SkillScopeDefault, skillsToggleNoticePersistenceFailed, "unchanged"},
		{"live apply rolled back", skills.ProjectVisibilityToggleRejected, skills.ProjectVisibilityToggleReasonLiveApplyRolledBack, skills.VisibilityNameOnly, skills.SkillScopeProject, skillsToggleNoticeRolledBack, "rolled back"},
		{"rollback degraded", skills.ProjectVisibilityToggleDegraded, skills.ProjectVisibilityToggleReasonRollbackFailed, skills.VisibilityOff, skills.SkillScopeProject, skillsToggleNoticeRefreshed, "Space may be used again"},
		{"refresh degraded", skills.ProjectVisibilityToggleDegraded, skills.ProjectVisibilityToggleReasonAuthoritativeRefresh, skills.VisibilityManualOnly, skills.SkillScopeProject, skillsToggleNoticeRefreshed, "Space may be used again"},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			initial := task30Snapshot(t, 10, base)
			backend := newTask30Backend(t, initial)
			authoritativeRow := task30WithVisibility(base, test.visibility, test.scope)
			authoritative := task30Snapshot(t, skills.CatalogRevision(20+index), authoritativeRow)
			result := task30ToggleResult(t, base.ID, initial.Revision, authoritative, test.outcome, test.reason)
			backend.toggle = func(task30ToggleCall) (skills.ProjectVisibilityToggleResult, error) {
				return result, errors.New("transaction detail")
			}
			state, root := task30Root(backend)
			task30OpenSkills(t, state, root, backend)
			dispatchRootKeyForTest(t, root, gtui.KeyEvent{Key: gtui.KeyRune, Rune: ' '})
			task30WaitFor(t, func() bool {
				menu := state.SkillsMenu.Get()
				return menu != nil && !menu.Toggle.Loading && menu.Toggle.Notice.Kind == test.wantNotice
			})
			menu := state.SkillsMenu.Get()
			row, ok := menu.Toggle.Snapshot.Find(base.ID)
			if !ok || row.Visibility != test.visibility || row.VisibilitySource != test.scope || menu.Toggle.Snapshot.Revision != authoritative.Revision {
				t.Fatalf("rejected/degraded redraw did not use result snapshot: row=%#v snapshot=%#v", row, menu.Toggle.Snapshot)
			}
			if text := formatSkillsToggleNotice(i18n.LangEN, menu.Toggle.Notice); !strings.Contains(text, test.wantText) {
				t.Fatalf("notice=%q, want %q", text, test.wantText)
			}
			if backend.toggleCallCount() != 1 {
				t.Fatalf("toggle calls=%d, want one", backend.toggleCallCount())
			}
		})
	}
}

func TestSkillsToggleManagedReadOnlyIsInspectableAndNeverMutated(t *testing.T) {
	tests := []struct {
		reason skills.CatalogPolicyReason
		key    i18n.Key
	}{
		{skills.CatalogPolicyReasonManagedReadOnly, i18n.KeyCommandSkillsReadOnlyManaged},
		{skills.CatalogPolicyReasonManagedDeny, i18n.KeyCommandSkillsReadOnlyDenied},
	}
	for _, test := range tests {
		t.Run(string(test.reason), func(t *testing.T) {
			locked := task30Skill("skill:managed:locked", "locked", skills.SourceManaged)
			locked.Mutable = false
			locked.ReadOnlyReason = string(test.reason)
			if test.reason == skills.CatalogPolicyReasonManagedDeny {
				locked = task30WithVisibility(locked, skills.VisibilityOff, skills.SkillScopeManaged)
			}
			backend := newTask30Backend(t, task30Snapshot(t, 3, locked))
			state, root := task30Root(backend)
			task30OpenSkills(t, state, root, backend)

			for _, lang := range i18n.AllLanguages() {
				state.Language.Set(lang)
				rendered := collectElementText(renderSkillsMenuForTest(root, state.SkillsMenu.Get()))
				localized := i18n.Text(lang, test.key)
				if !strings.Contains(rendered, localized) || strings.Contains(rendered, string(test.reason)) {
					t.Fatalf("%s detail=%q, want localized reason %q without internal code", lang.Code(), rendered, localized)
				}
				if notice := formatSkillsToggleNotice(lang, SkillsToggleNotice{
					Kind: skillsToggleNoticeReadOnly, SkillID: locked.ID, Name: locked.Name,
					ReadOnlyReason: string(test.reason),
				}); !strings.Contains(notice, localized) || strings.Contains(notice, string(test.reason)) {
					t.Fatalf("%s notice=%q, want localized reason %q without internal code", lang.Code(), notice, localized)
				}
			}

			dispatchRootKeyForTest(t, root, gtui.KeyEvent{Key: gtui.KeyRune, Rune: ' '})
			menu := state.SkillsMenu.Get()
			if backend.toggleCallCount() != 0 || menu.Toggle.Notice.Kind != skillsToggleNoticeReadOnly {
				t.Fatalf("read-only Space calls=%d notice=%#v", backend.toggleCallCount(), menu.Toggle.Notice)
			}
		})
	}
}

func TestSkillsToggleDegradedRefreshGateNeverMutatesTwice(t *testing.T) {
	row := task30Skill("skill:project:alpha", "alpha", skills.SourceProject)
	initial := task30Snapshot(t, 1, row)
	backend := newTask30Backend(t, initial)
	degradedRow := task30WithVisibility(row, skills.VisibilityOff, skills.SkillScopeProject)
	degraded := task30ToggleResult(t, row.ID, 1, task30Snapshot(t, 2, degradedRow), skills.ProjectVisibilityToggleDegraded, skills.ProjectVisibilityToggleReasonAuthoritativeRefresh)
	backend.toggle = func(task30ToggleCall) (skills.ProjectVisibilityToggleResult, error) {
		return degraded, errors.New("rollback and refresh failed")
	}
	state, root := task30Root(backend)
	task30OpenSkills(t, state, root, backend)
	backend.setSnapshotBehavior(errors.New("authoritative read unavailable"), nil)

	dispatchRootKeyForTest(t, root, gtui.KeyEvent{Key: gtui.KeyRune, Rune: ' '})
	task30WaitFor(t, func() bool {
		menu := state.SkillsMenu.Get()
		return menu != nil && !menu.Toggle.Loading && menu.Toggle.RefreshRequired && menu.Toggle.Notice.Kind == skillsToggleNoticeRefreshFailed
	})
	if backend.toggleCallCount() != 1 || backend.snapshotCallCount() != 2 {
		t.Fatalf("after automatic refresh: toggles=%d snapshots=%d, want 1/2", backend.toggleCallCount(), backend.snapshotCallCount())
	}

	refreshed := task30Snapshot(t, 3, degradedRow)
	retryBlock := make(chan struct{})
	backend.setSnapshot(refreshed)
	backend.setSnapshotBehavior(nil, retryBlock)
	dispatchRootKeyForTest(t, root, gtui.KeyEvent{Key: gtui.KeyRune, Rune: ' '})
	task30WaitFor(t, func() bool { return backend.snapshotCallCount() == 3 })
	// A second Space while the read-only refresh is in flight must not issue a
	// mutation or another read.
	dispatchRootKeyForTest(t, root, gtui.KeyEvent{Key: gtui.KeyRune, Rune: ' '})
	if backend.toggleCallCount() != 1 || backend.snapshotCallCount() != 3 {
		t.Fatalf("refresh gate leaked work: toggles=%d snapshots=%d", backend.toggleCallCount(), backend.snapshotCallCount())
	}
	close(retryBlock)
	task30WaitFor(t, func() bool {
		menu := state.SkillsMenu.Get()
		return menu != nil && !menu.Toggle.Loading && !menu.Toggle.RefreshRequired &&
			menu.Toggle.Notice.Kind == skillsToggleNoticeRefreshed && menu.Toggle.Snapshot.Revision == 3
	})
	if backend.toggleCallCount() != 1 {
		t.Fatalf("refresh retry issued %d mutations, want one original mutation", backend.toggleCallCount())
	}
}

func TestSkillsToggleRestoresBackendPersistedLastNonOffAfterReopen(t *testing.T) {
	manual := task30WithVisibility(task30Skill("skill:project:manual", "manual", skills.SourceProject), skills.VisibilityManualOnly, skills.SkillScopeProject)
	backend := newTask30Backend(t, task30Snapshot(t, 1, manual))
	backend.toggle = func(call task30ToggleCall) (skills.ProjectVisibilityToggleResult, error) {
		backend.mu.Lock()
		current, _ := backend.snapshot.Find(manual.ID)
		revision := backend.snapshot.Revision + 1
		backend.mu.Unlock()
		nextVisibility := skills.VisibilityOff
		if current.Visibility == skills.VisibilityOff {
			// The fake models the persisted backend contract: UI state is not
			// consulted to recover this value.
			nextVisibility = skills.VisibilityManualOnly
		}
		next := task30WithVisibility(current, nextVisibility, skills.SkillScopeProject)
		snapshot := task30Snapshot(t, revision, next)
		result := task30ToggleResult(t, call.id, call.revision, snapshot, skills.ProjectVisibilityToggleCommitted, skills.ProjectVisibilityToggleReasonNone)
		backend.setSnapshot(snapshot)
		return result, nil
	}
	state, root := task30Root(backend)
	task30OpenSkills(t, state, root, backend)

	dispatchRootKeyForTest(t, root, gtui.KeyEvent{Key: gtui.KeyRune, Rune: ' '})
	task30WaitFor(t, func() bool {
		row, _ := state.SkillsMenu.Get().Toggle.Snapshot.Find(manual.ID)
		return row.Visibility == skills.VisibilityOff && !state.SkillsMenu.Get().Toggle.Loading
	})
	// Close the direct list, then reopen it so the row is reloaded from the
	// backend rather than retained as a view-local remembered boolean.
	dispatchRootKeyForTest(t, root, gtui.KeyEvent{Key: gtui.KeyEscape})
	task30OpenSkills(t, state, root, backend)
	task30WaitFor(t, func() bool {
		menu := state.SkillsMenu.Get()
		if menu == nil || !menu.Toggle.HasSnapshot || menu.Toggle.Loading {
			return false
		}
		row, _ := menu.Toggle.Snapshot.Find(manual.ID)
		return row.Visibility == skills.VisibilityOff
	})
	dispatchRootKeyForTest(t, root, gtui.KeyEvent{Key: gtui.KeyRune, Rune: ' '})
	task30WaitFor(t, func() bool {
		menu := state.SkillsMenu.Get()
		row, _ := menu.Toggle.Snapshot.Find(manual.ID)
		return row.Visibility == skills.VisibilityManualOnly && !menu.Toggle.Loading
	})
	if backend.toggleCallCount() != 2 {
		t.Fatalf("toggle calls=%d, want two backend-owned transitions", backend.toggleCallCount())
	}
}

func TestSkillsToggleLoadingEmptyNarrowAndRuntimeLanguage(t *testing.T) {
	backend := newTask30Backend(t, task30Snapshot(t, 1))
	block := make(chan struct{})
	backend.snapshotBlock = block
	state, root := task30Root(backend)
	state.Language.Set(i18n.LangZH)
	menu := newSkillsMenuState(SkillsMenuOpenRequest{
		SessionID: func() string { return state.SessionID.Get() },
		Language:  func() i18n.Language { return state.Language.Get() },
		Backend:   backend,
	})
	state.SkillsMenu.Set(menu)
	root.openSkillsMenu(menu)
	task30WaitFor(t, func() bool { return backend.snapshotCallCount() == 1 })
	loading := collectElementText(renderSkillsMenuForTest(root, state.SkillsMenu.Get()))
	if !strings.Contains(loading, "技能") || !strings.Contains(loading, "正在加载") || strings.Contains(loading, "列出技能") {
		t.Fatalf("loading state not localized: %q", loading)
	}
	close(block)
	task30WaitFor(t, func() bool {
		menu := state.SkillsMenu.Get()
		return menu != nil && menu.Toggle.HasSnapshot && !menu.Toggle.Loading
	})
	root.termWidth, root.termHeight = 18, 8
	empty := collectElementText(renderSkillsMenuForTest(root, state.SkillsMenu.Get()))
	if !strings.Contains(empty, "没有可用技能") {
		t.Fatalf("empty narrow state=%q", empty)
	}
}

func TestSkillsToggleIgnoresLateResultAfterClose(t *testing.T) {
	row := task30Skill("skill:project:alpha", "alpha", skills.SourceProject)
	initial := task30Snapshot(t, 1, row)
	backend := newTask30Backend(t, initial)
	started := make(chan task30ToggleCall, 1)
	release := make(chan struct{})
	off := task30WithVisibility(row, skills.VisibilityOff, skills.SkillScopeProject)
	result := task30ToggleResult(t, row.ID, 1, task30Snapshot(t, 2, off), skills.ProjectVisibilityToggleCommitted, skills.ProjectVisibilityToggleReasonNone)
	backend.toggle = func(call task30ToggleCall) (skills.ProjectVisibilityToggleResult, error) {
		started <- call
		<-release
		return result, nil
	}
	state, root := task30Root(backend)
	task30OpenSkills(t, state, root, backend)
	dispatchRootKeyForTest(t, root, gtui.KeyEvent{Key: gtui.KeyRune, Rune: ' '})
	_ = task30ReceiveToggle(t, started)
	dispatchRootKeyForTest(t, root, gtui.KeyEvent{Key: gtui.KeyEscape})
	close(release)
	time.Sleep(20 * time.Millisecond)
	if menu := state.SkillsMenu.Get(); menu != nil {
		t.Fatalf("late toggle reopened closed direct menu: %#v", menu)
	}
}

type task30ToggleCall struct {
	sessionID string
	id        skills.SkillID
	revision  skills.CatalogRevision
}

type task30Backend struct {
	t             *testing.T
	mu            sync.Mutex
	snapshot      skills.CatalogSnapshot
	snapshotErr   error
	snapshotBlock <-chan struct{}
	snapshotCalls int
	toggleCalls   []task30ToggleCall
	toggle        func(task30ToggleCall) (skills.ProjectVisibilityToggleResult, error)
}

func newTask30Backend(t *testing.T, snapshot skills.CatalogSnapshot) *task30Backend {
	t.Helper()
	return &task30Backend{t: t, snapshot: snapshot}
}

func (backend *task30Backend) Snapshot(string) (skills.CatalogSnapshot, error) {
	backend.mu.Lock()
	backend.snapshotCalls++
	block := backend.snapshotBlock
	snapshot, err := backend.snapshot.Clone(), backend.snapshotErr
	backend.mu.Unlock()
	if block != nil {
		<-block
	}
	return snapshot, err
}

func (backend *task30Backend) ToggleProjectVisibility(sessionID string, id skills.SkillID, revision skills.CatalogRevision) (skills.ProjectVisibilityToggleResult, error) {
	call := task30ToggleCall{sessionID: sessionID, id: id, revision: revision}
	backend.mu.Lock()
	backend.toggleCalls = append(backend.toggleCalls, call)
	toggle := backend.toggle
	backend.mu.Unlock()
	if toggle == nil {
		return skills.ProjectVisibilityToggleResult{}, errors.New("toggle fixture is not configured")
	}
	result, err := toggle(call)
	if result.Validate() == nil {
		backend.setSnapshot(result.Snapshot)
	}
	return result, err
}

func (backend *task30Backend) setSnapshot(snapshot skills.CatalogSnapshot) {
	backend.mu.Lock()
	backend.snapshot = snapshot.Clone()
	backend.mu.Unlock()
}

func (backend *task30Backend) setSnapshotBehavior(err error, block <-chan struct{}) {
	backend.mu.Lock()
	backend.snapshotErr = err
	backend.snapshotBlock = block
	backend.mu.Unlock()
}

func (backend *task30Backend) snapshotCallCount() int {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	return backend.snapshotCalls
}

func (backend *task30Backend) toggleCallCount() int {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	return len(backend.toggleCalls)
}

func task30Root(backend SkillsManagementBackend) (*AppState, *RootComponent) {
	state := NewAppState()
	state.SessionID.Set("session-a")
	state.Language.Set(i18n.LangEN)
	return state, NewRootComponent(state, nil, nil)
}

func task30OpenSkills(t *testing.T, state *AppState, root *RootComponent, backend SkillsManagementBackend) {
	t.Helper()
	menu := newSkillsMenuState(SkillsMenuOpenRequest{
		SessionID: func() string { return state.SessionID.Get() },
		Language:  func() i18n.Language { return state.Language.Get() },
		Backend:   backend,
	})
	state.SkillsMenu.Set(menu)
	root.openSkillsMenu(menu)
	task30WaitFor(t, func() bool {
		menu := state.SkillsMenu.Get()
		return menu != nil && menu.Toggle.HasSnapshot && !menu.Toggle.Loading
	})
}

func task30Snapshot(t *testing.T, revision skills.CatalogRevision, rows ...skills.EffectiveSkill) skills.CatalogSnapshot {
	t.Helper()
	snapshot, err := skills.NewCatalogSnapshot(revision, rows)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func task30Skill(id skills.SkillID, name string, source skills.SkillSource) skills.EffectiveSkill {
	return skills.EffectiveSkill{
		ID: id, Name: name, Summary: "Summary for " + name, Source: source,
		Locator:  skills.SkillLocator("/skills/" + string(source) + "/" + name + "/SKILL.md"),
		Digest:   skills.SkillDigest("sha256:" + strings.Repeat("a", 64)),
		Revision: 1, Visibility: skills.VisibilityAuto, VisibilitySource: skills.SkillScopeDefault,
		ModelVisible: true, DescriptionVisible: true, UserInvocable: true, Executable: true, Mutable: true,
	}
}

func task30WithVisibility(row skills.EffectiveSkill, visibility skills.Visibility, scope skills.SkillScope) skills.EffectiveSkill {
	row.Visibility, row.VisibilitySource = visibility, scope
	switch visibility {
	case skills.VisibilityOff:
		row.ModelVisible, row.DescriptionVisible, row.UserInvocable, row.Executable = false, false, false, false
	case skills.VisibilityManualOnly:
		row.ModelVisible, row.DescriptionVisible = false, false
		row.UserInvocable, row.Executable = true, true
	case skills.VisibilityNameOnly:
		row.ModelVisible, row.DescriptionVisible = true, false
		row.UserInvocable, row.Executable = true, true
	default:
		row.ModelVisible, row.DescriptionVisible, row.UserInvocable, row.Executable = true, true, true, true
	}
	return row
}

func task30ToggleResult(t *testing.T, id skills.SkillID, observed skills.CatalogRevision, snapshot skills.CatalogSnapshot, outcome skills.ProjectVisibilityToggleOutcome, reason skills.ProjectVisibilityToggleReason) skills.ProjectVisibilityToggleResult {
	t.Helper()
	result := skills.ProjectVisibilityToggleResult{
		Outcome: outcome, Reason: reason, RequestedSkillID: id,
		ObservedRevision: observed, CurrentRevision: snapshot.Revision,
		Snapshot: snapshot.Clone(),
	}
	if row, ok := snapshot.Find(id); ok {
		result.Skill = &row
	}
	if err := result.Validate(); err != nil {
		t.Fatalf("invalid toggle fixture: %v", err)
	}
	return result
}

func task30WaitFor(t *testing.T, ready func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if ready() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("timed out waiting for skills menu state")
}

func task30ReceiveToggle(t *testing.T, calls <-chan task30ToggleCall) task30ToggleCall {
	t.Helper()
	select {
	case call := <-calls:
		return call
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for toggle call")
		return task30ToggleCall{}
	}
}
