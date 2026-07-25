package i18n

const (
	KeyToolLSPDescription               Key = "tool.lsp.description"
	KeyToolLSPInputOperationDescription Key = "tool.lsp.input.operation.description"
	KeyToolLSPInputFilePathDescription  Key = "tool.lsp.input.file_path.description"
	KeyToolLSPInputLineDescription      Key = "tool.lsp.input.line.description"
	KeyToolLSPInputCharacterDescription Key = "tool.lsp.input.character.description"
)

var toolLSPPromptKeys = [...]Key{
	KeyToolLSPDescription,
	KeyToolLSPInputOperationDescription,
	KeyToolLSPInputFilePathDescription,
	KeyToolLSPInputLineDescription,
	KeyToolLSPInputCharacterDescription,
}

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}

	add(KeyToolLSPDescription,
		"Interact with Language Server Protocol servers for code intelligence",
		"通过 Language Server Protocol server 获取代码智能信息",
		"Interagiert mit Language-Server-Protocol-Servern für Code-Intelligence",
		"Language Server Protocol server と連携してコードインテリジェンス情報を取得します",
		"Language Server Protocol server와 연동하여 코드 인텔리전스 정보를 가져옵니다",
		"Взаимодействует с Language Server Protocol server для анализа кода")
	add(KeyToolLSPInputOperationDescription,
		"LSP operation to perform",
		"要执行的 LSP operation",
		"Auszuführende LSP-Operation",
		"実行する LSP operation",
		"실행할 LSP operation",
		"Выполняемая LSP operation")
	add(KeyToolLSPInputFilePathDescription,
		"Absolute or relative path to the file to operate on",
		"目标文件的绝对或相对路径",
		"Absoluter oder relativer Pfad zur Zieldatei",
		"対象ファイルへの絶対パスまたは相対パス",
		"대상 파일의 절대 또는 상대 경로",
		"Абсолютный или относительный путь к целевому файлу")
	add(KeyToolLSPInputLineDescription,
		"1-based line number",
		"从 1 开始的行号",
		"Zeilennummer ab 1",
		"1 から始まる行番号",
		"1부터 시작하는 줄 번호",
		"Номер строки, начиная с 1")
	add(KeyToolLSPInputCharacterDescription,
		"1-based character offset",
		"从 1 开始的字符偏移量",
		"Zeichenposition ab 1",
		"1 から始まる文字オフセット",
		"1부터 시작하는 문자 오프셋",
		"Смещение символа, начиная с 1")
}
