package tui

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/skills"
	gtui "github.com/grindlemire/go-tui"
	"github.com/rivo/uniseg"
)

func TestSkillsLayoutUnicodeSingleLineTruncationPreservesGraphemes(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{name: "ascii", value: strings.Repeat("catalog", 8)},
		{name: "CJK", value: strings.Repeat("技能列表", 8)},
		{name: "combining", value: strings.Repeat("cafe\u0301", 8)},
		{name: "emoji ZWJ", value: strings.Repeat("👩🏽\u200d💻", 8)},
		{name: "variation selector", value: strings.Repeat("✈️", 12)},
		{name: "ANSI multiline", value: "\x1b[31mred\x1b[0m\nnext\trow"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for width := 0; width <= 18; width++ {
				got := truncateSkillsPanelLine(test.value, width)
				if !utf8.ValidString(got) {
					t.Fatalf("width %d produced invalid UTF-8: %q", width, got)
				}
				if strings.ContainsAny(got, "\r\n\t\x1b") {
					t.Fatalf("width %d retained control/layout bytes: %q", width, got)
				}
				if cells := skillsPanelCellWidth(got); cells > width {
					t.Fatalf("width %d emitted %d cells: %q", width, cells, got)
				}
				if physical := uniseg.StringWidth(got); physical != skillsPanelCellWidth(got) {
					t.Fatalf("width %d renderer/physical cells disagree: renderer=%d physical=%d text=%q", width, skillsPanelCellWidth(got), physical, got)
				}
				assertSkillsLayoutWholeGraphemePrefix(t, normalizeSkillsPanelLine(test.value), strings.TrimSuffix(got, "…"))
			}
		})
	}
}

func TestSkillsLayoutUnicodeNFCAndAtomicFallbackMatchRendererCells(t *testing.T) {
	if got := normalizeSkillsPanelLine("e\u0301"); got != "é" {
		t.Fatalf("composable combining sequence=%q, want NFC é", got)
	}
	for _, input := range []string{"👩🏽\u200d💻", "✈️", "x\u0301"} {
		got := normalizeSkillsPanelLine(input)
		if got != "�" || strings.Contains(got, "…") {
			t.Fatalf("mismatched cluster %q was not replaced atomically: %q", input, got)
		}
		if skillsPanelCellWidth(got) != uniseg.StringWidth(got) {
			t.Fatalf("fallback width disagrees for %q", got)
		}
	}
}

func TestSkillsLayoutNarrowNoticesKeepOutcomeAndRecoverySuffix(t *testing.T) {
	id := skills.SkillID("skill:project:" + strings.Repeat("a", 64))
	tests := []struct {
		name   string
		notice SkillsToggleNotice
		want   []string
	}{
		{
			name: "stale retry",
			notice: SkillsToggleNotice{
				Kind: skillsToggleNoticeStale, SkillID: id, Revision: 91,
			},
			want: []string{"The row", "Space again"},
		},
		{
			name: "session reset",
			notice: SkillsToggleNotice{
				Kind: skillsToggleNoticeSessionOverride, SkillID: id, Name: "alpha",
			},
			want: []string{"alpha", "--scope session", "Space"},
		},
		{
			name: "rollback conclusion",
			notice: SkillsToggleNotice{
				Kind: skillsToggleNoticeRolledBack, SkillID: id, Name: "alpha",
			},
			want: []string{"The live update", "rolled back"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			full := formatSkillsToggleNotice(i18n.LangEN, test.notice)
			lines := wrapSkillsPanelNoticeLines(full, 140)
			got := strings.Join(lines, "\n")
			for _, line := range lines {
				if width := skillsPanelCellWidth(line); width > 140 {
					t.Fatalf("notice width=%d: %q", width, line)
				}
				if strings.ContainsAny(line, "\r\n\t\x1b") {
					t.Fatalf("notice child is not one inert row: %q", line)
				}
			}
			for _, want := range test.want {
				if !strings.Contains(got, want) {
					t.Errorf("notice lost %q: %q", want, got)
				}
			}
			if test.notice.Kind == skillsToggleNoticeSessionOverride {
				short := strings.Join(visibleSkillsPanelNoticeLines(full, 140, 2), "\n")
				command := "/skills reset " + string(id) + " --scope session"
				if !strings.Contains(short, command) || !strings.Contains(short, "Space") {
					t.Fatalf("two-row recovery lost command/action: %q", short)
				}
			}
		})
	}
}

