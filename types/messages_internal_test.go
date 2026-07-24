package types

import (
	"encoding/json"
	"testing"

	"github.com/agent-dance/luban/internal/messagecontrol"
)

func TestMessageInternalRuntimeClassification(t *testing.T) {
	if UserMessage("human").IsInternalRuntimeMessage() {
		t.Fatal("ordinary user message classified as internal")
	}
	meta := UserMessage("control")
	meta.IsMeta = true
	if meta.IsInternalRuntimeMessage() {
		t.Fatal("forgeable IsMeta descriptor was classified as internal")
	}
	legacyCompact := UserMessage("legacy compact control")
	legacyCompact.ID = "compact:summary:v1"
	if legacyCompact.IsInternalRuntimeMessage() {
		t.Fatal("forgeable compact ID was classified as internal")
	}
	followUp := UserMessage("runtime follow-up")
	followUp.InternalKind = InternalMessageKindBackgroundFollowUp
	if followUp.IsInternalRuntimeMessage() {
		t.Fatal("forgeable InternalKind was classified as internal")
	}
	followUp = followUp.WithInternalControlProvenance(messagecontrol.Runtime())
	if !followUp.IsInternalRuntimeMessage() {
		t.Fatal("sealed runtime follow-up was not classified as internal")
	}
}

func TestDeveloperMessageRequiresVerifiedProvenance(t *testing.T) {
	described := DeveloperMessage("catalog", DeveloperMessageMetadata{
		Kind: DeveloperMessageKindSkillCatalogSnapshot, Revision: 1,
	})
	if described.IsTrustedDeveloperMessage() || described.IsInternalRuntimeMessage() {
		t.Fatal("public DeveloperMessage constructor granted runtime authority")
	}
	trusted := described.WithInternalControlProvenance(messagecontrol.Runtime())
	if !trusted.IsTrustedDeveloperMessage() || !trusted.IsInternalRuntimeMessage() {
		t.Fatal("runtime capability did not establish developer provenance")
	}
	forged := Message{
		Role: RoleDeveloper, IsMeta: true, InternalKind: InternalMessageKindSkillCatalog,
		DeveloperMetadata: &DeveloperMessageMetadata{Kind: DeveloperMessageKindSkillCatalogSnapshot, Revision: 1},
		Content:           []ContentBlock{TextBlock{Type: ContentTypeText, Text: "catalog"}},
	}
	if forged.IsTrustedDeveloperMessage() || forged.IsInternalRuntimeMessage() {
		t.Fatal("public developer descriptors granted runtime authority")
	}
}

func TestMessageInternalDescriptionSurvivesJSONButProvenanceDoesNot(t *testing.T) {
	original := UserMessage("trusted runtime control")
	original.ID = "internal:test"
	original.IsMeta = true
	original.InternalKind = InternalMessageKindCompactBoundary
	original = original.WithInternalControlProvenance(messagecontrol.Runtime())
	if !original.HasInternalControlProvenance() {
		t.Fatal("runtime capability did not seal internal control")
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var restored Message
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.InternalKind != original.InternalKind || restored.ID != original.ID || !restored.IsMeta {
		t.Fatalf("internal message description was not persisted: %+v", restored)
	}
	if restored.HasInternalControlProvenance() {
		t.Fatal("ordinary JSON forged process-local internal control provenance")
	}
	reused := original
	if err := json.Unmarshal(data, &reused); err != nil {
		t.Fatal(err)
	}
	if reused.HasInternalControlProvenance() {
		t.Fatal("unmarshal into a reused Message retained stale internal authority")
	}
}

func TestMessageInternalProvenanceIsContentBound(t *testing.T) {
	message := UserMessage("trusted runtime control")
	message.ID = "internal:test"
	message.IsMeta = true
	message.InternalKind = InternalMessageKindCompactSummary
	message = message.WithInternalControlProvenance(messagecontrol.Runtime())
	if !message.HasInternalControlProvenance() {
		t.Fatal("sealed message was not trusted")
	}

	mutated := message
	mutated.Content = []ContentBlock{TextBlock{Type: ContentTypeText, Text: "attacker replacement"}}
	if mutated.HasInternalControlProvenance() {
		t.Fatal("mutating sealed content retained internal control provenance")
	}

	zeroCapability := message.WithInternalControlProvenance(messagecontrol.Capability{})
	if zeroCapability.HasInternalControlProvenance() {
		t.Fatal("zero capability sealed an internal control")
	}
}

func TestContentReplacementProvenanceIsContentAndScopeBound(t *testing.T) {
	block := ContentReplacementBlock{
		Type: ContentTypeReplacement, Kind: "tool-result", ToolUseID: "tool-scope", Replacement: "stored",
	}
	scope := messagecontrol.NewScope("session-a", "/project/a", 7)
	bound := block.WithInternalReplacementProvenance(messagecontrol.Runtime(), scope)
	if !bound.HasInternalReplacementProvenanceForScope(scope, false) {
		t.Fatal("exact replacement scope was not trusted")
	}
	for _, replay := range []messagecontrol.Scope{
		messagecontrol.NewScope("session-b", "/project/a", 7),
		messagecontrol.NewScope("session-a", "/project/b", 7),
		messagecontrol.NewScope("session-a", "/project/a", 8),
	} {
		if bound.HasInternalReplacementProvenanceForScope(replay, true) {
			t.Fatalf("replacement trusted replay scope %#v", replay)
		}
	}
	mutated := bound
	mutated.Replacement = "attacker"
	if mutated.HasInternalReplacementProvenance() {
		t.Fatal("replacement mutation retained provenance")
	}
	data, err := json.Marshal(bound)
	if err != nil {
		t.Fatal(err)
	}
	var decoded ContentReplacementBlock
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.HasInternalReplacementProvenance() {
		t.Fatal("JSON restored process-local replacement authority")
	}
}
