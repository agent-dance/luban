package permissions

import (
	"os"
	"runtime"
	"testing"
)

func TestContainsAny_Positive(t *testing.T) {
	tests := []struct {
		s       string
		substrs []string
	}{
		{"12:devices:/docker/abc123", []string{"docker"}},
		{"12:devices:/lxc/container1", []string{"docker", "lxc"}},
		{"1:name=systemd:/kubepods/pod-xyz", []string{"kubepods"}},
		{"hello world", []string{"world"}},
		{"containerd-shim", []string{"docker", "lxc", "kubepods", "containerd"}},
	}
	for _, tt := range tests {
		if !containsAny(tt.s, tt.substrs...) {
			t.Errorf("containsAny(%q, %v) = false, want true", tt.s, tt.substrs)
		}
	}
}

func TestContainsAny_Negative(t *testing.T) {
	tests := []struct {
		s       string
		substrs []string
	}{
		{"normal host cgroup", []string{"docker", "lxc", "kubepods", "containerd"}},
		{"", []string{"docker"}},
		{"some text", []string{}},
		{"foobar", []string{"baz", "qux"}},
	}
	for _, tt := range tests {
		if containsAny(tt.s, tt.substrs...) {
			t.Errorf("containsAny(%q, %v) = true, want false", tt.s, tt.substrs)
		}
	}
}

func TestIsInContainer_ReturnsBool(t *testing.T) {
	// On a normal dev machine (not in a container), isInContainer should return false.
	// We only assert the return type is bool and no panic occurs.
	result := isInContainer()
	// On CI or Docker environments this may be true; just ensure no crash.
	_ = result
}

func TestValidateEnvironmentForBypass_NonRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Getuid not available on Windows")
	}
	// When running tests as a normal user (non-root), the function should return nil.
	if os.Getuid() == 0 {
		t.Skip("test must run as non-root user")
	}
	err := ValidateEnvironmentForBypass()
	if err != nil {
		t.Errorf("ValidateEnvironmentForBypass() = %v, want nil for non-root user", err)
	}
}
