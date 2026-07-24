package i18n

const (
	KeyToolInputValidationFailedSingle        Key = "tool.input_validation.failed_single"
	KeyToolInputValidationFailedPlural        Key = "tool.input_validation.failed_plural"
	KeyToolInputValidationUnexpectedParameter Key = "tool.input_validation.unexpected_parameter"
	KeyToolInputValidationEnvelope            Key = "tool.input_validation.envelope"
)

var toolInputValidationKeys = []Key{
	KeyToolInputValidationFailedSingle,
	KeyToolInputValidationFailedPlural,
	KeyToolInputValidationUnexpectedParameter,
	KeyToolInputValidationEnvelope,
}

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en,
			LangZH: zh,
			LangDE: de,
			LangJA: ja,
			LangKO: ko,
			LangRU: ru,
		}
	}

	add(KeyToolInputValidationFailedSingle,
		"%s failed due to the following issue:\n%s",
		"工具 %s 的输入验证因以下问题而失败：\n%s",
		"Die Eingabeprüfung für Tool %s ist aufgrund des folgenden Problems fehlgeschlagen:\n%s",
		"ツール %s の入力検証は次の問題により失敗しました：\n%s",
		"도구 %s의 입력 검증이 다음 문제로 인해 실패했습니다:\n%s",
		"Проверка входных данных инструмента %s завершилась ошибкой из-за следующей проблемы:\n%s")
	add(KeyToolInputValidationFailedPlural,
		"%s failed due to the following issues:\n%s",
		"工具 %s 的输入验证因以下问题而失败：\n%s",
		"Die Eingabeprüfung für Tool %s ist aufgrund der folgenden Probleme fehlgeschlagen:\n%s",
		"ツール %s の入力検証は次の問題により失敗しました：\n%s",
		"도구 %s의 입력 검증이 다음 문제로 인해 실패했습니다:\n%s",
		"Проверка входных данных инструмента %s завершилась ошибкой из-за следующих проблем:\n%s")
	add(KeyToolInputValidationUnexpectedParameter,
		"An unexpected parameter `%s` was provided",
		"提供了不支持的参数 `%s`",
		"Es wurde ein unerwarteter Parameter `%s` angegeben",
		"予期しないパラメーター `%s` が指定されました",
		"예기치 않은 매개변수 `%s`이(가) 제공되었습니다",
		"Передан непредусмотренный параметр `%s`")
	add(KeyToolInputValidationEnvelope,
		"<tool_use_error>InputValidationError: %s</tool_use_error>",
		"<tool_use_error>InputValidationError: %s</tool_use_error>",
		"<tool_use_error>InputValidationError: %s</tool_use_error>",
		"<tool_use_error>InputValidationError: %s</tool_use_error>",
		"<tool_use_error>InputValidationError: %s</tool_use_error>",
		"<tool_use_error>InputValidationError: %s</tool_use_error>")
}

// ToolInputValidationLocalizer lets structured validation errors render their
// product copy in the same language as the protocol envelope. Implementations
// must keep schema field names, tool identifiers, and raw validation details
// as formatting parameters rather than translating them.
type ToolInputValidationLocalizer interface {
	LocalizedToolInputValidation(Language) string
}

// FormatToolInputValidationError renders the Claude-compatible validation
// envelope exactly once. Structured errors may localize their detail through
// ToolInputValidationLocalizer; unknown schema errors remain raw.
func FormatToolInputValidationError(lang Language, err error) string {
	if err == nil {
		return ""
	}
	var detail string
	if localizer, ok := err.(ToolInputValidationLocalizer); ok {
		detail = localizer.LocalizedToolInputValidation(lang)
	} else {
		detail = err.Error()
	}
	return Format(lang, KeyToolInputValidationEnvelope, detail)
}
