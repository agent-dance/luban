package tui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/skills"
	gtui "github.com/grindlemire/go-tui"
	"github.com/rivo/uniseg"
)

func TestSkillsSurfaceAcceptanceRealManagerDirectChecklistShadowedOffOn(t *testing.T) {
	fixture := newTask28TUIFixture(t, true)
	snapshot := task28TUISnapshot(t, fixture.manager, "session-a")
	rows := task28TUIRowsByName(snapshot)
	for name, visibility := range map[string]skills.Visibility{
		"name-only":   skills.VisibilityNameOnly,
		"manual-only": skills.VisibilityManualOnly,
		"off":         skills.VisibilityOff,
	} {
		row := rows[name][0]
		if _, err := fixture.manager.SetVisibility("session-a", skills.VisibilityOverride{
			SkillID: row.ID, Scope: skills.SkillScopeProject, Visibility: visibility,
		}); err != nil {
			t.Fatal(err)
		}
	}

	state, root := task28TUIRoot()
	task28TUIOpen(t, state, root, fixture.manager)
	rendered := task28TUIText(root.renderSkillsMenu(state.SkillsMenu.Get()))
	for _, want := range []string{"[x] auto", "[x] name-only", "[x] manual-only", "[ ] off"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("direct checklist omitted %q: %q", want, rendered)
		}
	}
	for _, obsolete := range []string{"List Skills", "Enable/Disable Skills"} {
		if strings.Contains(rendered, obsolete) {
			t.Errorf("direct checklist retained intermediate action %q: %q", obsolete, rendered)
		}
	}
	if stopped := task28TUIDispatch(root, gtui.KeyEvent{Key: gtui.KeyEnter}); !stopped {
		t.Fatal("Enter leaked from direct checklist")
	}

	// The only user-source skill is the shadowed same-name row, so filtering by
	// source proves Space targets stable ID rather than display name.
	for _, char := range "user" {
		task28TUIDispatch(root, gtui.KeyEvent{Key: gtui.KeyRune, Rune: char})
	}
	menu := state.SkillsMenu.Get()
	selected := menu.Toggle.selectedRow()
	if len(menu.Toggle.Filtered) != 1 || selected == nil || selected.Source != skills.SourceUser || selected.ShadowedBy == "" {
		t.Fatalf("shadowed filtered selection=%#v row=%#v", menu.Toggle.Filtered, selected)
	}
	shadowID, winnerID := selected.ID, selected.ShadowedBy
	rendered = task28TUIText(root.renderSkillsMenu(menu))
	if !strings.Contains(rendered, "[x] review") || !strings.Contains(rendered, "Configuration: enabled, but currently inactive") {
		t.Fatalf("shadowed configured-on rendering=%q", rendered)
	}

	task28TUIDispatch(root, gtui.KeyEvent{Key: gtui.KeyRune, Rune: ' '})
	task28TUIWait(t, func() bool {
		current := state.SkillsMenu.Get()
		row, found := current.Toggle.Snapshot.Find(shadowID)
		return found && !current.Toggle.Loading && row.Visibility == skills.VisibilityOff
	})
	current := state.SkillsMenu.Get()
	if current.Toggle.selectedRow() == nil || current.Toggle.selectedRow().ID != shadowID {
		t.Fatalf("off redraw lost stable selection=%#v", current.Toggle.selectedRow())
	}
	task28TUIDispatch(root, gtui.KeyEvent{Key: gtui.KeyRune, Rune: ' '})
	task28TUIWait(t, func() bool {
		current := state.SkillsMenu.Get()
		row, found := current.Toggle.Snapshot.Find(shadowID)
		return found && !current.Toggle.Loading && row.Visibility == skills.VisibilityAuto && row.ShadowedBy == winnerID
	})
	rendered = task28TUIText(root.renderSkillsMenu(state.SkillsMenu.Get()))
	if !strings.Contains(rendered, "[x] review") || !strings.Contains(rendered, "Configuration: enabled, but currently inactive") {
		t.Fatalf("shadowed off/on lost configured/activity distinction=%q", rendered)
	}

	for range "user" {
		task28TUIDispatch(root, gtui.KeyEvent{Key: gtui.KeyBackspace})
	}
	if state.SkillsMenu.Get().Toggle.Query != "" {
		t.Fatalf("Backspace filter=%q", state.SkillsMenu.Get().Toggle.Query)
	}
	task28TUIDispatch(root, gtui.KeyEvent{Key: gtui.KeyEscape})
	if state.SkillsMenu.Get() != nil {
		t.Fatal("one Esc did not close direct checklist")
	}

	// Removing the higher-priority winner and refreshing the real manager must
	// promote the configured-on user row. Reopening performs the same
	// authoritative Snapshot read as a fresh /skills launch; no old view state
	// is allowed to preserve the shadow notice.
	if err := os.RemoveAll(filepath.Dir(fixture.pathsByName["review"])); err != nil {
		t.Fatal(err)
	}
	refreshed, err := fixture.manager.RefreshSnapshot("session-a")
	if err != nil {
		t.Fatal(err)
	}
	if _, found := refreshed.Find(winnerID); found {
		t.Fatalf("deleted winner remained in refreshed catalog: %#v", refreshed)
	}
	task28TUIOpen(t, state, root, fixture.manager)
	for _, char := range "user" {
		task28TUIDispatch(root, gtui.KeyEvent{Key: gtui.KeyRune, Rune: char})
	}
	promoted := state.SkillsMenu.Get().Toggle.selectedRow()
	if promoted == nil || promoted.ID != shadowID || promoted.ShadowedBy != "" ||
		promoted.Visibility != skills.VisibilityAuto || !promoted.ModelVisible ||
		!promoted.UserInvocable || !promoted.Executable {
		t.Fatalf("winner removal did not activate configured-on row: %#v", promoted)
	}
	rendered = task28TUIText(root.renderSkillsMenu(state.SkillsMenu.Get()))
	if strings.Contains(rendered, "currently inactive") || strings.Contains(rendered, string(winnerID)) {
		t.Fatalf("promoted row retained stale shadow detail: %q", rendered)
	}
	for range "user" {
		task28TUIDispatch(root, gtui.KeyEvent{Key: gtui.KeyBackspace})
	}

	// Toggle once more after promotion. The prior off/on restoration is stored
	// in the backend; this action is not reading any earlier view object.
	next := state.SkillsMenu.Get().clone()
	for visible, rowIndex := range next.Toggle.Filtered {
		if next.Toggle.Snapshot.Skills[rowIndex].ID == shadowID {
			next.Toggle.Selected = visible
			break
		}
	}
	state.SkillsMenu.Set(next)
	task28TUIDispatch(root, gtui.KeyEvent{Key: gtui.KeyRune, Rune: ' '})
	task28TUIWait(t, func() bool {
		current := state.SkillsMenu.Get()
		row, found := current.Toggle.Snapshot.Find(shadowID)
		return found && !current.Toggle.Loading && row.Visibility == skills.VisibilityOff
	})
}

