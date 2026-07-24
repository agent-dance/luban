package i18n

const (
	KeyCompactSummaryAPICallFailed         Key = "compact.error.summary_api_call_failed"
	KeyCompactSummaryStreamFailed          Key = "compact.error.summary_stream_failed"
	KeyCompactSummaryFailed                Key = "compact.error.summary_failed"
	KeyCompactHookBlocked                  Key = "compact.error.hook_blocked"
	KeyCompactHookBlockedWithoutReason     Key = "compact.error.hook_blocked_without_reason"
	KeyCompactReactiveCompactorUnavailable Key = "compact.error.reactive_compactor_unavailable"
	KeyLoopCompactionResultRejected        Key = "loop.compaction.result_rejected"
	KeyLoopPostCompactResetTrackingFailed  Key = "loop.post_compact.reset_tracking_failed"

	KeyLoopPostCompactSkillCatalogEpochChanged Key = "loop.post_compact.skill_catalog_epoch_changed"
	KeyLoopPostCompactSkillCatalogMissing      Key = "loop.post_compact.skill_catalog_missing"
	KeyLoopPostCompactSkillBodyEpochMissing    Key = "loop.post_compact.skill_body_epoch_missing"
	KeyLoopPostCompactSkillEnvelopeTrailing    Key = "loop.post_compact.skill_envelope_trailing"
	KeyLoopPostCompactSkillEnvelopeNoBody      Key = "loop.post_compact.skill_envelope_no_body"
)

