package i18n

// Semantic copy emitted by tool runtimes onto user-visible result, progress,
// notification, and accessibility surfaces. Tool names, commands, paths,
// identifiers, protocol values, and raw external errors are format arguments
// and remain untranslated.
const (
	KeyToolRuntimeErrorPrefix           Key = "tool.runtime.error.prefix"
	KeyToolRuntimeInvalidInput          Key = "tool.runtime.error.invalid_input"
	KeyToolRuntimeResponseMarshalFailed Key = "tool.runtime.error.response_marshal_failed"
	KeyToolRuntimeRequiredFieldMissing  Key = "tool.runtime.error.required_field_missing"
	KeyToolRuntimeFieldStringRequired   Key = "tool.runtime.error.field_string_required"

	KeyToolRuntimeAgentSourceProjectLabel Key = "tool.runtime.agent.source.project"
	KeyToolRuntimeAgentSourceUserLabel    Key = "tool.runtime.agent.source.user"
	KeyToolRuntimeAgentSourcePluginLabel  Key = "tool.runtime.agent.source.plugin"
	KeyToolRuntimeAgentSourceManagedLabel Key = "tool.runtime.agent.source.managed"
	KeyToolRuntimeAgentSourceBuiltinLabel Key = "tool.runtime.agent.source.builtin"
	KeyToolRuntimeAgentSourceOtherLabel   Key = "tool.runtime.agent.source.other"

	KeyToolRuntimeBackgroundTaskNotificationTitle     Key = "tool.runtime.background.notification.title"
	KeyToolRuntimeBackgroundTaskNotification          Key = "tool.runtime.background.notification.message"
	KeyToolRuntimeBackgroundTaskNotificationWithLabel Key = "tool.runtime.background.notification.message_labeled"

	KeyToolRuntimeBashInvalidTypedResult          Key = "tool.runtime.bash.invalid_typed_result"
	KeyToolRuntimeBashPlanModeBlocked             Key = "tool.runtime.bash.plan_mode"
	KeyToolRuntimeBackgroundFieldDisabled         Key = "tool.runtime.bash.background_field_disabled"
	KeyToolRuntimeBlockingSleep                   Key = "tool.runtime.bash.blocking_sleep"
	KeyToolRuntimeCommandBlocked                  Key = "tool.runtime.bash.command_blocked"
	KeyToolRuntimeBackgroundUnavailable           Key = "tool.runtime.bash.background_unavailable"
	KeyToolRuntimeBuildBackgroundFailed           Key = "tool.runtime.bash.build_background_failed"
	KeyToolRuntimeStartBackgroundFailed           Key = "tool.runtime.bash.start_background_failed"
	KeyToolRuntimeBuildCommandFailed              Key = "tool.runtime.bash.build_command_failed"
	KeyToolRuntimeSandboxUnavailable              Key = "tool.runtime.bash.sandbox_unavailable"
	KeyToolRuntimeLinesTruncated                  Key = "tool.runtime.bash.lines_truncated"
	KeyToolRuntimeSleepFollowedBy                 Key = "tool.runtime.bash.sleep_followed_by"
	KeyToolRuntimeStandaloneSleep                 Key = "tool.runtime.bash.standalone_sleep"
	KeyToolRuntimeBackgroundResult                Key = "tool.runtime.bash.background_result"
	KeyToolRuntimeCommandTimedOut                 Key = "tool.runtime.bash.command_timed_out"
	KeyToolRuntimeExitCodeLabel                   Key = "tool.runtime.bash.exit_code"
	KeyToolRuntimeCommandAborted                  Key = "tool.runtime.bash.command_aborted"
	KeyToolRuntimeReturnInterrupted               Key = "tool.runtime.bash.return.interrupted"
	KeyToolRuntimeReturnSuccess                   Key = "tool.runtime.bash.return.success"
	KeyToolRuntimeReturnNoStatus                  Key = "tool.runtime.bash.return.no_status"
	KeyToolRuntimeReturnGeneralError              Key = "tool.runtime.bash.return.general_error"
	KeyToolRuntimeReturnBuiltinMisuse             Key = "tool.runtime.bash.return.builtin_misuse"
	KeyToolRuntimeReturnNotExecutable             Key = "tool.runtime.bash.return.not_executable"
	KeyToolRuntimeReturnNotFound                  Key = "tool.runtime.bash.return.not_found"
	KeyToolRuntimeReturnSIGINT                    Key = "tool.runtime.bash.return.sigint"
	KeyToolRuntimeReturnSIGKILL                   Key = "tool.runtime.bash.return.sigkill"
	KeyToolRuntimeReturnSIGSEGV                   Key = "tool.runtime.bash.return.sigsegv"
	KeyToolRuntimeReturnSIGTERM                   Key = "tool.runtime.bash.return.sigterm"
	KeyToolRuntimeReturnSignal                    Key = "tool.runtime.bash.return.signal"
	KeyToolRuntimeReturnFailed                    Key = "tool.runtime.bash.return.failed"
	KeyToolRuntimePowerShellPlanModeBlocked       Key = "tool.runtime.powershell.plan_mode"
	KeyToolRuntimePowerShellDynamicPath           Key = "tool.runtime.powershell.dynamic_path"
	KeyToolRuntimeBuildBackgroundPowerShellFailed Key = "tool.runtime.powershell.build_background_failed"
	KeyToolRuntimeStartBackgroundPowerShellFailed Key = "tool.runtime.powershell.start_background_failed"
	KeyToolRuntimeBuildPowerShellFailed           Key = "tool.runtime.powershell.build_failed"
	KeyToolRuntimeStdoutTruncated                 Key = "tool.runtime.powershell.stdout_truncated"
	KeyToolRuntimeStderrTruncated                 Key = "tool.runtime.powershell.stderr_truncated"
	KeyToolRuntimePowerShellDestructiveWarning    Key = "tool.runtime.powershell.destructive_warning"
	KeyToolRuntimePowerShellSecurityWarning       Key = "tool.runtime.powershell.security_warning"
	KeyToolRuntimePowerShellNotFound              Key = "tool.runtime.powershell.not_found"
)

func init() {
	entries := map[Key][6]string{
		KeyToolRuntimeErrorPrefix: {
			"Error: %s", "错误：%s", "Fehler: %s", "エラー: %s", "오류: %s", "Ошибка: %s"},
		KeyToolRuntimeInvalidInput: {
			"Error: invalid input: %s", "错误：输入无效：%s", "Fehler: Ungültige Eingabe: %s", "エラー: 入力が無効です: %s", "오류: 잘못된 입력: %s", "Ошибка: недопустимые входные данные: %s"},
		KeyToolRuntimeResponseMarshalFailed: {
			"Could not encode tool response: %v", "无法编码工具响应：%v", "Tool-Antwort konnte nicht codiert werden: %v", "ツール応答をエンコードできませんでした: %v", "도구 응답을 인코딩할 수 없습니다: %v", "Не удалось закодировать ответ инструмента: %v"},
		KeyToolRuntimeRequiredFieldMissing: {
			"Required field is missing: %q", "缺少必填字段：%q", "Erforderliches Feld fehlt: %q", "必須フィールドがありません: %q", "필수 필드가 없습니다: %q", "Отсутствует обязательное поле: %q"},
		KeyToolRuntimeFieldStringRequired: {
			"Field %q is missing or is not a string", "字段 %q 缺失或不是字符串", "Feld %q fehlt oder ist keine Zeichenfolge", "フィールド %q がないか、文字列ではありません", "필드 %q이(가) 없거나 문자열이 아닙니다", "Поле %q отсутствует или не является строкой"},
	}
	for key, values := range entries {
		semanticTranslations[key] = map[Language]string{
			LangEN: values[0], LangZH: values[1], LangDE: values[2],
			LangJA: values[3], LangKO: values[4], LangRU: values[5],
		}
	}
}

func init() {
	addToolRuntime(KeyToolRuntimeAgentSourceProjectLabel, "Project agents", "项目 Agent", "Projekt-Agents", "プロジェクト Agent", "프로젝트 Agent", "Agent проекта")
	addToolRuntime(KeyToolRuntimeAgentSourceUserLabel, "User agents", "用户 Agent", "Benutzer-Agents", "ユーザー Agent", "사용자 Agent", "Пользовательские Agent")
	addToolRuntime(KeyToolRuntimeAgentSourcePluginLabel, "Plugin agents", "插件 Agent", "Plugin-Agents", "Plugin Agent", "Plugin Agent", "Agent плагинов")
	addToolRuntime(KeyToolRuntimeAgentSourceManagedLabel, "Managed agents", "托管 Agent", "Verwaltete Agents", "管理対象 Agent", "관리형 Agent", "Управляемые Agent")
	addToolRuntime(KeyToolRuntimeAgentSourceBuiltinLabel, "Built-in agents", "内置 Agent", "Integrierte Agents", "組み込み Agent", "내장 Agent", "Встроенные Agent")
	addToolRuntime(KeyToolRuntimeAgentSourceOtherLabel, "%s agents", "%s Agent", "%s-Agents", "%s Agent", "%s Agent", "Agent: %s")

	addToolRuntime(KeyToolRuntimeBackgroundTaskNotificationTitle, "Background task %s", "后台任务 %s", "Hintergrundaufgabe %s", "バックグラウンドタスク %s", "백그라운드 작업 %s", "Фоновая задача: %s")
	addToolRuntime(KeyToolRuntimeBackgroundTaskNotification, "task %s (%s) %s with exit=%d", "任务 %s（%s）状态为 %s，exit=%d", "Aufgabe %s (%s) ist %s, exit=%d", "タスク %s（%s）は %s、exit=%d", "작업 %s(%s)의 상태가 %s입니다. exit=%d", "Задача %s (%s): %s, exit=%d")
	addToolRuntime(KeyToolRuntimeBackgroundTaskNotificationWithLabel, "%s: task %s (%s) %s with exit=%d", "%s：任务 %s（%s）状态为 %s，exit=%d", "%s: Aufgabe %s (%s) ist %s, exit=%d", "%s: タスク %s（%s）は %s、exit=%d", "%s: 작업 %s(%s)의 상태가 %s입니다. exit=%d", "%s: задача %s (%s): %s, exit=%d")

	addToolRuntime(KeyToolRuntimeBashInvalidTypedResult, "Bash returned an invalid typed result", "Bash 返回了无效的类型化结果", "Bash hat ein ungültiges typisiertes Ergebnis zurückgegeben", "Bash が無効な型付き結果を返しました", "Bash가 잘못된 형식의 결과를 반환했습니다", "Bash вернул недопустимый типизированный результат")
	addToolRuntime(KeyToolRuntimeBashPlanModeBlocked, "cannot use Bash in plan mode — exit plan mode first", "计划模式下无法使用 Bash，请先退出计划模式", "Bash kann im Planungsmodus nicht verwendet werden — verlasse zuerst den Planungsmodus", "plan mode では Bash を使用できません。先に plan mode を終了してください", "plan mode에서는 Bash를 사용할 수 없습니다. 먼저 plan mode를 종료하세요", "Bash нельзя использовать в режиме планирования — сначала выйдите из него")
	addToolRuntime(KeyToolRuntimeBackgroundFieldDisabled, "invalid input: unknown field %q while background tasks are disabled", "输入无效：后台任务已禁用，不能使用未知字段 %q", "Ungültige Eingabe: Unbekanntes Feld %q bei deaktivierten Hintergrundaufgaben", "入力が無効です: バックグラウンドタスクが無効なため、不明なフィールド %q は使用できません", "잘못된 입력: 백그라운드 작업이 비활성화된 상태에서 알 수 없는 필드 %q을(를) 사용할 수 없습니다", "Недопустимые входные данные: неизвестное поле %q при отключённых фоновых задачах")
	addToolRuntime(KeyToolRuntimeBlockingSleep, "Blocked: %s. Run blocking commands in the background with run_in_background: true — you'll get a completion notification when done. For streaming events (watching logs, polling APIs), use the Monitor tool. If you genuinely need a delay (rate limiting, deliberate pacing), keep it under 2 seconds.", "已阻止：%s。请使用 run_in_background: true 在后台运行阻塞命令；完成后会收到通知。对于流式事件（查看日志、轮询 API），请使用 Monitor 工具。如果确实需要延迟（限流或有意控制节奏），请控制在 2 秒以内。", "Blockiert: %s. Führe blockierende Befehle mit run_in_background: true im Hintergrund aus; nach Abschluss erhältst du eine Benachrichtigung. Verwende für Streaming-Ereignisse (Logs beobachten, APIs abfragen) das Monitor-Tool. Falls eine Verzögerung wirklich nötig ist, halte sie unter 2 Sekunden.", "ブロックしました: %s。ブロックするコマンドは run_in_background: true でバックグラウンド実行してください。完了時に通知されます。ストリーミングイベント（ログ監視、API ポーリング）には Monitor ツールを使用してください。待機が本当に必要な場合は 2 秒未満にしてください。", "차단됨: %s. 차단 명령은 run_in_background: true로 백그라운드에서 실행하세요. 완료되면 알림을 받습니다. 스트리밍 이벤트(로그 확인, API 폴링)에는 Monitor 도구를 사용하세요. 지연이 꼭 필요하다면 2초 미만으로 유지하세요.", "Заблокировано: %s. Запускайте блокирующие команды в фоне с run_in_background: true; по завершении придёт уведомление. Для потоковых событий (просмотр логов, опрос API) используйте Monitor. Если задержка действительно нужна, ограничьте её двумя секундами.")
	addToolRuntime(KeyToolRuntimeCommandBlocked, "command blocked: %s", "命令已被阻止：%s", "Befehl blockiert: %s", "コマンドをブロックしました: %s", "명령이 차단되었습니다: %s", "Команда заблокирована: %s")
	addToolRuntime(KeyToolRuntimeBackgroundUnavailable, "run_in_background is not available in this runtime", "当前 runtime 不支持 run_in_background", "run_in_background ist in dieser Runtime nicht verfügbar", "この runtime では run_in_background を使用できません", "이 runtime에서는 run_in_background를 사용할 수 없습니다", "run_in_background недоступен в этой runtime")
	addToolRuntime(KeyToolRuntimeBuildBackgroundFailed, "failed to build background command: %v", "无法构建后台命令：%v", "Hintergrundbefehl konnte nicht erstellt werden: %v", "バックグラウンドコマンドを構築できませんでした: %v", "백그라운드 명령을 구성하지 못했습니다: %v", "Не удалось подготовить фоновую команду: %v")
	addToolRuntime(KeyToolRuntimeStartBackgroundFailed, "failed to start background task: %v", "无法启动后台任务：%v", "Hintergrundaufgabe konnte nicht gestartet werden: %v", "バックグラウンドタスクを開始できませんでした: %v", "백그라운드 작업을 시작하지 못했습니다: %v", "Не удалось запустить фоновую задачу: %v")
	addToolRuntime(KeyToolRuntimeBuildCommandFailed, "failed to build command: %v", "无法构建命令：%v", "Befehl konnte nicht erstellt werden: %v", "コマンドを構築できませんでした: %v", "명령을 구성하지 못했습니다: %v", "Не удалось подготовить команду: %v")
	addToolRuntime(KeyToolRuntimeSandboxUnavailable, "filesystem sandbox is unavailable for this isolated shell", "此隔离 shell 无法使用 filesystem sandbox", "Für diese isolierte Shell ist keine Dateisystem-Sandbox verfügbar", "この隔離 shell では filesystem sandbox を使用できません", "이 격리 shell에서는 filesystem sandbox를 사용할 수 없습니다", "Для этой изолированной shell недоступна filesystem sandbox")
	addToolRuntime(KeyToolRuntimeLinesTruncated, "... [%d lines truncated] ...", "…［已截断 %d 行］…", "... [%d Zeilen gekürzt] ...", "…［%d 行を省略］…", "... [%d줄 잘림] ...", "... [усечено строк: %d] ...")
	addToolRuntime(KeyToolRuntimeSleepFollowedBy, "sleep %d followed by: %s", "sleep %d 后接：%s", "Auf sleep %d folgt: %s", "sleep %d の後に続く処理: %s", "sleep %d 다음에 실행됨: %s", "После sleep %d выполняется: %s")
	addToolRuntime(KeyToolRuntimeStandaloneSleep, "standalone sleep %d", "单独的 sleep %d", "Alleinstehendes sleep %d", "単独の sleep %d", "단독 sleep %d", "Отдельный sleep %d")
	addToolRuntime(KeyToolRuntimeBackgroundResult, "Command running in background with ID: %s. Output is being written to: %s", "命令正在后台运行，ID：%s。输出将写入：%s", "Befehl läuft im Hintergrund mit ID %s. Ausgabe wird geschrieben nach: %s", "コマンドは ID %s でバックグラウンド実行中です。出力先: %s", "명령이 ID %s로 백그라운드에서 실행 중입니다. 출력 위치: %s", "Команда выполняется в фоне с ID %s. Вывод записывается в: %s")
	addToolRuntime(KeyToolRuntimeCommandTimedOut, "Command timed out", "命令超时", "Zeitüberschreitung beim Befehl", "コマンドがタイムアウトしました", "명령 시간이 초과되었습니다", "Превышено время ожидания команды")
	addToolRuntime(KeyToolRuntimeExitCodeLabel, "Exit code %d", "退出码 %d", "Exit-Code %d", "終了コード %d", "종료 코드 %d", "Код выхода %d")
	addToolRuntime(KeyToolRuntimeCommandAborted, "<error>Command was aborted before completion</error>", "<error>命令在完成前已中止</error>", "<error>Befehl wurde vor Abschluss abgebrochen</error>", "<error>コマンドは完了前に中止されました</error>", "<error>명령이 완료되기 전에 중단되었습니다</error>", "<error>Команда была прервана до завершения</error>")
	addToolRuntime(KeyToolRuntimeReturnInterrupted, "command interrupted (timeout or signal)", "命令已中断（超时或收到 signal）", "Befehl unterbrochen (Zeitüberschreitung oder Signal)", "コマンドが中断されました（タイムアウトまたは signal）", "명령이 중단되었습니다(시간 초과 또는 signal)", "Команда прервана (тайм-аут или signal)")
	addToolRuntime(KeyToolRuntimeReturnSuccess, "success", "成功", "Erfolgreich", "成功", "성공", "Успешно")
	addToolRuntime(KeyToolRuntimeReturnNoStatus, "process did not start or was killed before reporting an exit status", "进程未启动，或在报告退出状态前已被终止", "Prozess wurde nicht gestartet oder vor Ausgabe eines Exit-Status beendet", "プロセスが開始されなかったか、終了状態を返す前に強制終了されました", "프로세스가 시작되지 않았거나 종료 상태를 보고하기 전에 종료되었습니다", "Процесс не запустился или был завершён до получения статуса выхода")
	addToolRuntime(KeyToolRuntimeReturnGeneralError, "exit code 1: general error", "退出码 1：常规错误", "Exit-Code 1: allgemeiner Fehler", "終了コード 1: 一般エラー", "종료 코드 1: 일반 오류", "Код выхода 1: общая ошибка")
	addToolRuntime(KeyToolRuntimeReturnBuiltinMisuse, "exit code 2: misuse of shell builtins", "退出码 2：shell builtin 使用错误", "Exit-Code 2: fehlerhafte Verwendung von Shell-Built-ins", "終了コード 2: shell builtin の誤用", "종료 코드 2: shell builtin 오용", "Код выхода 2: неверное использование встроенных команд shell")
	addToolRuntime(KeyToolRuntimeReturnNotExecutable, "exit code 126: command invoked but not executable", "退出码 126：已调用命令，但不可执行", "Exit-Code 126: Befehl aufgerufen, aber nicht ausführbar", "終了コード 126: コマンドは呼び出されましたが実行できません", "종료 코드 126: 명령이 호출되었지만 실행할 수 없습니다", "Код выхода 126: команда вызвана, но не исполняема")
	addToolRuntime(KeyToolRuntimeReturnNotFound, "exit code 127: command not found", "退出码 127：找不到命令", "Exit-Code 127: Befehl nicht gefunden", "終了コード 127: コマンドが見つかりません", "종료 코드 127: 명령을 찾을 수 없습니다", "Код выхода 127: команда не найдена")
	addToolRuntime(KeyToolRuntimeReturnSIGINT, "exit code 130: terminated by SIGINT (Ctrl+C)", "退出码 130：被 SIGINT（Ctrl+C）终止", "Exit-Code 130: durch SIGINT (Ctrl+C) beendet", "終了コード 130: SIGINT（Ctrl+C）で終了", "종료 코드 130: SIGINT(Ctrl+C)로 종료됨", "Код выхода 130: завершено сигналом SIGINT (Ctrl+C)")
	addToolRuntime(KeyToolRuntimeReturnSIGKILL, "exit code 137: terminated by SIGKILL", "退出码 137：被 SIGKILL 终止", "Exit-Code 137: durch SIGKILL beendet", "終了コード 137: SIGKILL で終了", "종료 코드 137: SIGKILL로 종료됨", "Код выхода 137: завершено сигналом SIGKILL")
	addToolRuntime(KeyToolRuntimeReturnSIGSEGV, "exit code 139: terminated by SIGSEGV", "退出码 139：被 SIGSEGV 终止", "Exit-Code 139: durch SIGSEGV beendet", "終了コード 139: SIGSEGV で終了", "종료 코드 139: SIGSEGV로 종료됨", "Код выхода 139: завершено сигналом SIGSEGV")
	addToolRuntime(KeyToolRuntimeReturnSIGTERM, "exit code 143: terminated by SIGTERM", "退出码 143：被 SIGTERM 终止", "Exit-Code 143: durch SIGTERM beendet", "終了コード 143: SIGTERM で終了", "종료 코드 143: SIGTERM으로 종료됨", "Код выхода 143: завершено сигналом SIGTERM")
	addToolRuntime(KeyToolRuntimeReturnSignal, "exit code %d: terminated by signal %d", "退出码 %d：被 signal %d 终止", "Exit-Code %d: durch Signal %d beendet", "終了コード %d: signal %d で終了", "종료 코드 %d: signal %d로 종료됨", "Код выхода %d: завершено сигналом %d")
	addToolRuntime(KeyToolRuntimeReturnFailed, "exit code %d: command failed", "退出码 %d：命令执行失败", "Exit-Code %d: Befehl fehlgeschlagen", "終了コード %d: コマンドが失敗しました", "종료 코드 %d: 명령이 실패했습니다", "Код выхода %d: команда завершилась с ошибкой")

	addToolRuntime(KeyToolRuntimePowerShellPlanModeBlocked, "cannot use PowerShell in plan mode — exit plan mode first", "计划模式下无法使用 PowerShell，请先退出计划模式", "PowerShell kann im Planungsmodus nicht verwendet werden — verlasse zuerst den Planungsmodus", "plan mode では PowerShell を使用できません。先に plan mode を終了してください", "plan mode에서는 PowerShell을 사용할 수 없습니다. 먼저 plan mode를 종료하세요", "PowerShell нельзя использовать в режиме планирования — сначала выйдите из него")
	addToolRuntime(KeyToolRuntimePowerShellDynamicPath, "dynamic PowerShell path %q cannot be verified against allowed directories", "无法根据允许目录验证动态 PowerShell 路径 %q", "Der dynamische PowerShell-Pfad %q kann nicht gegen zulässige Verzeichnisse geprüft werden", "動的な PowerShell パス %q を許可済みディレクトリと照合できません", "동적 PowerShell 경로 %q을(를) 허용된 디렉터리와 대조할 수 없습니다", "Динамический путь PowerShell %q нельзя сверить с разрешёнными каталогами")
	addToolRuntime(KeyToolRuntimeBuildBackgroundPowerShellFailed, "failed to build background PowerShell command: %v", "无法构建后台 PowerShell 命令：%v", "PowerShell-Hintergrundbefehl konnte nicht erstellt werden: %v", "バックグラウンド PowerShell コマンドを構築できませんでした: %v", "백그라운드 PowerShell 명령을 구성하지 못했습니다: %v", "Не удалось подготовить фоновую команду PowerShell: %v")
	addToolRuntime(KeyToolRuntimeStartBackgroundPowerShellFailed, "failed to start background PowerShell task: %v", "无法启动后台 PowerShell 任务：%v", "PowerShell-Hintergrundaufgabe konnte nicht gestartet werden: %v", "バックグラウンド PowerShell タスクを開始できませんでした: %v", "백그라운드 PowerShell 작업을 시작하지 못했습니다: %v", "Не удалось запустить фоновую задачу PowerShell: %v")
	addToolRuntime(KeyToolRuntimeBuildPowerShellFailed, "failed to build PowerShell command: %v", "无法构建 PowerShell 命令：%v", "PowerShell-Befehl konnte nicht erstellt werden: %v", "PowerShell コマンドを構築できませんでした: %v", "PowerShell 명령을 구성하지 못했습니다: %v", "Не удалось подготовить команду PowerShell: %v")
	addToolRuntime(KeyToolRuntimeStdoutTruncated, "... [stdout truncated]", "…［stdout 已截断］", "... [stdout gekürzt]", "…［stdout を省略］", "... [stdout 잘림]", "... [stdout усечён]")
	addToolRuntime(KeyToolRuntimeStderrTruncated, "... [stderr truncated]", "…［stderr 已截断］", "... [stderr gekürzt]", "…［stderr を省略］", "... [stderr 잘림]", "... [stderr усечён]")
	addToolRuntime(KeyToolRuntimePowerShellDestructiveWarning, "PowerShell command can irreversibly remove or reset state; confirm the target", "PowerShell 命令可能不可逆地删除或重置状态，请确认目标", "Der PowerShell-Befehl kann Zustand unwiderruflich löschen oder zurücksetzen; bestätige das Ziel", "PowerShell コマンドは状態を不可逆に削除またはリセットする可能性があります。対象を確認してください", "PowerShell 명령이 상태를 되돌릴 수 없게 삭제하거나 재설정할 수 있습니다. 대상을 확인하세요", "Команда PowerShell может необратимо удалить или сбросить состояние; подтвердите цель")
	addToolRuntime(KeyToolRuntimePowerShellSecurityWarning, "PowerShell command uses dynamic, encoded, or elevated execution", "PowerShell 命令使用了动态、编码或提权执行", "Der PowerShell-Befehl verwendet dynamische, codierte oder erhöhte Ausführung", "PowerShell コマンドは動的、エンコード済み、または昇格実行を使用します", "PowerShell 명령이 동적, 인코딩 또는 권한 상승 실행을 사용합니다", "Команда PowerShell использует динамическое, кодированное или привилегированное выполнение")
	addToolRuntime(KeyToolRuntimePowerShellNotFound, "PowerShell executable not found in PATH", "在 PATH 中找不到 PowerShell 可执行文件", "PowerShell-Programm wurde in PATH nicht gefunden", "PATH に PowerShell 実行ファイルが見つかりません", "PATH에서 PowerShell 실행 파일을 찾을 수 없습니다", "Исполняемый файл PowerShell не найден в PATH")
}

