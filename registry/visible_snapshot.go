package registry

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/agent-dance/luban/types"
)

// ModelToolProfile fixes the model-facing tool surface independently from the
// larger execution registry. The zero value preserves legacy visibility.
type ModelToolProfile uint8

const (
	ModelToolProfileLegacy ModelToolProfile = iota
	ModelToolProfileAgenticV2
)

var agenticV2VisibleToolOrder = [...]string{"Inspect", "ApplyPatch", "Run"}

// VisibleToolSnapshot is an immutable, content-addressed provider catalog.
// Callers receive defensive definition copies so neither prompt construction
// nor provider serialization can mutate the snapshot seen by the other.
type VisibleToolSnapshot struct {
	definitions []types.ToolDefinition
	digest      string
	generation  uint64
	profile     ModelToolProfile
}

// SetModelToolProfile selects the model-facing visibility contract. Changing
// it advances the catalog generation even when the registered tools are
// unchanged, forcing the next request onto a new envelope.
func (r *Registry) SetModelToolProfile(profile ModelToolProfile) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.modelToolProfile == profile {
		return
	}
	r.modelToolProfile = profile
	r.catalogGeneration++
	if r.catalogGeneration == 0 {
		r.catalogGeneration++
	}
}

// ModelToolProfile returns the current visibility contract.
func (r *Registry) ModelToolProfile() ModelToolProfile {
	if r == nil {
		return ModelToolProfileLegacy
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.modelToolProfile
}

// Valid reports whether the snapshot was produced by a registry.
func (s VisibleToolSnapshot) Valid() bool { return s.digest != "" }

// Digest returns the stable SHA-256 identity of the ordered full schemas and
// visibility profile.
func (s VisibleToolSnapshot) Digest() string { return s.digest }

// Generation returns the source registry generation captured with the
// schemas. It is diagnostic only; Digest is the semantic envelope identity.
func (s VisibleToolSnapshot) Generation() uint64 { return s.generation }

// Profile returns the model-facing visibility contract captured in the
// snapshot.
func (s VisibleToolSnapshot) Profile() ModelToolProfile { return s.profile }

// Definitions returns a deep defensive copy of the ordered provider schemas.
func (s VisibleToolSnapshot) Definitions() []types.ToolDefinition {
	return cloneToolDefinitions(s.definitions)
}

// Names returns the ordered canonical tool names.
func (s VisibleToolSnapshot) Names() []string {
	names := make([]string, len(s.definitions))
	for i := range s.definitions {
		names[i] = s.definitions[i].Name
	}
	return names
}

// Allows reports whether name belongs to this exact immutable provider
// catalog. It is intentionally case-sensitive because provider tool names are
// protocol identifiers, not user-facing aliases.
func (s VisibleToolSnapshot) Allows(name string) bool {
	if !s.Valid() {
		return false
	}
	for i := range s.definitions {
		if s.definitions[i].Name == name {
			return true
		}
	}
	return false
}

// SnapshotVisibleTools atomically identifies one model-facing schema set. It
// retries when registration changes during schema materialization rather than
// returning a mixed-generation catalog.
func (r *Registry) SnapshotVisibleTools(loaded map[string]struct{}) (VisibleToolSnapshot, error) {
	if r == nil {
		return VisibleToolSnapshot{}, errors.New("nil registry")
	}
	for attempt := 0; attempt < 16; attempt++ {
		generation, profile := r.catalogIdentity()
		tools := r.VisibleTools(loaded)
		if profile == ModelToolProfileAgenticV2 {
			var err error
			tools, err = exactAgenticV2Tools(tools)
			if err != nil {
				return VisibleToolSnapshot{}, err
			}
		}
		definitions := types.ToDefinitions(tools)
		if afterGeneration, afterProfile := r.catalogIdentity(); generation == afterGeneration && profile == afterProfile {
			return newVisibleToolSnapshot(definitions, generation, profile)
		}
	}
	return VisibleToolSnapshot{}, errors.New("tool catalog changed continuously while snapshotting")
}

func (r *Registry) catalogIdentity() (uint64, ModelToolProfile) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.catalogGeneration, r.modelToolProfile
}

func exactAgenticV2Tools(tools []types.Tool) ([]types.Tool, error) {
	byName := make(map[string]types.Tool, len(tools))
	for _, tool := range tools {
		if tool != nil {
			byName[tool.Name()] = tool
		}
	}
	exact := make([]types.Tool, 0, len(agenticV2VisibleToolOrder))
	for _, name := range agenticV2VisibleToolOrder {
		tool := byName[name]
		if tool == nil {
			return nil, fmt.Errorf("agentic v2 visible catalog missing %s", name)
		}
		exact = append(exact, tool)
	}
	return exact, nil
}

func newVisibleToolSnapshot(definitions []types.ToolDefinition, generation uint64, profile ModelToolProfile) (VisibleToolSnapshot, error) {
	definitions = cloneToolDefinitions(definitions)
	payload := struct {
		Profile     ModelToolProfile       `json:"profile"`
		Definitions []types.ToolDefinition `json:"definitions"`
	}{Profile: profile, Definitions: definitions}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return VisibleToolSnapshot{}, fmt.Errorf("marshal visible tool catalog: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return VisibleToolSnapshot{
		definitions: definitions,
		digest:      "sha256:" + hex.EncodeToString(digest[:]),
		generation:  generation,
		profile:     profile,
	}, nil
}

func cloneToolDefinitions(in []types.ToolDefinition) []types.ToolDefinition {
	if len(in) == 0 {
		return nil
	}
	encoded, err := json.Marshal(in)
	if err != nil {
		return nil
	}
	var out []types.ToolDefinition
	if err := json.Unmarshal(encoded, &out); err != nil {
		return nil
	}
	return out
}
