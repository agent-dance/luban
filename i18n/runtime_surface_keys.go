package i18n

const (
	KeyRuntimeErrorEvidenceRetention Key = "runtime.error.evidence_retention"
	KeyRuntimeHookEvidenceRetention  Key = "runtime.hook.evidence_retention"
	KeyRuntimeToolCallPresentation   Key = "runtime.tool_call.presentation_error"
	KeyRuntimeToolResultRetention    Key = "runtime.tool_result.evidence_retention"
	KeyRuntimeHiddenToolCall         Key = "runtime.hidden_tool_call.presentation_error"
	KeyRuntimeHiddenToolResult       Key = "runtime.hidden_tool_result.presentation_error"
	KeyRuntimeContextCompaction      Key = "runtime.context_compaction"
	KeyRuntimeLegacyAction           Key = "runtime.permission.legacy_action"
	KeyRuntimeLegacyImpact           Key = "runtime.permission.legacy_impact"
	KeyRuntimeLegacyRule             Key = "runtime.permission.legacy_rule"
	KeyRuntimeLegacyScope            Key = "runtime.permission.legacy_scope"
	KeyRuntimeQueryCancelled         Key = "runtime.query.cancelled"
	KeyDeleteHistoryAction           Key = "session.delete_history.action"
	KeyDeleteHistoryImpact           Key = "session.delete_history.impact"
	KeyDeleteHistoryRisk             Key = "session.delete_history.risk"
	KeyDeleteHistoryRule             Key = "session.delete_history.rule"
	KeyDeleteHistoryScope            Key = "session.delete_history.scope"
	KeyDeleteHistoryBody             Key = "session.delete_history.body"
	KeyDeleteHistoryMessage          Key = "session.delete_history.message"
	KeyMarkdownImagePrefix           Key = "markdown.image.prefix"
)

