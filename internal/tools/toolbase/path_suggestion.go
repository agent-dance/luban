package toolbase

import (
	"os"
	"path/filepath"
	"strings"
)

// SuggestNearbyPath returns the closest sibling path for a misspelled input.
func SuggestNearbyPath(filePath, cwd string) string {
	targetName := strings.TrimSpace(filepath.Base(filePath))
	if targetName == "" || targetName == "." || targetName == string(filepath.Separator) {
		return ""
	}
	parent := filepath.Dir(filePath)
	if parent == "." || parent == "" {
		parent = cwd
	}
	if best := closestEntryInDir(parent, targetName); best != "" {
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
	previous := make([]int, len(b)+1)
	current := make([]int, len(b)+1)
	for index := range previous {
		previous[index] = index
	}
	for i := 1; i <= len(a); i++ {
		current[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 0
			if a[i-1] != b[j-1] {
				cost = 1
			}
			current[j] = min(previous[j]+1, current[j-1]+1, previous[j-1]+cost)
		}
		copy(previous, current)
	}
	return previous[len(b)]
}
