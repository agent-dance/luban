package inspect

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"sort"
	"sync"

	"github.com/agent-dance/luban/internal/runtime/flight"
	toolfile "github.com/agent-dance/luban/internal/tools/file"
)

const (
	maximumEvidenceNamespaces          = 32
	maximumEvidenceEntriesPerNamespace = 65_536
	maximumEvidenceEntriesTotal        = 131_072
)

// evidenceStore remembers source fragments already made model-visible. It is
// deliberately actor-local: WithRuntime shares it for the same actor while
// WithRuntimeAndReadState creates a new ledger for a child actor.
type evidenceStore struct {
	mu         sync.Mutex
	namespaces map[string]*evidenceNamespace
	serial     uint64
}

type evidenceNamespace struct {
	seen       map[string]struct{}
	lastAccess uint64
}

type evidenceView struct {
	seen map[string]struct{}
}

type evidenceObservation struct {
	key string
}

func newEvidenceStore() *evidenceStore {
	return &evidenceStore{namespaces: make(map[string]*evidenceNamespace)}
}

func evidenceNamespaceKey(workspace workspaceSnapshot) string {
	return workspace.root + "\x00" + workspace.sessionID + "\x00" +
		workspace.runID + "\x00" + workspace.cacheLineageID + "\x00" +
		workspace.historyEpoch + "\x00" + integerString(int(workspace.workspaceRevision))
}

