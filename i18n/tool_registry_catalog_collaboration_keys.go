package i18n

const (
	KeyRegistryToolExecuteFailed                             Key = "registry.tool.execute_failed"
	KeyRegistryToolUnknown                                   Key = "registry.tool.unknown"
	KeyToolSearchCatalogNoMatches                            Key = "tool.search.catalog.no_matches"
	KeyToolSearchCatalogRequestedToolsMissing                Key = "tool.search.catalog.requested_tools_missing"
	KeyToolSearchCatalogLoadedWithMissing                    Key = "tool.search.catalog.loaded_with_missing"
	KeyToolSearchCatalogLoadedForQuery                       Key = "tool.search.catalog.loaded_for_query"
	KeyToolSearchCatalogMatchScore                           Key = "tool.search.catalog.match.score"
	KeyToolSearchCatalogMatchSnippet                         Key = "tool.search.catalog.match.snippet"
	KeyMCPVisibilityReconnectAttempt                         Key = "mcp.visibility.reconnect_attempt"
	KeyMCPVisibilityStateEntry                               Key = "mcp.visibility.state_entry"
	KeyMCPVisibilityPendingServers                           Key = "mcp.visibility.pending_servers"
	KeyMCPVisibilityServerStates                             Key = "mcp.visibility.server_states"
	KeyToolSendMessageToRequired                             Key = "tool.send_message.input.to.required"
	KeyToolSendMessageAddressSchemeUnsupported               Key = "tool.send_message.address.scheme_unsupported"
	KeyToolSendMessageAddressTargetRequired                  Key = "tool.send_message.address.target_required"
	KeyToolSendMessageBareRecipientRequired                  Key = "tool.send_message.input.to.bare_recipient_required"
	KeyToolSendMessageSummaryRequired                        Key = "tool.send_message.input.summary.required"
	KeyToolSendMessageStructuredBroadcastUnsupported         Key = "tool.send_message.structured.broadcast_unsupported"
	KeyToolSendMessageStructuredCrossSessionUnsupported      Key = "tool.send_message.structured.cross_session_unsupported"
	KeyToolSendMessageStructuredShutdownResponseTarget       Key = "tool.send_message.structured.shutdown_response.target"
	KeyToolSendMessageStructuredShutdownRejectReasonRequired Key = "tool.send_message.structured.shutdown_response.reject_reason_required"
	KeyToolSendMessageAgentResumeFailed                      Key = "tool.send_message.agent.resume_failed"
	KeyToolSendMessageDeliveryQueued                         Key = "tool.send_message.delivery.queued"
	KeyToolSendMessageAgentResumed                           Key = "tool.send_message.agent.resumed"
	KeyToolSendMessageTeamContextRequired                    Key = "tool.send_message.team_context.required"
	KeyToolSendMessageTeamMissing                            Key = "tool.send_message.team.missing"
	KeyToolSendMessageInputMessageInvalid                    Key = "tool.send_message.input.message.invalid"
	KeyToolTeamCreateAlreadyLeading                          Key = "tool.team_create.already_leading"
	KeyToolTeamDeleteNothingToDelete                         Key = "tool.team_delete.nothing_to_delete"
	KeyToolTeamDeleteActiveMembersBlocked                    Key = "tool.team_delete.active_members_blocked"
	KeyToolTeamDeleteCompleted                               Key = "tool.team_delete.completed"
)

