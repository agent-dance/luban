package i18n

// Semantic errors returned by lower-level tool validators and runtimes whose
// callers surface the error through ToolResult, task status, or session UI.
// Identifiers, paths, option values, hook IDs, exit codes, and raw subprocess
// output remain format arguments and are intentionally not translated.
const (
	KeyToolGrepUnknownTypeSuggestion       Key = "tool.grep.unknown_type.suggestion"
	KeyToolGrepUnknownTypeHint             Key = "tool.grep.unknown_type.hint"
	KeyToolAgentDefinitionUnknownTool      Key = "tool.agent.definition.unknown_tool"
	KeyToolPermissionModeRequired          Key = "tool.runtime.permission_mode.required"
	KeyToolWorktreeBaseRefInvalid          Key = "tool.worktree.base_ref.invalid"
	KeyToolWorktreeSparsePathInvalid       Key = "tool.worktree.sparse.path_invalid"
	KeyToolWorktreeSparseConfigureFailed   Key = "tool.worktree.sparse.configure_failed"
	KeyToolWorktreeSparseCheckoutFailed    Key = "tool.worktree.sparse.checkout_failed"
	KeyToolFileLockTimedOut                Key = "tool.file.lock.timed_out"
	KeyToolPDFCreateSwiftRendererScript    Key = "tool.pdf.swift_renderer.create_script"
	KeyToolPDFWriteSwiftRendererScript     Key = "tool.pdf.swift_renderer.write_script"
	KeyToolNotificationManagerShuttingDown Key = "tool.notification.manager_shutting_down"
	KeyToolNotificationHookUnavailable     Key = "tool.notification.hook_unavailable"
	KeyToolNotificationHookFailed          Key = "tool.notification.hook_failed"
)

