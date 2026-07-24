package tui

import (
	"strings"
	"testing"

	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/i18n"
	gtui "github.com/grindlemire/go-tui"
)

func TestRenderMessageArea_ShowsBrandLogoOnEmptyHome(t *testing.T) {
	state := NewAppState()
	state.Provider.Set("deepseek")
	state.Model.Set("deepseek-v4-flash")
	root := NewRootComponent(state, nil, nil)
	root.termWidth = 120

	text := collectElementText(root.renderMessageArea(18))

	for _, want := range []string{"█████", brand.RuntimeName, i18n.Text(state.Language.Get(), i18n.KeyBrandTagline)} {
		if !strings.Contains(text, want) {
			t.Fatalf("home screen missing %q:\n%s", want, text)
		}
	}
}

func TestRootRenderFillsConfiguredBackground(t *testing.T) {
	root := NewRootComponent(NewAppState(), nil, nil)
	element := root.renderAtSize(nil, 80, 24)
	if element.Background() == nil || !element.Background().Bg.Equal(gtui.Black) {
		t.Fatalf("root background = %+v, want configured background %+v", element.Background(), gtui.Black)
	}
	buffer := gtui.NewBuffer(80, 24)
	element.Render(buffer, 80, 24)
	for _, point := range [][2]int{{0, 0}, {79, 0}, {0, 23}, {79, 23}, {40, 12}} {
		cell := buffer.Cell(point[0], point[1])
		if !cell.Style.Bg.Equal(gtui.Black) {
			t.Fatalf("root cell (%d,%d) background = %+v, want configured background %+v", point[0], point[1], cell.Style.Bg, gtui.Black)
		}
	}
}

func TestRenderMessageArea_HidesBrandLogoAfterConversationStarts(t *testing.T) {
	state := NewAppState()
	state.Messages.Set([]Message{{Kind: MsgInfo, Text: "ready"}})
	root := NewRootComponent(state, nil, nil)
	root.termWidth = 120

	text := collectElementText(root.renderMessageArea(18))

	if strings.Contains(text, brand.RuntimeName) || strings.Contains(text, i18n.Text(state.Language.Get(), i18n.KeyBrandTagline)) {
		t.Fatalf("home logo should not be shown once messages exist:\n%s", text)
	}
	if !strings.Contains(text, "ready") {
		t.Fatalf("expected conversation message to render, got:\n%s", text)
	}
}

func TestRenderStartupViewportShowsCompleteLogoAndCleanStatus(t *testing.T) {
	state := NewAppState()
	state.Provider.Set("deepseek")
	state.Model.Set("deepseek-v4-flash")
	root := NewRootComponent(state, nil, nil)

	rendered := renderElementText(root.renderAtSize(nil, 100, 30), 100, 30)
	normalized := strings.ReplaceAll(rendered, "\u2800", " ")
	for _, want := range []string{
		"█████",
		"███",
		"Auto mode",
	} {
		if !strings.Contains(normalized, want) {
			t.Fatalf("startup viewport missing %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(strings.ToLower(rendered), "usage unknown") {
		t.Fatalf("startup viewport exposed unknown usage:\n%s", rendered)
	}
	t.Logf("startup viewport:\n%s", rendered)
}
