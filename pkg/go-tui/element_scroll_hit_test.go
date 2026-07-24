package tui

import "testing"

func TestContainsPointConvertsScrolledContentToScreenCoordinates(t *testing.T) {
	root := New(WithDirection(Column), WithWidth(20), WithHeight(10))
	root.AddChild(New(WithText("banner"), WithWidth(20), WithHeight(2)))

	scroller := New(
		WithDirection(Column),
		WithWidth(20),
		WithHeight(4),
		WithScrollable(ScrollVertical),
	)
	rows := make([]*Element, 6)
	for index := range rows {
		rows[index] = New(WithText("row"), WithWidthPercent(100), WithHeight(1))
		scroller.AddChild(rows[index])
	}
	root.AddChild(scroller)
	root.Render(NewBuffer(20, 10), 20, 10)
	scroller.ScrollTo(0, 1)
	root.Render(NewBuffer(20, 10), 20, 10)

	header := rows[3]
	viewport := scroller.ContentRect()
	contentRect := header.Rect()
	screenX := viewport.X + contentRect.X
	screenY := viewport.Y + contentRect.Y - 1
	if !header.ContainsPoint(screenX+10, screenY) {
		t.Fatalf("scrolled row missed screen point; content=%+v viewport=%+v", contentRect, viewport)
	}
	if header.ContainsPoint(contentRect.X+10, contentRect.Y) {
		t.Fatalf("scrolled row incorrectly accepted raw content-space point; content=%+v viewport=%+v", contentRect, viewport)
	}
	if hit := root.ElementAt(screenX+10, screenY); hit != header {
		t.Fatalf("ElementAt hit %p, want scrolled header %p", hit, header)
	}

	clipped := rows[0]
	if clipped.ContainsPoint(viewport.X, viewport.Y) {
		t.Fatal("row above the scroll viewport remained clickable")
	}
}
