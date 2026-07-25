package i18n

// Semantic copy sent to models through repository-search tool descriptions
// and schemas. Tool names, field names, commands and protocol identifiers stay
// untranslated inside localized text.
const (
	KeyToolSearchGlobDescription                         Key = "tool.search.glob.description"
	KeyToolSearchGlobInputPatternDescription             Key = "tool.search.glob.input.pattern.description"
	KeyToolSearchGlobInputPathDescription                Key = "tool.search.glob.input.path.description"
	KeyToolSearchGrepDescription                         Key = "tool.search.grep.description"
	KeyToolSearchGrepInputPatternDescription             Key = "tool.search.grep.input.pattern.description"
	KeyToolSearchGrepInputPathDescription                Key = "tool.search.grep.input.path.description"
	KeyToolSearchGrepInputGlobDescription                Key = "tool.search.grep.input.glob.description"
	KeyToolSearchGrepInputOutputModeDescription          Key = "tool.search.grep.input.output_mode.description"
	KeyToolSearchGrepInputBeforeDescription              Key = "tool.search.grep.input.before.description"
	KeyToolSearchGrepInputAfterDescription               Key = "tool.search.grep.input.after.description"
	KeyToolSearchGrepInputCaseInsensitiveDescription     Key = "tool.search.grep.input.case_insensitive.description"
	KeyToolSearchGrepInputContextDescription             Key = "tool.search.grep.input.context.description"
	KeyToolSearchGrepInputLineNumbersDescription         Key = "tool.search.grep.input.line_numbers.description"
	KeyToolSearchGrepInputTypeDescription                Key = "tool.search.grep.input.type.description"
	KeyToolSearchGrepInputHeadLimitDescription           Key = "tool.search.grep.input.head_limit.description"
	KeyToolSearchGrepInputOffsetDescription              Key = "tool.search.grep.input.offset.description"
	KeyToolSearchGrepInputMultilineDescription           Key = "tool.search.grep.input.multiline.description"
	KeyToolSearchCatalogDescription                      Key = "tool.search.catalog.description"
	KeyToolSearchCatalogInputQueryDescription            Key = "tool.search.catalog.input.query.description"
	KeyToolSearchCatalogInputMaxResultsDescription       Key = "tool.search.catalog.input.max_results.description"
)

var toolSearchPromptKeys = [...]Key{
	KeyToolSearchGlobDescription,
	KeyToolSearchGlobInputPatternDescription,
	KeyToolSearchGlobInputPathDescription,
	KeyToolSearchGrepDescription,
	KeyToolSearchGrepInputPatternDescription,
	KeyToolSearchGrepInputPathDescription,
	KeyToolSearchGrepInputGlobDescription,
	KeyToolSearchGrepInputOutputModeDescription,
	KeyToolSearchGrepInputBeforeDescription,
	KeyToolSearchGrepInputAfterDescription,
	KeyToolSearchGrepInputCaseInsensitiveDescription,
	KeyToolSearchGrepInputContextDescription,
	KeyToolSearchGrepInputLineNumbersDescription,
	KeyToolSearchGrepInputTypeDescription,
	KeyToolSearchGrepInputHeadLimitDescription,
	KeyToolSearchGrepInputOffsetDescription,
	KeyToolSearchGrepInputMultilineDescription,
	KeyToolSearchCatalogDescription,
	KeyToolSearchCatalogInputQueryDescription,
	KeyToolSearchCatalogInputMaxResultsDescription,
}

