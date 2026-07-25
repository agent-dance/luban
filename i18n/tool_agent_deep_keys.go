package i18n

// Semantic copy for errors created below the Agent tool-result boundary.
// Agent/profile identifiers, paths, status values, protocol field names, and
// raw causes remain format arguments so their canonical values are preserved.
const (
	KeyToolAgentDeepPermissionSnapshotUnavailable    Key = "tool.agent.deep.permission_snapshot_unavailable"
	KeyToolAgentDeepResumeContextUntrusted           Key = "tool.agent.deep.resume_context_untrusted"
	KeyToolAgentDeepRunOutcome                       Key = "tool.agent.deep.run_outcome"
	KeyToolAgentDeepBackgroundManagerUnavailable     Key = "tool.agent.deep.background_manager_unavailable"
	KeyToolAgentDeepAutoBackgroundStartFailed        Key = "tool.agent.deep.auto_background_start_failed"
	KeyToolAgentDeepProviderNotConfigured            Key = "tool.agent.deep.provider_not_configured"
	KeyToolAgentDeepRegistryNotConfigured            Key = "tool.agent.deep.registry_not_configured"
	KeyToolAgentDeepCWDWorktreeConflict              Key = "tool.agent.deep.cwd_worktree_conflict"
	KeyToolAgentDeepWorktreeTrustedRootRequired      Key = "tool.agent.deep.worktree_trusted_root_required"
	KeyToolAgentDeepSessionMissingInput              Key = "tool.agent.deep.session_missing_input"
	KeyToolAgentDeepErrorCause                       Key = "tool.agent.deep.error_cause"
	KeyToolAgentDeepCWDAbsoluteRequired              Key = "tool.agent.deep.cwd_absolute_required"
	KeyToolAgentDeepCWDInaccessible                  Key = "tool.agent.deep.cwd_inaccessible"
	KeyToolAgentDeepCWDDirectoryRequired             Key = "tool.agent.deep.cwd_directory_required"
	KeyToolAgentDeepCWDOutsideParentScope            Key = "tool.agent.deep.cwd_outside_parent_scope"
	KeyToolAgentDeepEncounteredError                 Key = "tool.agent.deep.encountered_error"
	KeyToolAgentDeepRunFailedWithDetail              Key = "tool.agent.deep.run_failed_with_detail"
	KeyToolAgentDeepSubagentTypeNotAllowed           Key = "tool.agent.deep.subagent_type_not_allowed"
	KeyToolAgentDeepForkContextRequired              Key = "tool.agent.deep.fork_context_required"
	KeyToolAgentDeepForkNestedUnavailable            Key = "tool.agent.deep.fork_nested_unavailable"
	KeyToolAgentDeepIsolationUnsupported             Key = "tool.agent.deep.isolation_unsupported"
	KeyToolAgentDeepUnknownSubagentType              Key = "tool.agent.deep.unknown_subagent_type"
	KeyToolAgentDeepMCPServersRequired               Key = "tool.agent.deep.mcp_servers_required"
	KeyToolAgentDeepFrontmatterParseFailed           Key = "tool.agent.deep.frontmatter_parse_failed"
	KeyToolAgentDeepCustomPromptEmpty                Key = "tool.agent.deep.custom_prompt_empty"
	KeyToolAgentDeepMCPServerNamedError              Key = "tool.agent.deep.mcp_server_named_error"
	KeyToolAgentDeepMCPServerConfigExpected          Key = "tool.agent.deep.mcp_server_config_expected"
	KeyToolAgentDeepMCPServersValueExpected          Key = "tool.agent.deep.mcp_servers_value_expected"
	KeyToolAgentDeepMCPCommandRequired               Key = "tool.agent.deep.mcp_command_required"
	KeyToolAgentDeepAgentsJSONParseFailed            Key = "tool.agent.deep.agents_json_parse_failed"
	KeyToolAgentDeepJSONNameEmpty                    Key = "tool.agent.deep.json_name_empty"
	KeyToolAgentDeepJSONDescriptionMissing           Key = "tool.agent.deep.json_description_missing"
	KeyToolAgentDeepJSONPromptMissing                Key = "tool.agent.deep.json_prompt_missing"
	KeyToolAgentDeepJSONModelEmpty                   Key = "tool.agent.deep.json_model_empty"
	KeyToolAgentDeepJSONMaxTurnsUnsupported          Key = "tool.agent.deep.json_max_turns_unsupported"
	KeyToolAgentDeepJSONMCPServersInvalid            Key = "tool.agent.deep.json_mcp_servers_invalid"
	KeyToolAgentDeepJSONHooksInvalid                 Key = "tool.agent.deep.json_hooks_invalid"
	KeyToolAgentDeepJSONMemoryUnsupported            Key = "tool.agent.deep.json_memory_unsupported"
	KeyToolAgentDeepJSONIsolationUnsupported         Key = "tool.agent.deep.json_isolation_unsupported"
	KeyToolAgentDeepJSONArrayExpected                Key = "tool.agent.deep.json_array_expected"
	KeyToolAgentDeepParentProjectRootEmpty           Key = "tool.agent.deep.parent_project_root_empty"
	KeyToolAgentDeepWorktreeGitRepoRequired          Key = "tool.agent.deep.worktree_git_repo_required"
	KeyToolAgentDeepWorktreeCommitRequired           Key = "tool.agent.deep.worktree_commit_required"
	KeyToolAgentDeepWorktreeCreateFailed             Key = "tool.agent.deep.worktree_create_failed"
	KeyToolAgentDeepPersistedWorktreeBranchMissing   Key = "tool.agent.deep.persisted_worktree_branch_missing"
	KeyToolAgentDeepPersistedWorktreeRepoRootMissing Key = "tool.agent.deep.persisted_worktree_repo_root_missing"
	KeyToolAgentDeepWorktreeRestoreFailed            Key = "tool.agent.deep.worktree_restore_failed"
	KeyToolAgentDeepWorktreeRemoveFailed             Key = "tool.agent.deep.worktree_remove_failed"
	KeyToolAgentDeepOutputDirCreateFailed            Key = "tool.agent.deep.output_dir_create_failed"
	KeyToolAgentDeepSessionRecordIDMissing           Key = "tool.agent.deep.session_record_id_missing"
	KeyToolAgentDeepSessionRunningElsewhere          Key = "tool.agent.deep.session_running_elsewhere"
	KeyToolAgentDeepSessionRestoreUnsupported        Key = "tool.agent.deep.session_restore_unsupported"
	KeyToolAgentDeepSessionRestoreEmpty              Key = "tool.agent.deep.session_restore_empty"
	KeyToolAgentDeepSessionUnavailable               Key = "tool.agent.deep.session_unavailable"
	KeyToolAgentDeepPromptEmpty                      Key = "tool.agent.deep.prompt_empty"
	KeyToolAgentDeepTaskKilledBeforeStart            Key = "tool.agent.deep.task_killed_before_start"
)

