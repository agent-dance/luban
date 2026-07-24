package i18n

const (
	KeyToolIndirectWorktreeKillTmux          Key = "tool.indirect.worktree.kill_tmux"
	KeyToolIndirectWorktreeWaitGitLocks      Key = "tool.indirect.worktree.wait_git_locks"
	KeyToolIndirectWorktreeDeleteBranch      Key = "tool.indirect.worktree.delete_branch"
	KeyToolIndirectWorktreeRemoveHookMissing Key = "tool.indirect.worktree.remove_hook_missing"
	KeyToolIndirectWorktreeRemoveHookFailed  Key = "tool.indirect.worktree.remove_hook_failed"

	KeyToolIndirectPlanApprovalLeadOnly        Key = "tool.indirect.plan_approval.lead_only"
	KeyToolIndirectPlanApprovalCommit          Key = "tool.indirect.plan_approval.commit"
	KeyToolIndirectPlanApprovalModeRequired    Key = "tool.indirect.plan_approval.mode_required"
	KeyToolIndirectPlanApprovalPlanRequired    Key = "tool.indirect.plan_approval.plan_required"
	KeyToolIndirectPlanApprovalPrepareDir      Key = "tool.indirect.plan_approval.prepare_dir"
	KeyToolIndirectPlanApprovalPersist         Key = "tool.indirect.plan_approval.persist"
	KeyToolIndirectPlanApprovalTeamRequired    Key = "tool.indirect.plan_approval.team_required"
	KeyToolIndirectPlanApprovalEncodeRequest   Key = "tool.indirect.plan_approval.encode_request"
	KeyToolIndirectPlanStateRequired           Key = "tool.indirect.plan_state.required"
	KeyToolIndirectPlanStateNotActive          Key = "tool.indirect.plan_state.not_active"
	KeyToolIndirectPlanStateRestoreMode        Key = "tool.indirect.plan_state.restore_mode"
	KeyToolIndirectPlanStateChangedDuringExit  Key = "tool.indirect.plan_state.changed_during_exit"
	KeyToolIndirectPlanStatePersistExitedState Key = "tool.indirect.plan_state.persist_exited_state"

	KeyToolIndirectBashModeDeprecated       Key = "tool.indirect.bash_mode.deprecated"
	KeyToolIndirectBashModeUnknown          Key = "tool.indirect.bash_mode.unknown"
	KeyToolIndirectBashModeNonReadForbidden Key = "tool.indirect.bash_mode.non_read_forbidden"
	KeyToolIndirectBashModeDestructive      Key = "tool.indirect.bash_mode.destructive_forbidden"
	KeyToolIndirectBashModePattern          Key = "tool.indirect.bash_mode.destructive_pattern"
)

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}

	add(KeyToolIndirectWorktreeKillTmux,
		"kill tmux session %q: %v",
		"终止 tmux session %q 失败：%v",
		"tmux-Session %q konnte nicht beendet werden: %v",
		"tmux session %q を終了できませんでした: %v",
		"tmux session %q을(를) 종료하지 못했습니다: %v",
		"Не удалось завершить tmux session %q: %v")
	add(KeyToolIndirectWorktreeWaitGitLocks,
		"wait for git worktree locks: %v",
		"等待 git worktree lock 释放失败：%v",
		"Warten auf git-worktree-Sperren fehlgeschlagen: %v",
		"git worktree lock の解除待機に失敗しました: %v",
		"git worktree lock 해제를 기다리지 못했습니다: %v",
		"Не удалось дождаться снятия блокировок git worktree: %v")
	add(KeyToolIndirectWorktreeDeleteBranch,
		"delete worktree branch %q: %s",
		"删除 worktree branch %q 失败：%s",
		"worktree-Branch %q konnte nicht gelöscht werden: %s",
		"worktree branch %q を削除できませんでした: %s",
		"worktree branch %q을(를) 삭제하지 못했습니다: %s",
		"Не удалось удалить worktree branch %q: %s")
	add(KeyToolIndirectWorktreeRemoveHookMissing,
		"no WorktreeRemove hook configured; hook-based worktree left at %s",
		"未配置 WorktreeRemove hook；基于 hook 创建的 worktree 保留在 %s",
		"Kein WorktreeRemove-Hook konfiguriert; der Hook-basierte Worktree verbleibt unter %s",
		"WorktreeRemove hook が設定されていないため、hook ベースの worktree は %s に残されています",
		"WorktreeRemove hook이 구성되지 않아 hook 기반 worktree가 %s에 남아 있습니다",
		"Hook WorktreeRemove не настроен; созданный через hook worktree оставлен в %s")
	add(KeyToolIndirectWorktreeRemoveHookFailed,
		"WorktreeRemove hook failed for %s: %v",
		"对 %s 执行 WorktreeRemove hook 失败：%v",
		"WorktreeRemove-Hook für %s fehlgeschlagen: %v",
		"%s の WorktreeRemove hook に失敗しました: %v",
		"%s의 WorktreeRemove hook 실행에 실패했습니다: %v",
		"Ошибка hook WorktreeRemove для %s: %v")

	add(KeyToolIndirectPlanApprovalLeadOnly,
		"only %s can resolve plan approval requests",
		"只有 %s 可以处理 plan 审批请求",
		"Nur %s kann Plan-Genehmigungsanfragen bearbeiten",
		"plan の承認リクエストを処理できるのは %s だけです",
		"plan 승인 요청은 %s만 처리할 수 있습니다",
		"Только %s может обрабатывать запросы на утверждение плана")
	add(KeyToolIndirectPlanApprovalCommit,
		"commit approved teammate plan exit: %v",
		"提交已批准的 teammate plan mode 退出操作失败：%v",
		"Genehmigten Plan-Modus-Ausstieg des Teammitglieds übernehmen: %v",
		"承認済みの teammate の plan mode 終了を確定できませんでした: %v",
		"승인된 teammate의 plan mode 종료를 확정하지 못했습니다: %v",
		"Не удалось зафиксировать одобренный выход участника команды из режима плана: %v")
	add(KeyToolIndirectPlanApprovalModeRequired,
		"teammate ExitPlanMode is not in required plan mode",
		"teammate 的 ExitPlanMode 未处于所需的 plan mode",
		"ExitPlanMode des Teammitglieds befindet sich nicht im erforderlichen Planmodus",
		"teammate の ExitPlanMode は必要な plan mode になっていません",
		"teammate의 ExitPlanMode가 필요한 plan mode에 있지 않습니다",
		"ExitPlanMode участника команды не находится в требуемом режиме плана")
	add(KeyToolIndirectPlanApprovalPlanRequired,
		"no plan file found at %s; write a non-empty plan before calling ExitPlanMode",
		"在 %s 未找到 plan 文件；请先写入非空 plan，再调用 ExitPlanMode",
		"Unter %s wurde keine Plandatei gefunden; schreibe vor dem Aufruf von ExitPlanMode einen nicht leeren Plan",
		"%s に plan ファイルがありません。空でない plan を書いてから ExitPlanMode を呼び出してください",
		"%s에서 plan 파일을 찾을 수 없습니다. 비어 있지 않은 plan을 작성한 후 ExitPlanMode를 호출하세요",
		"Файл плана не найден по пути %s; перед вызовом ExitPlanMode запишите непустой план")
	add(KeyToolIndirectPlanApprovalPrepareDir,
		"prepare teammate plan directory: %v",
		"准备 teammate plan 目录失败：%v",
		"Planverzeichnis des Teammitglieds konnte nicht vorbereitet werden: %v",
		"teammate の plan ディレクトリを準備できませんでした: %v",
		"teammate의 plan 디렉터리를 준비하지 못했습니다: %v",
		"Не удалось подготовить каталог плана участника команды: %v")
	add(KeyToolIndirectPlanApprovalPersist,
		"persist teammate plan: %v",
		"持久化 teammate plan 失败：%v",
		"Plan des Teammitglieds konnte nicht gespeichert werden: %v",
		"teammate の plan を保存できませんでした: %v",
		"teammate의 plan을 저장하지 못했습니다: %v",
		"Не удалось сохранить план участника команды: %v")
	add(KeyToolIndirectPlanApprovalTeamRequired,
		"teammate plan approval requires a team name",
		"teammate plan 审批需要 team 名称",
		"Für die Plangenehmigung eines Teammitglieds ist ein Teamname erforderlich",
		"teammate の plan 承認には team 名が必要です",
		"teammate의 plan 승인에는 team 이름이 필요합니다",
		"Для утверждения плана участника команды требуется имя команды")
	add(KeyToolIndirectPlanApprovalEncodeRequest,
		"encode plan approval request",
		"无法编码 plan 审批请求",
		"Plangenehmigungsanfrage konnte nicht codiert werden",
		"plan の承認リクエストをエンコードできませんでした",
		"plan 승인 요청을 인코딩하지 못했습니다",
		"Не удалось закодировать запрос на утверждение плана")
	add(KeyToolIndirectPlanStateRequired,
		"plan state is required",
		"必须提供 plan state",
		"Der Planstatus ist erforderlich",
		"plan state が必要です",
		"plan state가 필요합니다",
		"Требуется состояние плана")
	add(KeyToolIndirectPlanStateNotActive,
		"not in plan mode",
		"当前不在 plan mode",
		"Nicht im Planmodus",
		"plan mode ではありません",
		"plan mode가 아닙니다",
		"Сейчас не активен режим плана")
	add(KeyToolIndirectPlanStateRestoreMode,
		"restore pre-plan permission mode %q: %v",
		"恢复 plan 前的 permission mode %q 失败：%v",
		"Berechtigungsmodus %q vor dem Plan konnte nicht wiederhergestellt werden: %v",
		"plan 前の permission mode %q を復元できませんでした: %v",
		"plan 이전 permission mode %q을(를) 복원하지 못했습니다: %v",
		"Не удалось восстановить режим разрешений %q, действовавший до плана: %v")
	add(KeyToolIndirectPlanStateChangedDuringExit,
		"plan state changed during exit transaction",
		"退出事务期间 plan state 已发生变化",
		"Der Planstatus hat sich während der Ausstiegstransaktion geändert",
		"終了トランザクション中に plan state が変更されました",
		"종료 트랜잭션 중 plan state가 변경되었습니다",
		"Состояние плана изменилось во время транзакции выхода")
	add(KeyToolIndirectPlanStatePersistExitedState,
		"persist exited plan state: %v",
		"持久化已退出的 plan state 失败：%v",
		"Beendeten Planstatus speichern: %v",
		"終了済みの plan state を保存できませんでした: %v",
		"종료된 plan state를 저장하지 못했습니다: %v",
		"Не удалось сохранить состояние после выхода из режима плана: %v")

	add(KeyToolIndirectBashModeDeprecated,
		"bash execution mode %q is deprecated and was renamed to %q; please update your config",
		"bash execution mode %q 已弃用并更名为 %q；请更新 config",
		"Der Bash-Ausführungsmodus %q ist veraltet und wurde in %q umbenannt; aktualisiere deine Konfiguration",
		"bash execution mode %q は非推奨となり、%q に名前が変更されました。config を更新してください",
		"bash execution mode %q은(는) 더 이상 사용되지 않으며 %q(으)로 이름이 변경되었습니다. config를 업데이트하세요",
		"Режим выполнения bash %q устарел и переименован в %q; обновите конфигурацию")
	add(KeyToolIndirectBashModeUnknown,
		"unknown bash execution mode %q",
		"未知的 bash execution mode %q",
		"Unbekannter Bash-Ausführungsmodus %q",
		"不明な bash execution mode %q",
		"알 수 없는 bash execution mode %q",
		"Неизвестный режим выполнения bash %q")
	add(KeyToolIndirectBashModeNonReadForbidden,
		"acceptEdits mode forbids non-read command (semantic=%s)",
		"acceptEdits mode 禁止非只读命令（semantic=%s）",
		"Der acceptEdits-Modus verbietet nicht schreibgeschützte Befehle (semantic=%s)",
		"acceptEdits mode では読み取り専用でないコマンドは禁止されています（semantic=%s）",
		"acceptEdits mode에서는 읽기 전용이 아닌 명령을 사용할 수 없습니다(semantic=%s)",
		"Режим acceptEdits запрещает команды, не являющиеся только читающими (semantic=%s)")
	add(KeyToolIndirectBashModeDestructive,
		"dontAsk mode forbids destructive command",
		"dontAsk mode 禁止破坏性命令",
		"Der dontAsk-Modus verbietet destruktive Befehle",
		"dontAsk mode では破壊的なコマンドは禁止されています",
		"dontAsk mode에서는 파괴적인 명령을 사용할 수 없습니다",
		"Режим dontAsk запрещает разрушительные команды")
	add(KeyToolIndirectBashModePattern,
		"dontAsk mode flagged destructive pattern: %s",
		"dontAsk mode 检测到破坏性模式：%s",
		"Der dontAsk-Modus hat ein destruktives Muster erkannt: %s",
		"dontAsk mode が破壊的なパターンを検出しました: %s",
		"dontAsk mode가 파괴적인 패턴을 감지했습니다: %s",
		"Режим dontAsk обнаружил разрушительный шаблон: %s")
}
