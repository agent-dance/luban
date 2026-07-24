// Package i18n provides multi-language support for the LUBAN Code UI.
// It includes translations for English, Chinese, German, Japanese, Korean, and Russian.
package i18n

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"github.com/agent-dance/luban/brand"
)

// Language represents a supported language.
type Language int

var detectedLanguageCache atomic.Int32

func init() {
	detectedLanguageCache.Store(-1)
}

const (
	LangEN Language = iota // English
	LangZH                 // Chinese (Simplified)
	LangDE                 // German
	LangJA                 // Japanese
	LangKO                 // Korean
	LangRU                 // Russian
)

// String returns the display name of the language.
func (l Language) String() string {
	switch l {
	case LangEN:
		return "English"
	case LangZH:
		return "中文"
	case LangDE:
		return "Deutsch"
	case LangJA:
		return "日本語"
	case LangKO:
		return "한국어"
	case LangRU:
		return "Русский"
	default:
		return "English"
	}
}

// Code returns the ISO 639-1 language code.
func (l Language) Code() string {
	switch l {
	case LangEN:
		return "en"
	case LangZH:
		return "zh"
	case LangDE:
		return "de"
	case LangJA:
		return "ja"
	case LangKO:
		return "ko"
	case LangRU:
		return "ru"
	default:
		return "en"
	}
}

// Next returns the next language in the cycle.
func (l Language) Next() Language {
	switch l {
	case LangEN:
		return LangZH
	case LangZH:
		return LangDE
	case LangDE:
		return LangJA
	case LangJA:
		return LangKO
	case LangKO:
		return LangRU
	case LangRU:
		return LangEN
	default:
		return LangEN
	}
}

// AllLanguages returns all supported languages.
func AllLanguages() []Language {
	return []Language{LangEN, LangZH, LangDE, LangJA, LangKO, LangRU}
}

// DetectLanguage detects the system language by checking:
// 1. The LANG environment variable
// 2. The LC_ALL / LC_MESSAGES environment variables
// 3. The OS locale command output
// 4. Falls back to timezone-based detection
// 5. Defaults to English
func DetectLanguage() Language {
	// Try LANG env var
	if lang := os.Getenv("LANG"); lang != "" {
		if l := parseLangCode(lang); l != nil {
			return *l
		}
	}

	// Try LC_ALL and LC_MESSAGES
	for _, env := range []string{"LC_ALL", "LC_MESSAGES"} {
		if v := os.Getenv(env); v != "" {
			if l := parseLangCode(v); l != nil {
				return *l
			}
		}
	}

	// Try locale command on Unix
	if runtime.GOOS != "windows" {
		cmd := exec.Command("locale", "-a")
		if output, err := cmd.Output(); err == nil {
			lines := strings.Split(string(output), "\n")
			for _, line := range lines {
				if l := parseLangCode(line); l != nil {
					return *l
				}
			}
		}
	}

	// Fallback: detect by timezone
	return detectLanguageByTimezone()
}

func parseLangCode(s string) *Language {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	// Handle formats like "en_US.UTF-8", "zh_CN", "de_DE@euro", "ja_JP"
	// Also handle plain codes like "en", "zh"
	lang := strings.Split(s, ".")[0]
	lang = strings.Split(lang, "@")[0]
	lang = strings.Split(lang, "_")[0]
	lang = strings.ToLower(lang)

	var l Language
	switch lang {
	case "en":
		l = LangEN
	case "zh", "cmn":
		l = LangZH
	case "de":
		l = LangDE
	case "ja":
		l = LangJA
	case "ko":
		l = LangKO
	case "ru":
		l = LangRU
	default:
		return nil
	}
	return &l
}

