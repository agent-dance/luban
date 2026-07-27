package localbench

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/agent-dance/luban/i18n"
)

const (
	currentCodexBaselineJSON = "codex-baseline.json"
	currentCodexBaselineHTML = "codex-baseline.html"
)

type loadedCodexBaseline struct {
	snapshot     CodexBaselineSnapshot
	snapshotPath string
	sha256       string
	reference    CodexBaselineReference
}

func absoluteResultsRoot(value string) (string, error) {
	if value == "" {
		value = "benchmark-results"
	}
	return filepath.Abs(value)
}

func loadCodexBaseline(resultsRoot string, tasks []TaskSelection) (*loadedCodexBaseline, error) {
	currentPath := filepath.Join(resultsRoot, currentCodexBaselineJSON)
	raw, err := os.ReadFile(currentPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, i18n.NewError(i18n.KeyLocalBenchmarkBaselineRequired)
	}
	if err != nil {
		return nil, i18n.WrapInternalError(i18n.KeyLocalBenchmarkBaselineIncompatible, err)
	}
	var snapshot CodexBaselineSnapshot
	if err := decodeStrictJSON(raw, &snapshot); err != nil {
		return nil, i18n.WrapInternalError(i18n.KeyLocalBenchmarkBaselineIncompatible, err)
	}
	if err := validateCodexBaseline(snapshot, tasks); err != nil {
		return nil, i18n.WrapInternalError(i18n.KeyLocalBenchmarkBaselineIncompatible, err)
	}
	sourceRunPath, ok := secureRelativePath(snapshot.SourceRunPath)
	if !ok {
		return nil, i18n.NewError(i18n.KeyLocalBenchmarkBaselineIncompatible)
	}
	immutablePath := filepath.Join(resultsRoot, sourceRunPath, currentCodexBaselineJSON)
	immutableRaw, err := os.ReadFile(immutablePath)
	if err != nil || !bytes.Equal(raw, immutableRaw) {
		return nil, i18n.WrapInternalError(i18n.KeyLocalBenchmarkBaselineIncompatible, err)
	}
	digest := sha256.Sum256(raw)
	sha := hex.EncodeToString(digest[:])
	return &loadedCodexBaseline{
		snapshot: snapshot, snapshotPath: immutablePath, sha256: sha,
		reference: CodexBaselineReference{
			SnapshotPath:   filepath.ToSlash(filepath.Join(snapshot.SourceRunPath, currentCodexBaselineJSON)),
			SnapshotSHA256: sha, SourceRunID: snapshot.SourceRunID, CapturedAt: snapshot.CapturedAt,
			CodexVersion: snapshot.Binary.Version,
		},
	}, nil
}

func validateCodexBaseline(snapshot CodexBaselineSnapshot, tasks []TaskSelection) error {
	if snapshot.SchemaVersion != CodexBaselineSchemaVersion || snapshot.SourceRunID == "" || snapshot.CapturedAt.IsZero() ||
		snapshot.Dataset != DatasetName || snapshot.DatasetRevision != DatasetRevision || snapshot.SelectionPolicy != selectionPolicy ||
		snapshot.Model != ModelID || snapshot.ReasoningEffort != ReasoningEffort || len(snapshot.GatewaySHA256) != 64 ||
		snapshot.Binary.Name != "codex" || strings.TrimSpace(snapshot.Binary.Version) == "" || !validSHA256(snapshot.Binary.SHA256) ||
		!validSHA256(snapshot.GatewaySHA256) || !reflect.DeepEqual(snapshot.Pricing, FrozenPricing()) || len(snapshot.Tasks) < len(tasks) || len(snapshot.Tasks) > CatalogSize() ||
		len(snapshot.Runs) != len(snapshot.Tasks) || len(snapshot.Evaluations) != len(snapshot.Tasks) || len(snapshot.GoldEvaluations) != len(snapshot.Tasks) {
		return errors.New("codex baseline identity is invalid")
	}
	expectedTasks, err := loadSelection(len(snapshot.Tasks))
	if err != nil || !reflect.DeepEqual(snapshot.Tasks, expectedTasks) {
		return errors.New("codex baseline task selection is invalid")
	}
	for index, task := range tasks {
		if snapshot.Tasks[index] != task || !hasRun(snapshot.Runs, task.InstanceID, "codex") ||
			!hasEvaluation(snapshot.Evaluations, task.InstanceID, "codex") || !resolvedEvaluation(snapshot.GoldEvaluations, task.InstanceID, "gold") {
			return errors.New("codex baseline coverage is insufficient")
		}
	}
	for _, run := range snapshot.Runs {
		if run.Agent != "codex" || run.Model != ModelID || run.ReasoningEffort != ReasoningEffort || run.Binary.SHA256 != snapshot.Binary.SHA256 {
			return errors.New("codex baseline run identity is invalid")
		}
		if _, ok := secureRelativePath(run.EvidenceRoot); !ok {
			return errors.New("codex baseline run evidence path is invalid")
		}
	}
	for _, evaluation := range append(append([]Evaluation(nil), snapshot.Evaluations...), snapshot.GoldEvaluations...) {
		if _, ok := secureRelativePath(evaluation.EvidenceRoot); !ok {
			return errors.New("codex baseline evaluation evidence path is invalid")
		}
	}
	return nil
}

