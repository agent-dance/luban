package tui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agent-dance/luban/i18n"
)

type failingObservationEvidenceJournal struct {
	inner DetailStore
	cause error
}

func (s *failingObservationEvidenceJournal) Put(key string, data []byte) (DetailRef, error) {
	return s.inner.Put(key, data)
}

func (s *failingObservationEvidenceJournal) Get(ref DetailRef) ([]byte, error) {
	return s.inner.Get(ref)
}

func (s *failingObservationEvidenceJournal) SaveObservationEvidence(Observation) error {
	return s.cause
}

func (s *failingObservationEvidenceJournal) LoadObservationEvidence() ([]Observation, error) {
	return nil, nil
}

func TestObservationEvidenceIndexFailureUsesSemanticErrorAndPreservesCause(t *testing.T) {
	cause := errors.New("raw-journal-cause")
	details := &failingObservationEvidenceJournal{inner: NewMemoryDetailStore(), cause: cause}
	ref, err := details.Put("evidence", []byte("raw evidence"))
	if err != nil {
		t.Fatal(err)
	}
	err = NewObservationStore(details).UpsertEvidenceObservation(Observation{ID: "observation-17"}, ref)
	if !errors.Is(err, cause) {
		t.Fatalf("UpsertEvidenceObservation error = %v, want raw cause", err)
	}
	info, ok := i18n.DescribeSemanticError(err)
	if !ok || info.Key != i18n.KeyTUIObservationRetainEvidenceIndex || !info.IncludeCause {
		t.Fatalf("semantic error = %+v, %v", info, ok)
	}
}

func TestActivityRestoreValidationUsesSemanticErrorsAndInternalSentinel(t *testing.T) {
	tests := []struct {
		event ActivityEvent
		key   i18n.Key
	}{
		{
			event: ActivityEvent{State: ActivityRunning, Lifecycle: ActivityLifecycleCompleted},
			key:   i18n.KeyTUIActivityStateLifecycleIncompatible,
		},
		{
			event: ActivityEvent{State: ActivityRunning, Outcome: OutcomeFailed},
			key:   i18n.KeyTUIActivityStateOutcomeIncompatible,
		},
	}
	for _, test := range tests {
		_, err := normalizeActivityEvent(test.event)
		if !errors.Is(err, ErrActivityStateOutcomeMismatch) {
			t.Errorf("normalizeActivityEvent(%+v) error = %v, want internal sentinel", test.event, err)
			continue
		}
		info, ok := i18n.DescribeSemanticError(err)
		if !ok || info.Key != test.key || info.IncludeCause {
			t.Errorf("semantic error = %+v, %v, want key %q and hidden cause", info, ok, test.key)
		}
	}
}

func TestSessionViewCheckpointFilesystemAndDecodeFailuresUseSemanticErrors(t *testing.T) {
	digest := strings.Repeat("a", 64)
	checkpoint := sessionViewCheckpoint{Version: sessionViewCheckpointVersion, TranscriptDigest: digest}

	rootFile := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(rootFile, []byte("file"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := writeSessionViewCheckpoint(rootFile, checkpoint)
	var pathError *os.PathError
	if !errors.As(err, &pathError) {
		t.Fatalf("writeSessionViewCheckpoint error = %v, want *os.PathError", err)
	}
	info, ok := i18n.DescribeSemanticError(err)
	if !ok || info.Key != i18n.KeyTUISessionViewPrepareCheckpointDir || !info.IncludeCause {
		t.Fatalf("filesystem semantic error = %+v, %v", info, ok)
	}

	root := t.TempDir()
	path := sessionViewCheckpointPath(root, 0, digest)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not-json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err = readSessionViewCheckpoint(root, 0, digest)
	info, ok = i18n.DescribeSemanticError(err)
	if !ok || info.Key != i18n.KeyTUISessionViewDecodeCheckpointFile || !info.IncludeCause {
		t.Fatalf("decode semantic error = %+v, %v, error %v", info, ok, err)
	}
}
