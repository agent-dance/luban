package transport

import (
	"os"
	"sort"
	"strings"
)

var githubActionsSubprocessScrub = []string{
	"ANTHROPIC_API_KEY",
	"LUBAN_CODE_OAUTH_TOKEN",
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

// subprocessEnv returns a copy of the current environment for child
// subprocesses. When LUBAN_CODE_SUBPROCESS_ENV_SCRUB is truthy, sensitive
// GitHub Actions credentials are stripped from child processes.
func subprocessEnv() map[string]string {
	env := environToMap(os.Environ())
	if !subprocessEnvTruthy(os.Getenv("LUBAN_CODE_SUBPROCESS_ENV_SCRUB")) {
		return env
	}
	for _, key := range githubActionsSubprocessScrub {
		delete(env, key)
		delete(env, "INPUT_"+key)
	}
	return env
}

// buildSubprocessEnv returns a deterministic KEY=VALUE slice suitable for
// exec.Cmd.Env, applying server-specific overrides after subprocessEnv.
func buildSubprocessEnv(overrides map[string]string) []string {
	env := subprocessEnv()
	for key, value := range overrides {
		env[key] = value
	}
	return envMapToList(env)
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
