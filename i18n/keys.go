package i18n

import (
	"fmt"
)

// Key is a stable, semantic identifier for user-visible product copy.
// New UI text must use Text or Format with one of these keys rather than
// passing an English sentence to the legacy T/TString helpers.
type Key string

const (
	KeyLanguageMenuTitle    Key = "language.menu.title"
	KeyLanguageCurrent      Key = "language.current"
	KeyLanguageSwitched     Key = "language.switched"
	KeyLanguageUnsupported  Key = "language.unsupported"
	KeyLanguageUnavailable  Key = "language.unavailable"
	KeyModeSwitchedAuto     Key = "mode.switched.auto"
	KeyModeSwitchedAsk      Key = "mode.switched.ask"
	KeyModeSwitchedPlan     Key = "mode.switched.plan"
	KeyActivityRunning      Key = "tui.activity.running"
	KeyTUISessionValueTotal Key = "tui.session.value_total"

	KeyAdapterAggregateAction        Key = "presentation.adapter.aggregate.action"
	KeyAdapterAggregateMembers       Key = "presentation.adapter.aggregate.members"
	KeyAdapterAggregateSummary       Key = "presentation.adapter.aggregate.summary"
	KeyAdapterActionShell            Key = "presentation.adapter.action.shell"
	KeyAdapterActionFileRead         Key = "presentation.adapter.action.file_read"
	KeyAdapterActionFileWrite        Key = "presentation.adapter.action.file_write"
	KeyAdapterActionSearch           Key = "presentation.adapter.action.search"
	KeyAdapterActionWeb              Key = "presentation.adapter.action.web"
	KeyAdapterActionMCP              Key = "presentation.adapter.action.mcp"
	KeyAdapterActionAgent            Key = "presentation.adapter.action.agent"
	KeyAdapterActionDecision         Key = "presentation.adapter.action.decision"
	KeyAdapterActionMessage          Key = "presentation.adapter.action.message"
	KeyAdapterCommandRunning         Key = "presentation.adapter.command.running"
	KeyAdapterCommandUnstructured    Key = "presentation.adapter.command.unstructured"
	KeyAdapterCommandTerminal        Key = "presentation.adapter.command.terminal"
	KeyAdapterCommandDisplayRisk     Key = "presentation.adapter.command.display_risk"
	KeyAdapterCommandNext            Key = "presentation.adapter.command.next"
	KeyAdapterCommandEvidenceRefs    Key = "presentation.adapter.command.evidence_refs"
	KeyAdapterCommandMoreRetained    Key = "presentation.adapter.command.more_retained"
	KeyAdapterCommandSensitiveHidden Key = "presentation.adapter.command.sensitive_hidden"
	KeyAdapterReviewNext             Key = "presentation.adapter.review_next"
	KeyToolSegmentReadFiles          Key = "tui.tool_segment.read_files"
	KeyToolSegmentUsedTools          Key = "tui.tool_segment.used_tools"
	KeyToolSegmentIssues             Key = "tui.tool_segment.issues"
)

// Text resolves a semantic UI key in lang.
func Text(lang Language, key Key) string {
	return Format(lang, key)
}

// Format resolves a semantic UI key and applies printf-style arguments.
func Format(lang Language, key Key, args ...any) string {
	translations, ok := semanticTranslations[key]
	if !ok {
		return fmt.Sprintf("[%s]", key)
	}
	text, ok := translations[lang]
	if !ok {
		text = translations[LangEN]
	}
	if len(args) == 0 {
		return text
	}
	return fmt.Sprintf(text, args...)
}

// ValidateSemanticCatalog verifies that every semantic key is translated in
// every language supported by the application. It is intended for tests and
// release checks.
func ValidateSemanticCatalog() error {
	for key, translations := range semanticTranslations {
		for _, lang := range AllLanguages() {
			if translations[lang] == "" {
				return fmt.Errorf("missing translation for %q in %s", key, lang.Code())
			}
		}
	}
	return nil
}

