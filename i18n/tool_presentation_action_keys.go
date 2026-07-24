package i18n

// Semantic actions used by the unified tool lifecycle row. Tool and protocol
// identifiers remain available in the evidence view; the default transcript
// uses these user-facing actions instead of implementation type names.
const (
	KeyToolActionRunCommand        Key = "tool.presentation.action.run_command"
	KeyToolActionReadFile          Key = "tool.presentation.action.read_file"
	KeyToolActionCreateFile        Key = "tool.presentation.action.create_file"
	KeyToolActionUpdateFile        Key = "tool.presentation.action.update_file"
	KeyToolActionEditNotebook      Key = "tool.presentation.action.edit_notebook"
	KeyToolActionFindFiles         Key = "tool.presentation.action.find_files"
	KeyToolActionSearchText        Key = "tool.presentation.action.search_text"
	KeyToolActionInspectCode       Key = "tool.presentation.action.inspect_code"
	KeyToolActionFindTools         Key = "tool.presentation.action.find_tools"
	KeyToolActionFetchWeb          Key = "tool.presentation.action.fetch_web"
	KeyToolActionSearchWeb         Key = "tool.presentation.action.search_web"
	KeyToolActionUseMCPTool        Key = "tool.presentation.action.use_mcp_tool"
	KeyToolActionListMCPResources  Key = "tool.presentation.action.list_mcp_resources"
	KeyToolActionReadMCPResource   Key = "tool.presentation.action.read_mcp_resource"
	KeyToolActionRunAgent          Key = "tool.presentation.action.run_agent"
	KeyToolActionCreateTask        Key = "tool.presentation.action.create_task"
	KeyToolActionListTasks         Key = "tool.presentation.action.list_tasks"
	KeyToolActionUpdateTask        Key = "tool.presentation.action.update_task"
	KeyToolActionGetTask           Key = "tool.presentation.action.get_task"
	KeyToolActionStopTask          Key = "tool.presentation.action.stop_task"
	KeyToolActionReadTaskOutput    Key = "tool.presentation.action.read_task_output"
	KeyToolActionUpdateTodos       Key = "tool.presentation.action.update_todos"
	KeyToolActionGetGoal           Key = "tool.presentation.action.get_goal"
	KeyToolActionCreateGoal        Key = "tool.presentation.action.create_goal"
	KeyToolActionUpdateGoal        Key = "tool.presentation.action.update_goal"
	KeyToolActionEnterPlanMode     Key = "tool.presentation.action.enter_plan_mode"
	KeyToolActionExitPlanMode      Key = "tool.presentation.action.exit_plan_mode"
	KeyToolActionAskUser           Key = "tool.presentation.action.ask_user"
	KeyToolActionSendUserMessage   Key = "tool.presentation.action.send_user_message"
	KeyToolActionSendMessage       Key = "tool.presentation.action.send_message"
	KeyToolActionCreateTeam        Key = "tool.presentation.action.create_team"
	KeyToolActionDeleteTeam        Key = "tool.presentation.action.delete_team"
	KeyToolActionCreateSchedule    Key = "tool.presentation.action.create_schedule"
	KeyToolActionDeleteSchedule    Key = "tool.presentation.action.delete_schedule"
	KeyToolActionListSchedules     Key = "tool.presentation.action.list_schedules"
	KeyToolActionEnterWorktree     Key = "tool.presentation.action.enter_worktree"
	KeyToolActionExitWorktree      Key = "tool.presentation.action.exit_worktree"
	KeyToolActionReadConfiguration Key = "tool.presentation.action.read_configuration"
	KeyToolActionConfigure         Key = "tool.presentation.action.configure"
	KeyToolActionLoadSkill         Key = "tool.presentation.action.load_skill"
	KeyToolActionRemoteRequest     Key = "tool.presentation.action.remote_request"

	KeyToolEmptyMatches   Key = "tool.presentation.empty.matches"
	KeyToolEmptyFiles     Key = "tool.presentation.empty.files"
	KeyToolEmptyTools     Key = "tool.presentation.empty.tools"
	KeyToolEmptySources   Key = "tool.presentation.empty.sources"
	KeyToolEmptyResources Key = "tool.presentation.empty.resources"
	KeyToolEmptyTasks     Key = "tool.presentation.empty.tasks"
	KeyToolEmptySchedules Key = "tool.presentation.empty.schedules"
)

