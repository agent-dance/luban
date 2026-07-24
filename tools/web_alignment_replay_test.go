package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type webFetchReplayCase struct {
	Name     string                   `json:"name"`
	Input    webFetchReplayInput      `json:"input"`
	Expected webFetchNormalizedResult `json:"expected"`
}

type webFetchReplayInput struct {
	URL       string `json:"url"`
	Prompt    string `json:"prompt"`
	Body      string `json:"body"`
	Truncated bool   `json:"truncated"`
	CacheHit  bool   `json:"cacheHit"`
	Error     string `json:"error,omitempty"`
}

type webSearchReplayCase struct {
	Name     string                    `json:"name"`
	Input    webSearchReplayInput      `json:"input"`
	Expected webSearchNormalizedResult `json:"expected"`
}

type webSearchReplayInput struct {
	Query          string         `json:"query"`
	AllowedDomains []string       `json:"allowedDomains,omitempty"`
	BlockedDomains []string       `json:"blockedDomains,omitempty"`
	Results        []searchResult `json:"results,omitempty"`
	CacheHit       bool           `json:"cacheHit"`
	FallbackReason string         `json:"fallbackReason,omitempty"`
	Error          string         `json:"error,omitempty"`
}

func loadWebFetchReplayCase(t *testing.T, path string) webFetchReplayCase {
	t.Helper()
	var c webFetchReplayCase
	readJSONFixture(t, path, &c)
	return c
}

func loadWebSearchReplayCase(t *testing.T, path string) webSearchReplayCase {
	t.Helper()
	var c webSearchReplayCase
	readJSONFixture(t, path, &c)
	return c
}

func readJSONFixture(t *testing.T, path string, out any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	if err := json.Unmarshal(data, out); err != nil {
		t.Fatalf("unmarshal fixture %s: %v", path, err)
	}
}

func replayFixturePath(parts ...string) string {
	all := append([]string{"testdata", "web_alignment"}, parts...)
	return filepath.Join(all...)
}
