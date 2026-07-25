package i18n

const (
	KeyToolFileInvalidInput          Key = "tool.file.invalid_input"
	KeyToolFileOffsetNonNegative     Key = "tool.file.offset_non_negative"
	KeyToolFileLimitNonNegative      Key = "tool.file.limit_non_negative"
	KeyToolFileLimitPositive         Key = "tool.file.limit_positive"
	KeyToolFilePagesInvalid          Key = "tool.file.pages_invalid"
	KeyToolFilePageRangeTooLarge     Key = "tool.file.page_range_too_large"
	KeyToolFileDirectoryDenied       Key = "tool.file.directory_denied"
	KeyToolFileUNCRequiresPermission Key = "tool.file.unc_requires_permission"
	KeyToolFileDeviceBlocked         Key = "tool.file.device_blocked"
	KeyToolFileBinaryUnsupported     Key = "tool.file.binary_unsupported"
	KeyToolFileOpenFailed            Key = "tool.file.open_failed"
	KeyToolFilePathVerification      Key = "tool.file.path_verification_failed"
	KeyToolFileReadFailed            Key = "tool.file.read_failed"
	KeyToolFileTooLarge              Key = "tool.file.too_large"
	KeyToolFileNotFound              Key = "tool.file.not_found"
	KeyToolFileNotFoundInCWD         Key = "tool.file.not_found_in_cwd"
	KeyToolFileNotFoundSuggestion    Key = "tool.file.not_found_suggestion"
	KeyToolFilePlanModeBlocked       Key = "tool.file.plan_mode_blocked"
	KeyToolFilePathRequired          Key = "tool.file.path_required"
	KeyToolFileContentRequired       Key = "tool.file.content_required"
	KeyToolFileResolveFailed         Key = "tool.file.resolve_failed"
	KeyToolFileTeamMemorySecret      Key = "tool.file.team_memory_secret"
	KeyToolFileCreateDirectoryFailed Key = "tool.file.create_directory_failed"
	KeyToolFileNotReadForWrite       Key = "tool.file.not_read_for_write"
	KeyToolFilePartiallyReadForWrite Key = "tool.file.partially_read_for_write"
	KeyToolFileChangedForWrite       Key = "tool.file.changed_for_write"
	KeyToolFileWriteFailed           Key = "tool.file.write_failed"
	KeyToolFileReadInvalidTyped      Key = "tool.file.read_invalid_typed"
	KeyToolFilePDFRead               Key = "tool.file.pdf_read"
	KeyToolFilePDFPagesExtracted     Key = "tool.file.pdf_pages_extracted"
)

var toolFileRuntimeKeys = [...]Key{
	KeyToolFileInvalidInput,
	KeyToolFileOffsetNonNegative,
	KeyToolFileLimitNonNegative,
	KeyToolFileLimitPositive,
	KeyToolFilePagesInvalid,
	KeyToolFilePageRangeTooLarge,
	KeyToolFileDirectoryDenied,
	KeyToolFileUNCRequiresPermission,
	KeyToolFileDeviceBlocked,
	KeyToolFileBinaryUnsupported,
	KeyToolFileOpenFailed,
	KeyToolFilePathVerification,
	KeyToolFileReadFailed,
	KeyToolFileTooLarge,
	KeyToolFileNotFound,
	KeyToolFileNotFoundInCWD,
	KeyToolFileNotFoundSuggestion,
	KeyToolFilePlanModeBlocked,
	KeyToolFilePathRequired,
	KeyToolFileContentRequired,
	KeyToolFileResolveFailed,
	KeyToolFileTeamMemorySecret,
	KeyToolFileCreateDirectoryFailed,
	KeyToolFileNotReadForWrite,
	KeyToolFilePartiallyReadForWrite,
	KeyToolFileChangedForWrite,
	KeyToolFileWriteFailed,
	KeyToolFileReadInvalidTyped,
	KeyToolFilePDFRead,
	KeyToolFilePDFPagesExtracted,
}

