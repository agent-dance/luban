package session

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agent-dance/luban/internal/messagecontrol"
	"github.com/agent-dance/luban/types"
)

const compactionManifestSchemaV2 = "compaction-manifest/v2"
const auditSegmentSchemaV2 = "append-only-audit-segment/v2"
const internalControlProvenanceSchemaV1 = "internal-control-provenance/v1"

// ErrStaleContextGeneration means a writer prepared its projection from an
// older committed generation. The writer must reload instead of overwriting a
// newer model context.
var ErrStaleContextGeneration = errors.New("stale context generation")

// ErrCorruptSessionHistory means a referenced immutable history artifact or
// its digest chain failed validation. Callers must fail closed; a valid JSONL
// prefix is never returned as a session.
var ErrCorruptSessionHistory = errors.New("corrupt session history")

// HistoryCommitStage identifies durable publish boundaries used by crash
// injection tests. A fault after ManifestPublished represents a crash after
// the new generation became authoritative.
type HistoryCommitStage string

const (
	HistoryStageAuditSegmentPrepared HistoryCommitStage = "audit_segment_prepared"
	HistoryStageAuditPrepared        HistoryCommitStage = "audit_prepared"
	HistoryStageViewPrepared         HistoryCommitStage = "view_prepared"
	HistoryStageBeforeManifestCAS    HistoryCommitStage = "before_manifest_cas"
	HistoryStageManifestPrepared     HistoryCommitStage = "manifest_prepared"
	HistoryStageManifestPublished    HistoryCommitStage = "manifest_published"
)

// ImmutableArtifactRef identifies a content-addressed immutable artifact.
type ImmutableArtifactRef struct {
	Digest       string `json:"digest"`
	Bytes        int64  `json:"bytes"`
	MessageCount uint64 `json:"message_count"`
}

// ContextCommitError reports an error after the manifest CAS has already made
// Manifest authoritative. Retrying with the previous generation is unsafe;
// callers must advance their fence to Manifest.ContextGeneration first.
type ContextCommitError struct {
	Manifest CompactionManifestV2
	Cause    error
}

func (e *ContextCommitError) Error() string {
	return "context generation committed before follow-up persistence failed"
}

func (e *ContextCommitError) Unwrap() error { return e.Cause }

type ManifestBoundaryV2 struct {
	BoundaryID        string          `json:"boundary_id"`
	MessageDigest     string          `json:"message_digest"`
	Trigger           string          `json:"trigger,omitempty"`
	PreCompactTokens  int             `json:"pre_compact_tokens,omitempty"`
	PostCompactTokens int             `json:"post_compact_tokens,omitempty"`
	TruePostCompact   int             `json:"true_post_compact_tokens,omitempty"`
	CompactionUsage   json.RawMessage `json:"compaction_usage,omitempty"`
}

type ManifestSummaryV2 struct {
	Schema       string `json:"schema"`
	Digest       string `json:"digest"`
	ParentDigest string `json:"parent_digest,omitempty"`
}

type RetainedMessageRefV2 struct {
	ViewIndex    uint64                    `json:"view_index"`
	Digest       string                    `json:"digest"`
	MessageID    string                    `json:"message_id,omitempty"`
	InternalKind types.InternalMessageKind `json:"internal_kind,omitempty"`
}

// TrustedControlRefV2 is the durable, content-addressed counterpart of the
// process-local Message provenance. A ref is emitted only for a message that
// was sealed by the runtime before the manifest commit.
type TrustedControlRefV2 struct {
	ViewIndex uint64                    `json:"view_index"`
	Digest    string                    `json:"digest"`
	Kind      types.InternalMessageKind `json:"kind"`
}

type TrustedContentReplacementRefV2 struct {
	ViewIndex  uint64 `json:"view_index"`
	BlockIndex uint64 `json:"block_index"`
	Digest     string `json:"digest"`
}

// CompactionManifestV2 is the single atomic pointer for a model-context
// generation. Audit segments and the model view are immutable and fully
// durable before this manifest is published.
type CompactionManifestV2 struct {
	SchemaVersion              string                           `json:"schema_version"`
	SessionID                  string                           `json:"session_id"`
	ContextGeneration          uint64                           `json:"context_generation"`
	ParentManifestDigest       string                           `json:"parent_manifest_digest,omitempty"`
	AuditSegments              []ImmutableArtifactRef           `json:"audit_segments,omitempty"`
	AuditTailDigest            string                           `json:"audit_tail_digest,omitempty"`
	AuditMessageCount          uint64                           `json:"audit_message_count"`
	AuditTranscript            ImmutableArtifactRef             `json:"audit_transcript"`
	ModelContextView           ImmutableArtifactRef             `json:"model_context_view"`
	ModelContextPreview        string                           `json:"model_context_preview,omitempty"`
	Boundary                   *ManifestBoundaryV2              `json:"boundary,omitempty"`
	Summary                    *ManifestSummaryV2               `json:"summary,omitempty"`
	RetainedMessages           []RetainedMessageRefV2           `json:"retained_messages,omitempty"`
	ControlProvenance          string                           `json:"control_provenance,omitempty"`
	TrustedControls            []TrustedControlRefV2            `json:"trusted_controls,omitempty"`
	TrustedContentReplacements []TrustedContentReplacementRefV2 `json:"trusted_content_replacements,omitempty"`
	CommittedAt                time.Time                        `json:"committed_at"`
	Digest                     string                           `json:"digest"`
}

type appendOnlyAuditSegmentV2 struct {
	SchemaVersion  string          `json:"schema_version"`
	SessionID      string          `json:"session_id"`
	SegmentIndex   uint64          `json:"segment_index"`
	MessageOffset  uint64          `json:"message_offset"`
	PreviousDigest string          `json:"previous_digest,omitempty"`
	Messages       []types.Message `json:"messages"`
	Digest         string          `json:"digest"`
}

func (s *FileStore) manifestPath(id string) string {
	if validateStorageID(id) != nil {
		return filepath.Join(s.dir, ".invalid-session-id")
	}
	return filepath.Join(s.dir, id+".context-v2.json")
}

func (s *FileStore) historyDir(id string) string {
	if validateStorageID(id) != nil {
		return filepath.Join(s.dir, ".invalid-session-id")
	}
	return filepath.Join(s.ArtifactsDir(id), "context-v2")
}

func (s *FileStore) auditDir(id string) string {
	return filepath.Join(s.historyDir(id), "audit")
}

func (s *FileStore) auditTranscriptDir(id string) string {
	return filepath.Join(s.historyDir(id), "audit-transcripts")
}

func (s *FileStore) viewDir(id string) string {
	return filepath.Join(s.historyDir(id), "views")
}

func (s *FileStore) immutableManifestDir(id string) string {
	return filepath.Join(s.historyDir(id), "manifests")
}

func digestBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func digestFileName(digest, suffix string) (string, error) {
	const prefix = "sha256:"
	if !strings.HasPrefix(digest, prefix) || len(digest) != len(prefix)+sha256.Size*2 {
		return "", fmt.Errorf("%w: invalid artifact digest", ErrCorruptSessionHistory)
	}
	hexDigest := strings.TrimPrefix(digest, prefix)
	if _, err := hex.DecodeString(hexDigest); err != nil {
		return "", fmt.Errorf("%w: invalid artifact digest: %v", ErrCorruptSessionHistory, err)
	}
	return hexDigest + suffix, nil
}