func init() {
	addSearchPrompt := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}

	addSearchPrompt(KeyToolSearchGlobDescription,
		"Fast file pattern matching. Supports glob patterns such as \"**/*.go\" and \"src/**/*.ts\".",
		"快速匹配文件路径。支持 \"**/*.go\"、\"src/**/*.ts\" 等 glob 模式。",
		"Schnelle Dateisuche nach Mustern. Unterstützt Glob-Muster wie \"**/*.go\" und \"src/**/*.ts\".",
		"ファイルパスを高速に照合します。\"**/*.go\" や \"src/**/*.ts\" などの glob パターンを使用できます。",
		"파일 경로를 빠르게 일치시킵니다. \"**/*.go\", \"src/**/*.ts\" 같은 glob 패턴을 지원합니다.",
		"Быстро сопоставляет пути файлов. Поддерживает glob-шаблоны, например \"**/*.go\" и \"src/**/*.ts\".")
	addSearchPrompt(KeyToolSearchGlobInputPatternDescription,
		"Glob pattern used to match files", "用于匹配文件的 glob 模式", "Glob-Muster zum Abgleichen von Dateien", "ファイル照合に使う glob パターン", "파일 일치에 사용할 glob 패턴", "Glob-шаблон для сопоставления файлов")
	addSearchPrompt(KeyToolSearchGlobInputPathDescription,
		"Directory to search; defaults to the current directory", "要搜索的目录；默认为当前目录", "Zu durchsuchendes Verzeichnis; standardmäßig das aktuelle Verzeichnis", "検索するディレクトリ。既定値は現在のディレクトリ", "검색할 디렉터리. 기본값은 현재 디렉터리", "Каталог поиска; по умолчанию текущий каталог")

	addSearchPrompt(KeyToolSearchGrepDescription,
		"Searches file contents with ripgrep-compatible regular expressions.\n\nUsage:\n- Always use Grep for repository content searches; do not invoke grep or rg through Bash.\n- Filter files with glob or type.\n- output_mode accepts content, files_with_matches, or count.\n- Use Agent for open-ended searches that need several rounds.\n- Escape literal braces for ripgrep patterns.\n- Set multiline to true only for patterns that span lines.",
		"使用兼容 ripgrep 的正则表达式搜索文件内容。\n\n用法：\n- 搜索仓库内容时始终使用 Grep；不要通过 Bash 调用 grep 或 rg。\n- 使用 glob 或 type 筛选文件。\n- output_mode 可取 content、files_with_matches 或 count。\n- 需要多轮探索的开放式搜索请使用 Agent。\n- ripgrep 模式中的字面大括号需要转义。\n- 仅当模式跨行时才将 multiline 设为 true。",
		"Durchsucht Dateiinhalte mit ripgrep-kompatiblen regulären Ausdrücken.\n\nVerwendung:\n- Verwende für Inhaltssuchen immer Grep; rufe grep oder rg nicht über Bash auf.\n- Filtere Dateien mit glob oder type.\n- output_mode akzeptiert content, files_with_matches oder count.\n- Verwende Agent für offene Suchen über mehrere Schritte.\n- Literale Klammern in ripgrep-Mustern müssen maskiert werden.\n- Setze multiline nur für mehrzeilige Muster auf true.",
		"ripgrep 互換の正規表現でファイル内容を検索します。\n\n使用方法:\n- リポジトリ内検索には必ず Grep を使い、Bash から grep や rg を実行しないでください。\n- glob または type でファイルを絞り込みます。\n- output_mode は content、files_with_matches、count を指定できます。\n- 複数回の探索が必要な検索には Agent を使います。\n- ripgrep パターンのリテラル波括弧はエスケープします。\n- 行をまたぐパターンだけ multiline を true にします。",
		"ripgrep 호환 정규식으로 파일 내용을 검색합니다.\n\n사용법:\n- 저장소 내용 검색에는 항상 Grep을 사용하고 Bash에서 grep이나 rg를 실행하지 마세요.\n- glob 또는 type으로 파일을 필터링합니다.\n- output_mode는 content, files_with_matches, count를 지원합니다.\n- 여러 단계가 필요한 탐색에는 Agent를 사용합니다.\n- ripgrep 패턴의 리터럴 중괄호는 이스케이프합니다.\n- 줄을 넘는 패턴에만 multiline을 true로 설정합니다.",
		"Ищет по содержимому файлов с регулярными выражениями, совместимыми с ripgrep.\n\nИспользование:\n- Для поиска по репозиторию всегда используйте Grep; не запускайте grep или rg через Bash.\n- Фильтруйте файлы через glob или type.\n- output_mode принимает content, files_with_matches или count.\n- Для многошагового исследования используйте Agent.\n- Экранируйте литеральные фигурные скобки в шаблонах ripgrep.\n- Включайте multiline только для многострочных шаблонов.")
	addSearchPrompt(KeyToolSearchGrepInputPatternDescription,
		"Regular expression to search for", "要搜索的正则表达式", "Zu suchender regulärer Ausdruck", "検索する正規表現", "검색할 정규식", "Регулярное выражение для поиска")
	addSearchPrompt(KeyToolSearchGrepInputPathDescription,
		"File or directory to search; defaults to the current directory", "要搜索的文件或目录；默认为当前目录", "Zu durchsuchende Datei oder Verzeichnis; standardmäßig das aktuelle Verzeichnis", "検索するファイルまたはディレクトリ。既定値は現在のディレクトリ", "검색할 파일 또는 디렉터리. 기본값은 현재 디렉터리", "Файл или каталог поиска; по умолчанию текущий каталог")
	addSearchPrompt(KeyToolSearchGrepInputGlobDescription,
		"Glob pattern used to filter files, for example \"*.go\"", "用于筛选文件的 glob 模式，例如 \"*.go\"", "Glob-Muster zum Filtern von Dateien, zum Beispiel \"*.go\"", "ファイルを絞り込む glob パターン（例: \"*.go\"）", "파일 필터용 glob 패턴(예: \"*.go\")", "Glob-шаблон для фильтрации файлов, например \"*.go\"")
	addSearchPrompt(KeyToolSearchGrepInputOutputModeDescription,
		"Output mode: content, files_with_matches, or count", "输出模式：content、files_with_matches 或 count", "Ausgabemodus: content, files_with_matches oder count", "出力モード: content、files_with_matches、count", "출력 모드: content, files_with_matches 또는 count", "Режим вывода: content, files_with_matches или count")
	addSearchPrompt(KeyToolSearchGrepInputBeforeDescription,
		"Number of lines before each match (rg -B)", "每个匹配项之前显示的行数（rg -B）", "Anzahl der Zeilen vor jedem Treffer (rg -B)", "各一致箇所の前に表示する行数（rg -B）", "각 일치 항목 앞에 표시할 줄 수(rg -B)", "Число строк перед каждым совпадением (rg -B)")
	addSearchPrompt(KeyToolSearchGrepInputAfterDescription,
		"Number of lines after each match (rg -A)", "每个匹配项之后显示的行数（rg -A）", "Anzahl der Zeilen nach jedem Treffer (rg -A)", "各一致箇所の後に表示する行数（rg -A）", "각 일치 항목 뒤에 표시할 줄 수(rg -A)", "Число строк после каждого совпадения (rg -A)")
	addSearchPrompt(KeyToolSearchGrepInputCaseInsensitiveDescription,
		"Search without case sensitivity", "搜索时忽略大小写", "Groß- und Kleinschreibung ignorieren", "大文字と小文字を区別せず検索", "대소문자를 구분하지 않고 검색", "Искать без учёта регистра")
	addSearchPrompt(KeyToolSearchGrepInputContextDescription,
		"Number of context lines before and after each match (rg -C)", "每个匹配项前后显示的上下文行数（rg -C）", "Anzahl der Kontextzeilen vor und nach jedem Treffer (rg -C)", "各一致箇所の前後に表示する文脈行数（rg -C）", "각 일치 항목 앞뒤에 표시할 문맥 줄 수(rg -C)", "Число контекстных строк до и после каждого совпадения (rg -C)")
	addSearchPrompt(KeyToolSearchGrepInputLineNumbersDescription,
		"Show line numbers in output (rg -n)", "在输出中显示行号（rg -n）", "Zeilennummern in der Ausgabe anzeigen (rg -n)", "出力に行番号を表示（rg -n）", "출력에 줄 번호 표시(rg -n)", "Показывать номера строк в выводе (rg -n)")
	addSearchPrompt(KeyToolSearchGrepInputTypeDescription,
		"File type to search (rg --type)", "要搜索的文件类型（rg --type）", "Zu durchsuchender Dateityp (rg --type)", "検索するファイル種別（rg --type）", "검색할 파일 형식(rg --type)", "Тип файлов для поиска (rg --type)")
	addSearchPrompt(KeyToolSearchGrepInputHeadLimitDescription,
		"Maximum number of entries to return; zero means unlimited", "最多返回的条目数；0 表示不限制", "Maximale Anzahl zurückzugebender Einträge; null bedeutet unbegrenzt", "返す最大件数。0 は無制限", "반환할 최대 항목 수. 0은 제한 없음", "Максимальное число результатов; ноль означает без ограничения")
	addSearchPrompt(KeyToolSearchGrepInputOffsetDescription,
		"Number of entries to skip before applying head_limit", "应用 head_limit 前跳过的条目数", "Anzahl der vor head_limit zu überspringenden Einträge", "head_limit を適用する前に読み飛ばす件数", "head_limit 적용 전에 건너뛸 항목 수", "Число результатов, пропускаемых перед применением head_limit")
	addSearchPrompt(KeyToolSearchGrepInputMultilineDescription,
		"Enable matching patterns that span lines", "启用跨行模式匹配", "Muster über mehrere Zeilen hinweg abgleichen", "行をまたぐパターン照合を有効化", "여러 줄에 걸친 패턴 일치 활성화", "Включить сопоставление шаблонов через несколько строк")

	addSearchPrompt(KeyToolSearchCatalogDescription,
		"Finds deferred tools and loads their full schemas. Use select:<tool_name> for direct selection or keywords for BM25-ranked discovery. Results include match snippets and scores.",
		"查找延迟加载的工具并载入其完整 schema。使用 select:<tool_name> 直接选择，或用关键词进行 BM25 排序发现。结果包含匹配片段和得分。",
		"Findet zurückgestellte Tools und lädt ihre vollständigen Schemas. Verwende select:<tool_name> zur direkten Auswahl oder Schlüsselwörter für eine BM25-Suche. Ergebnisse enthalten Treffertexte und Bewertungen.",
		"遅延ツールを検索し、完全な schema を読み込みます。直接選択には select:<tool_name>、BM25 順位検索にはキーワードを使います。結果には一致箇所とスコアが含まれます。",
		"지연 도구를 찾아 전체 schema를 불러옵니다. 직접 선택은 select:<tool_name>, BM25 순위 검색은 키워드를 사용합니다. 결과에는 일치 문맥과 점수가 포함됩니다.",
		"Находит отложенные инструменты и загружает их полные schema. Для прямого выбора используйте select:<tool_name>, для ранжирования BM25 — ключевые слова. Результаты содержат фрагменты и оценки.")
	addSearchPrompt(KeyToolSearchCatalogInputQueryDescription,
		"Deferred-tool query: select:<tool_name> or BM25 search keywords", "延迟工具查询：select:<tool_name> 或 BM25 搜索关键词", "Abfrage für zurückgestellte Tools: select:<tool_name> oder BM25-Schlüsselwörter", "遅延ツールの検索: select:<tool_name> または BM25 検索キーワード", "지연 도구 쿼리: select:<tool_name> 또는 BM25 검색 키워드", "Запрос отложенных инструментов: select:<tool_name> или ключевые слова BM25")
	addSearchPrompt(KeyToolSearchCatalogInputMaxResultsDescription,
		"Maximum results to return, from 1 through 50; defaults to 5", "最多返回 1 到 50 条结果；默认为 5", "Maximale Ergebniszahl von 1 bis 50; Standardwert 5", "返す最大件数（1〜50）。既定値は 5", "반환할 최대 결과 수(1~50). 기본값은 5", "Максимальное число результатов от 1 до 50; по умолчанию 5")
}
