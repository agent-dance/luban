package ui

import (
	"bytes"
	"strings"
	"testing"
)

func TestTermRendererSemanticToolPresentationSuccessUsesStableFieldOrder(t *testing.T) {
	var output bytes.Buffer
	renderer := NewTermRenderer(&output)
	renderer.RenderToolPresentation(completeToolPresentation(ToolPresentationStateSucceeded))

	assertToolPresentationFieldOrder(t, output.String())
	for _, want := range []string{
		"Tool update. Tool: Bash. Tool use ID: tool-7. Work unit: work-2",
		"State: succeeded",
		"Details available",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("classic presentation missing %q:\n%s", want, output.String())
		}
	}
}

func TestScreenReaderSemanticToolPresentationFailureIsAppendOnlyAndTextual(t *testing.T) {
	var output bytes.Buffer
	renderer := NewScreenReaderRenderer(&output, nil)
	t.Cleanup(func() { _ = renderer.Close() })

	presentation := completeToolPresentation(ToolPresentationStateFailed)
	presentation.Result = "exit status 1"
	renderer.RenderToolPresentation(presentation)

	got := output.String()
	assertToolPresentationFieldOrder(t, got)
	for _, want := range []string{"State: failed.", "Result: exit status 1.", "Details available."} {
		if !strings.Contains(got, want) {
			t.Fatalf("screen-reader presentation missing %q:\n%s", want, got)
		}
	}
	if strings.ContainsAny(got, "\r\x1b") || strings.Contains(got, "spinner") {
		t.Fatalf("screen-reader presentation contains overwrite or animation noise: %q", got)
	}
}

func TestSemanticToolPresentationRedactionMarkerIsExplicit(t *testing.T) {
	for _, test := range []struct {
		name   string
		render func(*bytes.Buffer)
	}{
		{
			name: "classic",
			render: func(output *bytes.Buffer) {
				renderer := NewTermRenderer(output)
				renderer.RenderToolPresentation(ToolPresentation{
					ToolName: "MCPTool", State: ToolPresentationStateSucceeded,
					Result: "credential fields removed", Redacted: true,
				})
			},
		},
		{
			name: "screen_reader",
			render: func(output *bytes.Buffer) {
				renderer := NewScreenReaderRenderer(output, nil)
				defer renderer.Close()
				renderer.RenderToolPresentation(ToolPresentation{
					ToolName: "MCPTool", State: ToolPresentationStateSucceeded,
					Result: "credential fields removed", Redacted: true,
				})
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			test.render(&output)
			if !strings.Contains(output.String(), "Redacted: sensitive content omitted") {
				t.Fatalf("redaction marker missing: %q", output.String())
			}
		})
	}
}

func TestSemanticToolPresentationBoundsDetailsAndControlCharacters(t *testing.T) {
	longDetail := strings.Repeat("x", maxToolPresentationDetailRunes+200)
	details := []string{
		"first\x1b[2Jdetail",
		longDetail,
		"third\nline",
		"fourth must not be rendered",
		"fifth must not be rendered",
	}
	var output bytes.Buffer
	renderer := NewScreenReaderRenderer(&output, nil)
	defer renderer.Close()
	renderer.RenderToolPresentation(ToolPresentation{
		ToolName: "Bash", State: ToolPresentationStatePartial,
		Result: strings.Repeat("result ", 200), DetailLines: details, HasMore: true,
		PresentationLevel: ToolPresentationLevelEvidence,
	})

	got := output.String()
	if strings.Contains(got, "fourth must not be rendered") || strings.Contains(got, "fifth must not be rendered") {
		t.Fatalf("unbounded detail leaked into append-only output: %q", got)
	}
	if strings.Contains(got, "\x1b") || strings.Contains(got, "\nline") {
		t.Fatalf("control characters were not normalized: %q", got)
	}
	for _, want := range []string{"Detail 3: third line.", "Details omitted: 2 additional lines.", "Details available."} {
		if !strings.Contains(got, want) {
			t.Fatalf("bounded presentation missing %q: %q", want, got)
		}
	}
	if len(got) > 1800 {
		t.Fatalf("bounded presentation grew to %d bytes: %q", len(got), got)
	}
}

func TestSemanticToolPresentationHiddenLevelIsSilent(t *testing.T) {
	for _, render := range []func(*bytes.Buffer){
		func(output *bytes.Buffer) {
			NewTermRenderer(output).RenderToolPresentation(ToolPresentation{
				State: ToolPresentationStateRunning, PresentationLevel: ToolPresentationLevelHidden,
			})
		},
		func(output *bytes.Buffer) {
			renderer := NewScreenReaderRenderer(output, nil)
			defer renderer.Close()
			renderer.RenderToolPresentation(ToolPresentation{
				State: ToolPresentationStateRunning, PresentationLevel: ToolPresentationLevelHidden,
			})
		},
	} {
		var output bytes.Buffer
		render(&output)
		if output.Len() != 0 {
			t.Fatalf("hidden presentation wrote %q", output.String())
		}
	}
}

func TestSemanticToolPresentationFoldedOmitsStructuredDetails(t *testing.T) {
	var output bytes.Buffer
	renderer := NewScreenReaderRenderer(&output, nil)
	defer renderer.Close()
	renderer.RenderToolPresentation(ToolPresentation{
		ToolName: "Aggregate", State: ToolPresentationStateSucceeded, Result: "Read - 6 operations",
		DetailLines: []string{"Member IDs: a, b, c, d, e, f"}, ReasonCodes: []string{"aggregation_candidate"},
		PresentationLevel: ToolPresentationLevelFolded, HasMore: true,
	})
	got := output.String()
	if strings.Contains(got, "Member IDs") || strings.Contains(got, "Reason codes") {
		t.Fatalf("folded projection leaked structured details: %q", got)
	}
	if !strings.Contains(got, "Read - 6 operations") || !strings.Contains(got, "Details available") {
		t.Fatalf("folded projection lost summary or drill-down signal: %q", got)
	}
}

func completeToolPresentation(state string) ToolPresentation {
	return ToolPresentation{
		ToolName: "Bash", ToolUseID: "tool-7", WorkUnitID: "work-2",
		Actor: "agent-1", Action: "run tests", Object: "./...", State: state,
		Result: "42 tests passed", NextAction: "review changes",
		DetailLines:       []string{"duration 1.2 seconds"},
		PresentationLevel: ToolPresentationLevelStructured,
		ReasonCodes:       []string{"side_effect", "needs_review"}, HasMore: true,
	}
}

func assertToolPresentationFieldOrder(t *testing.T, output string) {
	t.Helper()
	fields := []string{
		"Actor:", "Action:", "Object:", "State:", "Result:", "Next action:",
		"Detail 1:", "Presentation level:", "Reason codes:", "Details available",
	}
	previous := -1
	for _, field := range fields {
		position := strings.Index(output, field)
		if position < 0 {
			t.Fatalf("field %q missing from presentation:\n%s", field, output)
		}
		if position <= previous {
			t.Fatalf("field %q is out of order in presentation:\n%s", field, output)
		}
		previous = position
	}
}
