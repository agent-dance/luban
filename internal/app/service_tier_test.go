package app

import (
	"testing"

	"github.com/agent-dance/luban/cli"
	"github.com/agent-dance/luban/provider"
)

func TestServiceTierFromOptionsDoesNotCanonicalize(t *testing.T) {
	for _, value := range []string{"", "default", "auto", " DEFAULT "} {
		if got := serviceTierFromOptions(cli.Options{ServiceTier: value}); got != provider.ServiceTier(value) {
			t.Fatalf("serviceTierFromOptions(%q) = %q, want exact value", value, got)
		}
	}
}
