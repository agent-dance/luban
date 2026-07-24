package i18n

const (
	KeyCommandPresentationWait                 Key = "command.presentation.wait"
	KeyCommandPresentationInspectResult        Key = "command.presentation.inspect_result"
	KeyCommandPresentationInspectError         Key = "command.presentation.inspect_error"
	KeyCommandPresentationExitRequested        Key = "command.presentation.exit_requested"
	KeyCommandPresentationCompleted            Key = "command.presentation.completed"
	KeyCommandPresentationResult               Key = "command.presentation.result"
	KeyCommandPresentationExtensionSuccess     Key = "command.presentation.extension.success"
	KeyCommandPresentationExtensionFailure     Key = "command.presentation.extension.failure"
	KeyCommandPresentationMCPPromptDescription Key = "command.presentation.mcp_prompt.description"
	KeyCommandPresentationMCPPromptSuccess     Key = "command.presentation.mcp_prompt.success"
	KeyCommandPresentationMCPPromptFailure     Key = "command.presentation.mcp_prompt.failure"
	KeyCommandPresentationModelSaveWarning     Key = "command.presentation.model_save_warning"
	KeyCommandPresentationProviderWarning      Key = "command.presentation.provider_save_warning"

	KeyCommandOutcomeSucceeded     Key = "command.outcome.succeeded"
	KeyCommandOutcomeWarning       Key = "command.outcome.warning"
	KeyCommandOutcomePartial       Key = "command.outcome.partial"
	KeyCommandOutcomeFailed        Key = "command.outcome.failed"
	KeyCommandOutcomeDenied        Key = "command.outcome.denied"
	KeyCommandOutcomeCancelled     Key = "command.outcome.cancelled"
	KeyCommandOutcomeTimedOut      Key = "command.outcome.timed_out"
	KeyCommandOutcomeInterrupted   Key = "command.outcome.interrupted"
	KeyCommandOutcomeExitRequested Key = "command.outcome.exit_requested"
	KeyCommandOutcomeUnknown       Key = "command.outcome.unknown"
	KeyCommandDisplayReceipt       Key = "command.display.receipt"
	KeyCommandDisplayInspector     Key = "command.display.inspector"
	KeyCommandDisplayEvidence      Key = "command.display.evidence"
	KeyCommandDisplayDecision      Key = "command.display.decision"
	KeyCommandRiskUnknown          Key = "command.risk.unknown"
	KeyCommandRiskLow              Key = "command.risk.low"
	KeyCommandRiskMedium           Key = "command.risk.medium"
	KeyCommandRiskHigh             Key = "command.risk.high"
	KeyCommandRiskDestructive      Key = "command.risk.destructive"

	KeyCommandPresentationExitSuccess        Key = "command.presentation.exit.success"
	KeyCommandPresentationExitFailure        Key = "command.presentation.exit.failure"
	KeyCommandPresentationHelpSuccess        Key = "command.presentation.help.success"
	KeyCommandPresentationHelpFailure        Key = "command.presentation.help.failure"
	KeyCommandPresentationClearSuccess       Key = "command.presentation.clear.success"
	KeyCommandPresentationClearFailure       Key = "command.presentation.clear.failure"
	KeyCommandPresentationGoalSuccess        Key = "command.presentation.goal.success"
	KeyCommandPresentationGoalFailure        Key = "command.presentation.goal.failure"
	KeyCommandPresentationSearchSuccess      Key = "command.presentation.search.success"
	KeyCommandPresentationSearchFailure      Key = "command.presentation.search.failure"
	KeyCommandPresentationExportSuccess      Key = "command.presentation.export.success"
	KeyCommandPresentationExportFailure      Key = "command.presentation.export.failure"
	KeyCommandPresentationEditorSuccess      Key = "command.presentation.editor.success"
	KeyCommandPresentationEditorFailure      Key = "command.presentation.editor.failure"
	KeyCommandPresentationMouseSuccess       Key = "command.presentation.mouse.success"
	KeyCommandPresentationMouseFailure       Key = "command.presentation.mouse.failure"
	KeyCommandPresentationActivitySuccess    Key = "command.presentation.activity.success"
	KeyCommandPresentationActivityFailure    Key = "command.presentation.activity.failure"
	KeyCommandPresentationDetailSuccess      Key = "command.presentation.detail.success"
	KeyCommandPresentationDetailFailure      Key = "command.presentation.detail.failure"
	KeyCommandPresentationCompactSuccess     Key = "command.presentation.compact.success"
	KeyCommandPresentationCompactFailure     Key = "command.presentation.compact.failure"
	KeyCommandPresentationModelSuccess       Key = "command.presentation.model.success"
	KeyCommandPresentationModelFailure       Key = "command.presentation.model.failure"
	KeyCommandPresentationSessionSuccess     Key = "command.presentation.session.success"
	KeyCommandPresentationSessionFailure     Key = "command.presentation.session.failure"
	KeyCommandPresentationConfigSuccess      Key = "command.presentation.config.success"
	KeyCommandPresentationConfigFailure      Key = "command.presentation.config.failure"
	KeyCommandPresentationStatusSuccess      Key = "command.presentation.status.success"
	KeyCommandPresentationStatusFailure      Key = "command.presentation.status.failure"
	KeyCommandPresentationContextSuccess     Key = "command.presentation.context.success"
	KeyCommandPresentationContextFailure     Key = "command.presentation.context.failure"
	KeyCommandPresentationInitSuccess        Key = "command.presentation.init.success"
	KeyCommandPresentationInitFailure        Key = "command.presentation.init.failure"
	KeyCommandPresentationResumeSuccess      Key = "command.presentation.resume.success"
	KeyCommandPresentationResumeFailure      Key = "command.presentation.resume.failure"
	KeyCommandPresentationForkSuccess        Key = "command.presentation.fork.success"
	KeyCommandPresentationForkFailure        Key = "command.presentation.fork.failure"
	KeyCommandPresentationReviewSuccess      Key = "command.presentation.review.success"
	KeyCommandPresentationReviewFailure      Key = "command.presentation.review.failure"
	KeyCommandPresentationDoctorSuccess      Key = "command.presentation.doctor.success"
	KeyCommandPresentationDoctorFailure      Key = "command.presentation.doctor.failure"
	KeyCommandPresentationSkillsSuccess      Key = "command.presentation.skills.success"
	KeyCommandPresentationSkillsFailure      Key = "command.presentation.skills.failure"
	KeyCommandPresentationMCPSuccess         Key = "command.presentation.mcp.success"
	KeyCommandPresentationMCPFailure         Key = "command.presentation.mcp.failure"
	KeyCommandPresentationLanguageSuccess    Key = "command.presentation.language.success"
	KeyCommandPresentationLanguageFailure    Key = "command.presentation.language.failure"
	KeyCommandPresentationConnectSuccess     Key = "command.presentation.connect.success"
	KeyCommandPresentationConnectFailure     Key = "command.presentation.connect.failure"
	KeyCommandPresentationPasteSuccess       Key = "command.presentation.paste.success"
	KeyCommandPresentationPasteFailure       Key = "command.presentation.paste.failure"
	KeyCommandPresentationPermissionsSuccess Key = "command.presentation.permissions.success"
	KeyCommandPresentationPermissionsFailure Key = "command.presentation.permissions.failure"
	KeyCommandPresentationCostSuccess        Key = "command.presentation.cost.success"
	KeyCommandPresentationCostFailure        Key = "command.presentation.cost.failure"
	KeyCommandPresentationVersionSuccess     Key = "command.presentation.version.success"
	KeyCommandPresentationVersionFailure     Key = "command.presentation.version.failure"
	KeyCommandPresentationRenameSuccess      Key = "command.presentation.rename.success"
	KeyCommandPresentationRenameFailure      Key = "command.presentation.rename.failure"
	KeyCommandPresentationMemorySuccess      Key = "command.presentation.memory.success"
	KeyCommandPresentationMemoryFailure      Key = "command.presentation.memory.failure"
	KeyCommandPresentationDiffSuccess        Key = "command.presentation.diff.success"
	KeyCommandPresentationDiffFailure        Key = "command.presentation.diff.failure"
)

