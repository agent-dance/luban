package pierbackend

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidLockedBaseCommitExpandsOnlyMatchingLowercaseSHA(t *testing.T) {
	full := "68dafce012345678901234567890123456789012"
	for _, source := range []string{"68dafce", full} {
		if !validLockedBaseCommit(source, full) {
			t.Fatalf("valid source %q did not bind to %q", source, full)
		}
	}
	for _, source := range []string{
		"68dafc", "68DAFCE", "78dafce", full + "0", "../../commit",
	} {
		if validLockedBaseCommit(source, full) {
			t.Fatalf("invalid source %q bound to %q", source, full)
		}
	}
	if validLockedBaseCommit("68dafce", "68dafce") {
		t.Fatal("abbreviated lock commit was accepted")
	}
}

func TestMaterializeTaskDoesNotOverrideOfficialArtifactHook(t *testing.T) {
	source := t.TempDir()
	destination := filepath.Join(t.TempDir(), "task")
	const image = "registry.example/task:1"
	const digest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const hook = "#!/bin/bash\nset -eu\ngit diff --binary BASE HEAD > /logs/artifacts/model.patch\n"
	if err := os.WriteFile(filepath.Join(source, "task.toml"), []byte("docker_image = \""+image+"\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "pre_artifacts.sh"), []byte(hook), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := materializeTask(source, destination, image, digest); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(destination, "pre_artifacts.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != hook {
		t.Fatalf("official pre_artifacts hook was changed:\n%s", got)
	}
}
