package i18n

// Semantic copy sent to models through file-tool descriptions and schemas.
// Tool and protocol identifiers remain untranslated inside the localized text.
const (
	KeyToolFileReadDescription                Key = "tool.file.read.description"
	KeyToolFileEditDescription                Key = "tool.file.edit.description"
	KeyToolFileWriteDescription               Key = "tool.file.write.description"
	KeyToolFileReadInputFilePathDescription   Key = "tool.file.read.input.file_path.description"
	KeyToolFileReadInputOffsetDescription     Key = "tool.file.read.input.offset.description"
	KeyToolFileReadInputLimitDescription      Key = "tool.file.read.input.limit.description"
	KeyToolFileReadInputPagesDescription      Key = "tool.file.read.input.pages.description"
	KeyToolFileEditInputFilePathDescription   Key = "tool.file.edit.input.file_path.description"
	KeyToolFileEditInputOldStringDescription  Key = "tool.file.edit.input.old_string.description"
	KeyToolFileEditInputNewStringDescription  Key = "tool.file.edit.input.new_string.description"
	KeyToolFileEditInputReplaceAllDescription Key = "tool.file.edit.input.replace_all.description"
	KeyToolFileWriteInputFilePathDescription  Key = "tool.file.write.input.file_path.description"
	KeyToolFileWriteInputContentDescription   Key = "tool.file.write.input.content.description"
)

var toolFilePromptKeys = [...]Key{
	KeyToolFileReadDescription, KeyToolFileEditDescription, KeyToolFileWriteDescription,
	KeyToolFileReadInputFilePathDescription, KeyToolFileReadInputOffsetDescription,
	KeyToolFileReadInputLimitDescription, KeyToolFileReadInputPagesDescription,
	KeyToolFileEditInputFilePathDescription, KeyToolFileEditInputOldStringDescription,
	KeyToolFileEditInputNewStringDescription, KeyToolFileEditInputReplaceAllDescription,
	KeyToolFileWriteInputFilePathDescription, KeyToolFileWriteInputContentDescription,
}

