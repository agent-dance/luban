package loop

// SetMaxTokens updates the per-request output budget for future turns. The
// non-input request envelope changes, so provider-native continuation state
// cannot be reused across this transition.
func (q *QueryLoop) SetMaxTokens(maxTokens int) {
	if maxTokens <= 0 || maxTokens == q.config.MaxTokens {
		return
	}
	q.invalidateProviderContinuation()
	q.config.MaxTokens = maxTokens
	q.config.MaxOutputTokens = maxTokens
	if q.ctxWindow != nil {
		q.ctxWindow.MaxOutputTokens = maxTokens
	}
}
