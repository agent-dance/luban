package i18n

// Semantic copy used by session-oriented slash commands. Command arguments,
// file paths, git output, skill metadata, and raw errors remain format values.
const (
	KeyCommandResumeStoreUnavailable      Key = "command.resume.store_unavailable"
	KeyCommandResumeListError             Key = "command.resume.list_error"
	KeyCommandResumeNone                  Key = "command.resume.none"
	KeyCommandResumeRecent                Key = "command.resume.recent"
	KeyCommandResumeEntry                 Key = "command.resume.entry"
	KeyCommandResumeSearchError           Key = "command.resume.search_error"
	KeyCommandResumeMultiple              Key = "command.resume.multiple"
	KeyCommandResumeLoadError             Key = "command.resume.load_error"
	KeyCommandResumeLoaded                Key = "command.resume.loaded"
	KeyCommandResumeTransitionUnavailable Key = "command.resume.transition_unavailable"

	KeyCommandRenameStoreUnavailable Key = "command.rename.store_unavailable"
	KeyCommandRenameGenerateFailed   Key = "command.rename.generate_failed"
	KeyCommandRenameError            Key = "command.rename.error"
	KeyCommandRenameSucceeded        Key = "command.rename.succeeded"

	KeyCommandForkUsage             Key = "command.fork.usage"
	KeyCommandForkPickerUnavailable Key = "command.fork.picker_unavailable"

	KeyCommandDiffSummary Key = "command.diff.summary"

	KeyCommandReviewGitMissing    Key = "command.review.git_missing"
	KeyCommandReviewNotRepository Key = "command.review.not_repository"
	KeyCommandReviewNoStaged      Key = "command.review.no_staged"
	KeyCommandReviewClean         Key = "command.review.clean"
	KeyCommandReviewAllChanges    Key = "command.review.all_changes"
	KeyCommandReviewStagedChanges Key = "command.review.staged_changes"

	KeyCommandSkillsUnavailable         Key = "command.skills.unavailable"
	KeyCommandSkillsShowUsage           Key = "command.skills.show_usage"
	KeyCommandSkillsNotFound            Key = "command.skills.not_found"
	KeyCommandSkillsRefreshUsage        Key = "command.skills.refresh_usage"
	KeyCommandSkillsRefreshed           Key = "command.skills.refreshed"
	KeyCommandSkillsListHeader          Key = "command.skills.list_header"
	KeyCommandSkillsNone                Key = "command.skills.none"
	KeyCommandSkillsListEntry           Key = "command.skills.list_entry"
	KeyCommandSkillsListSummary         Key = "command.skills.list_summary"
	KeyCommandSkillsDetailHeader        Key = "command.skills.detail_header"
	KeyCommandSkillsDetailStatus        Key = "command.skills.detail_status"
	KeyCommandSkillsDetailSummary       Key = "command.skills.detail_summary"
	KeyCommandSkillsDetailPath          Key = "command.skills.detail_path"
	KeyCommandSkillsDetailDirectory     Key = "command.skills.detail_directory"
	KeyCommandSkillsDetailModelInvoke   Key = "command.skills.detail_model_invocation"
	KeyCommandSkillsDetailUserInvoke    Key = "command.skills.detail_user_invocation"
	KeyCommandSkillsDetailContext       Key = "command.skills.detail_context"
	KeyCommandSkillsDetailModel         Key = "command.skills.detail_model"
	KeyCommandSkillsDetailVersion       Key = "command.skills.detail_version"
	KeyCommandSkillsDetailTools         Key = "command.skills.detail_tools"
	KeyCommandSkillsStatusEnabled       Key = "command.skills.status_enabled"
	KeyCommandSkillsStatusDisabled      Key = "command.skills.status_disabled"
	KeyCommandSkillsDisabledFrontmatter Key = "command.skills.disabled_frontmatter"
	KeyCommandSkillsSessionCurrent      Key = "command.skills.session_current"
	KeyCommandSkillsSession             Key = "command.skills.session"
	KeyCommandSkillsNoneValue           Key = "command.skills.none_value"
	KeyCommandSkillsVirtualPath         Key = "command.skills.virtual_path"
	KeyCommandSkillsFullUsage           Key = "command.skills.full_usage"
	KeyCommandSkillsDescription         Key = "command.skills.description"
	KeyCommandSkillsSetUsage            Key = "command.skills.set_usage"
	KeyCommandSkillsResetUsage          Key = "command.skills.reset_usage"
	KeyCommandSkillsOperationFailed     Key = "command.skills.operation_failed"
	KeyCommandSkillsInvalidSelector     Key = "command.skills.invalid_selector"
	KeyCommandSkillsAmbiguous           Key = "command.skills.ambiguous"
	KeyCommandSkillsAmbiguousCandidate  Key = "command.skills.ambiguous_candidate"
	KeyCommandSkillsReadOnly            Key = "command.skills.read_only"
	KeyCommandSkillsSetResult           Key = "command.skills.set_result"
	KeyCommandSkillsResetResult         Key = "command.skills.reset_result"
	KeyCommandSkillsReadOnlyManaged     Key = "command.skills.read_only_reason.managed"
	KeyCommandSkillsReadOnlyDenied      Key = "command.skills.read_only_reason.denied"
	KeyCommandSkillsCatalogRevision     Key = "command.skills.catalog_revision"
	KeyCommandSkillsListIdentity        Key = "command.skills.list_identity"
	KeyCommandSkillsListRevision        Key = "command.skills.list_revision"
	KeyCommandSkillsListPolicy          Key = "command.skills.list_policy"
	KeyCommandSkillsListShadowed        Key = "command.skills.list_shadowed"
	KeyCommandSkillsDetailIdentity      Key = "command.skills.detail_identity"
	KeyCommandSkillsDetailRevision      Key = "command.skills.detail_revision"
	KeyCommandSkillsDetailPolicy        Key = "command.skills.detail_policy"
	KeyCommandSkillsMutableYes          Key = "command.skills.mutable_yes"
	KeyCommandSkillsMutableNo           Key = "command.skills.mutable_no"
)

