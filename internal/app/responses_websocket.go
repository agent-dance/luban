package app

import (
	"strings"

	"github.com/agent-dance/luban/cli"
	"github.com/agent-dance/luban/provider"
)

// providerRuntimeOverridesFromOptions is the single CLI-to-provider boundary
// for invocation-scoped wire choices. WebSocket remains Unknown unless the
// user explicitly opted in; no environment or model heuristic may enable it.
func providerRuntimeOverridesFromOptions(opts cli.Options) provider.RuntimeOverrides {
	webSocket := provider.CapabilityUnknown
	if opts.ResponsesWebSocket {
		webSocket = provider.CapabilitySupported
	}
	return provider.RuntimeOverrides{
		APIFormat:          strings.TrimSpace(opts.API),
		ResponsesWebSocket: webSocket,
	}
}
