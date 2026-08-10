package i18n

const (
	KeyCompactSummaryHeading               Key = "compact.summary.heading"
	KeyCompactContinuationPreamble         Key = "compact.summary.continuation_preamble"
	KeyCompactTranscriptRecovery           Key = "compact.summary.transcript_recovery"
	KeyCompactTranscriptUnavailable        Key = "compact.summary.transcript_unavailable"
	KeyCompactRecentMessagesPreserved      Key = "compact.summary.recent_messages_preserved"
	KeyCompactResponseStyleBoundary        Key = "compact.summary.response_style_boundary"
	KeyCompactContinueDirective            Key = "compact.summary.continue_directive"
	KeyCompactPartialLaterPreamble         Key = "compact.summary.partial_later_preamble"
	KeyCompactPartialTranscriptRecovery    Key = "compact.summary.partial_transcript_recovery"
	KeyCompactPartialTranscriptUnavailable Key = "compact.summary.partial_transcript_unavailable"
	KeyCompactEarlierMessagesPreserved     Key = "compact.summary.earlier_messages_preserved"

	KeyCompactAttachmentPlanTitle            Key = "compact.attachment.plan.title"
	KeyCompactAttachmentPlanFile             Key = "compact.attachment.plan.file"
	KeyCompactAttachmentPlanModeTitle        Key = "compact.attachment.plan_mode.title"
	KeyCompactAttachmentPlanModeBody         Key = "compact.attachment.plan_mode.body"
	KeyCompactAttachmentSkillsTitle          Key = "compact.attachment.skills.title"
	KeyCompactAttachmentBackgroundTitle      Key = "compact.attachment.background.title"
	KeyCompactAttachmentUnknownStatus        Key = "compact.attachment.background.unknown_status"
	KeyCompactAttachmentTypeLabel            Key = "compact.attachment.background.type"
	KeyCompactAttachmentErrorLabel           Key = "compact.attachment.background.error"
	KeyCompactAttachmentDeferredTitle        Key = "compact.attachment.deferred.title"
	KeyCompactAttachmentLoadedTools          Key = "compact.attachment.deferred.loaded"
	KeyCompactAttachmentDeferredPool         Key = "compact.attachment.deferred.pool"
	KeyCompactAttachmentAgentTitle           Key = "compact.attachment.agent.title"
	KeyCompactAttachmentMCPTitle             Key = "compact.attachment.mcp.title"
	KeyCompactAttachmentMCPToolsLabel        Key = "compact.attachment.mcp.tools"
	KeyCompactAttachmentMCPInstructionsLabel Key = "compact.attachment.mcp.instructions"
)

