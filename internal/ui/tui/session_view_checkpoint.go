package tui

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/messagecontrol"
	"github.com/agent-dance/luban/types"
)

const (
	sessionViewCheckpointVersion               = 6
	sessionViewCheckpointOldestReadableVersion = 5
	sessionViewManifestVersion                 = 1
	// sessionViewProjectionPolicyVersion is part of the checkpoint projection
	// identity. Any change that can alter which persisted blocks are visible or
	// how they are projected must advance this value.
	sessionViewProjectionPolicyVersion = 1
	sessionViewCheckpointLimit         = 64 << 20
	sessionViewManifestLimit           = 64 << 10
	sessionViewCaptureAttempts         = 8
)

// sessionViewCheckpoint is the content-addressed semantic source for one
// settled TUI frame. It stores renderer inputs rather than terminal cells, so
// live, resume, and fork always pass through the same Root renderer.
//
// DurableSessionView is intentionally embedded instead of duplicated. Any new
// session-owned visible state must enter that one schema and is then shared by
// checkpoint persistence and SessionSnapshot publication.
type sessionViewCheckpoint struct {
	Version          int    `json:"version"`
	ViewSequence     uint64 `json:"view_sequence"`
	WriterID         string `json:"writer_id"`
	TranscriptCount  int    `json:"transcript_count"`
	TranscriptDigest string `json:"transcript_sha256"`
	// ProjectionDigest binds the frozen presentation to the active projection
	// policy, language, exact control scope, and every message's authenticated
	// trust domain. It is a plain SHA-256 digest; process-local provenance MACs
	// are deliberately neither included nor serialized.
	ProjectionDigest string                 `json:"projection_sha256,omitempty"`
	Language         string                 `json:"language,omitempty"`
	Identity         SessionIdentity        `json:"identity"`
	Messages         []Message              `json:"messages"`
	Observations     []Observation          `json:"observations"`
	Aggregates       []ObservationAggregate `json:"aggregates"`
	EvidenceManifest []DetailRef            `json:"evidence_manifest,omitempty"`
	DurableSessionView
}

// sessionViewManifest marks an artifact root as checkpoint-capable. Once
// the marker exists, a missing or incompatible exact checkpoint is an
// integrity error; silently guessing the view is forbidden.
type sessionViewManifest struct {
	Version                     int `json:"version"`
	CheckpointVersion           int `json:"checkpoint_version"`
	RequiredFromTranscriptCount int `json:"required_from_transcript_count"`
}

type sessionViewCheckpointEnvelope struct {
	Payload       json.RawMessage `json:"payload"`
	PayloadSHA256 string          `json:"payload_sha256"`
}

// SessionViewCapture is an immutable in-memory transcript/view pair. Capture
// it on the TUI event loop, then perform evidence and filesystem I/O after the
// event loop is released.
type SessionViewCapture struct {
	View         DurableSessionView
	Observations []Observation
	checkpoint   sessionViewCheckpoint
	details      DetailStore
}

var sessionViewWriteLocks sync.Map

func newSessionViewWriterID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return ""
	}
	return hex.EncodeToString(value[:])
}

func sessionViewWriteLock(path string) *sync.Mutex {
	value, _ := sessionViewWriteLocks.LoadOrStore(path, &sync.Mutex{})
	return value.(*sync.Mutex)
}

// SaveSessionViewCheckpoint materializes one exact, durable transcript/view
// pair. Referenced evidence is first copied into the session-local immutable
// detail store; the checkpoint and capability manifest are published last.
func SaveSessionViewCheckpoint(artifactRoot string, state *AppState, transcript []types.Message) error {
	if state == nil || strings.TrimSpace(artifactRoot) == "" || strings.TrimSpace(state.SessionID.Get()) == "" {
		return nil
	}
	capture, err := CaptureSessionViewCheckpoint(state, transcript)
	if err != nil {
		return err
	}
	return SaveSessionViewCapture(artifactRoot, capture)
}

// CaptureSessionViewCheckpoint performs no filesystem I/O. Production callers
// with a running App must invoke it through App.UpdateSync so the whole clone
// is one event-loop transaction.
func CaptureSessionViewCheckpoint(state *AppState, transcript []types.Message) (SessionViewCapture, error) {
	if state == nil || strings.TrimSpace(state.SessionID.Get()) == "" {
		return SessionViewCapture{}, nil
	}
	checkpoint, details, err := captureStableSessionViewCheckpoint(state, transcript)
	if err != nil {
		return SessionViewCapture{}, err
	}
	return SessionViewCapture{
		View: cloneDurableSessionView(checkpoint.DurableSessionView), Observations: cloneObservations(checkpoint.Observations),
		checkpoint: checkpoint, details: details,
	}, nil
}

func cloneObservations(source []Observation) []Observation {
	out := make([]Observation, len(source))
	for index := range source {
		out[index] = cloneObservation(source[index])
	}
	return out
}

// SaveSessionViewCapture publishes a previously frozen capture.
func SaveSessionViewCapture(artifactRoot string, capture SessionViewCapture) error {
	if strings.TrimSpace(artifactRoot) == "" || strings.TrimSpace(capture.checkpoint.Identity.SessionID) == "" {
		return nil
	}
	checkpoint := capture.checkpoint
	lock := sessionViewWriteLock(sessionViewCheckpointPath(artifactRoot, checkpoint.TranscriptCount, checkpoint.TranscriptDigest))
	lock.Lock()
	defer lock.Unlock()
	current, exists, err := readSessionViewCheckpoint(artifactRoot, checkpoint.TranscriptCount, checkpoint.TranscriptDigest)
	if err != nil {
		return err
	}
	if exists {
		if current.ViewSequence > checkpoint.ViewSequence ||
			(current.ViewSequence == checkpoint.ViewSequence && current.WriterID != checkpoint.WriterID) {
			return i18n.NewError(i18n.KeyTUISessionViewStaleCapture)
		}
		if current.ViewSequence == checkpoint.ViewSequence && current.WriterID == checkpoint.WriterID {
			return writeSessionViewManifest(artifactRoot, checkpoint.TranscriptCount)
		}
	}
	if err := materializeSessionViewEvidence(artifactRoot, capture.details, &checkpoint); err != nil {
		return err
	}
	if err := writeSessionViewCheckpoint(artifactRoot, checkpoint); err != nil {
		return err
	}
	return writeSessionViewManifest(artifactRoot, checkpoint.TranscriptCount)
}

// LoadSessionViewCheckpoint prepares the exact persisted snapshot for a
// transcript and session identity.
func LoadSessionViewCheckpoint(artifactRoot string, transcript []types.Message, identity SessionIdentity) (SessionSnapshot, bool, error) {
	return loadSessionViewCheckpointInLanguage(i18n.DetectOrLoadLanguage(), artifactRoot, transcript, identity)
}

