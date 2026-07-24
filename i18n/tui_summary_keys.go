package i18n

const (
	KeyTUIImageTag                  Key = "tui.image.tag"
	KeyTUIStructuredEvidenceDecode  Key = "tui.evidence.structured_decode_failed"
	KeyTUIStructuredEvidenceEncode  Key = "tui.evidence.structured_encode_failed"
	KeyTUIObservationSummary        Key = "tui.observation.summary"
	KeyTUIAdditionalLines           Key = "tui.message.additional_lines"
	KeyTUIToolAdditionalLines       Key = "tui.message.tool_additional_lines"
	KeyTUIAgentTools                Key = "tui.agent.tools"
	KeyTUIAgentTokens               Key = "tui.agent.tokens"
	KeyTUIAgentDuration             Key = "tui.agent.duration_seconds"
	KeyTUIAgentCompleted            Key = "tui.agent.completed"
	KeyTUIAgentBackgrounded         Key = "tui.agent.backgrounded"
	KeyTUITeammateSpawned           Key = "tui.agent.teammate_spawned"
	KeyTUIAgentOutput               Key = "tui.agent.output"
	KeyTUIAgentStatus               Key = "tui.agent.status"
	KeyTUIActivityAttempt           Key = "tui.activity.attempt"
	KeyTUIActivityParent            Key = "tui.activity.parent"
	KeyTUIActivityProgress          Key = "tui.activity.progress"
	KeyTUIActivityOccurrences       Key = "tui.activity.occurrences"
	KeyTUISessionInputTotal         Key = "tui.session.input_total"
	KeyTUISessionUsageUnknownCost   Key = "tui.session.usage_unknown_cost"
	KeyTUISessionUsage              Key = "tui.session.usage"
	KeyTUICompactionRetainedRange   Key = "tui.compaction.retained_range"
	KeyTUICompactionDiscardedRange  Key = "tui.compaction.discarded_range"
	KeyTUICompactionSummary         Key = "tui.compaction.summary"
	KeyTUICompactionTerminal        Key = "tui.compaction.terminal"
	KeyTUICompactionTriggerManual   Key = "tui.compaction.trigger.manual"
	KeyTUICompactionTriggerAuto     Key = "tui.compaction.trigger.auto"
	KeyTUICompactionTriggerReactive Key = "tui.compaction.trigger.reactive"
	KeyTUICompactionTriggerSnip     Key = "tui.compaction.trigger.snip"
	KeyTUICompactionTriggerPartial  Key = "tui.compaction.trigger.partial"
	KeyTUICompactionTriggerUnknown  Key = "tui.compaction.trigger.unknown"
	KeyTUIOutcomeRunning            Key = "tui.outcome.running"
	KeyTUIOutcomeSucceeded          Key = "tui.outcome.succeeded"
	KeyTUIOutcomeCompleted          Key = "tui.outcome.completed"
	KeyTUIOutcomeFailed             Key = "tui.outcome.failed"
	KeyTUIOutcomePartial            Key = "tui.outcome.partial"
	KeyTUIOutcomeDenied             Key = "tui.outcome.denied"
	KeyTUIOutcomeCancelled          Key = "tui.outcome.cancelled"
	KeyTUIOutcomeTimedOut           Key = "tui.outcome.timed_out"
	KeyTUIOutcomeConflict           Key = "tui.outcome.identity_conflict"
	KeyTUIOutcomeOrphan             Key = "tui.outcome.unmatched_legacy_event"
	KeyTUIOutcomeUnknown            Key = "tui.outcome.unknown"
	KeyTUIConnectAPIKeyRequired     Key = "tui.connect.api_key_required"
	KeyTUIConnectSavingCredentials  Key = "tui.connect.saving_credentials"
	KeyTUIConnectInlineUnavailable  Key = "tui.connect.inline_unavailable"
)

