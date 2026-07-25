package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// HashMCPConfig returns the stable connection hash used to decide whether an
// existing MCP connection and its fetched catalogues are still valid. Scope is
// intentionally excluded by the MCPServerConfig JSON tags, so provenance-only
// changes do not reconnect.
func HashMCPConfig(config MCPServerConfig) string {
	// MCPServerConfig's closed schema contains only JSON-safe field types.
	data, _ := json.Marshal(config)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])[:16]
}