func computeManifestDigest(manifest CompactionManifestV2) (string, error) {
	manifest.Digest = ""
	data, err := json.Marshal(manifest)
	if err != nil {
		return "", err
	}
	return digestBytes(data), nil
}

func computeAuditSegmentDigest(segment appendOnlyAuditSegmentV2) (string, error) {
	segment.Digest = ""
	data, err := json.Marshal(segment)
	if err != nil {
		return "", err
	}
	return digestBytes(data), nil
}

func encodeMessagesJSONL(messages []types.Message) ([]byte, error) {
	var payload bytes.Buffer
	enc := json.NewEncoder(&payload)
	for _, message := range messages {
		if err := enc.Encode(message); err != nil {
			return nil, fmt.Errorf("encode model context message: %w", err)
		}
	}
	return payload.Bytes(), nil
}

func decodeMessagesJSONL(data []byte) ([]types.Message, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	messages := make([]types.Message, 0)
	for {
		var message types.Message
		if err := dec.Decode(&message); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("%w: decode model context: %v", ErrCorruptSessionHistory, err)
		}
		messages = append(messages, message)
	}
	return messages, nil
}

func (s *FileStore) writeImmutablePrivateFile(dir, path, pattern string, data []byte) error {
	root, err := s.storageRoot()
	if err != nil {
		return err
	}
	rel, err := s.storageRelative(path)
	if err != nil {
		return err
	}
	parentRel := filepath.Dir(rel)
	expected, err := s.storageRelative(dir)
	if err != nil || filepath.Clean(expected) != filepath.Clean(parentRel) {
		return fs.ErrInvalid
	}
	parent, err := root.OpenRoot(parentRel, true)
	if err != nil {
		return err
	}
	defer parent.Close()
	base := filepath.Base(rel)
	readExisting := func() ([]byte, error) {
		file, openErr := parent.OpenFile(base, os.O_RDONLY, 0)
		if openErr != nil {
			return nil, openErr
		}
		defer file.Close()
		if _, validateErr := validateAndTightenPrivateRegularFile(file, path); validateErr != nil {
			return nil, validateErr
		}
		payload, readErr := io.ReadAll(file)
		if readErr != nil {
			return nil, readErr
		}
		if _, validateErr := validateAndTightenPrivateRegularFile(file, path); validateErr != nil {
			return nil, validateErr
		}
		return payload, nil
	}
	if existing, readErr := readExisting(); readErr == nil {
		if bytes.Equal(existing, data) {
			return nil
		}
		return fmt.Errorf("%w: immutable artifact content mismatch", ErrCorruptSessionHistory)
	} else if !errors.Is(readErr, fs.ErrNotExist) {
		return readErr
	}
	tmp, tmpName, err := parent.CreateTemp(".", pattern)
	if err != nil {
		return err
	}
	tmpName = filepath.Base(tmpName)
	closed := false
	defer func() {
		if !closed {
			_ = tmp.Close()
		}
		_ = parent.Remove(tmpName)
	}()
	if err := tmp.Chmod(0o600); err != nil {
		return err
	}
	if n, err := tmp.Write(data); err != nil {
		return err
	} else if n != len(data) {
		return io.ErrShortWrite
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	tmpInfo, err := validateAndTightenPrivateRegularFile(tmp, parent.Path(tmpName))
	if err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		closed = true
		return err
	}
	closed = true
	if s.storageBeforePublish != nil {
		s.storageBeforePublish()
	}
	if err := root.Validate(); err != nil {
		return err
	}
	if err := parent.Validate(); err != nil {
		return err
	}
	if err := s.validateActiveHistoryLockForPath(path); err != nil {
		return err
	}
	expectedFinal := tmpInfo
	if err := parent.Link(tmpName, base); err != nil {
		if !errors.Is(err, fs.ErrExist) {
			return err
		}
		existing, readErr := readExisting()
		if readErr != nil {
			return readErr
		}
		if !bytes.Equal(existing, data) {
			return fmt.Errorf("%w: immutable artifact collision", ErrCorruptSessionHistory)
		}
		existingFile, openErr := parent.OpenFile(base, os.O_RDONLY, 0)
		if openErr != nil {
			return openErr
		}
		existingInfo, validateErr := validateAndTightenPrivateRegularFile(existingFile, path)
		closeErr := existingFile.Close()
		if validateErr != nil {
			return validateErr
		}
		if closeErr != nil {
			return closeErr
		}
		expectedFinal = existingInfo
	} else {
		published, openErr := parent.OpenFile(base, os.O_RDONLY, 0)
		if openErr != nil {
			return openErr
		}
		publishedInfo, statErr := published.Stat()
		if statErr == nil && (publishedInfo == nil || !publishedInfo.Mode().IsRegular() || !os.SameFile(tmpInfo, publishedInfo)) {
			statErr = fs.ErrInvalid
		}
		publishedData, readErr := io.ReadAll(published)
		closeErr := published.Close()
		if statErr != nil {
			return statErr
		}
		if readErr != nil {
			return readErr
		}
		if closeErr != nil {
			return closeErr
		}
		if !bytes.Equal(publishedData, data) {
			return fs.ErrInvalid
		}
	}
	if err := parent.Remove(tmpName); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	final, err := parent.OpenFile(base, os.O_RDONLY, 0)
	if err != nil {
		return err
	}
	finalInfo, validateErr := validateAndTightenPrivateRegularFile(final, path)
	if validateErr == nil && !os.SameFile(expectedFinal, finalInfo) {
		validateErr = fs.ErrInvalid
	}
	closeErr := final.Close()
	if validateErr != nil {
		return validateErr
	}
	if closeErr != nil {
		return closeErr
	}
	if err := parent.Validate(); err != nil {
		return err
	}
	return parent.Sync(".")
}

func (s *FileStore) injectHistoryFault(stage HistoryCommitStage) error {
	if s.historyCommitFault == nil {
		return nil
	}
	return s.historyCommitFault(stage)
}

func (s *FileStore) loadManifestLocked(sessionID string) (CompactionManifestV2, bool, error) {
	data, err := s.readPrivateRegularFile(s.manifestPath(sessionID))
	if errors.Is(err, fs.ErrNotExist) {
		return CompactionManifestV2{}, false, nil
	}
	if err != nil {
		return CompactionManifestV2{}, false, err
	}
	var manifest CompactionManifestV2
	if err := json.Unmarshal(data, &manifest); err != nil {
		return CompactionManifestV2{}, false, fmt.Errorf("%w: parse compaction manifest: %v", ErrCorruptSessionHistory, err)
	}
	if manifest.SchemaVersion != compactionManifestSchemaV2 || manifest.SessionID != sessionID || manifest.ContextGeneration == 0 {
		return CompactionManifestV2{}, false, fmt.Errorf("%w: invalid compaction manifest identity", ErrCorruptSessionHistory)
	}
	wantDigest, err := computeManifestDigest(manifest)
	if err != nil {
		return CompactionManifestV2{}, false, err
	}
	if manifest.Digest != wantDigest {
		return CompactionManifestV2{}, false, fmt.Errorf("%w: compaction manifest digest mismatch", ErrCorruptSessionHistory)
	}
	if err := s.validateManifestChainLocked(manifest); err != nil {
		return CompactionManifestV2{}, false, err
	}
	return manifest, true, nil
}