var semanticTranslations = map[Key]map[Language]string{
	KeyTUISessionValueTotal: {
		LangEN: "%s (%s total)", LangZH: "%s（总计 %s）", LangDE: "%s (%s gesamt)",
		LangJA: "%s（合計 %s）", LangKO: "%s(총 %s)", LangRU: "%s (всего %s)",
	},
	KeyLanguageMenuTitle: {
		LangEN: "Display language — Up/Down move, Tab complete, Enter select, Esc close",
		LangZH: "显示语言 — 上/下选择，Tab 补全，Enter 确认，Esc 关闭",
		LangDE: "Anzeigesprache — ↑/↓ auswählen, Tab vervollständigen, Enter bestätigen, Esc schließen",
		LangJA: "表示言語 — ↑/↓で移動、Tabで補完、Enterで選択、Escで閉じる",
		LangKO: "표시 언어 — ↑/↓ 이동, Tab 완성, Enter 선택, Esc 닫기",
		LangRU: "Язык интерфейса — ↑/↓ выбрать, Tab дополнить, Enter подтвердить, Esc закрыть",
	},
	KeyLanguageCurrent: {
		LangEN: "Language: %s", LangZH: "语言：%s", LangDE: "Sprache: %s",
		LangJA: "言語：%s", LangKO: "언어: %s", LangRU: "Язык: %s",
	},
	KeyLanguageSwitched: {
		LangEN: "Switched to %s",
		LangZH: "已切换为%s",
		LangDE: "Gewechselt zu %s",
		LangJA: "%s に切り替えました",
		LangKO: "%s(으)로 전환했습니다",
		LangRU: "Переключено на %s",
	},
	KeyLanguageUnsupported: {
		LangEN: "Unsupported language. Choose an item from /language, or use a supported code.",
		LangZH: "不支持该语言。请从 /language 列表中选择，或输入受支持的语言代码。",
		LangDE: "Nicht unterstützte Sprache. Wähle einen Eintrag unter /language oder verwende einen unterstützten Code.",
		LangJA: "未対応の言語です。/language の一覧から選ぶか、対応するコードを入力してください。",
		LangKO: "지원하지 않는 언어입니다. /language 목록에서 선택하거나 지원되는 코드를 사용하세요.",
		LangRU: "Неподдерживаемый язык. Выберите пункт в /language или используйте поддерживаемый код.",
	},
	KeyLanguageUnavailable: {
		LangEN: "Language switching is not available.",
		LangZH: "当前无法切换语言。",
		LangDE: "Die Sprache kann derzeit nicht gewechselt werden.",
		LangJA: "現在、言語を切り替えることはできません。",
		LangKO: "현재 언어를 전환할 수 없습니다.",
		LangRU: "Переключение языка сейчас недоступно.",
	},
	KeyModeSwitchedAuto: {
		LangEN: "⚡ Switched to Auto mode — tools run automatically",
		LangZH: "⚡ 已切换到自动模式 — 工具将自动运行",
		LangDE: "⚡ In den Automatikmodus gewechselt — Tools werden automatisch ausgeführt",
		LangJA: "⚡ 自動モードに切り替えました — ツールは自動で実行されます",
		LangKO: "⚡ 자동 모드로 전환했습니다 — 도구가 자동으로 실행됩니다",
		LangRU: "⚡ Переключено в автоматический режим — инструменты запускаются автоматически",
	},
	KeyModeSwitchedAsk: {
		LangEN: "❓ Switched to Ask mode — you'll be asked before each tool use",
		LangZH: "❓ 已切换到询问模式 — 每次使用工具前都会征求你的确认",
		LangDE: "❓ In den Bestätigungsmodus gewechselt — vor jeder Tool-Nutzung wirst du gefragt",
		LangJA: "❓ 確認モードに切り替えました — ツールを使う前に毎回確認します",
		LangKO: "❓ 확인 모드로 전환했습니다 — 도구를 사용할 때마다 확인합니다",
		LangRU: "❓ Переключено в режим подтверждения — перед каждым использованием инструмента будет запрос",
	},
	KeyModeSwitchedPlan: {
		LangEN: "📋 Switched to Plan mode — AI will plan before making changes",
		LangZH: "📋 已切换到计划模式 — AI 会先制定计划再进行修改",
		LangDE: "📋 In den Planmodus gewechselt — die KI plant Änderungen vor der Ausführung",
		LangJA: "📋 計画モードに切り替えました — AI は変更前に計画を立てます",
		LangKO: "📋 계획 모드로 전환했습니다 — AI가 변경 전에 계획을 세웁니다",
		LangRU: "📋 Переключено в режим планирования — ИИ составит план перед изменениями",
	},
}
