// Package tools — file_read_state.go implements the ReadFileState shared
// store mirroring TS toolUseContext.readFileState (Map<absPath, entry>).
//
// Purpose:
//   - Read tool: store (path, mtime, offset, limit) so subsequent reads can
//     dedup via file_unchanged when nothing has changed.
//   - Edit tool: enforce Read→Edit ordering and detect stale reads (the Edit
//     would otherwise be applied against an out-of-date copy of the file).
//   - Write tool: stamp the entry with the post-write mtime so the very
//     next Read against the same path doesn't dedup against pre-write data.
//
// Thread-safety: all mutations go through a sync.Mutex. Get/Set/Clear are
// safe to call from multiple goroutines (sub-agents, background tasks, etc.).
package tools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/agent-dance/luban/loop"
)

// ReadLineRange is a 1-based, half-open line interval [StartLine, EndLine)
// that was made visible to the model by a successful Read call.
type ReadLineRange struct {
	StartLine int `json:"start_line"`
	EndLine   int `json:"end_line"`
}

// ReadFileEntry captures everything Edit/Read need to know about a prior
// successful read. Mirrors TS readFileState entry shape (timestamp, offset,
// limit, isPartialView).
type ReadFileEntry struct {
	// TimestampMs is the file's mtime at the time of the read, in
	// milliseconds since epoch (matching TS Math.floor(stats.mtimeMs)).
	TimestampMs int64

	// MtimeNs keeps the full filesystem timestamp for new entries. Legacy
	// entries leave it at zero and continue to use TimestampMs.
	MtimeNs int64

	// TotalBytes is used with the timestamp to decide whether two observations
	// belong to the same file version before their coverage is merged. It is a
	// diagnostic/fast-path hint only; ContentDigest is the version authority.
	TotalBytes int64

	// ContentDigest is the SHA-256 digest of the complete raw file snapshot,
	// including any BOM and original line endings. New Read/Edit/Write entries
	// always populate it. Empty is reserved for legacy callers and is never
	// sufficient to merge observations from separate reads.
	ContentDigest string

	// FileIdentity is the opaque filesystem identity captured from the same
	// already-open descriptor that produced ContentDigest and the model-visible
	// bytes. New digest-bearing evidence always populates it. Keeping the
	// os.FileInfo value lets os.SameFile apply the platform's native identity
	// rules without serializing inode/device details into user-visible state.
	FileIdentity os.FileInfo

	// Offset is the 1-based starting line of the read (0 means "from start").
	Offset int

	// Limit is the maximum number of lines read; 0 means "no limit".
	Limit int

	// OffsetSpecified and LimitSpecified distinguish an omitted value from an
	// explicit zero/one without relying on the legacy Offset sentinel.
	OffsetSpecified bool
	LimitSpecified  bool

	// CoverageKnown distinguishes new range-aware entries from legacy entries
	// constructed before coverage tracking existed.
	CoverageKnown bool

	// Coverage records all model-visible line ranges for the current file
	// version. Ranges are merged, sorted, and non-overlapping.
	Coverage []ReadLineRange

	// TotalLines is the line count for the observed file version.
	TotalLines int

	// CoverageComplete means the union of Coverage spans the whole file.
	// FullSnapshot is stricter: Content itself is the complete decoded file and
	// can therefore be used for the mtime-only stale-content fallback.
	CoverageComplete bool
	FullSnapshot     bool

	// IsPartialView is true only when the model-visible content was transformed
	// and no longer faithfully represents the corresponding bytes on disk (for
	// example, a truncated or filtered automatic injection). Ordinary ranged
	// Read calls are represented by Coverage and do not set this flag.
	IsPartialView bool

	// Content is an optional snapshot of the read content. Populated by Read,
	// consumed by Edit for fuzzy diffing. Empty means "snapshot unavailable".
	Content string

	// LastTool records which tool last touched this entry ("Read", "Edit",
	// "Write"). Empty for legacy entries.
	LastTool string

	// DedupEligible is true only for Read-origin text/notebook content that is
	// still represented by a model-visible tool result. Write/Edit/NotebookEdit
	// refresh timestamps for stale-write checks but leave this false.
	DedupEligible bool

	// Encoding is the detected source encoding of the file at read time.
	// Empty means UTF-8 / not applicable. Used by FileWrite to preserve the
	// original encoding when overwriting (mirroring TS).
	Encoding FileEncoding

	// BOM holds the literal byte-order-mark bytes (if any) that prefixed the
	// file. FileWrite reapplies them on overwrite when Encoding indicates a
	// BOM-bearing variant.
	BOM []byte
}

