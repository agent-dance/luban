package swarm

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/agent-dance/luban/brand"
)

func TestMailboxTask23_LayoutReadFlagAndColor(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	mailbox, err := NewMailbox("layout-team")
	if err != nil {
		t.Fatal(err)
	}
	if err := mailbox.Send(context.Background(), "worker-1", Message{
		From: "team-lead", Text: "hello", Summary: "send current status", Color: "blue", Read: true,
	}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, brand.ConfigDirName, "teams", "layout-team", "inboxes", "worker-1.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read TS-compatible inbox path %q: %v", path, err)
	}
	var raw []map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if len(raw) != 1 || raw[0]["read"] != false || raw[0]["color"] != "blue" || raw[0]["summary"] != "send current status" {
		t.Fatalf("persisted TeammateMessage = %#v", raw)
	}
	unread, err := mailbox.ReadUnread(context.Background(), "worker-1")
	if err != nil || len(unread) != 1 {
		t.Fatalf("ReadUnread = %#v, err=%v", unread, err)
	}
	if err := mailbox.MarkAllRead(context.Background(), "worker-1"); err != nil {
		t.Fatal(err)
	}
	all, err := mailbox.Read(context.Background(), "worker-1")
	if err != nil || len(all) != 1 || !all[0].Read || all[0].Color != "blue" {
		t.Fatalf("audit-preserving Read = %#v, err=%v", all, err)
	}
}

func TestMailboxTask23_LegacyLayoutMigratesWithoutLoss(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	legacy := filepath.Join(home, brand.ConfigDirName, "teams", "migration-team", "worker-1", "inbox.json")
	if err := os.MkdirAll(filepath.Dir(legacy), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, []byte(`[{"from":"old","text":"legacy","timestamp":"2026-01-01T00:00:00Z","color":"red"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	mailbox, err := NewMailbox("migration-team")
	if err != nil {
		t.Fatal(err)
	}
	messages, err := mailbox.Read(context.Background(), "worker-1")
	if err != nil || len(messages) != 1 || messages[0].Text != "legacy" || messages[0].Read {
		t.Fatalf("migrated messages = %#v, err=%v", messages, err)
	}
	canonical := filepath.Join(home, brand.ConfigDirName, "teams", "migration-team", "inboxes", "worker-1.json")
	if _, err := os.Stat(canonical); err != nil {
		t.Fatalf("canonical inbox was not created: %v", err)
	}
	if _, err := os.Stat(legacy); err != nil {
		t.Fatalf("legacy inbox must remain during mixed-version migration: %v", err)
	}
	again, err := mailbox.Read(context.Background(), "worker-1")
	if err != nil || len(again) != 1 || again[0].ID != messages[0].ID || again[0].Sequence != 1 {
		t.Fatalf("repeated migration must be idempotent: first=%#v again=%#v err=%v", messages, again, err)
	}
}
