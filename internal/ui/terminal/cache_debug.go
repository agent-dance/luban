package ui

import (
	"os"
	"strings"
	"time"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

const (
	cacheDebugMinDropTokens = 2000
	cacheDebugDropThreshold = 0.95
)

// CacheBreakDebugDetector tracks cache-read drops for opt-in renderer logging.
// It is intentionally local to renderers so ordinary loop behavior stays quiet.
type CacheBreakDebugDetector struct {
	prevRead    int
	prevCreate  int
	prevTime    time.Time
	callCount   int
	hasBaseline bool
}

// CacheBreakDebugEnabled reports whether renderer-level cache diagnostics are enabled.
func CacheBreakDebugEnabled() bool {
	for _, name := range []string{
		"LUBAN_CODE_CACHE_BREAK_DEBUG",
		"PROMPT_CACHE_BREAK_DEBUG",
	} {
		switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
		case "1", "true", "yes", "y", "on":
			return true
		}
	}
	return false
}

// Check returns a debug line when usage indicates a significant cache-read drop.
func (d *CacheBreakDebugDetector) Check(usage *types.Usage) string {
	return d.CheckInLanguage(i18n.DetectOrLoadLanguage(), usage)
}

// CheckInLanguage formats opt-in diagnostic copy in the active renderer
// language while retaining numeric measurements as parameters.
func (d *CacheBreakDebugDetector) CheckInLanguage(lang i18n.Language, usage *types.Usage) string {
	if usage == nil {
		return ""
	}
	d.callCount++
	now := time.Now()
	currRead := usage.CacheReadInputTokens
	currCreate := usage.CacheCreationInputTokens
	defer func() {
		d.prevRead = currRead
		d.prevCreate = currCreate
		d.prevTime = now
		d.hasBaseline = true
	}()
	if !d.hasBaseline || d.prevRead == 0 {
		return ""
	}
	tokenDrop := d.prevRead - currRead
	if tokenDrop < cacheDebugMinDropTokens {
		return ""
	}
	if float64(currRead) >= float64(d.prevRead)*cacheDebugDropThreshold {
		return ""
	}
	dropPercent := float64(tokenDrop) / float64(d.prevRead)
	return i18n.Format(lang, i18n.KeyRuntimeCacheBreakDebug,
		d.callCount,
		d.prevRead/1000,
		currRead/1000,
		tokenDrop/1000,
		dropPercent*100,
		currCreate/1000,
		d.prevCreate/1000,
		now.Sub(d.prevTime).Round(time.Second))
}
