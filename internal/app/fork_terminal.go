package app

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/agent-dance/luban/cli"
	"github.com/agent-dance/luban/i18n"
)

var errExecutableNotFound = errors.New("executable not found")

type terminalLaunchSpec struct {
	Command string
	Args    []string
	Dir     string
}

func forkedSessionArgs(opts cli.Options, providerName, modelID, sessionID string) []string {
	args := []string{"--session-id", sessionID}
	appendValue := func(flag, value string) {
		if strings.TrimSpace(value) != "" {
			args = append(args, flag, value)
		}
	}
	appendValue("--provider", providerName)
	appendValue("--model", modelID)
	appendValue("--api", opts.API)
	appendValue("--reasoning-effort", opts.ReasoningEffort)
	appendValue("--service-tier", opts.ServiceTier)
	if opts.ResponsesWebSocket {
		args = append(args, "--responses-websocket")
	}
	if opts.PinnedModel {
		args = append(args, "--pinned-model")
	}
	if opts.MaxTurns > 0 {
		args = append(args, "--max-turns", strconv.Itoa(opts.MaxTurns))
	}
	appendValue("--system-prompt", opts.SystemPrompt)
	for _, dir := range opts.AllowedDirs {
		appendValue("--allowed-dir", dir)
	}
	if opts.AllowAll {
		args = append(args, "--allow-all")
	}
	if opts.Sandbox {
		args = append(args, "--sandbox")
	}
	if opts.ForceSandboxTools {
		args = append(args, "--force-sandbox-tools")
	}
	appendValue("--allowed-tools", opts.AllowedTools)
	appendValue("--disallowed-tools", opts.DisallowedTools)
	if opts.ScreenReader {
		args = append(args, "--screen-reader")
	}
	appendValue("--language", opts.Language)
	appendValue("--output-style", opts.OutputStyle)
	appendValue("--allowed-domains", opts.AllowedDomains)
	appendValue("--disallowed-domains", opts.DisallowedDomains)
	appendValue("--debug-file", forkDebugFilePath(opts.DebugFile, sessionID))
	if opts.NoColor {
		args = append(args, "--no-color")
	}
	if opts.Verbose {
		args = append(args, "--verbose")
	}
	appendValue("--agents", opts.Agents)
	return args
}

func forkDebugFilePath(path, sessionID string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	safeSessionID := strings.Map(func(char rune) rune {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '-' || char == '_' {
			return char
		}
		return '_'
	}, strings.TrimSpace(sessionID))
	if safeSessionID == "" {
		safeSessionID = "session"
	}
	extension := filepath.Ext(path)
	base := strings.TrimSuffix(path, extension)
	return base + ".fork-" + safeSessionID + extension
}

func openForkSessionTerminal(ctx context.Context, opts cli.Options, providerName, modelID, sessionID, cwd string) error {
	executable, err := os.Executable()
	if err != nil {
		return rootRuntimeWrap(i18n.KeyRootForkExecutableResolve, err)
	}
	preferred := strings.TrimSpace(os.Getenv("TERM_PROGRAM"))
	if preferred == "" {
		preferred = strings.TrimSpace(os.Getenv("TERMINAL"))
	}
	spec, err := buildTerminalLaunchSpec(runtime.GOOS, preferred, os.Getenv("TMUX"), cwd, executable, forkedSessionArgs(opts, providerName, modelID, sessionID), exec.LookPath)
	if err != nil {
		return err
	}
	command := exec.CommandContext(ctx, spec.Command, spec.Args...)
	if spec.Dir != "" {
		command.Dir = spec.Dir
	}
	if output, err := command.CombinedOutput(); err != nil {
		detail := strings.TrimSpace(string(output))
		if detail != "" {
			return rootRuntimeWrapWithRawDetail(i18n.KeyRootForkProcessLaunch, err, detail, filepath.Base(spec.Command))
		}
		return rootRuntimeWrap(i18n.KeyRootForkProcessLaunch, err, filepath.Base(spec.Command))
	}
	return nil
}

func buildTerminalLaunchSpec(goos, termProgram, tmux, cwd, executable string, args []string, lookPath func(string) (string, error)) (terminalLaunchSpec, error) {
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	if strings.TrimSpace(tmux) != "" {
		if path, err := lookPath("tmux"); err == nil {
			return terminalLaunchSpec{Command: path, Args: append([]string{"new-window", "-c", cwd, executable}, args...)}, nil
		}
	}

	switch goos {
	case "darwin":
		path, err := lookPath("osascript")
		if err != nil {
			return terminalLaunchSpec{}, rootRuntimeWrap(i18n.KeyRootForkOSAScriptFind, err)
		}
		script := terminalAppleScript
		if normalizedTerminalName(termProgram) == "iterm" {
			script = iTermAppleScript
		}
		launchArgs := []string{"-e", script, "--", cwd, executable}
		launchArgs = append(launchArgs, args...)
		return terminalLaunchSpec{Command: path, Args: launchArgs}, nil
	case "linux":
		for _, candidate := range linuxTerminalCandidates(termProgram) {
			launchName := candidate
			if normalizedTerminalName(candidate) == "kitty" {
				if kittenPath, err := lookPath("kitten"); err == nil {
					if spec, ok := linuxTerminalLaunchSpec("kitten", kittenPath, cwd, executable, args); ok {
						return spec, nil
					}
				}
			}
			path, err := lookPath(candidate)
			if err != nil {
				continue
			}
			if spec, ok := linuxTerminalLaunchSpec(launchName, path, cwd, executable, args); ok {
				return spec, nil
			}
		}
		return terminalLaunchSpec{}, rootRuntimeErrorWithCause(i18n.KeyRootForkSupportedTerminalAbsent, errExecutableNotFound)
	case "windows":
		path, err := lookPath("wt.exe")
		if err != nil {
			return terminalLaunchSpec{}, rootRuntimeWrap(i18n.KeyRootForkWindowsTerminalRequired, err)
		}
		launchArgs := []string{"-w", "0", "new-tab"}
		if cwd != "" {
			launchArgs = append(launchArgs, "-d", escapeWindowsTerminalArg(cwd))
		}
		launchArgs = append(launchArgs, "--", escapeWindowsTerminalArg(executable))
		for _, arg := range args {
			launchArgs = append(launchArgs, escapeWindowsTerminalArg(arg))
		}
		return terminalLaunchSpec{Command: path, Args: launchArgs}, nil
	default:
		return terminalLaunchSpec{}, rootRuntimeError(i18n.KeyRootForkTerminalUnsupportedOS, goos)
	}
}