var commandPresentationNextKeys = map[string][2]Key{
	"exit":        {KeyCommandPresentationExitSuccess, KeyCommandPresentationExitFailure},
	"help":        {KeyCommandPresentationHelpSuccess, KeyCommandPresentationHelpFailure},
	"clear":       {KeyCommandPresentationClearSuccess, KeyCommandPresentationClearFailure},
	"goal":        {KeyCommandPresentationGoalSuccess, KeyCommandPresentationGoalFailure},
	"search":      {KeyCommandPresentationSearchSuccess, KeyCommandPresentationSearchFailure},
	"export":      {KeyCommandPresentationExportSuccess, KeyCommandPresentationExportFailure},
	"editor":      {KeyCommandPresentationEditorSuccess, KeyCommandPresentationEditorFailure},
	"mouse":       {KeyCommandPresentationMouseSuccess, KeyCommandPresentationMouseFailure},
	"activity":    {KeyCommandPresentationActivitySuccess, KeyCommandPresentationActivityFailure},
	"detail":      {KeyCommandPresentationDetailSuccess, KeyCommandPresentationDetailFailure},
	"compact":     {KeyCommandPresentationCompactSuccess, KeyCommandPresentationCompactFailure},
	"model":       {KeyCommandPresentationModelSuccess, KeyCommandPresentationModelFailure},
	"session":     {KeyCommandPresentationSessionSuccess, KeyCommandPresentationSessionFailure},
	"config":      {KeyCommandPresentationConfigSuccess, KeyCommandPresentationConfigFailure},
	"status":      {KeyCommandPresentationStatusSuccess, KeyCommandPresentationStatusFailure},
	"context":     {KeyCommandPresentationContextSuccess, KeyCommandPresentationContextFailure},
	"init":        {KeyCommandPresentationInitSuccess, KeyCommandPresentationInitFailure},
	"resume":      {KeyCommandPresentationResumeSuccess, KeyCommandPresentationResumeFailure},
	"fork":        {KeyCommandPresentationForkSuccess, KeyCommandPresentationForkFailure},
	"review":      {KeyCommandPresentationReviewSuccess, KeyCommandPresentationReviewFailure},
	"doctor":      {KeyCommandPresentationDoctorSuccess, KeyCommandPresentationDoctorFailure},
	"skills":      {KeyCommandPresentationSkillsSuccess, KeyCommandPresentationSkillsFailure},
	"mcp":         {KeyCommandPresentationMCPSuccess, KeyCommandPresentationMCPFailure},
	"language":    {KeyCommandPresentationLanguageSuccess, KeyCommandPresentationLanguageFailure},
	"connect":     {KeyCommandPresentationConnectSuccess, KeyCommandPresentationConnectFailure},
	"paste":       {KeyCommandPresentationPasteSuccess, KeyCommandPresentationPasteFailure},
	"permissions": {KeyCommandPresentationPermissionsSuccess, KeyCommandPresentationPermissionsFailure},
	"cost":        {KeyCommandPresentationCostSuccess, KeyCommandPresentationCostFailure},
	"version":     {KeyCommandPresentationVersionSuccess, KeyCommandPresentationVersionFailure},
	"rename":      {KeyCommandPresentationRenameSuccess, KeyCommandPresentationRenameFailure},
	"memory":      {KeyCommandPresentationMemorySuccess, KeyCommandPresentationMemoryFailure},
	"diff":        {KeyCommandPresentationDiffSuccess, KeyCommandPresentationDiffFailure},
}

