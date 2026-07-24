package tui

import (
	"testing"
	"time"

	"github.com/agent-dance/luban/types"
)

func TestNewAppState(t *testing.T) {
	s := NewAppState()
	if s == nil {
		t.Fatal("NewAppState returned nil")
	}
	if msgs := s.Messages.Get(); len(msgs) != 0 {
		t.Errorf("expected empty messages, got %d", len(msgs))
	}
	if req := s.PermReq.Get(); req != nil {
		t.Error("expected nil perm req")
	}
	if mode := s.Mode.Get(); mode != ModeAutoEdit {
		t.Fatalf("default interaction mode = %v, want auto", mode)
	}
}

func TestAppendMessage(t *testing.T) {
	s := NewAppState()
	s.AppendMessage(Message{Kind: MsgUser, Text: "hello"})
	s.AppendMessage(Message{Kind: MsgAssistant, Text: "hi there"})

	msgs := s.Messages.Get()
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Kind != MsgUser || msgs[0].Text != "hello" {
		t.Errorf("msg[0] = %+v", msgs[0])
	}
	if msgs[1].Kind != MsgAssistant || msgs[1].Text != "hi there" {
		t.Errorf("msg[1] = %+v", msgs[1])
	}
}

func TestAppendToLast(t *testing.T) {
	s := NewAppState()
	s.AppendMessage(Message{Kind: MsgAssistant, Text: "hel"})
	s.AppendToLast("lo")
	s.AppendToLast(" world")

	msgs := s.Messages.Get()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Text != "hello world" {
		t.Errorf("expected 'hello world', got %q", msgs[0].Text)
	}
}

func TestAppendToLastEmpty(t *testing.T) {
	s := NewAppState()
	// AppendToLast on empty messages should be a no-op
	s.AppendToLast("text")
	msgs := s.Messages.Get()
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages, got %d", len(msgs))
	}
}

func TestAccumulateSessionUsageKeepsLatestContextAndSessionTotals(t *testing.T) {
	s := NewAppState()

	s.AccumulateSessionUsage(&types.Usage{
		InputTokens:              1000,
		OutputTokens:             120,
		CacheReadInputTokens:     400,
		CacheCreationInputTokens: 100,
	})
	s.AccumulateSessionUsage(&types.Usage{
		InputTokens:              500,
		OutputTokens:             80,
		CacheReadInputTokens:     200,
		CacheCreationInputTokens: 50,
	})

	if got := s.SessionInputTokens.Get(); got != 500 {
		t.Fatalf("SessionInputTokens = %d, want latest turn 500", got)
	}
	if got := s.SessionOutputTokens.Get(); got != 80 {
		t.Fatalf("SessionOutputTokens = %d, want latest request 80", got)
	}
	if got := s.SessionCacheReadTokens.Get(); got != 200 {
		t.Fatalf("SessionCacheReadTokens = %d, want latest turn 200", got)
	}
	if got := s.SessionCacheCreateTokens.Get(); got != 50 {
		t.Fatalf("SessionCacheCreateTokens = %d, want latest turn 50", got)
	}

	totals := s.ActiveSessionUsage()
	if totals.InputTokens != 1500 || totals.OutputTokens != 200 || totals.CacheReadTokens != 600 || totals.CacheCreateTokens != 150 ||
		totals.LastInputTokens != 500 || totals.LastOutputTokens != 80 {
		t.Fatalf("ActiveSessionUsage() = %+v, want cumulative input/output/cache 1500/200/600/150", totals)
	}
}