func (s *FileStore) validateManifestChainLocked(manifest CompactionManifestV2) error {
	manifestName, err := digestFileName(manifest.Digest, ".json")
	if err != nil {
		return err
	}
	immutable, err := s.readPrivateRegularFile(filepath.Join(s.immutableManifestDir(manifest.SessionID), manifestName))
	if err != nil {
		return fmt.Errorf("%w: read immutable manifest: %v", ErrCorruptSessionHistory, err)
	}
	wantImmutable, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	if !bytes.Equal(immutable, wantImmutable) {
		return fmt.Errorf("%w: immutable manifest mismatch", ErrCorruptSessionHistory)
	}
	current := manifest
	for current.ContextGeneration > 1 {
		if current.ParentManifestDigest == "" {
			return fmt.Errorf("%w: missing parent manifest digest", ErrCorruptSessionHistory)
		}
		name, err := digestFileName(current.ParentManifestDigest, ".json")
		if err != nil {
			return err
		}
		data, err := s.readPrivateRegularFile(filepath.Join(s.immutableManifestDir(current.SessionID), name))
		if err != nil {
			return fmt.Errorf("%w: read parent manifest: %v", ErrCorruptSessionHistory, err)
		}
		var parent CompactionManifestV2
		if err := json.Unmarshal(data, &parent); err != nil {
			return fmt.Errorf("%w: parse parent manifest: %v", ErrCorruptSessionHistory, err)
		}
		wantDigest, err := computeManifestDigest(parent)
		if err != nil {
			return err
		}
		if parent.SchemaVersion != compactionManifestSchemaV2 || parent.SessionID != current.SessionID ||
			parent.ContextGeneration+1 != current.ContextGeneration || parent.Digest != current.ParentManifestDigest ||
			parent.Digest != wantDigest || parent.AuditMessageCount > current.AuditMessageCount ||
			len(parent.AuditSegments) > len(current.AuditSegments) {
			return fmt.Errorf("%w: invalid parent manifest chain", ErrCorruptSessionHistory)
		}
		for index := range parent.AuditSegments {
			if parent.AuditSegments[index] != current.AuditSegments[index] {
				return fmt.Errorf("%w: audit chain was rewritten across generations", ErrCorruptSessionHistory)
			}
		}
		if parent.Summary != nil && current.Summary != nil && parent.Summary.Digest != current.Summary.Digest &&
			current.Summary.ParentDigest != parent.Summary.Digest {
			return fmt.Errorf("%w: compact summary parent chain mismatch", ErrCorruptSessionHistory)
		}
		current = parent
	}
	if current.ContextGeneration != 1 || current.ParentManifestDigest != "" {
		return fmt.Errorf("%w: invalid manifest chain root", ErrCorruptSessionHistory)
	}
	return nil
}

func (s *FileStore) loadViewFromManifestLocked(manifest CompactionManifestV2) ([]types.Message, error) {
	name, err := digestFileName(manifest.ModelContextView.Digest, ".jsonl")
	if err != nil {
		return nil, err
	}
	data, err := s.readPrivateRegularFile(filepath.Join(s.viewDir(manifest.SessionID), name))
	if err != nil {
		return nil, fmt.Errorf("%w: read model context view: %v", ErrCorruptSessionHistory, err)
	}
	if int64(len(data)) != manifest.ModelContextView.Bytes || digestBytes(data) != manifest.ModelContextView.Digest {
		return nil, fmt.Errorf("%w: model context view digest mismatch", ErrCorruptSessionHistory)
	}
	messages, err := decodeMessagesJSONL(data)
	if err != nil {
		return nil, err
	}
	if uint64(len(messages)) != manifest.ModelContextView.MessageCount {
		return nil, fmt.Errorf("%w: model context view count mismatch", ErrCorruptSessionHistory)
	}
	if err := validateManifestProjection(manifest, messages); err != nil {
		return nil, err
	}
	messages, err = s.restoreTrustedControlProvenance(manifest, messages)
	if err != nil {
		return nil, err
	}
	return messages, nil
}

func (s *FileStore) loadAuditFromManifestLocked(manifest CompactionManifestV2) ([]types.Message, error) {
	messages := make([]types.Message, 0, manifest.AuditMessageCount)
	previous := ""
	for index, ref := range manifest.AuditSegments {
		name, err := digestFileName(ref.Digest, ".json")
		if err != nil {
			return nil, err
		}
		data, err := s.readPrivateRegularFile(filepath.Join(s.auditDir(manifest.SessionID), name))
		if err != nil {
			return nil, fmt.Errorf("%w: read audit segment: %v", ErrCorruptSessionHistory, err)
		}
		if int64(len(data)) != ref.Bytes || digestBytes(data) != ref.Digest {
			return nil, fmt.Errorf("%w: audit artifact digest mismatch", ErrCorruptSessionHistory)
		}
		var segment appendOnlyAuditSegmentV2
		if err := json.Unmarshal(data, &segment); err != nil {
			return nil, fmt.Errorf("%w: parse audit segment: %v", ErrCorruptSessionHistory, err)
		}
		wantDigest, err := computeAuditSegmentDigest(segment)
		if err != nil {
			return nil, err
		}
		if segment.SchemaVersion != auditSegmentSchemaV2 || segment.SessionID != manifest.SessionID ||
			segment.SegmentIndex != uint64(index+1) || segment.MessageOffset != uint64(len(messages)) ||
			segment.PreviousDigest != previous || segment.Digest != wantDigest ||
			uint64(len(segment.Messages)) != ref.MessageCount {
			return nil, fmt.Errorf("%w: invalid append-only audit chain", ErrCorruptSessionHistory)
		}
		messages = append(messages, segment.Messages...)
		previous = segment.Digest
	}
	if uint64(len(messages)) != manifest.AuditMessageCount || previous != manifest.AuditTailDigest {
		return nil, fmt.Errorf("%w: append-only audit tail mismatch", ErrCorruptSessionHistory)
	}
	name, err := digestFileName(manifest.AuditTranscript.Digest, ".jsonl")
	if err != nil {
		return nil, err
	}
	transcriptData, err := s.readPrivateRegularFile(filepath.Join(s.auditTranscriptDir(manifest.SessionID), name))
	if err != nil {
		return nil, fmt.Errorf("%w: read immutable audit transcript: %v", ErrCorruptSessionHistory, err)
	}
	if int64(len(transcriptData)) != manifest.AuditTranscript.Bytes ||
		digestBytes(transcriptData) != manifest.AuditTranscript.Digest ||
		manifest.AuditTranscript.MessageCount != manifest.AuditMessageCount {
		return nil, fmt.Errorf("%w: immutable audit transcript digest mismatch", ErrCorruptSessionHistory)
	}
	transcriptMessages, err := decodeMessagesJSONL(transcriptData)
	if err != nil {
		return nil, err
	}
	if !equalMessageSlices(transcriptMessages, messages) {
		return nil, fmt.Errorf("%w: immutable audit transcript does not match segment chain", ErrCorruptSessionHistory)
	}
	return messages, nil
}

