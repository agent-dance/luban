package prompt

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// EnvironmentContextBuilder constructs the session-specific environment block.
type EnvironmentContextBuilder struct {
	PrimaryCWD       string
	AdditionalDirs   []string
	Shell            string
	Platform         string
	OSVersion        string
	ModelID          string
	ModelDescription string
	KnowledgeCutoff  string
}

// Build returns the model-visible environment context for the current session.
func (b EnvironmentContextBuilder) Build() string {
	cwd := strings.TrimSpace(b.PrimaryCWD)
	if cwd == "" {
		if current, err := os.Getwd(); err == nil {
			cwd = current
		} else {
			cwd = "."
		}
	}

	platform := strings.TrimSpace(b.Platform)
	if platform == "" {
		platform = runtime.GOOS
	}
	shell := strings.TrimSpace(b.Shell)
	if shell == "" {
		shell = os.Getenv("SHELL")
		if shell == "" && runtime.GOOS == "windows" {
			shell = os.Getenv("ComSpec")
		}
	}
	osVersion := strings.TrimSpace(b.OSVersion)
	if osVersion == "" {
		osVersion = detectOSVersion()
	}

	var lines []string
	lines = append(lines, "You have been invoked in the following environment:")
	lines = append(lines, fmt.Sprintf(" - Primary working directory: %s", cwd))
	if len(b.AdditionalDirs) > 0 {
		lines = append(lines, " - Additional working directories:")
		for _, dir := range b.AdditionalDirs {
			if trimmed := strings.TrimSpace(dir); trimmed != "" && trimmed != cwd {
				lines = append(lines, "  - "+trimmed)
			}
		}
	}
	lines = append(lines, fmt.Sprintf(" - Platform: %s", platform))
	if shell != "" {
		lines = append(lines, fmt.Sprintf(" - Shell: %s", shell))
	}
	if osVersion != "" {
		lines = append(lines, fmt.Sprintf(" - OS version: %s", osVersion))
	}
	if b.ModelID != "" && b.ModelDescription != "" {
		lines = append(lines, fmt.Sprintf(" - You are powered by the model named %s. The exact model ID is %s.", strings.TrimSpace(b.ModelDescription), strings.TrimSpace(b.ModelID)))
	} else if b.ModelID != "" {
		lines = append(lines, fmt.Sprintf(" - You are powered by the model with exact model ID %s.", strings.TrimSpace(b.ModelID)))
	} else if b.ModelDescription != "" {
		lines = append(lines, fmt.Sprintf(" - You are powered by the model named %s.", strings.TrimSpace(b.ModelDescription)))
	}
	if cutoff := strings.TrimSpace(b.KnowledgeCutoff); cutoff != "" {
		lines = append(lines, fmt.Sprintf(" - Assistant knowledge cutoff is %s.", cutoff))
	}

	return strings.Join(lines, "\n")
}

func detectOSVersion() string {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.CommandContext(ctx, "sw_vers", "-productVersion")
	case "windows":
		cmd = exec.CommandContext(ctx, "cmd", "/c", "ver")
	default:
		cmd = exec.CommandContext(ctx, "uname", "-sr")
	}
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return ""
	}
	return strings.TrimSpace(out.String())
}
