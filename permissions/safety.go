package permissions

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

// protectedPaths lists sensitive paths that should not be written to.
// This is the single source of truth for both file-tool safety checks and
// bash redirect checks (tools/dangerous.go imports this list via GetProtectedPaths).
//
// Entries ending with "/" are treated as directory prefixes (HasPrefix match).
// Other entries without "/" are matched against the basename (exact match).
// Entries with "/" but no trailing slash are exact relative path matches.
//
// C3 fix: unexported to prevent external code from clearing the protection list.
// Use GetProtectedPaths() to obtain a read-only copy.
var protectedPaths = []string{
	".git/",
	".luban-code/",
	".deepseek-code/",
	".claude/",
	".env",
	".env.local",
	".env.development",
	".env.production",
	".env.staging",
	".env.test",
	".bashrc",
	".zshrc",
	".profile",
	".bash_profile",
	".ssh/",
	".gnupg/",
	".aws/",
	".kube/config",
}

// GetProtectedPaths returns a copy of the protected paths list.
// The returned slice is a copy — callers cannot modify the canonical list.
func GetProtectedPaths() []string {
	cp := make([]string, len(protectedPaths))
	copy(cp, protectedPaths)
	return cp
}

// SafetyConfig holds injected dependencies for safety checks.
// This avoids circular imports between permissions and tools packages.
type SafetyConfig struct {
	// ShellPolicyAnalyzer is the authoritative shell policy implementation.
	// Block is consumed by SafetyCheck and RequiredAsk by
	// MandatoryApprovalCheck; normal Allow still proceeds through rules/mode.
	ShellPolicyAnalyzer func(command string, context types.PolicyContext) types.PolicyDecision

	// Deprecated compatibility adapters. New runtimes must install only
	// ShellPolicyAnalyzer so all layers consume the same decision model.
	// DangerousCommandChecker checks if a bash command is dangerous.
	// Returns a warning description if dangerous, empty string if safe.
	DangerousCommandChecker func(command string) string

	// BashProtectedPathChecker checks if a bash command writes to a protected path.
	// Returns (true, path) if a write to a protected path is detected.
	BashProtectedPathChecker func(command string) (bool, string)

	// BashNeedsApprovalChecker identifies sandboxed/read-only looking commands
	// that still require an interactive approval step due to structural risks
	// like cd+git, process substitution, or bare-repo git execution.
	// Returns (true, reason) when permission prompting must not be skipped.
	BashNeedsApprovalChecker func(command string) (bool, string)
}

var (
	safetyConfig SafetyConfig
	safetyMu     sync.RWMutex
)

// SetSafetyConfig sets the global safety configuration with injected dependencies.
func SetSafetyConfig(cfg SafetyConfig) {
	safetyMu.Lock()
	defer safetyMu.Unlock()
	safetyConfig = cfg
}

// getSafetyConfig returns the current safety configuration.
func getSafetyConfig() SafetyConfig {
	safetyMu.RLock()
	defer safetyMu.RUnlock()
	return safetyConfig
}

// writeTools are tool names that perform write operations and should be subject
// to protected-path checks. The value is the input field name that holds the file path.
var writeTools = map[string]string{
	"Write":        "file_path",
	"FileWrite":    "file_path",
	"Edit":         "file_path",
	"FileEdit":     "file_path",
	"FileDelete":   "file_path",
	"FileAppend":   "file_path",
	"NotebookEdit": "notebook_path",
}

// multiPathTools have two path fields that both need checking.
var multiPathTools = map[string][2]string{
	"FileMove": {"source", "destination"},
	"FileLink": {"target", "link_path"}, // both symlink target and link location must be checked
}

// readTools are tool names that perform read-only operations.
// These are explicitly allowed even on protected paths.
var readTools = map[string]bool{
	"Read":     true,
	"FileRead": true,
}

