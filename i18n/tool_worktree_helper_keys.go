package i18n

const (
	KeyToolWorktreeNameTooLong           Key = "tool.worktree.helper.name_too_long"
	KeyToolWorktreeNamePathSegment       Key = "tool.worktree.helper.name_path_segment"
	KeyToolWorktreeNameCharacters        Key = "tool.worktree.helper.name_characters"
	KeyToolWorktreeWorkingDirectoryEmpty Key = "tool.worktree.helper.working_directory_empty"
	KeyToolWorktreePRNumberInvalid       Key = "tool.worktree.helper.pr_number_invalid"
	KeyToolWorktreePRReferenceInvalid    Key = "tool.worktree.helper.pr_reference_invalid"
	KeyToolWorktreePRReferenceNil        Key = "tool.worktree.helper.pr_reference_nil"
	KeyToolWorktreePRFetchFailed         Key = "tool.worktree.helper.pr_fetch_failed"
	KeyToolWorktreePathClaimed           Key = "tool.worktree.helper.path_claimed"
	KeyToolWorktreeListFailed            Key = "tool.worktree.helper.list_failed"
	KeyToolWorktreeRepositoryRootEmpty   Key = "tool.worktree.helper.repository_root_empty"
	KeyToolWorktreeBranchMismatch        Key = "tool.worktree.helper.branch_mismatch"
	KeyToolWorktreePathUnregistered      Key = "tool.worktree.helper.path_unregistered"
	KeyToolWorktreeInspectPathFailed     Key = "tool.worktree.helper.inspect_path_failed"
	KeyToolWorktreeCreateParentFailed    Key = "tool.worktree.helper.create_parent_failed"
	KeyToolWorktreeResolveBaseRefFailed  Key = "tool.worktree.helper.resolve_base_ref_failed"
	KeyToolWorktreeCreateFailed          Key = "tool.worktree.helper.create_failed"
	KeyToolWorktreeRollbackFailed        Key = "tool.worktree.helper.rollback_failed"
	KeyToolWorktreeRolledBack            Key = "tool.worktree.helper.rolled_back"
	KeyToolWorktreeInspectChangesFailed  Key = "tool.worktree.helper.inspect_changes_failed"
	KeyToolWorktreeUncommittedChanges    Key = "tool.worktree.helper.uncommitted_changes"
	KeyToolWorktreeRemoveFailed          Key = "tool.worktree.helper.remove_failed"
)