func init() {
	addToolFilePrompt := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}

	addToolFilePrompt(KeyToolFileReadDescription,
		"Reads a file from the local filesystem.\n\nUsage:\n- Use `file_path` for a file; directories are not supported.\n- For text, omit `offset` and `limit` to read from the beginning. For a large file or a known target, use them to read a focused range.\n- Text output prefixes each line with a 1-based line number and a tab. When using `Edit`, copy only the text after that prefix and preserve indentation exactly.\n- Images are presented visually. Use `pages` to select PDF pages. Jupyter notebooks return cells and outputs.",
		"从本地文件系统读取文件。\n\n用法：\n- 通过 `file_path` 指定文件；不支持目录。\n- 对于文本文件，省略 `offset` 和 `limit` 可从开头读取。对于大型文件或已知目标位置，请使用它们读取指定范围。\n- 文本输出会在每行前添加从 1 开始的行号和一个制表符。使用 `Edit` 时，只复制该前缀之后的文本，并严格保留缩进。\n- 图像会以可视形式呈现。使用 `pages` 选择 PDF 页面。Jupyter notebook 会返回单元格及其输出。",
		"Liest eine Datei aus dem lokalen Dateisystem.\n\nVerwendung:\n- Gib die Datei mit `file_path` an; Verzeichnisse werden nicht unterstützt.\n- Lasse bei Textdateien `offset` und `limit` weg, um am Anfang zu lesen. Verwende sie bei großen Dateien oder einem bekannten Ziel, um einen gezielten Bereich zu lesen.\n- In der Textausgabe steht vor jeder Zeile eine 1-basierte Zeilennummer und ein Tabulator. Übernimm bei `Edit` nur den Text nach diesem Präfix und behalte die Einrückung exakt bei.\n- Bilder werden visuell dargestellt. Wähle PDF-Seiten mit `pages` aus. Bei Jupyter-Notebooks werden Zellen und Ausgaben zurückgegeben.",
		"ローカルファイルシステムからファイルを読み取ります。\n\n使用方法:\n- `file_path` でファイルを指定します。ディレクトリはサポートされません。\n- テキストでは、先頭から読み取る場合は `offset` と `limit` を省略します。大きなファイルや対象箇所が分かっている場合は、これらを使って必要な範囲だけを読み取ります。\n- テキスト出力では、各行の先頭に 1 始まりの行番号とタブが付きます。`Edit` では、その接頭辞より後のテキストだけをコピーし、インデントを正確に保持してください。\n- 画像は視覚的に表示されます。PDF のページは `pages` で選択します。Jupyter notebook ではセルと出力が返されます。",
		"로컬 파일 시스템에서 파일을 읽습니다.\n\n사용법:\n- `file_path`로 파일을 지정합니다. 디렉터리는 지원하지 않습니다.\n- 텍스트는 처음부터 읽으려면 `offset`과 `limit`을 생략합니다. 큰 파일이거나 대상 위치를 알고 있으면 두 값을 사용해 필요한 범위만 읽습니다.\n- 텍스트 출력은 각 줄 앞에 1부터 시작하는 줄 번호와 탭을 붙입니다. `Edit`을 사용할 때는 이 접두사 뒤의 텍스트만 복사하고 들여쓰기를 정확히 유지하세요.\n- 이미지는 시각적으로 표시됩니다. `pages`로 PDF 페이지를 선택합니다. Jupyter notebook은 셀과 출력을 반환합니다.",
		"Читает файл из локальной файловой системы.\n\nИспользование:\n- Укажите файл в `file_path`; каталоги не поддерживаются.\n- Для текста не задавайте `offset` и `limit`, чтобы читать с начала. Для большого файла или известного участка используйте их, чтобы прочитать только нужный диапазон.\n- В текстовом выводе перед каждой строкой ставятся номер с единицы и символ табуляции. Для `Edit` копируйте только текст после этого префикса и точно сохраняйте отступы.\n- Изображения показываются визуально. Выбирайте страницы PDF через `pages`. Для Jupyter notebook возвращаются ячейки и их вывод.")

	addToolFilePrompt(KeyToolFileEditDescription,
		"Performs exact string replacements in files.\n\nUsage:\n- Read an existing file with `Read` before editing it. If it changes after that read, read it again.\n- `old_string` and `new_string` contain file text only: remove the line-number-and-tab prefix from `Read` output and preserve indentation exactly.\n- Without `replace_all`, `old_string` must identify exactly one occurrence. Add nearby context if it is not unique. Set `replace_all` to replace every occurrence.\n- Prefer `Edit` for changes to existing files; use `Write` for new files or intentional complete rewrites.",
		"在文件中执行精确字符串替换。\n\n用法：\n- 编辑现有文件前，先用 `Read` 读取它。如果文件在读取后发生变化，请重新读取。\n- `old_string` 和 `new_string` 只能包含文件文本：移除 `Read` 输出中的行号和制表符前缀，并严格保留缩进。\n- 未设置 `replace_all` 时，`old_string` 必须只匹配一处。如果不唯一，请加入相邻上下文。设置 `replace_all` 可替换所有匹配项。\n- 修改现有文件时优先使用 `Edit`；新建文件或有意完整重写时使用 `Write`。",
		"Führt exakte Zeichenfolgenersetzungen in Dateien durch.\n\nVerwendung:\n- Lies eine vorhandene Datei vor der Bearbeitung mit `Read`. Wenn sie danach geändert wird, lies sie erneut.\n- `old_string` und `new_string` enthalten nur Dateitext: Entferne das Präfix aus Zeilennummer und Tabulator der `Read`-Ausgabe und behalte die Einrückung exakt bei.\n- Ohne `replace_all` muss `old_string` genau eine Stelle kennzeichnen. Ergänze Kontext aus benachbarten Zeilen, wenn der Text nicht eindeutig ist. Setze `replace_all`, um alle Vorkommen zu ersetzen.\n- Verwende für Änderungen an vorhandenen Dateien bevorzugt `Edit`; nutze `Write` für neue Dateien oder absichtliche vollständige Neuschreibungen.",
		"ファイル内の文字列を完全一致で置換します。\n\n使用方法:\n- 既存ファイルを編集する前に `Read` で読み取ります。読み取り後にファイルが変更された場合は、もう一度読み取ってください。\n- `old_string` と `new_string` にはファイル本文だけを指定します。`Read` 出力の行番号とタブの接頭辞を除き、インデントを正確に保持してください。\n- `replace_all` を設定しない場合、`old_string` は 1 箇所だけに一致する必要があります。一意でなければ周辺の文脈を追加してください。すべての一致箇所を置換するには `replace_all` を設定します。\n- 既存ファイルの変更には `Edit` を優先し、新規ファイルや意図的な全面書き換えには `Write` を使用します。",
		"파일에서 정확히 일치하는 문자열을 바꿉니다.\n\n사용법:\n- 기존 파일을 편집하기 전에 `Read`로 읽으세요. 읽은 뒤 파일이 변경되면 다시 읽어야 합니다.\n- `old_string`과 `new_string`에는 파일 텍스트만 넣습니다. `Read` 출력의 줄 번호와 탭 접두사를 제거하고 들여쓰기를 정확히 유지하세요.\n- `replace_all`을 설정하지 않으면 `old_string`이 정확히 한 곳과 일치해야 합니다. 고유하지 않으면 주변 문맥을 추가하세요. 모든 일치 항목을 바꾸려면 `replace_all`을 설정합니다.\n- 기존 파일의 변경에는 `Edit`을 우선 사용하고, 새 파일이나 의도적인 전체 재작성에는 `Write`를 사용합니다.",
		"Выполняет точную замену строк в файлах.\n\nИспользование:\n- Перед изменением существующего файла прочитайте его через `Read`. Если после чтения файл изменился, прочитайте его снова.\n- `old_string` и `new_string` должны содержать только текст файла: уберите префикс с номером строки и табуляцией из вывода `Read` и точно сохраняйте отступы.\n- Без `replace_all` значение `old_string` должно соответствовать ровно одному месту. Если оно не уникально, добавьте соседний контекст. Задайте `replace_all`, чтобы заменить все вхождения.\n- Для изменений существующих файлов предпочитайте `Edit`; используйте `Write` для новых файлов или намеренной полной перезаписи.")

	addToolFilePrompt(KeyToolFileWriteDescription,
		"Writes a complete file to the local filesystem.\n\nUsage:\n- Creates a missing file or overwrites an existing file in full.\n- Read an existing file in full with `Read` first; omit `offset` and `limit`. If it changes after that read, read it again.\n- Prefer `Edit` for focused changes to existing files. Use `Write` for new files or intentional complete rewrites.\n- Do not create documentation files or add emojis unless the user explicitly requests them.",
		"将完整文件写入本地文件系统。\n\n用法：\n- 创建缺失的文件，或完整覆盖现有文件。\n- 覆盖现有文件前，先用 `Read` 完整读取它，并省略 `offset` 和 `limit`。如果文件在读取后发生变化，请重新读取。\n- 对现有文件进行局部修改时优先使用 `Edit`。新建文件或有意完整重写时使用 `Write`。\n- 除非用户明确要求，否则不要创建文档文件或添加 emoji。",
		"Schreibt eine vollständige Datei in das lokale Dateisystem.\n\nVerwendung:\n- Erstellt eine fehlende Datei oder überschreibt eine vorhandene Datei vollständig.\n- Lies eine vorhandene Datei zuerst vollständig mit `Read` und lasse `offset` und `limit` weg. Wenn sie danach geändert wird, lies sie erneut.\n- Verwende für gezielte Änderungen an vorhandenen Dateien bevorzugt `Edit`. Nutze `Write` für neue Dateien oder absichtliche vollständige Neuschreibungen.\n- Erstelle keine Dokumentationsdateien und füge keine Emojis hinzu, sofern der Benutzer dies nicht ausdrücklich verlangt.",
		"完全なファイルをローカルファイルシステムに書き込みます。\n\n使用方法:\n- 存在しないファイルを作成するか、既存ファイル全体を上書きします。\n- 既存ファイルは `offset` と `limit` を省略して、先に `Read` で全体を読み取ります。読み取り後に変更された場合は、もう一度読み取ってください。\n- 既存ファイルの局所的な変更には `Edit` を優先します。新規ファイルや意図的な全面書き換えには `Write` を使用します。\n- ユーザーが明示的に求めない限り、ドキュメントファイルを作成したり emoji を追加したりしないでください。",
		"완전한 파일을 로컬 파일 시스템에 씁니다.\n\n사용법:\n- 없는 파일을 만들거나 기존 파일 전체를 덮어씁니다.\n- 기존 파일은 `offset`과 `limit`을 생략하고 먼저 `Read`로 전체를 읽으세요. 읽은 뒤 변경되면 다시 읽어야 합니다.\n- 기존 파일의 일부만 변경할 때는 `Edit`을 우선 사용하세요. 새 파일이나 의도적인 전체 재작성에는 `Write`를 사용합니다.\n- 사용자가 명시적으로 요청하지 않으면 문서 파일을 만들거나 emoji를 추가하지 마세요.",
		"Записывает полный файл в локальную файловую систему.\n\nИспользование:\n- Создаёт отсутствующий файл или полностью перезаписывает существующий.\n- Сначала полностью прочитайте существующий файл через `Read`, не задавая `offset` и `limit`. Если после чтения он изменился, прочитайте его снова.\n- Для точечных изменений существующих файлов предпочитайте `Edit`. Используйте `Write` для новых файлов или намеренной полной перезаписи.\n- Не создавайте файлы документации и не добавляйте emoji, если пользователь явно этого не запросил.")

	addToolFilePrompt(KeyToolFileReadInputFilePathDescription, "Path to the file to read", "要读取的文件路径", "Pfad der zu lesenden Datei", "読み取るファイルのパス", "읽을 파일 경로", "Путь к читаемому файлу")
	addToolFilePrompt(KeyToolFileReadInputOffsetDescription, "1-based line number to start reading from", "开始读取的行号（从 1 开始）", "Zeilennummer ab 1, bei der das Lesen beginnt", "読み取り開始行（1 始まり）", "읽기를 시작할 줄 번호(1부터 시작)", "Номер строки с единицы, с которой начать чтение")
	addToolFilePrompt(KeyToolFileReadInputLimitDescription, "Maximum number of lines to read", "最多读取的行数", "Maximale Anzahl zu lesender Zeilen", "読み取る最大行数", "읽을 최대 줄 수", "Максимальное число читаемых строк")
	addToolFilePrompt(KeyToolFileReadInputPagesDescription, "Optional page selector for PDF-style sources", "PDF 类来源的可选页面选择器", "Optionale Seitenauswahl für PDF-artige Quellen", "PDF 形式のソース向けの任意ページ指定", "PDF 형식 소스의 선택적 페이지 지정", "Необязательный выбор страниц для источников формата PDF")
	addToolFilePrompt(KeyToolFileEditInputFilePathDescription, "Absolute path to the file to modify", "要修改的文件绝对路径", "Absoluter Pfad der zu ändernden Datei", "変更するファイルの絶対パス", "수정할 파일의 절대 경로", "Абсолютный путь к изменяемому файлу")
	addToolFilePrompt(KeyToolFileEditInputOldStringDescription, "Exact file text to replace, without Read line-number prefixes", "要替换的精确文件文本，不含 Read 行号前缀", "Exakter zu ersetzender Dateitext ohne Read-Zeilennummernpräfix", "置換する正確なファイル本文（Read の行番号接頭辞を除く）", "바꿀 정확한 파일 텍스트(Read 줄 번호 접두사 제외)", "Точный заменяемый текст файла без префиксов номеров строк Read")
	addToolFilePrompt(KeyToolFileEditInputNewStringDescription, "Replacement text; must differ from old_string", "替换文本；必须与 old_string 不同", "Ersatztext; muss sich von old_string unterscheiden", "置換後のテキスト。old_string と異なる必要があります", "대체 텍스트. old_string과 달라야 합니다", "Текст замены; должен отличаться от old_string")
	addToolFilePrompt(KeyToolFileEditInputReplaceAllDescription, "Replace every occurrence; defaults to false", "替换所有匹配项；默认为 false", "Alle Vorkommen ersetzen; Standardwert false", "すべての一致箇所を置換します。既定値は false", "모든 일치 항목을 바꿉니다. 기본값은 false", "Заменить все вхождения; по умолчанию false")
	addToolFilePrompt(KeyToolFileWriteInputFilePathDescription, "Absolute path to the file to write", "要写入的文件绝对路径", "Absoluter Pfad der zu schreibenden Datei", "書き込むファイルの絶対パス", "쓸 파일의 절대 경로", "Абсолютный путь к записываемому файлу")
	addToolFilePrompt(KeyToolFileWriteInputContentDescription, "Complete content to write to the file", "要写入文件的完整内容", "Vollständiger Inhalt, der in die Datei geschrieben wird", "ファイルに書き込む完全な内容", "파일에 쓸 전체 내용", "Полное содержимое для записи в файл")
}
