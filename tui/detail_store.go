package tui

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/agent-dance/luban/i18n"
)

const (
	memoryDetailSource = "memory"
	fileDetailSource   = "file"
)

var (
	// ErrDetailNotFound indicates that a syntactically valid reference is not
	// present in the selected store.
	ErrDetailNotFound = i18n.NewError(i18n.KeyTUIDetailStoreNotFound)
	// ErrInvalidDetailRef indicates that a reference does not belong to the
	// selected store or has been malformed.
	ErrInvalidDetailRef = i18n.NewError(i18n.KeyTUIDetailStoreInvalidReference)
)

// DetailRef is an immutable reference to exact evidence retained by a
// DetailStore. Key is opaque to callers.
type DetailRef struct {
	Source string `json:"source"`
	Key    string `json:"key"`
	Size   int    `json:"size"`
	Digest string `json:"sha256"`
}

// DetailStore retains exact evidence independently from its presentation
// summary. Implementations must not expose mutable store-owned byte slices.
type DetailStore interface {
	Put(key string, data []byte) (DetailRef, error)
	Get(ref DetailRef) ([]byte, error)
}

// ObservationEvidenceJournal durably indexes evidence references at the same
// boundary that publishes them into the in-memory observation graph. It lets a
// resumed session recover evidence even when the process exited before the
// broader lifecycle sidecar was rewritten.
type ObservationEvidenceJournal interface {
	SaveObservationEvidence(Observation) error
	LoadObservationEvidence() ([]Observation, error)
}

// MemoryDetailStore retains evidence for the lifetime of the process.
type MemoryDetailStore struct {
	mu      sync.RWMutex
	details map[string][]byte
}

// NewMemoryDetailStore creates an empty in-memory detail store.
func NewMemoryDetailStore() *MemoryDetailStore {
	return &MemoryDetailStore{details: make(map[string][]byte)}
}

// Put retains an immutable copy of data and returns a stable logical reference.
func (s *MemoryDetailStore) Put(key string, data []byte) (DetailRef, error) {
	ref, err := makeDetailRef(memoryDetailSource, key, data)
	if err != nil {
		return DetailRef{}, err
	}

	copyOfData := append([]byte(nil), data...)
	s.mu.Lock()
	s.details[ref.Digest] = copyOfData
	s.mu.Unlock()
	return ref, nil
}

// Get returns an independent copy of the retained evidence.
func (s *MemoryDetailStore) Get(ref DetailRef) ([]byte, error) {
	if err := validateDetailRef(ref, memoryDetailSource); err != nil {
		return nil, err
	}

	s.mu.RLock()
	data, ok := s.details[ref.Digest]
	if ok {
		data = append([]byte(nil), data...)
	}
	s.mu.RUnlock()
	if !ok {
		return nil, i18n.WrapInternalError(i18n.KeyTUIDetailStoreNotFoundKey, ErrDetailNotFound, ref.Key)
	}
	if len(data) != ref.Size {
		return nil, i18n.WrapInternalError(i18n.KeyTUIDetailStoreRetainedIntegrity, ErrInvalidDetailRef, ref.Key, len(data), ref.Size)
	}
	actualDigest := digestBytes(data)
	if actualDigest != ref.Digest {
		return nil, i18n.WrapInternalError(i18n.KeyTUIDetailStoreRetainedDigest, ErrInvalidDetailRef, ref.Key, actualDigest, ref.Digest)
	}
	return data, nil
}

// FileDetailStore retains evidence below a single artifact root. Logical keys
// never become path components; only validated SHA-256 digests reach the
// filesystem.
type FileDetailStore struct {
	root   string
	source string
	mu     sync.RWMutex
}

