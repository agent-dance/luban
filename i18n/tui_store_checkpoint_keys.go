package i18n

// Semantic errors from TUI evidence indexing, activity restoration, durable
// view checkpoints, and the shared skill invocation boundary. Stable activity
// states and raw filesystem/JSON causes remain formatting parameters.
const (
	KeyTUIObservationRetainEvidenceIndex     Key = "tui.observation.retain_evidence_index"
	KeyTUIActivityStateLifecycleIncompatible Key = "tui.activity.state_lifecycle_incompatible"
	KeyTUIActivityStateOutcomeIncompatible   Key = "tui.activity.state_outcome_incompatible"
	KeyTUIActivityRunAttemptInvalid          Key = "tui.activity.run_attempt_invalid"
	KeyTUISessionViewMarshalTranscript       Key = "tui.session_view.marshal_transcript"
	KeyTUISessionViewMarshalCheckpoint       Key = "tui.session_view.marshal_checkpoint"
	KeyTUISessionViewPrepareCheckpointDir    Key = "tui.session_view.prepare_checkpoint_directory"
	KeyTUISessionViewCreateCheckpoint        Key = "tui.session_view.create_checkpoint"
	KeyTUISessionViewSecureCheckpoint        Key = "tui.session_view.secure_checkpoint"
	KeyTUISessionViewWriteCheckpoint         Key = "tui.session_view.write_checkpoint"
	KeyTUISessionViewSyncCheckpoint          Key = "tui.session_view.sync_checkpoint"
	KeyTUISessionViewCloseCheckpoint         Key = "tui.session_view.close_checkpoint"
	KeyTUISessionViewPublishCheckpoint       Key = "tui.session_view.publish_checkpoint"
	KeyTUISessionViewOpenCheckpoint          Key = "tui.session_view.open_checkpoint"
	KeyTUISessionViewDecodeCheckpointFile    Key = "tui.session_view.decode_checkpoint_file"
	KeyTUISessionViewTrailingCheckpointData  Key = "tui.session_view.trailing_checkpoint_data"
)

