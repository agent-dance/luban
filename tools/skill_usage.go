package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/agent-dance/luban/brand"
)

// SkillUsageStore records per-user skill usage counts to disk so that the
// /skills picker can rank most-used skills first. Mirrors TS recordSkillUsage
// in src/utils/suggestions/skillUsageTracking.ts.
//
// On-disk format (JSON):
//
//	{
//	  "version": 1,
//	  "counts": {
//	    "commit": { "count": 42, "last_used": "2026-05-17T..." },
//	    "review": { "count": 17, "last_used": "..." }
//	  }
//	}
type SkillUsageStore struct {
	mu     sync.Mutex
	path   string
	loaded bool
	data   skillUsageData

	// nowFn is overrideable for tests.
	nowFn func() time.Time
}

type skillUsageData struct {
	Version int                          `json:"version"`
	Counts  map[string]*skillUsageRecord `json:"counts"`
}

type skillUsageRecord struct {
	Count    int       `json:"count"`
	LastUsed time.Time `json:"last_used"`
}

// NewSkillUsageStore returns a store that persists to the given file path.
// When path is empty, an in-memory only store is returned (no disk writes).
func NewSkillUsageStore(path string) *SkillUsageStore {
	return &SkillUsageStore{
		path:  path,
		nowFn: time.Now,
	}
}

// DefaultSkillUsageStore returns the per-user store at
// ~/<brand-config-dir>/skill-usage.json (e.g. ~/.luban-code/skill-usage.json),
// falling back to an in-memory store when no home directory is available.
func DefaultSkillUsageStore() *SkillUsageStore {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return NewSkillUsageStore("")
	}
	return NewSkillUsageStore(filepath.Join(home, brand.ConfigDirName, "skill-usage.json"))
}

// Record bumps the usage count for the given skill and updates last_used.
// Best-effort — disk write failures are swallowed (TS does the same; the
// ranking is a UX improvement, not correctness).
func (s *SkillUsageStore) Record(name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ensureLoadedLocked()

	rec, ok := s.data.Counts[name]
	if !ok {
		rec = &skillUsageRecord{}
		s.data.Counts[name] = rec
	}
	rec.Count++
	rec.LastUsed = s.nowFn().UTC()

	s.flushLocked()
}

// Count returns the recorded usage count for the given skill (0 if unseen).
func (s *SkillUsageStore) Count(name string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()
	if rec, ok := s.data.Counts[name]; ok {
		return rec.Count
	}
	return 0
}

// RankedNames returns the given names sorted by usage count desc (stable
// secondary sort: alphabetical for ties). Names not in the store sort after
// any name with non-zero usage.
func (s *SkillUsageStore) RankedNames(names []string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()

	out := make([]string, len(names))
	copy(out, names)
	sort.SliceStable(out, func(i, j int) bool {
		ci := 0
		if rec, ok := s.data.Counts[out[i]]; ok {
			ci = rec.Count
		}
		cj := 0
		if rec, ok := s.data.Counts[out[j]]; ok {
			cj = rec.Count
		}
		if ci != cj {
			return ci > cj
		}
		return out[i] < out[j]
	})
	return out
}

func (s *SkillUsageStore) ensureLoadedLocked() {
	if s.loaded {
		return
	}
	s.loaded = true
	s.data = skillUsageData{Version: 1, Counts: map[string]*skillUsageRecord{}}

	if s.path == "" {
		return
	}
	raw, err := os.ReadFile(s.path)
	if err != nil {
		return // missing or unreadable — start empty
	}
	var disk skillUsageData
	if err := json.Unmarshal(raw, &disk); err != nil {
		return
	}
	if disk.Counts == nil {
		disk.Counts = map[string]*skillUsageRecord{}
	}
	if disk.Version == 0 {
		disk.Version = 1
	}
	s.data = disk
}

func (s *SkillUsageStore) flushLocked() {
	if s.path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return
	}
	raw, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return
	}
	_ = os.Rename(tmp, s.path)
}