func (s *FileStore) prepareAuditSegmentLocked(sessionID string, current CompactionManifestV2, delta []types.Message) (CompactionManifestV2, error) {
	next := current
	if len(delta) == 0 {
		return next, nil
	}
	segment := appendOnlyAuditSegmentV2{
		SchemaVersion:  auditSegmentSchemaV2,
		SessionID:      sessionID,
		SegmentIndex:   uint64(len(current.AuditSegments) + 1),
		MessageOffset:  current.AuditMessageCount,
		PreviousDigest: current.AuditTailDigest,
		Messages:       append([]types.Message(nil), delta...),
	}
	chainDigest, err := computeAuditSegmentDigest(segment)
	if err != nil {
		return CompactionManifestV2{}, err
	}
	segment.Digest = chainDigest
	data, err := json.Marshal(segment)
	if err != nil {
		return CompactionManifestV2{}, err
	}
	artifactDigest := digestBytes(data)
	name, err := digestFileName(artifactDigest, ".json")
	if err != nil {
		return CompactionManifestV2{}, err
	}
	if err := s.writeImmutablePrivateFile(s.auditDir(sessionID), filepath.Join(s.auditDir(sessionID), name), ".audit-segment-*", data); err != nil {
		return CompactionManifestV2{}, err
	}
	next.AuditSegments = append(append([]ImmutableArtifactRef(nil), current.AuditSegments...), ImmutableArtifactRef{
		Digest:       artifactDigest,
		Bytes:        int64(len(data)),
		MessageCount: uint64(len(delta)),
	})
	next.AuditTailDigest = chainDigest
	next.AuditMessageCount += uint64(len(delta))
	return next, nil
}

func (s *FileStore) prepareViewLocked(sessionID string, messages []types.Message) (ImmutableArtifactRef, error) {
	data, err := encodeMessagesJSONL(messages)
	if err != nil {
		return ImmutableArtifactRef{}, err
	}
	digest := digestBytes(data)
	name, err := digestFileName(digest, ".jsonl")
	if err != nil {
		return ImmutableArtifactRef{}, err
	}
	if err := s.writeImmutablePrivateFile(s.viewDir(sessionID), filepath.Join(s.viewDir(sessionID), name), ".model-view-*", data); err != nil {
		return ImmutableArtifactRef{}, err
	}
	return ImmutableArtifactRef{Digest: digest, Bytes: int64(len(data)), MessageCount: uint64(len(messages))}, nil
}

func (s *FileStore) prepareAuditTranscriptLocked(sessionID string, current CompactionManifestV2, currentAudit, delta []types.Message) (ImmutableArtifactRef, error) {
	if len(delta) == 0 && current.AuditTranscript.Digest != "" {
		return current.AuditTranscript, nil
	}
	messages := append(append([]types.Message(nil), currentAudit...), delta...)
	data, err := encodeMessagesJSONL(messages)
	if err != nil {
		return ImmutableArtifactRef{}, err
	}
	digest := digestBytes(data)
	name, err := digestFileName(digest, ".jsonl")
	if err != nil {
		return ImmutableArtifactRef{}, err
	}
	if err := s.writeImmutablePrivateFile(s.auditTranscriptDir(sessionID), filepath.Join(s.auditTranscriptDir(sessionID), name), ".audit-transcript-*", data); err != nil {
		return ImmutableArtifactRef{}, err
	}
	return ImmutableArtifactRef{Digest: digest, Bytes: int64(len(data)), MessageCount: uint64(len(messages))}, nil
}

func (s *FileStore) commitModelContextLocked(sessionID string, expectedGeneration uint64, view, auditDelta []types.Message) (CompactionManifestV2, error) {
	if err := s.ensureNotDeletedLocked(sessionID); err != nil {
		return CompactionManifestV2{}, err
	}
	current, exists, err := s.loadManifestLocked(sessionID)
	if err != nil {
		return CompactionManifestV2{}, err
	}
	currentGeneration := uint64(0)
	var currentAudit []types.Message
	var currentView []types.Message
	if exists {
		currentGeneration = current.ContextGeneration
		currentView, err = s.loadViewFromManifestLocked(current)
		if err != nil {
			return CompactionManifestV2{}, err
		}
		currentAudit, err = s.loadAuditFromManifestLocked(current)
		if err != nil {
			return CompactionManifestV2{}, err
		}
	}
	if expectedGeneration != currentGeneration {
		return CompactionManifestV2{}, fmt.Errorf("%w: expected %d, current %d", ErrStaleContextGeneration, expectedGeneration, currentGeneration)
	}
	projectScope, err := s.internalControlProjectScope()
	if err != nil {
		return CompactionManifestV2{}, err
	}
	if err := validateInternalControlScopesForCommit(view, sessionID, projectScope, currentGeneration); err != nil {
		return CompactionManifestV2{}, err
	}
	if err := validateInternalControlScopesForCommit(auditDelta, sessionID, projectScope, currentGeneration); err != nil {
		return CompactionManifestV2{}, err
	}
	if exists && len(auditDelta) == 0 && equalCommitProjectionSlices(currentView, view) {
		return current, nil
	}
	nextScope := messagecontrol.NewScope(sessionID, projectScope, currentGeneration+1)
	view = bindInternalControlScopesForCommit(view, nextScope)
	if err := s.ensureHistoryDirectoriesLocked(sessionID); err != nil {
		return CompactionManifestV2{}, err
	}

	next, err := s.prepareAuditSegmentLocked(sessionID, current, auditDelta)
	if err != nil {
		return CompactionManifestV2{}, err
	}
	if err := s.injectHistoryFault(HistoryStageAuditSegmentPrepared); err != nil {
		return CompactionManifestV2{}, err
	}
	next.AuditTranscript, err = s.prepareAuditTranscriptLocked(sessionID, current, currentAudit, auditDelta)
	if err != nil {
		return CompactionManifestV2{}, err
	}
	if err := s.injectHistoryFault(HistoryStageAuditPrepared); err != nil {
		return CompactionManifestV2{}, err
	}
	viewRef, err := s.prepareViewLocked(sessionID, view)
	if err != nil {
		return CompactionManifestV2{}, err
	}
	if err := s.injectHistoryFault(HistoryStageViewPrepared); err != nil {
		return CompactionManifestV2{}, err
	}

	next.SchemaVersion = compactionManifestSchemaV2
	next.SessionID = sessionID
	next.ContextGeneration = currentGeneration + 1
	next.ModelContextView = viewRef
	next.ModelContextPreview = derivePreviewText(view)
	next.Boundary, next.Summary, next.RetainedMessages, next.TrustedControls = deriveCompactionManifestState(current, view)
	next.TrustedContentReplacements = deriveTrustedContentReplacementRefs(view, nextScope)
	next.ControlProvenance = internalControlProvenanceSchemaV1
	if err := validateManifestProjection(next, view); err != nil {
		return CompactionManifestV2{}, err
	}
	next.CommittedAt = time.Now().UTC()
	if exists {
		next.ParentManifestDigest = current.Digest
	}
	next.Digest, err = computeManifestDigest(next)
	if err != nil {
		return CompactionManifestV2{}, err
	}
	if err := s.injectHistoryFault(HistoryStageBeforeManifestCAS); err != nil {
		return CompactionManifestV2{}, err
	}
	manifestData, err := json.Marshal(next)
	if err != nil {
		return CompactionManifestV2{}, err
	}
	manifestName, err := digestFileName(next.Digest, ".json")
	if err != nil {
		return CompactionManifestV2{}, err
	}
	if err := s.writeImmutablePrivateFile(s.immutableManifestDir(sessionID), filepath.Join(s.immutableManifestDir(sessionID), manifestName), ".immutable-manifest-*", manifestData); err != nil {
		return CompactionManifestV2{}, err
	}
	if err := s.injectHistoryFault(HistoryStageManifestPrepared); err != nil {
		return CompactionManifestV2{}, err
	}

	latest, latestExists, err := s.loadManifestLocked(sessionID)
	if err != nil {
		return CompactionManifestV2{}, err
	}
	latestGeneration := uint64(0)
	if latestExists {
		latestGeneration = latest.ContextGeneration
	}
	if latestGeneration != expectedGeneration {
		return CompactionManifestV2{}, fmt.Errorf("%w: expected %d, current %d", ErrStaleContextGeneration, expectedGeneration, latestGeneration)
	}
	if err := s.writePrivateFileAtomic(s.dir, s.manifestPath(sessionID), ".context-manifest-*", append(manifestData, '\n')); err != nil {
		return CompactionManifestV2{}, err
	}
	if err := s.injectHistoryFault(HistoryStageManifestPublished); err != nil {
		return next, &ContextCommitError{Manifest: next, Cause: err}
	}
	return next, nil
}

