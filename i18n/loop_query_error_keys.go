package i18n

// Semantic copy for query-loop failures that can reach terminal, TUI, or
// screen-reader error surfaces. Provider/tool causes remain raw parameters;
// internal catalog diagnostics are retained only through errors.Is/errors.As.
const (
	KeyLoopQueryCompactionNotConfigured       Key = "loop.query.compaction_not_configured"
	KeyLoopQueryMessageHistoryLimitExceeded   Key = "loop.query.message_history_limit_exceeded"
	KeyLoopQueryForcedCompactionFailed        Key = "loop.query.forced_compaction_failed"
	KeyLoopQuerySnapshotSkillCatalogFailed    Key = "loop.query.snapshot_skill_catalog_failed"
	KeyLoopQueryPlanSkillCatalogFailed        Key = "loop.query.plan_skill_catalog_failed"
	KeyLoopQuerySkillCatalogContextChanged    Key = "loop.query.skill_catalog_context_changed"
	KeyLoopQueryValidateSkillReceiptFailed    Key = "loop.query.validate_skill_receipt_failed"
	KeyLoopQueryBindSkillGenerationFailed     Key = "loop.query.bind_skill_generation_failed"
	KeyLoopQueryValidateSkillGenerationFailed Key = "loop.query.validate_skill_generation_failed"
	KeyLoopQueryControlScopeInvalid           Key = "loop.query.control_scope_invalid"
	KeyLoopQueryAPICallRecoveryFailed         Key = "loop.query.api_call_recovery_failed"
	KeyLoopQueryAPICallFailed                 Key = "loop.query.api_call_failed"
	KeyLoopQueryStreamNotEstablished          Key = "loop.query.stream_not_established"
	KeyLoopQueryStreamMissingAfterAttempts    Key = "loop.query.stream_missing_after_attempts"
	KeyLoopQueryPinnedModelFallback           Key = "loop.query.pinned_model_fallback"
	KeyLoopQueryStreamFallbackFailed          Key = "loop.query.stream_fallback_failed"
	KeyLoopQueryStreamAfterModelFallback      Key = "loop.query.stream_after_model_fallback"
	KeyLoopQueryStreamAfterRetry              Key = "loop.query.stream_after_retry"
	KeyLoopQueryStreamAfterFallback           Key = "loop.query.stream_after_fallback"
	KeyLoopQueryStreamRecoveryFailed          Key = "loop.query.stream_recovery_failed"
	KeyLoopQueryStreamFailed                  Key = "loop.query.stream_failed"
	KeyLoopQueryEmptyResponse                 Key = "loop.query.empty_response"
	KeyLoopQueryEmptyResponseAfterRetry       Key = "loop.query.empty_response_after_retry"
	KeyLoopQueryToolExecutionFailed           Key = "loop.query.tool_execution_failed"
	KeyLoopQueryToolExecutionInterrupted      Key = "loop.query.tool_execution_interrupted"
	KeyLoopQueryHookCancelled                 Key = "loop.query.hook_cancelled"
	KeyLoopQueryHookFailedClosed              Key = "loop.query.hook_failed_closed"
	KeyLoopQueryHookRefused                   Key = "loop.query.hook_refused"
	KeyLoopVisibleOutputTokenRecovery         Key = "loop.visible.output_token_recovery"
	KeyLoopVisibleTokenBudgetContinuation     Key = "loop.visible.token_budget_continuation"
	KeyLoopVisibleGoalReasonDefault           Key = "loop.visible.goal_reason_default"
	KeyLoopVisibleGoalContinuation            Key = "loop.visible.goal_continuation"
	KeyLoopVisibleMCPInstructions             Key = "loop.visible.mcp_instructions"
	KeyLoopVisibleMCPDisconnected             Key = "loop.visible.mcp_disconnected"
	KeyLoopAttachmentCommandFailed            Key = "loop.attachment.command_failed"
	KeyLoopAttachmentMemoryPrefetchFailed     Key = "loop.attachment.memory_prefetch_failed"
	KeyLoopAttachmentSkillPrefetchFailed      Key = "loop.attachment.skill_prefetch_failed"
	KeyLoopAttachmentToolRefreshFailed        Key = "loop.attachment.tool_refresh_failed"
	KeyLoopToolIdentityInvalid                Key = "loop.tool_identity.invalid"
	KeyLoopToolIdentityDuplicate              Key = "loop.tool_identity.duplicate"
	KeyLoopToolIdentityReused                 Key = "loop.tool_identity.reused"
	KeyLoopToolIdentityAtIndex                Key = "loop.tool_identity.at_index"
	KeyLoopPartialStreamInterrupted           Key = "loop.stream.partial_interrupted"
	KeyLoopStreamClosedBeforeCommit           Key = "loop.stream.closed_before_commit"
	KeyLoopMaxTurnsExceeded                   Key = "loop.max_turns.exceeded"
)

