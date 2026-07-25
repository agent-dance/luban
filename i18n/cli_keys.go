package i18n

// Semantic copy for the command-line parser and help screen. Flag names,
// environment variables, formats, commands, and paths remain literal.
const (
	KeyCLIUsage                 Key = "cli.help.usage"
	KeyCLIOptions               Key = "cli.help.options"
	KeyCLIExamples              Key = "cli.help.examples"
	KeyCLIExampleInteractive    Key = "cli.help.example.interactive"
	KeyCLIExamplePrint          Key = "cli.help.example.print"
	KeyCLIExampleModel          Key = "cli.help.example.model"
	KeyCLIExampleAllowedDir     Key = "cli.help.example.allowed_dir"
	KeyCLIFlagDefault           Key = "cli.help.flag.default"
	KeyCLIError                 Key = "cli.error"
	KeyCLIParseFailure          Key = "cli.parse.failure"
	KeyCLIInvalidSessionChars   Key = "cli.session_id.invalid_chars"
	KeyCLIInvalidSessionParent  Key = "cli.session_id.invalid_parent"
	KeyCLIInputModeSDKPrint     Key = "cli.input_mode.conflict.sdk_print"
	KeyCLIStdinReadFailure      Key = "cli.stdin.read_failure"
	KeyCLIStdinTooLarge         Key = "cli.stdin.too_large"
	KeyCLIScreenReaderSDK       Key = "cli.screen_reader.conflict.sdk"
	KeyCLIScreenReaderPrint     Key = "cli.screen_reader.conflict.print"
	KeyCLIScreenReaderOutput    Key = "cli.screen_reader.conflict.output"
	KeyCLIScreenReaderTerminal  Key = "cli.screen_reader.requires_terminal"
	KeyCLIWorkingDirectoryError Key = "cli.working_directory.error"
	KeyCLIFlagModel             Key = "cli.flag.model"
	KeyCLIFlagProvider          Key = "cli.flag.provider"
	KeyCLIFlagAPI               Key = "cli.flag.api"
	KeyCLIFlagPrint             Key = "cli.flag.print"
	KeyCLIFlagResume            Key = "cli.flag.resume"
	KeyCLIFlagSessionID         Key = "cli.flag.session_id"
	KeyCLIFlagMaxTurns          Key = "cli.flag.max_turns"
	KeyCLIFlagSystemPrompt      Key = "cli.flag.system_prompt"
	KeyCLIFlagAllowedDir        Key = "cli.flag.allowed_dir"
	KeyCLIFlagAllowAll          Key = "cli.flag.allow_all"
	KeyCLIFlagAllowedTools      Key = "cli.flag.allowed_tools"
	KeyCLIFlagDisallowedTools   Key = "cli.flag.disallowed_tools"
	KeyCLIFlagSandbox           Key = "cli.flag.sandbox"
	KeyCLIFlagSDK               Key = "cli.flag.sdk"
	KeyCLIFlagVersion           Key = "cli.flag.version"
	KeyCLIFlagVerbose           Key = "cli.flag.verbose"
	KeyCLIFlagDebugFile         Key = "cli.flag.debug_file"
	KeyCLIFlagNoColor           Key = "cli.flag.no_color"
	KeyCLIFlagOutputFormat      Key = "cli.flag.output_format"
	KeyCLIFlagQuiet             Key = "cli.flag.quiet"
	KeyCLIFlagScreenReader      Key = "cli.flag.screen_reader"
	KeyCLIFlagAgents            Key = "cli.flag.agents"
	KeyCLIFlagPromptDump        Key = "cli.flag.prompt_dump"
	KeyCLIFlagPromptDumpJSON    Key = "cli.flag.prompt_dump_json"
	KeyCLIFlagLanguage          Key = "cli.flag.language"
	KeyCLIFlagOutputStyle       Key = "cli.flag.output_style"
	KeyCLIFlagAllowedDomains    Key = "cli.flag.allowed_domains"
	KeyCLIFlagDisallowedDomains Key = "cli.flag.disallowed_domains"
)

