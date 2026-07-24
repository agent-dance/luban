package i18n

// Semantic copy returned by PDF helpers whose errors flow into Read tool
// results. PDF, pdftoppm, poppler-utils, commands, paths, sizes, subprocess
// output, and underlying causes remain raw format arguments or product terms.
const (
	KeyToolPDFHelperReadFileFailed            Key = "tool.pdf.helper.read_file_failed"
	KeyToolPDFHelperFileEmpty                 Key = "tool.pdf.helper.file_empty"
	KeyToolPDFHelperFileTooLarge              Key = "tool.pdf.helper.file_too_large"
	KeyToolPDFHelperInvalidHeader             Key = "tool.pdf.helper.invalid_header"
	KeyToolPDFHelperExtractionFileTooLarge    Key = "tool.pdf.helper.extraction_file_too_large"
	KeyToolPDFHelperRendererUnavailable       Key = "tool.pdf.helper.renderer_unavailable"
	KeyToolPDFHelperCreateResultsDirectory    Key = "tool.pdf.helper.create_results_directory"
	KeyToolPDFHelperCreateExtractionDirectory Key = "tool.pdf.helper.create_extraction_directory"
	KeyToolPDFHelperReadExtractionOutput      Key = "tool.pdf.helper.read_extraction_output"
	KeyToolPDFHelperNoOutputPages             Key = "tool.pdf.helper.no_output_pages"
	KeyToolPDFHelperReadExtractedPageImage    Key = "tool.pdf.helper.read_extracted_page_image"
	KeyToolPDFHelperPasswordProtected         Key = "tool.pdf.helper.password_protected"
	KeyToolPDFHelperCorrupted                 Key = "tool.pdf.helper.corrupted"
	KeyToolPDFHelperPDFToPPMFailed            Key = "tool.pdf.helper.pdftoppm_failed"
)