func TestMarkSessionCompactedAccumulatesOnlyEachRoundsLastRequest(t *testing.T) {
	s := NewAppState()
	s.AccumulateSessionUsage(&types.Usage{InputTokens: 1000, OutputTokens: 120, CacheReadInputTokens: 400})
	s.AccumulateSessionUsage(&types.Usage{InputTokens: 1500, OutputTokens: 80, CacheReadInputTokens: 600})
	s.MarkSessionCompacted()

	usage := s.ActiveSessionUsage()
	if !usage.HasCompacted || usage.CompactionCount != 1 || usage.CompletedRoundInputTokens != 1500 || usage.CompletedRoundOutputTokens != 80 {
		t.Fatalf("first completed round = %+v, want count/input/output 1/1500/80", usage)
	}
	if usage.LastInputTokens != 0 || usage.LastOutputTokens != 0 || usage.LastCacheReadTokens != 0 {
		t.Fatalf("new round inherited the previous request: %+v", usage)
	}

	s.AccumulateSessionUsage(&types.Usage{InputTokens: 700, OutputTokens: 60, CacheReadInputTokens: 200})
	s.AccumulateSessionUsage(&types.Usage{InputTokens: 900, OutputTokens: 70, CacheReadInputTokens: 450})
	s.MarkSessionCompacted()
	s.AccumulateSessionUsage(&types.Usage{InputTokens: 600, OutputTokens: 40, CacheReadInputTokens: 300})
	usage = s.ActiveSessionUsage()
	if usage.CompactionCount != 2 || usage.CompletedRoundInputTokens != 2400 || usage.CompletedRoundOutputTokens != 150 {
		t.Fatalf("completed round totals = %+v, want count/input/output 2/2400/150", usage)
	}
	if usage.LastInputTokens != 600 || usage.LastOutputTokens != 40 || usage.LastCacheReadTokens != 300 {
		t.Fatalf("current round latest request = %+v, want input/output/cache 600/40/300", usage)
	}
	if usage.InputTokens != 4700 || usage.OutputTokens != 370 {
		t.Fatalf("full request ledger changed by round accounting: %+v", usage)
	}
}

func TestApplySessionInfo_ResetsUsageOnSessionChange(t *testing.T) {
	s := NewAppState()
	s.ApplySessionInfo("sess-1", []string{"Read"})
	s.CumulativeCost.Set(0.0834)
	s.SessionInputTokens.Set(1000)
	s.SessionOutputTokens.Set(250)
	s.SessionCacheReadTokens.Set(400)
	s.SessionCacheCreateTokens.Set(100)

	changed := s.ApplySessionInfo("sess-2", []string{"Read", "Write"})
	if !changed {
		t.Fatal("expected session change to be reported")
	}
	if got := s.SessionID.Get(); got != "sess-2" {
		t.Fatalf("SessionID = %q, want sess-2", got)
	}
	if got := s.CumulativeCost.Get(); got != 0 {
		t.Fatalf("CumulativeCost = %f, want 0", got)
	}
	if got := s.SessionInputTokens.Get(); got != 0 {
		t.Fatalf("SessionInputTokens = %d, want 0", got)
	}
	if got := s.SessionOutputTokens.Get(); got != 0 {
		t.Fatalf("SessionOutputTokens = %d, want 0", got)
	}
	if got := s.SessionCacheReadTokens.Get(); got != 0 {
		t.Fatalf("SessionCacheReadTokens = %d, want 0", got)
	}
	if got := s.SessionCacheCreateTokens.Get(); got != 0 {
		t.Fatalf("SessionCacheCreateTokens = %d, want 0", got)
	}
	if got := s.Tools.Get(); len(got) != 2 || got[1] != "Write" {
		t.Fatalf("Tools = %v, want updated tool list", got)
	}
}

func TestAppendOrStreamText_NewMessage(t *testing.T) {
	s := NewAppState()

	// First call creates a new assistant message
	s.AppendOrStreamText("Hello")
	msgs := s.Messages.Get()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Kind != MsgAssistant {
		t.Errorf("expected MsgAssistant, got %d", msgs[0].Kind)
	}
	if msgs[0].Text != "Hello" {
		t.Errorf("expected 'Hello', got %q", msgs[0].Text)
	}

	// Subsequent call appends to existing assistant message
	s.AppendOrStreamText(" World")
	msgs = s.Messages.Get()
	if len(msgs) != 1 {
		t.Fatalf("expected still 1 message, got %d", len(msgs))
	}
	if msgs[0].Text != "Hello World" {
		t.Errorf("expected 'Hello World', got %q", msgs[0].Text)
	}
}

