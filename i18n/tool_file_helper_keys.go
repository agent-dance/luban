package i18n

// Semantic copy returned by filesystem helpers whose errors flow into
// user-visible file tool results. Paths, settings fields, concrete types, and
// underlying filesystem/schema errors remain raw format arguments.
const (
	KeyToolFileHelperStatFailed                 Key = "tool.file.helper.stat_failed"
	KeyToolFileHelperResolveSymlinkFailed       Key = "tool.file.helper.resolve_symlink_failed"
	KeyToolFileHelperPathIsDirectory            Key = "tool.file.helper.path_is_directory"
	KeyToolFileHelperReadFailed                 Key = "tool.file.helper.read_failed"
	KeyToolFileHelperWriteTargetOutsideAllowed  Key = "tool.file.helper.write_target_outside_allowed"
	KeyToolFileHelperCreatedAfterCheck          Key = "tool.file.helper.created_after_check"
	KeyToolFileHelperRecheckWriteTargetFailed   Key = "tool.file.helper.recheck_write_target_failed"
	KeyToolFileHelperSymlinkTargetChanged       Key = "tool.file.helper.symlink_target_changed"
	KeyToolFileHelperWriteTargetReplaced        Key = "tool.file.helper.write_target_replaced"
	KeyToolFileHelperEditTargetReplacedBefore   Key = "tool.file.helper.edit_target_replaced_before_read"
	KeyToolFileHelperEditTargetChangedWhileRead Key = "tool.file.helper.edit_target_changed_while_read"
	KeyToolFileHelperEditTargetOutsideAllowed   Key = "tool.file.helper.edit_target_outside_allowed"
	KeyToolFileHelperRecheckEditTargetFailed    Key = "tool.file.helper.recheck_edit_target_failed"
	KeyToolFileHelperEditThroughSymlink         Key = "tool.file.helper.edit_through_symlink"
	KeyToolFileHelperEditTargetReplacedAfter    Key = "tool.file.helper.edit_target_replaced_after_read"
	KeyToolFileHelperEditTargetChangedAfter     Key = "tool.file.helper.edit_target_changed_after_read"
	KeyToolFileHelperPathOutsideAllowed         Key = "tool.file.helper.path_outside_allowed"
	KeyToolFileHelperVerifyFDPathFailed         Key = "tool.file.helper.verify_fd_path_failed"
)