func init() {
	for key, translations := range map[Key]map[Language]string{
		KeyToolActionRunCommand:        toolPresentationAction("Run command", "运行命令", "Befehl ausführen", "コマンドを実行", "명령 실행", "Выполнить команду"),
		KeyToolActionReadFile:          toolPresentationAction("Read file", "读取文件", "Datei lesen", "ファイルを読み取り", "파일 읽기", "Прочитать файл"),
		KeyToolActionCreateFile:        toolPresentationAction("Create file", "创建文件", "Datei erstellen", "ファイルを作成", "파일 만들기", "Создать файл"),
		KeyToolActionUpdateFile:        toolPresentationAction("Update file", "更新文件", "Datei aktualisieren", "ファイルを更新", "파일 업데이트", "Обновить файл"),
		KeyToolActionEditNotebook:      toolPresentationAction("Edit notebook", "编辑 Notebook", "Notebook bearbeiten", "Notebook を編集", "Notebook 편집", "Изменить Notebook"),
		KeyToolActionFindFiles:         toolPresentationAction("Find files", "查找文件", "Dateien finden", "ファイルを検索", "파일 찾기", "Найти файлы"),
		KeyToolActionSearchText:        toolPresentationAction("Search text", "搜索文本", "Text durchsuchen", "テキストを検索", "텍스트 검색", "Найти текст"),
		KeyToolActionInspectCode:       toolPresentationAction("Inspect code", "检查代码", "Code untersuchen", "コードを調査", "코드 검사", "Исследовать код"),
		KeyToolActionFindTools:         toolPresentationAction("Find tools", "查找工具", "Tools finden", "ツールを検索", "도구 찾기", "Найти инструменты"),
		KeyToolActionFetchWeb:          toolPresentationAction("Fetch web page", "获取网页", "Webseite abrufen", "Web ページを取得", "웹 페이지 가져오기", "Получить веб-страницу"),
		KeyToolActionSearchWeb:         toolPresentationAction("Search web", "搜索网页", "Web durchsuchen", "Web を検索", "웹 검색", "Искать в интернете"),
		KeyToolActionUseMCPTool:        toolPresentationAction("Use MCP tool", "调用 MCP 工具", "MCP-Tool verwenden", "MCP ツールを使用", "MCP 도구 사용", "Использовать инструмент MCP"),
		KeyToolActionListMCPResources:  toolPresentationAction("List MCP resources", "获取 MCP 资源", "MCP-Ressourcen auflisten", "MCP リソースを取得", "MCP 리소스 조회", "Получить ресурсы MCP"),
		KeyToolActionReadMCPResource:   toolPresentationAction("Read MCP resource", "读取 MCP 资源", "MCP-Ressource lesen", "MCP リソースを読み取り", "MCP 리소스 읽기", "Прочитать ресурс MCP"),
		KeyToolActionRunAgent:          toolPresentationAction("Run Agent", "运行 Agent", "Agent ausführen", "Agent を実行", "Agent 실행", "Запустить Agent"),
		KeyToolActionCreateTask:        toolPresentationAction("Create task", "创建任务", "Aufgabe erstellen", "タスクを作成", "작업 만들기", "Создать задачу"),
		KeyToolActionListTasks:         toolPresentationAction("List tasks", "查看任务", "Aufgaben auflisten", "タスクを表示", "작업 보기", "Показать задачи"),
		KeyToolActionUpdateTask:        toolPresentationAction("Update task", "更新任务", "Aufgabe aktualisieren", "タスクを更新", "작업 업데이트", "Обновить задачу"),
		KeyToolActionGetTask:           toolPresentationAction("Get task", "获取任务", "Aufgabe abrufen", "タスクを取得", "작업 가져오기", "Получить задачу"),
		KeyToolActionStopTask:          toolPresentationAction("Stop task", "停止任务", "Aufgabe stoppen", "タスクを停止", "작업 중지", "Остановить задачу"),
		KeyToolActionReadTaskOutput:    toolPresentationAction("Read task output", "读取任务输出", "Aufgabenausgabe lesen", "タスク出力を読み取り", "작업 출력 읽기", "Прочитать вывод задачи"),
		KeyToolActionUpdateTodos:       toolPresentationAction("Update checklist", "更新清单", "Checkliste aktualisieren", "チェックリストを更新", "체크리스트 업데이트", "Обновить список"),
		KeyToolActionGetGoal:           toolPresentationAction("Get goal", "获取目标", "Ziel abrufen", "目標を取得", "목표 가져오기", "Получить цель"),
		KeyToolActionCreateGoal:        toolPresentationAction("Create goal", "创建目标", "Ziel erstellen", "目標を作成", "목표 만들기", "Создать цель"),
		KeyToolActionUpdateGoal:        toolPresentationAction("Update goal", "更新目标", "Ziel aktualisieren", "目標を更新", "목표 업데이트", "Обновить цель"),
		KeyToolActionEnterPlanMode:     toolPresentationAction("Enter plan mode", "进入计划模式", "Planmodus starten", "計画モードに入る", "계획 모드 시작", "Перейти в режим плана"),
		KeyToolActionExitPlanMode:      toolPresentationAction("Submit plan", "提交计划", "Plan einreichen", "計画を提出", "계획 제출", "Отправить план"),
		KeyToolActionAskUser:           toolPresentationAction("Ask for input", "请求输入", "Eingabe anfordern", "入力を求める", "입력 요청", "Запросить ввод"),
		KeyToolActionSendUserMessage:   toolPresentationAction("Send update", "发送更新", "Update senden", "更新を送信", "업데이트 보내기", "Отправить обновление"),
		KeyToolActionSendMessage:       toolPresentationAction("Send message", "发送消息", "Nachricht senden", "メッセージを送信", "메시지 보내기", "Отправить сообщение"),
		KeyToolActionCreateTeam:        toolPresentationAction("Create team", "创建团队", "Team erstellen", "チームを作成", "팀 만들기", "Создать команду"),
		KeyToolActionDeleteTeam:        toolPresentationAction("Delete team", "删除团队", "Team löschen", "チームを削除", "팀 삭제", "Удалить команду"),
		KeyToolActionCreateSchedule:    toolPresentationAction("Create schedule", "创建定时任务", "Zeitplan erstellen", "スケジュールを作成", "일정 만들기", "Создать расписание"),
		KeyToolActionDeleteSchedule:    toolPresentationAction("Delete schedule", "删除定时任务", "Zeitplan löschen", "スケジュールを削除", "일정 삭제", "Удалить расписание"),
		KeyToolActionListSchedules:     toolPresentationAction("List schedules", "查看定时任务", "Zeitpläne auflisten", "スケジュールを表示", "일정 보기", "Показать расписания"),
		KeyToolActionEnterWorktree:     toolPresentationAction("Enter worktree", "进入 Worktree", "Worktree öffnen", "Worktree に入る", "Worktree 열기", "Открыть Worktree"),
		KeyToolActionExitWorktree:      toolPresentationAction("Exit worktree", "退出 Worktree", "Worktree verlassen", "Worktree を終了", "Worktree 종료", "Закрыть Worktree"),
		KeyToolActionReadConfiguration: toolPresentationAction("Read configuration", "查看配置", "Konfiguration lesen", "設定を読み取り", "구성 읽기", "Прочитать конфигурацию"),
		KeyToolActionConfigure:         toolPresentationAction("Update configuration", "更新配置", "Konfiguration aktualisieren", "設定を更新", "구성 업데이트", "Обновить конфигурацию"),
		KeyToolActionLoadSkill:         toolPresentationAction("Load skill", "加载 Skill", "Skill laden", "Skill を読み込み", "Skill 불러오기", "Загрузить Skill"),
		KeyToolActionRemoteRequest:     toolPresentationAction("Remote request", "远程请求", "Remote-Anfrage", "リモートリクエスト", "원격 요청", "Удалённый запрос"),

		KeyToolEmptyMatches:   toolPresentationAction("No matches found", "未找到匹配项", "Keine Treffer gefunden", "一致する結果はありません", "일치 항목 없음", "Совпадений не найдено"),
		KeyToolEmptyFiles:     toolPresentationAction("No files found", "未找到文件", "Keine Dateien gefunden", "ファイルが見つかりません", "파일 없음", "Файлы не найдены"),
		KeyToolEmptyTools:     toolPresentationAction("No tools found", "未找到工具", "Keine Tools gefunden", "ツールが見つかりません", "도구 없음", "Инструменты не найдены"),
		KeyToolEmptySources:   toolPresentationAction("No sources found", "未找到来源", "Keine Quellen gefunden", "情報源が見つかりません", "소스 없음", "Источники не найдены"),
		KeyToolEmptyResources: toolPresentationAction("No MCP resources found", "未找到 MCP 资源", "Keine MCP-Ressourcen gefunden", "MCP リソースが見つかりません", "MCP 리소스 없음", "Ресурсы MCP не найдены"),
		KeyToolEmptyTasks:     toolPresentationAction("No tasks", "没有任务", "Keine Aufgaben", "タスクはありません", "작업 없음", "Задач нет"),
		KeyToolEmptySchedules: toolPresentationAction("No schedules", "没有定时任务", "Keine Zeitpläne", "スケジュールはありません", "일정 없음", "Расписаний нет"),
	} {
		semanticTranslations[key] = translations
	}
}

func toolPresentationAction(en, zh, de, ja, ko, ru string) map[Language]string {
	return map[Language]string{
		LangEN: en,
		LangZH: zh,
		LangDE: de,
		LangJA: ja,
		LangKO: ko,
		LangRU: ru,
	}
}
