package tui

import (
	"strings"

	gtui "github.com/grindlemire/go-tui"
	"github.com/rivo/uniseg"
	"golang.org/x/text/unicode/norm"
)

const (
	skillsPanelBorderRows       = 2
	skillsPanelHorizontalChrome = 4 // two border cells plus one padding cell per side
	skillsPanelMaximumListRows  = 7
	skillsPanelMinimumListRows  = 3
)

// skillsPanelLayoutRequest describes only presentation facts. AvailableHeight
// is the number of terminal rows the caller has reserved for the whole panel,
// including both borders; it is deliberately not the full terminal height
// when the panel is a normal child of the application's root column.
type skillsPanelLayoutRequest struct {
	TerminalWidth   int
	AvailableHeight int
	HasSnapshot     bool
	TotalRows       int
	Selected        int
	DetailRows      int
	HasFilter       bool
	NoticeRows      int
	Refreshing      bool
}

// skillsPanelLayout is a complete pre-render budget. Every true chrome flag,
// visible list row, and detail row consumes exactly one content row. Rendering
// must not append anything that is absent from this value.
type skillsPanelLayout struct {
	PanelHeight int
	InnerWidth  int
	ContentRows int
	ChromeRows  int

	Start int
	End   int

	DetailRows int

	ShowTitle       bool
	ShowFilter      bool
	ShowPlaceholder bool
	ShowPager       bool
	NoticeRows      int
	ShowRefreshing  bool
	ShowHelp        bool
}

// calculateSkillsPanelLayout assigns scarce rows in a deterministic order:
// borders, one selected list row (or the empty-state message), title/filter,
// a small usable list window, live status, selected details, pager/help, then
// additional list rows. The selected row therefore remains visible even when
// optional detail and guidance rows must disappear.
func calculateSkillsPanelLayout(request skillsPanelLayoutRequest) skillsPanelLayout {
	layout := skillsPanelLayout{InnerWidth: request.TerminalWidth - skillsPanelHorizontalChrome}
	if layout.InnerWidth < 0 {
		layout.InnerWidth = 0
	}
	height := request.AvailableHeight
	if height <= 0 {
		return layout
	}
	if height <= skillsPanelBorderRows {
		layout.PanelHeight = height
		return layout
	}

	remaining := height - skillsPanelBorderRows
	consumeChrome := func(target *bool) bool {
		if remaining <= 0 || *target {
			return false
		}
		*target = true
		remaining--
		return true
	}
	consumeNotice := func() {
		rows := request.NoticeRows
		if rows < 0 {
			rows = 0
		}
		layout.NoticeRows = min(rows, remaining)
		remaining -= layout.NoticeRows
	}

	if !request.HasSnapshot {
		// While loading or reporting a failed load, the status is the useful
		// content. The title and help are secondary at very short heights.
		consumeNotice()
		consumeChrome(&layout.ShowTitle)
		consumeChrome(&layout.ShowHelp)
		return finalizeSkillsPanelLayout(layout)
	}

	total := request.TotalRows
	if total < 0 {
		total = 0
	}
	selected := request.Selected
	if total > 0 {
		if selected < 0 {
			selected = 0
		}
		if selected >= total {
			selected = total - 1
		}

		visibleRows := 1
		remaining-- // the selected row is the first usable content priority
		consumeChrome(&layout.ShowTitle)
		if request.HasFilter {
			consumeChrome(&layout.ShowFilter)
		}

		minimumRows := min(total, skillsPanelMinimumListRows)
		for visibleRows < minimumRows && remaining > 0 {
			visibleRows++
			remaining--
		}
		consumeNotice()
		if request.Refreshing {
			consumeChrome(&layout.ShowRefreshing)
		}

		details := request.DetailRows
		if details < 0 {
			details = 0
		}
		layout.DetailRows = min(details, remaining)
		remaining -= layout.DetailRows

		maximumRows := min(total, skillsPanelMaximumListRows)
		extraRows := maximumRows - visibleRows
		footerRows := 1 // help
		if total > maximumRows {
			footerRows++ // pager
		}
		// When all remaining list rows plus truthful footer chrome fit, add
		// them before the footer. This makes an unconstrained layout a fixed
		// point when its PanelHeight is later reserved by the root.
		if remaining >= extraRows+footerRows {
			for visibleRows < maximumRows {
				visibleRows++
				remaining--
			}
			if total > visibleRows {
				consumeChrome(&layout.ShowPager)
			}
			consumeChrome(&layout.ShowHelp)
		} else {
			if total > visibleRows {
				consumeChrome(&layout.ShowPager)
			}
			consumeChrome(&layout.ShowHelp)
		}
		for visibleRows < maximumRows && remaining > 0 {
			visibleRows++
			remaining--
		}
		// A short allocation can reach the end only after reserving a pager.
		// Reclaim that now-untruthful row for a detail or help line.
		if visibleRows == total && layout.ShowPager {
			layout.ShowPager = false
			remaining++
			if layout.DetailRows < details {
				layout.DetailRows++
				remaining--
			} else if !layout.ShowHelp {
				consumeChrome(&layout.ShowHelp)
			}
		}

		layout.Start, layout.End = skillsToggleVisibleRange(total, selected, visibleRows)
		return finalizeSkillsPanelLayout(layout)
	}

	// An empty catalog or a filter with no matches gets one bounded message
	// before optional chrome. This keeps a three-row panel informative.
	consumeChrome(&layout.ShowPlaceholder)
	consumeChrome(&layout.ShowTitle)
	if request.HasFilter {
		consumeChrome(&layout.ShowFilter)
	}
	consumeNotice()
	if request.Refreshing {
		consumeChrome(&layout.ShowRefreshing)
	}
	consumeChrome(&layout.ShowHelp)
	return finalizeSkillsPanelLayout(layout)
}

