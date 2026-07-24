package commands

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/agent-dance/luban/i18n"
)

// ---------------------------------------------------------------------------
// /diff  (/d)
// ---------------------------------------------------------------------------

type diffCmd struct{}

func (c *diffCmd) Name() string      { return "diff" }
func (c *diffCmd) Aliases() []string { return []string{"d"} }
func (c *diffCmd) Description() string {
	return builtinCommandDescription("diff")
}

func (c *diffCmd) Execute(ctx *Context, _ string) error {
	cwd := ctx.CWD
	if cwd == "" {
		cwd, _ = os.Getwd()
	}

	// Verify git is available.
	gitPath, err := exec.LookPath("git")
	if err != nil {
		ctx.OnEvent(i18n.Text(ctx.Language, i18n.KeyCommandDiffGitMissing))
		return nil
	}

	// Verify we are inside a git repository.
	if !isGitRepo(gitPath, cwd) {
		ctx.OnEvent(i18n.Text(ctx.Language, i18n.KeyCommandDiffNotRepository))
		return nil
	}

	// -----------------------------------------------------------------------
	// 1. Collect summary stats via --stat (staged + unstaged).
	// -----------------------------------------------------------------------
	statOut := runGit(gitPath, cwd, "diff", "HEAD", "--stat")
	cachedStatOut := ""
	// If HEAD doesn't exist yet (initial commit, no commits at all) fall back
	// to showing the index against the empty tree.
	if statOut == "" {
		// Check if there are staged files at all.
		cachedStatOut = runGit(gitPath, cwd, "diff", "--cached", "--stat")
	}

	// -----------------------------------------------------------------------
	// 2. Untracked files (not yet `git add`-ed).
	// -----------------------------------------------------------------------
	untrackedFiles := untrackedList(gitPath, cwd)

	// -----------------------------------------------------------------------
	// 3. Full diff output (staged + unstaged against HEAD).
	// -----------------------------------------------------------------------
	diffOut := runGit(gitPath, cwd, "diff", "HEAD")
	if diffOut == "" {
		// No HEAD yet — show staged diff against empty tree.
		diffOut = runGit(gitPath, cwd, "diff", "--cached")
	}

	// -----------------------------------------------------------------------
	// 4. Determine if anything changed at all.
	// -----------------------------------------------------------------------
	hasStatOutput := strings.TrimSpace(statOut) != "" || strings.TrimSpace(cachedStatOut) != ""
	hasDiff := strings.TrimSpace(diffOut) != ""
	hasUntracked := len(untrackedFiles) > 0

	if !hasStatOutput && !hasDiff && !hasUntracked {
		ctx.OnEvent(i18n.Text(ctx.Language, i18n.KeyCommandDiffClean))
		return nil
	}

	var sb strings.Builder

	// -----------------------------------------------------------------------
	// 5. Summary header.
	// -----------------------------------------------------------------------
	sb.WriteString("\n")
	summaryLine := buildSummaryLine(statOut + cachedStatOut)
	if summaryLine != "" {
		sb.WriteString(bold(i18n.Text(ctx.Language, i18n.KeyCommandDiffSummary)))
		sb.WriteString(summaryLine)
		sb.WriteString("\n")
	}

	// Untracked files section.
	if hasUntracked {
		sb.WriteString(bold(i18n.Text(ctx.Language, i18n.KeyCommandDiffUntracked)))
		for _, f := range untrackedFiles {
			sb.WriteString("  ")
			sb.WriteString(colorize(colorGreen, f))
			sb.WriteString("\n")
		}
	}

	if hasDiff {
		sb.WriteString("\n")
		sb.WriteString(coloriseDiff(diffOut))
	}

	sb.WriteString("\n")
	ctx.OnEvent(sb.String())
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// runGit executes a git command under dir and returns its combined output as a
// trimmed string. Errors (e.g. non-zero exit) produce an empty string so
// callers can treat absence of output uniformly.
func runGit(gitPath, dir string, args ...string) string {
	ctxT, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctxT, gitPath, append([]string{"-C", dir}, args...)...)
	out, _ := cmd.Output()
	return string(out)
}

