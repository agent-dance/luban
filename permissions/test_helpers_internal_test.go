package permissions

import "testing"

func installNoopSafetyChecks(t *testing.T) {
	t.Helper()
	SetSafetyConfig(SafetyConfig{
		DangerousCommandChecker: func(string) string { return "" },
		BashProtectedPathChecker: func(string) (bool, string) {
			return false, ""
		},
	})
	t.Cleanup(func() { SetSafetyConfig(SafetyConfig{}) })
}
