package i18n

// Deep tool-runtime errors are raised below the immediate Tool.Execute layer
// and may later be surfaced through a ToolResult. Tool and protocol names,
// identifiers, statuses, and raw causes are supplied as format arguments so
// they remain unchanged in every locale.
const (
	KeyToolRuntimeBackgroundTaskRunningOtherProcess Key = "tool.runtime.background.task_running_other_process"
	KeyToolRuntimeBackgroundTaskNotRunning          Key = "tool.runtime.background.task_not_running"
	KeyToolRuntimeBackgroundTaskNotFound            Key = "tool.runtime.background.task_not_found"
	KeyToolRuntimeBackgroundOutputDirCreateFailed   Key = "tool.runtime.background.output_dir_create_failed"
	KeyToolRuntimeBackgroundCommandStartFailed      Key = "tool.runtime.background.command_start_failed"
	KeyToolRuntimeTeamUniqueNameGenerationFailed    Key = "tool.runtime.team.unique_name_generation_failed"
)

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}

	add(KeyToolRuntimeBackgroundTaskRunningOtherProcess,
		"%s %s is running in another process and cannot be stopped from this session",
		"%s %s 正在另一个进程中运行，无法从当前会话停止",
		"%s %s läuft in einem anderen Prozess und kann in dieser Sitzung nicht gestoppt werden",
		"%s %s は別のプロセスで実行中のため、このセッションから停止できません",
		"%s %s은(는) 다른 프로세스에서 실행 중이므로 이 세션에서 중지할 수 없습니다",
		"%s %s выполняется в другом процессе, поэтому его нельзя остановить из этого сеанса")
	add(KeyToolRuntimeBackgroundTaskNotRunning,
		"%s %s is not running (status: %s)",
		"%s %s 未在运行（状态：%s）",
		"%s %s wird nicht ausgeführt (Status: %s)",
		"%s %s は実行されていません（ステータス: %s）",
		"%s %s은(는) 실행 중이 아닙니다(상태: %s)",
		"%s %s не выполняется (статус: %s)")
	add(KeyToolRuntimeBackgroundTaskNotFound,
		"No %s found with %s: %s",
		"未找到 %s（%s：%s）",
		"Kein %s mit %s %s gefunden",
		"%s が見つかりません（%s: %s）",
		"%s을(를) 찾을 수 없습니다(%s: %s)",
		"Не найден %s с %s: %s")
	add(KeyToolRuntimeBackgroundOutputDirCreateFailed,
		"create background task output dir: %v",
		"创建后台任务输出目录失败：%v",
		"Ausgabeverzeichnis der Hintergrundaufgabe konnte nicht erstellt werden: %v",
		"バックグラウンドタスクの出力ディレクトリを作成できませんでした: %v",
		"백그라운드 작업 출력 디렉터리를 만들 수 없습니다: %v",
		"Не удалось создать каталог вывода фоновой задачи: %v")
	add(KeyToolRuntimeBackgroundCommandStartFailed,
		"start background command: %v",
		"启动后台命令失败：%v",
		"Hintergrundbefehl konnte nicht gestartet werden: %v",
		"バックグラウンドコマンドを開始できませんでした: %v",
		"백그라운드 명령을 시작할 수 없습니다: %v",
		"Не удалось запустить фоновую команду: %v")
	add(KeyToolRuntimeTeamUniqueNameGenerationFailed,
		"failed to generate a unique %s name",
		"无法生成唯一的 %s 名称",
		"Es konnte kein eindeutiger %s-Name erzeugt werden",
		"一意の %s 名を生成できませんでした",
		"고유한 %s 이름을 생성할 수 없습니다",
		"Не удалось создать уникальное имя %s")
}
