package i18n

// Semantic copy for legacy tool result paths migrated as one bounded batch.
// Tool names, paths, identifiers, media types, sizes, and raw external errors
// remain format arguments so their canonical values are never translated.
const (
	KeyToolLegacyAAgentPromptRequired           Key = "tool.legacy_a.agent.prompt_required"
	KeyToolLegacyAAgentDescriptionRequired      Key = "tool.legacy_a.agent.description_required"
	KeyToolLegacyAAgentMaxDepth                 Key = "tool.legacy_a.agent.max_depth"
	KeyToolLegacyAAgentTeammateCannotSpawn      Key = "tool.legacy_a.agent.teammate_cannot_spawn"
	KeyToolLegacyAAgentTeamNameUnavailable      Key = "tool.legacy_a.agent.team_name_unavailable"
	KeyToolLegacyAAgentTeammateBackground       Key = "tool.legacy_a.agent.teammate_background"
	KeyToolLegacyAAgentTeammateNameRequired     Key = "tool.legacy_a.agent.teammate_name_required"
	KeyToolLegacyAAgentTeamMissing              Key = "tool.legacy_a.agent.team_missing"
	KeyToolLegacyAAgentWorktreeUnavailable      Key = "tool.legacy_a.agent.worktree_unavailable"
	KeyToolLegacyAAgentRemoteUnavailable        Key = "tool.legacy_a.agent.remote_unavailable"
	KeyToolLegacyAAgentIsolationUnsupported     Key = "tool.legacy_a.agent.isolation_unsupported"
	KeyToolLegacyAAgentRemotePermissionSnapshot Key = "tool.legacy_a.agent.remote_permission_snapshot"
	KeyToolLegacyAAgentRemoteSpawnFailed        Key = "tool.legacy_a.agent.remote_spawn_failed"
	KeyToolLegacyAAgentBackgroundStartFailed    Key = "tool.legacy_a.agent.background_start_failed"
	KeyToolLegacyAAgentTeammateSessionsRequired Key = "tool.legacy_a.agent.teammate_sessions_required"
	KeyToolLegacyAAgentTeamManagerRequired      Key = "tool.legacy_a.agent.team_manager_required"
	KeyToolLegacyAAgentSpawnTeammateFailed      Key = "tool.legacy_a.agent.spawn_teammate_failed"
	KeyToolLegacyAAgentPersistTeammateFailed    Key = "tool.legacy_a.agent.persist_teammate_failed"
	KeyToolLegacyAAgentStartTeammateFailed      Key = "tool.legacy_a.agent.start_teammate_failed"
	KeyToolLegacyAAgentMarshalTeammateFailed    Key = "tool.legacy_a.agent.marshal_teammate_failed"
	KeyToolLegacyAAgentTeammateSpawned          Key = "tool.legacy_a.agent.teammate_spawned"
	KeyToolLegacyAAgentContinueAsync            Key = "tool.legacy_a.agent.continue_async"
	KeyToolLegacyAAgentRemoteLaunched           Key = "tool.legacy_a.agent.remote_launched"
	KeyToolLegacyAAgentEmptyOutput              Key = "tool.legacy_a.agent.empty_output"
	KeyToolLegacyAAgentInvalidTyped             Key = "tool.legacy_a.agent.invalid_typed"
	KeyToolLegacyAAgentReadUnmappable           Key = "tool.legacy_a.agent.read_unmappable"
	KeyToolLegacyAAgentCompletedMetadata        Key = "tool.legacy_a.agent.completed_metadata"
	KeyToolLegacyAAgentRemotePartial            Key = "tool.legacy_a.agent.remote_partial"
	KeyToolLegacyAAgentAsyncPartial             Key = "tool.legacy_a.agent.async_partial"
	KeyToolLegacyAAgentAsyncOutputHint          Key = "tool.legacy_a.agent.async_output_hint"
	KeyToolLegacyAAgentAsyncCompletionHint      Key = "tool.legacy_a.agent.async_completion_hint"

	KeyToolLegacyAConfigEmpty         Key = "tool.legacy_a.config.empty"
	KeyToolLegacyAConfigNotSet        Key = "tool.legacy_a.config.not_set"
	KeyToolLegacyAConfigSetRequired   Key = "tool.legacy_a.config.set_required"
	KeyToolLegacyAConfigSaveFailed    Key = "tool.legacy_a.config.save_failed"
	KeyToolLegacyAConfigUpdated       Key = "tool.legacy_a.config.updated"
	KeyToolLegacyAConfigUnknownAction Key = "tool.legacy_a.config.unknown_action"

	KeyToolLegacyABase64DecodeFailed       Key = "tool.legacy_a.data.base64_decode_failed"
	KeyToolLegacyAHashUnsupported          Key = "tool.legacy_a.data.hash_unsupported"
	KeyToolLegacyAJSONInvalid              Key = "tool.legacy_a.data.json_invalid"
	KeyToolLegacyAJSONFormattingFailed     Key = "tool.legacy_a.data.json_formatting_failed"
	KeyToolLegacyAJSONPathNotFound         Key = "tool.legacy_a.data.json_path_not_found"
	KeyToolLegacyAHexDecodeFailed          Key = "tool.legacy_a.data.hex_decode_failed"
	KeyToolLegacyACronTooMany              Key = "tool.legacy_a.cron.too_many"
	KeyToolLegacyACronDurableLeaderOnly    Key = "tool.legacy_a.cron.durable_leader_only"
	KeyToolLegacyACronDeleteFailed         Key = "tool.legacy_a.cron.delete_failed"
	KeyToolLegacyACronErrorCode            Key = "tool.legacy_a.cron.error_code"
	KeyToolLegacyAExitPlanNotActive        Key = "tool.legacy_a.exit_plan.not_active"
	KeyToolLegacyAExitPlanInvalidTypedData Key = "tool.legacy_a.exit_plan.invalid_typed_data"
	KeyToolLegacyAExitPlanAwaitingLeader   Key = "tool.legacy_a.exit_plan.awaiting_leader"
	KeyToolLegacyAExitPlanAgentApproved    Key = "tool.legacy_a.exit_plan.agent_approved"
	KeyToolLegacyAExitPlanApproved         Key = "tool.legacy_a.exit_plan.approved"
	KeyToolLegacyAExitPlanTeamHint         Key = "tool.legacy_a.exit_plan.team_hint"
	KeyToolLegacyAExitPlanLabel            Key = "tool.legacy_a.exit_plan.label"
	KeyToolLegacyAExitPlanEditedLabel      Key = "tool.legacy_a.exit_plan.edited_label"
	KeyToolLegacyAExitPlanApprovedBody     Key = "tool.legacy_a.exit_plan.approved_body"
	KeyToolLegacyAExitPlanRejected         Key = "tool.legacy_a.exit_plan.rejected"
	KeyToolLegacyAExitPlanFeedback         Key = "tool.legacy_a.exit_plan.feedback"
	KeyToolLegacyAExitPlanStateRequired    Key = "tool.legacy_a.exit_plan.state_required"
	KeyToolLegacyAExitPlanPathMismatch     Key = "tool.legacy_a.exit_plan.path_mismatch"
	KeyToolLegacyAExitPlanNoActiveFile     Key = "tool.legacy_a.exit_plan.no_active_file"
	KeyToolLegacyAExitPlanReadFile         Key = "tool.legacy_a.exit_plan.read_file"
	KeyToolLegacyAExitPlanMarshalInput     Key = "tool.legacy_a.exit_plan.marshal_input"
	KeyToolLegacyAExitPlanInvalidInput     Key = "tool.legacy_a.exit_plan.invalid_input"
	KeyToolLegacyAExitPlanInvalidPrompts   Key = "tool.legacy_a.exit_plan.invalid_prompts"
	KeyToolLegacyAExitPlanReadBeforeExit   Key = "tool.legacy_a.exit_plan.read_before_exit"
	KeyToolLegacyAExitPlanPersistEdited    Key = "tool.legacy_a.exit_plan.persist_edited"
	KeyToolLegacyAExitPlanCommitRollback   Key = "tool.legacy_a.exit_plan.commit_rollback"
	KeyToolLegacyAExitPlanCommit           Key = "tool.legacy_a.exit_plan.commit"

	KeyToolLegacyAFileInvalidInput          Key = "tool.legacy_a.file.invalid_input"
	KeyToolLegacyAFileOffsetNonNegative     Key = "tool.legacy_a.file.offset_non_negative"
	KeyToolLegacyAFileLimitNonNegative      Key = "tool.legacy_a.file.limit_non_negative"
	KeyToolLegacyAFileLimitPositive         Key = "tool.legacy_a.file.limit_positive"
	KeyToolLegacyAFilePagesInvalid          Key = "tool.legacy_a.file.pages_invalid"
	KeyToolLegacyAFilePageRangeTooLarge     Key = "tool.legacy_a.file.page_range_too_large"
	KeyToolLegacyAFileDirectoryDenied       Key = "tool.legacy_a.file.directory_denied"
	KeyToolLegacyAFileUNCRequiresPermission Key = "tool.legacy_a.file.unc_requires_permission"
	KeyToolLegacyAFileDeviceBlocked         Key = "tool.legacy_a.file.device_blocked"
	KeyToolLegacyAFileBinaryUnsupported     Key = "tool.legacy_a.file.binary_unsupported"
	KeyToolLegacyAFileOpenFailed            Key = "tool.legacy_a.file.open_failed"
	KeyToolLegacyAFilePathVerification      Key = "tool.legacy_a.file.path_verification_failed"
	KeyToolLegacyAFileReadFailed            Key = "tool.legacy_a.file.read_failed"
	KeyToolLegacyAFileTooLarge              Key = "tool.legacy_a.file.too_large"
	KeyToolLegacyAFileNotFound              Key = "tool.legacy_a.file.not_found"
	KeyToolLegacyAFileNotFoundInCWD         Key = "tool.legacy_a.file.not_found_in_cwd"
	KeyToolLegacyAFileNotFoundSuggestion    Key = "tool.legacy_a.file.not_found_suggestion"
	KeyToolLegacyAFilePlanModeBlocked       Key = "tool.legacy_a.file.plan_mode_blocked"
	KeyToolLegacyAFilePathRequired          Key = "tool.legacy_a.file.path_required"
	KeyToolLegacyAFileContentRequired       Key = "tool.legacy_a.file.content_required"
	KeyToolLegacyAFileResolveFailed         Key = "tool.legacy_a.file.resolve_failed"
	KeyToolLegacyAFileOutsideAllowed        Key = "tool.legacy_a.file.outside_allowed"
	KeyToolLegacyAFileTeamMemorySecret      Key = "tool.legacy_a.file.team_memory_secret"
	KeyToolLegacyAFileCreateDirectoryFailed Key = "tool.legacy_a.file.create_directory_failed"
	KeyToolLegacyAFileNotReadForWrite       Key = "tool.legacy_a.file.not_read_for_write"
	KeyToolLegacyAFilePartiallyReadForWrite Key = "tool.legacy_a.file.partially_read_for_write"
	KeyToolLegacyAFileChangedForWrite       Key = "tool.legacy_a.file.changed_for_write"
	KeyToolLegacyAFileWriteFailed           Key = "tool.legacy_a.file.write_failed"
	KeyToolLegacyAFileAppendFailed          Key = "tool.legacy_a.file.append_failed"
	KeyToolLegacyAFileDeleteFailed          Key = "tool.legacy_a.file.delete_failed"
	KeyToolLegacyAFileListFailed            Key = "tool.legacy_a.file.list_failed"
	KeyToolLegacyAFileGlobInvalid           Key = "tool.legacy_a.file.glob_invalid"
	KeyToolLegacyAFileMoveFailed            Key = "tool.legacy_a.file.move_failed"
	KeyToolLegacyAFileSymlinkFailed         Key = "tool.legacy_a.file.symlink_failed"
	KeyToolLegacyAFileReadInvalidTyped      Key = "tool.legacy_a.file.read_invalid_typed"
	KeyToolLegacyAFilePDFRead               Key = "tool.legacy_a.file.pdf_read"
	KeyToolLegacyAFilePDFPagesExtracted     Key = "tool.legacy_a.file.pdf_pages_extracted"
	KeyToolLegacyAImagePlaceholder          Key = "tool.legacy_a.image.placeholder"
)