func addToolRuntime(key Key, en, zh, de, ja, ko, ru string) {
	semanticTranslations[key] = map[Language]string{
		LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
	}
}

const (
	KeyToolRuntimeDestructiveRmRecursiveForce        Key = "tool.runtime.safety.rm_recursive_force"
	KeyToolRuntimeDestructiveRmTopLevel              Key = "tool.runtime.safety.rm_top_level"
	KeyToolRuntimeDestructiveDd                      Key = "tool.runtime.safety.dd"
	KeyToolRuntimeDestructiveMkfs                    Key = "tool.runtime.safety.mkfs"
	KeyToolRuntimeDestructiveShred                   Key = "tool.runtime.safety.shred"
	KeyToolRuntimeDestructiveWipe                    Key = "tool.runtime.safety.wipe"
	KeyToolRuntimeDestructiveRawDeviceRedirect       Key = "tool.runtime.safety.raw_device_redirect"
	KeyToolRuntimeDestructiveFindExecRm              Key = "tool.runtime.safety.find_exec_rm"
	KeyToolRuntimeDestructiveFindDelete              Key = "tool.runtime.safety.find_delete"
	KeyToolRuntimeDestructiveGitPushForce            Key = "tool.runtime.safety.git_push_force"
	KeyToolRuntimeDestructiveGitPushForceLease       Key = "tool.runtime.safety.git_push_force_lease"
	KeyToolRuntimeDestructiveGitResetHard            Key = "tool.runtime.safety.git_reset_hard"
	KeyToolRuntimeDestructiveGitCleanForce           Key = "tool.runtime.safety.git_clean_force"
	KeyToolRuntimeDestructiveGitBranchForceDelete    Key = "tool.runtime.safety.git_branch_force_delete"
	KeyToolRuntimeDestructiveSQLDrop                 Key = "tool.runtime.safety.sql_drop"
	KeyToolRuntimeDestructiveSQLTruncate             Key = "tool.runtime.safety.sql_truncate"
	KeyToolRuntimeDestructiveSQLDelete               Key = "tool.runtime.safety.sql_delete"
	KeyToolRuntimeDestructiveKubectlNamespace        Key = "tool.runtime.safety.kubectl_namespace"
	KeyToolRuntimeDestructiveKubectlAll              Key = "tool.runtime.safety.kubectl_all"
	KeyToolRuntimeDestructiveKubectlPersistentVolume Key = "tool.runtime.safety.kubectl_persistent_volume"
	KeyToolRuntimeDestructiveTerraformDestroy        Key = "tool.runtime.safety.terraform_destroy"
	KeyToolRuntimeDestructiveHelmUninstall           Key = "tool.runtime.safety.helm_uninstall"
	KeyToolRuntimeDestructiveRmRecursive             Key = "tool.runtime.safety.rm_recursive"
	KeyToolRuntimeDestructiveRmdirRecursive          Key = "tool.runtime.safety.rmdir_recursive"
	KeyToolRuntimeDestructiveDdPath                  Key = "tool.runtime.safety.dd_path"

	KeyToolRuntimeResolvePathFailed       Key = "tool.runtime.file.resolve_path_failed"
	KeyToolRuntimeStatFileFailed          Key = "tool.runtime.file.stat_failed"
	KeyToolRuntimeFileMissing             Key = "tool.runtime.file.missing"
	KeyToolRuntimePathIsDirectory         Key = "tool.runtime.file.path_is_directory"
	KeyToolRuntimeFileNotRead             Key = "tool.runtime.file.not_read"
	KeyToolRuntimeFilePartiallyRead       Key = "tool.runtime.file.partially_read"
	KeyToolRuntimeFileViewTransformed     Key = "tool.runtime.file.view_transformed"
	KeyToolRuntimeEditRangeNotObserved    Key = "tool.runtime.edit.range_not_observed"
	KeyToolRuntimeReadFileFailed          Key = "tool.runtime.file.read_failed"
	KeyToolRuntimeFileChangedSinceRead    Key = "tool.runtime.file.changed_since_read"
	KeyToolRuntimeBashSedReadRequired     Key = "tool.runtime.file.bash_sed_read_required"
	KeyToolRuntimeWriteFileFailed         Key = "tool.runtime.file.write_failed"
	KeyToolRuntimeCreateDirectoryFailed   Key = "tool.runtime.file.create_directory_failed"
	KeyToolRuntimeFileUpdated             Key = "tool.runtime.file.updated"
	KeyToolRuntimeFileCreated             Key = "tool.runtime.file.created"
	KeyToolRuntimeWriteInvalidData        Key = "tool.runtime.file.write_invalid_data"
	KeyToolRuntimeEditPlanMode            Key = "tool.runtime.edit.plan_mode"
	KeyToolRuntimeEditNoChanges           Key = "tool.runtime.edit.no_changes"
	KeyToolRuntimeEditNotebook            Key = "tool.runtime.edit.notebook"
	KeyToolRuntimeEditTeamMemorySecret    Key = "tool.runtime.edit.team_memory_secret"
	KeyToolRuntimeEditSymlink             Key = "tool.runtime.edit.symlink"
	KeyToolRuntimeEditFileTooLarge        Key = "tool.runtime.edit.file_too_large"
	KeyToolRuntimeEditTargetAppeared      Key = "tool.runtime.edit.target_appeared"
	KeyToolRuntimeEditRecheckTargetFailed Key = "tool.runtime.edit.recheck_target_failed"
	KeyToolRuntimeEditStringMissing       Key = "tool.runtime.edit.string_missing"
	KeyToolRuntimeEditAmbiguousMatch      Key = "tool.runtime.edit.ambiguous_match"
	KeyToolRuntimeEditCreateExisting      Key = "tool.runtime.edit.create_existing"
	KeyToolRuntimeEditSummaryReplaceAll   Key = "tool.runtime.edit.summary_replace_all"
	KeyToolRuntimeEditSummary             Key = "tool.runtime.edit.summary"
	KeyToolRuntimeEditInvalidData         Key = "tool.runtime.edit.invalid_data"
	KeyToolRuntimeEditDidYouMean          Key = "tool.runtime.edit.did_you_mean"

	KeyToolRuntimeNotebookUnknownEditMode        Key = "tool.runtime.notebook.unknown_edit_mode"
	KeyToolRuntimeNotebookInvalidData            Key = "tool.runtime.notebook.invalid_data"
	KeyToolRuntimeNotebookCellUpdated            Key = "tool.runtime.notebook.cell_updated"
	KeyToolRuntimeNotebookCellInserted           Key = "tool.runtime.notebook.cell_inserted"
	KeyToolRuntimeNotebookCellDeleted            Key = "tool.runtime.notebook.cell_deleted"
	KeyToolRuntimeNotebookResolvePathFailed      Key = "tool.runtime.notebook.resolve_path_failed"
	KeyToolRuntimeNotebookFileRequired           Key = "tool.runtime.notebook.file_required"
	KeyToolRuntimeNotebookCellTypeInvalid        Key = "tool.runtime.notebook.cell_type_invalid"
	KeyToolRuntimeNotebookEditModeInvalid        Key = "tool.runtime.notebook.edit_mode_invalid"
	KeyToolRuntimeNotebookInsertCellTypeRequired Key = "tool.runtime.notebook.insert_cell_type_required"
	KeyToolRuntimeNotebookCellIDRequired         Key = "tool.runtime.notebook.cell_id_required"
	KeyToolRuntimeNotebookNotRead                Key = "tool.runtime.notebook.not_read"
	KeyToolRuntimeNotebookNotFound               Key = "tool.runtime.notebook.not_found"
	KeyToolRuntimeNotebookChangedSinceRead       Key = "tool.runtime.notebook.changed_since_read"
	KeyToolRuntimeNotebookReadFailed             Key = "tool.runtime.notebook.read_failed"
	KeyToolRuntimeNotebookInvalidJSON            Key = "tool.runtime.notebook.invalid_json"
	KeyToolRuntimeNotebookSerializeFailed        Key = "tool.runtime.notebook.serialize_failed"
	KeyToolRuntimeNotebookWriteFailed            Key = "tool.runtime.notebook.write_failed"
)

