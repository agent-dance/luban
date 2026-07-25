package engine

// CompactResult describes the authoritative durable outcome of one manual
// compaction attempt. ContextGeneration is populated only when the session
// manager confirms a persisted generation.
type CompactResult struct {
	Compacted          bool
	BeforeMessageCount int
	AfterMessageCount  int
	ContextGeneration  uint64
}
