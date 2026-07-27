package pierbackend

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agent-dance/luban/benchmark/agentic/harness"
)

func TestCheckedInFullReleaseLockHasExactFileAndInventoryIdentities(t *testing.T) {
	path := filepath.Join("..", "manifests", "deepswe-v1.1-release-full-inventory-lock-8cae5984-v2.json")
	fileSHA, err := harness.HashFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if fileSHA != "e23cb7c40f696e191122647295d24ef6a4c2e7d2df2dca359acfaebc05e28263" {
		t.Fatalf("full lock file SHA-256 = %s", fileSHA)
	}
	lock, err := loadInventoryLock(path)
	if err != nil {
		t.Fatal(err)
	}
	inventory := make([]harness.Task, 0, len(lock.Tasks))
	for _, task := range lock.Tasks {
		inventory = append(inventory, task.HarnessTask())
	}
	inventorySHA, err := harness.HashTaskInventory(inventory)
	if err != nil {
		t.Fatal(err)
	}
	if lock.Coverage != "full" || len(lock.Tasks) != 113 || lock.UniverseTaskCount != 113 || inventorySHA != "85f7f80eb0c48ea3480f95e145d13bacf5782c9aea1c576f79c65a14626d3a7a" {
		t.Fatalf("full release lock identity = coverage=%s tasks=%d universe=%d inventory=%s", lock.Coverage, len(lock.Tasks), lock.UniverseTaskCount, inventorySHA)
	}
}

func TestResolveImageDigestRetriesTransientRegistryFailure(t *testing.T) {
	t.Parallel()
	const digest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodHead || request.URL.Path != "/v2/example/image/manifests/v1" {
			t.Fatalf("unexpected registry request %s %s", request.Method, request.URL.Path)
		}
		if requests.Add(1) == 1 {
			writer.Header().Set("Retry-After", "0")
			writer.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		writer.Header().Set("Docker-Content-Digest", digest)
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	host := strings.TrimPrefix(server.URL, "https://")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	actual, err := resolveImageDigest(ctx, server.Client(), host+"/example/image:v1", filepath.Join(t.TempDir(), "registry-gate"))
	if err != nil {
		t.Fatal(err)
	}
	if actual != digest || requests.Load() != 2 {
		t.Fatalf("resolved %q after %d requests", actual, requests.Load())
	}
}

func TestRegistryRetryPolicy(t *testing.T) {
	t.Parallel()
	if !retryableRegistryStatus(http.StatusTooManyRequests) || !retryableRegistryStatus(http.StatusServiceUnavailable) {
		t.Fatal("expected transient registry statuses to be retryable")
	}
	if retryableRegistryStatus(http.StatusNotFound) || retryableRegistryStatus(http.StatusUnauthorized) {
		t.Fatal("permanent registry statuses must fail closed")
	}
	if delay := registryRetryDelay(0, "90"); delay != 60*time.Second {
		t.Fatalf("Retry-After cap = %s", delay)
	}
	if delay := registryRetryDelay(2, "invalid"); delay != 4*time.Second {
		t.Fatalf("exponential delay = %s", delay)
	}
}

func TestInventoryCoverageRejectsPartialFullRun(t *testing.T) {
	t.Parallel()
	lock := InventoryLock{
		SchemaVersion: InventorySchemaVersion, DatasetCommit: strings.Repeat("a", 40),
		Coverage: "tasks", UniverseTaskCount: 113, TaskIDs: []string{"a", "b"},
		Tasks: []LockedTask{{ID: "a"}, {ID: "b"}},
	}
	if err := validateInventoryLockStructure(lock); err != nil {
		t.Fatal(err)
	}
	if err := validateInventoryCoverage(lock, harness.SelectionSpec{Mode: "tasks", TaskIDs: []string{"b", "a"}, ExpectedTaskCount: 113}); err != nil {
		t.Fatal(err)
	}
	if err := validateInventoryCoverage(lock, harness.SelectionSpec{Mode: "full", ExpectedTaskCount: 113}); err == nil {
		t.Fatal("partial inventory was accepted for a full run")
	}
}