func (s *FileStore) ensureHistoryDirectoriesLocked(sessionID string) error {
	for _, path := range []string{s.ArtifactsDir(sessionID), s.historyDir(sessionID), s.auditDir(sessionID), s.auditTranscriptDir(sessionID), s.viewDir(sessionID), s.immutableManifestDir(sessionID)} {
		if err := s.ensurePrivateDirectory(path); err != nil {
			return err
		}
	}
	return nil
}

func deriveCompactionManifestState(current CompactionManifestV2, view []types.Message) (*ManifestBoundaryV2, *ManifestSummaryV2, []RetainedMessageRefV2, []TrustedControlRefV2) {
	var boundary *ManifestBoundaryV2
	var summary *ManifestSummaryV2
	retained := make([]RetainedMessageRefV2, 0, len(view))
	trustedControls := make([]TrustedControlRefV2, 0)
	for index, message := range view {
		encoded, err := json.Marshal(message)
		if err != nil {
			continue
		}
		messageDigest := digestBytes(encoded)
		trusted := message.HasInternalControlProvenance()
		if trusted {
			trustedControls = append(trustedControls, TrustedControlRefV2{
				ViewIndex: uint64(index), Digest: messageDigest, Kind: message.InternalKind,
			})
		}
		switch message.InternalKind {
		case types.InternalMessageKindCompactBoundary:
			if !trusted {
				retained = append(retained, RetainedMessageRefV2{
					ViewIndex: uint64(index), Digest: messageDigest, MessageID: message.ID, InternalKind: message.InternalKind,
				})
				continue
			}
			if current.Boundary != nil && current.Boundary.MessageDigest == messageDigest {
				boundary = current.Boundary
			} else {
				boundary = &ManifestBoundaryV2{
					BoundaryID:    stableBoundaryID(messageDigest),
					MessageDigest: messageDigest,
				}
				applyCompactBoundaryPayload(boundary, message.GetText())
			}
		case types.InternalMessageKindCompactSummary:
			if !trusted {
				retained = append(retained, RetainedMessageRefV2{
					ViewIndex: uint64(index), Digest: messageDigest, MessageID: message.ID, InternalKind: message.InternalKind,
				})
				continue
			}
			if current.Summary != nil && current.Summary.Digest == messageDigest {
				summary = current.Summary
			} else {
				nextSummary := &ManifestSummaryV2{Schema: "compact-summary/v2", Digest: messageDigest}
				if current.Summary != nil {
					nextSummary.ParentDigest = current.Summary.Digest
				}
				summary = nextSummary
			}
		default:
			retained = append(retained, RetainedMessageRefV2{
				ViewIndex: uint64(index), Digest: messageDigest, MessageID: message.ID, InternalKind: message.InternalKind,
			})
		}
	}
	return cloneManifestBoundary(boundary), cloneManifestSummary(summary), retained, trustedControls
}

func deriveTrustedContentReplacementRefs(view []types.Message, scope messagecontrol.Scope) []TrustedContentReplacementRefV2 {
	var refs []TrustedContentReplacementRefV2
	for messageIndex, message := range view {
		if message.Role != types.RoleUser {
			continue
		}
		for blockIndex, content := range message.Content {
			block, ok := content.(types.ContentReplacementBlock)
			if !ok || block.Kind != "tool-result" || block.ToolUseID == "" ||
				!block.HasInternalReplacementProvenanceForScope(scope) {
				continue
			}
			encoded, err := json.Marshal(block)
			if err != nil {
				continue
			}
			refs = append(refs, TrustedContentReplacementRefV2{
				ViewIndex: uint64(messageIndex), BlockIndex: uint64(blockIndex), Digest: digestBytes(encoded),
			})
		}
	}
	return refs
}

func stableBoundaryID(messageDigest string) string {
	return "boundary:" + strings.TrimPrefix(messageDigest, "sha256:")
}

func applyCompactBoundaryPayload(boundary *ManifestBoundaryV2, text string) {
	const prefix = "[compact-boundary]"
	if boundary == nil || !strings.HasPrefix(text, prefix) {
		return
	}
	encoded := strings.TrimSpace(strings.TrimPrefix(text, prefix))
	decoded, err := decodeCanonicalBase64(encoded)
	if err != nil {
		return
	}
	var payload struct {
		Trigger               string          `json:"trigger"`
		PreCompactTokenCount  int             `json:"pre_compact_token_count"`
		PostCompactTokenCount int             `json:"post_compact_token_count"`
		TruePostCompactCount  int             `json:"true_post_compact_token_count"`
		CompactionUsage       json.RawMessage `json:"compaction_usage"`
	}
	if err := json.Unmarshal(decoded, &payload); err != nil {
		return
	}
	boundary.Trigger = payload.Trigger
	boundary.PreCompactTokens = payload.PreCompactTokenCount
	boundary.PostCompactTokens = payload.PostCompactTokenCount
	boundary.TruePostCompact = payload.TruePostCompactCount
	boundary.CompactionUsage = append(json.RawMessage(nil), payload.CompactionUsage...)
}

func decodeCanonicalBase64(encoded string) ([]byte, error) {
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}
	if base64.StdEncoding.EncodeToString(decoded) != encoded {
		return nil, errors.New("non-canonical base64")
	}
	return decoded, nil
}

func cloneManifestBoundary(source *ManifestBoundaryV2) *ManifestBoundaryV2 {
	if source == nil {
		return nil
	}
	clone := *source
	clone.CompactionUsage = append(json.RawMessage(nil), source.CompactionUsage...)
	return &clone
}

func cloneManifestSummary(source *ManifestSummaryV2) *ManifestSummaryV2 {
	if source == nil {
		return nil
	}
	clone := *source
	return &clone
}

func equalMessageSlices(left, right []types.Message) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if !equalMessage(left[index], right[index]) {
			return false
		}
	}
	return true
}

