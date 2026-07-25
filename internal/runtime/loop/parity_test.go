package loop

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/types"
)

type parityFixture struct {
	SchemaVersion string                `json:"schema_version"`
	ID            string                `json:"id"`
	Title         string                `json:"title"`
	Status        string                `json:"status"`
	UpdatedAt     string                `json:"updated_at"`
	SourceTests   []string              `json:"source_tests"`
	CoverageTags  []string              `json:"coverage_tags"`
	ParityTasks   []string              `json:"parity_tasks"`
	Input         parityInputFixture    `json:"input"`
	Tools         []parityToolFixture   `json:"tools"`
	Turns         []parityTurnFixture   `json:"turns"`
	Expected      parityExpectedFixture `json:"expected"`
}

type parityInputFixture struct {
	UserMessage     string `json:"user_message"`
	MaxTurns        int    `json:"max_turns"`
	MaxTokens       int    `json:"max_tokens"`
	DisableMaxTurns bool   `json:"disable_max_turns"`
}

type parityToolFixture struct {
	Name           string `json:"name"`
	Kind           string `json:"kind"`
	Content        string `json:"content"`
	EchoPrefix     string `json:"echo_prefix"`
	IsError        bool   `json:"is_error"`
	ConcurrentSafe bool   `json:"concurrent_safe"`
	ReferenceTool  string `json:"reference_tool"`
}

type parityTurnFixture struct {
	Events  []types.StreamEvent `json:"events"`
	Error   *types.APIError     `json:"error"`
	DelayMS int                 `json:"delay_ms"`
}

type parityExpectedFixture struct {
	TerminalReason      string                       `json:"terminal_reason"`
	FinalText           string                       `json:"final_text"`
	MessageCount        int                          `json:"message_count"`
	ErrorContains       string                       `json:"error_contains"`
	Events              []parityExpectedEventFixture `json:"events"`
	ProviderCallCount   int                          `json:"provider_call_count"`
	ToolVisibilityTurns []parityToolVisibility       `json:"tool_visibility_turns"`
	SupplementalImage   bool                         `json:"supplemental_image"`
}

type parityExpectedEventFixture struct {
	Type               stream.EventType `json:"type"`
	Text               string           `json:"text"`
	TextContains       string           `json:"text_contains"`
	ToolName           string           `json:"tool_name"`
	ToolResultContains string           `json:"tool_result_contains"`
}

type parityToolVisibility struct {
	Turn    int      `json:"turn"`
	Has     []string `json:"has"`
	Missing []string `json:"missing"`
}

func TestParityFixtures(t *testing.T) {
	fixtures := loadParityFixtures(t)
	if len(fixtures) < 5 {
		t.Fatalf("loaded %d active parity fixtures, want at least 5", len(fixtures))
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.ID, func(t *testing.T) {
			if fixture.Status == "pending" || fixture.Status == "expected_failure" {
				t.Skipf("fixture is coverage metadata only: %s", fixture.Status)
			}
			runParityFixture(t, fixture)
		})
	}
}

func loadParityFixtures(t *testing.T) []parityFixture {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join("testdata", "parity", "*.json"))
	if err != nil {
		t.Fatalf("glob parity fixtures: %v", err)
	}
	var fixtures []parityFixture
	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		var fixture parityFixture
		if err := json.Unmarshal(data, &fixture); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
		if fixture.SchemaVersion != "1" {
			t.Fatalf("%s schema_version = %q, want 1", path, fixture.SchemaVersion)
		}
		if fixture.ID == "" {
			t.Fatalf("%s missing id", path)
		}
		fixtures = append(fixtures, fixture)
	}
	return fixtures
}