// NewFileDetailStore creates a file-backed detail store rooted at artifactRoot.
// The root and its shard directories are private to the current user.
func NewFileDetailStore(artifactRoot string) (*FileDetailStore, error) {
	if strings.TrimSpace(artifactRoot) == "" {
		return nil, i18n.WrapInternalError(i18n.KeyTUIDetailStoreArtifactRootEmpty, ErrInvalidDetailRef)
	}
	root, err := filepath.Abs(artifactRoot)
	if err != nil {
		return nil, i18n.WrapError(i18n.KeyTUIDetailStoreResolveArtifactRoot, err, artifactRoot)
	}
	root = filepath.Clean(root)
	if err := ensurePrivateDirectory(root); err != nil {
		return nil, i18n.WrapError(i18n.KeyTUIDetailStorePrepareArtifactRoot, err, root)
	}

	return &FileDetailStore{
		root:   root,
		source: fileDetailSource,
	}, nil
}

// Put atomically writes an immutable evidence object with mode 0600.
func (s *FileDetailStore) Put(key string, data []byte) (DetailRef, error) {
	ref, err := makeDetailRef(s.source, key, data)
	if err != nil {
		return DetailRef{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	path, err := s.pathForRef(ref)
	if err != nil {
		return DetailRef{}, err
	}
	dir := filepath.Dir(path)
	if err := ensurePrivateDirectory(dir); err != nil {
		return DetailRef{}, i18n.WrapError(i18n.KeyTUIDetailStorePrepareShard, err, dir)
	}

	if existing, readErr := s.readLocked(ref); readErr == nil {
		if bytes.Equal(existing, data) {
			return ref, nil
		}
	}

	tmp, err := os.CreateTemp(dir, ".detail-*")
	if err != nil {
		return DetailRef{}, i18n.WrapError(i18n.KeyTUIDetailStoreCreateTemporary, err, dir)
	}
	tmpPath := tmp.Name()
	closed := false
	defer func() {
		if !closed {
			_ = tmp.Close()
		}
		_ = os.Remove(tmpPath)
	}()

	if err := tmp.Chmod(0o600); err != nil {
		return DetailRef{}, i18n.WrapError(i18n.KeyTUIDetailStoreSecureTemporary, err, tmpPath)
	}
	if n, err := tmp.Write(data); err != nil {
		return DetailRef{}, i18n.WrapError(i18n.KeyTUIDetailStoreWrite, err, tmpPath)
	} else if n != len(data) {
		return DetailRef{}, i18n.WrapError(i18n.KeyTUIDetailStoreWrite, io.ErrShortWrite, tmpPath)
	}
	if err := tmp.Sync(); err != nil {
		return DetailRef{}, i18n.WrapError(i18n.KeyTUIDetailStoreSync, err, tmpPath)
	}
	if err := tmp.Close(); err != nil {
		closed = true
		return DetailRef{}, i18n.WrapError(i18n.KeyTUIDetailStoreClose, err, tmpPath)
	}
	closed = true
	if err := os.Rename(tmpPath, path); err != nil {
		return DetailRef{}, i18n.WrapError(i18n.KeyTUIDetailStorePublish, err, tmpPath, path)
	}
	if err := syncDirectory(dir); err != nil {
		return DetailRef{}, i18n.WrapError(i18n.KeyTUIDetailStoreSyncDirectory, err, dir)
	}
	return ref, nil
}

// Get safely reads and verifies evidence referenced by ref.
func (s *FileDetailStore) Get(ref DetailRef) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.readLocked(ref)
}

func (s *FileDetailStore) SaveObservationEvidence(observation Observation) error {
	if strings.TrimSpace(observation.ID) == "" {
		return i18n.WrapInternalError(i18n.KeyTUIDetailStoreJournalInvalid, ErrInvalidDetailRef)
	}
	observation = resetProcessLocalObservationDisclosure(cloneObservation(observation))
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ref := range append(append([]DetailRef(nil), observation.ResultRefs...), observation.EnvelopeRefs...) {
		if _, err := s.readLocked(ref); err != nil {
			return i18n.WrapError(i18n.KeyTUIDetailStoreJournalReference, err, observation.ID)
		}
	}
	data, err := json.Marshal(observation)
	if err != nil {
		return i18n.WrapError(i18n.KeyTUIDetailStoreEncodeJournal, err, observation.ID)
	}
	dir := filepath.Join(s.root, ".observations")
	if err := ensurePrivateDirectory(dir); err != nil {
		return i18n.WrapError(i18n.KeyTUIDetailStorePrepareJournal, err, dir)
	}
	path := filepath.Join(dir, digestBytes([]byte(observation.ID))+".json")
	if err := writePrivateAtomic(path, data); err != nil {
		return i18n.WrapError(i18n.KeyTUIDetailStorePublishJournal, err, path)
	}
	if err := syncDirectory(dir); err != nil {
		return i18n.WrapError(i18n.KeyTUIDetailStoreSyncJournal, err, dir)
	}
	return nil
}

func (s *FileDetailStore) LoadObservationEvidence() ([]Observation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	dir := filepath.Join(s.root, ".observations")
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, i18n.WrapError(i18n.KeyTUIDetailStoreReadJournal, err, dir)
	}
	observations := make([]Observation, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		info, err := os.Lstat(path)
		if err != nil {
			return nil, i18n.WrapError(i18n.KeyTUIDetailStoreInspectJournalEntry, err, entry.Name(), path)
		}
		if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
			return nil, i18n.WrapInternalError(i18n.KeyTUIDetailStoreJournalEntryInvalid, ErrInvalidDetailRef, entry.Name(), path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, i18n.WrapError(i18n.KeyTUIDetailStoreReadJournalEntry, err, entry.Name(), path)
		}
		var observation Observation
		if err := json.Unmarshal(data, &observation); err != nil {
			return nil, i18n.WrapError(i18n.KeyTUIDetailStoreDecodeJournalEntry, err, entry.Name())
		}
		if strings.TrimSpace(observation.ID) == "" {
			return nil, i18n.WrapInternalError(i18n.KeyTUIDetailStoreJournalEntryInvalid, ErrInvalidDetailRef, entry.Name(), path)
		}
		for _, ref := range append(append([]DetailRef(nil), observation.ResultRefs...), observation.EnvelopeRefs...) {
			if _, err := s.readLocked(ref); err != nil {
				return nil, i18n.WrapError(i18n.KeyTUIDetailStoreValidateJournal, err, observation.ID)
			}
		}
		observations = append(observations, cloneObservation(observation))
	}
	sort.Slice(observations, func(i, j int) bool { return observations[i].ID < observations[j].ID })
	return observations, nil
}

func writePrivateAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".journal-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	closed := false
	defer func() {
		if !closed {
			_ = tmp.Close()
		}
		_ = os.Remove(tmpPath)
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
	if err := tmp.Close(); err != nil {
		return err
	}
	closed = true
	return os.Rename(tmpPath, path)
}

func (s *FileDetailStore) readLocked(ref DetailRef) ([]byte, error) {
	path, err := s.pathForRef(ref)
	if err != nil {
		return nil, err
	}

	before, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, i18n.WrapInternalError(i18n.KeyTUIDetailStoreNotFoundKey, ErrDetailNotFound, ref.Key)
		}
		return nil, i18n.WrapError(i18n.KeyTUIDetailStoreInspectDetail, err, path)
	}
	if !before.Mode().IsRegular() {
		return nil, i18n.WrapInternalError(i18n.KeyTUIDetailStoreDetailNotRegular, ErrInvalidDetailRef, path)
	}
	if before.Mode().Perm()&0o077 != 0 {
		return nil, i18n.WrapInternalError(i18n.KeyTUIDetailStoreDetailPermissions, ErrInvalidDetailRef, path, fmt.Sprintf("%04o", before.Mode().Perm()))
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, i18n.WrapError(i18n.KeyTUIDetailStoreOpenDetail, err, path)
	}
	defer f.Close()
	after, err := f.Stat()
	if err != nil {
		return nil, i18n.WrapError(i18n.KeyTUIDetailStoreStatDetail, err, path)
	}
	if !after.Mode().IsRegular() || !os.SameFile(before, after) {
		return nil, i18n.WrapInternalError(i18n.KeyTUIDetailStoreDetailChanged, ErrInvalidDetailRef, path)
	}
	if after.Size() != int64(ref.Size) {
		return nil, i18n.WrapInternalError(i18n.KeyTUIDetailStoreDetailSize, ErrInvalidDetailRef, path, after.Size(), ref.Size)
	}

	data, err := io.ReadAll(io.LimitReader(f, int64(ref.Size)+1))
	if err != nil {
		return nil, i18n.WrapError(i18n.KeyTUIDetailStoreReadDetail, err, path)
	}
	if len(data) != ref.Size {
		return nil, i18n.WrapInternalError(i18n.KeyTUIDetailStoreDetailIntegrity, ErrInvalidDetailRef, path, len(data), ref.Size)
	}
	actualDigest := digestBytes(data)
	if actualDigest != ref.Digest {
		return nil, i18n.WrapInternalError(i18n.KeyTUIDetailStoreDetailDigest, ErrInvalidDetailRef, path, actualDigest, ref.Digest)
	}
	return data, nil
}

