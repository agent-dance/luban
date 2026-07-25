package taskstore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func newTestStore(t *testing.T, listID string) *Store {
	t.Helper()
	store := New(func() string { return listID })
	store.baseDir = t.TempDir()
	return store
}

func TestStoreCRUDAndDependencyGraph(t *testing.T) {
	store := newTestStore(t, "session-a")
	first, err := store.Create("implement", "write the implementation", "implementing", map[string]any{"kind": "code"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Create("verify", "run verification", "verifying", nil)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != "1" || second.ID != "2" {
		t.Fatalf("ids = %q, %q", first.ID, second.ID)
	}
	if !store.AddBlockingEdge(first.ID, second.ID) {
		t.Fatal("expected dependency edge")
	}
	if store.AddBlockingEdge(second.ID, first.ID) {
		t.Fatal("cycle was accepted")
	}
	updated, fields, ok := store.Update(first.ID, map[string]any{"status": "in_progress", "owner": "worker"})
	if !ok || updated.Status != "in_progress" || updated.Owner != "worker" {
		t.Fatalf("update = %#v, %v", updated, ok)
	}
	sort.Strings(fields)
	if strings.Join(fields, ",") != "owner,status" {
		t.Fatalf("fields = %#v", fields)
	}
	loaded, ok := store.Get(second.ID)
	if !ok || len(loaded.BlockedBy) != 1 || loaded.BlockedBy[0] != first.ID {
		t.Fatalf("blocked task = %#v, %v", loaded, ok)
	}
	deleted, ok := store.Delete(first.ID)
	if !ok || deleted.ID != first.ID {
		t.Fatalf("delete = %#v, %v", deleted, ok)
	}
	loaded, ok = store.Get(second.ID)
	if !ok || len(loaded.BlockedBy) != 0 {
		t.Fatalf("dependency cleanup = %#v, %v", loaded, ok)
	}
}

func TestStoreConcurrentCreateUsesUniqueMonotonicIDs(t *testing.T) {
	store := newTestStore(t, "session-a")
	const count = 24
	ids := make(chan string, count)
	errs := make(chan error, count)
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			task, err := store.Create("subject", "description", "", nil)
			if err != nil {
				errs <- err
				return
			}
			ids <- task.ID
		}()
	}
	wg.Wait()
	close(errs)
	close(ids)
	for err := range errs {
		t.Fatal(err)
	}
	seen := make(map[string]bool, count)
	for id := range ids {
		if seen[id] {
			t.Fatalf("duplicate id %q", id)
		}
		seen[id] = true
	}
	if len(seen) != count || len(store.List()) != count {
		t.Fatalf("created=%d listed=%d", len(seen), len(store.List()))
	}
}

func TestStoreRejectsMalformedRowsAndPersistsCurrentShape(t *testing.T) {
	store := newTestStore(t, "session-a")
	created, err := store.Create("subject", "description", "active", nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(store.taskPath(created.ID))
	if err != nil {
		t.Fatal(err)
	}
	var row map[string]json.RawMessage
	if err := json.Unmarshal(data, &row); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"id", "subject", "description", "activeForm", "status", "blocks", "blockedBy"} {
		if _, ok := row[field]; !ok {
			t.Fatalf("missing field %q in %s", field, data)
		}
	}
	for _, dead := range []string{"comments", "createdAt", "updatedAt", "completedAt"} {
		if _, ok := row[dead]; ok {
			t.Fatalf("dead field %q persisted in %s", dead, data)
		}
	}
	malformed := filepath.Join(store.tasksDir(), "99.json")
	if err := os.WriteFile(malformed, []byte(`{"id":"99","subject":"bad"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.Get("99"); ok {
		t.Fatal("malformed row was returned")
	}
	if got := store.List(); len(got) != 1 || got[0].ID != created.ID {
		t.Fatalf("list = %#v", got)
	}
}

func TestStoreScopeAndPathComponentsUseCurrentGoRuneSemantics(t *testing.T) {
	listID := "team-😀"
	store := newTestStore(t, listID)
	if got := store.TaskListID(); got != listID {
		t.Fatalf("list id = %q", got)
	}
	if got := filepath.Base(store.tasksDir()); got != "team--" {
		t.Fatalf("directory = %q", got)
	}
	if got := sanitizeTaskPathComponent("a😀b"); got != "a-b" {
		t.Fatalf("component = %q", got)
	}
}

func TestStoreSubscriptionsAreMutationOnlyAndPanicIsolated(t *testing.T) {
	store := newTestStore(t, "session-a")
	var calls atomic.Int32
	unsubscribe := store.Subscribe(func() { calls.Add(1) })
	store.Subscribe(func() { panic("isolated") })
	if _, err := store.Create("subject", "description", "", nil); err != nil {
		t.Fatal(err)
	}
	store.Invalidate()
	if got := calls.Load(); got != 2 {
		t.Fatalf("calls = %d", got)
	}
	unsubscribe()
	store.Invalidate()
	if got := calls.Load(); got != 2 {
		t.Fatalf("unsubscribed calls = %d", got)
	}
}