func loadSessionViewCheckpointInLanguage(lang i18n.Language, artifactRoot string, transcript []types.Message, identity SessionIdentity) (SessionSnapshot, bool, error) {
	if strings.TrimSpace(artifactRoot) == "" {
		return SessionSnapshot{}, false, nil
	}
	manifest, capable, err := readSessionViewManifest(artifactRoot)
	if err != nil {
		return SessionSnapshot{}, false, err
	}
	digest, err := sessionViewTranscriptDigest(transcript)
	if err != nil {
		return SessionSnapshot{}, false, err
	}
	checkpoint, ok, err := readSessionViewCheckpoint(artifactRoot, len(transcript), digest)
	if err != nil {
		return SessionSnapshot{}, false, err
	}
	if !ok {
		if capable && len(transcript) >= manifest.RequiredFromTranscriptCount {
			return SessionSnapshot{}, false, i18n.NewError(i18n.KeyTUISessionViewMissingCheckpoint)
		}
		return SessionSnapshot{}, false, nil
	}
	if checkpoint.Identity.SessionID != identity.SessionID || checkpoint.Identity.Namespace != identity.Namespace {
		return SessionSnapshot{}, false, i18n.NewError(i18n.KeyTUISessionViewIdentityMismatch)
	}
	details, err := validateSessionViewEvidence(artifactRoot, checkpoint)
	if err != nil {
		return SessionSnapshot{}, false, err
	}
	snapshot := SessionSnapshot{Identity: identity}
	projectionDigest, projectionErr := sessionViewProjectionDigest(lang, identity, transcript)
	if projectionErr != nil {
		return SessionSnapshot{}, false, projectionErr
	}
	if checkpoint.ProjectionDigest == projectionDigest {
		applyCheckpointToSnapshot(checkpoint, details, &snapshot)
	} else {
		// The durable interaction state remains usable, but the old message and
		// observation projection never enters the prepared snapshot when its
		// authority context differs. Re-project directly under the current scope.
		projection, projectionErr := ProjectPersistedMessagesInLanguage(lang, identity, transcript, details)
		if projectionErr != nil {
			return SessionSnapshot{}, false, projectionErr
		}
		snapshot.Projection = projection
		snapshot.ViewSequence = checkpoint.ViewSequence
		snapshot.DurableSessionView = cloneDurableSessionView(checkpoint.DurableSessionView)
	}
	return snapshot, true, nil
}

// ForkSessionViewCheckpoint copies the selected source-prefix view into a new
// session identity. All reachable evidence is copied and verified before the
// target checkpoint is published, so the fork never depends on source files.
func ForkSessionViewCheckpoint(
	sourceRoot, targetRoot string,
	sourceIdentity, targetIdentity SessionIdentity,
	sourceTranscript, targetTranscript []types.Message,
) error {
	if strings.TrimSpace(sourceRoot) == "" || strings.TrimSpace(targetRoot) == "" {
		return nil
	}
	manifest, capable, err := readSessionViewManifest(sourceRoot)
	if err != nil {
		return err
	}
	sourceDigest, err := sessionViewTranscriptDigest(sourceTranscript)
	if err != nil {
		return err
	}
	checkpoint, ok, err := readSessionViewCheckpoint(sourceRoot, len(sourceTranscript), sourceDigest)
	if err != nil {
		return err
	}
	if !ok {
		if capable && len(sourceTranscript) >= manifest.RequiredFromTranscriptCount {
			return i18n.NewError(i18n.KeyTUISessionViewMissingCheckpoint)
		}
		return nil
	}
	if checkpoint.Identity.SessionID != sourceIdentity.SessionID || checkpoint.Identity.Namespace != sourceIdentity.Namespace {
		return i18n.NewError(i18n.KeyTUISessionViewIdentityMismatch)
	}
	if !sourceIdentity.controlScopeSet && sessionViewTranscriptHasAuthenticatedControl(sourceTranscript) {
		return i18n.NewError(i18n.KeyTUISessionViewInvalidCheckpoint)
	}
	// Forking preserves session-local presentation rows that cannot be rebuilt
	// from the model transcript. That makes a permissive re-projection unsafe:
	// before any IDs, evidence refs, or receipts are remapped, require the source
	// checkpoint to have been captured under this exact language, projection
	// policy, message trust domain, and authoritative control generation.
	sourceProjectionDigest, err := sessionViewProjectionDigest(
		languageForSessionViewCheckpoint(checkpoint), sourceIdentity, sourceTranscript,
	)
	if err != nil {
		return err
	}
	details, err := validateSessionViewEvidence(sourceRoot, checkpoint)
	if err != nil {
		return err
	}
	sourceProjectionRebuilt := checkpoint.ProjectionDigest != sourceProjectionDigest
	if sourceProjectionRebuilt {
		// A historical prefix may have a legitimate local interaction snapshot
		// but an obsolete control generation. Keep the non-authority-bearing
		// durable interaction state, discard the entire frozen transcript
		// projection, and rebuild it under the caller's current source scope.
		projection, projectionErr := ProjectPersistedMessagesInLanguage(
			languageForSessionViewCheckpoint(checkpoint), sourceIdentity, sourceTranscript, details,
		)
		if projectionErr != nil {
			return projectionErr
		}
		replaceSessionViewCheckpointProjection(&checkpoint, projection)
		sanitizeReprojectedCheckpointReferences(&checkpoint)
		checkpoint.EvidenceManifest = collectCheckpointDetailRefs(checkpoint)
	}
	targetDigest, err := sessionViewTranscriptDigest(targetTranscript)
	if err != nil {
		return err
	}
	if !targetIdentity.controlScopeSet && sessionViewTranscriptHasAuthenticatedControl(targetTranscript) {
		return i18n.NewError(i18n.KeyTUISessionViewInvalidCheckpoint)
	}
	preserveSourceProjection, err := sessionViewForkProjectionsEquivalent(
		languageForSessionViewCheckpoint(checkpoint), sourceIdentity, targetIdentity, sourceTranscript, targetTranscript,
	)
	if err != nil {
		return err
	}
	// A source digest mismatch invalidates
	// the frozen source projection as a whole. Even if a freshly rebuilt source
	// baseline happens to compare equal to the target baseline, do not use that
	// equality to carry the source projection through the fork. Remap only the
	// independently durable checkpoint state, then install the target's fresh
	// projection below.
	preserveSourceProjection = preserveSourceProjection && !sourceProjectionRebuilt
	var targetProjection SessionProjection
	if !preserveSourceProjection {
		targetProjection, err = ProjectPersistedMessagesInLanguage(
			languageForSessionViewCheckpoint(checkpoint), targetIdentity, targetTranscript, details,
		)
		if err != nil {
			return err
		}
	}
	remapSessionViewCheckpoint(&checkpoint, sourceIdentity, targetIdentity, sourceTranscript, targetTranscript)
	if !preserveSourceProjection {
		// The target transcript is the authority for target visibility. Never
		// bless a remapped source projection with a target digest when its trust
		// semantics differ (for example trusted -> untrusted or the reverse).
		replaceSessionViewCheckpointProjection(&checkpoint, targetProjection)
		sanitizeReprojectedCheckpointReferences(&checkpoint)
		checkpoint.EvidenceManifest = collectCheckpointDetailRefs(checkpoint)
	}
	checkpoint.TranscriptCount = len(targetTranscript)
	checkpoint.TranscriptDigest = targetDigest
	targetProjectionDigest, err := sessionViewProjectionDigest(languageForSessionViewCheckpoint(checkpoint), targetIdentity, targetTranscript)
	if err != nil {
		return err
	}
	checkpoint.ProjectionDigest = targetProjectionDigest
	if err := copySessionViewEvidence(sourceRoot, targetRoot, &checkpoint); err != nil {
		return err
	}
	if err := writeSessionViewCheckpoint(targetRoot, checkpoint); err != nil {
		return err
	}
	return writeSessionViewManifest(targetRoot, len(targetTranscript))
}

