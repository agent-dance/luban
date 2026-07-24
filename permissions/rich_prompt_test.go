package permissions

import (
	"bytes"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

type blockingPromptReader struct {
	mu           sync.Mutex
	active       int
	maxActive    int
	reads        int
	firstEntered chan struct{}
	releaseFirst chan struct{}
}

func (r *blockingPromptReader) Read(p []byte) (int, error) {
	r.mu.Lock()
	r.active++
	if r.active > r.maxActive {
		r.maxActive = r.active
	}
	r.reads++
	read := r.reads
	r.mu.Unlock()

	if read == 1 {
		close(r.firstEntered)
		<-r.releaseFirst
	}
	n := copy(p, "y\n")
	r.mu.Lock()
	r.active--
	r.mu.Unlock()
	if n == 0 {
		return 0, io.EOF
	}
	return n, nil
}

func (r *blockingPromptReader) maximumConcurrency() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.maxActive
}

func TestRichPromptSerializesConcurrentPermissionQuestions(t *testing.T) {
	reader := &blockingPromptReader{
		firstEntered: make(chan struct{}),
		releaseFirst: make(chan struct{}),
	}
	var out bytes.Buffer
	prompt := NewRichPrompt(&out, reader).PromptFunc()
	results := make(chan Decision, 2)
	go func() { results <- prompt("Write", map[string]any{"file_path": "a.txt"}) }()
	<-reader.firstEntered
	go func() { results <- prompt("Write", map[string]any{"file_path": "b.txt"}) }()

	time.Sleep(20 * time.Millisecond)
	if got := reader.maximumConcurrency(); got != 1 {
		close(reader.releaseFirst)
		t.Fatalf("permission reader concurrency = %d, want 1", got)
	}
	close(reader.releaseFirst)
	for range 2 {
		select {
		case decision := <-results:
			if decision != DecisionAllowOnce {
				t.Fatalf("decision = %v, want allow once", decision)
			}
		case <-time.After(time.Second):
			t.Fatal("concurrent permission prompt did not finish")
		}
	}
}

func TestRichPrompt_LowRiskBadge(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	in := strings.NewReader("n\n")
	rp := NewRichPrompt(&out, in)
	fn := rp.PromptFunc()
	fn("Read", map[string]any{"file_path": "foo.go"})
	got := out.String()
	if !strings.Contains(got, "Low") {
		t.Errorf("expected 'Low' in output for Read tool, got:\n%s", got)
	}
}

func TestRichPrompt_MediumRiskBadge(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	in := strings.NewReader("n\n")
	rp := NewRichPrompt(&out, in)
	fn := rp.PromptFunc()
	fn("Bash", map[string]any{"command": "mkdir build"})
	got := out.String()
	if !strings.Contains(got, "Medium") {
		t.Errorf("expected 'Medium' in output for mkdir, got:\n%s", got)
	}
}

func TestRichPrompt_HighRiskBadge(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	in := strings.NewReader("n\n")
	rp := NewRichPrompt(&out, in)
	fn := rp.PromptFunc()
	fn("Bash", map[string]any{"command": "rm -rf /"})
	got := out.String()
	if !strings.Contains(got, "High") {
		t.Errorf("expected 'High' in output for rm -rf /, got:\n%s", got)
	}
}

func TestRichPrompt_ShowsToolName(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	in := strings.NewReader("n\n")
	rp := NewRichPrompt(&out, in)
	fn := rp.PromptFunc()
	fn("Bash", map[string]any{"command": "ls"})
	got := out.String()
	if !strings.Contains(got, "Bash") {
		t.Errorf("expected tool name 'Bash' in output, got:\n%s", got)
	}
}

func TestRichPrompt_ResponseY_ReturnsAllowOnce(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	in := strings.NewReader("y\n")
	rp := NewRichPrompt(&out, in)
	fn := rp.PromptFunc()
	d := fn("Bash", map[string]any{"command": "ls"})
	if d != DecisionAllowOnce {
		t.Errorf("response 'y' should return DecisionAllowOnce, got %v", d)
	}
}

func TestRichPrompt_ResponseA_ReturnsAllow(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	in := strings.NewReader("a\n")
	rp := NewRichPrompt(&out, in)
	fn := rp.PromptFunc()
	d := fn("Bash", map[string]any{"command": "ls"})
	if d != DecisionAllow {
		t.Errorf("response 'a' should return DecisionAllow, got %v", d)
	}
}

func TestRichPrompt_ResponseN_ReturnsDeny(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	in := strings.NewReader("n\n")
	rp := NewRichPrompt(&out, in)
	fn := rp.PromptFunc()
	d := fn("Bash", map[string]any{"command": "ls"})
	if d != DecisionDeny {
		t.Errorf("response 'n' should return DecisionDeny, got %v", d)
	}
}

func TestRichPrompt_EmptyResponse_ReturnsDeny(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	in := strings.NewReader("\n")
	rp := NewRichPrompt(&out, in)
	fn := rp.PromptFunc()
	d := fn("Bash", map[string]any{"command": "ls"})
	if d != DecisionDeny {
		t.Errorf("empty response should return DecisionDeny, got %v", d)
	}
}

func TestRichPrompt_EOF_ReturnsDeny(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	in := strings.NewReader("") // EOF immediately
	rp := NewRichPrompt(&out, in)
	fn := rp.PromptFunc()
	d := fn("Bash", map[string]any{"command": "ls"})
	if d != DecisionDeny {
		t.Errorf("EOF should return DecisionDeny, got %v", d)
	}
}

func TestRichPrompt_ShowsPromptLine(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	in := strings.NewReader("n\n")
	rp := NewRichPrompt(&out, in)
	fn := rp.PromptFunc()
	fn("Write", map[string]any{"file_path": "output.txt"})
	got := out.String()
	// The prompt text should contain the option hints
	if !strings.Contains(got, "y") || !strings.Contains(got, "N") || !strings.Contains(got, "a") {
		t.Errorf("expected prompt options [y/N/a] in output, got:\n%s", got)
	}
}

func TestRichPrompt_FilePathShown(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	in := strings.NewReader("n\n")
	rp := NewRichPrompt(&out, in)
	fn := rp.PromptFunc()
	fn("Write", map[string]any{"file_path": "unique_sentinel_path.go"})
	got := out.String()
	if !strings.Contains(got, "unique_sentinel_path.go") {
		t.Errorf("expected file path in output, got:\n%s", got)
	}
}

func TestRichPrompt_BashCommandShown(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	in := strings.NewReader("n\n")
	rp := NewRichPrompt(&out, in)
	fn := rp.PromptFunc()
	fn("Bash", map[string]any{"command": "echo sentinel_command_xyz"})
	got := out.String()
	if !strings.Contains(got, "sentinel_command_xyz") {
		t.Errorf("expected command in output, got:\n%s", got)
	}
}

func TestRichPrompt_SendMessageShowsTargetAndAlways(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	in := strings.NewReader("n\n")
	rp := NewRichPrompt(&out, in)
	fn := rp.PromptFunc()
	fn("SendMessage", map[string]any{
		"to":      "worker-1",
		"message": "hello teammate",
	})
	got := out.String()
	if !strings.Contains(got, "worker-1") {
		t.Errorf("expected SendMessage target in output, got:\n%s", got)
	}
	if !strings.Contains(got, "Low") {
		t.Errorf("expected read-only plain SendMessage to be low risk, got:\n%s", got)
	}
	if !strings.Contains(got, "a(lways)") {
		t.Errorf("expected always-allow option for SendMessage, got:\n%s", got)
	}
}
