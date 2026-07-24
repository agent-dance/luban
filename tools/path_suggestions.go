package tools

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/agent-dance/luban/i18n"
)

func fileReadNotFoundError(filePath string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return i18n.NewError(i18n.KeyToolLegacyAFileNotFound, filePath)
	}

	if suggestion := suggestNearbyPath(filePath, cwd); suggestion != "" {
		return i18n.NewError(i18n.KeyToolLegacyAFileNotFoundSuggestion, cwd, suggestion)
	}
	return i18n.NewError(i18n.KeyToolLegacyAFileNotFoundInCWD, cwd)
}

func suggestNearbyPath(filePath, cwd string) string {
	targetName := strings.TrimSpace(filepath.Base(filePath))
	if targetName == "" || targetName == "." || targetName == string(filepath.Separator) {
		return ""
	}

	parent := filepath.Dir(filePath)
	if parent == "." || parent == "" {
		parent = cwd
	}

	best := closestEntryInDir(parent, targetName)
	if best != "" {
		return best
	}

	return closestEntryInDir(cwd, targetName)
}

func closestEntryInDir(dir, targetName string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}

	targetLower := strings.ToLower(targetName)
	bestDistance := -1
	bestPath := ""
	for _, entry := range entries {
		name := entry.Name()
		distance := levenshteinDistance(strings.ToLower(name), targetLower)
		threshold := max(2, len(targetLower)/3)
		if distance > threshold {
			continue
		}
		if bestDistance == -1 || distance < bestDistance || (distance == bestDistance && name < filepath.Base(bestPath)) {
			bestDistance = distance
			bestPath = filepath.Join(dir, name)
		}
	}
	return bestPath
}

func levenshteinDistance(a, b string) int {
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
	for j := 0; j <= len(b); j++ {
		prev[j] = j
	}

	for i := 1; i <= len(a); i++ {
		curr[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 0
			if a[i-1] != b[j-1] {
				cost = 1
			}
			curr[j] = min(
				prev[j]+1,
				curr[j-1]+1,
				prev[j-1]+cost,
			)
		}
		copy(prev, curr)
	}
	return prev[len(b)]
}
