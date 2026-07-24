package i18n

const (
	KeyToolLegacyDRegistryExecuteFailed       Key = "tool.legacy_d.registry.execute_failed"
	KeyToolLegacyDRegistryUnknownTool         Key = "tool.legacy_d.registry.unknown_tool"
	KeyToolLegacyDRegistryToolDisabled        Key = "tool.legacy_d.registry.tool_disabled"
	KeyToolLegacyDRegistryPermissionRequired  Key = "tool.legacy_d.registry.permission_required"
	KeyToolLegacyDTestingPermissionSucceeded  Key = "tool.legacy_d.testing_permission.succeeded"
	KeyToolLegacyDTodoInputRequired           Key = "tool.legacy_d.todo.input_required"
	KeyToolLegacyDTodoLimitExceeded           Key = "tool.legacy_d.todo.limit_exceeded"
	KeyToolLegacyDTodoContentRequired         Key = "tool.legacy_d.todo.content_required"
	KeyToolLegacyDTodoActiveFormRequired      Key = "tool.legacy_d.todo.active_form_required"
	KeyToolLegacyDTodoStatusInvalid           Key = "tool.legacy_d.todo.status_invalid"
	KeyToolLegacyDTodoContentDuplicate        Key = "tool.legacy_d.todo.content_duplicate"
	KeyToolLegacyDTodoSaveFailed              Key = "tool.legacy_d.todo.save_failed"
	KeyToolLegacyDTodoRegressionWarning       Key = "tool.legacy_d.todo.regression_warning"
	KeyToolLegacyDTodoTypedResultInvalid      Key = "tool.legacy_d.todo.typed_result_invalid"
	KeyToolLegacyDTodoModified                Key = "tool.legacy_d.todo.modified"
	KeyToolLegacyDRequiredFieldQuoted         Key = "tool.legacy_d.input.required_field_quoted"
	KeyToolLegacyDToolSearchNoMatches         Key = "tool.legacy_d.tool_search.no_matches"
	KeyToolLegacyDToolSearchRequestedMissing  Key = "tool.legacy_d.tool_search.requested_missing"
	KeyToolLegacyDToolSearchLoadedWithMissing Key = "tool.legacy_d.tool_search.loaded_with_missing"
	KeyToolLegacyDToolSearchLoadedForQuery    Key = "tool.legacy_d.tool_search.loaded_for_query"
	KeyToolLegacyDToolSearchScore             Key = "tool.legacy_d.tool_search.score"
	KeyToolLegacyDToolSearchSnippet           Key = "tool.legacy_d.tool_search.snippet"
	KeyToolLegacyDMCPReconnect                Key = "tool.legacy_d.mcp.reconnect"
	KeyToolLegacyDMCPStatePending             Key = "tool.legacy_d.mcp.state_pending"
	KeyToolLegacyDMCPStateFailed              Key = "tool.legacy_d.mcp.state_failed"
	KeyToolLegacyDMCPStateNeedsAuth           Key = "tool.legacy_d.mcp.state_needs_auth"
	KeyToolLegacyDMCPStateDisabled            Key = "tool.legacy_d.mcp.state_disabled"
	KeyToolLegacyDMCPStateEntry               Key = "tool.legacy_d.mcp.state_entry"
	KeyToolLegacyDMCPPendingServers           Key = "tool.legacy_d.mcp.pending_servers"
	KeyToolLegacyDMCPServerStates             Key = "tool.legacy_d.mcp.server_states"
	KeyToolLegacyDSendToRequired              Key = "tool.legacy_d.send_message.to_required"
	KeyToolLegacyDSendSchemeUnsupported       Key = "tool.legacy_d.send_message.scheme_unsupported"
	KeyToolLegacyDSendAddressRequired         Key = "tool.legacy_d.send_message.address_required"
	KeyToolLegacyDSendBridgeConsent           Key = "tool.legacy_d.send_message.bridge_consent"
	KeyToolLegacyDSendBridgeUnavailable       Key = "tool.legacy_d.send_message.bridge_unavailable"
	KeyToolLegacyDSendBareRecipientRequired   Key = "tool.legacy_d.send_message.bare_recipient_required"
	KeyToolLegacyDSendDecodeFailed            Key = "tool.legacy_d.send_message.decode_failed"
	KeyToolLegacyDSendSummaryRequired         Key = "tool.legacy_d.send_message.summary_required"
	KeyToolLegacyDSendStructuredBroadcast     Key = "tool.legacy_d.send_message.structured_broadcast"
	KeyToolLegacyDSendStructuredCrossSession  Key = "tool.legacy_d.send_message.structured_cross_session"
	KeyToolLegacyDSendShutdownTarget          Key = "tool.legacy_d.send_message.shutdown_target"
	KeyToolLegacyDSendShutdownRejectReason    Key = "tool.legacy_d.send_message.shutdown_reject_reason"
	KeyToolLegacyDSendAgentResumeFailed       Key = "tool.legacy_d.send_message.agent_resume_failed"
	KeyToolLegacyDSendQueued                  Key = "tool.legacy_d.send_message.queued"
	KeyToolLegacyDSendAgentResumed            Key = "tool.legacy_d.send_message.agent_resumed"
	KeyToolLegacyDSendNoTeamContext           Key = "tool.legacy_d.send_message.no_team_context"
	KeyToolLegacyDSendTeamMissing             Key = "tool.legacy_d.send_message.team_missing"
	KeyToolLegacyDSendMessageTypeInvalid      Key = "tool.legacy_d.send_message.message_type_invalid"
	KeyToolLegacyDTeamAlreadyLeading          Key = "tool.legacy_d.team.already_leading"
	KeyToolLegacyDTeamAgentIDRequired         Key = "tool.legacy_d.team.agent_id_required"
	KeyToolLegacyDTeamAgentNoOutput           Key = "tool.legacy_d.team.agent_no_output"
	KeyToolLegacyDTeamAgentCompleted          Key = "tool.legacy_d.team.agent_completed"
	KeyToolLegacyDTeamNothingToDelete         Key = "tool.legacy_d.team.nothing_to_delete"
	KeyToolLegacyDTeamActiveMembers           Key = "tool.legacy_d.team.active_members"
	KeyToolLegacyDTeamDeleted                 Key = "tool.legacy_d.team.deleted"
	KeyToolLegacyDTeamTasksRequired           Key = "tool.legacy_d.team.tasks_required"
	KeyToolLegacyDTeamNotFound                Key = "tool.legacy_d.team.not_found"
	KeyToolLegacyDTeamTaskDescriptionRequired Key = "tool.legacy_d.team.task_description_required"
	KeyToolLegacyDTeamDispatchEmpty           Key = "tool.legacy_d.team.dispatch_empty"
	KeyToolLegacyDTeamDispatchComplete        Key = "tool.legacy_d.team.dispatch_complete"
	KeyToolLegacyDTeamDispatchTaskHeader      Key = "tool.legacy_d.team.dispatch_task_header"
	KeyToolLegacyDTeamDispatchError           Key = "tool.legacy_d.team.dispatch_error"
)