// isGitRepo returns true when dir is inside a git work-tree.
func isGitRepo(gitPath, dir string) bool {
	ctxT, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctxT, gitPath, "-C", dir, "rev-parse", "--is-inside-work-tree").Output()
	return err == nil && strings.TrimSpace(string(out)) == "true"
}

// untrackedList returns the list of untracked files via `git status --porcelain`.
func untrackedList(gitPath, dir string) []string {
	raw := runGit(gitPath, dir, "status", "--porcelain")
	var files []string
	for _, line := range strings.Split(raw, "\n") {
		if len(line) < 3 {
			continue
		}
		xy := line[:2]
		// Porcelain codes: "??" = untracked, "!!" = ignored (skip ignored).
		if xy == "??" {
			files = append(files, strings.TrimSpace(line[3:]))
		}
	}
	return files
}

// buildSummaryLine parses the last line of `git diff --stat` output which
// looks like "3 files changed, 12 insertions(+), 4 deletions(-)".
func buildSummaryLine(statOutput string) string {
	lines := strings.Split(strings.TrimSpace(statOutput), "\n")
	// The summary is always the last non-empty line.
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" && strings.Contains(line, "changed") {
			return colorizeSummary(line)
		}
	}
	return ""
}

// colorizeSummary adds colour to the key numbers in a git stat summary line.
func colorizeSummary(line string) string {
	// Colour "insertions" segments green and "deletions" red.
	parts := strings.Split(line, ", ")
	for i, part := range parts {
		switch {
		case strings.Contains(part, "insertion"):
			parts[i] = colorize(colorGreen, part)
		case strings.Contains(part, "deletion"):
			parts[i] = colorize(colorRed, part)
		}
	}
	return strings.Join(parts, ", ")
}

// coloriseDiff applies ANSI colours to a unified diff string line-by-line.
// Added lines become green, removed lines red, headers bold/cyan, and binary
// file notices are highlighted yellow.
func coloriseDiff(diff string) string {
	if !isTTY() {
		return diff
	}
	var sb strings.Builder
	for _, line := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---"):
			sb.WriteString(bold(line))
		case strings.HasPrefix(line, "diff ") || strings.HasPrefix(line, "index "):
			sb.WriteString(colorize(colorCyan, line))
		case strings.HasPrefix(line, "@@"):
			sb.WriteString(colorize(colorCyan, line))
		case strings.HasPrefix(line, "+"):
			sb.WriteString(colorize(colorGreen, line))
		case strings.HasPrefix(line, "-"):
			sb.WriteString(colorize(colorRed, line))
		case strings.Contains(line, "Binary files"):
			sb.WriteString(colorize(colorYellow, line))
		default:
			sb.WriteString(line)
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// ---------------------------------------------------------------------------
// ANSI colour helpers
// ---------------------------------------------------------------------------

const (
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
	colorBold   = "\033[1m"
	colorReset  = "\033[0m"
)

func colorize(code, s string) string {
	if !isTTY() {
		return s
	}
	return code + s + colorReset
}

func bold(s string) string {
	if !isTTY() {
		return s
	}
	return colorBold + s + colorReset
}

// isTTY reports whether stdout looks like an interactive terminal.
// We check fd 1 (stdout) via the environment variable shortcut first so that
// tests (which redirect stdout) get plain output without colours.
func isTTY() bool {
	// Honour explicit NO_COLOR / TERM=dumb conventions.
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	// FORCE_COLOR allows tests to exercise colour paths.
	if v := os.Getenv("FORCE_COLOR"); v != "" {
		b, err := strconv.ParseBool(v)
		if err == nil {
			return b
		}
	}
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// Ensure strconv is used (imported for FORCE_COLOR parsing).
var _ = strconv.ParseBool