func init() {
	addCommandPresentation(KeyCommandPresentationWait, cp("Wait for the command to finish.", "等待命令完成。", "Warte, bis der Befehl abgeschlossen ist.", "コマンドの完了を待ってください。", "명령이 완료될 때까지 기다리세요.", "Дождитесь завершения команды."))
	addCommandPresentation(KeyCommandPresentationInspectResult, cp("Review the command result before continuing.", "继续前请检查命令结果。", "Prüfe vor dem Fortfahren das Befehlsergebnis.", "続行する前にコマンド結果を確認してください。", "계속하기 전에 명령 결과를 검토하세요.", "Перед продолжением проверьте результат команды."))
	addCommandPresentation(KeyCommandPresentationInspectError, cp("Review the error and retry when it is safe.", "请检查错误，并在安全时重试。", "Prüfe den Fehler und wiederhole den Vorgang, wenn es sicher ist.", "エラーを確認し、安全に再試行できる場合はやり直してください。", "오류를 검토하고 안전할 때 다시 시도하세요.", "Проверьте ошибку и повторите попытку, когда это безопасно."))
	addCommandPresentation(KeyCommandPresentationExitRequested, cp("Exit requested.", "已请求退出。", "Beenden angefordert.", "終了が要求されました。", "종료가 요청되었습니다.", "Запрошен выход."))
	addCommandPresentation(KeyCommandPresentationCompleted, cp("/%s %s completed.", "/%s %s 已完成。", "/%s %s abgeschlossen.", "/%s %s が完了しました。", "/%s %s 완료됨.", "/%s %s завершена."))
	addCommandPresentation(KeyCommandPresentationResult, cp("Result", "结果", "Ergebnis", "結果", "결과", "Результат"))
	addCommandPresentation(KeyCommandPresentationExtensionSuccess, cp("Review the extension command result before continuing.", "继续前请检查扩展命令的结果。", "Prüfe vor dem Fortfahren das Ergebnis des Erweiterungsbefehls.", "続行する前に拡張コマンドの結果を確認してください。", "계속하기 전에 확장 명령 결과를 검토하세요.", "Перед продолжением проверьте результат команды расширения."))
	addCommandPresentation(KeyCommandPresentationExtensionFailure, cp("Review the extension error and retry when it is safe.", "请检查扩展错误，并在安全时重试。", "Prüfe den Erweiterungsfehler und wiederhole den Vorgang, wenn es sicher ist.", "拡張機能のエラーを確認し、安全に再試行できる場合はやり直してください。", "확장 오류를 검토하고 안전할 때 다시 시도하세요.", "Проверьте ошибку расширения и повторите попытку, когда это безопасно."))
	addCommandPresentation(KeyCommandPresentationMCPPromptDescription, cp("MCP prompt %s from %s", "MCP prompt %s（来自 %s）", "MCP-Prompt %s von %s", "MCP prompt %s（%s）", "MCP prompt %s(%s)", "MCP prompt %s с сервера %s"))
	addCommandPresentation(KeyCommandPresentationMCPPromptSuccess, cp("Continue with the MCP prompt messages added to this session.", "使用已添加到本会话的 MCP prompt 消息继续。", "Fahre mit den dieser Sitzung hinzugefügten MCP-Prompt-Nachrichten fort.", "このセッションに追加された MCP prompt メッセージを使って続行してください。", "이 세션에 추가된 MCP prompt 메시지로 계속하세요.", "Продолжите работу с сообщениями MCP prompt, добавленными в этот сеанс."))
	addCommandPresentation(KeyCommandPresentationMCPPromptFailure, cp("Check the MCP server, prompt arguments, and connection before retrying.", "检查 MCP server、prompt 参数和连接后重试。", "Prüfe MCP-Server, Prompt-Argumente und Verbindung, bevor du es erneut versuchst.", "MCP server、prompt 引数、接続を確認してから再試行してください。", "MCP server, prompt 인수, 연결을 확인한 후 다시 시도하세요.", "Перед повторной попыткой проверьте MCP-сервер, аргументы prompt и соединение."))
	addCommandPresentation(KeyCommandPresentationModelSaveWarning, cp("Fix the project settings permissions; the in-memory model selection is already active.", "请修复项目设置权限；内存中的 model 选择已生效。", "Korrigiere die Berechtigungen der Projekteinstellungen; die Modellauswahl im Speicher ist bereits aktiv.", "プロジェクト設定の権限を修正してください。メモリ上の model 選択はすでに有効です。", "프로젝트 설정 권한을 수정하세요. 메모리의 model 선택은 이미 적용되었습니다.", "Исправьте разрешения настроек проекта; выбранная в памяти модель уже активна."))
	addCommandPresentation(KeyCommandPresentationProviderWarning, cp("Fix the project settings permissions; the in-memory provider and model selection is already active.", "请修复项目设置权限；内存中的 Provider 和 model 选择已生效。", "Korrigiere die Berechtigungen der Projekteinstellungen; die Provider- und Modellauswahl im Speicher ist bereits aktiv.", "プロジェクト設定の権限を修正してください。メモリ上の Provider と model の選択はすでに有効です。", "프로젝트 설정 권한을 수정하세요. 메모리의 Provider 및 model 선택은 이미 적용되었습니다.", "Исправьте разрешения настроек проекта; выбранные в памяти провайдер и модель уже активны."))

	addCommandPresentation(KeyCommandOutcomeSucceeded, cp("succeeded", "成功", "erfolgreich", "成功", "성공", "успешно"))
	addCommandPresentation(KeyCommandOutcomeWarning, cp("completed with a warning", "已完成，但有警告", "mit Warnung abgeschlossen", "警告付きで完了", "경고와 함께 완료", "завершено с предупреждением"))
	addCommandPresentation(KeyCommandOutcomePartial, cp("partially completed", "部分完成", "teilweise abgeschlossen", "部分的に完了", "부분 완료", "частично завершено"))
	addCommandPresentation(KeyCommandOutcomeFailed, cp("failed", "失败", "fehlgeschlagen", "失敗", "실패", "ошибка"))
	addCommandPresentation(KeyCommandOutcomeDenied, cp("denied", "已拒绝", "abgelehnt", "拒否", "거부됨", "отклонено"))
	addCommandPresentation(KeyCommandOutcomeCancelled, cp("cancelled", "已取消", "abgebrochen", "キャンセル", "취소됨", "отменено"))
	addCommandPresentation(KeyCommandOutcomeTimedOut, cp("timed out", "已超时", "Zeitüberschreitung", "タイムアウト", "시간 초과", "истекло время ожидания"))
	addCommandPresentation(KeyCommandOutcomeInterrupted, cp("interrupted", "已中断", "unterbrochen", "中断", "중단됨", "прервано"))
	addCommandPresentation(KeyCommandOutcomeExitRequested, cp("exit requested", "已请求退出", "Beenden angefordert", "終了要求", "종료 요청됨", "запрошен выход"))
	addCommandPresentation(KeyCommandOutcomeUnknown, cp("unknown", "未知", "unbekannt", "不明", "알 수 없음", "неизвестно"))
	addCommandPresentation(KeyCommandDisplayReceipt, cp("receipt", "回执", "Bestätigung", "確認", "확인", "подтверждение"))
	addCommandPresentation(KeyCommandDisplayInspector, cp("details", "详情", "Details", "詳細", "세부 정보", "сведения"))
	addCommandPresentation(KeyCommandDisplayEvidence, cp("evidence", "证据", "Belege", "エビデンス", "증거", "данные"))
	addCommandPresentation(KeyCommandDisplayDecision, cp("decision", "决策", "Entscheidung", "決定", "결정", "решение"))
	addCommandPresentation(KeyCommandRiskUnknown, cp("unknown", "未知", "unbekannt", "不明", "알 수 없음", "неизвестный"))
	addCommandPresentation(KeyCommandRiskLow, cp("low", "低", "niedrig", "低", "낮음", "низкий"))
	addCommandPresentation(KeyCommandRiskMedium, cp("medium", "中", "mittel", "中", "보통", "средний"))
	addCommandPresentation(KeyCommandRiskHigh, cp("high", "高", "hoch", "高", "높음", "высокий"))
	addCommandPresentation(KeyCommandRiskDestructive, cp("destructive", "破坏性", "destruktiv", "破壊的", "파괴적", "разрушительный"))

	addCommandNext("exit", cp("No further action is required.", "无需进一步操作。", "Keine weitere Aktion erforderlich.", "これ以上の操作は不要です。", "추가 작업이 필요하지 않습니다.", "Дополнительные действия не требуются."), cp("Resolve unfinished work and retry /exit.", "处理未完成的工作后重试 /exit。", "Schließe offene Arbeiten ab und wiederhole /exit.", "未完了の作業を解決してから /exit を再試行してください。", "완료되지 않은 작업을 정리한 후 /exit를 다시 시도하세요.", "Завершите незаконченные задачи и повторите /exit."))
	addCommandNext("help", cp("Run one of the listed commands, or open /help again.", "运行列出的命令，或再次打开 /help。", "Führe einen aufgeführten Befehl aus oder öffne /help erneut.", "一覧のコマンドを実行するか、もう一度 /help を開いてください。", "목록의 명령을 실행하거나 /help를 다시 여세요.", "Выполните одну из перечисленных команд или снова откройте /help."), cp("Retry /help; command execution remains available.", "重试 /help；其他命令仍可执行。", "Wiederhole /help; andere Befehle bleiben verfügbar.", "/help を再試行してください。ほかのコマンドは引き続き実行できます。", "/help를 다시 시도하세요. 다른 명령은 계속 사용할 수 있습니다.", "Повторите /help; остальные команды по-прежнему доступны."))
	addCommandNext("clear", cp("Continue in the cleared scope.", "在已清理的范围内继续。", "Fahre im bereinigten Bereich fort.", "クリアされた範囲で続行してください。", "정리된 범위에서 계속하세요.", "Продолжите работу в очищенной области."), cp("Review what was preserved, then retry /clear.", "检查保留的内容后重试 /clear。", "Prüfe die beibehaltenen Inhalte und wiederhole dann /clear.", "保持された内容を確認してから /clear を再試行してください。", "보존된 내용을 검토한 후 /clear를 다시 시도하세요.", "Проверьте сохранённые данные и повторите /clear."))
	addCommandNext("goal", cp("Continue working toward the displayed goal.", "继续推进显示的目标。", "Arbeite weiter auf das angezeigte Ziel hin.", "表示された目標に向けて作業を続けてください。", "표시된 목표를 향해 계속 작업하세요.", "Продолжайте работу над показанной целью."), cp("Correct the goal transition and retry /goal.", "修正目标状态转换后重试 /goal。", "Korrigiere den Zielübergang und wiederhole /goal.", "目標の状態遷移を修正して /goal を再試行してください。", "목표 상태 전환을 수정한 후 /goal을 다시 시도하세요.", "Исправьте переход цели и повторите /goal."))
	addCommandNext("search", cp("Use /search --next, --previous, or --close as needed.", "按需使用 /search --next、--previous 或 --close。", "Nutze nach Bedarf /search --next, --previous oder --close.", "必要に応じて /search --next、--previous、--close を使用してください。", "필요에 따라 /search --next, --previous 또는 --close를 사용하세요.", "При необходимости используйте /search --next, --previous или --close."), cp("Adjust the query or close transcript search.", "调整查询，或关闭对话记录搜索。", "Passe die Anfrage an oder schließe die Transkriptsuche.", "クエリを調整するか、トランスクリプト検索を閉じてください。", "질의를 조정하거나 대화 기록 검색을 닫으세요.", "Измените запрос или закройте поиск по стенограмме."))
	addCommandNext("export", cp("Open or inspect the exported transcript.", "打开或检查导出的对话记录。", "Öffne oder prüfe das exportierte Transkript.", "エクスポートしたトランスクリプトを開くか確認してください。", "내보낸 대화 기록을 열거나 검토하세요.", "Откройте или проверьте экспортированную стенограмму."), cp("Choose a writable path and retry /export.", "选择可写路径后重试 /export。", "Wähle einen beschreibbaren Pfad und wiederhole /export.", "書き込み可能なパスを選んで /export を再試行してください。", "쓸 수 있는 경로를 선택한 후 /export를 다시 시도하세요.", "Выберите доступный для записи путь и повторите /export."))
	addCommandNext("editor", cp("Return to the transcript after closing the editor.", "关闭编辑器后返回对话记录。", "Kehre nach dem Schließen des Editors zum Transkript zurück.", "エディターを閉じたらトランスクリプトに戻ってください。", "편집기를 닫은 후 대화 기록으로 돌아오세요.", "После закрытия редактора вернитесь к стенограмме."), cp("Configure VISUAL or EDITOR and retry /editor.", "配置 VISUAL 或 EDITOR 后重试 /editor。", "Konfiguriere VISUAL oder EDITOR und wiederhole /editor.", "VISUAL または EDITOR を設定して /editor を再試行してください。", "VISUAL 또는 EDITOR를 설정한 후 /editor를 다시 시도하세요.", "Настройте VISUAL или EDITOR и повторите /editor."))
	addCommandNext("mouse", cp("Continue with the reported mouse state.", "使用当前显示的鼠标状态继续。", "Fahre mit dem gemeldeten Mausstatus fort.", "表示されたマウス状態で続行してください。", "표시된 마우스 상태로 계속하세요.", "Продолжите работу с показанным состоянием мыши."), cp("Use keyboard navigation or retry /mouse.", "使用键盘导航，或重试 /mouse。", "Nutze die Tastaturnavigation oder wiederhole /mouse.", "キーボード操作を使用するか、/mouse を再試行してください。", "키보드 탐색을 사용하거나 /mouse를 다시 시도하세요.", "Используйте клавиатуру или повторите /mouse."))
	addCommandNext("activity", cp("Inspect or act on the selected activity.", "检查或操作选中的活动。", "Prüfe die ausgewählte Aktivität oder führe eine Aktion aus.", "選択したアクティビティを確認または操作してください。", "선택한 활동을 검토하거나 작업을 수행하세요.", "Проверьте выбранную активность или выполните действие."), cp("Refresh /activity and choose a valid action.", "刷新 /activity 并选择有效操作。", "Aktualisiere /activity und wähle eine gültige Aktion.", "/activity を更新して有効な操作を選んでください。", "/activity를 새로 고치고 유효한 작업을 선택하세요.", "Обновите /activity и выберите допустимое действие."))
	addCommandNext("detail", cp("Inspect the selected disclosure level.", "检查选中的披露级别。", "Prüfe die ausgewählte Offenlegungsstufe.", "選択した開示レベルを確認してください。", "선택한 공개 수준을 검토하세요.", "Проверьте выбранный уровень раскрытия."), cp("Verify the observation ID and disclosure level.", "检查 observation ID 和披露级别。", "Prüfe Beobachtungs-ID und Offenlegungsstufe.", "observation ID と開示レベルを確認してください。", "observation ID와 공개 수준을 확인하세요.", "Проверьте ID наблюдения и уровень раскрытия."))
	addCommandNext("compact", cp("Continue with the compacted context and retained evidence.", "使用压缩后的上下文和保留的证据继续。", "Fahre mit dem komprimierten Kontext und den beibehaltenen Belegen fort.", "圧縮されたコンテキストと保持されたエビデンスで続行してください。", "압축된 컨텍스트와 보존된 증거로 계속하세요.", "Продолжите работу со сжатым контекстом и сохранёнными данными."), cp("Inspect the compaction error and retry when recoverable.", "检查压缩错误，并在可恢复时重试。", "Prüfe den Komprimierungsfehler und wiederhole den Vorgang, wenn er behebbar ist.", "圧縮エラーを確認し、復旧可能な場合は再試行してください。", "압축 오류를 검토하고 복구할 수 있을 때 다시 시도하세요.", "Проверьте ошибку сжатия и повторите попытку, если восстановление возможно."))
	addCommandNext("model", cp("Continue with the displayed or selected model.", "使用显示或选中的 model 继续。", "Fahre mit dem angezeigten oder ausgewählten Modell fort.", "表示または選択された model で続行してください。", "표시되거나 선택된 model로 계속하세요.", "Продолжите работу с показанной или выбранной моделью."), cp("Keep the previous model and choose an available model.", "保留之前的 model，并选择可用 model。", "Behalte das vorherige Modell und wähle ein verfügbares Modell.", "以前の model を維持し、利用可能な model を選んでください。", "이전 model을 유지하고 사용 가능한 model을 선택하세요.", "Сохраните предыдущую модель и выберите доступную."))
	addCommandNext("session", cp("Continue with the displayed session state.", "使用显示的会话状态继续。", "Fahre mit dem angezeigten Sitzungsstatus fort.", "表示されたセッション状態で続行してください。", "표시된 세션 상태로 계속하세요.", "Продолжите работу с показанным состоянием сеанса."), cp("Verify the session ID and retry without changing the current session.", "检查会话 ID，并在不更改当前会话的情况下重试。", "Prüfe die Sitzungs-ID und wiederhole den Vorgang, ohne die aktuelle Sitzung zu ändern.", "セッション ID を確認し、現在のセッションを変更せずに再試行してください。", "세션 ID를 확인하고 현재 세션을 변경하지 않은 채 다시 시도하세요.", "Проверьте ID сеанса и повторите попытку, не меняя текущий сеанс."))
	addCommandNext("config", cp("Continue with the effective configuration.", "使用当前生效的配置继续。", "Fahre mit der wirksamen Konfiguration fort.", "有効な設定で続行してください。", "현재 적용된 구성으로 계속하세요.", "Продолжите работу с действующей конфигурацией."), cp("Correct the key, value, or settings file and retry /config.", "修正 key、value 或 settings 文件后重试 /config。", "Korrigiere Schlüssel, Wert oder Einstellungsdatei und wiederhole /config.", "key、value、settings ファイルを修正して /config を再試行してください。", "key, value 또는 settings 파일을 수정한 후 /config를 다시 시도하세요.", "Исправьте ключ, значение или файл настроек и повторите /config."))
	addCommandNext("status", cp("Continue from the reported runtime snapshot.", "从显示的运行时快照继续。", "Fahre mit dem gemeldeten Laufzeit-Snapshot fort.", "表示されたランタイムスナップショットから続行してください。", "표시된 런타임 스냅샷에서 계속하세요.", "Продолжите с показанного снимка среды выполнения."), cp("Restore runtime state and retry /status.", "恢复运行时状态后重试 /status。", "Stelle den Laufzeitstatus wieder her und wiederhole /status.", "ランタイム状態を復元して /status を再試行してください。", "런타임 상태를 복원한 후 /status를 다시 시도하세요.", "Восстановите состояние среды выполнения и повторите /status."))
	addCommandNext("context", cp("Use /compact if the reported context state requires it.", "如果显示的上下文状态需要，请使用 /compact。", "Nutze /compact, wenn der gemeldete Kontextstatus dies erfordert.", "表示されたコンテキスト状態で必要な場合は /compact を使用してください。", "표시된 컨텍스트 상태에 필요하면 /compact를 사용하세요.", "Если состояние контекста требует этого, используйте /compact."), cp("Restore context tracking and retry /context.", "恢复上下文跟踪后重试 /context。", "Stelle die Kontextverfolgung wieder her und wiederhole /context.", "コンテキスト追跡を復元して /context を再試行してください。", "컨텍스트 추적을 복원한 후 /context를 다시 시도하세요.", "Восстановите отслеживание контекста и повторите /context."))
	addCommandNext("init", cp("Review the created and existing project files.", "检查已创建和现有的项目文件。", "Prüfe die erstellten und vorhandenen Projektdateien.", "作成済みおよび既存のプロジェクトファイルを確認してください。", "생성된 파일과 기존 프로젝트 파일을 검토하세요.", "Проверьте созданные и существующие файлы проекта."), cp("Resolve file permissions or partial writes before retrying /init.", "解决文件权限或部分写入问题后重试 /init。", "Behebe Dateiberechtigungen oder unvollständige Schreibvorgänge, bevor du /init wiederholst.", "ファイル権限または部分書き込みを解決してから /init を再試行してください。", "파일 권한 또는 부분 쓰기 문제를 해결한 후 /init을 다시 시도하세요.", "Устраните проблемы с разрешениями или неполной записью и повторите /init."))
	addCommandNext("resume", cp("Continue in the listed or resumed session.", "在列出或恢复的会话中继续。", "Fahre in der aufgeführten oder fortgesetzten Sitzung fort.", "一覧または再開したセッションで続行してください。", "목록에 표시되거나 재개된 세션에서 계속하세요.", "Продолжите в указанном или возобновлённом сеансе."), cp("Refine the session query and retry without changing the current session.", "细化会话查询，并在不更改当前会话的情况下重试。", "Verfeinere die Sitzungssuche und wiederhole den Vorgang, ohne die aktuelle Sitzung zu ändern.", "セッション検索を絞り込み、現在のセッションを変更せずに再試行してください。", "세션 질의를 구체화하고 현재 세션을 변경하지 않은 채 다시 시도하세요.", "Уточните запрос сеанса и повторите попытку, не меняя текущий сеанс."))
	addCommandNext("fork", cp("Continue in the new fork after selecting a turn.", "选择轮次后在新的 fork 中继续。", "Fahre nach Auswahl einer Runde im neuen Fork fort.", "ターンを選択したら新しい fork で続行してください。", "턴을 선택한 후 새 fork에서 계속하세요.", "После выбора хода продолжите работу в новой ветви."), cp("Keep the current conversation and retry /fork when the picker is available.", "保留当前对话，并在选择器可用时重试 /fork。", "Behalte die aktuelle Unterhaltung und wiederhole /fork, sobald die Auswahl verfügbar ist.", "現在の会話を維持し、選択画面が利用可能になったら /fork を再試行してください。", "현재 대화를 유지하고 선택기를 사용할 수 있을 때 /fork를 다시 시도하세요.", "Сохраните текущий диалог и повторите /fork, когда выбор станет доступен."))
	addCommandNext("review", cp("Inspect the findings, or continue if the review is clean.", "检查发现的问题；如果审查无问题则继续。", "Prüfe die Ergebnisse oder fahre bei einer sauberen Prüfung fort.", "指摘事項を確認するか、問題がなければ続行してください。", "검토 결과를 확인하고 문제가 없으면 계속하세요.", "Проверьте замечания или продолжите, если проверка чистая."), cp("Restore git access and retry /review.", "恢复 git 访问后重试 /review。", "Stelle den git-Zugriff wieder her und wiederhole /review.", "git へのアクセスを復元して /review を再試行してください。", "git 접근을 복원한 후 /review를 다시 시도하세요.", "Восстановите доступ к git и повторите /review."))
	addCommandNext("doctor", cp("Apply the reported fixes, then rerun /doctor.", "应用显示的修复后再次运行 /doctor。", "Wende die gemeldeten Korrekturen an und führe /doctor erneut aus.", "表示された修正を適用してから /doctor を再実行してください。", "표시된 수정 사항을 적용한 후 /doctor를 다시 실행하세요.", "Примените предложенные исправления и снова запустите /doctor."), cp("Restore diagnostic prerequisites and rerun /doctor.", "恢复诊断所需条件后再次运行 /doctor。", "Stelle die Diagnosevoraussetzungen wieder her und führe /doctor erneut aus.", "診断の前提条件を復元して /doctor を再実行してください。", "진단에 필요한 조건을 복원한 후 /doctor를 다시 실행하세요.", "Восстановите условия диагностики и снова запустите /doctor."))
	addCommandNext("skills", cp("Continue with the displayed skill availability.", "根据显示的 skill 可用状态继续。", "Fahre mit der angezeigten Skill-Verfügbarkeit fort.", "表示された skill の利用可能状態で続行してください。", "표시된 skill 사용 가능 상태로 계속하세요.", "Продолжите с учётом показанной доступности skill."), cp("Correct the skill name or runtime manager and retry /skills.", "修正 skill 名称或 runtime manager 后重试 /skills。", "Korrigiere den Skill-Namen oder Laufzeitmanager und wiederhole /skills.", "skill 名または runtime manager を修正して /skills を再試行してください。", "skill 이름 또는 runtime manager를 수정한 후 /skills를 다시 시도하세요.", "Исправьте имя skill или runtime manager и повторите /skills."))
	addCommandNext("mcp", cp("Inspect server state or choose an MCP management action.", "检查 server 状态，或选择 MCP 管理操作。", "Prüfe den Serverstatus oder wähle eine MCP-Verwaltungsaktion.", "server 状態を確認するか、MCP 管理操作を選んでください。", "server 상태를 확인하거나 MCP 관리 작업을 선택하세요.", "Проверьте состояние сервера или выберите действие управления MCP."), cp("Check the server, authentication, and connection before retrying /mcp.", "检查 server、认证和连接后重试 /mcp。", "Prüfe Server, Authentifizierung und Verbindung, bevor du /mcp wiederholst.", "server、認証、接続を確認してから /mcp を再試行してください。", "server, 인증, 연결을 확인한 후 /mcp를 다시 시도하세요.", "Проверьте сервер, аутентификацию и соединение перед повтором /mcp."))
	addCommandNext("language", cp("Continue in the displayed language.", "使用显示的语言继续。", "Fahre in der angezeigten Sprache fort.", "表示された言語で続行してください。", "표시된 언어로 계속하세요.", "Продолжите работу на показанном языке."), cp("Choose a supported language and retry /language.", "选择受支持的语言后重试 /language。", "Wähle eine unterstützte Sprache und wiederhole /language.", "対応する言語を選んで /language を再試行してください。", "지원되는 언어를 선택한 후 /language를 다시 시도하세요.", "Выберите поддерживаемый язык и повторите /language."))
	addCommandNext("connect", cp("Continue with the reported Provider connection state.", "根据显示的 Provider 连接状态继续。", "Fahre mit dem gemeldeten Provider-Verbindungsstatus fort.", "表示された Provider 接続状態で続行してください。", "표시된 Provider 연결 상태로 계속하세요.", "Продолжите с показанным состоянием подключения Provider."), cp("Keep the previous Provider and repair credentials before retrying.", "保留之前的 Provider，并修复凭据后重试。", "Behalte den vorherigen Provider und repariere die Anmeldedaten vor dem erneuten Versuch.", "以前の Provider を維持し、認証情報を修復してから再試行してください。", "이전 Provider를 유지하고 자격 증명을 수정한 후 다시 시도하세요.", "Сохраните предыдущий Provider и исправьте учётные данные перед повтором."))
	addCommandNext("paste", cp("Review the pasted content before submitting it.", "提交前请检查粘贴的内容。", "Prüfe den eingefügten Inhalt vor dem Absenden.", "送信する前に貼り付けた内容を確認してください。", "제출하기 전에 붙여넣은 내용을 검토하세요.", "Проверьте вставленные данные перед отправкой."), cp("Restore clipboard access or cancel the paste operation.", "恢复剪贴板访问，或取消粘贴操作。", "Stelle den Zugriff auf die Zwischenablage wieder her oder brich das Einfügen ab.", "クリップボードへのアクセスを復元するか、貼り付けをキャンセルしてください。", "클립보드 접근을 복원하거나 붙여넣기 작업을 취소하세요.", "Восстановите доступ к буферу обмена или отмените вставку."))
	addCommandNext("permissions", cp("Continue with the displayed permission policy.", "根据显示的权限策略继续。", "Fahre mit der angezeigten Berechtigungsrichtlinie fort.", "表示された権限ポリシーで続行してください。", "표시된 권한 정책으로 계속하세요.", "Продолжите с показанной политикой разрешений."), cp("Correct the permission scope before retrying.", "修正权限范围后重试。", "Korrigiere den Berechtigungsumfang vor dem erneuten Versuch.", "権限範囲を修正してから再試行してください。", "권한 범위를 수정한 후 다시 시도하세요.", "Исправьте область разрешений перед повтором."))
	addCommandNext("cost", cp("Continue with the reported pricing state.", "根据显示的定价状态继续。", "Fahre mit dem gemeldeten Preisstatus fort.", "表示された料金状態で続行してください。", "표시된 가격 상태로 계속하세요.", "Продолжите с показанным состоянием цен."), cp("Restore usage accounting and retry /cost.", "恢复用量统计后重试 /cost。", "Stelle die Nutzungsabrechnung wieder her und wiederhole /cost.", "使用量集計を復元して /cost を再試行してください。", "사용량 집계를 복원한 후 /cost를 다시 시도하세요.", "Восстановите учёт использования и повторите /cost."))
	addCommandNext("version", cp("Use the reported version for diagnostics.", "使用显示的版本进行诊断。", "Nutze die gemeldete Version für die Diagnose.", "表示されたバージョンを診断に使用してください。", "표시된 버전을 진단에 사용하세요.", "Используйте показанную версию для диагностики."), cp("Restore version metadata and retry /version.", "恢复版本元数据后重试 /version。", "Stelle die Versionsmetadaten wieder her und wiederhole /version.", "バージョンメタデータを復元して /version を再試行してください。", "버전 메타데이터를 복원한 후 /version을 다시 시도하세요.", "Восстановите метаданные версии и повторите /version."))
	addCommandNext("rename", cp("Continue with the renamed session.", "在已重命名的会话中继续。", "Fahre mit der umbenannten Sitzung fort.", "名前を変更したセッションで続行してください。", "이름이 변경된 세션에서 계속하세요.", "Продолжите работу в переименованном сеансе."), cp("Keep the current title and retry with a valid name.", "保留当前标题，并使用有效名称重试。", "Behalte den aktuellen Titel und wiederhole den Vorgang mit einem gültigen Namen.", "現在のタイトルを維持し、有効な名前で再試行してください。", "현재 제목을 유지하고 유효한 이름으로 다시 시도하세요.", "Сохраните текущий заголовок и повторите с допустимым именем."))
	addCommandNext("memory", cp("Review saved instruction changes before continuing.", "继续前请检查已保存的指令更改。", "Prüfe die gespeicherten Anweisungsänderungen vor dem Fortfahren.", "続行する前に保存した指示の変更を確認してください。", "계속하기 전에 저장된 지침 변경 사항을 검토하세요.", "Перед продолжением проверьте сохранённые изменения инструкций."), cp("Resolve editor or file errors before retrying /memory.", "解决编辑器或文件错误后重试 /memory。", "Behebe Editor- oder Dateifehler, bevor du /memory wiederholst.", "エディターまたはファイルのエラーを解決してから /memory を再試行してください。", "편집기 또는 파일 오류를 해결한 후 /memory를 다시 시도하세요.", "Устраните ошибки редактора или файла и повторите /memory."))
	addCommandNext("diff", cp("Review the displayed diff and retained evidence.", "检查显示的 diff 和保留的证据。", "Prüfe den angezeigten Diff und die beibehaltenen Belege.", "表示された diff と保持されたエビデンスを確認してください。", "표시된 diff와 보존된 증거를 검토하세요.", "Проверьте показанный diff и сохранённые данные."), cp("Restore repository access and retry /diff.", "恢复 repository 访问后重试 /diff。", "Stelle den Repository-Zugriff wieder her und wiederhole /diff.", "repository へのアクセスを復元して /diff を再試行してください。", "repository 접근을 복원한 후 /diff를 다시 시도하세요.", "Восстановите доступ к repository и повторите /diff."))
}