func TestSkillsSurfaceAcceptanceRealConcurrentRefreshMakesSpaceStale(t *testing.T) {
	fixture := newTask28TUIFixture(t, false)
	initial := task28TUISnapshot(t, fixture.manager, "session-a")
	target := initial.Skills[0]
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	backend := &task28TUIBlockingToggleBackend{manager: fixture.manager, started: started, release: release}
	state, root := task28TUIRoot()
	task28TUIOpen(t, state, root, backend)

	task28TUIDispatch(root, gtui.KeyEvent{Key: gtui.KeyRune, Rune: ' '})
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("Space did not reach real backend")
	}
	path := fixture.pathsByName[target.Name]
	if err := os.WriteFile(path, []byte("---\nname: "+target.Name+"\ndescription: refreshed\n---\nupdated body\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	refreshed, err := fixture.manager.RefreshSnapshot("session-a")
	if err != nil || refreshed.Revision == initial.Revision {
		t.Fatalf("real concurrent refresh=%#v err=%v", refreshed, err)
	}
	close(release)
	task28TUIWait(t, func() bool {
		menu := state.SkillsMenu.Get()
		return menu != nil && !menu.Toggle.Loading && menu.Toggle.Notice.Kind == skillsToggleNoticeStale
	})
	call := backend.lastCall()
	if call.id != target.ID || call.revision != initial.Revision {
		t.Fatalf("stale Space call=%#v", call)
	}
	menu := state.SkillsMenu.Get()
	if menu.Toggle.Snapshot.Revision != refreshed.Revision || menu.Toggle.selectedRow() == nil || menu.Toggle.selectedRow().ID != target.ID {
		t.Fatalf("stale authoritative redraw snapshot=%#v selected=%#v", menu.Toggle.Snapshot, menu.Toggle.selectedRow())
	}
	rendered := task28TUIText(root.renderSkillsMenu(menu))
	semanticRendered := strings.Join(strings.Fields(rendered), " ")
	for _, want := range []string{
		"was stale", "refreshed to catalog revision", "press Space again",
	} {
		if !strings.Contains(semanticRendered, want) {
			t.Errorf("rendered stale guidance omitted %q: %q", want, rendered)
		}
	}
	visible := task28TUIRenderBuffer(root, menu)
	for _, want := range []string{"was stale", "press Space again"} {
		if !strings.Contains(visible, want) {
			t.Errorf("visible stale guidance omitted %q (hidden nodes do not count):\n%s", want, visible)
		}
	}
}

func TestSkillsSurfaceAcceptanceActiveSessionOverrideBlocksProjectSpaceWithClearGuidance(t *testing.T) {
	fixture := newTask28TUIFixture(t, false)
	initial := task28TUISnapshot(t, fixture.manager, "session-a")
	target := initial.Skills[0]
	withSession, err := fixture.manager.SetVisibility("session-a", skills.VisibilityOverride{
		SkillID: target.ID, Scope: skills.SkillScopeSession, Visibility: skills.VisibilityManualOnly,
	})
	if err != nil {
		t.Fatal(err)
	}
	row, found := withSession.Find(target.ID)
	if !found || row.VisibilitySource != skills.SkillScopeSession {
		t.Fatalf("active session override precondition=%#v found=%t", row, found)
	}

	state, root := task28TUIRoot()
	task28TUIOpen(t, state, root, fixture.manager)
	task28TUIDispatch(root, gtui.KeyEvent{Key: gtui.KeyRune, Rune: ' '})
	task28TUIWait(t, func() bool {
		menu := state.SkillsMenu.Get()
		return menu != nil && !menu.Toggle.Loading && menu.Toggle.Notice.Kind == skillsToggleNoticeSessionOverride
	})

	authority, err := fixture.store.Snapshot("session-a")
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := authority.Project[target.ID]; exists {
		t.Fatalf("session-override rejection wrote project state: %#v", authority.Project)
	}
	menu := state.SkillsMenu.Get()
	current, found := menu.Toggle.Snapshot.Find(target.ID)
	if !found || current.Visibility != skills.VisibilityManualOnly || current.VisibilitySource != skills.SkillScopeSession {
		t.Fatalf("session rejection did not redraw authoritative row: %#v found=%t", current, found)
	}
	rendered := task28TUIText(root.renderSkillsMenu(menu))
	wantClear := "/skills reset " + string(target.ID) + " --scope session"
	for _, want := range []string{"active session override", wantClear, "then use Space"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("session-override guidance omitted %q: %q", want, rendered)
		}
	}
	visible := task28TUIRenderBuffer(root, menu)
	for _, want := range []string{wantClear, "then use Space"} {
		if !strings.Contains(visible, want) {
			t.Errorf("visible session-override recovery omitted %q (hidden nodes do not count):\n%s", want, visible)
		}
	}
}