var tuiStoreCheckpointKeys = [...]Key{
	KeyTUIObservationRetainEvidenceIndex,
	KeyTUIActivityStateLifecycleIncompatible,
	KeyTUIActivityStateOutcomeIncompatible,
	KeyTUIActivityRunAttemptInvalid,
	KeyTUISessionViewMarshalTranscript,
	KeyTUISessionViewMarshalCheckpoint,
	KeyTUISessionViewPrepareCheckpointDir,
	KeyTUISessionViewCreateCheckpoint,
	KeyTUISessionViewSecureCheckpoint,
	KeyTUISessionViewWriteCheckpoint,
	KeyTUISessionViewSyncCheckpoint,
	KeyTUISessionViewCloseCheckpoint,
	KeyTUISessionViewPublishCheckpoint,
	KeyTUISessionViewOpenCheckpoint,
	KeyTUISessionViewDecodeCheckpointFile,
	KeyTUISessionViewTrailingCheckpointData,
}

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de,
			LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}

	add(KeyTUIObservationRetainEvidenceIndex,
		"retain observation evidence index: %v",
		"保留观察证据索引失败：%v",
		"Der Index der Beobachtungsbelege konnte nicht gespeichert werden: %v",
		"観測の証拠インデックスを保持できませんでした：%v",
		"관찰 증거 인덱스를 보관하지 못했습니다: %v",
		"Не удалось сохранить индекс доказательств наблюдения: %v")
	add(KeyTUIActivityStateLifecycleIncompatible,
		"activity state/outcome mismatch: state %q is incompatible with lifecycle %s",
		"活动状态/结果不匹配：状态 %q 与生命周期 %s 不兼容",
		"Status und Ergebnis der Aktivität stimmen nicht überein: Status %q ist mit Lebenszyklus %s nicht kompatibel",
		"アクティビティの状態/結果が一致しません：状態 %q はライフサイクル %s と互換性がありません",
		"활동 상태/결과가 일치하지 않습니다: 상태 %q은(는) 수명 주기 %s와 호환되지 않습니다",
		"Состояние и результат действия не согласованы: состояние %q несовместимо с жизненным циклом %s")
	add(KeyTUIActivityStateOutcomeIncompatible,
		"activity state/outcome mismatch: state %q is incompatible with outcome %s",
		"活动状态/结果不匹配：状态 %q 与结果 %s 不兼容",
		"Status und Ergebnis der Aktivität stimmen nicht überein: Status %q ist mit Ergebnis %s nicht kompatibel",
		"アクティビティの状態/結果が一致しません：状態 %q は結果 %s と互換性がありません",
		"활동 상태/결과가 일치하지 않습니다: 상태 %q은(는) 결과 %s와 호환되지 않습니다",
		"Состояние и результат действия не согласованы: состояние %q несовместимо с результатом %s")
	add(KeyTUIActivityRunAttemptInvalid,
		"invalid activity attempt for run %q: got %d; attempt must be positive",
		"运行 %q 的活动尝试次数无效：当前为 %d；尝试次数必须为正数",
		"Ungültiger Aktivitätsversuch für Lauf %q: %d erhalten; der Versuch muss positiv sein",
		"実行 %q のアクティビティ試行回数が無効です：%d が指定されました。試行回数は正の値である必要があります",
		"실행 %q의 활동 시도 횟수가 잘못되었습니다: %d이(가) 지정되었습니다. 시도 횟수는 양수여야 합니다",
		"Недопустимый номер попытки действия для запуска %q: получено %d; номер попытки должен быть положительным")
	add(KeyTUISessionViewMarshalTranscript,
		"marshal session view transcript: %v",
		"序列化 session 视图的对话记录失败：%v",
		"Das Transkript der Sessionansicht konnte nicht serialisiert werden: %v",
		"session ビューの会話履歴をシリアライズできませんでした：%v",
		"session 보기의 대화 기록을 직렬화하지 못했습니다: %v",
		"Не удалось сериализовать стенограмму представления session: %v")
	add(KeyTUISessionViewMarshalCheckpoint,
		"marshal session view checkpoint: %v",
		"序列化 session 视图检查点失败：%v",
		"Der Prüfpunkt der Sessionansicht konnte nicht serialisiert werden: %v",
		"session ビューのチェックポイントをシリアライズできませんでした：%v",
		"session 보기 체크포인트를 직렬화하지 못했습니다: %v",
		"Не удалось сериализовать контрольную точку представления session: %v")
	add(KeyTUISessionViewPrepareCheckpointDir,
		"prepare session view checkpoint directory: %v",
		"准备 session 视图检查点目录失败：%v",
		"Das Verzeichnis für Prüfpunkte der Sessionansicht konnte nicht vorbereitet werden: %v",
		"session ビューのチェックポイント用ディレクトリを準備できませんでした：%v",
		"session 보기 체크포인트 디렉터리를 준비하지 못했습니다: %v",
		"Не удалось подготовить каталог контрольных точек представления session: %v")
	add(KeyTUISessionViewCreateCheckpoint,
		"create session view checkpoint: %v",
		"创建 session 视图检查点失败：%v",
		"Der Prüfpunkt der Sessionansicht konnte nicht erstellt werden: %v",
		"session ビューのチェックポイントを作成できませんでした：%v",
		"session 보기 체크포인트를 만들지 못했습니다: %v",
		"Не удалось создать контрольную точку представления session: %v")
	add(KeyTUISessionViewSecureCheckpoint,
		"secure session view checkpoint: %v",
		"保护 session 视图检查点失败：%v",
		"Der Prüfpunkt der Sessionansicht konnte nicht abgesichert werden: %v",
		"session ビューのチェックポイントを保護できませんでした：%v",
		"session 보기 체크포인트를 보호하지 못했습니다: %v",
		"Не удалось защитить контрольную точку представления session: %v")
	add(KeyTUISessionViewWriteCheckpoint,
		"write session view checkpoint: %v",
		"写入 session 视图检查点失败：%v",
		"Der Prüfpunkt der Sessionansicht konnte nicht geschrieben werden: %v",
		"session ビューのチェックポイントを書き込めませんでした：%v",
		"session 보기 체크포인트를 쓰지 못했습니다: %v",
		"Не удалось записать контрольную точку представления session: %v")
	add(KeyTUISessionViewSyncCheckpoint,
		"sync session view checkpoint: %v",
		"同步 session 视图检查点失败：%v",
		"Der Prüfpunkt der Sessionansicht konnte nicht synchronisiert werden: %v",
		"session ビューのチェックポイントを同期できませんでした：%v",
		"session 보기 체크포인트를 동기화하지 못했습니다: %v",
		"Не удалось синхронизировать контрольную точку представления session: %v")
	add(KeyTUISessionViewCloseCheckpoint,
		"close session view checkpoint: %v",
		"关闭 session 视图检查点失败：%v",
		"Der Prüfpunkt der Sessionansicht konnte nicht geschlossen werden: %v",
		"session ビューのチェックポイントを閉じられませんでした：%v",
		"session 보기 체크포인트를 닫지 못했습니다: %v",
		"Не удалось закрыть контрольную точку представления session: %v")
	add(KeyTUISessionViewPublishCheckpoint,
		"publish session view checkpoint: %v",
		"发布 session 视图检查点失败：%v",
		"Der Prüfpunkt der Sessionansicht konnte nicht veröffentlicht werden: %v",
		"session ビューのチェックポイントを公開できませんでした：%v",
		"session 보기 체크포인트를 게시하지 못했습니다: %v",
		"Не удалось опубликовать контрольную точку представления session: %v")
	add(KeyTUISessionViewOpenCheckpoint,
		"open session view checkpoint: %v",
		"打开 session 视图检查点失败：%v",
		"Der Prüfpunkt der Sessionansicht konnte nicht geöffnet werden: %v",
		"session ビューのチェックポイントを開けませんでした：%v",
		"session 보기 체크포인트를 열지 못했습니다: %v",
		"Не удалось открыть контрольную точку представления session: %v")
	add(KeyTUISessionViewDecodeCheckpointFile,
		"decode session view checkpoint: %v",
		"解码 session 视图检查点失败：%v",
		"Der Prüfpunkt der Sessionansicht konnte nicht dekodiert werden: %v",
		"session ビューのチェックポイントをデコードできませんでした：%v",
		"session 보기 체크포인트를 디코딩하지 못했습니다: %v",
		"Не удалось декодировать контрольную точку представления session: %v")
	add(KeyTUISessionViewTrailingCheckpointData,
		"decode session view checkpoint trailing data",
		"解码 session 视图检查点失败：存在尾随数据",
		"Der Prüfpunkt der Sessionansicht enthält unerwartete nachgestellte Daten",
		"session ビューのチェックポイントに予期しない後続データがあります",
		"session 보기 체크포인트에 예기치 않은 후행 데이터가 있습니다",
		"Контрольная точка представления session содержит лишние данные в конце")
}