func TestSkillsLayoutBudgetsBordersListDetailsAndOptionalChrome(t *testing.T) {
	for _, height := range []int{0, 1, 2, 3, 4, 6, 8, 12, 24} {
		layout := calculateSkillsPanelLayout(skillsPanelLayoutRequest{
			TerminalWidth: 24, AvailableHeight: height,
			HasSnapshot: true, TotalRows: 20, Selected: 17, DetailRows: 7,
			HasFilter: true, NoticeRows: 3, Refreshing: true,
		})
		if layout.PanelHeight < 0 || layout.PanelHeight > height {
			t.Fatalf("height %d produced panel height %d", height, layout.PanelHeight)
		}
		if layout.PanelHeight >= 2 && layout.ContentRows+2 != layout.PanelHeight {
			t.Fatalf("height %d budget content=%d panel=%d", height, layout.ContentRows, layout.PanelHeight)
		}
		if layout.End-layout.Start+layout.DetailRows+layout.ChromeRows != layout.ContentRows {
			t.Fatalf("height %d line accounting mismatch: %+v", height, layout)
		}
		if layout.End > layout.Start && !(layout.Start <= 17 && 17 < layout.End) {
			t.Fatalf("height %d hid selected row: %+v", height, layout)
		}
		if height == 2 && layout.PanelHeight != 2 {
			t.Fatalf("two-row viewport lost one of its borders: %+v", layout)
		}
		if height == 3 && layout.End-layout.Start != 1 {
			t.Fatalf("three-row viewport should prioritize selected list row: %+v", layout)
		}
	}
}

func TestSkillsMenuHelpPrecedesManagedList(t *testing.T) {
	backend := newTask30Backend(t, task30Snapshot(t, 1, task30Skill("skill:project:alpha", "alpha", skills.SourceProject)))
	state, root := task30Root(backend)
	root.termWidth, root.termHeight = 120, 40
	task30OpenSkills(t, state, root, backend)

	rendered := task28TUIText(root.renderSkillsMenu(state.SkillsMenu.Get()))
	help := i18n.Text(i18n.LangEN, i18n.KeySkillsMenuHelp)
	helpIndex := strings.Index(rendered, help)
	rowIndex := strings.Index(rendered, "→ [x] alpha")
	if helpIndex < 0 || rowIndex < 0 || helpIndex > rowIndex {
		t.Fatalf("management help must precede the managed list: help=%d row=%d\n%s", helpIndex, rowIndex, rendered)
	}
}

func TestSkillsMenuSingleLineRowsAndSelectedOnlyDetails(t *testing.T) {
	first := task31Skill("skill:project:first", "first-技能-👩🏽\u200d💻", "/first/only/SKILL.md")
	first.Summary = "\x1b[31mfirst summary\x1b[0m\n" + strings.Repeat("详情👩🏽\u200d💻", 30)
	second := task31Skill("skill:project:second", "second-技能-✈️", "/second/only/SKILL.md")
	second.Summary = strings.Repeat("second-summary-", 30)
	rows := []skills.EffectiveSkill{first, second}
	for index := 0; index < 8; index++ {
		rows = append(rows, task31Skill(skills.SkillID("skill:project:row-"+string(rune('a'+index))), "row-"+strings.Repeat("中", 20), "/row/SKILL.md"))
	}
	backend := newTask30Backend(t, task30Snapshot(t, 1, rows...))
	state, root := task30Root(backend)
	root.termWidth, root.termHeight = 38, 24
	task30OpenSkills(t, state, root, backend)

	menu := state.SkillsMenu.Get().clone()
	task31SelectSkill(t, menu, first.ID)
	state.SkillsMenu.Set(menu)
	panel := root.renderSkillsMenu(menu)
	task31RenderPanel(t, panel, 38, 24)
	task31AssertEveryPanelLineBounded(t, panel)
	text := collectElementText(panel)
	if !strings.Contains(text, "/first/only/SKILL.md") || strings.Contains(text, "/second/only/SKILL.md") {
		t.Fatalf("details were not selected-only:\n%s", text)
	}
	if strings.ContainsAny(text, "\r\t\x1b") {
		t.Fatalf("panel retained multiline/ANSI input: %q", text)
	}

	menu = state.SkillsMenu.Get().clone()
	task31SelectSkill(t, menu, second.ID)
	state.SkillsMenu.Set(menu)
	panel = root.renderSkillsMenu(menu)
	task31RenderPanel(t, panel, 38, 24)
	text = collectElementText(panel)
	if !strings.Contains(text, "/second/only/SKILL.md") || strings.Contains(text, "/first/only/SKILL.md") {
		t.Fatalf("moving selection retained prior details:\n%s", text)
	}

	menu = state.SkillsMenu.Get().clone()
	menu.Toggle.Query = "first"
	menu.Toggle.applyFilter("")
	state.SkillsMenu.Set(menu)
	panel = root.renderSkillsMenu(menu)
	task31RenderPanel(t, panel, 38, 24)
	text = collectElementText(panel)
	if !strings.Contains(text, "/first/only/SKILL.md") || strings.Contains(text, "/second/only/SKILL.md") {
		t.Fatalf("filter did not replace selected detail block:\n%s", text)
	}
}