func init() {
	addCLI(KeyCLIUsage, "Usage: %s [options] [query]\n\n", "用法：%s [选项] [查询]\n\n", "Verwendung: %s [Optionen] [Anfrage]\n\n", "使い方: %s [オプション] [クエリ]\n\n", "사용법: %s [옵션] [질의]\n\n", "Использование: %s [параметры] [запрос]\n\n")
	addCLI(KeyCLIOptions, "Options:\n", "选项：\n", "Optionen:\n", "オプション:\n", "옵션:\n", "Параметры:\n")
	addCLI(KeyCLIExamples, "\nExamples:\n", "\n示例：\n", "\nBeispiele:\n", "\n例:\n", "\n예시:\n", "\nПримеры:\n")
	addCLI(KeyCLIExampleInteractive, "  %s                              # interactive terminal UI\n", "  %s                              # 交互式终端 UI\n", "  %s                              # interaktive Terminal-UI\n", "  %s                              # 対話型ターミナル UI\n", "  %s                              # 대화형 터미널 UI\n", "  %s                              # интерактивный терминальный UI\n")
	addCLI(KeyCLIExamplePrint, "  %s -p \"list files\"              # single query, print and exit\n", "  %s -p \"列出文件\"                # 单次查询，输出后退出\n", "  %s -p \"Dateien auflisten\"       # eine Anfrage, Ausgabe und Ende\n", "  %s -p \"ファイルを一覧表示\"       # 1 回だけ問い合わせ、出力して終了\n", "  %s -p \"파일 목록 보여줘\"         # 한 번 질의하고 출력 후 종료\n", "  %s -p \"покажи файлы\"             # один запрос, вывод и завершение\n")
	addCLI(KeyCLIExampleModel, "  %s --model %s       # start TUI with this model\n", "  %s --model %s       # 使用此模型启动 TUI\n", "  %s --model %s       # TUI mit diesem Modell starten\n", "  %s --model %s       # このモデルで TUI を開始\n", "  %s --model %s       # 이 모델로 TUI 시작\n", "  %s --model %s       # запустить TUI с этой моделью\n")
	addCLI(KeyCLIExampleAllowedDir, "  %s --allowed-dir /tmp -p \"write to /tmp/out.txt\"\n", "  %s --allowed-dir /tmp -p \"写入 /tmp/out.txt\"\n", "  %s --allowed-dir /tmp -p \"in /tmp/out.txt schreiben\"\n", "  %s --allowed-dir /tmp -p \"/tmp/out.txt に書き込む\"\n", "  %s --allowed-dir /tmp -p \"/tmp/out.txt에 쓰기\"\n", "  %s --allowed-dir /tmp -p \"запиши в /tmp/out.txt\"\n")
	addCLI(KeyCLIFlagDefault, " (default %s)", "（默认值：%s）", " (Standard: %s)", "（デフォルト: %s）", " (기본값: %s)", " (по умолчанию: %s)")
	addCLI(KeyCLIError, "Error: %v\n", "错误：%v\n", "Fehler: %v\n", "エラー: %v\n", "오류: %v\n", "Ошибка: %v\n")
	addCLI(KeyCLIParseFailure, "Could not parse command-line options: %v", "无法解析命令行选项：%v", "Befehlszeilenoptionen konnten nicht ausgewertet werden: %v", "コマンドラインオプションを解析できませんでした: %v", "명령줄 옵션을 해석할 수 없습니다: %v", "Не удалось разобрать параметры командной строки: %v")
	addCLI(KeyCLIInvalidSessionChars, "invalid session ID %q: use only letters, numbers, underscores, dots, colons, and hyphens", "会话 ID %q 无效：只能使用字母、数字、下划线、点、冒号和连字符", "Ungültige Sitzungs-ID %q: Nur Buchstaben, Zahlen, Unterstriche, Punkte, Doppelpunkte und Bindestriche sind erlaubt", "セッション ID %q は無効です。英数字、アンダースコア、ピリオド、コロン、ハイフンのみ使用できます", "세션 ID %q이(가) 올바르지 않습니다. 영문자, 숫자, 밑줄, 마침표, 콜론, 하이픈만 사용하세요", "Недопустимый ID сеанса %q: используйте только буквы, цифры, подчёркивания, точки, двоеточия и дефисы")
	addCLI(KeyCLIInvalidSessionParent, "invalid session ID %q: '..' is not allowed", "会话 ID %q 无效：不允许包含 '..'", "Ungültige Sitzungs-ID %q: '..' ist nicht erlaubt", "セッション ID %q は無効です。'..' は使用できません", "세션 ID %q이(가) 올바르지 않습니다. '..'은(는) 사용할 수 없습니다", "Недопустимый ID сеанса %q: '..' запрещено")
	addCLI(KeyCLIInputModeSDKPrint, "--sdk cannot be combined with --print; select one input transport", "--sdk 不能与 --print 同时使用；请选择一种输入传输模式", "--sdk kann nicht mit --print kombiniert werden; wählen Sie genau einen Eingabetransport", "--sdk と --print は同時に指定できません。入力モードを 1 つ選択してください", "--sdk와 --print는 함께 사용할 수 없습니다. 입력 전송 모드를 하나만 선택하세요", "--sdk нельзя сочетать с --print; выберите один режим ввода")
	addCLI(KeyCLIStdinReadFailure, "Could not read the piped prompt: %v", "无法读取管道中的 prompt：%v", "Der weitergeleitete Prompt konnte nicht gelesen werden: %v", "パイプから prompt を読み取れませんでした: %v", "파이프로 전달된 prompt를 읽을 수 없습니다: %v", "Не удалось прочитать prompt из конвейера: %v")
	addCLI(KeyCLIStdinTooLarge, "The piped prompt exceeds the %d-byte limit", "管道中的 prompt 超过 %d 字节限制", "Der weitergeleitete Prompt überschreitet das Limit von %d Byte", "パイプからの prompt が %d バイトの上限を超えています", "파이프로 전달된 prompt가 %d바이트 제한을 초과했습니다", "Размер prompt из конвейера превышает предел в %d байт")
	addCLI(KeyCLIScreenReaderSDK, "--screen-reader cannot be combined with --sdk because both modes use stdin", "--screen-reader 不能与 --sdk 同时使用，因为两种模式都会占用 stdin", "--screen-reader kann nicht mit --sdk kombiniert werden, da beide Modi stdin verwenden", "--screen-reader と --sdk はどちらも stdin を使用するため、同時に指定できません", "--screen-reader와 --sdk는 모두 stdin을 사용하므로 함께 사용할 수 없습니다", "--screen-reader нельзя сочетать с --sdk: оба режима используют stdin")
	addCLI(KeyCLIScreenReaderPrint, "--screen-reader cannot be combined with --print because both modes use stdin", "--screen-reader 不能与 --print 同时使用，因为两种模式都会占用 stdin", "--screen-reader kann nicht mit --print kombiniert werden, da beide Modi stdin verwenden", "--screen-reader と --print はどちらも stdin を使用するため、同時に指定できません", "--screen-reader와 --print는 모두 stdin을 사용하므로 함께 사용할 수 없습니다", "--screen-reader нельзя сочетать с --print: оба режима используют stdin")
	addCLI(KeyCLIScreenReaderOutput, "--screen-reader cannot be combined with --output-format=%s; screen-reader output must be append-only text", "--screen-reader 不能与 --output-format=%s 同时使用；屏幕阅读器输出必须为仅追加文本", "--screen-reader kann nicht mit --output-format=%s kombiniert werden; die Ausgabe muss fortlaufender Text sein", "--screen-reader と --output-format=%s は同時に指定できません。出力は追記型テキストである必要があります", "--screen-reader와 --output-format=%s는 함께 사용할 수 없습니다. 스크린 리더 출력은 추가 전용 텍스트여야 합니다", "--screen-reader нельзя сочетать с --output-format=%s; вывод должен быть добавляемым текстом")
	addCLI(KeyCLIScreenReaderTerminal, "--screen-reader requires terminal stdin; piped input belongs to print or SDK mode", "--screen-reader 需要终端 stdin；管道输入应使用 print 或 SDK 模式", "--screen-reader benötigt ein Terminal als stdin; weitergeleitete Eingaben gehören in den Print- oder SDK-Modus", "--screen-reader には端末の stdin が必要です。パイプ入力は print または SDK モードで使用してください", "--screen-reader에는 터미널 stdin이 필요합니다. 파이프 입력은 print 또는 SDK 모드에서 사용하세요", "--screen-reader требует терминальный stdin; конвейерный ввод предназначен для режима print или SDK")
	addCLI(KeyCLIWorkingDirectoryError, "cannot determine working directory: %v", "无法确定工作目录：%v", "Arbeitsverzeichnis kann nicht ermittelt werden: %v", "作業ディレクトリを特定できません: %v", "작업 디렉터리를 확인할 수 없습니다: %v", "Не удалось определить рабочий каталог: %v")

	addCLI(KeyCLIFlagModel, "Model ID to use; overrides the environment", "要使用的模型 ID；覆盖环境设置", "Zu verwendende Modell-ID; überschreibt die Umgebung", "使用するモデル ID。環境設定より優先されます", "사용할 모델 ID. 환경 설정보다 우선합니다", "ID используемой модели; переопределяет окружение")
	addCLI(KeyCLIFlagProvider, "Provider: deepseek | anthropic | openai | ollama | gemini | groq | mistral; overrides PROVIDER", "Provider：deepseek | anthropic | openai | ollama | gemini | groq | mistral；覆盖 PROVIDER", "Provider: deepseek | anthropic | openai | ollama | gemini | groq | mistral; überschreibt PROVIDER", "プロバイダー: deepseek | anthropic | openai | ollama | gemini | groq | mistral。PROVIDER より優先されます", "Provider: deepseek | anthropic | openai | ollama | gemini | groq | mistral. PROVIDER보다 우선합니다", "Провайдер: deepseek | anthropic | openai | ollama | gemini | groq | mistral; переопределяет PROVIDER")
	addCLI(KeyCLIFlagAPI, "API format: chat-completions | responses; overrides OPENAI_API", "API 格式：chat-completions | responses；覆盖 OPENAI_API", "API-Format: chat-completions | responses; überschreibt OPENAI_API", "API 形式: chat-completions | responses。OPENAI_API より優先されます", "API 형식: chat-completions | responses. OPENAI_API보다 우선합니다", "Формат API: chat-completions | responses; переопределяет OPENAI_API")
	addCLI(KeyCLIFlagPrint, "Send one query, print the result, and exit; no REPL", "发送一次查询，输出结果后退出；不启动 REPL", "Eine Anfrage senden, Ergebnis ausgeben und beenden; keine REPL", "1 回だけ問い合わせ、結果を出力して終了します。REPL は起動しません", "한 번 질의하고 결과를 출력한 뒤 종료합니다. REPL은 시작하지 않습니다", "Отправить один запрос, вывести результат и завершить работу; без REPL")
	addCLI(KeyCLIFlagResume, "Resume the most recent session", "恢复最近的会话", "Letzte Sitzung fortsetzen", "直近のセッションを再開", "가장 최근 세션 재개", "Возобновить последний сеанс")
	addCLI(KeyCLIFlagSessionID, "Resume a session by ID", "按 ID 恢复会话", "Sitzung anhand ihrer ID fortsetzen", "ID を指定してセッションを再開", "ID로 세션 재개", "Возобновить сеанс по ID")
	addCLI(KeyCLIFlagMaxTurns, "Maximum agentic turns per query", "每次查询最多执行的 Agent 轮次", "Maximale Agent-Runden pro Anfrage", "クエリごとの Agent 最大ターン数", "질의당 최대 Agent 턴 수", "Максимум агентных ходов на запрос")
	addCLI(KeyCLIFlagSystemPrompt, "Use a custom system prompt instead of the default", "使用自定义 system prompt 替代默认值", "Eigenen System-Prompt statt der Vorgabe verwenden", "デフォルトの代わりに独自の system prompt を使用", "기본값 대신 사용자 지정 system prompt 사용", "Использовать собственный system prompt вместо стандартного")
	addCLI(KeyCLIFlagAllowedDir, "Additional directory allowed for file tools; repeatable", "文件工具可访问的额外目录；可重复指定", "Zusätzliches erlaubtes Verzeichnis für Datei-Tools; wiederholbar", "ファイルツールに許可する追加ディレクトリ。複数回指定できます", "파일 도구에 허용할 추가 디렉터리. 여러 번 지정할 수 있습니다", "Дополнительный разрешённый каталог для файловых инструментов; можно повторять")
	addCLI(KeyCLIFlagAllowAll, "Skip permission prompts and allow every tool use", "跳过权限确认并允许所有工具调用", "Berechtigungsabfragen überspringen und jede Tool-Nutzung erlauben", "権限確認を省略し、すべてのツール使用を許可", "권한 확인을 건너뛰고 모든 도구 사용 허용", "Пропустить запросы разрешений и разрешить все вызовы инструментов")
	addCLI(KeyCLIFlagAllowedTools, "Comma-separated allowlist of tools", "以逗号分隔的工具允许列表", "Kommagetrennte Positivliste der Tools", "カンマ区切りのツール許可リスト", "쉼표로 구분한 도구 허용 목록", "Разделённый запятыми список разрешённых инструментов")
	addCLI(KeyCLIFlagDisallowedTools, "Comma-separated denylist of tools", "以逗号分隔的工具拒绝列表", "Kommagetrennte Sperrliste der Tools", "カンマ区切りのツール拒否リスト", "쉼표로 구분한 도구 차단 목록", "Разделённый запятыми список запрещённых инструментов")
	addCLI(KeyCLIFlagSandbox, "Enable OS-level sandboxing for shell commands", "为 shell 命令启用操作系统级 sandbox", "Sandboxing auf Betriebssystemebene für Shell-Befehle aktivieren", "shell コマンドに OS レベルの sandbox を有効化", "shell 명령에 OS 수준 sandbox 활성화", "Включить системную sandbox-изоляцию для команд shell")
	addCLI(KeyCLIFlagSDK, "SDK mode: read NDJSON from stdin and write NDJSON to stdout", "SDK 模式：从 stdin 读取 NDJSON，向 stdout 写入 NDJSON", "SDK-Modus: NDJSON von stdin lesen und nach stdout schreiben", "SDK モード: stdin から NDJSON を読み、stdout に NDJSON を出力", "SDK 모드: stdin에서 NDJSON을 읽고 stdout에 NDJSON 쓰기", "Режим SDK: читать NDJSON из stdin и писать NDJSON в stdout")
	addCLI(KeyCLIFlagVersion, "Print the version and exit", "输出版本后退出", "Version ausgeben und beenden", "バージョンを表示して終了", "버전을 출력하고 종료", "Вывести версию и завершить работу")
	addCLI(KeyCLIFlagVerbose, "Enable detailed logging", "启用详细日志", "Ausführliche Protokollierung aktivieren", "詳細ログを有効化", "상세 로그 활성화", "Включить подробное журналирование")
	addCLI(KeyCLIFlagDebugFile, "Write LLM prompts, message history, tool schemas, responses, and compaction calls to this file", "将 LLM prompt、消息历史、工具 schema、响应和压缩调用写入此文件", "LLM-Prompts, Nachrichtenverlauf, Tool-Schemata, Antworten und Komprimierungsaufrufe in diese Datei schreiben", "LLM prompt、メッセージ履歴、ツール schema、応答、圧縮呼び出しをこのファイルに書き込む", "LLM prompt, 메시지 기록, 도구 schema, 응답, 압축 호출을 이 파일에 기록", "Записывать в файл LLM prompt, историю сообщений, схемы инструментов, ответы и вызовы сжатия")
	addCLI(KeyCLIFlagNoColor, "Disable ANSI color output", "禁用 ANSI 彩色输出", "ANSI-Farbausgabe deaktivieren", "ANSI カラー出力を無効化", "ANSI 색상 출력 비활성화", "Отключить цветной вывод ANSI")
	addCLI(KeyCLIFlagOutputFormat, "Output format: text | json | stream-json", "输出格式：text | json | stream-json", "Ausgabeformat: text | json | stream-json", "出力形式: text | json | stream-json", "출력 형식: text | json | stream-json", "Формат вывода: text | json | stream-json")
	addCLI(KeyCLIFlagQuiet, "Output only the final assistant text; hide tools, cost, and banners", "仅输出 Assistant 的最终文本；隐藏工具、费用和横幅", "Nur den abschließenden Assistant-Text ausgeben; Tools, Kosten und Banner ausblenden", "Assistant の最終テキストのみ出力し、ツール、料金、バナーを非表示", "Assistant 최종 텍스트만 출력하고 도구, 비용, 배너 숨기기", "Выводить только итоговый текст Assistant; скрыть инструменты, стоимость и баннеры")
	addCLI(KeyCLIFlagScreenReader, "Use append-only screen-reader mode without cursor control, mouse capture, or animation", "使用仅追加的屏幕阅读器模式，不启用光标控制、鼠标捕获或动画", "Fortlaufenden Screenreader-Modus ohne Cursorsteuerung, Mauserfassung oder Animation verwenden", "カーソル制御、マウスキャプチャ、アニメーションを使わない追記型スクリーンリーダーモード", "커서 제어, 마우스 캡처, 애니메이션이 없는 추가 전용 스크린 리더 모드 사용", "Использовать добавляемый режим чтения с экрана без управления курсором, захвата мыши и анимации")
	addCLI(KeyCLIFlagAgents, "JSON object defining additional agents", "定义额外 Agent 的 JSON 对象", "JSON-Objekt zur Definition zusätzlicher Agents", "追加 Agent を定義する JSON オブジェクト", "추가 Agent를 정의하는 JSON 객체", "JSON-объект с определениями дополнительных агентов")
	addCLI(KeyCLIFlagPromptDump, "Output the rendered prompt blocks and context, then exit without an API call", "输出渲染后的 prompt 块和上下文，然后退出且不调用 API", "Gerenderte Prompt-Blöcke und Kontext ausgeben, dann ohne API-Aufruf beenden", "レンダリング済み prompt ブロックとコンテキストを出力し、API を呼ばずに終了", "렌더링된 prompt 블록과 컨텍스트를 출력하고 API 호출 없이 종료", "Вывести сформированные блоки prompt и контекст, затем завершить работу без вызова API")
	addCLI(KeyCLIFlagPromptDumpJSON, "Output the rendered prompt blocks and context as JSON, then exit without an API call", "以 JSON 输出渲染后的 prompt 块和上下文，然后退出且不调用 API", "Gerenderte Prompt-Blöcke und Kontext als JSON ausgeben, dann ohne API-Aufruf beenden", "レンダリング済み prompt ブロックとコンテキストを JSON で出力し、API を呼ばずに終了", "렌더링된 prompt 블록과 컨텍스트를 JSON으로 출력하고 API 호출 없이 종료", "Вывести сформированные блоки prompt и контекст в JSON, затем завершить работу без вызова API")
	addCLI(KeyCLIFlagLanguage, "Preferred language for assistant responses", "Assistant 回复的首选语言", "Bevorzugte Sprache für Assistant-Antworten", "Assistant の応答で優先する言語", "Assistant 응답에 사용할 기본 언어", "Предпочтительный язык ответов Assistant")
	addCLI(KeyCLIFlagOutputStyle, "Output style for assistant responses", "Assistant 回复的输出风格", "Ausgabestil für Assistant-Antworten", "Assistant の応答スタイル", "Assistant 응답 출력 스타일", "Стиль вывода ответов Assistant")
	addCLI(KeyCLIFlagAllowedDomains, "Comma-separated domains allowed for web tools; supports *.example.com", "Web 工具可访问的域名，以逗号分隔；支持 *.example.com", "Kommagetrennte, für Web-Tools erlaubte Domains; unterstützt *.example.com", "Web ツールに許可するドメイン。カンマ区切りで *.example.com に対応", "웹 도구에 허용할 도메인. 쉼표로 구분하며 *.example.com 지원", "Разделённые запятыми домены, разрешённые веб-инструментам; поддерживается *.example.com")
	addCLI(KeyCLIFlagDisallowedDomains, "Comma-separated domains blocked for web tools", "Web 工具禁止访问的域名，以逗号分隔", "Kommagetrennte, für Web-Tools gesperrte Domains", "Web ツールで拒否するドメイン。カンマ区切り", "웹 도구에서 차단할 도메인. 쉼표로 구분", "Разделённые запятыми домены, запрещённые веб-инструментам")
}

func addCLI(key Key, en, zh, de, ja, ko, ru string) {
	semanticTranslations[key] = map[Language]string{LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru}
}
