package i18n

// Semantic copy for the composite, repository-scoped Inspect tool.
const (
	KeyToolInspectDescription                  Key = "tool.inspect.description"
	KeyToolInspectSearchHint                   Key = "tool.inspect.search_hint"
	KeyToolInspectInputOperationDescription    Key = "tool.inspect.input.operation.description"
	KeyToolInspectInputModeDescription         Key = "tool.inspect.input.mode.description"
	KeyToolInspectInputRequestsDescription     Key = "tool.inspect.input.requests.description"
	KeyToolInspectInputCursorDescription       Key = "tool.inspect.input.cursor.description"
	KeyToolInspectInputPageDescription         Key = "tool.inspect.input.page.description"
	KeyToolInspectInputMaxCharsDescription     Key = "tool.inspect.input.max_chars.description"
	KeyToolInspectInputMaxFilesDescription     Key = "tool.inspect.input.max_files.description"
	KeyToolInspectInputMaxMatchesDescription   Key = "tool.inspect.input.max_matches.description"
	KeyToolInspectRequestIDDescription         Key = "tool.inspect.request.id.description"
	KeyToolInspectRequestKindDescription       Key = "tool.inspect.request.kind.description"
	KeyToolInspectRequestPathDescription       Key = "tool.inspect.request.path.description"
	KeyToolInspectRequestPatternDescription    Key = "tool.inspect.request.pattern.description"
	KeyToolInspectRequestRangesDescription     Key = "tool.inspect.request.ranges.description"
	KeyToolInspectRequestRangeStartDescription Key = "tool.inspect.request.range.start.description"
	KeyToolInspectRequestRangeEndDescription   Key = "tool.inspect.request.range.end.description"
	KeyToolInspectRequestContextDescription    Key = "tool.inspect.request.context.description"
	KeyToolInspectRequestMaxResultsDescription Key = "tool.inspect.request.max_results.description"
	KeyToolInspectInvalidInput                 Key = "tool.inspect.error.invalid_input"
	KeyToolInspectMalformedInput               Key = "tool.inspect.error.malformed_input"
	KeyToolInspectRequestsRequired             Key = "tool.inspect.error.requests_required"
	KeyToolInspectTooManyRequests              Key = "tool.inspect.error.too_many_requests"
	KeyToolInspectRequestIDRequired            Key = "tool.inspect.error.request_id_required"
	KeyToolInspectDuplicateRequestID           Key = "tool.inspect.error.duplicate_request_id"
	KeyToolInspectUnsupportedKind              Key = "tool.inspect.error.unsupported_kind"
	KeyToolInspectPathRequired                 Key = "tool.inspect.error.path_required"
	KeyToolInspectInvalidRange                 Key = "tool.inspect.error.invalid_range"
	KeyToolInspectTooManyRanges                Key = "tool.inspect.error.too_many_ranges"
	KeyToolInspectContextOutOfRange            Key = "tool.inspect.error.context_out_of_range"
	KeyToolInspectMaxResultsOutOfRange         Key = "tool.inspect.error.max_results_out_of_range"
	KeyToolInspectLimitOutOfRange              Key = "tool.inspect.error.limit_out_of_range"
	KeyToolInspectValueTooLong                 Key = "tool.inspect.error.value_too_long"
	KeyToolInspectProjectRootUnavailable       Key = "tool.inspect.error.project_root_unavailable"
	KeyToolInspectPathOutsideRepository        Key = "tool.inspect.error.path_outside_repository"
	KeyToolInspectCursorInvalid                Key = "tool.inspect.error.cursor_invalid"
	KeyToolInspectTextOnly                     Key = "tool.inspect.error.text_only"
	KeyToolInspectResultEncodingFailed         Key = "tool.inspect.error.result_encoding_failed"
)

