package provider

import (
	"fmt"
	"strings"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

// ValidateParams checks that the provider supports the capabilities required by params.
// Returns an error if a critical capability mismatch is detected, preventing silent degradation.
func ValidateParams(p Provider, params Params) error {
	if params.ServiceTier != "" && params.ServiceTier != ServiceTierDefault {
		return fmt.Errorf("%s", i18n.Format(
			i18n.DetectOrLoadLanguage(),
			i18n.KeyProviderServiceTierInvalid,
			params.ServiceTier,
		))
	}
	customRequested, err := validateToolDefinitionContracts(params.Tools)
	if err != nil {
		return err
	}
	cp, ok := p.(CapabilityProvider)
	if !ok {
		if params.ServiceTier != "" {
			return unsupportedServiceTierError(p, params)
		}
		if customRequested {
			return unsupportedCustomToolsError(p, params)
		}
		return nil // ordinary function tools preserve legacy compatibility
	}
	caps := cp.Capabilities()

	// Thinking: fail loudly rather than silently dropping chain-of-thought content
	if params.Thinking != nil && params.Thinking.Enabled && !caps.Thinking {
		return fmt.Errorf("%s", i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyProviderThinkingUnsupported, p.Name(), p.ModelID()))
	}
	if customRequested && caps.CustomTools != CapabilitySupported {
		return unsupportedCustomToolsError(p, params)
	}
	if params.ServiceTier != "" && caps.ServiceTier != CapabilitySupported {
		return unsupportedServiceTierError(p, params)
	}

	return nil
}

func unsupportedServiceTierError(p Provider, params Params) error {
	model := strings.TrimSpace(params.Model)
	if model == "" {
		model = p.ModelID()
	}
	return fmt.Errorf("%s", i18n.Format(
		i18n.DetectOrLoadLanguage(),
		i18n.KeyProviderServiceTierUnsupported,
		p.Name(),
		model,
	))
}

func validateToolDefinitionContracts(definitions []types.ToolDefinition) (bool, error) {
	custom := false
	for _, definition := range definitions {
		switch definition.Type {
		case types.ToolDefinitionTypeFunction:
			if definition.Format != nil {
				return false, invalidCustomToolDefinitionError(definition.Name)
			}
		case types.ToolDefinitionTypeCustom:
			custom = true
			format := definition.Format
			if format == nil || format.Type != "grammar" || format.Syntax != "lark" || strings.TrimSpace(format.Definition) == "" {
				return false, invalidCustomToolDefinitionError(definition.Name)
			}
		default:
			return false, invalidCustomToolDefinitionError(definition.Name)
		}
	}
	return custom, nil
}

func invalidCustomToolDefinitionError(name string) error {
	return fmt.Errorf("%s", i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyProviderCustomToolDefinitionInvalid, name))
}

func unsupportedCustomToolsError(p Provider, params Params) error {
	model := strings.TrimSpace(params.Model)
	if model == "" {
		model = p.ModelID()
	}
	return fmt.Errorf("%s", i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyProviderCustomToolsUnsupported, p.Name(), model))
}
