package i18n

import "strings"

// Semantic copy owned by the root runtime and its session/fork boundaries.
// Paths, IDs, commands, provider/model names, raw process output, and wrapped
// errors remain parameters so callers can preserve them verbatim.
const (
	KeyRootGoalProgressSaveAfterEvaluatorFailure Key = "root.goal.progress_save_after_evaluator_failure"
	KeyRootGoalProgressSaveFailed                Key = "root.goal.progress_save_failed"
	KeyRootGoalReasonEvaluatorUnavailable        Key = "root.goal.reason.evaluator_unavailable"
	KeyRootGoalReasonEvaluatorFailed             Key = "root.goal.reason.evaluator_failed"
	KeyRootGoalObjectiveRequired                 Key = "root.goal.objective_required"
	KeyRootGoalObjectiveTooLong                  Key = "root.goal.objective_too_long"
	KeyRootGoalTransitionInvalid                 Key = "root.goal.transition_invalid"
	KeyRootGoalStatusActive                      Key = "root.goal.status.active"
	KeyRootGoalStatusPaused                      Key = "root.goal.status.paused"
	KeyRootGoalStatusAchieved                    Key = "root.goal.status.achieved"
	KeyRootGoalStatusBlocked                     Key = "root.goal.status.blocked"
	KeyRootGoalStatusCleared                     Key = "root.goal.status.cleared"
	KeyRootGoalActionEdit                        Key = "root.goal.action.edit"
	KeyRootGoalActionPause                       Key = "root.goal.action.pause"
	KeyRootGoalActionResume                      Key = "root.goal.action.resume"
	KeyRootGoalActionClear                       Key = "root.goal.action.clear"

	KeyRootGoalRuntimeRepositoryMissing Key = "root.goal_runtime.repository_missing"
	KeyRootGoalRuntimeMetadataLoad      Key = "root.goal_runtime.metadata_load"
	KeyRootGoalRuntimeMetadataSave      Key = "root.goal_runtime.metadata_save"
	KeyRootGoalRuntimeTranscriptCreate  Key = "root.goal_runtime.transcript_create"
	KeyRootGoalRuntimeMetadataUpdate    Key = "root.goal_runtime.metadata_update"
	KeyRootGoalRuntimeSessionResolver   Key = "root.goal_runtime.session_resolver_missing"
	KeyRootGoalRuntimeSessionID         Key = "root.goal_runtime.session_id_required"
	KeyRootGoalRuntimeProjectResolver   Key = "root.goal_runtime.project_resolver_missing"
	KeyRootGoalRuntimeProjectDirectory  Key = "root.goal_runtime.project_directory_required"
	KeyRootGoalRuntimeExecutionContext  Key = "root.goal_runtime.execution_context_required"
	KeyRootGoalRuntimeProjectRoot       Key = "root.goal_runtime.project_root_required"

	KeyRootForkImagePlaceholder        Key = "root.fork.placeholder.image"
	KeyRootForkDocumentPlaceholder     Key = "root.fork.placeholder.document"
	KeyRootForkPromptPlaceholder       Key = "root.fork.placeholder.prompt"
	KeyRootForkRepositoryUnavailable   Key = "root.fork.repository_unavailable"
	KeyRootForkPointOutsideSnapshot    Key = "root.fork.point_outside_snapshot"
	KeyRootForkIdentityIncomplete      Key = "root.fork.identity_incomplete"
	KeyRootForkMetadataUpdate          Key = "root.fork.metadata_update"
	KeyRootForkTerminalUnavailable     Key = "root.fork.terminal_unavailable"
	KeyRootForkTerminalOpen            Key = "root.fork.terminal_open"
	KeyRootForkExecutableResolve       Key = "root.fork.executable_resolve"
	KeyRootForkProcessLaunch           Key = "root.fork.process_launch"
	KeyRootForkOSAScriptFind           Key = "root.fork.osascript_find"
	KeyRootForkSupportedTerminalAbsent Key = "root.fork.supported_terminal_absent"
	KeyRootForkWindowsTerminalRequired Key = "root.fork.windows_terminal_required"
	KeyRootForkTerminalUnsupportedOS   Key = "root.fork.terminal_unsupported_os"

	KeyRootSessionForkSourceIncomplete    Key = "root.session_fork.source_incomplete"
	KeyRootSessionForkSnapshotEmpty       Key = "root.session_fork.snapshot_empty"
	KeyRootSessionForkSourceMissing       Key = "root.session_fork.source_missing"
	KeyRootSessionForkMetadataLoad        Key = "root.session_fork.metadata_load"
	KeyRootSessionForkArtifactsCopy       Key = "root.session_fork.artifacts_copy"
	KeyRootSessionForkTranscriptSave      Key = "root.session_fork.transcript_save"
	KeyRootSessionForkMetadataSave        Key = "root.session_fork.metadata_save"
	KeyRootSessionForkArtifactNotDir      Key = "root.session_fork.artifact_not_directory"
	KeyRootSessionForkArtifactSymlink     Key = "root.session_fork.artifact_symlink"
	KeyRootSessionForkArtifactUnsupported Key = "root.session_fork.artifact_unsupported"

	KeyRootSessionHookSettingsLoad    Key = "root.session_switch.hook_settings_load"
	KeyRootSessionHookDirectoryLoad   Key = "root.session_switch.hook_directory_load"
	KeyRootSessionSwitcherIncomplete  Key = "root.session_switch.incomplete"
	KeyRootSessionTargetIDRequired    Key = "root.session_switch.target_id_required"
	KeyRootSessionCWDUnavailable      Key = "root.session_switch.cwd_unavailable"
	KeyRootSessionCWDNotDirectory     Key = "root.session_switch.cwd_not_directory"
	KeyRootSessionPreparedMismatch    Key = "root.session_switch.prepared_mismatch"
	KeyRootSessionResumeUnsupported   Key = "root.session_switch.resume_unsupported"
	KeyRootSessionTargetDirectoryOpen Key = "root.session_switch.target_directory_open"
	KeyRootSessionConversationCommit  Key = "root.session_switch.conversation_commit"
	KeyRootWorkspaceRequired          Key = "root.session_switch.workspace_required"
	KeyRootMCPSettingsLoad            Key = "root.session_switch.mcp_settings_load"
	KeyRootMCPSettingsInvalid         Key = "root.session_switch.mcp_settings_invalid"
	KeyRootWorktreeRebindRejected     Key = "root.worktree.rebind_rejected"

	KeyRootModelSettingsRead       Key = "root.model_settings.read"
	KeyRootModelSettingsParse      Key = "root.model_settings.parse"
	KeyRootModelSettingsHome       Key = "root.model_settings.home"
	KeyRootModelSettingsDirectory  Key = "root.model_settings.directory"
	KeyRootModelSettingsMarshal    Key = "root.model_settings.marshal"
	KeyRootModelSettingsTempCreate Key = "root.model_settings.temp_create"
	KeyRootModelSettingsTempWrite  Key = "root.model_settings.temp_write"
	KeyRootModelSettingsTempChmod  Key = "root.model_settings.temp_chmod"
	KeyRootModelSettingsTempClose  Key = "root.model_settings.temp_close"
	KeyRootModelSettingsReplace    Key = "root.model_settings.replace"
	KeyRootPlanModeRestore         Key = "root.plan_mode.restore"
	KeyRootPlanModeUnsupported     Key = "root.plan_mode.unsupported"
	KeyRootImageRead               Key = "root.image.read"

	KeyRootLogCronExecutionFailed Key = "root.log.cron_execution_failed"
	KeyRootLogWorktreeHooksFailed Key = "root.log.worktree_hooks_failed"
	KeyRootLogMCPRefreshFailed    Key = "root.log.mcp_refresh_failed"
	KeyRootLogSessionCorrupt      Key = "root.log.session_corrupt"

	KeyRootAgentEvidenceTranscript     Key = "root.agent.evidence_transcript"
	KeyRootAgentPhaseStart             Key = "root.agent.phase.start"
	KeyRootAgentPhaseMCPReady          Key = "root.agent.phase.mcp_ready"
	KeyRootAgentPhaseToolUse           Key = "root.agent.phase.tool_use"
	KeyRootAgentPhaseAssistant         Key = "root.agent.phase.assistant"
	KeyRootAgentPhaseError             Key = "root.agent.phase.error"
	KeyRootAgentPhaseAborted           Key = "root.agent.phase.aborted"
	KeyRootAgentPhaseBackground        Key = "root.agent.phase.background"
	KeyRootAgentPhaseRemoteLaunched    Key = "root.agent.phase.remote_launched"
	KeyRootAgentQueueActiveRun         Key = "root.agent.queue.active_run"
	KeyRootAgentQueueWorkerCapacity    Key = "root.agent.queue.worker_capacity"
	KeyRootAgentReasonCompleted        Key = "root.agent.reason.completed"
	KeyRootAgentReasonMaxTurns         Key = "root.agent.reason.max_turns"
	KeyRootAgentReasonRuntimeInterrupt Key = "root.agent.reason.runtime_interrupted"
	KeyRootAgentReasonDeadline         Key = "root.agent.reason.deadline_exceeded"
	KeyRootAgentReasonContextCancel    Key = "root.agent.reason.context_cancelled"
	KeyRootAgentReasonPartialError     Key = "root.agent.reason.error_after_partial"
	KeyRootAgentReasonError            Key = "root.agent.reason.error"
	KeyRootAgentReasonOutputOpen       Key = "root.agent.reason.output_open_failed"
	KeyRootAgentReasonKilled           Key = "root.agent.reason.killed"
	KeyRootAgentReasonProcessRestart   Key = "root.agent.reason.process_restart"
)

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}

	add(KeyRootGoalProgressSaveAfterEvaluatorFailure, "Could not save goal progress after the evaluator failed", "目标评估失败后无法保存进度", "Zielfortschritt konnte nach dem Ausfall der Auswertung nicht gespeichert werden", "目標評価の失敗後に進捗を保存できませんでした", "목표 평가 실패 후 진행 상황을 저장하지 못했습니다", "Не удалось сохранить ход работы после сбоя оценки цели")
	add(KeyRootGoalProgressSaveFailed, "Could not save goal progress; automatic continuation stopped", "无法保存目标进度；已停止自动续写", "Zielfortschritt konnte nicht gespeichert werden; automatische Fortsetzung gestoppt", "目標の進捗を保存できませんでした。自動続行を停止しました", "목표 진행 상황을 저장하지 못해 자동 계속을 중지했습니다", "Не удалось сохранить ход работы по цели; автоматическое продолжение остановлено")
	add(KeyRootGoalReasonEvaluatorUnavailable, "Goal evaluator unavailable", "目标评估器不可用", "Zielauswertung nicht verfügbar", "目標評価を利用できません", "목표 평가기를 사용할 수 없음", "Оценщик цели недоступен")
	add(KeyRootGoalReasonEvaluatorFailed, "goal evaluator failed: %v", "目标评估失败：%v", "Zielauswertung fehlgeschlagen: %v", "目標の評価に失敗しました: %v", "목표 평가 실패: %v", "Ошибка оценки цели: %v")
	add(KeyRootGoalObjectiveRequired, "A goal objective is required", "必须填写目标", "Ein Ziel ist erforderlich", "目標を入力してください", "목표를 입력해야 합니다", "Необходимо указать цель")
	add(KeyRootGoalObjectiveTooLong, "The goal objective must not exceed %d characters", "目标不得超过 %d 个字符", "Das Ziel darf höchstens %d Zeichen lang sein", "目標は %d 文字以内で入力してください", "목표는 %d자를 초과할 수 없습니다", "Цель не должна превышать %d символов")
	add(KeyRootGoalTransitionInvalid, "Cannot %s a goal whose status is %s", "状态为%[2]s的目标无法%[1]s", "Ein Ziel mit Status %[2]s kann nicht %[1]s werden", "状態が%[2]sの目標に%[1]sを実行できません", "상태가 %[2]s인 목표에는 %[1]s 작업을 수행할 수 없습니다", "Нельзя выполнить действие «%s» для цели со статусом «%s»")
	add(KeyRootGoalStatusActive, "active", "进行中", "aktiv", "進行中", "진행 중", "активна")
	add(KeyRootGoalStatusPaused, "paused", "已暂停", "pausiert", "一時停止中", "일시 중지됨", "приостановлена")
	add(KeyRootGoalStatusAchieved, "achieved", "已完成", "erreicht", "達成済み", "달성됨", "достигнута")
	add(KeyRootGoalStatusBlocked, "blocked", "受阻", "blockiert", "ブロック中", "차단됨", "заблокирована")
	add(KeyRootGoalStatusCleared, "cleared", "已清除", "gelöscht", "消去済み", "지워짐", "очищена")
	add(KeyRootGoalActionEdit, "edit", "编辑", "bearbeiten", "編集", "수정", "изменить")
	add(KeyRootGoalActionPause, "pause", "暂停", "pausieren", "一時停止", "일시 중지", "приостановить")
	add(KeyRootGoalActionResume, "resume", "恢复", "fortsetzen", "再開", "재개", "возобновить")
	add(KeyRootGoalActionClear, "clear", "清除", "löschen", "消去", "지우기", "очистить")

	add(KeyRootGoalRuntimeRepositoryMissing, "Goal repository is not configured", "尚未配置目标存储库", "Ziel-Repository ist nicht konfiguriert", "目標リポジトリが設定されていません", "목표 저장소가 구성되지 않았습니다", "Репозиторий целей не настроен")
	add(KeyRootGoalRuntimeMetadataLoad, "Could not load goal metadata", "无法加载目标元数据", "Zielmetadaten konnten nicht geladen werden", "目標のメタデータを読み込めませんでした", "목표 메타데이터를 불러오지 못했습니다", "Не удалось загрузить метаданные цели")
	add(KeyRootGoalRuntimeMetadataSave, "Could not save goal metadata", "无法保存目标元数据", "Zielmetadaten konnten nicht gespeichert werden", "目標のメタデータを保存できませんでした", "목표 메타데이터를 저장하지 못했습니다", "Не удалось сохранить метаданные цели")
	add(KeyRootGoalRuntimeTranscriptCreate, "Could not create the goal transcript", "无法创建目标对话记录", "Zieltranskript konnte nicht erstellt werden", "目標の会話履歴を作成できませんでした", "목표 대화 기록을 만들지 못했습니다", "Не удалось создать журнал цели")
	add(KeyRootGoalRuntimeMetadataUpdate, "Could not update goal metadata", "无法更新目标元数据", "Zielmetadaten konnten nicht aktualisiert werden", "目標のメタデータを更新できませんでした", "목표 메타데이터를 업데이트하지 못했습니다", "Не удалось обновить метаданные цели")
	add(KeyRootGoalRuntimeSessionResolver, "Goal session resolver is not configured", "尚未配置目标会话解析器", "Sitzungsauflösung für Ziele ist nicht konfiguriert", "目標のセッション解決処理が設定されていません", "목표 세션 확인기가 구성되지 않았습니다", "Средство разрешения сеанса цели не настроено")
	add(KeyRootGoalRuntimeSessionID, "A goal session ID is required", "必须提供目标会话 ID", "Eine Sitzungs-ID für das Ziel ist erforderlich", "目標のセッション ID が必要です", "목표 세션 ID가 필요합니다", "Необходим ID сеанса цели")
	add(KeyRootGoalRuntimeProjectResolver, "Goal project resolver is not configured", "尚未配置目标项目解析器", "Projektauflösung für Ziele ist nicht konfiguriert", "目標のプロジェクト解決処理が設定されていません", "목표 프로젝트 확인기가 구성되지 않았습니다", "Средство разрешения проекта цели не настроено")
	add(KeyRootGoalRuntimeProjectDirectory, "A goal project directory is required", "必须提供目标项目目录", "Ein Projektverzeichnis für das Ziel ist erforderlich", "目標のプロジェクトディレクトリが必要です", "목표 프로젝트 디렉터리가 필요합니다", "Необходим каталог проекта цели")
	add(KeyRootGoalRuntimeExecutionContext, "A goal tool execution context is required", "必须提供目标工具执行上下文", "Ein Ausführungskontext für das Ziel-Tool ist erforderlich", "目標ツールの実行コンテキストが必要です", "목표 도구 실행 컨텍스트가 필요합니다", "Необходим контекст выполнения инструмента цели")
	add(KeyRootGoalRuntimeProjectRoot, "A goal project root is required", "必须提供目标项目根目录", "Ein Projektstammverzeichnis für das Ziel ist erforderlich", "目標のプロジェクトルートが必要です", "목표 프로젝트 루트가 필요합니다", "Необходим корневой каталог проекта цели")

	add(KeyRootForkImagePlaceholder, "[image]", "[图像]", "[Bild]", "[画像]", "[이미지]", "[изображение]")
	add(KeyRootForkDocumentPlaceholder, "[document]", "[文档]", "[Dokument]", "[ドキュメント]", "[문서]", "[документ]")
	add(KeyRootForkPromptPlaceholder, "[prompt]", "[提示词]", "[Prompt]", "[prompt]", "[prompt]", "[prompt]")
	add(KeyRootForkRepositoryUnavailable, "Session repository is unavailable", "会话存储库不可用", "Sitzungs-Repository ist nicht verfügbar", "セッションリポジトリを利用できません", "세션 저장소를 사용할 수 없습니다", "Репозиторий сеансов недоступен")
	add(KeyRootForkPointOutsideSnapshot, "Fork point %d is outside the %d-message snapshot", "分叉点 %d 超出包含 %d 条消息的快照", "Fork-Punkt %d liegt außerhalb des Snapshots mit %d Nachrichten", "フォーク地点 %d は %d 件のメッセージを含むスナップショットの範囲外です", "포크 지점 %d이(가) 메시지 %d개의 스냅샷 범위를 벗어났습니다", "Точка ветвления %d находится за пределами снимка из %d сообщений")
	add(KeyRootForkIdentityIncomplete, "Active session identity is incomplete", "当前会话标识不完整", "Identität der aktiven Sitzung ist unvollständig", "アクティブなセッションの識別情報が不完全です", "활성 세션 식별 정보가 완전하지 않습니다", "Идентификатор активного сеанса неполон")
	add(KeyRootForkMetadataUpdate, "Forked session %s was created, but its runtime metadata could not be updated", "已创建分叉会话 %s，但无法更新其运行时元数据", "Fork-Sitzung %s wurde erstellt, ihre Laufzeitmetadaten konnten jedoch nicht aktualisiert werden", "フォークしたセッション %s は作成されましたが、ランタイムメタデータを更新できませんでした", "포크된 세션 %s을(를) 만들었지만 런타임 메타데이터를 업데이트하지 못했습니다", "Ответвлённый сеанс %s создан, но его метаданные среды выполнения обновить не удалось")
	add(KeyRootForkTerminalUnavailable, "Forked session %s was created, but a new terminal tab cannot be opened; resume it with %s --session-id %s", "已创建分叉会话 %s，但无法打开新的终端标签页；请使用 %s --session-id %s 恢复", "Fork-Sitzung %s wurde erstellt, aber es kann kein neuer Terminal-Tab geöffnet werden; setze sie mit %s --session-id %s fort", "フォークしたセッション %s は作成されましたが、新しいターミナルタブを開けません。%s --session-id %s で再開してください", "포크된 세션 %s을(를) 만들었지만 새 터미널 탭을 열 수 없습니다. %s --session-id %s로 재개하세요", "Ответвлённый сеанс %s создан, но открыть новую вкладку терминала нельзя; возобновите его командой %s --session-id %s")
	add(KeyRootForkTerminalOpen, "Forked session %s was created, but its terminal tab could not be opened; resume it with %s --session-id %s", "已创建分叉会话 %s，但无法打开其终端标签页；请使用 %s --session-id %s 恢复", "Fork-Sitzung %s wurde erstellt, ihr Terminal-Tab konnte jedoch nicht geöffnet werden; setze sie mit %s --session-id %s fort", "フォークしたセッション %s は作成されましたが、ターミナルタブを開けませんでした。%s --session-id %s で再開してください", "포크된 세션 %s을(를) 만들었지만 터미널 탭을 열지 못했습니다. %s --session-id %s로 재개하세요", "Ответвлённый сеанс %s создан, но его вкладку терминала открыть не удалось; возобновите его командой %s --session-id %s")
	add(KeyRootForkExecutableResolve, "Could not resolve the current executable", "无法确定当前可执行文件", "Aktuelle ausführbare Datei konnte nicht ermittelt werden", "現在の実行ファイルを特定できませんでした", "현재 실행 파일을 확인하지 못했습니다", "Не удалось определить текущий исполняемый файл")
	add(KeyRootForkProcessLaunch, "Could not launch %s", "无法启动 %s", "%s konnte nicht gestartet werden", "%s を起動できませんでした", "%s을(를) 실행하지 못했습니다", "Не удалось запустить %s")
	add(KeyRootForkOSAScriptFind, "Could not find osascript", "找不到 osascript", "osascript wurde nicht gefunden", "osascript が見つかりませんでした", "osascript를 찾지 못했습니다", "Не удалось найти osascript")
	add(KeyRootForkSupportedTerminalAbsent, "No supported terminal capable of opening a new tab was found", "未找到可打开新标签页的受支持终端", "Kein unterstütztes Terminal zum Öffnen eines neuen Tabs gefunden", "新しいタブを開ける対応ターミナルが見つかりませんでした", "새 탭을 열 수 있는 지원 터미널을 찾지 못했습니다", "Не найден поддерживаемый терминал, способный открыть новую вкладку")
	add(KeyRootForkWindowsTerminalRequired, "Windows Terminal is required to open a new tab", "打开新标签页需要 Windows Terminal", "Zum Öffnen eines neuen Tabs ist Windows Terminal erforderlich", "新しいタブを開くには Windows Terminal が必要です", "새 탭을 열려면 Windows Terminal이 필요합니다", "Для открытия новой вкладки необходим Windows Terminal")
	add(KeyRootForkTerminalUnsupportedOS, "Opening a new terminal tab is unsupported on %s", "%s 不支持打开新的终端标签页", "Das Öffnen eines neuen Terminal-Tabs wird unter %s nicht unterstützt", "%s では新しいターミナルタブを開けません", "%s에서는 새 터미널 탭 열기를 지원하지 않습니다", "Открытие новой вкладки терминала не поддерживается в %s")

	add(KeyRootSessionForkSourceIncomplete, "Source session reference is incomplete", "源会话引用不完整", "Referenz der Quellsitzung ist unvollständig", "元のセッション参照が不完全です", "원본 세션 참조가 완전하지 않습니다", "Ссылка на исходный сеанс неполна")
	add(KeyRootSessionForkSnapshotEmpty, "Fork snapshot is empty", "分叉快照为空", "Fork-Snapshot ist leer", "フォーク用スナップショットが空です", "포크 스냅샷이 비어 있습니다", "Снимок для ветвления пуст")
	add(KeyRootSessionForkSourceMissing, "Source session %q was not found", "找不到源会话 %q", "Quellsitzung %q wurde nicht gefunden", "元のセッション %q が見つかりません", "원본 세션 %q을(를) 찾지 못했습니다", "Исходный сеанс %q не найден")
	add(KeyRootSessionForkMetadataLoad, "Could not load source session metadata", "无法加载源会话元数据", "Metadaten der Quellsitzung konnten nicht geladen werden", "元のセッションのメタデータを読み込めませんでした", "원본 세션 메타데이터를 불러오지 못했습니다", "Не удалось загрузить метаданные исходного сеанса")
	add(KeyRootSessionForkArtifactsCopy, "Could not copy fork artifacts", "无法复制分叉产物", "Fork-Artefakte konnten nicht kopiert werden", "フォーク用の成果物をコピーできませんでした", "포크 산출물을 복사하지 못했습니다", "Не удалось скопировать артефакты ветвления")
	add(KeyRootSessionForkTranscriptSave, "Could not save the fork transcript", "无法保存分叉对话记录", "Fork-Transkript konnte nicht gespeichert werden", "フォークした会話履歴を保存できませんでした", "포크 대화 기록을 저장하지 못했습니다", "Не удалось сохранить журнал ответвления")
	add(KeyRootSessionForkMetadataSave, "Could not save fork metadata", "无法保存分叉元数据", "Fork-Metadaten konnten nicht gespeichert werden", "フォークのメタデータを保存できませんでした", "포크 메타데이터를 저장하지 못했습니다", "Не удалось сохранить метаданные ветвления")
	add(KeyRootSessionForkArtifactNotDir, "Source artifact path is not a directory: %s", "源产物路径不是目录：%s", "Quellpfad der Artefakte ist kein Verzeichnis: %s", "元の成果物パスはディレクトリではありません: %s", "원본 산출물 경로가 디렉터리가 아닙니다: %s", "Путь к исходным артефактам не является каталогом: %s")
	add(KeyRootSessionForkArtifactSymlink, "Referenced artifact symlinks are not supported: %s", "不支持引用符号链接形式的产物：%s", "Referenzierte Artefakt-Symlinks werden nicht unterstützt: %s", "参照された成果物のシンボリックリンクには対応していません: %s", "참조된 산출물 심볼릭 링크는 지원하지 않습니다: %s", "Символические ссылки на артефакты не поддерживаются: %s")
	add(KeyRootSessionForkArtifactUnsupported, "Unsupported artifact type: %s", "不支持的产物类型：%s", "Nicht unterstützter Artefakttyp: %s", "未対応の成果物形式です: %s", "지원하지 않는 산출물 유형: %s", "Неподдерживаемый тип артефакта: %s")

	add(KeyRootSessionHookSettingsLoad, "Could not load hook settings %s", "无法加载 hook 设置 %s", "Hook-Einstellungen %s konnten nicht geladen werden", "hook 設定 %s を読み込めませんでした", "hook 설정 %s을(를) 불러오지 못했습니다", "Не удалось загрузить настройки hook %s")
	add(KeyRootSessionHookDirectoryLoad, "Could not load hook directory %s", "无法加载 hook 目录 %s", "Hook-Verzeichnis %s konnte nicht geladen werden", "hook ディレクトリ %s を読み込めませんでした", "hook 디렉터리 %s을(를) 불러오지 못했습니다", "Не удалось загрузить каталог hook %s")
	add(KeyRootSessionSwitcherIncomplete, "Session switcher is not fully configured", "会话切换器配置不完整", "Sitzungsumschaltung ist nicht vollständig konfiguriert", "セッション切り替えの設定が不完全です", "세션 전환기 구성이 완전하지 않습니다", "Переключатель сеансов настроен не полностью")
	add(KeyRootSessionTargetIDRequired, "A target session ID is required", "必须提供目标会话 ID", "Eine Ziel-Sitzungs-ID ist erforderlich", "切り替え先のセッション ID が必要です", "대상 세션 ID가 필요합니다", "Необходим ID целевого сеанса")
	add(KeyRootSessionCWDUnavailable, "Session working directory is unavailable", "会话工作目录不可用", "Arbeitsverzeichnis der Sitzung ist nicht verfügbar", "セッションの作業ディレクトリを利用できません", "세션 작업 디렉터리를 사용할 수 없습니다", "Рабочий каталог сеанса недоступен")
	add(KeyRootSessionCWDNotDirectory, "Session working directory is not a directory: %s", "会话工作目录不是目录：%s", "Arbeitsverzeichnis der Sitzung ist kein Verzeichnis: %s", "セッションの作業ディレクトリはディレクトリではありません: %s", "세션 작업 디렉터리가 디렉터리가 아닙니다: %s", "Рабочий каталог сеанса не является каталогом: %s")
	add(KeyRootSessionPreparedMismatch, "Prepared registry context does not match target workspace %q", "已准备的 registry 上下文与目标工作区 %q 不匹配", "Vorbereiteter Registry-Kontext stimmt nicht mit dem Zielarbeitsbereich %q überein", "準備済みの registry コンテキストが対象ワークスペース %q と一致しません", "준비된 registry 컨텍스트가 대상 작업 공간 %q과(와) 일치하지 않습니다", "Подготовленный контекст registry не соответствует целевой рабочей области %q")
	add(KeyRootSessionResumeUnsupported, "Engine does not support two-phase session resume", "Engine 不支持两阶段会话恢复", "Engine unterstützt keine zweiphasige Sitzungsfortsetzung", "Engine は 2 段階のセッション再開に対応していません", "Engine이 2단계 세션 재개를 지원하지 않습니다", "Engine не поддерживает двухэтапное возобновление сеанса")
	add(KeyRootSessionTargetDirectoryOpen, "Could not open the target working directory", "无法打开目标工作目录", "Ziel-Arbeitsverzeichnis konnte nicht geöffnet werden", "対象の作業ディレクトリを開けませんでした", "대상 작업 디렉터리를 열지 못했습니다", "Не удалось открыть целевой рабочий каталог")
	add(KeyRootSessionConversationCommit, "Could not commit the target conversation", "无法提交目标对话", "Zielunterhaltung konnte nicht übernommen werden", "対象の会話を確定できませんでした", "대상 대화를 확정하지 못했습니다", "Не удалось зафиксировать целевой диалог")
	add(KeyRootWorkspaceRequired, "A target workspace is required", "必须提供目标工作区", "Ein Zielarbeitsbereich ist erforderlich", "対象のワークスペースが必要です", "대상 작업 공간이 필요합니다", "Необходима целевая рабочая область")
	add(KeyRootMCPSettingsLoad, "Could not load MCP settings %s", "无法加载 MCP 设置 %s", "MCP-Einstellungen %s konnten nicht geladen werden", "MCP 設定 %s を読み込めませんでした", "MCP 설정 %s을(를) 불러오지 못했습니다", "Не удалось загрузить настройки MCP %s")
	add(KeyRootMCPSettingsInvalid, "Invalid MCP settings %s (%s): %s", "MCP 设置无效 %s（%s）：%s", "Ungültige MCP-Einstellungen %s (%s): %s", "MCP 設定 %s（%s）が無効です: %s", "잘못된 MCP 설정 %s(%s): %s", "Недопустимые настройки MCP %s (%s): %s")
	add(KeyRootWorktreeRebindRejected, "This worktree change no longer belongs to the active conversation", "此 worktree 变更已不再属于当前会话", "Diese Worktree-Änderung gehört nicht mehr zur aktiven Unterhaltung", "この worktree の変更は現在の会話には属していません", "이 worktree 변경은 더 이상 현재 대화에 속하지 않습니다", "Это изменение worktree больше не относится к активному диалогу")

	add(KeyRootModelSettingsRead, "Could not read settings %s", "无法读取设置 %s", "Einstellungen %s konnten nicht gelesen werden", "設定 %s を読み込めませんでした", "설정 %s을(를) 읽지 못했습니다", "Не удалось прочитать настройки %s")
	add(KeyRootModelSettingsParse, "Could not parse settings %s", "无法解析设置 %s", "Einstellungen %s konnten nicht geparst werden", "設定 %s を解析できませんでした", "설정 %s을(를) 파싱하지 못했습니다", "Не удалось разобрать настройки %s")
	add(KeyRootModelSettingsHome, "Could not determine the home directory for model settings", "无法确定 model 设置所需的主目录", "Home-Verzeichnis für Modelleinstellungen konnte nicht ermittelt werden", "model 設定用のホームディレクトリを特定できませんでした", "model 설정용 홈 디렉터리를 확인하지 못했습니다", "Не удалось определить домашний каталог для настроек model")
	add(KeyRootModelSettingsDirectory, "Could not create settings directory %s", "无法创建设置目录 %s", "Einstellungsverzeichnis %s konnte nicht erstellt werden", "設定ディレクトリ %s を作成できませんでした", "설정 디렉터리 %s을(를) 만들지 못했습니다", "Не удалось создать каталог настроек %s")
	add(KeyRootModelSettingsMarshal, "Could not encode model settings", "无法编码 model 设置", "Modelleinstellungen konnten nicht kodiert werden", "model 設定をエンコードできませんでした", "model 설정을 인코딩하지 못했습니다", "Не удалось закодировать настройки model")
	add(KeyRootModelSettingsTempCreate, "Could not create the temporary settings file", "无法创建临时设置文件", "Temporäre Einstellungsdatei konnte nicht erstellt werden", "一時設定ファイルを作成できませんでした", "임시 설정 파일을 만들지 못했습니다", "Не удалось создать временный файл настроек")
	add(KeyRootModelSettingsTempWrite, "Could not write the temporary settings file", "无法写入临时设置文件", "Temporäre Einstellungsdatei konnte nicht geschrieben werden", "一時設定ファイルに書き込めませんでした", "임시 설정 파일에 쓰지 못했습니다", "Не удалось записать временный файл настроек")
	add(KeyRootModelSettingsTempChmod, "Could not secure the temporary settings file", "无法设置临时设置文件的安全权限", "Temporäre Einstellungsdatei konnte nicht abgesichert werden", "一時設定ファイルの権限を安全に設定できませんでした", "임시 설정 파일의 보안 권한을 설정하지 못했습니다", "Не удалось защитить временный файл настроек")
	add(KeyRootModelSettingsTempClose, "Could not close the temporary settings file", "无法关闭临时设置文件", "Temporäre Einstellungsdatei konnte nicht geschlossen werden", "一時設定ファイルを閉じられませんでした", "임시 설정 파일을 닫지 못했습니다", "Не удалось закрыть временный файл настроек")
	add(KeyRootModelSettingsReplace, "Could not replace settings %s", "无法替换设置 %s", "Einstellungen %s konnten nicht ersetzt werden", "設定 %s を置き換えられませんでした", "설정 %s을(를) 교체하지 못했습니다", "Не удалось заменить настройки %s")
	add(KeyRootPlanModeRestore, "Could not restore the active Plan mode permission settings", "无法恢复当前 Plan 模式的权限设置", "Berechtigungseinstellungen des aktiven Plan-Modus konnten nicht wiederhergestellt werden", "有効な Plan モードの権限設定を復元できませんでした", "활성 Plan 모드 권한 설정을 복원하지 못했습니다", "Не удалось восстановить разрешения активного режима Plan")
	add(KeyRootPlanModeUnsupported, "Unsupported permission mode %q", "不支持的权限模式 %q", "Nicht unterstützter Berechtigungsmodus %q", "未対応の権限モード %q", "지원하지 않는 권한 모드 %q", "Неподдерживаемый режим разрешений %q")
	add(KeyRootImageRead, "Could not read image %q", "无法读取图像 %q", "Bild %q konnte nicht gelesen werden", "画像 %q を読み込めませんでした", "이미지 %q을(를) 읽지 못했습니다", "Не удалось прочитать изображение %q")

	add(KeyRootLogCronExecutionFailed, "[cron] WARNING: Job %s fired but execution failed: %v", "[cron] 警告：任务 %s 已触发，但执行失败：%v", "[cron] WARNUNG: Job %s wurde ausgelöst, die Ausführung ist jedoch fehlgeschlagen: %v", "[cron] 警告: ジョブ %s は起動しましたが、実行に失敗しました: %v", "[cron] 경고: 작업 %s이(가) 트리거되었지만 실행에 실패했습니다: %v", "[cron] ПРЕДУПРЕЖДЕНИЕ: задача %s запущена, но выполнить её не удалось: %v")
	add(KeyRootLogWorktreeHooksFailed, "[worktree] WARNING: Could not load worktree hooks: %v", "[worktree] 警告：无法加载 worktree hook：%v", "[worktree] WARNUNG: Worktree-Hooks konnten nicht geladen werden: %v", "[worktree] 警告: worktree hook を読み込めませんでした: %v", "[worktree] 경고: worktree hook을 불러오지 못했습니다: %v", "[worktree] ПРЕДУПРЕЖДЕНИЕ: не удалось загрузить hook рабочего дерева: %v")
	add(KeyRootLogMCPRefreshFailed, "[mcp] WARNING: Dynamic MCP tool refresh failed: %v", "[mcp] 警告：动态 MCP 工具刷新失败：%v", "[mcp] WARNUNG: Dynamische MCP-Tools konnten nicht aktualisiert werden: %v", "[mcp] 警告: 動的 MCP ツールの更新に失敗しました: %v", "[mcp] 경고: 동적 MCP 도구 새로고침에 실패했습니다: %v", "[mcp] ПРЕДУПРЕЖДЕНИЕ: не удалось обновить динамические инструменты MCP: %v")
	add(KeyRootLogSessionCorrupt, "WARNING: Corrupt session entry; returning partial history: %v", "警告：会话条目已损坏；将返回部分历史记录：%v", "WARNUNG: Beschädigter Sitzungseintrag; unvollständiger Verlauf wird zurückgegeben: %v", "警告: セッションのエントリが壊れています。読み込めた履歴のみ返します: %v", "경고: 세션 항목이 손상되어 일부 기록만 반환합니다: %v", "ПРЕДУПРЕЖДЕНИЕ: повреждена запись сеанса; возвращается частичная история: %v")

	add(KeyRootAgentEvidenceTranscript, "%s transcript", "%s 的对话记录", "%s-Transkript", "%s の記録", "%s 대화 기록", "Журнал %s")
	add(KeyRootAgentPhaseStart, "starting", "正在启动", "wird gestartet", "開始中", "시작 중", "запускается")
	add(KeyRootAgentPhaseMCPReady, "MCP ready", "MCP 已就绪", "MCP bereit", "MCP 準備完了", "MCP 준비됨", "MCP готов")
	add(KeyRootAgentPhaseToolUse, "using a tool", "正在使用工具", "verwendet ein Tool", "ツールを使用中", "도구 사용 중", "использует инструмент")
	add(KeyRootAgentPhaseAssistant, "responding", "正在生成回复", "antwortet", "応答中", "응답 중", "формирует ответ")
	add(KeyRootAgentPhaseError, "error", "出错", "Fehler", "エラー", "오류", "ошибка")
	add(KeyRootAgentPhaseAborted, "aborted", "已中止", "abgebrochen", "中止済み", "중단됨", "прерван")
	add(KeyRootAgentPhaseBackground, "running in background", "正在后台运行", "läuft im Hintergrund", "バックグラウンドで実行中", "백그라운드에서 실행 중", "выполняется в фоне")
	add(KeyRootAgentPhaseRemoteLaunched, "launched remotely", "已远程启动", "remote gestartet", "リモートで起動済み", "원격으로 실행됨", "запущен удалённо")
	add(KeyRootAgentQueueActiveRun, "waiting for the active run", "正在等待当前运行完成", "wartet auf den aktiven Lauf", "実行中の処理を待機中", "활성 실행이 끝나기를 기다리는 중", "ожидает завершения активного запуска")
	add(KeyRootAgentQueueWorkerCapacity, "waiting for Agent worker capacity", "正在等待 Agent worker 空闲", "wartet auf freie Agent-Worker-Kapazität", "Agent worker の空きを待機中", "Agent worker 여유 용량을 기다리는 중", "ожидает свободного Agent worker")
	add(KeyRootAgentReasonCompleted, "completed", "已完成", "abgeschlossen", "完了", "완료됨", "завершено")
	add(KeyRootAgentReasonMaxTurns, "maximum turns reached", "已达到最大轮次", "maximale Rundenzahl erreicht", "最大ターン数に到達", "최대 턴 수에 도달함", "достигнуто максимальное число ходов")
	add(KeyRootAgentReasonRuntimeInterrupt, "runtime interrupted", "运行时已中断", "Laufzeit unterbrochen", "ランタイムが中断", "런타임이 중단됨", "среда выполнения прервана")
	add(KeyRootAgentReasonDeadline, "deadline exceeded", "已超过截止时间", "Zeitlimit überschritten", "期限を超過", "기한 초과", "превышен срок выполнения")
	add(KeyRootAgentReasonContextCancel, "context cancelled", "上下文已取消", "Kontext abgebrochen", "コンテキストがキャンセル済み", "컨텍스트가 취소됨", "контекст отменён")
	add(KeyRootAgentReasonPartialError, "error after a partial result", "返回部分结果后出错", "Fehler nach einem Teilergebnis", "部分的な結果の生成後にエラー", "일부 결과 생성 후 오류 발생", "ошибка после частичного результата")
	add(KeyRootAgentReasonError, "error", "出错", "Fehler", "エラー", "오류", "ошибка")
	add(KeyRootAgentReasonOutputOpen, "could not open Agent output", "无法打开 Agent 输出", "Agent-Ausgabe konnte nicht geöffnet werden", "Agent の出力を開けませんでした", "Agent 출력을 열지 못했습니다", "не удалось открыть вывод Agent")
	add(KeyRootAgentReasonKilled, "terminated", "已终止", "beendet", "強制終了", "종료됨", "остановлен")
	add(KeyRootAgentReasonProcessRestart, "interrupted by process restart", "因进程重启而中断", "durch Prozessneustart unterbrochen", "プロセスの再起動により中断", "프로세스 재시작으로 중단됨", "прервано перезапуском процесса")
}