func TestSkillsMenuOverflowViewportAndResizeKeepBottomBorderAndSelection(t *testing.T) {
	rows := make([]skills.EffectiveSkill, 0, 16)
	for index := 0; index < 16; index++ {
		id := skills.SkillID("skill:project:item-" + string(rune('a'+index)))
		row := task31Skill(id, "item-"+string(rune('a'+index))+"-"+strings.Repeat("中👩🏽\u200d💻", 18), "/"+strings.Repeat("路径/", 24)+"SKILL.md")
		rows = append(rows, row)
	}
	backend := newTask30Backend(t, task30Snapshot(t, 1, rows...))
	state, root := task30Root(backend)
	task30OpenSkills(t, state, root, backend)
	target := rows[14].ID
	menu := state.SkillsMenu.Get().clone()
	task31SelectSkill(t, menu, target)
	state.SkillsMenu.Set(menu)

	for _, size := range []struct{ width, height int }{
		{0, 0}, {1, 1}, {8, 2}, {12, 3}, {18, 4}, {24, 6}, {40, 8}, {80, 20}, {18, 4}, {80, 20},
	} {
		root.termWidth, root.termHeight = size.width, size.height
		panel := root.renderSkillsMenuAtHeight(state.SkillsMenu.Get(), size.height)
		buffer := task31RenderPanel(t, panel, size.width, size.height)
		if rect := panel.Rect(); rect.X < 0 || rect.Y < 0 || rect.Right() > size.width || rect.Bottom() > size.height {
			t.Fatalf("%dx%d panel escaped viewport: %+v", size.width, size.height, rect)
		}
		if size.height >= 2 && size.width >= 2 {
			bottom := panel.Rect().Bottom() - 1
			if bottom < 0 || bottom >= size.height {
				t.Fatalf("%dx%d bottom border is outside viewport: %+v", size.width, size.height, panel.Rect())
			}
			if left := buffer.Cell(panel.Rect().X, bottom).Rune; left != '╰' {
				t.Fatalf("%dx%d bottom border was clipped, left corner=%q\n%s", size.width, size.height, left, buffer.String())
			}
		}
		if size.height >= 3 && !strings.Contains(collectElementText(panel), "→ [x]") {
			t.Fatalf("%dx%d selected row fell out of computed range: %s", size.width, size.height, collectElementText(panel))
		}
		if selected := state.SkillsMenu.Get().Toggle.selectedRow(); selected == nil || selected.ID != target {
			t.Fatalf("%dx%d resize mutated selection: %#v", size.width, size.height, selected)
		}
		task31AssertChildrenInsidePanel(t, panel)
	}
}