func TestSkillsSurfaceAcceptanceRootRendersRealTransactionalRollbackTruth(t *testing.T) {
	tests := []struct {
		name       string
		fault      *task28TUITransactionFault
		wantNotice SkillsToggleNoticeKind
		wantText   []string
	}{
		{
			name: "persistence failure", fault: &task28TUITransactionFault{compareErr: errors.New("task28 injected persistence failure")},
			wantNotice: skillsToggleNoticePersistenceFailed,
			wantText:   []string{"could not be saved", "authoritative state is unchanged"},
		},
		{
			name: "live apply failure with successful rollback", fault: &task28TUITransactionFault{failSnapshotsAfterCAS: 1},
			wantNotice: skillsToggleNoticeRolledBack,
			wantText:   []string{"live update", "rolled back"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manager, base, target := task28TUIFaultManager(t, test.fault)
			state, root := task28TUIRoot()
			task28TUIOpen(t, state, root, manager)
			task28TUIDispatch(root, gtui.KeyEvent{Key: gtui.KeyRune, Rune: ' '})
			task28TUIWait(t, func() bool {
				menu := state.SkillsMenu.Get()
				return menu != nil && !menu.Toggle.Loading && menu.Toggle.Notice.Kind == test.wantNotice
			})

			persisted, err := base.Snapshot("session-a")
			if err != nil {
				t.Fatal(err)
			}
			if _, exists := persisted.Project[target.ID]; exists {
				t.Fatalf("failed transaction left project override: %#v", persisted.Project)
			}
			menu := state.SkillsMenu.Get()
			row, found := menu.Toggle.Snapshot.Find(target.ID)
			if !found || row.Visibility != skills.VisibilityAuto || row.VisibilitySource != skills.SkillScopeDefault {
				t.Fatalf("failed transaction view is not authoritative: %#v found=%t", row, found)
			}
			rendered := task28TUIText(root.renderSkillsMenu(menu))
			for _, want := range test.wantText {
				if !strings.Contains(rendered, want) {
					t.Errorf("transaction notice omitted %q: %q", want, rendered)
				}
			}
			visible := task28TUIRenderBuffer(root, menu)
			for _, want := range test.wantText {
				if !strings.Contains(visible, want) {
					t.Errorf("visible transaction recovery omitted %q (hidden nodes do not count):\n%s", want, visible)
				}
			}
		})
	}
}

