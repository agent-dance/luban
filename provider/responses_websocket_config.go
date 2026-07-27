package provider

import (
	"github.com/agent-dance/luban/i18n"
)

func responsesWebSocketProfileUnsupportedError() error {
	return i18n.NewError(i18n.KeyProviderResponsesWebSocketProfileUnsupported)
}
