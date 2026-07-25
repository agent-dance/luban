package loop

// CompactResult describes the authoritative visible-history outcome of one
// manual compaction attempt. Compacted is true only after a semantic
// replacement and its post-compact cleanup have committed to the QueryLoop.
type CompactResult struct {
	Compacted          bool
	BeforeMessageCount int
	AfterMessageCount  int
}
