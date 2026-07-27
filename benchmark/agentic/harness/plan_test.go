package harness

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestHashTaskInventoryPreservesPublishedV1WireOrder(t *testing.T) {
	task := Task{
		ID: "task-a", BaseCommit: strings.Repeat("d", 40),
		ManifestSHA256: strings.Repeat("a", 64), InstructionSHA256: strings.Repeat("b", 64),
		Image: "registry.example/task:a", ImageDigest: "sha256:" + strings.Repeat("c", 64),
	}
	got, err := HashTaskInventory([]Task{task})
	if err != nil {
		t.Fatal(err)
	}
	wire := `[{"id":"task-a","base_commit":"` + strings.Repeat("d", 40) + `","manifest_sha256":"` + strings.Repeat("a", 64) + `","image":"registry.example/task:a","image_digest":"sha256:` + strings.Repeat("c", 64) + `","instruction_sha256":"` + strings.Repeat("b", 64) + `"}]`
	wantDigest := sha256.Sum256([]byte(wire))
	want := hex.EncodeToString(wantDigest[:])
	if got != want {
		t.Fatalf("task inventory digest = %s, want published v1 wire digest %s", got, want)
	}
}

func TestBuildPlanIsStablePairedAndRandomizesFirstAgent(t *testing.T) {
	manifest := fixtureManifest(t)
	manifest.Selection = SelectionSpec{Mode: "full", ExpectedTaskCount: 12}
	manifest.Scheduling.Repetitions = 4
	tasks := fixtureTasks(12)
	manifestSHA := strings.Repeat("f", 64)

	first, err := BuildPlan(manifestSHA, manifest, tasks)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildPlan(manifestSHA, manifest, tasks)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("identical seed produced different plans")
	}
	firstCounts := map[string]int{}
	for index := 0; index < len(first.Entries); index += 2 {
		left, right := first.Entries[index], first.Entries[index+1]
		if left.PairID != right.PairID || left.TaskID != right.TaskID || left.AgentID == right.AgentID {
			t.Fatalf("entries %d and %d are not an adjacent pair: %#v %#v", index, index+1, left, right)
		}
		firstCounts[left.AgentID]++
	}
	if firstCounts["codex"] == 0 || firstCounts["luban"] == 0 {
		t.Fatalf("one agent always ran first: %v", firstCounts)
	}
}

func TestBuildPlanUsesBalancedBlockedCrossover(t *testing.T) {
	manifest := fixtureManifest(t)
	manifest.Selection = SelectionSpec{Mode: "full", ExpectedTaskCount: 113}
	manifest.Scheduling.Repetitions = 4
	plan, err := BuildPlan(strings.Repeat("f", 64), manifest, fixtureNumberedTasks(113))
	if err != nil {
		t.Fatal(err)
	}
	perRepetition := make([]map[string]int, 4)
	total := map[string]int{}
	for index := range perRepetition {
		perRepetition[index] = map[string]int{}
	}
	for index := 0; index < len(plan.Entries); index += 2 {
		first := plan.Entries[index]
		perRepetition[first.Repetition][first.AgentID]++
		total[first.AgentID]++
	}
	for repetition, counts := range perRepetition {
		if !((counts["codex"] == 57 && counts["luban"] == 56) || (counts["codex"] == 56 && counts["luban"] == 57)) {
			t.Fatalf("repetition %d first-position counts = %v", repetition, counts)
		}
	}
	if total["codex"] != 226 || total["luban"] != 226 {
		t.Fatalf("four-run crossover totals = %v", total)
	}

	pilot := fixtureManifest(t)
	pilot.Selection = SelectionSpec{Mode: "full", ExpectedTaskCount: 5}
	pilot.Scheduling.Repetitions = 1
	pilotPlan, err := BuildPlan(strings.Repeat("e", 64), pilot, fixtureNumberedTasks(5))
	if err != nil {
		t.Fatal(err)
	}
	pilotFirst := map[string]int{}
	for index := 0; index < len(pilotPlan.Entries); index += 2 {
		pilotFirst[pilotPlan.Entries[index].AgentID]++
	}
	if !((pilotFirst["codex"] == 3 && pilotFirst["luban"] == 2) || (pilotFirst["codex"] == 2 && pilotFirst["luban"] == 3)) {
		t.Fatalf("pilot crossover counts = %v", pilotFirst)
	}
}

func TestSelectTasksStableSampleAndRequiresImageDigest(t *testing.T) {
	tasks := fixtureTasks(20)
	selection := SelectionSpec{Mode: "sample", SampleCount: 5, SampleSeed: 7, ExpectedTaskCount: 20}
	first, err := SelectTasks(selection, tasks)
	if err != nil {
		t.Fatal(err)
	}
	second, err := SelectTasks(selection, tasks)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) || len(first) != 5 {
		t.Fatalf("sample is not stable: %#v %#v", first, second)
	}
	tasks[0].ImageDigest = "latest"
	if _, err := SelectTasks(selection, tasks); err == nil {
		t.Fatal("mutable image reference was accepted")
	}
}

func TestSelectTasksAcceptsOnlyExactExplicitPartialCoverage(t *testing.T) {
	all := fixtureTasks(12)
	selection := SelectionSpec{Mode: "tasks", TaskIDs: []string{"task-d", "task-b"}, ExpectedTaskCount: 12}
	partial := []Task{all[1], all[3]}
	selected, err := SelectTasks(selection, partial)
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 2 || selected[0].ID != "task-d" || selected[1].ID != "task-b" {
		t.Fatalf("explicit order was not preserved: %#v", selected)
	}
	partial[1] = all[4]
	if _, err := SelectTasks(selection, partial); err == nil {
		t.Fatal("non-exact partial coverage was accepted")
	}
	if _, err := SelectTasks(SelectionSpec{Mode: "full", ExpectedTaskCount: 12}, all[:2]); err == nil {
		t.Fatal("partial coverage was accepted for a full run")
	}
}

func fixtureTasks(count int) []Task {
	tasks := make([]Task, 0, count)
	for index := 0; index < count; index++ {
		id := strings.ReplaceAll(strings.ToLower(string(rune('a'+index))), " ", "-")
		tasks = append(tasks, Task{
			ID:                "task-" + id,
			BaseCommit:        strings.Repeat("d", 40),
			ManifestSHA256:    strings.Repeat("a", 64),
			InstructionSHA256: strings.Repeat("b", 64),
			Image:             "registry.example/task:" + id,
			ImageDigest:       "sha256:" + strings.Repeat("c", 64),
		})
	}
	return tasks
}

func fixtureNumberedTasks(count int) []Task {
	tasks := make([]Task, 0, count)
	for index := 0; index < count; index++ {
		tasks = append(tasks, Task{
			ID: fmt.Sprintf("task-%03d", index), BaseCommit: strings.Repeat("d", 40),
			ManifestSHA256: strings.Repeat("a", 64), InstructionSHA256: strings.Repeat("b", 64),
			Image: fmt.Sprintf("registry.example/task:%03d", index), ImageDigest: "sha256:" + strings.Repeat("c", 64),
		})
	}
	return tasks
}