func detectLanguageByTimezone() Language {
	now := time.Now()
	name, offset := now.Zone()
	name = strings.ToLower(name)

	// Timezone region detection
	// Asia timezones
	asianZones := []string{"cst", "asia", "shanghai", "beijing", "chongqing", "hongkong", "taipei", "singapore"}
	for _, z := range asianZones {
		if strings.Contains(name, z) {
			// Distinguish Chinese from Japanese/Korean
			// China is UTC+8, Japan is UTC+9, Korea is UTC+9
			switch {
			case strings.Contains(name, "japan") || strings.Contains(name, "jst") || strings.Contains(name, "tokyo"):
				return LangJA
			case strings.Contains(name, "korea") || strings.Contains(name, "kst") || strings.Contains(name, "seoul"):
				return LangKO
			case offset == 28800: // UTC+8
				return LangZH
			case offset == 32400: // UTC+9
				// Could be Japan or Korea, default to Japanese
				return LangJA
			}
		}
	}

	// European timezones
	if strings.Contains(name, "cet") || strings.Contains(name, "eet") ||
		strings.Contains(name, "europe/berlin") || strings.Contains(name, "europe/vienna") ||
		strings.Contains(name, "europe/zurich") {
		return LangDE
	}
	if strings.Contains(name, "eet") || strings.Contains(name, "europe") ||
		strings.Contains(name, "msk") || strings.Contains(name, "moscow") {
		return LangRU
	}

	// Japanese timezone directly
	if strings.Contains(name, "tokyo") || strings.Contains(name, "jst") {
		return LangJA
	}

	// Korean timezone directly
	if strings.Contains(name, "seoul") || strings.Contains(name, "kst") {
		return LangKO
	}

	return LangEN
}

// LanguageFile returns the path to the language preference file.
func LanguageFile() string {
	return filepath.Join(brand.UserConfigDir(), "language.json")
}

// SaveLanguage persists the language preference to disk.
func SaveLanguage(lang Language) error {
	dir := brand.UserConfigDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.Marshal(map[string]string{"language": lang.Code()})
	if err != nil {
		return err
	}
	if err := os.WriteFile(LanguageFile(), data, 0644); err != nil {
		return err
	}
	detectedLanguageCache.Store(int32(lang))
	return nil
}

// LoadLanguage loads the language preference from disk.
// Returns LangEN and false if no saved preference exists.
func LoadLanguage() (Language, bool) {
	data, err := os.ReadFile(LanguageFile())
	if err != nil {
		return LangEN, false
	}
	var cfg map[string]string
	if err := json.Unmarshal(data, &cfg); err != nil {
		return LangEN, false
	}
	code, ok := cfg["language"]
	if !ok {
		return LangEN, false
	}
	for _, l := range AllLanguages() {
		if l.Code() == code {
			return l, true
		}
	}
	return LangEN, false
}

// DetectOrLoadLanguage detects the system language on first run,
// or loads the saved preference on subsequent runs.
func DetectOrLoadLanguage() Language {
	if cached := detectedLanguageCache.Load(); cached >= int32(LangEN) && cached <= int32(LangRU) {
		return Language(cached)
	}
	resolved := LangEN
	if lang, ok := LoadLanguage(); ok {
		resolved = lang
	} else {
		resolved = DetectLanguage()
	}
	detectedLanguageCache.CompareAndSwap(-1, int32(resolved))
	return Language(detectedLanguageCache.Load())
}

// --- Translation helpers ---

// T translates a key string into the given language.
// It uses fmt.Sprintf-style formatting with args.
func T(lang Language, key string, args ...interface{}) string {
	text := getTranslation(lang, key)
	if text == "" {
		// i18n:allow forced-english wire-compat -- Legacy helper fallback retained only for unmigrated callers.
		text = getTranslation(LangEN, key)
	}
	if text == "" {
		text = key // preserve the English key while still applying format arguments
	}
	if len(args) > 0 {
		return fmt.Sprintf(text, args...)
	}
	return text
}

// TString returns the translated string without formatting.
func TString(lang Language, key string) string {
	text := getTranslation(lang, key)
	if text == "" {
		// i18n:allow forced-english wire-compat -- Legacy helper fallback retained only for unmigrated callers.
		text = getTranslation(LangEN, key)
	}
	if text == "" {
		return key
	}
	return text
}

// getTranslation looks up a key in the translations map.
func getTranslation(lang Language, key string) string {
	if m, ok := translations[key]; ok {
		if s, ok := m[lang]; ok {
			return s
		}
	}
	return ""
}

