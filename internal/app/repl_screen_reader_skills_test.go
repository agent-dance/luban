package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agent-dance/luban/internal/ui/terminal"
	"github.com/agent-dance/luban/skills"
)

func TestScreenReaderSkillsCommandUsesLiveManager(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, "skills", "review")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	skillPath := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillPath, []byte("---\ndescription: Review carefully\n---\n# Review\n"), 0644); err != nil {
		t.Fatal(err)
	}
	manager := skills.NewManager(skills.DirSource{Dir: filepath.Join(root, "skills"), Source: skills.SourceProject})
	manager.SetOverrideStore(newRegistryTestSkillOverrideStore(t, root))

	var output bytes.Buffer
	renderer := ui.NewScreenReaderRenderer(&output, strings.NewReader(""))
	sessionID, cwd := "session-skills", root
	cfg := TUIREPLConfig{
		Engine:       screenReaderLifecycleEngine{},
		SessionID:    &sessionID,
		CWD:          &cwd,
		SkillManager: manager,
	}

	handled, exit, err := handleScreenReaderCommand(context.Background(), cfg, renderer, ui.NewCostTracker("test"), "/skills")
	if err != nil || !handled || exit {
		t.Fatalf("handle /skills = handled %t exit %t err %v", handled, exit, err)
	}
	text := output.String()
	for _, want := range []string{"review", "Review carefully", skillPath} {
		if !strings.Contains(text, want) {
			t.Fatalf("screen-reader /skills omitted %q:\n%s", want, text)
		}
	}
	if strings.ContainsAny(text, "\x1b\r") {
		t.Fatalf("screen-reader /skills emitted terminal control: %q", text)
	}

}
