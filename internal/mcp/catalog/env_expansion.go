package catalog

import (
	"os"
	"regexp"
	"strings"
)

var envVarPattern = regexp.MustCompile(`\$\{([^}]+)\}`)

type envExpansion struct {
	Expanded    string
	MissingVars []string
}

func expandEnvVarsInString(value string) envExpansion {
	missing := []string{}
	expanded := envVarPattern.ReplaceAllStringFunc(value, func(match string) string {
		parts := envVarPattern.FindStringSubmatch(match)
		if len(parts) != 2 {
			return match
		}
		varName, defaultValue, hasDefault := strings.Cut(parts[1], ":-")
		if envValue, ok := os.LookupEnv(varName); ok {
			return envValue
		}
		if hasDefault {
			return defaultValue
		}
		missing = append(missing, varName)
		return match
	})
	return envExpansion{Expanded: expanded, MissingVars: uniqueStrings(missing)}
}

// expandEnvVarsInConfig expands command/args/env and remote URL/header values.
func expandEnvVarsInConfig(config MCPServerConfig) (MCPServerConfig, []string) {
	missing := []string{}
	expand := func(value string) string {
		result := expandEnvVarsInString(value)
		missing = append(missing, result.MissingVars...)
		return result.Expanded
	}

	switch config.Type {
	case "", TransportStdio:
		config.Type = TransportStdio
		config.Command = expand(config.Command)
		for i, arg := range config.Args {
			config.Args[i] = expand(arg)
		}
		if config.Env != nil {
			for key, value := range config.Env {
				config.Env[key] = expand(value)
			}
		}
	case TransportSSE, TransportSSEIDE, TransportHTTP, TransportWebSocket, TransportWebSocketIDE:
		config.URL = expand(config.URL)
		if config.Headers != nil {
			for key, value := range config.Headers {
				config.Headers[key] = expand(value)
			}
		}
	}

	return config, uniqueStrings(missing)
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