var toolLegacyAKeys = []Key{
	KeyToolLegacyAAgentPromptRequired, KeyToolLegacyAAgentDescriptionRequired,
	KeyToolLegacyAAgentMaxDepth, KeyToolLegacyAAgentTeammateCannotSpawn,
	KeyToolLegacyAAgentTeamNameUnavailable, KeyToolLegacyAAgentTeammateBackground,
	KeyToolLegacyAAgentTeammateNameRequired, KeyToolLegacyAAgentTeamMissing,
	KeyToolLegacyAAgentWorktreeUnavailable, KeyToolLegacyAAgentRemoteUnavailable,
	KeyToolLegacyAAgentIsolationUnsupported, KeyToolLegacyAAgentRemotePermissionSnapshot,
	KeyToolLegacyAAgentRemoteSpawnFailed, KeyToolLegacyAAgentBackgroundStartFailed,
	KeyToolLegacyAAgentTeammateSessionsRequired, KeyToolLegacyAAgentTeamManagerRequired,
	KeyToolLegacyAAgentSpawnTeammateFailed, KeyToolLegacyAAgentPersistTeammateFailed,
	KeyToolLegacyAAgentStartTeammateFailed, KeyToolLegacyAAgentMarshalTeammateFailed,
	KeyToolLegacyAAgentTeammateSpawned, KeyToolLegacyAAgentContinueAsync,
	KeyToolLegacyAAgentRemoteLaunched, KeyToolLegacyAAgentEmptyOutput,
	KeyToolLegacyAAgentInvalidTyped,
	KeyToolLegacyAAgentReadUnmappable,
	KeyToolLegacyAAgentCompletedMetadata, KeyToolLegacyAAgentRemotePartial,
	KeyToolLegacyAAgentAsyncPartial, KeyToolLegacyAAgentAsyncOutputHint,
	KeyToolLegacyAAgentAsyncCompletionHint,
	KeyToolLegacyAConfigEmpty, KeyToolLegacyAConfigNotSet, KeyToolLegacyAConfigSetRequired,
	KeyToolLegacyAConfigSaveFailed, KeyToolLegacyAConfigUpdated, KeyToolLegacyAConfigUnknownAction,
	KeyToolLegacyABase64DecodeFailed, KeyToolLegacyAHashUnsupported, KeyToolLegacyAJSONInvalid,
	KeyToolLegacyAJSONFormattingFailed, KeyToolLegacyAJSONPathNotFound, KeyToolLegacyAHexDecodeFailed,
	KeyToolLegacyACronTooMany, KeyToolLegacyACronDurableLeaderOnly, KeyToolLegacyACronDeleteFailed,
	KeyToolLegacyACronErrorCode,
	KeyToolLegacyAExitPlanNotActive, KeyToolLegacyAExitPlanInvalidTypedData,
	KeyToolLegacyAExitPlanAwaitingLeader, KeyToolLegacyAExitPlanAgentApproved,
	KeyToolLegacyAExitPlanApproved, KeyToolLegacyAExitPlanTeamHint,
	KeyToolLegacyAExitPlanLabel, KeyToolLegacyAExitPlanEditedLabel,
	KeyToolLegacyAExitPlanApprovedBody, KeyToolLegacyAExitPlanRejected,
	KeyToolLegacyAExitPlanFeedback,
	KeyToolLegacyAExitPlanStateRequired, KeyToolLegacyAExitPlanPathMismatch,
	KeyToolLegacyAExitPlanNoActiveFile, KeyToolLegacyAExitPlanReadFile,
	KeyToolLegacyAExitPlanMarshalInput, KeyToolLegacyAExitPlanInvalidInput,
	KeyToolLegacyAExitPlanInvalidPrompts, KeyToolLegacyAExitPlanReadBeforeExit,
	KeyToolLegacyAExitPlanPersistEdited, KeyToolLegacyAExitPlanCommitRollback,
	KeyToolLegacyAExitPlanCommit,
	KeyToolLegacyAFileInvalidInput, KeyToolLegacyAFileOffsetNonNegative,
	KeyToolLegacyAFileLimitNonNegative, KeyToolLegacyAFileLimitPositive,
	KeyToolLegacyAFilePagesInvalid, KeyToolLegacyAFilePageRangeTooLarge,
	KeyToolLegacyAFileDirectoryDenied, KeyToolLegacyAFileUNCRequiresPermission,
	KeyToolLegacyAFileDeviceBlocked, KeyToolLegacyAFileBinaryUnsupported,
	KeyToolLegacyAFileOpenFailed, KeyToolLegacyAFilePathVerification,
	KeyToolLegacyAFileReadFailed, KeyToolLegacyAFileTooLarge,
	KeyToolLegacyAFileNotFound, KeyToolLegacyAFileNotFoundInCWD,
	KeyToolLegacyAFileNotFoundSuggestion, KeyToolLegacyAFilePlanModeBlocked,
	KeyToolLegacyAFilePathRequired, KeyToolLegacyAFileContentRequired,
	KeyToolLegacyAFileResolveFailed, KeyToolLegacyAFileTeamMemorySecret,
	KeyToolLegacyAFileOutsideAllowed,
	KeyToolLegacyAFileCreateDirectoryFailed, KeyToolLegacyAFileNotReadForWrite,
	KeyToolLegacyAFilePartiallyReadForWrite, KeyToolLegacyAFileChangedForWrite,
	KeyToolLegacyAFileWriteFailed, KeyToolLegacyAFileAppendFailed,
	KeyToolLegacyAFileDeleteFailed, KeyToolLegacyAFileListFailed,
	KeyToolLegacyAFileGlobInvalid, KeyToolLegacyAFileMoveFailed,
	KeyToolLegacyAFileSymlinkFailed, KeyToolLegacyAFileReadInvalidTyped,
	KeyToolLegacyAFilePDFRead, KeyToolLegacyAFilePDFPagesExtracted,
	KeyToolLegacyAImagePlaceholder,
}

