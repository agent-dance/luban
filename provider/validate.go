package provider

import (
	"fmt"

	"github.com/agent-dance/luban/i18n"
)

// ValidateParams checks that the provider supports the capabilities required by params.
// Returns an error if a critical capability mismatch is detected, preventing silent degradation.
func ValidateParams(p Provider, params Params) error {
	cp, ok := p.(CapabilityProvider)
	if !ok {
		return nil // no capability info available; allow through
	}
	caps := cp.Capabilities()

	// Thinking: fail loudly rather than silently dropping chain-of-thought content
	if params.Thinking != nil && params.Thinking.Enabled && !caps.Thinking {
		return fmt.Errorf("%s", i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyProviderThinkingUnsupported, p.Name(), p.ModelID()))
	}

	return nil
}