func TestAppendOrStreamText_AfterNonAssistant(t *testing.T) {
	s := NewAppState()

	// Add a non-assistant message first
	s.AppendMessage(Message{Kind: MsgError, Text: "something broke"})

	// AppendOrStreamText should create a NEW assistant message,
	// not try to append to the error
	s.AppendOrStreamText("recovery text")
	msgs := s.Messages.Get()
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Kind != MsgError {
		t.Errorf("msg[0] expected MsgError, got %d", msgs[0].Kind)
	}
	if msgs[1].Kind != MsgAssistant || msgs[1].Text != "recovery text" {
		t.Errorf("msg[1] = %+v", msgs[1])
	}
}

func TestQueryCancel(t *testing.T) {
	s := NewAppState()

	// No cancel set → TryCancelQuery returns false
	if s.TryCancelQuery() {
		t.Error("expected false when no cancel set")
	}

	// Set cancel → TryCancelQuery returns true and calls fn
	called := 0
	s.SetQueryCancel(func() { called++ })
	if !s.TryCancelQuery() {
		t.Error("expected true when cancel set")
	}
	if called != 1 {
		t.Errorf("expected cancel fn called once, got %d", called)
	}
	if !s.HasActiveQuery() {
		t.Fatal("cancel request cleared in-flight state before terminal checkpoint commit")
	}

	// Swap-and-nil: second TryCancelQuery should return false (no double cancel)
	if s.TryCancelQuery() {
		t.Error("expected false on second call (swap-and-nil)")
	}
	if called != 1 {
		t.Errorf("expected cancel fn still called once after second try, got %d", called)
	}
	s.ClearQueryCancel()
	if s.HasActiveQuery() {
		t.Fatal("ClearQueryCancel did not publish the terminal query boundary")
	}

	// Clear → back to false
	s.SetQueryCancel(func() { called++ })
	s.ClearQueryCancel()
	if s.TryCancelQuery() {
		t.Error("expected false after clear")
	}
}

func TestToolInputPreview(t *testing.T) {
	tests := []struct {
		name   string
		input  map[string]any
		expect string
	}{
		{"Bash", map[string]any{"command": "ls -la"}, "`ls -la`"},
		{"Read", map[string]any{"file_path": "/etc/hosts"}, "/etc/hosts"},
		{"Glob", map[string]any{"pattern": "*.go"}, "*.go"},
		{"Grep", map[string]any{"pattern": "TODO"}, "/TODO/"},
		{"Unknown", map[string]any{"key": "val"}, ""},
	}

	for _, tt := range tests {
		result := toolInputPreview(tt.name, tt.input)
		if result != tt.expect {
			t.Errorf("toolInputPreview(%q, %v) = %q, want %q", tt.name, tt.input, result, tt.expect)
		}
	}
}

func TestPermissionReqState(t *testing.T) {
	s := NewAppState()

	// Initially nil
	if s.PermReq.Get() != nil {
		t.Error("expected nil")
	}

	// Can set
	s.PermReq.Set(&PermissionReq{ToolName: "Bash", RiskLevel: 2})
	req := s.PermReq.Get()
	if req == nil {
		t.Fatal("expected non-nil")
	}
	if req.ToolName != "Bash" {
		t.Errorf("expected Bash, got %q", req.ToolName)
	}
	if req.RiskLevel != 2 {
		t.Errorf("expected risk 2, got %d", req.RiskLevel)
	}
}

func TestMessageTimestamp(t *testing.T) {
	s := NewAppState()
	before := time.Now()
	s.AppendMessage(Message{Kind: MsgUser, Text: "test", Timestamp: time.Now()})
	after := time.Now()

	msgs := s.Messages.Get()
	ts := msgs[0].Timestamp
	if ts.Before(before) || ts.After(after) {
		t.Errorf("timestamp %v not in range [%v, %v]", ts, before, after)
	}
}

