package i18n

const (
	KeyToolAgentPromptRequired           Key = "tool.agent.prompt_required"
	KeyToolAgentDescriptionRequired      Key = "tool.agent.description_required"
	KeyToolAgentMaxDepth                 Key = "tool.agent.max_depth"
	KeyToolAgentTeammateCannotSpawn      Key = "tool.agent.teammate_cannot_spawn"
	KeyToolAgentTeamNameUnavailable      Key = "tool.agent.team_name_unavailable"
	KeyToolAgentTeammateBackground       Key = "tool.agent.teammate_background"
	KeyToolAgentTeammateNameRequired     Key = "tool.agent.teammate_name_required"
	KeyToolAgentTeamMissing              Key = "tool.agent.team_missing"
	KeyToolAgentWorktreeUnavailable      Key = "tool.agent.worktree_unavailable"
	KeyToolAgentIsolationUnsupported     Key = "tool.agent.isolation_unsupported"
	KeyToolAgentBackgroundStartFailed    Key = "tool.agent.background_start_failed"
	KeyToolAgentTeammateSessionsRequired Key = "tool.agent.teammate_sessions_required"
	KeyToolAgentTeamManagerRequired      Key = "tool.agent.team_manager_required"
	KeyToolAgentPersistTeammateFailed    Key = "tool.agent.persist_teammate_failed"
	KeyToolAgentTeammateSpawned          Key = "tool.agent.teammate_spawned"
	KeyToolAgentContinueAsync            Key = "tool.agent.continue_async"
	KeyToolAgentEmptyOutput              Key = "tool.agent.empty_output"
	KeyToolAgentInvalidTyped             Key = "tool.agent.invalid_typed"
	KeyToolAgentReadUnmappable           Key = "tool.agent.read_unmappable"
	KeyToolAgentCompletedMetadata        Key = "tool.agent.completed_metadata"
	KeyToolAgentAsyncPartial             Key = "tool.agent.async_partial"
	KeyToolAgentAsyncOutputHint          Key = "tool.agent.async_output_hint"
	KeyToolAgentAsyncCompletionHint      Key = "tool.agent.async_completion_hint"
)

var toolAgentRuntimeKeys = [...]Key{
	KeyToolAgentPromptRequired,
	KeyToolAgentDescriptionRequired,
	KeyToolAgentMaxDepth,
	KeyToolAgentTeammateCannotSpawn,
	KeyToolAgentTeamNameUnavailable,
	KeyToolAgentTeammateBackground,
	KeyToolAgentTeammateNameRequired,
	KeyToolAgentTeamMissing,
	KeyToolAgentWorktreeUnavailable,
	KeyToolAgentIsolationUnsupported,
	KeyToolAgentBackgroundStartFailed,
	KeyToolAgentTeammateSessionsRequired,
	KeyToolAgentTeamManagerRequired,
	KeyToolAgentPersistTeammateFailed,
	KeyToolAgentTeammateSpawned,
	KeyToolAgentContinueAsync,
	KeyToolAgentEmptyOutput,
	KeyToolAgentInvalidTyped,
	KeyToolAgentReadUnmappable,
	KeyToolAgentCompletedMetadata,
	KeyToolAgentAsyncPartial,
	KeyToolAgentAsyncOutputHint,
	KeyToolAgentAsyncCompletionHint,
}

