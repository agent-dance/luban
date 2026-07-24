//go:build windows

package sandbox

func init() {
	// NoopBackend is always registered as the final fallback in Detect().
	// No platform-specific backend is added here.
}
