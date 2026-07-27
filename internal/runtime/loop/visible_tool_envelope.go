package loop

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/agent-dance/luban/prompt"
	"github.com/agent-dance/luban/registry"
)

// refreshVisibleToolEnvelope advances a run-local immutable catalog only at a
// provider-turn boundary. Prompt blocks, provider schemas, response-chain
// fingerprinting, and prompt-cache affinity therefore move as one envelope.
func (q *QueryLoop) refreshVisibleToolEnvelope(snapshot *QueryConfigSnapshot) error {
	if q == nil || snapshot == nil {
		return nil
	}
	if q.registry == nil {
		if !snapshot.VisibleTools.Valid() {
			return nil
		}
		return errors.New("visible tool envelope has no registry")
	}
	if !snapshot.VisibleTools.Valid() {
		// Agentic V2 is fail-closed even for direct loop embeddings that omitted
		// the application-prepared snapshot. Legacy embeddings retain their
		// existing opt-in behavior, but the production exact-three profile can
		// never execute against an unbound catalog.
		if q.registry.ModelToolProfile() != registry.ModelToolProfileAgenticV2 {
			return nil
		}
		fresh, err := q.registry.SnapshotVisibleTools(q.loadedToolNames)
		if err != nil {
			return err
		}
		snapshot.VisibleTools = fresh
		if snapshot.GeneratedToolPrompt {
			blocks := prompt.BuildSystemPromptBlocksForDefinitions(fresh.Definitions(), snapshot.ToolPromptConfig)
			snapshot.SystemBlocks = blocks
			snapshot.System = blocks.JoinedText()
		}
		return nil
	}
	fresh, err := q.registry.SnapshotVisibleTools(q.loadedToolNames)
	if err != nil {
		return err
	}
	if fresh.Profile() != snapshot.VisibleTools.Profile() {
		return errors.New("visible tool profile changed during query")
	}
	if fresh.Digest() == snapshot.VisibleTools.Digest() {
		return nil
	}
	snapshot.VisibleTools = fresh
	if snapshot.GeneratedToolPrompt {
		blocks := prompt.BuildSystemPromptBlocksForDefinitions(fresh.Definitions(), snapshot.ToolPromptConfig)
		snapshot.SystemBlocks = blocks
		snapshot.System = blocks.JoinedText()
	}
	return nil
}

func catalogCacheLineage(lineage string, snapshot QueryConfigSnapshot) string {
	lineage = strings.TrimSpace(lineage)
	if lineage == "" || !snapshot.VisibleTools.Valid() {
		return lineage
	}
	digest := sha256.Sum256([]byte(lineage + "\x00" + snapshot.VisibleTools.Digest()))
	return "catalog-" + hex.EncodeToString(digest[:16])
}