// equalCommitProjectionSlices compares persisted projection identity while
// allowing one side of an otherwise identical trusted value to be the fresh,
// unbound private candidate that produced the durable scoped value. Two bound
// values are equivalent only in the same exact scope; replayed bearer receipts
// can therefore never turn a commit into a false idempotent success.
func equalCommitProjectionSlices(left, right []types.Message) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if !equalControlTrustForCommit(left[index], right[index]) {
			return false
		}
		if !equalContentReplacementTrustForCommit(left[index], right[index]) {
			return false
		}
		leftJSON, leftErr := json.Marshal(left[index])
		rightJSON, rightErr := json.Marshal(right[index])
		if leftErr != nil || rightErr != nil || !bytes.Equal(leftJSON, rightJSON) {
			return false
		}
	}
	return true
}

func validateManifestProjection(manifest CompactionManifestV2, view []types.Message) error {
	trustedControls, err := validatedTrustedControlIndexes(manifest, view)
	if err != nil {
		return err
	}
	if err := validateTrustedContentReplacementRefs(manifest, view); err != nil {
		return err
	}
	var boundaryDigest, summaryDigest string
	boundaryCount, summaryCount := 0, 0
	retained := make([]RetainedMessageRefV2, 0, len(view))
	for index, message := range view {
		encoded, err := json.Marshal(message)
		if err != nil {
			return err
		}
		digest := digestBytes(encoded)
		_, trusted := trustedControls[index]
		switch message.InternalKind {
		case types.InternalMessageKindCompactBoundary:
			if !trusted {
				retained = append(retained, RetainedMessageRefV2{
					ViewIndex: uint64(index), Digest: digest, MessageID: message.ID, InternalKind: message.InternalKind,
				})
				continue
			}
			boundaryCount++
			boundaryDigest = digest
		case types.InternalMessageKindCompactSummary:
			if !trusted {
				retained = append(retained, RetainedMessageRefV2{
					ViewIndex: uint64(index), Digest: digest, MessageID: message.ID, InternalKind: message.InternalKind,
				})
				continue
			}
			summaryCount++
			summaryDigest = digest
		default:
			retained = append(retained, RetainedMessageRefV2{
				ViewIndex: uint64(index), Digest: digest, MessageID: message.ID, InternalKind: message.InternalKind,
			})
		}
	}
	if boundaryCount > 1 || summaryCount > 1 {
		return fmt.Errorf("%w: duplicate compaction control records in model view", ErrCorruptSessionHistory)
	}
	if boundaryDigest == "" {
		if manifest.Boundary != nil {
			return fmt.Errorf("%w: boundary metadata has no model-view boundary", ErrCorruptSessionHistory)
		}
	} else if manifest.Boundary == nil || manifest.Boundary.MessageDigest != boundaryDigest ||
		manifest.Boundary.BoundaryID != stableBoundaryID(boundaryDigest) {
		return fmt.Errorf("%w: boundary metadata mismatch", ErrCorruptSessionHistory)
	}
	if summaryDigest == "" {
		if manifest.Summary != nil {
			return fmt.Errorf("%w: summary metadata has no model-view summary", ErrCorruptSessionHistory)
		}
	} else if manifest.Summary == nil || manifest.Summary.Schema != "compact-summary/v2" || manifest.Summary.Digest != summaryDigest {
		return fmt.Errorf("%w: summary metadata mismatch", ErrCorruptSessionHistory)
	}
	if len(retained) != len(manifest.RetainedMessages) {
		return fmt.Errorf("%w: retained message reference count mismatch", ErrCorruptSessionHistory)
	}
	for index := range retained {
		if retained[index] != manifest.RetainedMessages[index] {
			return fmt.Errorf("%w: retained message reference mismatch", ErrCorruptSessionHistory)
		}
	}
	return nil
}

func validateTrustedContentReplacementRefs(manifest CompactionManifestV2, view []types.Message) error {
	previousMessage, previousBlock := -1, -1
	for _, ref := range manifest.TrustedContentReplacements {
		messageIndex, blockIndex := int(ref.ViewIndex), int(ref.BlockIndex)
		if messageIndex < 0 || messageIndex >= len(view) || blockIndex < 0 || blockIndex >= len(view[messageIndex].Content) ||
			messageIndex < previousMessage || (messageIndex == previousMessage && blockIndex <= previousBlock) {
			return fmt.Errorf("%w: invalid trusted content replacement reference", ErrCorruptSessionHistory)
		}
		previousMessage, previousBlock = messageIndex, blockIndex
		block, ok := view[messageIndex].Content[blockIndex].(types.ContentReplacementBlock)
		if !ok {
			return fmt.Errorf("%w: trusted content replacement type mismatch", ErrCorruptSessionHistory)
		}
		if view[messageIndex].Role != types.RoleUser || block.Kind != "tool-result" || block.ToolUseID == "" {
			return fmt.Errorf("%w: invalid trusted content replacement descriptor", ErrCorruptSessionHistory)
		}
		encoded, err := json.Marshal(block)
		if err != nil || ref.Digest == "" || digestBytes(encoded) != ref.Digest {
			return fmt.Errorf("%w: trusted content replacement digest mismatch", ErrCorruptSessionHistory)
		}
	}
	return nil
}

func validatedTrustedControlIndexes(manifest CompactionManifestV2, view []types.Message) (map[int]TrustedControlRefV2, error) {
	trusted := make(map[int]TrustedControlRefV2)
	if manifest.ControlProvenance == "" {
		if len(manifest.TrustedControls) != 0 {
			return nil, fmt.Errorf("%w: trusted controls lack provenance schema", ErrCorruptSessionHistory)
		}
		// Manifests created before explicit provenance refs used the dedicated,
		// content-addressed boundary/summary records as their attestations.
		for index, message := range view {
			encoded, err := json.Marshal(message)
			if err != nil {
				return nil, err
			}
			digest := digestBytes(encoded)
			if manifest.Boundary != nil && message.InternalKind == types.InternalMessageKindCompactBoundary &&
				manifest.Boundary.MessageDigest == digest {
				trusted[index] = TrustedControlRefV2{ViewIndex: uint64(index), Digest: digest, Kind: message.InternalKind}
			}
			if manifest.Summary != nil && message.InternalKind == types.InternalMessageKindCompactSummary &&
				manifest.Summary.Digest == digest {
				trusted[index] = TrustedControlRefV2{ViewIndex: uint64(index), Digest: digest, Kind: message.InternalKind}
			}
		}
		return trusted, nil
	}
	if manifest.ControlProvenance != internalControlProvenanceSchemaV1 {
		return nil, fmt.Errorf("%w: unsupported internal control provenance schema", ErrCorruptSessionHistory)
	}
	var previousIndex uint64
	for position, ref := range manifest.TrustedControls {
		if ref.ViewIndex >= uint64(len(view)) || ref.Kind == "" ||
			(position > 0 && ref.ViewIndex <= previousIndex) {
			return nil, fmt.Errorf("%w: invalid trusted control reference", ErrCorruptSessionHistory)
		}
		previousIndex = ref.ViewIndex
		index := int(ref.ViewIndex)
		if _, duplicate := trusted[index]; duplicate {
			return nil, fmt.Errorf("%w: duplicate trusted control reference", ErrCorruptSessionHistory)
		}
		encoded, err := json.Marshal(view[index])
		if err != nil {
			return nil, err
		}
		if digestBytes(encoded) != ref.Digest || view[index].InternalKind != ref.Kind {
			return nil, fmt.Errorf("%w: trusted control reference mismatch", ErrCorruptSessionHistory)
		}
		trusted[index] = ref
	}
	return trusted, nil
}