func TestSkillsSurfaceAcceptanceRootDegradedToggleUsesAuthoritativeRefreshGate(t *testing.T) {
	fixture := newTask28TUIFixture(t, false)
	refreshStarted := make(chan struct{}, 1)
	releaseRefresh := make(chan struct{})
	backend := &task28TUIDegradedBackend{manager: fixture.manager, refreshStarted: refreshStarted, releaseRefresh: releaseRefresh}
	state, root := task28TUIRoot()
	task28TUIOpen(t, state, root, backend)

	task28TUIDispatch(root, gtui.KeyEvent{Key: gtui.KeyRune, Rune: ' '})
	select {
	case <-refreshStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("degraded toggle did not enter automatic authoritative refresh")
	}
	// While refresh is blocked, Space is a read gate and cannot mutate again.
	task28TUIDispatch(root, gtui.KeyEvent{Key: gtui.KeyRune, Rune: ' '})
	if got := backend.toggleCount(); got != 1 {
		t.Fatalf("degraded refresh gate allowed %d toggles", got)
	}
	close(releaseRefresh)
	task28TUIWait(t, func() bool {
		menu := state.SkillsMenu.Get()
		return menu != nil && !menu.Toggle.Loading && !menu.Toggle.RefreshRequired && menu.Toggle.Notice.Kind == skillsToggleNoticeRefreshed
	})
	if backend.snapshotCount() != 2 {
		t.Fatalf("snapshot calls=%d, want initial plus authoritative refresh", backend.snapshotCount())
	}
	row := state.SkillsMenu.Get().Toggle.selectedRow()
	if row == nil || row.Visibility != skills.VisibilityOff {
		t.Fatalf("refresh gate did not redraw real manager authority=%#v", row)
	}
}