func cp(en, zh, de, ja, ko, ru string) [6]string { return [6]string{en, zh, de, ja, ko, ru} }

func addCommandPresentation(key Key, values [6]string) {
	semanticTranslations[key] = map[Language]string{LangEN: values[0], LangZH: values[1], LangDE: values[2], LangJA: values[3], LangKO: values[4], LangRU: values[5]}
}

func addCommandNext(command string, success, failure [6]string) {
	keys, ok := commandPresentationNextKeys[command]
	if !ok {
		panic("command presentation keys are not declared for " + command)
	}
	successKey, failureKey := keys[0], keys[1]
	addCommandPresentation(successKey, success)
	addCommandPresentation(failureKey, failure)
}

func CommandPresentationNextKey(command string, failed bool) (Key, bool) {
	keys, ok := commandPresentationNextKeys[command]
	if !ok {
		return "", false
	}
	if failed {
		return keys[1], true
	}
	return keys[0], true
}

func CommandOutcomeLabel(lang Language, outcome string) string {
	keys := map[string]Key{"succeeded": KeyCommandOutcomeSucceeded, "warning": KeyCommandOutcomeWarning, "partial": KeyCommandOutcomePartial, "failed": KeyCommandOutcomeFailed, "denied": KeyCommandOutcomeDenied, "cancelled": KeyCommandOutcomeCancelled, "timed_out": KeyCommandOutcomeTimedOut, "interrupted": KeyCommandOutcomeInterrupted, "exit_requested": KeyCommandOutcomeExitRequested, "unknown": KeyCommandOutcomeUnknown}
	if key, ok := keys[outcome]; ok {
		return Text(lang, key)
	}
	return outcome
}

func CommandDisplayLabel(lang Language, display string) string {
	keys := map[string]Key{"receipt": KeyCommandDisplayReceipt, "inspector": KeyCommandDisplayInspector, "evidence": KeyCommandDisplayEvidence, "decision": KeyCommandDisplayDecision}
	if key, ok := keys[display]; ok {
		return Text(lang, key)
	}
	return display
}

func CommandRiskLabel(lang Language, risk string) string {
	keys := map[string]Key{"unknown": KeyCommandRiskUnknown, "low": KeyCommandRiskLow, "medium": KeyCommandRiskMedium, "high": KeyCommandRiskHigh, "destructive": KeyCommandRiskDestructive}
	if key, ok := keys[risk]; ok {
		return Text(lang, key)
	}
	return risk
}