// ReadFileState is a thread-safe map keyed on the absolute path of a file
// (as resolved by filepath.Abs) holding the most recent ReadFileEntry.
//
// Use NewReadFileState to construct one; the zero value is also valid.
type ReadFileState struct {
	mu      sync.Mutex
	entries map[string]ReadFileEntry
	epoch   uint64
}

// NewReadFileState constructs an empty ReadFileState ready for use.
func NewReadFileState() *ReadFileState {
	return &ReadFileState{entries: make(map[string]ReadFileEntry), epoch: 1}
}

// Get returns the entry for absPath. If no entry exists the second return
// value is false.
func (s *ReadFileState) Get(absPath string) (ReadFileEntry, bool) {
	return s.getInScope("", absPath)
}

// GetForContext retrieves evidence in the active QueryLoop/actor/context
// scope. Direct tool calls without a loop context use the legacy local scope;
// retained or forged loop contexts fail closed.
func (s *ReadFileState) GetForContext(ctx context.Context, absPath string) (ReadFileEntry, bool) {
	scope, valid := activeReadEvidenceScope(ctx)
	if !valid {
		return ReadFileEntry{}, false
	}
	return s.getInScope(scope, absPath)
}

func (s *ReadFileState) getInScope(scope, absPath string) (ReadFileEntry, bool) {
	if s == nil {
		return ReadFileEntry{}, false
	}
	key := scopedReadStateKey(scope, absPath)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.entries == nil {
		return ReadFileEntry{}, false
	}
	entry, ok := s.entries[key]
	return cloneReadFileEntry(entry), ok
}

// Set stores an entry, overwriting any prior value at the same absolute path.
func (s *ReadFileState) Set(absPath string, entry ReadFileEntry) {
	s.setInScope("", absPath, entry)
}

// SetForContext replaces the current version in the active evidence scope.
func (s *ReadFileState) SetForContext(ctx context.Context, absPath string, entry ReadFileEntry) {
	scope, valid := activeReadEvidenceScope(ctx)
	if !valid {
		return
	}
	s.setInScope(scope, absPath, entry)
}

func (s *ReadFileState) setInScope(scope, absPath string, entry ReadFileEntry) {
	if s == nil {
		return
	}
	key := scopedReadStateKey(scope, absPath)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.entries == nil {
		s.entries = make(map[string]ReadFileEntry)
	}
	pruneOlderReadEvidenceScopes(s.entries, scope)
	s.entries[key] = cloneReadFileEntry(entry)
}

// RecordRead stores a successful direct Read observation. Observations of the
// same file version merge their visible ranges so a focused follow-up Read
// cannot revoke an earlier full-file observation. Mutating tools should keep
// using Set because their post-image replaces the prior version entirely.
func (s *ReadFileState) RecordRead(absPath string, entry ReadFileEntry) {
	s.recordReadInScope("", absPath, entry)
}

// RecordReadForContext merges a successful Read into the active evidence
// scope. Invalid retained contexts cannot create evidence.
func (s *ReadFileState) RecordReadForContext(ctx context.Context, absPath string, entry ReadFileEntry) {
	scope, valid := activeReadEvidenceScope(ctx)
	if !valid {
		return
	}
	s.recordReadInScope(scope, absPath, entry)
}

func (s *ReadFileState) recordReadInScope(scope, absPath string, entry ReadFileEntry) {
	if s == nil {
		return
	}
	key := scopedReadStateKey(scope, absPath)
	entry = cloneReadFileEntry(entry)
	entry.Coverage = mergeReadLineRanges(entry.Coverage)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.entries == nil {
		s.entries = make(map[string]ReadFileEntry)
	}
	pruneOlderReadEvidenceScopes(s.entries, scope)
	previous, ok := s.entries[key]
	if !ok || !sameReadFileVersion(previous, entry) {
		s.entries[key] = entry
		return
	}

	merged := entry
	merged.Coverage = mergeReadLineRanges(append(cloneReadLineRanges(previous.Coverage), entry.Coverage...))
	merged.CoverageComplete = entry.CoverageComplete || previous.CoverageComplete ||
		coverageSpansWholeFile(merged.Coverage, merged.TotalLines)
	if previous.FullSnapshot && !entry.FullSnapshot {
		merged.Content = previous.Content
		merged.FullSnapshot = true
		merged.Encoding = previous.Encoding
		merged.BOM = append([]byte(nil), previous.BOM...)
	}
	// A faithful direct Read remains usable even if a later transformed
	// attachment for the same version is observed. Only transformed-only
	// evidence is marked uneditable.
	merged.IsPartialView = previous.IsPartialView && entry.IsPartialView
	s.entries[key] = merged
}