// SafetyCheck performs a hard safety check on a tool invocation.
// It returns (DecisionDeny, reason) if the operation is blocked,
// or (DecisionAllow, "") if it passes the safety check.
//
// This check is bypass-immune: it runs before any permission mode
// (including ModeAllowAll) and cannot be overridden by user rules.
func SafetyCheck(toolName string, input map[string]any) (Decision, string) {
	// Read operations are always allowed, even on protected paths.
	if readTools[toolName] {
		return DecisionAllow, ""
	}

	// For write tools, check the file path against protected paths.
	if pathField, ok := writeTools[toolName]; ok {
		filePath, _ := input[pathField].(string)
		if filePath != "" && IsProtectedPath(filePath) {
			return DecisionDeny, permissionFormat(i18n.KeyPermissionSafetyProtectedPath, filePath)
		}
	}

	// For multi-path tools (FileMove, FileLink), check both path fields.
	if fields, ok := multiPathTools[toolName]; ok {
		for _, field := range fields {
			filePath, _ := input[field].(string)
			if filePath != "" && IsProtectedPath(filePath) {
				return DecisionDeny, permissionFormat(i18n.KeyPermissionSafetyProtectedPath, filePath)
			}
		}
	}

	// For Bash tool, check dangerous commands and protected path writes.
	if toolName == "Bash" {
		command, _ := input["command"].(string)
		if command == "" {
			return DecisionAllow, ""
		}

		cfg := getSafetyConfig()
		if cfg.ShellPolicyAnalyzer != nil {
			decision := cfg.ShellPolicyAnalyzer(command, defaultShellPolicyContext())
			if decision.IsBlock() {
				return DecisionDeny, renderPolicyDecision(decision)
			}
			return DecisionAllow, ""
		}

		// Fail-closed: if neither the unified analyzer nor the compatibility
		// checkers are injected, deny all Bash commands.
		// This prevents silent bypass when SetSafetyConfig is accidentally
		// skipped or called after the first Check().
		if cfg.DangerousCommandChecker == nil || cfg.BashProtectedPathChecker == nil {
			return DecisionDeny, permissionText(i18n.KeyPermissionSafetyUnavailable)
		}

		// Check for dangerous commands (rm -rf /, fork bombs, etc.)
		if warning := cfg.DangerousCommandChecker(command); warning != "" {
			return DecisionDeny, permissionFormat(i18n.KeyPermissionSafetyDangerousCommand, warning)
		}

		// Check for bash writes to protected paths (redirections)
		if hit, target := cfg.BashProtectedPathChecker(command); hit {
			return DecisionDeny, permissionFormat(i18n.KeyPermissionSafetyShellProtectedPath, target)
		}
	}

	if toolName == "PowerShell" {
		command, _ := input["command"].(string)
		if command == "" {
			return DecisionAllow, ""
		}
		if reason := dangerousPowerShellCommandReason(command); reason != "" {
			return DecisionDeny, reason
		}
	}

	return DecisionAllow, ""
}

func dangerousPowerShellCommandReason(command string) string {
	lower := strings.ToLower(command)
	for _, pattern := range highRiskPowerShellPatterns {
		if strings.Contains(lower, pattern) {
			return permissionFormat(i18n.KeyPermissionSafetyPowerShell, strings.TrimSpace(pattern))
		}
	}
	return ""
}

// AdvisoryCheck performs non-deny permission escalation checks.
// It is used for Bash-only structural cases that must still prompt even when
// sandbox auto-allow or read-only auto-allow would otherwise apply.
func AdvisoryCheck(toolName string, input map[string]any) (Decision, string) {
	if toolName != "Bash" {
		return DecisionAllow, ""
	}

	command, _ := input["command"].(string)
	if command == "" {
		return DecisionAllow, ""
	}

	cfg := getSafetyConfig()
	if cfg.ShellPolicyAnalyzer != nil {
		return DecisionAllow, ""
	}
	if cfg.BashNeedsApprovalChecker != nil {
		if needsApproval, reason := cfg.BashNeedsApprovalChecker(command); needsApproval {
			return DecisionAsk, reason
		}
	}

	return DecisionAllow, ""
}

// MandatoryApprovalCheck identifies invocation-scoped ask checks. Interactive
// modes must prompt and must not satisfy them from a cached "always allow"
// decision; an explicit automatic mode may consume PolicyRequiredAsk directly.
func MandatoryApprovalCheck(toolName string, input map[string]any) (Decision, string) {
	if toolName != "Bash" {
		return DecisionAllow, ""
	}
	command, _ := input["command"].(string)
	if strings.TrimSpace(command) == "" {
		return DecisionAllow, ""
	}
	cfg := getSafetyConfig()
	if cfg.ShellPolicyAnalyzer != nil {
		decision := cfg.ShellPolicyAnalyzer(command, defaultShellPolicyContext())
		if decision.IsRequiredAsk() {
			return DecisionAsk, renderPolicyDecision(decision)
		}
	}
	return DecisionAllow, ""
}