func captureStableSessionViewCheckpoint(state *AppState, transcript []types.Message) (sessionViewCheckpoint, DetailStore, error) {
	for attempt := 0; attempt < sessionViewCaptureAttempts; attempt++ {
		before := state.SessionLifecycleRevision()
		checkpoint, details, err := captureSessionViewCheckpoint(state, transcript)
		if err != nil {
			return sessionViewCheckpoint{}, nil, err
		}
		after := state.SessionLifecycleRevision()
		if before == after && checkpointIdentityMatchesState(checkpoint, state) {
			return checkpoint, details, nil
		}
	}
	return sessionViewCheckpoint{}, nil, i18n.NewError(i18n.KeyTUISessionViewUnstableCheckpoint)
}

func captureSessionViewCheckpoint(state *AppState, transcript []types.Message) (sessionViewCheckpoint, DetailStore, error) {
	identity := sessionViewIdentityForState(state)
	language := state.Language.Get()
	digest, err := sessionViewTranscriptDigest(transcript)
	if err != nil {
		return sessionViewCheckpoint{}, nil, err
	}
	projectionDigest, err := sessionViewProjectionDigest(language, identity, transcript)
	if err != nil {
		return sessionViewCheckpoint{}, nil, err
	}
	messages := state.Messages.Get()
	checkpointMessages := make([]Message, len(messages))
	for index := range messages {
		checkpointMessages[index] = clonePresentationMessage(messages[index])
		checkpointMessages[index].Stream = nil
		for imageIndex := range checkpointMessages[index].Images {
			// Sent image payloads already live in the model transcript/artifact
			// store. Rendering only needs their stable tag and media type.
			checkpointMessages[index].Images[imageIndex].Base64 = ""
		}
	}
	observations := state.ObservationSnapshot()
	disclosureByObservation := make(map[string]DisclosureState, len(observations))
	for index := range observations {
		observations[index] = resetProcessLocalObservationDisclosure(observations[index])
		disclosureByObservation[observations[index].ID] = observations[index].Disclosure
	}
	for index := range checkpointMessages {
		if disclosure, ok := disclosureByObservation[checkpointMessages[index].ObservationID]; ok {
			checkpointMessages[index].Disclosure = disclosure
		}
	}
	view := state.DurableSessionViewSnapshot()
	if !view.Interaction.InputCursorSet {
		view.Interaction.InputCursor = len([]rune(view.Interaction.InputDraft))
		view.Interaction.InputCursorSet = true
	}
	for id, interaction := range view.DisclosureReturns {
		if !interaction.InputCursorSet {
			interaction.InputCursor = len([]rune(interaction.InputDraft))
			interaction.InputCursorSet = true
			view.DisclosureReturns[id] = interaction
		}
	}
	state.mu.Lock()
	details := state.Details
	state.checkpointSequence++
	viewSequence := state.checkpointSequence
	writerID := state.checkpointWriterID
	state.mu.Unlock()
	return sessionViewCheckpoint{
		Version: sessionViewCheckpointVersion, ViewSequence: viewSequence, WriterID: writerID,
		TranscriptCount: len(transcript), TranscriptDigest: digest, ProjectionDigest: projectionDigest,
		Language: language.Code(),
		Identity: identity,
		Messages: checkpointMessages, Observations: observations,
		Aggregates:         state.ObservationAggregateSnapshot(),
		DurableSessionView: view,
	}, details, nil
}

func applyCheckpointToSnapshot(checkpoint sessionViewCheckpoint, details DetailStore, snapshot *SessionSnapshot) {
	snapshot.Projection.Messages = make([]Message, len(checkpoint.Messages))
	for index := range checkpoint.Messages {
		snapshot.Projection.Messages[index] = clonePresentationMessage(checkpoint.Messages[index])
		snapshot.Projection.Messages[index].Stream = nil
	}
	snapshot.Projection.Observations = make([]Observation, len(checkpoint.Observations))
	for index := range checkpoint.Observations {
		snapshot.Projection.Observations[index] = resetProcessLocalObservationDisclosure(cloneObservation(checkpoint.Observations[index]))
	}
	disclosureByObservation := make(map[string]DisclosureState, len(snapshot.Projection.Observations))
	for index := range snapshot.Projection.Observations {
		disclosureByObservation[snapshot.Projection.Observations[index].ID] = snapshot.Projection.Observations[index].Disclosure
	}
	for index := range snapshot.Projection.Messages {
		if disclosure, ok := disclosureByObservation[snapshot.Projection.Messages[index].ObservationID]; ok {
			snapshot.Projection.Messages[index].Disclosure = disclosure
		}
	}
	snapshot.Projection.Aggregates = cloneObservationAggregates(checkpoint.Aggregates)
	snapshot.Projection.Details = details
	snapshot.ViewSequence = checkpoint.ViewSequence
	snapshot.DurableSessionView = cloneDurableSessionView(checkpoint.DurableSessionView)
}

func cloneDurableSessionView(view DurableSessionView) DurableSessionView {
	view.Goal = cloneGoalViewState(view.Goal)
	view.DisclosureReturns = cloneInteractionMap(view.DisclosureReturns)
	view.ToolSegmentExpansion = cloneBoolMap(view.ToolSegmentExpansion)
	view.Decisions = append([]DecisionRecord(nil), view.Decisions...)
	view.Activities = append([]Activity(nil), view.Activities...)
	for index := range view.Activities {
		view.Activities[index].Actions = append([]ActivityAction(nil), view.Activities[index].Actions...)
		view.Activities[index].Control.DetailRefs = append([]DetailRef(nil), view.Activities[index].Control.DetailRefs...)
	}
	view.TaskViewItems = append([]TaskViewItem(nil), view.TaskViewItems...)
	for index := range view.TaskViewItems {
		view.TaskViewItems[index].BlockedBy = append([]string(nil), view.TaskViewItems[index].BlockedBy...)
	}
	view.PendingImages = append([]ImageAttachment(nil), view.PendingImages...)
	return view
}

