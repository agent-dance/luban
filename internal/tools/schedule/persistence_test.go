package schedule

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestRepositoryUsesStrictCurrentSchema(t *testing.T) {
	repo := newTestRepository(t)
	createdAt := time.Date(2026, time.July, 25, 9, 30, 0, 0, time.UTC)
	job := testDurableJob(repo.root, createdAt)

	if _, err := repo.update(func(jobs []*storedJob) ([]*storedJob, bool, error) {
		return append(jobs, job), true, nil
	}); err != nil {
		t.Fatalf("write current schedule schema: %v", err)
	}

	data, err := os.ReadFile(repo.path)
	if err != nil {
		t.Fatalf("read schedule file: %v", err)
	}
	for _, field := range []string{`"schema_version"`, `"jobs"`, `"created_at"`} {
		if !bytes.Contains(data, []byte(field)) {
			t.Fatalf("current schema is missing %s: %s", field, data)
		}
	}
	for _, legacyField := range []string{`"tasks"`, `"createdAt"`, `"permanent"`} {
		if bytes.Contains(data, []byte(legacyField)) {
			t.Fatalf("current schema unexpectedly contains legacy field %s: %s", legacyField, data)
		}
	}

	jobs, err := repo.read()
	if err != nil {
		t.Fatalf("read current schedule schema: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ID != job.ID || jobs[0].ProjectRoot != repo.root {
		t.Fatalf("unexpected current-schema round trip: %#v", jobs)
	}
}

func TestRepositoryRejectsLegacySchemasWithoutOverwriting(t *testing.T) {
	validJobPrefix := `{"schema_version":1,"jobs":[{"id":"legacy","expression":"* * * * *","prompt":"prompt","recurring":false,`
	tests := map[string]string{
		"tasks":     `{"schema_version":1,"tasks":[]}`,
		"createdAt": validJobPrefix + `"createdAt":"2026-07-25T09:30:00Z"}]}`,
		"permanent": validJobPrefix + `"created_at":"2026-07-25T09:30:00Z","permanent":true}]}`,
	}

	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			repo := newTestRepository(t)
			writePrivateTestFile(t, repo.path, []byte(body))
			before := mustReadTestFile(t, repo.path)
			callbackCalled := false

			_, err := repo.update(func(jobs []*storedJob) ([]*storedJob, bool, error) {
				callbackCalled = true
				return jobs, true, nil
			})
			if err == nil {
				t.Fatal("legacy schema unexpectedly accepted")
			}
			if callbackCalled {
				t.Fatal("update callback ran after strict schema rejection")
			}
			after := mustReadTestFile(t, repo.path)
			if !bytes.Equal(after, before) {
				t.Fatalf("rejected legacy file was overwritten:\nbefore=%s\nafter=%s", before, after)
			}
		})
	}
}

func TestRepositoryCreatesPrivateDirectoryAndFiles(t *testing.T) {
	repo := newTestRepository(t)
	job := testDurableJob(repo.root, time.Date(2026, time.July, 25, 9, 30, 0, 0, time.UTC))
	if _, err := repo.update(func(jobs []*storedJob) ([]*storedJob, bool, error) {
		return append(jobs, job), true, nil
	}); err != nil {
		t.Fatalf("write private schedule store: %v", err)
	}

	assertSchedulePermissions(t, repo.dir, 0o700)
	assertSchedulePermissions(t, repo.path, 0o600)
	assertSchedulePermissions(t, repo.lockPath, 0o600)
}

func TestRepositoryRejectsSymlinkWithoutTouchingTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation commonly requires additional privileges on Windows")
	}
	repo := newTestRepository(t)
	target := filepath.Join(t.TempDir(), "target.json")
	original := []byte(`{"sentinel":"unchanged"}`)
	writePrivateTestFile(t, target, original)
	if err := os.MkdirAll(repo.dir, 0o700); err != nil {
		t.Fatalf("create repository directory: %v", err)
	}
	if err := os.Symlink(target, repo.path); err != nil {
		t.Fatalf("create schedule symlink: %v", err)
	}

	if _, err := repo.read(); err == nil {
		t.Fatal("symlinked schedule store unexpectedly accepted")
	}
	if _, err := repo.update(func(jobs []*storedJob) ([]*storedJob, bool, error) {
		return jobs, true, nil
	}); err == nil {
		t.Fatal("symlinked schedule store unexpectedly updated")
	}
	if got := mustReadTestFile(t, target); !bytes.Equal(got, original) {
		t.Fatalf("symlink target was modified: got %q, want %q", got, original)
	}
}

func TestRepositoryRejectsHardlinkWithoutTouchingSource(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("hardlink security contract is covered by platform-specific secure I/O tests")
	}
	repo := newTestRepository(t)
	source := filepath.Join(t.TempDir(), "source.json")
	original := []byte(`{"schema_version":1,"jobs":[]}`)
	writePrivateTestFile(t, source, original)
	if err := os.MkdirAll(repo.dir, 0o700); err != nil {
		t.Fatalf("create repository directory: %v", err)
	}
	if err := os.Link(source, repo.path); err != nil {
		if errors.Is(err, fs.ErrPermission) {
			t.Skipf("hardlinks unavailable: %v", err)
		}
		t.Fatalf("create schedule hardlink: %v", err)
	}

	if _, err := repo.read(); err == nil {
		t.Fatal("hardlinked schedule store unexpectedly accepted")
	}
	if _, err := repo.update(func(jobs []*storedJob) ([]*storedJob, bool, error) {
		return jobs, true, nil
	}); err == nil {
		t.Fatal("hardlinked schedule store unexpectedly updated")
	}
	if got := mustReadTestFile(t, source); !bytes.Equal(got, original) {
		t.Fatalf("hardlink source was modified: got %q, want %q", got, original)
	}
}

func newTestRepository(t *testing.T) *repository {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	project := t.TempDir()
	repo, err := newRepository(project)
	if err != nil {
		t.Fatalf("new repository: %v", err)
	}
	if strings.HasPrefix(filepath.Clean(repo.dir), filepath.Clean(project)+string(filepath.Separator)) {
		t.Fatalf("schedule repository remained inside project: %q", repo.dir)
	}
	return repo
}

func testDurableJob(root string, createdAt time.Time) *storedJob {
	return &storedJob{Job: Job{
		ID:          "job-0001",
		Expression:  "* * * * *",
		Prompt:      "scheduled prompt",
		Recurring:   true,
		Durable:     true,
		CreatedAt:   createdAt.UTC(),
		ProjectRoot: root,
	}}
}

func writePrivateTestFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("create private parent: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write private test file: %v", err)
	}
}

func mustReadTestFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

func assertSchedulePermissions(t *testing.T, path string, want fs.FileMode) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("permissions for %s = %04o, want %04o", path, got, want)
	}
}

func TestPersistedFileDoesNotAdmitCamelCaseByCaseFolding(t *testing.T) {
	// encoding/json matches field names case-insensitively. The explicit
	// underscore makes the old camelCase name distinct from created_at.
	repo := newTestRepository(t)
	body := strings.ReplaceAll(
		`{"schema_version":1,"jobs":[{"id":"job","expression":"* * * * *","prompt":"prompt","recurring":false,"created_at":"2026-07-25T09:30:00Z"}]}`,
		"created_at", "createdAt",
	)
	writePrivateTestFile(t, repo.path, []byte(body))
	if _, err := repo.read(); err == nil {
		t.Fatal("camelCase historical field unexpectedly accepted")
	}
}