func init() {
	compactCopy := func(en, zh, de, ja, ko, ru string) map[Language]string {
		return map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}
	for key, translations := range map[Key]map[Language]string{
		KeyCompactSummaryHeading: compactCopy(
			"Summary:", "摘要：", "Zusammenfassung:", "要約：", "요약:", "Сводка:",
		),
		KeyCompactContinuationPreamble: compactCopy(
			"Earlier conversation content was compacted into the summary below.",
			"较早的对话内容已压缩为以下摘要。",
			"Frühere Gesprächsinhalte wurden in der folgenden Zusammenfassung komprimiert.",
			"以前の会話内容は、以下の要約に圧縮されています。",
			"이전 대화 내용은 아래 요약으로 압축되었습니다.",
			"Предыдущая часть диалога сжата в приведённую ниже сводку.",
		),
		KeyCompactTranscriptRecovery: compactCopy(
			"If you need specific details from before compaction (like exact code snippets, error messages, or content you generated), read the full transcript at: %s",
			"如需查看压缩前的具体细节（例如完整代码片段、错误信息或已生成的内容），请读取完整会话记录：%s",
			"Wenn du genaue Details aus der Zeit vor der Komprimierung brauchst (etwa exakte Codeausschnitte, Fehlermeldungen oder erzeugte Inhalte), lies das vollständige Transkript unter: %s",
			"圧縮前の具体的な情報（正確なコード断片、エラーメッセージ、生成した内容など）が必要な場合は、次の完全なトランスクリプトを参照してください: %s",
			"압축 전의 구체적인 정보(정확한 코드 조각, 오류 메시지, 생성한 내용 등)가 필요하면 다음 전체 기록을 읽으세요: %s",
			"Если нужны точные сведения до сжатия (например, фрагменты кода, сообщения об ошибках или созданный материал), прочитайте полную расшифровку: %s",
		),
		KeyCompactTranscriptUnavailable: compactCopy(
			"Transcript reference: unavailable in this runtime; rely on this summary plus preserved recent messages for recovery details.",
			"会话记录引用：当前运行时不可用；如需恢复细节，请依据本摘要和保留的近期消息。",
			"Transkriptverweis: in dieser Laufzeit nicht verfügbar; nutze für Wiederherstellungsdetails diese Zusammenfassung und die beibehaltenen letzten Nachrichten.",
			"トランスクリプト参照: このランタイムでは利用できません。復元に必要な詳細は、この要約と保持された直近のメッセージを参照してください。",
			"기록 참조: 이 런타임에서는 사용할 수 없습니다. 복구 세부 정보는 이 요약과 보존된 최근 메시지를 참고하세요.",
			"Ссылка на расшифровку недоступна в этой среде; для восстановления деталей используйте эту сводку и сохранённые последние сообщения.",
		),
		KeyCompactRecentMessagesPreserved: compactCopy(
			"Recent messages are preserved verbatim.", "近期消息已原样保留。", "Die letzten Nachrichten wurden unverändert beibehalten.", "直近のメッセージは原文のまま保持されています。", "최근 메시지는 원문 그대로 보존되었습니다.", "Последние сообщения сохранены дословно.",
		),
		KeyCompactResponseStyleBoundary: compactCopy(
			"This summary is memory, not a response-style instruction. Always follow the latest ordinary user message's requested length, format, and level of detail; keep the answer concise when requested.",
			"本摘要仅用于保留记忆，不是回答风格指令。始终遵循最新一条普通用户消息要求的篇幅、格式和详细程度；用户要求简洁时，请保持简洁。",
			"Diese Zusammenfassung dient als Gedächtnis und ist keine Vorgabe für den Antwortstil. Befolge stets die in der neuesten regulären Benutzernachricht gewünschte Länge, Form und Detailtiefe; antworte knapp, wenn dies verlangt wird.",
			"この要約は記憶の保持を目的としたもので、回答スタイルの指示ではありません。通常のユーザーメッセージのうち最新のものが指定する長さ、形式、詳細度に常に従い、簡潔さを求められた場合は簡潔に答えてください。",
			"이 요약은 기억 보존용이며 답변 스타일 지시가 아닙니다. 항상 가장 최근의 일반 사용자 메시지가 요청한 길이, 형식, 상세 수준을 따르고, 간결한 답변을 요청받으면 간결하게 답하세요.",
			"Эта сводка служит для сохранения контекста, а не задаёт стиль ответа. Всегда соблюдайте требования последнего обычного сообщения пользователя к объёму, формату и детализации; если просят ответить кратко, отвечайте кратко.",
		),
		KeyCompactContinueDirective: compactCopy(
			"Continue the conversation from where it left off without asking the user any further questions. Resume directly — do not acknowledge the summary, do not recap what was happening, do not preface with \"I'll continue\" or similar. Pick up the last task as if the break never happened.",
			"从中断处直接继续，不要再向用户提问。不要提及本摘要，不要回顾此前内容，也不要用“我将继续”等类似措辞开场；像从未中断过一样接着处理最后一项任务。",
			"Setze die Unterhaltung ohne weitere Rückfragen an den Benutzer direkt an der unterbrochenen Stelle fort. Erwähne die Zusammenfassung nicht, wiederhole den bisherigen Verlauf nicht und beginne nicht mit „Ich mache weiter“ oder Ähnlichem. Fahre mit der letzten Aufgabe fort, als hätte es keine Unterbrechung gegeben.",
			"ユーザーに追加の質問をせず、中断した箇所からそのまま会話を続けてください。この要約には触れず、それまでの経緯を振り返らず、「続けます」などの前置きも不要です。中断がなかったものとして最後のタスクを再開してください。",
			"사용자에게 추가 질문을 하지 말고 중단된 지점에서 바로 대화를 이어가세요. 이 요약을 언급하거나 이전 상황을 되풀이하지 말고, \"계속하겠습니다\" 같은 말로 시작하지도 마세요. 중단이 없었던 것처럼 마지막 작업을 계속하세요.",
			"Продолжите диалог прямо с места остановки, не задавая пользователю дополнительных вопросов. Не упоминайте эту сводку, не пересказывайте произошедшее и не начинайте со слов «Я продолжу» или подобных. Вернитесь к последней задаче так, будто перерыва не было.",
		),
		KeyCompactPartialLaterPreamble: compactCopy(
			"This session includes an earlier portion of the conversation preserved verbatim. The summary below covers the later portion that was compacted.",
			"本会话原样保留了对话的较早部分。以下摘要涵盖随后被压缩的部分。",
			"Diese Sitzung enthält einen unverändert beibehaltenen früheren Teil der Unterhaltung. Die folgende Zusammenfassung deckt den später komprimierten Teil ab.",
			"このセッションでは、会話の前半部分が原文のまま保持されています。以下の要約は、圧縮された後半部分をまとめたものです。",
			"이 세션에는 대화의 앞부분이 원문 그대로 보존되어 있습니다. 아래 요약은 압축된 뒷부분을 다룹니다.",
			"В этом сеансе более ранняя часть диалога сохранена дословно. Сводка ниже охватывает более позднюю часть, подвергнутую сжатию.",
		),
		KeyCompactPartialTranscriptRecovery: compactCopy(
			"If you need specific details from the compacted portion (like exact code snippets, error messages, or content you generated), read the full transcript at: %s",
			"如需查看被压缩部分的具体细节（例如完整代码片段、错误信息或已生成的内容），请读取完整会话记录：%s",
			"Wenn du genaue Details aus dem komprimierten Teil brauchst (etwa exakte Codeausschnitte, Fehlermeldungen oder erzeugte Inhalte), lies das vollständige Transkript unter: %s",
			"圧縮された部分の具体的な情報（正確なコード断片、エラーメッセージ、生成した内容など）が必要な場合は、次の完全なトランスクリプトを参照してください: %s",
			"압축된 부분의 구체적인 정보(정확한 코드 조각, 오류 메시지, 생성한 내용 등)가 필요하면 다음 전체 기록을 읽으세요: %s",
			"Если нужны точные сведения из сжатой части (например, фрагменты кода, сообщения об ошибках или созданный материал), прочитайте полную расшифровку: %s",
		),
		KeyCompactPartialTranscriptUnavailable: compactCopy(
			"Transcript reference: unavailable in this runtime; rely on this summary plus preserved messages for recovery details.",
			"会话记录引用：当前运行时不可用；如需恢复细节，请依据本摘要和保留的消息。",
			"Transkriptverweis: in dieser Laufzeit nicht verfügbar; nutze für Wiederherstellungsdetails diese Zusammenfassung und die beibehaltenen Nachrichten.",
			"トランスクリプト参照: このランタイムでは利用できません。復元に必要な詳細は、この要約と保持されたメッセージを参照してください。",
			"기록 참조: 이 런타임에서는 사용할 수 없습니다. 복구 세부 정보는 이 요약과 보존된 메시지를 참고하세요.",
			"Ссылка на расшифровку недоступна в этой среде; для восстановления деталей используйте эту сводку и сохранённые сообщения.",
		),
		KeyCompactEarlierMessagesPreserved: compactCopy(
			"Earlier messages are preserved verbatim.", "较早的消息已原样保留。", "Die früheren Nachrichten wurden unverändert beibehalten.", "前半のメッセージは原文のまま保持されています。", "이전 메시지는 원문 그대로 보존되었습니다.", "Более ранние сообщения сохранены дословно.",
		),
		KeyCompactAttachmentPlanTitle: compactCopy(
			"Post-compaction plan state", "压缩后的计划状态", "Planstatus nach der Komprimierung", "圧縮後の計画状態", "압축 후 계획 상태", "Состояние плана после сжатия",
		),
		KeyCompactAttachmentPlanFile: compactCopy(
			"Active plan file: %s", "当前计划文件：%s", "Aktive Plandatei: %s", "現在の計画ファイル: %s", "현재 계획 파일: %s", "Активный файл плана: %s",
		),
		KeyCompactAttachmentPlanModeTitle: compactCopy(
			"Post-compaction plan mode reminder", "压缩后的 Plan mode 提醒", "Erinnerung an den Planmodus nach der Komprimierung", "圧縮後の Plan mode リマインダー", "압축 후 Plan mode 알림", "Напоминание о режиме планирования после сжатия",
		),
		KeyCompactAttachmentPlanModeBody: compactCopy(
			"Plan mode is still active. Continue read-only planning and do not modify files until plan mode is exited.",
			"Plan mode 仍处于启用状态。请继续进行只读规划，在退出 Plan mode 前不要修改文件。",
			"Der Planmodus ist weiterhin aktiv. Setze die schreibgeschützte Planung fort und ändere keine Dateien, bis der Planmodus beendet wurde.",
			"Plan mode は引き続き有効です。読み取り専用で計画を続け、Plan mode を終了するまでファイルを変更しないでください。",
			"Plan mode가 아직 활성화되어 있습니다. 읽기 전용 계획을 계속하고 Plan mode를 종료할 때까지 파일을 수정하지 마세요.",
			"Режим планирования всё ещё активен. Продолжайте планирование только для чтения и не изменяйте файлы до выхода из этого режима.",
		),
		KeyCompactAttachmentSkillsTitle: compactCopy(
			"Post-compaction invoked skills", "压缩后已调用的 Skill", "Nach der Komprimierung aufgerufene Skills", "圧縮後に呼び出された Skill", "압축 후 호출된 Skill", "Вызванные после сжатия Skill",
		),
		KeyCompactAttachmentBackgroundTitle: compactCopy(
			"Post-compaction background tasks", "压缩后的后台任务", "Hintergrundaufgaben nach der Komprimierung", "圧縮後のバックグラウンドタスク", "압축 후 백그라운드 작업", "Фоновые задачи после сжатия",
		),
		KeyCompactAttachmentUnknownStatus: compactCopy(
			"unknown", "未知", "unbekannt", "不明", "알 수 없음", "неизвестно",
		),
		KeyCompactAttachmentTypeLabel: compactCopy(
			" type=%s", " 类型=%s", " Typ=%s", " 種類=%s", " 유형=%s", " тип=%s",
		),
		KeyCompactAttachmentErrorLabel: compactCopy(
			" error=%s", " 错误=%s", " Fehler=%s", " エラー=%s", " 오류=%s", " ошибка=%s",
		),
		KeyCompactAttachmentDeferredTitle: compactCopy(
			"Post-compaction deferred tools", "压缩后的延迟加载工具", "Verzögert geladene Tools nach der Komprimierung", "圧縮後の遅延読み込みツール", "압축 후 지연 로드 도구", "Инструменты отложенной загрузки после сжатия",
		),
		KeyCompactAttachmentLoadedTools: compactCopy(
			"Loaded deferred tools: %s", "已加载的延迟工具：%s", "Geladene verzögerte Tools: %s", "読み込み済みの遅延ツール: %s", "로드된 지연 도구: %s", "Загруженные отложенные инструменты: %s",
		),
		KeyCompactAttachmentDeferredPool: compactCopy(
			"Deferred tool pool: %s", "延迟工具池：%s", "Pool verzögerter Tools: %s", "遅延ツールプール: %s", "지연 도구 풀: %s", "Пул отложенных инструментов: %s",
		),
		KeyCompactAttachmentAgentTitle: compactCopy(
			"Post-compaction agent listing", "压缩后的 Agent 列表", "Agent-Liste nach der Komprimierung", "圧縮後の Agent 一覧", "압축 후 Agent 목록", "Список Agent после сжатия",
		),
		KeyCompactAttachmentMCPTitle: compactCopy(
			"Post-compaction MCP state", "压缩后的 MCP 状态", "MCP-Status nach der Komprimierung", "圧縮後の MCP 状態", "압축 후 MCP 상태", "Состояние MCP после сжатия",
		),
		KeyCompactAttachmentMCPToolsLabel: compactCopy(
			" tools=%s", " 工具=%s", " Tools=%s", " ツール=%s", " 도구=%s", " инструменты=%s",
		),
		KeyCompactAttachmentMCPInstructionsLabel: compactCopy(
			"\n  instructions: %s", "\n  说明：%s", "\n  Anweisungen: %s", "\n  指示: %s", "\n  지침: %s", "\n  инструкции: %s",
		),
	} {
		semanticTranslations[key] = translations
	}
}