func init() {
	addToolRuntime(KeyToolRuntimeDestructiveRmRecursiveForce, "rm -rf recursively deletes files; confirm the target path", "rm -rf 会递归删除文件，请确认目标路径", "rm -rf löscht Dateien rekursiv; Zielpfad bestätigen", "rm -rf はファイルを再帰的に削除します。対象パスを確認してください", "rm -rf는 파일을 재귀적으로 삭제합니다. 대상 경로를 확인하세요", "rm -rf рекурсивно удаляет файлы; проверьте целевой путь")
	addToolRuntime(KeyToolRuntimeDestructiveRmTopLevel, "rm -r targets a top-level path; confirm before proceeding", "rm -r 的目标是顶层路径，请确认后再继续", "rm -r zielt auf einen obersten Pfad; vor dem Fortfahren bestätigen", "rm -r の対象が最上位パスです。続行前に確認してください", "rm -r의 대상이 최상위 경로입니다. 계속하기 전에 확인하세요", "rm -r нацелен на корневой путь; подтвердите перед продолжением")
	addToolRuntime(KeyToolRuntimeDestructiveDd, "dd writes raw bytes; confirm the of= path", "dd 会写入原始字节，请确认 of= 路径", "dd schreibt Rohdaten; den Pfad in of= bestätigen", "dd は生のバイト列を書き込みます。of= のパスを確認してください", "dd는 원시 바이트를 씁니다. of= 경로를 확인하세요", "dd записывает необработанные байты; проверьте путь of=")
	addToolRuntime(KeyToolRuntimeDestructiveMkfs, "mkfs formats a filesystem and erases its contents", "mkfs 会格式化文件系统并清除其中的内容", "mkfs formatiert ein Dateisystem und löscht dessen Inhalt", "mkfs はファイルシステムをフォーマットし、内容を消去します", "mkfs는 파일 시스템을 포맷하고 내용을 지웁니다", "mkfs форматирует файловую систему и стирает её содержимое")
	addToolRuntime(KeyToolRuntimeDestructiveShred, "shred irreversibly overwrites a file", "shred 会不可逆地覆写文件内容", "shred überschreibt eine Datei unwiderruflich", "shred はファイルを不可逆的に上書きします", "shred는 파일을 되돌릴 수 없게 덮어씁니다", "shred необратимо перезаписывает файл")
	addToolRuntime(KeyToolRuntimeDestructiveWipe, "wipe permanently deletes data; confirm the target", "wipe 会永久删除数据，请确认目标", "wipe löscht Daten dauerhaft; Ziel bestätigen", "wipe はデータを完全に削除します。対象を確認してください", "wipe는 데이터를 영구 삭제합니다. 대상을 확인하세요", "wipe безвозвратно удаляет данные; проверьте цель")
	addToolRuntime(KeyToolRuntimeDestructiveRawDeviceRedirect, "redirecting to a raw block device overwrites the disk", "重定向到原始块设备会覆写磁盘", "Umleitung auf ein Blockgerät überschreibt den Datenträger", "ブロックデバイスへのリダイレクトはディスクを上書きします", "원시 블록 장치로 리디렉션하면 디스크를 덮어씁니다", "Перенаправление на блочное устройство перезапишет диск")
	addToolRuntime(KeyToolRuntimeDestructiveFindExecRm, "find -exec rm removes every match; confirm the predicate", "find -exec rm 会删除所有匹配项，请确认筛选条件", "find -exec rm entfernt alle Treffer; Prädikat bestätigen", "find -exec rm は一致した項目をすべて削除します。条件を確認してください", "find -exec rm은 일치하는 모든 항목을 삭제합니다. 조건을 확인하세요", "find -exec rm удаляет все совпадения; проверьте условие")
	addToolRuntime(KeyToolRuntimeDestructiveFindDelete, "find -delete removes every match; confirm the predicate", "find -delete 会删除所有匹配项，请确认筛选条件", "find -delete entfernt alle Treffer; Prädikat bestätigen", "find -delete は一致した項目をすべて削除します。条件を確認してください", "find -delete는 일치하는 모든 항목을 삭제합니다. 조건을 확인하세요", "find -delete удаляет все совпадения; проверьте условие")
	addToolRuntime(KeyToolRuntimeDestructiveGitPushForce, "git push --force overwrites remote history; confirm the branch", "git push --force 会覆写远端历史，请确认分支", "git push --force überschreibt den Remote-Verlauf; Branch bestätigen", "git push --force はリモート履歴を上書きします。ブランチを確認してください", "git push --force는 원격 기록을 덮어씁니다. 브랜치를 확인하세요", "git push --force перезаписывает удалённую историю; проверьте ветку")
	addToolRuntime(KeyToolRuntimeDestructiveGitPushForceLease, "git push --force-with-lease may rewrite remote history", "git push --force-with-lease 可能会重写远端历史", "git push --force-with-lease kann den Remote-Verlauf umschreiben", "git push --force-with-lease はリモート履歴を書き換える可能性があります", "git push --force-with-lease는 원격 기록을 다시 쓸 수 있습니다", "git push --force-with-lease может переписать удалённую историю")
	addToolRuntime(KeyToolRuntimeDestructiveGitResetHard, "git reset --hard irreversibly discards local changes", "git reset --hard 会不可逆地丢弃本地更改", "git reset --hard verwirft lokale Änderungen unwiderruflich", "git reset --hard はローカル変更を不可逆的に破棄します", "git reset --hard는 로컬 변경 사항을 되돌릴 수 없게 버립니다", "git reset --hard необратимо отбрасывает локальные изменения")
	addToolRuntime(KeyToolRuntimeDestructiveGitCleanForce, "git clean -f deletes untracked files", "git clean -f 会删除未跟踪文件", "git clean -f löscht nicht verfolgte Dateien", "git clean -f は未追跡ファイルを削除します", "git clean -f는 추적되지 않은 파일을 삭제합니다", "git clean -f удаляет неотслеживаемые файлы")
	addToolRuntime(KeyToolRuntimeDestructiveGitBranchForceDelete, "git branch -D force-deletes a branch", "git branch -D 会强制删除分支", "git branch -D löscht einen Branch zwangsweise", "git branch -D はブランチを強制削除します", "git branch -D는 브랜치를 강제로 삭제합니다", "git branch -D принудительно удаляет ветку")
	addToolRuntime(KeyToolRuntimeDestructiveSQLDrop, "SQL DROP irreversibly removes the object; confirm the target", "SQL DROP 会不可逆地删除对象，请确认目标", "SQL DROP entfernt das Objekt unwiderruflich; Ziel bestätigen", "SQL DROP はオブジェクトを不可逆的に削除します。対象を確認してください", "SQL DROP은 객체를 되돌릴 수 없게 삭제합니다. 대상을 확인하세요", "SQL DROP необратимо удаляет объект; проверьте цель")
	addToolRuntime(KeyToolRuntimeDestructiveSQLTruncate, "SQL TRUNCATE TABLE irreversibly empties a table", "SQL TRUNCATE TABLE 会不可逆地清空表", "SQL TRUNCATE TABLE leert eine Tabelle unwiderruflich", "SQL TRUNCATE TABLE はテーブルを不可逆的に空にします", "SQL TRUNCATE TABLE은 테이블을 되돌릴 수 없게 비웁니다", "SQL TRUNCATE TABLE необратимо очищает таблицу")
	addToolRuntime(KeyToolRuntimeDestructiveSQLDelete, "SQL DELETE FROM changes rows; confirm the predicate", "SQL DELETE FROM 会更改行，请确认筛选条件", "SQL DELETE FROM ändert Zeilen; Prädikat bestätigen", "SQL DELETE FROM は行を変更します。条件を確認してください", "SQL DELETE FROM은 행을 변경합니다. 조건을 확인하세요", "SQL DELETE FROM изменяет строки; проверьте условие")
	addToolRuntime(KeyToolRuntimeDestructiveKubectlNamespace, "kubectl delete namespace cascades to every resource in the namespace", "kubectl delete namespace 会级联删除命名空间中的所有资源", "kubectl delete namespace löscht kaskadierend alle Ressourcen im Namespace", "kubectl delete namespace は namespace 内の全リソースを連鎖削除します", "kubectl delete namespace는 namespace의 모든 리소스를 연쇄 삭제합니다", "kubectl delete namespace каскадно удаляет все ресурсы в namespace")
	addToolRuntime(KeyToolRuntimeDestructiveKubectlAll, "kubectl delete --all removes every resource of that kind", "kubectl delete --all 会删除该类型的所有资源", "kubectl delete --all entfernt alle Ressourcen dieses Typs", "kubectl delete --all はその種類のリソースをすべて削除します", "kubectl delete --all은 해당 종류의 모든 리소스를 삭제합니다", "kubectl delete --all удаляет все ресурсы этого типа")
	addToolRuntime(KeyToolRuntimeDestructiveKubectlPersistentVolume, "kubectl delete persistent volume removes storage", "kubectl delete persistent volume 会删除存储", "kubectl delete persistent volume entfernt Speicher", "kubectl delete persistent volume はストレージを削除します", "kubectl delete persistent volume은 스토리지를 삭제합니다", "kubectl delete persistent volume удаляет хранилище")
	addToolRuntime(KeyToolRuntimeDestructiveTerraformDestroy, "terraform destroy tears down managed infrastructure", "terraform destroy 会销毁受管基础设施", "terraform destroy baut die verwaltete Infrastruktur ab", "terraform destroy は管理対象インフラを破棄します", "terraform destroy는 관리 중인 인프라를 제거합니다", "terraform destroy уничтожает управляемую инфраструктуру")
	addToolRuntime(KeyToolRuntimeDestructiveHelmUninstall, "helm uninstall removes the release, including persistent volumes", "helm uninstall 会移除 release，包括持久卷", "helm uninstall entfernt das Release einschließlich persistenter Volumes", "helm uninstall は永続ボリュームを含む release を削除します", "helm uninstall은 영구 볼륨을 포함한 release를 제거합니다", "helm uninstall удаляет release, включая постоянные тома")
	addToolRuntime(KeyToolRuntimeDestructiveRmRecursive, "rm -r recursively deletes files; confirm the target", "rm -r 会递归删除文件，请确认目标", "rm -r löscht Dateien rekursiv; Ziel bestätigen", "rm -r はファイルを再帰的に削除します。対象を確認してください", "rm -r은 파일을 재귀적으로 삭제합니다. 대상을 확인하세요", "rm -r рекурсивно удаляет файлы; проверьте цель")
	addToolRuntime(KeyToolRuntimeDestructiveRmdirRecursive, "rmdir recursively removes directories; confirm the target", "rmdir 会递归删除目录，请确认目标", "rmdir entfernt Verzeichnisse rekursiv; Ziel bestätigen", "rmdir はディレクトリを再帰的に削除します。対象を確認してください", "rmdir은 디렉터리를 재귀적으로 삭제합니다. 대상을 확인하세요", "rmdir рекурсивно удаляет каталоги; проверьте цель")
	addToolRuntime(KeyToolRuntimeDestructiveDdPath, "dd writes to %s; confirm before proceeding", "dd 将写入 %s，请确认后再继续", "dd schreibt nach %s; vor dem Fortfahren bestätigen", "dd は %s に書き込みます。続行前に確認してください", "dd가 %s에 씁니다. 계속하기 전에 확인하세요", "dd записывает в %s; подтвердите перед продолжением")

	addToolRuntime(KeyToolRuntimeResolvePathFailed, "Could not resolve path: %v", "无法解析路径：%v", "Pfad konnte nicht aufgelöst werden: %v", "パスを解決できませんでした: %v", "경로를 확인할 수 없습니다: %v", "Не удалось определить путь: %v")
	addToolRuntime(KeyToolRuntimeStatFileFailed, "Could not inspect file: %v", "无法检查文件：%v", "Datei konnte nicht geprüft werden: %v", "ファイルを確認できませんでした: %v", "파일을 확인할 수 없습니다: %v", "Не удалось проверить файл: %v")
	addToolRuntime(KeyToolRuntimeFileMissing, "File does not exist. The current working directory is %s.%s", "文件不存在。当前工作目录为 %s。%s", "Datei ist nicht vorhanden. Das aktuelle Arbeitsverzeichnis ist %s.%s", "ファイルがありません。現在の作業ディレクトリは %s です。%s", "파일이 없습니다. 현재 작업 디렉터리는 %s입니다.%s", "Файл не существует. Текущий рабочий каталог: %s.%s")
	addToolRuntime(KeyToolRuntimePathIsDirectory, "Path %q is a directory, not a file", "路径 %q 是目录，不是文件", "Pfad %q ist ein Verzeichnis, keine Datei", "パス %q はファイルではなくディレクトリです", "경로 %q은(는) 파일이 아니라 디렉터리입니다", "Путь %q указывает на каталог, а не файл")
	addToolRuntime(KeyToolRuntimeFileNotRead, "File has not been read yet. Read it before editing.", "尚未读取该文件，请先读取再编辑。", "Die Datei wurde noch nicht gelesen. Lies sie vor dem Bearbeiten.", "ファイルはまだ読み込まれていません。編集前に読み込んでください。", "파일을 아직 읽지 않았습니다. 편집하기 전에 먼저 읽으세요.", "Файл ещё не прочитан. Прочитайте его перед редактированием.")
	addToolRuntime(KeyToolRuntimeFilePartiallyRead, "File has only been partially read. Read the whole file before editing it.", "仅读取了文件的一部分，请先读取完整文件再编辑。", "Die Datei wurde nur teilweise gelesen. Lies sie vor dem Bearbeiten vollständig.", "ファイルの一部しか読み込まれていません。編集前に全体を読み込んでください。", "파일의 일부만 읽었습니다. 편집하기 전에 전체 파일을 읽으세요.", "Файл прочитан лишь частично. Перед редактированием прочитайте его полностью.")
	addToolRuntime(KeyToolRuntimeFileViewTransformed, "The available file view was transformed and cannot safely anchor an edit. Read the target text directly and retry.", "当前文件视图经过了转换，无法安全地作为编辑锚点。请直接读取目标文本后重试。", "Die verfügbare Dateiansicht wurde verändert und kann eine Bearbeitung nicht sicher verankern. Lies den Zieltext direkt und versuche es erneut.", "利用可能なファイル表示は変換されているため、安全な編集の基準にできません。対象テキストを直接読み取って再試行してください。", "사용 가능한 파일 보기가 변환되어 안전한 편집 기준으로 사용할 수 없습니다. 대상 텍스트를 직접 읽고 다시 시도하세요.", "Доступное представление файла было преобразовано и не может безопасно служить основой правки. Прочитайте нужный текст напрямую и повторите попытку.")
	addToolRuntime(KeyToolRuntimeEditRangeNotObserved, "The edit target is outside the ranges returned by Read. Read lines %d-%d and retry.", "编辑目标不在 Read 返回的范围内。请读取第 %d-%d 行后重试。", "Das Bearbeitungsziel liegt außerhalb der von Read zurückgegebenen Bereiche. Lies die Zeilen %d-%d und versuche es erneut.", "編集対象は Read が返した範囲外です。%d-%d 行目を読み取って再試行してください。", "편집 대상이 Read가 반환한 범위 밖에 있습니다. %d-%d줄을 읽고 다시 시도하세요.", "Цель правки находится вне диапазонов, возвращённых Read. Прочитайте строки %d-%d и повторите попытку.")
	addToolRuntime(KeyToolRuntimeReadFileFailed, "Could not read file: %v", "无法读取文件：%v", "Datei konnte nicht gelesen werden: %v", "ファイルを読み込めませんでした: %v", "파일을 읽을 수 없습니다: %v", "Не удалось прочитать файл: %v")
	addToolRuntime(KeyToolRuntimeFileChangedSinceRead, "File has been modified since read, either by the user or by a linter. Read it again before attempting to edit it.", "文件在读取后已发生更改，请重新读取后再编辑。", "Die Datei wurde nach dem Lesen geändert. Lies sie vor dem Bearbeiten erneut.", "読み込み後にファイルが変更されました。編集前にもう一度読み込んでください。", "파일을 읽은 뒤 변경되었습니다. 편집하기 전에 다시 읽으세요.", "Файл изменился после чтения. Прочитайте его снова перед редактированием.")
	addToolRuntime(KeyToolRuntimeBashSedReadRequired, "Read the file first using the Read tool. The Bash sed command will modify file %s", "请先使用 Read 工具读取文件。Bash sed 命令将修改文件 %s", "Lies die Datei zuerst mit dem Read-Werkzeug. Der Bash-sed-Befehl ändert die Datei %s", "最初に Read ツールでファイルを読み取ってください。Bash の sed コマンドはファイル %s を変更します", "먼저 Read 도구로 파일을 읽으세요. Bash sed 명령이 파일 %s을(를) 변경합니다", "Сначала прочитайте файл инструментом Read. Команда Bash sed изменит файл %s")
	addToolRuntime(KeyToolRuntimeWriteFileFailed, "Could not write file: %v", "无法写入文件：%v", "Datei konnte nicht geschrieben werden: %v", "ファイルに書き込めませんでした: %v", "파일에 쓸 수 없습니다: %v", "Не удалось записать файл: %v")
	addToolRuntime(KeyToolRuntimeCreateDirectoryFailed, "Could not create directory: %v", "无法创建目录：%v", "Verzeichnis konnte nicht erstellt werden: %v", "ディレクトリを作成できませんでした: %v", "디렉터리를 만들 수 없습니다: %v", "Не удалось создать каталог: %v")
	addToolRuntime(KeyToolRuntimeFileUpdated, "The file %s has been updated successfully.", "文件已成功更新：%s", "Datei erfolgreich aktualisiert: %s", "ファイルを更新しました: %s", "파일을 업데이트했습니다: %s", "Файл успешно обновлён: %s")
	addToolRuntime(KeyToolRuntimeFileCreated, "File created successfully at: %s", "文件已成功创建：%s", "Datei erfolgreich erstellt: %s", "ファイルを作成しました: %s", "파일을 만들었습니다: %s", "Файл успешно создан: %s")
	addToolRuntime(KeyToolRuntimeWriteInvalidData, "Write returned invalid typed data", "Write 返回了无效的类型化数据", "Write hat ungültige typisierte Daten zurückgegeben", "Write が無効な型付きデータを返しました", "Write가 잘못된 형식의 데이터를 반환했습니다", "Write вернул недопустимые типизированные данные")
	addToolRuntime(KeyToolRuntimeEditPlanMode, "Edit is unavailable in plan mode; exit plan mode first", "计划模式下无法使用 Edit，请先退出计划模式", "Edit ist im Planungsmodus nicht verfügbar; verlasse ihn zuerst", "plan mode では Edit を使用できません。先に plan mode を終了してください", "plan mode에서는 Edit를 사용할 수 없습니다. 먼저 plan mode를 종료하세요", "Edit недоступен в режиме планирования; сначала выйдите из него")
	addToolRuntime(KeyToolRuntimeEditNoChanges, "No changes to make: old_string and new_string are exactly the same.", "无需更改：old_string 与 new_string 完全相同。", "Keine Änderung nötig: old_string und new_string sind identisch.", "変更はありません。old_string と new_string は同一です。", "변경할 내용이 없습니다. old_string과 new_string이 같습니다.", "Изменения не требуются: old_string и new_string совпадают.")
	addToolRuntime(KeyToolRuntimeEditNotebook, "This is a Jupyter Notebook. Use NotebookEdit to edit it.", "这是 Jupyter Notebook，请使用 NotebookEdit 进行编辑。", "Dies ist ein Jupyter Notebook. Verwende NotebookEdit zum Bearbeiten.", "これは Jupyter Notebook です。編集には NotebookEdit を使用してください。", "Jupyter Notebook 파일입니다. NotebookEdit로 편집하세요.", "Это Jupyter Notebook. Для редактирования используйте NotebookEdit.")
	addToolRuntime(KeyToolRuntimeEditTeamMemorySecret, "Refusing to edit team memory: proposed content may contain %s. Store secrets elsewhere.", "拒绝编辑团队记忆：拟写入的内容可能包含 %s。请将 secret 存储在其他位置。", "Team-Speicher wird nicht bearbeitet: Der vorgeschlagene Inhalt könnte %s enthalten. Geheimnisse anderweitig speichern.", "チームメモリの編集を拒否しました。提案内容に %s が含まれる可能性があります。secret は別の場所に保存してください。", "팀 메모리 편집을 거부했습니다. 제안된 내용에 %s이(가) 포함될 수 있습니다. secret은 다른 곳에 저장하세요.", "Редактирование памяти команды отклонено: содержимое может включать %s. Храните секреты отдельно.")
	addToolRuntime(KeyToolRuntimeEditSymlink, "Refusing to edit through symlink %q; read and edit the resolved target", "拒绝通过符号链接 %q 进行编辑，请读取并直接编辑解析后的目标", "Bearbeitung über Symlink %q abgelehnt; aufgelöstes Ziel lesen und bearbeiten", "symlink %q 経由の編集を拒否しました。解決先を読み込んで直接編集してください", "symlink %q을(를) 통한 편집을 거부했습니다. 확인된 대상을 읽고 직접 편집하세요", "Редактирование через symlink %q отклонено; прочитайте и измените фактическую цель")
	addToolRuntime(KeyToolRuntimeEditFileTooLarge, "File is too large to edit (%d bytes); the limit is %d bytes.", "文件过大，无法编辑（%d 字节）；上限为 %d 字节。", "Datei ist zu groß zum Bearbeiten (%d Byte); das Limit beträgt %d Byte.", "ファイルが大きすぎて編集できません（%d バイト）。上限は %d バイトです。", "파일이 너무 커서 편집할 수 없습니다(%d바이트). 제한은 %d바이트입니다.", "Файл слишком велик для редактирования (%d байт); предел — %d байт.")
	addToolRuntime(KeyToolRuntimeEditTargetAppeared, "File appeared after validation; read it before editing", "文件在校验后出现，请先读取再编辑", "Datei erschien nach der Prüfung; vor dem Bearbeiten lesen", "検証後にファイルが作成されました。編集前に読み込んでください", "검증 후 파일이 생성되었습니다. 편집하기 전에 읽으세요", "Файл появился после проверки; прочитайте его перед редактированием")
	addToolRuntime(KeyToolRuntimeEditRecheckTargetFailed, "Could not recheck new file target: %v", "无法重新检查新文件目标：%v", "Neues Dateiziel konnte nicht erneut geprüft werden: %v", "新規ファイルの対象を再確認できませんでした: %v", "새 파일 대상을 다시 확인할 수 없습니다: %v", "Не удалось повторно проверить цель нового файла: %v")
	addToolRuntime(KeyToolRuntimeEditStringMissing, "String to replace not found in file.\nString: %s", "在文件中找不到要替换的字符串。\n字符串：%s", "Zu ersetzende Zeichenfolge wurde nicht gefunden.\nZeichenfolge: %s", "置換する文字列がファイル内に見つかりません。\n文字列: %s", "파일에서 바꿀 문자열을 찾을 수 없습니다.\n문자열: %s", "Строка для замены не найдена в файле.\nСтрока: %s")
	addToolRuntime(KeyToolRuntimeEditAmbiguousMatch, "Found %d matches. Add surrounding context to identify one occurrence, or set replace_all to replace every occurrence.", "%d 个位置与目标匹配。请增加相邻上下文以唯一定位，或设置 replace_all 替换所有匹配项。", "%d Treffer gefunden. Ergänze umgebenden Kontext, um ein Vorkommen eindeutig zu bestimmen, oder setze replace_all, um alle zu ersetzen.", "%d 件一致しました。周辺の文脈を追加して 1 箇所に特定するか、replace_all を設定してすべて置換してください。", "%d개의 일치 항목을 찾았습니다. 한 곳을 식별하도록 주변 문맥을 추가하거나 replace_all을 설정해 모두 바꾸세요.", "Найдено совпадений: %d. Добавьте окружающий контекст, чтобы выбрать одно, или задайте replace_all для замены всех.")
	addToolRuntime(KeyToolRuntimeEditCreateExisting, "Cannot create a new file because it already exists.", "无法创建新文件：文件已存在。", "Neue Datei kann nicht erstellt werden, da sie bereits vorhanden ist.", "ファイルがすでに存在するため、新規作成できません。", "파일이 이미 있어 새로 만들 수 없습니다.", "Нельзя создать новый файл: он уже существует.")
	addToolRuntime(KeyToolRuntimeEditSummaryReplaceAll, "The file %s has been updated. All %d occurrences were successfully replaced.", "文件 %s 已成功更新；共替换 %d 处。", "Datei %s wurde erfolgreich aktualisiert. Alle %d Vorkommen wurden ersetzt.", "ファイル %s を更新しました。%d 件すべてを置換しました。", "파일 %s을(를) 업데이트했습니다. %d개 항목을 모두 바꿨습니다.", "Файл %s успешно обновлён. Заменены все вхождения: %d.")
	addToolRuntime(KeyToolRuntimeEditSummary, "The file %s has been updated successfully.", "文件 %s 已成功更新。", "Datei %s wurde erfolgreich aktualisiert.", "ファイル %s を更新しました。", "파일 %s을(를) 업데이트했습니다.", "Файл %s успешно обновлён.")
	addToolRuntime(KeyToolRuntimeEditInvalidData, "Edit returned invalid typed data", "Edit 返回了无效的类型化数据", "Edit hat ungültige typisierte Daten zurückgegeben", "Edit が無効な型付きデータを返しました", "Edit가 잘못된 형식의 데이터를 반환했습니다", "Edit вернул недопустимые типизированные данные")
	addToolRuntime(KeyToolRuntimeEditDidYouMean, " Did you mean %s?", " 你是不是想输入 %s？", " Meintest du %s?", " %s のことですか？", " %s을(를) 의미했나요?", " Возможно, имелось в виду %s?")

	addToolRuntime(KeyToolRuntimeNotebookUnknownEditMode, "Unknown edit mode", "未知的编辑模式", "Unbekannter Bearbeitungsmodus", "不明な編集モード", "알 수 없는 편집 모드", "Неизвестный режим редактирования")
	addToolRuntime(KeyToolRuntimeNotebookInvalidData, "NotebookEdit returned invalid typed data", "NotebookEdit 返回了无效的类型化数据", "NotebookEdit hat ungültige typisierte Daten zurückgegeben", "NotebookEdit が無効な型付きデータを返しました", "NotebookEdit가 잘못된 형식의 결과를 반환했습니다", "NotebookEdit вернул недопустимые типизированные данные")
	addToolRuntime(KeyToolRuntimeNotebookCellUpdated, "Updated cell %s with %s", "已使用 %s 更新单元格 %s", "Zelle %s mit %s aktualisiert", "セル %s を %s で更新しました", "셀 %s을(를) %s(으)로 업데이트했습니다", "Ячейка %s обновлена: %s")
	addToolRuntime(KeyToolRuntimeNotebookCellInserted, "Inserted cell %s with %s", "已插入单元格 %s，内容为 %s", "Zelle %s mit %s eingefügt", "セル %s を %s で挿入しました", "셀 %s을(를) %s(으)로 삽입했습니다", "Вставлена ячейка %s: %s")
	addToolRuntime(KeyToolRuntimeNotebookCellDeleted, "Deleted cell %s", "已删除单元格 %s", "Zelle %s gelöscht", "セル %s を削除しました", "셀 %s을(를) 삭제했습니다", "Ячейка %s удалена")
	addToolRuntime(KeyToolRuntimeNotebookResolvePathFailed, "Could not resolve notebook path: %v", "无法解析 Notebook 路径：%v", "Notebook-Pfad konnte nicht aufgelöst werden: %v", "Notebook のパスを解決できませんでした: %v", "Notebook 경로를 확인할 수 없습니다: %v", "Не удалось определить путь к Notebook: %v")
	addToolRuntime(KeyToolRuntimeNotebookFileRequired, "File must be a Jupyter Notebook (.ipynb). Use FileEdit for other files.", "文件必须是 Jupyter Notebook（.ipynb）；其他文件请使用 FileEdit。", "Die Datei muss ein Jupyter Notebook (.ipynb) sein. Für andere Dateien FileEdit verwenden.", "Jupyter Notebook（.ipynb）を指定してください。その他のファイルには FileEdit を使用してください。", "Jupyter Notebook(.ipynb) 파일이어야 합니다. 다른 파일에는 FileEdit를 사용하세요.", "Файл должен быть Jupyter Notebook (.ipynb). Для других файлов используйте FileEdit.")
	addToolRuntime(KeyToolRuntimeNotebookCellTypeInvalid, "invalid cell_type %q (expected 'code' or 'markdown')", "cell_type %q 无效；应为 code 或 markdown", "Ungültiger cell_type %q; erwartet wird code oder markdown", "cell_type %q は無効です。code または markdown を指定してください", "cell_type %q이(가) 잘못되었습니다. code 또는 markdown이어야 합니다", "Недопустимый cell_type %q; ожидается code или markdown")
	addToolRuntime(KeyToolRuntimeNotebookEditModeInvalid, "Edit mode must be replace, insert, or delete.", "编辑模式必须是 replace、insert 或 delete。", "Der Bearbeitungsmodus muss replace, insert oder delete sein.", "編集モードは replace、insert、delete のいずれかである必要があります。", "편집 모드는 replace, insert 또는 delete여야 합니다.", "Режим редактирования должен быть replace, insert или delete.")
	addToolRuntime(KeyToolRuntimeNotebookInsertCellTypeRequired, "cell_type is required when edit_mode=insert.", "edit_mode=insert 时必须提供 cell_type。", "Bei edit_mode=insert ist cell_type erforderlich.", "edit_mode=insert の場合は cell_type が必要です。", "edit_mode=insert일 때 cell_type이 필요합니다.", "При edit_mode=insert требуется cell_type.")
	addToolRuntime(KeyToolRuntimeNotebookCellIDRequired, "cell_id is required unless inserting a new cell.", "除插入新单元格外，必须提供 cell_id。", "cell_id ist erforderlich, außer beim Einfügen einer neuen Zelle.", "新しいセルを挿入する場合を除き、cell_id が必要です。", "새 셀을 삽입하는 경우가 아니면 cell_id가 필요합니다.", "cell_id обязателен, кроме вставки новой ячейки.")
	addToolRuntime(KeyToolRuntimeNotebookNotRead, "Notebook has not been read yet. Read it before writing.", "尚未读取该 Notebook，请先读取再写入。", "Das Notebook wurde noch nicht gelesen. Lies es vor dem Schreiben.", "Notebook はまだ読み込まれていません。書き込み前に読み込んでください。", "Notebook을 아직 읽지 않았습니다. 쓰기 전에 먼저 읽으세요.", "Notebook ещё не прочитан. Прочитайте его перед записью.")
	addToolRuntime(KeyToolRuntimeNotebookNotFound, "Notebook does not exist: %v", "Notebook 不存在：%v", "Notebook ist nicht vorhanden: %v", "Notebook がありません: %v", "Notebook이 없습니다: %v", "Notebook не существует: %v")
	addToolRuntime(KeyToolRuntimeNotebookChangedSinceRead, "File has been modified since read, either by the user or by a linter. Read it again before attempting to write it.", "Notebook 在读取后已发生更改，请重新读取后再写入。", "Das Notebook wurde nach dem Lesen geändert. Lies es vor dem Schreiben erneut.", "読み込み後に Notebook が変更されました。書き込み前にもう一度読み込んでください。", "Notebook을 읽은 뒤 변경되었습니다. 쓰기 전에 다시 읽으세요.", "Notebook изменился после чтения. Прочитайте его снова перед записью.")
	addToolRuntime(KeyToolRuntimeNotebookReadFailed, "Could not read notebook: %v", "无法读取 Notebook：%v", "Notebook konnte nicht gelesen werden: %v", "Notebook を読み込めませんでした: %v", "Notebook을 읽을 수 없습니다: %v", "Не удалось прочитать Notebook: %v")
	addToolRuntime(KeyToolRuntimeNotebookInvalidJSON, "Notebook is not valid JSON.", "Notebook 不是有效的 JSON。", "Das Notebook enthält kein gültiges JSON.", "Notebook は有効な JSON ではありません。", "Notebook이 유효한 JSON이 아닙니다.", "Notebook содержит недопустимый JSON.")
	addToolRuntime(KeyToolRuntimeNotebookSerializeFailed, "Could not serialize notebook: %v", "无法序列化 Notebook：%v", "Notebook konnte nicht serialisiert werden: %v", "Notebook をシリアライズできませんでした: %v", "Notebook을 직렬화할 수 없습니다: %v", "Не удалось сериализовать Notebook: %v")
	addToolRuntime(KeyToolRuntimeNotebookWriteFailed, "Could not write notebook: %v", "无法写入 Notebook：%v", "Notebook konnte nicht geschrieben werden: %v", "Notebook に書き込めませんでした: %v", "Notebook에 쓸 수 없습니다: %v", "Не удалось записать Notebook: %v")
}