func (s *FileStore) restoreTrustedControlProvenance(manifest CompactionManifestV2, messages []types.Message) ([]types.Message, error) {
	trusted, err := validatedTrustedControlIndexes(manifest, messages)
	if err != nil {
		return nil, err
	}
	restored := append([]types.Message(nil), messages...)
	projectScope, err := s.internalControlProjectScope()
	if err != nil {
		return nil, err
	}
	scope := messagecontrol.NewScope(manifest.SessionID, projectScope, manifest.ContextGeneration)
	for index := range trusted {
		restored[index] = restored[index].WithInternalControlProvenance(messagecontrol.Runtime(), scope)
	}
	for _, ref := range manifest.TrustedContentReplacements {
		messageIndex, blockIndex := int(ref.ViewIndex), int(ref.BlockIndex)
		content := append([]types.ContentBlock(nil), restored[messageIndex].Content...)
		block := content[blockIndex].(types.ContentReplacementBlock)
		content[blockIndex] = block.WithInternalReplacementProvenance(messagecontrol.Runtime(), scope)
		restored[messageIndex].Content = content
	}
	return restored, nil
}

func (s *FileStore) internalControlProjectScope() (string, error) {
	return filepath.Abs(filepath.Clean(s.dir))
}

// MessageControlScope returns the exact durable authority scope for the
// current model-context view. Callers use it to keep provider and presentation
// projections generation-fenced after a successful commit.
func (s *FileStore) MessageControlScope(sessionID string) (messagecontrol.Scope, error) {
	manifest, err := s.GetCompactionManifest(sessionID)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return messagecontrol.Scope{}, err
	}
	projectScope, err := s.internalControlProjectScope()
	if err != nil {
		return messagecontrol.Scope{}, err
	}
	generation := uint64(0)
	if err == nil {
		generation = manifest.ContextGeneration
	}
	return messagecontrol.NewScope(sessionID, projectScope, generation), nil
}

func validateInternalControlScopesForCommit(messages []types.Message, sessionID, projectScope string, currentGeneration uint64) error {
	expected := messagecontrol.NewScope(sessionID, projectScope, currentGeneration)
	for index, message := range messages {
		scope, bound := message.InternalControlProvenanceScope()
		if message.HasInternalControlProvenance() && !bound {
			return fmt.Errorf("%w: internal control %d lacks commit authority", ErrCorruptSessionHistory, index)
		}
		if bound && !scope.Equal(expected) {
			return fmt.Errorf("%w: internal control %d scope replay", ErrCorruptSessionHistory, index)
		}
		for blockIndex, content := range message.Content {
			replacement, ok := content.(types.ContentReplacementBlock)
			if !ok || !replacement.HasInternalReplacementProvenance() {
				continue
			}
			if message.Role != types.RoleUser || replacement.Kind != "tool-result" || replacement.ToolUseID == "" {
				return fmt.Errorf("%w: invalid trusted content replacement at message %d block %d", ErrCorruptSessionHistory, index, blockIndex)
			}
			replacementScope, replacementBound := replacement.InternalReplacementProvenanceScope()
			if !replacementBound {
				return fmt.Errorf("%w: content replacement %d:%d lacks commit authority", ErrCorruptSessionHistory, index, blockIndex)
			}
			if replacementBound && !replacementScope.Equal(expected) {
				return fmt.Errorf("%w: content replacement %d:%d scope replay", ErrCorruptSessionHistory, index, blockIndex)
			}
		}
	}
	return nil
}

func bindInternalControlScopesForCommit(messages []types.Message, scope messagecontrol.Scope) []types.Message {
	bound := append([]types.Message(nil), messages...)
	for index, message := range bound {
		content := append([]types.ContentBlock(nil), message.Content...)
		contentChanged := false
		for blockIndex, raw := range content {
			replacement, ok := raw.(types.ContentReplacementBlock)
			if !ok || !replacement.HasInternalReplacementProvenance() {
				continue
			}
			content[blockIndex] = replacement.WithInternalReplacementProvenance(messagecontrol.Runtime(), scope)
			contentChanged = true
		}
		if contentChanged {
			message.Content = content
		}
		if message.HasInternalControlProvenance() {
			message = message.WithInternalControlProvenance(messagecontrol.Runtime(), scope)
		}
		bound[index] = message
	}
	return bound
}

// CommitModelContext publishes a prepared model view using a generation CAS.
// auditDelta contains only newly observed raw messages/events. Passing nil for
// auditDelta is the normal compaction operation and leaves the audit digest and
// count unchanged.
func (s *FileStore) CommitModelContext(sessionID string, expectedGeneration uint64, view, auditDelta []types.Message) (CompactionManifestV2, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureReadyLocked(sessionID); err != nil {
		return CompactionManifestV2{}, err
	}
	unlock, err := s.lockSessionHistory(sessionID, true)
	if err != nil {
		return CompactionManifestV2{}, err
	}
	defer unlock()
	return s.commitModelContextLocked(sessionID, expectedGeneration, view, auditDelta)
}

// GetCompactionManifest returns a validated manifest and every referenced
// artifact must also validate before success.
func (s *FileStore) GetCompactionManifest(sessionID string) (CompactionManifestV2, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureNotDeletedLocked(sessionID); err != nil {
		return CompactionManifestV2{}, err
	}
	unlock, err := s.lockSessionHistory(sessionID, false)
	if err != nil {
		return CompactionManifestV2{}, err
	}
	defer unlock()
	manifest, exists, err := s.loadManifestLocked(sessionID)
	if err != nil {
		return CompactionManifestV2{}, err
	}
	if !exists {
		return CompactionManifestV2{}, fs.ErrNotExist
	}
	if _, err := s.loadViewFromManifestLocked(manifest); err != nil {
		return CompactionManifestV2{}, err
	}
	if _, err := s.loadAuditFromManifestLocked(manifest); err != nil {
		return CompactionManifestV2{}, err
	}
	return manifest, nil
}

// LoadAuditLog loads and validates the complete immutable raw audit chain.
func (s *FileStore) LoadAuditLog(sessionID string) ([]types.Message, CompactionManifestV2, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureNotDeletedLocked(sessionID); err != nil {
		return nil, CompactionManifestV2{}, err
	}
	unlock, err := s.lockSessionHistory(sessionID, true)
	if err != nil {
		return nil, CompactionManifestV2{}, err
	}
	defer unlock()
	manifest, err := s.ensureHistoryV2Locked(sessionID)
	if err != nil {
		return nil, CompactionManifestV2{}, err
	}
	messages, err := s.loadAuditFromManifestLocked(manifest)
	return messages, manifest, err
}