func TestSkillsSurfaceAcceptanceLayoutBufferMatrixKeepsRowsDetailsAndBorderInViewport(t *testing.T) {
	const (
		selectedID   = skills.SkillID("skill:project:layout-selected")
		selectedPath = "/SELECTED_PATH_MARKER/SKILL.md"
		newlinePath  = "/NEWLINE_PATH_MARKER/SKILL.md"
	)
	rows := []skills.EffectiveSkill{
		task28TUILayoutSkill(
			"skill:project:layout-combining",
			"layout-cafe\u0301-"+strings.Repeat("组合", 24),
			strings.Repeat("combining-cafe\u0301-", 24),
			"/UNSELECTED_COMBINING_PATH_MARKER/SKILL.md",
		),
		task28TUILayoutSkill(
			"skill:project:layout-newline",
			"layout-newline\nNEWLINE_MARKER\x1b[31mESCAPE_MARKER\x1b[0m",
			strings.Repeat("unselected-summary-", 32),
			newlinePath,
		),
		task28TUILayoutSkill(
			"skill:project:layout-unselected",
			"layout-unselected-"+strings.Repeat("技能👩🏽\u200d💻✈️", 18),
			strings.Repeat("UNSELECTED_SUMMARY_MARKER-", 20),
			"/UNSELECTED_PATH_MARKER/SKILL.md",
		),
		task28TUILayoutSkill(
			selectedID,
			"layout-selected-"+strings.Repeat("技能👩🏽\u200d💻✈️", 18),
			strings.Repeat("SELECTED_SUMMARY_MARKER-", 20),
			selectedPath,
		),
	}
	snapshot, err := skills.NewCatalogSnapshot(73, rows)
	if err != nil {
		t.Fatal(err)
	}

	state, root := task28TUIRoot()
	menu := newSkillsMenuState(SkillsMenuOpenRequest{
		SessionID: func() string { return state.SessionID.Get() },
		Language:  func() i18n.Language { return state.Language.Get() },
	})
	menu.Toggle.Query = "layout"
	menu.Toggle.installSnapshot(snapshot, selectedID)
	menu.Toggle.Notice = SkillsToggleNotice{
		Kind:     skillsToggleNoticeStale,
		SkillID:  selectedID,
		Name:     strings.Repeat("very-long-notice-name-", 16),
		Revision: snapshot.Revision,
	}
	state.SkillsMenu.Set(menu)

	sizes := []struct{ width, height int }{
		{80, 24},
		{24, 10},
		{12, 6},
		{8, 2},
		// Re-grow after the two constrained renders to prove that a render-time
		// layout calculation neither rewrites nor strands the stable selection.
		{80, 24},
	}
	var wideText string
	for index, size := range sizes {
		root.termWidth, root.termHeight = size.width, size.height
		panel := root.renderSkillsMenu(state.SkillsMenu.Get())
		buffer := gtui.NewBuffer(size.width, size.height)
		panel.Render(buffer, size.width, size.height)

		rect := panel.Rect()
		if rect.X < 0 || rect.Y < 0 || rect.Right() > size.width || rect.Bottom() > size.height {
			t.Fatalf("%dx%d skills panel escaped viewport: %+v\n%s", size.width, size.height, rect, buffer.String())
		}
		if size.height >= 2 {
			task28TUIAssertCompleteBottomBorder(t, buffer, rect, size.width, size.height)
		}
		for rowIndex, line := range strings.Split(buffer.String(), "\n") {
			checkboxes := strings.Count(line, "[x]") + strings.Count(line, "[ ]")
			if checkboxes > 1 {
				t.Fatalf("%dx%d row %d wrapped/coalesced multiple skill rows: %q", size.width, size.height, rowIndex, line)
			}
		}
		if size.height >= 3 && size.width >= 12 && !task28TUIBufferHasSelectedRow(buffer) {
			t.Fatalf("%dx%d buffer hid selected row after resize:\n%s", size.width, size.height, buffer.String())
		}
		selected := state.SkillsMenu.Get().Toggle.selectedRow()
		if selected == nil || selected.ID != selectedID {
			t.Fatalf("%dx%d render mutated stable selection: %#v", size.width, size.height, selected)
		}
		if index == 0 || index == len(sizes)-1 {
			if index == 0 {
				wideText = buffer.String()
			} else if buffer.String() != wideText {
				t.Fatalf("shrink/grow did not reproduce stable wide skills frame\n--- first ---\n%s\n--- regrown ---\n%s", wideText, buffer.String())
			}
		}
	}

	if strings.ContainsRune(wideText, '\x1b') || strings.Contains(wideText, "[31m") {
		t.Fatalf("skills buffer retained terminal control input: %q", wideText)
	}
	if !strings.Contains(wideText, selectedPath) {
		t.Fatalf("selected skill details were not rendered in the wide buffer:\n%s", wideText)
	}
	for _, unselectedDetail := range []string{
		newlinePath,
		"/UNSELECTED_PATH_MARKER/SKILL.md",
		"/UNSELECTED_COMBINING_PATH_MARKER/SKILL.md",
	} {
		if strings.Contains(wideText, unselectedDetail) {
			t.Fatalf("unselected skill leaked detail %q into buffer:\n%s", unselectedDetail, wideText)
		}
	}
	newlineMarkerLine := ""
	checkboxRows := 0
	for _, line := range strings.Split(wideText, "\n") {
		if strings.Contains(line, "[x]") || strings.Contains(line, "[ ]") {
			checkboxRows++
		}
		if strings.Contains(line, "NEWLINE_MARKER") {
			newlineMarkerLine = line
		}
	}
	// Details/status/help are intentionally allowed to consume rows before the
	// whole catalog fits. The acceptance property is that every row which is
	// actually visible consumes one physical buffer row; it is not that a
	// bounded panel must display the entire catalog at once.
	if checkboxRows < 2 || checkboxRows > len(rows) {
		t.Fatalf("wide buffer rendered an invalid number of physical checkbox rows: got %d catalog=%d\n%s", checkboxRows, len(rows), wideText)
	}
	if newlineMarkerLine == "" || !strings.Contains(newlineMarkerLine, "[x]") || !strings.Contains(newlineMarkerLine, "ESCAPE_MARKER") {
		t.Fatalf("embedded newline/escape metadata escaped its one-line row: %q\n%s", newlineMarkerLine, wideText)
	}

	// A production filter transition must discard the prior selected detail
	// block, not merely move the highlight while leaving stale metadata below.
	filtered := state.SkillsMenu.Get().clone()
	filtered.Toggle.Query = "NEWLINE_MARKER"
	filtered.Toggle.applyFilter("")
	state.SkillsMenu.Set(filtered)
	root.termWidth, root.termHeight = 80, 24
	panel := root.renderSkillsMenu(filtered)
	buffer := gtui.NewBuffer(80, 24)
	panel.Render(buffer, 80, 24)
	filteredText := buffer.String()
	if !strings.Contains(filteredText, newlinePath) || strings.Contains(filteredText, selectedPath) {
		t.Fatalf("filter transition retained stale selected details:\n%s", filteredText)
	}
}

