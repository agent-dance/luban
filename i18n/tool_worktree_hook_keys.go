package i18n

const (
	KeyToolWorktreeHookCommandEmpty  Key = "tool.worktree_hook.command_empty"
	KeyToolWorktreeHookEncodePayload Key = "tool.worktree_hook.encode_payload"
	KeyToolWorktreeHookTimedOut      Key = "tool.worktree_hook.timed_out"
	KeyToolWorktreeHookFailed        Key = "tool.worktree_hook.failed"
	KeyToolWorktreeHookOutputLarge   Key = "tool.worktree_hook.output_too_large"
	KeyToolWorktreeHookReadSettings  Key = "tool.worktree_hook.read_settings"
	KeyToolWorktreeHookParseSettings Key = "tool.worktree_hook.parse_settings"
	KeyToolWorktreeHookNoOutput      Key = "tool.worktree_hook.create.no_output"
	KeyToolWorktreeHookPathMissing   Key = "tool.worktree_hook.create.path_missing"
	KeyToolWorktreeHookOutputFormat  Key = "tool.worktree_hook.create.output_format"
)

var toolWorktreeHookKeys = [...]Key{
	KeyToolWorktreeHookCommandEmpty,
	KeyToolWorktreeHookEncodePayload,
	KeyToolWorktreeHookTimedOut,
	KeyToolWorktreeHookFailed,
	KeyToolWorktreeHookOutputLarge,
	KeyToolWorktreeHookReadSettings,
	KeyToolWorktreeHookParseSettings,
	KeyToolWorktreeHookNoOutput,
	KeyToolWorktreeHookPathMissing,
	KeyToolWorktreeHookOutputFormat,
}

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}
	add(KeyToolWorktreeHookCommandEmpty,
		"%s hook command is empty", "%s hook 命令为空", "Der Befehl für Hook %s ist leer", "%s hook のコマンドが空です", "%s hook 명령이 비어 있습니다", "Команда hook %s пуста")
	add(KeyToolWorktreeHookEncodePayload,
		"encode %s hook payload: %v", "编码 %s hook payload 失败：%v", "Payload für Hook %s konnte nicht codiert werden: %v", "%s hook の payload をエンコードできませんでした: %v", "%s hook payload를 인코딩하지 못했습니다: %v", "Не удалось закодировать payload hook %s: %v")
	add(KeyToolWorktreeHookTimedOut,
		"%s hook timed out after %s", "%s hook 在 %s 后超时", "Zeitüberschreitung bei Hook %s nach %s", "%s hook は %s でタイムアウトしました", "%s hook이 %s 후 시간 초과되었습니다", "Истекло время ожидания hook %s (%s)")
	add(KeyToolWorktreeHookFailed,
		"%s hook failed: %s", "%s hook 失败：%s", "Hook %s ist fehlgeschlagen: %s", "%s hook に失敗しました: %s", "%s hook 실패: %s", "Hook %s завершился ошибкой: %s")
	add(KeyToolWorktreeHookOutputLarge,
		"%s hook output exceeded 1 MiB", "%s hook 输出超过 1 MiB", "Die Ausgabe von Hook %s überschritt 1 MiB", "%s hook の出力が 1 MiB を超えました", "%s hook 출력이 1MiB를 초과했습니다", "Вывод hook %s превысил 1 МиБ")
	add(KeyToolWorktreeHookReadSettings,
		"read worktree hooks from %s: %v", "从 %s 读取 worktree hook 失败：%v", "Worktree-Hooks konnten nicht aus %s gelesen werden: %v", "%s から worktree hook を読み込めませんでした: %v", "%s에서 worktree hook을 읽지 못했습니다: %v", "Не удалось прочитать hook worktree из %s: %v")
	add(KeyToolWorktreeHookParseSettings,
		"parse worktree hooks from %s: %v", "解析 %s 中的 worktree hook 失败：%v", "Worktree-Hooks aus %s konnten nicht verarbeitet werden: %v", "%s の worktree hook を解析できませんでした: %v", "%s의 worktree hook을 파싱하지 못했습니다: %v", "Не удалось разобрать hook worktree из %s: %v")
	add(KeyToolWorktreeHookNoOutput,
		"WorktreeCreate hook failed: no successful output", "WorktreeCreate hook 失败：没有成功输出", "WorktreeCreate-Hook fehlgeschlagen: keine erfolgreiche Ausgabe", "WorktreeCreate hook に失敗しました: 正常な出力がありません", "WorktreeCreate hook 실패: 정상 출력이 없습니다", "Hook WorktreeCreate завершился ошибкой: нет успешного вывода")
	add(KeyToolWorktreeHookPathMissing,
		"WorktreeCreate hook output is missing path", "WorktreeCreate hook 输出缺少 path", "In der Ausgabe des WorktreeCreate-Hooks fehlt der Pfad", "WorktreeCreate hook の出力に path がありません", "WorktreeCreate hook 출력에 path가 없습니다", "В выводе hook WorktreeCreate отсутствует path")
	add(KeyToolWorktreeHookOutputFormat,
		"WorktreeCreate hook output must be JSON or a single path", "WorktreeCreate hook 输出必须是 JSON 或单个路径", "Die Ausgabe des WorktreeCreate-Hooks muss JSON oder ein einzelner Pfad sein", "WorktreeCreate hook の出力は JSON または単一のパスである必要があります", "WorktreeCreate hook 출력은 JSON 또는 단일 경로여야 합니다", "Вывод hook WorktreeCreate должен быть JSON или одним путём")
}
