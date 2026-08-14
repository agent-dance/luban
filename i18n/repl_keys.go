package i18n

// Semantic keys for REPL-owned receipts and wrappers. Values interpolated into
// these templates are runtime data (session IDs, task IDs, paths, and errors)
// and must remain unchanged.
const (
	KeyREPLInputBlocked              Key = "repl.input_blocked"
	KeyREPLHelpScreenReader          Key = "repl.screen_reader.help"
	KeyREPLModeReceipt               Key = "repl.mode.receipt"
	KeyREPLBackgroundStarted         Key = "repl.background.started"
	KeyREPLBackgroundDiscarded       Key = "repl.background.discarded"
	KeyREPLBackgroundFailed          Key = "repl.background.failed"
	KeyREPLBackgroundCompleted       Key = "repl.background.completed"
	KeyREPLClearViewReceipt          Key = "repl.clear_view.receipt"
	KeyREPLForkListIntro             Key = "repl.fork.list_intro"
	KeyREPLForkOpened                Key = "repl.fork.opened"
	KeyREPLQueryCompleted            Key = "repl.query.completed"
	KeyREPLQueryCancelled            Key = "repl.query.cancelled"
	KeyREPLQueryFailed               Key = "repl.query.failed"
	KeyREPLDeleteHistoryRejected     Key = "repl.delete_history.rejected"
	KeyREPLDeleteHistoryCompleted    Key = "repl.delete_history.completed"
	KeyREPLClearFailedClosed         Key = "repl.clear.failed_closed"
	KeyREPLClearConversation         Key = "repl.clear.conversation"
	KeyREPLResumeFailedClosed        Key = "repl.resume.failed_closed"
	KeyREPLResumeDegraded            Key = "repl.resume.degraded"
	KeyREPLResumeCompleted           Key = "repl.resume.completed"
	KeyREPLExportCompleted           Key = "repl.export.completed"
	KeyREPLTranscriptBegins          Key = "repl.transcript.begins"
	KeyREPLTranscriptEnds            Key = "repl.transcript.ends"
	KeyREPLTurnInterrupted           Key = "repl.turn.interrupted"
	KeyREPLCompactionStarted         Key = "repl.compaction.started"
	KeyREPLCompactionCompleted       Key = "repl.compaction.completed"
	KeyREPLCompactionFailed          Key = "repl.compaction.failed"
	KeyREPLCompactionCancelled       Key = "repl.compaction.cancelled"
	KeyREPLCompactionReason          Key = "repl.compaction.reason"
	KeyREPLCompactionBoundary        Key = "repl.compaction.boundary"
	KeyREPLTUIFullAgentThread        Key = "repl.tui.full_agent_thread"
	KeyREPLTUIErrorPrefix            Key = "repl.tui.error_prefix"
	KeyREPLTUIBackgroundCancelled    Key = "repl.tui.background_cancelled"
	KeyREPLTUIQueryCancelled         Key = "repl.tui.query_cancelled"
	KeyREPLTUIForkCancelled          Key = "repl.tui.fork_cancelled"
	KeyREPLTUIForkRunning            Key = "repl.tui.fork_running"
	KeyREPLTUIForkSnapshotChanged    Key = "repl.tui.fork_snapshot_changed"
	KeyREPLTUIForkFailed             Key = "repl.tui.fork_failed"
	KeyREPLTUIForkOpened             Key = "repl.tui.fork_opened"
	KeyREPLTUIUnknownCommand         Key = "repl.tui.unknown_command"
	KeyREPLTUICommandError           Key = "repl.tui.command_error"
	KeyREPLTUIResumeCancelled        Key = "repl.tui.resume_cancelled"
	KeyREPLTUIResumeFailed           Key = "repl.tui.resume_failed"
	KeyREPLTUIResumeCompleted        Key = "repl.tui.resume_completed"
	KeyREPLTUIGoalRefreshFailed      Key = "repl.tui.goal_refresh_failed"
	KeyREPLTUIQueryRunning           Key = "repl.tui.query_running"
	KeyREPLTUIInputBlocked           Key = "repl.tui.input_blocked"
	KeyREPLTUIImageLoadFailed        Key = "repl.tui.image_load_failed"
	KeyREPLTUIImageUnsupported       Key = "repl.tui.image_unsupported"
	KeyREPLTUIBackgroundFailed       Key = "repl.tui.background.failed"
	KeyREPLTUIModelSaveFailed        Key = "repl.tui.model.save_failed"
	KeyREPLTUIContextWindowRange     Key = "repl.tui.model.context_window_range"
	KeyREPLTUIModeSwitchFailed       Key = "repl.tui.mode.switch_failed"
	KeyREPLTUIActivityActionFailed   Key = "repl.tui.activity.action_failed"
	KeyREPLTUILifecycleSaveFailed    Key = "repl.tui.lifecycle.save_failed"
	KeyREPLTUICleanupFailed          Key = "repl.tui.cleanup.failed"
	KeyREPLTUIAgentGroupTotal        Key = "repl.tui.agent.group_total"
	KeyREPLTUIAgentCountFailed       Key = "repl.tui.agent.count_failed"
	KeyREPLTUIAgentCountRunning      Key = "repl.tui.agent.count_running"
	KeyREPLTUIAgentCountReady        Key = "repl.tui.agent.count_ready"
	KeyREPLTUIAgentCountCancelled    Key = "repl.tui.agent.count_cancelled"
	KeyREPLTUIAgentMember            Key = "repl.tui.agent.member"
	KeyREPLTUIStatusReady            Key = "repl.tui.status.ready"
	KeyREPLTUIStatusFailed           Key = "repl.tui.status.failed"
	KeyREPLTUIStatusRunning          Key = "repl.tui.status.running"
	KeyREPLTUIStatusCancelled        Key = "repl.tui.status.cancelled"
	KeyREPLTUIStatusKilled           Key = "repl.tui.status.killed"
	KeyREPLTUIAgentAttempt           Key = "repl.tui.agent.attempt"
	KeyREPLTUIAgentPath              Key = "repl.tui.agent.path"
	KeyREPLTUIAgentDetails           Key = "repl.tui.agent.details"
	KeyREPLTUISummarySeparator       Key = "repl.tui.summary.separator"
	KeyREPLTUISummaryEnd             Key = "repl.tui.summary.end"
	KeyREPLTUITokenCount             Key = "repl.tui.metric.tokens"
	KeyREPLTUIThreadRetained         Key = "repl.tui.metric.thread_retained"
	KeyREPLTUIThreadUnavailable      Key = "repl.tui.metric.thread_unavailable"
	KeyREPLTUITranscriptUnretained   Key = "repl.tui.agent.transcript_unretained"
	KeyREPLTUIQueuedCount            Key = "repl.tui.metric.queued"
	KeyREPLTUIQueuedReason           Key = "repl.tui.metric.queued_reason"
	KeyREPLTUITerminalReason         Key = "repl.tui.metric.reason"
	KeyREPLTUIArtifactCount          Key = "repl.tui.metric.artifacts"
	KeyREPLTUIVerificationCount      Key = "repl.tui.metric.verification_refs"
	KeyREPLTUIToolName               Key = "repl.tui.metric.tool"
	KeyREPLTUIUpdatedAt              Key = "repl.tui.metric.updated_at"
	KeyREPLTUITranscriptClosed       Key = "repl.tui.transcript.closed"
	KeyREPLTUITranscriptNoMatches    Key = "repl.tui.transcript.no_matches"
	KeyREPLTUITranscriptMatch        Key = "repl.tui.transcript.match"
	KeyREPLTUIStopped                Key = "repl.tui.stopped"
	KeyREPLTUIModelCatalogMissing    Key = "repl.tui.model.catalog_missing"
	KeyREPLTUIMouseModeInvalid       Key = "repl.tui.mouse.invalid_mode"
	KeyREPLTUIActivityViewOpened     Key = "repl.tui.activity.view_opened"
	KeyREPLTUIActivityViewClosed     Key = "repl.tui.activity.view_closed"
	KeyREPLTUIDisclosureUnknown      Key = "repl.tui.disclosure.unknown"
	KeyREPLTUIDisclosureReceipt      Key = "repl.tui.disclosure.receipt"
	KeyREPLTUINoProviders            Key = "repl.tui.provider.none"
	KeyREPLTUIProviderCredsFailed    Key = "repl.tui.provider.credentials_failed"
	KeyREPLTUIProviderCreateFailed   Key = "repl.tui.provider.create_failed"
	KeyREPLTUIModelSwitched          Key = "repl.tui.model.switched"
	KeyREPLTUIModelSwitchedReasoning Key = "repl.tui.model.switched_reasoning"
	KeyREPLTUIModelPickerCancelled   Key = "repl.tui.model.picker_cancelled"
	KeyREPLTUICredentialStoreMissing Key = "repl.tui.credentials.store_missing"
	KeyREPLTUICredentialSaveFailed   Key = "repl.tui.credentials.save_failed"
	KeyREPLTUIProviderConnected      Key = "repl.tui.provider.connected"
	KeyREPLTUIFetchingModels         Key = "repl.tui.provider.fetching_models"
	KeyREPLTUIProviderDeleted        Key = "repl.tui.provider.deleted"
	KeyREPLTUIOAuthWaiting           Key = "repl.tui.oauth.waiting"
	KeyREPLTUIOAuthStarting          Key = "repl.tui.oauth.starting"
	KeyREPLTUIOAuthFailed            Key = "repl.tui.oauth.failed"
	KeyREPLTUIDeviceAuthWaiting      Key = "repl.tui.device_auth.waiting"
	KeyREPLTUIDeviceAuthStarting     Key = "repl.tui.device_auth.starting"
	KeyREPLTUIDeviceAuthFailed       Key = "repl.tui.device_auth.failed"
	KeyREPLTUIVertexBaseURLRequired  Key = "repl.tui.base_url.vertex_required"
	KeyREPLTUIBaseURLAbsolute        Key = "repl.tui.base_url.absolute"
	KeyREPLTUIBaseURLCredentials     Key = "repl.tui.base_url.credentials"
	KeyREPLTUIUnknownProvider        Key = "repl.tui.provider.unknown"
	KeyREPLTUIActivityNotFound       Key = "repl.tui.activity.not_found"
	KeyREPLTUIActivityNotCancellable Key = "repl.tui.activity.not_cancellable"
	KeyREPLTUIActivityNoController   Key = "repl.tui.activity.no_controller"
	KeyREPLTUIActivityCancelled      Key = "repl.tui.activity.cancelled"
	KeyREPLTUIActivityNoJump         Key = "repl.tui.activity.no_jump"
	KeyREPLTUIActivityJumped         Key = "repl.tui.activity.jumped"
	KeyREPLTUIActivityLocated        Key = "repl.tui.activity.located"
	KeyREPLTUIActivityNoDetails      Key = "repl.tui.activity.no_details"
	KeyREPLTUIActivityEvidenceOpened Key = "repl.tui.activity.evidence_opened"
	KeyREPLTUIActivityNoAttention    Key = "repl.tui.activity.no_attention"
	KeyREPLTUIActivityAcknowledged   Key = "repl.tui.activity.acknowledged"
	KeyREPLTUIActivityUnknownAction  Key = "repl.tui.activity.unknown_action"
	KeyREPLTUIQueryStartFailed       Key = "repl.tui.query.start_failed"
	KeyREPLTUIHookSummary            Key = "repl.tui.hook.summary"
	KeyREPLTUIContextCompaction      Key = "repl.tui.compaction.name"
	KeyREPLTUICompactionCompacting   Key = "repl.tui.compaction.compacting"
	KeyREPLTUICompactionFailed       Key = "repl.tui.compaction.failed"
	KeyREPLTUICompactionCancelled    Key = "repl.tui.compaction.cancelled"
	KeyREPLTUICompactionIdle         Key = "repl.tui.compaction.idle"
	KeyREPLTUIHookBlocked            Key = "repl.tui.hook.blocked"
	KeyREPLTUIHookSucceeded          Key = "repl.tui.hook.succeeded"
	KeyREPLTUIHookFailed             Key = "repl.tui.hook.failed"
	KeyREPLTUIHookCancelled          Key = "repl.tui.hook.cancelled"
	KeyREPLTUIHookRunning            Key = "repl.tui.hook.running"
)