// Windows Terminal parses semicolons as separators even when it is started
// directly rather than through a shell. Its command-line contract uses \; for
// a literal semicolon inside the child command line.
func escapeWindowsTerminalArg(value string) string {
	return strings.ReplaceAll(value, ";", `\;`)
}

func normalizedTerminalName(value string) string {
	name := strings.ToLower(strings.TrimSpace(filepath.Base(value)))
	name = strings.TrimSuffix(name, ".app")
	switch name {
	case "iterm2", "iterm.app":
		return "iterm"
	case "apple_terminal", "terminal.app":
		return "terminal"
	default:
		return name
	}
}

func linuxTerminalCandidates(preferred string) []string {
	seen := make(map[string]struct{})
	var candidates []string
	add := func(name string) {
		name = strings.TrimSpace(filepath.Base(name))
		if name == "" {
			return
		}
		if _, exists := seen[name]; exists {
			return
		}
		seen[name] = struct{}{}
		candidates = append(candidates, name)
	}
	add(preferred)
	for _, name := range []string{"gnome-terminal", "konsole", "kitty", "wezterm", "xfce4-terminal", "mate-terminal"} {
		add(name)
	}
	return candidates
}

func linuxTerminalLaunchSpec(name, path, cwd, executable string, args []string) (terminalLaunchSpec, bool) {
	name = normalizedTerminalName(name)
	var launchArgs []string
	switch name {
	case "gnome-terminal":
		launchArgs = []string{"--tab"}
		if cwd != "" {
			launchArgs = append(launchArgs, "--working-directory="+cwd)
		}
		launchArgs = append(launchArgs, "--", executable)
	case "konsole":
		launchArgs = []string{"--new-tab"}
		if cwd != "" {
			launchArgs = append(launchArgs, "--workdir", cwd)
		}
		launchArgs = append(launchArgs, "-e", executable)
	case "kitty", "kitten":
		launchArgs = []string{"@", "launch", "--type=tab"}
		if cwd != "" {
			launchArgs = append(launchArgs, "--cwd", cwd)
		}
		launchArgs = append(launchArgs, executable)
	case "wezterm":
		launchArgs = []string{"cli", "spawn"}
		if cwd != "" {
			launchArgs = append(launchArgs, "--cwd", cwd)
		}
		launchArgs = append(launchArgs, "--", executable)
	case "xfce4-terminal", "mate-terminal":
		launchArgs = []string{"--tab"}
		if cwd != "" {
			launchArgs = append(launchArgs, "--working-directory="+cwd)
		}
		launchArgs = append(launchArgs, "-x", executable)
	default:
		return terminalLaunchSpec{}, false
	}
	launchArgs = append(launchArgs, args...)
	return terminalLaunchSpec{Command: path, Args: launchArgs}, true
}

const terminalAppleScript = `on run argv
set workingDirectory to item 1 of argv
set executablePath to item 2 of argv
set commandText to ""
if workingDirectory is not "" then set commandText to "cd " & quoted form of workingDirectory & " && "
set commandText to commandText & "exec " & quoted form of executablePath
repeat with argumentIndex from 3 to count of argv
  set commandText to commandText & " " & quoted form of (item argumentIndex of argv)
end repeat
tell application "Terminal"
  activate
  if (count of windows) is 0 then
    do script commandText
  else
    tell application "System Events" to keystroke "t" using command down
    delay 0.1
    do script commandText in selected tab of front window
  end if
end tell
end run`

const iTermAppleScript = `on run argv
set workingDirectory to item 1 of argv
set executablePath to item 2 of argv
set commandText to ""
if workingDirectory is not "" then set commandText to "cd " & quoted form of workingDirectory & " && "
set commandText to commandText & "exec " & quoted form of executablePath
repeat with argumentIndex from 3 to count of argv
  set commandText to commandText & " " & quoted form of (item argumentIndex of argv)
end repeat
tell application "iTerm"
  activate
  if (count of windows) is 0 then
    set targetWindow to (create window with default profile)
    tell current session of targetWindow to write text commandText
  else
    tell current window
      set targetTab to (create tab with default profile)
      tell current session of targetTab to write text commandText
    end tell
  end if
end tell
end run`