func init() {
	entry := func(en, zh, de, ja, ko, ru string) map[Language]string {
		return map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}
	for key, translations := range map[Key]map[Language]string{
		KeyToolLegacyDRegistryExecuteFailed: entry(
			"Error executing tool '%s': %s",
			"执行工具“%s”时出错：%s",
			"Fehler beim Ausführen des Tools „%s“: %s",
			"ツール「%s」の実行中にエラーが発生しました：%s",
			"도구 '%s' 실행 중 오류: %s",
			"Ошибка при выполнении инструмента «%s»: %s",
		),
		KeyToolLegacyDRegistryUnknownTool: entry(
			"Error: unknown tool '%s'. Available tools: %v",
			"错误：未知工具“%s”。可用工具：%v",
			"Fehler: Unbekanntes Tool „%s“. Verfügbare Tools: %v",
			"エラー：不明なツール「%s」です。利用可能なツール：%v",
			"오류: 알 수 없는 도구 '%s'. 사용 가능한 도구: %v",
			"Ошибка: неизвестный инструмент «%s». Доступные инструменты: %v",
		),
		KeyToolLegacyDRegistryToolDisabled: entry(
			"Tool %q is not enabled in the current runtime context",
			"当前运行时上下文未启用工具 %q",
			"Tool %q ist im aktuellen Laufzeitkontext nicht aktiviert",
			"現在のランタイムコンテキストではツール %q が有効になっていません",
			"현재 런타임 컨텍스트에서 도구 %q이(가) 활성화되어 있지 않습니다",
			"Инструмент %q не включён в текущем контексте выполнения",
		),
		KeyToolLegacyDRegistryPermissionRequired: entry(
			"Permission required for tool: %s",
			"工具需要权限：%s",
			"Berechtigung für Tool erforderlich: %s",
			"ツールの権限が必要です：%s",
			"도구 권한 필요: %s",
			"Для инструмента требуется разрешение: %s",
		),
		KeyToolLegacyDTestingPermissionSucceeded: entry(
			"TestingPermission executed successfully",
			"TestingPermission 执行成功",
			"TestingPermission wurde erfolgreich ausgeführt",
			"TestingPermission が正常に実行されました",
			"TestingPermission이(가) 성공적으로 실행되었습니다",
			"TestingPermission успешно выполнен",
		),
		KeyToolLegacyDTodoInputRequired: entry(
			"Error: invalid input: todos is required",
			"错误：输入无效：必须提供 todos",
			"Fehler: Ungültige Eingabe: todos ist erforderlich",
			"エラー：入力が無効です：todos は必須です",
			"오류: 잘못된 입력: todos가 필요합니다",
			"Ошибка: недопустимый ввод: требуется todos",
		),
		KeyToolLegacyDTodoLimitExceeded: entry(
			"Error: todo list exceeds %d items (got %d). Trim to fewer focus items.",
			"错误：todo 列表不能超过 %d 项（当前 %d 项）。请精简为更少的重点事项。",
			"Fehler: Die Todo-Liste überschreitet %d Einträge (erhalten: %d). Kürze sie auf weniger Schwerpunkte.",
			"エラー：todo リストが上限の %d 件を超えています（現在 %d 件）。重要な項目に絞ってください。",
			"오류: todo 목록이 %d개 제한을 초과했습니다(현재 %d개). 핵심 항목만 남겨 주세요.",
			"Ошибка: список todo превышает %d пунктов (получено: %d). Сократите его до основных задач.",
		),
		KeyToolLegacyDTodoContentRequired: entry(
			"Error: all todo items must have non-empty content",
			"错误：每个 todo 项都必须包含非空 content",
			"Fehler: Alle Todo-Einträge benötigen einen nicht leeren content-Wert",
			"エラー：すべての todo 項目に空でない content が必要です",
			"오류: 모든 todo 항목의 content는 비어 있지 않아야 합니다",
			"Ошибка: у каждого пункта todo должно быть непустое поле content",
		),
		KeyToolLegacyDTodoActiveFormRequired: entry(
			"Error: all todo items must have non-empty activeForm",
			"错误：每个 todo 项都必须包含非空 activeForm",
			"Fehler: Alle Todo-Einträge benötigen einen nicht leeren activeForm-Wert",
			"エラー：すべての todo 項目に空でない activeForm が必要です",
			"오류: 모든 todo 항목의 activeForm은 비어 있지 않아야 합니다",
			"Ошибка: у каждого пункта todo должно быть непустое поле activeForm",
		),
		KeyToolLegacyDTodoStatusInvalid: entry(
			"Error: all todo items must use pending, in_progress, or completed status",
			"错误：所有 todo 项的 status 必须是 pending、in_progress 或 completed",
			"Fehler: Alle Todo-Einträge müssen den Status pending, in_progress oder completed verwenden",
			"エラー：すべての todo 項目の status は pending、in_progress、completed のいずれかである必要があります",
			"오류: 모든 todo 항목의 status는 pending, in_progress 또는 completed여야 합니다",
			"Ошибка: статус каждого пункта todo должен быть pending, in_progress или completed",
		),
		KeyToolLegacyDTodoContentDuplicate: entry(
			"Error: duplicate todo content within the same write",
			"错误：同一次写入中存在重复的 todo content",
			"Fehler: Doppelte Todo-Inhalte innerhalb desselben Schreibvorgangs",
			"エラー：同じ書き込みに重複する todo content があります",
			"오류: 같은 쓰기 요청에 중복된 todo content가 있습니다",
			"Ошибка: в одной операции записи повторяется content пункта todo",
		),
		KeyToolLegacyDTodoSaveFailed: entry(
			"failed to save todo list: %v",
			"保存 todo 列表失败：%v",
			"Todo-Liste konnte nicht gespeichert werden: %v",
			"todo リストを保存できませんでした：%v",
			"todo 목록 저장 실패: %v",
			"Не удалось сохранить список todo: %v",
		),
		KeyToolLegacyDTodoRegressionWarning: entry(
			"\n\nWarning: %d item(s) regressed from completed → earlier status: %s",
			"\n\n警告：%d 项从 completed 回退到了之前的状态：%s",
			"\n\nWarnung: %d Einträge wurden von completed auf einen früheren Status zurückgesetzt: %s",
			"\n\n警告：%d 件が completed から以前のステータスに戻りました：%s",
			"\n\n경고: %d개 항목이 completed에서 이전 상태로 되돌아갔습니다: %s",
			"\n\nПредупреждение: %d пунктов вернулись из completed в более ранний статус: %s",
		),
		KeyToolLegacyDTodoTypedResultInvalid: entry(
			"TodoWrite returned an invalid typed result",
			"TodoWrite 返回了无效的类型化结果",
			"TodoWrite hat ein ungültiges typisiertes Ergebnis zurückgegeben",
			"TodoWrite が無効な型付き結果を返しました",
			"TodoWrite가 잘못된 형식의 결과를 반환했습니다",
			"TodoWrite вернул недопустимый типизированный результат",
		),
		KeyToolLegacyDTodoModified: entry(
			"Todos have been modified successfully. Ensure that you continue to use the todo list to track your progress. Please proceed with the current tasks if applicable",
			"Todo 已更新成功。请继续使用 todo 列表跟踪进度，并在适用时继续处理当前任务",
			"Die Todos wurden erfolgreich aktualisiert. Verwende die Todo-Liste weiterhin zur Fortschrittsverfolgung und fahre gegebenenfalls mit den aktuellen Aufgaben fort",
			"Todo を更新しました。引き続き todo リストで進捗を管理し、必要に応じて現在のタスクを進めてください",
			"Todo가 성공적으로 업데이트되었습니다. 계속 todo 목록으로 진행 상황을 추적하고, 해당하는 경우 현재 작업을 진행하세요",
			"Список todo успешно обновлён. Продолжайте отслеживать ход работы по списку и при необходимости переходите к текущим задачам",
		),
		KeyToolLegacyDRequiredFieldQuoted: entry(
			"Error: '%s' is required",
			"错误：必须提供“%s”",
			"Fehler: „%s“ ist erforderlich",
			"エラー：「%s」は必須です",
			"오류: '%s'이(가) 필요합니다",
			"Ошибка: требуется «%s»",
		),
		KeyToolLegacyDToolSearchNoMatches: entry(
			"No matching deferred tools found for %q. Deferred tool pool: %d tool(s).",
			"未找到与 %q 匹配的延迟加载工具。延迟加载工具池：%d 个工具。",
			"Keine passenden verzögert geladenen Tools für %q gefunden. Pool: %d Tool(s).",
			"%q に一致する遅延読み込みツールが見つかりません。ツールプール：%d 件。",
			"%q에 맞는 지연 로드 도구를 찾지 못했습니다. 도구 풀: %d개.",
			"Для %q не найдены подходящие инструменты отложенной загрузки. В пуле: %d.",
		),
		KeyToolLegacyDToolSearchRequestedMissing: entry(
			"No requested tools were found for %q. Missing: %s. Deferred tool pool: %d tool(s).",
			"未找到 %q 请求的工具。缺少：%s。延迟加载工具池：%d 个工具。",
			"Für %q wurden keine angeforderten Tools gefunden. Fehlend: %s. Pool: %d Tool(s).",
			"%q で指定されたツールが見つかりません。見つからないツール：%s。ツールプール：%d 件。",
			"%q에서 요청한 도구를 찾지 못했습니다. 누락: %s. 도구 풀: %d개.",
			"Для %q не найдены запрошенные инструменты. Отсутствуют: %s. В пуле: %d.",
		),
		KeyToolLegacyDToolSearchLoadedWithMissing: entry(
			"Loaded %d tool(s): %s. Missing: %s. Deferred tool pool: %d tool(s).",
			"已加载 %d 个工具：%s。缺少：%s。延迟加载工具池：%d 个工具。",
			"%d Tool(s) geladen: %s. Fehlend: %s. Pool: %d Tool(s).",
			"%d 件のツールを読み込みました：%s。見つからないツール：%s。ツールプール：%d 件。",
			"도구 %d개 로드: %s. 누락: %s. 도구 풀: %d개.",
			"Загружено инструментов: %d: %s. Отсутствуют: %s. В пуле: %d.",
		),
		KeyToolLegacyDToolSearchLoadedForQuery: entry(
			"Loaded %d tool(s) for %q: %s. Deferred tool pool: %d tool(s).",
			"已加载 %d 个工具（查询 %q）：%s。延迟加载工具池：%d 个工具。",
			"%d Tool(s) für %q geladen: %s. Pool: %d Tool(s).",
			"%d 件のツールを読み込みました（検索 %q）：%s。ツールプール：%d 件。",
			"도구 %d개 로드(검색 %q): %s. 도구 풀: %d개.",
			"Загружено инструментов: %d для %q: %s. В пуле: %d.",
		),
		KeyToolLegacyDToolSearchScore: entry(
			"\n  - %s score=%.4f",
			"\n  - %s 得分=%.4f",
			"\n  - %s Bewertung=%.4f",
			"\n  - %s スコア=%.4f",
			"\n  - %s 점수=%.4f",
			"\n  - %s оценка=%.4f",
		),
		KeyToolLegacyDToolSearchSnippet: entry(
			" Snippet: matched=%s",
			" 片段：匹配=%s",
			" Ausschnitt: Treffer=%s",
			" スニペット：一致=%s",
			" 스니펫: 일치=%s",
			" Фрагмент: совпадение=%s",
		),
		KeyToolLegacyDMCPReconnect: entry(
			"%s (reconnect %d/%d)",
			"%s（重连 %d/%d）",
			"%s (Neuverbinden %d/%d)",
			"%s（再接続 %d/%d）",
			"%s (재연결 %d/%d)",
			"%s (переподключение %d/%d)",
		),
		KeyToolLegacyDMCPStatePending:   entry("pending", "连接中", "ausstehend", "接続中", "연결 중", "ожидание"),
		KeyToolLegacyDMCPStateFailed:    entry("failed", "失败", "fehlgeschlagen", "失敗", "실패", "ошибка"),
		KeyToolLegacyDMCPStateNeedsAuth: entry("needs-auth", "需要认证", "Authentifizierung nötig", "認証が必要", "인증 필요", "требуется авторизация"),
		KeyToolLegacyDMCPStateDisabled:  entry("disabled", "已禁用", "deaktiviert", "無効", "비활성화", "отключён"),
		KeyToolLegacyDMCPStateEntry:     entry("%s: %s", "%s：%s", "%s: %s", "%s：%s", "%s: %s", "%s: %s"),
		KeyToolLegacyDMCPPendingServers: entry(
			"Pending MCP servers (still connecting): %s. ",
			"仍在连接的 MCP server：%s。",
			"Ausstehende MCP-Server (Verbindung läuft): %s. ",
			"接続中の MCP server：%s。",
			"연결 중인 MCP server: %s. ",
			"MCP-серверы всё ещё подключаются: %s. ",
		),
		KeyToolLegacyDMCPServerStates: entry(
			"MCP server states: %s.",
			"MCP server 状态：%s。",
			"Status der MCP-Server: %s.",
			"MCP server の状態：%s。",
			"MCP server 상태: %s.",
			"Состояния MCP-серверов: %s.",
		),
		KeyToolLegacyDSendToRequired: entry("to must not be empty", "to 不能为空", "to darf nicht leer sein", "to は空にできません", "to는 비워 둘 수 없습니다", "to не может быть пустым"),
		KeyToolLegacyDSendSchemeUnsupported: entry(
			`unsupported SendMessage address scheme %q; use a teammate name, "*", or "uds:/path/to.sock"`,
			`不支持 SendMessage 地址方案 %q；请使用 teammate 名称、"*" 或 "uds:/path/to.sock"`,
			`Nicht unterstütztes SendMessage-Adressschema %q; verwende einen Teammate-Namen, "*" oder "uds:/path/to.sock"`,
			`未対応の SendMessage アドレススキーム %q です。teammate 名、"*"、または "uds:/path/to.sock" を使用してください`,
			`지원하지 않는 SendMessage 주소 스킴 %q입니다. teammate 이름, "*" 또는 "uds:/path/to.sock"을 사용하세요`,
			`Неподдерживаемая схема адреса SendMessage %q; используйте имя teammate, "*" или "uds:/path/to.sock"`,
		),
		KeyToolLegacyDSendAddressRequired: entry("address target must not be empty", "地址目标不能为空", "Adressziel darf nicht leer sein", "アドレスの宛先は空にできません", "주소 대상은 비워 둘 수 없습니다", "Целевой адрес не может быть пустым"),
		KeyToolLegacyDSendBridgeConsent: entry(
			"bridge:%s requires explicit user consent and cannot be auto-approved (classifierApprovable=false)",
			"bridge:%s 需要用户明确同意，不能自动批准（classifierApprovable=false）",
			"bridge:%s erfordert eine ausdrückliche Zustimmung und kann nicht automatisch genehmigt werden (classifierApprovable=false)",
			"bridge:%s にはユーザーの明示的な同意が必要で、自動承認できません（classifierApprovable=false）",
			"bridge:%s에는 사용자의 명시적 동의가 필요하며 자동 승인할 수 없습니다(classifierApprovable=false)",
			"Для bridge:%s требуется явное согласие пользователя; автоматическое одобрение невозможно (classifierApprovable=false)",
		),
		KeyToolLegacyDSendBridgeUnavailable: entry(
			"bridge delivery is not implemented in the Go SendMessage transport",
			"Go SendMessage transport 尚未实现 bridge 投递",
			"Die bridge-Zustellung ist im Go-SendMessage-Transport nicht implementiert",
			"Go SendMessage transport には bridge 配信が実装されていません",
			"Go SendMessage transport에는 bridge 전달이 구현되어 있지 않습니다",
			"Доставка через bridge не реализована в транспорте Go SendMessage",
		),
		KeyToolLegacyDSendBareRecipientRequired: entry(
			`to must be a bare teammate name or "*" - there is only one team per session`,
			`to 必须是单独的 teammate 名称或 "*"——每个 session 只有一个 team`,
			`to muss ein einfacher Teammate-Name oder "*" sein – pro Sitzung gibt es nur ein Team`,
			`to には単独の teammate 名または "*" を指定してください。session ごとに team は 1 つだけです`,
			`to는 teammate 이름 또는 "*"만 사용할 수 있습니다. session당 team은 하나뿐입니다`,
			`to должно содержать только имя teammate или "*" — в одной сессии может быть только одна команда`,
		),
		KeyToolLegacyDSendDecodeFailed:           entry("failed to decode structured message: %v", "解析结构化消息失败：%v", "Strukturierte Nachricht konnte nicht dekodiert werden: %v", "構造化メッセージをデコードできませんでした：%v", "구조화된 메시지 디코딩 실패: %v", "Не удалось декодировать структурированное сообщение: %v"),
		KeyToolLegacyDSendSummaryRequired:        entry("summary is required when message is a string", "message 为字符串时必须提供 summary", "summary ist erforderlich, wenn message eine Zeichenkette ist", "message が文字列の場合は summary が必要です", "message가 문자열이면 summary가 필요합니다", "Если message — строка, требуется summary"),
		KeyToolLegacyDSendStructuredBroadcast:    entry(`structured messages cannot be broadcast (to: "*")`, `结构化消息不能广播（to: "*"）`, `Strukturierte Nachrichten können nicht gesendet werden (to: "*")`, `構造化メッセージはブロードキャストできません（to: "*"）`, `구조화된 메시지는 브로드캐스트할 수 없습니다(to: "*")`, `Структурированные сообщения нельзя рассылать всем (to: "*")`),
		KeyToolLegacyDSendStructuredCrossSession: entry("structured messages cannot be sent cross-session - only plain text", "结构化消息不能跨 session 发送，只支持纯文本", "Strukturierte Nachrichten können nicht sitzungsübergreifend gesendet werden – nur Klartext", "構造化メッセージは session をまたいで送信できません。利用できるのはプレーンテキストだけです", "구조화된 메시지는 session 간에 보낼 수 없습니다. 일반 텍스트만 가능합니다", "Структурированные сообщения нельзя отправлять между сессиями — доступен только обычный текст"),
		KeyToolLegacyDSendShutdownTarget:         entry(`shutdown_response must be sent to "%s"`, `shutdown_response 必须发送给“%s”`, `shutdown_response muss an „%s“ gesendet werden`, `shutdown_response は「%s」に送信する必要があります`, `shutdown_response는 "%s"에게 보내야 합니다`, `shutdown_response должен быть отправлен адресату «%s»`),
		KeyToolLegacyDSendShutdownRejectReason:   entry("reason is required when rejecting a shutdown request", "拒绝 shutdown request 时必须提供 reason", "Beim Ablehnen einer shutdown request ist reason erforderlich", "shutdown request を拒否する場合は reason が必要です", "shutdown request를 거부할 때는 reason이 필요합니다", "При отклонении shutdown request требуется reason"),
		KeyToolLegacyDSendAgentResumeFailed:      entry("Agent %q could not be resumed: %s", "无法恢复 Agent %q：%s", "Agent %q konnte nicht fortgesetzt werden: %s", "Agent %q を再開できませんでした：%s", "Agent %q을(를) 재개하지 못했습니다: %s", "Не удалось возобновить Agent %q: %s"),
		KeyToolLegacyDSendQueued:                 entry("Message queued for delivery to %s at its next tool round.", "消息已排队，将在 %s 的下一轮 tool 调用时投递。", "Die Nachricht wurde zur Zustellung an %s in dessen nächster Tool-Runde eingereiht.", "%s の次の tool ラウンドで配信するようメッセージをキューに追加しました。", "%s의 다음 tool 라운드에 전달하도록 메시지를 대기열에 추가했습니다.", "Сообщение поставлено в очередь для доставки %s на следующем цикле инструмента."),
		KeyToolLegacyDSendAgentResumed: entry(
			"Agent %q was stopped (%s); resumed it in the background with your message. You'll be notified when it finishes. Output: %s",
			"Agent %q 已停止（%s）；现已携带你的消息在后台恢复运行。完成后会通知你。输出：%s",
			"Agent %q war angehalten (%s) und wurde mit deiner Nachricht im Hintergrund fortgesetzt. Nach Abschluss wirst du benachrichtigt. Ausgabe: %s",
			"Agent %q は停止していました（%s）が、メッセージを渡してバックグラウンドで再開しました。完了時に通知します。出力：%s",
			"Agent %q이(가) 중지 상태(%s)였으며 메시지와 함께 백그라운드에서 재개되었습니다. 완료되면 알림을 보냅니다. 출력: %s",
			"Agent %q был остановлен (%s), но возобновлён в фоне с вашим сообщением. После завершения вы получите уведомление. Вывод: %s",
		),
		KeyToolLegacyDSendNoTeamContext: entry(
			"Not in a team context. Create a team with Teammate spawnTeam first, or set CLAUDE_CODE_TEAM_NAME.",
			"当前不在 team 上下文中。请先使用 Teammate spawnTeam 创建 team，或设置 CLAUDE_CODE_TEAM_NAME。",
			"Kein Team-Kontext aktiv. Erstelle zuerst mit Teammate spawnTeam ein Team oder setze CLAUDE_CODE_TEAM_NAME.",
			"team コンテキストではありません。まず Teammate spawnTeam で team を作成するか、CLAUDE_CODE_TEAM_NAME を設定してください。",
			"team 컨텍스트가 아닙니다. 먼저 Teammate spawnTeam으로 team을 만들거나 CLAUDE_CODE_TEAM_NAME을 설정하세요.",
			"Контекст команды отсутствует. Сначала создайте команду через Teammate spawnTeam или задайте CLAUDE_CODE_TEAM_NAME.",
		),
		KeyToolLegacyDSendTeamMissing:        entry("Team %q does not exist", "Team %q 不存在", "Team %q existiert nicht", "Team %q は存在しません", "Team %q이(가) 없습니다", "Команда %q не существует"),
		KeyToolLegacyDSendMessageTypeInvalid: entry("message must be a string or supported structured object", "message 必须是字符串或受支持的结构化对象", "message muss eine Zeichenkette oder ein unterstütztes strukturiertes Objekt sein", "message は文字列または対応する構造化オブジェクトである必要があります", "message는 문자열 또는 지원되는 구조화 객체여야 합니다", "message должен быть строкой или поддерживаемым структурированным объектом"),
		KeyToolLegacyDTeamAlreadyLeading: entry(
			`Already leading team "%s". A leader can only manage one team at a time. Use TeamDelete to end the current team before creating a new one.`,
			`你已在带领 team“%s”。leader 一次只能管理一个 team。请先使用 TeamDelete 结束当前 team，再创建新 team。`,
			`Du leitest bereits das Team „%s“. Eine Leitung kann jeweils nur ein Team verwalten. Beende das aktuelle Team mit TeamDelete, bevor du ein neues erstellst.`,
			`すでに team「%s」を率いています。leader が同時に管理できる team は 1 つだけです。新しい team を作成する前に TeamDelete で現在の team を終了してください。`,
			`이미 team "%s"을(를) 이끌고 있습니다. leader는 한 번에 하나의 team만 관리할 수 있습니다. 새 team을 만들기 전에 TeamDelete로 현재 team을 종료하세요.`,
			`Вы уже руководите командой «%s». Руководитель может управлять только одной командой. Перед созданием новой завершите текущую через TeamDelete.`,
		),
		KeyToolLegacyDTeamAgentIDRequired: entry("Error: agent 'id' must not be empty", "错误：Agent 的“id”不能为空", "Fehler: Die „id“ des Agent darf nicht leer sein", "エラー：Agent の「id」は空にできません", "오류: Agent의 'id'는 비워 둘 수 없습니다", "Ошибка: поле «id» Agent не может быть пустым"),
		KeyToolLegacyDTeamAgentNoOutput:   entry("(agent %s completed task %s with no text output)", "（Agent %s 已完成任务 %s，但没有文本输出）", "(Agent %s hat Aufgabe %s ohne Textausgabe abgeschlossen)", "（Agent %s はタスク %s を完了しましたが、テキスト出力はありません）", "(Agent %s이(가) 작업 %s을(를) 완료했지만 텍스트 출력이 없습니다)", "(Agent %s завершил задачу %s без текстового вывода)"),
		KeyToolLegacyDTeamAgentCompleted:  entry("agent %s completed task %s", "Agent %s 已完成任务 %s", "Agent %s hat Aufgabe %s abgeschlossen", "Agent %s がタスク %s を完了しました", "Agent %s이(가) 작업 %s을(를) 완료했습니다", "Agent %s завершил задачу %s"),
		KeyToolLegacyDTeamNothingToDelete: entry("No team name found, nothing to clean up", "未找到 team 名称，无需清理", "Kein Teamname gefunden; es gibt nichts zu bereinigen", "team 名が見つからないため、クリーンアップするものはありません", "team 이름을 찾지 못해 정리할 항목이 없습니다", "Имя команды не найдено; очищать нечего"),
		KeyToolLegacyDTeamActiveMembers: entry(
			"Cannot cleanup team with %d active member(s): %s. Use requestShutdown to gracefully terminate teammates first.",
			"无法清理仍有 %d 名活跃成员的 team：%s。请先使用 requestShutdown 正常终止 teammates。",
			"Team mit %d aktiven Mitgliedern kann nicht bereinigt werden: %s. Beende die Teammates zuerst ordnungsgemäß mit requestShutdown.",
			"%d 名のアクティブメンバーがいる team はクリーンアップできません：%s。先に requestShutdown で teammates を正常終了してください。",
			"활성 멤버 %d명이 있는 team은 정리할 수 없습니다: %s. 먼저 requestShutdown으로 teammates를 정상 종료하세요.",
			"Нельзя очистить команду с активными участниками (%d): %s. Сначала корректно завершите teammates через requestShutdown.",
		),
		KeyToolLegacyDTeamDeleted:                 entry(`Cleaned up directories and worktrees for team "%s"`, `已清理 team“%s”的目录和 worktree`, `Verzeichnisse und Worktrees für Team „%s“ wurden bereinigt`, `team「%s」のディレクトリと worktree をクリーンアップしました`, `team "%s"의 디렉터리와 worktree를 정리했습니다`, `Каталоги и worktree команды «%s» очищены`),
		KeyToolLegacyDTeamTasksRequired:           entry("Error: at least one task is required", "错误：至少需要一个任务", "Fehler: Mindestens eine Aufgabe ist erforderlich", "エラー：少なくとも 1 つのタスクが必要です", "오류: 작업이 하나 이상 필요합니다", "Ошибка: требуется хотя бы одна задача"),
		KeyToolLegacyDTeamNotFound:                entry("Error: team %q not found", "错误：找不到 team %q", "Fehler: Team %q nicht gefunden", "エラー：team %q が見つかりません", "오류: team %q을(를) 찾지 못했습니다", "Ошибка: команда %q не найдена"),
		KeyToolLegacyDTeamTaskDescriptionRequired: entry("Error: task 'description' must not be empty", "错误：任务的“description”不能为空", "Fehler: Die „description“ der Aufgabe darf nicht leer sein", "エラー：タスクの「description」は空にできません", "오류: 작업의 'description'은 비워 둘 수 없습니다", "Ошибка: поле «description» задачи не может быть пустым"),
		KeyToolLegacyDTeamDispatchEmpty:           entry("Dispatch complete: no tasks were executed (no available agents?)", "分派完成：未执行任何任务（没有可用 Agent？）", "Dispatch abgeschlossen: Keine Aufgaben ausgeführt (keine Agents verfügbar?)", "分配完了：タスクは実行されませんでした（利用可能な Agent がいませんか？）", "디스패치 완료: 실행된 작업이 없습니다(사용 가능한 Agent가 없나요?)", "Распределение завершено: ни одна задача не выполнена (нет доступных Agent?)"),
		KeyToolLegacyDTeamDispatchComplete:        entry("Dispatch complete: %d task(s) executed\n\n", "分派完成：已执行 %d 个任务\n\n", "Dispatch abgeschlossen: %d Aufgabe(n) ausgeführt\n\n", "分配完了：%d 件のタスクを実行しました\n\n", "디스패치 완료: 작업 %d개 실행\n\n", "Распределение завершено: выполнено задач: %d\n\n"),
		KeyToolLegacyDTeamDispatchTaskHeader:      entry("--- Task %d (id=%s, agent=%s) ---\n", "--- 任务 %d（id=%s，Agent=%s）---\n", "--- Aufgabe %d (id=%s, Agent=%s) ---\n", "--- タスク %d（id=%s、Agent=%s）---\n", "--- 작업 %d (id=%s, Agent=%s) ---\n", "--- Задача %d (id=%s, Agent=%s) ---\n"),
		KeyToolLegacyDTeamDispatchError:           entry("ERROR: %s\n", "错误：%s\n", "FEHLER: %s\n", "エラー：%s\n", "오류: %s\n", "ОШИБКА: %s\n"),
	} {
		semanticTranslations[key] = translations
	}
}
