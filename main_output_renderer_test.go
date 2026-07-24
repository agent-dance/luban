package main

import (
	"bytes"
	"testing"

	"github.com/agent-dance/luban/cli"
	"github.com/agent-dance/luban/ui"
)

func TestOutputRendererSelection(t *testing.T) {
	for _, test := range []struct {
		name   string
		opts   cli.Options
		assert func(*testing.T, ui.Renderer)
	}{
		{name: "text", opts: cli.Options{OutputFormat: "text"}, assert: func(t *testing.T, renderer ui.Renderer) {
			if _, ok := renderer.(*ui.TermRenderer); !ok {
				t.Fatalf("text renderer = %T, want *ui.TermRenderer", renderer)
			}
		}},
		{name: "json", opts: cli.Options{OutputFormat: "json"}, assert: func(t *testing.T, renderer ui.Renderer) {
			if _, ok := renderer.(*ui.JSONRenderer); !ok {
				t.Fatalf("json renderer = %T, want *ui.JSONRenderer", renderer)
			}
		}},
		{name: "stream-json", opts: cli.Options{OutputFormat: "stream-json"}, assert: func(t *testing.T, renderer ui.Renderer) {
			if _, ok := renderer.(*ui.JSONRenderer); !ok {
				t.Fatalf("stream-json renderer = %T, want *ui.JSONRenderer", renderer)
			}
		}},
		{name: "quiet text", opts: cli.Options{OutputFormat: "text", Quiet: true}, assert: func(t *testing.T, renderer ui.Renderer) {
			if _, ok := renderer.(*ui.QuietRenderer); !ok {
				t.Fatalf("quiet renderer = %T, want *ui.QuietRenderer", renderer)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			renderer, err := newOutputRenderer(test.opts, &bytes.Buffer{})
			if err != nil {
				t.Fatal(err)
			}
			test.assert(t, renderer)
		})
	}
	if _, err := newOutputRenderer(cli.Options{OutputFormat: "xml"}, &bytes.Buffer{}); err == nil {
		t.Fatal("unknown output format was accepted")
	}
}