func (s *FileDetailStore) pathForRef(ref DetailRef) (string, error) {
	if err := validateDetailRef(ref, s.source); err != nil {
		return "", err
	}
	path := filepath.Join(s.root, ref.Digest[:2], ref.Digest[2:]+".detail")
	rel, err := filepath.Rel(s.root, path)
	if err != nil {
		return "", i18n.WrapError(i18n.KeyTUIDetailStoreRelativizePath, err, path, s.root)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", i18n.WrapInternalError(i18n.KeyTUIDetailStorePathEscapesRoot, ErrInvalidDetailRef, path, s.root)
	}
	return path, nil
}

func makeDetailRef(source, logicalKey string, data []byte) (DetailRef, error) {
	if logicalKey == "" {
		return DetailRef{}, i18n.WrapInternalError(i18n.KeyTUIDetailStoreLogicalKeyInvalid, ErrInvalidDetailRef, logicalKey)
	}
	return DetailRef{
		Source: source,
		Key:    logicalKey,
		Size:   len(data),
		Digest: digestBytes(data),
	}, nil
}

func validateDetailRef(ref DetailRef, source string) error {
	if ref.Source != source {
		return i18n.WrapInternalError(i18n.KeyTUIDetailStoreSourceMismatch, ErrInvalidDetailRef, ref.Source, source)
	}
	if ref.Size < 0 || ref.Key == "" {
		return i18n.WrapInternalError(i18n.KeyTUIDetailStoreReferenceMalformed, ErrInvalidDetailRef, ref.Key, ref.Size)
	}
	if len(ref.Digest) != sha256.Size*2 {
		return i18n.WrapInternalError(i18n.KeyTUIDetailStoreDigestMalformed, ErrInvalidDetailRef, ref.Digest)
	}
	if _, err := hex.DecodeString(ref.Digest); err != nil {
		return i18n.WrapInternalError(i18n.KeyTUIDetailStoreDigestMalformed, ErrInvalidDetailRef, ref.Digest)
	}
	return nil
}

func digestBytes(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func ensurePrivateDirectory(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return i18n.NewError(i18n.KeyTUIDetailStorePathNotRealDirectory, path)
	}
	if info.Mode().Perm() != 0o700 {
		if err := os.Chmod(path, 0o700); err != nil {
			return err
		}
	}
	return nil
}

func syncDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

var (
	_ DetailStore = (*MemoryDetailStore)(nil)
	_ DetailStore = (*FileDetailStore)(nil)
)