func init() {
	entry := func(en, zh, de, ja, ko, ru string) map[Language]string {
		return map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}
	for key, translations := range map[Key]map[Language]string{
		KeyRegistryToolExecuteFailed: entry(
			"Error executing tool '%s': %s",
			"执行工具“%s”时出错：%s",
			"Fehler beim Ausführen des Tools „%s“: %s",
			"ツール「%s」の実行中にエラーが発生しました：%s",
			"도구 '%s' 실행 중 오류: %s",
			"Ошибка при выполнении инструмента «%s»: %s",
		),
		KeyRegistryToolUnknown: entry(
			"Error: unknown tool '%s'. Available tools: %v",
			"错误：未知工具“%s”。可用工具：%v",
			"Fehler: Unbekanntes Tool „%s“. Verfügbare Tools: %v",
			"エラー：不明なツール「%s」です。利用可能なツール：%v",
			"오류: 알 수 없는 도구 '%s'. 사용 가능한 도구: %v",
			"Ошибка: неизвестный инструмент «%s». Доступные инструменты: %v",
		),
		KeyToolSearchCatalogNoMatches: entry(
			"No matching deferred tools found for %q. Deferred tool pool: %d tool(s).",
			"未找到与 %q 匹配的延迟加载工具。延迟加载工具池：%d 个工具。",
			"Keine passenden verzögert geladenen Tools für %q gefunden. Pool: %d Tool(s).",
			"%q に一致する遅延読み込みツールが見つかりません。ツールプール：%d 件。",
			"%q에 맞는 지연 로드 도구를 찾지 못했습니다. 도구 풀: %d개.",
			"Для %q не найдены подходящие инструменты отложенной загрузки. В пуле: %d.",
		),
		KeyToolSearchCatalogRequestedToolsMissing: entry(
			"No requested tools were found for %q. Missing: %s. Deferred tool pool: %d tool(s).",
			"未找到 %q 请求的工具。缺少：%s。延迟加载工具池：%d 个工具。",
			"Für %q wurden keine angeforderten Tools gefunden. Fehlend: %s. Pool: %d Tool(s).",
			"%q で指定されたツールが見つかりません。見つからないツール：%s。ツールプール：%d 件。",
			"%q에서 요청한 도구를 찾지 못했습니다. 누락: %s. 도구 풀: %d개.",
			"Для %q не найдены запрошенные инструменты. Отсутствуют: %s. В пуле: %d.",
		),
		KeyToolSearchCatalogLoadedWithMissing: entry(
			"Loaded %d tool(s): %s. Missing: %s. Deferred tool pool: %d tool(s).",
			"已加载 %d 个工具：%s。缺少：%s。延迟加载工具池：%d 个工具。",
			"%d Tool(s) geladen: %s. Fehlend: %s. Pool: %d Tool(s).",
			"%d 件のツールを読み込みました：%s。見つからないツール：%s。ツールプール：%d 件。",
			"도구 %d개 로드: %s. 누락: %s. 도구 풀: %d개.",
			"Загружено инструментов: %d: %s. Отсутствуют: %s. В пуле: %d.",
		),
		KeyToolSearchCatalogLoadedForQuery: entry(
			"Loaded %d tool(s) for %q: %s. Deferred tool pool: %d tool(s).",
			"已加载 %d 个工具（查询 %q）：%s。延迟加载工具池：%d 个工具。",
			"%d Tool(s) für %q geladen: %s. Pool: %d Tool(s).",
			"%d 件のツールを読み込みました（検索 %q）：%s。ツールプール：%d 件。",
			"도구 %d개 로드(검색 %q): %s. 도구 풀: %d개.",
			"Загружено инструментов: %d для %q: %s. В пуле: %d.",
		),
		KeyToolSearchCatalogMatchScore: entry(
			"\n  - %s score=%.4f",
			"\n  - %s 得分=%.4f",
			"\n  - %s Bewertung=%.4f",
			"\n  - %s スコア=%.4f",
			"\n  - %s 점수=%.4f",
			"\n  - %s оценка=%.4f",
		),
		KeyToolSearchCatalogMatchSnippet: entry(
			" Snippet: matched=%s",
			" 片段：匹配=%s",
			" Ausschnitt: Treffer=%s",
			" スニペット：一致=%s",
			" 스니펫: 일치=%s",
			" Фрагмент: совпадение=%s",
		),
		KeyMCPVisibilityReconnectAttempt: entry(
			"%s (reconnect %d/%d)",
			"%s（重连 %d/%d）",
			"%s (Neuverbinden %d/%d)",
			"%s（再接続 %d/%d）",
			"%s (재연결 %d/%d)",
			"%s (переподключение %d/%d)",
		),
		KeyMCPVisibilityStateEntry: entry("%s: %s", "%s：%s", "%s: %s", "%s：%s", "%s: %s", "%s: %s"),
		KeyMCPVisibilityPendingServers: entry(
			"Pending MCP servers (still connecting): %s. ",
			"仍在连接的 MCP server：%s。",
			"Ausstehende MCP-Server (Verbindung läuft): %s. ",
			"接続中の MCP server：%s。",
			"연결 중인 MCP server: %s. ",
			"MCP-серверы всё ещё подключаются: %s. ",
		),
		KeyMCPVisibilityServerStates: entry(
			"MCP server states: %s.",
			"MCP server 状态：%s。",
			"Status der MCP-Server: %s.",
			"MCP server の状態：%s。",
			"MCP server 상태: %s.",
			"Состояния MCP-серверов: %s.",
		),
		KeyToolSendMessageToRequired: entry("to must not be empty", "to 不能为空", "to darf nicht leer sein", "to は空にできません", "to는 비워 둘 수 없습니다", "to не может быть пустым"),
		KeyToolSendMessageAddressSchemeUnsupported: entry(
			`unsupported SendMessage address scheme %q; use a teammate name, "*", or "uds:/path/to.sock"`,
			`不支持 SendMessage 地址方案 %q；请使用 teammate 名称、"*" 或 "uds:/path/to.sock"`,
			`Nicht unterstütztes SendMessage-Adressschema %q; verwende einen Teammate-Namen, "*" oder "uds:/path/to.sock"`,
			`未対応の SendMessage アドレススキーム %q です。teammate 名、"*"、または "uds:/path/to.sock" を使用してください`,
			`지원하지 않는 SendMessage 주소 스킴 %q입니다. teammate 이름, "*" 또는 "uds:/path/to.sock"을 사용하세요`,
			`Неподдерживаемая схема адреса SendMessage %q; используйте имя teammate, "*" или "uds:/path/to.sock"`,
		),
		KeyToolSendMessageAddressTargetRequired: entry("address target must not be empty", "地址目标不能为空", "Adressziel darf nicht leer sein", "アドレスの宛先は空にできません", "주소 대상은 비워 둘 수 없습니다", "Целевой адрес не может быть пустым"),
		KeyToolSendMessageBareRecipientRequired: entry(
			`to must be a bare teammate name or "*" - there is only one team per session`,
			`to 必须是单独的 teammate 名称或 "*"——每个 session 只有一个 team`,
			`to muss ein einfacher Teammate-Name oder "*" sein – pro Sitzung gibt es nur ein Team`,
			`to には単独の teammate 名または "*" を指定してください。session ごとに team は 1 つだけです`,
			`to는 teammate 이름 또는 "*"만 사용할 수 있습니다. session당 team은 하나뿐입니다`,
			`to должно содержать только имя teammate или "*" — в одной сессии может быть только одна команда`,
		),
		KeyToolSendMessageSummaryRequired:                        entry("summary is required when message is a string", "message 为字符串时必须提供 summary", "summary ist erforderlich, wenn message eine Zeichenkette ist", "message が文字列の場合は summary が必要です", "message가 문자열이면 summary가 필요합니다", "Если message — строка, требуется summary"),
		KeyToolSendMessageStructuredBroadcastUnsupported:         entry(`structured messages cannot be broadcast (to: "*")`, `结构化消息不能广播（to: "*"）`, `Strukturierte Nachrichten können nicht gesendet werden (to: "*")`, `構造化メッセージはブロードキャストできません（to: "*"）`, `구조화된 메시지는 브로드캐스트할 수 없습니다(to: "*")`, `Структурированные сообщения нельзя рассылать всем (to: "*")`),
		KeyToolSendMessageStructuredCrossSessionUnsupported:      entry("structured messages cannot be sent cross-session - only plain text", "结构化消息不能跨 session 发送，只支持纯文本", "Strukturierte Nachrichten können nicht sitzungsübergreifend gesendet werden – nur Klartext", "構造化メッセージは session をまたいで送信できません。利用できるのはプレーンテキストだけです", "구조화된 메시지는 session 간에 보낼 수 없습니다. 일반 텍스트만 가능합니다", "Структурированные сообщения нельзя отправлять между сессиями — доступен только обычный текст"),
		KeyToolSendMessageStructuredShutdownResponseTarget:       entry(`shutdown_response must be sent to "%s"`, `shutdown_response 必须发送给“%s”`, `shutdown_response muss an „%s“ gesendet werden`, `shutdown_response は「%s」に送信する必要があります`, `shutdown_response는 "%s"에게 보내야 합니다`, `shutdown_response должен быть отправлен адресату «%s»`),
		KeyToolSendMessageStructuredShutdownRejectReasonRequired: entry("reason is required when rejecting a shutdown request", "拒绝 shutdown request 时必须提供 reason", "Beim Ablehnen einer shutdown request ist reason erforderlich", "shutdown request を拒否する場合は reason が必要です", "shutdown request를 거부할 때는 reason이 필요합니다", "При отклонении shutdown request требуется reason"),
		KeyToolSendMessageAgentResumeFailed:                      entry("Agent %q could not be resumed: %s", "无法恢复 Agent %q：%s", "Agent %q konnte nicht fortgesetzt werden: %s", "Agent %q を再開できませんでした：%s", "Agent %q을(를) 재개하지 못했습니다: %s", "Не удалось возобновить Agent %q: %s"),
		KeyToolSendMessageDeliveryQueued:                         entry("Message queued for delivery to %s at its next tool round.", "消息已排队，将在 %s 的下一轮 tool 调用时投递。", "Die Nachricht wurde zur Zustellung an %s in dessen nächster Tool-Runde eingereiht.", "%s の次の tool ラウンドで配信するようメッセージをキューに追加しました。", "%s의 다음 tool 라운드에 전달하도록 메시지를 대기열에 추가했습니다.", "Сообщение поставлено в очередь для доставки %s на следующем цикле инструмента."),
		KeyToolSendMessageAgentResumed: entry(
			"Agent %q was stopped (%s); resumed it in the background with your message. You'll be notified when it finishes. Output: %s",
			"Agent %q 已停止（%s）；现已携带你的消息在后台恢复运行。完成后会通知你。输出：%s",
			"Agent %q war angehalten (%s) und wurde mit deiner Nachricht im Hintergrund fortgesetzt. Nach Abschluss wirst du benachrichtigt. Ausgabe: %s",
			"Agent %q は停止していました（%s）が、メッセージを渡してバックグラウンドで再開しました。完了時に通知します。出力：%s",
			"Agent %q이(가) 중지 상태(%s)였으며 메시지와 함께 백그라운드에서 재개되었습니다. 완료되면 알림을 보냅니다. 출력: %s",
			"Agent %q был остановлен (%s), но возобновлён в фоне с вашим сообщением. После завершения вы получите уведомление. Вывод: %s",
		),
		KeyToolSendMessageTeamContextRequired: entry(
			"Not in a team context. Create a team with Teammate spawnTeam first, or set LUBAN_CODE_TEAM_NAME.",
			"当前不在 team 上下文中。请先使用 Teammate spawnTeam 创建 team，或设置 LUBAN_CODE_TEAM_NAME。",
			"Kein Team-Kontext aktiv. Erstelle zuerst mit Teammate spawnTeam ein Team oder setze LUBAN_CODE_TEAM_NAME.",
			"team コンテキストではありません。まず Teammate spawnTeam で team を作成するか、LUBAN_CODE_TEAM_NAME を設定してください。",
			"team 컨텍스트가 아닙니다. 먼저 Teammate spawnTeam으로 team을 만들거나 LUBAN_CODE_TEAM_NAME을 설정하세요.",
			"Контекст команды отсутствует. Сначала создайте команду через Teammate spawnTeam или задайте LUBAN_CODE_TEAM_NAME.",
		),
		KeyToolSendMessageTeamMissing:         entry("Team %q does not exist", "Team %q 不存在", "Team %q existiert nicht", "Team %q は存在しません", "Team %q이(가) 없습니다", "Команда %q не существует"),
		KeyToolSendMessageInputMessageInvalid: entry("message must be a string or supported structured object", "message 必须是字符串或受支持的结构化对象", "message muss eine Zeichenkette oder ein unterstütztes strukturiertes Objekt sein", "message は文字列または対応する構造化オブジェクトである必要があります", "message는 문자열 또는 지원되는 구조화 객체여야 합니다", "message должен быть строкой или поддерживаемым структурированным объектом"),
		KeyToolTeamCreateAlreadyLeading: entry(
			`Already leading team "%s". A leader can only manage one team at a time. Use TeamDelete to end the current team before creating a new one.`,
			`你已在带领 team“%s”。leader 一次只能管理一个 team。请先使用 TeamDelete 结束当前 team，再创建新 team。`,
			`Du leitest bereits das Team „%s“. Eine Leitung kann jeweils nur ein Team verwalten. Beende das aktuelle Team mit TeamDelete, bevor du ein neues erstellst.`,
			`すでに team「%s」を率いています。leader が同時に管理できる team は 1 つだけです。新しい team を作成する前に TeamDelete で現在の team を終了してください。`,
			`이미 team "%s"을(를) 이끌고 있습니다. leader는 한 번에 하나의 team만 관리할 수 있습니다. 새 team을 만들기 전에 TeamDelete로 현재 team을 종료하세요.`,
			`Вы уже руководите командой «%s». Руководитель может управлять только одной командой. Перед созданием новой завершите текущую через TeamDelete.`,
		),
		KeyToolTeamDeleteNothingToDelete: entry("No team name found, nothing to clean up", "未找到 team 名称，无需清理", "Kein Teamname gefunden; es gibt nichts zu bereinigen", "team 名が見つからないため、クリーンアップするものはありません", "team 이름을 찾지 못해 정리할 항목이 없습니다", "Имя команды не найдено; очищать нечего"),
		KeyToolTeamDeleteActiveMembersBlocked: entry(
			"Cannot cleanup team with %d active member(s): %s. Use requestShutdown to gracefully terminate teammates first.",
			"无法清理仍有 %d 名活跃成员的 team：%s。请先使用 requestShutdown 正常终止 teammates。",
			"Team mit %d aktiven Mitgliedern kann nicht bereinigt werden: %s. Beende die Teammates zuerst ordnungsgemäß mit requestShutdown.",
			"%d 名のアクティブメンバーがいる team はクリーンアップできません：%s。先に requestShutdown で teammates を正常終了してください。",
			"활성 멤버 %d명이 있는 team은 정리할 수 없습니다: %s. 먼저 requestShutdown으로 teammates를 정상 종료하세요.",
			"Нельзя очистить команду с активными участниками (%d): %s. Сначала корректно завершите teammates через requestShutdown.",
		),
		KeyToolTeamDeleteCompleted: entry(`Cleaned up directories and worktrees for team "%s"`, `已清理 team“%s”的目录和 worktree`, `Verzeichnisse und Worktrees für Team „%s“ wurden bereinigt`, `team「%s」のディレクトリと worktree をクリーンアップしました`, `team "%s"의 디렉터리와 worktree를 정리했습니다`, `Каталоги и worktree команды «%s» очищены`),
	} {
		semanticTranslations[key] = translations
	}
}
