package collaboration

import (
	"encoding/json"
	"fmt"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/swarm"
	"github.com/agent-dance/luban/types"
)

func responseJSON(content any) (types.ToolResult, error) {
	data, err := json.Marshal(content)
	if err != nil {
		return errorResult(i18n.NewError(i18n.KeyToolRuntimeResponseMarshalFailed, err)), nil
	}
	return types.ToolResult{Content: string(data), Outcome: types.ToolOutcomeSucceeded}, nil
}

func errorResult(err error) types.ToolResult {
	return types.ToolResult{Content: err.Error(), IsError: true, Outcome: types.ToolOutcomeFailed}
}

func teamToolError(key i18n.Key, args ...any) types.ToolResult {
	return errorResult(i18n.NewError(key, args...))
}

func runtimeText(key i18n.Key) string {
	return i18n.Text(i18n.DetectOrLoadLanguage(), key)
}

func runtimeFormat(key i18n.Key, args ...any) string {
	return i18n.Format(i18n.DetectOrLoadLanguage(), key, args...)
}

func swarmErrorResult(err error) types.ToolResult {
	return errorResult(fmt.Errorf("%s", swarm.UserFacingError(i18n.DetectOrLoadLanguage(), err)))
}
