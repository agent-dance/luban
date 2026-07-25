package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/provider"
)

// ---------------------------------------------------------------------------
// /doctor — multi-provider aware diagnostics (Phase 6)
// ---------------------------------------------------------------------------

type doctorCmd struct{}

func (c *doctorCmd) Name() string        { return "doctor" }
func (c *doctorCmd) Aliases() []string   { return []string{"diagnose"} }
func (c *doctorCmd) Description() string { return builtinCommandDescription("doctor") }

// checkResult holds the outcome of a single diagnostic check.
type checkResult struct {
	ok      bool
	label   string
	message string
}

func (r checkResult) format(lang i18n.Language) string {
	mark := "✓"
	if !r.ok {
		mark = "✗"
	}
	return i18n.Format(lang, i18n.KeyDoctorResult, mark, r.label, r.message)
}

// healthCheck describes a single diagnostic check with optional provider scoping.
type healthCheck struct {
	// Label is the display name of the check (e.g. "API Key", "Git").
	Label i18n.Key

	// Fn performs the check, receiving the command Context for access to
	// ProviderRegistry, CredentialStore, CWD, etc.
	Fn func(ctx *Context) checkResult

	// Provider scopes the check to a specific provider.
	// Empty string means the check is universal (runs for all providers).
	// A non-empty value (e.g. "ollama") means the check only runs when
	// the current provider matches.
	Provider string
}

