package app

import (
	"github.com/agent-dance/luban/cli"
	"github.com/agent-dance/luban/provider"
)

// serviceTierFromOptions preserves the CLI's validated value byte-for-byte.
// An omitted tier remains omitted and is never silently converted to default
// after command-line validation.
func serviceTierFromOptions(opts cli.Options) provider.ServiceTier {
	return provider.ServiceTier(opts.ServiceTier)
}