// translations holds all UI strings in all supported languages.
// Key is the English text (or a semantic key), value is a map of Language->translation.
var translations = map[string]map[Language]string{
	"No conversation turns are available to fork.": {
		LangEN: "No conversation turns are available to fork.",
		LangZH: "当前没有可用于分叉的对话轮次。",
		LangDE: "Es sind keine Gesprächsrunden zum Verzweigen verfügbar.",
		LangJA: "フォークできる会話ターンがありません。",
		LangKO: "포크할 수 있는 대화 턴이 없습니다.",
		LangRU: "Нет доступных реплик диалога для ответвления.",
	},

	// --- Mode names ---
	"Auto": {
		LangEN: "Auto",
		LangZH: "自动",
		LangDE: "Auto",
		LangJA: "自動",
		LangKO: "자동",
		LangRU: "Авто",
	},
	"Ask": {
		LangEN: "Ask",
		LangZH: "询问",
		LangDE: "Fragen",
		LangJA: "確認",
		LangKO: "확인",
		LangRU: "Спрашивать",
	},
	"Plan": {
		LangEN: "Plan",
		LangZH: "计划",
		LangDE: "Planen",
		LangJA: "計画",
		LangKO: "계획",
		LangRU: "План",
	},
	"mode": {
		LangEN: "mode",
		LangZH: "模式",
		LangDE: "Modus",
		LangJA: "モード",
		LangKO: "모드",
		LangRU: "режим",
	},
	"%s mode": {
		LangEN: "%s mode",
		LangZH: "%s模式",
		LangDE: "%s-Modus",
		LangJA: "%sモード",
		LangKO: "%s 모드",
		LangRU: "режим %s",
	},

	// --- Goal status ---
	"Goal: ": {
		LangEN: "Goal: ",
		LangZH: "目标：",
		LangDE: "Ziel: ",
		LangJA: "目標：",
		LangKO: "목표: ",
		LangRU: "Цель: ",
	},
	"Goal paused: ": {
		LangEN: "Goal paused: ",
		LangZH: "目标已暂停：",
		LangDE: "Ziel pausiert: ",
		LangJA: "目標一時停止：",
		LangKO: "목표 일시 중지: ",
		LangRU: "Цель приостановлена: ",
	},

	// --- Session usage ---
	"Session: in %s · %d%% cached · out %s · $%.4f": {
		LangEN: "Session: in %s · %d%% cached · out %s · $%.4f",
		LangZH: "会话：输入 %s · %d%% 缓存 · 输出 %s · $%.4f",
		LangDE: "Sitzung: %s rein · %d%% gecached · %s raus · $%.4f",
		LangJA: "セッション：入力 %s · %d%% キャッシュ · 出力 %s · $%.4f",
		LangKO: "세션: 입력 %s · %d%% 캐시됨 · 출력 %s · $%.4f",
		LangRU: "Сессия: вх %s · %d%% кэш · вых %s · $%.4f",
	},
	"%d web search": {
		LangEN: "%d web search",
		LangZH: "%d 次网络搜索",
		LangDE: "%d Websuche",
		LangJA: "%d 回ウェブ検索",
		LangKO: "%d회 웹 검색",
		LangRU: "%d поисков в вебе",
	},
	"%d web searches": {
		LangEN: "%d web searches",
		LangZH: "%d 次网络搜索",
		LangDE: "%d Websuchen",
		LangJA: "%d 回のウェブ検索",
		LangKO: "%d회 웹 검색",
		LangRU: "%d поисков в вебе",
	},
	"show all evidence": {
		LangEN: "show all evidence",
		LangZH: "显示所有证据",
		LangDE: "alle Beweise anzeigen",
		LangJA: "すべての証拠を表示",
		LangKO: "모든 증거 표시",
		LangRU: "показать все доказательства",
	},

	// --- Input area ---
	"Type a message... (Ctrl+D to exit)": {
		LangEN: "Type a message... (Ctrl+D to exit)",
		LangZH: "输入消息... (Ctrl+D 退出)",
		LangDE: "Nachricht eingeben... (Strg+D zum Beenden)",
		LangJA: "メッセージを入力... (Ctrl+Dで終了)",
		LangKO: "메시지 입력... (Ctrl+D 종료)",
		LangRU: "Введите сообщение... (Ctrl+D для выхода)",
	},

	// --- Slash commands ---
	"Slash Commands — Up/Down move, Tab complete, Enter run, Esc close": {
		LangEN: "Slash Commands — Up/Down move, Tab complete, Enter run, Esc close",
		LangZH: "斜杠命令 — 上下键移动，Tab补全，回车执行，Esc关闭",
		LangDE: "Slash-Befehle — Hoch/Runter bewegen, Tab vervollständigen, Enter ausführen, Esc schließen",
		LangJA: "スラッシュコマンド — 上下で移動、Tabで補完、Enterで実行、Escで閉じる",
		LangKO: "슬래시 명령 — 위/아래 이동, Tab 완성, Enter 실행, Esc 닫기",
		LangRU: "Slash-команды — Вверх/Вниз движение, Tab завершение, Enter запуск, Esc закрыть",
	},

	// --- Permission dialog ---
	"Permission Decision": {
		LangEN: "Permission Decision",
		LangZH: "权限决定",
		LangDE: "Berechtigungsentscheidung",
		LangJA: "許可の決定",
		LangKO: "권한 결정",
		LangRU: "Решение о разрешении",
	},
	"Plan Decision": {
		LangEN: "Plan Decision",
		LangZH: "计划决定",
		LangDE: "Planungsentscheidung",
		LangJA: "計画の決定",
		LangKO: "계획 결정",
		LangRU: "Решение плана",
	},
	"Actor: %s (%s)  Work: %s": {
		LangEN: "Actor: %s (%s)  Work: %s",
		LangZH: "执行者：%s（%s）工作单元：%s",
		LangDE: "Akteur: %s (%s)  Arbeit: %s",
		LangJA: "実行者：%s（%s）作業：%s",
		LangKO: "실행자: %s(%s) 작업: %s",
		LangRU: "Исполнитель: %s (%s)  Работа: %s",
	},
	"Action: ": {
		LangEN: "Action: ",
		LangZH: "操作：",
		LangDE: "Aktion: ",
		LangJA: "操作：",
		LangKO: "작업: ",
		LangRU: "Действие: ",
	},
	"Target: ": {
		LangEN: "Target: ",
		LangZH: "目标：",
		LangDE: "Ziel: ",
		LangJA: "対象：",
		LangKO: "대상: ",
		LangRU: "Цель: ",
	},
	"Impact: ": {
		LangEN: "Impact: ",
		LangZH: "影响：",
		LangDE: "Auswirkung: ",
		LangJA: "影響：",
		LangKO: "영향: ",
		LangRU: "Влияние: ",
	},
	"Risk: ": {
		LangEN: "Risk: ",
		LangZH: "风险：",
		LangDE: "Risiko: ",
		LangJA: "リスク：",
		LangKO: "위험: ",
		LangRU: "Риск: ",
	},
	"Scope: ": {
		LangEN: "Scope: ",
		LangZH: "范围：",
		LangDE: "Umfang: ",
		LangJA: "範囲：",
		LangKO: "범위: ",
		LangRU: "Область: ",
	},
	"Input: ": {
		LangEN: "Input: ",
		LangZH: "输入：",
		LangDE: "Eingabe: ",
		LangJA: "入力：",
		LangKO: "입력: ",
		LangRU: "Ввод: ",
	},
	"After approval: permission mode ": {
		LangEN: "After approval: permission mode ",
		LangZH: "批准后权限模式：",
		LangDE: "Nach Genehmigung: Berechtigungsmodus ",
		LangJA: "承認後の許可モード：",
		LangKO: "승인 후 권한 모드: ",
		LangRU: "После утверждения: режим разрешений ",
	},
	"Agent execution session: ": {
		LangEN: "Agent execution session: ",
		LangZH: "代理执行会话：",
		LangDE: "Agenten-Ausführungssitzung: ",
		LangJA: "エージェント実行セッション：",
		LangKO: "에이전트 실행 세션: ",
		LangRU: "Сессия выполнения агента: ",
	},
	"low": {
		LangEN: "low",
		LangZH: "低",
		LangDE: "niedrig",
		LangJA: "低",
		LangKO: "낮음",
		LangRU: "низкий",
	},
	"medium": {
		LangEN: "medium",
		LangZH: "中",
		LangDE: "mittel",
		LangJA: "中",
		LangKO: "중간",
		LangRU: "средний",
	},
	"high": {
		LangEN: "high",
		LangZH: "高",
		LangDE: "hoch",
		LangJA: "高",
		LangKO: "높음",
		LangRU: "высокий",
	},
	"Allow once": {
		LangEN: "Allow once",
		LangZH: "允许一次",
		LangDE: "Einmal erlauben",
		LangJA: "一度だけ許可",
		LangKO: "한 번 허용",
		LangRU: "Разрешить один раз",
	},
	"Always allow": {
		LangEN: "Always allow",
		LangZH: "始终允许",
		LangDE: "Immer erlauben",
		LangJA: "常に許可",
		LangKO: "항상 허용",
		LangRU: "Всегда разрешать",
	},
	"Execute": {
		LangEN: "Execute",
		LangZH: "执行",
		LangDE: "Ausführen",
		LangJA: "実行",
		LangKO: "실행",
		LangRU: "Выполнить",
	},
	"Stay in Plan": {
		LangEN: "Stay in Plan",
		LangZH: "停留在计划模式",
		LangDE: "Im Planmodus bleiben",
		LangJA: "計画モードに留まる",
		LangKO: "계획 모드 유지",
		LangRU: "Оставаться в плане",
	},
	"Reject": {
		LangEN: "Reject",
		LangZH: "拒绝",
		LangDE: "Ablehnen",
		LangJA: "拒否",
		LangKO: "거부",
		LangRU: "Отклонить",
	},
	"Allow": {
		LangEN: "Allow",
		LangZH: "允许",
		LangDE: "Erlauben",
		LangJA: "許可",
		LangKO: "허용",
		LangRU: "Разрешить",
	},

	// --- Permission legacy ---
	"Permission Required": {
		LangEN: "Permission Required",
		LangZH: "需要权限",
		LangDE: "Berechtigung erforderlich",
		LangJA: "許可が必要です",
		LangKO: "권한 필요",
		LangRU: "Требуется разрешение",
	},
	"Allow? [y/N/a]: ": {
		LangEN: "Allow? [y/N/a]: ",
		LangZH: "允许？[y/N/a]：",
		LangDE: "Erlauben? [j/N/i]: ",
		LangJA: "許可しますか？[y/N/a]：",
		LangKO: "허용하시겠습니까? [y/N/a]: ",
		LangRU: "Разрешить? [y/N/a]: ",
	},
	"Tool": {
		LangEN: "Tool",
		LangZH: "工具",
		LangDE: "Werkzeug",
		LangJA: "ツール",
		LangKO: "도구",
		LangRU: "Инструмент",
	},

	// --- Term renderer strings ---
	"Session: %s": {
		LangEN: "Session: %s",
		LangZH: "会话：%s",
		LangDE: "Sitzung: %s",
		LangJA: "セッション：%s",
		LangKO: "세션: %s",
		LangRU: "Сессия: %s",
	},
	"Tools: %s": {
		LangEN: "Tools: %s",
		LangZH: "工具：%s",
		LangDE: "Werkzeuge: %s",
		LangJA: "ツール：%s",
		LangKO: "도구: %s",
		LangRU: "Инструменты: %s",
	},
	"Type a task. Use /help for commands, or 'exit' to quit.": {
		LangEN: "Type a task. Use /help for commands, or 'exit' to quit.",
		LangZH: "输入任务。使用 /help 查看命令，或输入 'exit' 退出。",
		LangDE: "Geben Sie eine Aufgabe ein. Verwenden Sie /help für Befehle, oder 'exit' zum Beenden.",
		LangJA: "タスクを入力してください。コマンドは /help、終了するには 'exit' と入力。",
		LangKO: "작업을 입력하세요. 명령어는 /help, 종료는 'exit'를 입력하세요.",
		LangRU: "Введите задачу. Используйте /help для команд или 'exit' для выхода.",
	},
	"Goodbye!": {
		LangEN: "Goodbye!",
		LangZH: "再见！",
		LangDE: "Auf Wiedersehen!",
		LangJA: "さようなら！",
		LangKO: "안녕히 가세요!",
		LangRU: "До свидания!",
	},
	"Session closed.": {
		LangEN: "Session closed.",
		LangZH: "会话已关闭。",
		LangDE: "Sitzung geschlossen.",
		LangJA: "セッションを閉じました。",
		LangKO: "세션이 종료되었습니다.",
		LangRU: "Сессия закрыта.",
	},
	"> ": {
		LangEN: "> ",
		LangZH: "> ",
		LangDE: "> ",
		LangJA: "> ",
		LangKO: "> ",
		LangRU: "> ",
	},
	"💰 Turn: $%.4f | Session: $%.4f | Tokens: %s in / %s out": {
		LangEN: "💰 Turn: $%.4f | Session: $%.4f | Tokens: %s in / %s out",
		LangZH: "💰 本轮：$%.4f | 会话：$%.4f | 令牌：输入 %s / 输出 %s",
		LangDE: "💰 Runde: $%.4f | Sitzung: $%.4f | Token: %s rein / %s raus",
		LangJA: "💰 ターン：$%.4f | セッション：$%.4f | トークン：入力 %s / 出力 %s",
		LangKO: "💰 턴: $%.4f | 세션: $%.4f | 토큰: 입력 %s / 출력 %s",
		LangRU: "💰 Шаг: $%.4f | Сессия: $%.4f | Токены: вх %s / вых %s",
	},
	"[Context: %s %.0f%% (%s/%s)]": {
		LangEN: "[Context: %s %.0f%% (%s/%s)]",
		LangZH: "[上下文：%s %.0f%%（%s/%s）]",
		LangDE: "[Kontext: %s %.0f%% (%s/%s)]",
		LangJA: "[コンテキスト：%s %.0f%%（%s/%s）]",
		LangKO: "[컨텍스트: %s %.0f%% (%s/%s)]",
		LangRU: "[Контекст: %s %.0f%% (%s/%s)]",
	},
	"Error: ": {
		LangEN: "Error: ",
		LangZH: "错误：",
		LangDE: "Fehler: ",
		LangJA: "エラー：",
		LangKO: "오류: ",
		LangRU: "Ошибка: ",
	},

	// --- User message prefix ---
	"You: ": {
		LangEN: "You: ",
		LangZH: "你：",
		LangDE: "Sie: ",
		LangJA: "あなた：",
		LangKO: "나: ",
		LangRU: "Вы: ",
	},

	// --- Clipboard / paste ---
	"Copied!": {
		LangEN: "Copied!",
		LangZH: "已复制！",
		LangDE: "Kopiert!",
		LangJA: "コピーしました！",
		LangKO: "복사됨!",
		LangRU: "Скопировано!",
	},
	"⚠ Prompt history not saved": {
		LangEN: "⚠ Prompt history not saved",
		LangZH: "⚠ 提示历史未保存",
		LangDE: "⚠ Verlauf nicht gespeichert",
		LangJA: "⚠ プロンプト履歴が保存されていません",
		LangKO: "⚠ 프롬프트 기록이 저장되지 않았습니다",
		LangRU: "⚠ История запросов не сохранена",
	},
	"[Pasted text #%d +%d lines]": {
		LangEN: "[Pasted text #%d +%d lines]",
		LangZH: "[粘贴文本 #%d 共%d行]",
		LangDE: "[Eingefügter Text #%d +%d Zeilen]",
		LangJA: "[貼り付けたテキスト #%d +%d行]",
		LangKO: "[붙여넣은 텍스트 #%d +%d줄]",
		LangRU: "[Вставленный текст #%d +%d строк]",
	},
	"📷 [Image #%d] (%s)": {
		LangEN: "📷 [Image #%d] (%s)",
		LangZH: "📷 [图片 #%d]（%s）",
		LangDE: "📷 [Bild #%d] (%s)",
		LangJA: "📷 [画像 #%d]（%s）",
		LangKO: "📷 [이미지 #%d] (%s)",
		LangRU: "📷 [Изображение #%d] (%s)",
	},

	// --- Slash suggestions navigation ---
	"  (%d/%d)": {
		LangEN: "  (%d/%d)",
		LangZH: "（%d/%d）",
		LangDE: "  (%d/%d)",
		LangJA: "（%d/%d）",
		LangKO: "（%d/%d）",
		LangRU: "  (%d/%d)",
	},

	// --- Notice / info text on home screen ---
	"Home": {
		LangEN: "Home",
		LangZH: "主页",
		LangDE: "Start",
		LangJA: "ホーム",
		LangKO: "홈",
		LangRU: "Главная",
	},

	// --- Language names for display ---
	"Language: %s": {
		LangEN: "Language: %s",
		LangZH: "语言：%s",
		LangDE: "Sprache: %s",
		LangJA: "言語：%s",
		LangKO: "언어: %s",
		LangRU: "Язык: %s",
	},
}
