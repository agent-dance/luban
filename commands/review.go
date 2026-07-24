package commands

import (
	"os"
	"os/exec"
	"strings"

	"github.com/agent-dance/luban/i18n"
)

// ---------------------------------------------------------------------------
// /review  (/rv)
// ---------------------------------------------------------------------------

type reviewCmd struct{}

func (c *reviewCmd) Name() string      { return "review" }
func (c *reviewCmd) Aliases() []string { return []string{"rv"} }
func (c *reviewCmd) Description() string {
	return builtinCommandDescription("review")
}

func (c *reviewCmd) Execute(ctx *Context, args string) error {
	cwd := ctx.CWD
	if cwd == "" {
		cwd, _ = os.Getwd()
	}

	gitPath, err := exec.LookPath("git")
	if err != nil {
		ctx.OnEvent(i18n.Text(ctx.Language, i18n.KeyCommandReviewGitMissing))
		reportCommandFailed(ctx)
		return nil
	}

	if !isGitRepo(gitPath, cwd) {
		ctx.OnEvent(i18n.Text(ctx.Language, i18n.KeyCommandReviewNotRepository))
		reportCommandFailed(ctx)
		return nil
	}

	staged := strings.Contains(args, "--staged") || strings.Contains(args, "--cached")

	var diffOut, statOut string
	if staged {
		statOut = runGit(gitPath, cwd, "diff", "--cached", "--stat")
		diffOut = runGit(gitPath, cwd, "diff", "--cached")
	} else {
		statOut = runGit(gitPath, cwd, "diff", "HEAD", "--stat")
		diffOut = runGit(gitPath, cwd, "diff", "HEAD")
		// Fall back to staged-only when there are no commits yet.
		if strings.TrimSpace(statOut) == "" && strings.TrimSpace(diffOut) == "" {
			statOut = runGit(gitPath, cwd, "diff", "--cached", "--stat")
			diffOut = runGit(gitPath, cwd, "diff", "--cached")
		}
	}

	if strings.TrimSpace(diffOut) == "" && strings.TrimSpace(statOut) == "" {
		if staged {
			ctx.OnEvent(i18n.Text(ctx.Language, i18n.KeyCommandReviewNoStaged))
		} else {
			ctx.OnEvent(i18n.Text(ctx.Language, i18n.KeyCommandReviewClean))
		}
		reportCommandSucceeded(ctx)
		return nil
	}

	var sb strings.Builder
	label := i18n.Text(ctx.Language, i18n.KeyCommandReviewAllChanges)
	if staged {
		label = i18n.Text(ctx.Language, i18n.KeyCommandReviewStagedChanges)
	}
	sb.WriteString("\n")
	sb.WriteString(bold(label + ":\n"))

	summaryLine := buildSummaryLine(statOut)
	if summaryLine != "" {
		sb.WriteString(bold(i18n.Text(ctx.Language, i18n.KeyCommandDiffSummary)))
		sb.WriteString(summaryLine)
		sb.WriteString("\n")
	}

	if strings.TrimSpace(diffOut) != "" {
		sb.WriteString("\n")
		sb.WriteString(coloriseDiff(diffOut))
	}

	sb.WriteString("\n")
	ctx.OnEvent(sb.String())
	reportCommandSucceeded(ctx)
	return nil
}
