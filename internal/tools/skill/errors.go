package skill

// skillErrorCode classifies structured Skill tool failures. The model uses
// these codes to distinguish failures without parsing localized text.
//
// Codes:
//
//	1 = invalid format (empty / malformed name)
//	2 = unknown skill (typo or not loaded)
//	3 = reserved (parse / load failure)
//	4 = disableModelInvocation set (slash-only command)
type skillErrorCode int

const (
	skillErrInvalidFormat      skillErrorCode = 1
	skillErrUnknownSkill       skillErrorCode = 2
	skillErrLoadFailure        skillErrorCode = 3
	skillErrDisableModelInvoke skillErrorCode = 4
)

// String returns a stable text label for telemetry / metadata use.
func (c skillErrorCode) String() string {
	switch c {
	case skillErrInvalidFormat:
		return "invalid_format"
	case skillErrUnknownSkill:
		return "unknown_skill"
	case skillErrLoadFailure:
		return "load_failure"
	case skillErrDisableModelInvoke:
		return "disable_model_invocation"
	default:
		return ""
	}
}
