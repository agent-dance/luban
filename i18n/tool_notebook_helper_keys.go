package i18n

const (
	KeyToolNotebookHelperNilNotebook       Key = "tool.notebook.helper.nil_notebook"
	KeyToolNotebookHelperInvalidJSON       Key = "tool.notebook.helper.invalid_json"
	KeyToolNotebookHelperUnknownEditMode   Key = "tool.notebook.helper.unknown_edit_mode"
	KeyToolNotebookHelperCellIDRequired    Key = "tool.notebook.helper.cell_id_required"
	KeyToolNotebookHelperCellNotFound      Key = "tool.notebook.helper.cell_not_found"
	KeyToolNotebookHelperCellIDNotFound    Key = "tool.notebook.helper.cell_id_not_found"
	KeyToolNotebookHelperCellIndexNotFound Key = "tool.notebook.helper.cell_index_not_found"
)

var toolNotebookHelperKeys = []Key{
	KeyToolNotebookHelperNilNotebook,
	KeyToolNotebookHelperInvalidJSON,
	KeyToolNotebookHelperUnknownEditMode,
	KeyToolNotebookHelperCellIDRequired,
	KeyToolNotebookHelperCellNotFound,
	KeyToolNotebookHelperCellIDNotFound,
	KeyToolNotebookHelperCellIndexNotFound,
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

	add(KeyToolNotebookHelperNilNotebook,
		"nil notebook",
		"notebook 不能为空",
		"Das Notebook ist nil",
		"notebook が nil です",
		"notebook이 nil입니다",
		"notebook равен nil")
	add(KeyToolNotebookHelperInvalidJSON,
		"invalid notebook JSON: %v",
		"Notebook JSON 无效：%v",
		"Ungültiges Notebook-JSON: %v",
		"Notebook JSON が無効です: %v",
		"Notebook JSON이 올바르지 않습니다: %v",
		"Недопустимый JSON notebook: %v")
	add(KeyToolNotebookHelperUnknownEditMode,
		"unknown edit_mode: %s",
		"未知的 edit_mode：%s",
		"Unbekannter edit_mode: %s",
		"不明な edit_mode です：%s",
		"알 수 없는 edit_mode입니다: %s",
		"Неизвестный edit_mode: %s")
	add(KeyToolNotebookHelperCellIDRequired,
		"cell_id is required for %s",
		"%s 操作必须提供 cell_id",
		"Für %s ist cell_id erforderlich",
		"%s には cell_id が必要です",
		"%s 작업에는 cell_id가 필요합니다",
		"Для операции %s требуется cell_id")
	add(KeyToolNotebookHelperCellNotFound,
		"Cell not found: %s",
		"未找到 cell：%s",
		"Zelle nicht gefunden: %s",
		"cell が見つかりません：%s",
		"cell을 찾을 수 없습니다: %s",
		"Ячейка не найдена: %s")
	add(KeyToolNotebookHelperCellIDNotFound,
		"Cell with ID %q not found in notebook.",
		"notebook 中未找到 ID 为 %q 的 cell。",
		"Die Zelle mit der ID %q wurde im Notebook nicht gefunden.",
		"ID %q の cell が notebook に見つかりません。",
		"ID가 %q인 cell을 notebook에서 찾을 수 없습니다.",
		"Ячейка с ID %q не найдена в notebook.")
	add(KeyToolNotebookHelperCellIndexNotFound,
		"Cell with index %d does not exist in notebook.",
		"notebook 中不存在索引为 %d 的 cell。",
		"Im Notebook ist keine Zelle mit dem Index %d vorhanden.",
		"インデックス %d の cell は notebook に存在しません。",
		"인덱스가 %d인 cell이 notebook에 없습니다.",
		"Ячейки с индексом %d нет в notebook.")
}