func init() {
	for key, values := range map[Key][6]string{
		KeyCommandResumeStoreUnavailable: {"Session store not available.\n", "会话存储不可用。\n", "Sitzungsspeicher nicht verfügbar.\n", "セッションストアは利用できません。\n", "세션 저장소를 사용할 수 없습니다.\n", "Хранилище сеансов недоступно.\n"},
		KeyCommandResumeListError:        {"Error listing sessions: %v\n", "列出会话时出错：%v\n", "Fehler beim Auflisten der Sitzungen: %v\n", "セッション一覧の取得エラー: %v\n", "세션 목록 오류: %v\n", "Ошибка при выводе сеансов: %v\n"}, KeyCommandResumeNone: {"No saved sessions found.\n", "未找到已保存的会话。\n", "Keine gespeicherten Sitzungen gefunden.\n", "保存済みのセッションが見つかりません。\n", "저장된 세션을 찾을 수 없습니다.\n", "Сохраненные сеансы не найдены.\n"},
		KeyCommandResumeRecent: {"Recent sessions (use /resume <id-or-title> to load one):\n", "最近会话（使用 /resume <id-or-title> 加载）：\n", "Letzte Sitzungen (mit /resume <id-or-title> laden):\n", "最近のセッション（/resume <id-or-title> で読み込み）:\n", "최근 세션(/resume <id-or-title>로 불러오기):\n", "Недавние сеансы (загрузите через /resume <id-or-title>):\n"}, KeyCommandResumeEntry: {"%s%s  (%s, %d msgs)\n", "%s%s（%s，%d 条消息）\n", "%s%s  (%s, %d Nachr.)\n", "%s%s（%s、%d 件）\n", "%s%s  (%s, 메시지 %d개)\n", "%s%s  (%s, %d сообщ.)\n"},
		KeyCommandResumeSearchError: {"Error searching sessions: %v\n", "搜索会话时出错：%v\n", "Fehler beim Suchen von Sitzungen: %v\n", "セッション検索エラー: %v\n", "세션 검색 오류: %v\n", "Ошибка поиска сеансов: %v\n"}, KeyCommandResumeMultiple: {"Found %d sessions matching %q. Please refine your query.\n", "找到 %d 个与 %q 匹配的会话。请缩小查询范围。\n", "%d Sitzungen entsprechen %q. Bitte verfeinere die Suche.\n", "%d 件のセッションが %q に一致しました。検索を絞り込んでください。\n", "%d개의 세션이 %q와 일치합니다. 검색어를 구체화하세요.\n", "Найдено %d сеансов по запросу %q. Уточните запрос.\n"}, KeyCommandResumeLoadError: {"Error loading session %q: %v\n", "加载会话 %q 时出错：%v\n", "Fehler beim Laden der Sitzung %q: %v\n", "セッション %q の読み込みエラー: %v\n", "세션 %q을(를) 불러오는 중 오류: %v\n", "Ошибка загрузки сеанса %q: %v\n"}, KeyCommandResumeLoaded: {"Resumed session %s.\n", "已恢复会话 %s。\n", "Sitzung %s fortgesetzt.\n", "セッション %s を再開しました。\n", "세션 %s을(를) 재개했습니다.\n", "Сеанс %s возобновлен.\n"}, KeyCommandResumeTransitionUnavailable: {"session transition is not configured", "会话切换尚未配置", "Sitzungswechsel ist nicht konfiguriert", "セッション切り替えが設定されていません", "세션 전환이 구성되지 않았습니다", "Переключение сеансов не настроено"},
		KeyCommandRenameStoreUnavailable: {"Session store not available.\n", "会话存储不可用。\n", "Sitzungsspeicher nicht verfügbar.\n", "セッションストアは利用できません。\n", "세션 저장소를 사용할 수 없습니다.\n", "Хранилище сеансов недоступно.\n"}, KeyCommandRenameGenerateFailed: {"Could not generate a session name yet. Usage: /rename <name>\n", "暂时无法生成会话名称。用法：/rename <name>\n", "Es konnte noch kein Sitzungsname erstellt werden. Verwendung: /rename <name>\n", "まだセッション名を生成できません。使い方: /rename <name>\n", "아직 세션 이름을 생성할 수 없습니다. 사용법: /rename <name>\n", "Пока не удалось создать имя сеанса. Использование: /rename <name>\n"}, KeyCommandRenameError: {"Error renaming session: %v\n", "重命名会话时出错：%v\n", "Fehler beim Umbenennen der Sitzung: %v\n", "セッション名の変更エラー: %v\n", "세션 이름 변경 오류: %v\n", "Ошибка переименования сеанса: %v\n"}, KeyCommandRenameSucceeded: {"Session renamed to: %s\n", "会话已重命名为：%s\n", "Sitzung umbenannt in: %s\n", "セッション名を変更しました: %s\n", "세션 이름을 변경했습니다: %s\n", "Сеанс переименован в: %s\n"},
		KeyCommandForkUsage: {"usage: /fork", "用法：/fork", "Verwendung: /fork", "使い方: /fork", "사용법: /fork", "Использование: /fork"}, KeyCommandForkPickerUnavailable: {"fork picker is not configured", "分支选择器尚未配置", "Fork-Auswahl ist nicht konfiguriert", "フォーク選択画面が設定されていません", "포크 선택기가 구성되지 않았습니다", "Выбор ветвления не настроен"},
		KeyCommandDiffSummary:      {"Summary: ", "摘要：", "Zusammenfassung: ", "概要: ", "요약: ", "Сводка: "},
		KeyCommandReviewGitMissing: {"git not found in PATH — cannot show review diff.\n", "未在 PATH 中找到 git，无法显示审查差异。\n", "git wurde nicht im PATH gefunden — Review-Diff kann nicht angezeigt werden.\n", "PATH に git が見つからないためレビュー用差分を表示できません。\n", "PATH에서 git을 찾을 수 없어 리뷰 diff를 표시할 수 없습니다.\n", "git не найден в PATH — невозможно показать diff для ревью.\n"}, KeyCommandReviewNotRepository: {"Not inside a git repository.\n", "当前不在 git 仓库中。\n", "Nicht in einem Git-Repository.\n", "git リポジトリ内ではありません。\n", "git 저장소 안이 아닙니다.\n", "Вы не в репозитории git.\n"}, KeyCommandReviewNoStaged: {"No staged changes to review.\n", "没有已暂存的变更可供审查。\n", "Keine bereitgestellten Änderungen zum Review.\n", "レビューするステージ済み変更はありません。\n", "검토할 스테이지된 변경 사항이 없습니다.\n", "Нет подготовленных изменений для ревью.\n"}, KeyCommandReviewClean: {"No changes to review — working tree is clean.\n", "没有变更可供审查，工作区是干净的。\n", "Keine Änderungen zum Review — Arbeitsbaum ist sauber.\n", "レビューする変更はありません。作業ツリーはクリーンです。\n", "검토할 변경 사항이 없습니다. 작업 트리가 깨끗합니다.\n", "Нет изменений для ревью — рабочее дерево чистое.\n"}, KeyCommandReviewAllChanges: {"Unstaged + staged changes", "未暂存 + 已暂存变更", "Nicht bereitgestellte + bereitgestellte Änderungen", "未ステージ + ステージ済み変更", "스테이지 전 + 스테이지된 변경 사항", "Неподготовленные + подготовленные изменения"}, KeyCommandReviewStagedChanges: {"Staged changes", "已暂存变更", "Bereitgestellte Änderungen", "ステージ済み変更", "스테이지된 변경 사항", "Подготовленные изменения"},
		KeyCommandSkillsUnavailable: {"skills manager is not configured", "技能管理器尚未配置", "Skill-Manager ist nicht konfiguriert", "スキルマネージャーが設定されていません", "스킬 관리자가 구성되지 않았습니다", "Менеджер навыков не настроен"}, KeyCommandSkillsShowUsage: {"Usage: /skills show <name>\n", "用法：/skills show <name>\n", "Verwendung: /skills show <name>\n", "使い方: /skills show <name>\n", "사용법: /skills show <name>\n", "Использование: /skills show <name>\n"}, KeyCommandSkillsNotFound: {"Skill %q not found. Run /skills list to inspect the catalog.\n", "未找到技能 %q。请运行 /skills list 查看目录。\n", "Skill %q nicht gefunden. Mit /skills list den Katalog anzeigen.\n", "スキル %q が見つかりません。/skills list でカタログを確認してください。\n", "스킬 %q을(를) 찾을 수 없습니다. /skills list로 카탈로그를 확인하세요.\n", "Навык %q не найден. Выполните /skills list для просмотра каталога.\n"}, KeyCommandSkillsRefreshUsage: {"Usage: /skills refresh\n", "用法：/skills refresh\n", "Verwendung: /skills refresh\n", "使い方: /skills refresh\n", "사용법: /skills refresh\n", "Использование: /skills refresh\n"}, KeyCommandSkillsRefreshed: {"Refreshed the skill catalog.\n", "已刷新技能目录。\n", "Skill-Katalog aktualisiert.\n", "スキルカタログを更新しました。\n", "스킬 카탈로그를 새로 고쳤습니다.\n", "Каталог навыков обновлен.\n"},
		KeyCommandSkillsListHeader: {"Skills: %d discovered, %d enabled, %d disabled (%s)\n", "技能：发现 %d 个，已启用 %d 个，已禁用 %d 个（%s）\n", "Skills: %d gefunden, %d aktiviert, %d deaktiviert (%s)\n", "スキル: %d 件検出、%d 件有効、%d 件無効（%s）\n", "스킬: %d개 발견, %d개 활성화, %d개 비활성화(%s)\n", "Навыки: найдено %d, включено %d, выключено %d (%s)\n"}, KeyCommandSkillsNone: {"  No skills discovered. Run /skills refresh after installing one.\n", "  未发现技能。安装后请运行 /skills refresh。\n", "  Keine Skills gefunden. Nach der Installation /skills refresh ausführen.\n", "  スキルが見つかりません。インストール後に /skills refresh を実行してください。\n", "  스킬을 찾지 못했습니다. 설치 후 /skills refresh를 실행하세요.\n", "  Навыки не найдены. После установки выполните /skills refresh.\n"}, KeyCommandSkillsListEntry: {"  [%s] %s\n", "  [%s] %s\n", "  [%s] %s\n", "  [%s] %s\n", "  [%s] %s\n", "  [%s] %s\n"}, KeyCommandSkillsListSummary: {"    Summary: %s\n", "    摘要：%s\n", "    Zusammenfassung: %s\n", "    概要: %s\n", "    요약: %s\n", "    Сводка: %s\n"},
		KeyCommandSkillsDetailHeader: {"Skill: %s\n", "技能：%s\n", "Skill: %s\n", "スキル: %s\n", "스킬: %s\n", "Навык: %s\n"}, KeyCommandSkillsDetailStatus: {"  Status: %s (%s)\n", "  状态：%s（%s）\n", "  Status: %s (%s)\n", "  状態: %s（%s）\n", "  상태: %s(%s)\n", "  Статус: %s (%s)\n"}, KeyCommandSkillsDetailSummary: {"  Summary: %s\n", "  摘要：%s\n", "  Zusammenfassung: %s\n", "  概要: %s\n", "  요약: %s\n", "  Сводка: %s\n"}, KeyCommandSkillsDetailPath: {"  Path: %s\n", "  路径：%s\n", "  Pfad: %s\n", "  パス: %s\n", "  경로: %s\n", "  Путь: %s\n"}, KeyCommandSkillsDetailDirectory: {"  Directory: %s\n", "  目录：%s\n", "  Verzeichnis: %s\n", "  ディレクトリ: %s\n", "  디렉터리: %s\n", "  Каталог: %s\n"}, KeyCommandSkillsDetailModelInvoke: {"  Model invocation: %s\n", "  模型调用：%s\n", "  Modellaufruf: %s\n", "  モデル呼び出し: %s\n", "  모델 호출: %s\n", "  Вызов моделью: %s\n"}, KeyCommandSkillsDetailUserInvoke: {"  User invocation: %s\n", "  用户调用：%s\n", "  Benutzeraufruf: %s\n", "  ユーザー呼び出し: %s\n", "  사용자 호출: %s\n", "  Вызов пользователем: %s\n"}, KeyCommandSkillsDetailContext: {"  Context: %s\n", "  上下文：%s\n", "  Kontext: %s\n", "  コンテキスト: %s\n", "  컨텍스트: %s\n", "  Контекст: %s\n"}, KeyCommandSkillsDetailModel: {"  Model override: %s\n", "  模型覆盖：%s\n", "  Modellüberschreibung: %s\n", "  モデル上書き: %s\n", "  모델 재정의: %s\n", "  Переопределение модели: %s\n"}, KeyCommandSkillsDetailVersion: {"  Version: %s\n", "  版本：%s\n", "  Version: %s\n", "  バージョン: %s\n", "  버전: %s\n", "  Версия: %s\n"}, KeyCommandSkillsDetailTools: {"  Allowed tools: %s\n", "  允许工具：%s\n", "  Erlaubte Tools: %s\n", "  許可ツール: %s\n", "  허용 도구: %s\n", "  Разрешенные инструменты: %s\n"},
		KeyCommandSkillsStatusEnabled: {"enabled", "已启用", "aktiviert", "有効", "활성화됨", "включен"}, KeyCommandSkillsStatusDisabled: {"disabled", "已禁用", "deaktiviert", "無効", "비활성화됨", "выключен"}, KeyCommandSkillsDisabledFrontmatter: {"disabled by frontmatter", "已由 frontmatter 禁用", "durch Frontmatter deaktiviert", "frontmatter により無効", "frontmatter로 비활성화됨", "отключен frontmatter"}, KeyCommandSkillsSessionCurrent: {"current runtime", "当前运行时", "aktuelle Laufzeit", "現在のランタイム", "현재 런타임", "текущая среда выполнения"}, KeyCommandSkillsSession: {"session %s", "会话 %s", "Sitzung %s", "セッション %s", "세션 %s", "сеанс %s"}, KeyCommandSkillsNoneValue: {"(none)", "（无）", "(keine)", "（なし）", "(없음)", "(нет)"}, KeyCommandSkillsVirtualPath: {"(virtual)", "（虚拟）", "(virtuell)", "（仮想）", "(가상)", "(виртуальный)"},
		KeyCommandSkillsFullUsage:          {"Usage:\n  /skills [list]\n  /skills show <selector>\n  /skills set <selector> <auto|name-only|manual-only|off> --scope <session|project|user>\n  /skills reset <selector> --scope <session|project|user>\n  /skills refresh\n", "用法：\n  /skills [list]\n  /skills show <selector>\n  /skills set <selector> <auto|name-only|manual-only|off> --scope <session|project|user>\n  /skills reset <selector> --scope <session|project|user>\n  /skills refresh\n", "Verwendung:\n  /skills [list]\n  /skills show <selector>\n  /skills set <selector> <auto|name-only|manual-only|off> --scope <session|project|user>\n  /skills reset <selector> --scope <session|project|user>\n  /skills refresh\n", "使い方:\n  /skills [list]\n  /skills show <selector>\n  /skills set <selector> <auto|name-only|manual-only|off> --scope <session|project|user>\n  /skills reset <selector> --scope <session|project|user>\n  /skills refresh\n", "사용법:\n  /skills [list]\n  /skills show <selector>\n  /skills set <selector> <auto|name-only|manual-only|off> --scope <session|project|user>\n  /skills reset <selector> --scope <session|project|user>\n  /skills refresh\n", "Использование:\n  /skills [list]\n  /skills show <selector>\n  /skills set <selector> <auto|name-only|manual-only|off> --scope <session|project|user>\n  /skills reset <selector> --scope <session|project|user>\n  /skills refresh\n"},
		KeyCommandSkillsSetUsage:           {"Usage: /skills set <selector> <auto|name-only|manual-only|off> --scope <session|project|user>\n", "用法：/skills set <selector> <auto|name-only|manual-only|off> --scope <session|project|user>\n", "Verwendung: /skills set <selector> <auto|name-only|manual-only|off> --scope <session|project|user>\n", "使い方: /skills set <selector> <auto|name-only|manual-only|off> --scope <session|project|user>\n", "사용법: /skills set <selector> <auto|name-only|manual-only|off> --scope <session|project|user>\n", "Использование: /skills set <selector> <auto|name-only|manual-only|off> --scope <session|project|user>\n"},
		KeyCommandSkillsResetUsage:         {"Usage: /skills reset <selector> --scope <session|project|user>\n", "用法：/skills reset <selector> --scope <session|project|user>\n", "Verwendung: /skills reset <selector> --scope <session|project|user>\n", "使い方: /skills reset <selector> --scope <session|project|user>\n", "사용법: /skills reset <selector> --scope <session|project|user>\n", "Использование: /skills reset <selector> --scope <session|project|user>\n"},
		KeyCommandSkillsOperationFailed:    {"Skill operation failed: %v\n", "技能操作失败：%v\n", "Skill-Aktion fehlgeschlagen: %v\n", "スキル操作に失敗しました: %v\n", "스킬 작업 실패: %v\n", "Операция с навыком завершилась ошибкой: %v\n"},
		KeyCommandSkillsInvalidSelector:    {"Invalid stable skill selector %q. Run /skills list and use the complete skill ID.\n", "稳定技能选择器 %q 无效。请运行 /skills list 并使用完整技能 ID。\n", "Ungültiger stabiler Skill-Selektor %q. Mit /skills list die vollständige Skill-ID ermitteln.\n", "安定スキルセレクター %q は無効です。/skills list で完全なスキル ID を確認してください。\n", "안정적인 스킬 선택자 %q이(가) 올바르지 않습니다. /skills list에서 전체 스킬 ID를 확인하세요.\n", "Недопустимый стабильный селектор навыка %q. Выполните /skills list и укажите полный ID навыка.\n"},
		KeyCommandSkillsAmbiguous:          {"Skill name %q is ambiguous. Use one of these stable IDs:\n", "技能名称 %q 存在歧义。请使用以下稳定 ID 之一：\n", "Der Skill-Name %q ist mehrdeutig. Eine dieser stabilen IDs verwenden:\n", "スキル名 %q は一意ではありません。次の安定 ID のいずれかを使用してください:\n", "스킬 이름 %q이(가) 모호합니다. 다음 안정적인 ID 중 하나를 사용하세요:\n", "Имя навыка %q неоднозначно. Используйте один из стабильных ID:\n"},
		KeyCommandSkillsAmbiguousCandidate: {"  %s (source: %s, locator: %s)\n", "  %s（来源：%s，定位符：%s）\n", "  %s (Quelle: %s, Locator: %s)\n", "  %s（提供元: %s、ロケーター: %s）\n", "  %s(출처: %s, 위치: %s)\n", "  %s (источник: %s, расположение: %s)\n"},
		KeyCommandSkillsReadOnly:           {"Skill %q (%s) is read-only: %s.\n", "技能 %q（%s）为只读：%s。\n", "Skill %q (%s) ist schreibgeschützt: %s.\n", "スキル %q（%s）は読み取り専用です: %s。\n", "스킬 %q(%s)은(는) 읽기 전용입니다: %s.\n", "Навык %q (%s) доступен только для чтения: %s.\n"},
		KeyCommandSkillsSetResult:          {"Set skill %q (%s) to %s at %s scope. Effective visibility: %s (source: %s).\n", "已将技能 %q（%s）设为 %s（作用域：%s）。有效可见性：%s（来源：%s）。\n", "Skill %q (%s) auf %s im Bereich %s gesetzt. Effektive Sichtbarkeit: %s (Quelle: %s).\n", "スキル %q（%s）を %s に設定しました（スコープ: %s）。有効な表示範囲: %s（ソース: %s）。\n", "스킬 %q(%s)을(를) %s(으)로 설정했습니다(범위: %s). 유효 표시 범위: %s(출처: %s).\n", "Для навыка %q (%s) задано %s в области %s. Итоговая видимость: %s (источник: %s).\n"},
		KeyCommandSkillsResetResult:        {"Reset skill %q (%s) at %s scope. Effective visibility: %s (source: %s).\n", "已重置技能 %q（%s）的 %s 作用域。有效可见性：%s（来源：%s）。\n", "Skill %q (%s) im Bereich %s zurückgesetzt. Effektive Sichtbarkeit: %s (Quelle: %s).\n", "スキル %q（%s）の %s スコープをリセットしました。有効な表示範囲: %s（ソース: %s）。\n", "스킬 %q(%s)의 %s 범위를 재설정했습니다. 유효 표시 범위: %s(출처: %s).\n", "Для навыка %q (%s) сброшена область %s. Итоговая видимость: %s (источник: %s).\n"},
		KeyCommandSkillsCatalogRevision:    {"Catalog revision: %d\n", "目录修订：%d\n", "Katalogrevision: %d\n", "カタログリビジョン: %d\n", "카탈로그 리비전: %d\n", "Ревизия каталога: %d\n"},
		KeyCommandSkillsListIdentity:       {"    ID: %s | Source: %s | Locator: %s\n", "    ID：%s｜来源：%s｜定位符：%s\n", "    ID: %s | Quelle: %s | Locator: %s\n", "    ID: %s｜提供元: %s｜ロケーター: %s\n", "    ID: %s | 출처: %s | 위치: %s\n", "    ID: %s | Источник: %s | Расположение: %s\n"},
		KeyCommandSkillsListRevision:       {"    Digest: %s | Skill revision: %d\n", "    摘要值：%s｜技能修订：%d\n", "    Digest: %s | Skill-Revision: %d\n", "    ダイジェスト: %s｜スキルリビジョン: %d\n", "    다이제스트: %s | 스킬 리비전: %d\n", "    Дайджест: %s | Ревизия навыка: %d\n"},
		KeyCommandSkillsListPolicy:         {"    Visibility: %s | State source: %s | Mutable: %s | Read-only reason: %s\n", "    可见性：%s｜状态来源：%s｜可修改：%s｜只读原因：%s\n", "    Sichtbarkeit: %s | Statusquelle: %s | Änderbar: %s | Schreibschutzgrund: %s\n", "    表示範囲: %s｜状態ソース: %s｜変更可能: %s｜読み取り専用の理由: %s\n", "    표시 범위: %s | 상태 출처: %s | 변경 가능: %s | 읽기 전용 사유: %s\n", "    Видимость: %s | Источник состояния: %s | Можно изменить: %s | Причина только для чтения: %s\n"},
		KeyCommandSkillsListShadowed:       {"    Shadowed by: %s\n", "    被以下技能遮蔽：%s\n", "    Überschattet durch: %s\n", "    次のスキルにより非優先: %s\n", "    다음 스킬에 의해 가려짐: %s\n", "    Перекрыт навыком: %s\n"},
		KeyCommandSkillsDetailIdentity:     {"  ID: %s | Source: %s | Locator: %s\n", "  ID：%s｜来源：%s｜定位符：%s\n", "  ID: %s | Quelle: %s | Locator: %s\n", "  ID: %s｜提供元: %s｜ロケーター: %s\n", "  ID: %s | 출처: %s | 위치: %s\n", "  ID: %s | Источник: %s | Расположение: %s\n"},
		KeyCommandSkillsDetailRevision:     {"  Digest: %s | Skill revision: %d | Catalog revision: %d\n", "  摘要值：%s｜技能修订：%d｜目录修订：%d\n", "  Digest: %s | Skill-Revision: %d | Katalogrevision: %d\n", "  ダイジェスト: %s｜スキルリビジョン: %d｜カタログリビジョン: %d\n", "  다이제스트: %s | 스킬 리비전: %d | 카탈로그 리비전: %d\n", "  Дайджест: %s | Ревизия навыка: %d | Ревизия каталога: %d\n"},
		KeyCommandSkillsDetailPolicy:       {"  Visibility: %s | State source: %s | Mutable: %s | Read-only reason: %s\n", "  可见性：%s｜状态来源：%s｜可修改：%s｜只读原因：%s\n", "  Sichtbarkeit: %s | Statusquelle: %s | Änderbar: %s | Schreibschutzgrund: %s\n", "  表示範囲: %s｜状態ソース: %s｜変更可能: %s｜読み取り専用の理由: %s\n", "  표시 범위: %s | 상태 출처: %s | 변경 가능: %s | 읽기 전용 사유: %s\n", "  Видимость: %s | Источник состояния: %s | Можно изменить: %s | Причина только для чтения: %s\n"},
		KeyCommandSkillsMutableYes:         {"yes", "是", "ja", "はい", "예", "да"},
		KeyCommandSkillsMutableNo:          {"no", "否", "nein", "いいえ", "아니요", "нет"},
	} {
		semanticTranslations[key] = commandSession(values[0], values[1], values[2], values[3], values[4], values[5])
	}

	semanticTranslations[KeyCommandSkillsReadOnlyManaged] = commandSession(
		"managed policy owns this setting",
		"此设置由托管策略控制",
		"diese Einstellung wird von einer verwalteten Richtlinie vorgegeben",
		"この設定は管理ポリシーによって制御されています",
		"이 설정은 관리된 정책에서 제어합니다",
		"эта настройка задана управляемой политикой",
	)
	semanticTranslations[KeyCommandSkillsReadOnlyDenied] = commandSession(
		"managed policy blocks this skill",
		"托管策略已禁用此技能",
		"eine verwaltete Richtlinie blockiert diesen Skill",
		"管理ポリシーによってこのスキルは禁止されています",
		"관리된 정책에서 이 스킬을 차단했습니다",
		"управляемая политика блокирует этот навык",
	)
}

func commandSession(en, zh, de, ja, ko, ru string) map[Language]string {
	return map[Language]string{LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru}
}