func finalizeSkillsPanelLayout(layout skillsPanelLayout) skillsPanelLayout {
	layout.ChromeRows = 0
	for _, visible := range []bool{
		layout.ShowTitle,
		layout.ShowFilter,
		layout.ShowPlaceholder,
		layout.ShowPager,
		layout.ShowRefreshing,
		layout.ShowHelp,
	} {
		if visible {
			layout.ChromeRows++
		}
	}
	layout.ChromeRows += layout.NoticeRows
	layout.ContentRows = layout.End - layout.Start + layout.DetailRows + layout.ChromeRows
	layout.PanelHeight = layout.ContentRows + skillsPanelBorderRows
	return layout
}

// normalizeSkillsPanelLine turns every catalog/backend-provided value into
// inert one-line text before width calculation. This shares the transcript's
// CSI/OSC/DCS/C0/C1 sanitizer so an embedded escape cannot move the cursor or
// invalidate a supposedly one-row panel budget. NFC normalization keeps
// canonically equivalent text stable without discarding valid grapheme clusters.
func normalizeSkillsPanelLine(value string) string {
	plain := strings.Join(strings.Fields(sanitizePresentationTerminalText(value)), " ")
	return norm.NFC.String(plain)
}

// wrapSkillsPanelNoticeLines preserves complete recovery instructions while
// keeping every visual child to one row. The slash reset command is treated as
// one token when it fits, so copying it from the rendered buffer never inserts
// a line break into the stable ID or its --scope argument.
func wrapSkillsPanelNoticeLines(value string, maxCells int) []string {
	plain := normalizeSkillsPanelLine(value)
	if plain == "" || maxCells <= 0 {
		return nil
	}
	tokens := skillsPanelNoticeTokens(plain)
	lines := make([]string, 0, 3)
	current := ""
	flush := func() {
		if current != "" {
			lines = append(lines, current)
			current = ""
		}
	}
	for _, token := range tokens {
		if token == "" {
			continue
		}
		candidate := token
		if current != "" {
			candidate = current + " " + token
		}
		if skillsPanelCellWidth(candidate) <= maxCells {
			current = candidate
			continue
		}
		flush()
		if skillsPanelCellWidth(token) > maxCells {
			lines = append(lines, truncateSkillsPanelLineMiddle(token, maxCells))
			continue
		}
		current = token
	}
	flush()
	return lines
}

func skillsPanelNoticeTokens(plain string) []string {
	const commandPrefix = "/skills reset "
	const commandSuffix = " --scope session"
	start := strings.Index(plain, commandPrefix)
	if start < 0 {
		return strings.Fields(plain)
	}
	relativeEnd := strings.Index(plain[start+len(commandPrefix):], commandSuffix)
	if relativeEnd < 0 {
		return strings.Fields(plain)
	}
	end := start + len(commandPrefix) + relativeEnd + len(commandSuffix)
	tokens := strings.Fields(plain[:start])
	tokens = append(tokens, plain[start:end])
	tokens = append(tokens, strings.Fields(plain[end:])...)
	return tokens
}