func (s *FileStore) ensureHistoryV2Locked(sessionID string) (CompactionManifestV2, error) {
	manifest, exists, err := s.loadManifestLocked(sessionID)
	if err != nil {
		return CompactionManifestV2{}, err
	}
	if exists {
		return manifest, nil
	}
	if info, statErr := s.lstatPrivate(s.historyDir(sessionID)); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return CompactionManifestV2{}, fmt.Errorf("%w: invalid history directory", ErrCorruptSessionHistory)
		}
		return CompactionManifestV2{}, fmt.Errorf("%w: history artifacts exist without manifest", ErrCorruptSessionHistory)
	} else if !errors.Is(statErr, fs.ErrNotExist) {
		return CompactionManifestV2{}, statErr
	}
	return CompactionManifestV2{}, fs.ErrNotExist
}

func (s *FileStore) loadModelContextLocked(sessionID string) ([]types.Message, CompactionManifestV2, error) {
	manifest, err := s.ensureHistoryV2Locked(sessionID)
	if err != nil {
		return nil, CompactionManifestV2{}, err
	}
	messages, err := s.loadViewFromManifestLocked(manifest)
	if err != nil {
		return nil, CompactionManifestV2{}, err
	}
	if _, err := s.loadAuditFromManifestLocked(manifest); err != nil {
		return nil, CompactionManifestV2{}, err
	}
	return messages, manifest, nil
}

func inferAuditDelta(previous, next []types.Message) ([]types.Message, error) {
	if len(previous) == 0 {
		return filterAuditProjectionMessages(next), nil
	}
	commonPrefix := 0
	for commonPrefix < len(previous) && commonPrefix < len(next) && equalMessage(previous[commonPrefix], next[commonPrefix]) {
		commonPrefix++
	}
	if commonPrefix == len(previous) {
		return filterAuditProjectionMessages(next[commonPrefix:]), nil
	}

	previousDigests := messageDigests(previous)
	nextDigests := messageDigests(next)
	previousRow := make([]int, len(nextDigests)+1)
	bestNewEnd, bestLength := 0, 0
	for oldIndex := range previousDigests {
		currentRow := make([]int, len(nextDigests)+1)
		for nextIndex := range nextDigests {
			if previousDigests[oldIndex] == nextDigests[nextIndex] {
				currentRow[nextIndex+1] = previousRow[nextIndex] + 1
			}
			if oldIndex == len(previousDigests)-1 && currentRow[nextIndex+1] > bestLength {
				bestLength = currentRow[nextIndex+1]
				bestNewEnd = nextIndex + 1
			}
		}
		previousRow = currentRow
	}
	if bestLength > 0 {
		return filterAuditProjectionMessages(next[bestNewEnd:]), nil
	}
	return nil, fmt.Errorf("%w: model context does not extend or compact the current generation", ErrCorruptSessionHistory)
}

func messageDigests(messages []types.Message) []string {
	digests := make([]string, len(messages))
	for index, message := range messages {
		encoded, err := json.Marshal(message)
		if err != nil {
			continue
		}
		// Provenance is intentionally not serialized, but both message controls
		// and replacement receipts are part of audit identity. A byte-identical
		// JSON value with a different trust domain or scope is new raw evidence.
		identity := messageProvenanceIdentity(message)
		digests[index] = digestBytes(append(identity, encoded...))
	}
	return digests
}

func filterAuditProjectionMessages(messages []types.Message) []types.Message {
	filtered := make([]types.Message, 0, len(messages))
	for _, message := range messages {
		if !message.HasInternalControlProvenance() {
			filtered = append(filtered, message)
			continue
		}
		switch message.InternalKind {
		case types.InternalMessageKindCompactBoundary,
			types.InternalMessageKindCompactSummary,
			types.InternalMessageKindCompactReminder:
			continue
		default:
			filtered = append(filtered, message)
		}
	}
	return filtered
}

func equalMessage(left, right types.Message) bool {
	if left.HasInternalControlProvenance() != right.HasInternalControlProvenance() {
		return false
	}
	if !equalContentReplacementTrust(left, right) {
		return false
	}
	leftScope, leftBound := left.InternalControlProvenanceScope()
	rightScope, rightBound := right.InternalControlProvenanceScope()
	if leftBound != rightBound || (leftBound && !leftScope.Equal(rightScope)) {
		return false
	}
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftJSON, rightJSON)
}

func equalContentReplacementTrust(left, right types.Message) bool {
	if len(left.Content) != len(right.Content) {
		return false
	}
	for index, leftBlock := range left.Content {
		leftReplacement, leftOK := leftBlock.(types.ContentReplacementBlock)
		rightReplacement, rightOK := right.Content[index].(types.ContentReplacementBlock)
		if leftOK != rightOK {
			return false
		}
		if leftOK {
			leftTrusted := leftReplacement.HasInternalReplacementProvenance()
			rightTrusted := rightReplacement.HasInternalReplacementProvenance()
			if leftTrusted != rightTrusted {
				return false
			}
			if leftTrusted {
				leftScope, _ := leftReplacement.InternalReplacementProvenanceScope()
				rightScope, _ := rightReplacement.InternalReplacementProvenanceScope()
				if !leftScope.Equal(rightScope) {
					return false
				}
			}
		}
	}
	return true
}

func equalControlTrustForCommit(left, right types.Message) bool {
	leftTrusted := left.HasInternalControlProvenance()
	rightTrusted := right.HasInternalControlProvenance()
	if leftTrusted != rightTrusted {
		return false
	}
	if !leftTrusted {
		return true
	}
	leftScope, leftBound := left.InternalControlProvenanceScope()
	rightScope, rightBound := right.InternalControlProvenanceScope()
	return !leftBound || !rightBound || leftScope.Equal(rightScope)
}

func equalContentReplacementTrustForCommit(left, right types.Message) bool {
	if len(left.Content) != len(right.Content) {
		return false
	}
	for index, leftBlock := range left.Content {
		leftReplacement, leftOK := leftBlock.(types.ContentReplacementBlock)
		rightReplacement, rightOK := right.Content[index].(types.ContentReplacementBlock)
		if leftOK != rightOK {
			return false
		}
		if !leftOK {
			continue
		}
		leftTrusted := leftReplacement.HasInternalReplacementProvenance()
		rightTrusted := rightReplacement.HasInternalReplacementProvenance()
		if leftTrusted != rightTrusted {
			return false
		}
		if !leftTrusted {
			continue
		}
		leftScope, leftBound := leftReplacement.InternalReplacementProvenanceScope()
		rightScope, rightBound := rightReplacement.InternalReplacementProvenanceScope()
		if leftBound && rightBound && !leftScope.Equal(rightScope) {
			return false
		}
	}
	return true
}

func messageProvenanceIdentity(message types.Message) []byte {
	identity := []byte("session-message-provenance/v2\x00")
	if message.HasInternalControlProvenance() {
		identity = append(identity, 1)
		scope, _ := message.InternalControlProvenanceScope()
		identity = append(identity, scope.AuthenticationPrefix()...)
	} else {
		identity = append(identity, 0)
	}
	for _, content := range message.Content {
		replacement, ok := content.(types.ContentReplacementBlock)
		if !ok {
			continue
		}
		identity = append(identity, 'R')
		if replacement.HasInternalReplacementProvenance() {
			identity = append(identity, 1)
			scope, _ := replacement.InternalReplacementProvenanceScope()
			identity = append(identity, scope.AuthenticationPrefix()...)
		} else {
			identity = append(identity, 0)
		}
	}
	return identity
}