func baselineReference(runRoot string, baseline *loadedCodexBaseline, refreshed bool) CodexBaselineReference {
	result := baseline.reference
	result.SnapshotPath = relativeLink(runRoot, baseline.snapshotPath)
	result.Refreshed = refreshed
	return result
}

func seedCodexBaseline(result *BenchmarkResult, runRoot string, baseline *loadedCodexBaseline) {
	sourceRoot := filepath.Dir(baseline.snapshotPath)
	selected := make(map[string]struct{}, len(result.Tasks))
	for _, task := range result.Tasks {
		selected[task.InstanceID] = struct{}{}
	}
	for _, run := range baseline.snapshot.Runs {
		if run.Agent != "codex" {
			continue
		}
		if _, ok := selected[run.InstanceID]; !ok {
			continue
		}
		run.EvidenceRoot = rebaseEvidenceRoot(runRoot, sourceRoot, run.EvidenceRoot)
		result.Runs = append(result.Runs, run)
	}
	for _, evaluation := range baseline.snapshot.Evaluations {
		if evaluation.Agent != "codex" {
			continue
		}
		if _, ok := selected[evaluation.InstanceID]; !ok {
			continue
		}
		evaluation.EvidenceRoot = rebaseEvidenceRoot(runRoot, sourceRoot, evaluation.EvidenceRoot)
		result.Evaluations = append(result.Evaluations, evaluation)
	}
}

func rebaseEvidenceRoot(destinationRoot, sourceRoot, evidenceRoot string) string {
	clean, ok := secureRelativePath(evidenceRoot)
	if !ok {
		return ""
	}
	return relativeLink(destinationRoot, filepath.Join(sourceRoot, clean))
}