func init() {
	addRuntimeSurface(KeyRuntimeErrorEvidenceRetention, "%s (could not retain error evidence: %v)", "%s（无法留存错误证据：%v）", "%s (Fehlerbeleg konnte nicht gespeichert werden: %v)", "%s（エラーエビデンスを保持できませんでした: %v）", "%s (오류 증거를 보관할 수 없음: %v)", "%s (не удалось сохранить сведения об ошибке: %v)")
	addRuntimeSurface(KeyRuntimeHookEvidenceRetention, "Could not retain hook evidence: %v", "无法留存 Hook 证据：%v", "Hook-Beleg konnte nicht gespeichert werden: %v", "Hook のエビデンスを保持できませんでした: %v", "Hook 증거를 보관할 수 없습니다: %v", "Не удалось сохранить сведения хука: %v")
	addRuntimeSurface(KeyRuntimeToolCallPresentation, "Could not present the tool call: %v", "无法展示工具调用：%v", "Tool-Aufruf konnte nicht dargestellt werden: %v", "ツール呼び出しを表示できませんでした: %v", "도구 호출을 표시할 수 없습니다: %v", "Не удалось показать вызов инструмента: %v")
	addRuntimeSurface(KeyRuntimeToolResultRetention, "Could not retain tool-result evidence: %v", "无法留存工具结果证据：%v", "Ergebnisbeleg des Tools konnte nicht gespeichert werden: %v", "ツール結果のエビデンスを保持できませんでした: %v", "도구 결과 증거를 보관할 수 없습니다: %v", "Не удалось сохранить сведения о результате инструмента: %v")
	addRuntimeSurface(KeyRuntimeHiddenToolCall, "Could not present the hidden tool call: %v", "无法展示隐藏的工具调用：%v", "Ausgeblendeter Tool-Aufruf konnte nicht dargestellt werden: %v", "非表示のツール呼び出しを表示できませんでした: %v", "숨겨진 도구 호출을 표시할 수 없습니다: %v", "Не удалось показать скрытый вызов инструмента: %v")
	addRuntimeSurface(KeyRuntimeHiddenToolResult, "Could not present the hidden tool result: %v", "无法展示隐藏的工具结果：%v", "Ausgeblendetes Tool-Ergebnis konnte nicht dargestellt werden: %v", "非表示のツール結果を表示できませんでした: %v", "숨겨진 도구 결과를 표시할 수 없습니다: %v", "Не удалось показать скрытый результат инструмента: %v")
	addRuntimeSurface(KeyRuntimeContextCompaction, "Context compaction", "上下文压缩", "Kontextkomprimierung", "コンテキスト圧縮", "컨텍스트 압축", "Сжатие контекста")
	addRuntimeSurface(KeyRuntimeLegacyAction, "Execute %s", "执行 %s", "%s ausführen", "%s を実行", "%s 실행", "Выполнить %s")
	addRuntimeSurface(KeyRuntimeLegacyImpact, "Run the requested tool with the supplied input", "使用给定输入运行所请求的工具", "Das angeforderte Tool mit der angegebenen Eingabe ausführen", "指定された入力で要求されたツールを実行します", "제공된 입력으로 요청한 도구 실행", "Запустить запрошенный инструмент с указанными входными данными")
	addRuntimeSurface(KeyRuntimeLegacyRule, "Legacy permission prompt", "旧版权限确认", "Alte Berechtigungsabfrage", "従来の権限確認", "기존 권한 확인", "Устаревший запрос разрешения")
	addRuntimeSurface(KeyRuntimeLegacyScope, "allow_once: this invocation; always_allow: the same tool and target in this session", "allow_once：仅本次调用；always_allow：本会话中相同工具和目标", "allow_once: dieser Aufruf; always_allow: dasselbe Tool und Ziel in dieser Sitzung", "allow_once: この呼び出しのみ。always_allow: このセッション内の同じツールと対象", "allow_once: 이번 호출만, always_allow: 이 세션에서 같은 도구와 대상", "allow_once: этот вызов; always_allow: тот же инструмент и цель в этом сеансе")
	addRuntimeSurface(KeyRuntimeQueryCancelled, "Query cancelled. Press Ctrl+C again to exit.", "查询已取消。再次按 Ctrl+C 退出。", "Anfrage abgebrochen. Drücke zum Beenden erneut Ctrl+C.", "クエリをキャンセルしました。終了するにはもう一度 Ctrl+C を押してください。", "질의가 취소되었습니다. 종료하려면 Ctrl+C를 다시 누르세요.", "Запрос отменён. Для выхода снова нажмите Ctrl+C.")
	addRuntimeSurface(KeyDeleteHistoryAction, "Delete session history", "删除会话历史", "Sitzungsverlauf löschen", "セッション履歴を削除", "세션 기록 삭제", "Удалить историю сеанса")
	addRuntimeSurface(KeyDeleteHistoryImpact, "Permanently remove the transcript, metadata, and session artifacts", "永久删除对话记录、元数据和会话产物", "Transkript, Metadaten und Sitzungsartefakte dauerhaft entfernen", "トランスクリプト、メタデータ、セッション成果物を完全に削除します", "대화 기록, 메타데이터, 세션 산출물을 영구 삭제", "Безвозвратно удалить стенограмму, метаданные и артефакты сеанса")
	addRuntimeSurface(KeyDeleteHistoryRisk, "Deletion cannot be undone in this application", "此应用无法撤销删除操作", "Das Löschen kann in dieser Anwendung nicht rückgängig gemacht werden", "このアプリでは削除を元に戻せません", "이 애플리케이션에서는 삭제를 취소할 수 없음", "Удаление нельзя отменить в этом приложении")
	addRuntimeSurface(KeyDeleteHistoryRule, "Explicit history-deletion policy", "显式历史删除策略", "Explizite Richtlinie zum Löschen des Verlaufs", "明示的な履歴削除ポリシー", "명시적 기록 삭제 정책", "Явная политика удаления истории")
	addRuntimeSurface(KeyDeleteHistoryScope, "This session only", "仅此会话", "Nur diese Sitzung", "このセッションのみ", "이 세션만", "Только этот сеанс")
	addRuntimeSurface(KeyDeleteHistoryBody, "Review the target session carefully. Approval permanently deletes its stored history and live engine conversation.", "请仔细核对目标会话。批准后将永久删除其已存储的历史和 Engine 中的当前对话。", "Prüfe die Zielsitzung sorgfältig. Die Genehmigung löscht ihren gespeicherten Verlauf und die laufende Engine-Unterhaltung dauerhaft.", "対象セッションをよく確認してください。承認すると、保存済みの履歴と Engine 上の現在の会話が完全に削除されます。", "대상 세션을 주의 깊게 확인하세요. 승인하면 저장된 기록과 Engine의 현재 대화가 영구 삭제됩니다.", "Внимательно проверьте целевой сеанс. Подтверждение безвозвратно удалит сохранённую историю и текущий диалог в Engine.")
	addRuntimeSurface(KeyDeleteHistoryMessage, "Permanently deleting session history requires explicit approval.", "永久删除会话历史需要明确批准。", "Das dauerhafte Löschen des Sitzungsverlaufs erfordert eine ausdrückliche Genehmigung.", "セッション履歴を完全に削除するには明示的な承認が必要です。", "세션 기록을 영구 삭제하려면 명시적 승인이 필요합니다.", "Для безвозвратного удаления истории сеанса требуется явное подтверждение.")
	addRuntimeSurface(KeyMarkdownImagePrefix, "[Image: ", "[图片：", "[Bild: ", "[画像: ", "[이미지: ", "[Изображение: ")
}

func addRuntimeSurface(key Key, en, zh, de, ja, ko, ru string) {
	semanticTranslations[key] = map[Language]string{LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru}
}
