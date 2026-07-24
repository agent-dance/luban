// Package tools — glob_sort_mtime.go provides the helper TS uses to sort
// matched paths by mtime ascending so ripgrep --sort=modified parity is
// preserved.
//
// Mirrors src/utils/glob.ts (--sort=modified). The sort is stable; paths whose
// stat fails (e.g. removed between match and stat, permission denied) sink to
// the bottom rather than aborting the call. Each path is statted at most once.
package tools

import (
	"os"
	"path/filepath"
	"sort"
)

// SortByMtime sorts paths in place, oldest first. Files whose stat fails are
// pushed to the bottom of the slice so callers still get a complete result.
// Returns the (possibly partial) error encountered while statting; callers
// can ignore this safely — the slice is always returned in a usable order.
func SortByMtime(paths []string) error {
	if len(paths) <= 1 {
		return nil
	}
	type record struct {
		path    string
		modTime int64
		ok      bool
	}
	records := make([]record, len(paths))
	var firstErr error
	for i, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			records[i] = record{path: p, ok: false}
			continue
		}
		records[i] = record{path: p, modTime: info.ModTime().UnixNano(), ok: true}
	}
	sort.SliceStable(records, func(i, j int) bool {
		// successful stats always rank above failed ones
		if records[i].ok != records[j].ok {
			return records[i].ok
		}
		if !records[i].ok {
			// preserve original order between two failed stats
			return false
		}
		if records[i].modTime == records[j].modTime {
			return records[i].path < records[j].path
		}
		return records[i].modTime < records[j].modTime
	})
	for i, rec := range records {
		paths[i] = rec.path
	}
	return firstErr
}

// SortByMtimeRelativeTo behaves like SortByMtime, resolving each path against
// the supplied root when the entry is relative. The original entries in paths
// are left untouched (no joining); the rooted path is only used for stat.
func SortByMtimeRelativeTo(paths []string, root string) error {
	if len(paths) <= 1 {
		return nil
	}
	type record struct {
		path    string
		modTime int64
		ok      bool
	}
	records := make([]record, len(paths))
	var firstErr error
	for i, p := range paths {
		statPath := p
		if root != "" && !filepath.IsAbs(p) {
			statPath = filepath.Join(root, p)
		}
		info, err := os.Stat(statPath)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			records[i] = record{path: p, ok: false}
			continue
		}
		records[i] = record{path: p, modTime: info.ModTime().UnixNano(), ok: true}
	}
	sort.SliceStable(records, func(i, j int) bool {
		if records[i].ok != records[j].ok {
			return records[i].ok
		}
		if !records[i].ok {
			return false
		}
		if records[i].modTime == records[j].modTime {
			return records[i].path < records[j].path
		}
		return records[i].modTime < records[j].modTime
	})
	for i, rec := range records {
		paths[i] = rec.path
	}
	return firstErr
}