const (
	KeyToolRuntimeMCPSummaryHeader                      Key = "tool.runtime.mcp.summary.header"
	KeyToolRuntimeMCPSummaryImage                       Key = "tool.runtime.mcp.summary.image"
	KeyToolRuntimeMCPSummaryImages                      Key = "tool.runtime.mcp.summary.images"
	KeyToolRuntimeMCPSummaryTextBlock                   Key = "tool.runtime.mcp.summary.text_block"
	KeyToolRuntimeMCPSummaryTextBlocks                  Key = "tool.runtime.mcp.summary.text_blocks"
	KeyToolRuntimeMCPSummaryOtherBlock                  Key = "tool.runtime.mcp.summary.other_block"
	KeyToolRuntimeMCPSummaryOtherBlocks                 Key = "tool.runtime.mcp.summary.other_blocks"
	KeyToolRuntimeMCPSummaryUnknownMedia                Key = "tool.runtime.mcp.summary.unknown_media"
	KeyToolRuntimeMCPSummaryImagePreview                Key = "tool.runtime.mcp.summary.image_preview"
	KeyToolRuntimeMCPSummaryTextPreview                 Key = "tool.runtime.mcp.summary.text_preview"
	KeyToolRuntimeMCPSummaryBlockPreview                Key = "tool.runtime.mcp.summary.block_preview"
	KeyToolRuntimeMCPSourceImage                        Key = "tool.runtime.mcp.source.image"
	KeyToolRuntimeMCPSourceAudio                        Key = "tool.runtime.mcp.source.audio"
	KeyToolRuntimeMCPSourceBlob                         Key = "tool.runtime.mcp.source.blob"
	KeyToolRuntimeMCPSourceResourceAt                   Key = "tool.runtime.mcp.source.resource_at"
	KeyToolRuntimeMCPSourceResource                     Key = "tool.runtime.mcp.source.resource"
	KeyToolRuntimeMCPInvalidBase64Image                 Key = "tool.runtime.mcp.invalid_base64_image"
	KeyToolRuntimeMCPInvalidBase64Binary                Key = "tool.runtime.mcp.invalid_base64_binary"
	KeyToolRuntimeMCPBinarySaveFailed                   Key = "tool.runtime.mcp.binary_save_failed"
	KeyToolRuntimeMCPBinarySaved                        Key = "tool.runtime.mcp.binary_saved"
	KeyToolRuntimeMCPResourceLink                       Key = "tool.runtime.mcp.resource_link"
	KeyToolRuntimeMCPUnknownType                        Key = "tool.runtime.mcp.unknown_type"
	KeyToolRuntimeMCPLargeOutputSaveFailed              Key = "tool.runtime.mcp.large_output_save_failed"
	KeyToolRuntimeMCPOutputTruncated                    Key = "tool.runtime.mcp.output_truncated"
	KeyToolRuntimeMCPServerUnavailable                  Key = "tool.runtime.mcp.server_unavailable"
	KeyToolRuntimeMCPToolCallFailed                     Key = "tool.runtime.mcp.tool_call_failed"
	KeyToolRuntimeMCPResourcesEmpty                     Key = "tool.runtime.mcp.resources.empty"
	KeyToolRuntimeMCPResourcesInvalidInput              Key = "tool.runtime.mcp.resources.invalid_input"
	KeyToolRuntimeMCPResourcesServerNotFound            Key = "tool.runtime.mcp.resources.server_not_found"
	KeyToolRuntimeMCPResourcesStateUnavailable          Key = "tool.runtime.mcp.resources.state_unavailable"
	KeyToolRuntimeMCPResourcesUnsupported               Key = "tool.runtime.mcp.resources.unsupported"
	KeyToolRuntimeMCPResourcesNoActiveClient            Key = "tool.runtime.mcp.resources.no_active_client"
	KeyToolRuntimeMCPResourcesReconnectNotConnected     Key = "tool.runtime.mcp.resources.reconnect_not_connected"
	KeyToolRuntimeMCPResourcesUnsupportedAfterReconnect Key = "tool.runtime.mcp.resources.unsupported_after_reconnect"
	KeyToolRuntimeMCPResourcesMarshalFailed             Key = "tool.runtime.mcp.resources.marshal_failed"
	KeyToolRuntimeMCPResourcesRequiresAuthentication    Key = "tool.runtime.mcp.resources.requires_authentication"
	KeyToolRuntimeMCPResourcesDisabled                  Key = "tool.runtime.mcp.resources.disabled"
	KeyToolRuntimeMCPResourcesPending                   Key = "tool.runtime.mcp.resources.pending"
	KeyToolRuntimeMCPResourcesNotConnected              Key = "tool.runtime.mcp.resources.not_connected"
)

func init() {
	addToolRuntime(KeyToolRuntimeMCPSummaryHeader, "MCP result: %s", "MCP 结果：%s", "MCP-Ergebnis: %s", "MCP の結果: %s", "MCP 결과: %s", "Результат MCP: %s")
	addToolRuntime(KeyToolRuntimeMCPSummaryImage, "%d image", "%d 张图片", "%d Bild", "画像 %d 件", "이미지 %d개", "%d изображение")
	addToolRuntime(KeyToolRuntimeMCPSummaryImages, "%d images", "%d 张图片", "%d Bilder", "画像 %d 件", "이미지 %d개", "%d изображений")
	addToolRuntime(KeyToolRuntimeMCPSummaryTextBlock, "%d text block", "%d 个文本块", "%d Textblock", "テキストブロック %d 件", "텍스트 블록 %d개", "%d текстовый блок")
	addToolRuntime(KeyToolRuntimeMCPSummaryTextBlocks, "%d text blocks", "%d 个文本块", "%d Textblöcke", "テキストブロック %d 件", "텍스트 블록 %d개", "%d текстовых блоков")
	addToolRuntime(KeyToolRuntimeMCPSummaryOtherBlock, "%d other block", "%d 个其他内容块", "%d weiterer Block", "その他のブロック %d 件", "기타 블록 %d개", "%d другой блок")
	addToolRuntime(KeyToolRuntimeMCPSummaryOtherBlocks, "%d other blocks", "%d 个其他内容块", "%d weitere Blöcke", "その他のブロック %d 件", "기타 블록 %d개", "%d других блоков")
	addToolRuntime(KeyToolRuntimeMCPSummaryUnknownMedia, "unknown", "未知", "unbekannt", "不明", "알 수 없음", "неизвестно")
	addToolRuntime(KeyToolRuntimeMCPSummaryImagePreview, "[image #%d: %s]", "[图片 #%d：%s]", "[Bild Nr. %d: %s]", "[画像 #%d: %s]", "[이미지 #%d: %s]", "[изображение №%d: %s]")
	addToolRuntime(KeyToolRuntimeMCPSummaryTextPreview, "[text #%d]: %s", "[文本 #%d]：%s", "[Text Nr. %d]: %s", "[テキスト #%d]: %s", "[텍스트 #%d]: %s", "[текст №%d]: %s")
	addToolRuntime(KeyToolRuntimeMCPSummaryBlockPreview, "[block #%d: %s]", "[内容块 #%d：%s]", "[Block Nr. %d: %s]", "[ブロック #%d: %s]", "[블록 #%d: %s]", "[блок №%d: %s]")
	addToolRuntime(KeyToolRuntimeMCPSourceImage, "[Image from %s] ", "[来自 %s 的图片] ", "[Bild von %s] ", "[%s からの画像] ", "[%s의 이미지] ", "[Изображение от %s] ")
	addToolRuntime(KeyToolRuntimeMCPSourceAudio, "[Audio from %s] ", "[来自 %s 的音频] ", "[Audio von %s] ", "[%s からの音声] ", "[%s의 오디오] ", "[Аудио от %s] ")
	addToolRuntime(KeyToolRuntimeMCPSourceBlob, "[Blob from %s] ", "[来自 %s 的二进制内容] ", "[Binärdaten von %s] ", "[%s からのバイナリデータ] ", "[%s의 바이너리 데이터] ", "[Двоичные данные от %s] ")
	addToolRuntime(KeyToolRuntimeMCPSourceResourceAt, "[Resource from %s at %s] ", "[来自 %s 的资源，位于 %s] ", "[Ressource von %s unter %s] ", "[%s のリソース（%s）] ", "[%s의 리소스(%s)] ", "[Ресурс от %s по адресу %s] ")
	addToolRuntime(KeyToolRuntimeMCPSourceResource, "[Resource from %s] ", "[来自 %s 的资源] ", "[Ressource von %s] ", "[%s のリソース] ", "[%s의 리소스] ", "[Ресурс от %s] ")
	addToolRuntime(KeyToolRuntimeMCPInvalidBase64Image, "%sInvalid base64 image: %v", "%s无效的 base64 图片：%v", "%sUngültiges Base64-Bild: %v", "%sbase64 画像が無効です: %v", "%s잘못된 base64 이미지: %v", "%sНедопустимое изображение base64: %v")
	addToolRuntime(KeyToolRuntimeMCPInvalidBase64Binary, "%sInvalid base64 binary data: %v", "%s无效的 base64 二进制数据：%v", "%sUngültige Base64-Binärdaten: %v", "%sbase64 バイナリデータが無効です: %v", "%s잘못된 base64 바이너리 데이터: %v", "%sНедопустимые двоичные данные base64: %v")
	addToolRuntime(KeyToolRuntimeMCPBinarySaveFailed, "%sCould not save binary content (%s, %d bytes): %s", "%s无法保存二进制内容（%s，%d 字节）：%s", "%sBinärinhalt (%s, %d Byte) konnte nicht gespeichert werden: %s", "%sバイナリデータ（%s、%d バイト）を保存できませんでした: %s", "%s바이너리 콘텐츠(%s, %d바이트)를 저장할 수 없습니다: %s", "%sНе удалось сохранить двоичные данные (%s, %d байт): %s")
	addToolRuntime(KeyToolRuntimeMCPBinarySaved, "%sBinary content (%s, %s) saved to %s", "%s二进制内容（%s，%s）已保存至 %s", "%sBinärinhalt (%s, %s) wurde unter %s gespeichert", "%sバイナリデータ（%s、%s）を %s に保存しました", "%s바이너리 콘텐츠(%s, %s)를 %s에 저장했습니다", "%sДвоичные данные (%s, %s) сохранены в %s")
	addToolRuntime(KeyToolRuntimeMCPResourceLink, "[Resource link: %s] %s", "[资源链接：%s] %s", "[Ressourcenlink: %s] %s", "[リソースリンク: %s] %s", "[리소스 링크: %s] %s", "[Ссылка на ресурс: %s] %s")
	addToolRuntime(KeyToolRuntimeMCPUnknownType, "unknown type", "未知类型", "unbekannter Typ", "不明な種類", "알 수 없는 형식", "неизвестный тип")
	addToolRuntime(KeyToolRuntimeMCPLargeOutputSaveFailed, "Error: result (%s characters) exceeds the token limit and could not be saved: %s.", "错误：结果（%s 个字符）超出 token 上限，且无法保存：%s。", "Fehler: Das Ergebnis (%s Zeichen) überschreitet das Token-Limit und konnte nicht gespeichert werden: %s.", "エラー: 結果（%s 文字）が token 上限を超え、保存にも失敗しました: %s。", "오류: 결과(%s자)가 token 제한을 초과했으며 저장할 수 없습니다: %s.", "Ошибка: результат (%s символов) превышает лимит token и не может быть сохранён: %s.")
	addToolRuntime(KeyToolRuntimeMCPOutputTruncated, "\n\n[OUTPUT TRUNCATED — exceeded the %d-token limit]\n\nThe tool output was truncated.", "\n\n[输出已截断——超出 %d token 上限]\n\n工具输出已被截断。", "\n\n[AUSGABE GEKÜRZT — Limit von %d Token überschritten]\n\nDie Tool-Ausgabe wurde gekürzt.", "\n\n[出力を省略 — %d token の上限を超過]\n\nツール出力を省略しました。", "\n\n[출력 잘림 — %d token 제한 초과]\n\n도구 출력이 잘렸습니다.", "\n\n[ВЫВОД УСЕЧЁН — превышен лимит %d token]\n\nВывод инструмента был усечён.")
	addToolRuntime(KeyToolRuntimeMCPServerUnavailable, "Error: server %q is unavailable (%s). Configured servers: %s", "错误：server %q 不可用（%s）。已配置的 server：%s", "Fehler: Server %q ist nicht verfügbar (%s). Konfigurierte Server: %s", "エラー: server %q は利用できません（%s）。設定済み server: %s", "오류: server %q을(를) 사용할 수 없습니다(%s). 구성된 server: %s", "Ошибка: server %q недоступен (%s). Настроенные server: %s")
	addToolRuntime(KeyToolRuntimeMCPToolCallFailed, "Error: MCP tool call failed: %s", "错误：MCP tool 调用失败：%s", "Fehler: MCP-Tool-Aufruf fehlgeschlagen: %s", "エラー: MCP tool の呼び出しに失敗しました: %s", "오류: MCP tool 호출 실패: %s", "Ошибка: вызов MCP tool завершился с ошибкой: %s")

	addToolRuntime(KeyToolRuntimeMCPResourcesEmpty, "No resources found. MCP servers may still provide tools even if they have no resources.", "未找到资源；MCP server 仍可能提供 tool。", "Keine Ressourcen gefunden. MCP-Server können weiterhin Tools bereitstellen.", "リソースが見つかりません。MCP server が tool を提供している場合があります。", "리소스를 찾지 못했습니다. MCP server가 tool을 제공할 수 있습니다.", "Ресурсы не найдены. MCP server всё ещё может предоставлять tool.")
	addToolRuntime(KeyToolRuntimeMCPResourcesInvalidInput, "Error: Invalid input: %s", "错误：输入无效：%s", "Fehler: ungültige Eingabe: %s", "エラー: 入力が無効です: %s", "오류: 잘못된 입력: %s", "Ошибка: недопустимые входные данные: %s")
	addToolRuntime(KeyToolRuntimeMCPResourcesServerNotFound, "Server %q not found. Available servers: %s", "找不到 server %q。可用 server：%s", "Server %q wurde nicht gefunden. Verfügbare Server: %s", "server %q が見つかりません。利用可能な server: %s", "server %q을(를) 찾을 수 없습니다. 사용 가능한 server: %s", "Server %q не найден. Доступные server: %s")
	addToolRuntime(KeyToolRuntimeMCPResourcesStateUnavailable, "Server runtime state is unavailable", "server runtime 状态不可用", "Server-Runtime-Status ist nicht verfügbar", "server runtime の状態を取得できません", "server runtime 상태를 확인할 수 없습니다", "Состояние runtime server недоступно")
	addToolRuntime(KeyToolRuntimeMCPResourcesUnsupported, "Server %q does not advertise resources capability", "server %q 未声明 resources capability", "Server %q bietet keine Ressourcenfunktion an", "server %q は resources capability を公開していません", "server %q이(가) resources capability를 제공하지 않습니다", "Server %q не объявляет capability ресурсов")
	addToolRuntime(KeyToolRuntimeMCPResourcesNoActiveClient, "Server %q has no active MCP client", "server %q 没有活动的 MCP client", "Server %q hat keinen aktiven MCP-Client", "server %q に有効な MCP client がありません", "server %q에 활성 MCP client가 없습니다", "У server %q нет активного MCP client")
	addToolRuntime(KeyToolRuntimeMCPResourcesReconnectNotConnected, "Server %q is not connected after reconnect (%s)", "server %q 重连后仍未连接（%s）", "Server %q ist nach dem Neuverbinden nicht verbunden (%s)", "server %q は再接続後も未接続です（%s）", "server %q이(가) 다시 연결한 후에도 연결되지 않았습니다(%s)", "Server %q не подключён после переподключения (%s)")
	addToolRuntime(KeyToolRuntimeMCPResourcesUnsupportedAfterReconnect, "Server %q does not advertise resources capability after reconnect", "server %q 重连后仍未声明 resources capability", "Server %q bietet auch nach dem Neuverbinden keine Ressourcenfunktion an", "server %q は再接続後も resources capability を公開していません", "server %q이(가) 다시 연결한 후에도 resources capability를 제공하지 않습니다", "Server %q после переподключения не объявляет capability ресурсов")
	addToolRuntime(KeyToolRuntimeMCPResourcesMarshalFailed, "Error: could not encode MCP resources: %s", "错误：无法编码 MCP 资源：%s", "Fehler: MCP-Ressourcen konnten nicht codiert werden: %s", "エラー: MCP リソースをエンコードできませんでした: %s", "오류: MCP 리소스를 인코딩할 수 없습니다: %s", "Ошибка: не удалось закодировать ресурсы MCP: %s")
	addToolRuntime(KeyToolRuntimeMCPResourcesRequiresAuthentication, "Server requires authentication", "server 需要身份验证", "Server erfordert Authentifizierung", "server には認証が必要です", "server에 인증이 필요합니다", "Server требует аутентификации")
	addToolRuntime(KeyToolRuntimeMCPResourcesDisabled, "Server is disabled", "server 已禁用", "Server ist deaktiviert", "server は無効です", "server가 비활성화되었습니다", "Server отключён")
	addToolRuntime(KeyToolRuntimeMCPResourcesPending, "Server is waiting to connect", "server 正在等待连接", "Server wartet auf die Verbindung", "server は接続待ちです", "server가 연결을 기다리고 있습니다", "Server ожидает подключения")
	addToolRuntime(KeyToolRuntimeMCPResourcesNotConnected, "Server is not connected", "server 未连接", "Server ist nicht verbunden", "server は未接続です", "server가 연결되지 않았습니다", "Server не подключён")
}

