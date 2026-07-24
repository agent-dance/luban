package i18n

// Semantic keys for built-in slash-command descriptions. Command names and
// syntax remain unchanged; only their explanatory copy is localized.
const (
	KeyCommandActivityDescription    Key = "command.activity.description"
	KeyCommandClearDescription       Key = "command.clear.description"
	KeyCommandCompactDescription     Key = "command.compact.description"
	KeyCommandConfigDescription      Key = "command.config.description"
	KeyCommandConnectDescription     Key = "command.connect.description"
	KeyCommandCostDescription        Key = "command.cost.description"
	KeyCommandDetailDescription      Key = "command.detail.description"
	KeyCommandDiffDescription        Key = "command.diff.description"
	KeyCommandDoctorDescription      Key = "command.doctor.description"
	KeyCommandEditorDescription      Key = "command.editor.description"
	KeyCommandExitDescription        Key = "command.exit.description"
	KeyCommandExportDescription      Key = "command.export.description"
	KeyCommandForkDescription        Key = "command.fork.description"
	KeyCommandHelpDescription        Key = "command.help.description"
	KeyCommandInitDescription        Key = "command.init.description"
	KeyCommandLanguageDescription    Key = "command.language.description"
	KeyCommandMCPDescription         Key = "command.mcp.description"
	KeyCommandMemoryDescription      Key = "command.memory.description"
	KeyCommandModelDescription       Key = "command.model.description"
	KeyCommandMouseDescription       Key = "command.mouse.description"
	KeyCommandPasteDescription       Key = "command.paste.description"
	KeyCommandPermissionsDescription Key = "command.permissions.description"
	KeyCommandRenameDescription      Key = "command.rename.description"
	KeyCommandResumeDescription      Key = "command.resume.description"
	KeyCommandReviewDescription      Key = "command.review.description"
	KeyCommandSearchDescription      Key = "command.search.description"
	KeyCommandSessionDescription     Key = "command.session.description"
	KeyCommandVersionDescription     Key = "command.version.description"
)

var commandDescriptionKeys = map[string]Key{
	"activity":    KeyCommandActivityDescription,
	"clear":       KeyCommandClearDescription,
	"compact":     KeyCommandCompactDescription,
	"config":      KeyCommandConfigDescription,
	"connect":     KeyCommandConnectDescription,
	"context":     KeyCommandContextDescription,
	"cost":        KeyCommandCostDescription,
	"detail":      KeyCommandDetailDescription,
	"diff":        KeyCommandDiffDescription,
	"doctor":      KeyCommandDoctorDescription,
	"editor":      KeyCommandEditorDescription,
	"exit":        KeyCommandExitDescription,
	"export":      KeyCommandExportDescription,
	"fork":        KeyCommandForkDescription,
	"goal":        KeyCommandGoalDescription,
	"help":        KeyCommandHelpDescription,
	"init":        KeyCommandInitDescription,
	"language":    KeyCommandLanguageDescription,
	"mcp":         KeyCommandMCPDescription,
	"memory":      KeyCommandMemoryDescription,
	"model":       KeyCommandModelDescription,
	"mouse":       KeyCommandMouseDescription,
	"paste":       KeyCommandPasteDescription,
	"permissions": KeyCommandPermissionsDescription,
	"rename":      KeyCommandRenameDescription,
	"resume":      KeyCommandResumeDescription,
	"review":      KeyCommandReviewDescription,
	"search":      KeyCommandSearchDescription,
	"session":     KeyCommandSessionDescription,
	"skills":      KeyCommandSkillsDescription,
	"status":      KeyCommandStatusDescription,
	"version":     KeyCommandVersionDescription,
}

// CommandDescriptionKey resolves a canonical slash-command name to its
// semantic description key. Extension commands without a registered key keep
// their own Description text.
func CommandDescriptionKey(name string) (Key, bool) {
	key, ok := commandDescriptionKeys[name]
	return key, ok
}

