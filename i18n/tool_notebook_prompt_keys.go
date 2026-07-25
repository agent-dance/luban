package i18n

const (
	KeyToolNotebookEditDescription               Key = "tool.notebook_edit.description"
	KeyToolNotebookEditInputPathDescription      Key = "tool.notebook_edit.input.notebook_path.description"
	KeyToolNotebookEditInputCellIDDescription    Key = "tool.notebook_edit.input.cell_id.description"
	KeyToolNotebookEditInputNewSourceDescription Key = "tool.notebook_edit.input.new_source.description"
	KeyToolNotebookEditInputCellTypeDescription  Key = "tool.notebook_edit.input.cell_type.description"
	KeyToolNotebookEditInputModeDescription      Key = "tool.notebook_edit.input.edit_mode.description"
)

var toolNotebookPromptKeys = []Key{
	KeyToolNotebookEditDescription,
	KeyToolNotebookEditInputPathDescription,
	KeyToolNotebookEditInputCellIDDescription,
	KeyToolNotebookEditInputNewSourceDescription,
	KeyToolNotebookEditInputCellTypeDescription,
	KeyToolNotebookEditInputModeDescription,
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

	add(KeyToolNotebookEditDescription,
		"Edit cells in a Jupyter notebook (.ipynb file). Supports replace, insert, and delete operations.",
		"编辑 Jupyter notebook（.ipynb 文件）中的 cell，支持替换、插入和删除操作。",
		"Zellen in einem Jupyter Notebook (einer .ipynb-Datei) bearbeiten. Unterstützt Ersetzen, Einfügen und Löschen.",
		"Jupyter notebook（.ipynb ファイル）の cell を編集します。置換、挿入、削除に対応しています。",
		"Jupyter notebook(.ipynb 파일)의 cell을 편집합니다. 바꾸기, 삽입, 삭제 작업을 지원합니다.",
		"Редактирует ячейки Jupyter notebook (файла .ipynb). Поддерживает замену, вставку и удаление.")
	add(KeyToolNotebookEditInputPathDescription,
		"Path to the .ipynb file, relative to the project root or absolute",
		".ipynb 文件路径，可使用相对于项目根目录的路径或绝对路径",
		"Pfad zur .ipynb-Datei, relativ zum Projektstamm oder absolut",
		".ipynb ファイルへのパス（プロジェクトルートからの相対パスまたは絶対パス）",
		".ipynb 파일 경로(프로젝트 루트 기준 상대 경로 또는 절대 경로)",
		"Путь к файлу .ipynb: относительно корня проекта или абсолютный")
	add(KeyToolNotebookEditInputCellIDDescription,
		"Cell ID to edit. For insert, the new cell is added after this ID.",
		"要编辑的 cell ID；插入时，新 cell 将添加在此 ID 之后。",
		"ID der zu bearbeitenden Zelle. Beim Einfügen wird die neue Zelle nach dieser ID hinzugefügt.",
		"編集する cell ID。挿入時は、この ID の後に新しい cell を追加します。",
		"편집할 cell ID입니다. 삽입 시 새 cell이 이 ID 뒤에 추가됩니다.",
		"ID редактируемой ячейки. При вставке новая ячейка добавляется после этого ID.")
	add(KeyToolNotebookEditInputNewSourceDescription,
		"New source content for the cell",
		"cell 的新源内容",
		"Neuer Quellinhalt der Zelle",
		"cell の新しいソース内容",
		"cell의 새 소스 내용",
		"Новое исходное содержимое ячейки")
	add(KeyToolNotebookEditInputCellTypeDescription,
		"Cell type: code or markdown",
		"cell 类型：code 或 markdown",
		"Zellentyp: code oder markdown",
		"cell の種類: code または markdown",
		"cell 유형: code 또는 markdown",
		"Тип ячейки: code или markdown")
	add(KeyToolNotebookEditInputModeDescription,
		"Edit mode: replace (default), insert, or delete",
		"编辑模式：replace（默认）、insert 或 delete",
		"Bearbeitungsmodus: replace (Standard), insert oder delete",
		"編集モード: replace（デフォルト）、insert、delete のいずれか",
		"편집 모드: replace(기본값), insert 또는 delete",
		"Режим редактирования: replace (по умолчанию), insert или delete")
}