var toolAgentDeepKeys = [...]Key{
	KeyToolAgentDeepPermissionSnapshotUnavailable,
	KeyToolAgentDeepResumeContextUntrusted,
	KeyToolAgentDeepRunOutcome,
	KeyToolAgentDeepBackgroundManagerUnavailable,
	KeyToolAgentDeepAutoBackgroundStartFailed,
	KeyToolAgentDeepProviderNotConfigured,
	KeyToolAgentDeepRegistryNotConfigured,
	KeyToolAgentDeepCWDWorktreeConflict,
	KeyToolAgentDeepWorktreeTrustedRootRequired,
	KeyToolAgentDeepSessionMissingInput,
	KeyToolAgentDeepErrorCause,
	KeyToolAgentDeepCWDAbsoluteRequired,
	KeyToolAgentDeepCWDInaccessible,
	KeyToolAgentDeepCWDDirectoryRequired,
	KeyToolAgentDeepCWDOutsideParentScope,
	KeyToolAgentDeepEncounteredError,
	KeyToolAgentDeepRunFailedWithDetail,
	KeyToolAgentDeepSubagentTypeNotAllowed,
	KeyToolAgentDeepForkContextRequired,
	KeyToolAgentDeepForkNestedUnavailable,
	KeyToolAgentDeepIsolationUnsupported,
	KeyToolAgentDeepUnknownSubagentType,
	KeyToolAgentDeepMCPServersRequired,
	KeyToolAgentDeepFrontmatterParseFailed,
	KeyToolAgentDeepCustomPromptEmpty,
	KeyToolAgentDeepMCPServerNamedError,
	KeyToolAgentDeepMCPServerConfigExpected,
	KeyToolAgentDeepMCPServersValueExpected,
	KeyToolAgentDeepMCPCommandRequired,
	KeyToolAgentDeepAgentsJSONParseFailed,
	KeyToolAgentDeepJSONNameEmpty,
	KeyToolAgentDeepJSONDescriptionMissing,
	KeyToolAgentDeepJSONPromptMissing,
	KeyToolAgentDeepJSONModelEmpty,
	KeyToolAgentDeepJSONMaxTurnsUnsupported,
	KeyToolAgentDeepJSONMCPServersInvalid,
	KeyToolAgentDeepJSONHooksInvalid,
	KeyToolAgentDeepJSONMemoryUnsupported,
	KeyToolAgentDeepJSONIsolationUnsupported,
	KeyToolAgentDeepJSONArrayExpected,
	KeyToolAgentDeepParentProjectRootEmpty,
	KeyToolAgentDeepWorktreeGitRepoRequired,
	KeyToolAgentDeepWorktreeCommitRequired,
	KeyToolAgentDeepWorktreeCreateFailed,
	KeyToolAgentDeepPersistedWorktreeBranchMissing,
	KeyToolAgentDeepPersistedWorktreeRepoRootMissing,
	KeyToolAgentDeepWorktreeRestoreFailed,
	KeyToolAgentDeepWorktreeRemoveFailed,
	KeyToolAgentDeepOutputDirCreateFailed,
	KeyToolAgentDeepSessionRecordIDMissing,
	KeyToolAgentDeepSessionRunningElsewhere,
	KeyToolAgentDeepSessionRestoreUnsupported,
	KeyToolAgentDeepSessionRestoreEmpty,
	KeyToolAgentDeepSessionUnavailable,
	KeyToolAgentDeepPromptEmpty,
	KeyToolAgentDeepTaskKilledBeforeStart,
}