// Clone returns an actor-local copy of the evidence ledger. The clone inherits
// currently visible evidence but future reads cannot authorize or invalidate
// the source actor's state.
func (s *ReadFileState) Clone() *ReadFileState {
	cloned := NewReadFileState()
	if s == nil {
		return cloned
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cloned.epoch = s.epoch
	for path, entry := range s.entries {
		cloned.entries[path] = cloneReadFileEntry(entry)
	}
	return cloned
}

// ResetContext invalidates all model-observation evidence after a session or
// context replacement. File version coordination is intentionally rebuilt by
// subsequent visible Read calls instead of referring to compacted-away text.
func (s *ReadFileState) ResetContext() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.epoch++
	s.entries = make(map[string]ReadFileEntry)
}

// Clear removes the entry for absPath. No-op if no entry exists.
func (s *ReadFileState) Clear(absPath string) {
	s.clearInScope("", absPath)
}

// ClearForContext removes one file only from the active evidence scope.
func (s *ReadFileState) ClearForContext(ctx context.Context, absPath string) {
	scope, valid := activeReadEvidenceScope(ctx)
	if !valid {
		return
	}
	s.clearInScope(scope, absPath)
}

func (s *ReadFileState) clearInScope(scope, absPath string) {
	if s == nil {
		return
	}
	key := scopedReadStateKey(scope, absPath)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.entries == nil {
		return
	}
	delete(s.entries, key)
}

func activeReadEvidenceScope(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", true
	}
	if execution, ok := loop.ToolExecutionContextFromContext(ctx); ok {
		return execution.ActiveReadEvidenceScope()
	}
	return "", true
}

func scopedReadStateKey(scope, path string) string {
	path = normalizeReadStateKey(path)
	if scope == "" {
		return path
	}
	return scope + "\x00" + path
}

func pruneOlderReadEvidenceScopes(entries map[string]ReadFileEntry, scope string) {
	if scope == "" {
		return
	}
	separator := strings.LastIndex(scope, "\x1f")
	if separator < 0 {
		return
	}
	family := scope[:separator+1]
	currentPrefix := scope + "\x00"
	for key := range entries {
		if strings.HasPrefix(key, family) && !strings.HasPrefix(key, currentPrefix) {
			delete(entries, key)
		}
	}
}

// Len returns the number of tracked entries (mainly for tests).
func (s *ReadFileState) Len() int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.entries)
}

func cloneReadLineRanges(values []ReadLineRange) []ReadLineRange {
	return append([]ReadLineRange(nil), values...)
}

func cloneReadFileEntry(entry ReadFileEntry) ReadFileEntry {
	entry.Coverage = cloneReadLineRanges(entry.Coverage)
	entry.BOM = append([]byte(nil), entry.BOM...)
	return entry
}

func sameReadFileVersion(left, right ReadFileEntry) bool {
	if left.ContentDigest == "" || right.ContentDigest == "" || left.ContentDigest != right.ContentDigest {
		return false
	}
	if left.FileIdentity == nil || right.FileIdentity == nil {
		// Digest-bearing production evidence is valid only when bound to the
		// exact descriptor identity that produced it. Digest-only observations
		// must never accumulate coverage into edit authorization.
		return false
	}
	return os.SameFile(left.FileIdentity, right.FileIdentity)
}

func readEntryMatchesFileIdentity(entry ReadFileEntry, info os.FileInfo) bool {
	return entry.FileIdentity != nil && info != nil && os.SameFile(entry.FileIdentity, info)
}

var errFileSnapshotChanged = errors.New("file snapshot changed while hashing")

func fileContentDigest(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// digestOpenFile hashes one complete raw snapshot from an already-authorized
// descriptor. The descriptor is rewound before returning. Metadata is checked
// on both sides of the read so an ordinary concurrent writer fails closed.
func digestOpenFile(file *os.File) (string, os.FileInfo, error) {
	if file == nil {
		return "", nil, os.ErrInvalid
	}
	before, err := file.Stat()
	if err != nil {
		return "", nil, err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", nil, err
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", nil, err
	}
	after, err := file.Stat()
	if err != nil {
		return "", nil, err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", nil, err
	}
	if !os.SameFile(before, after) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return "", nil, errFileSnapshotChanged
	}
	return hex.EncodeToString(hash.Sum(nil)), after, nil
}

