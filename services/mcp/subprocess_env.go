package mcp

import (
	"os"
	"sort"
	"strings"
	"sync"
)

var githubActionsSubprocessScrub = []string{
	"ANTHROPIC_API_KEY",
	"CLAUDE_CODE_OAUTH_TOKEN",
	"ANTHROPIC_AUTH_TOKEN",
	"ANTHROPIC_FOUNDRY_API_KEY",
	"ANTHROPIC_CUSTOM_HEADERS",
	"OTEL_EXPORTER_OTLP_HEADERS",
	"OTEL_EXPORTER_OTLP_LOGS_HEADERS",
	"OTEL_EXPORTER_OTLP_METRICS_HEADERS",
	"OTEL_EXPORTER_OTLP_TRACES_HEADERS",
	"AWS_SECRET_ACCESS_KEY",
	"AWS_SESSION_TOKEN",
	"AWS_BEARER_TOKEN_BEDROCK",
	"GOOGLE_APPLICATION_CREDENTIALS",
	"AZURE_CLIENT_SECRET",
	"AZURE_CLIENT_CERTIFICATE_PATH",
	"ACTIONS_ID_TOKEN_REQUEST_TOKEN",
	"ACTIONS_ID_TOKEN_REQUEST_URL",
	"ACTIONS_RUNTIME_TOKEN",
	"ACTIONS_RUNTIME_URL",
	"ALL_INPUTS",
	"OVERRIDE_GITHUB_TOKEN",
	"DEFAULT_WORKFLOW_TOKEN",
	"SSH_SIGNING_KEY",
}

var subprocessProxyEnv struct {
	sync.RWMutex
	fn func() map[string]string
}

// RegisterSubprocessProxyEnvFunc registers an optional proxy env provider,
// mirroring the TypeScript subprocessEnv hook used by CCR upstream proxy.
func RegisterSubprocessProxyEnvFunc(fn func() map[string]string) func() {
	subprocessProxyEnv.Lock()
	previous := subprocessProxyEnv.fn
	subprocessProxyEnv.fn = fn
	subprocessProxyEnv.Unlock()
	return func() {
		subprocessProxyEnv.Lock()
		subprocessProxyEnv.fn = previous
		subprocessProxyEnv.Unlock()
	}
}

// SubprocessEnv returns a copy of the current environment for child
// subprocesses. When CLAUDE_CODE_SUBPROCESS_ENV_SCRUB is truthy, sensitive
// GitHub Actions credentials are stripped from child processes.
func SubprocessEnv() map[string]string {
	env := environToMap(os.Environ())
	for key, value := range currentSubprocessProxyEnv() {
		env[key] = value
	}
	if !subprocessEnvTruthy(os.Getenv("CLAUDE_CODE_SUBPROCESS_ENV_SCRUB")) {
		return env
	}
	for _, key := range githubActionsSubprocessScrub {
		delete(env, key)
		delete(env, "INPUT_"+key)
	}
	return env
}

// BuildSubprocessEnv returns a deterministic KEY=VALUE slice suitable for
// exec.Cmd.Env, applying server-specific overrides after SubprocessEnv.
func BuildSubprocessEnv(overrides map[string]string) []string {
	env := SubprocessEnv()
	for key, value := range overrides {
		env[key] = value
	}
	return envMapToList(env)
}

func currentSubprocessProxyEnv() map[string]string {
	subprocessProxyEnv.RLock()
	fn := subprocessProxyEnv.fn
	subprocessProxyEnv.RUnlock()
	if fn == nil {
		return nil
	}
	return fn()
}

func environToMap(values []string) map[string]string {
	env := make(map[string]string, len(values))
	for _, value := range values {
		key, val, ok := strings.Cut(value, "=")
		if !ok || key == "" {
			continue
		}
		env[key] = val
	}
	return env
}

func envMapToList(env map[string]string) []string {
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, key+"="+env[key])
	}
	return out
}

func subprocessEnvTruthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