var loopQueryErrorKeys = [...]Key{
	KeyLoopQueryCompactionNotConfigured,
	KeyLoopQueryMessageHistoryLimitExceeded,
	KeyLoopQueryForcedCompactionFailed,
	KeyLoopQuerySnapshotSkillCatalogFailed,
	KeyLoopQueryPlanSkillCatalogFailed,
	KeyLoopQuerySkillCatalogContextChanged,
	KeyLoopQueryValidateSkillReceiptFailed,
	KeyLoopQueryBindSkillGenerationFailed,
	KeyLoopQueryValidateSkillGenerationFailed,
	KeyLoopQueryControlScopeInvalid,
	KeyLoopQueryAPICallRecoveryFailed,
	KeyLoopQueryAPICallFailed,
	KeyLoopQueryStreamNotEstablished,
	KeyLoopQueryStreamMissingAfterAttempts,
	KeyLoopQueryPinnedModelFallback,
	KeyLoopQueryStreamFallbackFailed,
	KeyLoopQueryStreamAfterModelFallback,
	KeyLoopQueryStreamAfterRetry,
	KeyLoopQueryStreamAfterFallback,
	KeyLoopQueryStreamRecoveryFailed,
	KeyLoopQueryStreamFailed,
	KeyLoopQueryEmptyResponse,
	KeyLoopQueryEmptyResponseAfterRetry,
	KeyLoopQueryToolExecutionFailed,
	KeyLoopQueryToolExecutionInterrupted,
	KeyLoopQueryHookCancelled,
	KeyLoopQueryHookFailedClosed,
	KeyLoopQueryHookRefused,
	KeyLoopVisibleOutputTokenRecovery,
	KeyLoopVisibleTokenBudgetContinuation,
	KeyLoopVisibleGoalReasonDefault,
	KeyLoopVisibleGoalContinuation,
	KeyLoopVisibleMCPInstructions,
	KeyLoopVisibleMCPDisconnected,
	KeyLoopAttachmentCommandFailed,
	KeyLoopAttachmentMemoryPrefetchFailed,
	KeyLoopAttachmentSkillPrefetchFailed,
	KeyLoopAttachmentToolRefreshFailed,
	KeyLoopToolIdentityInvalid,
	KeyLoopToolIdentityDuplicate,
	KeyLoopToolIdentityReused,
	KeyLoopToolIdentityAtIndex,
	KeyLoopPartialStreamInterrupted,
	KeyLoopStreamClosedBeforeCommit,
	KeyLoopMaxTurnsExceeded,
}

