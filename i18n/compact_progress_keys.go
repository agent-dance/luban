package i18n

const (
	KeyTUICompactProgressTitle               Key = "tui.compact_progress.title"
	KeyTUICompactProgressPreparing           Key = "tui.compact_progress.stage.preparing"
	KeyTUICompactProgressSummarizing         Key = "tui.compact_progress.stage.summarizing"
	KeyTUICompactProgressInstalling          Key = "tui.compact_progress.stage.installing"
	KeyTUICompactProgressPersisting          Key = "tui.compact_progress.stage.persisting"
	KeyTUICompactProgressElapsedCancel       Key = "tui.compact_progress.elapsed_cancel"
	KeyTUICompactProgressInputQueues         Key = "tui.compact_progress.input_queues"
	KeyTUICompactProgressInputQueued         Key = "tui.compact_progress.input_queued"
	KeyTUICompactProgressCompleted           Key = "tui.compact_progress.completed"
	KeyTUICompactProgressCompletedNoCounts   Key = "tui.compact_progress.completed_no_counts"
	KeyTUICompactProgressFailed              Key = "tui.compact_progress.failed"
	KeyTUICompactProgressCancelled           Key = "tui.compact_progress.cancelled"
	KeyTUICompactProgressCause               Key = "tui.compact_progress.cause"
	KeyTUICompactProgressProviderCalibration Key = "tui.compact_progress.provider_calibration"
)

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = repl(en, zh, de, ja, ko, ru)
	}
	add(KeyTUICompactProgressTitle,
		"Compacting context", "正在压缩上下文", "Kontext wird komprimiert", "コンテキストを圧縮中", "컨텍스트 압축 중", "Сжатие контекста")
	add(KeyTUICompactProgressPreparing,
		"Preparing context", "准备上下文", "Kontext wird vorbereitet", "コンテキストを準備中", "컨텍스트 준비 중", "Подготовка контекста")
	add(KeyTUICompactProgressSummarizing,
		"Generating LLM summary", "生成 LLM 摘要", "LLM-Zusammenfassung wird erstellt", "LLM 要約を生成中", "LLM 요약 생성 중", "Создание сводки LLM")
	add(KeyTUICompactProgressInstalling,
		"Installing compaction boundary", "安装压缩边界", "Komprimierungsgrenze wird installiert", "圧縮境界を適用中", "압축 경계 설치 중", "Установка границы сжатия")
	add(KeyTUICompactProgressPersisting,
		"Persisting session", "持久化会话", "Sitzung wird gespeichert", "セッションを保存中", "세션 저장 중", "Сохранение сеанса")
	add(KeyTUICompactProgressElapsedCancel,
		"%s elapsed · Esc to cancel", "已用 %s · 按 Esc 取消", "%s vergangen · Esc zum Abbrechen", "経過 %s · Esc でキャンセル", "%s 경과 · Esc로 취소", "Прошло %s · Esc — отменить")
	add(KeyTUICompactProgressInputQueues,
		"New input will be queued", "新输入将排队", "Neue Eingaben werden eingereiht", "新しい入力は待機します", "새 입력은 대기열에 추가됩니다", "Новый ввод будет поставлен в очередь")
	add(KeyTUICompactProgressInputQueued,
		"New input will be queued · %d waiting", "新输入将排队 · %d 条等待中", "Neue Eingaben werden eingereiht · %d warten", "新しい入力は待機します · %d 件待機中", "새 입력은 대기열에 추가됩니다 · %d개 대기 중", "Новый ввод будет поставлен в очередь · ожидают: %d")
	add(KeyTUICompactProgressCompleted,
		"Context compacted: about %d → %d tokens · %d → %d messages · %s", "上下文已压缩：约 %d → %d tokens · %d → %d 条消息 · %s", "Kontext komprimiert: ca. %d → %d Token · %d → %d Nachrichten · %s", "コンテキストを圧縮しました: 約 %d → %d tokens · %d → %d 件 · %s", "컨텍스트 압축 완료: 약 %d → %d tokens · 메시지 %d → %d개 · %s", "Контекст сжат: примерно %d → %d токенов · сообщений %d → %d · %s")
	add(KeyTUICompactProgressCompletedNoCounts,
		"Context compacted · %s", "上下文已压缩 · %s", "Kontext komprimiert · %s", "コンテキストを圧縮しました · %s", "컨텍스트 압축 완료 · %s", "Контекст сжат · %s")
	add(KeyTUICompactProgressFailed,
		"Context compaction failed · %s", "上下文压缩失败 · %s", "Kontextkomprimierung fehlgeschlagen · %s", "コンテキストの圧縮に失敗しました · %s", "컨텍스트 압축 실패 · %s", "Не удалось сжать контекст · %s")
	add(KeyTUICompactProgressCancelled,
		"Context compaction cancelled · %s", "已取消上下文压缩 · %s", "Kontextkomprimierung abgebrochen · %s", "コンテキストの圧縮をキャンセルしました · %s", "컨텍스트 압축 취소됨 · %s", "Сжатие контекста отменено · %s")
	add(KeyTUICompactProgressCause,
		"Cause: %s", "原因：%s", "Ursache: %s", "原因: %s", "원인: %s", "Причина: %s")
	add(KeyTUICompactProgressProviderCalibration,
		"≈ Local estimate; the next provider response will calibrate it", "≈ 本地估算；将在下一次 provider 响应后校准", "≈ Lokale Schätzung; die nächste Provider-Antwort kalibriert sie", "≈ ローカル推定値。次の provider 応答で補正されます", "≈ 로컬 추정치이며 다음 provider 응답에서 보정됩니다", "≈ Локальная оценка; следующий ответ provider уточнит её")
}