const (
	KeyToolRuntimeWebTruncatedMarker                Key = "tool.runtime.web.truncated_marker"
	KeyToolRuntimeWebError                          Key = "tool.runtime.web.error"
	KeyToolRuntimeWebFetchStoppedAfterRedirects     Key = "tool.runtime.web.fetch.stopped_after_redirects"
	KeyToolRuntimeWebFetchURLRequired               Key = "tool.runtime.web.fetch.url_required"
	KeyToolRuntimeWebFetchPromptRequired            Key = "tool.runtime.web.fetch.prompt_required"
	KeyToolRuntimeWebFetchInvalidURL                Key = "tool.runtime.web.fetch.invalid_url"
	KeyToolRuntimeWebFetchBlockedURL                Key = "tool.runtime.web.fetch.blocked_url"
	KeyToolRuntimeWebFetchDomainUnavailable         Key = "tool.runtime.web.fetch.domain_unavailable"
	KeyToolRuntimeWebFetchRedirectBlocked           Key = "tool.runtime.web.fetch.redirect_blocked"
	KeyToolRuntimeWebFetchRedirectBlockedSSRF       Key = "tool.runtime.web.fetch.redirect_blocked_ssrf"
	KeyToolRuntimeWebFetchFailed                    Key = "tool.runtime.web.fetch.failed"
	KeyToolRuntimeWebFetchEgressBlocked             Key = "tool.runtime.web.fetch.egress_blocked"
	KeyToolRuntimeWebFetchHTTPError                 Key = "tool.runtime.web.fetch.http_error"
	KeyToolRuntimeWebFetchResponseTooLargeWithBytes Key = "tool.runtime.web.fetch.response_too_large_with_bytes"
	KeyToolRuntimeWebFetchReadResponseFailed        Key = "tool.runtime.web.fetch.read_response_failed"
	KeyToolRuntimeWebFetchResponseTooLarge          Key = "tool.runtime.web.fetch.response_too_large"
	KeyToolRuntimeWebFetchInvalidURLPrefix          Key = "tool.runtime.web.fetch.invalid_url_prefix"
	KeyToolRuntimeWebFetchBinarySaved               Key = "tool.runtime.web.fetch.binary_saved"
	KeyToolRuntimeWebFetchBytes                     Key = "tool.runtime.web.fetch.bytes"
	KeyToolRuntimeWebSearchResultsForQuery          Key = "tool.runtime.web.search.results_for_query"
	KeyToolRuntimeWebSearchNoLinks                  Key = "tool.runtime.web.search.no_links"
	KeyToolRuntimeWebSearchLinks                    Key = "tool.runtime.web.search.links"
	KeyToolRuntimeWebSearchQueryRequired            Key = "tool.runtime.web.search.query_required"
	KeyToolRuntimeWebSearchQueryTooShort            Key = "tool.runtime.web.search.query_too_short"
	KeyToolRuntimeWebSearchConflictingDomainFilters Key = "tool.runtime.web.search.conflicting_domain_filters"
	KeyToolRuntimeWebSearchFailed                   Key = "tool.runtime.web.search.failed"
	KeyToolRuntimeWebSearchDomainEmpty              Key = "tool.runtime.web.search.domain_empty"
	KeyToolRuntimeWebSearchDomainScheme             Key = "tool.runtime.web.search.domain_scheme"
	KeyToolRuntimeWebSearchDomainWhitespace         Key = "tool.runtime.web.search.domain_whitespace"
	KeyToolRuntimeWebSearchDomainLeadingSlash       Key = "tool.runtime.web.search.domain_leading_slash"
)

func init() {
	addToolRuntime(KeyToolRuntimeWebTruncatedMarker, "\n…[truncated]", "\n…[已截断]", "\n…[gekürzt]", "\n…[省略]", "\n…[잘림]", "\n…[усечено]")
	addToolRuntime(KeyToolRuntimeWebError, "Error: %s", "错误：%s", "Fehler: %s", "エラー: %s", "오류: %s", "Ошибка: %s")
	addToolRuntime(KeyToolRuntimeWebFetchStoppedAfterRedirects, "Stopped after %d redirects", "在 %d 次重定向后停止", "Nach %d Weiterleitungen abgebrochen", "%d 回のリダイレクト後に停止しました", "%d회 리디렉션 후 중단했습니다", "Остановлено после %d перенаправлений")
	addToolRuntime(KeyToolRuntimeWebFetchURLRequired, "Error: url is required", "错误：必须提供 url", "Fehler: url ist erforderlich", "エラー: url は必須です", "오류: url이 필요합니다", "Ошибка: требуется url")
	addToolRuntime(KeyToolRuntimeWebFetchPromptRequired, "Error: prompt is required", "错误：必须提供 prompt", "Fehler: prompt ist erforderlich", "エラー: prompt は必須です", "오류: prompt가 필요합니다", "Ошибка: требуется prompt")
	addToolRuntime(KeyToolRuntimeWebFetchInvalidURL, "Error: URL %q is invalid and could not be parsed", "错误：URL %q 无效，无法解析", "Fehler: URL %q ist ungültig und konnte nicht geparst werden", "エラー: URL %q は無効で、解析できませんでした", "오류: URL %q이(가) 잘못되어 파싱할 수 없습니다", "Ошибка: URL %q недопустим и не может быть разобран")
	addToolRuntime(KeyToolRuntimeWebFetchBlockedURL, "Error: blocked URL: %s", "错误：URL 已被阻止：%s", "Fehler: blockierte URL: %s", "エラー: URL がブロックされました: %s", "오류: 차단된 URL: %s", "Ошибка: URL заблокирован: %s")
	addToolRuntime(KeyToolRuntimeWebFetchDomainUnavailable, "Error: unable to fetch from %s", "错误：无法从 %s 获取内容", "Fehler: Abruf von %s nicht möglich", "エラー: %s から取得できません", "오류: %s에서 가져올 수 없습니다", "Ошибка: не удалось получить данные с %s")
	addToolRuntime(KeyToolRuntimeWebFetchRedirectBlocked, "Redirect blocked", "重定向已被阻止", "Weiterleitung blockiert", "リダイレクトがブロックされました", "리디렉션이 차단되었습니다", "Перенаправление заблокировано")
	addToolRuntime(KeyToolRuntimeWebFetchRedirectBlockedSSRF, "Redirect blocked by SSRF protection", "重定向已被 SSRF 防护阻止", "Weiterleitung durch SSRF-Schutz blockiert", "SSRF 対策によりリダイレクトがブロックされました", "SSRF 보호로 리디렉션이 차단되었습니다", "Перенаправление заблокировано защитой SSRF")
	addToolRuntime(KeyToolRuntimeWebFetchFailed, "Error: fetch failed: %s", "错误：获取失败：%s", "Fehler: Abruf fehlgeschlagen: %s", "エラー: 取得に失敗しました: %s", "오류: 가져오기 실패: %s", "Ошибка: не удалось получить данные: %s")
	addToolRuntime(KeyToolRuntimeWebFetchEgressBlocked, "Error: egress blocked by allowlist (HTTP %d): %s", "错误：出站请求被 allowlist 阻止（HTTP %d）：%s", "Fehler: Ausgehende Anfrage durch Allowlist blockiert (HTTP %d): %s", "エラー: allowlist により外向き通信がブロックされました（HTTP %d）: %s", "오류: allowlist가 송신 요청을 차단했습니다(HTTP %d): %s", "Ошибка: исходящий запрос заблокирован allowlist (HTTP %d): %s")
	addToolRuntime(KeyToolRuntimeWebFetchHTTPError, "Error: HTTP %d %s", "错误：HTTP %d %s", "Fehler: HTTP %d %s", "エラー: HTTP %d %s", "오류: HTTP %d %s", "Ошибка: HTTP %d %s")
	addToolRuntime(KeyToolRuntimeWebFetchResponseTooLargeWithBytes, "Error: response exceeds the 10 MB limit (%d bytes)", "错误：响应超出 10 MB 上限（%d 字节）", "Fehler: Antwort überschreitet das Limit von 10 MB (%d Byte)", "エラー: レスポンスが 10 MB の上限を超えています（%d バイト）", "오류: 응답이 10MB 제한을 초과했습니다(%d바이트)", "Ошибка: ответ превышает предел 10 МБ (%d байт)")
	addToolRuntime(KeyToolRuntimeWebFetchReadResponseFailed, "Error reading response: %s", "读取响应时出错：%s", "Fehler beim Lesen der Antwort: %s", "レスポンスの読み込みエラー: %s", "응답 읽기 오류: %s", "Ошибка чтения ответа: %s")
	addToolRuntime(KeyToolRuntimeWebFetchResponseTooLarge, "Error: response exceeds the 10 MB limit", "错误：响应超出 10 MB 上限", "Fehler: Antwort überschreitet das Limit von 10 MB", "エラー: レスポンスが 10 MB の上限を超えています", "오류: 응답이 10MB 제한을 초과했습니다", "Ошибка: ответ превышает предел 10 МБ")
	addToolRuntime(KeyToolRuntimeWebFetchInvalidURLPrefix, "Invalid URL", "URL 无效", "Ungültige URL", "URL が無効です", "잘못된 URL", "Недопустимый URL")
	addToolRuntime(KeyToolRuntimeWebFetchBinarySaved, "\n\n[Binary content (%s, %s) also saved to %s]", "\n\n[二进制内容（%s，%s）另存至 %s]", "\n\n[Binärinhalt (%s, %s) zusätzlich unter %s gespeichert]", "\n\n[バイナリデータ（%s、%s）を %s にも保存しました]", "\n\n[바이너리 콘텐츠(%s, %s)를 %s에도 저장했습니다]", "\n\n[Двоичные данные (%s, %s) также сохранены в %s]")
	addToolRuntime(KeyToolRuntimeWebFetchBytes, "%d bytes", "%d 字节", "%d Byte", "%d バイト", "%d바이트", "%d байт")
	addToolRuntime(KeyToolRuntimeWebSearchResultsForQuery, "Web search results for query: %q", "Web 搜索结果：%q", "Web-Suchergebnisse für: %q", "Web 検索結果: %q", "Web 검색 결과: %q", "Результаты Web-поиска для: %q")
	addToolRuntime(KeyToolRuntimeWebSearchNoLinks, "No links found.", "未找到链接。", "Keine Links gefunden.", "リンクが見つかりません。", "링크를 찾지 못했습니다.", "Ссылки не найдены.")
	addToolRuntime(KeyToolRuntimeWebSearchLinks, "Links: %s", "链接：%s", "Links: %s", "リンク: %s", "링크: %s", "Ссылки: %s")
	addToolRuntime(KeyToolRuntimeWebSearchQueryRequired, "Error: Missing query", "错误：必须提供 query", "Fehler: query ist erforderlich", "エラー: query は必須です", "오류: query가 필요합니다", "Ошибка: требуется query")
	addToolRuntime(KeyToolRuntimeWebSearchQueryTooShort, "Error: query must contain at least 2 characters", "错误：query 至少需要 2 个字符", "Fehler: query muss mindestens 2 Zeichen enthalten", "エラー: query は 2 文字以上である必要があります", "오류: query는 두 글자 이상이어야 합니다", "Ошибка: query должен содержать не менее 2 символов")
	addToolRuntime(KeyToolRuntimeWebSearchConflictingDomainFilters, "Error: Cannot specify both allowed_domains and blocked_domains in the same request", "错误：不能同时设置 allowed_domains 和 blocked_domains", "Fehler: allowed_domains und blocked_domains dürfen nicht gleichzeitig gesetzt sein", "エラー: allowed_domains と blocked_domains は同時に指定できません", "오류: allowed_domains와 blocked_domains를 함께 설정할 수 없습니다", "Ошибка: allowed_domains и blocked_domains нельзя задавать одновременно")
	addToolRuntime(KeyToolRuntimeWebSearchFailed, "Error: search failed: %s", "错误：搜索失败：%s", "Fehler: Suche fehlgeschlagen: %s", "エラー: 検索に失敗しました: %s", "오류: 검색 실패: %s", "Ошибка: поиск завершился с ошибкой: %s")
	addToolRuntime(KeyToolRuntimeWebSearchDomainEmpty, "%s[%d]: empty domain entries are not allowed", "%s[%d]：不允许空的 domain 条目", "%s[%d]: Leere Domain-Einträge sind nicht zulässig", "%s[%d]: 空の domain は指定できません", "%s[%d]: 빈 domain 항목은 허용되지 않습니다", "%s[%d]: пустые записи domain недопустимы")
	addToolRuntime(KeyToolRuntimeWebSearchDomainScheme, "%s[%d]: domain must not include a scheme (got %q)", "%s[%d]：domain 不能包含 scheme（收到 %q）", "%s[%d]: Domain darf kein Schema enthalten (erhalten: %q)", "%s[%d]: domain に scheme を含めることはできません（入力: %q）", "%s[%d]: domain에 scheme을 포함할 수 없습니다(입력: %q)", "%s[%d]: domain не должен содержать scheme (получено %q)")
	addToolRuntime(KeyToolRuntimeWebSearchDomainWhitespace, "%s[%d]: domain must not contain whitespace (got %q)", "%s[%d]：domain 不能包含空白字符（收到 %q）", "%s[%d]: Domain darf keine Leerzeichen enthalten (erhalten: %q)", "%s[%d]: domain に空白を含めることはできません（入力: %q）", "%s[%d]: domain에 공백을 포함할 수 없습니다(입력: %q)", "%s[%d]: domain не должен содержать пробелы (получено %q)")
	addToolRuntime(KeyToolRuntimeWebSearchDomainLeadingSlash, "%s[%d]: domain must not start with / (got %q)", "%s[%d]：domain 不能以 / 开头（收到 %q）", "%s[%d]: Domain darf nicht mit / beginnen (erhalten: %q)", "%s[%d]: domain を / で始めることはできません（入力: %q）", "%s[%d]: domain은 /로 시작할 수 없습니다(입력: %q)", "%s[%d]: domain не должен начинаться с / (получено %q)")
}

const (
	KeyToolRuntimeLSPInitializeForLanguage   Key = "tool.runtime.lsp.initialize_for_language"
	KeyToolRuntimeLSPNoServerConfigured      Key = "tool.runtime.lsp.no_server_configured"
	KeyToolRuntimeLSPStdinPipe               Key = "tool.runtime.lsp.stdin_pipe"
	KeyToolRuntimeLSPStdoutPipe              Key = "tool.runtime.lsp.stdout_pipe"
	KeyToolRuntimeLSPStartProcess            Key = "tool.runtime.lsp.start_process"
	KeyToolRuntimeLSPInitializeRequest       Key = "tool.runtime.lsp.initialize_request"
	KeyToolRuntimeLSPInitializedNotification Key = "tool.runtime.lsp.initialized_notification"
	KeyToolRuntimeLSPMissingOperation        Key = "tool.runtime.lsp.missing_operation"
	KeyToolRuntimeLSPMissingFilePath         Key = "tool.runtime.lsp.missing_file_path"
	KeyToolRuntimeLSPInvalidLine             Key = "tool.runtime.lsp.invalid_line"
	KeyToolRuntimeLSPInvalidCharacter        Key = "tool.runtime.lsp.invalid_character"
	KeyToolRuntimeLSPUnsupportedOperation    Key = "tool.runtime.lsp.unsupported_operation"
	KeyToolRuntimeLSPStateUnavailable        Key = "tool.runtime.lsp.state_unavailable"
	KeyToolRuntimeLSPBinaryNotFound          Key = "tool.runtime.lsp.binary_not_found"
	KeyToolRuntimeLSPManagerUnavailable      Key = "tool.runtime.lsp.manager_unavailable"
	KeyToolRuntimeLSPStartServerError        Key = "tool.runtime.lsp.start_server_error"
	KeyToolRuntimeLSPOpenFileError           Key = "tool.runtime.lsp.open_file_error"
	KeyToolRuntimeLSPUnknownOperation        Key = "tool.runtime.lsp.unknown_operation"
	KeyToolRuntimeLSPNoHover                 Key = "tool.runtime.lsp.no_hover"
	KeyToolRuntimeLSPNoSymbols               Key = "tool.runtime.lsp.no_symbols"
	KeyToolRuntimeLSPNoWorkspaceSymbols      Key = "tool.runtime.lsp.no_workspace_symbols"
	KeyToolRuntimeLSPNoCallHierarchyItem     Key = "tool.runtime.lsp.no_call_hierarchy_item"
	KeyToolRuntimeLSPNoIncomingCalls         Key = "tool.runtime.lsp.no_incoming_calls"
	KeyToolRuntimeLSPIncomingCallsHeader     Key = "tool.runtime.lsp.incoming_calls_header"
	KeyToolRuntimeLSPNoOutgoingCalls         Key = "tool.runtime.lsp.no_outgoing_calls"
	KeyToolRuntimeLSPOutgoingCallsHeader     Key = "tool.runtime.lsp.outgoing_calls_header"
	KeyToolRuntimeLSPInstallBinaryFallback   Key = "tool.runtime.lsp.install_binary_fallback"
	KeyToolRuntimeLSPOperationError          Key = "tool.runtime.lsp.operation_error"
	KeyToolRuntimeLSPNoOperationResults      Key = "tool.runtime.lsp.no_operation_results"
	KeyToolRuntimeLSPSymbolLine              Key = "tool.runtime.lsp.symbol_line"
)

func init() {
	addToolRuntime(KeyToolRuntimeLSPInitializeForLanguage, "Could not initialize LSP for %s: %v", "无法为 %s 初始化 LSP：%v", "LSP für %s konnte nicht initialisiert werden: %v", "%s の LSP を初期化できませんでした: %v", "%s용 LSP를 초기화할 수 없습니다: %v", "Не удалось инициализировать LSP для %s: %v")
	addToolRuntime(KeyToolRuntimeLSPNoServerConfigured, "No LSP server is configured for language %q", "未为语言 %q 配置 LSP server", "Für die Sprache %q ist kein LSP-Server konfiguriert", "言語 %q 用の LSP server が設定されていません", "언어 %q에 구성된 LSP server가 없습니다", "Для языка %q не настроен LSP server")
	addToolRuntime(KeyToolRuntimeLSPStdinPipe, "Could not open stdin pipe: %v", "无法打开 stdin 管道：%v", "stdin-Pipe konnte nicht geöffnet werden: %v", "stdin パイプを開けませんでした: %v", "stdin 파이프를 열 수 없습니다: %v", "Не удалось открыть канал stdin: %v")
	addToolRuntime(KeyToolRuntimeLSPStdoutPipe, "Could not open stdout pipe: %v", "无法打开 stdout 管道：%v", "stdout-Pipe konnte nicht geöffnet werden: %v", "stdout パイプを開けませんでした: %v", "stdout 파이프를 열 수 없습니다: %v", "Не удалось открыть канал stdout: %v")
	addToolRuntime(KeyToolRuntimeLSPStartProcess, "Could not start %q: %v", "无法启动 %q：%v", "%q konnte nicht gestartet werden: %v", "%q を起動できませんでした: %v", "%q을(를) 시작할 수 없습니다: %v", "Не удалось запустить %q: %v")
	addToolRuntime(KeyToolRuntimeLSPInitializeRequest, "LSP initialize request failed: %v", "LSP initialize 请求失败：%v", "LSP-initialize-Anfrage fehlgeschlagen: %v", "LSP initialize リクエストに失敗しました: %v", "LSP initialize 요청 실패: %v", "Запрос LSP initialize завершился с ошибкой: %v")
	addToolRuntime(KeyToolRuntimeLSPInitializedNotification, "LSP initialized notification failed: %v", "LSP initialized 通知失败：%v", "LSP-initialized-Benachrichtigung fehlgeschlagen: %v", "LSP initialized 通知に失敗しました: %v", "LSP initialized 알림 실패: %v", "Уведомление LSP initialized завершилось с ошибкой: %v")
	addToolRuntime(KeyToolRuntimeLSPMissingOperation, "Error: operation is required", "错误：必须提供 operation", "Fehler: operation ist erforderlich", "エラー: operation は必須です", "오류: operation이 필요합니다", "Ошибка: требуется operation")
	addToolRuntime(KeyToolRuntimeLSPMissingFilePath, "Error: filePath is required", "错误：必须提供 filePath", "Fehler: filePath ist erforderlich", "エラー: filePath は必須です", "오류: filePath가 필요합니다", "Ошибка: требуется filePath")
	addToolRuntime(KeyToolRuntimeLSPInvalidLine, "Error: line must be a positive integer", "错误：line 必须是正整数", "Fehler: line muss eine positive ganze Zahl sein", "エラー: line は正の整数である必要があります", "오류: line은 양의 정수여야 합니다", "Ошибка: line должен быть положительным целым числом")
	addToolRuntime(KeyToolRuntimeLSPInvalidCharacter, "Error: character must be a positive integer", "错误：character 必须是正整数", "Fehler: character muss eine positive ganze Zahl sein", "エラー: character は正の整数である必要があります", "오류: character는 양의 정수여야 합니다", "Ошибка: character должен быть положительным целым числом")
	addToolRuntime(KeyToolRuntimeLSPUnsupportedOperation, "Operation %s is not yet supported for %s", "%s 尚不支持 operation %s", "Operation %s wird für %s noch nicht unterstützt", "%s では operation %s はまだサポートされていません", "%s에서는 operation %s을(를) 아직 지원하지 않습니다", "Operation %s пока не поддерживается для %s")
	addToolRuntime(KeyToolRuntimeLSPStateUnavailable, "Internal error: LSP state is unavailable", "内部错误：LSP 状态不可用", "Interner Fehler: LSP-Status ist nicht verfügbar", "内部エラー: LSP の状態を取得できません", "내부 오류: LSP 상태를 사용할 수 없습니다", "Внутренняя ошибка: состояние LSP недоступно")
	addToolRuntime(KeyToolRuntimeLSPBinaryNotFound, "%s not found. Install with: %s", "找不到 %s。请使用以下命令安装：%s", "%s wurde nicht gefunden. Installation: %s", "%s が見つかりません。次のコマンドでインストールしてください: %s", "%s을(를) 찾을 수 없습니다. 다음 명령으로 설치하세요: %s", "%s не найден. Установите его командой: %s")
	addToolRuntime(KeyToolRuntimeLSPManagerUnavailable, "Internal error: LSP manager is unavailable", "内部错误：LSP manager 不可用", "Interner Fehler: LSP-Manager ist nicht verfügbar", "内部エラー: LSP manager を利用できません", "내부 오류: LSP manager를 사용할 수 없습니다", "Внутренняя ошибка: LSP manager недоступен")
	addToolRuntime(KeyToolRuntimeLSPStartServerError, "Error starting LSP server: %v", "启动 LSP server 时出错：%v", "Fehler beim Starten des LSP-Servers: %v", "LSP server の起動エラー: %v", "LSP server 시작 오류: %v", "Ошибка запуска LSP server: %v")
	addToolRuntime(KeyToolRuntimeLSPOpenFileError, "Error opening file in LSP server: %v", "在 LSP server 中打开文件时出错：%v", "Fehler beim Öffnen der Datei im LSP-Server: %v", "LSP server でファイルを開く際のエラー: %v", "LSP server에서 파일 열기 오류: %v", "Ошибка открытия файла в LSP server: %v")
	addToolRuntime(KeyToolRuntimeLSPUnknownOperation, "Unknown operation: %s", "未知 operation：%s", "Unbekannte Operation: %s", "不明な operation: %s", "알 수 없는 operation: %s", "Неизвестная operation: %s")
	addToolRuntime(KeyToolRuntimeLSPNoHover, "No hover information is available", "没有可用的 hover 信息", "Keine Hover-Informationen verfügbar", "hover 情報はありません", "사용 가능한 hover 정보가 없습니다", "Информация hover отсутствует")
	addToolRuntime(KeyToolRuntimeLSPNoSymbols, "No symbols found", "未找到 symbol", "Keine Symbole gefunden", "symbol が見つかりません", "symbol을 찾지 못했습니다", "Символы не найдены")
	addToolRuntime(KeyToolRuntimeLSPNoWorkspaceSymbols, "No workspace symbols found", "未找到 workspace symbol", "Keine Workspace-Symbole gefunden", "workspace symbol が見つかりません", "workspace symbol을 찾지 못했습니다", "Символы workspace не найдены")
	addToolRuntime(KeyToolRuntimeLSPNoCallHierarchyItem, "No call hierarchy item was found at this position", "此位置未找到调用层次结构项", "An dieser Position wurde kein Aufrufhierarchieelement gefunden", "この位置に call hierarchy の項目はありません", "이 위치에서 call hierarchy 항목을 찾지 못했습니다", "В этой позиции не найден элемент иерархии вызовов")
	addToolRuntime(KeyToolRuntimeLSPNoIncomingCalls, "No incoming calls to %s", "没有对 %s 的传入调用", "Keine eingehenden Aufrufe für %s", "%s への呼び出しはありません", "%s에 대한 수신 호출이 없습니다", "Входящие вызовы для %s отсутствуют")
	addToolRuntime(KeyToolRuntimeLSPIncomingCallsHeader, "Incoming calls to %s:", "对 %s 的传入调用：", "Eingehende Aufrufe für %s:", "%s への呼び出し:", "%s에 대한 수신 호출:", "Входящие вызовы для %s:")
	addToolRuntime(KeyToolRuntimeLSPNoOutgoingCalls, "No outgoing calls from %s", "%s 没有传出调用", "Keine ausgehenden Aufrufe von %s", "%s からの呼び出しはありません", "%s의 송신 호출이 없습니다", "Исходящие вызовы из %s отсутствуют")
	addToolRuntime(KeyToolRuntimeLSPOutgoingCallsHeader, "Outgoing calls from %s:", "%s 的传出调用：", "Ausgehende Aufrufe von %s:", "%s からの呼び出し:", "%s의 송신 호출:", "Исходящие вызовы из %s:")
	addToolRuntime(KeyToolRuntimeLSPInstallBinaryFallback, "install %s", "安装 %s", "%s installieren", "%s をインストール", "%s 설치", "установите %s")
	addToolRuntime(KeyToolRuntimeLSPOperationError, "LSP %s error: %v", "LSP %s 错误：%v", "LSP-%s-Fehler: %v", "LSP %s エラー: %v", "LSP %s 오류: %v", "Ошибка LSP %s: %v")
	addToolRuntime(KeyToolRuntimeLSPNoOperationResults, "No %s results found", "未找到 %s 结果", "Keine Ergebnisse für %s gefunden", "%s の結果が見つかりません", "%s 결과를 찾지 못했습니다", "Результаты %s не найдены")
	addToolRuntime(KeyToolRuntimeLSPSymbolLine, "(line %d)", "（第 %d 行）", "(Zeile %d)", "（%d 行目）", "(%d행)", "(строка %d)")
}

