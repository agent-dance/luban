package i18n

// Semantic runtime errors produced while rebuilding a persisted TUI session
// or navigating its transcript. Message/block indexes, session and observation
// IDs, and raw underlying errors remain formatting parameters.
const (
	KeyTUISessionProjectionRetainToolResult           Key = "tui.session_projection.retain_tool_result"
	KeyTUISessionProjectionEncodeToolResult           Key = "tui.session_projection.encode_tool_result"
	KeyTUISessionProjectionRetainStructuredToolResult Key = "tui.session_projection.retain_structured_tool_result"
	KeyTUISessionProjectionEncodeBlock                Key = "tui.session_projection.encode_block"
	KeyTUISessionProjectionRetainBlock                Key = "tui.session_projection.retain_block"

	KeyTUISessionSnapshotEmptySessionID       Key = "tui.session_snapshot.empty_session_id"
	KeyTUISessionSnapshotObservationEmptyID   Key = "tui.session_snapshot.observation_empty_id"
	KeyTUISessionSnapshotDuplicateObservation Key = "tui.session_snapshot.duplicate_observation"
	KeyTUISessionSnapshotRestoreActivities    Key = "tui.session_snapshot.restore_activities"

	KeyTUITranscriptSearchNotPrepared    Key = "tui.transcript_search.not_prepared"
	KeyTUITranscriptSearchSessionChanged Key = "tui.transcript_search.session_changed"
	KeyTUITranscriptSearchNotOpen        Key = "tui.transcript_search.not_open"

	KeyTUIActivityNotFound         Key = "tui.activity.not_found"
	KeyTUIStateObservationNotFound Key = "tui.state.observation_not_found"
	KeyTUIActivityStoreUnavailable Key = "tui.activity.store_unavailable"

	KeyTUISessionViewInvalidCheckpoint   Key = "tui.session_view.invalid_checkpoint"
	KeyTUISessionViewMissingCheckpoint   Key = "tui.session_view.missing_checkpoint"
	KeyTUISessionViewUnsupportedVersion  Key = "tui.session_view.unsupported_version"
	KeyTUISessionViewIdentityMismatch    Key = "tui.session_view.identity_mismatch"
	KeyTUISessionViewUnstableCheckpoint  Key = "tui.session_view.unstable_checkpoint"
	KeyTUISessionViewStaleCapture        Key = "tui.session_view.stale_capture"
	KeyTUISessionViewMaterializeEvidence Key = "tui.session_view.materialize_evidence"
	KeyTUISessionViewValidateEvidence    Key = "tui.session_view.validate_evidence"
)

