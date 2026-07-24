package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agent-dance/luban/skills"
	"github.com/agent-dance/luban/ui"
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
	if strings.Count(text, "Review carefully") != 1 {
		t.Fatalf("screen-reader /skills duplicated the legacy body:\n%s", text)
	}
	if strings.ContainsAny(text, "\x1b\r") {
		t.Fatalf("screen-reader /skills emitted terminal control: %q", text)
	}

	output.Reset()
	handled, exit, err = handleScreenReaderCommand(context.Background(), cfg, renderer, ui.NewCostTracker("test"), "/skills disable review")
	if err != nil || !handled || exit || manager.IsEnabled(sessionID, "review") {
		t.Fatalf("handle disable = handled %t exit %t enabled %t err %v", handled, exit, manager.IsEnabled(sessionID, "review"), err)
	}
}
