package tools

import (
	"fmt"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/swarm"
	"github.com/agent-dance/luban/types"
)

func swarmErrorResponse(err error) types.ToolResult {
	return ErrorResponse(fmt.Errorf("%s", swarm.UserFacingError(i18n.DetectOrLoadLanguage(), err)))
}