func init() {
	addToolLegacyA := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}

	addToolLegacyA(KeyToolLegacyAAgentPromptRequired, "Error: 'prompt' parameter is required", "错误：必须提供 prompt 参数", "Fehler: Der Parameter prompt ist erforderlich", "エラー: prompt パラメーターは必須です", "오류: prompt 매개변수가 필요합니다", "Ошибка: требуется параметр prompt")
	addToolLegacyA(KeyToolLegacyAAgentDescriptionRequired, "Error: 'description' parameter is required", "错误：必须提供 description 参数", "Fehler: Der Parameter description ist erforderlich", "エラー: description パラメーターは必須です", "오류: description 매개변수가 필요합니다", "Ошибка: требуется параметр description")
	addToolLegacyA(KeyToolLegacyAAgentMaxDepth, "Error: maximum agent nesting depth (%d) exceeded. Cannot spawn sub-agent.", "错误：已超过 Agent 最大嵌套深度（%d），无法生成 sub-agent。", "Fehler: Die maximale Agent-Verschachtelungstiefe (%d) wurde überschritten. Sub-Agent kann nicht gestartet werden.", "エラー: Agent の最大ネスト深度（%d）を超えたため、sub-agent を起動できません。", "오류: Agent 최대 중첩 깊이(%d)를 초과하여 sub-agent를 생성할 수 없습니다.", "Ошибка: превышена максимальная глубина вложенности Agent (%d). Невозможно запустить sub-agent.")
	addToolLegacyA(KeyToolLegacyAAgentTeammateCannotSpawn, "Teammates cannot spawn other teammates - the team roster is flat. To spawn a subagent instead, omit the name parameter.", "Teammate 无法生成其他 teammate；团队成员列表是扁平的。若要生成 subagent，请省略 name 参数。", "Teammates können keine weiteren Teammates starten, da die Teamstruktur flach ist. Zum Starten eines Subagent den Parameter name weglassen.", "teammate は別の teammate を起動できません。チーム構成はフラットです。subagent を起動するには name パラメーターを省略してください。", "teammate는 다른 teammate를 생성할 수 없습니다. 팀 구성은 단일 계층입니다. subagent를 생성하려면 name 매개변수를 생략하세요.", "Teammate не может запускать другого teammate: состав команды одноуровневый. Чтобы запустить subagent, не указывайте параметр name.")
	addToolLegacyA(KeyToolLegacyAAgentTeamNameUnavailable, "The team_name parameter is not available in teammate subagent calls. Omit it to spawn a synchronous subagent.", "teammate 的 subagent 调用不支持 team_name 参数。请省略该参数以生成同步 subagent。", "Der Parameter team_name ist bei Subagent-Aufrufen durch Teammates nicht verfügbar. Für einen synchronen Subagent weglassen.", "teammate からの subagent 呼び出しでは team_name パラメーターを使用できません。同期 subagent を起動するには省略してください。", "teammate의 subagent 호출에서는 team_name 매개변수를 사용할 수 없습니다. 동기 subagent를 생성하려면 생략하세요.", "Параметр team_name недоступен при вызове subagent из teammate. Не указывайте его для запуска синхронного subagent.")
	addToolLegacyA(KeyToolLegacyAAgentTeammateBackground, "In-process teammates cannot spawn background agents. Use run_in_background=false for synchronous subagents.", "进程内 teammate 无法生成后台 Agent。请使用 run_in_background=false 生成同步 subagent。", "Prozessinterne Teammates können keine Hintergrund-Agents starten. Für synchrone Subagents run_in_background=false verwenden.", "プロセス内 teammate はバックグラウンド Agent を起動できません。同期 subagent には run_in_background=false を使用してください。", "프로세스 내 teammate는 백그라운드 Agent를 생성할 수 없습니다. 동기 subagent에는 run_in_background=false를 사용하세요.", "Внутрипроцессный teammate не может запускать фоновые Agent. Для синхронного subagent используйте run_in_background=false.")
	addToolLegacyA(KeyToolLegacyAAgentTeammateNameRequired, "teammate spawning requires a name when team_name is provided. Omit team_name to spawn a regular subagent.", "提供 team_name 时，生成 teammate 必须同时提供 name。若要生成普通 subagent，请省略 team_name。", "Zum Starten eines Teammate ist bei angegebenem team_name ein name erforderlich. Für einen normalen Subagent team_name weglassen.", "team_name を指定して teammate を起動する場合は name が必要です。通常の subagent を起動するには team_name を省略してください。", "team_name을 지정해 teammate를 생성하려면 name이 필요합니다. 일반 subagent를 생성하려면 team_name을 생략하세요.", "Для запуска teammate с параметром team_name требуется name. Чтобы запустить обычный subagent, не указывайте team_name.")
	addToolLegacyA(KeyToolLegacyAAgentTeamMissing, "Team %q does not exist. Create it with TeamCreate before spawning teammates.", "团队 %q 不存在。请先使用 TeamCreate 创建团队，再生成 teammate。", "Team %q ist nicht vorhanden. Vor dem Starten von Teammates mit TeamCreate erstellen.", "チーム %q は存在しません。teammate を起動する前に TeamCreate で作成してください。", "팀 %q이(가) 없습니다. teammate를 생성하기 전에 TeamCreate로 팀을 만드세요.", "Команда %q не существует. Создайте её с помощью TeamCreate перед запуском teammate.")
	addToolLegacyA(KeyToolLegacyAAgentWorktreeUnavailable, `Agent error: isolation="worktree" is unavailable in this runtime; configure a worktree-capable provider before requesting isolation`, `Agent 错误：当前 runtime 不支持 isolation="worktree"；请先配置支持 worktree 的 provider，再请求隔离`, `Agent-Fehler: isolation="worktree" ist in dieser Runtime nicht verfügbar; vor der Isolationsanforderung einen worktree-fähigen Provider konfigurieren`, `Agent エラー: この runtime では isolation="worktree" を使用できません。隔離を要求する前に worktree 対応 provider を設定してください`, `Agent 오류: 이 runtime에서는 isolation="worktree"를 사용할 수 없습니다. 격리를 요청하기 전에 worktree 지원 provider를 구성하세요`, `Ошибка Agent: isolation="worktree" недоступна в этой runtime; перед запросом изоляции настройте provider с поддержкой worktree`)
	addToolLegacyA(KeyToolLegacyAAgentRemoteUnavailable, `Agent error: isolation="remote" requires a RemoteRuntimeProvider; configure AgentTool.RemoteRuntime to enable remote sub-agents`, `Agent 错误：isolation="remote" 需要 RemoteRuntimeProvider；请配置 AgentTool.RemoteRuntime 以启用远程 sub-agent`, `Agent-Fehler: isolation="remote" erfordert einen RemoteRuntimeProvider; AgentTool.RemoteRuntime konfigurieren, um Remote-Sub-Agents zu aktivieren`, `Agent エラー: isolation="remote" には RemoteRuntimeProvider が必要です。リモート sub-agent を有効にするには AgentTool.RemoteRuntime を設定してください`, `Agent 오류: isolation="remote"에는 RemoteRuntimeProvider가 필요합니다. 원격 sub-agent를 사용하려면 AgentTool.RemoteRuntime을 구성하세요`, `Ошибка Agent: для isolation="remote" требуется RemoteRuntimeProvider; настройте AgentTool.RemoteRuntime, чтобы включить удалённые sub-agent`)
	addToolLegacyA(KeyToolLegacyAAgentIsolationUnsupported, "Agent error: unsupported isolation mode %q (expected worktree or remote)", "Agent 错误：不支持 isolation 模式 %q（应为 worktree 或 remote）", "Agent-Fehler: Nicht unterstützter Isolationsmodus %q (erwartet: worktree oder remote)", "Agent エラー: isolation モード %q はサポートされていません（worktree または remote を指定してください）", "Agent 오류: 지원하지 않는 isolation 모드 %q입니다(worktree 또는 remote 필요)", "Ошибка Agent: режим isolation %q не поддерживается (ожидается worktree или remote)")
	addToolLegacyA(KeyToolLegacyAAgentRemotePermissionSnapshot, "Agent error: remote subagent requires a complete parent permission snapshot with project scope", "Agent 错误：远程 subagent 需要包含项目范围的完整父级权限快照", "Agent-Fehler: Ein Remote-Subagent benötigt einen vollständigen Berechtigungs-Snapshot des Elternprozesses mit Projektbereich", "Agent エラー: リモート subagent にはプロジェクト範囲を含む完全な親権限スナップショットが必要です", "Agent 오류: 원격 subagent에는 프로젝트 범위가 포함된 완전한 상위 권한 스냅샷이 필요합니다", "Ошибка Agent: удалённому subagent требуется полный снимок родительских разрешений с областью проекта")
	addToolLegacyA(KeyToolLegacyAAgentRemoteSpawnFailed, "remote agent spawn failed: %v", "无法生成远程 Agent：%v", "Remote-Agent konnte nicht gestartet werden: %v", "リモート Agent を起動できませんでした: %v", "원격 Agent를 생성하지 못했습니다: %v", "Не удалось запустить удалённый Agent: %v")
	addToolLegacyA(KeyToolLegacyAAgentBackgroundStartFailed, "failed to start background agent: %v", "无法启动后台 Agent：%v", "Hintergrund-Agent konnte nicht gestartet werden: %v", "バックグラウンド Agent を開始できませんでした: %v", "백그라운드 Agent를 시작하지 못했습니다: %v", "Не удалось запустить фоновый Agent: %v")
	addToolLegacyA(KeyToolLegacyAAgentTeammateSessionsRequired, "teammate spawning requires background agent sessions", "生成 teammate 需要后台 Agent session", "Zum Starten von Teammates sind Hintergrund-Agent-Sitzungen erforderlich", "teammate の起動にはバックグラウンド Agent session が必要です", "teammate 생성에는 백그라운드 Agent session이 필요합니다", "Для запуска teammate требуются фоновые сессии Agent")
	addToolLegacyA(KeyToolLegacyAAgentTeamManagerRequired, "teammate spawning requires an active team manager", "生成 teammate 需要处于活动状态的 team manager", "Zum Starten von Teammates ist ein aktiver Team-Manager erforderlich", "teammate の起動には有効な team manager が必要です", "teammate 생성에는 활성 team manager가 필요합니다", "Для запуска teammate требуется активный менеджер команды")
	addToolLegacyA(KeyToolLegacyAAgentSpawnTeammateFailed, "failed to spawn teammate: %v", "无法生成 teammate：%v", "Teammate konnte nicht gestartet werden: %v", "teammate を起動できませんでした: %v", "teammate를 생성하지 못했습니다: %v", "Не удалось запустить teammate: %v")
	addToolLegacyA(KeyToolLegacyAAgentPersistTeammateFailed, "failed to persist teammate in team config: %s", "无法将 teammate 保存到团队配置：%s", "Teammate konnte nicht in der Teamkonfiguration gespeichert werden: %s", "teammate をチーム設定に保存できませんでした: %s", "teammate를 팀 설정에 저장하지 못했습니다: %s", "Не удалось сохранить teammate в конфигурации команды: %s")
	addToolLegacyA(KeyToolLegacyAAgentStartTeammateFailed, "failed to start teammate: %v", "无法启动 teammate：%v", "Teammate konnte nicht gestartet werden: %v", "teammate を開始できませんでした: %v", "teammate를 시작하지 못했습니다: %v", "Не удалось запустить teammate: %v")
	addToolLegacyA(KeyToolLegacyAAgentMarshalTeammateFailed, "failed to marshal teammate result: %v", "无法序列化 teammate 结果：%v", "Teammate-Ergebnis konnte nicht serialisiert werden: %v", "teammate の結果をシリアル化できませんでした: %v", "teammate 결과를 직렬화하지 못했습니다: %v", "Не удалось сериализовать результат teammate: %v")
	addToolLegacyA(KeyToolLegacyAAgentTeammateSpawned, "Teammate spawned and running in the background.", "Teammate 已生成，正在后台运行。", "Teammate wurde gestartet und läuft im Hintergrund.", "teammate を起動し、バックグラウンドで実行しています。", "teammate가 생성되어 백그라운드에서 실행 중입니다.", "Teammate запущен и работает в фоне.")
	addToolLegacyA(KeyToolLegacyAAgentContinueAsync, "Use SendMessage with to: %q to continue this agent. The agent is working in the background and will notify when it completes.", "要继续此 Agent，请使用 SendMessage，并将 to 设为 %q。Agent 正在后台运行，完成后会发出通知。", "Zum Fortsetzen dieses Agent SendMessage mit to: %q verwenden. Der Agent arbeitet im Hintergrund und meldet sich nach Abschluss.", "この Agent を続行するには、to: %q を指定して SendMessage を使用してください。Agent はバックグラウンドで動作しており、完了時に通知します。", "이 Agent를 계속하려면 to: %q로 SendMessage를 사용하세요. Agent는 백그라운드에서 작업 중이며 완료되면 알립니다.", "Чтобы продолжить работу этого Agent, используйте SendMessage с to: %q. Agent работает в фоне и уведомит о завершении.")
	addToolLegacyA(KeyToolLegacyAAgentRemoteLaunched, "Remote agent launched (taskId=%s). The remote runtime is responsible for completion notifications.", "远程 Agent 已启动（taskId=%s）。完成通知由远程 runtime 负责。", "Remote-Agent gestartet (taskId=%s). Die Remote-Runtime ist für Abschlussbenachrichtigungen zuständig.", "リモート Agent を起動しました（taskId=%s）。完了通知はリモート runtime が担当します。", "원격 Agent가 시작되었습니다(taskId=%s). 완료 알림은 원격 runtime이 담당합니다.", "Удалённый Agent запущен (taskId=%s). За уведомления о завершении отвечает удалённая runtime.")
	addToolLegacyA(KeyToolLegacyAAgentEmptyOutput, "(Subagent completed but returned no output.)", "（Subagent 已完成，但未返回任何输出。）", "(Subagent wurde abgeschlossen, hat aber keine Ausgabe zurückgegeben.)", "（Subagent は完了しましたが、出力はありませんでした。）", "(Subagent가 완료되었지만 출력이 없습니다.)", "(Subagent завершён, но не вернул вывод.)")
	addToolLegacyA(KeyToolLegacyAAgentInvalidTyped, "Agent returned an invalid typed result", "Agent 返回了无效的类型化结果", "Agent hat ein ungültiges typisiertes Ergebnis zurückgegeben", "Agent が無効な型付き結果を返しました", "Agent가 잘못된 형식의 결과를 반환했습니다", "Agent вернул недопустимый типизированный результат")
	addToolLegacyA(KeyToolLegacyAAgentReadUnmappable, "Read returned an unmappable result", "Read 返回了无法映射的结果", "Read hat ein nicht abbildbares Ergebnis zurückgegeben", "Read がマッピングできない結果を返しました", "Read가 매핑할 수 없는 결과를 반환했습니다", "Read вернул результат, который невозможно сопоставить")
	addToolLegacyA(KeyToolLegacyAAgentCompletedMetadata,
		"agentId: %s (use SendMessage with to: '%s' to continue this agent)%s\n<usage>total_tokens: %d\ntool_uses: %d\nduration_ms: %d</usage>",
		"agentId: %s（要继续此 Agent，请使用 SendMessage，并将 to 设为 '%s'）%s\n<usage>total_tokens: %d\ntool_uses: %d\nduration_ms: %d</usage>",
		"agentId: %s (zum Fortsetzen dieses Agent SendMessage mit to: '%s' verwenden)%s\n<usage>total_tokens: %d\ntool_uses: %d\nduration_ms: %d</usage>",
		"agentId: %s（この Agent を続行するには、to: '%s' を指定して SendMessage を使用）%s\n<usage>total_tokens: %d\ntool_uses: %d\nduration_ms: %d</usage>",
		"agentId: %s(이 Agent를 계속하려면 to: '%s'로 SendMessage 사용)%s\n<usage>total_tokens: %d\ntool_uses: %d\nduration_ms: %d</usage>",
		"agentId: %s (для продолжения этого Agent используйте SendMessage с to: '%s')%s\n<usage>total_tokens: %d\ntool_uses: %d\nduration_ms: %d</usage>")
	addToolLegacyA(KeyToolLegacyAAgentRemotePartial,
		"Remote agent launched in CCR.\ntaskId: %s\nsession_url: %s\noutput_file: %s\nThe agent is running remotely. You will be notified automatically when it completes.\nBriefly tell the user what you launched and end your response.",
		"远程 Agent 已在 CCR 中启动。\ntaskId: %s\nsession_url: %s\noutput_file: %s\nAgent 正在远程运行，完成后会自动通知。\n请简要告知用户已启动的任务，然后结束回复。",
		"Remote-Agent in CCR gestartet.\ntaskId: %s\nsession_url: %s\noutput_file: %s\nDer Agent läuft remote; nach Abschluss erfolgt automatisch eine Benachrichtigung.\nTeile dem Benutzer kurz mit, was gestartet wurde, und beende die Antwort.",
		"CCR でリモート Agent を起動しました。\ntaskId: %s\nsession_url: %s\noutput_file: %s\nAgent はリモートで実行中です。完了時に自動で通知されます。\n起動した内容をユーザーに簡潔に伝え、応答を終了してください。",
		"CCR에서 원격 Agent를 시작했습니다.\ntaskId: %s\nsession_url: %s\noutput_file: %s\nAgent가 원격으로 실행 중이며 완료되면 자동으로 알림을 받습니다.\n사용자에게 시작한 작업을 간단히 알리고 응답을 끝내세요.",
		"Удалённый Agent запущен в CCR.\ntaskId: %s\nsession_url: %s\noutput_file: %s\nAgent выполняется удалённо; после завершения придёт автоматическое уведомление.\nКратко сообщите пользователю, что было запущено, и завершите ответ.")
	addToolLegacyA(KeyToolLegacyAAgentAsyncPartial,
		"Async agent launched successfully.\nagentId: %s (internal ID - do not mention to user. Use SendMessage with to: '%s' to continue this agent.)\nThe agent is working in the background. You will be notified automatically when it completes.",
		"异步 Agent 已成功启动。\nagentId: %s（内部 ID，请勿向用户提及。要继续此 Agent，请使用 SendMessage，并将 to 设为 '%s'。）\nAgent 正在后台工作，完成后会自动通知。",
		"Asynchroner Agent erfolgreich gestartet.\nagentId: %s (interne ID – nicht gegenüber dem Benutzer erwähnen. Zum Fortsetzen dieses Agent SendMessage mit to: '%s' verwenden.)\nDer Agent arbeitet im Hintergrund; nach Abschluss erfolgt automatisch eine Benachrichtigung.",
		"非同期 Agent を起動しました。\nagentId: %s（内部 ID。ユーザーには伝えないでください。この Agent を続行するには、to: '%s' を指定して SendMessage を使用します。）\nAgent はバックグラウンドで動作中です。完了時に自動で通知されます。",
		"비동기 Agent를 시작했습니다.\nagentId: %s(내부 ID이므로 사용자에게 언급하지 마세요. 이 Agent를 계속하려면 to: '%s'로 SendMessage를 사용하세요.)\nAgent가 백그라운드에서 작업 중이며 완료되면 자동으로 알림을 받습니다.",
		"Асинхронный Agent успешно запущен.\nagentId: %s (внутренний ID — не сообщайте его пользователю. Для продолжения этого Agent используйте SendMessage с to: '%s'.)\nAgent работает в фоне; после завершения придёт автоматическое уведомление.")
	addToolLegacyA(KeyToolLegacyAAgentAsyncOutputHint,
		"\nDo not duplicate this agent's work. Work on non-overlapping tasks, or briefly tell the user what you launched and end your response.\noutput_file: %s",
		"\n不要重复此 Agent 的工作。请处理不重叠的任务，或简要告知用户已启动的任务，然后结束回复。\noutput_file: %s",
		"\nDupliziere die Arbeit dieses Agent nicht. Bearbeite nicht überlappende Aufgaben oder teile dem Benutzer kurz mit, was gestartet wurde, und beende die Antwort.\noutput_file: %s",
		"\nこの Agent の作業を重複して行わないでください。重ならないタスクを進めるか、起動した内容をユーザーに簡潔に伝えて応答を終了してください。\noutput_file: %s",
		"\n이 Agent의 작업을 중복하지 마세요. 겹치지 않는 작업을 수행하거나 사용자에게 시작한 작업을 간단히 알리고 응답을 끝내세요.\noutput_file: %s",
		"\nНе дублируйте работу этого Agent. Выполняйте непересекающиеся задачи либо кратко сообщите пользователю, что было запущено, и завершите ответ.\noutput_file: %s")
	addToolLegacyA(KeyToolLegacyAAgentAsyncCompletionHint,
		"\nBriefly tell the user what you launched and end your response. Do not generate any other text; agent results will arrive in a subsequent message.",
		"\n请简要告知用户已启动的任务，然后结束回复。不要生成其他内容；Agent 结果会在后续消息中送达。",
		"\nTeile dem Benutzer kurz mit, was gestartet wurde, und beende die Antwort. Erzeuge keinen weiteren Text; die Agent-Ergebnisse treffen in einer späteren Nachricht ein.",
		"\n起動した内容をユーザーに簡潔に伝え、応答を終了してください。それ以外のテキストは生成しないでください。Agent の結果は後続メッセージで届きます。",
		"\n사용자에게 시작한 작업을 간단히 알리고 응답을 끝내세요. 다른 텍스트는 생성하지 마세요. Agent 결과는 후속 메시지로 도착합니다.",
		"\nКратко сообщите пользователю, что было запущено, и завершите ответ. Не создавайте другой текст; результаты Agent поступят в следующем сообщении.")

	addToolLegacyA(KeyToolLegacyAConfigEmpty, "(no settings configured)", "（尚未配置任何设置）", "(keine Einstellungen konfiguriert)", "（設定はまだありません）", "(구성된 설정 없음)", "(настройки не заданы)")
	addToolLegacyA(KeyToolLegacyAConfigNotSet, "(not set) %s", "（未设置）%s", "(nicht gesetzt) %s", "（未設定）%s", "(설정되지 않음) %s", "(не задано) %s")
	addToolLegacyA(KeyToolLegacyAConfigSetRequired, "Error: 'setting' is required for set action", "错误：set 操作必须提供 setting", "Fehler: Für die Aktion set ist setting erforderlich", "エラー: set 操作には setting が必要です", "오류: set 작업에는 setting이 필요합니다", "Ошибка: для действия set требуется setting")
	addToolLegacyA(KeyToolLegacyAConfigSaveFailed, "Error: failed to save config: %s", "错误：无法保存配置：%s", "Fehler: Konfiguration konnte nicht gespeichert werden: %s", "エラー: 設定を保存できませんでした: %s", "오류: 설정을 저장하지 못했습니다: %s", "Ошибка: не удалось сохранить конфигурацию: %s")
	addToolLegacyA(KeyToolLegacyAConfigUpdated, "Config updated: %s = %s", "配置已更新：%s = %s", "Konfiguration aktualisiert: %s = %s", "設定を更新しました: %s = %s", "설정이 업데이트되었습니다: %s = %s", "Конфигурация обновлена: %s = %s")
	addToolLegacyA(KeyToolLegacyAConfigUnknownAction, `Error: unknown action %q; must be "get" or "set"`, `错误：未知操作 %q；必须为 "get" 或 "set"`, `Fehler: Unbekannte Aktion %q; zulässig sind "get" oder "set"`, `エラー: 不明な操作 %q です。"get" または "set" を指定してください`, `오류: 알 수 없는 작업 %q입니다. "get" 또는 "set"이어야 합니다`, `Ошибка: неизвестное действие %q; ожидается "get" или "set"`)

	addToolLegacyA(KeyToolLegacyABase64DecodeFailed, "base64 decode failed: %v", "base64 解码失败：%v", "Base64-Decodierung fehlgeschlagen: %v", "base64 のデコードに失敗しました: %v", "base64 디코딩에 실패했습니다: %v", "Не удалось декодировать base64: %v")
	addToolLegacyA(KeyToolLegacyAHashUnsupported, "unsupported hash algorithm: %s", "不支持的 hash 算法：%s", "Nicht unterstützter Hash-Algorithmus: %s", "サポートされていない hash アルゴリズムです: %s", "지원하지 않는 hash 알고리즘: %s", "Неподдерживаемый алгоритм hash: %s")
	addToolLegacyA(KeyToolLegacyAJSONInvalid, "invalid JSON: %v", "JSON 无效：%v", "Ungültiges JSON: %v", "JSON が無効です: %v", "잘못된 JSON: %v", "Недопустимый JSON: %v")
	addToolLegacyA(KeyToolLegacyAJSONFormattingFailed, "JSON formatting failed: %v", "JSON 格式化失败：%v", "JSON-Formatierung fehlgeschlagen: %v", "JSON の整形に失敗しました: %v", "JSON 형식 지정에 실패했습니다: %v", "Не удалось отформатировать JSON: %v")
	addToolLegacyA(KeyToolLegacyAJSONPathNotFound, "path not found: %s", "找不到 path：%s", "Pfad nicht gefunden: %s", "path が見つかりません: %s", "path를 찾을 수 없습니다: %s", "Путь не найден: %s")
	addToolLegacyA(KeyToolLegacyAHexDecodeFailed, "hex decode failed: %v", "hex 解码失败：%v", "Hex-Decodierung fehlgeschlagen: %v", "hex のデコードに失敗しました: %v", "hex 디코딩에 실패했습니다: %v", "Не удалось декодировать hex: %v")
	addToolLegacyA(KeyToolLegacyACronTooMany, "Too many scheduled jobs (max 50). Cancel one first.", "计划任务过多（最多 50 个）。请先取消一个。", "Zu viele geplante Aufgaben (maximal 50). Zuerst eine abbrechen.", "スケジュール済みジョブが多すぎます（最大 50 件）。先に 1 件キャンセルしてください。", "예약된 작업이 너무 많습니다(최대 50개). 먼저 하나를 취소하세요.", "Слишком много запланированных задач (максимум 50). Сначала отмените одну.")
	addToolLegacyA(KeyToolLegacyACronDurableLeaderOnly, "Error: durable cron jobs are only allowed on the leader session (errorCode=%d). This session is a non-durable teammate; remove durable=true and try again.", "错误：只有 leader session 可以创建持久 cron 任务（errorCode=%d）。当前 session 是非持久 teammate；请移除 durable=true 后重试。", "Fehler: Dauerhafte Cron-Aufgaben sind nur in der Leader-Sitzung erlaubt (errorCode=%d). Diese Sitzung ist ein nicht dauerhafter Teammate; durable=true entfernen und erneut versuchen.", "エラー: 永続 cron ジョブは leader session でのみ作成できます（errorCode=%d）。この session は非永続 teammate です。durable=true を削除して再試行してください。", "오류: 영구 cron 작업은 leader session에서만 허용됩니다(errorCode=%d). 이 session은 비영구 teammate입니다. durable=true를 제거하고 다시 시도하세요.", "Ошибка: постоянные задачи cron разрешены только в сессии leader (errorCode=%d). Эта сессия — непостоянный teammate; удалите durable=true и повторите попытку.")
	addToolLegacyA(KeyToolLegacyACronDeleteFailed, "Error deleting cron job id=%s: %v", "删除 cron 任务 id=%s 时出错：%v", "Fehler beim Löschen der Cron-Aufgabe id=%s: %v", "cron ジョブ id=%s の削除中にエラーが発生しました: %v", "cron 작업 id=%s 삭제 오류: %v", "Ошибка удаления задачи cron id=%s: %v")
	addToolLegacyA(KeyToolLegacyACronErrorCode, "%s (errorCode=%d)", "%s（errorCode=%d）", "%s (errorCode=%d)", "%s（errorCode=%d）", "%s(errorCode=%d)", "%s (errorCode=%d)")
	addToolLegacyA(KeyToolLegacyAExitPlanNotActive, "not in plan mode; call EnterPlanMode first", "当前不在 plan mode；请先调用 EnterPlanMode", "Nicht im Planungsmodus; zuerst EnterPlanMode aufrufen", "plan mode ではありません。先に EnterPlanMode を呼び出してください", "plan mode가 아닙니다. 먼저 EnterPlanMode를 호출하세요", "Сейчас не режим планирования; сначала вызовите EnterPlanMode")
	addToolLegacyA(KeyToolLegacyAExitPlanInvalidTypedData, "ExitPlanMode returned invalid typed data", "ExitPlanMode 返回了无效的类型化数据", "ExitPlanMode hat ungültige typisierte Daten zurückgegeben", "ExitPlanMode が無効な型付きデータを返しました", "ExitPlanMode가 잘못된 형식의 데이터를 반환했습니다", "ExitPlanMode вернул недопустимые типизированные данные")
	addToolLegacyA(KeyToolLegacyAExitPlanAwaitingLeader,
		"Your plan has been submitted to the team lead for approval.\n\nPlan file: %s\n\n**What happens next:**\n1. Wait for the team lead to review your plan\n2. You will receive a message in your inbox with approval/rejection\n3. If approved, you can proceed with implementation\n4. If rejected, refine your plan based on the feedback\n\n**Important:** Do NOT proceed until you receive approval. Check your inbox for response.\n\nRequest ID: %s",
		"你的 Plan 已提交给 team lead 审批。\n\nPlan 文件：%s\n\n**接下来：**\n1. 等待 team lead 审阅 Plan\n2. 收件箱会收到批准或驳回消息\n3. 如获批准，即可开始实现\n4. 如被驳回，请根据反馈修改 Plan\n\n**重要：**收到批准前不要继续。请查看收件箱中的回复。\n\n请求 ID：%s",
		"Dein Plan wurde dem Team Lead zur Genehmigung vorgelegt.\n\nPlan-Datei: %s\n\n**Nächste Schritte:**\n1. Warte auf die Prüfung durch den Team Lead\n2. Die Genehmigung oder Ablehnung trifft im Posteingang ein\n3. Nach der Genehmigung kannst du mit der Umsetzung beginnen\n4. Überarbeite den Plan nach einer Ablehnung anhand des Feedbacks\n\n**Wichtig:** Fahre erst nach der Genehmigung fort. Prüfe deinen Posteingang.\n\nAnfrage-ID: %s",
		"Plan を team lead に提出し、承認を待っています。\n\nPlan ファイル: %s\n\n**次の手順:**\n1. team lead による Plan の確認を待つ\n2. 承認または却下のメッセージが受信トレイに届く\n3. 承認されたら実装を開始できる\n4. 却下されたらフィードバックに基づいて Plan を修正する\n\n**重要:** 承認を受けるまで先に進まないでください。受信トレイを確認してください。\n\nリクエスト ID: %s",
		"Plan을 team lead에게 제출했으며 승인을 기다리고 있습니다.\n\nPlan 파일: %s\n\n**다음 단계:**\n1. team lead의 Plan 검토를 기다립니다\n2. 받은 편지함으로 승인 또는 거절 메시지가 옵니다\n3. 승인되면 구현을 진행할 수 있습니다\n4. 거절되면 피드백에 따라 Plan을 수정합니다\n\n**중요:** 승인받기 전에는 진행하지 마세요. 받은 편지함을 확인하세요.\n\n요청 ID: %s",
		"Ваш Plan отправлен Team Lead на утверждение.\n\nФайл Plan: %s\n\n**Что дальше:**\n1. Дождитесь проверки Plan руководителем команды\n2. В почтовый ящик придёт сообщение об утверждении или отклонении\n3. После утверждения можно приступать к реализации\n4. После отклонения доработайте Plan с учётом замечаний\n\n**Важно:** Не продолжайте без утверждения. Проверьте входящие сообщения.\n\nID запроса: %s")
	addToolLegacyA(KeyToolLegacyAExitPlanAgentApproved, `User has approved the plan. There is nothing else needed from you now. Please respond with "ok"`, `用户已批准 Plan。现在无需再执行其他操作，请回复“ok”`, `Der Benutzer hat den Plan genehmigt. Es ist nichts weiter zu tun; antworte bitte mit „ok“`, `ユーザーが Plan を承認しました。ほかに必要な作業はありません。「ok」と返答してください`, `사용자가 Plan을 승인했습니다. 이제 추가 작업은 필요하지 않습니다. "ok"라고 응답하세요`, `Пользователь утвердил Plan. Больше ничего делать не нужно; ответьте «ok»`)
	addToolLegacyA(KeyToolLegacyAExitPlanApproved, "User has approved exiting plan mode. You can now proceed.", "用户已批准退出 plan mode，现在可以继续。", "Der Benutzer hat das Verlassen des Planungsmodus genehmigt. Du kannst jetzt fortfahren.", "ユーザーが plan mode の終了を承認しました。続行できます。", "사용자가 plan mode 종료를 승인했습니다. 이제 계속 진행할 수 있습니다.", "Пользователь разрешил выйти из режима планирования. Можно продолжать.")
	addToolLegacyA(KeyToolLegacyAExitPlanTeamHint, "\n\nIf this plan can be broken down into multiple independent tasks, consider using the TeamCreate tool to create a team and parallelize the work.", "\n\n如果此 Plan 可以拆分为多个独立任务，可考虑使用 TeamCreate 创建团队并行处理。", "\n\nWenn sich dieser Plan in mehrere unabhängige Aufgaben aufteilen lässt, erwäge mit TeamCreate ein Team zu erstellen und die Arbeit zu parallelisieren.", "\n\nこの Plan を複数の独立したタスクに分割できる場合は、TeamCreate でチームを作成して並行作業することを検討してください。", "\n\n이 Plan을 여러 독립 작업으로 나눌 수 있다면 TeamCreate로 팀을 만들어 병렬로 진행하는 것을 고려하세요.", "\n\nЕсли этот Plan можно разбить на независимые задачи, рассмотрите создание команды через TeamCreate для параллельной работы.")
	addToolLegacyA(KeyToolLegacyAExitPlanLabel, "Approved Plan", "已批准的 Plan", "Genehmigter Plan", "承認済み Plan", "승인된 Plan", "Утверждённый Plan")
	addToolLegacyA(KeyToolLegacyAExitPlanEditedLabel, "Approved Plan (edited by user)", "已批准的 Plan（经用户编辑）", "Genehmigter Plan (vom Benutzer bearbeitet)", "承認済み Plan（ユーザーが編集）", "승인된 Plan(사용자 편집)", "Утверждённый Plan (изменён пользователем)")
	addToolLegacyA(KeyToolLegacyAExitPlanApprovedBody, "User has approved your plan. You can now start coding. Start with updating your todo list if applicable\n\nYour plan has been saved to: %s\nYou can refer back to it if needed during implementation.%s\n\n## %s:\n%s", "用户已批准你的 Plan，现在可以开始编码。如适用，请先更新 todo list。\n\nPlan 已保存至：%s\n实现过程中可随时查阅。%s\n\n## %s：\n%s", "Der Benutzer hat deinen Plan genehmigt. Du kannst jetzt mit der Umsetzung beginnen. Aktualisiere gegebenenfalls zuerst deine Todo-Liste.\n\nDein Plan wurde hier gespeichert: %s\nDu kannst während der Umsetzung darauf zurückgreifen.%s\n\n## %s:\n%s", "ユーザーが Plan を承認しました。実装を開始できます。必要に応じて、まず todo list を更新してください。\n\nPlan の保存先: %s\n実装中に必要であれば参照できます。%s\n\n## %s:\n%s", "사용자가 Plan을 승인했습니다. 이제 코딩을 시작할 수 있습니다. 해당하는 경우 먼저 todo list를 업데이트하세요.\n\nPlan 저장 위치: %s\n구현 중 필요하면 다시 참고할 수 있습니다.%s\n\n## %s:\n%s", "Пользователь утвердил ваш Plan. Теперь можно приступать к реализации. При необходимости сначала обновите todo list.\n\nPlan сохранён здесь: %s\nПри необходимости обращайтесь к нему во время реализации.%s\n\n## %s:\n%s")
	addToolLegacyA(KeyToolLegacyAExitPlanRejected, "User rejected the plan. Stay in plan mode and revise it before requesting approval again.", "用户驳回了 Plan。请保持 plan mode，修改后再申请批准。", "Der Benutzer hat den Plan abgelehnt. Bleibe im Planungsmodus und überarbeite ihn, bevor du erneut um Genehmigung bittest.", "ユーザーが Plan を却下しました。plan mode のまま修正し、改めて承認を求めてください。", "사용자가 Plan을 거절했습니다. plan mode를 유지하고 수정한 뒤 다시 승인을 요청하세요.", "Пользователь отклонил Plan. Оставайтесь в режиме планирования, доработайте его и снова запросите утверждение.")
	addToolLegacyA(KeyToolLegacyAExitPlanFeedback, "\n\nFeedback: %s", "\n\n反馈：%s", "\n\nFeedback: %s", "\n\nフィードバック: %s", "\n\n피드백: %s", "\n\nКомментарий: %s")
	addToolLegacyA(KeyToolLegacyAExitPlanStateRequired, "ExitPlanMode requires plan state", "ExitPlanMode 需要 plan state", "ExitPlanMode benötigt einen Planungsstatus", "ExitPlanMode には plan state が必要です", "ExitPlanMode에는 plan state가 필요합니다", "Для ExitPlanMode требуется plan state")
	addToolLegacyA(KeyToolLegacyAExitPlanPathMismatch, "ExitPlanMode planFilePath does not match the active plan", "ExitPlanMode 的 planFilePath 与当前 Plan 不匹配", "Der planFilePath von ExitPlanMode stimmt nicht mit dem aktiven Plan überein", "ExitPlanMode の planFilePath が現在の Plan と一致しません", "ExitPlanMode planFilePath가 활성 Plan과 일치하지 않습니다", "planFilePath ExitPlanMode не соответствует активному Plan")
	addToolLegacyA(KeyToolLegacyAExitPlanNoActiveFile, "ExitPlanMode has no active plan file", "ExitPlanMode 没有活动的 Plan 文件", "ExitPlanMode hat keine aktive Plan-Datei", "ExitPlanMode に有効な Plan ファイルがありません", "ExitPlanMode에 활성 Plan 파일이 없습니다", "У ExitPlanMode нет активного файла Plan")
	addToolLegacyA(KeyToolLegacyAExitPlanReadFile, "read plan file %s: %v", "读取 Plan 文件 %s 失败：%v", "Plan-Datei %s konnte nicht gelesen werden: %v", "Plan ファイル %s を読み取れませんでした: %v", "Plan 파일 %s을(를) 읽지 못했습니다: %v", "Не удалось прочитать файл Plan %s: %v")
	addToolLegacyA(KeyToolLegacyAExitPlanMarshalInput, "marshal ExitPlanMode input: %v", "序列化 ExitPlanMode 输入失败：%v", "ExitPlanMode-Eingabe konnte nicht serialisiert werden: %v", "ExitPlanMode の入力をシリアル化できませんでした: %v", "ExitPlanMode 입력을 직렬화하지 못했습니다: %v", "Не удалось сериализовать вход ExitPlanMode: %v")
	addToolLegacyA(KeyToolLegacyAExitPlanInvalidInput, "invalid ExitPlanMode input: %v", "ExitPlanMode 输入无效：%v", "Ungültige ExitPlanMode-Eingabe: %v", "ExitPlanMode の入力が無効です: %v", "잘못된 ExitPlanMode 입력: %v", "Недопустимый вход ExitPlanMode: %v")
	addToolLegacyA(KeyToolLegacyAExitPlanInvalidPrompts, "invalid ExitPlanMode allowedPrompts entry: tool must be Bash and prompt must be non-empty", "ExitPlanMode 的 allowedPrompts 条目无效：tool 必须为 Bash，且 prompt 不能为空", "Ungültiger allowedPrompts-Eintrag für ExitPlanMode: tool muss Bash sein und prompt darf nicht leer sein", "ExitPlanMode の allowedPrompts エントリが無効です: tool は Bash、prompt は空でない必要があります", "잘못된 ExitPlanMode allowedPrompts 항목입니다. tool은 Bash여야 하고 prompt는 비어 있으면 안 됩니다", "Недопустимая запись allowedPrompts ExitPlanMode: tool должен быть Bash, а prompt — непустым")
	addToolLegacyA(KeyToolLegacyAExitPlanReadBeforeExit, "read plan file %s before exit: %v", "退出前读取 Plan 文件 %s 失败：%v", "Plan-Datei %s konnte vor dem Beenden nicht gelesen werden: %v", "終了前に Plan ファイル %s を読み取れませんでした: %v", "종료 전에 Plan 파일 %s을(를) 읽지 못했습니다: %v", "Не удалось прочитать файл Plan %s перед выходом: %v")
	addToolLegacyA(KeyToolLegacyAExitPlanPersistEdited, "persist edited plan: %v", "保存用户编辑后的 Plan 失败：%v", "Bearbeiteter Plan konnte nicht gespeichert werden: %v", "編集済み Plan を保存できませんでした: %v", "편집된 Plan을 저장하지 못했습니다: %v", "Не удалось сохранить изменённый Plan: %v")
	addToolLegacyA(KeyToolLegacyAExitPlanCommitRollback, "commit plan exit: %[2]v; restore original plan: %[1]v", "提交 Plan 退出失败：%[2]v；恢复原始 Plan 失败：%[1]v", "Beenden des Planungsmodus konnte nicht übernommen werden: %[2]v; ursprünglicher Plan konnte nicht wiederhergestellt werden: %[1]v", "Plan の終了を確定できませんでした: %[2]v。元の Plan の復元にも失敗しました: %[1]v", "Plan 종료를 반영하지 못했습니다: %[2]v. 원래 Plan 복원도 실패했습니다: %[1]v", "Не удалось применить выход из Plan: %[2]v; также не удалось восстановить исходный Plan: %[1]v")
	addToolLegacyA(KeyToolLegacyAExitPlanCommit, "commit plan exit: %v", "提交 Plan 退出失败：%v", "Beenden des Planungsmodus konnte nicht übernommen werden: %v", "Plan の終了を確定できませんでした: %v", "Plan 종료를 반영하지 못했습니다: %v", "Не удалось применить выход из Plan: %v")

	addToolLegacyA(KeyToolLegacyAFileInvalidInput, "invalid input: %v", "输入无效：%v", "Ungültige Eingabe: %v", "入力が無効です: %v", "잘못된 입력: %v", "Недопустимые входные данные: %v")
	addToolLegacyA(KeyToolLegacyAFileOffsetNonNegative, "'offset' must be a non-negative integer", "offset 必须是非负整数", "offset muss eine nicht negative ganze Zahl sein", "offset は 0 以上の整数である必要があります", "offset은 0 이상의 정수여야 합니다", "offset должен быть неотрицательным целым числом")
	addToolLegacyA(KeyToolLegacyAFileLimitNonNegative, "'limit' must be a non-negative integer", "limit 必须是非负整数", "limit muss eine nicht negative ganze Zahl sein", "limit は 0 以上の整数である必要があります", "limit은 0 이상의 정수여야 합니다", "limit должен быть неотрицательным целым числом")
	addToolLegacyA(KeyToolLegacyAFileLimitPositive, "'limit' must be a positive integer", "limit 必须是正整数", "limit muss eine positive ganze Zahl sein", "limit は正の整数である必要があります", "limit은 양의 정수여야 합니다", "limit должен быть положительным целым числом")
	addToolLegacyA(KeyToolLegacyAFilePagesInvalid, `Invalid pages parameter: %q. Use formats like "1-5", "3", or "10-20". Pages are 1-indexed.`, `pages 参数无效：%q。请使用 "1-5"、"3" 或 "10-20" 等格式。页码从 1 开始。`, `Ungültiger pages-Parameter: %q. Formate wie "1-5", "3" oder "10-20" verwenden. Seiten werden ab 1 gezählt.`, `pages パラメーターが無効です: %q。"1-5"、"3"、"10-20" のような形式を使用してください。ページ番号は 1 から始まります。`, `잘못된 pages 매개변수: %q. "1-5", "3", "10-20" 같은 형식을 사용하세요. 페이지 번호는 1부터 시작합니다.`, `Недопустимый параметр pages: %q. Используйте формат "1-5", "3" или "10-20". Нумерация страниц начинается с 1.`)
	addToolLegacyA(KeyToolLegacyAFilePageRangeTooLarge, "Page range %q exceeds maximum of %d pages per request. Please use a smaller range.", "页码范围 %q 超过单次请求最多 %d 页的限制。请使用更小的范围。", "Der Seitenbereich %q überschreitet das Maximum von %d Seiten pro Anfrage. Einen kleineren Bereich verwenden.", "ページ範囲 %q は 1 回のリクエストあたり最大 %d ページを超えています。範囲を小さくしてください。", "페이지 범위 %q이(가) 요청당 최대 %d페이지를 초과합니다. 더 작은 범위를 사용하세요.", "Диапазон страниц %q превышает максимум в %d страниц на запрос. Укажите меньший диапазон.")
	addToolLegacyA(KeyToolLegacyAFileDirectoryDenied, "File is in a directory that is denied by your permission settings.", "文件位于权限设置禁止访问的目录中。", "Die Datei befindet sich in einem durch die Berechtigungseinstellungen gesperrten Verzeichnis.", "ファイルは権限設定で拒否されているディレクトリ内にあります。", "파일이 권한 설정에서 거부된 디렉터리에 있습니다.", "Файл находится в каталоге, запрещённом настройками разрешений.")
	addToolLegacyA(KeyToolLegacyAFileUNCRequiresPermission, "Permission is required before reading UNC path %s.", "读取 UNC 路径 %s 前需要获得权限。", "Vor dem Lesen des UNC-Pfads %s ist eine Berechtigung erforderlich.", "UNC パス %s を読み込む前に権限が必要です。", "UNC 경로 %s을(를) 읽기 전에 권한이 필요합니다.", "Перед чтением UNC-пути %s требуется разрешение.")
	addToolLegacyA(KeyToolLegacyAFileDeviceBlocked, "Cannot read '%s': this device file would block or produce infinite output.", "无法读取“%s”：该设备文件会导致阻塞或产生无限输出。", "'%s' kann nicht gelesen werden: Diese Gerätedatei würde blockieren oder eine endlose Ausgabe erzeugen.", "「%s」を読み込めません。このデバイスファイルはブロックするか、無限に出力します。", "'%s'을(를) 읽을 수 없습니다. 이 장치 파일은 차단되거나 무한 출력을 생성합니다.", "Невозможно прочитать '%s': этот файл устройства заблокирует чтение или создаст бесконечный вывод.")
	addToolLegacyA(KeyToolLegacyAFileBinaryUnsupported, "This tool cannot read binary files. The file appears to be a binary %s file. Please use appropriate tools for binary file analysis.", "此工具无法读取二进制文件。该文件似乎是 %s 二进制文件，请使用适合分析二进制文件的工具。", "Dieses Tool kann keine Binärdateien lesen. Die Datei scheint eine binäre %s-Datei zu sein. Ein geeignetes Tool zur Binäranalyse verwenden.", "このツールはバイナリファイルを読み込めません。このファイルは %s バイナリファイルのようです。バイナリ解析に適したツールを使用してください。", "이 도구는 바이너리 파일을 읽을 수 없습니다. 파일이 바이너리 %s 파일로 보입니다. 바이너리 분석에 적합한 도구를 사용하세요.", "Этот инструмент не читает двоичные файлы. Похоже, это двоичный файл %s. Используйте подходящие инструменты для анализа двоичных файлов.")
	addToolLegacyA(KeyToolLegacyAFileOpenFailed, "failed to open file: %v", "无法打开文件：%v", "Datei konnte nicht geöffnet werden: %v", "ファイルを開けませんでした: %v", "파일을 열지 못했습니다: %v", "Не удалось открыть файл: %v")
	addToolLegacyA(KeyToolLegacyAFilePathVerification, "path verification failed: %v", "路径验证失败：%v", "Pfadprüfung fehlgeschlagen: %v", "パスの検証に失敗しました: %v", "경로 검증에 실패했습니다: %v", "Не удалось проверить путь: %v")
	addToolLegacyA(KeyToolLegacyAFileReadFailed, "failed to read file: %v", "无法读取文件：%v", "Datei konnte nicht gelesen werden: %v", "ファイルを読み込めませんでした: %v", "파일을 읽지 못했습니다: %v", "Не удалось прочитать файл: %v")
	addToolLegacyA(KeyToolLegacyAFileTooLarge, "File content (%s) exceeds maximum allowed size (%s). Use offset and limit parameters to read specific portions of the file, or search for specific content instead of reading the whole file.", "文件内容（%s）超过允许的最大大小（%s）。请使用 offset 和 limit 参数读取指定部分，或搜索特定内容，不要读取整个文件。", "Der Dateiinhalt (%s) überschreitet die maximal zulässige Größe (%s). Mit offset und limit bestimmte Abschnitte lesen oder nach konkretem Inhalt suchen, statt die ganze Datei zu lesen.", "ファイル内容（%s）は最大許容サイズ（%s）を超えています。ファイル全体を読み込まず、offset と limit で必要な範囲を読み込むか、特定の内容を検索してください。", "파일 내용(%s)이 허용된 최대 크기(%s)를 초과합니다. 전체 파일을 읽는 대신 offset과 limit으로 필요한 부분을 읽거나 특정 내용을 검색하세요.", "Содержимое файла (%s) превышает максимально допустимый размер (%s). Используйте offset и limit для чтения нужных фрагментов либо ищите конкретное содержимое вместо чтения всего файла.")
	addToolLegacyA(KeyToolLegacyAFileNotFound, "file does not exist: %s", "文件不存在：%s", "Datei ist nicht vorhanden: %s", "ファイルがありません: %s", "파일이 없습니다: %s", "Файл не существует: %s")
	addToolLegacyA(KeyToolLegacyAFileNotFoundInCWD, "File does not exist. Current working directory is %s.", "文件不存在。当前工作目录为 %s。", "Datei ist nicht vorhanden. Das aktuelle Arbeitsverzeichnis ist %s.", "ファイルがありません。現在の作業ディレクトリは %s です。", "파일이 없습니다. 현재 작업 디렉터리는 %s입니다.", "Файл не существует. Текущий рабочий каталог: %s.")
	addToolLegacyA(KeyToolLegacyAFileNotFoundSuggestion, "File does not exist. Current working directory is %s. Did you mean %s?", "文件不存在。当前工作目录为 %s。你是不是想输入 %s？", "Datei ist nicht vorhanden. Das aktuelle Arbeitsverzeichnis ist %s. Meintest du %s?", "ファイルがありません。現在の作業ディレクトリは %s です。%s のことですか？", "파일이 없습니다. 현재 작업 디렉터리는 %s입니다. %s을(를) 의미했나요?", "Файл не существует. Текущий рабочий каталог: %s. Возможно, имелось в виду %s?")
	addToolLegacyA(KeyToolLegacyAFilePlanModeBlocked, "cannot use %s in plan mode — exit plan mode first", "plan mode 下无法使用 %s，请先退出 plan mode", "%s kann im Planungsmodus nicht verwendet werden — zuerst den Planungsmodus verlassen", "plan mode では %s を使用できません。先に plan mode を終了してください", "plan mode에서는 %s을(를) 사용할 수 없습니다. 먼저 plan mode를 종료하세요", "%s нельзя использовать в режиме планирования — сначала выйдите из него")
	addToolLegacyA(KeyToolLegacyAFilePathRequired, "'file_path' is required", "必须提供 file_path", "file_path ist erforderlich", "file_path は必須です", "file_path가 필요합니다", "Требуется file_path")
	addToolLegacyA(KeyToolLegacyAFileContentRequired, "'content' is required", "必须提供 content", "content ist erforderlich", "content は必須です", "content가 필요합니다", "Требуется content")
	addToolLegacyA(KeyToolLegacyAFileResolveFailed, "cannot resolve path: %v", "无法解析路径：%v", "Pfad kann nicht aufgelöst werden: %v", "パスを解決できません: %v", "경로를 확인할 수 없습니다: %v", "Не удалось определить путь: %v")
	addToolLegacyA(KeyToolLegacyAFileOutsideAllowed, "path %q is outside allowed directories", "路径 %q 位于允许访问的目录之外", "Pfad %q liegt außerhalb der erlaubten Verzeichnisse", "パス %q は許可されたディレクトリの外にあります", "경로 %q이(가) 허용된 디렉터리 밖에 있습니다", "Путь %q находится вне разрешённых каталогов")
	addToolLegacyA(KeyToolLegacyAFileTeamMemorySecret, "Refusing to write to team memory file %q: content appears to contain a secret (%s). Remove the credential before writing — team memory is shared in version control.", "拒绝写入团队记忆文件 %q：内容似乎包含 secret（%s）。团队记忆会在版本控制中共享，请移除凭据后再写入。", "Schreiben in die Team-Speicherdatei %q abgelehnt: Der Inhalt scheint ein Secret (%s) zu enthalten. Vor dem Schreiben die Zugangsdaten entfernen — der Team-Speicher wird über die Versionsverwaltung geteilt.", "チームメモリファイル %q への書き込みを拒否しました。内容に secret（%s）が含まれている可能性があります。チームメモリはバージョン管理で共有されるため、書き込む前に認証情報を削除してください。", "팀 메모리 파일 %q에 쓰기를 거부했습니다. 내용에 secret(%s)이 포함된 것으로 보입니다. 팀 메모리는 버전 관리에서 공유되므로 쓰기 전에 자격 증명을 제거하세요.", "Запись в файл памяти команды %q отклонена: содержимое похоже на secret (%s). Удалите учётные данные перед записью — память команды хранится в системе контроля версий.")
	addToolLegacyA(KeyToolLegacyAFileCreateDirectoryFailed, "failed to create directory: %v", "无法创建目录：%v", "Verzeichnis konnte nicht erstellt werden: %v", "ディレクトリを作成できませんでした: %v", "디렉터리를 만들지 못했습니다: %v", "Не удалось создать каталог: %v")
	addToolLegacyA(KeyToolLegacyAFileNotReadForWrite, "File has not been read yet. Read it first before writing to it.", "尚未读取该文件。请先读取再写入。", "Die Datei wurde noch nicht gelesen. Vor dem Schreiben zuerst lesen.", "ファイルはまだ読み込まれていません。書き込む前に読み込んでください。", "파일을 아직 읽지 않았습니다. 쓰기 전에 먼저 읽으세요.", "Файл ещё не прочитан. Прочитайте его перед записью.")
	addToolLegacyA(KeyToolLegacyAFilePartiallyReadForWrite, "File has only been read partially. Read the whole file before writing to it.", "只读取了文件的一部分。请先读取整个文件再写入。", "Die Datei wurde nur teilweise gelesen. Vor dem Schreiben vollständig lesen.", "ファイルの一部しか読み込まれていません。書き込む前に全体を読み込んでください。", "파일의 일부만 읽었습니다. 쓰기 전에 전체 파일을 읽으세요.", "Файл прочитан лишь частично. Перед записью прочитайте его полностью.")
	addToolLegacyA(KeyToolLegacyAFileChangedForWrite, "File has been modified since read, either by the user or by a linter. Read it again before attempting to write it.", "文件在读取后已被用户或 linter 修改。请重新读取后再写入。", "Die Datei wurde nach dem Lesen vom Benutzer oder einem Linter geändert. Vor dem Schreiben erneut lesen.", "読み込み後にユーザーまたは linter がファイルを変更しました。書き込む前にもう一度読み込んでください。", "파일을 읽은 뒤 사용자 또는 linter가 수정했습니다. 쓰기 전에 다시 읽으세요.", "После чтения файл был изменён пользователем или linter. Прочитайте его снова перед записью.")
	addToolLegacyA(KeyToolLegacyAFileWriteFailed, "failed to write file: %v", "无法写入文件：%v", "Datei konnte nicht geschrieben werden: %v", "ファイルに書き込めませんでした: %v", "파일에 쓰지 못했습니다: %v", "Не удалось записать файл: %v")
	addToolLegacyA(KeyToolLegacyAFileAppendFailed, "failed to append to file: %v", "无法追加写入文件：%v", "An die Datei konnte nicht angehängt werden: %v", "ファイルに追記できませんでした: %v", "파일에 추가하지 못했습니다: %v", "Не удалось дописать в файл: %v")
	addToolLegacyA(KeyToolLegacyAFileDeleteFailed, "failed to delete file: %v", "无法删除文件：%v", "Datei konnte nicht gelöscht werden: %v", "ファイルを削除できませんでした: %v", "파일을 삭제하지 못했습니다: %v", "Не удалось удалить файл: %v")
	addToolLegacyA(KeyToolLegacyAFileListFailed, "failed to list directory: %v", "无法列出目录内容：%v", "Verzeichnis konnte nicht aufgelistet werden: %v", "ディレクトリの一覧を取得できませんでした: %v", "디렉터리 목록을 가져오지 못했습니다: %v", "Не удалось получить содержимое каталога: %v")
	addToolLegacyA(KeyToolLegacyAFileGlobInvalid, "invalid glob pattern: %v", "glob pattern 无效：%v", "Ungültiges Glob-Muster: %v", "glob pattern が無効です: %v", "잘못된 glob pattern: %v", "Недопустимый glob pattern: %v")
	addToolLegacyA(KeyToolLegacyAFileMoveFailed, "failed to move file: %v", "无法移动文件：%v", "Datei konnte nicht verschoben werden: %v", "ファイルを移動できませんでした: %v", "파일을 이동하지 못했습니다: %v", "Не удалось переместить файл: %v")
	addToolLegacyA(KeyToolLegacyAFileSymlinkFailed, "failed to create symlink: %v", "无法创建 symlink：%v", "Symlink konnte nicht erstellt werden: %v", "symlink を作成できませんでした: %v", "symlink를 만들지 못했습니다: %v", "Не удалось создать symlink: %v")
	addToolLegacyA(KeyToolLegacyAFileReadInvalidTyped, "Read returned an invalid typed result", "Read 返回了无效的类型化结果", "Read hat ein ungültiges typisiertes Ergebnis zurückgegeben", "Read が無効な型付き結果を返しました", "Read가 잘못된 형식의 결과를 반환했습니다", "Read вернул недопустимый типизированный результат")
	addToolLegacyA(KeyToolLegacyAFilePDFRead, "PDF file read: %s (%s)", "已读取 PDF 文件：%s（%s）", "PDF-Datei gelesen: %s (%s)", "PDF ファイルを読み込みました: %s（%s）", "PDF 파일을 읽었습니다: %s(%s)", "PDF-файл прочитан: %s (%s)")
	addToolLegacyA(KeyToolLegacyAFilePDFPagesExtracted, "PDF pages extracted: %d page(s) from %s (%s)", "已从 %[2]s（%[3]s）提取 %[1]d 页 PDF", "PDF-Seiten extrahiert: %d Seite(n) aus %s (%s)", "%[2]s（%[3]s）から PDF を %[1]d ページ抽出しました", "%[2]s(%[3]s)에서 PDF %[1]d페이지를 추출했습니다", "Извлечены страницы PDF: %d из %s (%s)")
	addToolLegacyA(KeyToolLegacyAImagePlaceholder, "[image: %s, %d bytes base64]", "[图片：%s，%d 字节 base64]", "[Bild: %s, %d Byte Base64]", "[画像: %s、%d バイト base64]", "[이미지: %s, %d바이트 base64]", "[изображение: %s, %d байт base64]")
}