func TestSkillsSurfaceAcceptanceLayoutExactGraphemeWidthDoesNotEllipsize(t *testing.T) {
	tests := []struct {
		name         string
		id           skills.SkillID
		text         string
		wantSemantic string
		atomic       bool
	}{
		{name: "emoji ZWJ", id: "skill:project:exact-emoji", text: "👩🏽‍💻", atomic: true},
		{name: "variation selector", id: "skill:project:exact-variation", text: "✈️", atomic: true},
		{name: "combining NFC", id: "skill:project:exact-combining", text: "e\u0301", wantSemantic: "é"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot, err := skills.NewCatalogSnapshot(1, []skills.EffectiveSkill{
				task28TUILayoutSkill(test.id, test.text, "", "/exact/SKILL.md"),
			})
			if err != nil {
				t.Fatal(err)
			}
			state, root := task28TUIRoot()
			menu := newSkillsMenuState(SkillsMenuOpenRequest{})
			menu.Toggle.installSnapshot(snapshot, test.id)
			state.SkillsMenu.Set(menu)

			prefix := "→ [x] "
			wantWidth := uniseg.StringWidth(prefix + test.text)
			// Four cells are consumed by the two borders and horizontal padding.
			// The remaining width exactly fits the visible grapheme contract.
			width := wantWidth + skillsPanelHorizontalChrome
			root.termWidth, root.termHeight = width, 5
			panel := root.renderSkillsMenu(menu)
			rowText := ""
			for _, child := range panel.Children() {
				if strings.Contains(child.Text(), "[x]") {
					rowText = child.Text()
					break
				}
			}
			if strings.Contains(rowText, "…") {
				t.Fatalf("exact terminal-cell width used ellipsis instead of an atomic grapheme representation: cells=%d got=%q", wantWidth, rowText)
			}
			if gotWidth := uniseg.StringWidth(rowText); gotWidth > wantWidth {
				t.Fatalf("atomic grapheme representation overflowed: width=%d limit=%d row=%q", gotWidth, wantWidth, rowText)
			}
			suffix := strings.TrimPrefix(rowText, prefix)
			if rowText == suffix || suffix == "" {
				t.Fatalf("selected row lost its checkbox prefix or atomic grapheme: %q", rowText)
			}
			if test.wantSemantic != "" {
				if suffix != test.wantSemantic {
					t.Fatalf("NFC-representable grapheme changed semantics: got=%q want=%q", suffix, test.wantSemantic)
				}
			} else if test.atomic {
				if suffix == test.text {
					t.Fatalf("renderer-incompatible grapheme was emitted verbatim instead of an atomic safe representation: %q", suffix)
				}
				for _, originalRune := range test.text {
					if strings.ContainsRune(suffix, originalRune) {
						t.Fatalf("atomic replacement leaked a partial original grapheme rune %q: original=%q replacement=%q", originalRune, test.text, suffix)
					}
				}
			}
			buffer := gtui.NewBuffer(width, 5)
			panel.Render(buffer, width, 5)
			if rendered := buffer.String(); strings.ContainsRune(rendered, '\x1b') || !strings.Contains(rendered, rowText) {
				t.Fatalf("atomic row was not represented safely in the real buffer: row=%q\n%s", rowText, rendered)
			}
			task28TUIAssertCompleteBottomBorder(t, buffer, panel.Rect(), width, 5)
		})
	}
}

func task28TUILayoutSkill(id skills.SkillID, name, summary, locator string) skills.EffectiveSkill {
	return skills.EffectiveSkill{
		ID: id, Name: name, Summary: summary,
		Source: skills.SourceProject, Locator: skills.SkillLocator(locator),
		Digest: skills.SkillDigest("sha256:" + strings.Repeat("a", 64)), Revision: 1,
		Visibility: skills.VisibilityAuto, VisibilitySource: skills.SkillScopeDefault,
		ModelVisible: true, DescriptionVisible: true, UserInvocable: true, Executable: true,
		Mutable: true,
	}
}

func task28TUIAssertCompleteBottomBorder(t *testing.T, buffer *gtui.Buffer, rect gtui.Rect, width, height int) {
	t.Helper()
	if rect.Height < 2 || rect.Width < 2 {
		t.Fatalf("%dx%d panel cannot contain both rounded borders: %+v", width, height, rect)
	}
	bottom := rect.Bottom() - 1
	if bottom < 0 || bottom >= height {
		t.Fatalf("%dx%d bottom border is outside viewport: %+v", width, height, rect)
	}
	if got := buffer.Cell(rect.X, bottom).Rune; got != '╰' {
		t.Fatalf("%dx%d missing bottom-left corner at %+v: %q\n%s", width, height, rect, got, buffer.String())
	}
	if got := buffer.Cell(rect.Right()-1, bottom).Rune; got != '╯' {
		t.Fatalf("%dx%d missing bottom-right corner at %+v: %q\n%s", width, height, rect, got, buffer.String())
	}
	for x := rect.X + 1; x < rect.Right()-1; x++ {
		if got := buffer.Cell(x, bottom).Rune; got != '─' {
			t.Fatalf("%dx%d incomplete bottom border at x=%d y=%d: %q\n%s", width, height, x, bottom, got, buffer.String())
		}
	}
}

func task28TUIBufferHasSelectedRow(buffer *gtui.Buffer) bool {
	for _, line := range strings.Split(buffer.String(), "\n") {
		if strings.Contains(line, "→") && (strings.Contains(line, "[x]") || strings.Contains(line, "[ ]")) {
			return true
		}
	}
	return false
}

type task28TUIFixture struct {
	manager     *skills.Manager
	store       *skills.FileOverrideStore
	pathsByName map[string]string
}

