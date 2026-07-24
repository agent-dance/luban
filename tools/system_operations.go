package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

// SystemInfoTool returns comprehensive system information
type SystemInfoTool struct{}

func (t *SystemInfoTool) Name() string {
	return "SystemInfo"
}

func (t *SystemInfoTool) Description() string {
	return "Get comprehensive system information"
}

func (t *SystemInfoTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type:       "object",
		Properties: map[string]any{},
		Required:   []string{},
	}
}

func (t *SystemInfoTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	hostname, _ := os.Hostname()
	user, _ := os.LookupEnv("USER")

	return ResponseJSON(map[string]any{
		"hostname":   hostname,
		"os":         runtime.GOOS,
		"arch":       runtime.GOARCH,
		"num_cpu":    runtime.NumCPU(),
		"go_version": runtime.Version(),
		"user":       user,
	})
}

// OsTool returns the operating system name
type OsTool struct{}

func (t *OsTool) Name() string {
	return "Os"
}

func (t *OsTool) Description() string {
	return "Get the operating system name"
}

func (t *OsTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type:       "object",
		Properties: map[string]any{},
		Required:   []string{},
	}
}

func (t *OsTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	return ResponseJSON(map[string]string{
		"os": runtime.GOOS,
	})
}

// ArchTool returns the system architecture
type ArchTool struct{}

func (t *ArchTool) Name() string {
	return "Arch"
}

func (t *ArchTool) Description() string {
	return "Get the system architecture"
}

func (t *ArchTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type:       "object",
		Properties: map[string]any{},
		Required:   []string{},
	}
}

func (t *ArchTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	return ResponseJSON(map[string]string{
		"arch": runtime.GOARCH,
	})
}

// UptimeTool returns system uptime
type UptimeTool struct{}

func (t *UptimeTool) Name() string {
	return "Uptime"
}

func (t *UptimeTool) Description() string {
	return "Get system uptime"
}

func (t *UptimeTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type:       "object",
		Properties: map[string]any{},
		Required:   []string{},
	}
}

func (t *UptimeTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	var cmd *exec.Cmd

	// Different uptime command for different OS
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "wmic", "os", "get", "lastbootuptime")
	} else {
		cmd = exec.CommandContext(ctx, "uptime")
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyCUptimeFailed, err)), nil
	}

	var uptime string
	outputStr := string(output)
	if runtime.GOOS == "windows" {
		uptime = toolRuntimeFormat(i18n.KeyToolLegacyCLastBootUptime, outputStr)
	} else {
		uptime = outputStr
	}

	return ResponseJSON(map[string]string{
		"uptime": uptime,
	})
}

// MemoryTool returns memory information
type MemoryTool struct{}

func (t *MemoryTool) Name() string {
	return "Memory"
}

func (t *MemoryTool) Description() string {
	return "Get memory usage information"
}

func (t *MemoryTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type:       "object",
		Properties: map[string]any{},
		Required:   []string{},
	}
}

func (t *MemoryTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// Convert bytes to MB
	totalMB := float64(m.TotalAlloc) / 1024 / 1024
	allocMB := float64(m.Alloc) / 1024 / 1024
	sysMemMB := float64(m.Sys) / 1024 / 1024

	return ResponseJSON(map[string]any{
		"total_alloc_mb": fmt.Sprintf("%.2f", totalMB),
		"alloc_mb":       fmt.Sprintf("%.2f", allocMB),
		"sys_memory_mb":  fmt.Sprintf("%.2f", sysMemMB),
		"num_goroutine":  runtime.NumGoroutine(),
	})
}

// ProcessListTool lists running processes
type ProcessListTool struct{}

func (t *ProcessListTool) Name() string {
	return "ProcessList"
}

func (t *ProcessListTool) Description() string {
	return "List running processes"
}

func (t *ProcessListTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"filter": map[string]any{
				"type":        "string",
				"description": "Optional filter for process names",
			},
		},
		Required: []string{},
	}
}

func (t *ProcessListTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	filter := GetStringField(input, "filter", "")

	var cmd *exec.Cmd

	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "tasklist")
	} else {
		cmd = exec.CommandContext(ctx, "ps", "aux")
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyCProcessListFailed, err)), nil
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var results []string

	for _, line := range lines {
		if filter == "" || strings.Contains(strings.ToLower(line), strings.ToLower(filter)) {
			results = append(results, line)
		}
	}

	return ResponseJSON(map[string]any{
		"processes": results,
		"count":     len(results),
	})
}

// ProcessKillTool kills a process by PID
type ProcessKillTool struct{}