func visibleSkillsPanelNoticeLines(value string, maxCells, limit int) []string {
	lines := wrapSkillsPanelNoticeLines(value, maxCells)
	if limit <= 0 || len(lines) == 0 {
		return nil
	}
	if limit >= len(lines) {
		return lines
	}
	if limit == 1 {
		return []string{truncateSkillsPanelLineMiddle(value, maxCells)}
	}
	selected := make([]string, 0, limit)
	appendUnique := func(line string) {
		if line == "" {
			return
		}
		for _, existing := range selected {
			if existing == line {
				return
			}
		}
		if len(selected) < limit {
			selected = append(selected, line)
		}
	}
	commandLine := ""
	for _, line := range lines[1 : len(lines)-1] {
		if strings.Contains(line, "/skills reset ") {
			commandLine = line
			break
		}
	}
	if commandLine == "" || limit > 2 {
		appendUnique(lines[0])
	}
	appendUnique(commandLine)
	appendUnique(lines[len(lines)-1])
	for _, line := range lines[1:] {
		appendUnique(line)
	}
	return selected
}

// skillsPanelCellWidth matches go-tui's grapheme-aware buffer renderer.
func skillsPanelCellWidth(value string) int {
	return gtui.StringWidth(value)
}

// truncateSkillsPanelLine returns normalized valid UTF-8 that fits maxCells,
// appending one ellipsis only when truncation occurs. Grapheme iteration keeps
// combining/variation/ZWJ sequences intact; budgeting each complete cluster
// with the renderer's cell widths prevents its later no-wrap draw from
// disagreeing with this calculation.
func truncateSkillsPanelLine(value string, maxCells int) string {
	plain := normalizeSkillsPanelLine(value)
	if maxCells <= 0 || plain == "" {
		return ""
	}
	if skillsPanelCellWidth(plain) <= maxCells {
		return plain
	}
	const ellipsis = "…"
	ellipsisWidth := skillsPanelCellWidth(ellipsis)
	if ellipsisWidth > maxCells {
		return ""
	}
	available := maxCells - ellipsisWidth
	used := 0
	var output strings.Builder
	graphemes := uniseg.NewGraphemes(plain)
	for graphemes.Next() {
		cluster := graphemes.Str()
		clusterWidth := skillsPanelCellWidth(cluster)
		if used+clusterWidth > available {
			break
		}
		output.WriteString(cluster)
		used += clusterWidth
	}
	output.WriteString(ellipsis)
	return output.String()
}

// truncateSkillsPanelLineMiddle is reserved for transactional notices whose
// outcome is stated up front and whose recovery action is stated at the end.
// Keeping both ends makes the visible one-row receipt actionable while the
// complete localized sentence remains available to linear consumers.
func truncateSkillsPanelLineMiddle(value string, maxCells int) string {
	plain := normalizeSkillsPanelLine(value)
	if maxCells <= 0 || plain == "" {
		return ""
	}
	if skillsPanelCellWidth(plain) <= maxCells {
		return plain
	}
	const ellipsis = "…"
	ellipsisWidth := skillsPanelCellWidth(ellipsis)
	if ellipsisWidth > maxCells {
		return ""
	}
	available := maxCells - ellipsisWidth
	// Recovery guidance and reset commands live at the end of every current
	// transaction notice, so give the suffix the larger share.
	prefixBudget := available * 2 / 5
	suffixBudget := available - prefixBudget
	return skillsPanelGraphemePrefix(plain, prefixBudget) + ellipsis + skillsPanelGraphemeSuffix(plain, suffixBudget)
}

func skillsPanelGraphemePrefix(value string, maxCells int) string {
	if maxCells <= 0 {
		return ""
	}
	used := 0
	var output strings.Builder
	graphemes := uniseg.NewGraphemes(value)
	for graphemes.Next() {
		cluster := graphemes.Str()
		width := skillsPanelCellWidth(cluster)
		if used+width > maxCells {
			break
		}
		output.WriteString(cluster)
		used += width
	}
	return output.String()
}

func skillsPanelGraphemeSuffix(value string, maxCells int) string {
	if maxCells <= 0 {
		return ""
	}
	clusters := make([]string, 0, len(value))
	graphemes := uniseg.NewGraphemes(value)
	for graphemes.Next() {
		clusters = append(clusters, graphemes.Str())
	}
	used := 0
	start := len(clusters)
	for start > 0 {
		width := skillsPanelCellWidth(clusters[start-1])
		if used+width > maxCells {
			break
		}
		start--
		used += width
	}
	return strings.Join(clusters[start:], "")
}
