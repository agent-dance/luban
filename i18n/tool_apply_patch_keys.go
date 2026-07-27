package i18n

// Semantic model-facing and runtime copy for the ApplyPatch core tool.
const (
	KeyToolApplyPatchDescription           Key = "tool.apply_patch.description"
	KeyToolApplyPatchCustomDescription     Key = "tool.apply_patch.custom.description"
	KeyToolApplyPatchInputPatchDescription Key = "tool.apply_patch.input.patch.description"
	KeyToolApplyPatchPlanMode              Key = "tool.apply_patch.runtime.plan_mode"
	KeyToolApplyPatchParseFailed           Key = "tool.apply_patch.runtime.parse_failed"
	KeyToolApplyPatchConflict              Key = "tool.apply_patch.runtime.conflict"
	KeyToolApplyPatchReadRequired          Key = "tool.apply_patch.runtime.read_required"
	KeyToolApplyPatchPermissionDenied      Key = "tool.apply_patch.runtime.permission_denied"
	KeyToolApplyPatchCommitFailed          Key = "tool.apply_patch.runtime.commit_failed"
	KeyToolApplyPatchRevisionReceiptFailed Key = "tool.apply_patch.runtime.revision_receipt_failed"
	KeyToolApplyPatchInvalidResult         Key = "tool.apply_patch.runtime.invalid_result"
	KeyToolApplyPatchSuccess               Key = "tool.apply_patch.runtime.success"
	KeyToolApplyPatchPermissionPrompt      Key = "tool.apply_patch.permission.prompt"
	KeyToolApplyPatchPermissionInvalid     Key = "tool.apply_patch.permission.invalid"
)

var toolApplyPatchKeys = [...]Key{
	KeyToolApplyPatchDescription,
	KeyToolApplyPatchCustomDescription,
	KeyToolApplyPatchInputPatchDescription,
	KeyToolApplyPatchPlanMode,
	KeyToolApplyPatchParseFailed,
	KeyToolApplyPatchConflict,
	KeyToolApplyPatchReadRequired,
	KeyToolApplyPatchPermissionDenied,
	KeyToolApplyPatchCommitFailed,
	KeyToolApplyPatchRevisionReceiptFailed,
	KeyToolApplyPatchInvalidResult,
	KeyToolApplyPatchSuccess,
	KeyToolApplyPatchPermissionPrompt,
	KeyToolApplyPatchPermissionInvalid,
}