// RootGoalStatusLabel localizes known first-party status identifiers while
// preserving unknown extension identifiers verbatim.
func RootGoalStatusLabel(lang Language, status string) string {
	var key Key
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "active":
		key = KeyRootGoalStatusActive
	case "paused":
		key = KeyRootGoalStatusPaused
	case "achieved":
		key = KeyRootGoalStatusAchieved
	case "blocked":
		key = KeyRootGoalStatusBlocked
	case "cleared":
		key = KeyRootGoalStatusCleared
	default:
		return strings.TrimSpace(status)
	}
	return Text(lang, key)
}

func RootGoalActionLabel(lang Language, action string) string {
	var key Key
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "edit":
		key = KeyRootGoalActionEdit
	case "pause":
		key = KeyRootGoalActionPause
	case "resume":
		key = KeyRootGoalActionResume
	case "clear":
		key = KeyRootGoalActionClear
	default:
		return strings.TrimSpace(action)
	}
	return Text(lang, key)
}

// RootGoalEvaluatorReasonLabel re-renders first-party persisted evaluator
// failures in the active language. Evaluator-authored reasons remain raw.
func RootGoalEvaluatorReasonLabel(lang Language, reason string) string {
	reason = strings.TrimSpace(reason)
	for _, sourceLang := range AllLanguages() {
		if reason == Text(sourceLang, KeyRootGoalReasonEvaluatorUnavailable) {
			return Text(lang, KeyRootGoalReasonEvaluatorUnavailable)
		}
		const marker = "__ROOT_GOAL_REASON_DETAIL__"
		formatted := Format(sourceLang, KeyRootGoalReasonEvaluatorFailed, marker)
		prefix, suffix, found := strings.Cut(formatted, marker)
		if !found || !strings.HasPrefix(reason, prefix) || !strings.HasSuffix(reason, suffix) {
			continue
		}
		detail := strings.TrimSuffix(strings.TrimPrefix(reason, prefix), suffix)
		return Format(lang, KeyRootGoalReasonEvaluatorFailed, detail)
	}
	return reason
}

