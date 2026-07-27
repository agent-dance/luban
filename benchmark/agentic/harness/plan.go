package harness

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// HashTaskInventory binds the dataset manifest hash to every task manifest,
// instruction, image name, and resolved immutable image digest.
func HashTaskInventory(inventory []Task) (string, error) {
	canonical := slices.Clone(inventory)
	slices.SortFunc(canonical, func(left, right Task) int { return strings.Compare(left.ID, right.ID) })
	for _, task := range canonical {
		if !idPattern.MatchString(task.ID) || !hex40Pattern.MatchString(task.BaseCommit) || !hex64Pattern.MatchString(task.ManifestSHA256) || !hex64Pattern.MatchString(task.InstructionSHA256) || !IsImageDigest(task.ImageDigest) {
			return "", fmt.Errorf("task %s has incomplete content-addressed identity", task.ID)
		}
	}
	// TaskInventoryHashAlgorithm is a published v1 wire identity. Never marshal
	// Task directly here: reordering fields in that general-purpose runtime
	// struct would otherwise silently change every frozen inventory digest.
	// This private projection preserves the registered v1 field order.
	type canonicalTaskV1 struct {
		ID                string `json:"id"`
		BaseCommit        string `json:"base_commit"`
		ManifestSHA256    string `json:"manifest_sha256"`
		Image             string `json:"image"`
		ImageDigest       string `json:"image_digest"`
		InstructionSHA256 string `json:"instruction_sha256"`
	}
	projected := make([]canonicalTaskV1, 0, len(canonical))
	for _, task := range canonical {
		projected = append(projected, canonicalTaskV1{
			ID: task.ID, BaseCommit: task.BaseCommit, ManifestSHA256: task.ManifestSHA256,
			Image: task.Image, ImageDigest: task.ImageDigest, InstructionSHA256: task.InstructionSHA256,
		})
	}
	encoded, err := json.Marshal(projected)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

// SelectTasks applies a stable SHA-256 ordering rather than math/rand so the
// same manifest produces the same pilot across Go versions and platforms.
func SelectTasks(selection SelectionSpec, inventory []Task) ([]Task, error) {
	partialExplicitCoverage := selection.Mode == "tasks" && len(inventory) == len(selection.TaskIDs)
	if len(inventory) != selection.ExpectedTaskCount && !partialExplicitCoverage {
		return nil, fmt.Errorf("dataset inventory has %d tasks; manifest requires %d", len(inventory), selection.ExpectedTaskCount)
	}
	byID := make(map[string]Task, len(inventory))
	for _, task := range inventory {
		if !idPattern.MatchString(task.ID) {
			return nil, fmt.Errorf("dataset contains invalid task ID %q", task.ID)
		}
		if _, exists := byID[task.ID]; exists {
			return nil, fmt.Errorf("dataset contains duplicate task ID %q", task.ID)
		}
		if !hex64Pattern.MatchString(task.ManifestSHA256) || !hex64Pattern.MatchString(task.InstructionSHA256) {
			return nil, fmt.Errorf("task %s lacks content-addressed manifests", task.ID)
		}
		if !hex40Pattern.MatchString(task.BaseCommit) {
			return nil, fmt.Errorf("task %s lacks a pinned base commit", task.ID)
		}
		if !IsImageDigest(task.ImageDigest) {
			return nil, fmt.Errorf("task %s image is not pinned by digest", task.ID)
		}
		byID[task.ID] = task
	}
	switch selection.Mode {
	case "full":
		selected := slices.Clone(inventory)
		slices.SortFunc(selected, func(left, right Task) int { return strings.Compare(left.ID, right.ID) })
		return selected, nil
	case "tasks":
		if partialExplicitCoverage {
			for _, id := range selection.TaskIDs {
				if _, exists := byID[id]; !exists {
					return nil, fmt.Errorf("partial inventory does not exactly cover selected task %s", id)
				}
			}
		}
		selected := make([]Task, 0, len(selection.TaskIDs))
		for _, id := range selection.TaskIDs {
			task, exists := byID[id]
			if !exists {
				return nil, fmt.Errorf("selected task %s is absent from dataset", id)
			}
			selected = append(selected, task)
		}
		return selected, nil
	case "sample":
		selected := slices.Clone(inventory)
		slices.SortFunc(selected, func(left, right Task) int {
			leftHash := stableHash(selection.SampleSeed, "sample", left.ID)
			rightHash := stableHash(selection.SampleSeed, "sample", right.ID)
			if comparison := strings.Compare(string(leftHash[:]), string(rightHash[:])); comparison != 0 {
				return comparison
			}
			return strings.Compare(left.ID, right.ID)
		})
		return selected[:selection.SampleCount], nil
	default:
		return nil, fmt.Errorf("unsupported selection mode %q", selection.Mode)
	}
}

// BuildPlan creates adjacent task-agent pairs. Task order and the order of
// agents inside each pair are independently randomized with stable hashes.
func BuildPlan(manifestSHA256 string, manifest Manifest, selected []Task) (RunPlan, error) {
	if !hex64Pattern.MatchString(manifestSHA256) {
		return RunPlan{}, fmt.Errorf("manifest SHA-256 is invalid")
	}
	if err := ValidateManifest(manifest); err != nil {
		return RunPlan{}, err
	}
	var entries []PlanEntry
	ordinal := 0
	for repetition := 0; repetition < manifest.Scheduling.Repetitions; repetition++ {
		tasks := slices.Clone(selected)
		slices.SortFunc(tasks, func(left, right Task) int {
			leftHash := stableHash(manifest.Scheduling.Seed, "task", fmt.Sprint(repetition), left.ID)
			rightHash := stableHash(manifest.Scheduling.Seed, "task", fmt.Sprint(repetition), right.ID)
			if comparison := strings.Compare(string(leftHash[:]), string(rightHash[:])); comparison != 0 {
				return comparison
			}
			return strings.Compare(left.ID, right.ID)
		})
		firstByTask := crossoverFirstAgents(manifest.Scheduling.Seed, repetition, selected, manifest.Agents)
		for _, task := range tasks {
			agents := slices.Clone(manifest.Agents)
			if agents[0].ID != firstByTask[task.ID] {
				agents[0], agents[1] = agents[1], agents[0]
			}
			pairID := fmt.Sprintf("r%03d-%s", repetition, task.ID)
			for _, agent := range agents {
				entries = append(entries, PlanEntry{Ordinal: ordinal, PairID: pairID, TaskID: task.ID, AgentID: agent.ID, Repetition: repetition})
				ordinal++
			}
		}
	}
	return RunPlan{SchemaVersion: "agentic-bench/plan-v1", ManifestSHA256: manifestSHA256, Entries: entries}, nil
}

// crossoverFirstAgents assigns an exactly balanced first-position block for
// every repetition. With an odd task count, adjacent repetitions invert which
// agent receives the single extra first slot; across four DeepSWE repetitions
// both agents therefore receive exactly 226 first positions.
func crossoverFirstAgents(seed uint64, repetition int, tasks []Task, agents []AgentSpec) map[string]string {
	orderedAgents := slices.Clone(agents)
	slices.SortFunc(orderedAgents, func(left, right AgentSpec) int { return strings.Compare(left.ID, right.ID) })
	block := repetition / 2
	slices.SortFunc(orderedAgents, func(left, right AgentSpec) int {
		leftHash := stableHash(seed, "crossover-agent", fmt.Sprint(block), left.ID)
		rightHash := stableHash(seed, "crossover-agent", fmt.Sprint(block), right.ID)
		if comparison := strings.Compare(string(leftHash[:]), string(rightHash[:])); comparison != 0 {
			return comparison
		}
		return strings.Compare(left.ID, right.ID)
	})
	favored := orderedAgents[repetition%2].ID
	other := orderedAgents[1-repetition%2].ID
	rankedTasks := slices.Clone(tasks)
	slices.SortFunc(rankedTasks, func(left, right Task) int {
		leftHash := stableHash(seed, "crossover-task", fmt.Sprint(repetition), left.ID)
		rightHash := stableHash(seed, "crossover-task", fmt.Sprint(repetition), right.ID)
		if comparison := strings.Compare(string(leftHash[:]), string(rightHash[:])); comparison != 0 {
			return comparison
		}
		return strings.Compare(left.ID, right.ID)
	})
	result := make(map[string]string, len(tasks))
	favoredCount := (len(rankedTasks) + 1) / 2
	for index, task := range rankedTasks {
		result[task.ID] = other
		if index < favoredCount {
			result[task.ID] = favored
		}
	}
	return result
}

func stableHash(seed uint64, parts ...string) [32]byte {
	hasher := sha256.New()
	var encodedSeed [8]byte
	binary.BigEndian.PutUint64(encodedSeed[:], seed)
	_, _ = hasher.Write(encodedSeed[:])
	for _, part := range parts {
		_, _ = hasher.Write([]byte{0})
		_, _ = hasher.Write([]byte(part))
	}
	var result [32]byte
	copy(result[:], hasher.Sum(nil))
	return result
}
