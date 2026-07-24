package i18n

// Semantic keys for REPL errors that can reach command receipts, renderers, or
// terminal diagnostics. Underlying errors, IDs, command syntax, and mode codes
// are interpolated unchanged by the caller.
const (
	KeyREPLErrorScreenReaderNotConfigured     Key = "repl.error.screen_reader.not_configured"
	KeyREPLErrorCloseScreenReaderInput        Key = "repl.error.screen_reader.close_input"
	KeyREPLErrorFollowUpUnsupported           Key = "repl.error.follow_up.unsupported"
	KeyREPLErrorFollowUpUnavailable           Key = "repl.error.follow_up.unavailable"
	KeyREPLErrorFollowUpTaskUnresolved        Key = "repl.error.follow_up.task_unresolved"
	KeyREPLErrorUsage                         Key = "repl.error.usage"
	KeyREPLErrorUnknownCommandHelp            Key = "repl.error.command.unknown_help"
	KeyREPLErrorUnknownCommand                Key = "repl.error.command.unknown"
	KeyREPLErrorForkUnavailable               Key = "repl.error.fork.unavailable"
	KeyREPLErrorForkLoadConversation          Key = "repl.error.fork.load_conversation"
	KeyREPLErrorForkSelectionRange            Key = "repl.error.fork.selection_range"
	KeyREPLErrorDoctorEngineRequired          Key = "repl.error.doctor.engine_required"
	KeyREPLErrorSessionRepositoryUnavailable  Key = "repl.error.session.repository_unavailable"
	KeyREPLErrorDeletionBoundaryUnavailable   Key = "repl.error.session.deletion_boundary_unavailable"
	KeyREPLErrorSessionIDRequired             Key = "repl.error.session.id_required"
	KeyREPLErrorDeleteActiveSession           Key = "repl.error.session.delete_active"
	KeyREPLErrorDeleteActiveSessionGuidance   Key = "repl.error.session.delete_active_guidance"
	KeyREPLErrorDeletionNotApproved           Key = "repl.error.session.deletion_not_approved"
	KeyREPLErrorActiveSessionChanged          Key = "repl.error.session.active_changed"
	KeyREPLErrorSessionSwitchUnavailable      Key = "repl.error.session.switch_unavailable"
	KeyREPLErrorSessionTransitionLockMissing  Key = "repl.error.session.transition_lock_missing"
	KeyREPLErrorSessionTransitionBoundary     Key = "repl.error.session.transition_boundary_unavailable"
	KeyREPLErrorActiveSessionIdentityMissing  Key = "repl.error.session.active_identity_missing"
	KeyREPLErrorSwitchWhileQueryRunning       Key = "repl.error.session.switch_query_running"
	KeyREPLErrorClearWhileQueryRunning        Key = "repl.error.session.clear_query_running"
	KeyREPLErrorSaveCurrentLifecycle          Key = "repl.error.session.save_current_lifecycle"
	KeyREPLErrorRestorePermissionMode         Key = "repl.error.session.restore_permission_mode"
	KeyREPLErrorResetPermissionMode           Key = "repl.error.session.reset_permission_mode"
	KeyREPLErrorSessionTransitionFailed       Key = "repl.error.session.transition_failed"
	KeyREPLErrorRollbackModeFailedClosed      Key = "repl.error.rollback.mode_failed_closed"
	KeyREPLErrorRollbackMode                  Key = "repl.error.rollback.mode"
	KeyREPLErrorRollbackSessionTargetRetained Key = "repl.error.rollback.session_target_retained"
	KeyREPLErrorRollbackSessionTargetPublish  Key = "repl.error.rollback.session_target_publish"
	KeyREPLErrorPublishSessionStopped         Key = "repl.error.session.publish_stopped"
	KeyREPLErrorPublishSession                Key = "repl.error.session.publish"
	KeyREPLErrorPublishEmptySessionStopped    Key = "repl.error.empty_session.publish_stopped"
	KeyREPLErrorPublishEmptySessionDetails    Key = "repl.error.empty_session.publish_details"
	KeyREPLErrorActivateEmptySession          Key = "repl.error.empty_session.activate"
	KeyREPLErrorCreateEmptySession            Key = "repl.error.empty_session.create"
	KeyREPLErrorSaveEmptySessionLifecycle     Key = "repl.error.empty_session.save_lifecycle"
	KeyREPLErrorEmptySessionResumeUnsupported Key = "repl.error.empty_session.resume_unsupported"
	KeyREPLErrorPrepareEmptySession           Key = "repl.error.empty_session.prepare"
	KeyREPLErrorCreateTUIApp                  Key = "repl.error.tui.create_app"
	KeyREPLErrorLoadInitialTUISession         Key = "repl.error.tui.load_initial_session"
	KeyREPLErrorPrepareInitialTUISession      Key = "repl.error.tui.prepare_initial_session"
	KeyREPLErrorApplyInitialTUISession        Key = "repl.error.tui.apply_initial_session"
	KeyREPLErrorRestoreInitialTUIMode         Key = "repl.error.tui.restore_initial_mode"
	KeyREPLErrorLoadLifecycleMetadata         Key = "repl.error.session.load_lifecycle_metadata"
	KeyREPLErrorLoadScreenReaderMetadata      Key = "repl.error.screen_reader.load_lifecycle_metadata"
	KeyREPLErrorRestoreScreenReaderMode       Key = "repl.error.screen_reader.restore_permission_mode"
	KeyREPLErrorRecoverDurableEvidence        Key = "repl.error.tui.recover_durable_evidence"
	KeyREPLErrorLoadTranscriptForLifecycle    Key = "repl.error.session.load_transcript_for_lifecycle"
	KeyREPLErrorCreateTranscriptForLifecycle  Key = "repl.error.session.create_transcript_for_lifecycle"
	KeyREPLErrorModeSwitchCommitting          Key = "repl.error.mode.transition_committing"
	KeyREPLErrorTUIStateRequired              Key = "repl.error.tui.state_required"
	KeyREPLErrorExitPlanMode                  Key = "repl.error.mode.exit_plan"
	KeyREPLErrorPersistPlanMode               Key = "repl.error.mode.persist_plan"
	KeyREPLErrorSwitchMode                    Key = "repl.error.mode.switch"
	KeyREPLErrorPersistRestoredPlanMode       Key = "repl.error.mode.persist_restored_plan"
	KeyREPLErrorDecisionSessionMissing        Key = "repl.error.decision.session_missing"
	KeyREPLErrorDecisionWrongSession          Key = "repl.error.decision.wrong_session"
	KeyREPLErrorDecisionSessionEmpty          Key = "repl.error.decision.session_empty"
)

