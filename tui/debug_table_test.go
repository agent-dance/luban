package tui

import (
	"testing"

	"github.com/grindlemire/go-tui"
)

// makeSimpleTable creates a simple 3×3 table with header row for testing.
// Intrinsic size: 7 wide (3 cols × 1 + 2 gaps + 2 border), 6 tall (3 rows + 1 separator + 2 border).
func makeSimpleTable(opts ...tui.Option) *tui.Element {
	baseOpts := []tui.Option{
		tui.WithTag("table"),
		tui.WithDisplay(tui.DisplayFlex),
		tui.WithDirection(tui.Column),
		tui.WithBorder(tui.BorderRounded),
		tui.WithTableHeaderRows(1),
		tui.WithChildren(
			tui.New(
				tui.WithTag("tr"),
				tui.WithDirection(tui.Row),
				tui.WithChildren(
					tui.New(tui.WithTag("th"), tui.WithText("A"), tui.WithWrap(false)),
					tui.New(tui.WithTag("th"), tui.WithText("B"), tui.WithWrap(false)),
					tui.New(tui.WithTag("th"), tui.WithText("C"), tui.WithWrap(false)),
				),
			),
			tui.New(
				tui.WithTag("tr"),
				tui.WithDirection(tui.Row),
				tui.WithChildren(
					tui.New(tui.WithTag("td"), tui.WithText("x"), tui.WithWrap(false)),
					tui.New(tui.WithTag("td"), tui.WithText("y"), tui.WithWrap(false)),
					tui.New(tui.WithTag("td"), tui.WithText("z"), tui.WithWrap(false)),
				),
			),
			tui.New(
				tui.WithTag("tr"),
				tui.WithDirection(tui.Row),
				tui.WithChildren(
					tui.New(tui.WithTag("td"), tui.WithText("1"), tui.WithWrap(false)),
					tui.New(tui.WithTag("td"), tui.WithText("2"), tui.WithWrap(false)),
					tui.New(tui.WithTag("td"), tui.WithText("3"), tui.WithWrap(false)),
				),
			),
		),
	}
	baseOpts = append(baseOpts, opts...)
	return tui.New(baseOpts...)
}

func TestTableIntrinsicSize(t *testing.T) {
	table := makeSimpleTable()
	w, h := table.IntrinsicSize()
	if w != 7 || h != 6 {
		t.Errorf("IntrinsicSize: got (%d, %d), want (7, 6)", w, h)
	}
}

func TestTableExactIntrinsicRender(t *testing.T) {
	table := makeSimpleTable()
	w, h := table.IntrinsicSize()
	buf := tui.NewBuffer(w, h)
	table.Render(buf, w, h)

	rect := table.Rect()
	if rect.Width != 7 || rect.Height != 6 {
		t.Errorf("Table at exact intrinsic size: Rect=%v, want {0 0 7 6}", rect)
	}
}

func TestTableInColumnParent_NoStretch(t *testing.T) {
	// Table with AlignSelf(Start) in a Column parent should keep its intrinsic size,
	// not stretch to fill the parent.
	table := makeSimpleTable(tui.WithAlignSelf(tui.AlignStart))

	root := tui.New(
		tui.WithDirection(tui.Column),
		tui.WithChildren(table),
	)
	buf := tui.NewBuffer(30, 10)
	root.Render(buf, 30, 10)

	rect := table.Rect()
	if rect.Width != 7 {
		t.Errorf("Table width in Column parent with AlignStart: got %d, want 7", rect.Width)
	}
	if rect.Height != 6 {
		t.Errorf("Table height in Column parent with AlignStart: got %d, want 6", rect.Height)
	}
}

func TestTableHeightForWidth_MatchesIntrinsic(t *testing.T) {
	// HeightForWidth for a table should return its intrinsic height,
	// not a value computed by walking children as a generic column container.
	table := makeSimpleTable()
	_, intrinsicH := table.IntrinsicSize()
	h4w := table.HeightForWidth(7)
	if h4w != intrinsicH {
		t.Errorf("HeightForWidth(7)=%d, IntrinsicSize height=%d; should match", h4w, intrinsicH)
	}
	// Even with a different width, table HeightForWidth should return intrinsic height
	// because table layout doesn't reflow based on width.
	h4w30 := table.HeightForWidth(30)
	if h4w30 != intrinsicH {
		t.Errorf("HeightForWidth(30)=%d, IntrinsicSize height=%d; should match", h4w30, intrinsicH)
	}
}
