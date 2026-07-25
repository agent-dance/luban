package i18n

const (
	KeyToolIndirectWorktreeKillTmux          Key = "tool.indirect.worktree.kill_tmux"
	KeyToolIndirectWorktreeWaitGitLocks      Key = "tool.indirect.worktree.wait_git_locks"
	KeyToolIndirectWorktreeDeleteBranch      Key = "tool.indirect.worktree.delete_branch"
	KeyToolIndirectWorktreeRemoveHookMissing Key = "tool.indirect.worktree.remove_hook_missing"
	KeyToolIndirectWorktreeRemoveHookFailed  Key = "tool.indirect.worktree.remove_hook_failed"

	KeyToolIndirectPlanStateRequired            Key = "tool.indirect.plan_state.required"
	KeyToolIndirectPlanStateProjectRootRequired Key = "tool.indirect.plan_state.project_root_required"
	KeyToolIndirectPlanStateResolveProjectRoot  Key = "tool.indirect.plan_state.resolve_project_root"
	KeyToolIndirectPlanStateLoad                Key = "tool.indirect.plan_state.load"
	KeyToolIndirectPlanStateDecode              Key = "tool.indirect.plan_state.decode"
	KeyToolIndirectPlanStateNotActive           Key = "tool.indirect.plan_state.not_active"
	KeyToolIndirectPlanStateRestoreMode         Key = "tool.indirect.plan_state.restore_mode"
	KeyToolIndirectPlanStateChangedDuringExit   Key = "tool.indirect.plan_state.changed_during_exit"
	KeyToolIndirectPlanStatePersistExitedState  Key = "tool.indirect.plan_state.persist_exited_state"
	KeyToolIndirectPlanStateSchemaVersion       Key = "tool.indirect.plan_state.schema_version"
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

	add(KeyToolIndirectPlanStateRequired,
		"plan state is required",
		"必须提供 plan state",
		"Der Planstatus ist erforderlich",
		"plan state が必要です",
		"plan state가 필요합니다",
		"Требуется состояние плана")
	add(KeyToolIndirectPlanStateProjectRootRequired,
		"plan state project root is required",
		"必须提供 plan state 项目根目录",
		"Das Projektstammverzeichnis für den Planstatus ist erforderlich",
		"plan state のプロジェクトルートが必要です",
		"plan state 프로젝트 루트가 필요합니다",
		"Требуется корневой каталог проекта для состояния плана")
	add(KeyToolIndirectPlanStateResolveProjectRoot,
		"resolve plan state project root %q: %v",
		"解析 plan state 项目根目录 %q 失败：%v",
		"Projektstammverzeichnis %q für den Planstatus konnte nicht aufgelöst werden: %v",
		"plan state のプロジェクトルート %q を解決できませんでした: %v",
		"plan state 프로젝트 루트 %q을(를) 확인하지 못했습니다: %v",
		"Не удалось определить корневой каталог проекта %q для состояния плана: %v")
	add(KeyToolIndirectPlanStateLoad,
		"load plan state %q: %v",
		"加载 plan state %q 失败：%v",
		"Planstatus %q konnte nicht geladen werden: %v",
		"plan state %q を読み込めませんでした: %v",
		"plan state %q을(를) 불러오지 못했습니다: %v",
		"Не удалось загрузить состояние плана %q: %v")
	add(KeyToolIndirectPlanStateDecode,
		"decode plan state %q: %v",
		"解析 plan state %q 失败：%v",
		"Planstatus %q konnte nicht decodiert werden: %v",
		"plan state %q をデコードできませんでした: %v",
		"plan state %q을(를) 디코딩하지 못했습니다: %v",
		"Не удалось декодировать состояние плана %q: %v")
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
	add(KeyToolIndirectPlanStateSchemaVersion,
		"Unsupported plan-state schema version %d; expected %d",
		"不支持的计划状态架构版本 %d；应为 %d",
		"Nicht unterstützte Planschema-Version %d; erwartet wurde %d",
		"未対応のプラン状態スキーマバージョン %d です。%d が必要です",
		"지원하지 않는 계획 상태 스키마 버전 %d입니다. 예상 버전은 %d입니다",
		"Неподдерживаемая версия схемы состояния плана %d; ожидалась %d")
}