var tuiOutcomeKeys = map[string]Key{
	"running": KeyTUIOutcomeRunning, "succeeded": KeyTUIOutcomeSucceeded, "completed": KeyTUIOutcomeCompleted,
	"failed": KeyTUIOutcomeFailed, "partial": KeyTUIOutcomePartial,
	"denied": KeyTUIOutcomeDenied, "cancelled": KeyTUIOutcomeCancelled,
	"timed_out": KeyTUIOutcomeTimedOut, "identity_conflict": KeyTUIOutcomeConflict,
	"unmatched_legacy_event": KeyTUIOutcomeOrphan, "unknown": KeyTUIOutcomeUnknown,
}

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}
	add(KeyTUIImageTag, "[Image #%d]", "[图片 #%d]", "[Bild #%d]", "[画像 #%d]", "[이미지 #%d]", "[Изображение #%d]")
	add(KeyTUIStructuredEvidenceDecode, "[structured evidence could not be safely decoded]", "[无法安全解码结构化证据]", "[strukturierte Belege konnten nicht sicher dekodiert werden]", "[構造化された証拠を安全にデコードできませんでした]", "[구조화된 증거를 안전하게 디코딩할 수 없음]", "[не удалось безопасно декодировать структурированные данные]")
	add(KeyTUIStructuredEvidenceEncode, "[structured evidence could not be safely encoded]", "[无法安全编码结构化证据]", "[strukturierte Belege konnten nicht sicher kodiert werden]", "[構造化された証拠を安全にエンコードできませんでした]", "[구조화된 증거를 안전하게 인코딩할 수 없음]", "[не удалось безопасно закодировать структурированные данные]")
	add(KeyTUIObservationSummary, "%s — %s — %d bytes", "%s — %s — %d 字节", "%s — %s — %d Byte", "%s — %s — %d バイト", "%s — %s — %d바이트", "%s — %s — %d байт")
	add(KeyTUIAdditionalLines, "  ↳ (%d lines)", "  ↳（%d 行）", "  ↳ (%d Zeilen)", "  ↳（%d 行）", "  ↳ (%d줄)", "  ↳ (%d строк)")
	add(KeyTUIToolAdditionalLines, "  ↳ %s (%d lines)", "  ↳ %s（%d 行）", "  ↳ %s (%d Zeilen)", "  ↳ %s（%d 行）", "  ↳ %s (%d줄)", "  ↳ %s (%d строк)")
	add(KeyTUIAgentTools, "%d tools", "%d 个工具", "%d Tools", "%d 個のツール", "도구 %d개", "%d инструментов")
	add(KeyTUIAgentTokens, "%d tokens", "%d 个 Token", "%d Token", "%d トークン", "토큰 %d개", "%d токенов")
	add(KeyTUIAgentDuration, "%.1f s", "%.1f 秒", "%.1f s", "%.1f 秒", "%.1f초", "%.1f с")
	add(KeyTUIAgentCompleted, "Agent completed", "Agent 已完成", "Agent abgeschlossen", "Agent が完了しました", "Agent 완료", "Agent завершил работу")
	add(KeyTUIAgentBackgrounded, "Agent moved to the background", "Agent 已转到后台", "Agent läuft jetzt im Hintergrund", "Agent をバックグラウンドに移しました", "Agent가 백그라운드로 전환됨", "Agent переведён в фон")
	add(KeyTUITeammateSpawned, "Teammate started", "队友已启动", "Teammitglied gestartet", "チームメイトを開始しました", "팀원 시작됨", "Участник команды запущен")
	add(KeyTUIAgentOutput, "output: %s", "输出：%s", "Ausgabe: %s", "出力: %s", "출력: %s", "вывод: %s")
	add(KeyTUIAgentStatus, "Agent %s", "Agent %s", "Agent %s", "Agent %s", "Agent %s", "Agent: %s")
	add(KeyTUIActivityAttempt, "  attempt=%d run=%s", "  尝试=%d 运行=%s", "  Versuch=%d Lauf=%s", "  試行=%d 実行=%s", "  시도=%d 실행=%s", "  попытка=%d запуск=%s")
	add(KeyTUIActivityParent, " parent=%s", " 上级=%s", " übergeordnet=%s", " 親=%s", " 상위=%s", " родитель=%s")
	add(KeyTUIActivityProgress, "  progress=%s", "  进度=%s", "  Fortschritt=%s", "  進捗=%s", "  진행=%s", "  ход=%s")
	add(KeyTUIActivityOccurrences, "  occurrences=%d sequence=%d..%d", "  出现次数=%d 序列=%d..%d", "  Vorkommen=%d Sequenz=%d..%d", "  発生回数=%d シーケンス=%d..%d", "  발생 횟수=%d 시퀀스=%d..%d", "  повторов=%d последовательность=%d..%d")
	add(KeyTUISessionInputTotal, "%s (%s total)", "%s（总计 %s）", "%s (%s gesamt)", "%s（合計 %s）", "%s(총 %s)", "%s (всего %s)")
	add(KeyTUISessionUsageUnknownCost, "Session: in %s · %d%% cached · out %s · cost unknown", "会话：输入 %s · 缓存 %d%% · 输出 %s · 费用未知", "Sitzung: %s ein · %d%% im Cache · %s aus · Kosten unbekannt", "セッション: 入力 %s · キャッシュ %d%% · 出力 %s · 料金不明", "세션: 입력 %s · 캐시 %d%% · 출력 %s · 비용 알 수 없음", "Сеанс: вход %s · %d%% из кэша · выход %s · стоимость неизвестна")
	add(KeyTUISessionUsage, "Session: in %s · %d%% cached · out %s · $%.4f", "会话：输入 %s · 缓存 %d%% · 输出 %s · $%.4f", "Sitzung: %s ein · %d%% im Cache · %s aus · $%.4f", "セッション: 入力 %s · キャッシュ %d%% · 出力 %s · $%.4f", "세션: 입력 %s · 캐시 %d%% · 출력 %s · $%.4f", "Сеанс: вход %s · %d%% из кэша · выход %s · $%.4f")
	add(KeyTUICompactionRetainedRange, "0..%d estimated post-compaction tokens", "0..%d 个压缩后预估 tokens", "0..%d geschätzte Token nach der Komprimierung", "0..%d 圧縮後の推定 tokens", "0..%d 압축 후 예상 tokens", "0..%d оценочных токенов после сжатия")
	add(KeyTUICompactionDiscardedRange, "%d..%d estimated pre-compaction tokens", "%d..%d 个压缩前预估 tokens", "%d..%d geschätzte Token vor der Komprimierung", "%d..%d 圧縮前の推定 tokens", "%d..%d 압축 전 예상 tokens", "%d..%d оценочных токенов до сжатия")
	add(KeyTUICompactionSummary, "Context compacted (%s): %d to %d tokens; retained %d; discarded %d", "上下文已压缩（%s）：%d → %d tokens；保留 %d；丢弃 %d", "Kontext komprimiert (%s): %d auf %d Token; %d behalten; %d verworfen", "コンテキストを圧縮（%s）: %d → %d tokens、%d を保持、%d を破棄", "컨텍스트 압축됨(%s): %d → %d tokens, %d 유지, %d 폐기", "Контекст сжат (%s): %d → %d токенов; сохранено %d; отброшено %d")
	add(KeyTUICompactionTerminal, "Context compaction %s (%s)", "上下文压缩%s（%s）", "Kontextkomprimierung %s (%s)", "コンテキスト圧縮: %s（%s）", "컨텍스트 압축 %s(%s)", "Сжатие контекста: %s (%s)")
	add(KeyTUICompactionTriggerManual, "manual", "手动", "manuell", "手動", "수동", "вручную")
	add(KeyTUICompactionTriggerAuto, "automatic", "自动", "automatisch", "自動", "자동", "автоматически")
	add(KeyTUICompactionTriggerReactive, "reactive", "响应式", "reaktiv", "リアクティブ", "반응형", "реактивно")
	add(KeyTUICompactionTriggerSnip, "snip", "裁剪", "Ausschnitt", "切り詰め", "잘라내기", "усечение")
	add(KeyTUICompactionTriggerPartial, "partial", "局部", "teilweise", "部分", "부분", "частично")
	add(KeyTUICompactionTriggerUnknown, "unknown", "未知", "unbekannt", "不明", "알 수 없음", "неизвестно")
	add(KeyTUIOutcomeRunning, "running", "运行中", "läuft", "実行中", "실행 중", "выполняется")
	add(KeyTUIOutcomeSucceeded, "succeeded", "成功", "erfolgreich", "成功", "성공", "успешно")
	add(KeyTUIOutcomeCompleted, "completed", "已完成", "abgeschlossen", "完了", "완료", "завершено")
	add(KeyTUIOutcomeFailed, "failed", "失败", "fehlgeschlagen", "失敗", "실패", "ошибка")
	add(KeyTUIOutcomePartial, "partial", "部分完成", "teilweise", "部分的", "부분 완료", "частично")
	add(KeyTUIOutcomeDenied, "denied", "已拒绝", "abgelehnt", "拒否", "거부됨", "отклонено")
	add(KeyTUIOutcomeCancelled, "cancelled", "已取消", "abgebrochen", "キャンセル", "취소됨", "отменено")
	add(KeyTUIOutcomeTimedOut, "timed out", "已超时", "Zeitüberschreitung", "タイムアウト", "시간 초과", "истекло время ожидания")
	add(KeyTUIOutcomeConflict, "identity conflict", "身份冲突", "Identitätskonflikt", "識別情報の競合", "ID 충돌", "конфликт идентичности")
	add(KeyTUIOutcomeOrphan, "unmatched legacy event", "无法匹配的旧版事件", "nicht zugeordnetes Legacy-Ereignis", "対応する記録がない旧形式イベント", "일치하지 않는 레거시 이벤트", "несопоставленное устаревшее событие")
	add(KeyTUIOutcomeUnknown, "unknown", "未知", "unbekannt", "不明", "알 수 없음", "неизвестно")
	add(KeyTUIConnectAPIKeyRequired, "Please enter an API key", "请输入 API key", "Bitte gib einen API-Schlüssel ein", "API キーを入力してください", "API 키를 입력하세요", "Введите API-ключ")
	add(KeyTUIConnectSavingCredentials, "Saving credentials…", "正在保存凭据…", "Zugangsdaten werden gespeichert…", "認証情報を保存しています…", "자격 증명 저장 중…", "Сохранение учётных данных…")
	add(KeyTUIConnectInlineUnavailable, "No inline connection flow is available for this Provider", "此 Provider 没有可用的内联连接流程", "Für diesen Provider ist keine direkte Verbindung verfügbar", "この Provider ではインライン接続を利用できません", "이 Provider에는 인라인 연결 흐름이 없습니다", "Для этого Provider недоступно встроенное подключение")
}

func TUIOutcomeLabel(lang Language, outcome string) string {
	if key, ok := tuiOutcomeKeys[outcome]; ok {
		return Text(lang, key)
	}
	return outcome
}

func TUICompactionTriggerLabel(lang Language, trigger string) string {
	key := KeyTUICompactionTriggerUnknown
	switch trigger {
	case "manual":
		key = KeyTUICompactionTriggerManual
	case "auto", "automatic":
		key = KeyTUICompactionTriggerAuto
	case "reactive":
		key = KeyTUICompactionTriggerReactive
	case "snip":
		key = KeyTUICompactionTriggerSnip
	default:
		if len(trigger) >= len("partial") && trigger[:len("partial")] == "partial" {
			key = KeyTUICompactionTriggerPartial
		}
	}
	return Text(lang, key)
}