func init() {
	for key, translations := range map[Key]map[Language]string{
		KeyREPLErrorScreenReaderNotConfigured:     repl("Screen-reader mode is not fully configured", "屏幕阅读器模式尚未完成配置", "Der Screenreader-Modus ist nicht vollständig konfiguriert", "スクリーンリーダーモードの設定が完了していません", "스크린 리더 모드 설정이 완료되지 않았습니다", "Режим экранного диктора настроен не полностью"),
		KeyREPLErrorCloseScreenReaderInput:        repl("Could not close screen-reader input", "无法关闭屏幕阅读器输入", "Screenreader-Eingabe konnte nicht geschlossen werden", "スクリーンリーダー入力を閉じられませんでした", "스크린 리더 입력을 닫지 못했습니다", "Не удалось закрыть ввод экранного диктора"),
		KeyREPLErrorFollowUpUnsupported:           repl("The engine does not support queued follow-up turns", "当前 engine 不支持排队的后续轮次", "Die Engine unterstützt keine eingereihten Folgerunden", "この engine はキューに入れた継続ターンに対応していません", "현재 engine은 대기열 후속 턴을 지원하지 않습니다", "Engine не поддерживает очереди последующих ходов"),
		KeyREPLErrorFollowUpUnavailable:           repl("Background follow-up is unavailable", "后台后续处理不可用", "Die Hintergrund-Fortsetzung ist nicht verfügbar", "バックグラウンド継続を利用できません", "백그라운드 후속 작업을 사용할 수 없습니다", "Фоновое продолжение недоступно"),
		KeyREPLErrorFollowUpTaskUnresolved:        repl("Could not resolve task %s to its owning session", "无法确定 task %s 所属的 session", "Die zuständige Sitzung für Aufgabe %s konnte nicht ermittelt werden", "task %s を所有する session を特定できませんでした", "task %s의 소유 session을 확인하지 못했습니다", "Не удалось определить session, которой принадлежит task %s"),
		KeyREPLErrorUsage:                         repl("Usage: %s", "用法：%s", "Verwendung: %s", "使用方法: %s", "사용법: %s", "Использование: %s"),
		KeyREPLErrorUnknownCommandHelp:            repl("Unknown command %s; use /help", "未知命令 %s；请使用 /help", "Unbekannter Befehl %s; verwende /help", "不明なコマンド %s です。/help を使用してください", "알 수 없는 명령 %s; /help를 사용하세요", "Неизвестная команда %s; используйте /help"),
		KeyREPLErrorUnknownCommand:                repl("Unknown command %s", "未知命令 %s", "Unbekannter Befehl %s", "不明なコマンド %s", "알 수 없는 명령 %s", "Неизвестная команда %s"),
		KeyREPLErrorForkUnavailable:               repl("Fork runtime is unavailable", "Fork runtime 不可用", "Die Fork-Laufzeit ist nicht verfügbar", "Fork runtime を利用できません", "Fork runtime을 사용할 수 없습니다", "Fork runtime недоступна"),
		KeyREPLErrorForkLoadConversation:          repl("Could not load the conversation for forking", "无法加载要 fork 的对话", "Die Unterhaltung für den Fork konnte nicht geladen werden", "Fork する会話を読み込めませんでした", "Fork할 대화를 불러오지 못했습니다", "Не удалось загрузить диалог для Fork"),
		KeyREPLErrorForkSelectionRange:            repl("Fork selection must be a number from 1 to %d", "Fork 选项必须是 1 到 %d 之间的数字", "Die Fork-Auswahl muss eine Zahl von 1 bis %d sein", "Fork の選択には 1 から %d までの数字を指定してください", "Fork 선택은 1부터 %d 사이의 숫자여야 합니다", "Для Fork нужно выбрать число от 1 до %d"),
		KeyREPLErrorDoctorEngineRequired:          repl("Doctor diagnostics require an active engine", "Doctor 诊断需要可用的 engine", "Die Doctor-Diagnose benötigt eine aktive Engine", "Doctor 診断には有効な engine が必要です", "Doctor 진단에는 활성 engine이 필요합니다", "Для диагностики Doctor требуется активная engine"),
		KeyREPLErrorSessionRepositoryUnavailable:  repl("Session repository is unavailable", "Session repository 不可用", "Das Sitzungs-Repository ist nicht verfügbar", "Session repository を利用できません", "Session repository를 사용할 수 없습니다", "Session repository недоступен"),
		KeyREPLErrorDeletionBoundaryUnavailable:   repl("The session-deletion decision boundary is unavailable", "Session 删除决策边界不可用", "Die Entscheidungsgrenze zum Löschen der Sitzung ist nicht verfügbar", "Session 削除の決定境界を利用できません", "Session 삭제 결정 경계를 사용할 수 없습니다", "Контур принятия решения об удалении session недоступен"),
		KeyREPLErrorSessionIDRequired:             repl("Session ID is required", "需要 session ID", "Eine Sitzungs-ID ist erforderlich", "Session ID が必要です", "Session ID가 필요합니다", "Требуется session ID"),
		KeyREPLErrorDeleteActiveSession:           repl("The active session cannot be deleted", "无法删除当前 session", "Die aktive Sitzung kann nicht gelöscht werden", "アクティブな session は削除できません", "활성 session은 삭제할 수 없습니다", "Активную session нельзя удалить"),
		KeyREPLErrorDeleteActiveSessionGuidance:   repl("The active session cannot be deleted; start or resume another conversation first", "无法删除当前 session；请先开始或恢复另一个对话", "Die aktive Sitzung kann nicht gelöscht werden; starte oder setze zuerst eine andere Unterhaltung fort", "アクティブな session は削除できません。先に別の会話を開始または再開してください", "활성 session은 삭제할 수 없습니다. 먼저 다른 대화를 시작하거나 재개하세요", "Активную session нельзя удалить; сначала начните или возобновите другой диалог"),
		KeyREPLErrorDeletionNotApproved:           repl("Session history deletion was not approved", "未批准删除 session 历史记录", "Das Löschen des Sitzungsverlaufs wurde nicht genehmigt", "Session 履歴の削除は承認されませんでした", "Session 기록 삭제가 승인되지 않았습니다", "Удаление истории session не было одобрено"),
		KeyREPLErrorActiveSessionChanged:          repl("The active session changed after deletion approval; session history was not deleted", "删除获批后当前 session 已发生变化；未删除 session 历史记录", "Die aktive Sitzung hat sich nach der Löschfreigabe geändert; der Sitzungsverlauf wurde nicht gelöscht", "削除の承認後にアクティブな session が変わったため、履歴は削除されませんでした", "삭제 승인 후 활성 session이 변경되어 기록을 삭제하지 않았습니다", "После одобрения удаления активная session изменилась; история не удалена"),
		KeyREPLErrorSessionSwitchUnavailable:      repl("Session switching is unavailable in this runtime", "当前 runtime 不支持切换 session", "Der Sitzungswechsel ist in dieser Laufzeit nicht verfügbar", "この runtime では session を切り替えられません", "현재 runtime에서는 session을 전환할 수 없습니다", "Переключение session недоступно в этой runtime"),
		KeyREPLErrorSessionTransitionLockMissing:  repl("The session-transition lock is not configured", "尚未配置 session 切换锁", "Die Sperre für Sitzungswechsel ist nicht konfiguriert", "Session 切り替えロックが設定されていません", "Session 전환 잠금이 설정되지 않았습니다", "Блокировка перехода session не настроена"),
		KeyREPLErrorSessionTransitionBoundary:     repl("The session-transition boundary is unavailable", "Session 切换边界不可用", "Die Grenze für Sitzungswechsel ist nicht verfügbar", "Session 切り替え境界を利用できません", "Session 전환 경계를 사용할 수 없습니다", "Граница перехода session недоступна"),
		KeyREPLErrorActiveSessionIdentityMissing:  repl("The active session identity is unavailable", "当前 session 标识不可用", "Die Identität der aktiven Sitzung ist nicht verfügbar", "アクティブな session の識別情報を利用できません", "활성 session ID를 사용할 수 없습니다", "Идентификатор активной session недоступен"),
		KeyREPLErrorSwitchWhileQueryRunning:       repl("Sessions cannot be switched while a query is running", "查询运行期间无法切换 session", "Während einer laufenden Abfrage kann die Sitzung nicht gewechselt werden", "クエリ実行中は session を切り替えられません", "쿼리 실행 중에는 session을 전환할 수 없습니다", "Нельзя переключить session во время выполнения запроса"),
		KeyREPLErrorClearWhileQueryRunning:        repl("The conversation cannot be cleared while a query is running", "查询运行期间无法清空对话", "Während einer laufenden Abfrage kann die Unterhaltung nicht geleert werden", "クエリ実行中は会話を消去できません", "쿼리 실행 중에는 대화를 지울 수 없습니다", "Нельзя очистить диалог во время выполнения запроса"),
		KeyREPLErrorSaveCurrentLifecycle:          repl("Could not save the current session lifecycle", "无法保存当前 session 生命周期", "Der Lebenszyklus der aktuellen Sitzung konnte nicht gespeichert werden", "現在の session ライフサイクルを保存できませんでした", "현재 session 수명 주기를 저장하지 못했습니다", "Не удалось сохранить жизненный цикл текущей session"),
		KeyREPLErrorRestorePermissionMode:         repl("Could not restore the session permission mode", "无法恢复 session 权限模式", "Der Berechtigungsmodus der Sitzung konnte nicht wiederhergestellt werden", "Session の権限モードを復元できませんでした", "Session 권한 모드를 복원하지 못했습니다", "Не удалось восстановить режим разрешений session"),
		KeyREPLErrorResetPermissionMode:           repl("Could not reset the permission mode", "无法重置权限模式", "Der Berechtigungsmodus konnte nicht zurückgesetzt werden", "権限モードをリセットできませんでした", "권한 모드를 재설정하지 못했습니다", "Не удалось сбросить режим разрешений"),
		KeyREPLErrorSessionTransitionFailed:       repl("Session transition failed", "Session 切换失败", "Sitzungswechsel fehlgeschlagen", "Session の切り替えに失敗しました", "Session 전환 실패", "Переход session завершился ошибкой"),
		KeyREPLErrorRollbackModeFailedClosed:      repl("Session rollback succeeded, but mode rollback failed: %s; failed closed in surviving %s mode", "Session 回滚成功，但模式回滚失败：%s；已在保留下来的 %s 模式下执行故障关闭", "Sitzungs-Rollback erfolgreich, aber Modus-Rollback fehlgeschlagen: %s; im verbliebenen Modus %s sicher geschlossen", "Session のロールバックは成功しましたが、モードのロールバックに失敗しました: %s。残存する %s モードでフェイルクローズしました", "Session 롤백은 성공했지만 모드 롤백에 실패했습니다: %s. 유지된 %s 모드에서 fail-closed 처리했습니다", "Откат session выполнен, но откат режима завершился ошибкой: %s; выполнено безопасное закрытие в оставшемся режиме %s"),
		KeyREPLErrorRollbackMode:                  repl("Mode rollback: %s", "模式回滚：%s", "Modus-Rollback: %s", "モードのロールバック: %s", "모드 롤백: %s", "Откат режима: %s"),
		KeyREPLErrorRollbackSessionTargetRetained: repl("Session rollback failed: %s; the target session was retained coherently in %s mode", "Session 回滚失败：%s；目标 session 已在 %s 模式下保持一致", "Sitzungs-Rollback fehlgeschlagen: %s; die Zielsitzung wurde konsistent im Modus %s beibehalten", "Session のロールバックに失敗しました: %s。対象 session は %s モードで整合性を保ったまま維持されました", "Session 롤백 실패: %s. 대상 session은 %s 모드에서 일관되게 유지되었습니다", "Откат session завершился ошибкой: %s; целевая session согласованно сохранена в режиме %s"),
		KeyREPLErrorRollbackSessionTargetPublish:  repl("Session rollback failed: %s; the target session was retained coherently; target presentation update: %s", "Session 回滚失败：%s；目标 session 已保持一致；目标展示更新：%s", "Sitzungs-Rollback fehlgeschlagen: %s; die Zielsitzung wurde konsistent beibehalten; Aktualisierung der Zieldarstellung: %s", "Session のロールバックに失敗しました: %s。対象 session は整合性を保ったまま維持されました。対象表示の更新: %s", "Session 롤백 실패: %s. 대상 session은 일관되게 유지되었습니다. 대상 화면 업데이트: %s", "Откат session завершился ошибкой: %s; целевая session согласованно сохранена; обновление представления цели: %s"),
		KeyREPLErrorPublishSessionStopped:         repl("Could not publish the session presentation because TUI stopped", "TUI 已停止，无法发布 session 展示", "Die Sitzungsdarstellung konnte nicht veröffentlicht werden, da die TUI beendet wurde", "TUI が停止したため session 表示を公開できませんでした", "TUI가 중지되어 session 화면을 게시하지 못했습니다", "Не удалось опубликовать представление session: TUI остановлен"),
		KeyREPLErrorPublishSession:                repl("Could not publish the session presentation", "无法发布 session 展示", "Die Sitzungsdarstellung konnte nicht veröffentlicht werden", "Session 表示を公開できませんでした", "Session 화면을 게시하지 못했습니다", "Не удалось опубликовать представление session"),
		KeyREPLErrorPublishEmptySessionStopped:    repl("Could not publish the empty session because TUI stopped", "TUI 已停止，无法发布空 session", "Die leere Sitzung konnte nicht veröffentlicht werden, da die TUI beendet wurde", "TUI が停止したため空の session を公開できませんでした", "TUI가 중지되어 빈 session을 게시하지 못했습니다", "Не удалось опубликовать пустую session: TUI остановлен"),
		KeyREPLErrorPublishEmptySessionDetails:    repl("Mode rollback: %s; coherent target commit: %s; target presentation update: %s", "模式回滚：%s；一致的目标提交：%s；目标展示更新：%s", "Modus-Rollback: %s; konsistenter Ziel-Commit: %s; Aktualisierung der Zieldarstellung: %s", "モードのロールバック: %s。整合した対象 commit: %s。対象表示の更新: %s", "모드 롤백: %s. 일관된 대상 commit: %s. 대상 화면 업데이트: %s", "Откат режима: %s; согласованный commit цели: %s; обновление представления цели: %s"),
		KeyREPLErrorActivateEmptySession:          repl("Could not activate the empty session", "无法激活空 session", "Die leere Sitzung konnte nicht aktiviert werden", "空の session を有効化できませんでした", "빈 session을 활성화하지 못했습니다", "Не удалось активировать пустую session"),
		KeyREPLErrorCreateEmptySession:            repl("Could not create the empty session", "无法创建空 session", "Die leere Sitzung konnte nicht erstellt werden", "空の session を作成できませんでした", "빈 session을 만들지 못했습니다", "Не удалось создать пустую session"),
		KeyREPLErrorSaveEmptySessionLifecycle:     repl("Could not save the empty session lifecycle", "无法保存空 session 的生命周期", "Der Lebenszyklus der leeren Sitzung konnte nicht gespeichert werden", "空の session のライフサイクルを保存できませんでした", "빈 session 수명 주기를 저장하지 못했습니다", "Не удалось сохранить жизненный цикл пустой session"),
		KeyREPLErrorEmptySessionResumeUnsupported: repl("The engine does not support two-phase empty-session resume", "当前 engine 不支持两阶段恢复空 session", "Die Engine unterstützt keine zweiphasige Wiederaufnahme einer leeren Sitzung", "この engine は空の session の 2 段階再開に対応していません", "현재 engine은 빈 session의 2단계 재개를 지원하지 않습니다", "Engine не поддерживает двухфазное возобновление пустой session"),
		KeyREPLErrorPrepareEmptySession:           repl("Could not prepare the empty session", "无法准备空 session", "Die leere Sitzung konnte nicht vorbereitet werden", "空の session を準備できませんでした", "빈 session을 준비하지 못했습니다", "Не удалось подготовить пустую session"),
		KeyREPLErrorCreateTUIApp:                  repl("Could not create the TUI app", "无法创建 TUI 应用", "Die TUI-App konnte nicht erstellt werden", "TUI アプリを作成できませんでした", "TUI 앱을 만들지 못했습니다", "Не удалось создать приложение TUI"),
		KeyREPLErrorLoadInitialTUISession:         repl("Could not load the initial TUI session", "无法加载初始 TUI session", "Die erste TUI-Sitzung konnte nicht geladen werden", "初期 TUI session を読み込めませんでした", "초기 TUI session을 불러오지 못했습니다", "Не удалось загрузить начальную TUI session"),
		KeyREPLErrorPrepareInitialTUISession:      repl("Could not prepare the initial TUI session", "无法准备初始 TUI session", "Die erste TUI-Sitzung konnte nicht vorbereitet werden", "初期 TUI session を準備できませんでした", "초기 TUI session을 준비하지 못했습니다", "Не удалось подготовить начальную TUI session"),
		KeyREPLErrorApplyInitialTUISession:        repl("Could not apply the initial TUI session", "无法应用初始 TUI session", "Die erste TUI-Sitzung konnte nicht angewendet werden", "初期 TUI session を適用できませんでした", "초기 TUI session을 적용하지 못했습니다", "Не удалось применить начальную TUI session"),
		KeyREPLErrorRestoreInitialTUIMode:         repl("Could not restore the initial TUI permission mode", "无法恢复初始 TUI 权限模式", "Der anfängliche TUI-Berechtigungsmodus konnte nicht wiederhergestellt werden", "初期 TUI 権限モードを復元できませんでした", "초기 TUI 권한 모드를 복원하지 못했습니다", "Не удалось восстановить начальный режим разрешений TUI"),
		KeyREPLErrorLoadLifecycleMetadata:         repl("Could not load the session lifecycle metadata", "无法加载 session 生命周期 metadata", "Die Metadaten zum Sitzungslebenszyklus konnten nicht geladen werden", "Session ライフサイクルの metadata を読み込めませんでした", "Session 수명 주기 metadata를 불러오지 못했습니다", "Не удалось загрузить metadata жизненного цикла session"),
		KeyREPLErrorLoadScreenReaderMetadata:      repl("Could not load the screen-reader lifecycle metadata", "无法加载屏幕阅读器生命周期 metadata", "Die Metadaten zum Screenreader-Lebenszyklus konnten nicht geladen werden", "スクリーンリーダーのライフサイクル metadata を読み込めませんでした", "스크린 리더 수명 주기 metadata를 불러오지 못했습니다", "Не удалось загрузить metadata жизненного цикла экранного диктора"),
		KeyREPLErrorRestoreScreenReaderMode:       repl("Could not restore the screen-reader permission mode", "无法恢复屏幕阅读器权限模式", "Der Screenreader-Berechtigungsmodus konnte nicht wiederhergestellt werden", "スクリーンリーダーの権限モードを復元できませんでした", "스크린 리더 권한 모드를 복원하지 못했습니다", "Не удалось восстановить режим разрешений экранного диктора"),
		KeyREPLErrorRecoverDurableEvidence:        repl("Could not recover durable TUI evidence", "无法恢复持久化的 TUI 证据", "Dauerhafte TUI-Nachweise konnten nicht wiederhergestellt werden", "永続化された TUI エビデンスを復元できませんでした", "영구 TUI 근거를 복구하지 못했습니다", "Не удалось восстановить сохранённые свидетельства TUI"),
		KeyREPLErrorLoadTranscriptForLifecycle:    repl("Could not load the transcript before saving the lifecycle", "保存生命周期前无法加载 transcript", "Das Transkript konnte vor dem Speichern des Lebenszyklus nicht geladen werden", "ライフサイクルの保存前に transcript を読み込めませんでした", "수명 주기 저장 전에 transcript를 불러오지 못했습니다", "Не удалось загрузить transcript перед сохранением жизненного цикла"),
		KeyREPLErrorCreateTranscriptForLifecycle:  repl("Could not create the transcript before saving the lifecycle", "保存生命周期前无法创建 transcript", "Das Transkript konnte vor dem Speichern des Lebenszyklus nicht erstellt werden", "ライフサイクルの保存前に transcript を作成できませんでした", "수명 주기 저장 전에 transcript를 만들지 못했습니다", "Не удалось создать transcript перед сохранением жизненного цикла"),
		KeyREPLErrorModeSwitchCommitting:          repl("The mode cannot be switched while a session transition is being committed; try again when it finishes", "Session 切换提交期间无法切换模式；请在完成后重试", "Der Modus kann während eines laufenden Sitzungswechsels nicht geändert werden; versuche es danach erneut", "Session 切り替えの commit 中はモードを変更できません。完了後に再試行してください", "Session 전환 commit 중에는 모드를 변경할 수 없습니다. 완료 후 다시 시도하세요", "Нельзя переключить режим во время commit перехода session; повторите после завершения"),
		KeyREPLErrorTUIStateRequired:              repl("TUI state is required", "需要 TUI state", "Ein TUI-Status ist erforderlich", "TUI state が必要です", "TUI state가 필요합니다", "Требуется состояние TUI"),
		KeyREPLErrorExitPlanMode:                  repl("Could not exit Plan mode", "无法退出 Plan 模式", "Der Plan-Modus konnte nicht beendet werden", "Plan モードを終了できませんでした", "Plan 모드를 종료하지 못했습니다", "Не удалось выйти из режима Plan"),
		KeyREPLErrorPersistPlanMode:               repl("Could not persist Plan mode", "无法持久化 Plan 模式", "Der Plan-Modus konnte nicht gespeichert werden", "Plan モードを永続化できませんでした", "Plan 모드를 저장하지 못했습니다", "Не удалось сохранить режим Plan"),
		KeyREPLErrorSwitchMode:                    repl("Could not switch to %s mode", "无法切换到 %s 模式", "Es konnte nicht in den Modus %s gewechselt werden", "%s モードに切り替えられませんでした", "%s 모드로 전환하지 못했습니다", "Не удалось переключиться в режим %s"),
		KeyREPLErrorPersistRestoredPlanMode:       repl("Could not persist the restored Plan mode", "无法持久化已恢复的 Plan 模式", "Der wiederhergestellte Plan-Modus konnte nicht gespeichert werden", "復元した Plan モードを永続化できませんでした", "복원된 Plan 모드를 저장하지 못했습니다", "Не удалось сохранить восстановленный режим Plan"),
		KeyREPLErrorDecisionSessionMissing:        repl("Decision %q has no session identity", "Decision %q 没有 session 标识", "Entscheidung %q hat keine Sitzungsidentität", "Decision %q に session 識別情報がありません", "Decision %q에 session ID가 없습니다", "У decision %q нет идентификатора session"),
		KeyREPLErrorDecisionWrongSession:          repl("Decision %q belongs to session %q, but the active session is %q", "Decision %q 属于 session %q，但当前 session 是 %q", "Entscheidung %q gehört zur Sitzung %q, aktiv ist jedoch %q", "Decision %q は session %q に属していますが、アクティブな session は %q です", "Decision %q은(는) session %q에 속하지만 활성 session은 %q입니다", "Decision %q относится к session %q, но активна session %q"),
		KeyREPLErrorDecisionSessionEmpty:          repl("Decision session identity is empty", "Decision 的 session 标识为空", "Die Sitzungsidentität der Entscheidung ist leer", "Decision の session 識別情報が空です", "Decision session ID가 비어 있습니다", "Идентификатор session у decision пуст"),
	} {
		semanticTranslations[key] = translations
	}
}