func init() {
	entries := map[Key][6]string{
		KeyToolAgentDeepPermissionSnapshotUnavailable: {
			"cannot resume subagent without its complete parent permission snapshot",
			"缺少完整的父级权限快照，无法恢复 subagent",
			"Der Subagent kann ohne vollständigen Berechtigungs-Snapshot des Elternprozesses nicht fortgesetzt werden",
			"完全な親権限スナップショットがないため、subagent を再開できません",
			"완전한 상위 권한 스냅샷이 없어 subagent를 재개할 수 없습니다",
			"Невозможно возобновить subagent без полного снимка родительских разрешений",
		},
		KeyToolAgentDeepResumeContextUntrusted: {
			"cannot resume subagent from untrusted or modified persisted security metadata",
			"持久化安全元数据不可信或已被修改，无法恢复 subagent",
			"Der Subagent kann nicht aus nicht vertrauenswürdigen oder geänderten gespeicherten Sicherheitsmetadaten fortgesetzt werden",
			"永続化されたセキュリティメタデータが信頼できないか変更されているため、subagent を再開できません",
			"저장된 보안 메타데이터를 신뢰할 수 없거나 변경되어 subagent를 재개할 수 없습니다",
			"Невозможно возобновить subagent по недоверенным или изменённым сохранённым метаданным безопасности",
		},
		KeyToolAgentDeepRunOutcome: {
			"agent run %s: %s",
			"Agent 运行状态为 %s：%s",
			"Agent-Lauf %s: %s",
			"Agent の実行 %s: %s",
			"Agent 실행 %s: %s",
			"Запуск Agent %s: %s",
		},
		KeyToolAgentDeepBackgroundManagerUnavailable: {
			"background manager is not available",
			"后台任务管理器不可用",
			"Der Hintergrundmanager ist nicht verfügbar",
			"バックグラウンドマネージャーを使用できません",
			"백그라운드 관리자를 사용할 수 없습니다",
			"Менеджер фоновых задач недоступен",
		},
		KeyToolAgentDeepAutoBackgroundStartFailed: {
			"failed to start auto-background agent: %v",
			"启动自动转后台 Agent 失败：%v",
			"Der automatisch im Hintergrund laufende Agent konnte nicht gestartet werden: %v",
			"自動バックグラウンド Agent を起動できませんでした: %v",
			"자동 백그라운드 Agent를 시작하지 못했습니다: %v",
			"Не удалось запустить Agent с автоматическим переходом в фоновый режим: %v",
		},
		KeyToolAgentDeepProviderNotConfigured: {
			"Agent error: provider is not configured",
			"Agent 错误：尚未配置 provider",
			"Agent-Fehler: Provider ist nicht konfiguriert",
			"Agent エラー: provider が設定されていません",
			"Agent 오류: provider가 구성되지 않았습니다",
			"Ошибка Agent: provider не настроен",
		},
		KeyToolAgentDeepRegistryNotConfigured: {
			"Agent error: registry is not configured",
			"Agent 错误：尚未配置 registry",
			"Agent-Fehler: Registry ist nicht konfiguriert",
			"Agent エラー: registry が設定されていません",
			"Agent 오류: registry가 구성되지 않았습니다",
			"Ошибка Agent: registry не настроен",
		},
		KeyToolAgentDeepCWDWorktreeConflict: {
			`Agent error: cwd cannot be combined with isolation="worktree"`,
			`Agent 错误：cwd 不能与 isolation="worktree" 同时使用`,
			`Agent-Fehler: cwd kann nicht mit isolation="worktree" kombiniert werden`,
			`Agent エラー: cwd と isolation="worktree" は同時に指定できません`,
			`Agent 오류: cwd를 isolation="worktree"와 함께 사용할 수 없습니다`,
			`Ошибка Agent: cwd нельзя сочетать с isolation="worktree"`,
		},
		KeyToolAgentDeepWorktreeTrustedRootRequired: {
			"Agent error: isolation=worktree requires a trusted parent project root: %v",
			"Agent 错误：isolation=worktree 需要可信的父项目根目录：%v",
			"Agent-Fehler: isolation=worktree erfordert ein vertrauenswürdiges übergeordnetes Projektstammverzeichnis: %v",
			"Agent エラー: isolation=worktree には信頼できる親プロジェクトルートが必要です: %v",
			"Agent 오류: isolation=worktree에는 신뢰할 수 있는 상위 프로젝트 루트가 필요합니다: %v",
			"Ошибка Agent: для isolation=worktree требуется доверенный корневой каталог родительского проекта: %v",
		},
		KeyToolAgentDeepSessionMissingInput: {
			"agent session %q is missing persisted input",
			"Agent session %q 缺少持久化输入",
			"In der Agent-Sitzung %q fehlt die gespeicherte Eingabe",
			"Agent session %q に永続化された入力がありません",
			"Agent session %q에 저장된 입력이 없습니다",
			"В Agent session %q отсутствуют сохранённые входные данные",
		},
		KeyToolAgentDeepErrorCause: {
			"Agent error: %v",
			"Agent 错误：%v",
			"Agent-Fehler: %v",
			"Agent エラー: %v",
			"Agent 오류: %v",
			"Ошибка Agent: %v",
		},
		KeyToolAgentDeepCWDAbsoluteRequired: {
			"Agent error: cwd must be an absolute path",
			"Agent 错误：cwd 必须是绝对路径",
			"Agent-Fehler: cwd muss ein absoluter Pfad sein",
			"Agent エラー: cwd は絶対パスで指定してください",
			"Agent 오류: cwd는 절대 경로여야 합니다",
			"Ошибка Agent: cwd должен быть абсолютным путём",
		},
		KeyToolAgentDeepCWDInaccessible: {
			"Agent error: cwd is not accessible: %v",
			"Agent 错误：无法访问 cwd：%v",
			"Agent-Fehler: Auf cwd kann nicht zugegriffen werden: %v",
			"Agent エラー: cwd にアクセスできません: %v",
			"Agent 오류: cwd에 접근할 수 없습니다: %v",
			"Ошибка Agent: cwd недоступен: %v",
		},
		KeyToolAgentDeepCWDDirectoryRequired: {
			"Agent error: cwd must be a directory",
			"Agent 错误：cwd 必须是目录",
			"Agent-Fehler: cwd muss ein Verzeichnis sein",
			"Agent エラー: cwd はディレクトリである必要があります",
			"Agent 오류: cwd는 디렉터리여야 합니다",
			"Ошибка Agent: cwd должен указывать на каталог",
		},
		KeyToolAgentDeepCWDOutsideParentScope: {
			"Agent error: cwd is outside the parent permission scope",
			"Agent 错误：cwd 超出父级权限范围",
			"Agent-Fehler: cwd liegt außerhalb des Berechtigungsbereichs des Elternprozesses",
			"Agent エラー: cwd は親の権限範囲外です",
			"Agent 오류: cwd가 상위 권한 범위를 벗어났습니다",
			"Ошибка Agent: cwd находится вне области родительских разрешений",
		},
		KeyToolAgentDeepEncounteredError: {
			"(Agent encountered error: %v)",
			"（Agent 遇到错误：%v）",
			"(Beim Agent ist ein Fehler aufgetreten: %v)",
			"（Agent でエラーが発生しました: %v）",
			"(Agent에 오류가 발생했습니다: %v)",
			"(В Agent произошла ошибка: %v)",
		},
		KeyToolAgentDeepRunFailedWithDetail: {
			"Agent error: %v: %v",
			"Agent 错误：%v：%v",
			"Agent-Fehler: %v: %v",
			"Agent エラー: %v: %v",
			"Agent 오류: %v: %v",
			"Ошибка Agent: %v: %v",
		},
		KeyToolAgentDeepSubagentTypeNotAllowed: {
			`Agent error: subagent_type %q is not allowed by this agent's Agent(...) tool restriction. Allowed agents: %s`,
			`Agent 错误：当前 Agent(...) tool 的限制不允许 subagent_type %q。允许的 Agent：%s`,
			`Agent-Fehler: subagent_type %q ist durch die Agent(...)-Toolbeschränkung dieses Agents nicht zulässig. Zulässige Agents: %s`,
			`Agent エラー: この Agent の Agent(...) tool 制限では subagent_type %q は許可されていません。許可されている Agent: %s`,
			`Agent 오류: 이 Agent의 Agent(...) tool 제한에서 subagent_type %q은(는) 허용되지 않습니다. 허용된 Agent: %s`,
			`Ошибка Agent: subagent_type %q запрещён ограничением tool Agent(...) для этого Agent. Разрешённые Agent: %s`,
		},
		KeyToolAgentDeepForkContextRequired: {
			"Agent error: fork subagent requires parent tool execution context",
			"Agent 错误：fork subagent 需要父级 tool 执行上下文",
			"Agent-Fehler: Ein Fork-Subagent benötigt den Ausführungskontext des übergeordneten Tools",
			"Agent エラー: fork subagent には親 tool の実行コンテキストが必要です",
			"Agent 오류: fork subagent에는 상위 tool 실행 컨텍스트가 필요합니다",
			"Ошибка Agent: для fork subagent требуется контекст выполнения родительского tool",
		},
		KeyToolAgentDeepForkNestedUnavailable: {
			"Agent error: fork is not available inside a forked worker. Complete the task directly using available tools",
			"Agent 错误：已 fork 的 worker 内不能再次 fork。请直接使用现有 tool 完成任务",
			"Agent-Fehler: Innerhalb eines geforkten Workers ist kein weiterer Fork verfügbar. Schließe die Aufgabe direkt mit den verfügbaren Tools ab",
			"Agent エラー: fork 済み worker 内では fork を使用できません。利用可能な tool でタスクを直接完了してください",
			"Agent 오류: fork된 worker 내부에서는 다시 fork할 수 없습니다. 사용 가능한 tool로 작업을 직접 완료하세요",
			"Ошибка Agent: fork недоступен внутри уже ответвлённого worker. Выполните задачу напрямую с помощью доступных tool",
		},
		KeyToolAgentDeepIsolationUnsupported: {
			"Agent error: unsupported isolation mode %q",
			"Agent 错误：不支持 isolation 模式 %q",
			"Agent-Fehler: Nicht unterstützter Isolationsmodus %q",
			"Agent エラー: isolation モード %q はサポートされていません",
			"Agent 오류: 지원하지 않는 isolation 모드 %q",
			"Ошибка Agent: режим isolation %q не поддерживается",
		},
		KeyToolAgentDeepUnknownSubagentType: {
			"Agent error: unknown subagent_type %q. Available agents: %s",
			"Agent 错误：未知的 subagent_type %q。可用 Agent：%s",
			"Agent-Fehler: Unbekannter subagent_type %q. Verfügbare Agents: %s",
			"Agent エラー: 不明な subagent_type %q です。利用可能な Agent: %s",
			"Agent 오류: 알 수 없는 subagent_type %q입니다. 사용 가능한 Agent: %s",
			"Ошибка Agent: неизвестный subagent_type %q. Доступные Agent: %s",
		},
		KeyToolAgentDeepMCPServersRequired: {
			"Agent error: agent %q requires MCP servers with tools: %s",
			"Agent 错误：Agent %q 需要提供相应 tool 的 MCP server：%s",
			"Agent-Fehler: Agent %q benötigt MCP-Server mit folgenden Tools: %s",
			"Agent エラー: Agent %q には次の tool を提供する MCP server が必要です: %s",
			"Agent 오류: Agent %q에는 다음 tool을 제공하는 MCP server가 필요합니다: %s",
			"Ошибка Agent: Agent %q требуются серверы MCP со следующими tool: %s",
		},
		KeyToolAgentDeepFrontmatterParseFailed: {
			"Agent error: failed to parse agent frontmatter in %s: %v",
			"Agent 错误：解析 %s 中的 Agent frontmatter 失败：%v",
			"Agent-Fehler: Das Agent-Frontmatter in %s konnte nicht geparst werden: %v",
			"Agent エラー: %s の Agent frontmatter を解析できませんでした: %v",
			"Agent 오류: %s의 Agent frontmatter를 파싱하지 못했습니다: %v",
			"Ошибка Agent: не удалось разобрать frontmatter Agent в %s: %v",
		},
		KeyToolAgentDeepCustomPromptEmpty: {
			"Agent error: custom agent %q has an empty prompt",
			"Agent 错误：自定义 Agent %q 的 prompt 为空",
			"Agent-Fehler: Der benutzerdefinierte Agent %q hat einen leeren Prompt",
			"Agent エラー: カスタム Agent %q の prompt が空です",
			"Agent 오류: 사용자 지정 Agent %q의 prompt가 비어 있습니다",
			"Ошибка Agent: у пользовательского Agent %q пустой prompt",
		},
		KeyToolAgentDeepMCPServerNamedError: {
			"%s: %v",
			"%s：%v",
			"%s: %v",
			"%s: %v",
			"%s: %v",
			"%s: %v",
		},
		KeyToolAgentDeepMCPServerConfigExpected: {
			"expected server name or inline server config",
			"应提供 server 名称或内联 server 配置",
			"Servername oder eingebettete Serverkonfiguration erwartet",
			"server 名またはインライン server 設定を指定してください",
			"server 이름 또는 인라인 server 구성이 필요합니다",
			"Ожидалось имя server или встроенная конфигурация server",
		},
		KeyToolAgentDeepMCPServersValueExpected: {
			"expected string, list, or object",
			"应提供字符串、列表或对象",
			"Zeichenfolge, Liste oder Objekt erwartet",
			"文字列、リスト、またはオブジェクトを指定してください",
			"문자열, 목록 또는 객체가 필요합니다",
			"Ожидалась строка, список или объект",
		},
		KeyToolAgentDeepMCPCommandRequired: {
			"command is required",
			"必须提供 command",
			"command ist erforderlich",
			"command は必須です",
			"command가 필요합니다",
			"Требуется command",
		},
		KeyToolAgentDeepAgentsJSONParseFailed: {
			"Agent error: failed to parse --agents JSON: %v",
			"Agent 错误：解析 --agents JSON 失败：%v",
			"Agent-Fehler: --agents-JSON konnte nicht geparst werden: %v",
			"Agent エラー: --agents JSON を解析できませんでした: %v",
			"Agent 오류: --agents JSON을 파싱하지 못했습니다: %v",
			"Ошибка Agent: не удалось разобрать JSON из --agents: %v",
		},
		KeyToolAgentDeepJSONNameEmpty: {
			"Agent error: JSON agent name must not be empty",
			"Agent 错误：JSON Agent 名称不能为空",
			"Agent-Fehler: Der Name des JSON-Agents darf nicht leer sein",
			"Agent エラー: JSON Agent の名前は空にできません",
			"Agent 오류: JSON Agent 이름은 비워 둘 수 없습니다",
			"Ошибка Agent: имя JSON Agent не должно быть пустым",
		},
		KeyToolAgentDeepJSONDescriptionMissing: {
			"Agent error: JSON agent %q is missing description",
			"Agent 错误：JSON Agent %q 缺少 description",
			"Agent-Fehler: Beim JSON-Agent %q fehlt description",
			"Agent エラー: JSON Agent %q に description がありません",
			"Agent 오류: JSON Agent %q에 description이 없습니다",
			"Ошибка Agent: у JSON Agent %q отсутствует description",
		},
		KeyToolAgentDeepJSONPromptMissing: {
			"Agent error: JSON agent %q is missing prompt",
			"Agent 错误：JSON Agent %q 缺少 prompt",
			"Agent-Fehler: Beim JSON-Agent %q fehlt prompt",
			"Agent エラー: JSON Agent %q に prompt がありません",
			"Agent 오류: JSON Agent %q에 prompt가 없습니다",
			"Ошибка Agent: у JSON Agent %q отсутствует prompt",
		},
		KeyToolAgentDeepJSONModelEmpty: {
			"Agent error: JSON agent %q uses empty model",
			"Agent 错误：JSON Agent %q 使用了空的 model",
			"Agent-Fehler: JSON-Agent %q verwendet ein leeres model",
			"Agent エラー: JSON Agent %q の model が空です",
			"Agent 오류: JSON Agent %q이(가) 빈 model을 사용합니다",
			"Ошибка Agent: JSON Agent %q использует пустой model",
		},
		KeyToolAgentDeepJSONMaxTurnsUnsupported: {
			"Agent error: JSON agent %q uses unsupported maxTurns %d",
			"Agent 错误：JSON Agent %q 使用了不支持的 maxTurns 值 %d",
			"Agent-Fehler: JSON-Agent %q verwendet den nicht unterstützten maxTurns-Wert %d",
			"Agent エラー: JSON Agent %q はサポートされていない maxTurns %d を使用しています",
			"Agent 오류: JSON Agent %q이(가) 지원하지 않는 maxTurns %d을(를) 사용합니다",
			"Ошибка Agent: JSON Agent %q использует неподдерживаемое значение maxTurns %d",
		},
		KeyToolAgentDeepJSONMCPServersInvalid: {
			"Agent error: JSON agent %q has invalid mcpServers: %v",
			"Agent 错误：JSON Agent %q 的 mcpServers 无效：%v",
			"Agent-Fehler: JSON-Agent %q enthält ungültige mcpServers: %v",
			"Agent エラー: JSON Agent %q の mcpServers が無効です: %v",
			"Agent 오류: JSON Agent %q의 mcpServers가 올바르지 않습니다: %v",
			"Ошибка Agent: у JSON Agent %q недопустимое значение mcpServers: %v",
		},
		KeyToolAgentDeepJSONHooksInvalid: {
			"Agent error: JSON agent %q has invalid hooks: %v",
			"Agent 错误：JSON Agent %q 的 hooks 无效：%v",
			"Agent-Fehler: JSON-Agent %q enthält ungültige hooks: %v",
			"Agent エラー: JSON Agent %q の hooks が無効です: %v",
			"Agent 오류: JSON Agent %q의 hooks가 올바르지 않습니다: %v",
			"Ошибка Agent: у JSON Agent %q недопустимое значение hooks: %v",
		},
		KeyToolAgentDeepJSONMemoryUnsupported: {
			"Agent error: JSON agent %q uses unsupported memory scope %q",
			"Agent 错误：JSON Agent %q 使用了不支持的 memory scope %q",
			"Agent-Fehler: JSON-Agent %q verwendet den nicht unterstützten memory scope %q",
			"Agent エラー: JSON Agent %q はサポートされていない memory scope %q を使用しています",
			"Agent 오류: JSON Agent %q이(가) 지원하지 않는 memory scope %q을(를) 사용합니다",
			"Ошибка Agent: JSON Agent %q использует неподдерживаемый memory scope %q",
		},
		KeyToolAgentDeepJSONIsolationUnsupported: {
			"Agent error: JSON agent %q uses unsupported isolation %q",
			"Agent 错误：JSON Agent %q 使用了不支持的 isolation %q",
			"Agent-Fehler: JSON-Agent %q verwendet die nicht unterstützte isolation %q",
			"Agent エラー: JSON Agent %q はサポートされていない isolation %q を使用しています",
			"Agent 오류: JSON Agent %q이(가) 지원하지 않는 isolation %q을(를) 사용합니다",
			"Ошибка Agent: JSON Agent %q использует неподдерживаемое значение isolation %q",
		},
		KeyToolAgentDeepJSONArrayExpected: {
			"expected array",
			"应提供数组",
			"Array erwartet",
			"配列を指定してください",
			"배열이 필요합니다",
			"Ожидался массив",
		},
		KeyToolAgentDeepParentProjectRootEmpty: {
			"parent project root is empty",
			"父项目根目录为空",
			"Das übergeordnete Projektstammverzeichnis ist leer",
			"親プロジェクトのルートが空です",
			"상위 프로젝트 루트가 비어 있습니다",
			"Корневой каталог родительского проекта пуст",
		},
		KeyToolAgentDeepWorktreeGitRepoRequired: {
			"Agent error: isolation=worktree requires running inside a git repository",
			"Agent 错误：isolation=worktree 要求当前位于 git 仓库中",
			"Agent-Fehler: isolation=worktree erfordert die Ausführung in einem git-Repository",
			"Agent エラー: isolation=worktree を使用するには git リポジトリ内で実行する必要があります",
			"Agent 오류: isolation=worktree를 사용하려면 git 저장소 안에서 실행해야 합니다",
			"Ошибка Agent: для isolation=worktree необходимо выполнять команду внутри репозитория git",
		},
		KeyToolAgentDeepWorktreeCommitRequired: {
			"Agent error: isolation=worktree requires a git repository with at least one commit",
			"Agent 错误：isolation=worktree 要求 git 仓库中至少有一个 commit",
			"Agent-Fehler: isolation=worktree erfordert ein git-Repository mit mindestens einem Commit",
			"Agent エラー: isolation=worktree には 1 件以上の commit がある git リポジトリが必要です",
			"Agent 오류: isolation=worktree에는 commit이 하나 이상 있는 git 저장소가 필요합니다",
			"Ошибка Agent: для isolation=worktree требуется репозиторий git хотя бы с одним commit",
		},
		KeyToolAgentDeepWorktreeCreateFailed: {
			"Agent error: failed to create worktree: %s",
			"Agent 错误：创建 worktree 失败：%s",
			"Agent-Fehler: Worktree konnte nicht erstellt werden: %s",
			"Agent エラー: worktree を作成できませんでした: %s",
			"Agent 오류: worktree를 만들지 못했습니다: %s",
			"Ошибка Agent: не удалось создать worktree: %s",
		},
		KeyToolAgentDeepPersistedWorktreeBranchMissing: {
			"Agent error: persisted worktree %q is missing branch metadata",
			"Agent 错误：持久化 worktree %q 缺少 branch 元数据",
			"Agent-Fehler: Beim gespeicherten Worktree %q fehlen Branch-Metadaten",
			"Agent エラー: 永続化された worktree %q に branch メタデータがありません",
			"Agent 오류: 저장된 worktree %q에 branch 메타데이터가 없습니다",
			"Ошибка Agent: у сохранённого worktree %q отсутствуют метаданные branch",
		},
		KeyToolAgentDeepPersistedWorktreeRepoRootMissing: {
			"Agent error: persisted worktree %q is missing repo root metadata",
			"Agent 错误：持久化 worktree %q 缺少 repo root 元数据",
			"Agent-Fehler: Beim gespeicherten Worktree %q fehlen Metadaten zum Repository-Stammverzeichnis",
			"Agent エラー: 永続化された worktree %q に repo root メタデータがありません",
			"Agent 오류: 저장된 worktree %q에 repo root 메타데이터가 없습니다",
			"Ошибка Agent: у сохранённого worktree %q отсутствуют метаданные repo root",
		},
		KeyToolAgentDeepWorktreeRestoreFailed: {
			"Agent error: failed to restore worktree %q from branch %q: %s",
			"Agent 错误：恢复 worktree %q（branch %q）失败：%s",
			"Agent-Fehler: Worktree %q konnte nicht aus Branch %q wiederhergestellt werden: %s",
			"Agent エラー: worktree %q を branch %q から復元できませんでした: %s",
			"Agent 오류: worktree %q을(를) branch %q에서 복원하지 못했습니다: %s",
			"Ошибка Agent: не удалось восстановить worktree %q из branch %q: %s",
		},
		KeyToolAgentDeepWorktreeRemoveFailed: {
			"Agent error: failed to remove clean worktree %q: %s",
			"Agent 错误：无法移除无变更的 worktree %q：%s",
			"Agent-Fehler: Der unveränderte Worktree %q konnte nicht entfernt werden: %s",
			"Agent エラー: 変更のない worktree %q を削除できませんでした: %s",
			"Agent 오류: 변경 사항이 없는 worktree %q을(를) 제거하지 못했습니다: %s",
			"Ошибка Agent: не удалось удалить worktree %q без изменений: %s",
		},
		KeyToolAgentDeepOutputDirCreateFailed: {
			"create background task output dir: %v",
			"创建后台任务输出目录失败：%v",
			"Das Ausgabeverzeichnis für Hintergrundaufgaben konnte nicht erstellt werden: %v",
			"バックグラウンドタスクの出力ディレクトリを作成できませんでした: %v",
			"백그라운드 작업 출력 디렉터리를 만들지 못했습니다: %v",
			"Не удалось создать каталог вывода фоновой задачи: %v",
		},
		KeyToolAgentDeepSessionRecordIDMissing: {
			"agent session record is missing id",
			"Agent session 记录缺少 id",
			"Im Datensatz der Agent-Sitzung fehlt die id",
			"Agent session レコードに id がありません",
			"Agent session 레코드에 id가 없습니다",
			"В записи Agent session отсутствует id",
		},
		KeyToolAgentDeepSessionRunningElsewhere: {
			"agent %q is running in another process and cannot be resumed from this session",
			"Agent %q 正在另一个进程中运行，无法从当前 session 恢复",
			"Agent %q läuft in einem anderen Prozess und kann aus dieser Sitzung nicht fortgesetzt werden",
			"Agent %q は別のプロセスで実行中のため、この session から再開できません",
			"Agent %q이(가) 다른 프로세스에서 실행 중이므로 이 session에서 재개할 수 없습니다",
			"Agent %q выполняется в другом процессе и не может быть возобновлён из этой session",
		},
		KeyToolAgentDeepSessionRestoreUnsupported: {
			"agent %q is persisted but this runtime cannot restore agent sessions",
			"Agent %q 已持久化，但当前 runtime 无法恢复 Agent session",
			"Agent %q ist gespeichert, aber diese Runtime kann Agent-Sitzungen nicht wiederherstellen",
			"Agent %q は永続化されていますが、この runtime では Agent session を復元できません",
			"Agent %q이(가) 저장되어 있지만 이 runtime은 Agent session을 복원할 수 없습니다",
			"Agent %q сохранён, но эта runtime не может восстанавливать Agent session",
		},
		KeyToolAgentDeepSessionRestoreEmpty: {
			"agent %q restore produced no runnable session",
			"恢复 Agent %q 后未得到可运行的 session",
			"Die Wiederherstellung von Agent %q ergab keine ausführbare Sitzung",
			"Agent %q を復元しましたが、実行可能な session がありません",
			"Agent %q을(를) 복원했지만 실행 가능한 session이 없습니다",
			"Восстановление Agent %q не создало пригодную для запуска session",
		},
		KeyToolAgentDeepSessionUnavailable: {
			"%v: agent session is unavailable",
			"%v：Agent session 不可用",
			"%v: Agent-Sitzung ist nicht verfügbar",
			"%v: Agent session を使用できません",
			"%v: Agent session을 사용할 수 없습니다",
			"%v: Agent session недоступна",
		},
		KeyToolAgentDeepPromptEmpty: {
			"prompt must not be empty",
			"prompt 不能为空",
			"prompt darf nicht leer sein",
			"prompt は空にできません",
			"prompt는 비워 둘 수 없습니다",
			"prompt не должен быть пустым",
		},
		KeyToolAgentDeepTaskKilledBeforeStart: {
			"agent task was killed before it started",
			"Agent 任务在启动前已被终止",
			"Die Agent-Aufgabe wurde vor dem Start beendet",
			"Agent タスクは開始前に強制終了されました",
			"Agent 작업이 시작되기 전에 종료되었습니다",
			"Задача Agent была завершена до запуска",
		},
	}

	for key, translations := range entries {
		semanticTranslations[key] = map[Language]string{
			LangEN: translations[0], LangZH: translations[1], LangDE: translations[2],
			LangJA: translations[3], LangKO: translations[4], LangRU: translations[5],
		}
	}
}
