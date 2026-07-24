package i18n

const (
	KeyEngineSessionSaveFailed       Key = "engine.session.save_failed"
	KeyEngineSessionLoadFailed       Key = "engine.session.load_failed"
	KeyEngineSessionAmbiguous        Key = "engine.session.ambiguous"
	KeyEngineSessionResumeConflict   Key = "engine.session.resume_conflict"
	KeyEngineSessionResumeCompleted  Key = "engine.session.resume_completed"
	KeyEngineSessionSkillStateFailed Key = "engine.session.skill_state_failed"
	KeyEngineSessionCompactFailed    Key = "engine.session.compact_failed"
	KeyEngineSessionDeleteFailed     Key = "engine.session.delete_failed"
	KeyEngineSessionIDRequired       Key = "engine.session.id_required"

	KeyHookHTTPURLValidationFailed Key = "hook.http.url_validation_failed"
	KeyHookHTTPInputMarshalFailed  Key = "hook.http.input_marshal_failed"
	KeyHookNotificationDefault     Key = "hook.notification.default_message"
	KeyHookNotificationFailed      Key = "hook.notification.failed"

	KeyCoordinatorAgentPanicked        Key = "coordinator.agent.panicked"
	KeyCoordinatorAgentNotSubscribed   Key = "coordinator.agent.not_subscribed"
	KeyCoordinatorAgentChannelFull     Key = "coordinator.agent.channel_full"
	KeyCoordinatorDependencyFailedSkip Key = "coordinator.dependency.failed_skip"
)

var engineHookCoordinatorKeys = [...]Key{
	KeyEngineSessionSaveFailed,
	KeyEngineSessionLoadFailed,
	KeyEngineSessionAmbiguous,
	KeyEngineSessionResumeConflict,
	KeyEngineSessionResumeCompleted,
	KeyEngineSessionSkillStateFailed,
	KeyEngineSessionCompactFailed,
	KeyEngineSessionDeleteFailed,
	KeyEngineSessionIDRequired,
	KeyHookHTTPURLValidationFailed,
	KeyHookHTTPInputMarshalFailed,
	KeyHookNotificationDefault,
	KeyHookNotificationFailed,
	KeyCoordinatorAgentPanicked,
	KeyCoordinatorAgentNotSubscribed,
	KeyCoordinatorAgentChannelFull,
	KeyCoordinatorDependencyFailedSkip,
}

