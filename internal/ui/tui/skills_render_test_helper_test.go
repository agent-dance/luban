package tui

import gtui "github.com/grindlemire/go-tui"

func renderSkillsMenuForTest(root *RootComponent, menu *SkillsMenuState) *gtui.Element {
	width := root.termWidth
	if width <= 0 {
		width = 80
	}
	availableHeight := 1 << 20
	if root.termHeight > 0 {
		availableHeight = root.termHeight * 2 / 3
		if availableHeight < skillsPanelBorderRows {
			availableHeight = min(root.termHeight, skillsPanelBorderRows)
		}
	}
	return root.renderSkillsMenuAtSize(menu, width, availableHeight)
}