var toolFileHelperKeys = []Key{
	KeyToolFileHelperStatFailed,
	KeyToolFileHelperResolveSymlinkFailed,
	KeyToolFileHelperPathIsDirectory,
	KeyToolFileHelperReadFailed,
	KeyToolFileHelperWriteTargetOutsideAllowed,
	KeyToolFileHelperCreatedAfterCheck,
	KeyToolFileHelperRecheckWriteTargetFailed,
	KeyToolFileHelperSymlinkTargetChanged,
	KeyToolFileHelperWriteTargetReplaced,
	KeyToolFileHelperEditTargetReplacedBefore,
	KeyToolFileHelperEditTargetChangedWhileRead,
	KeyToolFileHelperEditTargetOutsideAllowed,
	KeyToolFileHelperRecheckEditTargetFailed,
	KeyToolFileHelperEditThroughSymlink,
	KeyToolFileHelperEditTargetReplacedAfter,
	KeyToolFileHelperEditTargetChangedAfter,
	KeyToolFileHelperPathOutsideAllowed,
	KeyToolFileHelperVerifyFDPathFailed,
}

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}

	add(KeyToolFileHelperStatFailed,
		"failed to stat file: %v",
		"无法获取文件状态：%v",
		"Dateistatus konnte nicht ermittelt werden: %v",
		"ファイルの状態を取得できませんでした: %v",
		"파일 상태를 확인할 수 없습니다: %v",
		"Не удалось получить сведения о файле: %v")
	add(KeyToolFileHelperResolveSymlinkFailed,
		"failed to resolve symlink %q: %v",
		"无法解析符号链接 %q：%v",
		"Symbolischer Link %q konnte nicht aufgelöst werden: %v",
		"シンボリックリンク %q を解決できませんでした: %v",
		"심볼릭 링크 %q을(를) 확인할 수 없습니다: %v",
		"Не удалось разрешить символическую ссылку %q: %v")
	add(KeyToolFileHelperPathIsDirectory,
		"path %q is a directory, not a file",
		"路径 %q 是目录，不是文件",
		"Pfad %q ist ein Verzeichnis, keine Datei",
		"パス %q はファイルではなくディレクトリです",
		"경로 %q은(는) 파일이 아니라 디렉터리입니다",
		"Путь %q ведёт к каталогу, а не к файлу")
	add(KeyToolFileHelperReadFailed,
		"failed to read file: %v",
		"无法读取文件：%v",
		"Datei konnte nicht gelesen werden: %v",
		"ファイルを読み込めませんでした: %v",
		"파일을 읽을 수 없습니다: %v",
		"Не удалось прочитать файл: %v")
	add(KeyToolFileHelperWriteTargetOutsideAllowed,
		"write target changed outside allowed directories: %v",
		"写入目标已变更到允许目录之外：%v",
		"Das Schreibziel wurde außerhalb der zulässigen Verzeichnisse geändert: %v",
		"書き込み先が許可されたディレクトリの外に変更されました: %v",
		"쓰기 대상이 허용된 디렉터리 밖으로 변경되었습니다: %v",
		"Цель записи была изменена и теперь находится вне разрешённых каталогов: %v")
	add(KeyToolFileHelperCreatedAfterCheck,
		"file was created after it was checked; read it before writing",
		"文件在检查后被创建；请在写入前先读取该文件",
		"Die Datei wurde nach der Prüfung erstellt; lies sie vor dem Schreiben ein",
		"確認後にファイルが作成されました。書き込む前に読み込んでください",
		"확인 후 파일이 생성되었습니다. 쓰기 전에 파일을 읽으세요",
		"Файл был создан после проверки; прочитайте его перед записью")
	add(KeyToolFileHelperRecheckWriteTargetFailed,
		"failed to recheck write target: %v",
		"无法重新检查写入目标：%v",
		"Schreibziel konnte nicht erneut geprüft werden: %v",
		"書き込み先を再確認できませんでした: %v",
		"쓰기 대상을 다시 확인할 수 없습니다: %v",
		"Не удалось повторно проверить цель записи: %v")
	add(KeyToolFileHelperSymlinkTargetChanged,
		"symlink target changed after it was read; read it again before writing",
		"符号链接的目标在读取后发生了变化；请在写入前重新读取",
		"Das Ziel des symbolischen Links wurde nach dem Lesen geändert; lies die Datei vor dem Schreiben erneut ein",
		"読み込み後にシンボリックリンクの参照先が変更されました。書き込む前にもう一度読み込んでください",
		"읽은 후 심볼릭 링크 대상이 변경되었습니다. 쓰기 전에 다시 읽으세요",
		"Цель символической ссылки изменилась после чтения; прочитайте файл повторно перед записью")
	add(KeyToolFileHelperWriteTargetReplaced,
		"file was replaced after it was read; read it again before writing",
		"文件在读取后被替换；请在写入前重新读取",
		"Die Datei wurde nach dem Lesen ersetzt; lies sie vor dem Schreiben erneut ein",
		"読み込み後にファイルが置き換えられました。書き込む前にもう一度読み込んでください",
		"읽은 후 파일이 교체되었습니다. 쓰기 전에 다시 읽으세요",
		"Файл был заменён после чтения; прочитайте его повторно перед записью")
	add(KeyToolFileHelperEditTargetReplacedBefore,
		"file was replaced before it could be read; read it again before editing",
		"文件在读取前被替换；请在编辑前重新读取",
		"Die Datei wurde ersetzt, bevor sie gelesen werden konnte; lies sie vor dem Bearbeiten erneut ein",
		"読み込む前にファイルが置き換えられました。編集する前にもう一度読み込んでください",
		"파일을 읽기 전에 교체되었습니다. 편집하기 전에 다시 읽으세요",
		"Файл был заменён до чтения; прочитайте его повторно перед редактированием")
	add(KeyToolFileHelperEditTargetChangedWhileRead,
		"file changed while it was being read; read it again before editing",
		"文件在读取过程中发生了变化；请在编辑前重新读取",
		"Die Datei wurde während des Lesens geändert; lies sie vor dem Bearbeiten erneut ein",
		"読み込み中にファイルが変更されました。編集する前にもう一度読み込んでください",
		"파일을 읽는 동안 변경되었습니다. 편집하기 전에 다시 읽으세요",
		"Файл изменился во время чтения; прочитайте его повторно перед редактированием")
	add(KeyToolFileHelperEditTargetOutsideAllowed,
		"edit target changed outside allowed directories: %v",
		"编辑目标已变更到允许目录之外：%v",
		"Das Bearbeitungsziel wurde außerhalb der zulässigen Verzeichnisse geändert: %v",
		"編集先が許可されたディレクトリの外に変更されました: %v",
		"편집 대상이 허용된 디렉터리 밖으로 변경되었습니다: %v",
		"Цель редактирования была изменена и теперь находится вне разрешённых каталогов: %v")
	add(KeyToolFileHelperRecheckEditTargetFailed,
		"failed to recheck edit target: %v",
		"无法重新检查编辑目标：%v",
		"Bearbeitungsziel konnte nicht erneut geprüft werden: %v",
		"編集先を再確認できませんでした: %v",
		"편집 대상을 다시 확인할 수 없습니다: %v",
		"Не удалось повторно проверить цель редактирования: %v")
	add(KeyToolFileHelperEditThroughSymlink,
		"refusing to edit through symlink %q",
		"拒绝通过符号链接 %q 编辑文件",
		"Bearbeitung über den symbolischen Link %q wird verweigert",
		"シンボリックリンク %q 経由の編集は拒否されました",
		"심볼릭 링크 %q을(를) 통한 편집을 거부합니다",
		"Редактирование через символическую ссылку %q запрещено")
	add(KeyToolFileHelperEditTargetReplacedAfter,
		"file was replaced after it was read; read it again before editing",
		"文件在读取后被替换；请在编辑前重新读取",
		"Die Datei wurde nach dem Lesen ersetzt; lies sie vor dem Bearbeiten erneut ein",
		"読み込み後にファイルが置き換えられました。編集する前にもう一度読み込んでください",
		"읽은 후 파일이 교체되었습니다. 편집하기 전에 다시 읽으세요",
		"Файл был заменён после чтения; прочитайте его повторно перед редактированием")
	add(KeyToolFileHelperEditTargetChangedAfter,
		"file changed after it was read; read it again before editing",
		"文件在读取后发生了变化；请在编辑前重新读取",
		"Die Datei wurde nach dem Lesen geändert; lies sie vor dem Bearbeiten erneut ein",
		"読み込み後にファイルが変更されました。編集する前にもう一度読み込んでください",
		"읽은 후 파일이 변경되었습니다. 편집하기 전에 다시 읽으세요",
		"Файл изменился после чтения; прочитайте его повторно перед редактированием")
	add(KeyToolFileHelperPathOutsideAllowed,
		"path is outside allowed directories (resolved to %s)",
		"路径位于允许目录之外（解析为 %s）",
		"Der Pfad liegt außerhalb der zulässigen Verzeichnisse (aufgelöst zu %s)",
		"パスは許可されたディレクトリの外です（解決先: %s）",
		"경로가 허용된 디렉터리 밖에 있습니다(확인된 경로: %s)",
		"Путь находится вне разрешённых каталогов (разрешён в %s)")
	add(KeyToolFileHelperVerifyFDPathFailed,
		"cannot verify fd path: %v",
		"无法验证文件描述符路径：%v",
		"Pfad des Dateideskriptors kann nicht geprüft werden: %v",
		"ファイル記述子のパスを確認できません: %v",
		"파일 디스크립터 경로를 확인할 수 없습니다: %v",
		"Не удалось проверить путь файлового дескриптора: %v")
}
