package i18n

// Semantic copy for first-party errors created below a ToolResult or compact
// command boundary. Runtime values such as paths, counts, and raw library or
// operating-system causes remain formatting parameters.
const (
	KeyCompactSummaryNoSummarizer        Key = "compact.summary.no_summarizer"
	KeyCompactSummaryPTLRetriesExhausted Key = "compact.summary.ptl_retries_exhausted"
	KeyCompactSummaryPTLHistoryPreserved Key = "compact.summary.ptl_history_preserved"

	KeyToolSourceSinkReadDirectory          Key = "tool.source_sink.read.directory"
	KeyToolSourceSinkParseMarshal           Key = "tool.source_sink.parse.marshal"
	KeyToolSourceSinkParseDecode            Key = "tool.source_sink.parse.decode"
	KeyToolSourceSinkConfigCreateDirectory  Key = "tool.source_sink.config.create_directory"
	KeyToolSourceSinkConfigMarshal          Key = "tool.source_sink.config.marshal"
	KeyToolSourceSinkConfigWrite            Key = "tool.source_sink.config.write"
	KeyToolSourceSinkAtomicCreateTemporary  Key = "tool.source_sink.atomic.create_temporary"
	KeyToolSourceSinkAtomicWriteTemporary   Key = "tool.source_sink.atomic.write_temporary"
	KeyToolSourceSinkAtomicSyncTemporary    Key = "tool.source_sink.atomic.sync_temporary"
	KeyToolSourceSinkAtomicCloseTemporary   Key = "tool.source_sink.atomic.close_temporary"
	KeyToolSourceSinkAtomicChmodTemporary   Key = "tool.source_sink.atomic.chmod_temporary"
	KeyToolSourceSinkAtomicReplaceTarget    Key = "tool.source_sink.atomic.replace_target"
	KeyToolSourceSinkReadPathNullBytes      Key = "tool.source_sink.read.path_null_bytes"
	KeyToolSourceSinkReadImageEmpty         Key = "tool.source_sink.read.image_empty"
	KeyToolSourceSinkReadImageTokenLimit    Key = "tool.source_sink.read.image_token_limit"
	KeyToolSourceSinkReadImagePrepare       Key = "tool.source_sink.read.image_prepare"
	KeyToolSourceSinkReadPNGInvalidSize     Key = "tool.source_sink.read.png_invalid_size"
	KeyToolSourceSinkReadPNGTooLarge        Key = "tool.source_sink.read.png_too_large"
	KeyToolSourceSinkNotebookSourceFormat   Key = "tool.source_sink.read.notebook_source_format"
	KeyToolSourceSinkNotebookParse          Key = "tool.source_sink.read.notebook_parse"
	KeyToolSourceSinkNotebookCellSource     Key = "tool.source_sink.read.notebook_cell_source"
	KeyToolSourceSinkSearchInvalidRegex     Key = "tool.source_sink.search.invalid_regex"
	KeyToolSourceSinkSearchInvalidContext   Key = "tool.source_sink.search.invalid_context"
	KeyToolSourceSinkSearchOutsideAllowed   Key = "tool.source_sink.search.outside_allowed"
	KeyToolSourceSinkMCPReadSettings        Key = "tool.source_sink.mcp.read_settings"
	KeyToolSourceSinkMCPParseSettings       Key = "tool.source_sink.mcp.parse_settings"
	KeyToolSourceSinkMCPNotConfigured       Key = "tool.source_sink.mcp.not_configured"
	KeyToolSourceSinkMCPConnect             Key = "tool.source_sink.mcp.connect"
	KeyToolSourceSinkMCPConnectTimeout      Key = "tool.source_sink.mcp.connect_timeout"
	KeyToolSourceSinkMCPListTools           Key = "tool.source_sink.mcp.list_tools"
	KeyToolSourceSinkWorktreeRuntimeMissing Key = "tool.source_sink.worktree.runtime_missing"
	KeyToolSourceSinkWorktreeCWDEmpty       Key = "tool.source_sink.worktree.cwd_empty"
	KeyToolSourceSinkWorktreeCWDUnavailable Key = "tool.source_sink.worktree.cwd_unavailable"
	KeyToolSourceSinkWorktreeCWDNotDir      Key = "tool.source_sink.worktree.cwd_not_directory"
	KeyToolSourceSinkWorktreePersistSession Key = "tool.source_sink.worktree.persist_session"
	KeyToolSourceSinkWorktreeSwitchCWD      Key = "tool.source_sink.worktree.switch_cwd"
)