func init() {
	for key, translations := range map[Key]map[Language]string{
		KeyREPLInputBlocked:        repl("Input blocked by hook: %s", "输入被钩子阻止：%s", "Eingabe wurde durch Hook blockiert: %s", "入力はフックによりブロックされました: %s", "입력이 훅에 의해 차단됨: %s", "Ввод заблокирован хуком: %s"),
		KeyREPLHelpScreenReader:    repl("Screen-reader session commands: /delete-history SESSION_ID; /mode auto|ask|plan.", "屏幕阅读器会话命令：/delete-history SESSION_ID；/mode auto|ask|plan。", "Screenreader-Sitzungsbefehle: /delete-history SESSION_ID; /mode auto|ask|plan.", "スクリーンリーダーのセッションコマンド: /delete-history SESSION_ID; /mode auto|ask|plan。", "스크린 리더 세션 명령: /delete-history SESSION_ID; /mode auto|ask|plan.", "Команды сеанса экранного диктора: /delete-history SESSION_ID; /mode auto|ask|plan."),
		KeyREPLModeReceipt:         repl("Mode receipt: active mode is %s.", "模式回执：当前模式为%s。", "Modusbestätigung: Aktiver Modus ist %s.", "モード確認: 現在のモードは %s です。", "모드 확인: 현재 모드는 %s입니다.", "Подтверждение режима: активный режим — %s."),
		KeyREPLBackgroundStarted:   repl("Background follow-up started for session %s, task %s.", "会话 %s 的后台后续任务 %s 已启动。", "Hintergrund-Fortsetzung für Sitzung %s, Aufgabe %s gestartet.", "セッション %s のバックグラウンド継続タスク %s を開始しました。", "세션 %s의 백그라운드 후속 작업 %s을 시작했습니다.", "Фоновое продолжение для сеанса %s, задача %s, запущено."),
		KeyREPLBackgroundDiscarded: repl("Background follow-up for task %s was discarded because its session history was deleted.", "任务 %s 的后台后续处理已丢弃，因为其会话历史已删除。", "Hintergrund-Fortsetzung für Aufgabe %s wurde verworfen, weil ihr Sitzungsverlauf gelöscht wurde.", "タスク %s のバックグラウンド継続は、セッション履歴が削除されたため破棄されました。", "작업 %s의 백그라운드 후속 처리는 세션 기록이 삭제되어 폐기되었습니다.", "Фоновое продолжение задачи %s отброшено: история сеанса удалена."),
		KeyREPLBackgroundFailed:    repl("Background follow-up receipt for session %s failed: %s", "会话 %s 的后台后续处理回执失败：%s", "Bestätigung der Hintergrund-Fortsetzung für Sitzung %s fehlgeschlagen: %s", "セッション %s のバックグラウンド継続確認に失敗しました: %s", "세션 %s의 백그라운드 후속 작업 확인 실패: %s", "Подтверждение фонового продолжения для сеанса %s не удалось: %s"),
		KeyREPLBackgroundCompleted: repl("Background follow-up receipt for session %s: completed.", "会话 %s 的后台后续处理回执：已完成。", "Bestätigung der Hintergrund-Fortsetzung für Sitzung %s: abgeschlossen.", "セッション %s のバックグラウンド継続確認: 完了しました。", "세션 %s의 백그라운드 후속 작업 확인: 완료됨.", "Подтверждение фонового продолжения для сеанса %s: завершено."),
		KeyREPLClearViewReceipt:    repl("Clear view receipt: screen-reader output is append-only; no prior output was removed and model context unchanged.", "清空视图回执：屏幕阅读器输出仅可追加；未移除先前输出，模型上下文未变。", "Ansichtsbereinigung: Die Screenreader-Ausgabe ist nur anhängbar; keine frühere Ausgabe wurde entfernt und der Modellkontext blieb unverändert.", "ビュー消去の確認: スクリーンリーダー出力は追記のみです。以前の出力は削除されず、モデルのコンテキストも変わりません。", "보기 지우기 확인: 스크린 리더 출력은 추가 전용이며 이전 출력과 모델 컨텍스트는 변경되지 않았습니다.", "Подтверждение очистки вида: вывод экранного диктора только добавляется; предыдущий вывод и контекст модели не изменены."),
		KeyREPLForkListIntro:       repl("Fork points retained in the active model context, newest first. The default selection is 1. Run /fork NUMBER to create the fork.", "活动模型上下文中保留的分叉点（最新在前）。默认选择为 1。运行 /fork NUMBER 创建分叉。", "Im aktiven Modellkontext gespeicherte Fork-Punkte, neueste zuerst. Standardauswahl ist 1. Mit /fork NUMBER einen Fork erstellen.", "アクティブなモデルコンテキストに保持されたフォーク地点です（新しい順）。既定の選択は 1 です。/fork NUMBER でフォークを作成します。", "활성 모델 컨텍스트에 보존된 포크 지점이며 최신순입니다. 기본 선택은 1입니다. /fork NUMBER로 포크를 만드세요.", "Точки ветвления, сохранённые в активном контексте модели; новые сверху. По умолчанию выбрано 1. Выполните /fork NUMBER для создания ветви."),
		KeyREPLForkOpened:          repl("Fork receipt: session %s opened in a new terminal tab.", "分叉回执：会话 %s 已在新的终端标签页中打开。", "Fork-Bestätigung: Sitzung %s wurde in einem neuen Terminal-Tab geöffnet.", "フォーク確認: セッション %s を新しいターミナルタブで開きました。", "포크 확인: 세션 %s을 새 터미널 탭에서 열었습니다.", "Подтверждение ветви: сеанс %s открыт в новой вкладке терминала."),
		KeyREPLQueryCompleted:      repl("Query receipt: completed.", "查询回执：已完成。", "Abfragebestätigung: abgeschlossen.", "クエリ確認: 完了しました。", "쿼리 확인: 완료됨.", "Подтверждение запроса: завершено."), KeyREPLQueryCancelled: repl("Query receipt: cancelled.", "查询回执：已取消。", "Abfragebestätigung: abgebrochen.", "クエリ確認: キャンセルされました。", "쿼리 확인: 취소됨.", "Подтверждение запроса: отменено."), KeyREPLQueryFailed: repl("Query receipt: failed: %s", "查询回执：失败：%s", "Abfragebestätigung: fehlgeschlagen: %s", "クエリ確認: 失敗しました: %s", "쿼리 확인: 실패: %s", "Подтверждение запроса: ошибка: %s"),
		KeyREPLDeleteHistoryRejected: repl("Delete history receipt: outcome %s; session %s was not deleted.", "删除历史回执：结果为 %s；会话 %s 未删除。", "Bestätigung zum Löschen des Verlaufs: Ergebnis %s; Sitzung %s wurde nicht gelöscht.", "履歴削除の確認: 結果は %s です。セッション %s は削除されませんでした。", "기록 삭제 확인: 결과 %s, 세션 %s은 삭제되지 않았습니다.", "Подтверждение удаления истории: результат %s; сеанс %s не удалён."), KeyREPLDeleteHistoryCompleted: repl("Delete history receipt: permanently deleted session %s.", "删除历史回执：已永久删除会话 %s。", "Bestätigung zum Löschen des Verlaufs: Sitzung %s wurde dauerhaft gelöscht.", "履歴削除の確認: セッション %s を完全に削除しました。", "기록 삭제 확인: 세션 %s을 영구 삭제했습니다.", "Подтверждение удаления истории: сеанс %s удалён навсегда."),
		KeyREPLClearFailedClosed: repl("Clear failed-closed receipt: previous session remains active; permission mode is %s.", "清空故障关闭回执：先前会话仍处于活动状态；权限模式为 %s。", "Fail-closed-Bestätigung beim Leeren: Die vorherige Sitzung bleibt aktiv; Berechtigungsmodus ist %s.", "消去のフェイルクローズ確認: 前のセッションはアクティブのままです。権限モードは %s です。", "지우기 페일 클로즈 확인: 이전 세션은 활성 상태이며 권한 모드는 %s입니다.", "Подтверждение безопасного сбоя очистки: предыдущий сеанс активен; режим разрешений — %s."), KeyREPLClearConversation: repl("Clear conversation receipt: new empty session %s. Previous session %s remains recoverable.", "清空会话回执：新建空会话 %s。先前会话 %s 仍可恢复。", "Bestätigung zum Leeren der Unterhaltung: Neue leere Sitzung %s. Die vorherige Sitzung %s bleibt wiederherstellbar.", "会話消去の確認: 新しい空のセッション %s。以前のセッション %s は復元できます。", "대화 지우기 확인: 새 빈 세션 %s. 이전 세션 %s은 복구할 수 있습니다.", "Подтверждение очистки диалога: новый пустой сеанс %s. Предыдущий сеанс %s можно восстановить."),
		KeyREPLResumeFailedClosed: repl("Resume failed-closed receipt: previous session remains active; permission mode is %s.", "恢复故障关闭回执：先前会话仍处于活动状态；权限模式为 %s。", "Fail-closed-Bestätigung beim Fortsetzen: Die vorherige Sitzung bleibt aktiv; Berechtigungsmodus ist %s.", "再開のフェイルクローズ確認: 前のセッションはアクティブのままです。権限モードは %s です。", "재개 페일 클로즈 확인: 이전 세션은 활성 상태이며 권한 모드는 %s입니다.", "Подтверждение безопасного сбоя возобновления: предыдущий сеанс активен; режим разрешений — %s."), KeyREPLResumeDegraded: repl("Resume degraded receipt: rollback failed; active session remains %s.", "恢复降级回执：回滚失败；活动会话保持为 %s。", "Eingeschränkte Fortsetzungsbestätigung: Rollback fehlgeschlagen; aktive Sitzung bleibt %s.", "縮退した再開確認: ロールバックに失敗しました。アクティブなセッションは %s のままです。", "저하된 재개 확인: 롤백에 실패했으며 활성 세션은 %s입니다.", "Подтверждение деградированного возобновления: откат не удался; активный сеанс остаётся %s."), KeyREPLResumeCompleted: repl("Resume receipt: active session is %s.", "恢复回执：活动会话为 %s。", "Fortsetzungsbestätigung: Aktive Sitzung ist %s.", "再開確認: アクティブなセッションは %s です。", "재개 확인: 활성 세션은 %s입니다.", "Подтверждение возобновления: активный сеанс — %s."),
		KeyREPLExportCompleted: repl("Export receipt: wrote %d complete messages to %s.", "导出回执：已将 %d 条完整消息写入 %s。", "Exportbestätigung: %d vollständige Nachrichten nach %s geschrieben.", "エクスポート確認: 完全な %d 件のメッセージを %s に書き込みました。", "내보내기 확인: 완전한 메시지 %d개를 %s에 썼습니다.", "Подтверждение экспорта: %d полных сообщений записано в %s."), KeyREPLTranscriptBegins: repl("Transcript begins.", "记录开始。", "Transkript beginnt.", "記録を開始します。", "기록 시작.", "Расшифровка начинается."), KeyREPLTranscriptEnds: repl("Transcript ends.", "记录结束。", "Transkript endet.", "記録を終了します。", "기록 끝.", "Расшифровка закончена."), KeyREPLTurnInterrupted: repl("Turn interrupted.", "当前轮次已中断。", "Zug unterbrochen.", "ターンが中断されました。", "턴이 중단되었습니다.", "Ход прерван."),
		KeyREPLCompactionStarted: repl("Compaction started: trigger %s.", "压缩已开始：触发条件 %s。", "Komprimierung gestartet: Auslöser %s.", "圧縮を開始しました: トリガー %s。", "압축 시작: 트리거 %s.", "Сжатие начато: триггер %s."), KeyREPLCompactionCompleted: repl("Compaction completed: trigger %s.", "压缩已完成：触发条件 %s。", "Komprimierung abgeschlossen: Auslöser %s.", "圧縮が完了しました: トリガー %s。", "압축 완료: 트리거 %s.", "Сжатие завершено: триггер %s."), KeyREPLCompactionFailed: repl("Compaction failed: trigger %s%s.", "压缩失败：触发条件 %s%s。", "Komprimierung fehlgeschlagen: Auslöser %s%s.", "圧縮に失敗しました: トリガー %s%s。", "압축 실패: 트리거 %s%s.", "Сжатие не удалось: триггер %s%s."), KeyREPLCompactionCancelled: repl("Compaction cancelled: trigger %s%s.", "压缩已取消：触发条件 %s%s。", "Komprimierung abgebrochen: Auslöser %s%s.", "圧縮がキャンセルされました: トリガー %s%s。", "압축 취소: 트리거 %s%s.", "Сжатие отменено: триггер %s%s."), KeyREPLCompactionReason: repl("; reason %s", "；原因 %s", "; Grund %s", "；理由 %s", "; 사유 %s", "; причина %s"), KeyREPLCompactionBoundary: repl("Compaction boundary: trigger %s; tokens %d to %d; retained %d; discarded %d.", "压缩边界：触发条件 %s；令牌从 %d 到 %d；保留 %d；丢弃 %d。", "Komprimierungsgrenze: Auslöser %s; Token %d zu %d; behalten %d; verworfen %d.", "圧縮境界: トリガー %s、トークン %d から %d、保持 %d、破棄 %d。", "압축 경계: 트리거 %s, 토큰 %d에서 %d, 유지 %d, 폐기 %d.", "Граница сжатия: триггер %s; токены %d→%d; сохранено %d; отброшено %d."),
		KeyREPLTUIFullAgentThread: repl("[Full agent thread retained in details.]", "[完整 Agent 线程已保留在详情中。]", "[Vollständiger Agent-Thread in Details erhalten.]", "[完全な Agent スレッドは詳細に保持されています。]", "[전체 Agent 스레드는 세부 정보에 보존됩니다.]", "[Полная ветка Agent сохранена в деталях.]"), KeyREPLTUIErrorPrefix: repl("Error: %s", "错误：%s", "Fehler: %s", "エラー: %s", "오류: %s", "Ошибка: %s"), KeyREPLTUIBackgroundCancelled: repl("(background follow-up cancelled)", "（后台后续处理已取消）", "(Hintergrund-Fortsetzung abgebrochen)", "（バックグラウンド継続はキャンセルされました）", "(백그라운드 후속 작업 취소됨)", "(фоновое продолжение отменено)"), KeyREPLTUIQueryCancelled: repl("(cancelled)", "（已取消）", "(abgebrochen)", "（キャンセル済み）", "(취소됨)", "(отменено)"),
		KeyREPLTUIForkCancelled: repl("Fork cancelled", "分叉已取消", "Fork abgebrochen", "フォークをキャンセルしました", "포크 취소됨", "Ветвление отменено"), KeyREPLTUIForkRunning: repl("Cannot fork while a query is running", "查询运行期间无法分叉", "Während einer laufenden Abfrage kann kein Fork erstellt werden", "クエリ実行中はフォークできません", "쿼리 실행 중에는 포크할 수 없습니다", "Нельзя создать ветвь во время выполнения запроса"), KeyREPLTUIForkSnapshotChanged: repl("The conversation changed after the fork menu opened. Open /fork again to use current fork points.", "打开分叉菜单后对话已发生变化。请重新打开 /fork 以使用当前分叉点。", "Die Unterhaltung wurde nach dem Öffnen des Fork-Menüs geändert. Öffnen Sie /fork erneut, um die aktuellen Fork-Punkte zu verwenden.", "フォークメニューを開いた後に会話が変更されました。現在のフォーク地点を使用するには /fork を開き直してください。", "포크 메뉴를 연 후 대화가 변경되었습니다. 현재 포크 지점을 사용하려면 /fork를 다시 여세요.", "После открытия меню ветвления диалог изменился. Снова откройте /fork, чтобы использовать актуальные точки ветвления."), KeyREPLTUIForkFailed: repl("Fork failed: %s", "分叉失败：%s", "Fork fehlgeschlagen: %s", "フォークに失敗しました: %s", "포크 실패: %s", "Не удалось создать ветвь: %s"), KeyREPLTUIForkOpened: repl("Forked session %s in a new terminal tab", "已在新的终端标签页中分叉会话 %s", "Sitzung %s in einem neuen Terminal-Tab geforkt", "セッション %s を新しいターミナルタブにフォークしました", "세션 %s을 새 터미널 탭에 포크했습니다", "Сеанс %s разветвлён в новой вкладке терминала"),
		KeyREPLTUIUnknownCommand: repl("Unknown command: %s", "未知命令：%s", "Unbekannter Befehl: %s", "不明なコマンド: %s", "알 수 없는 명령: %s", "Неизвестная команда: %s"), KeyREPLTUICommandError: repl("Command error: %s", "命令错误：%s", "Befehlsfehler: %s", "コマンドエラー: %s", "명령 오류: %s", "Ошибка команды: %s"), KeyREPLTUIResumeCancelled: repl("Resume cancelled", "恢复已取消", "Fortsetzen abgebrochen", "再開をキャンセルしました", "재개 취소됨", "Возобновление отменено"), KeyREPLTUIResumeFailed: repl("Error loading session: %s", "加载会话出错：%s", "Fehler beim Laden der Sitzung: %s", "セッション読み込みエラー: %s", "세션 로드 오류: %s", "Ошибка загрузки сеанса: %s"), KeyREPLTUIResumeCompleted: repl("Resumed session %s", "已恢复会话 %s", "Sitzung %s fortgesetzt", "セッション %s を再開しました", "세션 %s을 재개했습니다", "Сеанс %s возобновлён"), KeyREPLTUIGoalRefreshFailed: repl("Goal status refresh failed: %s", "目标状态刷新失败：%s", "Aktualisierung des Zielstatus fehlgeschlagen: %s", "目標状態の更新に失敗しました: %s", "목표 상태 새로고침 실패: %s", "Не удалось обновить статус цели: %s"), KeyREPLTUIQueryRunning: repl("A query is already running", "已有查询正在运行", "Eine Abfrage läuft bereits", "すでにクエリが実行中です", "이미 쿼리가 실행 중입니다", "Запрос уже выполняется"), KeyREPLTUIInputBlocked: repl("(input blocked by hook: %s)", "（输入被钩子阻止：%s）", "(Eingabe wurde durch Hook blockiert: %s)", "（入力はフックによりブロックされました: %s）", "(입력이 훅에 의해 차단됨: %s)", "(ввод заблокирован хуком: %s)"), KeyREPLTUIImageLoadFailed: repl("image load error: %s", "图像加载错误：%s", "Fehler beim Laden des Bildes: %s", "画像読み込みエラー: %s", "이미지 로드 오류: %s", "Ошибка загрузки изображения: %s"), KeyREPLTUIImageUnsupported: repl("Current model does not support image input. Switch to a vision model before sending images.", "当前模型不支持图像输入。发送图像前请切换到视觉模型。", "Das aktuelle Modell unterstützt keine Bildeingabe. Wechsle vor dem Senden von Bildern zu einem Vision-Modell.", "現在のモデルは画像入力に対応していません。画像を送る前にビジョンモデルへ切り替えてください。", "현재 모델은 이미지 입력을 지원하지 않습니다. 이미지를 보내기 전에 비전 모델로 전환하세요.", "Текущая модель не поддерживает изображения. Перед отправкой переключитесь на vision-модель."),
		KeyREPLTUIBackgroundFailed:       repl("Background follow-up failed: %s", "后台后续处理失败：%s", "Hintergrund-Fortsetzung fehlgeschlagen: %s", "バックグラウンド継続に失敗しました: %s", "백그라운드 후속 작업 실패: %s", "Фоновое продолжение не удалось: %s"),
		KeyREPLTUIModelSaveFailed:        repl("Could not save model preference: %s", "无法保存 model 偏好设置：%s", "Modelleinstellung konnte nicht gespeichert werden: %s", "model の設定を保存できませんでした: %s", "model 설정을 저장하지 못했습니다: %s", "Не удалось сохранить настройку model: %s"),
		KeyREPLTUIContextWindowRange:     repl("Context window must be between %d and %d", "上下文窗口必须介于 %d 和 %d 之间", "Das Kontextfenster muss zwischen %d und %d liegen", "コンテキストウィンドウは %d から %d の範囲にしてください", "컨텍스트 창은 %d에서 %d 사이여야 합니다", "Размер окна контекста должен быть от %d до %d"),
		KeyREPLTUIModeSwitchFailed:       repl("Could not switch mode: %s", "无法切换模式：%s", "Modus konnte nicht gewechselt werden: %s", "モードを切り替えられませんでした: %s", "모드를 전환하지 못했습니다: %s", "Не удалось переключить режим: %s"),
		KeyREPLTUIActivityActionFailed:   repl("Activity action failed: %s", "活动操作失败：%s", "Aktivitätsaktion fehlgeschlagen: %s", "アクティビティ操作に失敗しました: %s", "활동 작업 실패: %s", "Действие с активностью не выполнено: %s"),
		KeyREPLTUILifecycleSaveFailed:    repl("Could not save TUI session state: %v", "无法保存 TUI 会话状态：%v", "TUI-Sitzungsstatus konnte nicht gespeichert werden: %v", "TUI セッションの状態を保存できませんでした: %v", "TUI 세션 상태를 저장하지 못했습니다: %v", "Не удалось сохранить состояние сеанса TUI: %v"),
		KeyREPLTUICleanupFailed:          repl("TUI cleanup failed", "TUI 清理失败", "TUI-Bereinigung fehlgeschlagen", "TUI のクリーンアップに失敗しました", "TUI 정리 실패", "Не удалось очистить TUI"),
		KeyREPLTUIAgentGroupTotal:        repl("Agent group: %d total", "Agent 组：共 %d 个", "Agent-Gruppe: insgesamt %d", "Agent グループ: 合計 %d", "Agent 그룹: 총 %d개", "Группа Agent: всего %d"),
		KeyREPLTUIAgentCountFailed:       repl("%d failed", "%d 个失败", "%d fehlgeschlagen", "%d 件失敗", "%d개 실패", "ошибок: %d"),
		KeyREPLTUIAgentCountRunning:      repl("%d running", "%d 个运行中", "%d aktiv", "%d 件実行中", "%d개 실행 중", "выполняется: %d"),
		KeyREPLTUIAgentCountReady:        repl("%d ready for review", "%d 个待审核", "%d zur Prüfung bereit", "%d 件レビュー待ち", "%d개 검토 대기", "готово к проверке: %d"),
		KeyREPLTUIAgentCountCancelled:    repl("%d cancelled", "%d 个已取消", "%d abgebrochen", "%d 件キャンセル済み", "%d개 취소됨", "отменено: %d"),
		KeyREPLTUIAgentMember:            repl("Agent %s, %s", "Agent %s，%s", "Agent %s, %s", "Agent %s、%s", "Agent %s, %s", "Agent %s, %s"),
		KeyREPLTUIStatusReady:            repl("ready for review", "待审核", "zur Prüfung bereit", "レビュー待ち", "검토 대기", "готов к проверке"),
		KeyREPLTUIStatusFailed:           repl("failed", "失败", "fehlgeschlagen", "失敗", "실패", "ошибка"),
		KeyREPLTUIStatusRunning:          repl("running", "运行中", "aktiv", "実行中", "실행 중", "выполняется"),
		KeyREPLTUIStatusCancelled:        repl("cancelled", "已取消", "abgebrochen", "キャンセル済み", "취소됨", "отменён"),
		KeyREPLTUIStatusKilled:           repl("killed", "已终止", "beendet", "強制終了", "종료됨", "остановлен"),
		KeyREPLTUIAgentAttempt:           repl("attempt %d", "第 %d 次尝试", "Versuch %d", "%d 回目の試行", "%d번째 시도", "попытка %d"),
		KeyREPLTUIAgentPath:              repl("path %s", "路径 %s", "Pfad %s", "パス %s", "경로 %s", "путь %s"),
		KeyREPLTUIAgentDetails:           repl("details available", "可查看详情", "Details verfügbar", "詳細を確認できます", "세부 정보 있음", "подробности доступны"),
		KeyREPLTUISummarySeparator:       repl(". ", "；", ". ", "。", ". ", ". "),
		KeyREPLTUISummaryEnd:             repl(".", "。", ".", "。", ".", "."),
		KeyREPLTUITokenCount:             repl("%d tokens", "%d 个 Token", "%d Token", "%d トークン", "토큰 %d개", "%d токенов"),
		KeyREPLTUIThreadRetained:         repl("thread retained", "线程已保留", "Thread gespeichert", "スレッドを保持済み", "스레드 보존됨", "ветка сохранена"),
		KeyREPLTUIThreadUnavailable:      repl("thread unavailable", "线程不可用", "Thread nicht verfügbar", "スレッドを利用できません", "스레드 사용 불가", "ветка недоступна"),
		KeyREPLTUITranscriptUnretained:   repl("Agent transcript could not be retained", "无法保留 Agent 记录", "Agent-Transkript konnte nicht gespeichert werden", "Agent の記録を保持できませんでした", "Agent 기록을 보존하지 못했습니다", "Не удалось сохранить журнал Agent"),
		KeyREPLTUIQueuedCount:            repl("%d queued", "%d 个排队中", "%d in Warteschlange", "%d 件待機中", "%d개 대기 중", "в очереди: %d"),
		KeyREPLTUIQueuedReason:           repl("%s (%s)", "%s（%s）", "%s (%s)", "%s（%s）", "%s (%s)", "%s (%s)"),
		KeyREPLTUITerminalReason:         repl("reason %s", "原因 %s", "Grund %s", "理由 %s", "사유 %s", "причина: %s"),
		KeyREPLTUIArtifactCount:          repl("%d artifacts", "%d 个 artifacts", "%d Artefakte", "%d 件の artifacts", "artifacts %d개", "артефактов: %d"),
		KeyREPLTUIVerificationCount:      repl("%d verification refs", "%d 个验证引用", "%d Prüfverweise", "%d 件の検証参照", "검증 참조 %d개", "ссылок проверки: %d"),
		KeyREPLTUIToolName:               repl("tool %s", "工具 %s", "Tool %s", "ツール %s", "도구 %s", "инструмент %s"),
		KeyREPLTUIUpdatedAt:              repl("updated %s", "更新于 %s", "aktualisiert %s", "更新 %s", "%s 업데이트", "обновлено %s"),
		KeyREPLTUITranscriptClosed:       repl("Transcript search closed", "已关闭记录搜索", "Transkriptsuche geschlossen", "記録の検索を閉じました", "기록 검색 닫힘", "Поиск по журналу закрыт"),
		KeyREPLTUITranscriptNoMatches:    repl("No transcript matches", "记录中没有匹配项", "Keine Treffer im Transkript", "記録に一致する項目はありません", "기록에서 일치 항목을 찾지 못했습니다", "В журнале нет совпадений"),
		KeyREPLTUITranscriptMatch:        repl("Transcript match (%d total): %s", "记录匹配项（共 %d 个）：%s", "Transkripttreffer (insgesamt %d): %s", "記録の一致項目（合計 %d 件）: %s", "기록 일치 항목(총 %d개): %s", "Совпадение в журнале (всего %d): %s"),
		KeyREPLTUIStopped:                repl("TUI stopped", "TUI 已停止", "TUI wurde beendet", "TUI は停止しました", "TUI가 중지되었습니다", "TUI остановлен"),
		KeyREPLTUIModelCatalogMissing:    repl("Model catalog is unavailable", "Model catalog 不可用", "Model Catalog ist nicht verfügbar", "Model catalog を利用できません", "Model catalog를 사용할 수 없습니다", "Model catalog недоступен"),
		KeyREPLTUIMouseModeInvalid:       repl("Invalid mouse mode %q", "无效的鼠标模式 %q", "Ungültiger Mausmodus %q", "無効なマウスモード %q", "잘못된 마우스 모드 %q", "Недопустимый режим мыши %q"),
		KeyREPLTUIActivityViewOpened:     repl("Activity view opened: %d total, %d running, %d need input", "活动视图已打开：共 %d 个，%d 个运行中，%d 个需要输入", "Aktivitätsansicht geöffnet: insgesamt %d, %d aktiv, %d benötigen Eingabe", "アクティビティ表示を開きました: 合計 %d、実行中 %d、入力待ち %d", "활동 보기를 열었습니다: 총 %d개, 실행 중 %d개, 입력 필요 %d개", "Просмотр активностей открыт: всего %d, выполняется %d, ожидают ввода %d"),
		KeyREPLTUIActivityViewClosed:     repl("Activity view closed", "活动视图已关闭", "Aktivitätsansicht geschlossen", "アクティビティ表示を閉じました", "활동 보기를 닫았습니다", "Просмотр активностей закрыт"),
		KeyREPLTUIDisclosureUnknown:      repl("Unknown disclosure level %q", "未知的披露级别 %q", "Unbekannte Offenlegungsstufe %q", "不明な開示レベル %q", "알 수 없는 공개 수준 %q", "Неизвестный уровень раскрытия %q"),
		KeyREPLTUIDisclosureReceipt:      repl("Observation %s disclosure: %d", "Observation %s 的披露级别：%d", "Offenlegungsstufe der Observation %s: %d", "Observation %s の開示レベル: %d", "Observation %s 공개 수준: %d", "Уровень раскрытия Observation %s: %d"),
		KeyREPLTUINoProviders:            repl("No providers registered.", "尚未注册 Provider。", "Keine Provider registriert.", "Provider が登録されていません。", "등록된 Provider가 없습니다.", "Нет зарегистрированных Provider."),
		KeyREPLTUIProviderCredsFailed:    repl("Failed to load provider credentials: %s", "加载 Provider 凭据失败：%s", "Provider-Anmeldedaten konnten nicht geladen werden: %s", "Provider の認証情報を読み込めませんでした: %s", "Provider 자격 증명을 불러오지 못했습니다: %s", "Не удалось загрузить данные доступа Provider: %s"),
		KeyREPLTUIProviderCreateFailed:   repl("Failed to create provider: %s", "创建 Provider 失败：%s", "Provider konnte nicht erstellt werden: %s", "Provider を作成できませんでした: %s", "Provider를 만들지 못했습니다: %s", "Не удалось создать Provider: %s"),
		KeyREPLTUIModelSwitched:          repl("Switched to %s/%s", "已切换到 %s/%s", "Zu %s/%s gewechselt", "%s/%s に切り替えました", "%s/%s(으)로 전환했습니다", "Выполнено переключение на %s/%s"),
		KeyREPLTUIModelSwitchedReasoning: repl("Switched to %s/%s (reasoning: %s)", "已切换到 %s/%s（reasoning：%s）", "Zu %s/%s gewechselt (Reasoning: %s)", "%s/%s に切り替えました（reasoning: %s）", "%s/%s(으)로 전환했습니다(reasoning: %s)", "Выполнено переключение на %s/%s (reasoning: %s)"),
		KeyREPLTUIModelPickerCancelled:   repl("Model picker cancelled", "已取消 Model 选择", "Model-Auswahl abgebrochen", "Model の選択をキャンセルしました", "Model 선택 취소됨", "Выбор Model отменён"),
		KeyREPLTUICredentialStoreMissing: repl("Credential store is unavailable", "Credential store 不可用", "Credential Store ist nicht verfügbar", "Credential store を利用できません", "Credential store를 사용할 수 없습니다", "Credential store недоступен"),
		KeyREPLTUICredentialSaveFailed:   repl("Failed to save credentials: %s", "保存凭据失败：%s", "Anmeldedaten konnten nicht gespeichert werden: %s", "認証情報を保存できませんでした: %s", "자격 증명을 저장하지 못했습니다: %s", "Не удалось сохранить данные доступа: %s"),
		KeyREPLTUIProviderConnected:      repl("Connected to %s - select a model", "已连接到 %s，请选择 model", "Mit %s verbunden - Model auswählen", "%s に接続しました - model を選択してください", "%s에 연결됨 - model을 선택하세요", "Подключено к %s - выберите model"),
		KeyREPLTUIFetchingModels:         repl("Fetching models…", "正在获取模型列表…", "Modelle werden abgerufen…", "モデル一覧を取得中…", "모델 목록을 가져오는 중…", "Получаем список моделей…"),
		KeyREPLTUIProviderDeleted:        repl("Deleted provider %s", "已删除供应商 %s", "Anbieter %s gelöscht", "プロバイダー %s を削除しました", "공급자 %s 삭제됨", "Поставщик %s удалён"),
		KeyREPLTUIOAuthWaiting:           repl("Waiting for OAuth authorization in your browser...", "正在等待浏览器中的 OAuth 授权...", "Warte auf die OAuth-Autorisierung im Browser ...", "ブラウザでの OAuth 認証を待っています...", "브라우저의 OAuth 인증을 기다리는 중...", "Ожидание авторизации OAuth в браузере..."),
		KeyREPLTUIOAuthStarting:          repl("Starting OAuth flow for %s - check your browser...", "正在为 %s 启动 OAuth flow，请查看浏览器...", "OAuth-Flow für %s wird gestartet - bitte Browser prüfen ...", "%s の OAuth flow を開始しています - ブラウザを確認してください...", "%s의 OAuth flow 시작 중 - 브라우저를 확인하세요...", "Запускается OAuth flow для %s - проверьте браузер..."),
		KeyREPLTUIOAuthFailed:            repl("OAuth failed for %s: %s", "%s 的 OAuth 失败：%s", "OAuth für %s fehlgeschlagen: %s", "%s の OAuth に失敗しました: %s", "%s의 OAuth 실패: %s", "OAuth для %s завершился ошибкой: %s"),
		KeyREPLTUIDeviceAuthWaiting:      repl("Waiting for device authorization...", "正在等待 device auth...", "Warte auf device auth ...", "device auth を待っています...", "device auth 대기 중...", "Ожидание device auth..."),
		KeyREPLTUIDeviceAuthStarting:     repl("Starting device auth for %s...", "正在为 %s 启动 device auth...", "device auth für %s wird gestartet ...", "%s の device auth を開始しています...", "%s의 device auth 시작 중...", "Запускается device auth для %s..."),
		KeyREPLTUIDeviceAuthFailed:       repl("Device auth failed for %s: %s", "%s 的 device auth 失败：%s", "device auth für %s fehlgeschlagen: %s", "%s の device auth に失敗しました: %s", "%s의 device auth 실패: %s", "device auth для %s завершился ошибкой: %s"),
		KeyREPLTUIVertexBaseURLRequired:  repl("Base URL is required for Vertex API key mode", "Vertex API key 模式需要 Base URL", "Für den Vertex-API-key-Modus ist eine Base URL erforderlich", "Vertex API key モードには Base URL が必要です", "Vertex API key 모드에는 Base URL이 필요합니다", "Для режима Vertex API key требуется Base URL"),
		KeyREPLTUIBaseURLAbsolute:        repl("Base URL must be an absolute http:// or https:// URL", "Base URL 必须是绝对的 http:// 或 https:// URL", "Die Base URL muss eine absolute http://- oder https://-URL sein", "Base URL は絶対 http:// URL または https:// URL である必要があります", "Base URL은 절대 http:// 또는 https:// URL이어야 합니다", "Base URL должен быть абсолютным URL с http:// или https://"),
		KeyREPLTUIBaseURLCredentials:     repl("Base URL must not contain embedded credentials", "Base URL 不能包含嵌入式凭据", "Die Base URL darf keine eingebetteten Anmeldedaten enthalten", "Base URL に認証情報を埋め込むことはできません", "Base URL에 자격 증명을 포함할 수 없습니다", "Base URL не должен содержать встроенные данные доступа"),
		KeyREPLTUIUnknownProvider:        repl("Unknown provider: %s", "未知 Provider：%s", "Unbekannter Provider: %s", "不明な Provider: %s", "알 수 없는 Provider: %s", "Неизвестный Provider: %s"),
		KeyREPLTUIActivityNotFound:       repl("Activity %q not found", "未找到活动 %q", "Aktivität %q nicht gefunden", "アクティビティ %q が見つかりません", "활동 %q을(를) 찾을 수 없습니다", "Активность %q не найдена"),
		KeyREPLTUIActivityNotCancellable: repl("Activity %q cannot be cancelled", "活动 %q 无法取消", "Aktivität %q kann nicht abgebrochen werden", "アクティビティ %q はキャンセルできません", "활동 %q은(는) 취소할 수 없습니다", "Активность %q нельзя отменить"),
		KeyREPLTUIActivityNoController:   repl("Activity %q has no cancellation controller", "活动 %q 没有取消控制器", "Aktivität %q hat keine Abbruchsteuerung", "アクティビティ %q にはキャンセルコントローラーがありません", "활동 %q에 취소 컨트롤러가 없습니다", "У активности %q нет контроллера отмены"),
		KeyREPLTUIActivityCancelled:      repl("Cancelled activity %s", "已取消活动 %s", "Aktivität %s abgebrochen", "アクティビティ %s をキャンセルしました", "활동 %s 취소됨", "Активность %s отменена"),
		KeyREPLTUIActivityNoJump:         repl("Activity %q has no jump target", "活动 %q 没有跳转目标", "Aktivität %q hat kein Sprungziel", "アクティビティ %q には移動先がありません", "활동 %q에 이동 대상이 없습니다", "У активности %q нет цели перехода"),
		KeyREPLTUIActivityJumped:         repl("Jumped to %s", "已跳转到 %s", "Zu %s gesprungen", "%s に移動しました", "%s(으)로 이동했습니다", "Переход к %s выполнен"),
		KeyREPLTUIActivityLocated:        repl("Located activity %s", "已定位活动 %s", "Aktivität %s gefunden", "アクティビティ %s を表示しました", "활동 %s을(를) 찾았습니다", "Активность %s найдена"),
		KeyREPLTUIActivityNoDetails:      repl("Activity %q has no retained details", "活动 %q 没有保留详情", "Für Aktivität %q sind keine Details gespeichert", "アクティビティ %q の詳細は保持されていません", "활동 %q에 보존된 세부 정보가 없습니다", "Для активности %q нет сохранённых подробностей"),
		KeyREPLTUIActivityEvidenceOpened: repl("Opened evidence for %s", "已打开 %s 的证据", "Nachweise für %s geöffnet", "%s のエビデンスを開きました", "%s의 근거를 열었습니다", "Открыты свидетельства для %s"),
		KeyREPLTUIActivityNoAttention:    repl("Activity %q has no unread attention", "活动 %q 没有未读提醒", "Aktivität %q hat keinen ungelesenen Hinweis", "アクティビティ %q に未読の注意事項はありません", "활동 %q에 읽지 않은 알림이 없습니다", "У активности %q нет непрочитанных уведомлений"),
		KeyREPLTUIActivityAcknowledged:   repl("Acknowledged activity %s", "已确认活动 %s", "Aktivität %s bestätigt", "アクティビティ %s を確認しました", "활동 %s 확인됨", "Активность %s отмечена как прочитанная"),
		KeyREPLTUIActivityUnknownAction:  repl("Unknown activity action %q", "未知的活动操作 %q", "Unbekannte Aktivitätsaktion %q", "不明なアクティビティ操作 %q", "알 수 없는 활동 작업 %q", "Неизвестное действие с активностью %q"),
		KeyREPLTUIQueryStartFailed:       repl("Could not start query: %s", "无法启动查询：%s", "Abfrage konnte nicht gestartet werden: %s", "クエリを開始できませんでした: %s", "쿼리를 시작하지 못했습니다: %s", "Не удалось запустить запрос: %s"),
		KeyREPLTUIHookSummary:            repl("Hook %s: %s%s", "Hook %s：%s%s", "Hook %s: %s%s", "Hook %s: %s%s", "Hook %s: %s%s", "Hook %s: %s%s"),
		KeyREPLTUIContextCompaction:      repl("Context compaction", "上下文压缩", "Kontextkomprimierung", "コンテキスト圧縮", "컨텍스트 압축", "Сжатие контекста"),
		KeyREPLTUICompactionCompacting:   repl("compacting", "压缩中", "wird komprimiert", "圧縮中", "압축 중", "сжатие"),
		KeyREPLTUICompactionFailed:       repl("failed", "失败", "fehlgeschlagen", "失敗", "실패", "ошибка"),
		KeyREPLTUICompactionCancelled:    repl("cancelled", "已取消", "abgebrochen", "キャンセル済み", "취소됨", "отменено"),
		KeyREPLTUICompactionIdle:         repl("idle", "空闲", "inaktiv", "待機中", "대기 중", "ожидание"),
		KeyREPLTUIHookBlocked:            repl("blocked", "已阻止", "blockiert", "ブロック済み", "차단됨", "заблокирован"),
		KeyREPLTUIHookSucceeded:          repl("succeeded", "成功", "erfolgreich", "成功", "성공", "успешно"),
		KeyREPLTUIHookFailed:             repl("failed", "失败", "fehlgeschlagen", "失敗", "실패", "ошибка"),
		KeyREPLTUIHookCancelled:          repl("cancelled", "已取消", "abgebrochen", "キャンセル済み", "취소됨", "отменён"),
		KeyREPLTUIHookRunning:            repl("running", "运行中", "aktiv", "実行中", "실행 중", "выполняется"),
	} {
		semanticTranslations[key] = translations
	}
}

func repl(en, zh, de, ja, ko, ru string) map[Language]string {
	return map[Language]string{LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru}
}
