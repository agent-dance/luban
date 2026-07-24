package permissions

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/agent-dance/luban/i18n"
)

// ValidateEnvironmentForBypass checks that the runtime environment is safe
// for running in --allow-all mode. Returns an error if the environment is
// considered unsafe.
func ValidateEnvironmentForBypass() error {
	// 1. 禁止 root 运行（除非在沙箱/Docker 内）
	// 仅在 Linux/macOS 上检查（Windows 没有 Getuid）
	if runtime.GOOS != "windows" {
		if os.Getuid() == 0 && !isInContainer() {
			return fmt.Errorf("%s", permissionText(i18n.KeyPermissionEnvironmentRoot))
		}
	}
	return nil
}

// isInContainer checks if the process is running inside a Docker/Podman/LXC container.
func isInContainer() bool {
	// Check for Docker
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	// Check for Podman
	if _, err := os.Stat("/run/.containerenv"); err == nil {
		return true
	}
	// Check cgroup for container indicators (Linux only)
	if runtime.GOOS == "linux" {
		data, err := os.ReadFile("/proc/1/cgroup")
		if err == nil {
			s := string(data)
			if containsAny(s, "docker", "lxc", "kubepods", "containerd") {
				return true
			}
		}
	}
	return false
}

// containsAny returns true if s contains any of the given substrings.
func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