func init() {
	addApplyPatchCopy := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}

	addApplyPatchCopy(KeyToolApplyPatchDescription,
		"ApplyPatch applies one patch across multiple files in a single validated transaction. Accepts `*** Begin Patch` or unified diff syntax, including multiple hunk sections and create, update, or delete operations. Paths must be project-relative. Renames and symbolic links are not supported. Existing files are matched against exact patch context and protected by a snapshot check at commit time.",
		"ApplyPatch 在一个经过完整校验的事务中对多个文件应用补丁。支持 `*** Begin Patch` 或 unified diff 语法，可包含多个 hunk，以及创建、更新或删除操作。路径必须相对于项目。暂不支持重命名和符号链接。现有文件会与补丁上下文精确匹配，并在提交时接受快照校验。",
		"ApplyPatch wendet einen Patch in einer vollständig geprüften Transaktion auf mehrere Dateien an. Unterstützt `*** Begin Patch` und unified diff mit mehreren hunk-Abschnitten sowie Erstellen, Aktualisieren und Löschen. Pfade müssen relativ zum Projekt sein. Umbenennungen und symbolische Links werden nicht unterstützt. Vorhandene Dateien werden exakt mit dem Patch-Kontext abgeglichen und beim Commit durch eine Snapshot-Prüfung geschützt.",
		"ApplyPatch は検証済みの単一トランザクションで複数ファイルにパッチを適用します。`*** Begin Patch` または unified diff 構文、複数の hunk、作成・更新・削除に対応します。パスはプロジェクト相対で指定してください。名前変更とシンボリックリンクには対応していません。既存ファイルはパッチのコンテキストと厳密に照合され、commit 時に snapshot 検証で保護されます。",
		"ApplyPatch는 완전히 검증된 단일 트랜잭션으로 여러 파일에 패치를 적용합니다. `*** Begin Patch` 또는 unified diff 구문, 여러 hunk, 생성·수정·삭제 작업을 지원합니다. 경로는 프로젝트 기준 상대 경로여야 합니다. 이름 변경과 symbolic link는 지원하지 않습니다. 기존 파일은 패치 문맥과 정확히 대조되며 commit 시 snapshot 검증으로 보호됩니다.",
		"ApplyPatch применяет один patch к нескольким файлам в рамках единой проверенной транзакции. Поддерживает синтаксис `*** Begin Patch` и unified diff, несколько hunk, а также создание, обновление и удаление. Пути должны быть заданы относительно проекта. Переименование и символические ссылки не поддерживаются. Существующие файлы точно сверяются с контекстом patch и защищаются проверкой snapshot при commit.")
	addApplyPatchCopy(KeyToolApplyPatchCustomDescription,
		"ApplyPatch applies one freeform `*** Begin Patch` payload across multiple project-relative files in a single validated transaction. Emit the patch directly, without JSON or Markdown fences. Include every create, update, delete, and hunk in the same call. Renames and symbolic links are not supported. Existing files are matched against exact context and protected by a snapshot check at commit time.",
		"ApplyPatch 在一个经过完整校验的事务中，将一份自由格式的 `*** Begin Patch` 内容应用到多个项目相对路径。请直接输出补丁，不要使用 JSON 或 Markdown 代码围栏。所有创建、更新、删除和 hunk 都应放在同一次调用中。暂不支持重命名和符号链接。现有文件会与精确上下文匹配，并在提交时接受快照校验。",
		"ApplyPatch wendet einen frei formulierten `*** Begin Patch`-Inhalt in einer geprüften Transaktion auf mehrere projekt-relative Dateien an. Gib den Patch direkt aus, ohne JSON oder Markdown-Codeblock. Fasse alle Erstell-, Änderungs- und Löschvorgänge sowie Hunks in demselben Aufruf zusammen. Umbenennungen und symbolische Links werden nicht unterstützt. Vorhandene Dateien werden mit exaktem Kontext abgeglichen und beim Commit durch eine Snapshot-Prüfung geschützt.",
		"ApplyPatch は自由形式の `*** Begin Patch` 形式を、検証済みの単一トランザクションで複数のプロジェクト相対ファイルに適用します。JSON や Markdown のコードフェンスを使わず、パッチを直接出力してください。作成・更新・削除とすべての hunk を同じ呼び出しに含めます。名前変更とシンボリックリンクには対応していません。既存ファイルは正確なコンテキストと照合され、commit 時の snapshot 検証で保護されます。",
		"ApplyPatch는 자유 형식의 `*** Begin Patch` 내용을 검증된 단일 트랜잭션으로 여러 프로젝트 상대 경로 파일에 적용합니다. JSON이나 Markdown 코드 펜스 없이 패치를 직접 출력하세요. 모든 생성, 수정, 삭제 및 hunk를 같은 호출에 포함해야 합니다. 이름 변경과 symbolic link는 지원하지 않습니다. 기존 파일은 정확한 문맥과 대조되고 commit 시 snapshot 검증으로 보호됩니다.",
		"ApplyPatch применяет свободно сформированный payload `*** Begin Patch` к нескольким путям относительно проекта в одной проверенной транзакции. Выводите patch напрямую, без JSON и блоков Markdown. Включайте все операции создания, изменения, удаления и все hunk в один вызов. Переименование и символические ссылки не поддерживаются. Существующие файлы сверяются с точным контекстом и защищаются проверкой snapshot при commit.")
	addApplyPatchCopy(KeyToolApplyPatchInputPatchDescription,
		"The complete `*** Begin Patch` or unified diff payload. Include all files and hunks in this one value.",
		"完整的 `*** Begin Patch` 或 unified diff 内容。请在这一个值中包含全部文件和 hunk。",
		"Der vollständige Inhalt im Format `*** Begin Patch` oder Unified Diff. Nimm alle Dateien und Hunks in diesen einen Wert auf.",
		"完全な `*** Begin Patch` または unified diff の内容です。この 1 つの値にすべてのファイルと hunk を含めてください。",
		"완전한 `*** Begin Patch` 또는 unified diff 내용입니다. 모든 파일과 hunk를 이 값 하나에 포함하세요.",
		"Полное содержимое в формате `*** Begin Patch` или unified diff. Включите в одно это значение все файлы и hunk.")
	addApplyPatchCopy(KeyToolApplyPatchPlanMode,
		"ApplyPatch cannot modify files while plan mode is active.",
		"计划模式启用时，ApplyPatch 无法修改文件。",
		"ApplyPatch kann keine Dateien ändern, solange der Planungsmodus aktiv ist.",
		"plan mode が有効な間、ApplyPatch はファイルを変更できません。",
		"plan mode가 활성화된 동안 ApplyPatch는 파일을 수정할 수 없습니다.",
		"ApplyPatch не может изменять файлы, пока активен режим планирования.")
	addApplyPatchCopy(KeyToolApplyPatchParseFailed,
		"The patch could not be parsed (reason: %s, path: %s).",
		"无法解析补丁（原因：%s，路径：%s）。",
		"Der Patch konnte nicht geparst werden (Grund: %s, Pfad: %s).",
		"パッチを解析できませんでした（理由: %s、パス: %s）。",
		"패치를 구문 분석할 수 없습니다(원인: %s, 경로: %s).",
		"Не удалось разобрать patch (причина: %s, путь: %s).")
	addApplyPatchCopy(KeyToolApplyPatchConflict,
		"The patch does not match the current snapshot of %s. Refresh the context and retry.",
		"补丁与 %s 的当前快照不匹配。请刷新上下文后重试。",
		"Der Patch stimmt nicht mit dem aktuellen Snapshot von %s überein. Aktualisiere den Kontext und versuche es erneut.",
		"パッチが %s の現在の snapshot と一致しません。コンテキストを更新して再試行してください。",
		"패치가 %s의 현재 snapshot과 일치하지 않습니다. 문맥을 새로 고친 뒤 다시 시도하세요.",
		"Patch не соответствует текущему snapshot файла %s. Обновите контекст и повторите попытку.")
	addApplyPatchCopy(KeyToolApplyPatchReadRequired,
		"A complete Read of %s is required before a context-free delete.",
		"执行无上下文删除前，必须先完整 Read %s。",
		"Vor einer kontextfreien Löschung ist ein vollständiger Read von %s erforderlich.",
		"コンテキストなしで削除する前に、%s を完全に Read する必要があります。",
		"문맥 없이 삭제하기 전에 %s를 완전히 Read해야 합니다.",
		"Перед удалением без контекста необходимо полностью выполнить Read для %s.")
	addApplyPatchCopy(KeyToolApplyPatchPermissionDenied,
		"ApplyPatch cannot modify %s under the current path policy (reason: %s).",
		"根据当前路径策略，ApplyPatch 无法修改 %s（原因：%s）。",
		"ApplyPatch darf %s gemäß der aktuellen Pfadrichtlinie nicht ändern (Grund: %s).",
		"現在のパスポリシーでは、ApplyPatch は %s を変更できません（理由: %s）。",
		"현재 경로 정책에 따라 ApplyPatch는 %s을(를) 수정할 수 없습니다(원인: %s).",
		"Текущая политика путей не позволяет ApplyPatch изменить %s (причина: %s).")
	addApplyPatchCopy(KeyToolApplyPatchCommitFailed,
		"The ApplyPatch transaction could not be committed (reason: %s).",
		"ApplyPatch 事务无法提交（原因：%s）。",
		"Die ApplyPatch-Transaktion konnte nicht committet werden (Grund: %s).",
		"ApplyPatch トランザクションを commit できませんでした（理由: %s）。",
		"ApplyPatch 트랜잭션을 commit할 수 없습니다(원인: %s).",
		"Не удалось выполнить commit транзакции ApplyPatch (причина: %s).")
	addApplyPatchCopy(KeyToolApplyPatchRevisionReceiptFailed,
		"The patch was applied, but its workspace revision could not be certified; verification was not started.",
		"补丁已应用，但无法认证其工作区版本；验证尚未启动。",
		"Der Patch wurde angewendet, aber seine Workspace-Revision konnte nicht bestätigt werden; die Verifizierung wurde nicht gestartet.",
		"パッチは適用されましたが、workspace revision を証明できなかったため、検証は開始されませんでした。",
		"패치는 적용되었지만 workspace revision을 인증할 수 없어 검증을 시작하지 않았습니다.",
		"Patch применён, но подтвердить его ревизию рабочей области не удалось; проверка не запускалась.")
	addApplyPatchCopy(KeyToolApplyPatchInvalidResult,
		"ApplyPatch returned an invalid result.",
		"ApplyPatch 返回了无效结果。",
		"ApplyPatch hat ein ungültiges Ergebnis zurückgegeben.",
		"ApplyPatch が無効な結果を返しました。",
		"ApplyPatch가 올바르지 않은 결과를 반환했습니다.",
		"ApplyPatch вернул недопустимый результат.")
	addApplyPatchCopy(KeyToolApplyPatchSuccess,
		"Applied patch: %d files changed, %d insertions(+), %d deletions(-).",
		"补丁已应用：变更 %d 个文件，新增 %d 行，删除 %d 行。",
		"Patch angewendet: %d Dateien geändert, %d Einfügungen(+), %d Löschungen(-).",
		"パッチを適用しました: %d ファイルを変更、%d 行を追加、%d 行を削除。",
		"패치를 적용했습니다: 파일 %d개 변경, %d줄 추가, %d줄 삭제.",
		"Patch применён: изменено файлов — %d, добавлено строк — %d, удалено строк — %d.")
	addApplyPatchCopy(KeyToolApplyPatchPermissionPrompt,
		"ApplyPatch requires permission to modify %d files; first target: %s.",
		"ApplyPatch 需要授权才能修改 %d 个文件；首个目标：%s。",
		"ApplyPatch benötigt eine Berechtigung zum Ändern von %d Dateien; erstes Ziel: %s.",
		"ApplyPatch で %d 個のファイルを変更するには許可が必要です。最初の対象: %s。",
		"ApplyPatch로 파일 %d개를 수정하려면 권한이 필요합니다. 첫 대상: %s.",
		"ApplyPatch требуется разрешение для изменения %d файлов; первая цель: %s.")
	addApplyPatchCopy(KeyToolApplyPatchPermissionInvalid,
		"ApplyPatch permission validation failed (reason: %s).",
		"ApplyPatch 权限校验失败（原因：%s）。",
		"Die Berechtigungsprüfung für ApplyPatch ist fehlgeschlagen (Grund: %s).",
		"ApplyPatch の権限検証に失敗しました（理由: %s）。",
		"ApplyPatch 권한 검증에 실패했습니다(원인: %s).",
		"Проверка разрешений ApplyPatch завершилась ошибкой (причина: %s).")
}