var toolValidationResidualKeys = [...]Key{
	KeyToolGrepUnknownTypeSuggestion,
	KeyToolGrepUnknownTypeHint,
	KeyToolAgentDefinitionUnknownTool,
	KeyToolPermissionModeRequired,
	KeyToolWorktreeBaseRefInvalid,
	KeyToolWorktreeSparsePathInvalid,
	KeyToolWorktreeSparseConfigureFailed,
	KeyToolWorktreeSparseCheckoutFailed,
	KeyToolFileLockTimedOut,
	KeyToolPDFCreateSwiftRendererScript,
	KeyToolPDFWriteSwiftRendererScript,
	KeyToolNotificationManagerShuttingDown,
	KeyToolNotificationHookUnavailable,
	KeyToolNotificationHookFailed,
}

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de,
			LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}

	add(KeyToolGrepUnknownTypeSuggestion,
		"Unknown rg type %q. Did you mean %s?",
		"未知的 rg 类型 %q。你是不是想输入 %s？",
		"Unbekannter rg-Typ %q. Meintest du %s?",
		"rg の種類 %q は不明です。%s のことですか？",
		"알 수 없는 rg 유형 %q입니다. %s을(를) 의미했나요?",
		"Неизвестный тип rg %q. Возможно, имелось в виду %s?")
	add(KeyToolGrepUnknownTypeHint,
		"Unknown rg type %q. Run `rg --type-list` to see valid types.",
		"未知的 rg 类型 %q。运行 `rg --type-list` 可查看有效类型。",
		"Unbekannter rg-Typ %q. Gültige Typen zeigt `rg --type-list` an.",
		"rg の種類 %q は不明です。有効な種類は `rg --type-list` で確認できます。",
		"알 수 없는 rg 유형 %q입니다. `rg --type-list`를 실행해 유효한 유형을 확인하세요.",
		"Неизвестный тип rg %q. Допустимые типы можно посмотреть командой `rg --type-list`.")
	add(KeyToolAgentDefinitionUnknownTool,
		"Agent error: agent %q allows unknown tool %q",
		"Agent 错误：Agent %q 允许使用未知工具 %q",
		"Agent-Fehler: Agent %q erlaubt das unbekannte Tool %q",
		"Agent エラー: Agent %q で不明な Tool %q が許可されています",
		"Agent 오류: Agent %q에 알 수 없는 Tool %q이(가) 허용되어 있습니다",
		"Ошибка Agent: для Agent %q разрешён неизвестный Tool %q")
	add(KeyToolPermissionModeRequired,
		"Permission mode is required",
		"必须指定 permission 模式",
		"Ein Berechtigungsmodus ist erforderlich",
		"permission モードを指定してください",
		"permission 모드를 지정해야 합니다",
		"Необходимо указать режим разрешений")
	add(KeyToolWorktreeBaseRefInvalid,
		"invalid worktree.baseRef %q (expected %q or %q)",
		"worktree.baseRef %q 无效（应为 %q 或 %q）",
		"Ungültiger Wert %q für worktree.baseRef (erwartet: %q oder %q)",
		"worktree.baseRef %q は無効です（%q または %q を指定してください）",
		"worktree.baseRef %q이(가) 잘못되었습니다(%q 또는 %q 필요)",
		"Недопустимое значение worktree.baseRef %q (ожидается %q или %q)")
	add(KeyToolWorktreeSparsePathInvalid,
		"invalid sparse-checkout path %q",
		"sparse-checkout 路径 %q 无效",
		"Ungültiger sparse-checkout-Pfad %q",
		"sparse-checkout のパス %q は無効です",
		"sparse-checkout 경로 %q이(가) 잘못되었습니다",
		"Недопустимый путь sparse-checkout %q")
	add(KeyToolWorktreeSparseConfigureFailed,
		"Failed to configure sparse-checkout: %s",
		"配置 sparse-checkout 失败：%s",
		"sparse-checkout konnte nicht konfiguriert werden: %s",
		"sparse-checkout を設定できませんでした: %s",
		"sparse-checkout을 구성하지 못했습니다: %s",
		"Не удалось настроить sparse-checkout: %s")
	add(KeyToolWorktreeSparseCheckoutFailed,
		"Failed to check out sparse worktree: %s",
		"检出 sparse worktree 失败：%s",
		"Sparse Worktree konnte nicht ausgecheckt werden: %s",
		"sparse worktree をチェックアウトできませんでした: %s",
		"sparse worktree를 체크아웃하지 못했습니다: %s",
		"Не удалось выполнить checkout разреженного worktree: %s")
	add(KeyToolFileLockTimedOut,
		"Timed out waiting for lock %s",
		"等待锁 %s 超时",
		"Zeitüberschreitung beim Warten auf die Sperre %s",
		"ロック %s の待機中にタイムアウトしました",
		"잠금 %s을(를) 기다리는 동안 시간이 초과되었습니다",
		"Истекло время ожидания блокировки %s")
	add(KeyToolPDFCreateSwiftRendererScript,
		"Failed to create Swift PDF renderer script: %v",
		"无法创建 Swift PDF renderer 脚本：%v",
		"Swift-Skript für den PDF-Renderer konnte nicht erstellt werden: %v",
		"Swift PDF renderer スクリプトを作成できませんでした: %v",
		"Swift PDF renderer 스크립트를 만들지 못했습니다: %v",
		"Не удалось создать Swift-скрипт PDF renderer: %v")
	add(KeyToolPDFWriteSwiftRendererScript,
		"Failed to write Swift PDF renderer script: %v",
		"无法写入 Swift PDF renderer 脚本：%v",
		"Swift-Skript für den PDF-Renderer konnte nicht geschrieben werden: %v",
		"Swift PDF renderer スクリプトを書き込めませんでした: %v",
		"Swift PDF renderer 스크립트를 쓰지 못했습니다: %v",
		"Не удалось записать Swift-скрипт PDF renderer: %v")
	add(KeyToolNotificationManagerShuttingDown,
		"Background notification manager is shutting down",
		"后台通知管理器正在关闭",
		"Der Manager für Hintergrundbenachrichtigungen wird beendet",
		"バックグラウンド通知マネージャーを終了しています",
		"백그라운드 알림 관리자를 종료하는 중입니다",
		"Диспетчер фоновых уведомлений завершает работу")
	add(KeyToolNotificationHookUnavailable,
		"Notification hook is unavailable",
		"notification hook 不可用",
		"Der Notification-Hook ist nicht verfügbar",
		"notification hook を利用できません",
		"notification hook을 사용할 수 없습니다",
		"Hook уведомлений недоступен")
	add(KeyToolNotificationHookFailed,
		"Notification hook %s failed (exit=%d): %s",
		"notification hook %s 失败（exit=%d）：%s",
		"Notification-Hook %s ist fehlgeschlagen (exit=%d): %s",
		"notification hook %s が失敗しました（exit=%d）: %s",
		"notification hook %s이(가) 실패했습니다(exit=%d): %s",
		"Hook уведомлений %s завершился с ошибкой (exit=%d): %s")
}
