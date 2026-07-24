package i18n

// Model-facing Bash tool and schema descriptions.
const (
	KeyToolPromptBashDescription     Key = "tool.prompt.bash.description"
	KeyToolPromptBashCommand         Key = "tool.prompt.bash.command"
	KeyToolPromptBashTimeout         Key = "tool.prompt.bash.timeout"
	KeyToolPromptBashSummary         Key = "tool.prompt.bash.summary"
	KeyToolPromptBashDisableSandbox  Key = "tool.prompt.bash.disable_sandbox"
	KeyToolPromptBashRunInBackground Key = "tool.prompt.bash.run_in_background"
)

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}
	add(KeyToolPromptBashDescription,
		"Execute a Bash command under the active permission and sandbox policy.", "在当前权限与 sandbox 策略下执行 Bash 命令。", "Führt einen Bash-Befehl unter der aktiven Berechtigungs- und Sandbox-Richtlinie aus.", "有効な権限および sandbox ポリシーの下で Bash コマンドを実行します。", "현재 권한 및 sandbox 정책에 따라 Bash 명령을 실행합니다.", "Выполняет команду Bash в соответствии с действующей политикой разрешений и sandbox.")
	add(KeyToolPromptBashCommand,
		"The Bash command to execute.", "要执行的 Bash 命令。", "Der auszuführende Bash-Befehl.", "実行する Bash コマンド。", "실행할 Bash 명령입니다.", "Команда Bash для выполнения.")
	add(KeyToolPromptBashTimeout,
		"Timeout in milliseconds (maximum %d).", "超时时间，单位为毫秒（最大 %d）。", "Zeitlimit in Millisekunden (maximal %d).", "タイムアウト（ミリ秒、最大 %d）。", "제한 시간(밀리초, 최대 %d)입니다.", "Тайм-аут в миллисекундах (не более %d).")
	add(KeyToolPromptBashSummary,
		"A clear, concise, active-voice description of what the command does.", "用清晰、简洁的主动语态描述命令用途。", "Eine klare, knappe Beschreibung im Aktiv, was der Befehl ausführt.", "コマンドの動作を能動態で明確かつ簡潔に説明します。", "명령이 수행하는 작업을 능동태로 명확하고 간결하게 설명합니다.", "Ясное и краткое описание действия команды в активном залоге.")
	add(KeyToolPromptBashDisableSandbox,
		"Set to true only to explicitly run without sandbox protection; safety policy still applies.", "仅在明确需要无 sandbox 保护运行时设为 true；安全策略仍然生效。", "Nur auf true setzen, um ausdrücklich ohne Sandbox-Schutz auszuführen; die Sicherheitsrichtlinie gilt weiterhin.", "sandbox 保護なしでの実行を明示的に要求する場合のみ true にします。安全ポリシーは引き続き適用されます。", "sandbox 보호 없이 실행하도록 명시적으로 요청할 때만 true로 설정하세요. 안전 정책은 계속 적용됩니다.", "Установите true только для явного запуска без защиты sandbox; политика безопасности продолжает действовать.")
	add(KeyToolPromptBashRunInBackground,
		"Set to true to run in the background. Use Read to inspect its output later.", "设为 true 可在后台运行；之后使用 Read 查看输出。", "Auf true setzen, um den Befehl im Hintergrund auszuführen. Die Ausgabe kann später mit Read geprüft werden.", "バックグラウンドで実行する場合は true にします。後で Read を使って出力を確認してください。", "백그라운드에서 실행하려면 true로 설정하세요. 나중에 Read로 출력을 확인할 수 있습니다.", "Установите true для фонового выполнения. Позже используйте Read для просмотра вывода.")
}