func init() {
	addToolAgentRuntime(KeyToolAgentPromptRequired, "Error: 'prompt' parameter is required", "错误：必须提供 prompt 参数", "Fehler: Der Parameter prompt ist erforderlich", "エラー: prompt パラメーターは必須です", "오류: prompt 매개변수가 필요합니다", "Ошибка: требуется параметр prompt")
	addToolAgentRuntime(KeyToolAgentDescriptionRequired, "Error: 'description' parameter is required", "错误：必须提供 description 参数", "Fehler: Der Parameter description ist erforderlich", "エラー: description パラメーターは必須です", "오류: description 매개변수가 필요합니다", "Ошибка: требуется параметр description")
	addToolAgentRuntime(KeyToolAgentMaxDepth, "Error: maximum agent nesting depth (%d) exceeded. Cannot spawn sub-agent.", "错误：已超过 Agent 最大嵌套深度（%d），无法生成 sub-agent。", "Fehler: Die maximale Agent-Verschachtelungstiefe (%d) wurde überschritten. Sub-Agent kann nicht gestartet werden.", "エラー: Agent の最大ネスト深度（%d）を超えたため、sub-agent を起動できません。", "오류: Agent 최대 중첩 깊이(%d)를 초과하여 sub-agent를 생성할 수 없습니다.", "Ошибка: превышена максимальная глубина вложенности Agent (%d). Невозможно запустить sub-agent.")
	addToolAgentRuntime(KeyToolAgentTeammateCannotSpawn, "Teammates cannot spawn other teammates - the team roster is flat. To spawn a subagent instead, omit the name parameter.", "Teammate 无法生成其他 teammate；团队成员列表是扁平的。若要生成 subagent，请省略 name 参数。", "Teammates können keine weiteren Teammates starten, da die Teamstruktur flach ist. Zum Starten eines Subagent den Parameter name weglassen.", "teammate は別の teammate を起動できません。チーム構成はフラットです。subagent を起動するには name パラメーターを省略してください。", "teammate는 다른 teammate를 생성할 수 없습니다. 팀 구성은 단일 계층입니다. subagent를 생성하려면 name 매개변수를 생략하세요.", "Teammate не может запускать другого teammate: состав команды одноуровневый. Чтобы запустить subagent, не указывайте параметр name.")
	addToolAgentRuntime(KeyToolAgentTeamNameUnavailable, "The team_name parameter is not available in teammate subagent calls. Omit it to spawn a synchronous subagent.", "teammate 的 subagent 调用不支持 team_name 参数。请省略该参数以生成同步 subagent。", "Der Parameter team_name ist bei Subagent-Aufrufen durch Teammates nicht verfügbar. Für einen synchronen Subagent weglassen.", "teammate からの subagent 呼び出しでは team_name パラメーターを使用できません。同期 subagent を起動するには省略してください。", "teammate의 subagent 호출에서는 team_name 매개변수를 사용할 수 없습니다. 동기 subagent를 생성하려면 생략하세요.", "Параметр team_name недоступен при вызове subagent из teammate. Не указывайте его для запуска синхронного subagent.")
	addToolAgentRuntime(KeyToolAgentTeammateBackground, "In-process teammates cannot spawn background agents. Use run_in_background=false for synchronous subagents.", "进程内 teammate 无法生成后台 Agent。请使用 run_in_background=false 生成同步 subagent。", "Prozessinterne Teammates können keine Hintergrund-Agents starten. Für synchrone Subagents run_in_background=false verwenden.", "プロセス内 teammate はバックグラウンド Agent を起動できません。同期 subagent には run_in_background=false を使用してください。", "프로세스 내 teammate는 백그라운드 Agent를 생성할 수 없습니다. 동기 subagent에는 run_in_background=false를 사용하세요.", "Внутрипроцессный teammate не может запускать фоновые Agent. Для синхронного subagent используйте run_in_background=false.")
	addToolAgentRuntime(KeyToolAgentTeammateNameRequired, "teammate spawning requires a name when team_name is provided. Omit team_name to spawn a regular subagent.", "提供 team_name 时，生成 teammate 必须同时提供 name。若要生成普通 subagent，请省略 team_name。", "Zum Starten eines Teammate ist bei angegebenem team_name ein name erforderlich. Für einen normalen Subagent team_name weglassen.", "team_name を指定して teammate を起動する場合は name が必要です。通常の subagent を起動するには team_name を省略してください。", "team_name을 지정해 teammate를 생성하려면 name이 필요합니다. 일반 subagent를 생성하려면 team_name을 생략하세요.", "Для запуска teammate с параметром team_name требуется name. Чтобы запустить обычный subagent, не указывайте team_name.")
	addToolAgentRuntime(KeyToolAgentTeamMissing, "Team %q does not exist. Create it with TeamCreate before spawning teammates.", "团队 %q 不存在。请先使用 TeamCreate 创建团队，再生成 teammate。", "Team %q ist nicht vorhanden. Vor dem Starten von Teammates mit TeamCreate erstellen.", "チーム %q は存在しません。teammate を起動する前に TeamCreate で作成してください。", "팀 %q이(가) 없습니다. teammate를 생성하기 전에 TeamCreate로 팀을 만드세요.", "Команда %q не существует. Создайте её с помощью TeamCreate перед запуском teammate.")
	addToolAgentRuntime(KeyToolAgentWorktreeUnavailable, `Agent error: isolation="worktree" is unavailable in this runtime; configure a worktree-capable provider before requesting isolation`, `Agent 错误：当前 runtime 不支持 isolation="worktree"；请先配置支持 worktree 的 provider，再请求隔离`, `Agent-Fehler: isolation="worktree" ist in dieser Runtime nicht verfügbar; vor der Isolationsanforderung einen worktree-fähigen Provider konfigurieren`, `Agent エラー: この runtime では isolation="worktree" を使用できません。隔離を要求する前に worktree 対応 provider を設定してください`, `Agent 오류: 이 runtime에서는 isolation="worktree"를 사용할 수 없습니다. 격리를 요청하기 전에 worktree 지원 provider를 구성하세요`, `Ошибка Agent: isolation="worktree" недоступна в этой runtime; перед запросом изоляции настройте provider с поддержкой worktree`)
	addToolAgentRuntime(KeyToolAgentIsolationUnsupported, "Agent error: unsupported isolation mode %q (expected worktree)", "Agent 错误：不支持 isolation 模式 %q（应为 worktree）", "Agent-Fehler: Nicht unterstützter Isolationsmodus %q (erwartet: worktree)", "Agent エラー: isolation モード %q はサポートされていません（worktree を指定してください）", "Agent 오류: 지원하지 않는 isolation 모드 %q입니다(worktree 필요)", "Ошибка Agent: режим isolation %q не поддерживается (ожидается worktree)")
	addToolAgentRuntime(KeyToolAgentBackgroundStartFailed, "failed to start background agent: %v", "无法启动后台 Agent：%v", "Hintergrund-Agent konnte nicht gestartet werden: %v", "バックグラウンド Agent を開始できませんでした: %v", "백그라운드 Agent를 시작하지 못했습니다: %v", "Не удалось запустить фоновый Agent: %v")
	addToolAgentRuntime(KeyToolAgentTeammateSessionsRequired, "teammate spawning requires background agent sessions", "生成 teammate 需要后台 Agent session", "Zum Starten von Teammates sind Hintergrund-Agent-Sitzungen erforderlich", "teammate の起動にはバックグラウンド Agent session が必要です", "teammate 생성에는 백그라운드 Agent session이 필요합니다", "Для запуска teammate требуются фоновые сессии Agent")
	addToolAgentRuntime(KeyToolAgentTeamManagerRequired, "teammate spawning requires an active team manager", "生成 teammate 需要处于活动状态的 team manager", "Zum Starten von Teammates ist ein aktiver Team-Manager erforderlich", "teammate の起動には有効な team manager が必要です", "teammate 생성에는 활성 team manager가 필요합니다", "Для запуска teammate требуется активный менеджер команды")
	addToolAgentRuntime(KeyToolAgentPersistTeammateFailed, "failed to persist teammate in team config: %s", "无法将 teammate 保存到团队配置：%s", "Teammate konnte nicht in der Teamkonfiguration gespeichert werden: %s", "teammate をチーム設定に保存できませんでした: %s", "teammate를 팀 설정에 저장하지 못했습니다: %s", "Не удалось сохранить teammate в конфигурации команды: %s")
	addToolAgentRuntime(KeyToolAgentTeammateSpawned, "Teammate spawned and running in the background.", "Teammate 已生成，正在后台运行。", "Teammate wurde gestartet und läuft im Hintergrund.", "teammate を起動し、バックグラウンドで実行しています。", "teammate가 생성되어 백그라운드에서 실행 중입니다.", "Teammate запущен и работает в фоне.")
	addToolAgentRuntime(KeyToolAgentContinueAsync, "Use SendMessage with to: %q to continue this agent. The agent is working in the background and will notify when it completes.", "要继续此 Agent，请使用 SendMessage，并将 to 设为 %q。Agent 正在后台运行，完成后会发出通知。", "Zum Fortsetzen dieses Agent SendMessage mit to: %q verwenden. Der Agent arbeitet im Hintergrund und meldet sich nach Abschluss.", "この Agent を続行するには、to: %q を指定して SendMessage を使用してください。Agent はバックグラウンドで動作しており、完了時に通知します。", "이 Agent를 계속하려면 to: %q로 SendMessage를 사용하세요. Agent는 백그라운드에서 작업 중이며 완료되면 알립니다.", "Чтобы продолжить работу этого Agent, используйте SendMessage с to: %q. Agent работает в фоне и уведомит о завершении.")
	addToolAgentRuntime(KeyToolAgentEmptyOutput, "(Subagent completed but returned no output.)", "（Subagent 已完成，但未返回任何输出。）", "(Subagent wurde abgeschlossen, hat aber keine Ausgabe zurückgegeben.)", "（Subagent は完了しましたが、出力はありませんでした。）", "(Subagent가 완료되었지만 출력이 없습니다.)", "(Subagent завершён, но не вернул вывод.)")
	addToolAgentRuntime(KeyToolAgentInvalidTyped, "Agent returned an invalid typed result", "Agent 返回了无效的类型化结果", "Agent hat ein ungültiges typisiertes Ergebnis zurückgegeben", "Agent が無効な型付き結果を返しました", "Agent가 잘못된 형식의 결과를 반환했습니다", "Agent вернул недопустимый типизированный результат")
	addToolAgentRuntime(KeyToolAgentReadUnmappable, "Read returned an unmappable result", "Read 返回了无法映射的结果", "Read hat ein nicht abbildbares Ergebnis zurückgegeben", "Read がマッピングできない結果を返しました", "Read가 매핑할 수 없는 결과를 반환했습니다", "Read вернул результат, который невозможно сопоставить")
	addToolAgentRuntime(KeyToolAgentCompletedMetadata,
		"agentId: %s (use SendMessage with to: '%s' to continue this agent)%s\n<usage>total_tokens: %d\ntool_uses: %d\nduration_ms: %d</usage>",
		"agentId: %s（要继续此 Agent，请使用 SendMessage，并将 to 设为 '%s'）%s\n<usage>total_tokens: %d\ntool_uses: %d\nduration_ms: %d</usage>",
		"agentId: %s (zum Fortsetzen dieses Agent SendMessage mit to: '%s' verwenden)%s\n<usage>total_tokens: %d\ntool_uses: %d\nduration_ms: %d</usage>",
		"agentId: %s（この Agent を続行するには、to: '%s' を指定して SendMessage を使用）%s\n<usage>total_tokens: %d\ntool_uses: %d\nduration_ms: %d</usage>",
		"agentId: %s(이 Agent를 계속하려면 to: '%s'로 SendMessage 사용)%s\n<usage>total_tokens: %d\ntool_uses: %d\nduration_ms: %d</usage>",
		"agentId: %s (для продолжения этого Agent используйте SendMessage с to: '%s')%s\n<usage>total_tokens: %d\ntool_uses: %d\nduration_ms: %d</usage>")
	addToolAgentRuntime(KeyToolAgentAsyncPartial,
		"Async agent launched successfully.\nagentId: %s (internal ID - do not mention to user. Use SendMessage with to: '%s' to continue this agent.)\nThe agent is working in the background. You will be notified automatically when it completes.",
		"异步 Agent 已成功启动。\nagentId: %s（内部 ID，请勿向用户提及。要继续此 Agent，请使用 SendMessage，并将 to 设为 '%s'。）\nAgent 正在后台工作，完成后会自动通知。",
		"Asynchroner Agent erfolgreich gestartet.\nagentId: %s (interne ID – nicht gegenüber dem Benutzer erwähnen. Zum Fortsetzen dieses Agent SendMessage mit to: '%s' verwenden.)\nDer Agent arbeitet im Hintergrund; nach Abschluss erfolgt automatisch eine Benachrichtigung.",
		"非同期 Agent を起動しました。\nagentId: %s（内部 ID。ユーザーには伝えないでください。この Agent を続行するには、to: '%s' を指定して SendMessage を使用します。）\nAgent はバックグラウンドで動作中です。完了時に自動で通知されます。",
		"비동기 Agent를 시작했습니다.\nagentId: %s(내부 ID이므로 사용자에게 언급하지 마세요. 이 Agent를 계속하려면 to: '%s'로 SendMessage를 사용하세요.)\nAgent가 백그라운드에서 작업 중이며 완료되면 자동으로 알림을 받습니다.",
		"Асинхронный Agent успешно запущен.\nagentId: %s (внутренний ID — не сообщайте его пользователю. Для продолжения этого Agent используйте SendMessage с to: '%s'.)\nAgent работает в фоне; после завершения придёт автоматическое уведомление.")
	addToolAgentRuntime(KeyToolAgentAsyncOutputHint,
		"\nDo not duplicate this agent's work. Work on non-overlapping tasks, or briefly tell the user what you launched and end your response.\noutput_file: %s",
		"\n不要重复此 Agent 的工作。请处理不重叠的任务，或简要告知用户已启动的任务，然后结束回复。\noutput_file: %s",
		"\nDupliziere die Arbeit dieses Agent nicht. Bearbeite nicht überlappende Aufgaben oder teile dem Benutzer kurz mit, was gestartet wurde, und beende die Antwort.\noutput_file: %s",
		"\nこの Agent の作業を重複して行わないでください。重ならないタスクを進めるか、起動した内容をユーザーに簡潔に伝えて応答を終了してください。\noutput_file: %s",
		"\n이 Agent의 작업을 중복하지 마세요. 겹치지 않는 작업을 수행하거나 사용자에게 시작한 작업을 간단히 알리고 응답을 끝내세요.\noutput_file: %s",
		"\nНе дублируйте работу этого Agent. Выполняйте непересекающиеся задачи либо кратко сообщите пользователю, что было запущено, и завершите ответ.\noutput_file: %s")
	addToolAgentRuntime(KeyToolAgentAsyncCompletionHint,
		"\nBriefly tell the user what you launched and end your response. Do not generate any other text; agent results will arrive in a subsequent message.",
		"\n请简要告知用户已启动的任务，然后结束回复。不要生成其他内容；Agent 结果会在后续消息中送达。",
		"\nTeile dem Benutzer kurz mit, was gestartet wurde, und beende die Antwort. Erzeuge keinen weiteren Text; die Agent-Ergebnisse treffen in einer späteren Nachricht ein.",
		"\n起動した内容をユーザーに簡潔に伝え、応答を終了してください。それ以外のテキストは生成しないでください。Agent の結果は後続メッセージで届きます。",
		"\n사용자에게 시작한 작업을 간단히 알리고 응답을 끝내세요. 다른 텍스트는 생성하지 마세요. Agent 결과는 후속 메시지로 도착합니다.",
		"\nКратко сообщите пользователю, что было запущено, и завершите ответ. Не создавайте другой текст; результаты Agent поступят в следующем сообщении.")
}

func addToolAgentRuntime(key Key, en, zh, de, ja, ko, ru string) {
	semanticTranslations[key] = map[Language]string{
		LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
	}
}
