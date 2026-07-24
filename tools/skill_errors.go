package tools

import "fmt"

// SkillErrorCode mirrors the structured error codes returned by the
// TS SkillTool's validateInput. The model uses these to distinguish
// "typo" / "permission denied" / "wrong type" / "remote not loaded"
// without parsing English error strings.
//
// Codes:
//
//	1 = invalid format (empty / malformed name)
//	2 = unknown skill (typo or not loaded)
//	3 = reserved (parse / load failure)
//	4 = disableModelInvocation set (slash-only command)
//	5 = wrong type (not a prompt-based skill)
//	6 = remote skill not yet discovered
type SkillErrorCode int

const (
	SkillErrInvalidFormat       SkillErrorCode = 1
	SkillErrUnknownSkill        SkillErrorCode = 2
	SkillErrLoadFailure         SkillErrorCode = 3
	SkillErrDisableModelInvoke  SkillErrorCode = 4
	SkillErrNotPromptType       SkillErrorCode = 5
	SkillErrRemoteNotDiscovered SkillErrorCode = 6
)

// String returns a stable text label for telemetry / metadata use.
func (c SkillErrorCode) String() string {
	switch c {
	case SkillErrInvalidFormat:
		return "invalid_format"
	case SkillErrUnknownSkill:
		return "unknown_skill"
	case SkillErrLoadFailure:
		return "load_failure"
	case SkillErrDisableModelInvoke:
		return "disable_model_invocation"
	case SkillErrNotPromptType:
		return "not_prompt_type"
	case SkillErrRemoteNotDiscovered:
		return "remote_not_discovered"
	default:
		return fmt.Sprintf("unknown_code_%d", int(c))
	}
}

// Int returns the numeric code (mirrors TS errorCode field).
func (c SkillErrorCode) Int() int { return int(c) }