// RootGoalEvaluatorReasonStateLabel renders structured first-party persisted
// reasons in the active language. Unknown kinds and evaluator-authored text
// remain raw; RootGoalEvaluatorReasonLabel only supports legacy sessions.
func RootGoalEvaluatorReasonStateLabel(lang Language, reason, kind, semanticKey, detail string) string {
	switch strings.TrimSpace(kind) {
	case "evaluator_unavailable":
		return Text(lang, KeyRootGoalReasonEvaluatorUnavailable)
	case "evaluator_failed":
		inner := persistedGoalEvaluatorErrorLabel(lang, Key(strings.TrimSpace(semanticKey)), strings.TrimSpace(detail))
		if inner == "" {
			inner = strings.TrimSpace(detail)
		}
		if inner == "" {
			inner = Text(lang, KeyPresentationReasonUnavailable)
		}
		return Format(lang, KeyRootGoalReasonEvaluatorFailed, inner)
	case "model_marked_complete":
		return Text(lang, KeyToolGoalReasonComplete)
	case "model_marked_blocked":
		return Text(lang, KeyToolGoalReasonBlocked)
	default:
		return RootGoalEvaluatorReasonLabel(lang, reason)
	}
}

func persistedGoalEvaluatorErrorLabel(lang Language, key Key, detail string) string {
	switch key {
	case KeyLoopGoalEvaluatorProviderCallFailed, KeyLoopGoalEvaluatorStreamError,
		KeyLoopGoalEvaluatorParseFailed, KeyLoopGoalEvaluatorTrailingParseFailed:
		if detail == "" {
			return ""
		}
		return Format(lang, key, detail)
	case KeyLoopGoalEvaluatorReasonTooLong:
		return Format(lang, key, 512)
	case KeyLoopGoalEvaluatorProviderUnavailable, KeyLoopGoalEvaluatorNilStream,
		KeyLoopGoalEvaluatorStreamEnded, KeyLoopGoalEvaluatorOutputLimit,
		KeyLoopGoalEvaluatorAttemptedTool, KeyLoopGoalEvaluatorStreamFailed,
		KeyLoopGoalEvaluatorMarshalFailed, KeyLoopGoalEvaluatorEmptyResponse,
		KeyLoopGoalEvaluatorMissingMet, KeyLoopGoalEvaluatorMissingReason,
		KeyLoopGoalEvaluatorMultipleJSON:
		return Text(lang, key)
	default:
		return ""
	}
}

