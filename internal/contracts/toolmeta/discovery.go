// Package toolmeta defines implementation-neutral metadata contracts shared
// by tool producers, the registry, and discovery consumers.
package toolmeta

// Metadata controls whether a tool participates in deferred discovery and
// supplies internal search terms used to rank it. SearchHint is model-routing
// metadata, not user-visible copy.
type Metadata struct {
	ShouldDefer bool
	AlwaysLoad  bool
	SearchHint  string
}
