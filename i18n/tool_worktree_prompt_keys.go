package i18n

const (
	KeyToolPromptWorktreeEnterDescription Key = "tool.prompt.worktree.enter.description"
	KeyToolPromptWorktreeName             Key = "tool.prompt.worktree.name"
	KeyToolPromptWorktreePath             Key = "tool.prompt.worktree.path"
	KeyToolPromptWorktreeBaseRef          Key = "tool.prompt.worktree.base_ref"
	KeyToolPromptWorktreeExitDescription  Key = "tool.prompt.worktree.exit.description"
	KeyToolPromptWorktreeAction           Key = "tool.prompt.worktree.action"
	KeyToolPromptWorktreeDiscardChanges   Key = "tool.prompt.worktree.discard_changes"
)

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}
	add(KeyToolPromptWorktreeEnterDescription,
		"Create an isolated worktree with git or configured hooks, then switch this session into it.",
		"通过 git 或已配置的 hook 创建隔离 worktree，并将当前会话切换到其中。",
		"Erstellt mit git oder konfigurierten Hooks einen isolierten Worktree und wechselt diese Sitzung dorthin.",
		"git または設定済みの hook で分離された worktree を作成し、このセッションをそこへ切り替えます。",
		"git 또는 구성된 hook으로 격리된 worktree를 만들고 현재 세션을 해당 worktree로 전환합니다.",
		"Создаёт изолированный worktree с помощью git или настроенных hooks и переключает в него текущий сеанс.")
	add(KeyToolPromptWorktreeName,
		"Optional worktree name. Each slash-separated segment may contain only letters, digits, dots, underscores, and dashes; the total length is at most 64 characters. A random name is generated when omitted.",
		"可选的 worktree 名称。以斜杠分隔的每个片段只能包含字母、数字、点、下划线和连字符，总长度最多为 64 个字符；省略时会随机生成名称。",
		"Optionaler Worktree-Name. Jedes durch Schrägstriche getrennte Segment darf nur Buchstaben, Ziffern, Punkte, Unterstriche und Bindestriche enthalten; die Gesamtlänge beträgt höchstens 64 Zeichen. Ohne Angabe wird ein zufälliger Name erzeugt.",
		"省略可能な worktree 名です。スラッシュで区切られた各要素には英数字、ピリオド、アンダースコア、ハイフンのみ使用でき、全体で 64 文字以内です。省略するとランダムな名前が生成されます。",
		"선택적 worktree 이름입니다. 슬래시로 구분된 각 세그먼트에는 문자, 숫자, 마침표, 밑줄, 하이픈만 사용할 수 있고 전체 길이는 최대 64자입니다. 생략하면 임의의 이름이 생성됩니다.",
		"Необязательное имя worktree. Каждый сегмент, разделённый косой чертой, может содержать только буквы, цифры, точки, знаки подчёркивания и дефисы; общая длина — не более 64 символов. Если имя не указано, оно создаётся автоматически.")
	add(KeyToolPromptWorktreePath,
		"Path to an existing worktree of the current repository. Cannot be combined with name.",
		"当前仓库中现有 worktree 的路径，不能与 name 同时使用。",
		"Pfad zu einem vorhandenen Worktree des aktuellen Repositorys. Kann nicht zusammen mit name verwendet werden.",
		"現在のリポジトリにある既存 worktree のパスです。name とは同時に指定できません。",
		"현재 저장소의 기존 worktree 경로입니다. name과 함께 사용할 수 없습니다.",
		"Путь к существующему worktree текущего репозитория. Нельзя использовать вместе с name.")
	add(KeyToolPromptWorktreeBaseRef,
		"Base for a new worktree: fresh, head, pr:<number>, or pr:<owner>/<repo>#<number>.",
		"新 worktree 的基准：fresh、head、pr:<number> 或 pr:<owner>/<repo>#<number>。",
		"Basis für einen neuen Worktree: fresh, head, pr:<number> oder pr:<owner>/<repo>#<number>.",
		"新しい worktree の基点です。fresh、head、pr:<number>、pr:<owner>/<repo>#<number> のいずれかを指定します。",
		"새 worktree의 기준입니다: fresh, head, pr:<number> 또는 pr:<owner>/<repo>#<number>.",
		"Основа нового worktree: fresh, head, pr:<number> или pr:<owner>/<repo>#<number>.")
	add(KeyToolPromptWorktreeExitDescription,
		"Exit the worktree session created by EnterWorktree and restore the original working directory.",
		"退出由 EnterWorktree 创建的 worktree 会话，并恢复原工作目录。",
		"Beendet die von EnterWorktree erstellte Worktree-Sitzung und stellt das ursprüngliche Arbeitsverzeichnis wieder her.",
		"EnterWorktree で作成した worktree セッションを終了し、元の作業ディレクトリを復元します。",
		"EnterWorktree가 만든 worktree 세션을 종료하고 원래 작업 디렉터리를 복원합니다.",
		"Завершает сеанс worktree, созданный EnterWorktree, и восстанавливает исходный рабочий каталог.")
	add(KeyToolPromptWorktreeAction,
		"Use keep to leave the worktree and branch on disk, or remove to delete both.",
		"使用 keep 将 worktree 和分支保留在磁盘上；使用 remove 删除二者。",
		"Mit keep bleiben Worktree und Branch auf dem Datenträger; mit remove werden beide gelöscht.",
		"keep は worktree とブランチをディスクに残し、remove は両方を削除します。",
		"keep은 worktree와 브랜치를 디스크에 유지하고, remove는 둘 다 삭제합니다.",
		"keep оставляет worktree и ветку на диске, а remove удаляет их.")
	add(KeyToolPromptWorktreeDiscardChanges,
		"Must be true when remove would discard uncommitted files or commits on the worktree branch. Otherwise the tool refuses removal and reports the pending work.",
		"当 remove 会丢弃未提交文件或 worktree 分支上的 commit 时必须设为 true；否则工具会拒绝移除并报告待处理工作。",
		"Muss true sein, wenn remove nicht committete Dateien oder Commits auf dem Worktree-Branch verwerfen würde. Andernfalls verweigert das Tool das Entfernen und meldet die ausstehenden Änderungen.",
		"remove によって未コミットのファイルまたは worktree ブランチ上の commit が破棄される場合は true にする必要があります。それ以外では削除を拒否し、保留中の作業を報告します。",
		"remove가 커밋되지 않은 파일이나 worktree 브랜치의 commit을 폐기하는 경우 true여야 합니다. 그렇지 않으면 도구가 제거를 거부하고 보류 중인 작업을 보고합니다.",
		"Должно быть true, если remove отбросит незакоммиченные файлы или коммиты в ветке worktree. Иначе инструмент откажется от удаления и сообщит о незавершённой работе.")
}