// digestFileAtPath hashes the exact file identity represented by expected.
// It is used for digest-aware Read dedup without trusting path metadata alone.
func digestFileAtPath(path string, expected os.FileInfo) (string, os.FileInfo, error) {
	return digestFileAtPathWithHook(path, expected, nil)
}

// digestFileAtPathWithHook is the implementation seam used by the path-swap
// regression test. The hook runs only after the descriptor identity has been
// captured and must remain nil in production.
func digestFileAtPathWithHook(path string, expected os.FileInfo, afterOpen func()) (string, os.FileInfo, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return "", nil, err
	}
	if expected != nil && !os.SameFile(expected, opened) {
		return "", nil, errFileSnapshotChanged
	}
	if afterOpen != nil {
		afterOpen()
	}
	digest, snapshotInfo, err := digestOpenFile(file)
	if err != nil {
		return "", nil, err
	}
	// Revalidate the pathname after hashing. A rename can leave the opened fd
	// stable while replacing the name with a different inode; accepting that
	// digest would let Read incorrectly return file_unchanged for the new file.
	current, err := os.Stat(path)
	if err != nil || !os.SameFile(snapshotInfo, current) {
		return "", nil, errFileSnapshotChanged
	}
	return digest, snapshotInfo, nil
}

func mergeReadLineRanges(values []ReadLineRange) []ReadLineRange {
	filtered := make([]ReadLineRange, 0, len(values))
	for _, value := range values {
		if value.StartLine < 1 {
			value.StartLine = 1
		}
		if value.EndLine <= value.StartLine {
			continue
		}
		filtered = append(filtered, value)
	}
	if len(filtered) < 2 {
		return filtered
	}
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].StartLine == filtered[j].StartLine {
			return filtered[i].EndLine < filtered[j].EndLine
		}
		return filtered[i].StartLine < filtered[j].StartLine
	})
	merged := filtered[:1]
	for _, current := range filtered[1:] {
		last := &merged[len(merged)-1]
		if current.StartLine <= last.EndLine {
			if current.EndLine > last.EndLine {
				last.EndLine = current.EndLine
			}
			continue
		}
		merged = append(merged, current)
	}
	return merged
}

func coverageSpansWholeFile(ranges []ReadLineRange, totalLines int) bool {
	if totalLines == 0 {
		return true
	}
	merged := mergeReadLineRanges(ranges)
	return len(merged) == 1 && merged[0].StartLine <= 1 && merged[0].EndLine > totalLines
}

func readObservationCoverage(startLine, lineCount, totalLines int) ([]ReadLineRange, bool) {
	if totalLines == 0 {
		return nil, true
	}
	if startLine < 1 {
		startLine = 1
	}
	if lineCount <= 0 || startLine > totalLines {
		return nil, false
	}
	endLine := startLine + lineCount
	if endLine > totalLines+1 {
		endLine = totalLines + 1
	}
	ranges := []ReadLineRange{{StartLine: startLine, EndLine: endLine}}
	return ranges, coverageSpansWholeFile(ranges, totalLines)
}

func readStateTotalLines(content string) int {
	if content == "" {
		return 0
	}
	return strings.Count(content, "\n") + 1
}

func readEntryCoverageComplete(entry ReadFileEntry) bool {
	if entry.CoverageKnown {
		return entry.CoverageComplete || coverageSpansWholeFile(entry.Coverage, entry.TotalLines)
	}
	return !entry.IsPartialView && entry.Offset == 0 && entry.Limit == 0
}

func readEntryHasFullSnapshot(entry ReadFileEntry) bool {
	if entry.CoverageKnown {
		return entry.FullSnapshot
	}
	return !entry.IsPartialView && entry.Offset == 0 && entry.Limit == 0
}

// normalizeReadStateKey converts a path to its absolute form for map lookup.
// On failure (e.g. malformed path) returns the input unchanged so callers
// still get consistent behaviour.
func normalizeReadStateKey(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		return canonicalPathForComparison(abs)
	}
	return canonicalPathForComparison(path)
}

// defaultReadFileState is a process-wide default used by Edit/Write when no
// explicit ReadState is wired into the tool struct. Production callers
// (engine, sub-agents) should pass an explicit instance so reads and writes
// share the same state, but tests and stand-alone tool invocations can fall
// back to this singleton.
var defaultReadFileState = NewReadFileState()

// DefaultReadFileState returns the package-wide default ReadFileState. Edit
// and Write fall back to this when their ReadState field is nil.
func DefaultReadFileState() *ReadFileState {
	return defaultReadFileState
}