func (t *ProcessKillTool) Name() string {
	return "ProcessKill"
}

func (t *ProcessKillTool) Description() string {
	return "Kill a process by PID"
}

func (t *ProcessKillTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"pid": map[string]any{
				"type":        "number",
				"description": "Process ID to kill",
			},
			"force": map[string]any{
				"type":        "boolean",
				"description": "Force kill (SIGKILL) instead of graceful (SIGTERM)",
			},
		},
		Required: []string{"pid"},
	}
}

func (t *ProcessKillTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	pid := GetIntField(input, "pid", 0)
	force := GetBoolField(input, "force", false)

	if pid <= 0 {
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyCInvalidPID, pid)), nil
	}

	var cmd *exec.Cmd

	if runtime.GOOS == "windows" {
		if force {
			cmd = exec.CommandContext(ctx, "taskkill", "/PID", fmt.Sprintf("%d", pid), "/F")
		} else {
			cmd = exec.CommandContext(ctx, "taskkill", "/PID", fmt.Sprintf("%d", pid))
		}
	} else {
		if force {
			cmd = exec.CommandContext(ctx, "kill", "-9", fmt.Sprintf("%d", pid))
		} else {
			cmd = exec.CommandContext(ctx, "kill", fmt.Sprintf("%d", pid))
		}
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyCProcessKillFailed, err)), nil
	}

	return ResponseJSON(map[string]any{
		"status": "success",
		"pid":    pid,
		"output": strings.TrimSpace(string(output)),
	})
}

// ProcessStdoutTool captures stdout from a running process.
//
// SECURITY NOTE: This tool can indirectly leak sensitive data if the captured
// process reads protected files (e.g., "cat /etc/shadow"). The safety layer
// does not inspect process output for leaked content. Future hardening should
// consider checking the command argument for reads of protected paths, similar
// to BashWritesToProtectedPath but for read-side leakage.
type ProcessStdoutTool struct{}

func (t *ProcessStdoutTool) Name() string {
	return "ProcessStdout"
}

func (t *ProcessStdoutTool) Description() string {
	return "Capture stdout from a process"
}

func (t *ProcessStdoutTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "Command to run",
			},
			"args": map[string]any{
				"type":        "string",
				"description": "Arguments (space-separated)",
			},
			"timeout_seconds": map[string]any{
				"type":        "number",
				"description": "Timeout in seconds (default: 30)",
			},
		},
		Required: []string{"command"},
	}
}

func (t *ProcessStdoutTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	command, err := MustGetStringField(input, "command")
	if err != nil {
		return ErrorResponse(err), nil
	}

	args := GetStringField(input, "args", "")
	timeout := GetIntField(input, "timeout_seconds", 30)

	// Create context with timeout
	execCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	var cmd *exec.Cmd
	if args != "" {
		argsList := strings.Fields(args)
		cmd = exec.CommandContext(execCtx, command, argsList...)
	} else {
		cmd = exec.CommandContext(execCtx, command)
	}

	output, err := cmd.Output()
	if err != nil {
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyCCommandFailed, err)), nil
	}

	return ResponseJSON(map[string]string{
		"stdout": string(output),
	})
}

// ProcessStderrTool captures stderr from a running process.
//
// SECURITY NOTE: Same indirect-leakage risk as ProcessStdoutTool — see above.
type ProcessStderrTool struct{}

func (t *ProcessStderrTool) Name() string {
	return "ProcessStderr"
}

func (t *ProcessStderrTool) Description() string {
	return "Capture stderr from a process"
}

func (t *ProcessStderrTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "Command to run",
			},
			"args": map[string]any{
				"type":        "string",
				"description": "Arguments (space-separated)",
			},
			"timeout_seconds": map[string]any{
				"type":        "number",
				"description": "Timeout in seconds (default: 30)",
			},
		},
		Required: []string{"command"},
	}
}

func (t *ProcessStderrTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	command, err := MustGetStringField(input, "command")
	if err != nil {
		return ErrorResponse(err), nil
	}

	args := GetStringField(input, "args", "")
	timeout := GetIntField(input, "timeout_seconds", 30)

	// Create context with timeout
	execCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	var cmd *exec.Cmd
	if args != "" {
		argsList := strings.Fields(args)
		cmd = exec.CommandContext(execCtx, command, argsList...)
	} else {
		cmd = exec.CommandContext(execCtx, command)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyCCommandFailed, err)), nil
	}

	return ResponseJSON(map[string]string{
		"stderr": string(output),
	})
}