func TestFmtK(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{0, "0"},
		{500, "500"},
		{999, "999"},
		{1000, "1.0K"},
		{1500, "1.5K"},
		{10000, "10.0K"},
		{134000, "134.0K"},
	}
	for _, tt := range tests {
		got := fmtK(tt.n)
		if got != tt.want {
			t.Errorf("fmtK(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestClearMessages(t *testing.T) {
	s := NewAppState()
	s.AppendMessage(Message{Kind: MsgUser, Text: "hello"})
	s.AppendMessage(Message{Kind: MsgAssistant, Text: "hi"})

	if got := len(s.Messages.Get()); got != 2 {
		t.Fatalf("expected 2 messages before clear, got %d", got)
	}

	s.ClearMessages()

	msgs := s.Messages.Get()
	if len(msgs) != 0 {
		t.Errorf("expected empty messages after clear, got %d", len(msgs))
	}
}

func TestCopyOnWriteAppend(t *testing.T) {
	s := NewAppState()
	s.AppendMessage(Message{Kind: MsgUser, Text: "first"})

	// Grab a reference to the current slice
	before := s.Messages.Get()
	if len(before) != 1 {
		t.Fatalf("expected 1 message, got %d", len(before))
	}

	// Append another message — should NOT modify the 'before' slice
	s.AppendMessage(Message{Kind: MsgAssistant, Text: "second"})

	after := s.Messages.Get()
	if len(after) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(after))
	}

	// The old snapshot must still have exactly 1 element
	if len(before) != 1 {
		t.Errorf("copy-on-write violated: old slice now has %d elements", len(before))
	}
}

func TestCopyOnWriteStreamAppend(t *testing.T) {
	s := NewAppState()
	s.AppendMessage(Message{Kind: MsgAssistant, Text: "hel"})

	// Grab a reference to the current slice
	before := s.Messages.Get()

	// Stream-append — should NOT modify 'before'
	s.AppendToLast("lo")

	after := s.Messages.Get()
	if after[0].Text != "hello" {
		t.Errorf("expected 'hello', got %q", after[0].Text)
	}

	// Old snapshot must still show "hel"
	if before[0].Text != "hel" {
		t.Errorf("copy-on-write violated: old slice text changed to %q", before[0].Text)
	}
}

func TestSessionPickerStateClamp(t *testing.T) {
	s := &SessionPickerState{
		Entries:  []SessionEntry{{ID: "a"}, {ID: "b"}},
		Selected: 10,
	}
	s.clamp()
	if s.Selected != 1 {
		t.Fatalf("expected selected to clamp to 1, got %d", s.Selected)
	}
	s.Selected = -5
	s.clamp()
	if s.Selected != 0 {
		t.Fatalf("expected selected to clamp to 0, got %d", s.Selected)
	}
}

func TestPreviewMessages(t *testing.T) {
	msgs := []Message{
		{Text: "one"},
		{Text: "two"},
		{Text: "three"},
		{Text: "four"},
		{Text: "five"},
	}
	out := previewMessages(msgs, 3)
	if len(out) != 3 {
		t.Fatalf("expected 3 preview messages, got %d", len(out))
	}
	if out[0].Text != "three" || out[2].Text != "five" {
		t.Fatalf("unexpected preview slice: %#v", out)
	}
}

func TestBannerStateFields(t *testing.T) {
	s := NewAppState()

	if got := s.Provider.Get(); got != "" {
		t.Errorf("expected empty provider, got %q", got)
	}
	if got := s.Model.Get(); got != "" {
		t.Errorf("expected empty model, got %q", got)
	}

	s.Provider.Set("Anthropic")
	s.Model.Set("claude-3.5-sonnet")
	s.SessionID.Set("sess-123")
	s.Tools.Set([]string{"Bash", "Read", "Write"})

	if got := s.Provider.Get(); got != "Anthropic" {
		t.Errorf("expected 'Anthropic', got %q", got)
	}
	if got := s.Model.Get(); got != "claude-3.5-sonnet" {
		t.Errorf("expected 'claude-3.5-sonnet', got %q", got)
	}
	if got := s.SessionID.Get(); got != "sess-123" {
		t.Errorf("expected 'sess-123', got %q", got)
	}
	tools := s.Tools.Get()
	if len(tools) != 3 || tools[0] != "Bash" {
		t.Errorf("expected [Bash Read Write], got %v", tools)
	}
}