func init() {
	addEngineHookCoordinator := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en,
			LangZH: zh,
			LangDE: de,
			LangJA: ja,
			LangKO: ko,
			LangRU: ru,
		}
	}

	addEngineHookCoordinator(KeyEngineSessionSaveFailed,
		"Could not save the session.",
		"无法保存会话。",
		"Die Sitzung konnte nicht gespeichert werden.",
		"セッションを保存できませんでした。",
		"세션을 저장할 수 없습니다.",
		"Не удалось сохранить сеанс.")
	addEngineHookCoordinator(KeyEngineSessionLoadFailed,
		"Could not load the session.",
		"无法加载会话。",
		"Die Sitzung konnte nicht geladen werden.",
		"セッションを読み込めませんでした。",
		"세션을 불러올 수 없습니다.",
		"Не удалось загрузить сеанс.")
	addEngineHookCoordinator(KeyEngineSessionAmbiguous,
		"Session ID %s exists in more than one project. Select the session from its project.",
		"会话 ID %s 存在于多个项目中。请从对应项目中选择该会话。",
		"Die Sitzungs-ID %s existiert in mehreren Projekten. Wähle die Sitzung in ihrem Projekt aus.",
		"セッション ID %s は複数のプロジェクトに存在します。対象のプロジェクトからセッションを選択してください。",
		"세션 ID %s이(가) 여러 프로젝트에 있습니다. 해당 프로젝트에서 세션을 선택하세요.",
		"ID сеанса %s встречается в нескольких проектах. Выберите сеанс в нужном проекте.")
	addEngineHookCoordinator(KeyEngineSessionResumeConflict,
		"Session %s changed or is already active, so it could not be resumed.",
		"会话 %s 已发生变化或正在使用，无法恢复。",
		"Die Sitzung %s wurde geändert oder ist bereits aktiv und konnte daher nicht fortgesetzt werden.",
		"セッション %s は変更されたか、すでに使用中のため再開できませんでした。",
		"세션 %s이(가) 변경되었거나 이미 활성 상태여서 재개할 수 없습니다.",
		"Сеанс %s изменился или уже активен, поэтому его не удалось возобновить.")
	addEngineHookCoordinator(KeyEngineSessionResumeCompleted,
		"This prepared session resume has already been completed.",
		"此次会话恢复准备已完成。",
		"Diese vorbereitete Sitzungsfortsetzung wurde bereits abgeschlossen.",
		"準備済みのセッション再開処理はすでに完了しています。",
		"준비된 세션 재개 작업이 이미 완료되었습니다.",
		"Подготовленное возобновление сеанса уже завершено.")
	addEngineHookCoordinator(KeyEngineSessionSkillStateFailed,
		"Could not restore this session's skill state.",
		"无法恢复此会话的 skill 状态。",
		"Der Skill-Status dieser Sitzung konnte nicht wiederhergestellt werden.",
		"このセッションの skill 状態を復元できませんでした。",
		"이 세션의 skill 상태를 복원할 수 없습니다.",
		"Не удалось восстановить состояние skill для этого сеанса.")
	addEngineHookCoordinator(KeyEngineSessionCompactFailed,
		"Could not compact session %s: %v",
		"无法压缩会话 %s：%v",
		"Die Sitzung %s konnte nicht komprimiert werden: %v",
		"セッション %s を圧縮できませんでした: %v",
		"세션 %s을(를) 압축할 수 없습니다: %v",
		"Не удалось сжать сеанс %s: %v")
	addEngineHookCoordinator(KeyEngineSessionDeleteFailed,
		"Could not delete the session history.",
		"无法删除会话历史记录。",
		"Der Sitzungsverlauf konnte nicht gelöscht werden.",
		"セッション履歴を削除できませんでした。",
		"세션 기록을 삭제할 수 없습니다.",
		"Не удалось удалить историю сеанса.")
	addEngineHookCoordinator(KeyEngineSessionIDRequired,
		"A session ID is required.",
		"必须提供会话 ID。",
		"Eine Sitzungs-ID ist erforderlich.",
		"セッション ID が必要です。",
		"세션 ID가 필요합니다.",
		"Необходимо указать ID сеанса.")

	addEngineHookCoordinator(KeyHookHTTPURLValidationFailed,
		"Hook URL validation failed: %v",
		"hook URL 校验失败：%v",
		"Die Prüfung der Hook-URL ist fehlgeschlagen: %v",
		"hook URL の検証に失敗しました: %v",
		"hook URL 검증에 실패했습니다: %v",
		"Не удалось проверить URL hook: %v")
	addEngineHookCoordinator(KeyHookHTTPInputMarshalFailed,
		"Could not encode the HTTP hook input.",
		"无法编码 HTTP hook 输入。",
		"Die Eingabe für den HTTP-Hook konnte nicht codiert werden.",
		"HTTP hook の入力をエンコードできませんでした。",
		"HTTP hook 입력을 인코딩할 수 없습니다.",
		"Не удалось закодировать входные данные HTTP hook.")
	addEngineHookCoordinator(KeyHookNotificationDefault,
		"Hook event: %s",
		"Hook 事件：%s",
		"Hook-Ereignis: %s",
		"Hook イベント: %s",
		"Hook 이벤트: %s",
		"Событие Hook: %s")
	addEngineHookCoordinator(KeyHookNotificationFailed,
		"Notification hook failed: %v",
		"通知 Hook 失败：%v",
		"Der Benachrichtigungs-Hook ist fehlgeschlagen: %v",
		"通知 Hook に失敗しました: %v",
		"알림 Hook이 실패했습니다: %v",
		"Hook уведомления завершился ошибкой: %v")

	addEngineHookCoordinator(KeyCoordinatorAgentPanicked,
		"agent %s panicked: %v",
		"Agent %s 发生 panic：%v",
		"Agent %s ist mit einem Panic abgebrochen: %v",
		"Agent %s で panic が発生しました: %v",
		"Agent %s에서 panic이 발생했습니다: %v",
		"В Agent %s произошёл panic: %v")
	addEngineHookCoordinator(KeyCoordinatorAgentNotSubscribed,
		"Agent %s is not subscribed.",
		"Agent %s 尚未订阅。",
		"Agent %s ist nicht registriert.",
		"Agent %s は購読していません。",
		"Agent %s이(가) 구독되어 있지 않습니다.",
		"Agent %s не подписан.")
	addEngineHookCoordinator(KeyCoordinatorAgentChannelFull,
		"Agent %s cannot receive more messages right now.",
		"Agent %s 暂时无法接收更多消息。",
		"Agent %s kann derzeit keine weiteren Nachrichten empfangen.",
		"Agent %s は現在これ以上メッセージを受信できません。",
		"Agent %s은(는) 현재 더 이상 메시지를 받을 수 없습니다.",
		"Agent %s сейчас не может принять новые сообщения.")
	addEngineHookCoordinator(KeyCoordinatorDependencyFailedSkip,
		"Skipped because dependency %s failed.",
		"已跳过，因为依赖项 %s 失败。",
		"Übersprungen, weil die Abhängigkeit %s fehlgeschlagen ist.",
		"依存関係 %s が失敗したためスキップしました。",
		"종속 항목 %s이(가) 실패하여 건너뛰었습니다.",
		"Пропущено из-за ошибки зависимости %s.")
}