func sanitizeReprojectedCheckpointReferences(checkpoint *sessionViewCheckpoint) {
	if checkpoint == nil {
		return
	}
	validObservations := make(map[string]struct{}, len(checkpoint.Observations))
	for _, observation := range checkpoint.Observations {
		validObservations[observation.ID] = struct{}{}
	}
	keepObservation := func(id string) string {
		if _, ok := validObservations[id]; ok {
			return id
		}
		return ""
	}
	checkpoint.Interaction.FocusedObservationID = keepObservation(checkpoint.Interaction.FocusedObservationID)
	checkpoint.Interaction.ScrollAnchorID = keepObservation(checkpoint.Interaction.ScrollAnchorID)
	returns := make(map[string]SessionInteraction)
	for id, interaction := range checkpoint.DisclosureReturns {
		if _, ok := validObservations[id]; !ok {
			continue
		}
		interaction.FocusedObservationID = keepObservation(interaction.FocusedObservationID)
		interaction.ScrollAnchorID = keepObservation(interaction.ScrollAnchorID)
		returns[id] = interaction
	}
	if len(returns) == 0 {
		checkpoint.DisclosureReturns = nil
	} else {
		checkpoint.DisclosureReturns = returns
	}
	for index := range checkpoint.Activities {
		checkpoint.Activities[index].Control.JumpTarget = keepObservation(checkpoint.Activities[index].Control.JumpTarget)
	}
	validSegments := make(map[string]struct{})
	for _, id := range checkpointToolSegmentIDs(checkpoint.Messages) {
		validSegments[id] = struct{}{}
	}
	expansion := make(map[string]bool)
	for id, expanded := range checkpoint.ToolSegmentExpansion {
		if _, ok := validSegments[id]; ok {
			expansion[id] = expanded
		}
	}
	if len(expansion) == 0 {
		checkpoint.ToolSegmentExpansion = nil
	} else {
		checkpoint.ToolSegmentExpansion = expansion
	}
}

func replaceSessionViewCheckpointProjection(checkpoint *sessionViewCheckpoint, projection SessionProjection) {
	checkpoint.Messages = make([]Message, len(projection.Messages))
	for index := range projection.Messages {
		checkpoint.Messages[index] = clonePresentationMessage(projection.Messages[index])
	}
	checkpoint.Observations = cloneObservations(projection.Observations)
	checkpoint.Aggregates = cloneObservationAggregates(projection.Aggregates)
}

func cloneGoalViewState(goal *GoalViewState) *GoalViewState {
	if goal == nil {
		return nil
	}
	copy := *goal
	copy.Criteria = append([]GoalCriterionViewState(nil), goal.Criteria...)
	return &copy
}

