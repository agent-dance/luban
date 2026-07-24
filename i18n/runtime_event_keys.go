package i18n

const (
	KeyRuntimeWarningPublicSummary        Key = "runtime.warning.public_summary"
	KeyRuntimeHookEvidenceRetentionFailed Key = "runtime.hook.evidence_retention_failed"
	KeyRuntimeUserInterrupted             Key = "runtime.user_interrupted"
	KeyRuntimePermissionCheckFailed       Key = "runtime.tool.permission_check_failed"
	KeyRuntimePermissionDenied            Key = "runtime.tool.permission_denied"
	KeyRuntimeToolInputNormalization      Key = "runtime.tool.input_normalization_failed"
	KeyRuntimeToolExecutionFailed         Key = "runtime.tool.execution_failed"
	KeyRuntimeToolExecutionCancelled      Key = "runtime.tool.execution_cancelled"
	KeyRuntimeToolBlockedByHook           Key = "runtime.tool.blocked_by_hook"
	KeyRuntimeToolInputValidation         Key = "runtime.tool.input_validation_failed"
	KeyRuntimeStreamingToolDiscarded      Key = "runtime.tool.streaming_discarded"
	KeyRuntimeParallelToolCancelled       Key = "runtime.tool.parallel_cancelled"
	KeyRuntimeParallelNamedToolCancelled  Key = "runtime.tool.parallel_named_cancelled"
	KeyRuntimePromptTooLong               Key = "runtime.prompt_too_long"
	KeyRuntimeModelFallback               Key = "runtime.model.fallback"
	KeyRuntimeTransientAPIError           Key = "runtime.api.transient_error"
	KeyRuntimeStreamInterruptedPartial    Key = "runtime.stream.interrupted_partial"
	KeyRuntimeStreamRetryFullHistory      Key = "runtime.stream.retry_full_history"
	KeyRuntimeResponseTruncated           Key = "runtime.response.truncated"
	KeyRuntimeResponseRetryMaxTokens      Key = "runtime.response.retry_max_tokens"
	KeyRuntimeResponseRecovery            Key = "runtime.response.recovery_continuation"
	KeyRuntimeTokenBudgetContinuation     Key = "runtime.token_budget.continuation"
	KeyRuntimeTokenBudgetDiminishing      Key = "runtime.token_budget.diminishing_returns"
	KeyRuntimeGoalLoadMaxTokens           Key = "runtime.goal.load_max_tokens_failed"
	KeyRuntimeGoalLoadFailed              Key = "runtime.goal.load_failed"
	KeyRuntimeGoalEvaluatorUnavailable    Key = "runtime.goal.evaluator_unavailable"
	KeyRuntimeGoalEvaluatorFailed         Key = "runtime.goal.evaluator_failed"
	KeyRuntimeGoalLoadStopHook            Key = "runtime.goal.load_stop_hook_failed"
	KeyRuntimeGoalLoadToolExecution       Key = "runtime.goal.load_tool_execution_failed"
	KeyRuntimeGoalBudgetReached           Key = "runtime.goal.token_budget_reached"
	KeyRuntimeGoalChangedStale            Key = "runtime.goal.stale_evaluation"
	KeyRuntimeGoalChangedDuringSave       Key = "runtime.goal.changed_during_save"
	KeyRuntimeGoalUsageSaveFailed         Key = "runtime.goal.usage_save_failed"
	KeyRuntimeAutoCompactFailed           Key = "runtime.compaction.auto_failed"
	KeyRuntimePostCompactCleanupFailed    Key = "runtime.compaction.cleanup_failed"
	KeyRuntimeCompactionCommitFailed      Key = "runtime.compaction.commit_failed"
	KeyRuntimeContextOverflowDrain        Key = "runtime.compaction.context_overflow_drain"
	KeyRuntimeProviderRejectionRetry      Key = "runtime.provider_rejection.retry"
	KeyRuntimeReactiveCompact             Key = "runtime.compaction.reactive"
	KeyRuntimeMediaStrip                  Key = "runtime.compaction.media_strip"
	KeyRuntimeToolInputJSONFailed         Key = "runtime.tool.input_json_failed"
	KeyRuntimeToolInputJSONFlushFailed    Key = "runtime.tool.input_json_flush_failed"
	KeyRuntimeToolSkippedMalformed        Key = "runtime.tool.skipped_malformed_input"
	KeyRuntimeToolDisabled                Key = "runtime.tool.disabled"
	KeyRuntimeToolPlanDenied              Key = "runtime.tool.plan_denied"
	KeyRuntimeToolRuleDenied              Key = "runtime.tool.rule_denied"
	KeyRuntimeToolPermissionRequired      Key = "runtime.tool.permission_required"
	KeyRuntimeResponsesStreamIncomplete   Key = "runtime.responses.stream_incomplete"
	KeyRuntimePermissionActionExecute     Key = "runtime.permission.action.execute"
	KeyRuntimePermissionRuleToolContract  Key = "runtime.permission.rule.tool_contract"
	KeyRuntimePermissionScopeInvocation   Key = "runtime.permission.scope.invocation"
	KeyRuntimePermissionImpactDefault     Key = "runtime.permission.impact.default"
	KeyRuntimePermissionRuleRequired      Key = "runtime.permission.rule.required"
	KeyRuntimePlanActionExecute           Key = "runtime.permission.plan.action.execute"
	KeyRuntimePlanImpactExecute           Key = "runtime.permission.plan.impact.execute"
	KeyRuntimePlanRiskExecute             Key = "runtime.permission.plan.risk.execute"
	KeyRuntimePlanRuleGate                Key = "runtime.permission.plan.rule.gate"
	KeyRuntimePlanScopeTransition         Key = "runtime.permission.plan.scope.transition"
	KeyRuntimePlanAllowedPrompts          Key = "runtime.permission.plan.allowed_prompts"
	KeyRuntimePlanAutoModeFallback        Key = "runtime.permission.plan.auto_mode_fallback"
	KeyRuntimePermissionTargetInput       Key = "runtime.permission.target.input"
	KeyRuntimeMissingToolResult           Key = "runtime.tool.missing_result"
)

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}
	add(KeyRuntimeWarningPublicSummary,
		"The runtime recovered from an internal warning.", "运行时已从内部警告中恢复。", "Die Laufzeit hat sich von einer internen Warnung erholt.",
		"ランタイムは内部警告から復旧しました。", "런타임이 내부 경고에서 복구되었습니다.", "Среда выполнения восстановилась после внутреннего предупреждения.")
	add(KeyRuntimeHookEvidenceRetentionFailed,
		"Could not retain hook evidence: %v", "无法保留 hook 证据：%v", "Hook-Belege konnten nicht gespeichert werden: %v",
		"hook の証拠を保持できませんでした: %v", "hook 증거를 보관할 수 없습니다: %v", "Не удалось сохранить данные hook: %v")
	add(KeyRuntimeUserInterrupted,
		"Interrupted by user", "已被用户中断", "Vom Benutzer unterbrochen", "ユーザーにより中断されました", "사용자가 중단함", "Прервано пользователем")
	add(KeyRuntimePermissionCheckFailed,
		"Permission check failed for tool %s: %v", "工具 %s 的权限检查失败：%v", "Berechtigungsprüfung für Tool %s fehlgeschlagen: %v",
		"ツール %s の権限確認に失敗しました: %v", "도구 %s의 권한 확인에 실패했습니다: %v", "Проверка разрешения для инструмента %s завершилась ошибкой: %v")
	add(KeyRuntimePermissionDenied,
		"Permission denied for tool: %s", "已拒绝工具 %s 的权限", "Berechtigung für Tool %s abgelehnt", "ツール %s の権限が拒否されました", "도구 %s의 권한이 거부되었습니다", "Разрешение для инструмента %s отклонено")
	add(KeyRuntimeToolInputNormalization,
		"Tool input normalization failed: %v", "工具输入标准化失败：%v", "Normalisierung der Tool-Eingabe fehlgeschlagen: %v",
		"ツール入力の正規化に失敗しました: %v", "도구 입력 정규화에 실패했습니다: %v", "Не удалось нормализовать входные данные инструмента: %v")
	add(KeyRuntimeToolExecutionFailed,
		"Tool execution failed: %v", "工具执行失败：%v", "Tool-Ausführung fehlgeschlagen: %v",
		"ツールの実行に失敗しました: %v", "도구 실행에 실패했습니다: %v", "Ошибка выполнения инструмента: %v")
	add(KeyRuntimeToolExecutionCancelled,
		"Tool execution cancelled", "工具执行已取消", "Tool-Ausführung abgebrochen", "ツールの実行をキャンセルしました", "도구 실행이 취소되었습니다", "Выполнение инструмента отменено")
	add(KeyRuntimeToolBlockedByHook,
		"Tool execution blocked by hook", "工具执行被 hook 阻止", "Tool-Ausführung durch Hook blockiert", "ツールの実行が hook によりブロックされました", "도구 실행이 hook에 의해 차단되었습니다", "Выполнение инструмента заблокировано hook")
	add(KeyRuntimeToolInputValidation,
		"<tool_use_error>Input validation failed: %v</tool_use_error>", "<tool_use_error>输入验证失败：%v</tool_use_error>", "<tool_use_error>Eingabeprüfung fehlgeschlagen: %v</tool_use_error>",
		"<tool_use_error>入力検証に失敗しました: %v</tool_use_error>", "<tool_use_error>입력 검증에 실패했습니다: %v</tool_use_error>", "<tool_use_error>Ошибка проверки входных данных: %v</tool_use_error>")
	add(KeyRuntimeStreamingToolDiscarded,
		"Streaming fallback: tool execution discarded", "streaming fallback：已丢弃工具执行", "Streaming-Fallback: Tool-Ausführung verworfen",
		"streaming fallback: ツールの実行を破棄しました", "streaming fallback: 도구 실행을 폐기했습니다", "Streaming fallback: выполнение инструмента отброшено")
	add(KeyRuntimeParallelToolCancelled,
		"Cancelled because a parallel tool call failed", "因并行工具调用失败而取消", "Abgebrochen, weil ein paralleler Tool-Aufruf fehlgeschlagen ist",
		"並行ツール呼び出しが失敗したためキャンセルしました", "병렬 도구 호출이 실패하여 취소되었습니다", "Отменено из-за ошибки параллельного вызова инструмента")
	add(KeyRuntimeParallelNamedToolCancelled,
		"Cancelled because parallel tool call %s failed", "因并行工具调用 %s 失败而取消", "Abgebrochen, weil der parallele Tool-Aufruf %s fehlgeschlagen ist",
		"並行ツール呼び出し %s が失敗したためキャンセルしました", "병렬 도구 호출 %s이(가) 실패하여 취소되었습니다", "Отменено из-за ошибки параллельного вызова инструмента %s")
	add(KeyRuntimePromptTooLong,
		"The prompt is too long. Run /compact to compact the conversation and continue.", "prompt 过长。请运行 /compact 压缩对话后继续。", "Der Prompt ist zu lang. Komprimiere die Unterhaltung mit /compact und fahre dann fort.",
		"prompt が長すぎます。/compact で会話を圧縮してから続行してください。", "prompt가 너무 깁니다. /compact로 대화를 압축한 후 계속하세요.", "Prompt слишком длинный. Выполните /compact, чтобы сжать диалог и продолжить.")
	add(KeyRuntimeModelFallback,
		"Because %s is under high demand, switched to %s", "%s 当前负载较高，已切换到 %s", "Wegen hoher Auslastung von %s wurde zu %s gewechselt",
		"%s の負荷が高いため %s に切り替えました", "%s의 수요가 높아 %s(으)로 전환했습니다", "Из-за высокой нагрузки на %s выполнено переключение на %s")
	add(KeyRuntimeTransientAPIError,
		"Transient API error (attempt %d/%d); retrying", "临时 API 错误（第 %d/%d 次尝试）；正在重试", "Vorübergehender API-Fehler (Versuch %d/%d); erneuter Versuch",
		"一時的な API エラー（試行 %d/%d）。再試行します", "일시적인 API 오류(시도 %d/%d). 다시 시도합니다", "Временная ошибка API (попытка %d/%d); повтор")
	add(KeyRuntimeStreamInterruptedPartial,
		"Stream interrupted; continuing with %d partial blocks", "stream 已中断；将使用 %d 个部分 block 继续", "Stream unterbrochen; Fortsetzung mit %d Teilblöcken",
		"stream が中断されました。%d 個の部分 block で続行します", "stream이 중단되었습니다. 부분 block %d개로 계속합니다", "Stream прерван; продолжение с %d частичными блоками")
	add(KeyRuntimeStreamRetryFullHistory,
		"Stream failed; clearing the response chain and retrying with full message history", "stream 失败；正在清除 response chain，并使用完整消息历史重试", "Stream fehlgeschlagen; Response-Chain wird gelöscht und mit vollständigem Nachrichtenverlauf erneut versucht",
		"stream に失敗しました。response chain を消去し、完全なメッセージ履歴で再試行します", "stream 실패. response chain을 지우고 전체 메시지 기록으로 다시 시도합니다", "Ошибка stream; цепочка ответов очищена, повтор с полной историей сообщений")
	add(KeyRuntimeResponseTruncated,
		"Response truncated (max_tokens)", "响应已截断（max_tokens）", "Antwort gekürzt (max_tokens)", "応答が切り詰められました（max_tokens）", "응답이 잘렸습니다(max_tokens)", "Ответ усечён (max_tokens)")
	add(KeyRuntimeResponseRetryMaxTokens,
		"Response truncated (max_tokens); retrying with max_output_tokens=%d", "响应已截断（max_tokens）；正在以 max_output_tokens=%d 重试", "Antwort gekürzt (max_tokens); erneuter Versuch mit max_output_tokens=%d",
		"応答が切り詰められました（max_tokens）。max_output_tokens=%d で再試行します", "응답이 잘렸습니다(max_tokens). max_output_tokens=%d(으)로 다시 시도합니다", "Ответ усечён (max_tokens); повтор с max_output_tokens=%d")
	add(KeyRuntimeResponseRecovery,
		"Response truncated (max_tokens); recovery continuation %d/%d", "响应已截断（max_tokens）；恢复续写 %d/%d", "Antwort gekürzt (max_tokens); Wiederherstellungsfortsetzung %d/%d",
		"応答が切り詰められました（max_tokens）。復旧の続行 %d/%d", "응답이 잘렸습니다(max_tokens). 복구 계속 %d/%d", "Ответ усечён (max_tokens); продолжение восстановления %d/%d")
	add(KeyRuntimeTokenBudgetContinuation,
		"Token-budget continuation #%d: %d%% (%d / %d)", "Token budget 续写 #%d：%d%%（%d / %d）", "Token-Budget-Fortsetzung #%d: %d%% (%d / %d)",
		"Token budget の続行 #%d: %d%%（%d / %d）", "Token budget 계속 #%d: %d%%(%d / %d)", "Продолжение по бюджету токенов #%d: %d%% (%d / %d)")
	add(KeyRuntimeTokenBudgetDiminishing,
		"Token-budget early stop: diminishing returns at %d%%", "Token budget 提前停止：进度达到 %d%% 时收益递减", "Token-Budget vorzeitig beendet: ab %d%% abnehmender Nutzen",
		"Token budget を早期終了: %d%% で効果が逓減", "Token budget 조기 중지: %d%%에서 효율 감소", "Досрочная остановка по бюджету токенов: убывающая отдача на %d%%")
	add(KeyRuntimeGoalLoadMaxTokens,
		"Could not load the goal during max_tokens recovery; automatic continuation stopped", "max_tokens 恢复期间无法加载目标；已停止自动续写", "Ziel konnte während der max_tokens-Wiederherstellung nicht geladen werden; automatische Fortsetzung gestoppt",
		"max_tokens の復旧中に目標を読み込めませんでした。自動続行を停止しました", "max_tokens 복구 중 목표를 불러올 수 없어 자동 계속을 중지했습니다", "Не удалось загрузить цель при восстановлении max_tokens; автоматическое продолжение остановлено")
	add(KeyRuntimeGoalLoadFailed,
		"Could not load the goal; automatic continuation stopped", "无法加载目标；已停止自动续写", "Ziel konnte nicht geladen werden; automatische Fortsetzung gestoppt",
		"目標を読み込めませんでした。自動続行を停止しました", "목표를 불러올 수 없어 자동 계속을 중지했습니다", "Не удалось загрузить цель; автоматическое продолжение остановлено")
	add(KeyRuntimeGoalEvaluatorUnavailable,
		"The goal evaluator is unavailable; automatic continuation stopped", "目标评估器不可用；已停止自动续写", "Die Zielauswertung ist nicht verfügbar; automatische Fortsetzung gestoppt",
		"目標評価を利用できません。自動続行を停止しました", "목표 평가기를 사용할 수 없어 자동 계속을 중지했습니다", "Оценщик цели недоступен; автоматическое продолжение остановлено")
	add(KeyRuntimeGoalEvaluatorFailed,
		"Goal evaluation failed; automatic continuation stopped", "目标评估失败；已停止自动续写", "Zielauswertung fehlgeschlagen; automatische Fortsetzung gestoppt",
		"目標の評価に失敗しました。自動続行を停止しました", "목표 평가에 실패하여 자동 계속을 중지했습니다", "Ошибка оценки цели; автоматическое продолжение остановлено")
	add(KeyRuntimeGoalLoadStopHook,
		"Could not load the goal after Stop hook feedback; automatic continuation stopped", "收到 Stop hook 反馈后无法加载目标；已停止自动续写", "Ziel konnte nach der Rückmeldung des Stop-Hooks nicht geladen werden; automatische Fortsetzung gestoppt",
		"Stop hook のフィードバック後に目標を読み込めませんでした。自動続行を停止しました", "Stop hook 피드백 후 목표를 불러올 수 없어 자동 계속을 중지했습니다", "Не удалось загрузить цель после ответа Stop hook; автоматическое продолжение остановлено")
	add(KeyRuntimeGoalLoadToolExecution,
		"Could not load the goal after tool execution; automatic continuation stopped", "工具执行后无法加载目标；已停止自动续写", "Ziel konnte nach der Tool-Ausführung nicht geladen werden; automatische Fortsetzung gestoppt",
		"ツール実行後に目標を読み込めませんでした。自動続行を停止しました", "도구 실행 후 목표를 불러올 수 없어 자동 계속을 중지했습니다", "Не удалось загрузить цель после выполнения инструмента; автоматическое продолжение остановлено")
	add(KeyRuntimeGoalBudgetReached,
		"Goal token budget reached (%d / %d); automatic continuation stopped", "已达到目标 Token budget（%d / %d）；已停止自动续写", "Token-Budget des Ziels erreicht (%d / %d); automatische Fortsetzung gestoppt",
		"目標の Token budget に到達しました（%d / %d）。自動続行を停止しました", "목표 Token budget에 도달했습니다(%d / %d). 자동 계속을 중지했습니다", "Достигнут бюджет токенов цели (%d / %d); автоматическое продолжение остановлено")
	add(KeyRuntimeGoalChangedStale,
		"The goal changed during evaluation; the stale result was ignored and automatic continuation stopped", "评估期间目标已变更；已忽略过期结果并停止自动续写", "Das Ziel wurde während der Auswertung geändert; das veraltete Ergebnis wurde ignoriert und die automatische Fortsetzung gestoppt",
		"評価中に目標が変更されました。古い結果を無視し、自動続行を停止しました", "평가 중 목표가 변경되어 오래된 결과를 무시하고 자동 계속을 중지했습니다", "Цель изменилась во время оценки; устаревший результат проигнорирован, автоматическое продолжение остановлено")
	add(KeyRuntimeGoalChangedDuringSave,
		"The goal changed while turn usage was being saved; automatic continuation stopped", "保存本轮用量时目标已变更；已停止自动续写", "Das Ziel wurde beim Speichern der Rundennutzung geändert; automatische Fortsetzung gestoppt",
		"ターン使用量の保存中に目標が変更されました。自動続行を停止しました", "턴 사용량을 저장하는 동안 목표가 변경되어 자동 계속을 중지했습니다", "Цель изменилась при сохранении использования хода; автоматическое продолжение остановлено")
	add(KeyRuntimeGoalUsageSaveFailed,
		"Could not save goal turn usage; automatic continuation stopped", "无法保存目标本轮用量；已停止自动续写", "Die Nutzung der Zielrunde konnte nicht gespeichert werden; automatische Fortsetzung gestoppt",
		"目標ターンの使用量を保存できませんでした。自動続行を停止しました", "목표 턴 사용량을 저장할 수 없어 자동 계속을 중지했습니다", "Не удалось сохранить использование хода цели; автоматическое продолжение остановлено")
	add(KeyRuntimeAutoCompactFailed,
		"Context compaction failed; continuing without auto-compact", "上下文压缩失败；将不使用自动压缩继续", "Kontextkomprimierung fehlgeschlagen; Fortsetzung ohne automatische Komprimierung",
		"コンテキスト圧縮に失敗しました。自動圧縮なしで続行します", "컨텍스트 압축 실패. 자동 압축 없이 계속합니다", "Не удалось сжать контекст; продолжение без автосжатия")
	add(KeyRuntimePostCompactCleanupFailed,
		"Post-compaction cleanup failed", "压缩后清理失败", "Bereinigung nach der Komprimierung fehlgeschlagen",
		"圧縮後のクリーンアップに失敗しました", "압축 후 정리에 실패했습니다", "Ошибка очистки после сжатия")
	add(KeyRuntimeCompactionCommitFailed,
		"Context compaction could not be committed; the original conversation was restored", "无法提交上下文压缩；已恢复原始会话",
		"Die Kontextkomprimierung konnte nicht übernommen werden; die ursprüngliche Unterhaltung wurde wiederhergestellt",
		"コンテキスト圧縮を確定できなかったため、元の会話を復元しました",
		"컨텍스트 압축을 커밋하지 못해 원래 대화를 복원했습니다",
		"Не удалось зафиксировать сжатие контекста; исходный диалог восстановлен")
	add(KeyRuntimeContextOverflowDrain,
		"Context overflow; drained %d staged context collapses and retrying", "上下文溢出；已清理 %d 个暂存的上下文折叠，正在重试", "Kontextüberlauf; %d vorgemerkte Kontextreduzierungen geleert, erneuter Versuch",
		"コンテキストがあふれました。保留中のコンテキスト折りたたみ %d 件を解放して再試行します", "컨텍스트 초과. 대기 중인 컨텍스트 축소 %d개를 비우고 다시 시도합니다", "Переполнение контекста; сброшено отложенных свёрток: %d, выполняется повтор")
	add(KeyRuntimeProviderRejectionRetry,
		"The Provider rejected the request; recovery completed and the request is being retried", "Provider 拒绝了请求；恢复完成后正在重试", "Der Provider hat die Anfrage abgelehnt; die Wiederherstellung ist abgeschlossen und die Anfrage wird erneut versucht",
		"Provider がリクエストを拒否しました。復旧後に再試行しています", "Provider가 요청을 거부했습니다. 복구를 완료하고 다시 시도합니다", "Provider отклонил запрос; восстановление завершено, выполняется повтор")
	add(KeyRuntimeReactiveCompact, "reactive compaction", "响应式压缩", "reaktive Komprimierung", "リアクティブ圧縮", "반응형 압축", "реактивное сжатие")
	add(KeyRuntimeMediaStrip, "media removal", "移除媒体", "Entfernen von Medien", "メディアの除去", "미디어 제거", "удаление медиа")
	add(KeyRuntimeToolInputJSONFailed,
		"Could not parse tool input JSON for %s", "无法解析工具 %s 的输入 JSON", "Eingabe-JSON für Tool %s konnte nicht geparst werden",
		"ツール %s の入力 JSON を解析できませんでした", "도구 %s의 입력 JSON을 파싱할 수 없습니다", "Не удалось разобрать входной JSON инструмента %s")
	add(KeyRuntimeToolInputJSONFlushFailed,
		"Could not parse buffered tool input JSON for %s", "无法解析工具 %s 的缓冲输入 JSON", "Gepuffertes Eingabe-JSON für Tool %s konnte nicht geparst werden",
		"ツール %s のバッファ済み入力 JSON を解析できませんでした", "도구 %s의 버퍼 입력 JSON을 파싱할 수 없습니다", "Не удалось разобрать буферизованный входной JSON инструмента %s")
	add(KeyRuntimeToolSkippedMalformed,
		"[tool %s skipped: malformed input JSON]", "[已跳过工具 %s：输入 JSON 格式错误]", "[Tool %s übersprungen: fehlerhaftes Eingabe-JSON]",
		"[ツール %s をスキップ: 入力 JSON の形式が不正]", "[도구 %s 건너뜀: 잘못된 입력 JSON]", "[инструмент %s пропущен: неверный входной JSON]")
	add(KeyRuntimeToolDisabled,
		"Tool %s is not enabled in the current runtime context.", "当前运行时上下文未启用工具 %s。", "Tool %s ist im aktuellen Laufzeitkontext nicht aktiviert.",
		"現在のランタイムコンテキストではツール %s が有効ではありません。", "현재 런타임 컨텍스트에서 도구 %s이(가) 활성화되지 않았습니다.", "Инструмент %s не включён в текущем контексте выполнения.")
	add(KeyRuntimeToolPlanDenied,
		"Cannot use %s in plan mode; exit plan mode first.", "Plan 模式下无法使用 %s；请先退出 Plan 模式。", "%s kann im Plan-Modus nicht verwendet werden; beende zuerst den Plan-Modus.",
		"Plan モードでは %s を使用できません。先に Plan モードを終了してください。", "Plan 모드에서는 %s을(를) 사용할 수 없습니다. 먼저 Plan 모드를 종료하세요.", "Нельзя использовать %s в режиме Plan; сначала выйдите из режима Plan.")
	add(KeyRuntimeToolRuleDenied,
		"Permission to use %s has been denied.", "使用 %s 的权限已被拒绝。", "Die Berechtigung zur Verwendung von %s wurde abgelehnt.",
		"%s を使用する権限が拒否されました。", "%s 사용 권한이 거부되었습니다.", "Разрешение на использование %s отклонено.")
	add(KeyRuntimeToolPermissionRequired,
		"Permission is required to use %s.", "使用 %s 需要获得权限。", "Für die Verwendung von %s ist eine Berechtigung erforderlich.",
		"%s を使用するには権限が必要です。", "%s을(를) 사용하려면 권한이 필요합니다.", "Для использования %s требуется разрешение.")
	add(KeyRuntimeResponsesStreamIncomplete,
		"The SSE stream ended without a response.completed event; the response may be incomplete.", "SSE stream 在未收到 response.completed 事件时结束；响应可能不完整。", "Der SSE-Stream endete ohne response.completed-Ereignis; die Antwort ist möglicherweise unvollständig.",
		"response.completed イベントがないまま SSE stream が終了しました。応答が不完全な可能性があります。", "response.completed 이벤트 없이 SSE stream이 종료되어 응답이 불완전할 수 있습니다.", "SSE stream завершился без события response.completed; ответ может быть неполным.")
	add(KeyRuntimePermissionActionExecute,
		"Run %s", "运行 %s", "%s ausführen", "%s を実行", "%s 실행", "Запустить %s")
	add(KeyRuntimePermissionRuleToolContract,
		"Tool permission contract: %s", "工具权限约定：%s", "Tool-Berechtigungsvertrag: %s", "ツール権限の規約: %s", "도구 권한 계약: %s", "Контракт разрешений инструмента: %s")
	add(KeyRuntimePermissionScopeInvocation,
		"Allow once: this invocation; always allow: the same tool with the exact input and target in this session", "仅允许一次：本次调用；始终允许：本会话中使用完全相同输入和目标的同一工具", "Einmal zulassen: dieser Aufruf; immer zulassen: dasselbe Tool mit exakt derselben Eingabe und demselben Ziel in dieser Sitzung", "今回のみ許可: この呼び出し。常に許可: このセッションで入力と対象が完全に同じ同一ツール", "한 번 허용: 이번 호출; 항상 허용: 이 세션에서 입력과 대상이 완전히 같은 동일 도구", "Разрешить один раз: этот вызов; разрешать всегда: тот же инструмент с теми же входными данными и целью в этом сеансе")
	add(KeyRuntimePermissionImpactDefault,
		"Run the requested tool with the supplied input", "使用所提供的输入运行请求的工具", "Das angeforderte Tool mit der angegebenen Eingabe ausführen", "指定された入力で要求されたツールを実行", "제공된 입력으로 요청한 도구 실행", "Запустить запрошенный инструмент с указанными входными данными")
	add(KeyRuntimePermissionRuleRequired,
		"Required tool policy", "工具强制策略", "Verbindliche Tool-Richtlinie", "ツールの必須ポリシー", "필수 도구 정책", "Обязательная политика инструмента")
	add(KeyRuntimePlanActionExecute,
		"Run the approved plan", "执行已批准的计划", "Den genehmigten Plan ausführen", "承認済みの計画を実行", "승인된 계획 실행", "Выполнить утверждённый план")
	add(KeyRuntimePlanImpactExecute,
		"Leave Plan mode and begin implementation", "退出 Plan 模式并开始实施", "Den Plan-Modus verlassen und mit der Umsetzung beginnen", "Plan モードを終了して実装を開始", "Plan 모드를 종료하고 구현 시작", "Выйти из режима Plan и начать реализацию")
	add(KeyRuntimePlanRiskExecute,
		"Implementation can modify the workspace and run commands", "实施过程可以修改工作区并运行命令", "Die Umsetzung kann den Arbeitsbereich ändern und Befehle ausführen", "実装ではワークスペースの変更やコマンドの実行が可能です", "구현 과정에서 작업 공간을 수정하고 명령을 실행할 수 있습니다", "Реализация может изменять рабочее пространство и выполнять команды")
	add(KeyRuntimePlanRuleGate,
		"Plan mode gate", "Plan 模式门禁", "Freigabe für den Plan-Modus", "Plan モードのゲート", "Plan 모드 게이트", "Контроль выхода из режима Plan")
	add(KeyRuntimePlanScopeTransition,
		"This transition out of Plan mode", "本次退出 Plan 模式", "Dieser Übergang aus dem Plan-Modus", "今回の Plan モード終了", "이번 Plan 모드 종료", "Этот выход из режима Plan")
	add(KeyRuntimePlanAllowedPrompts,
		"Allowed prompts:\n%s", "允许的 prompt：\n%s", "Zulässige Prompts:\n%s", "許可された prompt:\n%s", "허용된 prompt:\n%s", "Разрешённые prompt:\n%s")
	add(KeyRuntimePlanAutoModeFallback,
		"Auto-mode fallback: %s", "自动模式回退：%s", "Fallback im Automatikmodus: %s", "自動モードの fallback: %s", "자동 모드 fallback: %s", "Fallback автоматического режима: %s")
	add(KeyRuntimePermissionTargetInput,
		"Supplied input", "所提供的输入", "Angegebene Eingabe", "指定された入力", "제공된 입력", "Указанные входные данные")
	add(KeyRuntimeMissingToolResult,
		"Tool result missing after the model response was interrupted", "模型响应中断后缺少工具结果", "Tool-Ergebnis fehlt nach der Unterbrechung der Modellantwort", "モデル応答の中断後にツール結果がありません", "모델 응답이 중단된 후 도구 결과가 없습니다", "После прерывания ответа модели отсутствует результат инструмента")
}