var toolInspectKeys = [...]Key{
	KeyToolInspectDescription,
	KeyToolInspectSearchHint,
	KeyToolInspectInputOperationDescription,
	KeyToolInspectInputModeDescription,
	KeyToolInspectInputRequestsDescription,
	KeyToolInspectInputCursorDescription,
	KeyToolInspectInputPageDescription,
	KeyToolInspectInputMaxCharsDescription,
	KeyToolInspectInputMaxFilesDescription,
	KeyToolInspectInputMaxMatchesDescription,
	KeyToolInspectRequestIDDescription,
	KeyToolInspectRequestKindDescription,
	KeyToolInspectRequestPathDescription,
	KeyToolInspectRequestPatternDescription,
	KeyToolInspectRequestRangesDescription,
	KeyToolInspectRequestRangeStartDescription,
	KeyToolInspectRequestRangeEndDescription,
	KeyToolInspectRequestContextDescription,
	KeyToolInspectRequestMaxResultsDescription,
	KeyToolInspectInvalidInput,
	KeyToolInspectMalformedInput,
	KeyToolInspectRequestsRequired,
	KeyToolInspectTooManyRequests,
	KeyToolInspectRequestIDRequired,
	KeyToolInspectDuplicateRequestID,
	KeyToolInspectUnsupportedKind,
	KeyToolInspectPathRequired,
	KeyToolInspectInvalidRange,
	KeyToolInspectTooManyRanges,
	KeyToolInspectContextOutOfRange,
	KeyToolInspectMaxResultsOutOfRange,
	KeyToolInspectLimitOutOfRange,
	KeyToolInspectValueTooLong,
	KeyToolInspectProjectRootUnavailable,
	KeyToolInspectPathOutsideRepository,
	KeyToolInspectCursorInvalid,
	KeyToolInspectTextOnly,
	KeyToolInspectResultEncodingFailed,
}