func sessionViewTranscriptDigest(transcript []types.Message) (string, error) {
	// Nil and an allocated empty slice are the same durable transcript. Session
	// decoding intentionally returns an allocated empty slice, while callers
	// creating or resuming an empty session may naturally pass nil.
	if len(transcript) == 0 {
		transcript = []types.Message{}
	}
	encoded, err := json.Marshal(transcript)
	if err != nil {
		return "", i18n.WrapError(i18n.KeyTUISessionViewMarshalTranscript, err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

type sessionViewProjectionScopeIdentity struct {
	Bound             bool   `json:"bound"`
	SessionID         string `json:"session_id,omitempty"`
	ProjectScope      string `json:"project_scope,omitempty"`
	ContextGeneration uint64 `json:"context_generation,omitempty"`
}

type sessionViewMessageTrustIdentity struct {
	// Domain is a closed protocol value: 0 is untrusted, 1 is authenticated
	// and unbound, 2 is authenticated in the exact current scope, and 3 is
	// authenticated in another (stale or foreign) scope.
	Domain             uint8                              `json:"domain"`
	AuthenticatedScope sessionViewProjectionScopeIdentity `json:"authenticated_scope"`
}

type sessionViewProjectionIdentity struct {
	SchemaVersion    int                                `json:"schema_version"`
	PolicyVersion    int                                `json:"policy_version"`
	Language         string                             `json:"language"`
	Namespace        string                             `json:"namespace"`
	SessionID        string                             `json:"session_id"`
	AllowUnbound     bool                               `json:"allow_unbound"`
	CurrentScope     sessionViewProjectionScopeIdentity `json:"current_scope"`
	TranscriptDigest string                             `json:"transcript_sha256"`
	MessageTrust     []sessionViewMessageTrustIdentity  `json:"message_trust"`
}

// sessionViewProjectionDigest is deliberately distinct from the serialized
// transcript digest. Message JSON cannot carry process-local provenance, so a
// JSON-only cache key aliases authenticated controls with forged lookalikes
// and aliases current controls with stale generations. This identity records
// only the verified trust domain and public scope tuple; it never records the
// HMAC used to prove that domain.
func sessionViewProjectionDigest(lang i18n.Language, identity SessionIdentity, transcript []types.Message) (string, error) {
	transcriptDigest, err := sessionViewTranscriptDigest(transcript)
	if err != nil {
		return "", err
	}
	wire := sessionViewProjectionIdentity{
		SchemaVersion: sessionViewCheckpointVersion,
		PolicyVersion: sessionViewProjectionPolicyVersion,
		Language:      lang.Code(), Namespace: identity.Namespace, SessionID: identity.SessionID,
		AllowUnbound:     !identity.controlScopeSet,
		TranscriptDigest: transcriptDigest,
		MessageTrust:     make([]sessionViewMessageTrustIdentity, len(transcript)),
	}
	if identity.controlScopeSet {
		wire.CurrentScope = sessionViewProjectionScope(identity.controlScope)
	}
	for index, message := range transcript {
		wire.MessageTrust[index] = sessionViewMessageTrust(message, identity)
	}
	encoded, err := json.Marshal(wire)
	if err != nil {
		return "", i18n.WrapError(i18n.KeyTUISessionViewMarshalTranscript, err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func sessionViewMessageTrust(message types.Message, identity SessionIdentity) sessionViewMessageTrustIdentity {
	if !message.HasInternalControlProvenance() {
		return sessionViewMessageTrustIdentity{}
	}
	scope, bound := message.InternalControlProvenanceScope()
	if !bound {
		return sessionViewMessageTrustIdentity{Domain: 1}
	}
	domain := uint8(3)
	if identity.controlScopeSet && scope.Equal(identity.controlScope) {
		domain = 2
	}
	return sessionViewMessageTrustIdentity{
		Domain: domain, AuthenticatedScope: sessionViewProjectionScope(scope),
	}
}

func sessionViewTranscriptHasAuthenticatedControl(transcript []types.Message) bool {
	for _, message := range transcript {
		if message.HasInternalControlProvenance() {
			return true
		}
	}
	return false
}

func sessionViewForkProjectionsEquivalent(
	lang i18n.Language,
	sourceIdentity, targetIdentity SessionIdentity,
	sourceTranscript, targetTranscript []types.Message,
) (bool, error) {
	source, err := ProjectPersistedMessagesInLanguage(lang, sourceIdentity, sourceTranscript, NewMemoryDetailStore())
	if err != nil {
		return false, err
	}
	target, err := ProjectPersistedMessagesInLanguage(lang, targetIdentity, targetTranscript, NewMemoryDetailStore())
	if err != nil {
		return false, err
	}
	sourceCheckpoint := sessionViewCheckpoint{
		Identity: sourceIdentity, Messages: source.Messages, Observations: source.Observations, Aggregates: source.Aggregates,
	}
	remapSessionViewCheckpoint(&sourceCheckpoint, sourceIdentity, targetIdentity, sourceTranscript, targetTranscript)
	targetCheckpoint := sessionViewCheckpoint{
		Identity: targetIdentity, Messages: target.Messages, Observations: target.Observations, Aggregates: target.Aggregates,
	}
	normalizeCheckpointProjectionForComparison(&sourceCheckpoint)
	normalizeCheckpointProjectionForComparison(&targetCheckpoint)
	return reflect.DeepEqual(sourceCheckpoint.Messages, targetCheckpoint.Messages) &&
		reflect.DeepEqual(sourceCheckpoint.Observations, targetCheckpoint.Observations) &&
		reflect.DeepEqual(sourceCheckpoint.Aggregates, targetCheckpoint.Aggregates), nil
}

func normalizeCheckpointProjectionForComparison(checkpoint *sessionViewCheckpoint) {
	for index := range checkpoint.Messages {
		checkpoint.Messages[index].DetailRefs = nil
		checkpoint.Messages[index].Stream = nil
		checkpoint.Messages[index].SessionID = ""
		checkpoint.Messages[index].TurnID = ""
		checkpoint.Messages[index].WorkUnitID = ""
		checkpoint.Messages[index].ActorID = ""
	}
	for index := range checkpoint.Observations {
		checkpoint.Observations[index].ResultRefs = nil
		checkpoint.Observations[index].EnvelopeRefs = nil
		checkpoint.Observations[index].PresentationID = ""
		checkpoint.Observations[index].PresentationWorkUnitID = ""
		checkpoint.Observations[index].PresentationActorID = ""
	}
	for index := range checkpoint.Aggregates {
		checkpoint.Aggregates[index].EvidenceRefs = nil
	}
}

func sessionViewProjectionScope(scope messagecontrol.Scope) sessionViewProjectionScopeIdentity {
	if !scope.Bound() {
		return sessionViewProjectionScopeIdentity{}
	}
	return sessionViewProjectionScopeIdentity{
		Bound: true, SessionID: scope.SessionID(), ProjectScope: scope.ProjectScope(),
		ContextGeneration: scope.ContextGeneration(),
	}
}

func sessionViewIdentityForState(state *AppState) SessionIdentity {
	identity := SessionIdentity{
		Namespace: state.SessionNS.Get(), SessionID: state.SessionID.Get(), Epoch: state.SessionEpoch.Get(),
	}
	generation := state.ContextGeneration.Get()
	if state.ContextGenerationPersisted.Get() && generation != 0 {
		identity = identity.WithInternalControlScope(messagecontrol.Runtime(), messagecontrol.NewScope(
			identity.SessionID, identity.Namespace, generation,
		))
	}
	return identity
}

func checkpointIdentityMatchesState(checkpoint sessionViewCheckpoint, state *AppState) bool {
	current := sessionViewIdentityForState(state)
	if checkpoint.Identity.Namespace != current.Namespace || checkpoint.Identity.SessionID != current.SessionID ||
		checkpoint.Identity.Epoch != current.Epoch || checkpoint.Identity.controlScopeSet != current.controlScopeSet ||
		checkpoint.Language != state.Language.Get().Code() {
		return false
	}
	return !current.controlScopeSet || checkpoint.Identity.controlScope.Equal(current.controlScope)
}

func languageForSessionViewCheckpoint(checkpoint sessionViewCheckpoint) i18n.Language {
	for _, lang := range i18n.AllLanguages() {
		if lang.Code() == checkpoint.Language {
			return lang
		}
	}
	return i18n.LangEN
}

func sessionViewCheckpointPath(root string, transcriptCount int, digest string) string {
	return filepath.Join(root, "tui-view", "checkpoints", checkpointFileName(transcriptCount, digest))
}

func validSessionViewCheckpointIdentity(transcriptCount int, digest string) bool {
	if transcriptCount < 0 || len(digest) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(digest)
	return err == nil
}

func checkpointFileName(transcriptCount int, digest string) string {
	return zeroPadCheckpointCount(transcriptCount) + "-" + digest + ".json"
}

func zeroPadCheckpointCount(value int) string {
	const width = 8
	digits := strconv.Itoa(value)
	if len(digits) < width {
		return strings.Repeat("0", width-len(digits)) + digits
	}
	return digits
}

func sessionViewManifestPath(root string) string {
	return filepath.Join(root, "tui-view", "manifest.json")
}

func writeSessionViewCheckpoint(root string, checkpoint sessionViewCheckpoint) error {
	payload, err := json.Marshal(checkpoint)
	if err != nil {
		return i18n.WrapError(i18n.KeyTUISessionViewMarshalCheckpoint, err)
	}
	payloadDigest := sha256.Sum256(payload)
	encoded, err := json.Marshal(sessionViewCheckpointEnvelope{
		Payload: payload, PayloadSHA256: hex.EncodeToString(payloadDigest[:]),
	})
	if err != nil {
		return i18n.WrapError(i18n.KeyTUISessionViewMarshalCheckpoint, err)
	}
	if len(payload) > sessionViewCheckpointLimit || len(encoded) > sessionViewCheckpointLimit {
		return i18n.NewError(i18n.KeyTUISessionViewInvalidCheckpoint)
	}
	if !validSessionViewCheckpointIdentity(checkpoint.TranscriptCount, checkpoint.TranscriptDigest) {
		return i18n.NewError(i18n.KeyTUISessionViewInvalidCheckpoint)
	}
	path := sessionViewCheckpointPath(root, checkpoint.TranscriptCount, checkpoint.TranscriptDigest)
	return writeSessionViewFile(path, encoded)
}

func writeSessionViewManifest(root string, transcriptCount int) error {
	requiredFrom := transcriptCount
	if current, ok, err := readSessionViewManifest(root); err != nil {
		return err
	} else if ok && current.RequiredFromTranscriptCount < requiredFrom {
		requiredFrom = current.RequiredFromTranscriptCount
	}
	encoded, err := json.Marshal(sessionViewManifest{
		Version: sessionViewManifestVersion, CheckpointVersion: sessionViewCheckpointVersion,
		RequiredFromTranscriptCount: requiredFrom,
	})
	if err != nil {
		return i18n.WrapError(i18n.KeyTUISessionViewMarshalCheckpoint, err)
	}
	return writeSessionViewFile(sessionViewManifestPath(root), encoded)
}

func writeSessionViewFile(path string, encoded []byte) error {
	dir := filepath.Dir(path)
	if err := ensurePrivateDirectory(dir); err != nil {
		return i18n.WrapError(i18n.KeyTUISessionViewPrepareCheckpointDir, err)
	}
	if err := writePrivateAtomic(path, append(encoded, '\n')); err != nil {
		return i18n.WrapError(i18n.KeyTUISessionViewPublishCheckpoint, err)
	}
	if err := syncDirectory(dir); err != nil {
		return i18n.WrapError(i18n.KeyTUISessionViewSyncCheckpoint, err)
	}
	return nil
}

func readSessionViewManifest(root string) (sessionViewManifest, bool, error) {
	var manifest sessionViewManifest
	ok, err := readSessionViewJSON(sessionViewManifestPath(root), sessionViewManifestLimit, &manifest)
	if err != nil || !ok {
		return sessionViewManifest{}, false, err
	}
	if manifest.Version != sessionViewManifestVersion {
		return sessionViewManifest{}, false, i18n.NewError(i18n.KeyTUISessionViewUnsupportedVersion, manifest.Version, sessionViewManifestVersion)
	}
	if !supportedSessionViewCheckpointVersion(manifest.CheckpointVersion) {
		return sessionViewManifest{}, false, i18n.NewError(i18n.KeyTUISessionViewUnsupportedVersion, manifest.CheckpointVersion, sessionViewCheckpointVersion)
	}
	if manifest.RequiredFromTranscriptCount < 0 {
		return sessionViewManifest{}, false, i18n.NewError(i18n.KeyTUISessionViewInvalidCheckpoint)
	}
	return manifest, true, nil
}

func readSessionViewCheckpoint(root string, count int, digest string) (sessionViewCheckpoint, bool, error) {
	if !validSessionViewCheckpointIdentity(count, digest) {
		return sessionViewCheckpoint{}, false, i18n.NewError(i18n.KeyTUISessionViewInvalidCheckpoint)
	}
	path := sessionViewCheckpointPath(root, count, digest)
	var envelope sessionViewCheckpointEnvelope
	ok, err := readSessionViewJSON(path, sessionViewCheckpointLimit, &envelope)
	if err != nil || !ok {
		return sessionViewCheckpoint{}, ok, err
	}
	actualPayloadDigest := sha256.Sum256(envelope.Payload)
	if envelope.PayloadSHA256 != hex.EncodeToString(actualPayloadDigest[:]) {
		return sessionViewCheckpoint{}, false, i18n.NewError(i18n.KeyTUISessionViewInvalidCheckpoint)
	}
	var header struct {
		Version *int `json:"version"`
	}
	if err := json.Unmarshal(envelope.Payload, &header); err != nil {
		return sessionViewCheckpoint{}, false, i18n.WrapError(i18n.KeyTUISessionViewDecodeCheckpointFile, err)
	}
	if header.Version == nil {
		return sessionViewCheckpoint{}, false, i18n.NewError(i18n.KeyTUISessionViewInvalidCheckpoint)
	}
	if !supportedSessionViewCheckpointVersion(*header.Version) {
		return sessionViewCheckpoint{}, false, i18n.NewError(i18n.KeyTUISessionViewUnsupportedVersion, *header.Version, sessionViewCheckpointVersion)
	}
	var checkpoint sessionViewCheckpoint
	decoder := json.NewDecoder(bytes.NewReader(envelope.Payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&checkpoint); err != nil {
		return sessionViewCheckpoint{}, false, i18n.WrapError(i18n.KeyTUISessionViewDecodeCheckpointFile, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return sessionViewCheckpoint{}, false, i18n.NewError(i18n.KeyTUISessionViewTrailingCheckpointData)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(envelope.Payload, &fields); err != nil {
		return sessionViewCheckpoint{}, false, i18n.WrapError(i18n.KeyTUISessionViewDecodeCheckpointFile, err)
	}
	if _, ok := fields["session_cost_known"]; !ok {
		return sessionViewCheckpoint{}, false, i18n.NewError(i18n.KeyTUISessionViewInvalidCheckpoint)
	}
	if !validSessionViewDigest(checkpoint.ProjectionDigest) {
		return sessionViewCheckpoint{}, false, i18n.NewError(i18n.KeyTUISessionViewInvalidCheckpoint)
	}
	if checkpoint.TranscriptCount != count || checkpoint.TranscriptDigest != digest {
		return sessionViewCheckpoint{}, false, i18n.NewError(i18n.KeyTUISessionViewInvalidCheckpoint)
	}
	for index := range checkpoint.Activities {
		normalized, normalizeErr := normalizeActivityEvent(checkpoint.Activities[index].ActivityEvent)
		if normalizeErr != nil {
			return sessionViewCheckpoint{}, false, normalizeErr
		}
		checkpoint.Activities[index].ActivityEvent = normalized
	}
	upgradeSessionViewCheckpoint(&checkpoint)
	return checkpoint, true, nil
}

func supportedSessionViewCheckpointVersion(version int) bool {
	return version >= sessionViewCheckpointOldestReadableVersion && version <= sessionViewCheckpointVersion
}

func upgradeSessionViewCheckpoint(checkpoint *sessionViewCheckpoint) {
	if checkpoint == nil || checkpoint.Version != sessionViewCheckpointOldestReadableVersion {
		return
	}
	completenessByObservation := make(map[string]types.ToolResultCompleteness, len(checkpoint.Observations))
	completenessByToolUse := make(map[string]types.ToolResultCompleteness, len(checkpoint.Observations))
	for _, observation := range checkpoint.Observations {
		completeness := observation.Presentation.Completeness.Clone()
		if observation.ID != "" {
			completenessByObservation[observation.ID] = completeness
		}
		if observation.ToolUseID != "" {
			completenessByToolUse[observation.ToolUseID] = completeness
		}
	}
	for index := range checkpoint.Messages {
		message := &checkpoint.Messages[index]
		if completeness, ok := completenessByObservation[message.ObservationID]; ok {
			message.Completeness = completeness.Clone()
		} else if completeness, ok := completenessByToolUse[message.ToolUseID]; ok {
			message.Completeness = completeness.Clone()
		}
		if observationIsNormalPagination(message.Outcome, message.Completeness) {
			message.IsError = false
		}
	}
	checkpoint.Version = sessionViewCheckpointVersion
}

func validSessionViewDigest(digest string) bool {
	if len(digest) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(digest)
	return err == nil
}

func readSessionViewJSON(path string, limit int64, target any) (bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, i18n.WrapError(i18n.KeyTUISessionViewOpenCheckpoint, err)
	}
	if !info.Mode().IsRegular() || !privateFilePermissionsValid(info) || info.Size() < 0 || info.Size() > limit {
		return false, i18n.NewError(i18n.KeyTUISessionViewInvalidCheckpoint)
	}
	file, err := os.Open(path)
	if err != nil {
		return false, i18n.WrapError(i18n.KeyTUISessionViewOpenCheckpoint, err)
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, limit+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return false, i18n.WrapError(i18n.KeyTUISessionViewDecodeCheckpointFile, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return false, i18n.NewError(i18n.KeyTUISessionViewTrailingCheckpointData)
	}
	return true, nil
}

func materializeSessionViewEvidence(root string, source DetailStore, checkpoint *sessionViewCheckpoint) error {
	target, err := NewFileDetailStore(filepath.Join(root, "tui-details"))
	if err != nil {
		return i18n.WrapError(i18n.KeyTUISessionViewMaterializeEvidence, err)
	}
	cache := make(map[DetailRef]DetailRef)
	rewrite := func(ref DetailRef) (DetailRef, error) {
		if mapped, ok := cache[ref]; ok {
			return mapped, nil
		}
		if source == nil {
			return DetailRef{}, i18n.WrapError(i18n.KeyTUISessionViewMaterializeEvidence, ErrDetailNotFound)
		}
		data, err := source.Get(ref)
		if err != nil {
			return DetailRef{}, i18n.WrapError(i18n.KeyTUISessionViewMaterializeEvidence, err)
		}
		mapped, err := target.Put(ref.Key, data)
		if err != nil {
			return DetailRef{}, i18n.WrapError(i18n.KeyTUISessionViewMaterializeEvidence, err)
		}
		cache[ref] = mapped
		return mapped, nil
	}
	if err := rewriteCheckpointDetailRefs(checkpoint, rewrite); err != nil {
		return err
	}
	checkpoint.EvidenceManifest = collectCheckpointDetailRefs(*checkpoint)
	return persistCheckpointObservationJournal(target, checkpoint.Observations)
}

func copySessionViewEvidence(sourceRoot, targetRoot string, checkpoint *sessionViewCheckpoint) error {
	source, err := NewFileDetailStore(filepath.Join(sourceRoot, "tui-details"))
	if err != nil {
		return i18n.WrapError(i18n.KeyTUISessionViewValidateEvidence, err)
	}
	target, err := NewFileDetailStore(filepath.Join(targetRoot, "tui-details"))
	if err != nil {
		return i18n.WrapError(i18n.KeyTUISessionViewMaterializeEvidence, err)
	}
	for _, ref := range checkpoint.EvidenceManifest {
		data, err := source.Get(ref)
		if err != nil {
			return i18n.WrapError(i18n.KeyTUISessionViewValidateEvidence, err)
		}
		written, err := target.Put(ref.Key, data)
		if err != nil {
			return i18n.WrapError(i18n.KeyTUISessionViewMaterializeEvidence, err)
		}
		if written != ref {
			return i18n.NewError(i18n.KeyTUISessionViewInvalidCheckpoint)
		}
	}
	return persistCheckpointObservationJournal(target, checkpoint.Observations)
}

func validateSessionViewEvidence(root string, checkpoint sessionViewCheckpoint) (DetailStore, error) {
	details, err := NewFileDetailStore(filepath.Join(root, "tui-details"))
	if err != nil {
		return nil, i18n.WrapError(i18n.KeyTUISessionViewValidateEvidence, err)
	}
	refs := collectCheckpointDetailRefs(checkpoint)
	if !equalDetailRefSets(refs, checkpoint.EvidenceManifest) {
		return nil, i18n.NewError(i18n.KeyTUISessionViewInvalidCheckpoint)
	}
	for _, ref := range refs {
		if _, err := details.Get(ref); err != nil {
			return nil, i18n.WrapError(i18n.KeyTUISessionViewValidateEvidence, err)
		}
	}
	return details, nil
}

func persistCheckpointObservationJournal(store *FileDetailStore, observations []Observation) error {
	for _, observation := range observations {
		if len(observation.ResultRefs) == 0 && len(observation.EnvelopeRefs) == 0 {
			continue
		}
		if err := store.SaveObservationEvidence(observation); err != nil {
			return i18n.WrapError(i18n.KeyTUISessionViewMaterializeEvidence, err)
		}
	}
	return nil
}

func rewriteCheckpointDetailRefs(checkpoint *sessionViewCheckpoint, rewrite func(DetailRef) (DetailRef, error)) error {
	for index := range checkpoint.Messages {
		refs, err := rewriteDetailRefs(checkpoint.Messages[index].DetailRefs, rewrite)
		if err != nil {
			return err
		}
		checkpoint.Messages[index].DetailRefs = refs
	}
	for index := range checkpoint.Observations {
		results, err := rewriteDetailRefs(checkpoint.Observations[index].ResultRefs, rewrite)
		if err != nil {
			return err
		}
		envelopes, err := rewriteDetailRefs(checkpoint.Observations[index].EnvelopeRefs, rewrite)
		if err != nil {
			return err
		}
		checkpoint.Observations[index].ResultRefs = results
		checkpoint.Observations[index].EnvelopeRefs = envelopes
	}
	for index := range checkpoint.Aggregates {
		refs, err := rewriteDetailRefs(checkpoint.Aggregates[index].EvidenceRefs, rewrite)
		if err != nil {
			return err
		}
		checkpoint.Aggregates[index].EvidenceRefs = refs
	}
	for index := range checkpoint.Activities {
		refs, err := rewriteDetailRefs(checkpoint.Activities[index].Control.DetailRefs, rewrite)
		if err != nil {
			return err
		}
		checkpoint.Activities[index].Control.DetailRefs = refs
	}
	return nil
}

func rewriteDetailRefs(refs []DetailRef, rewrite func(DetailRef) (DetailRef, error)) ([]DetailRef, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	out := make([]DetailRef, len(refs))
	for index, ref := range refs {
		mapped, err := rewrite(ref)
		if err != nil {
			return nil, err
		}
		out[index] = mapped
	}
	return out, nil
}

func collectCheckpointDetailRefs(checkpoint sessionViewCheckpoint) []DetailRef {
	unique := make(map[DetailRef]struct{})
	add := func(refs []DetailRef) {
		for _, ref := range refs {
			unique[ref] = struct{}{}
		}
	}
	for _, message := range checkpoint.Messages {
		add(message.DetailRefs)
	}
	for _, observation := range checkpoint.Observations {
		add(observation.ResultRefs)
		add(observation.EnvelopeRefs)
	}
	for _, aggregate := range checkpoint.Aggregates {
		add(aggregate.EvidenceRefs)
	}
	for _, activity := range checkpoint.Activities {
		add(activity.Control.DetailRefs)
	}
	refs := make([]DetailRef, 0, len(unique))
	for ref := range unique {
		refs = append(refs, ref)
	}
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].Digest != refs[j].Digest {
			return refs[i].Digest < refs[j].Digest
		}
		if refs[i].Key != refs[j].Key {
			return refs[i].Key < refs[j].Key
		}
		if refs[i].Source != refs[j].Source {
			return refs[i].Source < refs[j].Source
		}
		return refs[i].Size < refs[j].Size
	})
	return refs
}

func equalDetailRefSets(left, right []DetailRef) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func remapSessionViewCheckpoint(
	checkpoint *sessionViewCheckpoint,
	sourceIdentity, targetIdentity SessionIdentity,
	sourceTranscript, targetTranscript []types.Message,
) {
	sourceSegmentIDs := checkpointToolSegmentIDs(checkpoint.Messages)
	idMap := checkpointObservationIDMap(sourceIdentity, targetIdentity, sourceTranscript, targetTranscript)
	remapID := func(id string) string {
		if mapped := idMap[id]; mapped != "" {
			return mapped
		}
		return rewriteSessionScopedValue(id, sourceIdentity.SessionID, targetIdentity.SessionID)
	}
	checkpoint.Identity = targetIdentity
	for index := range checkpoint.Messages {
		message := &checkpoint.Messages[index]
		message.SessionID = targetIdentity.SessionID
		message.TurnID = rewriteSessionScopedValue(message.TurnID, sourceIdentity.SessionID, targetIdentity.SessionID)
		message.WorkUnitID = rewriteSessionScopedValue(message.WorkUnitID, sourceIdentity.SessionID, targetIdentity.SessionID)
		message.ObservationID = remapID(message.ObservationID)
	}
	for index := range checkpoint.Observations {
		observation := &checkpoint.Observations[index]
		if observation.PresentationID == "" {
			observation.PresentationID = observation.ID
		}
		if observation.PresentationWorkUnitID == "" {
			observation.PresentationWorkUnitID = observation.WorkUnitID
		}
		if observation.PresentationActorID == "" {
			observation.PresentationActorID = observation.ActorID
		}
		observation.ID = remapID(observation.ID)
		observation.SessionID = targetIdentity.SessionID
		observation.TurnID = rewriteSessionScopedValue(observation.TurnID, sourceIdentity.SessionID, targetIdentity.SessionID)
		observation.WorkUnitID = rewriteSessionScopedValue(observation.WorkUnitID, sourceIdentity.SessionID, targetIdentity.SessionID)
	}
	for index := range checkpoint.Aggregates {
		aggregate := &checkpoint.Aggregates[index]
		aggregate.SessionID = targetIdentity.SessionID
		aggregate.TurnID = rewriteSessionScopedValue(aggregate.TurnID, sourceIdentity.SessionID, targetIdentity.SessionID)
		aggregate.WorkUnitID = rewriteSessionScopedValue(aggregate.WorkUnitID, sourceIdentity.SessionID, targetIdentity.SessionID)
		for memberIndex := range aggregate.MemberIDs {
			aggregate.MemberIDs[memberIndex] = remapID(aggregate.MemberIDs[memberIndex])
		}
	}
	checkpoint.Interaction.FocusedObservationID = remapID(checkpoint.Interaction.FocusedObservationID)
	checkpoint.Interaction.ScrollAnchorID = remapID(checkpoint.Interaction.ScrollAnchorID)
	remappedReturns := make(map[string]SessionInteraction, len(checkpoint.DisclosureReturns))
	for id, interaction := range checkpoint.DisclosureReturns {
		interaction.FocusedObservationID = remapID(interaction.FocusedObservationID)
		interaction.ScrollAnchorID = remapID(interaction.ScrollAnchorID)
		remappedReturns[remapID(id)] = interaction
	}
	checkpoint.DisclosureReturns = remappedReturns
	for index := range checkpoint.Decisions {
		decision := &checkpoint.Decisions[index]
		decision.Prompt.SessionID = targetIdentity.SessionID
		decision.Prompt.ExecutionSessionID = rewriteSessionScopedValue(decision.Prompt.ExecutionSessionID, sourceIdentity.SessionID, targetIdentity.SessionID)
		decision.Prompt.TurnID = rewriteSessionScopedValue(decision.Prompt.TurnID, sourceIdentity.SessionID, targetIdentity.SessionID)
		decision.Prompt.WorkUnitID = rewriteSessionScopedValue(decision.Prompt.WorkUnitID, sourceIdentity.SessionID, targetIdentity.SessionID)
		decision.Prompt.DecisionID = rewriteSessionScopedValue(decision.Prompt.DecisionID, sourceIdentity.SessionID, targetIdentity.SessionID)
		decision.Response.DecisionID = rewriteSessionScopedValue(decision.Response.DecisionID, sourceIdentity.SessionID, targetIdentity.SessionID)
	}
	for index := range checkpoint.Activities {
		activity := &checkpoint.Activities[index]
		if activity.PresentationWorkUnitID == "" {
			activity.PresentationWorkUnitID = activity.WorkUnitID
		}
		activity.SessionID = targetIdentity.SessionID
		activity.TurnID = rewriteSessionScopedValue(activity.TurnID, sourceIdentity.SessionID, targetIdentity.SessionID)
		activity.WorkUnitID = rewriteSessionScopedValue(activity.WorkUnitID, sourceIdentity.SessionID, targetIdentity.SessionID)
		activity.Control.JumpTarget = remapID(activity.Control.JumpTarget)
		activity.Attention.DecisionID = rewriteSessionScopedValue(activity.Attention.DecisionID, sourceIdentity.SessionID, targetIdentity.SessionID)
	}
	targetSegmentIDs := checkpointToolSegmentIDs(checkpoint.Messages)
	remappedExpansion := make(map[string]bool, len(checkpoint.ToolSegmentExpansion))
	for id, expanded := range checkpoint.ToolSegmentExpansion {
		remappedExpansion[id] = expanded
	}
	for index := 0; index < len(sourceSegmentIDs) && index < len(targetSegmentIDs); index++ {
		if expanded, ok := checkpoint.ToolSegmentExpansion[sourceSegmentIDs[index]]; ok {
			delete(remappedExpansion, sourceSegmentIDs[index])
			remappedExpansion[targetSegmentIDs[index]] = expanded
		}
	}
	checkpoint.ToolSegmentExpansion = remappedExpansion
	checkpoint.EvidenceManifest = collectCheckpointDetailRefs(*checkpoint)
}

func checkpointToolSegmentIDs(messages []Message) []string {
	var ids []string
	for _, item := range BuildTranscriptToolSegments(messages) {
		if item.Segment != nil {
			ids = append(ids, item.Segment.ID)
		}
	}
	return ids
}

func checkpointObservationIDMap(source, target SessionIdentity, sourceTranscript, targetTranscript []types.Message) map[string]string {
	sourceProjection, sourceErr := ProjectPersistedMessagesInLanguage(i18nLanguageForCheckpoint(), source, sourceTranscript, NewMemoryDetailStore())
	targetProjection, targetErr := ProjectPersistedMessagesInLanguage(i18nLanguageForCheckpoint(), target, targetTranscript, NewMemoryDetailStore())
	if sourceErr != nil || targetErr != nil {
		return nil
	}
	mapping := make(map[string]string)
	limit := min(len(sourceProjection.Observations), len(targetProjection.Observations))
	for index := 0; index < limit; index++ {
		mapping[sourceProjection.Observations[index].ID] = targetProjection.Observations[index].ID
	}
	return mapping
}

// The language is irrelevant to identity derivation; projection output is
// discarded except for stable observation IDs.
func i18nLanguageForCheckpoint() i18n.Language { return i18n.LangEN }

func rewriteSessionScopedValue(value, sourceSessionID, targetSessionID string) string {
	if value == "" || sourceSessionID == "" || targetSessionID == "" {
		return value
	}
	if value == sourceSessionID {
		return targetSessionID
	}
	if strings.HasPrefix(value, sourceSessionID+":") {
		return targetSessionID + strings.TrimPrefix(value, sourceSessionID)
	}
	for _, prefix := range []string{"activity:", "runtime-error:"} {
		if strings.HasPrefix(value, prefix+sourceSessionID+":") {
			return prefix + targetSessionID + strings.TrimPrefix(value, prefix+sourceSessionID)
		}
	}
	return value
}

func cloneBoolMap(source map[string]bool) map[string]bool {
	if source == nil {
		return nil
	}
	out := make(map[string]bool, len(source))
	for key, value := range source {
		out[key] = value
	}
	return out
}

func cloneInteractionMap(source map[string]SessionInteraction) map[string]SessionInteraction {
	if source == nil {
		return nil
	}
	out := make(map[string]SessionInteraction, len(source))
	for key, value := range source {
		out[key] = value
	}
	return out
}