func init() {
	descriptions := map[Key]map[Language]string{
		KeyCommandActivityDescription: commandCore(
			"View or control parallel activity",
			"查看或控制并行任务",
			"Parallele Aktivitäten anzeigen oder steuern",
			"並行アクティビティを表示または操作",
			"병렬 활동 보기 또는 제어",
			"Просмотр и управление параллельными задачами"),
		KeyCommandClearDescription: commandCore(
			"Clear the view or start a new conversation",
			"清空当前视图或开始新对话",
			"Ansicht leeren oder eine neue Unterhaltung beginnen",
			"表示を消去または新しい会話を開始",
			"현재 보기를 지우거나 새 대화 시작",
			"Очистить экран или начать новый диалог"),
		KeyCommandCompactDescription: commandCore(
			"Manually compact the context",
			"手动压缩上下文",
			"Kontext manuell komprimieren",
			"コンテキストを手動で圧縮",
			"컨텍스트 수동 압축",
			"Сжать контекст вручную"),
		KeyCommandConfigDescription: commandCore(
			"Show or edit LUBAN Code settings",
			"查看或编辑 LUBAN Code 设置",
			"LUBAN Code-Einstellungen anzeigen oder bearbeiten",
			"LUBAN Code の設定を表示または編集",
			"LUBAN Code 설정 보기 또는 편집",
			"Показать или изменить настройки LUBAN Code"),
		KeyCommandConnectDescription: commandCore(
			"Manage provider connections: /connect [provider]",
			"管理 Provider 连接：/connect [provider]",
			"Provider-Verbindungen verwalten: /connect [provider]",
			"Provider 接続を管理: /connect [provider]",
			"Provider 연결 관리: /connect [provider]",
			"Управлять подключениями Provider: /connect [provider]"),
		KeyCommandCostDescription: commandCore(
			"Show cumulative token usage for this session",
			"查看当前会话的累计 token 用量",
			"Kumulierte Token-Nutzung dieser Sitzung anzeigen",
			"このセッションの累計トークン使用量を表示",
			"이 세션의 누적 토큰 사용량 보기",
			"Показать суммарное использование токенов за сеанс"),
		KeyCommandDetailDescription: commandCore(
			"Set the observation detail level",
			"设置观察信息的详细程度",
			"Detailstufe einer Beobachtung festlegen",
			"観察情報の詳細度を設定",
			"관찰 정보의 세부 수준 설정",
			"Задать уровень детализации наблюдения"),
		KeyCommandDiffDescription: commandCore(
			"Show all file changes in this session (git diff)",
			"查看当前会话中的所有文件变更（git diff）",
			"Alle Dateiänderungen dieser Sitzung anzeigen (git diff)",
			"このセッションの全ファイル変更を表示 (git diff)",
			"이 세션의 모든 파일 변경 사항 보기 (git diff)",
			"Показать все изменения файлов в этом сеансе (git diff)"),
		KeyCommandDoctorDescription: commandCore(
			"Diagnose environment and configuration issues",
			"诊断环境和配置问题",
			"Umgebungs- und Konfigurationsprobleme diagnostizieren",
			"環境と設定の問題を診断",
			"환경 및 구성 문제 진단",
			"Диагностировать проблемы окружения и конфигурации"),
		KeyCommandEditorDescription: commandCore(
			"Open the complete transcript in $VISUAL or $EDITOR",
			"在 $VISUAL 或 $EDITOR 中打开完整对话记录",
			"Vollständiges Transkript in $VISUAL oder $EDITOR öffnen",
			"完全なトランスクリプトを $VISUAL または $EDITOR で開く",
			"전체 대화 기록을 $VISUAL 또는 $EDITOR에서 열기",
			"Открыть полную стенограмму в $VISUAL или $EDITOR"),
		KeyCommandExitDescription: commandCore(
			"Exit the REPL",
			"退出交互模式",
			"REPL beenden",
			"REPL を終了",
			"REPL 종료",
			"Выйти из REPL"),
		KeyCommandExportDescription: commandCore(
			"Export the complete transcript",
			"导出完整对话记录",
			"Vollständiges Transkript exportieren",
			"完全なトランスクリプトをエクスポート",
			"전체 대화 기록 내보내기",
			"Экспортировать полную стенограмму"),
		KeyCommandForkDescription: commandCore(
			"Fork this conversation from an earlier turn",
			"从较早的对话轮次分叉当前会话",
			"Diese Unterhaltung ab einer früheren Runde verzweigen",
			"以前のターンからこの会話を分岐",
			"이전 대화 턴에서 현재 대화 분기",
			"Ответвить этот диалог от более раннего хода"),
		KeyCommandHelpDescription: commandCore(
			"List all available commands",
			"列出所有可用命令",
			"Alle verfügbaren Befehle auflisten",
			"利用可能なコマンドをすべて表示",
			"사용 가능한 모든 명령 나열",
			"Показать все доступные команды"),
		KeyCommandInitDescription: commandCore(
			"Initialize LUBAN Code project instructions and configuration",
			"初始化 LUBAN Code 项目指令和配置",
			"Projektanweisungen und Konfiguration für LUBAN Code initialisieren",
			"LUBAN Code のプロジェクト指示と設定を初期化",
			"LUBAN Code 프로젝트 지침 및 구성 초기화",
			"Инициализировать инструкции и конфигурацию проекта LUBAN Code"),
		KeyCommandLanguageDescription: commandCore(
			"Choose the display language",
			"选择显示语言",
			"Anzeigesprache auswählen",
			"表示言語を選択",
			"표시 언어 선택",
			"Выбрать язык интерфейса"),
		KeyCommandMCPDescription: commandCore(
			"Manage MCP servers: list, enable, disable, reconnect, authenticate",
			"管理 MCP 服务器：列出、启用、禁用、重连和认证",
			"MCP-Server verwalten: auflisten, aktivieren, deaktivieren, neu verbinden und authentifizieren",
			"MCP サーバーを管理: 一覧、有効化、無効化、再接続、認証",
			"MCP 서버 관리: 목록, 활성화, 비활성화, 재연결, 인증",
			"Управлять MCP-серверами: список, включение, отключение, переподключение и аутентификация"),
		KeyCommandMemoryDescription: commandCore(
			"Edit LUBAN Code instruction files",
			"编辑 LUBAN Code 指令文件",
			"LUBAN Code-Anweisungsdateien bearbeiten",
			"LUBAN Code の指示ファイルを編集",
			"LUBAN Code 지침 파일 편집",
			"Редактировать файлы инструкций LUBAN Code"),
		KeyCommandModelDescription: commandCore(
			"Show or switch the model: /model [provider/]<name>",
			"查看或切换模型：/model [provider/]<name>",
			"Modell anzeigen oder wechseln: /model [provider/]<name>",
			"モデルを表示または切り替え: /model [provider/]<name>",
			"모델 보기 또는 전환: /model [provider/]<name>",
			"Показать или сменить модель: /model [provider/]<name>"),
		KeyCommandMouseDescription: commandCore(
			"Enable or disable terminal mouse capture",
			"启用或禁用终端鼠标捕获",
			"Maussteuerung im Terminal aktivieren oder deaktivieren",
			"端末のマウスキャプチャーを有効または無効にする",
			"터미널 마우스 캡처 활성화 또는 비활성화",
			"Включить или отключить захват мыши в терминале"),
		KeyCommandPasteDescription: commandCore(
			"Paste a clipboard image and send it to LUBAN Code",
			"粘贴剪贴板图片并发送给 LUBAN Code",
			"Bild aus der Zwischenablage einfügen und an LUBAN Code senden",
			"クリップボードの画像を貼り付けて LUBAN Code に送信",
			"클립보드 이미지를 붙여넣어 LUBAN Code로 전송",
			"Вставить изображение из буфера и отправить его в LUBAN Code"),
		KeyCommandPermissionsDescription: commandCore(
			"List tool permissions or run /permissions allow|deny <tool>",
			"列出工具权限，或运行 /permissions allow|deny <tool>",
			"Tool-Berechtigungen auflisten oder /permissions allow|deny <tool> ausführen",
			"ツール権限を表示、または /permissions allow|deny <tool> を実行",
			"도구 권한을 나열하거나 /permissions allow|deny <tool> 실행",
			"Показать разрешения инструментов или выполнить /permissions allow|deny <tool>"),
		KeyCommandRenameDescription: commandCore(
			"Rename the current session or generate a name automatically",
			"重命名当前会话，或自动生成名称",
			"Aktuelle Sitzung umbenennen oder automatisch benennen",
			"現在のセッション名を変更または自動生成",
			"현재 세션 이름 변경 또는 자동 생성",
			"Переименовать текущий сеанс или создать имя автоматически"),
		KeyCommandResumeDescription: commandCore(
			"List recent sessions or resume one by ID or title",
			"列出最近会话，或按 ID 或标题恢复会话",
			"Letzte Sitzungen auflisten oder per ID bzw. Titel fortsetzen",
			"最近のセッションを表示または ID ・タイトルで再開",
			"최근 세션을 나열하거나 ID 또는 제목으로 재개",
			"Показать недавние сеансы или возобновить по ID или заголовку"),
		KeyCommandReviewDescription: commandCore(
			"Show git diff for code review (--staged for staged changes only)",
			"查看用于代码审查的 git diff（--staged 仅显示已暂存变更）",
			"Git-Diff für das Code-Review anzeigen (--staged nur für bereitgestellte Änderungen)",
			"コードレビュー用の git diff を表示 (--staged はステージ済みのみ)",
			"코드 리뷰용 git diff 보기 (--staged는 스테이지된 변경만)",
			"Показать git diff для ревью кода (--staged только для изменений в индексе)"),
		KeyCommandSearchDescription: commandCore(
			"Search the complete transcript",
			"搜索完整对话记录",
			"Vollständiges Transkript durchsuchen",
			"完全なトランスクリプトを検索",
			"전체 대화 기록 검색",
			"Искать по полной стенограмме"),
		KeyCommandSessionDescription: commandCore(
			"Show, load, rename, or explicitly delete session history",
			"查看、加载、重命名或显式删除会话历史",
			"Sitzungsverlauf anzeigen, laden, umbenennen oder ausdrücklich löschen",
			"セッション履歴の表示、読み込み、名前変更、明示的な削除",
			"세션 기록 보기, 로드, 이름 변경 또는 명시적 삭제",
			"Показать, загрузить, переименовать или явно удалить историю сеансов"),
		KeyCommandSkillsDescription: commandCore(
			"Manage skills: list, show, enable, disable, refresh",
			"管理 Skill：列出、查看、启用、禁用和刷新",
			"Skills verwalten: auflisten, anzeigen, aktivieren, deaktivieren und aktualisieren",
			"Skill を管理: 一覧、詳細、有効化、無効化、更新",
			"Skill 관리: 목록, 상세, 활성화, 비활성화, 새로 고침",
			"Управлять Skill: список, просмотр, включение, отключение и обновление"),
		KeyCommandVersionDescription: commandCore(
			"Print the application version",
			"显示应用版本",
			"Anwendungsversion anzeigen",
			"アプリケーションのバージョンを表示",
			"애플리케이션 버전 출력",
			"Показать версию приложения"),
	}
	for key, translations := range descriptions {
		semanticTranslations[key] = translations
	}
}