func newTask28TUIFixture(t *testing.T, collisions bool) *task28TUIFixture {
	t.Helper()
	root := t.TempDir()
	paths := make(map[string]string)
	projectRoot := filepath.Join(root, "project-skills")
	names := []string{"auto"}
	if collisions {
		names = append(names, "name-only", "manual-only", "off", "review")
	}
	for _, name := range names {
		path := task28TUIWriteSkill(t, projectRoot, name, name, name+" body")
		paths[name] = path
	}
	sources := []skills.DirSource{{Dir: projectRoot, Source: skills.SourceProject}}
	if collisions {
		userRoot := filepath.Join(root, "user-skills")
		task28TUIWriteSkill(t, userRoot, "review-user", "review", "user review body")
		sources = append(sources, skills.DirSource{Dir: userRoot, Source: skills.SourceUser})
	}
	store, err := skills.NewFileOverrideStoreAt(skills.OverrideStorePaths{
		UserSettings:    filepath.Join(root, "settings", "user.json"),
		ProjectSettings: filepath.Join(root, "settings", "project.json"),
	}, nil, skills.NewMemorySessionOverrideLayer())
	if err != nil {
		t.Fatal(err)
	}
	return &task28TUIFixture{
		manager:     skills.NewManagerWithOverrideStore(store, sources...),
		store:       store,
		pathsByName: paths,
	}
}

func task28TUIWriteSkill(t *testing.T, root, directory, name, body string) string {
	t.Helper()
	dir := filepath.Join(root, directory)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "SKILL.md")
	content := "---\nname: " + name + "\ndescription: " + name + " description\n---\n" + body + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func task28TUIRoot() (*AppState, *RootComponent) {
	state := NewAppState()
	state.SessionID.Set("session-a")
	state.Language.Set(i18n.LangEN)
	root := NewRootComponent(state, nil, nil)
	root.termWidth, root.termHeight = 140, 45
	return state, root
}

func task28TUIOpen(t *testing.T, state *AppState, root *RootComponent, backend SkillsManagementBackend) {
	t.Helper()
	menu := newSkillsMenuState(SkillsMenuOpenRequest{
		SessionID: func() string { return state.SessionID.Get() },
		Language:  func() i18n.Language { return state.Language.Get() },
		Backend:   backend,
	})
	state.SkillsMenu.Set(menu)
	root.openSkillsMenu(menu)
	task28TUIWait(t, func() bool {
		current := state.SkillsMenu.Get()
		return current != nil && current.Toggle.HasSnapshot && !current.Toggle.Loading
	})
}

func task28TUIDispatch(root *RootComponent, event gtui.KeyEvent) bool {
	for _, binding := range root.KeyMap() {
		if slashAwareKeyMatches(binding.Pattern, event) {
			binding.Handler(event)
			return binding.Stop
		}
	}
	return false
}

func task28TUIText(element *gtui.Element) string {
	if element == nil {
		return ""
	}
	parts := []string{}
	if value := element.Text(); value != "" {
		parts = append(parts, value)
	}
	for _, child := range element.Children() {
		if value := task28TUIText(child); value != "" {
			parts = append(parts, value)
		}
	}
	return strings.Join(parts, "\n")
}

func task28TUIRenderBuffer(root *RootComponent, menu *SkillsMenuState) string {
	panel := root.renderSkillsMenu(menu)
	buffer := gtui.NewBuffer(root.termWidth, root.termHeight)
	panel.Render(buffer, root.termWidth, root.termHeight)
	return buffer.String()
}

func task28TUIWait(t *testing.T, ready func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if ready() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("timed out waiting for skills surface")
}

