package i18n

// Semantic copy for the development-only five-task Agentic Coding runner.
// Command names, paths, environment variables, and JSON fields are protocol
// identifiers and remain untranslated.
const (
	KeyAgenticPilotUsage              Key = "benchmark.pilot.usage"
	KeyAgenticPilotHostReceiptFlag    Key = "benchmark.pilot.flag.host_receipt"
	KeyAgenticPilotGuestPreflightFlag Key = "benchmark.pilot.flag.guest_preflight"
	KeyAgenticPilotPairLimitFlag      Key = "benchmark.pilot.flag.pair_limit"
)

var agenticPilotKeys = []Key{
	KeyAgenticPilotUsage, KeyAgenticPilotHostReceiptFlag,
	KeyAgenticPilotGuestPreflightFlag,
	KeyAgenticPilotPairLimitFlag,
}

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}
	add(KeyAgenticPilotUsage,
		"Usage: agenticpilot <preflight|run> --manifest PATH --backend-config PATH --work-dir PATH --host-receipt PATH --guest-preflight-receipt PATH [--execute]",
		"用法：agenticpilot <preflight|run> --manifest 路径 --backend-config 路径 --work-dir 路径 --host-receipt 路径 --guest-preflight-receipt 路径 [--execute]",
		"Aufruf: agenticpilot <preflight|run> --manifest PFAD --backend-config PFAD --work-dir PFAD --host-receipt PFAD --guest-preflight-receipt PFAD [--execute]",
		"使用法: agenticpilot <preflight|run> --manifest パス --backend-config パス --work-dir パス --host-receipt パス --guest-preflight-receipt パス [--execute]",
		"사용법: agenticpilot <preflight|run> --manifest 경로 --backend-config 경로 --work-dir 경로 --host-receipt 경로 --guest-preflight-receipt 경로 [--execute]",
		"Использование: agenticpilot <preflight|run> --manifest ПУТЬ --backend-config ПУТЬ --work-dir ПУТЬ --host-receipt ПУТЬ --guest-preflight-receipt ПУТЬ [--execute]")
	add(KeyAgenticPilotHostReceiptFlag,
		"Expected path of the development host-storage receipt.",
		"开发型主机存储回执的预期路径。",
		"Erwarteter Pfad des Entwicklungsbelegs für den Host-Speicher.",
		"開発用ホストストレージ受領書の予定パス。",
		"개발용 호스트 스토리지 영수증의 예상 경로입니다.",
		"Ожидаемый путь к квитанции хранилища хоста для разработки.")
	add(KeyAgenticPilotGuestPreflightFlag,
		"Path of the development guest-storage preflight receipt.",
		"开发型 guest 存储预检回执的路径。",
		"Pfad des Entwicklungsbelegs für die Gastspeicher-Vorprüfung.",
		"開発用ゲストストレージ事前検査受領書のパス。",
		"개발용 게스트 스토리지 사전 점검 영수증 경로입니다.",
		"Путь к квитанции предварительной проверки гостевого хранилища.")
	add(KeyAgenticPilotPairLimitFlag,
		"Run only the first N complete task-agent pairs, then pause safely.",
		"仅运行前 N 个完整任务-agent 配对，随后安全暂停。",
		"Nur die ersten N vollständigen Aufgabe-Agent-Paare ausführen und dann sicher pausieren.",
		"先頭 N 個の完全なタスク-agent ペアだけを実行し、安全に一時停止します。",
		"처음 N개의 완전한 task-agent 쌍만 실행한 뒤 안전하게 일시 중지합니다.",
		"Выполнить только первые N полных пар задача-agent, затем безопасно приостановить запуск.")
}
