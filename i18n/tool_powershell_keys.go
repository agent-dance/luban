package i18n

// Model-facing PowerShell tool descriptions and its registry authorization
// failure. PowerShell is a product identifier and remains untranslated.
const (
	KeyToolPromptPowerShellDescription     Key = "tool.prompt.powershell.description"
	KeyToolPromptPowerShellCommand         Key = "tool.prompt.powershell.command"
	KeyToolPromptPowerShellTimeout         Key = "tool.prompt.powershell.timeout"
	KeyToolPromptPowerShellSummary         Key = "tool.prompt.powershell.summary"
	KeyToolPromptPowerShellRunInBackground Key = "tool.prompt.powershell.run_in_background"
	KeyToolPermissionPowerShellDispatch    Key = "tool.permission.powershell.registry_dispatch"
)

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}
	add(KeyToolPromptPowerShellDescription,
		"Execute a PowerShell command under the active permission policy.",
		"在当前权限策略下执行 PowerShell 命令。",
		"Führt einen PowerShell-Befehl gemäß der aktiven Berechtigungsrichtlinie aus.",
		"有効な権限ポリシーの下で PowerShell コマンドを実行します。",
		"현재 권한 정책에 따라 PowerShell 명령을 실행합니다.",
		"Выполняет команду PowerShell в соответствии с действующей политикой разрешений.")
	add(KeyToolPromptPowerShellCommand,
		"The PowerShell command to execute.",
		"要执行的 PowerShell 命令。",
		"Der auszuführende PowerShell-Befehl.",
		"実行する PowerShell コマンド。",
		"실행할 PowerShell 명령입니다.",
		"Команда PowerShell для выполнения.")
	add(KeyToolPromptPowerShellTimeout,
		"Timeout in milliseconds (maximum 600000).",
		"超时时间，单位为毫秒（最大 600000）。",
		"Zeitlimit in Millisekunden (maximal 600000).",
		"タイムアウト（ミリ秒、最大 600000）。",
		"제한 시간(밀리초, 최대 600000)입니다.",
		"Тайм-аут в миллисекундах (не более 600000).")
	add(KeyToolPromptPowerShellSummary,
		"A clear, concise, active-voice description of what the command does.",
		"用清晰、简洁的主动语态描述命令用途。",
		"Eine klare, knappe Beschreibung im Aktiv, was der Befehl ausführt.",
		"コマンドの動作を能動態で明確かつ簡潔に説明します。",
		"명령이 수행하는 작업을 능동태로 명확하고 간결하게 설명합니다.",
		"Ясное и краткое описание действия команды в активном залоге.")
	add(KeyToolPromptPowerShellRunInBackground,
		"Set to true to run in the background. Use TaskOutput to inspect its output later.",
		"设为 true 可在后台运行；之后使用 TaskOutput 查看输出。",
		"Auf true setzen, um den Befehl im Hintergrund auszuführen. Die Ausgabe kann später mit TaskOutput geprüft werden.",
		"バックグラウンドで実行する場合は true にします。後で TaskOutput を使って出力を確認してください。",
		"백그라운드에서 실행하려면 true로 설정하세요. 나중에 TaskOutput으로 출력을 확인할 수 있습니다.",
		"Установите true для фонового выполнения. Позже используйте TaskOutput для просмотра вывода.")
	add(KeyToolPermissionPowerShellDispatch,
		"The PowerShell command requires permission.",
		"此 PowerShell 命令需要授权。",
		"Der PowerShell-Befehl benötigt eine Berechtigung.",
		"PowerShell コマンドには許可が必要です。",
		"PowerShell 명령에 권한이 필요합니다.",
		"Для команды PowerShell требуется разрешение.")
}
