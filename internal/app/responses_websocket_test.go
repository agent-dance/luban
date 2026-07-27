package app

import (
	"testing"

	"github.com/agent-dance/luban/cli"
	"github.com/agent-dance/luban/provider"
)

func TestProviderRuntimeOverridesFromOptionsRequiresExplicitWebSocketOptIn(t *testing.T) {
	enabled := providerRuntimeOverridesFromOptions(cli.Options{API: " responses ", ResponsesWebSocket: true})
	if enabled.APIFormat != "responses" || enabled.ResponsesWebSocket != provider.CapabilitySupported {
		t.Fatalf("enabled runtime overrides = %#v", enabled)
	}

	omitted := providerRuntimeOverridesFromOptions(cli.Options{API: "responses"})
	if omitted.ResponsesWebSocket != provider.CapabilityUnknown {
		t.Fatalf("omitted WebSocket capability = %v, want unknown", omitted.ResponsesWebSocket)
	}
}