// RootAgentPhaseLabel localizes canonical runtime phases. Unknown values are
// extension identifiers and therefore remain unchanged.
func RootAgentPhaseLabel(lang Language, phase string) string {
	normalized := strings.ToLower(strings.TrimSpace(phase))
	var key Key
	switch normalized {
	case "start":
		key = KeyRootAgentPhaseStart
	case "mcp_ready":
		key = KeyRootAgentPhaseMCPReady
	case "tool_use":
		key = KeyRootAgentPhaseToolUse
	case "assistant":
		key = KeyRootAgentPhaseAssistant
	case "error":
		key = KeyRootAgentPhaseError
	case "aborted":
		key = KeyRootAgentPhaseAborted
	case "background":
		key = KeyRootAgentPhaseBackground
	case "remote_launched":
		key = KeyRootAgentPhaseRemoteLaunched
	default:
		return strings.TrimSpace(phase)
	}
	return Text(lang, key)
}

func RootAgentQueueReasonLabel(lang Language, reason string) string {
	normalized := strings.ToLower(strings.TrimSpace(reason))
	switch normalized {
	case "dependency:active_run":
		return Text(lang, KeyRootAgentQueueActiveRun)
	case "capacity:agent_session_worker":
		return Text(lang, KeyRootAgentQueueWorkerCapacity)
	default:
		return strings.TrimSpace(reason)
	}
}

func RootAgentTerminalReasonLabel(lang Language, reason string) string {
	normalized := strings.ToLower(strings.TrimSpace(reason))
	var key Key
	switch normalized {
	case "completed":
		key = KeyRootAgentReasonCompleted
	case "max_turns":
		key = KeyRootAgentReasonMaxTurns
	case "runtime_interrupted":
		key = KeyRootAgentReasonRuntimeInterrupt
	case "deadline_exceeded":
		key = KeyRootAgentReasonDeadline
	case "context_cancelled":
		key = KeyRootAgentReasonContextCancel
	case "error_after_partial_result":
		key = KeyRootAgentReasonPartialError
	case "error":
		key = KeyRootAgentReasonError
	case "output_open_failed":
		key = KeyRootAgentReasonOutputOpen
	case "killed":
		key = KeyRootAgentReasonKilled
	case "process_restart":
		key = KeyRootAgentReasonProcessRestart
	default:
		return strings.TrimSpace(reason)
	}
	return Text(lang, key)
}
