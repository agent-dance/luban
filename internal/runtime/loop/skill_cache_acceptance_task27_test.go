package loop

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agent-dance/luban/internal/contracts/stream"
	"github.com/agent-dance/luban/internal/runtime/compact"
	"github.com/agent-dance/luban/prompt"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/skills"
	"github.com/agent-dance/luban/types"
)

const task27CacheabilityEvidenceScope = "local cacheability/request-shape evidence only; not provider cache-hit or billing evidence"

// TestSkillCacheCatalogLifecyclePreservesSerializedPrefix proves the local
// exact-prefix property that provider prompt caches can exploit. It deliberately
// does not claim that a provider accepted, cached, billed, or hit this prefix.
func TestSkillCacheCatalogLifecyclePreservesSerializedPrefix(t *testing.T) {
	manager, originalSkillFile := task19SkillManager(t, "task27 summary v1", "task27 body v1")
	recording := &task19RecordingProvider{turns: []task19ProviderTurn{
		{events: task19TextEvents("initial answer", "task27-response-1")},
		{events: task19TextEvents("unchanged answer", "task27-response-2")},
		{events: task19TextEvents("added answer", "task27-response-3")},
		{events: task19TextEvents("updated answer", "task27-response-4")},
		{events: task19TextEvents("revoked answer", "task27-response-5")},
		{events: task19TextEvents("post-compact answer", "task27-response-6")},
		{events: task19TextEvents("post-compact unchanged answer", "task27-response-7")},
	}}
	reg := registry.New()
	reg.Register(task19TickTool{})
	q := New(recording, reg, Config{
		MaxTurns:         2,
		MaxTokens:        2048,
		MaxContextTokens: 64_000,
		SessionID:        "task27-stable-cache-key",
		System:           "task27 stable fallback system",
		SystemBlocks: []prompt.SystemPromptBlock{
			{Text: "task27 stable system block", Cache: true, CacheScope: "ephemeral"},
		},
		ReasoningEffort: "high",
		SkillManager:    manager,
	})
	q.compactor = task19Compactor{}

	if err := q.Run(context.Background(), "task27 initial user", func(stream.Event) {}); err != nil {
		t.Fatal(err)
	}
	calls := recording.Calls()
	if len(calls) != 1 {
		t.Fatalf("initial provider calls = %d, want 1", len(calls))
	}
	initial := calls[0]
	if len(initial.Messages) != 2 {
		t.Fatalf("initial messages = %#v, want snapshot then user", initial.Messages)
	}
	assertTask19CatalogMessage(t, initial.Messages[0], types.DeveloperMessageKindSkillCatalogSnapshot)
	if initial.Messages[1].GetText() != "task27 initial user" {
		t.Fatalf("initial current user = %#v", initial.Messages[1])
	}
	stableEnvelope := task27StableEnvelopeBytes(t, initial)

	assertAppend := func(user string, wantKind types.DeveloperMessageKind, wantReason string, mutate func()) provider.Params {
		t.Helper()
		before := compact.GetMessagesAfterCompactBoundaryForScope(q.Messages(), q.internalControlScope)
		beforeBytes := task27MessageSequenceBytes(t, before)
		if mutate != nil {
			mutate()
		}
		if err := q.Run(context.Background(), user, func(stream.Event) {}); err != nil {
			t.Fatal(err)
		}
		latestCalls := recording.Calls()
		call := latestCalls[len(latestCalls)-1]
		if got := task27StableEnvelopeBytes(t, call); !bytes.Equal(got, stableEnvelope) {
			t.Fatalf("%s changed stable request envelope\n got: %s\nwant: %s", user, got, stableEnvelope)
		}
		if len(call.Messages) < len(before) {
			t.Fatalf("%s request shortened pre-compact history: got %d want at least %d", user, len(call.Messages), len(before))
		}
		if got := task27MessageSequenceBytes(t, call.Messages[:len(before)]); !bytes.Equal(got, beforeBytes) {
			t.Fatalf("%s rewrote the serialized prior message prefix\n got: %s\nwant: %s", user, got, beforeBytes)
		}

		tail := call.Messages[len(before):]
		if wantKind == "" {
			if len(tail) != 1 || tail[0].Role != types.RoleUser || tail[0].GetText() != user {
				t.Fatalf("%s no-change tail = %#v, want only current user", user, tail)
			}
		} else {
			if len(tail) != 2 {
				t.Fatalf("%s changed tail = %#v, want delta and current user", user, tail)
			}
			assertTask19CatalogMessage(t, tail[0], wantKind)
			if tail[1].Role != types.RoleUser || tail[1].GetText() != user {
				t.Fatalf("%s current user is not immediately after delta: %#v", user, tail)
			}
			if wantReason != "" && !strings.Contains(tail[0].GetText(), `"reason":"`+wantReason+`"`) {
				t.Fatalf("%s catalog delta lacks reason %q: %s", user, wantReason, tail[0].GetText())
			}
		}
		return call
	}

	assertAppend("task27 unchanged user", "", "", nil)

	root := filepath.Dir(filepath.Dir(originalSkillFile))
	addedSkillFile := filepath.Join(root, "task27-added", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(addedSkillFile), 0o755); err != nil {
		t.Fatal(err)
	}
	assertAppend("task27 add user", types.DeveloperMessageKindSkillCatalogDelta, string(skills.CatalogUpsertAdded), func() {
		task19WriteSkill(t, addedSkillFile, "task27 added summary", "task27 added body")
		if _, err := manager.RefreshSnapshot("task27-stable-cache-key"); err != nil {
			t.Fatal(err)
		}
	})
	assertAppend("task27 update user", types.DeveloperMessageKindSkillCatalogDelta, string(skills.CatalogUpsertUpdated), func() {
		task19WriteSkill(t, originalSkillFile, "task27 summary v2", "task27 body v2")
		if _, err := manager.RefreshSnapshot("task27-stable-cache-key"); err != nil {
			t.Fatal(err)
		}
	})
	preCompact := assertAppend("task27 revoke user", types.DeveloperMessageKindSkillCatalogDelta, string(skills.CatalogRevokeDeleted), func() {
		if err := os.Remove(addedSkillFile); err != nil {
			t.Fatal(err)
		}
		if _, err := manager.RefreshSnapshot("task27-stable-cache-key"); err != nil {
			t.Fatal(err)
		}
	})
	if preCompact.PreviousResponseID == "" {
		t.Fatal("pre-compact stable envelope did not retain the response chain")
	}

	beforeEpoch := q.SkillCatalogState().ContextEpoch
	if _, err := q.ForceCompact(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := q.SkillCatalogState().ContextEpoch; got != beforeEpoch+1 {
		t.Fatalf("compact context epoch = %d, want %d", got, beforeEpoch+1)
	}
	if err := q.Run(context.Background(), "task27 post-compact user", func(stream.Event) {}); err != nil {
		t.Fatal(err)
	}
	calls = recording.Calls()
	compactCall := calls[len(calls)-1]
	if compactCall.PreviousResponseID != "" {
		t.Fatalf("post-compact request reused response chain %q", compactCall.PreviousResponseID)
	}
	if got := task27StableEnvelopeBytes(t, compactCall); !bytes.Equal(got, stableEnvelope) {
		t.Fatalf("compact changed stable request envelope\n got: %s\nwant: %s", got, stableEnvelope)
	}
	if got := task19CatalogMessageCount(compactCall.Messages); got != 1 {
		t.Fatalf("compact should perform one intentional prefix rebuild, catalog messages = %d: %#v", got, compactCall.Messages)
	}
	if len(compactCall.Messages) < 2 {
		t.Fatalf("compact request lacks rebuilt suffix: %#v", compactCall.Messages)
	}
	rebuiltSuffix := compactCall.Messages[len(compactCall.Messages)-2:]
	assertTask19CatalogMessage(t, rebuiltSuffix[0], types.DeveloperMessageKindSkillCatalogSnapshot)
	if !strings.Contains(rebuiltSuffix[0].GetText(), `"type":"skill_catalog_snapshot"`) ||
		strings.Contains(rebuiltSuffix[0].GetText(), `"type":"skill_catalog_delta"`) {
		t.Fatalf("compact rebuilt suffix does not begin with a full snapshot: %#v", rebuiltSuffix)
	}
	if rebuiltSuffix[1].Role != types.RoleUser || rebuiltSuffix[1].GetText() != "task27 post-compact user" {
		t.Fatalf("compact rebuilt suffix order = %#v, want full snapshot immediately followed by current user", rebuiltSuffix)
	}

	postCompactUnchanged := assertAppend("task27 post-compact unchanged user", "", "", nil)
	if got := task19CatalogMessageCount(postCompactUnchanged.Messages); got != 1 {
		t.Fatalf("post-compact unchanged turn repeated the one-time rebuild: catalog messages = %d", got)
	}
	if postCompactUnchanged.PreviousResponseID == "" {
		t.Fatal("post-compact unchanged turn did not re-establish response chaining")
	}

	t.Log(task27CacheabilityEvidenceScope)
}

type task27StableEnvelope struct {
	Model                   string                       `json:"model"`
	MaxTokens               int                          `json:"max_tokens"`
	MaxOutputTokensOverride int                          `json:"max_output_tokens_override"`
	System                  string                       `json:"system"`
	SystemBlocks            []prompt.SystemPromptBlock   `json:"system_blocks"`
	Tools                   []types.ToolDefinition       `json:"tools"`
	ExtraToolSchemas        []types.ServerToolDefinition `json:"extra_tool_schemas"`
	ToolChoice              *provider.ToolChoice         `json:"tool_choice"`
	Thinking                *provider.ThinkingConfig     `json:"thinking"`
	TaskBudget              *provider.TaskBudget         `json:"task_budget"`
	Conversation            string                       `json:"conversation"`
	Truncation              string                       `json:"truncation"`
	PromptCacheKey          string                       `json:"prompt_cache_key"`
	UsePromptCache          bool                         `json:"use_prompt_cache"`
	ReasoningEffort         string                       `json:"reasoning_effort"`
}

func task27StableEnvelopeBytes(t testing.TB, params provider.Params) []byte {
	t.Helper()
	encoded, err := json.Marshal(task27StableEnvelope{
		Model:                   params.Model,
		MaxTokens:               params.MaxTokens,
		MaxOutputTokensOverride: params.MaxOutputTokensOverride,
		System:                  params.System,
		SystemBlocks:            params.SystemBlocks,
		Tools:                   params.Tools,
		ExtraToolSchemas:        params.ExtraToolSchemas,
		ToolChoice:              params.ToolChoice,
		Thinking:                params.Thinking,
		TaskBudget:              params.TaskBudget,
		Conversation:            params.Conversation,
		Truncation:              params.Truncation,
		PromptCacheKey:          params.PromptCacheKey,
		UsePromptCache:          params.UsePromptCache,
		ReasoningEffort:         params.ReasoningEffort,
	})
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func task27MessageSequenceBytes(t testing.TB, messages []types.Message) []byte {
	t.Helper()
	encoded, err := json.Marshal(messages)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func BenchmarkSkillCatalogCacheabilityRenderAndDiff(b *testing.B) {
	for _, size := range []int{100, 1_000} {
		initial := task27BenchmarkCatalog(b, size, 1)
		initialPlan, err := PlanSkillCatalog(SkillCatalogCoordinatorInput{
			CurrentSnapshot: initial,
			ContextEpoch:    "task27-benchmark",
			CharBudget:      1_000_000,
		})
		if err != nil {
			b.Fatal(err)
		}
		visible := []types.Message{*initialPlan.Message, types.UserMessage("task27 benchmark user")}

		b.Run(fmt.Sprintf("snapshot/%d", size), func(b *testing.B) {
			b.ReportAllocs()
			b.ReportMetric(float64(size), "skills/op")
			for range b.N {
				plan, planErr := PlanSkillCatalog(SkillCatalogCoordinatorInput{
					CurrentSnapshot: initial,
					ContextEpoch:    "task27-benchmark-snapshot",
					CharBudget:      1_000_000,
				})
				if planErr != nil || plan.Message == nil {
					b.Fatalf("snapshot plan = %#v, %v", plan, planErr)
				}
			}
		})

		updated := initial.Clone()
		updated.Revision++
		middle := size / 2
		updated.Skills[middle].Summary += " updated"
		updated.Skills[middle].Digest = skills.ComputeSkillDigest(updated.Skills[middle].Summary)
		updated.Skills[middle].Revision++
		updated, err = skills.NewCatalogSnapshot(updated.Revision, updated.Skills)
		if err != nil {
			b.Fatal(err)
		}
		b.Run(fmt.Sprintf("single-update-delta/%d", size), func(b *testing.B) {
			b.ReportAllocs()
			b.ReportMetric(float64(size), "skills/op")
			for range b.N {
				plan, planErr := PlanSkillCatalog(SkillCatalogCoordinatorInput{
					CurrentSnapshot: updated,
					PriorCursor:     initialPlan.Cursor,
					ContextEpoch:    "task27-benchmark",
					VisibleHistory:  visible,
					CharBudget:      1_000_000,
				})
				if planErr != nil || plan.Message == nil || plan.Kind != SkillCatalogPlanDelta {
					b.Fatalf("delta plan = %#v, %v", plan, planErr)
				}
			}
		})
	}
}

func task27BenchmarkCatalog(tb testing.TB, size int, revision skills.CatalogRevision) skills.CatalogSnapshot {
	tb.Helper()
	entries := make([]skills.EffectiveSkill, size)
	for index := range entries {
		locator := skills.SkillLocator(fmt.Sprintf("/task27/benchmark/%06d/SKILL.md", index))
		id, err := skills.ComputeSkillID(skills.SourceProject, locator)
		if err != nil {
			tb.Fatal(err)
		}
		summary := fmt.Sprintf("bounded benchmark skill %06d", index)
		entries[index] = skills.EffectiveSkill{
			ID:                 id,
			Name:               fmt.Sprintf("task27-skill-%06d", index),
			Summary:            summary,
			Source:             skills.SourceProject,
			Locator:            locator,
			Digest:             skills.ComputeSkillDigest(summary),
			Revision:           1,
			Visibility:         skills.VisibilityAuto,
			VisibilitySource:   skills.SkillScopeDefault,
			ModelVisible:       true,
			DescriptionVisible: true,
			UserInvocable:      true,
			Executable:         true,
			Mutable:            true,
		}
	}
	snapshot, err := skills.NewCatalogSnapshot(revision, entries)
	if err != nil {
		tb.Fatal(err)
	}
	return snapshot
}