func task28TUISnapshot(t *testing.T, manager *skills.Manager, sessionID string) skills.CatalogSnapshot {
	t.Helper()
	snapshot, err := manager.Snapshot(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func task28TUIRowsByName(snapshot skills.CatalogSnapshot) map[string][]skills.EffectiveSkill {
	rows := make(map[string][]skills.EffectiveSkill)
	for _, row := range snapshot.Skills {
		rows[row.Name] = append(rows[row.Name], row)
	}
	return rows
}

type task28TUIToggleCall struct {
	id       skills.SkillID
	revision skills.CatalogRevision
}

type task28TUIBlockingToggleBackend struct {
	manager *skills.Manager
	started chan<- struct{}
	release <-chan struct{}
	mu      sync.Mutex
	call    task28TUIToggleCall
}

func (backend *task28TUIBlockingToggleBackend) Snapshot(sessionID string) (skills.CatalogSnapshot, error) {
	return backend.manager.Snapshot(sessionID)
}

func (backend *task28TUIBlockingToggleBackend) ToggleProjectVisibility(sessionID string, id skills.SkillID, revision skills.CatalogRevision) (skills.ProjectVisibilityToggleResult, error) {
	backend.mu.Lock()
	backend.call = task28TUIToggleCall{id: id, revision: revision}
	backend.mu.Unlock()
	backend.started <- struct{}{}
	<-backend.release
	return backend.manager.ToggleProjectVisibility(sessionID, id, revision)
}

func (backend *task28TUIBlockingToggleBackend) lastCall() task28TUIToggleCall {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	return backend.call
}

type task28TUIDegradedBackend struct {
	manager        *skills.Manager
	refreshStarted chan<- struct{}
	releaseRefresh <-chan struct{}
	mu             sync.Mutex
	snapshots      int
	toggles        int
}

func (backend *task28TUIDegradedBackend) Snapshot(sessionID string) (skills.CatalogSnapshot, error) {
	backend.mu.Lock()
	backend.snapshots++
	call := backend.snapshots
	backend.mu.Unlock()
	if call == 2 {
		backend.refreshStarted <- struct{}{}
		<-backend.releaseRefresh
	}
	return backend.manager.Snapshot(sessionID)
}

func (backend *task28TUIDegradedBackend) ToggleProjectVisibility(sessionID string, id skills.SkillID, revision skills.CatalogRevision) (skills.ProjectVisibilityToggleResult, error) {
	backend.mu.Lock()
	backend.toggles++
	backend.mu.Unlock()
	result, err := backend.manager.ToggleProjectVisibility(sessionID, id, revision)
	if err != nil {
		return result, err
	}
	result.Outcome = skills.ProjectVisibilityToggleDegraded
	result.Reason = skills.ProjectVisibilityToggleReasonRollbackFailed
	if validateErr := result.Validate(); validateErr != nil {
		return result, validateErr
	}
	return result, errors.New("injected rollback failure after persisted commit")
}

func (backend *task28TUIDegradedBackend) snapshotCount() int {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	return backend.snapshots
}

func (backend *task28TUIDegradedBackend) toggleCount() int {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	return backend.toggles
}

type task28TUITransactionFault struct {
	base                  skills.OverrideStore
	mu                    sync.Mutex
	compareErr            error
	failSnapshotsAfterCAS int
	snapshotFailures      int
}

func task28TUIFaultManager(t *testing.T, fault *task28TUITransactionFault) (*skills.Manager, *skills.FileOverrideStore, skills.EffectiveSkill) {
	t.Helper()
	root := t.TempDir()
	skillRoot := filepath.Join(root, "skills")
	task28TUIWriteSkill(t, skillRoot, "failure", "failure", "failure body")
	base, err := skills.NewFileOverrideStoreAt(skills.OverrideStorePaths{
		UserSettings:    filepath.Join(root, "settings", "user.json"),
		ProjectSettings: filepath.Join(root, "settings", "project.json"),
	}, nil, skills.NewMemorySessionOverrideLayer())
	if err != nil {
		t.Fatal(err)
	}
	if fault == nil {
		t.Fatal("nil transaction fault")
	}
	fault.base = base
	manager := skills.NewManagerWithOverrideStore(fault, skills.DirSource{Dir: skillRoot, Source: skills.SourceProject})
	snapshot := task28TUISnapshot(t, manager, "session-a")
	if len(snapshot.Skills) != 1 {
		t.Fatalf("transaction fixture snapshot=%#v", snapshot)
	}
	return manager, base, snapshot.Skills[0]
}

func (store *task28TUITransactionFault) Snapshot(sessionID string) (skills.OverrideSnapshot, error) {
	store.mu.Lock()
	if store.snapshotFailures > 0 {
		store.snapshotFailures--
		store.mu.Unlock()
		return skills.OverrideSnapshot{}, errors.New("task28 injected live snapshot failure")
	}
	store.mu.Unlock()
	return store.base.Snapshot(sessionID)
}

func (store *task28TUITransactionFault) Set(sessionID string, override skills.VisibilityOverride) error {
	return store.base.Set(sessionID, override)
}

func (store *task28TUITransactionFault) Toggle(sessionID string, scope skills.SkillScope, id skills.SkillID) (skills.VisibilityOverride, error) {
	return store.base.Toggle(sessionID, scope, id)
}

func (store *task28TUITransactionFault) Reset(sessionID string, scope skills.SkillScope, id skills.SkillID) error {
	return store.base.Reset(sessionID, scope, id)
}

func (store *task28TUITransactionFault) CompareAndSetProject(expected skills.OverrideStoreRevision, id skills.SkillID, next *skills.VisibilityOverride) (skills.ProjectOverrideRestore, error) {
	store.mu.Lock()
	compareErr := store.compareErr
	store.mu.Unlock()
	if compareErr != nil {
		return skills.ProjectOverrideRestore{}, compareErr
	}
	restore, err := store.base.CompareAndSetProject(expected, id, next)
	if err == nil {
		store.mu.Lock()
		store.snapshotFailures += store.failSnapshotsAfterCAS
		store.mu.Unlock()
	}
	return restore, err
}

func (store *task28TUITransactionFault) RestoreProject(restore skills.ProjectOverrideRestore) error {
	return store.base.RestoreProject(restore)
}

var _ skills.OverrideStore = (*task28TUITransactionFault)(nil)
