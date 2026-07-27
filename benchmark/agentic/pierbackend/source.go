package pierbackend

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/agent-dance/luban/benchmark/agentic/harness"
)

type networkPolicyAttestation struct {
	SchemaVersion       string `json:"schema_version"`
	TaskCount           int    `json:"task_count"`
	AgentNetworkDeny    bool   `json:"agent_network_deny"`
	VerifierNetworkDeny bool   `json:"verifier_network_deny"`
	ParserModuleSHA256  string `json:"parser_module_sha256"`
}

func attestPierNetworkPolicy(ctx context.Context, config Config, manifest harness.Manifest, tasks []LockedTask) (networkPolicyAttestation, error) {
	python := filepath.Join(filepath.Dir(config.PierBinary), "python")
	if info, err := os.Stat(python); err != nil || !info.Mode().IsRegular() {
		return networkPolicyAttestation{}, errors.New("Pier runtime Python executable is unavailable beside the frozen Pier binary")
	}
	parserModule := filepath.Join(config.EvaluatorRepositoryRoot, "src", "pier", "models", "task", "config.py")
	parserSHA, err := harness.HashFile(parserModule)
	if err != nil {
		return networkPolicyAttestation{}, err
	}
	script := strings.Join([]string{
		"import json,pathlib,sys",
		"from pier.models.task.config import TaskConfig",
		"paths=[pathlib.Path(p) for p in sys.argv[1:]]",
		"cfgs=[TaskConfig.model_validate_toml(p.read_text(encoding='utf-8')) for p in paths]",
		"print(json.dumps({'schema_version':'agentic-bench/pier-network-attestation-v1','task_count':len(cfgs),'agent_network_deny':all(not c.environment.allow_internet for c in cfgs),'verifier_network_deny':all(not c.verifier.environment.allow_internet for c in cfgs)}))",
	}, ";")
	args := []string{"-B", "-I", "-c", script}
	for _, task := range tasks {
		directory, pathErr := pathWithin(filepath.Join(config.DatasetRepositoryRoot, manifest.Dataset.Root), task.RelativePath)
		if pathErr != nil {
			return networkPolicyAttestation{}, pathErr
		}
		args = append(args, filepath.Join(directory, "task.toml"))
	}
	environment := sanitizedProcessEnvironment(nil, config.ProviderCredentialEnv)
	pythonPath := filepath.Join(config.EvaluatorRepositoryRoot, "src")
	// -I ignores PYTHONPATH by design, so inject the pinned source root into
	// the script's import path without consulting user or site configuration.
	script = "import sys;sys.path.insert(0," + strconv.Quote(pythonPath) + ");" + script
	args[3] = script
	output, err := runOutput(ctx, python, args, environment, config.EvaluatorRepositoryRoot)
	if err != nil {
		return networkPolicyAttestation{}, fmt.Errorf("attest effective Pier network policy: %w", err)
	}
	var attestation networkPolicyAttestation
	if err := json.Unmarshal(output, &attestation); err != nil {
		return networkPolicyAttestation{}, fmt.Errorf("decode effective Pier network policy: %w", err)
	}
	attestation.ParserModuleSHA256 = parserSHA
	if attestation.SchemaVersion != "agentic-bench/pier-network-attestation-v1" || attestation.TaskCount != len(tasks) || !attestation.AgentNetworkDeny || !attestation.VerifierNetworkDeny {
		return networkPolicyAttestation{}, errors.New("Pier runtime parser does not enforce agent and verifier network denial for every task")
	}
	return attestation, nil
}

func loadInventoryLock(path string) (InventoryLock, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return InventoryLock{}, err
	}
	return decodeInventoryLock(raw)
}