const (
	KeyToolRuntimeAskUserSelectedPreview        Key = "tool.runtime.ask_user.selected_preview"
	KeyToolRuntimeAskUserNotes                  Key = "tool.runtime.ask_user.notes"
	KeyToolRuntimeAskUserAnsweredContinue       Key = "tool.runtime.ask_user.answered_continue"
	KeyToolRuntimeAskUserAnswered               Key = "tool.runtime.ask_user.answered"
	KeyToolRuntimeAskUserPlanApproval           Key = "tool.runtime.ask_user.plan_approval"
	KeyToolRuntimeAskUserReadAnswerFailed       Key = "tool.runtime.ask_user.read_answer_failed"
	KeyToolRuntimeAskUserInteractiveUnavailable Key = "tool.runtime.ask_user.interactive_unavailable"
	KeyToolRuntimeAskUserAnswersRequired        Key = "tool.runtime.ask_user.answers_required"
	KeyToolRuntimeAskUserInteractionCancelled   Key = "tool.runtime.ask_user.interaction_cancelled"
	KeyToolRuntimeAskUserInteractionStale       Key = "tool.runtime.ask_user.interaction_stale"
	KeyToolRuntimeAskUserInvalidSingleSelection Key = "tool.runtime.ask_user.invalid_single_selection"
	KeyToolRuntimeAskUserInvalidMultiSelection  Key = "tool.runtime.ask_user.invalid_multi_selection"
	KeyToolRuntimeAskUserNoValidSelection       Key = "tool.runtime.ask_user.no_valid_selection"
	KeyToolRuntimeAskUserUnexpectedEnd          Key = "tool.runtime.ask_user.unexpected_end"
	KeyToolRuntimeAskUserMinQuestions           Key = "tool.runtime.ask_user.min_questions"
	KeyToolRuntimeAskUserMaxQuestions           Key = "tool.runtime.ask_user.max_questions"
	KeyToolRuntimeAskUserQuestionContext        Key = "tool.runtime.ask_user.question_context"
	KeyToolRuntimeAskUserDuplicateQuestion      Key = "tool.runtime.ask_user.duplicate_question"
	KeyToolRuntimeAskUserQuestionRequired       Key = "tool.runtime.ask_user.question_required"
	KeyToolRuntimeAskUserQuestionMarkRequired   Key = "tool.runtime.ask_user.question_mark_required"
	KeyToolRuntimeAskUserHeaderRequired         Key = "tool.runtime.ask_user.header_required"
	KeyToolRuntimeAskUserHeaderTooLong          Key = "tool.runtime.ask_user.header_too_long"
	KeyToolRuntimeAskUserMinOptions             Key = "tool.runtime.ask_user.min_options"
	KeyToolRuntimeAskUserMaxOptions             Key = "tool.runtime.ask_user.max_options"
	KeyToolRuntimeAskUserOptionContext          Key = "tool.runtime.ask_user.option_context"
	KeyToolRuntimeAskUserDuplicateOption        Key = "tool.runtime.ask_user.duplicate_option"
	KeyToolRuntimeAskUserLabelRequired          Key = "tool.runtime.ask_user.label_required"
	KeyToolRuntimeAskUserLabelTooLong           Key = "tool.runtime.ask_user.label_too_long"
	KeyToolRuntimeAskUserOtherReserved          Key = "tool.runtime.ask_user.other_reserved"
	KeyToolRuntimeAskUserPreviewMultiSelect     Key = "tool.runtime.ask_user.preview_multi_select"
	KeyToolRuntimeAskUserPreviewTooLong         Key = "tool.runtime.ask_user.preview_too_long"
)

func init() {
	addToolRuntime(KeyToolRuntimeAskUserSelectedPreview, "selected preview:\n%s", "已选预览：\n%s", "Ausgewählte Vorschau:\n%s", "選択したプレビュー:\n%s", "선택한 미리보기:\n%s", "Выбранный предпросмотр:\n%s")
	addToolRuntime(KeyToolRuntimeAskUserNotes, "user notes: %s", "用户备注：%s", "Benutzernotizen: %s", "ユーザーのメモ: %s", "사용자 메모: %s", "Заметки пользователя: %s")
	addToolRuntime(KeyToolRuntimeAskUserAnsweredContinue, "User has answered your questions: %s. You can now continue with the user's answers in mind.", "用户已回答你的问题：%s。现在可以基于这些答案继续。", "Der Benutzer hat deine Fragen beantwortet: %s. Du kannst mit diesen Antworten fortfahren.", "ユーザーが質問に回答しました: %s。この回答を踏まえて続行できます。", "사용자가 질문에 답했습니다: %s. 이 답변을 바탕으로 계속할 수 있습니다.", "Пользователь ответил на вопросы: %s. Можно продолжить с учётом этих ответов.")
	addToolRuntime(KeyToolRuntimeAskUserAnswered, "User has answered your questions: %s", "用户已回答你的问题：%s", "Der Benutzer hat deine Fragen beantwortet: %s", "ユーザーが質問に回答しました: %s", "사용자가 질문에 답했습니다: %s", "Пользователь ответил на вопросы: %s")
	addToolRuntime(KeyToolRuntimeAskUserPlanApproval, "Error: AskUserQuestion cannot ask for approval in plan mode. Use ExitPlanMode instead. Question: %q", "错误：计划模式下不能通过 AskUserQuestion 请求批准，请改用 ExitPlanMode。问题：%q", "Fehler: AskUserQuestion kann im Planungsmodus keine Genehmigung abfragen. Verwende stattdessen ExitPlanMode. Frage: %q", "エラー: plan mode では AskUserQuestion で承認を求められません。代わりに ExitPlanMode を使用してください。質問: %q", "오류: plan mode에서는 AskUserQuestion으로 승인을 요청할 수 없습니다. ExitPlanMode를 사용하세요. 질문: %q", "Ошибка: AskUserQuestion не может запрашивать одобрение в режиме планирования. Используйте ExitPlanMode. Вопрос: %q")
	addToolRuntime(KeyToolRuntimeAskUserReadAnswerFailed, "Error reading answer: %s", "读取回答时出错：%s", "Fehler beim Lesen der Antwort: %s", "回答の読み込みエラー: %s", "답변 읽기 오류: %s", "Ошибка чтения ответа: %s")
	addToolRuntime(KeyToolRuntimeAskUserInteractiveUnavailable, "The interactive question surface is unavailable. Ask the user in normal conversation instead.", "交互式提问界面不可用。请改为在普通对话中询问用户。", "Die interaktive Frageoberfläche ist nicht verfügbar. Frage stattdessen im normalen Gespräch nach.", "対話式の質問画面を利用できません。代わりに通常の会話でユーザーに質問してください。", "대화형 질문 화면을 사용할 수 없습니다. 대신 일반 대화에서 사용자에게 질문하세요.", "Интерактивный интерфейс вопросов недоступен. Задайте вопрос пользователю в обычном диалоге.")
	addToolRuntime(KeyToolRuntimeAskUserAnswersRequired, "AskUserQuestion cannot execute without one complete answer per question.", "AskUserQuestion 缺少每个问题的完整答案时无法执行。", "AskUserQuestion kann nicht ohne eine vollständige Antwort auf jede Frage ausgeführt werden.", "AskUserQuestion は、各質問への完全な回答がない場合は実行できません。", "AskUserQuestion은 각 질문에 대한 완전한 답변 없이는 실행할 수 없습니다.", "AskUserQuestion нельзя выполнить без полного ответа на каждый вопрос.")
	addToolRuntime(KeyToolRuntimeAskUserInteractionCancelled, "The user cancelled the question dialog. Do not infer an answer.", "用户取消了提问对话框。不要推断答案。", "Der Benutzer hat den Fragedialog abgebrochen. Leite keine Antwort daraus ab.", "ユーザーが質問ダイアログをキャンセルしました。回答を推測しないでください。", "사용자가 질문 대화 상자를 취소했습니다. 답변을 추측하지 마세요.", "Пользователь отменил диалог вопросов. Не пытайтесь угадать ответ.")
	addToolRuntime(KeyToolRuntimeAskUserInteractionStale, "The question dialog no longer belongs to the active session. Ask again if the answer is still needed.", "提问对话框已不属于当前会话。如仍需答案，请重新询问。", "Der Fragedialog gehört nicht mehr zur aktiven Sitzung. Frage bei Bedarf erneut.", "質問ダイアログは現在のセッションに属していません。回答がまだ必要なら、もう一度質問してください。", "질문 대화 상자가 더 이상 활성 세션에 속하지 않습니다. 답변이 여전히 필요하면 다시 질문하세요.", "Диалог вопросов больше не относится к активной сессии. При необходимости задайте вопрос снова.")
	addToolRuntime(KeyToolRuntimeAskUserInvalidSingleSelection, "Invalid selection %q: enter 1-%d or o", "选择 %q 无效：请输入 1-%d 或 o", "Ungültige Auswahl %q: Gib 1-%d oder o ein", "選択 %q は無効です。1〜%d または o を入力してください", "선택 %q이(가) 잘못되었습니다. 1-%d 또는 o를 입력하세요", "Недопустимый выбор %q: введите 1-%d или o")
	addToolRuntime(KeyToolRuntimeAskUserInvalidMultiSelection, "Invalid selection %q: enter 1-%d or an option label", "选择 %q 无效：请输入 1-%d 或选项标签", "Ungültige Auswahl %q: Gib 1-%d oder eine Optionsbezeichnung ein", "選択 %q は無効です。1〜%d または選択肢のラベルを入力してください", "선택 %q이(가) 잘못되었습니다. 1-%d 또는 옵션 레이블을 입력하세요", "Недопустимый выбор %q: введите 1-%d или метку варианта")
	addToolRuntime(KeyToolRuntimeAskUserNoValidSelection, "No valid selection was provided", "未提供有效选择", "Keine gültige Auswahl angegeben", "有効な選択がありません", "유효한 선택을 입력하지 않았습니다", "Не указан допустимый вариант")
	addToolRuntime(KeyToolRuntimeAskUserUnexpectedEnd, "Input ended unexpectedly", "输入意外结束", "Eingabe endete unerwartet", "入力が予期せず終了しました", "입력이 예기치 않게 끝났습니다", "Ввод неожиданно завершился")
	addToolRuntime(KeyToolRuntimeAskUserMinQuestions, "At least %d question is required", "至少需要 %d 个问题", "Mindestens %d Frage ist erforderlich", "質問が %d 件以上必要です", "질문이 최소 %d개 필요합니다", "Требуется не менее %d вопроса")
	addToolRuntime(KeyToolRuntimeAskUserMaxQuestions, "At most %d questions are allowed (got %d)", "最多允许 %d 个问题（收到 %d 个）", "Höchstens %d Fragen sind zulässig (%d erhalten)", "質問は最大 %d 件です（%d 件指定）", "질문은 최대 %d개까지 허용됩니다(%d개 입력)", "Допускается не более %d вопросов (получено %d)")
	addToolRuntime(KeyToolRuntimeAskUserQuestionContext, "Question %d (%q)", "问题 %d（%q）", "Frage %d (%q)", "質問 %d（%q）", "질문 %d(%q)", "Вопрос %d (%q)")
	addToolRuntime(KeyToolRuntimeAskUserDuplicateQuestion, "Question %d duplicates the text %q", "问题 %d 的文本 %q 重复", "Frage %d enthält den doppelten Text %q", "質問 %d の文面 %q が重複しています", "질문 %d의 텍스트 %q이(가) 중복됩니다", "В вопросе %d повторяется текст %q")
	addToolRuntime(KeyToolRuntimeAskUserQuestionRequired, "Question text is required", "必须提供问题文本", "Fragetext ist erforderlich", "質問文は必須です", "질문 텍스트가 필요합니다", "Требуется текст вопроса")
	addToolRuntime(KeyToolRuntimeAskUserQuestionMarkRequired, "Question text must end with ?", "问题文本必须以 ? 结尾", "Fragetext muss mit ? enden", "質問文は ? で終える必要があります", "질문 텍스트는 ?로 끝나야 합니다", "Текст вопроса должен оканчиваться знаком ?")
	addToolRuntime(KeyToolRuntimeAskUserHeaderRequired, "Header is required", "必须提供 header", "Header ist erforderlich", "header は必須です", "header가 필요합니다", "Требуется header")
	addToolRuntime(KeyToolRuntimeAskUserHeaderTooLong, "Header %q exceeds %d characters", "header %q 超过 %d 个字符", "Header %q überschreitet %d Zeichen", "header %q が %d 文字を超えています", "header %q이(가) %d자를 초과합니다", "Header %q превышает %d символов")
	addToolRuntime(KeyToolRuntimeAskUserMinOptions, "At least %d options are required (got %d)", "至少需要 %d 个选项（收到 %d 个）", "Mindestens %d Optionen sind erforderlich (%d erhalten)", "選択肢が %d 件以上必要です（%d 件指定）", "옵션이 최소 %d개 필요합니다(%d개 입력)", "Требуется не менее %d вариантов (получено %d)")
	addToolRuntime(KeyToolRuntimeAskUserMaxOptions, "At most %d options are allowed (got %d)", "最多允许 %d 个选项（收到 %d 个）", "Höchstens %d Optionen sind zulässig (%d erhalten)", "選択肢は最大 %d 件です（%d 件指定）", "옵션은 최대 %d개까지 허용됩니다(%d개 입력)", "Допускается не более %d вариантов (получено %d)")
	addToolRuntime(KeyToolRuntimeAskUserOptionContext, "Option %d (%q)", "选项 %d（%q）", "Option %d (%q)", "選択肢 %d（%q）", "옵션 %d(%q)", "Вариант %d (%q)")
	addToolRuntime(KeyToolRuntimeAskUserDuplicateOption, "Option %d duplicates label %q", "选项 %d 的标签 %q 重复", "Option %d enthält die doppelte Bezeichnung %q", "選択肢 %d のラベル %q が重複しています", "옵션 %d의 레이블 %q이(가) 중복됩니다", "В варианте %d повторяется метка %q")
	addToolRuntime(KeyToolRuntimeAskUserLabelRequired, "Option label is required", "必须提供选项标签", "Optionsbezeichnung ist erforderlich", "選択肢のラベルは必須です", "옵션 레이블이 필요합니다", "Требуется метка варианта")
	addToolRuntime(KeyToolRuntimeAskUserLabelTooLong, "Option label must be 1-5 words (got %d)", "选项标签必须为 1–5 个词（收到 %d 个）", "Optionsbezeichnung muss 1–5 Wörter lang sein (%d erhalten)", "選択肢のラベルは 1〜5 語にしてください（%d 語）", "옵션 레이블은 1~5단어여야 합니다(%d단어)", "Метка варианта должна содержать 1–5 слов (получено %d)")
	addToolRuntime(KeyToolRuntimeAskUserOtherReserved, "Other is reserved and added automatically by the UI", "Other 为保留项，由 UI 自动添加", "Other ist reserviert und wird von der UI automatisch hinzugefügt", "Other は予約済みで、UI が自動的に追加します", "Other는 예약되어 있으며 UI가 자동으로 추가합니다", "Other зарезервирован и автоматически добавляется UI")
	addToolRuntime(KeyToolRuntimeAskUserPreviewMultiSelect, "preview is not allowed for multiSelect questions", "multiSelect 问题不允许使用 preview", "preview ist bei multiSelect-Fragen nicht zulässig", "multiSelect の質問では preview を使用できません", "multiSelect 질문에는 preview를 사용할 수 없습니다", "preview недопустим для вопросов multiSelect")
	addToolRuntime(KeyToolRuntimeAskUserPreviewTooLong, "preview exceeds %d characters", "preview 超过 %d 个字符", "preview überschreitet %d Zeichen", "preview が %d 文字を超えています", "preview가 %d자를 초과합니다", "preview превышает %d символов")
}

const (
	KeyToolRuntimeWorktreeInputValidation        Key = "tool.runtime.worktree.input_validation"
	KeyToolRuntimeWorktreeNamePathExclusive      Key = "tool.runtime.worktree.name_path_exclusive"
	KeyToolRuntimeWorktreeBaseRefWithPath        Key = "tool.runtime.worktree.base_ref_with_path"
	KeyToolRuntimeWorktreePathEmpty              Key = "tool.runtime.worktree.path_empty"
	KeyToolRuntimeWorktreeCWDUnavailable         Key = "tool.runtime.worktree.cwd_unavailable"
	KeyToolRuntimeWorktreeAlreadyActive          Key = "tool.runtime.worktree.already_active"
	KeyToolRuntimeWorktreePathWithHook           Key = "tool.runtime.worktree.path_with_hook"
	KeyToolRuntimeWorktreeInvalidHookOutput      Key = "tool.runtime.worktree.invalid_hook_output"
	KeyToolRuntimeWorktreeNoRepositoryOrHook     Key = "tool.runtime.worktree.no_repository_or_hook"
	KeyToolRuntimeWorktreeResolvePath            Key = "tool.runtime.worktree.resolve_path"
	KeyToolRuntimeWorktreeRestoreFailed          Key = "tool.runtime.worktree.restore_failed"
	KeyToolRuntimeWorktreeRolledBack             Key = "tool.runtime.worktree.rolled_back"
	KeyToolRuntimeWorktreeOnBranch               Key = "tool.runtime.worktree.on_branch"
	KeyToolRuntimeWorktreeEntered                Key = "tool.runtime.worktree.entered"
	KeyToolRuntimeWorktreeNotDirectory           Key = "tool.runtime.worktree.not_directory"
	KeyToolRuntimeWorktreeHookNoStructuredOutput Key = "tool.runtime.worktree.hook_no_structured_output"
	KeyToolRuntimeWorktreeNotRegistered          Key = "tool.runtime.worktree.not_registered"
	KeyToolRuntimeWorktreeNoActiveSession        Key = "tool.runtime.worktree.no_active_session"
	KeyToolRuntimeWorktreeInvalidAction          Key = "tool.runtime.worktree.invalid_action"
	KeyToolRuntimeWorktreeRemoveEnteredPath      Key = "tool.runtime.worktree.remove_entered_path"
	KeyToolRuntimeWorktreeVerifyBeforeRemove     Key = "tool.runtime.worktree.verify_before_remove"
	KeyToolRuntimeWorktreeRestoreCWD             Key = "tool.runtime.worktree.restore_cwd"
	KeyToolRuntimeWorktreeKept                   Key = "tool.runtime.worktree.kept"
	KeyToolRuntimeWorktreeRemoved                Key = "tool.runtime.worktree.removed"
	KeyToolRuntimeWorktreeExitAlreadyRunning     Key = "tool.runtime.worktree.exit_already_running"
	KeyToolRuntimeWorktreeUncommittedFiles       Key = "tool.runtime.worktree.uncommitted_files"
	KeyToolRuntimeWorktreeBranchFallback         Key = "tool.runtime.worktree.branch_fallback"
	KeyToolRuntimeWorktreeCommitsOnBranch        Key = "tool.runtime.worktree.commits_on_branch"
	KeyToolRuntimeWorktreeDiscardConfirmation    Key = "tool.runtime.worktree.discard_confirmation"
	KeyToolRuntimeWorktreeAnd                    Key = "tool.runtime.worktree.and"
	KeyToolRuntimeWorktreeTmuxReattach           Key = "tool.runtime.worktree.tmux_reattach"
	KeyToolRuntimeWorktreeCommits                Key = "tool.runtime.worktree.commits"
	KeyToolRuntimeWorktreeDiscarded              Key = "tool.runtime.worktree.discarded"
	KeyToolRuntimeWorktreeCleanupIncomplete      Key = "tool.runtime.worktree.cleanup_incomplete"
)

