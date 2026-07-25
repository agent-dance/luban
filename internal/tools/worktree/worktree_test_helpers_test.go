package worktree

func resetBaseRefCacheForTests() {
	globalBaseRefCache.mu.Lock()
	globalBaseRefCache.values = make(map[string]string)
	globalBaseRefCache.mu.Unlock()
}