func init() {
	entries := map[Key][6]string{
		KeyLoopQueryCompactionNotConfigured: {
			"Compaction is not configured; set MaxContextTokens above 0.",
			"尚未配置压缩；请将 MaxContextTokens 设置为大于 0。",
			"Die Komprimierung ist nicht konfiguriert; MaxContextTokens muss größer als 0 sein.",
			"圧縮が設定されていません。MaxContextTokens を 0 より大きい値に設定してください。",
			"압축이 구성되지 않았습니다. MaxContextTokens를 0보다 크게 설정하세요.",
			"Сжатие не настроено; задайте MaxContextTokens больше 0.",
		},
		KeyLoopQueryMessageHistoryLimitExceeded: {
			"The conversation has %d messages, above the safe limit of %d. The over-limit history was not sent or truncated; compact it before retrying.",
			"当前会话包含 %d 条消息，超过 %d 条的安全上限。超限历史未发送，也未截断；请先压缩会话再重试。",
			"Die Unterhaltung enthält %d Nachrichten und überschreitet damit das Sicherheitslimit von %d. Der überlange Verlauf wurde weder gesendet noch gekürzt; komprimieren Sie ihn vor dem nächsten Versuch.",
			"会話には %d 件のメッセージがあり、安全上限の %d 件を超えています。上限超過分の履歴は送信も切り詰めもされていません。圧縮してから再試行してください。",
			"대화에 메시지가 %d개 있어 안전 한도 %d개를 초과했습니다. 한도를 초과한 기록은 전송하거나 잘라내지 않았습니다. 대화를 압축한 뒤 다시 시도하세요.",
			"В диалоге %d сообщений — это выше безопасного предела %d. Превышающая предел история не была отправлена или усечена; сожмите диалог перед повторной попыткой.",
		},
		KeyLoopQueryForcedCompactionFailed: {
			"forced compaction failed: %v",
			"强制压缩失败：%v",
			"Die erzwungene Komprimierung ist fehlgeschlagen: %v",
			"強制圧縮に失敗しました: %v",
			"강제 압축에 실패했습니다: %v",
			"Не удалось выполнить принудительное сжатие: %v",
		},
		KeyLoopQuerySnapshotSkillCatalogFailed: {
			"The current Skill catalog could not be read.",
			"无法读取当前 Skill 目录。",
			"Der aktuelle Skill-Katalog konnte nicht gelesen werden.",
			"現在の Skill カタログを読み取れませんでした。",
			"현재 Skill 카탈로그를 읽지 못했습니다.",
			"Не удалось прочитать текущий каталог Skill.",
		},
		KeyLoopQueryPlanSkillCatalogFailed: {
			"The Skill catalog could not be prepared for this request.",
			"无法为本次请求准备 Skill 目录。",
			"Der Skill-Katalog konnte für diese Anfrage nicht vorbereitet werden.",
			"このリクエスト用の Skill カタログを準備できませんでした。",
			"이 요청의 Skill 카탈로그를 준비하지 못했습니다.",
			"Не удалось подготовить каталог Skill для этого запроса.",
		},
		KeyLoopQuerySkillCatalogContextChanged: {
			"The Skill catalog changed while the request was being prepared; retry the request.",
			"准备请求期间 Skill 目录发生了变化，请重试。",
			"Der Skill-Katalog wurde während der Vorbereitung geändert; bitte die Anfrage wiederholen.",
			"リクエストの準備中に Skill カタログが変更されました。もう一度お試しください。",
			"요청을 준비하는 동안 Skill 카탈로그가 변경되었습니다. 다시 시도하세요.",
			"Каталог Skill изменился во время подготовки запроса; повторите запрос.",
		},
		KeyLoopQueryValidateSkillReceiptFailed: {
			"The Skill execution receipt %s could not be validated.",
			"无法验证 Skill 执行回执 %s。",
			"Der Skill-Ausführungsbeleg %s konnte nicht validiert werden.",
			"Skill 実行レシート %s を検証できませんでした。",
			"Skill 실행 영수증 %s을(를) 검증하지 못했습니다.",
			"Не удалось проверить квитанцию выполнения Skill %s.",
		},
		KeyLoopQueryBindSkillGenerationFailed: {
			"The current Skill project revision could not be selected.",
			"无法选择当前 Skill 项目版本。",
			"Die aktuelle Skill-Projektrevision konnte nicht ausgewählt werden.",
			"現在の Skill プロジェクトリビジョンを選択できませんでした。",
			"현재 Skill 프로젝트 리비전을 선택하지 못했습니다.",
			"Не удалось выбрать текущую ревизию проекта Skill.",
		},
		KeyLoopQueryValidateSkillGenerationFailed: {
			"The selected Skill project revision is no longer available.",
			"所选 Skill 项目版本已不可用。",
			"Die ausgewählte Skill-Projektrevision ist nicht mehr verfügbar.",
			"選択した Skill プロジェクトリビジョンは利用できなくなりました。",
			"선택한 Skill 프로젝트 리비전을 더 이상 사용할 수 없습니다.",
			"Выбранная ревизия проекта Skill больше недоступна.",
		},
		KeyLoopQueryControlScopeInvalid: {
			"The conversation control state is stale; reload the session before retrying.",
			"会话控制状态已过期；请重新加载会话后再重试。",
			"Der Steuerungsstatus der Unterhaltung ist veraltet; laden Sie die Sitzung vor dem nächsten Versuch neu.",
			"会話の制御状態が古くなっています。セッションを再読み込みしてから再試行してください。",
			"대화 제어 상태가 오래되었습니다. 세션을 다시 불러온 뒤 재시도하세요.",
			"Состояние управления диалогом устарело; перезагрузите сеанс перед повторной попыткой.",
		},
		KeyLoopQueryAPICallRecoveryFailed: {
			"API call failed: %[2]v; recovery failed: %[1]v",
			"API 调用失败：%[2]v；恢复也失败：%[1]v",
			"API-Aufruf fehlgeschlagen: %[2]v; Wiederherstellung fehlgeschlagen: %[1]v",
			"API 呼び出しに失敗しました: %[2]v。復旧にも失敗しました: %[1]v",
			"API 호출에 실패했습니다: %[2]v. 복구도 실패했습니다: %[1]v",
			"Вызов API завершился ошибкой: %[2]v; восстановление также не удалось: %[1]v",
		},
		KeyLoopQueryAPICallFailed: {
			"API call failed: %v", "API 调用失败：%v", "API-Aufruf fehlgeschlagen: %v",
			"API 呼び出しに失敗しました: %v", "API 호출에 실패했습니다: %v", "Вызов API завершился ошибкой: %v",
		},
		KeyLoopQueryStreamNotEstablished: {
			"The response stream could not be established.", "无法建立响应 stream。", "Der Antwort-Stream konnte nicht aufgebaut werden.",
			"レスポンス stream を確立できませんでした。", "응답 stream을 연결하지 못했습니다.", "Не удалось установить поток ответа.",
		},
		KeyLoopQueryStreamMissingAfterAttempts: {
			"API call failed: no response stream after %d attempts", "API 调用失败：尝试 %d 次后仍未建立响应 stream", "API-Aufruf fehlgeschlagen: Nach %d Versuchen wurde kein Antwort-Stream aufgebaut",
			"API 呼び出しに失敗しました: %d 回試行してもレスポンス stream を確立できませんでした", "API 호출에 실패했습니다. %d번 시도한 후에도 응답 stream을 연결하지 못했습니다", "Вызов API завершился ошибкой: поток ответа не установлен после %d попыток",
		},
		KeyLoopQueryPinnedModelFallback: {
			"The model contract pinned %s; the provider requested fallback to %s.", "模型契约已固定为 %s；provider 请求 fallback 到 %s。", "Der Modellvertrag hat %s festgelegt; der Provider hat einen Fallback auf %s angefordert.",
			"モデル契約では %s に固定されていますが、provider が %s への fallback を要求しました。", "모델 계약은 %s(으)로 고정되었지만 provider가 %s(으)로 fallback을 요청했습니다.", "Контракт модели закрепляет %s, но провайдер запросил переключение на %s.",
		},
		KeyLoopQueryStreamFallbackFailed: {
			"Stream processing failed; fallback also failed: %v", "处理 stream 失败，fallback 也失败：%v", "Stream-Verarbeitung fehlgeschlagen; auch der Fallback ist fehlgeschlagen: %v",
			"stream の処理に失敗し、fallback にも失敗しました: %v", "stream 처리에 실패했고 fallback도 실패했습니다: %v", "Не удалось обработать поток; fallback также завершился ошибкой: %v",
		},
		KeyLoopQueryStreamAfterModelFallback: {
			"Stream processing failed after model fallback: %v", "切换 fallback model 后处理 stream 仍失败：%v", "Stream-Verarbeitung nach dem Modell-Fallback fehlgeschlagen: %v",
			"fallback model への切り替え後も stream の処理に失敗しました: %v", "fallback model로 전환한 후에도 stream 처리에 실패했습니다: %v", "Обработка потока после переключения на fallback model завершилась ошибкой: %v",
		},
		KeyLoopQueryStreamAfterRetry: {
			"Stream processing failed after retry: %v", "重试后处理 stream 仍失败：%v", "Stream-Verarbeitung nach dem erneuten Versuch fehlgeschlagen: %v",
			"再試行後も stream の処理に失敗しました: %v", "재시도 후에도 stream 처리에 실패했습니다: %v", "Обработка потока после повторной попытки завершилась ошибкой: %v",
		},
		KeyLoopQueryStreamAfterFallback: {
			"Stream processing failed after fallback: %v", "fallback 后处理 stream 仍失败：%v", "Stream-Verarbeitung nach dem Fallback fehlgeschlagen: %v",
			"fallback 後も stream の処理に失敗しました: %v", "fallback 후에도 stream 처리에 실패했습니다: %v", "Обработка потока после fallback завершилась ошибкой: %v",
		},
		KeyLoopQueryStreamRecoveryFailed: {
			"Stream processing failed: %[2]v; recovery failed: %[1]v", "处理 stream 失败：%[2]v；恢复也失败：%[1]v", "Stream-Verarbeitung fehlgeschlagen: %[2]v; Wiederherstellung fehlgeschlagen: %[1]v",
			"stream の処理に失敗しました: %[2]v。復旧にも失敗しました: %[1]v", "stream 처리에 실패했습니다: %[2]v. 복구도 실패했습니다: %[1]v", "Обработка потока завершилась ошибкой: %[2]v; восстановление также не удалось: %[1]v",
		},
		KeyLoopQueryStreamFailed: {
			"Stream processing failed: %v", "处理 stream 失败：%v", "Stream-Verarbeitung fehlgeschlagen: %v",
			"stream の処理に失敗しました: %v", "stream 처리에 실패했습니다: %v", "Обработка потока завершилась ошибкой: %v",
		},
		KeyLoopQueryEmptyResponse: {
			"The assistant returned an empty response.", "助手返回了空响应。", "Der Assistent hat eine leere Antwort zurückgegeben.",
			"アシスタントから空のレスポンスが返されました。", "어시스턴트가 빈 응답을 반환했습니다.", "Ассистент вернул пустой ответ.",
		},
		KeyLoopQueryEmptyResponseAfterRetry: {
			"The assistant returned an empty response after retrying.", "重试后助手仍返回空响应。", "Der Assistent hat auch nach dem erneuten Versuch eine leere Antwort zurückgegeben.",
			"再試行後もアシスタントから空のレスポンスが返されました。", "재시도 후에도 어시스턴트가 빈 응답을 반환했습니다.", "После повторной попытки ассистент снова вернул пустой ответ.",
		},
		KeyLoopQueryToolExecutionFailed: {
			"Tool execution failed: %v", "Tool 执行失败：%v", "Tool-Ausführung fehlgeschlagen: %v",
			"Tool の実行に失敗しました: %v", "Tool 실행에 실패했습니다: %v", "Не удалось выполнить Tool: %v",
		},
		KeyLoopQueryToolExecutionInterrupted: {
			"Tool execution was interrupted: %v", "Tool 执行已中断：%v", "Die Tool-Ausführung wurde unterbrochen: %v",
			"Tool の実行が中断されました: %v", "Tool 실행이 중단되었습니다: %v", "Выполнение Tool было прервано: %v",
		},
		KeyLoopQueryHookCancelled: {
			"%s query hook cancelled: %v", "%s query hook 已取消：%v", "%s-Query-Hook wurde abgebrochen: %v",
			"%s query hook がキャンセルされました: %v", "%s query hook이 취소되었습니다: %v", "Query hook %s отменён: %v",
		},
		KeyLoopQueryHookFailedClosed: {
			"%s query hook failed closed (%s): %s", "%s query hook 已按 fail-closed 策略阻止请求（%s）：%s", "%s-Query-Hook hat die Anfrage nach dem Fail-Closed-Prinzip blockiert (%s): %s",
			"%s query hook が fail-closed としてリクエストを拒否しました（%s）: %s", "%s query hook이 fail-closed 방식으로 요청을 차단했습니다(%s): %s", "Query hook %s заблокировал запрос по принципу fail-closed (%s): %s",
		},
		KeyLoopQueryHookRefused: {
			"hook refused model I/O", "hook 拒绝了 model I/O", "Hook hat die Modell-I/O verweigert",
			"hook が model I/O を拒否しました", "hook이 model I/O를 거부했습니다", "Hook отклонил ввод-вывод модели",
		},
		KeyLoopVisibleOutputTokenRecovery: {
			"Output token limit hit. Resume directly - no apology, no recap of what you were doing. Pick up mid-thought if that is where the cut happened. Break remaining work into smaller pieces.",
			"已达到输出 token 上限。请直接继续，不要道歉，也不要复述之前的工作；如果内容在思路中间被截断，就从那里接着写，并把剩余工作拆成更小的部分。",
			"Das Ausgabetoken-Limit wurde erreicht. Fahre direkt fort – ohne Entschuldigung und ohne Zusammenfassung der bisherigen Arbeit. Setze mitten im Gedanken an, falls dort abgeschnitten wurde, und teile die verbleibende Arbeit in kleinere Schritte auf.",
			"出力 token の上限に達しました。謝罪や作業内容の要約はせず、そのまま続けてください。考えの途中で切れた場合はそこから再開し、残りの作業を小さく分けてください。",
			"출력 token 한도에 도달했습니다. 사과하거나 지금까지의 작업을 요약하지 말고 바로 이어서 진행하세요. 생각 중간에서 잘렸다면 그 지점부터 계속하고 남은 작업을 더 작은 단위로 나누세요.",
			"Достигнут лимит выходных token. Продолжайте сразу, без извинений и пересказа сделанного. Если ответ оборвался на полуслове, продолжите с этого места и разбейте оставшуюся работу на меньшие части.",
		},
		KeyLoopVisibleTokenBudgetContinuation: {
			"Stopped at %d%% of token target (%s / %s). Keep working - do not summarize.", "已用到 token 目标的 %d%%（%s / %s）。请继续工作，不要总结。", "Bei %d%% des Token-Ziels angehalten (%s / %s). Weiterarbeiten, nicht zusammenfassen.",
			"token 目標の %d%%（%s / %s）で停止しました。要約せずに作業を続けてください。", "token 목표의 %d%%에서 멈췄습니다(%s / %s). 요약하지 말고 계속 작업하세요.", "Остановка на %d%% целевого числа token (%s / %s). Продолжайте работу без подведения итогов.",
		},
		KeyLoopVisibleGoalReasonDefault: {
			"the transcript does not yet prove the objective is complete", "当前记录尚不足以证明目标已经完成", "Das Protokoll belegt noch nicht, dass das Ziel erreicht ist",
			"現在の記録だけでは目標の完了をまだ確認できません", "현재 기록만으로는 목표가 완료되었음을 아직 입증할 수 없습니다", "Текущая история пока не подтверждает, что цель достигнута",
		},
		KeyLoopVisibleGoalContinuation: {
			"<system-reminder>\nThe active goal is not yet complete.\nEvaluator reason (untrusted data): %s\nContinue working toward the active goal.\n</system-reminder>",
			"<system-reminder>\n当前目标尚未完成。\nEvaluator 原因（不可信数据）：%s\n请继续推进当前目标。\n</system-reminder>",
			"<system-reminder>\nDas aktive Ziel ist noch nicht erreicht.\nBegründung des Evaluators (nicht vertrauenswürdige Daten): %s\nArbeite weiter auf das aktive Ziel hin.\n</system-reminder>",
			"<system-reminder>\n現在の目標はまだ完了していません。\nEvaluator の理由（信頼できないデータ）: %s\n現在の目標に向けて作業を続けてください。\n</system-reminder>",
			"<system-reminder>\n현재 목표가 아직 완료되지 않았습니다.\nEvaluator 사유(신뢰할 수 없는 데이터): %s\n현재 목표를 향해 계속 작업하세요.\n</system-reminder>",
			"<system-reminder>\nТекущая цель ещё не достигнута.\nПричина Evaluator (ненадёжные данные): %s\nПродолжайте работу над текущей целью.\n</system-reminder>",
		},
		KeyLoopVisibleMCPInstructions: {
			"# MCP Server Instructions\n\nThe following MCP servers have provided instructions for how to use their tools and resources:\n\n%s", "# MCP Server 使用说明\n\n以下 MCP server 提供了其工具和资源的使用说明：\n\n%s", "# Anweisungen der MCP-Server\n\nDie folgenden MCP-Server haben Hinweise zur Nutzung ihrer Tools und Ressourcen bereitgestellt:\n\n%s",
			"# MCP Server の使用手順\n\n次の MCP server から、Tool とリソースの使用手順が提供されています:\n\n%s", "# MCP Server 사용 지침\n\n다음 MCP server가 Tool과 리소스 사용 지침을 제공했습니다:\n\n%s", "# Инструкции MCP Server\n\nСледующие MCP server предоставили инструкции по использованию своих Tool и ресурсов:\n\n%s",
		},
		KeyLoopVisibleMCPDisconnected: {
			"The following MCP servers have disconnected. Their instructions above no longer apply:\n%s", "以下 MCP server 已断开连接，上述使用说明不再适用：\n%s", "Die folgenden MCP-Server wurden getrennt. Ihre obigen Anweisungen gelten nicht mehr:\n%s",
			"次の MCP server は切断されました。上記の手順は適用されなくなりました:\n%s", "다음 MCP server의 연결이 끊어졌습니다. 위 지침은 더 이상 적용되지 않습니다:\n%s", "Следующие MCP server отключились. Приведённые выше инструкции больше не действуют:\n%s",
		},
		KeyLoopAttachmentCommandFailed: {
			"A queued command attachment could not be prepared.", "无法准备排队命令的附件。", "Ein Anhang für einen eingereihten Befehl konnte nicht vorbereitet werden.",
			"キュー内のコマンド添付情報を準備できませんでした。", "대기 중인 명령 첨부 정보를 준비하지 못했습니다.", "Не удалось подготовить вложение команды из очереди.",
		},
		KeyLoopAttachmentMemoryPrefetchFailed: {
			"Prefetched memory could not be attached.", "无法附加预取的 memory。", "Der vorab geladene Speicher konnte nicht angehängt werden.",
			"事前取得した memory を添付できませんでした。", "미리 가져온 memory를 첨부하지 못했습니다.", "Не удалось прикрепить предварительно загруженную memory.",
		},
		KeyLoopAttachmentSkillPrefetchFailed: {
			"Prefetched Skill content could not be attached.", "无法附加预取的 Skill 内容。", "Der vorab geladene Skill-Inhalt konnte nicht angehängt werden.",
			"事前取得した Skill の内容を添付できませんでした。", "미리 가져온 Skill 콘텐츠를 첨부하지 못했습니다.", "Не удалось прикрепить предварительно загруженное содержимое Skill.",
		},
		KeyLoopAttachmentToolRefreshFailed: {
			"The available tools could not be refreshed.", "无法刷新可用 Tool。", "Die verfügbaren Tools konnten nicht aktualisiert werden.",
			"利用可能な Tool を更新できませんでした。", "사용 가능한 Tool을 새로 고치지 못했습니다.", "Не удалось обновить доступные Tool.",
		},
		KeyLoopToolIdentityInvalid: {
			"invalid tool use identity", "tool use identity 无效", "Ungültige Tool-Use-Identität",
			"tool use identity が無効です", "tool use identity가 잘못되었습니다", "Недопустимая идентичность tool use",
		},
		KeyLoopToolIdentityDuplicate: {
			"invalid tool use identity: %s %q at index %d (first index %d)", "tool use identity 无效：%s %q 位于索引 %d（首次出现于索引 %d）", "Ungültige Tool-Use-Identität: %s %q an Index %d (zuerst an Index %d)",
			"tool use identity が無効です: %s %q（index %d、最初の index は %d）", "tool use identity가 잘못되었습니다: %s %q, index %d(첫 index %d)", "Недопустимая идентичность tool use: %s %q по индексу %d (первый индекс %d)",
		},
		KeyLoopToolIdentityReused: {
			"invalid tool use identity: %s %q at index %d (already used in this session)", "tool use identity 无效：%s %q 位于索引 %d（本会话中已使用）", "Ungültige Tool-Use-Identität: %s %q an Index %d (in dieser Sitzung bereits verwendet)",
			"tool use identity が無効です: %s %q（index %d、このセッションですでに使用済み）", "tool use identity가 잘못되었습니다: %s %q, index %d(이 세션에서 이미 사용됨)", "Недопустимая идентичность tool use: %s %q по индексу %d (уже использована в этом сеансе)",
		},
		KeyLoopToolIdentityAtIndex: {
			"invalid tool use identity: %s at index %d for %s", "%[3]s 在索引 %[2]d 处的 tool use identity 无效：%[1]s", "Ungültige Tool-Use-Identität: %s an Index %d für %s",
			"%[3]s の index %[2]d にある tool use identity が無効です: %[1]s", "%[3]s의 index %[2]d에 있는 tool use identity가 잘못되었습니다: %[1]s", "Недопустимая идентичность tool use: %s по индексу %d для %s",
		},
		KeyLoopPartialStreamInterrupted: {
			"stream interrupted with %d uncommitted block(s): %v", "stream 中断，%d 个未提交内容块已作废：%v", "Stream mit %d nicht bestätigten Block/Blöcken unterbrochen: %v",
			"stream が中断され、%d 個の未コミット block が破棄されました: %v", "stream이 중단되어 커밋되지 않은 block %d개가 폐기되었습니다: %v", "Поток прерван; неподтверждённых блоков: %d: %v",
		},
		KeyLoopStreamClosedBeforeCommit: {
			"provider stream closed before the response commit event", "provider stream 在响应提交事件前关闭", "Provider-Stream wurde vor dem Commit-Ereignis der Antwort geschlossen",
			"provider stream が応答のコミットイベント前に閉じられました", "provider stream이 응답 커밋 이벤트 전에 닫혔습니다", "Поток провайдера закрылся до события подтверждения ответа",
		},
		KeyLoopMaxTurnsExceeded: {
			"max turns (%d) exceeded", "已超过最大轮次（%d）", "Maximale Anzahl an Runden (%d) überschritten",
			"最大ターン数（%d）を超えました", "최대 턴 수(%d)를 초과했습니다", "Превышено максимальное число ходов (%d)",
		},
	}

	for key, values := range entries {
		semanticTranslations[key] = map[Language]string{
			LangEN: values[0], LangZH: values[1], LangDE: values[2],
			LangJA: values[3], LangKO: values[4], LangRU: values[5],
		}
	}
}