func init() {
	addToolRuntime(KeyToolRuntimeWorktreeInputValidation, "<tool_use_error>InputValidationError: %v</tool_use_error>", "<tool_use_error>输入校验错误：%v</tool_use_error>", "<tool_use_error>Fehler bei der Eingabeprüfung: %v</tool_use_error>", "<tool_use_error>入力検証エラー: %v</tool_use_error>", "<tool_use_error>입력 검증 오류: %v</tool_use_error>", "<tool_use_error>Ошибка проверки ввода: %v</tool_use_error>")
	addToolRuntime(KeyToolRuntimeWorktreeNamePathExclusive, "name and path are mutually exclusive", "name 和 path 不能同时使用", "name und path schließen sich gegenseitig aus", "name と path は同時に指定できません", "name과 path는 함께 사용할 수 없습니다", "name и path взаимоисключающие")
	addToolRuntime(KeyToolRuntimeWorktreeBaseRefWithPath, "base_ref cannot be used with path", "base_ref 不能与 path 一起使用", "base_ref kann nicht mit path verwendet werden", "base_ref と path は同時に使用できません", "base_ref와 path는 함께 사용할 수 없습니다", "base_ref нельзя использовать вместе с path")
	addToolRuntime(KeyToolRuntimeWorktreePathEmpty, "Worktree path must not be empty", "worktree 路径不能为空", "Der Worktree-Pfad darf nicht leer sein", "worktree のパスは空にできません", "worktree 경로는 비워 둘 수 없습니다", "Путь worktree не должен быть пустым")
	addToolRuntime(KeyToolRuntimeWorktreeCWDUnavailable, "Session working directory is unavailable", "session 工作目录不可用", "Das Arbeitsverzeichnis der Sitzung ist nicht verfügbar", "session の作業ディレクトリを取得できません", "session 작업 디렉터리를 사용할 수 없습니다", "Рабочий каталог session недоступен")
	addToolRuntime(KeyToolRuntimeWorktreeAlreadyActive, "already in a worktree (path: %s); call ExitWorktree first", "当前已在 worktree %s 中，请先调用 ExitWorktree", "Bereits in einem Worktree unter %s; zuerst ExitWorktree aufrufen", "すでに worktree %s 内です。先に ExitWorktree を呼び出してください", "이미 worktree %s에 있습니다. 먼저 ExitWorktree를 호출하세요", "Worktree уже активен по пути %s; сначала вызовите ExitWorktree")
	addToolRuntime(KeyToolRuntimeWorktreePathWithHook, "path cannot be used with a WorktreeCreate hook", "path 不能与 WorktreeCreate hook 一起使用", "path kann nicht mit einem WorktreeCreate-Hook verwendet werden", "path と WorktreeCreate hook は同時に使用できません", "path와 WorktreeCreate hook은 함께 사용할 수 없습니다", "path нельзя использовать вместе с hook WorktreeCreate")
	addToolRuntime(KeyToolRuntimeWorktreeInvalidHookOutput, "Invalid WorktreeCreate hook output: %v; hook cleanup was attempted", "WorktreeCreate hook 输出无效：%v；已尝试执行 hook 清理", "Ungültige Ausgabe des WorktreeCreate-Hooks: %v; Hook-Bereinigung wurde versucht", "WorktreeCreate hook の出力が無効です: %v。hook のクリーンアップを試みました", "WorktreeCreate hook 출력이 잘못되었습니다: %v. hook 정리를 시도했습니다", "Недопустимый вывод hook WorktreeCreate: %v; предпринята очистка hook")
	addToolRuntime(KeyToolRuntimeWorktreeNoRepositoryOrHook, "Cannot create a worktree: this is not a git repository and no WorktreeCreate hook is configured. Configure WorktreeCreate/WorktreeRemove in settings.json for other VCS systems.", "无法创建 worktree：当前目录不是 git 仓库，且未配置 WorktreeCreate hook。其他 VCS 请在 settings.json 中配置 WorktreeCreate/WorktreeRemove。", "Worktree kann nicht erstellt werden: Kein git-Repository und kein WorktreeCreate-Hook konfiguriert. Für andere VCS WorktreeCreate/WorktreeRemove in settings.json konfigurieren.", "worktree を作成できません。git リポジトリではなく、WorktreeCreate hook も設定されていません。他の VCS では settings.json に WorktreeCreate/WorktreeRemove を設定してください。", "worktree를 만들 수 없습니다. git 저장소가 아니며 WorktreeCreate hook도 구성되지 않았습니다. 다른 VCS는 settings.json에 WorktreeCreate/WorktreeRemove를 구성하세요.", "Нельзя создать worktree: это не репозиторий git и hook WorktreeCreate не настроен. Для других VCS настройте WorktreeCreate/WorktreeRemove в settings.json.")
	addToolRuntime(KeyToolRuntimeWorktreeResolvePath, "Could not resolve path %q: %s", "无法解析路径 %q：%s", "Pfad %q konnte nicht aufgelöst werden: %s", "パス %q を解決できませんでした: %s", "경로 %q을(를) 확인할 수 없습니다: %s", "Не удалось определить путь %q: %s")
	addToolRuntime(KeyToolRuntimeWorktreeRestoreFailed, "Could not restore worktree session: %v", "无法恢复 worktree session：%v", "Worktree-Sitzung konnte nicht wiederhergestellt werden: %v", "worktree session を復元できませんでした: %v", "worktree session을 복원할 수 없습니다: %v", "Не удалось восстановить session worktree: %v")
	addToolRuntime(KeyToolRuntimeWorktreeRolledBack, "%v; the worktree was rolled back", "%v；worktree 已回滚", "%v; der Worktree wurde zurückgesetzt", "%v。worktree をロールバックしました", "%v. worktree를 롤백했습니다", "%v; выполнен откат worktree")
	addToolRuntime(KeyToolRuntimeWorktreeOnBranch, " on branch %s", "，分支为 %s", " auf Branch %s", "（ブランチ %s）", "(브랜치 %s)", " в ветке %s")
	addToolRuntime(KeyToolRuntimeWorktreeEntered, "Created worktree at %s%s. The session is now working in the worktree. Use ExitWorktree to leave mid-session, or exit the session to be prompted.", "已在 %s%s 创建 worktree，当前 session 现已切换至该 worktree。若要在 session 中途离开，请使用 ExitWorktree；退出 session 时也会收到提示。", "Worktree unter %s%s erstellt. Die Sitzung verwendet ihn jetzt. Zum Verlassen während der Sitzung ExitWorktree verwenden; beim Beenden der Sitzung erfolgt eine Nachfrage.", "%s%s に worktree を作成し、この session で使用を開始しました。session の途中で離れるには ExitWorktree を使用してください。session の終了時にも確認されます。", "%s%s에 worktree를 만들었으며 현재 session이 이 worktree를 사용합니다. session 도중 나가려면 ExitWorktree를 사용하세요. session 종료 시에도 확인합니다.", "Worktree создан по пути %s%s. Session теперь использует его. Чтобы выйти во время session, используйте ExitWorktree; при завершении session также появится запрос.")
	addToolRuntime(KeyToolRuntimeWorktreeNotDirectory, "%q is not a directory", "%q 不是目录", "%q ist kein Verzeichnis", "%q はディレクトリではありません", "%q은(는) 디렉터리가 아닙니다", "%q не является каталогом")
	addToolRuntime(KeyToolRuntimeWorktreeHookNoStructuredOutput, "Hook bridge %s did not return structured output", "hook bridge %s 未返回结构化输出", "Hook-Bridge %s hat keine strukturierte Ausgabe zurückgegeben", "hook bridge %s が構造化出力を返しませんでした", "hook bridge %s이(가) 구조화된 출력을 반환하지 않았습니다", "Hook bridge %s не вернул структурированный результат")
	addToolRuntime(KeyToolRuntimeWorktreeNotRegistered, "%q is not a registered worktree for this repository; run git worktree list to view registered worktrees", "%q 不是该仓库已注册的 worktree；运行 git worktree list 可查看已注册的 worktree", "%q ist kein registrierter Worktree dieses Repositorys; registrierte Worktrees mit git worktree list anzeigen", "%q はこのリポジトリに登録された worktree ではありません。git worktree list で確認してください", "%q은(는) 이 저장소에 등록된 worktree가 아닙니다. git worktree list로 확인하세요", "%q не зарегистрирован как worktree этого репозитория; список: git worktree list")
	addToolRuntime(KeyToolRuntimeWorktreeNoActiveSession, "No-op: there is no active EnterWorktree session. ExitWorktree only affects worktrees entered in the current session; it does not touch manually created or earlier worktrees. No filesystem changes were made.", "无需操作：当前没有活动的 EnterWorktree session。ExitWorktree 只处理当前 session 中进入的 worktree，不会触碰手动创建或之前的 worktree。未更改文件系统。", "Keine Aktion: Es gibt keine aktive EnterWorktree-Sitzung. ExitWorktree betrifft nur Worktrees der aktuellen Sitzung und berührt weder manuell erstellte noch frühere Worktrees. Das Dateisystem wurde nicht geändert.", "処理は不要です。有効な EnterWorktree session がありません。ExitWorktree は現在の session で入った worktree のみを対象とし、手動作成または以前の worktree には触れません。ファイルシステムは変更されていません。", "작업 없음: 활성 EnterWorktree session이 없습니다. ExitWorktree는 현재 session에서 들어간 worktree만 처리하며 수동 생성 또는 이전 worktree는 건드리지 않습니다. 파일 시스템은 변경되지 않았습니다.", "Действие не требуется: активная session EnterWorktree отсутствует. ExitWorktree затрагивает только worktree текущей session и не меняет созданные вручную или прежние worktree. Файловая система не изменена.")
	addToolRuntime(KeyToolRuntimeWorktreeInvalidAction, "action must be keep or remove; got %q", "action 必须是 keep 或 remove；收到 %q", "action muss keep oder remove sein; erhalten: %q", "action は keep または remove である必要があります。入力: %q", "action은 keep 또는 remove여야 합니다. 입력: %q", "action должен быть keep или remove; получено %q")
	addToolRuntime(KeyToolRuntimeWorktreeRemoveEnteredPath, "This worktree was entered by path rather than created by EnterWorktree. Use action=keep and remove it manually with git worktree remove.", "该 worktree 是通过 path 进入的，并非由 EnterWorktree 创建。请使用 action=keep，并通过 git worktree remove 手动移除。", "Dieser Worktree wurde per Pfad betreten und nicht von EnterWorktree erstellt. action=keep verwenden und manuell mit git worktree remove entfernen.", "この worktree は EnterWorktree で作成されたものではなく、path から入りました。action=keep を使用し、git worktree remove で手動削除してください。", "이 worktree는 EnterWorktree가 만든 것이 아니라 path로 들어갔습니다. action=keep을 사용하고 git worktree remove로 직접 제거하세요.", "Этот worktree был открыт по пути, а не создан EnterWorktree. Используйте action=keep и удалите его вручную через git worktree remove.")
	addToolRuntime(KeyToolRuntimeWorktreeVerifyBeforeRemove, "Could not verify worktree state at %s. Removal requires explicit confirmation: retry with discard_changes=true, or use action=keep to preserve it.", "无法验证 %s 处的 worktree 状态。移除前需要明确确认：请使用 discard_changes=true 重试，或使用 action=keep 保留。", "Worktree-Status unter %s konnte nicht geprüft werden. Zum Entfernen ist eine ausdrückliche Bestätigung nötig: mit discard_changes=true erneut versuchen oder mit action=keep erhalten.", "%s の worktree 状態を確認できませんでした。削除には明示的な確認が必要です。discard_changes=true で再試行するか、action=keep で保持してください。", "%s의 worktree 상태를 확인할 수 없습니다. 제거하려면 명시적인 확인이 필요합니다. discard_changes=true로 다시 시도하거나 action=keep으로 보존하세요.", "Не удалось проверить состояние worktree по пути %s. Для удаления требуется явное подтверждение: повторите с discard_changes=true или сохраните через action=keep.")
	addToolRuntime(KeyToolRuntimeWorktreeRestoreCWD, "Could not restore the session working directory: %v", "无法恢复 session 工作目录：%v", "Arbeitsverzeichnis der Sitzung konnte nicht wiederhergestellt werden: %v", "session の作業ディレクトリを復元できませんでした: %v", "session 작업 디렉터리를 복원할 수 없습니다: %v", "Не удалось восстановить рабочий каталог session: %v")
	addToolRuntime(KeyToolRuntimeWorktreeKept, "Exited the worktree. Work is preserved at %s%s. The session is back in %s.%s", "已退出 worktree。工作内容保留在 %s%s。session 已返回 %s。%s", "Worktree verlassen. Die Arbeit bleibt unter %s%s erhalten. Die Sitzung ist wieder in %s.%s", "worktree を終了しました。作業内容は %s%s に保持されています。session は %s に戻りました。%s", "worktree를 나갔습니다. 작업은 %s%s에 보존되었습니다. session은 %s로 돌아왔습니다.%s", "Выход из worktree выполнен. Работа сохранена в %s%s. Session вернулась в %s.%s")
	addToolRuntime(KeyToolRuntimeWorktreeRemoved, "Exited and removed the worktree at %s.%s The session is back in %s.", "已退出并移除 %s 处的 worktree。%s session 已返回 %s。", "Worktree unter %s verlassen und entfernt.%s Die Sitzung ist wieder in %s.", "%s の worktree を終了して削除しました。%s session は %s に戻りました。", "%s의 worktree를 나가고 제거했습니다.%s session은 %s로 돌아왔습니다.", "Выполнен выход и удаление worktree по пути %s.%s Session вернулась в %s.")
	addToolRuntime(KeyToolRuntimeWorktreeExitAlreadyRunning, "ExitWorktree is already running for this session", "当前 session 已在运行 ExitWorktree", "ExitWorktree wird für diese Sitzung bereits ausgeführt", "この session では ExitWorktree がすでに実行中です", "이 session에서 ExitWorktree가 이미 실행 중입니다", "ExitWorktree уже выполняется для этой session")
	addToolRuntime(KeyToolRuntimeWorktreeUncommittedFiles, "%d uncommitted file(s)", "%d 个未提交文件", "%d nicht festgeschriebene Datei(en)", "未コミットのファイル %d 件", "커밋되지 않은 파일 %d개", "%d незакоммиченных файлов")
	addToolRuntime(KeyToolRuntimeWorktreeBranchFallback, "the worktree branch", "worktree 分支", "dem Worktree-Branch", "worktree ブランチ", "worktree 브랜치", "ветке worktree")
	addToolRuntime(KeyToolRuntimeWorktreeCommitsOnBranch, "%d commit(s) on %s", "%d 个 commit，位于 %s", "%d Commit(s) auf %s", "%d 件の commit（%s）", "%d개 commit(%s)", "%d commit в %s")
	addToolRuntime(KeyToolRuntimeWorktreeDiscardConfirmation, "The worktree has %s. Removing it will permanently discard this work. Confirm with the user, then retry with discard_changes=true; use action=keep to preserve it.", "该 worktree 包含%s。移除将永久丢弃这些工作。请向用户确认后使用 discard_changes=true 重试；如需保留，请使用 action=keep。", "Der Worktree enthält %s. Beim Entfernen geht diese Arbeit dauerhaft verloren. Nach Bestätigung mit discard_changes=true erneut versuchen; mit action=keep erhalten.", "worktree には%sがあります。削除するとこの作業は完全に失われます。ユーザーの確認後に discard_changes=true で再試行してください。保持するには action=keep を使用します。", "worktree에 %s이(가) 있습니다. 제거하면 이 작업이 영구 삭제됩니다. 사용자 확인 후 discard_changes=true로 다시 시도하세요. 보존하려면 action=keep을 사용합니다.", "Worktree содержит %s. Удаление безвозвратно уничтожит эту работу. После подтверждения повторите с discard_changes=true; для сохранения используйте action=keep.")
	addToolRuntime(KeyToolRuntimeWorktreeAnd, " and ", "和", " und ", "と", " 및 ", " и ")
	addToolRuntime(KeyToolRuntimeWorktreeTmuxReattach, " Tmux session %s is still running; reattach with: tmux attach -t %s", " Tmux session %s 仍在运行；可使用以下命令重新连接：tmux attach -t %s", " Tmux-Sitzung %s läuft weiter; erneut verbinden mit: tmux attach -t %s", " Tmux session %s は引き続き実行中です。再接続: tmux attach -t %s", " Tmux session %s이(가) 계속 실행 중입니다. 다시 연결: tmux attach -t %s", " Session Tmux %s продолжает работать; подключение: tmux attach -t %s")
	addToolRuntime(KeyToolRuntimeWorktreeCommits, "%d commit(s)", "%d 个 commit", "%d Commit(s)", "commit %d 件", "commit %d개", "%d commit")
	addToolRuntime(KeyToolRuntimeWorktreeDiscarded, " Discarded %s.", " 已丢弃%s。", " %s verworfen.", " %sを破棄しました。", " %s을(를) 버렸습니다.", " Отброшено: %s.")
	addToolRuntime(KeyToolRuntimeWorktreeCleanupIncomplete, "Exited the worktree at %s, but cleanup did not finish completely. The session is back in %s. Verify the worktree and branch before continuing.", "已退出 %s 处的 worktree，但清理未完全完成。session 已返回 %s。继续前请核验该 worktree 和分支。", "Worktree unter %s verlassen, die Bereinigung wurde jedoch nicht vollständig abgeschlossen. Die Sitzung ist wieder in %s. Prüfen Sie Worktree und Branch, bevor Sie fortfahren.", "%s の worktree を終了しましたが、クリーンアップは完全には完了しませんでした。session は %s に戻りました。続行する前に worktree とブランチを確認してください。", "%s의 worktree를 나갔지만 정리가 완전히 끝나지 않았습니다. session은 %s로 돌아왔습니다. 계속하기 전에 worktree와 브랜치를 확인하세요.", "Выход из worktree по пути %s выполнен, но очистка завершилась не полностью. Session вернулась в %s. Перед продолжением проверьте worktree и ветку.")
}

const (
	KeyToolRuntimeDangerousRootDelete            Key = "tool.runtime.safety.reason.root_delete"
	KeyToolRuntimeDangerousVariableDelete        Key = "tool.runtime.safety.reason.variable_delete"
	KeyToolRuntimeDangerousFilesystemFormat      Key = "tool.runtime.safety.reason.filesystem_format"
	KeyToolRuntimeDangerousDirectDiskWrite       Key = "tool.runtime.safety.reason.direct_disk_write"
	KeyToolRuntimeDangerousVariableDiskWrite     Key = "tool.runtime.safety.reason.variable_disk_write"
	KeyToolRuntimeDangerousChmodRoot             Key = "tool.runtime.safety.reason.chmod_root"
	KeyToolRuntimeDangerousProcessSubstitution   Key = "tool.runtime.safety.reason.process_substitution"
	KeyToolRuntimeDangerousLanguageOneLiner      Key = "tool.runtime.safety.reason.language_one_liner"
	KeyToolRuntimeDangerousRemotePipe            Key = "tool.runtime.safety.reason.remote_pipe"
	KeyToolRuntimeDangerousBase64Pipe            Key = "tool.runtime.safety.reason.base64_pipe"
	KeyToolRuntimeDangerousRawDeviceRedirect     Key = "tool.runtime.safety.reason.raw_device_redirect"
	KeyToolRuntimeDangerousProtectedWrite        Key = "tool.runtime.safety.reason.protected_write"
	KeyToolRuntimeDangerousTeeProtectedWrite     Key = "tool.runtime.safety.reason.tee_protected_write"
	KeyToolRuntimeDangerousSedProtectedEdit      Key = "tool.runtime.safety.reason.sed_protected_edit"
	KeyToolRuntimeDangerousCommandProtectedWrite Key = "tool.runtime.safety.reason.command_protected_write"
	KeyToolRuntimeDangerousMoveProtectedWrite    Key = "tool.runtime.safety.reason.move_protected_write"
	KeyToolRuntimeDangerousMoveProtectedSource   Key = "tool.runtime.safety.reason.move_protected_source"
	KeyToolRuntimeDangerousSCPProtectedWrite     Key = "tool.runtime.safety.reason.scp_protected_write"
	KeyToolRuntimeDangerousRsyncProtectedWrite   Key = "tool.runtime.safety.reason.rsync_protected_write"
	KeyToolRuntimeDangerousDDProtectedWrite      Key = "tool.runtime.safety.reason.dd_protected_write"
	KeyToolRuntimeDangerousTruncateProtected     Key = "tool.runtime.safety.reason.truncate_protected"
	KeyToolRuntimeDangerousForkBomb              Key = "tool.runtime.safety.reason.fork_bomb"
	KeyToolRuntimeDangerousPipeToShell           Key = "tool.runtime.safety.reason.pipe_to_shell"
	KeyToolRuntimeDangerousBase64ToShell         Key = "tool.runtime.safety.reason.base64_to_shell"
	KeyToolRuntimeDangerousReverseShell          Key = "tool.runtime.safety.reason.reverse_shell"
	KeyToolRuntimeDangerousScriptingOneLiner     Key = "tool.runtime.safety.reason.scripting_one_liner"
)