// withNamespace serializes projection and observation. Concurrent sibling
// Inspect calls may finish in either order, but exactly one of them emits any
// shared fragment and every reference is backed by content in the same result
// batch or an earlier provider-visible result.
func (s *evidenceStore) withNamespace(namespace string, fn func(*evidenceView) error) error {
	if s == nil {
		return fn(&evidenceView{seen: make(map[string]struct{})})
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.namespaces == nil {
		s.namespaces = make(map[string]*evidenceNamespace)
	}
	s.serial++
	entry := s.namespaces[namespace]
	if entry == nil {
		if len(s.namespaces) >= maximumEvidenceNamespaces {
			s.evictOldestNamespace()
		}
		entry = &evidenceNamespace{seen: make(map[string]struct{})}
		s.namespaces[namespace] = entry
	}
	entry.lastAccess = s.serial
	if err := fn(&evidenceView{seen: entry.seen}); err != nil {
		return err
	}
	if len(entry.seen) > maximumEvidenceEntriesPerNamespace {
		// Losing deduplication state only causes source to be emitted again; it
		// never loses evidence. Resetting one namespace keeps memory bounded.
		entry.seen = make(map[string]struct{})
	}
	for s.totalEntries() > maximumEvidenceEntriesTotal && len(s.namespaces) > 1 {
		s.evictOldestNamespaceExcept(namespace)
	}
	return nil
}

func (s *evidenceStore) observe(namespace string, observations []evidenceObservation) {
	if s == nil || namespace == "" || len(observations) == 0 {
		return
	}
	_ = s.withNamespace(namespace, func(view *evidenceView) error {
		view.observe(observations)
		return nil
	})
}

func (s *evidenceStore) evictOldestNamespace() {
	s.evictOldestNamespaceExcept("")
}

func (s *evidenceStore) evictOldestNamespaceExcept(excluded string) {
	var oldestKey string
	var oldestAccess uint64
	for key, entry := range s.namespaces {
		if key == excluded {
			continue
		}
		if oldestKey == "" || entry.lastAccess < oldestAccess {
			oldestKey = key
			oldestAccess = entry.lastAccess
		}
	}
	delete(s.namespaces, oldestKey)
}

func (s *evidenceStore) totalEntries() int {
	total := 0
	for _, entry := range s.namespaces {
		total += len(entry.seen)
	}
	return total
}

func (v *evidenceView) contains(key string) bool {
	if v == nil || v.seen == nil {
		return false
	}
	_, ok := v.seen[key]
	return ok
}

func (v *evidenceView) observe(observations []evidenceObservation) {
	if v == nil {
		return
	}
	if v.seen == nil {
		v.seen = make(map[string]struct{}, len(observations))
	}
	for _, observation := range observations {
		if observation.key != "" {
			v.seen[observation.key] = struct{}{}
		}
	}
}

func evidenceKey(path string, line, startColumn, endColumn int, content string) string {
	digest := sha256.Sum256([]byte(path + "\x00" + integerString(line) + "\x00" +
		integerString(startColumn) + "\x00" + integerString(endColumn) + "\x00" + content))
	return hex.EncodeToString(digest[:])
}

func evidenceReference(path string, startLine, endLine, startColumn, endColumn int, keys []string) string {
	hasher := sha256.New()
	hasher.Write([]byte(path))
	hasher.Write([]byte{0})
	hasher.Write([]byte(integerString(startLine)))
	hasher.Write([]byte{0})
	hasher.Write([]byte(integerString(endLine)))
	hasher.Write([]byte{0})
	hasher.Write([]byte(integerString(startColumn)))
	hasher.Write([]byte{0})
	hasher.Write([]byte(integerString(endColumn)))
	for _, key := range keys {
		hasher.Write([]byte{0})
		hasher.Write([]byte(key))
	}
	return "e_" + hex.EncodeToString(hasher.Sum(nil)[:8])
}

func evidenceSpanKey(path string, line, startColumn, endColumn int, content string) string {
	return evidenceKey(path, line, startColumn, endColumn, content)
}

type visibleReadEntry struct {
	path  string
	entry toolfile.ReadFileEntry
}

// visibleEvidenceReceipt is pending until the runtime proves that its exact
// model wire entered visible history. It is one-shot and also fences the
// workspace mutation epoch captured during source acquisition.
type visibleEvidenceReceipt struct {
	mu           sync.Mutex
	committed    bool
	expectedHash [sha256.Size]byte
	ctx          context.Context
	entries      []visibleReadEntry
	store        *evidenceStore
	namespace    string
	observations []evidenceObservation
	workspace    workspaceSnapshot
	tool         *Tool
}

func (r *visibleEvidenceReceipt) commit(content string) bool {
	if r == nil || sha256.Sum256([]byte(content)) != r.expectedHash {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.committed || !r.workspace.historyBound {
		return false
	}
	if r.workspace.workspaceRevisionBound &&
		(r.tool == nil || r.tool.WorkspaceRevisions == nil ||
			!r.tool.WorkspaceRevisions.MatchesEpoch(r.workspace.root, flight.Epoch(r.workspace.workspaceRevision))) {
		return false
	}
	for _, pending := range r.entries {
		if pending.path == "" {
			continue
		}
		r.tool.readState.RecordReadForContext(r.ctx, pending.path, pending.entry)
	}
	r.store.observe(r.namespace, r.observations)
	r.committed = true
	return true
}

func (t *Tool) newVisibleEvidenceReceipt(
	ctx context.Context,
	workspace workspaceSnapshot,
	content string,
	page Result,
	state *paginationState,
	observations []evidenceObservation,
) *visibleEvidenceReceipt {
	if t == nil || t.readState == nil || state == nil || !workspace.historyBound {
		return nil
	}
	entries := t.visibleReadEntries(ctx, page.Snippets, state.batch.sources)
	return &visibleEvidenceReceipt{
		expectedHash: sha256.Sum256([]byte(content)), ctx: ctx, entries: entries,
		store: t.evidence, namespace: evidenceNamespaceKey(workspace),
		observations: append([]evidenceObservation(nil), observations...),
		workspace:    workspace, tool: t,
	}
}

func (t *Tool) visibleReadEntries(ctx context.Context, snippets []Snippet, sources map[string]sourceSnapshot) []visibleReadEntry {
	byPath := make(map[string][]Snippet)
	for _, snippet := range snippets {
		byPath[snippet.Path] = append(byPath[snippet.Path], snippet)
	}
	paths := make([]string, 0, len(byPath))
	for path := range byPath {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	entries := make([]visibleReadEntry, 0, len(paths))
	for _, displayPath := range paths {
		source, ok := sources[displayPath]
		if !ok || source.conflicted || source.absPath == "" || source.entry.ContentDigest == "" || source.entry.FileIdentity == nil {
			continue
		}
		coverage := make([]toolfile.ReadLineRange, 0, len(byPath[displayPath]))
		columnSpans := make(map[int][][2]int)
		for _, snippet := range byPath[displayPath] {
			if snippet.StartColumn == 0 && snippet.EndColumn == 0 {
				start, end := snippet.StartLine, snippet.EndLine+1
				if source.entry.TotalLines > 0 && end > source.entry.TotalLines+1 {
					end = source.entry.TotalLines + 1
				}
				if start > 0 && end > start {
					coverage = append(coverage, toolfile.ReadLineRange{StartLine: start, EndLine: end})
				}
				continue
			}
			columnSpans[snippet.StartLine] = append(columnSpans[snippet.StartLine], [2]int{snippet.StartColumn, snippet.EndColumn})
		}
		for line, spans := range columnSpans {
			length := source.lineByteLengths[line]
			if length > 0 && spansCoverWholeLine(spans, length) {
				coverage = append(coverage, toolfile.ReadLineRange{StartLine: line, EndLine: line + 1})
			}
		}
		entry := source.entry
		entry.Coverage = mergeVisibleLineRanges(coverage)
		entry.CoverageComplete = visibleCoverageComplete(entry.Coverage, entry.TotalLines)
		entry.FullSnapshot = false
		entry.Content = ""
		entry.LastTool = "Inspect"
		entry.DedupEligible = false
		if previous, found := t.readState.GetForContext(ctx, source.absPath); found && sameVisibleSourceVersion(previous, source.entry) {
			entry.Coverage = mergeVisibleLineRanges(append(previous.Coverage, entry.Coverage...))
			entry.CoverageComplete = visibleCoverageComplete(entry.Coverage, entry.TotalLines)
		}
		if entry.CoverageComplete && source.fullContentRead {
			entry.FullSnapshot = true
			entry.Content = source.fullContent
		}
		entries = append(entries, visibleReadEntry{path: source.absPath, entry: entry})
	}
	return entries
}

func spansCoverWholeLine(spans [][2]int, length int) bool {
	if length == 0 {
		return true
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i][0] < spans[j][0] })
	next := 1
	for _, span := range spans {
		if span[0] > next || span[1] < next {
			continue
		}
		next = span[1] + 1
		if next > length {
			return true
		}
	}
	return false
}

func mergeVisibleLineRanges(ranges []toolfile.ReadLineRange) []toolfile.ReadLineRange {
	if len(ranges) < 2 {
		return append([]toolfile.ReadLineRange(nil), ranges...)
	}
	sort.Slice(ranges, func(i, j int) bool { return ranges[i].StartLine < ranges[j].StartLine })
	out := ranges[:1]
	for _, current := range ranges[1:] {
		last := &out[len(out)-1]
		if current.StartLine <= last.EndLine {
			if current.EndLine > last.EndLine {
				last.EndLine = current.EndLine
			}
			continue
		}
		out = append(out, current)
	}
	return out
}

func visibleCoverageComplete(ranges []toolfile.ReadLineRange, totalLines int) bool {
	if totalLines == 0 {
		return true
	}
	merged := mergeVisibleLineRanges(ranges)
	return len(merged) == 1 && merged[0].StartLine <= 1 && merged[0].EndLine > totalLines
}

func sameVisibleSourceVersion(left, right toolfile.ReadFileEntry) bool {
	return left.ContentDigest != "" && left.ContentDigest == right.ContentDigest &&
		left.FileIdentity != nil && right.FileIdentity != nil && os.SameFile(left.FileIdentity, right.FileIdentity)
}