func init() {
	addInspect := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}

	addInspect(KeyToolInspectDescription,
		"Inspect repository files in one read-only batch. Mix read, search, and glob requests; results establish edit evidence, group source by path, replace already visible lines with content-addressed references, and preserve complete pages with an opaque cursor.",
		"在一个只读批次中检查仓库文件。可混合 read、search 和 glob 请求；结果会建立编辑证据、按路径归并源码、用内容寻址引用替代模型已见的行，并通过不透明游标保持每页完整。",
		"Untersucht Repository-Dateien in einem schreibgeschützten Stapel. read-, search- und glob-Anfragen lassen sich mischen; die Ergebnisse begründen Bearbeitungsnachweise, gruppieren Quelltext nach Pfad, ersetzen bereits sichtbare Zeilen durch inhaltsadressierte Verweise und halten jede Seite mit einem opaken Cursor vollständig.",
		"リポジトリ内のファイルを 1 回の読み取り専用バッチで調査します。read、search、glob を混在でき、結果は編集証拠を確立し、ソースをパス単位でまとめ、既に表示した行を内容アドレス参照に置き換え、不透明カーソルで各ページの完全性を保ちます。",
		"하나의 읽기 전용 배치에서 저장소 파일을 조사합니다. read, search, glob 요청을 함께 사용할 수 있으며 결과는 편집 근거를 만들고, 소스를 경로별로 묶고, 이미 표시된 줄을 콘텐츠 주소 참조로 대체하며, 불투명 커서로 각 페이지를 온전하게 유지합니다.",
		"Исследует файлы репозитория одним пакетом только для чтения. Можно смешивать запросы read, search и glob; результаты создают основание для редактирования, группируют исходный текст по пути, заменяют уже показанные строки ссылками по содержимому и сохраняют целостность страниц с помощью непрозрачного курсора.")
	addInspect(KeyToolInspectSearchHint,
		"batch repository inspection, source reading, search, and file discovery",
		"批量仓库检查、源码读取、搜索与文件发现",
		"gebündelte Repository-Untersuchung, Quelltextlesen, Suche und Dateiermittlung",
		"リポジトリの一括調査、ソース読み取り、検索、ファイル探索",
		"저장소 일괄 조사, 소스 읽기, 검색 및 파일 탐색",
		"пакетное исследование репозитория, чтение исходников, поиск и обнаружение файлов")
	addInspect(KeyToolInspectInputOperationDescription,
		"Inspect operation: start a new request batch or continue one exact cursor snapshot", "Inspect 操作：启动新的请求批次，或继续一个精确的游标快照", "Inspect-Vorgang: einen neuen Anfragestapel beginnen oder exakt einen Cursor-Schnappschuss fortsetzen", "Inspect 操作。新しいリクエストバッチを開始するか、1 つのカーソルスナップショットをそのまま継続します", "Inspect 작업: 새 요청 배치를 시작하거나 정확한 커서 스냅샷 하나를 계속합니다", "Операция Inspect: начать новый пакет запросов или продолжить один точный снимок курсора")
	addInspect(KeyToolInspectInputModeDescription,
		"Operation branch discriminator: new or continue", "操作分支判别字段：new 或 continue", "Diskriminator des Vorgangszweigs: new oder continue", "操作分岐の判別子: new または continue", "작업 분기 판별자: new 또는 continue", "Дискриминатор ветви операции: new или continue")
	addInspect(KeyToolInspectInputRequestsDescription,
		"Ordered read, search, or glob requests in the new branch", "new 分支中按顺序执行的 read、search 或 glob 请求", "Geordnete read-, search- oder glob-Anfragen im new-Zweig", "new 分岐で順に実行する read、search、glob リクエスト", "new 분기에서 순서대로 실행할 read, search, glob 요청", "Упорядоченные запросы read, search или glob в ветви new")
	addInspect(KeyToolInspectInputCursorDescription,
		"Opaque cursor from a partial Inspect result in the continue branch", "continue 分支中使用的 Inspect 部分结果不透明游标", "Opaker Cursor aus einem unvollständigen Inspect-Ergebnis im continue-Zweig", "continue 分岐で使う Inspect 部分結果の不透明カーソル", "continue 분기에서 사용하는 Inspect 부분 결과의 불투명 커서", "Непрозрачный курсор из частичного результата Inspect в ветви continue")
	addInspect(KeyToolInspectInputPageDescription,
		"Optional page limits for a new inspection", "新检查可选的分页限制", "Optionale Seitengrenzen für eine neue Untersuchung", "新しい調査に適用する任意のページ上限", "새 조사에 적용할 선택적 페이지 제한", "Необязательные ограничения страницы для нового исследования")
	addInspect(KeyToolInspectInputMaxCharsDescription,
		"Requested model-visible page size; large values are safely paginated so the cursor is never truncated", "请求的模型可见页面大小；较大的值会被安全分页，确保游标不会被截断", "Angeforderte modellsichtbare Seitengröße; große Werte werden sicher paginiert, damit der Cursor nie abgeschnitten wird", "モデルに表示するページサイズの要求値。大きな値は安全にページ分割され、カーソルが切り捨てられることはありません", "모델에 표시할 페이지 크기 요청값입니다. 큰 값은 안전하게 페이지로 나뉘므로 커서가 잘리지 않습니다", "Запрошенный размер видимой модели страницы; большие значения безопасно разбиваются на страницы, поэтому курсор не обрезается")
	addInspect(KeyToolInspectInputMaxFilesDescription,
		"Maximum distinct files in this page", "本页最多包含的不同文件数", "Maximale Anzahl unterschiedlicher Dateien auf dieser Seite", "このページに含める異なるファイルの最大数", "이 페이지에 포함할 서로 다른 파일의 최대 개수", "Максимальное число различных файлов на странице")
	addInspect(KeyToolInspectInputMaxMatchesDescription,
		"Maximum search matches in this page", "本页最多包含的搜索匹配数", "Maximale Anzahl von Suchtreffern auf dieser Seite", "このページに含める検索一致の最大数", "이 페이지에 포함할 최대 검색 일치 수", "Максимальное число результатов поиска на странице")
	addInspect(KeyToolInspectRequestIDDescription,
		"Unique caller-chosen request identifier", "调用方指定的唯一请求标识符", "Eindeutige, vom Aufrufer gewählte Anfragekennung", "呼び出し側が指定する一意のリクエスト識別子", "호출자가 정하는 고유 요청 식별자", "Уникальный идентификатор запроса, заданный вызывающей стороной")
	addInspect(KeyToolInspectRequestKindDescription,
		"Operation kind: read, search, or glob", "操作类型：read、search 或 glob", "Vorgangsart: read, search oder glob", "操作種別: read、search、glob", "작업 종류: read, search 또는 glob", "Вид операции: read, search или glob")
	addInspect(KeyToolInspectRequestPathDescription,
		"Repository-relative file or directory path", "仓库内的相对文件或目录路径", "Repository-relativer Datei- oder Verzeichnispfad", "リポジトリ相対のファイルまたはディレクトリパス", "저장소 기준 파일 또는 디렉터리 경로", "Путь к файлу или каталогу относительно репозитория")
	addInspect(KeyToolInspectRequestPatternDescription,
		"Regular expression for search or file pattern for glob", "search 使用的正则表达式，或 glob 使用的文件模式", "Regulärer Ausdruck für search oder Dateimuster für glob", "search の正規表現、または glob のファイルパターン", "search용 정규식 또는 glob용 파일 패턴", "Регулярное выражение для search или шаблон файлов для glob")
	addInspect(KeyToolInspectRequestRangesDescription,
		"Inclusive 1-based line ranges for read; omitted means the whole text file", "read 使用的从 1 开始且含首尾的行范围；省略时读取整个文本文件", "Inklusive, 1-basierte Zeilenbereiche für read; ohne Angabe wird die gesamte Textdatei gelesen", "read 用の 1 始まり・両端を含む行範囲。省略時はテキストファイル全体", "read에 사용할 1부터 시작하는 양끝 포함 줄 범위. 생략하면 전체 텍스트 파일", "Диапазоны строк для read с нумерацией от 1 и включёнными границами; без них читается весь текстовый файл")
	addInspect(KeyToolInspectRequestRangeStartDescription,
		"First line in the inclusive range", "范围内的首行", "Erste Zeile des inklusiven Bereichs", "範囲に含める先頭行", "포함 범위의 첫 줄", "Первая строка включительного диапазона")
	addInspect(KeyToolInspectRequestRangeEndDescription,
		"Last line in the inclusive range", "范围内的末行", "Letzte Zeile des inklusiven Bereichs", "範囲に含める末尾行", "포함 범위의 마지막 줄", "Последняя строка включительного диапазона")
	addInspect(KeyToolInspectRequestContextDescription,
		"Lines of context before and after each search match", "每个 search 匹配项前后的上下文行数", "Kontextzeilen vor und nach jedem search-Treffer", "各 search 一致箇所の前後に含める行数", "각 search 일치 앞뒤의 문맥 줄 수", "Число строк контекста до и после каждого совпадения search")
	addInspect(KeyToolInspectRequestMaxResultsDescription,
		"Maximum files or matches acquired for this request", "此请求最多获取的文件或匹配项数量", "Maximale Anzahl erfasster Dateien oder Treffer für diese Anfrage", "このリクエストで取得するファイルまたは一致の最大数", "이 요청에서 가져올 파일 또는 일치 항목의 최대 개수", "Максимальное число файлов или совпадений для этого запроса")

	addInspect(KeyToolInspectInvalidInput,
		"invalid Inspect input: %v", "Inspect 输入无效：%v", "Ungültige Inspect-Eingabe: %v", "Inspect の入力が無効です: %v", "Inspect 입력이 올바르지 않습니다: %v", "Недопустимый ввод Inspect: %v")
	addInspect(KeyToolInspectMalformedInput,
		"Inspect input has an invalid value or type", "Inspect 输入包含无效的值或类型", "Die Inspect-Eingabe enthält einen ungültigen Wert oder Typ", "Inspect の入力に無効な値または型があります", "Inspect 입력에 잘못된 값이나 형식이 있습니다", "Ввод Inspect содержит недопустимое значение или тип")
	addInspect(KeyToolInspectRequestsRequired,
		"at least one Inspect request is required", "至少需要一个 Inspect 请求", "Mindestens eine Inspect-Anfrage ist erforderlich", "Inspect リクエストを 1 件以上指定してください", "Inspect 요청이 하나 이상 필요합니다", "Требуется хотя бы один запрос Inspect")
	addInspect(KeyToolInspectTooManyRequests,
		"Inspect accepts at most %d requests per batch", "每个 Inspect 批次最多接受 %d 个请求", "Inspect akzeptiert höchstens %d Anfragen pro Stapel", "Inspect の 1 バッチで指定できるリクエストは最大 %d 件です", "Inspect 배치 하나에는 최대 %d개의 요청을 넣을 수 있습니다", "Inspect принимает не более %d запросов в одном пакете")
	addInspect(KeyToolInspectRequestIDRequired,
		"every Inspect request needs a non-empty id", "每个 Inspect 请求都必须包含非空 id", "Jede Inspect-Anfrage benötigt eine nicht leere id", "各 Inspect リクエストには空でない id が必要です", "모든 Inspect 요청에는 비어 있지 않은 id가 필요합니다", "Каждому запросу Inspect нужен непустой id")
	addInspect(KeyToolInspectDuplicateRequestID,
		"duplicate Inspect request id: %s", "Inspect 请求 id 重复：%s", "Doppelte Inspect-Anfrage-id: %s", "Inspect リクエストの id が重複しています: %s", "중복된 Inspect 요청 id: %s", "Повторяющийся id запроса Inspect: %s")
	addInspect(KeyToolInspectUnsupportedKind,
		"unsupported Inspect request kind: %s", "不支持的 Inspect 请求类型：%s", "Nicht unterstützte Inspect-Anfrageart: %s", "未対応の Inspect リクエスト種別です: %s", "지원하지 않는 Inspect 요청 종류: %s", "Неподдерживаемый вид запроса Inspect: %s")
	addInspect(KeyToolInspectPathRequired,
		"Inspect read requests require a path", "Inspect read 请求必须提供 path", "Inspect-read-Anfragen benötigen einen path", "Inspect の read リクエストには path が必要です", "Inspect read 요청에는 path가 필요합니다", "Для запроса Inspect read требуется path")
	addInspect(KeyToolInspectInvalidRange,
		"invalid inclusive line range %d-%d", "无效的闭区间行范围 %d-%d", "Ungültiger inklusiver Zeilenbereich %d-%d", "両端を含む行範囲 %d-%d は無効です", "잘못된 양끝 포함 줄 범위 %d-%d", "Недопустимый включительный диапазон строк %d-%d")
	addInspect(KeyToolInspectTooManyRanges,
		"a read request accepts at most %d ranges", "一个 read 请求最多接受 %d 个范围", "Eine read-Anfrage akzeptiert höchstens %d Bereiche", "1 件の read リクエストで指定できる範囲は最大 %d 件です", "read 요청 하나에는 최대 %d개의 범위를 지정할 수 있습니다", "Один запрос read принимает не более %d диапазонов")
	addInspect(KeyToolInspectContextOutOfRange,
		"search context must be between 0 and %d lines", "search 的 context 必须在 0 到 %d 行之间", "Der search-Kontext muss zwischen 0 und %d Zeilen liegen", "search の context は 0〜%d 行で指定してください", "search context는 0~%d줄이어야 합니다", "Контекст search должен составлять от 0 до %d строк")
	addInspect(KeyToolInspectMaxResultsOutOfRange,
		"max_results must be between 1 and %d", "max_results 必须在 1 到 %d 之间", "max_results muss zwischen 1 und %d liegen", "max_results は 1〜%d で指定してください", "max_results는 1~%d여야 합니다", "max_results должен быть от 1 до %d")
	addInspect(KeyToolInspectLimitOutOfRange,
		"%s must be between %d and %d", "%s 必须在 %d 到 %d 之间", "%s muss zwischen %d und %d liegen", "%s は %d〜%d で指定してください", "%s은(는) %d~%d여야 합니다", "%s должен быть от %d до %d")
	addInspect(KeyToolInspectValueTooLong,
		"%s exceeds the maximum length of %d characters", "%s 超过了 %d 个字符的长度上限", "%s überschreitet die maximale Länge von %d Zeichen", "%s が最大長 %d 文字を超えています", "%s이(가) 최대 길이 %d자를 초과합니다", "%s превышает максимальную длину в %d символов")
	addInspect(KeyToolInspectProjectRootUnavailable,
		"Inspect could not resolve a repository root", "Inspect 无法解析仓库根目录", "Inspect konnte kein Repository-Stammverzeichnis bestimmen", "Inspect がリポジトリルートを解決できませんでした", "Inspect가 저장소 루트를 확인하지 못했습니다", "Inspect не удалось определить корень репозитория")
	addInspect(KeyToolInspectPathOutsideRepository,
		"Inspect path is outside the repository: %s", "Inspect 路径位于仓库之外：%s", "Inspect-Pfad liegt außerhalb des Repositorys: %s", "Inspect のパスがリポジトリ外です: %s", "Inspect 경로가 저장소 밖에 있습니다: %s", "Путь Inspect находится вне репозитория: %s")
	addInspect(KeyToolInspectCursorInvalid,
		"Inspect cursor is invalid, expired, already consumed, or belongs to another workspace", "Inspect 游标无效、已过期、已被使用，或属于其他工作区", "Der Inspect-Cursor ist ungültig, abgelaufen, bereits verbraucht oder gehört zu einem anderen Workspace", "Inspect カーソルが無効、期限切れ、使用済み、または別のワークスペースに属しています", "Inspect 커서가 잘못되었거나 만료 또는 이미 사용되었거나 다른 워크스페이스에 속합니다", "Курсор Inspect недействителен, истёк, уже использован или относится к другой рабочей области")
	addInspect(KeyToolInspectTextOnly,
		"Inspect read supports text files only: %s", "Inspect read 仅支持文本文件：%s", "Inspect read unterstützt nur Textdateien: %s", "Inspect read が対応するのはテキストファイルだけです: %s", "Inspect read는 텍스트 파일만 지원합니다: %s", "Inspect read поддерживает только текстовые файлы: %s")
	addInspect(KeyToolInspectResultEncodingFailed,
		"Inspect could not encode its result", "Inspect 无法编码结果", "Inspect konnte sein Ergebnis nicht kodieren", "Inspect の結果をエンコードできませんでした", "Inspect가 결과를 인코딩하지 못했습니다", "Inspect не удалось закодировать результат")
}