var toolWorktreeHelperKeys = []Key{
	KeyToolWorktreeNameTooLong,
	KeyToolWorktreeNamePathSegment,
	KeyToolWorktreeNameCharacters,
	KeyToolWorktreeWorkingDirectoryEmpty,
	KeyToolWorktreePRNumberInvalid,
	KeyToolWorktreePRReferenceInvalid,
	KeyToolWorktreePRReferenceNil,
	KeyToolWorktreePRFetchFailed,
	KeyToolWorktreePathClaimed,
	KeyToolWorktreeListFailed,
	KeyToolWorktreeRepositoryRootEmpty,
	KeyToolWorktreeBranchMismatch,
	KeyToolWorktreePathUnregistered,
	KeyToolWorktreeInspectPathFailed,
	KeyToolWorktreeCreateParentFailed,
	KeyToolWorktreeResolveBaseRefFailed,
	KeyToolWorktreeCreateFailed,
	KeyToolWorktreeRollbackFailed,
	KeyToolWorktreeRolledBack,
	KeyToolWorktreeInspectChangesFailed,
	KeyToolWorktreeUncommittedChanges,
	KeyToolWorktreeRemoveFailed,
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

	add(KeyToolWorktreeNameTooLong,
		"Invalid worktree name: must be %d characters or fewer (got %d)",
		"worktree 名称最多只能包含 %d 个字符（当前为 %d 个）",
		"Ungültiger Worktree-Name: Er darf höchstens %d Zeichen lang sein (aktuell %d)",
		"worktree 名が無効です。%d 文字以下にしてください（現在 %d 文字）",
		"잘못된 worktree 이름입니다. %d자 이하여야 합니다(현재 %d자)",
		"Недопустимое имя worktree: не более %d символов (сейчас %d)")
	add(KeyToolWorktreeNamePathSegment,
		"Invalid worktree name %q: must not contain \".\" or \"..\" path segments",
		"worktree 名称 %q 无效：路径片段不能是 \".\" 或 \"..\"",
		"Ungültiger Worktree-Name %q: Die Pfadsegmente \".\" und \"..\" sind nicht zulässig",
		"worktree 名 %q が無効です。パス要素 \".\" と \"..\" は使用できません",
		"잘못된 worktree 이름 %q: 경로 세그먼트로 \".\" 또는 \"..\"을 사용할 수 없습니다",
		"Недопустимое имя worktree %q: сегменты пути \".\" и \"..\" запрещены")
	add(KeyToolWorktreeNameCharacters,
		"Invalid worktree name %q: each \"/\"-separated segment must be non-empty and contain only letters, digits, dots, underscores, and dashes",
		"worktree 名称 %q 无效：以 \"/\" 分隔的每个片段都不能为空，且只能包含字母、数字、点、下划线和连字符",
		"Ungültiger Worktree-Name %q: Jedes durch \"/\" getrennte Segment muss ausgefüllt sein und darf nur Buchstaben, Ziffern, Punkte, Unterstriche und Bindestriche enthalten",
		"worktree 名 %q が無効です。\"/\" で区切られた各要素は空にできず、英数字、ピリオド、アンダースコア、ハイフンのみ使用できます",
		"잘못된 worktree 이름 %q: \"/\"로 구분된 각 세그먼트는 비어 있지 않아야 하며 문자, 숫자, 마침표, 밑줄, 하이픈만 포함할 수 있습니다",
		"Недопустимое имя worktree %q: каждый сегмент, разделённый \"/\", должен быть непустым и содержать только буквы, цифры, точки, знаки подчёркивания и дефисы")
	add(KeyToolWorktreeWorkingDirectoryEmpty,
		"working directory is empty",
		"当前工作目录为空",
		"Das aktuelle Arbeitsverzeichnis ist leer",
		"現在の作業ディレクトリが空です",
		"현재 작업 디렉터리가 비어 있습니다",
		"Текущий рабочий каталог пуст")
	add(KeyToolWorktreePRNumberInvalid,
		"invalid PR number in %q",
		"%q 中的 PR 编号无效",
		"Ungültige PR-Nummer in %q",
		"%q の PR 番号が無効です",
		"%q의 PR 번호가 올바르지 않습니다",
		"Недопустимый номер PR в %q")
	add(KeyToolWorktreePRReferenceInvalid,
		"unrecognised PR reference %q (expected `pr:<num>` or `pr:<owner>/<repo>#<num>`)",
		"无法识别 PR 引用 %q（应为 `pr:<num>` 或 `pr:<owner>/<repo>#<num>`）",
		"Unbekannte PR-Referenz %q (erwartet wird `pr:<num>` oder `pr:<owner>/<repo>#<num>`)",
		"PR 参照 %q を認識できません（`pr:<num>` または `pr:<owner>/<repo>#<num>` を指定してください）",
		"인식할 수 없는 PR 참조 %q입니다(`pr:<num>` 또는 `pr:<owner>/<repo>#<num>` 형식이어야 합니다)",
		"Нераспознанная ссылка на PR %q (ожидается `pr:<num>` или `pr:<owner>/<repo>#<num>`)")
	add(KeyToolWorktreePRReferenceNil,
		"nil PR ref",
		"PR 引用不能为空",
		"Die PR-Referenz darf nicht nil sein",
		"PR 参照が nil です",
		"PR 참조가 nil입니다",
		"Ссылка на PR равна nil")
	add(KeyToolWorktreePRFetchFailed,
		"git fetch %s %s failed: %s",
		"git fetch %s %s 失败：%s",
		"git fetch %s %s ist fehlgeschlagen: %s",
		"git fetch %s %s に失敗しました: %s",
		"git fetch %s %s에 실패했습니다: %s",
		"Не удалось выполнить git fetch %s %s: %s")
	add(KeyToolWorktreePathClaimed,
		"worktree %q is active in session %q",
		"worktree %q 已在会话 %q 中使用",
		"Worktree %q ist in Sitzung %q aktiv",
		"worktree %q はセッション %q で使用中です",
		"worktree %q은(는) 세션 %q에서 사용 중입니다",
		"Worktree %q активен в сеансе %q")
	add(KeyToolWorktreeListFailed,
		"git worktree list failed: %s",
		"git worktree list 失败：%s",
		"git worktree list ist fehlgeschlagen: %s",
		"git worktree list に失敗しました: %s",
		"git worktree list에 실패했습니다: %s",
		"Не удалось выполнить git worktree list: %s")
	add(KeyToolWorktreeRepositoryRootEmpty,
		"canonical repository root is empty",
		"规范化后的仓库根目录为空",
		"Das kanonische Repository-Stammverzeichnis ist leer",
		"正規化されたリポジトリのルートが空です",
		"정규화된 저장소 루트가 비어 있습니다",
		"Канонический корневой каталог репозитория пуст")
	add(KeyToolWorktreeBranchMismatch,
		"worktree path %q is registered for branch %q, expected %q",
		"worktree 路径 %q 已注册到 branch %q，但预期为 %q",
		"Der Worktree-Pfad %q ist für Branch %q registriert, erwartet wurde %q",
		"worktree パス %q は branch %q に登録されていますが、%q が必要です",
		"worktree 경로 %q은(는) branch %q에 등록되어 있지만 예상 branch는 %q입니다",
		"Путь worktree %q зарегистрирован для branch %q, ожидался %q")
	add(KeyToolWorktreePathUnregistered,
		"worktree path %q already exists but is not registered with git",
		"worktree 路径 %q 已存在，但尚未注册到 git",
		"Der Worktree-Pfad %q ist bereits vorhanden, aber nicht bei git registriert",
		"worktree パス %q は既に存在しますが、git に登録されていません",
		"worktree 경로 %q이(가) 이미 존재하지만 git에 등록되어 있지 않습니다",
		"Путь worktree %q уже существует, но не зарегистрирован в git")
	add(KeyToolWorktreeInspectPathFailed,
		"inspect worktree path %q: %v",
		"检查 worktree 路径 %q 时失败：%v",
		"Worktree-Pfad %q konnte nicht geprüft werden: %v",
		"worktree パス %q を確認できませんでした: %v",
		"worktree 경로 %q을(를) 확인하지 못했습니다: %v",
		"Не удалось проверить путь worktree %q: %v")
	add(KeyToolWorktreeCreateParentFailed,
		"create worktree parent: %v",
		"创建 worktree 父目录时失败：%v",
		"Das übergeordnete Worktree-Verzeichnis konnte nicht erstellt werden: %v",
		"worktree の親ディレクトリを作成できませんでした: %v",
		"worktree 상위 디렉터리를 만들지 못했습니다: %v",
		"Не удалось создать родительский каталог worktree: %v")
	add(KeyToolWorktreeResolveBaseRefFailed,
		"failed to resolve base ref %q: %s",
		"无法解析 base ref %q：%s",
		"Base Ref %q konnte nicht aufgelöst werden: %s",
		"base ref %q を解決できませんでした: %s",
		"base ref %q을(를) 확인하지 못했습니다: %s",
		"Не удалось разрешить base ref %q: %s")
	add(KeyToolWorktreeCreateFailed,
		"failed to create worktree: %s",
		"创建 worktree 失败：%s",
		"Worktree konnte nicht erstellt werden: %s",
		"worktree を作成できませんでした: %s",
		"worktree를 만들지 못했습니다: %s",
		"Не удалось создать worktree: %s")
	add(KeyToolWorktreeRollbackFailed,
		"%[2]v; rollback failed: %[1]s",
		"%[2]v；回滚失败：%[1]s",
		"%[2]v; Rollback fehlgeschlagen: %[1]s",
		"%[2]v。ロールバックにも失敗しました: %[1]s",
		"%[2]v. 롤백에도 실패했습니다: %[1]s",
		"%[2]v; откат также завершился ошибкой: %[1]s")
	add(KeyToolWorktreeRolledBack,
		"%v; worktree was rolled back",
		"%v；worktree 已回滚",
		"%v; der Worktree wurde zurückgesetzt",
		"%v。worktree はロールバックされました",
		"%v. worktree가 롤백되었습니다",
		"%v; worktree был отменён")
	add(KeyToolWorktreeInspectChangesFailed,
		"inspect worktree changes: %s",
		"检查 worktree 变更时失败：%s",
		"Worktree-Änderungen konnten nicht geprüft werden: %s",
		"worktree の変更を確認できませんでした: %s",
		"worktree 변경 사항을 확인하지 못했습니다: %s",
		"Не удалось проверить изменения worktree: %s")
	add(KeyToolWorktreeUncommittedChanges,
		"worktree has uncommitted changes",
		"worktree 中有未提交的变更",
		"Der Worktree enthält nicht committete Änderungen",
		"worktree に未コミットの変更があります",
		"worktree에 커밋하지 않은 변경 사항이 있습니다",
		"В worktree есть незакоммиченные изменения")
	add(KeyToolWorktreeRemoveFailed,
		"failed to remove worktree: %s",
		"移除 worktree 失败：%s",
		"Worktree konnte nicht entfernt werden: %s",
		"worktree を削除できませんでした: %s",
		"worktree를 제거하지 못했습니다: %s",
		"Не удалось удалить worktree: %s")
}
