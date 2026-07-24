package session

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"github.com/agent-dance/luban/internal/messagecontrol"
	"github.com/agent-dance/luban/types"
)

func compactProjectionForTest(scope messagecontrol.Scope, tail ...types.Message) []types.Message {
	boundary := types.UserMessage("sealed compact boundary")
	boundary.ID = "compact:boundary:v1"
	boundary.IsMeta = true
	boundary.InternalKind = types.InternalMessageKindCompactBoundary
	boundary = boundary.WithInternalControlProvenance(messagecontrol.Runtime(), scope)
	summary := types.UserMessage("rolling summary")
	summary.ID = "compact:summary:v1"
	summary.IsMeta = true
	summary.InternalKind = types.InternalMessageKindCompactSummary
	summary = summary.WithInternalControlProvenance(messagecontrol.Runtime(), scope)
	return append([]types.Message{boundary, summary}, tail...)
}

func compactProjectionWithMetadataForTest(t *testing.T, scope messagecontrol.Scope, boundaryText, summaryText string, tail ...types.Message) []types.Message {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"trigger":                       "manual",
		"pre_compact_token_count":       1200,
		"post_compact_token_count":      300,
		"true_post_compact_token_count": 320,
		"compaction_usage":              map[string]any{"input_tokens": 40, "output_tokens": 12},
		"previous_tail_identifier":      boundaryText,
	})
	if err != nil {
		t.Fatal(err)
	}
	boundary := types.UserMessage("[compact-boundary]" + base64.StdEncoding.EncodeToString(payload))
	boundary.ID = "compact:boundary:v1"
	boundary.IsMeta = true
	boundary.InternalKind = types.InternalMessageKindCompactBoundary
	boundary = boundary.WithInternalControlProvenance(messagecontrol.Runtime(), scope)
	summary := types.UserMessage(summaryText)
	summary.ID = "compact:summary:v1"
	summary.IsMeta = true
	summary.InternalKind = types.InternalMessageKindCompactSummary
	summary = summary.WithInternalControlProvenance(messagecontrol.Runtime(), scope)
	return append([]types.Message{boundary, summary}, tail...)
}