var compactLoopErrorKeys = []Key{
	KeyCompactSummaryAPICallFailed,
	KeyCompactSummaryStreamFailed,
	KeyCompactSummaryFailed,
	KeyCompactHookBlocked,
	KeyCompactHookBlockedWithoutReason,
	KeyCompactReactiveCompactorUnavailable,
	KeyLoopCompactionResultRejected,
	KeyLoopPostCompactResetTrackingFailed,
	KeyLoopPostCompactSkillCatalogEpochChanged,
	KeyLoopPostCompactSkillCatalogMissing,
	KeyLoopPostCompactSkillBodyEpochMissing,
	KeyLoopPostCompactSkillEnvelopeTrailing,
	KeyLoopPostCompactSkillEnvelopeNoBody,
}

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}

	add(KeyCompactSummaryAPICallFailed,
		"compaction API call failed: %v",
		"调用压缩 API 失败：%v",
		"Der API-Aufruf zur Komprimierung ist fehlgeschlagen: %v",
		"圧縮 API の呼び出しに失敗しました：%v",
		"압축 API 호출에 실패했습니다: %v",
		"Ошибка вызова API сжатия: %v")
	add(KeyCompactSummaryStreamFailed,
		"compaction stream error: %v",
		"压缩数据流出错：%v",
		"Fehler im Komprimierungsstream: %v",
		"圧縮ストリームでエラーが発生しました：%v",
		"압축 스트림 오류: %v",
		"Ошибка потока сжатия: %v")
	add(KeyCompactSummaryFailed,
		"compact summary failed: %v",
		"生成压缩摘要失败：%v",
		"Die Komprimierungszusammenfassung konnte nicht erstellt werden: %v",
		"圧縮用の要約を生成できませんでした：%v",
		"압축 요약을 생성하지 못했습니다: %v",
		"Не удалось создать сводку для сжатия: %v")
	add(KeyCompactHookBlocked,
		"%s hook blocked compaction: %s",
		"%s hook 阻止了压缩：%s",
		"Der Hook %s hat die Komprimierung blockiert: %s",
		"%s hook が圧縮をブロックしました：%s",
		"%s hook이(가) 압축을 차단했습니다: %s",
		"Hook %s заблокировал сжатие: %s")
	add(KeyCompactHookBlockedWithoutReason,
		"%s hook blocked compaction",
		"%s hook 阻止了压缩",
		"Der Hook %s hat die Komprimierung blockiert",
		"%s hook が圧縮をブロックしました",
		"%s hook이(가) 압축을 차단했습니다",
		"Hook %s заблокировал сжатие")
	add(KeyCompactReactiveCompactorUnavailable,
		"Context recovery stopped because no semantic compactor is configured; history was left unchanged.",
		"未配置语义压缩器，上下文恢复已停止；历史记录保持不变。",
		"Die Kontextwiederherstellung wurde beendet, weil kein semantischer Komprimierer konfiguriert ist; der Verlauf blieb unverändert.",
		"セマンティック圧縮が設定されていないため、コンテキストの復旧を停止しました。履歴は変更されていません。",
		"의미 기반 압축기가 구성되지 않아 컨텍스트 복구를 중단했습니다. 기록은 변경되지 않았습니다.",
		"Восстановление контекста остановлено, поскольку семантическое сжатие не настроено; история осталась без изменений.")
	add(KeyLoopCompactionResultRejected,
		"Compaction returned a result that cannot be installed safely; history was left unchanged.",
		"压缩返回的结果无法安全安装；历史记录保持不变。",
		"Die Komprimierung hat ein Ergebnis zurückgegeben, das nicht sicher installiert werden kann; der Verlauf blieb unverändert.",
		"圧縮から安全に反映できない結果が返されたため、履歴は変更されませんでした。",
		"압축 결과를 안전하게 반영할 수 없어 기록을 변경하지 않았습니다.",
		"Результат сжатия нельзя безопасно применить; история осталась без изменений.")
	add(KeyLoopPostCompactResetTrackingFailed,
		"reset session-memory compaction tracking: %v",
		"重置 session memory 压缩跟踪状态失败：%v",
		"Das Tracking der Session-Memory-Komprimierung konnte nicht zurückgesetzt werden: %v",
		"session memory の圧縮追跡をリセットできませんでした：%v",
		"session memory 압축 추적을 재설정하지 못했습니다: %v",
		"Не удалось сбросить отслеживание сжатия session memory: %v")

	add(KeyLoopPostCompactSkillCatalogEpochChanged,
		"The Skill catalog changed while restoring post-compaction state.",
		"恢复压缩后的状态时，Skill 目录已发生变化",
		"Der Skill-Katalog hat sich während der Wiederherstellung nach der Komprimierung geändert",
		"圧縮後の状態を復元している間に Skill カタログが変更されました",
		"압축 후 상태를 복원하는 동안 Skill 카탈로그가 변경되었습니다",
		"Каталог Skill изменился во время восстановления состояния после сжатия")
	add(KeyLoopPostCompactSkillCatalogMissing,
		"The current Skill catalog snapshot is missing from the post-compaction history.",
		"压缩后的历史记录中缺少当前 Skill 目录快照",
		"Im Verlauf nach der Komprimierung fehlt der aktuelle Skill-Katalog-Snapshot",
		"圧縮後の履歴に現在の Skill カタログのスナップショットがありません",
		"압축 후 기록에 현재 Skill 카탈로그 스냅샷이 없습니다",
		"В истории после сжатия отсутствует текущий снимок каталога Skill")
	add(KeyLoopPostCompactSkillBodyEpochMissing,
		"The post-compaction Skill body message is missing a valid context epoch.",
		"压缩后的 Skill 内容消息缺少有效的上下文 epoch",
		"Der Skill-Inhaltsnachricht nach der Komprimierung fehlt eine gültige Kontext-Epoche",
		"圧縮後の Skill 本文メッセージに有効なコンテキスト epoch がありません",
		"압축 후 Skill 본문 메시지에 유효한 컨텍스트 epoch가 없습니다",
		"В сообщении с содержимым Skill после сжатия отсутствует допустимая эпоха контекста")
	add(KeyLoopPostCompactSkillEnvelopeTrailing,
		"skill invocation envelope contains trailing JSON",
		"Skill 调用 envelope 末尾包含多余的 JSON",
		"Der Skill-Aufruf-Envelope enthält zusätzliches JSON am Ende",
		"Skill 呼び出し envelope の末尾に余分な JSON があります",
		"Skill 호출 envelope 뒤에 불필요한 JSON이 있습니다",
		"После envelope вызова Skill обнаружен лишний JSON")
	add(KeyLoopPostCompactSkillEnvelopeNoBody,
		"skill invocation envelope does not carry a body",
		"Skill 调用 envelope 中不包含内容正文",
		"Der Skill-Aufruf-Envelope enthält keinen Inhalt",
		"Skill 呼び出し envelope に本文が含まれていません",
		"Skill 호출 envelope에 본문이 없습니다",
		"Envelope вызова Skill не содержит тела")
}