func freezeCodexBaseline(result BenchmarkResult, resultsRoot, runRoot string, language i18n.Language) (CodexBaselineReference, error) {
	sourceRunPath, err := filepath.Rel(resultsRoot, runRoot)
	if err != nil {
		return CodexBaselineReference{}, err
	}
	identity, ok := binaryIdentity(result.Binaries, "codex")
	if !ok || strings.TrimSpace(identity.Version) == "" {
		return CodexBaselineReference{}, errors.New("codex binary version is unavailable")
	}
	captured := result.StartedAt
	if result.CompletedAt != nil {
		captured = *result.CompletedAt
	}
	snapshot := CodexBaselineSnapshot{
		SchemaVersion: CodexBaselineSchemaVersion, SourceRunID: result.RunID,
		SourceRunPath: filepath.ToSlash(sourceRunPath), CapturedAt: captured,
		Dataset: result.Dataset, DatasetRevision: result.DatasetRevision, SelectionPolicy: result.SelectionPolicy,
		Tasks: append([]TaskSelection(nil), result.Tasks...), Model: result.Model, ReasoningEffort: result.ReasoningEffort,
		GatewaySHA256: result.GatewaySHA256, EvaluatorEngine: result.EvaluatorEngine,
		AgentTimeout: result.AgentTimeout, EvaluatorTimeout: result.EvaluatorTimeout,
		Pricing: result.Pricing, Binary: identity,
		Runs: filterRuns(result.Runs, "codex"), Evaluations: filterEvaluations(result.Evaluations, "codex"),
		GoldEvaluations: append([]Evaluation(nil), result.GoldEvaluations...),
		Aggregate:       aggregateSubset("codex", result.Tasks, result.Runs, result.Evaluations, nil),
	}
	immutablePath := filepath.Join(runRoot, currentCodexBaselineJSON)
	if err := writeJSONAtomic(immutablePath, snapshot); err != nil {
		return CodexBaselineReference{}, err
	}
	immutableRaw, err := os.ReadFile(immutablePath)
	if err != nil {
		return CodexBaselineReference{}, err
	}
	digest := sha256.Sum256(immutableRaw)
	sha := hex.EncodeToString(digest[:])
	if err := GenerateCodexReport(snapshot, resultsRoot, filepath.Join(runRoot, "codex-report.html"), language); err != nil {
		return CodexBaselineReference{}, err
	}
	if err := GenerateCodexReport(snapshot, resultsRoot, filepath.Join(resultsRoot, currentCodexBaselineHTML), language); err != nil {
		return CodexBaselineReference{}, err
	}
	// The root JSON is the authoritative pointer. Publish it last so a failed
	// refresh never replaces the previous complete baseline.
	if err := writeBytesAtomic(filepath.Join(resultsRoot, currentCodexBaselineJSON), immutableRaw, 0o644); err != nil {
		return CodexBaselineReference{}, err
	}
	return CodexBaselineReference{
		SnapshotPath: currentCodexBaselineJSON, SnapshotSHA256: sha,
		SourceRunID: snapshot.SourceRunID, CapturedAt: snapshot.CapturedAt,
		CodexVersion: snapshot.Binary.Version, Refreshed: true,
	}, nil
}

func codexBaselineCoverage(result BenchmarkResult) bool {
	for _, task := range result.Tasks {
		if !hasRun(result.Runs, task.InstanceID, "codex") || !hasEvaluation(result.Evaluations, task.InstanceID, "codex") ||
			!resolvedEvaluation(result.GoldEvaluations, task.InstanceID, "gold") {
			return false
		}
	}
	return true
}

func hasEvaluation(values []Evaluation, taskID, agent string) bool {
	for _, value := range values {
		if value.InstanceID == taskID && value.Agent == agent {
			return true
		}
	}
	return false
}

func resolvedEvaluation(values []Evaluation, taskID, agent string) bool {
	for _, value := range values {
		if value.InstanceID == taskID && value.Agent == agent {
			return value.Resolved
		}
	}
	return false
}

func binaryIdentity(values []BinaryIdentity, name string) (BinaryIdentity, bool) {
	for _, value := range values {
		if value.Name == name {
			return value, true
		}
	}
	return BinaryIdentity{}, false
}

func filterRuns(values []RunSummary, agent string) []RunSummary {
	result := make([]RunSummary, 0, len(values))
	for _, value := range values {
		if value.Agent == agent {
			result = append(result, value)
		}
	}
	return result
}

func filterEvaluations(values []Evaluation, agent string) []Evaluation {
	result := make([]Evaluation, 0, len(values))
	for _, value := range values {
		if value.Agent == agent {
			result = append(result, value)
		}
	}
	return result
}

func secureRelativePath(value string) (string, bool) {
	if strings.TrimSpace(value) == "" || filepath.IsAbs(value) {
		return "", false
	}
	clean := filepath.Clean(filepath.FromSlash(value))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", false
	}
	return clean, true
}

func gatewaySHA256(origin string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(origin)))
	return hex.EncodeToString(digest[:])
}

func validSHA256(value string) bool {
	raw, err := hex.DecodeString(value)
	return err == nil && len(raw) == sha256.Size
}

func decodeStrictJSON(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("JSON contains trailing content")
	}
	return nil
}

func writeBytesAtomic(path string, raw []byte, mode os.FileMode) error {
	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(parent, ".benchmark-bytes-*.tmp")
	if err != nil {
		return err
	}
	name := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(name)
		}
	}()
	if _, err := temporary.Write(raw); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Chmod(name, mode); err != nil {
		return err
	}
	if err := os.Rename(name, path); err != nil {
		return err
	}
	committed = true
	return nil
}