var toolSourceSinkResidualKeys = [...]Key{
	KeyCompactSummaryNoSummarizer,
	KeyCompactSummaryPTLRetriesExhausted,
	KeyCompactSummaryPTLHistoryPreserved,
	KeyToolSourceSinkReadDirectory,
	KeyToolSourceSinkParseMarshal,
	KeyToolSourceSinkParseDecode,
	KeyToolSourceSinkConfigCreateDirectory,
	KeyToolSourceSinkConfigMarshal,
	KeyToolSourceSinkConfigWrite,
	KeyToolSourceSinkAtomicCreateTemporary,
	KeyToolSourceSinkAtomicWriteTemporary,
	KeyToolSourceSinkAtomicSyncTemporary,
	KeyToolSourceSinkAtomicCloseTemporary,
	KeyToolSourceSinkAtomicChmodTemporary,
	KeyToolSourceSinkAtomicReplaceTarget,
	KeyToolSourceSinkReadPathNullBytes,
	KeyToolSourceSinkReadImageEmpty,
	KeyToolSourceSinkReadImageTokenLimit,
	KeyToolSourceSinkReadImagePrepare,
	KeyToolSourceSinkReadPNGInvalidSize,
	KeyToolSourceSinkReadPNGTooLarge,
	KeyToolSourceSinkNotebookSourceFormat,
	KeyToolSourceSinkNotebookParse,
	KeyToolSourceSinkNotebookCellSource,
	KeyToolSourceSinkSearchInvalidRegex,
	KeyToolSourceSinkSearchInvalidContext,
	KeyToolSourceSinkSearchOutsideAllowed,
	KeyToolSourceSinkMCPReadSettings,
	KeyToolSourceSinkMCPParseSettings,
	KeyToolSourceSinkMCPNotConfigured,
	KeyToolSourceSinkMCPConnect,
	KeyToolSourceSinkMCPConnectTimeout,
	KeyToolSourceSinkMCPListTools,
	KeyToolSourceSinkWorktreeRuntimeMissing,
	KeyToolSourceSinkWorktreeCWDEmpty,
	KeyToolSourceSinkWorktreeCWDUnavailable,
	KeyToolSourceSinkWorktreeCWDNotDir,
	KeyToolSourceSinkWorktreePersistSession,
	KeyToolSourceSinkWorktreeSwitchCWD,
}

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en,
			LangZH: zh,
			LangDE: de,
			LangJA: ja,
			LangKO: ko,
			LangRU: ru,
		}
	}

	add(KeyCompactSummaryNoSummarizer,
		"no summarizer configured",
		"未配置 summarizer",
		"Kein Summarizer konfiguriert",
		"summarizer が設定されていません",
		"summarizer가 구성되지 않았습니다",
		"Summarizer не настроен")
	add(KeyCompactSummaryPTLRetriesExhausted,
		"compact summary prompt-too-long retry exhausted after %d attempts",
		"生成压缩摘要时，prompt-too-long 重试 %d 次后仍未成功",
		"Die prompt-too-long-Wiederholungen für die Kompaktzusammenfassung waren nach %d Versuchen ausgeschöpft",
		"圧縮要約の prompt-too-long リトライは %d 回で上限に達しました",
		"압축 요약의 prompt-too-long 재시도가 %d회 후 모두 소진되었습니다",
		"Повторные попытки prompt-too-long для краткой сводки исчерпаны после %d попыток")
	add(KeyCompactSummaryPTLHistoryPreserved,
		"compact summary input exceeds the context window; conversation history was preserved",
		"压缩摘要输入超出上下文窗口；会话历史已保留",
		"Die Eingabe für die Kompaktzusammenfassung überschreitet das Kontextfenster; der Gesprächsverlauf wurde beibehalten",
		"圧縮要約の入力がコンテキストウィンドウを超えたため、会話履歴は保持されました",
		"압축 요약 입력이 컨텍스트 창을 초과했으며 대화 기록은 보존되었습니다",
		"Входные данные для сжатого резюме превышают окно контекста; история диалога сохранена")

	add(KeyToolSourceSinkReadDirectory,
		"EISDIR: illegal operation on a directory, read %q",
		"EISDIR：无法读取目录 %q",
		"EISDIR: Verzeichnis %q kann nicht als Datei gelesen werden",
		"EISDIR: ディレクトリ %q をファイルとして読み取ることはできません",
		"EISDIR: 디렉터리 %q을(를) 파일로 읽을 수 없습니다",
		"EISDIR: каталог %q нельзя прочитать как файл")
	add(KeyToolSourceSinkParseMarshal,
		"failed to marshal input: %v",
		"序列化输入失败：%v",
		"Eingabe konnte nicht serialisiert werden: %v",
		"入力をシリアライズできませんでした: %v",
		"입력을 직렬화하지 못했습니다: %v",
		"Не удалось сериализовать входные данные: %v")
	add(KeyToolSourceSinkParseDecode,
		"failed to parse input: %v",
		"解析输入失败：%v",
		"Eingabe konnte nicht geparst werden: %v",
		"入力を解析できませんでした: %v",
		"입력을 파싱하지 못했습니다: %v",
		"Не удалось разобрать входные данные: %v")
	add(KeyToolSourceSinkConfigCreateDirectory,
		"failed to create config directory: %v",
		"无法创建 config 目录：%v",
		"Konfigurationsverzeichnis konnte nicht erstellt werden: %v",
		"config ディレクトリを作成できませんでした: %v",
		"config 디렉터리를 만들지 못했습니다: %v",
		"Не удалось создать каталог конфигурации: %v")
	add(KeyToolSourceSinkConfigMarshal,
		"failed to marshal config: %v",
		"序列化 config 失败：%v",
		"Konfiguration konnte nicht serialisiert werden: %v",
		"config をシリアライズできませんでした: %v",
		"config를 직렬화하지 못했습니다: %v",
		"Не удалось сериализовать конфигурацию: %v")
	add(KeyToolSourceSinkConfigWrite,
		"failed to write config: %v",
		"写入 config 失败：%v",
		"Konfiguration konnte nicht geschrieben werden: %v",
		"config を書き込めませんでした: %v",
		"config를 쓰지 못했습니다: %v",
		"Не удалось записать конфигурацию: %v")

	add(KeyToolSourceSinkAtomicCreateTemporary,
		"create temp file: %v",
		"创建临时文件失败：%v",
		"Temporäre Datei konnte nicht erstellt werden: %v",
		"一時ファイルを作成できませんでした: %v",
		"임시 파일을 만들지 못했습니다: %v",
		"Не удалось создать временный файл: %v")
	add(KeyToolSourceSinkAtomicWriteTemporary,
		"write temp file: %v",
		"写入临时文件失败：%v",
		"Temporäre Datei konnte nicht geschrieben werden: %v",
		"一時ファイルに書き込めませんでした: %v",
		"임시 파일에 쓰지 못했습니다: %v",
		"Не удалось записать временный файл: %v")
	add(KeyToolSourceSinkAtomicSyncTemporary,
		"fsync temp file: %v",
		"同步临时文件失败：%v",
		"Temporäre Datei konnte nicht mit fsync synchronisiert werden: %v",
		"一時ファイルを fsync できませんでした: %v",
		"임시 파일을 fsync하지 못했습니다: %v",
		"Не удалось выполнить fsync временного файла: %v")
	add(KeyToolSourceSinkAtomicCloseTemporary,
		"close temp file: %v",
		"关闭临时文件失败：%v",
		"Temporäre Datei konnte nicht geschlossen werden: %v",
		"一時ファイルを閉じられませんでした: %v",
		"임시 파일을 닫지 못했습니다: %v",
		"Не удалось закрыть временный файл: %v")
	add(KeyToolSourceSinkAtomicChmodTemporary,
		"chmod temp file: %v",
		"设置临时文件权限失败：%v",
		"Berechtigungen der temporären Datei konnten nicht mit chmod gesetzt werden: %v",
		"一時ファイルを chmod できませんでした: %v",
		"임시 파일을 chmod하지 못했습니다: %v",
		"Не удалось выполнить chmod временного файла: %v")
	add(KeyToolSourceSinkAtomicReplaceTarget,
		"rename temp to target: %v",
		"用临时文件替换目标文件失败：%v",
		"Temporäre Datei konnte nicht in die Zieldatei umbenannt werden: %v",
		"一時ファイルを対象ファイルへ置き換えられませんでした: %v",
		"임시 파일을 대상 파일로 바꾸지 못했습니다: %v",
		"Не удалось переименовать временный файл в целевой: %v")

	add(KeyToolSourceSinkReadPathNullBytes,
		"path contains null bytes",
		"路径包含 null byte",
		"Der Pfad enthält Nullbytes",
		"パスに null byte が含まれています",
		"경로에 null byte가 포함되어 있습니다",
		"Путь содержит нулевые байты")
	add(KeyToolSourceSinkReadImageEmpty,
		"Image file is empty: %s",
		"图片文件为空：%s",
		"Die Bilddatei ist leer: %s",
		"画像ファイルが空です: %s",
		"이미지 파일이 비어 있습니다: %s",
		"Файл изображения пуст: %s")
	add(KeyToolSourceSinkReadImageTokenLimit,
		"Image content (%d tokens) exceeds maximum allowed tokens (%d).",
		"图片内容（%d 个 token）超过允许的 token 上限（%d）。",
		"Der Bildinhalt (%d Token) überschreitet die maximal zulässige Token-Zahl (%d).",
		"画像の内容（%d token）が許容上限（%d）を超えています。",
		"이미지 내용(%d token)이 허용된 token 상한(%d)을 초과합니다.",
		"Содержимое изображения (%d token) превышает допустимый предел (%d).")
	add(KeyToolSourceSinkReadImagePrepare,
		"failed to prepare image data",
		"无法准备图片数据",
		"Bilddaten konnten nicht vorbereitet werden",
		"画像データを準備できませんでした",
		"이미지 데이터를 준비하지 못했습니다",
		"Не удалось подготовить данные изображения")
	add(KeyToolSourceSinkReadPNGInvalidSize,
		"PNG has invalid dimensions: %dx%d",
		"PNG 尺寸无效：%dx%d",
		"PNG hat ungültige Abmessungen: %dx%d",
		"PNG の寸法が無効です: %dx%d",
		"PNG 크기가 올바르지 않습니다: %dx%d",
		"Недопустимые размеры PNG: %dx%d")
	add(KeyToolSourceSinkReadPNGTooLarge,
		"PNG dimensions %dx%d exceed the maximum allowed (%d). Refusing to decode.",
		"PNG 尺寸 %dx%d 超过允许的上限（%d），已拒绝解码。",
		"Die PNG-Abmessungen %dx%d überschreiten das zulässige Maximum (%d). Die Dekodierung wird abgelehnt.",
		"PNG の寸法 %dx%d は上限（%d）を超えているため、デコードしません。",
		"PNG 크기 %dx%d이(가) 허용된 상한(%d)을 초과하여 디코딩을 거부했습니다.",
		"Размеры PNG %dx%d превышают допустимый максимум (%d). Декодирование отклонено.")
	add(KeyToolSourceSinkNotebookSourceFormat,
		"unsupported notebook source format",
		"不支持的 Notebook source 格式",
		"Nicht unterstütztes Notebook-Quellformat",
		"サポートされていない Notebook source 形式です",
		"지원되지 않는 Notebook source 형식입니다",
		"Неподдерживаемый формат исходного содержимого Notebook")
	add(KeyToolSourceSinkNotebookParse,
		"failed to parse notebook: %v",
		"解析 Notebook 失败：%v",
		"Notebook konnte nicht geparst werden: %v",
		"Notebook を解析できませんでした: %v",
		"Notebook을 파싱하지 못했습니다: %v",
		"Не удалось разобрать Notebook: %v")
	add(KeyToolSourceSinkNotebookCellSource,
		"failed to parse notebook cell source: %v",
		"解析 Notebook cell source 失败：%v",
		"Quelltext der Notebook-Zelle konnte nicht geparst werden: %v",
		"Notebook cell source を解析できませんでした: %v",
		"Notebook cell source를 파싱하지 못했습니다: %v",
		"Не удалось разобрать исходное содержимое ячейки Notebook: %v")

	add(KeyToolSourceSinkSearchInvalidRegex,
		"invalid regex pattern: %v",
		"无效的 regex pattern：%v",
		"Ungültiges Regex-Muster: %v",
		"regex pattern が無効です: %v",
		"regex pattern이 올바르지 않습니다: %v",
		"Недопустимый шаблон regex: %v")
	add(KeyToolSourceSinkSearchInvalidContext,
		"ripgrep usage error: context values must be non-negative integers",
		"ripgrep 用法错误：context 值必须是非负整数",
		"Ripgrep-Nutzungsfehler: Kontextwerte müssen nicht negative Ganzzahlen sein",
		"ripgrep の使用方法が正しくありません: context 値は 0 以上の整数で指定してください",
		"ripgrep 사용 오류: context 값은 0 이상의 정수여야 합니다",
		"Ошибка использования ripgrep: значения context должны быть неотрицательными целыми числами")
	add(KeyToolSourceSinkSearchOutsideAllowed,
		"path is outside allowed directories: %s",
		"路径不在允许的目录中：%s",
		"Der Pfad liegt außerhalb der zulässigen Verzeichnisse: %s",
		"パスは許可されたディレクトリの外にあります: %s",
		"경로가 허용된 디렉터리 밖에 있습니다: %s",
		"Путь находится вне разрешённых каталогов: %s")

	add(KeyToolSourceSinkMCPReadSettings,
		"read MCP settings %s: %v",
		"读取 MCP 设置 %s 失败：%v",
		"MCP-Einstellungen %s konnten nicht gelesen werden: %v",
		"MCP 設定 %s を読み込めませんでした: %v",
		"MCP 설정 %s을(를) 읽지 못했습니다: %v",
		"Не удалось прочитать настройки MCP %s: %v")
	add(KeyToolSourceSinkMCPParseSettings,
		"parse MCP settings %s: %v",
		"解析 MCP 设置 %s 失败：%v",
		"MCP-Einstellungen %s konnten nicht verarbeitet werden: %v",
		"MCP 設定 %s を解析できませんでした: %v",
		"MCP 설정 %s을(를) 파싱하지 못했습니다: %v",
		"Не удалось разобрать настройки MCP %s: %v")
	add(KeyToolSourceSinkMCPNotConfigured,
		"MCP server %q not configured",
		"尚未配置 MCP server %q",
		"MCP-Server %q ist nicht konfiguriert",
		"MCP server %q は設定されていません",
		"MCP server %q이(가) 구성되지 않았습니다",
		"MCP server %q не настроен")
	add(KeyToolSourceSinkMCPConnect,
		"connect MCP server %q: %v",
		"连接 MCP server %q 失败：%v",
		"Verbindung zum MCP-Server %q fehlgeschlagen: %v",
		"MCP server %q に接続できませんでした: %v",
		"MCP server %q에 연결하지 못했습니다: %v",
		"Не удалось подключиться к MCP server %q: %v")
	add(KeyToolSourceSinkMCPConnectTimeout,
		"connect MCP server %q: timed out after 30s",
		"连接 MCP server %q 超时（30 秒）",
		"Zeitüberschreitung nach 30 s beim Verbinden mit MCP-Server %q",
		"MCP server %q への接続が 30 秒でタイムアウトしました",
		"MCP server %q 연결이 30초 후 시간 초과되었습니다",
		"Истекло время подключения к MCP server %q (30 с)")
	add(KeyToolSourceSinkMCPListTools,
		"list tools from MCP server %q: %v",
		"列出 MCP server %q 的 Tool 失败：%v",
		"Tools des MCP-Servers %q konnten nicht aufgelistet werden: %v",
		"MCP server %q の Tool 一覧を取得できませんでした: %v",
		"MCP server %q의 Tool 목록을 가져오지 못했습니다: %v",
		"Не удалось получить список Tool с MCP server %q: %v")

	add(KeyToolSourceSinkWorktreeRuntimeMissing,
		"worktree runtime is not configured",
		"尚未配置 worktree runtime",
		"Die Worktree-Laufzeit ist nicht konfiguriert",
		"worktree runtime が設定されていません",
		"worktree runtime이 구성되지 않았습니다",
		"Среда выполнения worktree не настроена")
	add(KeyToolSourceSinkWorktreeCWDEmpty,
		"worktree cwd is empty",
		"worktree cwd 为空",
		"Das worktree cwd ist leer",
		"worktree cwd が空です",
		"worktree cwd가 비어 있습니다",
		"Значение cwd для worktree пусто")
	add(KeyToolSourceSinkWorktreeCWDUnavailable,
		"worktree cwd %q is unavailable: %v",
		"worktree cwd %q 不可用：%v",
		"Das worktree cwd %q ist nicht verfügbar: %v",
		"worktree cwd %q を利用できません: %v",
		"worktree cwd %q을(를) 사용할 수 없습니다: %v",
		"cwd %q для worktree недоступен: %v")
	add(KeyToolSourceSinkWorktreeCWDNotDir,
		"worktree cwd %q is not a directory",
		"worktree cwd %q 不是目录",
		"Das worktree cwd %q ist kein Verzeichnis",
		"worktree cwd %q はディレクトリではありません",
		"worktree cwd %q은(는) 디렉터리가 아닙니다",
		"cwd %q для worktree не является каталогом")
	add(KeyToolSourceSinkWorktreePersistSession,
		"persist worktree session: %v",
		"保存 worktree session 失败：%v",
		"Worktree-Sitzung konnte nicht gespeichert werden: %v",
		"worktree session を保存できませんでした: %v",
		"worktree session을 저장하지 못했습니다: %v",
		"Не удалось сохранить session worktree: %v")
	add(KeyToolSourceSinkWorktreeSwitchCWD,
		"switch session cwd: %v",
		"切换 session cwd 失败：%v",
		"Das cwd der Sitzung konnte nicht gewechselt werden: %v",
		"session cwd を切り替えられませんでした: %v",
		"session cwd를 전환하지 못했습니다: %v",
		"Не удалось переключить cwd session: %v")
}