func decodeInventoryLock(raw []byte) (InventoryLock, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var lock InventoryLock
	if err := decoder.Decode(&lock); err != nil {
		return InventoryLock{}, fmt.Errorf("decode Pier inventory lock: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return InventoryLock{}, errors.New("Pier inventory lock contains trailing JSON")
	}
	if lock.SchemaVersion != InventorySchemaVersion || len(lock.Tasks) == 0 {
		return InventoryLock{}, errors.New("Pier inventory lock has an unsupported schema or no tasks")
	}
	if err := validateInventoryLockStructure(lock); err != nil {
		return InventoryLock{}, err
	}
	return lock, nil
}

func validateInventoryLockStructure(lock InventoryLock) error {
	if len(lock.DatasetCommit) != 40 || !abbreviatedCommitPattern.MatchString(lock.DatasetCommit) || lock.UniverseTaskCount < 1 {
		return errors.New("Pier inventory lock has an invalid dataset identity")
	}
	if !slices.IsSortedFunc(lock.Tasks, func(left, right LockedTask) int { return strings.Compare(left.ID, right.ID) }) {
		return errors.New("Pier inventory lock tasks are not in canonical order")
	}
	switch lock.Coverage {
	case "full":
		if len(lock.Tasks) != lock.UniverseTaskCount || len(lock.TaskIDs) != 0 {
			return errors.New("full Pier inventory lock does not cover its declared universe")
		}
	case "tasks":
		if len(lock.Tasks) >= lock.UniverseTaskCount || len(lock.TaskIDs) != len(lock.Tasks) {
			return errors.New("partial Pier inventory lock has invalid coverage cardinality")
		}
		for index, task := range lock.Tasks {
			if lock.TaskIDs[index] != task.ID || (index > 0 && strings.Compare(lock.TaskIDs[index-1], lock.TaskIDs[index]) >= 0) {
				return errors.New("partial Pier inventory lock task IDs are not exact and canonical")
			}
		}
	default:
		return errors.New("Pier inventory lock has an unsupported coverage mode")
	}
	return nil
}

func validateInventoryCoverage(lock InventoryLock, selection harness.SelectionSpec) error {
	if lock.UniverseTaskCount != selection.ExpectedTaskCount {
		return errors.New("Pier inventory lock is bound to a different task universe")
	}
	switch selection.Mode {
	case "full", "sample":
		if lock.Coverage != "full" {
			return errors.New("full or sampled runs require a full-universe inventory lock")
		}
	case "tasks":
		if lock.Coverage != "tasks" {
			return errors.New("explicit pilot runs require an exact partial inventory lock")
		}
		expected := slices.Clone(selection.TaskIDs)
		slices.Sort(expected)
		if !slices.Equal(expected, lock.TaskIDs) {
			return errors.New("partial inventory lock does not exactly match the preregistered pilot")
		}
	default:
		return errors.New("unsupported selection mode for Pier inventory coverage")
	}
	return nil
}

type taskTOML map[string]string

func parseTaskTOML(path string) (taskTOML, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	values := taskTOML{}
	section := ""
	scanner := bufio.NewScanner(file)
	line := 0
	for scanner.Scan() {
		line++
		text := strings.TrimSpace(stripTOMLComment(scanner.Text()))
		if text == "" {
			continue
		}
		if strings.HasPrefix(text, "[") && strings.HasSuffix(text, "]") {
			section = strings.TrimSpace(text[1 : len(text)-1])
			continue
		}
		key, value, ok := strings.Cut(text, "=")
		if !ok {
			return nil, fmt.Errorf("task.toml line %d is not a key/value", line)
		}
		key = strings.TrimSpace(key)
		if section != "" {
			key = section + "." + key
		}
		values[key] = strings.Trim(strings.TrimSpace(value), "\"")
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return values, nil
}

func stripTOMLComment(value string) string {
	quoted := false
	escaped := false
	for index, char := range value {
		if escaped {
			escaped = false
			continue
		}
		if char == '\\' && quoted {
			escaped = true
			continue
		}
		if char == '"' {
			quoted = !quoted
			continue
		}
		if char == '#' && !quoted {
			return value[:index]
		}
	}
	return value
}

func validateLockedTask(repositoryRoot, taskRoot string, locked LockedTask, manifest harness.Manifest) error {
	if locked.ID == "" || filepath.Clean(locked.RelativePath) != locked.RelativePath || filepath.IsAbs(locked.RelativePath) || strings.HasPrefix(locked.RelativePath, "..") {
		return fmt.Errorf("task %q has an invalid relative path", locked.ID)
	}
	directory, err := pathWithin(filepath.Join(repositoryRoot, taskRoot), locked.RelativePath)
	if err != nil {
		return err
	}
	manifestPath := filepath.Join(directory, "task.toml")
	instructionPath := filepath.Join(directory, "instruction.md")
	manifestSHA, err := harness.HashFile(manifestPath)
	if err != nil {
		return err
	}
	instructionSHA, err := harness.HashFile(instructionPath)
	if err != nil {
		return err
	}
	if manifestSHA != locked.ManifestSHA256 || instructionSHA != locked.InstructionSHA256 || !harness.IsImageDigest(locked.ImageDigest) {
		return fmt.Errorf("task %s content or image digest differs from its lock", locked.ID)
	}
	values, err := parseTaskTOML(manifestPath)
	if err != nil {
		return err
	}
	if !validLockedBaseCommit(values["metadata.base_commit_hash"], locked.BaseCommit) {
		return fmt.Errorf("task %s base commit is not bound to the lock's full SHA", locked.ID)
	}
	expected := map[string]string{
		"schema_version":                         "1.1",
		"metadata.task_id":                       locked.ID,
		"environment.docker_image":               locked.Image,
		"environment.allow_internet":             "false",
		"verifier.environment.allow_internet":    "false",
		"verifier.environment_mode":              "separate",
		"agent.timeout_sec":                      strconv.Itoa(manifest.Timeouts.AgentSeconds) + ".0",
		"verifier.timeout_sec":                   strconv.Itoa(manifest.Timeouts.VerifierSeconds) + ".0",
		"environment.build_timeout_sec":          strconv.Itoa(manifest.Timeouts.SetupSeconds) + ".0",
		"environment.cpus":                       strconv.Itoa(manifest.Resources.CPUs),
		"environment.memory_mb":                  strconv.Itoa(manifest.Resources.MemoryMB),
		"environment.storage_mb":                 strconv.Itoa(manifest.Resources.StorageMB),
		"environment.gpus":                       strconv.Itoa(manifest.Resources.GPUs),
		"verifier.environment.cpus":              strconv.Itoa(manifest.Resources.CPUs),
		"verifier.environment.memory_mb":         strconv.Itoa(manifest.Resources.MemoryMB),
		"verifier.environment.storage_mb":        strconv.Itoa(manifest.Resources.StorageMB),
		"verifier.environment.build_timeout_sec": strconv.Itoa(manifest.Timeouts.SetupSeconds) + ".0",
	}
	for key, want := range expected {
		got := values[key]
		if (strings.HasSuffix(key, "timeout_sec") && numericEqual(got, want)) || got == want {
			continue
		}
		return fmt.Errorf("task %s has %s=%q, expected %q", locked.ID, key, got, want)
	}
	return nil
}

func validLockedBaseCommit(source, locked string) bool {
	return abbreviatedCommitPattern.MatchString(source) &&
		len(locked) == 40 && abbreviatedCommitPattern.MatchString(locked) &&
		strings.HasPrefix(locked, source)
}

func numericEqual(left, right string) bool {
	a, errA := strconv.ParseFloat(left, 64)
	b, errB := strconv.ParseFloat(right, 64)
	return errA == nil && errB == nil && a == b
}

func materializeTask(source, destination, image, digest string) (harness.TreeInventory, error) {
	if err := copyRegularTree(source, destination); err != nil {
		return harness.TreeInventory{}, err
	}
	manifestPath := filepath.Join(destination, "task.toml")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return harness.TreeInventory{}, err
	}
	oldLine := "docker_image = " + strconv.Quote(image)
	newLine := "docker_image = " + strconv.Quote(image+"@"+digest)
	if bytes.Count(raw, []byte(oldLine)) != 1 {
		return harness.TreeInventory{}, errors.New("task image line is not uniquely rewritable")
	}
	raw = bytes.Replace(raw, []byte(oldLine), []byte(newLine), 1)
	if err := os.WriteFile(manifestPath, raw, 0o644); err != nil {
		return harness.TreeInventory{}, err
	}
	return harness.HashTree(destination)
}

func copyRegularTree(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("task contains unsupported file type: %s", relative)
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
		if err != nil {
			input.Close()
			return err
		}
		_, copyErr := io.Copy(output, input)
		inputCloseErr := input.Close()
		closeErr := output.Close()
		if copyErr != nil {
			return copyErr
		}
		if inputCloseErr != nil {
			return inputCloseErr
		}
		return closeErr
	})
}

func pathWithin(root, relative string) (string, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	path, err := filepath.Abs(filepath.Join(root, relative))
	if err != nil {
		return "", err
	}
	if path != root && !strings.HasPrefix(path, root+string(filepath.Separator)) {
		return "", errors.New("path escapes pinned root")
	}
	return path, nil
}