func init() {
	addToolRuntime(KeyToolRuntimeDangerousRootDelete, "Recursive forced deletion of a root path", "递归强制删除根路径", "Rekursives erzwungenes Löschen eines Root-Pfads", "ルートパスの再帰的な強制削除", "루트 경로 강제 재귀 삭제", "Рекурсивное принудительное удаление корневого пути")
	addToolRuntime(KeyToolRuntimeDangerousVariableDelete, "Recursive forced deletion uses variable expansion; the target cannot be verified", "递归强制删除使用了变量展开，无法验证目标", "Rekursives erzwungenes Löschen verwendet Variablenexpansion; Ziel nicht prüfbar", "再帰的な強制削除に変数展開が含まれ、対象を確認できません", "강제 재귀 삭제에 변수 확장이 사용되어 대상을 확인할 수 없습니다", "Рекурсивное принудительное удаление использует подстановку переменных; цель нельзя проверить")
	addToolRuntime(KeyToolRuntimeDangerousFilesystemFormat, "Filesystem formatting command", "文件系统格式化命令", "Befehl zum Formatieren eines Dateisystems", "ファイルシステムのフォーマットコマンド", "파일 시스템 포맷 명령", "Команда форматирования файловой системы")
	addToolRuntime(KeyToolRuntimeDangerousDirectDiskWrite, "Direct disk write with dd", "使用 dd 直接写入磁盘", "Direkter Schreibzugriff auf Datenträger mit dd", "dd によるディスクへの直接書き込み", "dd를 사용한 디스크 직접 쓰기", "Прямая запись на диск через dd")
	addToolRuntime(KeyToolRuntimeDangerousVariableDiskWrite, "Direct disk write with dd uses variable expansion", "使用 dd 直接写入磁盘，且目标使用了变量展开", "Direkter Schreibzugriff mit dd verwendet Variablenexpansion", "dd による直接書き込みに変数展開が含まれています", "dd 직접 쓰기에 변수 확장이 사용됩니다", "Прямая запись через dd использует подстановку переменных")
	addToolRuntime(KeyToolRuntimeDangerousChmodRoot, "chmod 777 on root paths", "对根路径执行 chmod 777", "chmod 777 auf Root-Pfaden", "ルートパスへの chmod 777", "루트 경로에 chmod 777 실행", "chmod 777 для корневых путей")
	addToolRuntime(KeyToolRuntimeDangerousProcessSubstitution, "Remote code execution through process substitution", "通过进程替换执行远程代码", "Remote-Codeausführung durch Prozesssubstitution", "プロセス置換によるリモートコード実行", "프로세스 치환을 통한 원격 코드 실행", "Удалённое выполнение кода через подстановку процесса")
	addToolRuntime(KeyToolRuntimeDangerousLanguageOneLiner, "Destructive operation in a %s one-liner", "%s 单行命令中的破坏性操作", "Destruktive Operation in einem %s-Einzeiler", "%s のワンライナーによる破壊的操作", "%s 원라이너의 파괴적 작업", "Разрушительная операция в однострочнике %s")
	addToolRuntime(KeyToolRuntimeDangerousRemotePipe, "Remote code execution by piping %s to %s", "通过管道将 %s 传给 %s 来执行远程代码", "Remote-Codeausführung durch Weiterleitung von %s an %s", "%s から %s へのパイプによるリモートコード実行", "%s 출력을 %s에 파이프로 전달하는 원격 코드 실행", "Удалённое выполнение кода через канал из %s в %s")
	addToolRuntime(KeyToolRuntimeDangerousBase64Pipe, "Encoded payload execution through a base64 pipe to %s", "通过 base64 管道将编码 payload 传给 %s 执行", "Ausführung einer codierten Nutzlast über eine Base64-Pipe an %s", "base64 パイプから %s へのエンコード済み payload の実行", "base64 파이프를 통해 %s에서 인코딩된 payload 실행", "Выполнение закодированной нагрузки через base64-канал в %s")
	addToolRuntime(KeyToolRuntimeDangerousRawDeviceRedirect, "Redirect to a raw block device", "重定向到原始块设备", "Umleitung auf ein Blockgerät", "ブロックデバイスへのリダイレクト", "원시 블록 장치로 리디렉션", "Перенаправление на блочное устройство")
	addToolRuntime(KeyToolRuntimeDangerousProtectedWrite, "Write to protected path: %s", "写入受保护路径：%s", "Schreiben in geschützten Pfad: %s", "保護対象パスへの書き込み: %s", "보호된 경로에 쓰기: %s", "Запись в защищённый путь: %s")
	addToolRuntime(KeyToolRuntimeDangerousTeeProtectedWrite, "tee writes to protected path: %s", "tee 写入受保护路径：%s", "tee schreibt in geschützten Pfad: %s", "tee による保護対象パスへの書き込み: %s", "tee가 보호된 경로에 씁니다: %s", "tee записывает в защищённый путь: %s")
	addToolRuntime(KeyToolRuntimeDangerousSedProtectedEdit, "sed edits a protected path in place: %s", "sed 原地编辑受保护路径：%s", "sed bearbeitet geschützten Pfad direkt: %s", "sed による保護対象パスのインプレース編集: %s", "sed가 보호된 경로를 제자리 편집합니다: %s", "sed изменяет защищённый путь на месте: %s")
	addToolRuntime(KeyToolRuntimeDangerousCommandProtectedWrite, "%s writes to protected path: %s", "%s 写入受保护路径：%s", "%s schreibt in geschützten Pfad: %s", "%s による保護対象パスへの書き込み: %s", "%s이(가) 보호된 경로에 씁니다: %s", "%s записывает в защищённый путь: %s")
	addToolRuntime(KeyToolRuntimeDangerousMoveProtectedWrite, "mv writes to protected path: %s", "mv 写入受保护路径：%s", "mv schreibt in geschützten Pfad: %s", "mv による保護対象パスへの書き込み: %s", "mv가 보호된 경로에 씁니다: %s", "mv записывает в защищённый путь: %s")
	addToolRuntime(KeyToolRuntimeDangerousMoveProtectedSource, "mv removes content from protected path: %s", "mv 从受保护路径移出内容：%s", "mv entfernt Inhalt aus geschütztem Pfad: %s", "mv による保護対象パスからの移動: %s", "mv가 보호된 경로에서 콘텐츠를 이동합니다: %s", "mv перемещает содержимое из защищённого пути: %s")
	addToolRuntime(KeyToolRuntimeDangerousSCPProtectedWrite, "scp writes to protected path: %s", "scp 写入受保护路径：%s", "scp schreibt in geschützten Pfad: %s", "scp による保護対象パスへの書き込み: %s", "scp가 보호된 경로에 씁니다: %s", "scp записывает в защищённый путь: %s")
	addToolRuntime(KeyToolRuntimeDangerousRsyncProtectedWrite, "rsync writes to protected path: %s", "rsync 写入受保护路径：%s", "rsync schreibt in geschützten Pfad: %s", "rsync による保護対象パスへの書き込み: %s", "rsync가 보호된 경로에 씁니다: %s", "rsync записывает в защищённый путь: %s")
	addToolRuntime(KeyToolRuntimeDangerousDDProtectedWrite, "dd writes to protected path: %s", "dd 写入受保护路径：%s", "dd schreibt in geschützten Pfad: %s", "dd による保護対象パスへの書き込み: %s", "dd가 보호된 경로에 씁니다: %s", "dd записывает в защищённый путь: %s")
	addToolRuntime(KeyToolRuntimeDangerousTruncateProtected, "truncate modifies protected path: %s", "truncate 修改受保护路径：%s", "truncate ändert geschützten Pfad: %s", "truncate による保護対象パスの変更: %s", "truncate가 보호된 경로를 변경합니다: %s", "truncate изменяет защищённый путь: %s")
	addToolRuntime(KeyToolRuntimeDangerousForkBomb, "Fork bomb", "fork bomb", "Fork-Bombe", "fork bomb", "fork bomb", "Fork-бомба")
	addToolRuntime(KeyToolRuntimeDangerousPipeToShell, "Remote code execution through a pipe to a shell", "通过管道传给 shell 执行远程代码", "Remote-Codeausführung über eine Pipe an eine Shell", "shell へのパイプによるリモートコード実行", "shell로 파이프를 전달하는 원격 코드 실행", "Удалённое выполнение кода через канал в shell")
	addToolRuntime(KeyToolRuntimeDangerousBase64ToShell, "Encoded payload execution through a base64 pipe to a shell", "通过 base64 管道传给 shell 执行编码 payload", "Ausführung einer codierten Nutzlast über Base64-Pipe an eine Shell", "base64 パイプから shell へのエンコード済み payload の実行", "base64 파이프를 통해 shell에서 인코딩된 payload 실행", "Выполнение закодированной нагрузки через base64-канал в shell")
	addToolRuntime(KeyToolRuntimeDangerousReverseShell, "Potential reverse shell", "疑似 reverse shell", "Mögliche Reverse-Shell", "reverse shell の可能性", "reverse shell 가능성", "Возможная reverse shell")
	addToolRuntime(KeyToolRuntimeDangerousScriptingOneLiner, "Destructive scripting one-liner", "破坏性脚本单行命令", "Destruktiver Skript-Einzeiler", "破壊的なスクリプトのワンライナー", "파괴적 스크립트 원라이너", "Разрушительный однострочный скрипт")
}

const (
	KeyToolRuntimeBashNoMatches       Key = "tool.runtime.bash.semantic.no_matches"
	KeyToolRuntimeBashInvalidPattern  Key = "tool.runtime.bash.semantic.invalid_pattern"
	KeyToolRuntimeBashFindPartial     Key = "tool.runtime.bash.semantic.find_partial"
	KeyToolRuntimeBashFilesDiffer     Key = "tool.runtime.bash.semantic.files_differ"
	KeyToolRuntimeBashDiffTrouble     Key = "tool.runtime.bash.semantic.diff_trouble"
	KeyToolRuntimeBashConditionFalse  Key = "tool.runtime.bash.semantic.condition_false"
	KeyToolRuntimeBashGitNonFatal     Key = "tool.runtime.bash.semantic.git_non_fatal"
	KeyToolRuntimeBashMakeFailed      Key = "tool.runtime.bash.semantic.make_failed"
	KeyToolRuntimeBashTputUnsupported Key = "tool.runtime.bash.semantic.tput_unsupported"
	KeyToolRuntimeBashNoFilterMatches Key = "tool.runtime.bash.semantic.no_filter_matches"
)

func init() {
	addToolRuntime(KeyToolRuntimeBashNoMatches, "%s exit 1: no matches found (not an error)", "%s 退出码 1：未找到匹配项（并非错误）", "%s Exit-Code 1: Keine Treffer gefunden (kein Fehler)", "%s 終了コード 1: 一致なし（エラーではありません）", "%s 종료 코드 1: 일치 항목 없음(오류 아님)", "%s, код выхода 1: совпадения не найдены (не ошибка)")
	addToolRuntime(KeyToolRuntimeBashInvalidPattern, "%s exit 2: invalid pattern, missing file, or syntax error", "%s 退出码 2：pattern 无效、文件缺失或语法错误", "%s Exit-Code 2: Ungültiges Muster, fehlende Datei oder Syntaxfehler", "%s 終了コード 2: pattern が無効、ファイルがない、または構文エラー", "%s 종료 코드 2: 잘못된 pattern, 파일 누락 또는 구문 오류", "%s, код выхода 2: недопустимый pattern, отсутствующий файл или синтаксическая ошибка")
	addToolRuntime(KeyToolRuntimeBashFindPartial, "find exit 1: some paths were inaccessible; other search results are valid", "find 退出码 1：部分路径无法访问；其余搜索结果有效", "find Exit-Code 1: Einige Pfade waren nicht zugänglich; übrige Suchergebnisse sind gültig", "find 終了コード 1: 一部のパスにアクセスできませんでしたが、他の検索結果は有効です", "find 종료 코드 1: 일부 경로에 접근할 수 없었지만 나머지 검색 결과는 유효합니다", "find, код выхода 1: некоторые пути недоступны; остальные результаты поиска действительны")
	addToolRuntime(KeyToolRuntimeBashFilesDiffer, "%s exit 1: files differ (not an error)", "%s 退出码 1：文件不同（并非错误）", "%s Exit-Code 1: Dateien unterscheiden sich (kein Fehler)", "%s 終了コード 1: ファイルが異なります（エラーではありません）", "%s 종료 코드 1: 파일이 다름(오류 아님)", "%s, код выхода 1: файлы различаются (не ошибка)")
	addToolRuntime(KeyToolRuntimeBashDiffTrouble, "%s exit 2: missing file or unreadable input", "%s 退出码 2：文件缺失或输入无法读取", "%s Exit-Code 2: Fehlende Datei oder nicht lesbare Eingabe", "%s 終了コード 2: ファイルがない、または入力を読み取れません", "%s 종료 코드 2: 파일 누락 또는 입력을 읽을 수 없음", "%s, код выхода 2: отсутствующий файл или нечитаемый ввод")
	addToolRuntime(KeyToolRuntimeBashConditionFalse, "%s exit 1: condition is false (not an error)", "%s 退出码 1：条件为 false（并非错误）", "%s Exit-Code 1: Bedingung ist falsch (kein Fehler)", "%s 終了コード 1: 条件が false（エラーではありません）", "%s 종료 코드 1: 조건이 false(오류 아님)", "%s, код выхода 1: условие ложно (не ошибка)")
	addToolRuntime(KeyToolRuntimeBashGitNonFatal, "git exit 1: the subcommand reported a non-fatal status; this is not always an error", "git 退出码 1：子命令报告了非致命状态；这并不一定是错误", "git Exit-Code 1: Der Unterbefehl meldete einen nicht fatalen Status; nicht immer ein Fehler", "git 終了コード 1: サブコマンドが致命的でない状態を返しました。必ずしもエラーではありません", "git 종료 코드 1: 하위 명령이 치명적이지 않은 상태를 보고했습니다. 항상 오류인 것은 아닙니다", "git, код выхода 1: подкоманда сообщила некритический статус; это не всегда ошибка")
	addToolRuntime(KeyToolRuntimeBashMakeFailed, "make exit 2: build failed", "make 退出码 2：构建失败", "make Exit-Code 2: Build fehlgeschlagen", "make 終了コード 2: ビルド失敗", "make 종료 코드 2: 빌드 실패", "make, код выхода 2: сборка завершилась с ошибкой")
	addToolRuntime(KeyToolRuntimeBashTputUnsupported, "tput exit 1: the terminal does not support the requested capability", "tput 退出码 1：终端不支持所请求的 capability", "tput Exit-Code 1: Das Terminal unterstützt die angeforderte Funktion nicht", "tput 終了コード 1: 端末は要求された capability に対応していません", "tput 종료 코드 1: 터미널이 요청된 capability를 지원하지 않습니다", "tput, код выхода 1: терминал не поддерживает запрошенную capability")
	addToolRuntime(KeyToolRuntimeBashNoFilterMatches, "%s exit 1: no values matched the filter (not an error)", "%s 退出码 1：没有值匹配 filter（并非错误）", "%s Exit-Code 1: Keine Werte entsprechen dem Filter (kein Fehler)", "%s 終了コード 1: filter に一致する値がありません（エラーではありません）", "%s 종료 코드 1: filter와 일치하는 값 없음(오류 아님)", "%s, код выхода 1: значения не соответствуют filter (не ошибка)")
}

const (
	KeyToolRuntimeMCPDynamicNotInitialized   Key = "tool.runtime.mcp.dynamic_not_initialized"
	KeyToolRuntimeMCPToolCallFailedPlain     Key = "tool.runtime.mcp.tool_call_failed_plain"
	KeyToolRuntimeMCPSessionRecoveryFailed   Key = "tool.runtime.mcp.session_recovery_failed"
	KeyToolRuntimeMCPServerUnavailableReason Key = "tool.runtime.mcp.server_unavailable_reason"
	KeyToolRuntimeMCPServerDisabled          Key = "tool.runtime.mcp.server_disabled"
	KeyToolRuntimeMCPFailedToConnect         Key = "tool.runtime.mcp.failed_to_connect"
	KeyToolRuntimeMCPServerConnectFailed     Key = "tool.runtime.mcp.server_connect_failed"
	KeyToolRuntimeMCPServerConnecting        Key = "tool.runtime.mcp.server_connecting"
	KeyToolRuntimeMCPServerNotConnected      Key = "tool.runtime.mcp.server_not_connected"
	KeyToolRuntimeMCPServerNeedsAuth         Key = "tool.runtime.mcp.server_needs_auth"
	KeyToolRuntimeMCPCallTimedOut            Key = "tool.runtime.mcp.call_timed_out"
)

func init() {
	addToolRuntime(KeyToolRuntimeMCPDynamicNotInitialized, "Error: dynamic MCP tool is not initialized", "错误：动态 MCP tool 尚未初始化", "Fehler: Dynamisches MCP-Tool ist nicht initialisiert", "エラー: 動的 MCP tool が初期化されていません", "오류: 동적 MCP tool이 초기화되지 않았습니다", "Ошибка: динамический MCP tool не инициализирован")
	addToolRuntime(KeyToolRuntimeMCPToolCallFailedPlain, "MCP tool call failed", "MCP tool 调用失败", "MCP-Tool-Aufruf fehlgeschlagen", "MCP tool の呼び出しに失敗しました", "MCP tool 호출 실패", "Вызов MCP tool завершился с ошибкой")
	addToolRuntime(KeyToolRuntimeMCPSessionRecoveryFailed, "Error: MCP session recovery failed for %q: %s", "错误：无法恢复 %q 的 MCP session：%s", "Fehler: Wiederherstellung der MCP-Sitzung für %q fehlgeschlagen: %s", "エラー: %q の MCP session を復元できませんでした: %s", "오류: %q의 MCP session 복구 실패: %s", "Ошибка: не удалось восстановить session MCP для %q: %s")
	addToolRuntime(KeyToolRuntimeMCPServerUnavailableReason, "Error: MCP server %q is unavailable: %s", "错误：MCP server %q 不可用：%s", "Fehler: MCP-Server %q ist nicht verfügbar: %s", "エラー: MCP server %q は利用できません: %s", "오류: MCP server %q을(를) 사용할 수 없습니다: %s", "Ошибка: MCP server %q недоступен: %s")
	addToolRuntime(KeyToolRuntimeMCPServerDisabled, "Error: MCP server %q is disabled", "错误：MCP server %q 已禁用", "Fehler: MCP-Server %q ist deaktiviert", "エラー: MCP server %q は無効です", "오류: MCP server %q이(가) 비활성화되었습니다", "Ошибка: MCP server %q отключён")
	addToolRuntime(KeyToolRuntimeMCPFailedToConnect, "failed to connect", "连接失败", "Verbindung fehlgeschlagen", "接続に失敗しました", "연결 실패", "не удалось подключиться")
	addToolRuntime(KeyToolRuntimeMCPServerConnectFailed, "Error: MCP server %q failed to connect: %s", "错误：MCP server %q 连接失败：%s", "Fehler: Verbindung zu MCP-Server %q fehlgeschlagen: %s", "エラー: MCP server %q への接続に失敗しました: %s", "오류: MCP server %q 연결 실패: %s", "Ошибка: не удалось подключиться к MCP server %q: %s")
	addToolRuntime(KeyToolRuntimeMCPServerConnecting, "Error: MCP server %q is still connecting; retry shortly", "错误：MCP server %q 仍在连接，请稍后重试", "Fehler: MCP-Server %q stellt noch die Verbindung her; in Kürze erneut versuchen", "エラー: MCP server %q は接続中です。少し待って再試行してください", "오류: MCP server %q이(가) 연결 중입니다. 잠시 후 다시 시도하세요", "Ошибка: MCP server %q всё ещё подключается; повторите попытку позже")
	addToolRuntime(KeyToolRuntimeMCPServerNotConnected, "Error: MCP server %q is not connected", "错误：MCP server %q 未连接", "Fehler: MCP-Server %q ist nicht verbunden", "エラー: MCP server %q は未接続です", "오류: MCP server %q이(가) 연결되지 않았습니다", "Ошибка: MCP server %q не подключён")
	addToolRuntime(KeyToolRuntimeMCPServerNeedsAuth, "Error: MCP server %q requires authentication. Call %s first.", "错误：MCP server %q 需要身份验证，请先调用 %s。", "Fehler: MCP-Server %q erfordert Authentifizierung. Zuerst %s aufrufen.", "エラー: MCP server %q には認証が必要です。先に %s を呼び出してください。", "오류: MCP server %q에 인증이 필요합니다. 먼저 %s을(를) 호출하세요.", "Ошибка: MCP server %q требует аутентификации. Сначала вызовите %s.")
	addToolRuntime(KeyToolRuntimeMCPCallTimedOut, "Error: MCP call %s/%s timed out after %s", "错误：MCP 调用 %s/%s 在 %s 后超时", "Fehler: Zeitüberschreitung beim MCP-Aufruf %s/%s nach %s", "エラー: MCP 呼び出し %s/%s は %s 後にタイムアウトしました", "오류: MCP 호출 %s/%s이(가) %s 후 시간 초과되었습니다", "Ошибка: вызов MCP %s/%s превысил время ожидания %s")
}