func TestCompactionChangesOnlyModelViewAndPreservesAppendOnlyAuditDigest(t *testing.T) {
	store := NewFileStore(t.TempDir())
	const sessionID = "audit-preserved"
	raw := []types.Message{
		types.UserMessage("earliest decision"),
		types.AssistantMessage("decision accepted"),
		types.UserMessage("latest request"),
		types.AssistantMessage("latest answer"),
	}
	if err := store.Save(sessionID, raw); err != nil {
		t.Fatal(err)
	}
	before, err := store.GetCompactionManifest(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	scope, _ := store.MessageControlScope(sessionID)
	view := compactProjectionForTest(scope, raw[2:]...)
	after, err := store.CommitModelContext(sessionID, before.ContextGeneration, view, nil)
	if err != nil {
		t.Fatal(err)
	}
	if after.ContextGeneration != before.ContextGeneration+1 {
		t.Fatalf("generation = %d, want %d", after.ContextGeneration, before.ContextGeneration+1)
	}
	if after.AuditTailDigest != before.AuditTailDigest || after.AuditMessageCount != before.AuditMessageCount ||
		!reflect.DeepEqual(after.AuditSegments, before.AuditSegments) {
		t.Fatalf("compaction mutated audit identity: before=%+v after=%+v", before, after)
	}
	loadedView, err := store.Load(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	loadedJSON, loadedErr := json.Marshal(loadedView)
	viewJSON, viewErr := json.Marshal(view)
	if loadedErr != nil || viewErr != nil || !reflect.DeepEqual(loadedJSON, viewJSON) {
		t.Fatalf("model view = %#v, want %#v", loadedView, view)
	}
	for index := 0; index < 2; index++ {
		scope, bound := loadedView[index].InternalControlProvenanceScope()
		if !bound || scope.SessionID() != sessionID || scope.ContextGeneration() != after.ContextGeneration {
			t.Fatalf("model view control %d scope = %#v bound=%t", index, scope, bound)
		}
	}
	audit, manifest, err := store.LoadAuditLog(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(audit, raw) {
		t.Fatalf("audit = %#v, want immutable raw history %#v", audit, raw)
	}
	if manifest.AuditTailDigest != before.AuditTailDigest {
		t.Fatal("audit digest changed after compaction")
	}
}

func TestSaveAfterCompactionAppendsOnlyNewRawMessages(t *testing.T) {
	store := NewFileStore(t.TempDir())
	const sessionID = "audit-after-compact"
	raw := []types.Message{types.UserMessage("old request"), types.AssistantMessage("old answer")}
	if err := store.Save(sessionID, raw); err != nil {
		t.Fatal(err)
	}
	manifest, err := store.GetCompactionManifest(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	scope, _ := store.MessageControlScope(sessionID)
	view := compactProjectionForTest(scope, raw[1])
	if _, err := store.CommitModelContext(sessionID, manifest.ContextGeneration, view, nil); err != nil {
		t.Fatal(err)
	}
	newRaw := []types.Message{types.UserMessage("new request"), types.AssistantMessage("new answer")}
	committedView, err := store.Load(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(sessionID, append(committedView, newRaw...)); err != nil {
		t.Fatal(err)
	}
	audit, _, err := store.LoadAuditLog(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	want := append(append([]types.Message(nil), raw...), newRaw...)
	if !reflect.DeepEqual(audit, want) {
		t.Fatalf("audit = %#v, want %#v", audit, want)
	}
}

func TestTranscriptPathIsCompleteImmutableAuditAcrossMultipleCompactions(t *testing.T) {
	store := NewFileStore(t.TempDir())
	const sessionID = "full-audit-transcript"
	raw := []types.Message{types.UserMessage("first raw request"), types.AssistantMessage("first raw answer")}
	if err := store.Save(sessionID, raw); err != nil {
		t.Fatal(err)
	}
	firstPath := store.TranscriptPath(sessionID)
	assertTranscriptMessages(t, firstPath, raw)
	firstManifest, err := store.GetCompactionManifest(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	firstScope, _ := store.MessageControlScope(sessionID)
	firstCompact := compactProjectionWithMetadataForTest(t, firstScope, "first", "summary one", raw[1])
	secondManifest, err := store.CommitModelContext(sessionID, firstManifest.ContextGeneration, firstCompact, nil)
	if err != nil {
		t.Fatal(err)
	}
	if path := store.TranscriptPath(sessionID); path != firstPath {
		t.Fatalf("audit path changed for view-only compaction: %q -> %q", firstPath, path)
	}
	assertTranscriptMessages(t, firstPath, raw)
	if secondManifest.AuditTranscript != firstManifest.AuditTranscript {
		t.Fatal("compaction replaced immutable audit transcript reference")
	}

	newRaw := []types.Message{types.UserMessage("second raw request"), types.AssistantMessage("second raw answer")}
	committedFirst, err := store.Load(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(sessionID, append(committedFirst, newRaw...)); err != nil {
		t.Fatal(err)
	}
	wantAudit := append(append([]types.Message(nil), raw...), newRaw...)
	secondPath := store.TranscriptPath(sessionID)
	if secondPath == firstPath {
		t.Fatal("appending raw events did not publish a new content-addressed audit transcript")
	}
	assertTranscriptMessages(t, secondPath, wantAudit)
	beforeSecondCompact, err := store.GetCompactionManifest(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	secondScope, _ := store.MessageControlScope(sessionID)
	secondCompact := compactProjectionWithMetadataForTest(t, secondScope, "second", "summary two", newRaw...)
	afterSecondCompact, err := store.CommitModelContext(sessionID, beforeSecondCompact.ContextGeneration, secondCompact, nil)
	if err != nil {
		t.Fatal(err)
	}
	if path := store.TranscriptPath(sessionID); path != secondPath {
		t.Fatalf("second compaction changed complete audit path: %q -> %q", secondPath, path)
	}
	if afterSecondCompact.AuditTailDigest != beforeSecondCompact.AuditTailDigest ||
		afterSecondCompact.AuditTranscript != beforeSecondCompact.AuditTranscript {
		t.Fatal("second compaction changed audit digest or transcript reference")
	}
	assertTranscriptMessages(t, secondPath, wantAudit)
}

func TestManifestCarriesBoundarySummaryUsageChainAndRetainedRefs(t *testing.T) {
	store := NewFileStore(t.TempDir())
	const sessionID = "manifest-compaction-state"
	raw := []types.Message{types.UserMessage("raw"), types.AssistantMessage("answer")}
	if err := store.Save(sessionID, raw); err != nil {
		t.Fatal(err)
	}
	initial, err := store.GetCompactionManifest(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	firstScope, _ := store.MessageControlScope(sessionID)
	firstView := compactProjectionWithMetadataForTest(t, firstScope, "first", "summary generation one", raw[1])
	first, err := store.CommitModelContext(sessionID, initial.ContextGeneration, firstView, nil)
	if err != nil {
		t.Fatal(err)
	}
	if first.Boundary == nil || first.Boundary.BoundaryID == "" || first.Boundary.Trigger != "manual" ||
		first.Boundary.PreCompactTokens != 1200 || first.Boundary.PostCompactTokens != 300 ||
		first.Boundary.TruePostCompact != 320 || len(first.Boundary.CompactionUsage) == 0 {
		t.Fatalf("boundary metadata = %+v", first.Boundary)
	}
	if first.Summary == nil || first.Summary.Schema != "compact-summary/v2" || first.Summary.Digest == "" || first.Summary.ParentDigest != "" {
		t.Fatalf("summary metadata = %+v", first.Summary)
	}
	if len(first.RetainedMessages) != 1 || first.RetainedMessages[0].ViewIndex != 2 {
		t.Fatalf("retained refs = %+v", first.RetainedMessages)
	}

	secondScope, _ := store.MessageControlScope(sessionID)
	secondView := compactProjectionWithMetadataForTest(t, secondScope, "second", "summary generation two", raw[1])
	second, err := store.CommitModelContext(sessionID, first.ContextGeneration, secondView, nil)
	if err != nil {
		t.Fatal(err)
	}
	if second.Summary == nil || second.Summary.ParentDigest != first.Summary.Digest || second.Summary.Digest == first.Summary.Digest {
		t.Fatalf("rolling summary chain = first %+v second %+v", first.Summary, second.Summary)
	}
	if second.Boundary == nil || second.Boundary.BoundaryID == first.Boundary.BoundaryID {
		t.Fatalf("boundary identity did not advance: first %+v second %+v", first.Boundary, second.Boundary)
	}
}

func TestDuplicateCompactionViewIsIdempotent(t *testing.T) {
	store := NewFileStore(t.TempDir())
	const sessionID = "duplicate-boundary"
	raw := []types.Message{types.UserMessage("raw")}
	if err := store.Save(sessionID, raw); err != nil {
		t.Fatal(err)
	}
	initial, err := store.GetCompactionManifest(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	scope, _ := store.MessageControlScope(sessionID)
	view := compactProjectionWithMetadataForTest(t, scope, "stable", "stable summary", raw...)
	first, err := store.CommitModelContext(sessionID, initial.ContextGeneration, view, nil)
	if err != nil {
		t.Fatal(err)
	}
	committedView, err := store.Load(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := store.CommitModelContext(sessionID, first.ContextGeneration, committedView, nil)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate.ContextGeneration != first.ContextGeneration || duplicate.Digest != first.Digest {
		t.Fatalf("duplicate boundary published a new generation: first=%+v duplicate=%+v", first, duplicate)
	}
}

func TestPostPublishFailureReportsCommittedGenerationForSafeRetry(t *testing.T) {
	store := NewFileStore(t.TempDir())
	const sessionID = "post-publish-error"
	oldView := []types.Message{types.UserMessage("old")}
	if err := store.Save(sessionID, oldView); err != nil {
		t.Fatal(err)
	}
	oldManifest, err := store.GetCompactionManifest(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	injected := errors.New("process stopped after manifest publish")
	store.historyCommitFault = func(stage HistoryCommitStage) error {
		if stage == HistoryStageManifestPublished {
			return injected
		}
		return nil
	}
	newMessage := types.AssistantMessage("committed despite caller error")
	newView := append(append([]types.Message(nil), oldView...), newMessage)
	committed, err := store.SaveModelContextCAS(sessionID, oldManifest.ContextGeneration, newView)
	var commitErr *ContextCommitError
	if !errors.As(err, &commitErr) || !errors.Is(err, injected) {
		t.Fatalf("error = %v, want ContextCommitError wrapping injected cause", err)
	}
	if committed.ContextGeneration != oldManifest.ContextGeneration+1 || commitErr.Manifest.Digest != committed.Digest {
		t.Fatalf("committed manifest not returned: result=%+v error=%+v", committed, commitErr)
	}
	store.historyCommitFault = nil
	finalMessage := types.UserMessage("retry from committed generation")
	finalView := append(append([]types.Message(nil), newView...), finalMessage)
	finalManifest, err := store.SaveModelContextCAS(sessionID, committed.ContextGeneration, finalView)
	if err != nil {
		t.Fatalf("safe retry from committed generation: %v", err)
	}
	if finalManifest.ContextGeneration != committed.ContextGeneration+1 {
		t.Fatalf("final generation = %d", finalManifest.ContextGeneration)
	}
}

func TestMetadataWriteFailureKeepsManifestAuthoritativeAndDerivedMetaCoherent(t *testing.T) {
	store := NewFileStore(t.TempDir())
	const sessionID = "meta-after-manifest"
	oldView := []types.Message{types.UserMessage("old preview")}
	if err := store.Save(sessionID, oldView); err != nil {
		t.Fatal(err)
	}
	oldManifest, err := store.GetCompactionManifest(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	injected := errors.New("metadata write failed")
	store.metaWriteFault = func() error { return injected }
	newView := append(oldView, types.AssistantMessage("new authoritative preview"))
	committed, err := store.SaveModelContextCAS(sessionID, oldManifest.ContextGeneration, newView)
	var commitErr *ContextCommitError
	if !errors.As(err, &commitErr) || !errors.Is(err, injected) {
		t.Fatalf("SaveModelContextCAS error = %v, want committed metadata failure", err)
	}
	if committed.ContextGeneration != oldManifest.ContextGeneration+1 {
		t.Fatalf("committed generation = %d", committed.ContextGeneration)
	}
	store.metaWriteFault = nil
	loaded, err := store.Load(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded, newView) {
		t.Fatalf("model view rolled back after metadata failure: %#v", loaded)
	}
	meta, err := store.GetMeta(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if meta.MessageCount != len(newView) || meta.PreviewText != "new authoritative preview" {
		t.Fatalf("derived metadata did not project authoritative manifest: %+v", meta)
	}
}

func TestCompactionManifestCASRejectsLateGenerationAcrossStoreInstances(t *testing.T) {
	dir := t.TempDir()
	first := NewFileStore(dir)
	second := NewFileStore(dir)
	const sessionID = "generation-cas"
	initial := []types.Message{types.UserMessage("initial")}
	if err := first.Save(sessionID, initial); err != nil {
		t.Fatal(err)
	}
	manifest, err := first.GetCompactionManifest(sessionID)
	if err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	commit := func(store *FileStore, text string) {
		defer ready.Done()
		<-start
		_, err := store.CommitModelContext(sessionID, manifest.ContextGeneration,
			append(initial, types.AssistantMessage(text)), []types.Message{types.AssistantMessage(text)})
		results <- err
	}
	go commit(first, "writer one")
	go commit(second, "writer two")
	close(start)
	ready.Wait()
	close(results)

	succeeded, stale := 0, 0
	for err := range results {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrStaleContextGeneration):
			stale++
		default:
			t.Fatalf("unexpected commit error: %v", err)
		}
	}
	if succeeded != 1 || stale != 1 {
		t.Fatalf("succeeded=%d stale=%d, want 1/1", succeeded, stale)
	}
	committed, err := first.GetCompactionManifest(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if committed.ContextGeneration != manifest.ContextGeneration+1 {
		t.Fatalf("generation = %d", committed.ContextGeneration)
	}
}

func TestCompactionCommitCrashPointsRecoverOnlyCompleteOldOrNewGeneration(t *testing.T) {
	stages := []HistoryCommitStage{
		HistoryStageAuditSegmentPrepared,
		HistoryStageAuditPrepared,
		HistoryStageViewPrepared,
		HistoryStageBeforeManifestCAS,
		HistoryStageManifestPrepared,
		HistoryStageManifestPublished,
	}
	for _, stage := range stages {
		t.Run(string(stage), func(t *testing.T) {
			store := NewFileStore(t.TempDir())
			const sessionID = "crash-recovery"
			oldView := []types.Message{types.UserMessage("old generation")}
			if err := store.Save(sessionID, oldView); err != nil {
				t.Fatal(err)
			}
			oldManifest, err := store.GetCompactionManifest(sessionID)
			if err != nil {
				t.Fatal(err)
			}
			newMessage := types.AssistantMessage("new generation")
			newView := append(append([]types.Message(nil), oldView...), newMessage)
			injected := errors.New("injected crash")
			store.historyCommitFault = func(current HistoryCommitStage) error {
				if current == stage {
					return injected
				}
				return nil
			}
			if _, err := store.CommitModelContext(sessionID, oldManifest.ContextGeneration, newView, []types.Message{newMessage}); !errors.Is(err, injected) {
				t.Fatalf("commit error = %v, want injected crash", err)
			}
			store.historyCommitFault = nil

			recoveredView, err := store.Load(sessionID)
			if err != nil {
				t.Fatalf("recovery failed: %v", err)
			}
			recoveredAudit, recoveredManifest, err := store.LoadAuditLog(sessionID)
			if err != nil {
				t.Fatalf("audit recovery failed: %v", err)
			}
			if stage == HistoryStageManifestPublished {
				if !reflect.DeepEqual(recoveredView, newView) || !reflect.DeepEqual(recoveredAudit, newView) ||
					recoveredManifest.ContextGeneration != oldManifest.ContextGeneration+1 {
					t.Fatalf("post-publish recovery is not the complete new generation: view=%#v audit=%#v manifest=%+v", recoveredView, recoveredAudit, recoveredManifest)
				}
			} else if !reflect.DeepEqual(recoveredView, oldView) || !reflect.DeepEqual(recoveredAudit, oldView) ||
				recoveredManifest.ContextGeneration != oldManifest.ContextGeneration {
				t.Fatalf("pre-publish recovery is not the complete old generation: view=%#v audit=%#v manifest=%+v", recoveredView, recoveredAudit, recoveredManifest)
			}
		})
	}
}

func TestFirstGenerationManifestPublishSurvivesMissingCompatibilitySnapshot(t *testing.T) {
	root := t.TempDir()
	store := NewFileStore(root)
	const sessionID = "first-publish-crash"
	messages := []types.Message{types.UserMessage("first durable generation")}
	injected := errors.New("crash after first manifest publish")
	store.historyCommitFault = func(stage HistoryCommitStage) error {
		if stage == HistoryStageManifestPublished {
			return injected
		}
		return nil
	}
	if err := store.Save(sessionID, messages); !errors.Is(err, injected) {
		t.Fatalf("Save error = %v, want injected crash", err)
	}
	if _, err := os.Lstat(store.sessionPath(sessionID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("compatibility snapshot unexpectedly exists after crash: %v", err)
	}

	restarted := NewFileStore(root)
	loaded, err := restarted.Load(sessionID)
	if err != nil {
		t.Fatalf("Load authoritative first manifest: %v", err)
	}
	if !reflect.DeepEqual(loaded, messages) {
		t.Fatalf("recovered view = %#v, want %#v", loaded, messages)
	}
	audit, manifest, err := restarted.LoadAuditLog(sessionID)
	if err != nil || !reflect.DeepEqual(audit, messages) || manifest.ContextGeneration != 1 {
		t.Fatalf("recovered audit=%#v manifest=%+v err=%v", audit, manifest, err)
	}
}

func TestReferencedHistoryCorruptionAndTruncationFailClosed(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *FileStore, CompactionManifestV2)
	}{
		{
			name: "manifest truncated",
			mutate: func(t *testing.T, store *FileStore, _ CompactionManifestV2) {
				if err := os.WriteFile(store.manifestPath("corrupt"), []byte("{\"schema_version\":"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "model view truncated",
			mutate: func(t *testing.T, store *FileStore, manifest CompactionManifestV2) {
				name, err := digestFileName(manifest.ModelContextView.Digest, ".jsonl")
				if err != nil {
					t.Fatal(err)
				}
				if err := os.Truncate(filepath.Join(store.viewDir("corrupt"), name), 5); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "audit segment truncated",
			mutate: func(t *testing.T, store *FileStore, manifest CompactionManifestV2) {
				name, err := digestFileName(manifest.AuditSegments[0].Digest, ".json")
				if err != nil {
					t.Fatal(err)
				}
				if err := os.Truncate(filepath.Join(store.auditDir("corrupt"), name), 7); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "complete audit transcript truncated",
			mutate: func(t *testing.T, store *FileStore, manifest CompactionManifestV2) {
				name, err := digestFileName(manifest.AuditTranscript.Digest, ".jsonl")
				if err != nil {
					t.Fatal(err)
				}
				if err := os.Truncate(filepath.Join(store.auditTranscriptDir("corrupt"), name), 3); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := NewFileStore(t.TempDir())
			if err := store.Save("corrupt", []types.Message{types.UserMessage("must not return a valid prefix")}); err != nil {
				t.Fatal(err)
			}
			manifest, err := store.GetCompactionManifest("corrupt")
			if err != nil {
				t.Fatal(err)
			}
			test.mutate(t, store, manifest)
			if messages, err := store.Load("corrupt"); !errors.Is(err, ErrCorruptSessionHistory) || messages != nil {
				t.Fatalf("Load = %#v, %v; want nil and ErrCorruptSessionHistory", messages, err)
			}
		})
	}
}

func assertTranscriptMessages(t *testing.T, path string, want []types.Message) {
	t.Helper()
	if path == "" {
		t.Fatal("TranscriptPath returned empty")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeMessagesJSONL(data)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("transcript %q = %#v, want %#v", path, got, want)
	}
}

func TestLegacyJSONLSessionMigratesToManifestV2WithoutDataLoss(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)
	const sessionID = "legacy-migrate"
	legacy := []types.Message{types.UserMessage("legacy request"), types.AssistantMessage("legacy answer")}
	payload, err := encodeMessagesJSONL(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.sessionPath(sessionID), payload, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded, legacy) {
		t.Fatalf("loaded = %#v, want %#v", loaded, legacy)
	}
	audit, manifest, err := store.LoadAuditLog(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(audit, legacy) || manifest.SchemaVersion != compactionManifestSchemaV2 || manifest.ContextGeneration != 1 {
		t.Fatalf("migration audit=%#v manifest=%+v", audit, manifest)
	}
	if _, err := os.Stat(store.manifestPath(sessionID)); err != nil {
		t.Fatalf("manifest not published: %v", err)
	}
}

func TestLegacyTruncatedJSONLFailsClosedWithoutPublishingMigration(t *testing.T) {
	store := NewFileStore(t.TempDir())
	const sessionID = "legacy-truncated"
	if err := os.WriteFile(store.sessionPath(sessionID), []byte("{\"role\":\"user\",\"content\":"), 0o600); err != nil {
		t.Fatal(err)
	}
	if messages, err := store.Load(sessionID); !errors.Is(err, ErrCorruptSessionHistory) || messages != nil {
		t.Fatalf("Load = %#v, %v; want fail closed", messages, err)
	}
	if _, err := os.Stat(store.manifestPath(sessionID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("corrupt legacy session published a manifest: %v", err)
	}
}