func runParityFixture(t *testing.T, fixture parityFixture) {
	t.Helper()
	turns := make([]parityProviderTurn, 0, len(fixture.Turns))
	for _, turn := range fixture.Turns {
		turns = append(turns, parityProviderTurn(turn))
	}
	prov := newParityFakeProvider(turns)
	reg := newParityRegistry(t, fixture.Tools)
	cfg := Config{
		MaxTurns:        fixture.Input.MaxTurns,
		MaxTokens:       fixture.Input.MaxTokens,
		DisableMaxTurns: fixture.Input.DisableMaxTurns,
	}
	ql := New(prov, reg, cfg)

	var events []stream.Event
	err := ql.Run(context.Background(), fixture.Input.UserMessage, func(evt stream.Event) {
		events = append(events, evt)
	})

	if fixture.Expected.ErrorContains != "" {
		if err == nil {
			t.Fatalf("Run error = nil, want containing %q", fixture.Expected.ErrorContains)
		}
		if !strings.Contains(err.Error(), fixture.Expected.ErrorContains) {
			t.Fatalf("Run error = %q, want containing %q", err.Error(), fixture.Expected.ErrorContains)
		}
	} else if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if fixture.Expected.FinalText != "" {
		if got := joinedEventText(events); got != fixture.Expected.FinalText {
			t.Fatalf("final text = %q, want %q", got, fixture.Expected.FinalText)
		}
	}
	if fixture.Expected.MessageCount > 0 && len(ql.Messages()) != fixture.Expected.MessageCount {
		t.Fatalf("message count = %d, want %d", len(ql.Messages()), fixture.Expected.MessageCount)
	}
	if fixture.Expected.ProviderCallCount > 0 && len(prov.Calls) != fixture.Expected.ProviderCallCount {
		t.Fatalf("provider calls = %d, want %d", len(prov.Calls), fixture.Expected.ProviderCallCount)
	}
	assertExpectedEvents(t, events, fixture.Expected.Events)
	assertToolVisibility(t, prov.Calls, fixture.Expected.ToolVisibilityTurns)
	if fixture.Expected.SupplementalImage {
		assertSupplementalImage(t, ql.Messages())
	}
}

func assertExpectedEvents(t *testing.T, actual []stream.Event, expected []parityExpectedEventFixture) {
	t.Helper()
	next := 0
	for _, want := range expected {
		found := false
		for next < len(actual) {
			got := actual[next]
			next++
			if eventMatches(got, want) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing expected event after index %d: %+v; actual=%+v", next, want, actual)
		}
	}
}

func eventMatches(got stream.Event, want parityExpectedEventFixture) bool {
	if got.Type != want.Type {
		return false
	}
	if want.Text != "" && got.Text != want.Text {
		return false
	}
	if want.TextContains != "" && !strings.Contains(got.Text, want.TextContains) {
		return false
	}
	if want.ToolName != "" {
		if got.ToolUse == nil || got.ToolUse.Name != want.ToolName {
			return false
		}
	}
	if want.ToolResultContains != "" {
		if got.ToolResult == nil || !strings.Contains(got.ToolResult.TextContent(), want.ToolResultContains) {
			return false
		}
	}
	return true
}

func assertToolVisibility(t *testing.T, calls []provider.Params, expected []parityToolVisibility) {
	t.Helper()
	for _, want := range expected {
		if want.Turn < 0 || want.Turn >= len(calls) {
			t.Fatalf("tool visibility turn %d out of range; provider calls=%d", want.Turn, len(calls))
		}
		var names []string
		for _, tool := range calls[want.Turn].Tools {
			names = append(names, tool.Name)
		}
		for _, name := range want.Has {
			if !hasString(names, name) {
				t.Fatalf("turn %d tools missing %q: %v", want.Turn, name, names)
			}
		}
		for _, name := range want.Missing {
			if hasString(names, name) {
				t.Fatalf("turn %d tools unexpectedly include %q: %v", want.Turn, name, names)
			}
		}
	}
}

func assertSupplementalImage(t *testing.T, messages []types.Message) {
	t.Helper()
	for _, msg := range messages {
		for _, block := range msg.Content {
			img, ok := block.(types.ImageBlock)
			if ok && img.Source != nil && img.Source.Data != "" {
				return
			}
		}
	}
	t.Fatal("expected supplemental image message")
}