func TestSkillsMenuFullRootShortViewportDoesNotPushPanelBorderBelowInput(t *testing.T) {
	rows := make([]skills.EffectiveSkill, 0, 10)
	for index := 0; index < 10; index++ {
		rows = append(rows, task31Skill(skills.SkillID("skill:project:short-"+string(rune('a'+index))), "short-row-"+strings.Repeat("中", 20), "/short/SKILL.md"))
	}
	backend := newTask30Backend(t, task30Snapshot(t, 1, rows...))
	state, root := task30Root(backend)
	task30OpenSkills(t, state, root, backend)

	for _, size := range []struct{ width, height int }{{40, 12}, {24, 6}, {8, 2}, {1, 1}, {0, 0}} {
		element := root.renderAtSize(nil, size.width, size.height)
		buffer := gtui.NewBuffer(size.width, size.height)
		element.Render(buffer, size.width, size.height)
		if rect := element.Rect(); rect.X < 0 || rect.Y < 0 || rect.Right() > size.width || rect.Bottom() > size.height {
			t.Fatalf("%dx%d root escaped viewport: %+v", size.width, size.height, rect)
		}
		panel := task31FindSkillsPanel(element)
		if panel == nil {
			t.Fatalf("%dx%d full root omitted skills panel", size.width, size.height)
		}
		if rect := panel.Rect(); rect.X < 0 || rect.Y < 0 || rect.Right() > size.width || rect.Bottom() > size.height {
			t.Fatalf("%dx%d skills panel escaped root viewport: %+v", size.width, size.height, rect)
		}
		if size.height >= 2 && size.width >= 2 {
			bottom := panel.Rect().Bottom() - 1
			if left := buffer.Cell(panel.Rect().X, bottom).Rune; left != '╰' {
				t.Fatalf("%dx%d skills bottom border is clipped, corner=%q rect=%+v:\n%s", size.width, size.height, left, panel.Rect(), buffer.String())
			}
		}
	}
}

func assertSkillsLayoutWholeGraphemePrefix(t *testing.T, source, prefix string) {
	t.Helper()
	var whole strings.Builder
	graphemes := uniseg.NewGraphemes(source)
	for graphemes.Next() {
		cluster := graphemes.Str()
		if whole.Len()+len(cluster) > len(prefix) {
			break
		}
		whole.WriteString(cluster)
	}
	if whole.String() != prefix {
		t.Fatalf("truncation split a grapheme: source=%q prefix=%q whole=%q", source, prefix, whole.String())
	}
}

func task31Skill(id skills.SkillID, name, locator string) skills.EffectiveSkill {
	row := task30Skill(id, name, skills.SourceProject)
	row.Locator = skills.SkillLocator(locator)
	return row
}

func task31SelectSkill(t *testing.T, menu *SkillsMenuState, id skills.SkillID) {
	t.Helper()
	for visible, rowIndex := range menu.Toggle.Filtered {
		if menu.Toggle.Snapshot.Skills[rowIndex].ID == id {
			menu.Toggle.Selected = visible
			return
		}
	}
	t.Fatalf("skill %q is not filtered", id)
}

func task31RenderPanel(t *testing.T, panel *gtui.Element, width, height int) *gtui.Buffer {
	t.Helper()
	buffer := gtui.NewBuffer(width, height)
	panel.Render(buffer, width, height)
	return buffer
}

func task31AssertEveryPanelLineBounded(t *testing.T, panel *gtui.Element) {
	t.Helper()
	inner := panel.ContentRect().Width
	var walk func(*gtui.Element)
	walk = func(element *gtui.Element) {
		if element.Hidden() {
			return
		}
		if text := element.Text(); text != "" {
			if strings.ContainsAny(text, "\r\n\t\x1b") {
				t.Fatalf("line is not normalized: %q", text)
			}
			if element.HeightForWidth(inner) != 1 || element.Wrap() {
				t.Fatalf("panel line is not an explicit no-wrap row: text=%q height=%d wrap=%t", text, element.HeightForWidth(inner), element.Wrap())
			}
			if width := skillsPanelCellWidth(text); width > inner && !element.Truncate() {
				t.Fatalf("panel line width=%d exceeds inner=%d: %q", width, inner, text)
			}
		}
		for _, child := range element.Children() {
			walk(child)
		}
	}
	walk(panel)
}

func task31AssertChildrenInsidePanel(t *testing.T, panel *gtui.Element) {
	t.Helper()
	content := panel.ContentRect()
	for _, child := range panel.Children() {
		if !content.ContainsRect(child.Rect()) {
			t.Fatalf("pre-budgeted child escaped panel content: panel=%+v content=%+v child=%+v text=%q", panel.Rect(), content, child.Rect(), child.Text())
		}
	}
}

func task31FindSkillsPanel(element *gtui.Element) *gtui.Element {
	if element == nil {
		return nil
	}
	if element.Border() == gtui.BorderRounded && (len(element.Children()) == 0 || strings.Contains(collectElementText(element), "[x]")) {
		return element
	}
	for _, child := range element.Children() {
		if panel := task31FindSkillsPanel(child); panel != nil {
			return panel
		}
	}
	return nil
}
