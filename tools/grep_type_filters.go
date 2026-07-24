// Package tools — grep_type_filters.go validates the --type filter the
// GrepTool forwards to ripgrep, mirroring src/tools/GrepTool/typeFilters.ts.
//
// On first call we shell out to `rg --type-list` and cache the parsed result;
// once we have the list, the validator is a fast in-memory lookup. When
// ripgrep is missing entirely we accept the value optimistically rather than
// blocking calls — TS does the same.
package tools

import (
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"sync"

	"github.com/agent-dance/luban/i18n"
)

var (
	grepKnownTypesOnce  sync.Once
	grepKnownTypesValue map[string]struct{}
	grepKnownTypesNames []string
	grepKnownTypesErr   error
)

// KnownGrepTypes returns the set of file types ripgrep recognises. The first
// call invokes `rg --type-list` (cached for the lifetime of the process). On
// failure (rg unavailable, parse error) the returned map is nil — callers
// should treat this as "unknown known set" and accept any non-empty type.
func KnownGrepTypes() (map[string]struct{}, []string) {
	grepKnownTypesOnce.Do(loadGrepKnownTypes)
	return grepKnownTypesValue, grepKnownTypesNames
}

// ResetGrepKnownTypesCache clears the in-memory type cache. Tests use this to
// re-run discovery against mocked rg locations.
func ResetGrepKnownTypesCache() {
	grepKnownTypesOnce = sync.Once{}
	grepKnownTypesValue = nil
	grepKnownTypesNames = nil
	grepKnownTypesErr = nil
}

// ValidateGrepType returns nil when the requested type is empty or known to
// ripgrep. Returns a structured error listing the closest known types when
// the value is rejected.
func ValidateGrepType(raw string) error {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return nil
	}
	known, names := KnownGrepTypes()
	if known == nil {
		// rg unavailable; accept optimistically — search.go still needs to
		// pass the value through, and we can't reasonably block all greps
		// because type-list lookup failed.
		return nil
	}
	if _, ok := known[value]; ok {
		return nil
	}
	suggestions := suggestGrepTypes(value, names)
	if len(suggestions) > 0 {
		return i18n.NewError(i18n.KeyToolGrepUnknownTypeSuggestion, raw, strings.Join(suggestions, ", "))
	}
	return i18n.NewError(i18n.KeyToolGrepUnknownTypeHint, raw)
}

func loadGrepKnownTypes() {
	rg, err := LocateRipgrep()
	if err != nil {
		grepKnownTypesErr = err
		return
	}
	out, err := exec.Command(rg, "--type-list").Output()
	if err != nil {
		grepKnownTypesErr = err
		return
	}
	types := parseTypeList(string(out))
	if len(types) == 0 {
		grepKnownTypesErr = fmt.Errorf("rg --type-list produced no entries")
		return
	}
	set := make(map[string]struct{}, len(types))
	names := make([]string, 0, len(types))
	for _, t := range types {
		set[t] = struct{}{}
		names = append(names, t)
	}
	sort.Strings(names)
	grepKnownTypesValue = set
	grepKnownTypesNames = names
}

// parseTypeList parses `rg --type-list` output into a slice of type names.
// Each line has the form "name: glob1, glob2, ..." — we just take the first
// colon-separated field.
func parseTypeList(raw string) []string {
	out := make([]string, 0, 64)
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		colon := strings.IndexByte(line, ':')
		if colon <= 0 {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(line[:colon]))
		if name != "" {
			out = append(out, name)
		}
	}
	return out
}

// suggestGrepTypes proposes up to three near-matches for an unknown type
// using a cheap edit-distance heuristic.
func suggestGrepTypes(value string, names []string) []string {
	if len(names) == 0 {
		return nil
	}
	type scored struct {
		name string
		dist int
	}
	scoredList := make([]scored, 0, len(names))
	for _, n := range names {
		d := simpleEditDistance(value, n)
		if d <= 3 {
			scoredList = append(scoredList, scored{name: n, dist: d})
		}
	}
	sort.Slice(scoredList, func(i, j int) bool {
		if scoredList[i].dist == scoredList[j].dist {
			return scoredList[i].name < scoredList[j].name
		}
		return scoredList[i].dist < scoredList[j].dist
	})
	if len(scoredList) > 3 {
		scoredList = scoredList[:3]
	}
	out := make([]string, 0, len(scoredList))
	for _, s := range scoredList {
		out = append(out, s.name)
	}
	return out
}

// simpleEditDistance is a small Levenshtein implementation used for type-name
// suggestions. The string lengths involved (rg has ~80 types) make a naive
// O(n*m) implementation more than adequate.
func simpleEditDistance(a, b string) int {
	if a == b {
		return 0
	}
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}
	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		curr[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = minInt3(prev[j]+1, curr[j-1]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[len(b)]
}

func minInt3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}
