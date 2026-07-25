//go:build windows

package sandbox

func init() {
	// Windows currently has no platform-specific sandbox backend. Detect returns
	// the unsandboxed backend when the platform list remains empty.
}
