package i18n

const (
	KeyToolApplyPatchPreflightRules       Key = "tool.apply_patch.preflight.rules"
	KeyToolInspectReadDirectory           Key = "tool.inspect.runtime.read_directory"
	KeyToolInspectPartialReason           Key = "tool.inspect.runtime.partial_reason"
	KeyPresentationToolParameterSize      Key = "presentation.tool.parameter_size"
	KeyPresentationToolPatchChanges       Key = "presentation.tool.patch_changes"
	KeyPresentationToolReceiptSize        Key = "presentation.tool.receipt_size"
	KeyPresentationInspectPartialFailures Key = "presentation.inspect.partial_failures"
	KeyPresentationInspectPartialFailure  Key = "presentation.inspect.partial_failure"
	KeyPresentationInspectOtherSuccessful Key = "presentation.inspect.other_successful"
)

var toolFrictionKeys = [...]Key{
	KeyToolApplyPatchPreflightRules,
	KeyToolInspectReadDirectory,
	KeyToolInspectPartialReason,
	KeyPresentationToolParameterSize,
	KeyPresentationToolPatchChanges,
	KeyPresentationToolReceiptSize,
	KeyPresentationInspectPartialFailures,
	KeyPresentationInspectPartialFailure,
	KeyPresentationInspectOtherSuccessful,
}

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}
	add(KeyToolApplyPatchPreflightRules,
		"Each project-relative target may appear only once; combine multiple hunks for the same file in one section. A context-free delete requires a prior complete Inspect read of that file; otherwise include deletion context.",
		"每个项目相对目标只能出现一次；同一文件的多个 hunk 必须合并到一个区段。无上下文删除要求事先通过 Inspect 完整读取该文件；否则请提供删除上下文。",
		"Jedes projektbezogene Ziel darf nur einmal vorkommen; fasse mehrere Hunks derselben Datei in einem Abschnitt zusammen. Eine kontextfreie Löschung erfordert zuvor ein vollständiges Lesen der Datei mit Inspect; andernfalls muss Löschkontext angegeben werden.",
		"プロジェクト相対の各対象は 1 回だけ指定し、同じファイルの複数の hunk は 1 つのセクションにまとめてください。コンテキストなしの削除には、事前に Inspect でそのファイル全体を読み取る必要があります。そうでなければ削除コンテキストを含めてください。",
		"프로젝트 상대 대상은 각각 한 번만 지정하고, 같은 파일의 여러 hunk는 한 섹션에 합치세요. 문맥 없는 삭제는 먼저 Inspect로 해당 파일 전체를 읽어야 하며, 그렇지 않으면 삭제 문맥을 포함해야 합니다.",
		"Каждая цель относительно проекта должна встречаться только один раз; объединяйте несколько hunk одного файла в одной секции. Для удаления без контекста необходимо заранее полностью прочитать файл через Inspect; иначе добавьте контекст удаления.")
	add(KeyToolInspectReadDirectory,
		"%s is a directory and cannot be read as a file; use glob or search, or specify a file path.",
		"%s 是目录，不能按文件读取；请使用 glob、search，或指定文件路径。",
		"%s ist ein Verzeichnis und kann nicht als Datei gelesen werden; verwende glob oder search oder gib einen Dateipfad an.",
		"%s はディレクトリのためファイルとして読み取れません。glob または search を使うか、ファイルパスを指定してください。",
		"%s은(는) 디렉터리이므로 파일로 읽을 수 없습니다. glob이나 search를 사용하거나 파일 경로를 지정하세요.",
		"%s — каталог, его нельзя прочитать как файл; используйте glob или search либо укажите путь к файлу.")
	add(KeyToolInspectPartialReason,
		"The source was incomplete (reason: %s).",
		"来源不完整（原因：%s）。",
		"Die Quelle war unvollständig (Grund: %s).",
		"ソースが不完全です（理由: %s）。",
		"소스가 불완전합니다(원인: %s).",
		"Источник неполон (причина: %s).")
	add(KeyPresentationToolParameterSize,
		"parameters %s", "参数 %s", "Parameter %s", "パラメータ %s", "매개변수 %s", "параметры %s")
	add(KeyPresentationToolPatchChanges,
		"changes %d files / +%d -%d", "变更 %d 个文件 / +%d -%d", "Änderungen %d Dateien / +%d -%d", "変更 %d ファイル / +%d -%d", "변경 파일 %d개 / +%d -%d", "изменения: %d файлов / +%d -%d")
	add(KeyPresentationToolReceiptSize,
		"receipt %s", "回执 %s", "Beleg %s", "受領結果 %s", "영수 결과 %s", "квитанция %s")
	add(KeyPresentationInspectPartialFailures,
		"%d request(s) failed", "%d 项请求失败", "%d Anfrage(n) fehlgeschlagen", "%d 件のリクエストが失敗", "요청 %d개 실패", "ошибок запросов: %d")
	add(KeyPresentationInspectPartialFailure,
		"%s (%s) %s: %s", "%s（%s）%s：%s", "%s (%s) %s: %s", "%s（%s）%s: %s", "%s(%s) %s: %s", "%s (%s) %s: %s")
	add(KeyPresentationInspectOtherSuccessful,
		"%d other request(s) succeeded", "另有 %d 项请求成功", "%d weitere Anfrage(n) erfolgreich", "ほか %d 件のリクエストが成功", "그 밖의 요청 %d개 성공", "ещё %d запрос(а/ов) выполнено")
}