func init() {
	addToolFileRuntime(KeyToolFileInvalidInput, "invalid input: %v", "输入无效：%v", "Ungültige Eingabe: %v", "入力が無効です: %v", "잘못된 입력: %v", "Недопустимые входные данные: %v")
	addToolFileRuntime(KeyToolFileOffsetNonNegative, "'offset' must be a non-negative integer", "offset 必须是非负整数", "offset muss eine nicht negative ganze Zahl sein", "offset は 0 以上の整数である必要があります", "offset은 0 이상의 정수여야 합니다", "offset должен быть неотрицательным целым числом")
	addToolFileRuntime(KeyToolFileLimitNonNegative, "'limit' must be a non-negative integer", "limit 必须是非负整数", "limit muss eine nicht negative ganze Zahl sein", "limit は 0 以上の整数である必要があります", "limit은 0 이상의 정수여야 합니다", "limit должен быть неотрицательным целым числом")
	addToolFileRuntime(KeyToolFileLimitPositive, "'limit' must be a positive integer", "limit 必须是正整数", "limit muss eine positive ganze Zahl sein", "limit は正の整数である必要があります", "limit은 양의 정수여야 합니다", "limit должен быть положительным целым числом")
	addToolFileRuntime(KeyToolFilePagesInvalid, `Invalid pages parameter: %q. Use formats like "1-5", "3", or "10-20". Pages are 1-indexed.`, `pages 参数无效：%q。请使用 "1-5"、"3" 或 "10-20" 等格式。页码从 1 开始。`, `Ungültiger pages-Parameter: %q. Formate wie "1-5", "3" oder "10-20" verwenden. Seiten werden ab 1 gezählt.`, `pages パラメーターが無効です: %q。"1-5"、"3"、"10-20" のような形式を使用してください。ページ番号は 1 から始まります。`, `잘못된 pages 매개변수: %q. "1-5", "3", "10-20" 같은 형식을 사용하세요. 페이지 번호는 1부터 시작합니다.`, `Недопустимый параметр pages: %q. Используйте формат "1-5", "3" или "10-20". Нумерация страниц начинается с 1.`)
	addToolFileRuntime(KeyToolFilePageRangeTooLarge, "Page range %q exceeds maximum of %d pages per request. Please use a smaller range.", "页码范围 %q 超过单次请求最多 %d 页的限制。请使用更小的范围。", "Der Seitenbereich %q überschreitet das Maximum von %d Seiten pro Anfrage. Einen kleineren Bereich verwenden.", "ページ範囲 %q は 1 回のリクエストあたり最大 %d ページを超えています。範囲を小さくしてください。", "페이지 범위 %q이(가) 요청당 최대 %d페이지를 초과합니다. 더 작은 범위를 사용하세요.", "Диапазон страниц %q превышает максимум в %d страниц на запрос. Укажите меньший диапазон.")
	addToolFileRuntime(KeyToolFileDirectoryDenied, "File is in a directory that is denied by your permission settings.", "文件位于权限设置禁止访问的目录中。", "Die Datei befindet sich in einem durch die Berechtigungseinstellungen gesperrten Verzeichnis.", "ファイルは権限設定で拒否されているディレクトリ内にあります。", "파일이 권한 설정에서 거부된 디렉터리에 있습니다.", "Файл находится в каталоге, запрещённом настройками разрешений.")
	addToolFileRuntime(KeyToolFileUNCRequiresPermission, "Permission is required before reading UNC path %s.", "读取 UNC 路径 %s 前需要获得权限。", "Vor dem Lesen des UNC-Pfads %s ist eine Berechtigung erforderlich.", "UNC パス %s を読み込む前に権限が必要です。", "UNC 경로 %s을(를) 읽기 전에 권한이 필요합니다.", "Перед чтением UNC-пути %s требуется разрешение.")
	addToolFileRuntime(KeyToolFileDeviceBlocked, "Cannot read '%s': this device file would block or produce infinite output.", "无法读取“%s”：该设备文件会导致阻塞或产生无限输出。", "'%s' kann nicht gelesen werden: Diese Gerätedatei würde blockieren oder eine endlose Ausgabe erzeugen.", "「%s」を読み込めません。このデバイスファイルはブロックするか、無限に出力します。", "'%s'을(를) 읽을 수 없습니다. 이 장치 파일은 차단되거나 무한 출력을 생성합니다.", "Невозможно прочитать '%s': этот файл устройства заблокирует чтение или создаст бесконечный вывод.")
	addToolFileRuntime(KeyToolFileBinaryUnsupported, "This tool cannot read binary files. The file appears to be a binary %s file. Please use appropriate tools for binary file analysis.", "此工具无法读取二进制文件。该文件似乎是 %s 二进制文件，请使用适合分析二进制文件的工具。", "Dieses Tool kann keine Binärdateien lesen. Die Datei scheint eine binäre %s-Datei zu sein. Ein geeignetes Tool zur Binäranalyse verwenden.", "このツールはバイナリファイルを読み込めません。このファイルは %s バイナリファイルのようです。バイナリ解析に適したツールを使用してください。", "이 도구는 바이너리 파일을 읽을 수 없습니다. 파일이 바이너리 %s 파일로 보입니다. 바이너리 분석에 적합한 도구를 사용하세요.", "Этот инструмент не читает двоичные файлы. Похоже, это двоичный файл %s. Используйте подходящие инструменты для анализа двоичных файлов.")
	addToolFileRuntime(KeyToolFileOpenFailed, "failed to open file: %v", "无法打开文件：%v", "Datei konnte nicht geöffnet werden: %v", "ファイルを開けませんでした: %v", "파일을 열지 못했습니다: %v", "Не удалось открыть файл: %v")
	addToolFileRuntime(KeyToolFilePathVerification, "path verification failed: %v", "路径验证失败：%v", "Pfadprüfung fehlgeschlagen: %v", "パスの検証に失敗しました: %v", "경로 검증에 실패했습니다: %v", "Не удалось проверить путь: %v")
	addToolFileRuntime(KeyToolFileReadFailed, "failed to read file: %v", "无法读取文件：%v", "Datei konnte nicht gelesen werden: %v", "ファイルを読み込めませんでした: %v", "파일을 읽지 못했습니다: %v", "Не удалось прочитать файл: %v")
	addToolFileRuntime(KeyToolFileTooLarge, "File content (%s) exceeds maximum allowed size (%s). Use offset and limit parameters to read specific portions of the file, or search for specific content instead of reading the whole file.", "文件内容（%s）超过允许的最大大小（%s）。请使用 offset 和 limit 参数读取指定部分，或搜索特定内容，不要读取整个文件。", "Der Dateiinhalt (%s) überschreitet die maximal zulässige Größe (%s). Mit offset und limit bestimmte Abschnitte lesen oder nach konkretem Inhalt suchen, statt die ganze Datei zu lesen.", "ファイル内容（%s）は最大許容サイズ（%s）を超えています。ファイル全体を読み込まず、offset と limit で必要な範囲を読み込むか、特定の内容を検索してください。", "파일 내용(%s)이 허용된 최대 크기(%s)를 초과합니다. 전체 파일을 읽는 대신 offset과 limit으로 필요한 부분을 읽거나 특정 내용을 검색하세요.", "Содержимое файла (%s) превышает максимально допустимый размер (%s). Используйте offset и limit для чтения нужных фрагментов либо ищите конкретное содержимое вместо чтения всего файла.")
	addToolFileRuntime(KeyToolFileNotFound, "file does not exist: %s", "文件不存在：%s", "Datei ist nicht vorhanden: %s", "ファイルがありません: %s", "파일이 없습니다: %s", "Файл не существует: %s")
	addToolFileRuntime(KeyToolFileNotFoundInCWD, "File does not exist. Current working directory is %s.", "文件不存在。当前工作目录为 %s。", "Datei ist nicht vorhanden. Das aktuelle Arbeitsverzeichnis ist %s.", "ファイルがありません。現在の作業ディレクトリは %s です。", "파일이 없습니다. 현재 작업 디렉터리는 %s입니다.", "Файл не существует. Текущий рабочий каталог: %s.")
	addToolFileRuntime(KeyToolFileNotFoundSuggestion, "File does not exist. Current working directory is %s. Did you mean %s?", "文件不存在。当前工作目录为 %s。你是不是想输入 %s？", "Datei ist nicht vorhanden. Das aktuelle Arbeitsverzeichnis ist %s. Meintest du %s?", "ファイルがありません。現在の作業ディレクトリは %s です。%s のことですか？", "파일이 없습니다. 현재 작업 디렉터리는 %s입니다. %s을(를) 의미했나요?", "Файл не существует. Текущий рабочий каталог: %s. Возможно, имелось в виду %s?")
	addToolFileRuntime(KeyToolFilePlanModeBlocked, "cannot use %s in plan mode — exit plan mode first", "plan mode 下无法使用 %s，请先退出 plan mode", "%s kann im Planungsmodus nicht verwendet werden — zuerst den Planungsmodus verlassen", "plan mode では %s を使用できません。先に plan mode を終了してください", "plan mode에서는 %s을(를) 사용할 수 없습니다. 먼저 plan mode를 종료하세요", "%s нельзя использовать в режиме планирования — сначала выйдите из него")
	addToolFileRuntime(KeyToolFilePathRequired, "'file_path' is required", "必须提供 file_path", "file_path ist erforderlich", "file_path は必須です", "file_path가 필요합니다", "Требуется file_path")
	addToolFileRuntime(KeyToolFileContentRequired, "'content' is required", "必须提供 content", "content ist erforderlich", "content は必須です", "content가 필요합니다", "Требуется content")
	addToolFileRuntime(KeyToolFileResolveFailed, "cannot resolve path: %v", "无法解析路径：%v", "Pfad kann nicht aufgelöst werden: %v", "パスを解決できません: %v", "경로를 확인할 수 없습니다: %v", "Не удалось определить путь: %v")
	addToolFileRuntime(KeyToolFileTeamMemorySecret, "Refusing to write to team memory file %q: content appears to contain a secret (%s). Remove the credential before writing — team memory is shared in version control.", "拒绝写入团队记忆文件 %q：内容似乎包含 secret（%s）。团队记忆会在版本控制中共享，请移除凭据后再写入。", "Schreiben in die Team-Speicherdatei %q abgelehnt: Der Inhalt scheint ein Secret (%s) zu enthalten. Vor dem Schreiben die Zugangsdaten entfernen — der Team-Speicher wird über die Versionsverwaltung geteilt.", "チームメモリファイル %q への書き込みを拒否しました。内容に secret（%s）が含まれている可能性があります。チームメモリはバージョン管理で共有されるため、書き込む前に認証情報を削除してください。", "팀 메모리 파일 %q에 쓰기를 거부했습니다. 내용에 secret(%s)이 포함된 것으로 보입니다. 팀 메모리는 버전 관리에서 공유되므로 쓰기 전에 자격 증명을 제거하세요.", "Запись в файл памяти команды %q отклонена: содержимое похоже на secret (%s). Удалите учётные данные перед записью — память команды хранится в системе контроля версий.")
	addToolFileRuntime(KeyToolFileCreateDirectoryFailed, "failed to create directory: %v", "无法创建目录：%v", "Verzeichnis konnte nicht erstellt werden: %v", "ディレクトリを作成できませんでした: %v", "디렉터리를 만들지 못했습니다: %v", "Не удалось создать каталог: %v")
	addToolFileRuntime(KeyToolFileNotReadForWrite, "File has not been read yet. Read it first before writing to it.", "尚未读取该文件。请先读取再写入。", "Die Datei wurde noch nicht gelesen. Vor dem Schreiben zuerst lesen.", "ファイルはまだ読み込まれていません。書き込む前に読み込んでください。", "파일을 아직 읽지 않았습니다. 쓰기 전에 먼저 읽으세요.", "Файл ещё не прочитан. Прочитайте его перед записью.")
	addToolFileRuntime(KeyToolFilePartiallyReadForWrite, "File has only been read partially. Read the whole file before writing to it.", "只读取了文件的一部分。请先读取整个文件再写入。", "Die Datei wurde nur teilweise gelesen. Vor dem Schreiben vollständig lesen.", "ファイルの一部しか読み込まれていません。書き込む前に全体を読み込んでください。", "파일의 일부만 읽었습니다. 쓰기 전에 전체 파일을 읽으세요.", "Файл прочитан лишь частично. Перед записью прочитайте его полностью.")
	addToolFileRuntime(KeyToolFileChangedForWrite, "File has been modified since read, either by the user or by a linter. Read it again before attempting to write it.", "文件在读取后已被用户或 linter 修改。请重新读取后再写入。", "Die Datei wurde nach dem Lesen vom Benutzer oder einem Linter geändert. Vor dem Schreiben erneut lesen.", "読み込み後にユーザーまたは linter がファイルを変更しました。書き込む前にもう一度読み込んでください。", "파일을 읽은 뒤 사용자 또는 linter가 수정했습니다. 쓰기 전에 다시 읽으세요.", "После чтения файл был изменён пользователем или linter. Прочитайте его снова перед записью.")
	addToolFileRuntime(KeyToolFileWriteFailed, "failed to write file: %v", "无法写入文件：%v", "Datei konnte nicht geschrieben werden: %v", "ファイルに書き込めませんでした: %v", "파일에 쓰지 못했습니다: %v", "Не удалось записать файл: %v")
	addToolFileRuntime(KeyToolFileReadInvalidTyped, "Read returned an invalid typed result", "Read 返回了无效的类型化结果", "Read hat ein ungültiges typisiertes Ergebnis zurückgegeben", "Read が無効な型付き結果を返しました", "Read가 잘못된 형식의 결과를 반환했습니다", "Read вернул недопустимый типизированный результат")
	addToolFileRuntime(KeyToolFilePDFRead, "PDF file read: %s (%s)", "已读取 PDF 文件：%s（%s）", "PDF-Datei gelesen: %s (%s)", "PDF ファイルを読み込みました: %s（%s）", "PDF 파일을 읽었습니다: %s(%s)", "PDF-файл прочитан: %s (%s)")
	addToolFileRuntime(KeyToolFilePDFPagesExtracted, "PDF pages extracted: %d page(s) from %s (%s)", "已从 %[2]s（%[3]s）提取 %[1]d 页 PDF", "PDF-Seiten extrahiert: %d Seite(n) aus %s (%s)", "%[2]s（%[3]s）から PDF を %[1]d ページ抽出しました", "%[2]s(%[3]s)에서 PDF %[1]d페이지를 추출했습니다", "Извлечены страницы PDF: %d из %s (%s)")
}

func addToolFileRuntime(key Key, en, zh, de, ja, ko, ru string) {
	semanticTranslations[key] = map[Language]string{
		LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
	}
}
