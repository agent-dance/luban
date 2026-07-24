//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package compact

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/agent-dance/luban/types"
)

func TestResultStoreRejectsMultiplyLinkedExistingResult(t *testing.T) {
	rs := NewResultStore(t.TempDir())
	content := strings.Repeat("private result\n", 20)
	result := types.ToolResultBlock{
		Type:      types.ContentTypeToolResult,
		ToolUseID: "toolu_linked_result",
		Content:   content,
		Metadata:  map[string]string{"maxResultSizeChars": "10"},
	}
	if _, err := rs.ProcessResultForTool(result, "Bash"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(rs.dir, result.ToolUseID+".txt")
	alias := filepath.Join(t.TempDir(), "result.alias")
	if err := os.Link(path, alias); err != nil {
		t.Skipf("hard links are unavailable: %v", err)
	}
	if _, err := rs.ProcessResultForTool(result, "Bash"); !errors.Is(err, fs.ErrInvalid) {
		t.Fatalf("ProcessResultForTool error = %v, want fs.ErrInvalid", err)
	}
}

func TestResultStoreRejectsFIFOWithoutBlocking(t *testing.T) {
	rs := NewResultStore(t.TempDir())
	content := strings.Repeat("private result\n", 20)
	result := types.ToolResultBlock{
		Type:      types.ContentTypeToolResult,
		ToolUseID: "toolu_fifo_result",
		Content:   content,
		Metadata:  map[string]string{"maxResultSizeChars": "10"},
	}
	path := filepath.Join(rs.dir, result.ToolUseID+".txt")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Skipf("FIFOs are unavailable: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := rs.ProcessResultForTool(result, "Bash")
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, fs.ErrInvalid) {
			t.Fatalf("ProcessResultForTool FIFO error = %v, want fs.ErrInvalid", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ProcessResultForTool blocked while opening a FIFO")
	}
}