func (c *doctorCmd) Execute(ctx *Context, _ string) error {
	cwd := ctx.CWD
	if cwd == "" {
		cwd, _ = os.Getwd()
	}

	currentProvider := ctx.CurrentProvider

	// Build the check list — universal checks + provider-specific checks.
	allChecks := []healthCheck{
		// ── Universal checks ──────────────────────────────────────────
		{Label: i18n.KeyDoctorLabelCredentials, Fn: func(c *Context) checkResult { return checkProviderCredentials(c) }},
		{Label: i18n.KeyDoctorLabelModel, Fn: func(c *Context) checkResult { return checkModelAvailable(c) }},
		{Label: i18n.KeyDoctorLabelGit, Fn: func(c *Context) checkResult { return checkGit(cwd, ctx.Language) }},
		{Label: i18n.KeyDoctorLabelSandbox, Fn: func(_ *Context) checkResult { return checkSandbox(ctx.Language) }},
		{Label: i18n.KeyDoctorLabelMCP, Fn: func(c *Context) checkResult { return checkMCPServersWithBackend(cwd, c.MCPBackend) }},
		{Label: i18n.KeyDoctorLabelNode, Fn: func(_ *Context) checkResult { return checkNode(ctx.Language) }},
		{Label: i18n.KeyDoctorLabelDisk, Fn: func(_ *Context) checkResult { return checkDiskSpace(ctx.Language) }},
		{Label: i18n.KeyDoctorLabelConfig, Fn: func(_ *Context) checkResult { return checkConfig(cwd, ctx.Language) }},

		// ── Provider-specific checks ──────────────────────────────────
		{Label: i18n.KeyDoctorLabelOllama, Provider: "ollama", Fn: func(_ *Context) checkResult { return checkOllamaServer(ctx.Language) }},
	}

	// Filter to applicable checks.
	var checks []healthCheck
	for _, ch := range allChecks {
		if ch.Provider == "" || ch.Provider == currentProvider {
			checks = append(checks, ch)
		}
	}

	results := make([]checkResult, len(checks))
	var wg sync.WaitGroup
	for i, ch := range checks {
		wg.Add(1)
		go func(idx int, hc healthCheck) {
			defer wg.Done()
			results[idx] = hc.Fn(ctx)
			results[idx].label = i18n.Text(ctx.Language, hc.Label)
		}(i, ch)
	}
	wg.Wait()

	var sb strings.Builder
	sb.WriteString("\n")
	failedChecks := 0
	for _, r := range results {
		sb.WriteString(r.format(ctx.Language))
		sb.WriteString("\n")
		if !r.ok {
			failedChecks++
		}
	}
	sb.WriteString("\n")
	ctx.OnEvent(sb.String())
	if failedChecks > 0 {
		reportCommandDomainResult(ctx, CommandOutcomeFailed, "", i18n.Format(ctx.Language, i18n.KeyDoctorResolveFailures, failedChecks))
	} else {
		reportCommandSucceeded(ctx)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Multi-provider credential and model checks (Phase 6)
// ---------------------------------------------------------------------------

// checkProviderCredentials verifies that the current provider has valid
// credentials configured via environment variables or the CredentialStore.
func checkProviderCredentials(ctx *Context) checkResult {
	r := checkResult{}
	providerName := ctx.CurrentProvider
	if providerName == "" {
		providerName = brand.DefaultProvider
	}

	displayName := providerName
	if info, ok := ctx.ProviderRegistry.Get(providerName); ok && info.DisplayName != "" {
		displayName = info.DisplayName
	}
	connection := ctx.ProviderRegistry.ConnectionState(providerName)
	r.ok = connection.CanSelectModels
	if connection.DisplayName != "" {
		displayName = connection.DisplayName
	}
	r.message = i18n.Format(ctx.Language, i18n.KeyDoctorCredentialState, displayName, connection.DetailText(ctx.Language))
	return r
}

// checkModelAvailable verifies the current model exists in the ModelCatalog.
func checkModelAvailable(ctx *Context) checkResult {
	r := checkResult{}
	model := ctx.QueryLoop.Model()
	providerName := ctx.CurrentProvider
	if providerName == "" {
		providerName = brand.DefaultProvider
	}

	if model == "" {
		r.ok = false
		r.message = i18n.Text(ctx.Language, i18n.KeyDoctorNoModel)
		return r
	}

	if info, found := ctx.ProviderRegistry.Catalog().ResolveForProvider(providerName, model); found {
		r.ok = true
		parts := []string{fmt.Sprintf("%s/%s", providerName, model)}
		if info.ContextWindow > 0 {
			parts = append(parts, i18n.Format(ctx.Language, i18n.KeyDoctorContextWindow, provider.FormatContextWindow(info.ContextWindow)))
		}
		if info.CanReason {
			parts = append(parts, i18n.Text(ctx.Language, i18n.KeyDoctorReasoning))
		}
		r.message = strings.Join(parts, ", ")
		return r
	}

	// No catalog or model not found — still report what we have.
	r.ok = true
	r.message = i18n.Format(ctx.Language, i18n.KeyDoctorCustomModel, providerName, model)
	return r
}

// checkOllamaServer verifies that the Ollama server is reachable.
func checkOllamaServer(lang i18n.Language) checkResult {
	r := checkResult{}

	baseURL := os.Getenv("OLLAMA_BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	// Strip /v1 suffix if present (used by OpenAI-compatible endpoint).
	baseURL = strings.TrimSuffix(baseURL, "/v1")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(baseURL + "/api/tags")
	if err != nil {
		r.ok = false
		r.message = i18n.Format(lang, i18n.KeyDoctorOllamaUnreachable, baseURL, err)
		return r
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		r.ok = false
		r.message = i18n.Format(lang, i18n.KeyDoctorOllamaHTTP, resp.StatusCode, baseURL)
		return r
	}

	r.ok = true
	r.message = i18n.Format(lang, i18n.KeyDoctorOllamaRunning, baseURL)
	return r
}

// checkGit verifies git is installed and cwd is a repo.
func checkGit(cwd string, lang i18n.Language) checkResult {
	r := checkResult{}

	path, err := exec.LookPath("git")
	if err != nil {
		r.ok = false
		r.message = i18n.Text(lang, i18n.KeyDoctorGitMissing)
		return r
	}

	// Get git version.
	ctxTimeout, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctxTimeout, path, "version").Output()
	version := i18n.Text(lang, i18n.KeyRuntimeDoctorUnknownVersion)
	if err == nil {
		version = strings.TrimSpace(strings.TrimPrefix(string(out), "git version "))
		// e.g. "2.44.0" — strip trailing OS suffix.
		if sp := strings.Fields(version); len(sp) > 0 {
			version = sp[0]
		}
	}

	// Check if cwd is a git repo.
	ctxRepo, cancelRepo := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelRepo()
	repoOut, repoErr := exec.CommandContext(ctxRepo, path, "-C", cwd, "rev-parse", "--is-inside-work-tree").Output()
	isRepo := repoErr == nil && strings.TrimSpace(string(repoOut)) == "true"

	r.ok = true
	if isRepo {
		r.message = i18n.Format(lang, i18n.KeyDoctorGitRepo, version)
	} else {
		r.message = i18n.Format(lang, i18n.KeyDoctorGitNotRepo, version)
	}
	return r
}

// checkSandbox verifies sandboxing tool availability.
func checkSandbox(lang i18n.Language) checkResult {
	r := checkResult{}

	switch runtime.GOOS {
	case "darwin":
		_, err := exec.LookPath("sandbox-exec")
		if err != nil {
			r.ok = false
			r.message = i18n.Format(lang, i18n.KeyDoctorSandboxMissing, "sandbox-exec")
			return r
		}
		r.ok = true
		r.message = i18n.Format(lang, i18n.KeyDoctorSandboxAvailable, "sandbox-exec")
	case "linux":
		_, err := exec.LookPath("bwrap")
		if err != nil {
			r.ok = false
			r.message = i18n.Format(lang, i18n.KeyDoctorSandboxMissing, "bwrap")
			return r
		}
		r.ok = true
		r.message = i18n.Format(lang, i18n.KeyDoctorSandboxAvailable, "bwrap")
	default:
		r.ok = true
		r.message = i18n.Format(lang, i18n.KeyDoctorSandboxUnsupported, runtime.GOOS)
	}
	return r
}

func checkMCPServersWithBackend(cwd string, backend MCPBackend) checkResult {
	return mcpDoctorCheckWithBackend(cwd, backend)
}

// checkNode verifies node.js is installed.
func checkNode(lang i18n.Language) checkResult {
	r := checkResult{}

	path, err := exec.LookPath("node")
	if err != nil {
		r.ok = false
		r.message = i18n.Text(lang, i18n.KeyDoctorNodeMissing)
		return r
	}

	ctxTimeout, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctxTimeout, path, "--version").Output()
	if err != nil {
		r.ok = true
		r.message = i18n.Text(lang, i18n.KeyDoctorNodeUnknown)
		return r
	}
	version := strings.TrimSpace(string(out))
	r.ok = true
	r.message = version
	return r
}

// checkConfig verifies settings.json and instruction files are readable.
func checkConfig(cwd string, lang i18n.Language) checkResult {
	r := checkResult{}

	home, _ := os.UserHomeDir()
	candidates := []string{
		filepath.Join(cwd, brand.ConfigDirName, "settings.json"),
		filepath.Join(home, brand.ConfigDirName, "settings.json"),
	}
	instructions := []string{
		filepath.Join(cwd, brand.InstructionsFile),
		filepath.Join(cwd, brand.AgentsFile),
		filepath.Join(cwd, brand.ConfigDirName, brand.InstructionsFile),
		filepath.Join(home, brand.ConfigDirName, brand.InstructionsFile),
	}

	var found []string
	var invalid []string

	for _, p := range candidates {
		data, err := os.ReadFile(p)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			invalid = append(invalid, i18n.Format(lang, i18n.KeyDoctorConfigUnreadable, filepath.Base(p)))
			continue
		}
		var v any
		if err := json.Unmarshal(data, &v); err != nil {
			invalid = append(invalid, i18n.Format(lang, i18n.KeyDoctorConfigInvalid, filepath.Base(p)))
			continue
		}
		found = append(found, "settings.json")
		break
	}

	for _, p := range instructions {
		if _, err := os.Stat(p); err == nil {
			found = append(found, filepath.Base(p))
			break
		}
	}

	if len(invalid) > 0 {
		r.ok = false
		r.message = strings.Join(invalid, "; ")
		return r
	}
	if len(found) == 0 {
		r.ok = true
		r.message = i18n.Text(lang, i18n.KeyDoctorConfigNone)
		return r
	}
	r.ok = true
	r.message = i18n.Format(lang, i18n.KeyDoctorConfigValid, strings.Join(found, ", "))
	return r
}