func defaultShellPolicyContext() types.PolicyContext {
	cwd, _ := os.Getwd()
	home, _ := os.UserHomeDir()
	known := make(map[string]string)
	if home != "" {
		known["HOME"] = home
	}
	return types.PolicyContext{
		CWD: cwd, HomeDir: home, AllowedDirs: []string{cwd},
		TrustedTempRoots: []string{os.TempDir()}, KnownEnvironment: known,
	}
}

func renderPolicyDecision(decision types.PolicyDecision) string {
	if decision.PublicKey == "" {
		return ""
	}
	return permissionFormat(decision.PublicKey, decision.PublicArgs...)
}

// IsProtectedPath checks whether a file path targets a protected location.
// It normalizes the path to a relative form and checks against protectedPaths.
// C2 fix: also resolves to absolute path when possible to prevent traversal bypass.
// Exported so that tools package can use the same logic (avoiding duplicate implementations).
func IsProtectedPath(path string) bool {
	// First pass: try to resolve to absolute path for reliable matching.
	// This handles cases like "/safe/dir/../../../.git/HEAD" → "../../.git/HEAD".
	absPath, err := filepath.Abs(path)
	if err == nil {
		// Check if any protected path component exists in the absolute path.
		if checkPathAgainstProtected(absPath) {
			return true
		}
	}

	// Second pass: normalize to relative form (original behavior, enhanced).
	cleaned := filepath.ToSlash(filepath.Clean(path))
	rel := strings.TrimPrefix(cleaned, "/")

	return checkRelPathAgainstProtected(rel, cleaned)
}

// checkPathAgainstProtected checks an absolute path against protected patterns.
func checkPathAgainstProtected(absPath string) bool {
	// Normalize: if we can determine CWD, compute relative path for matching.
	cwd, err := os.Getwd()
	if err != nil {
		// Fall back to absolute path component matching.
		return checkAbsComponents(absPath)
	}
	rel, err := filepath.Rel(cwd, absPath)
	if err != nil {
		return checkAbsComponents(absPath)
	}
	return checkRelPathAgainstProtected(rel, absPath)
}

// checkAbsComponents checks if any component of an absolute path matches a protected entry.
func checkAbsComponents(absPath string) bool {
	absPath = filepath.ToSlash(absPath)
	for _, pp := range protectedPaths {
		if strings.HasSuffix(pp, "/") {
			dir := strings.TrimSuffix(pp, "/")
			needle := "/" + pp
			if strings.Contains(absPath+"/", needle) || strings.HasSuffix(absPath, "/"+dir) {
				return true
			}
		} else if !strings.Contains(pp, "/") {
			base := filepath.Base(absPath)
			if base == pp {
				return true
			}
		} else {
			// Exact relative-path entry like ".kube/config"
			if strings.HasSuffix(absPath, "/"+pp) {
				return true
			}
		}
	}
	return false
}

// checkRelPathAgainstProtected is the core matching logic against protectedPaths.
func checkRelPathAgainstProtected(rel, cleaned string) bool {
	rel = filepath.ToSlash(rel)
	cleaned = filepath.ToSlash(cleaned)
	for _, pp := range protectedPaths {
		if strings.HasSuffix(pp, "/") {
			// Directory prefix match: ".git/" matches ".git/HEAD", "project/.git/config", etc.
			dir := strings.TrimSuffix(pp, "/")
			// Check if the relative path starts with the directory
			if strings.HasPrefix(rel, pp) || rel == dir {
				return true
			}
			// Also check if the protected dir appears as a component deeper in the path
			// e.g., "project/.git/objects" should match ".git/"
			needle := "/" + pp
			if strings.Contains("/"+rel, needle) {
				return true
			}
		} else if strings.Contains(pp, "/") {
			// Path with slash but no trailing slash: exact relative path match.
			// e.g., ".kube/config"
			if rel == pp {
				return true
			}
			// Also match deeper paths: "project/.kube/config"
			if strings.HasSuffix(rel, "/"+pp) {
				return true
			}
		} else {
			// Basename exact match: ".bashrc" matches ".bashrc", "foo/.bashrc", "/home/user/.bashrc"
			base := filepath.Base(cleaned)
			if base == pp {
				return true
			}
		}
	}
	return false
}
