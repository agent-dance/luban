package engine

import (
	"errors"
	"io/fs"
	"sync"

	"github.com/agent-dance/luban/session"
	"github.com/agent-dance/luban/types"
)

type sessionGenerationTracker struct {
	mu     sync.Mutex
	values map[string]ContextGenerationState
}

func (t *sessionGenerationTracker) load(key string) (ContextGenerationState, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	value, ok := t.values[key]
	return value, ok
}

func (t *sessionGenerationTracker) store(key string, value ContextGenerationState) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.values == nil {
		t.values = make(map[string]ContextGenerationState)
	}
	t.values[key] = value
}

func (t *sessionGenerationTracker) current(store *session.FileStore, key, sessionID string) (ContextGenerationState, error) {
	manifest, err := store.GetCompactionManifest(sessionID)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			if cached, ok := t.load(key); ok {
				// Once a manifest-backed generation has been observed, losing the
				// manifest is an authority failure rather than a transition back to
				// an unpersisted legacy state.
				if cached.Persisted {
					return ContextGenerationState{}, err
				}
				return cached, nil
			}
			return ContextGenerationState{}, nil
		}
		return ContextGenerationState{}, err
	}
	state := ContextGenerationState{Generation: manifest.ContextGeneration, Persisted: true}
	t.store(key, state)
	return state, nil
}

func (t *sessionGenerationTracker) remove(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.values, key)
}

func (t *sessionGenerationTracker) recordLoaded(store *session.FileStore, key, sessionID string) error {
	manifest, err := store.GetCompactionManifest(sessionID)
	if err != nil {
		return err
	}
	t.store(key, ContextGenerationState{Generation: manifest.ContextGeneration, Persisted: true})
	return nil
}

func (t *sessionGenerationTracker) prepare(store *session.FileStore, key, sessionID string) error {
	if _, known := t.load(key); known {
		return nil
	}
	manifest, err := store.GetCompactionManifest(sessionID)
	if err == nil {
		t.store(key, ContextGenerationState{Generation: manifest.ContextGeneration, Persisted: true})
		return nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	// A manifest-less JSONL session is migrated under the store lock before
	// the generation snapshot is published. A truly new session starts at 0.
	if _, loadErr := store.Load(sessionID); loadErr != nil {
		if errors.Is(loadErr, fs.ErrNotExist) {
			t.store(key, ContextGenerationState{})
			return nil
		}
		return loadErr
	}
	return t.recordLoaded(store, key, sessionID)
}

func (t *sessionGenerationTracker) save(store *session.FileStore, key, sessionID string, messages []types.Message) error {
	state, known := t.load(key)
	expected := state.Generation
	if !known {
		manifest, err := store.GetCompactionManifest(sessionID)
		switch {
		case err == nil:
			expected = manifest.ContextGeneration
		case !errors.Is(err, fs.ErrNotExist):
			return err
		default:
			expected = 0
		}
	}
	manifest, err := store.SaveModelContextCAS(sessionID, expected, messages)
	return t.recordSaveResult(key, manifest, err)
}

func (t *sessionGenerationTracker) recordSaveResult(key string, manifest session.CompactionManifestV2, err error) error {
	if err != nil {
		var committed *session.ContextCommitError
		if errors.As(err, &committed) {
			if committed.Manifest.ContextGeneration != 0 {
				t.store(key, ContextGenerationState{Generation: committed.Manifest.ContextGeneration, Persisted: true})
			} else if manifest.ContextGeneration != 0 {
				t.store(key, ContextGenerationState{Generation: manifest.ContextGeneration, Persisted: true})
			}
		}
		return err
	}
	t.store(key, ContextGenerationState{Generation: manifest.ContextGeneration, Persisted: true})
	return nil
}
