package interaction

import "github.com/agent-dance/luban/i18n"

func enterPlanModePrompt() string {
	return i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolInteractionEnterPlanDescription)
}
