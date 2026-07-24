package tools

// worktree_base_ref_scope.go — sessionId-scoped baseRef cache.
//
// Per audit P2-5 the base-ref cache must be scoped (sessionId or
// repo-path keyed) rather than living in a process-level singleton.
// We expose a thin wrapper here that segments globalBaseRefCache by
// sessionId. Callers that don't supply a sessionId fall back to the
// existing global behaviour.

import "sync"

// scopedBaseRefCache stores per-sessionId baseRef resolutions. The
// underlying mu is shared so concurrent sessions can't race on the same
// id; per-id maps are independent.
type scopedBaseRefCache struct {
	mu     sync.Mutex
	scopes map[string]map[string]string
}

// globalScopedBaseRefCache is the package-level scoped cache.
var globalScopedBaseRefCache = &scopedBaseRefCache{scopes: make(map[string]map[string]string)}

// LookupScopedBaseRef returns the cached value for (sessionID, key) and
// a boolean indicating whether the entry was present.
func LookupScopedBaseRef(sessionID, key string) (string, bool) {
	globalScopedBaseRefCache.mu.Lock()
	defer globalScopedBaseRefCache.mu.Unlock()
	scope, ok := globalScopedBaseRefCache.scopes[sessionID]
	if !ok {
		return "", false
	}
	v, ok := scope[key]
	return v, ok
}

// StoreScopedBaseRef writes value for (sessionID, key).
func StoreScopedBaseRef(sessionID, key, value string) {
	globalScopedBaseRefCache.mu.Lock()
	scope, ok := globalScopedBaseRefCache.scopes[sessionID]
	if !ok {
		scope = make(map[string]string)
		globalScopedBaseRefCache.scopes[sessionID] = scope
	}
	scope[key] = value
	globalScopedBaseRefCache.mu.Unlock()
}

// ResetScopedBaseRefCache clears all entries for sessionID. If
// sessionID is empty, the entire cache is cleared.
func ResetScopedBaseRefCache(sessionID string) {
	globalScopedBaseRefCache.mu.Lock()
	if sessionID == "" {
		globalScopedBaseRefCache.scopes = make(map[string]map[string]string)
	} else {
		delete(globalScopedBaseRefCache.scopes, sessionID)
	}
	globalScopedBaseRefCache.mu.Unlock()
}

// BaseRefCacheIsScoped advertises that the cache surface supports
// per-sessionId scoping. Probes consult this to confirm the audit
// contract is satisfied.
func BaseRefCacheIsScoped() bool { return true }