var tuiSessionStateErrorKeys = []Key{
	KeyTUISessionProjectionRetainToolResult,
	KeyTUISessionProjectionEncodeToolResult,
	KeyTUISessionProjectionRetainStructuredToolResult,
	KeyTUISessionProjectionEncodeBlock,
	KeyTUISessionProjectionRetainBlock,
	KeyTUISessionSnapshotEmptySessionID,
	KeyTUISessionSnapshotObservationEmptyID,
	KeyTUISessionSnapshotDuplicateObservation,
	KeyTUISessionSnapshotRestoreActivities,
	KeyTUITranscriptSearchNotPrepared,
	KeyTUITranscriptSearchSessionChanged,
	KeyTUITranscriptSearchNotOpen,
	KeyTUIActivityNotFound,
	KeyTUIStateObservationNotFound,
	KeyTUIActivityStoreUnavailable,
	KeyTUISessionViewInvalidCheckpoint,
	KeyTUISessionViewMissingCheckpoint,
	KeyTUISessionViewUnsupportedVersion,
	KeyTUISessionViewIdentityMismatch,
	KeyTUISessionViewUnstableCheckpoint,
	KeyTUISessionViewStaleCapture,
	KeyTUISessionViewMaterializeEvidence,
	KeyTUISessionViewValidateEvidence,
}

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en,
			LangZH: zh,
			LangDE: de,
			LangJA: ja,
			LangKO: ko,
			LangRU: ru,
		}
	}

	add(KeyTUISessionProjectionRetainToolResult,
		"retain persisted tool result at message %d block %d: %v",
		"无法保留消息 %d 中块 %d 的持久化工具结果：%v",
		"Persistiertes Tool-Ergebnis in Nachricht %d, Block %d konnte nicht gespeichert werden: %v",
		"メッセージ %d、ブロック %d の永続化されたツール結果を保持できませんでした: %v",
		"메시지 %d, 블록 %d의 저장된 도구 결과를 보관할 수 없습니다: %v",
		"Не удалось сохранить результат инструмента из сообщения %d, блока %d: %v")
	add(KeyTUISessionProjectionEncodeToolResult,
		"encode persisted tool result at message %d block %d: %v",
		"无法编码消息 %d 中块 %d 的持久化工具结果：%v",
		"Persistiertes Tool-Ergebnis in Nachricht %d, Block %d konnte nicht kodiert werden: %v",
		"メッセージ %d、ブロック %d の永続化されたツール結果をエンコードできませんでした: %v",
		"메시지 %d, 블록 %d의 저장된 도구 결과를 인코딩할 수 없습니다: %v",
		"Не удалось закодировать результат инструмента из сообщения %d, блока %d: %v")
	add(KeyTUISessionProjectionRetainStructuredToolResult,
		"retain persisted structured tool result at message %d block %d: %v",
		"无法保留消息 %d 中块 %d 的持久化结构化工具结果：%v",
		"Persistiertes strukturiertes Tool-Ergebnis in Nachricht %d, Block %d konnte nicht gespeichert werden: %v",
		"メッセージ %d、ブロック %d の永続化された構造化ツール結果を保持できませんでした: %v",
		"메시지 %d, 블록 %d의 저장된 구조화 도구 결과를 보관할 수 없습니다: %v",
		"Не удалось сохранить структурированный результат инструмента из сообщения %d, блока %d: %v")
	add(KeyTUISessionProjectionEncodeBlock,
		"encode persisted block at message %d block %d: %v",
		"无法编码消息 %d 中的持久化块 %d：%v",
		"Persistierter Block in Nachricht %d, Block %d konnte nicht kodiert werden: %v",
		"メッセージ %d の永続化されたブロック %d をエンコードできませんでした: %v",
		"메시지 %d의 저장된 블록 %d을(를) 인코딩할 수 없습니다: %v",
		"Не удалось закодировать сохранённый блок из сообщения %d, блока %d: %v")
	add(KeyTUISessionProjectionRetainBlock,
		"retain persisted block at message %d block %d: %v",
		"无法保留消息 %d 中的持久化块 %d：%v",
		"Persistierter Block in Nachricht %d, Block %d konnte nicht gespeichert werden: %v",
		"メッセージ %d の永続化されたブロック %d を保持できませんでした: %v",
		"메시지 %d의 저장된 블록 %d을(를) 보관할 수 없습니다: %v",
		"Не удалось сохранить блок из сообщения %d, блока %d: %v")

	add(KeyTUISessionSnapshotEmptySessionID,
		"session snapshot has empty session ID",
		"session 快照中的 session ID 为空",
		"Der Session-Snapshot enthält keine Session-ID",
		"session スナップショットの session ID が空です",
		"session 스냅샷의 session ID가 비어 있습니다",
		"В снимке session отсутствует session ID")
	add(KeyTUISessionSnapshotObservationEmptyID,
		"session projection contains observation with empty ID",
		"session 投影中存在 ID 为空的 observation",
		"Die Session-Projektion enthält eine Beobachtung ohne ID",
		"session プロジェクションに ID が空の observation があります",
		"session 프로젝션에 ID가 비어 있는 observation이 있습니다",
		"Проекция session содержит observation без ID")
	add(KeyTUISessionSnapshotDuplicateObservation,
		"session projection contains duplicate observation %q",
		"session 投影中存在重复的 observation %q",
		"Die Session-Projektion enthält die Beobachtung %q mehrfach",
		"session プロジェクションに重複する observation %q があります",
		"session 프로젝션에 중복 observation %q이(가) 있습니다",
		"Проекция session содержит повторяющийся observation %q")
	add(KeyTUISessionSnapshotRestoreActivities,
		"restore session activities: %v",
		"恢复 session 活动失败：%v",
		"Session-Aktivitäten konnten nicht wiederhergestellt werden: %v",
		"session のアクティビティを復元できませんでした: %v",
		"session 활동을 복원할 수 없습니다: %v",
		"Не удалось восстановить действия session: %v")

	add(KeyTUITranscriptSearchNotPrepared,
		"transcript search is not prepared",
		"尚未准备好对话记录搜索",
		"Die Transkriptsuche ist nicht vorbereitet",
		"会話履歴検索の準備ができていません",
		"대화 기록 검색이 준비되지 않았습니다",
		"Поиск по стенограмме не подготовлен")
	add(KeyTUITranscriptSearchSessionChanged,
		"session changed while searching transcript",
		"搜索对话记录期间 session 已切换",
		"Während der Transkriptsuche wurde die Session gewechselt",
		"会話履歴の検索中に session が切り替わりました",
		"대화 기록을 검색하는 동안 session이 변경되었습니다",
		"Во время поиска по стенограмме session был изменён")
	add(KeyTUITranscriptSearchNotOpen,
		"transcript search is not open",
		"尚未打开对话记录搜索",
		"Die Transkriptsuche ist nicht geöffnet",
		"会話履歴検索が開かれていません",
		"대화 기록 검색이 열려 있지 않습니다",
		"Поиск по стенограмме не открыт")

	add(KeyTUIActivityNotFound,
		"activity %q not found",
		"找不到活动 %q",
		"Aktivität %q wurde nicht gefunden",
		"アクティビティ %q が見つかりません",
		"활동 %q을(를) 찾을 수 없습니다",
		"Действие %q не найдено")
	add(KeyTUIStateObservationNotFound,
		"observation %q not found",
		"找不到 observation %q",
		"Beobachtung %q wurde nicht gefunden",
		"observation %q が見つかりません",
		"observation %q을(를) 찾을 수 없습니다",
		"Observation %q не найден")
	add(KeyTUIActivityStoreUnavailable,
		"activity store is not initialized",
		"活动存储尚未初始化",
		"Der Aktivitätsspeicher ist nicht initialisiert",
		"アクティビティストアが初期化されていません",
		"활동 저장소가 초기화되지 않았습니다",
		"Хранилище действий не инициализировано")

	add(KeyTUISessionViewInvalidCheckpoint,
		"the session view checkpoint failed integrity validation",
		"session 视图检查点未通过完整性校验",
		"Der Session-Ansichts-Checkpoint hat die Integritätsprüfung nicht bestanden",
		"session ビュー・チェックポイントの整合性検証に失敗しました",
		"session 보기 체크포인트가 무결성 검증을 통과하지 못했습니다",
		"Контрольная точка представления session не прошла проверку целостности")
	add(KeyTUISessionViewMissingCheckpoint,
		"the session view checkpoint for the durable transcript is missing",
		"缺少与持久化对话记录对应的 session 视图检查点",
		"Der Session-Ansichts-Checkpoint für das gespeicherte Transkript fehlt",
		"永続化された会話履歴に対応する session ビュー・チェックポイントがありません",
		"저장된 대화 기록에 해당하는 session 보기 체크포인트가 없습니다",
		"Отсутствует контрольная точка представления session для сохранённой стенограммы")
	add(KeyTUISessionViewUnsupportedVersion,
		"session view checkpoint version %d is not supported; expected version %d",
		"不支持 session 视图检查点版本 %d；需要版本 %d",
		"Version %d des Session-Ansichts-Checkpoints wird nicht unterstützt; erwartet wird Version %d",
		"session ビュー・チェックポイントのバージョン %d はサポートされていません。必要なバージョンは %d です",
		"session 보기 체크포인트 버전 %d은(는) 지원되지 않습니다. 필요한 버전은 %d입니다",
		"Версия %d контрольной точки представления session не поддерживается; требуется версия %d")
	add(KeyTUISessionViewIdentityMismatch,
		"the session view checkpoint belongs to a different session",
		"session 视图检查点属于其他 session",
		"Der Session-Ansichts-Checkpoint gehört zu einer anderen Session",
		"session ビュー・チェックポイントは別の session に属しています",
		"session 보기 체크포인트가 다른 session에 속합니다",
		"Контрольная точка представления session принадлежит другой session")
	add(KeyTUISessionViewUnstableCheckpoint,
		"the session view kept changing while its checkpoint was being created",
		"创建检查点时 session 视图持续变化",
		"Die Session-Ansicht änderte sich während der Erstellung ihres Checkpoints fortlaufend",
		"チェックポイントの作成中に session ビューが変化し続けました",
		"체크포인트를 만드는 동안 session 보기가 계속 변경되었습니다",
		"Представление session продолжало изменяться во время создания контрольной точки")
	add(KeyTUISessionViewStaleCapture,
		"a stale session view capture cannot overwrite a newer checkpoint",
		"过期的 session 视图快照不能覆盖更新的检查点",
		"Eine veraltete Erfassung der Session-Ansicht darf keinen neueren Checkpoint überschreiben",
		"古い session ビューのキャプチャで新しいチェックポイントを上書きすることはできません",
		"오래된 session 보기 캡처는 더 최신 체크포인트를 덮어쓸 수 없습니다",
		"Устаревший снимок представления session не может перезаписать более новую контрольную точку")
	add(KeyTUISessionViewMaterializeEvidence,
		"retain session view evidence in its checkpoint: %v",
		"在检查点中保留 session 视图证据失败：%v",
		"Belege der Session-Ansicht konnten nicht im Checkpoint gespeichert werden: %v",
		"session ビューの証拠をチェックポイントに保持できませんでした: %v",
		"session 보기 증거를 체크포인트에 보관할 수 없습니다: %v",
		"Не удалось сохранить доказательства представления session в контрольной точке: %v")
	add(KeyTUISessionViewValidateEvidence,
		"validate session view checkpoint evidence: %v",
		"校验 session 视图检查点证据失败：%v",
		"Belege des Session-Ansichts-Checkpoints konnten nicht validiert werden: %v",
		"session ビュー・チェックポイントの証拠を検証できませんでした: %v",
		"session 보기 체크포인트 증거를 검증할 수 없습니다: %v",
		"Не удалось проверить доказательства контрольной точки представления session: %v")
}