var toolPDFHelperKeys = []Key{
	KeyToolPDFHelperReadFileFailed,
	KeyToolPDFHelperFileEmpty,
	KeyToolPDFHelperFileTooLarge,
	KeyToolPDFHelperInvalidHeader,
	KeyToolPDFHelperExtractionFileTooLarge,
	KeyToolPDFHelperRendererUnavailable,
	KeyToolPDFHelperCreateResultsDirectory,
	KeyToolPDFHelperCreateExtractionDirectory,
	KeyToolPDFHelperReadExtractionOutput,
	KeyToolPDFHelperNoOutputPages,
	KeyToolPDFHelperReadExtractedPageImage,
	KeyToolPDFHelperPasswordProtected,
	KeyToolPDFHelperCorrupted,
	KeyToolPDFHelperPDFToPPMFailed,
}

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}

	add(KeyToolPDFHelperReadFileFailed,
		"failed to read file: %v",
		"无法读取文件：%v",
		"Datei konnte nicht gelesen werden: %v",
		"ファイルを読み込めませんでした: %v",
		"파일을 읽지 못했습니다: %v",
		"Не удалось прочитать файл: %v")
	add(KeyToolPDFHelperFileEmpty,
		"PDF file is empty: %s",
		"PDF 文件为空：%s",
		"Die PDF-Datei ist leer: %s",
		"PDF ファイルが空です: %s",
		"PDF 파일이 비어 있습니다: %s",
		"PDF-файл пуст: %s")
	add(KeyToolPDFHelperFileTooLarge,
		"PDF file exceeds maximum allowed size of %s.",
		"PDF 文件超过允许的最大大小 %s。",
		"Die PDF-Datei überschreitet die maximal zulässige Größe von %s.",
		"PDF ファイルが許容最大サイズ %s を超えています。",
		"PDF 파일이 허용되는 최대 크기 %s을(를) 초과했습니다.",
		"Размер PDF-файла превышает допустимый максимум %s.")
	add(KeyToolPDFHelperInvalidHeader,
		"File is not a valid PDF (missing %%PDF- header): %s",
		"文件不是有效的 PDF（缺少 %%PDF- 文件头）：%s",
		"Die Datei ist keine gültige PDF-Datei (Header %%PDF- fehlt): %s",
		"有効な PDF ファイルではありません（%%PDF- ヘッダーがありません）: %s",
		"유효한 PDF 파일이 아닙니다(%%PDF- 헤더 없음): %s",
		"Файл не является допустимым PDF (отсутствует заголовок %%PDF-): %s")
	add(KeyToolPDFHelperExtractionFileTooLarge,
		"PDF file exceeds maximum allowed size for text extraction (%s).",
		"PDF 文件超过文本提取允许的最大大小（%s）。",
		"Die PDF-Datei überschreitet die maximal zulässige Größe für die Textextraktion (%s).",
		"PDF ファイルがテキスト抽出の許容最大サイズ（%s）を超えています。",
		"PDF 파일이 텍스트 추출에 허용되는 최대 크기(%s)를 초과했습니다.",
		"Размер PDF-файла превышает допустимый максимум для извлечения текста (%s).")
	add(KeyToolPDFHelperRendererUnavailable,
		"pdftoppm is not installed. Install poppler-utils (e.g. `brew install poppler` or `apt-get install poppler-utils`) to enable PDF page rendering.",
		"未安装 pdftoppm。请安装 poppler-utils（例如运行 `brew install poppler` 或 `apt-get install poppler-utils`）以启用 PDF 页面渲染。",
		"pdftoppm ist nicht installiert. Installiere poppler-utils (z. B. mit `brew install poppler` oder `apt-get install poppler-utils`), um PDF-Seiten rendern zu können.",
		"pdftoppm がインストールされていません。PDF ページのレンダリングを有効にするには、poppler-utils をインストールしてください（例: `brew install poppler` または `apt-get install poppler-utils`）。",
		"pdftoppm이 설치되어 있지 않습니다. PDF 페이지 렌더링을 사용하려면 poppler-utils를 설치하세요(예: `brew install poppler` 또는 `apt-get install poppler-utils`).",
		"pdftoppm не установлен. Установите poppler-utils (например, `brew install poppler` или `apt-get install poppler-utils`), чтобы включить рендеринг страниц PDF.")
	add(KeyToolPDFHelperCreateResultsDirectory,
		"failed to create tool results directory: %v",
		"无法创建工具结果目录：%v",
		"Verzeichnis für Tool-Ergebnisse konnte nicht erstellt werden: %v",
		"ツール結果ディレクトリを作成できませんでした: %v",
		"도구 결과 디렉터리를 만들지 못했습니다: %v",
		"Не удалось создать каталог результатов инструмента: %v")
	add(KeyToolPDFHelperCreateExtractionDirectory,
		"failed to create PDF extraction directory: %v",
		"无法创建 PDF 提取目录：%v",
		"Verzeichnis für die PDF-Extraktion konnte nicht erstellt werden: %v",
		"PDF 抽出ディレクトリを作成できませんでした: %v",
		"PDF 추출 디렉터리를 만들지 못했습니다: %v",
		"Не удалось создать каталог для извлечения PDF: %v")
	add(KeyToolPDFHelperReadExtractionOutput,
		"failed to read PDF extraction output: %v",
		"无法读取 PDF 提取结果：%v",
		"Ausgabe der PDF-Extraktion konnte nicht gelesen werden: %v",
		"PDF 抽出結果を読み込めませんでした: %v",
		"PDF 추출 결과를 읽지 못했습니다: %v",
		"Не удалось прочитать результат извлечения PDF: %v")
	add(KeyToolPDFHelperNoOutputPages,
		"pdftoppm produced no output pages. The PDF may be invalid.",
		"pdftoppm 未生成任何页面。该 PDF 可能无效。",
		"pdftoppm hat keine Seiten ausgegeben. Die PDF-Datei ist möglicherweise ungültig.",
		"pdftoppm からページが出力されませんでした。PDF が無効な可能性があります。",
		"pdftoppm이 페이지를 출력하지 않았습니다. PDF가 유효하지 않을 수 있습니다.",
		"pdftoppm не создал ни одной страницы. Возможно, PDF недействителен.")
	add(KeyToolPDFHelperReadExtractedPageImage,
		"failed to read extracted PDF page image: %v",
		"无法读取提取出的 PDF 页面图像：%v",
		"Extrahiertes PDF-Seitenbild konnte nicht gelesen werden: %v",
		"抽出した PDF ページ画像を読み込めませんでした: %v",
		"추출한 PDF 페이지 이미지를 읽지 못했습니다: %v",
		"Не удалось прочитать извлечённое изображение страницы PDF: %v")
	add(KeyToolPDFHelperPasswordProtected,
		"PDF is password-protected. Please provide an unprotected version.",
		"PDF 受密码保护。请提供未加密的版本。",
		"Die PDF-Datei ist kennwortgeschützt. Bitte stelle eine ungeschützte Version bereit.",
		"PDF はパスワードで保護されています。保護されていない版を指定してください。",
		"PDF가 암호로 보호되어 있습니다. 보호되지 않은 버전을 제공하세요.",
		"PDF защищён паролем. Предоставьте версию без защиты.")
	add(KeyToolPDFHelperCorrupted,
		"PDF file is corrupted or invalid.",
		"PDF 文件已损坏或无效。",
		"Die PDF-Datei ist beschädigt oder ungültig.",
		"PDF ファイルが破損しているか無効です。",
		"PDF 파일이 손상되었거나 유효하지 않습니다.",
		"PDF-файл повреждён или недействителен.")
	add(KeyToolPDFHelperPDFToPPMFailed,
		"pdftoppm failed: %s",
		"pdftoppm 失败：%s",
		"pdftoppm ist fehlgeschlagen: %s",
		"pdftoppm に失敗しました: %s",
		"pdftoppm 실행에 실패했습니다: %s",
		"Сбой pdftoppm: %s")
}
