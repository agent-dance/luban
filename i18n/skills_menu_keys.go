package i18n

// Semantic copy owned by the direct interactive /skills checklist. Skill names,
// summaries, stable IDs, source identifiers, locators, visibility/scope
// identifiers, slash commands, and raw backend errors remain format values.
const (
	KeySkillsMenuTitle                   Key = "tui.skills_menu.title"
	KeySkillsMenuHelp                    Key = "tui.skills_menu.help"
	KeySkillsMenuFilter                  Key = "tui.skills_menu.toggle.filter"
	KeySkillsMenuLoading                 Key = "tui.skills_menu.toggle.loading"
	KeySkillsMenuUpdating                Key = "tui.skills_menu.toggle.updating"
	KeySkillsMenuRefreshing              Key = "tui.skills_menu.toggle.refreshing"
	KeySkillsMenuEmpty                   Key = "tui.skills_menu.toggle.empty"
	KeySkillsMenuNoMatches               Key = "tui.skills_menu.toggle.no_matches"
	KeySkillsMenuShowing                 Key = "tui.skills_menu.toggle.showing"
	KeySkillsMenuDetailSummary           Key = "tui.skills_menu.detail.summary"
	KeySkillsMenuDetailSource            Key = "tui.skills_menu.detail.source"
	KeySkillsMenuDetailPath              Key = "tui.skills_menu.detail.path"
	KeySkillsMenuDetailVisibilityScope   Key = "tui.skills_menu.detail.visibility_scope"
	KeySkillsMenuDetailIdentity          Key = "tui.skills_menu.detail.identity"
	KeySkillsMenuDetailShadowed          Key = "tui.skills_menu.detail.shadowed"
	KeySkillsMenuDetailMutable           Key = "tui.skills_menu.detail.mutable"
	KeySkillsMenuDetailReadOnly          Key = "tui.skills_menu.detail.read_only"
	KeySkillsMenuReadOnlyUnspecified     Key = "tui.skills_menu.read_only.unspecified"
	KeySkillsMenuBackendUnavailable      Key = "tui.skills_menu.backend.unavailable"
	KeySkillsMenuSessionUnavailable      Key = "tui.skills_menu.session.unavailable"
	KeySkillsMenuLoadFailed              Key = "tui.skills_menu.load.failed"
	KeySkillsMenuInvalidResult           Key = "tui.skills_menu.result.invalid"
	KeySkillsMenuStatusStale             Key = "tui.skills_menu.status.stale"
	KeySkillsMenuStatusUnknown           Key = "tui.skills_menu.status.unknown"
	KeySkillsMenuStatusSessionOverride   Key = "tui.skills_menu.status.session_override"
	KeySkillsMenuStatusReadOnly          Key = "tui.skills_menu.status.read_only"
	KeySkillsMenuStatusPersistenceFailed Key = "tui.skills_menu.status.persistence_failed"
	KeySkillsMenuStatusRolledBack        Key = "tui.skills_menu.status.rolled_back"
	KeySkillsMenuStatusDegraded          Key = "tui.skills_menu.status.degraded"
	KeySkillsMenuStatusRefreshFailed     Key = "tui.skills_menu.status.refresh_failed"
	KeySkillsMenuStatusRefreshed         Key = "tui.skills_menu.status.refreshed"
	KeySkillsMenuStatusUnexpected        Key = "tui.skills_menu.status.unexpected"
)

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de,
			LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}

	add(KeySkillsMenuTitle,
		"Skills",
		"技能",
		"Skills",
		"スキル",
		"스킬",
		"Навыки")
	add(KeySkillsMenuHelp,
		"Type to filter · Up/Down move · Enter select · Space toggle · Backspace edit · Esc close",
		"输入以筛选 · 上/下移动 · 回车选用 · Space 开关 · Backspace 编辑 · Esc 关闭",
		"Tippen zum Filtern · Auf/Ab bewegen · Enter auswählen · Leertaste umschalten · Rücktaste bearbeiten · Esc schließen",
		"入力して絞り込み · 上下で移動 · Enter で選択 · Space で切替 · Backspace で編集 · Esc で閉じる",
		"입력하여 필터 · 위/아래 이동 · Enter 선택 · Space 전환 · Backspace 편집 · Esc 닫기",
		"Введите фильтр · Вверх/вниз — перемещение · Enter — выбрать · Space — переключить · Backspace — изменить · Esc — закрыть")
	add(KeySkillsMenuFilter,
		"Filter: %s",
		"筛选：%s",
		"Filter: %s",
		"フィルター: %s",
		"필터: %s",
		"Фильтр: %s")
	add(KeySkillsMenuLoading,
		"Loading the current skill catalog…",
		"正在加载当前技能目录…",
		"Aktueller Skill-Katalog wird geladen…",
		"現在のスキルカタログを読み込んでいます…",
		"현재 스킬 카탈로그를 불러오는 중…",
		"Загрузка текущего каталога навыков…")
	add(KeySkillsMenuUpdating,
		"Updating %s (%s)…",
		"正在更新 %s（%s）…",
		"%s (%s) wird aktualisiert…",
		"%s（%s）を更新しています…",
		"%s(%s) 업데이트 중…",
		"Обновление %s (%s)…")
	add(KeySkillsMenuRefreshing,
		"Refreshing the authoritative skill catalog before another toggle…",
		"正在刷新权威技能目录，完成前不会再次切换…",
		"Der maßgebliche Skill-Katalog wird vor dem nächsten Umschalten aktualisiert…",
		"次の切り替え前に正規のスキルカタログを再読み込みしています…",
		"다시 전환하기 전에 기준 스킬 카탈로그를 새로 고치는 중…",
		"Перед новым переключением обновляется актуальный каталог навыков…")
	add(KeySkillsMenuEmpty,
		"No skills are available in the current catalog.",
		"当前目录中没有可用技能。",
		"Im aktuellen Katalog sind keine Skills verfügbar.",
		"現在のカタログに利用可能なスキルはありません。",
		"현재 카탈로그에 사용 가능한 스킬이 없습니다.",
		"В текущем каталоге нет доступных навыков.")
	add(KeySkillsMenuNoMatches,
		"No skills match %q.",
		"没有技能匹配 %q。",
		"Keine Skills entsprechen %q.",
		"%q に一致するスキルはありません。",
		"%q과(와) 일치하는 스킬이 없습니다.",
		"Нет навыков, соответствующих %q.")
	add(KeySkillsMenuShowing,
		"Showing %d-%d of %d",
		"显示第 %d-%d 项，共 %d 项",
		"%d-%d von %d werden angezeigt",
		"%d-%d / %d 件を表示",
		"%d-%d / %d 표시",
		"Показаны %d-%d из %d")
	add(KeySkillsMenuDetailSummary,
		"Summary: %s",
		"摘要：%s",
		"Zusammenfassung: %s",
		"概要: %s",
		"요약: %s",
		"Описание: %s")
	add(KeySkillsMenuDetailSource,
		"Source: %s",
		"来源：%s",
		"Quelle: %s",
		"ソース: %s",
		"소스: %s",
		"Источник: %s")
	add(KeySkillsMenuDetailPath,
		"Path: %s",
		"路径：%s",
		"Pfad: %s",
		"パス: %s",
		"경로: %s",
		"Путь: %s")
	add(KeySkillsMenuDetailVisibilityScope,
		"Visibility: %s · Effective scope: %s",
		"可见性：%s · 生效范围：%s",
		"Sichtbarkeit: %s · Wirksamer Bereich: %s",
		"可視性: %s · 有効スコープ: %s",
		"표시 상태: %s · 유효 범위: %s",
		"Видимость: %s · Действующая область: %s")
	add(KeySkillsMenuDetailIdentity,
		"ID: %s · Catalog revision: %d",
		"ID：%s · 目录修订号：%d",
		"ID: %s · Katalogrevision: %d",
		"ID: %s · カタログリビジョン: %d",
		"ID: %s · 카탈로그 리비전: %d",
		"ID: %s · Ревизия каталога: %d")
	add(KeySkillsMenuDetailShadowed,
		"Configuration: enabled, but currently inactive — shadowed by %s",
		"配置：已启用，但当前未激活 — 被 %s 遮蔽",
		"Konfiguration: aktiviert, aber derzeit inaktiv — von %s überschattet",
		"設定: 有効ですが現在は非アクティブ — %s によってシャドウされています",
		"설정: 활성화되어 있지만 현재 비활성 — %s에 의해 가려짐",
		"Конфигурация: включено, но сейчас неактивно — перекрыто %s")
	add(KeySkillsMenuDetailMutable,
		"Mutability: project override can be changed",
		"可修改性：可更改项目覆盖设置",
		"Änderbarkeit: Projektüberschreibung kann geändert werden",
		"変更可否: プロジェクトの上書きを変更できます",
		"변경 가능: 프로젝트 재정의를 변경할 수 있음",
		"Изменяемость: переопределение проекта можно изменить")
	add(KeySkillsMenuDetailReadOnly,
		"Mutability: read-only — %s",
		"可修改性：只读 — %s",
		"Änderbarkeit: schreibgeschützt — %s",
		"変更可否: 読み取り専用 — %s",
		"변경 가능: 읽기 전용 — %s",
		"Изменяемость: только чтение — %s")
	add(KeySkillsMenuReadOnlyUnspecified,
		"no editable project override",
		"没有可编辑的项目覆盖设置",
		"keine änderbare Projektüberschreibung",
		"編集可能なプロジェクト上書きがありません",
		"편집 가능한 프로젝트 재정의 없음",
		"нет изменяемого переопределения проекта")
	add(KeySkillsMenuBackendUnavailable,
		"Skill management is unavailable in this runtime.",
		"当前运行环境不支持技能管理。",
		"Die Skill-Verwaltung ist in dieser Laufzeit nicht verfügbar.",
		"この実行環境ではスキル管理を利用できません。",
		"이 런타임에서는 스킬 관리를 사용할 수 없습니다.",
		"Управление навыками недоступно в этой среде.")
	add(KeySkillsMenuSessionUnavailable,
		"No active session is available for skill management.",
		"当前没有可用于技能管理的活动会话。",
		"Für die Skill-Verwaltung ist keine aktive Sitzung verfügbar.",
		"スキル管理に使用できるアクティブなセッションがありません。",
		"스킬 관리에 사용할 활성 세션이 없습니다.",
		"Нет активного сеанса для управления навыками.")
	add(KeySkillsMenuLoadFailed,
		"Could not load the current skill catalog: %v",
		"无法加载当前技能目录：%v",
		"Der aktuelle Skill-Katalog konnte nicht geladen werden: %v",
		"現在のスキルカタログを読み込めませんでした: %v",
		"현재 스킬 카탈로그를 불러올 수 없습니다: %v",
		"Не удалось загрузить текущий каталог навыков: %v")
	add(KeySkillsMenuInvalidResult,
		"The skill backend returned an invalid result; no local state was changed: %v",
		"技能后端返回了无效结果；本地状态未更改：%v",
		"Das Skill-Backend lieferte ein ungültiges Ergebnis; der lokale Zustand wurde nicht geändert: %v",
		"スキルバックエンドが無効な結果を返しました。ローカル状態は変更されていません: %v",
		"스킬 백엔드가 잘못된 결과를 반환했습니다. 로컬 상태는 변경되지 않았습니다: %v",
		"Сервер навыков вернул недопустимый результат; локальное состояние не изменено: %v")
	add(KeySkillsMenuStatusStale,
		"The row for %s was stale. The list was refreshed to catalog revision %d; press Space again if the new state is still intended.",
		"%s 的条目已过期。列表已刷新到目录修订号 %d；如果新状态仍符合预期，请再次按 Space。",
		"Die Zeile für %s war veraltet. Die Liste wurde auf Katalogrevision %d aktualisiert; Leertaste erneut drücken, wenn der neue Zustand weiterhin gewünscht ist.",
		"%s の行は古くなっていました。カタログリビジョン %d に更新しました。新しい状態でも変更する場合は Space をもう一度押してください。",
		"%s 행이 오래되었습니다. 목록을 카탈로그 리비전 %d(으)로 새로 고쳤습니다. 새 상태에서도 변경하려면 Space를 다시 누르세요.",
		"Строка %s устарела. Список обновлён до ревизии каталога %d; если изменение всё ещё нужно, снова нажмите Space.")
	add(KeySkillsMenuStatusUnknown,
		"Skill %s no longer exists; the authoritative list was refreshed.",
		"技能 %s 已不存在；已刷新权威列表。",
		"Skill %s existiert nicht mehr; die maßgebliche Liste wurde aktualisiert.",
		"スキル %s は存在しなくなりました。正規の一覧を更新しました。",
		"스킬 %s이(가) 더 이상 존재하지 않습니다. 기준 목록을 새로 고쳤습니다.",
		"Навык %s больше не существует; актуальный список обновлён.")
	add(KeySkillsMenuStatusSessionOverride,
		"%s (%s) has an active session override. Clear it with %s, then use Space for the project setting.",
		"%s（%s）存在活动的会话覆盖。请先使用 %s 清除，再用 Space 更改项目设置。",
		"%s (%s) hat eine aktive Sitzungsüberschreibung. Mit %s löschen und danach die Projekteinstellung mit der Leertaste ändern.",
		"%s（%s）には有効なセッション上書きがあります。%s で解除してから Space でプロジェクト設定を変更してください。",
		"%s(%s)에 활성 세션 재정의가 있습니다. %s로 해제한 뒤 Space로 프로젝트 설정을 변경하세요.",
		"Для %s (%s) действует переопределение сеанса. Сбросьте его командой %s, затем используйте Space для настройки проекта.")
	add(KeySkillsMenuStatusReadOnly,
		"%s (%s) is read-only: %s",
		"%s（%s）为只读：%s",
		"%s (%s) ist schreibgeschützt: %s",
		"%s（%s）は読み取り専用です: %s",
		"%s(%s)은(는) 읽기 전용입니다: %s",
		"%s (%s) доступен только для чтения: %s")
	add(KeySkillsMenuStatusPersistenceFailed,
		"The project setting for %s (%s) could not be saved; the authoritative state is unchanged.",
		"无法保存 %s（%s）的项目设置；权威状态保持不变。",
		"Die Projekteinstellung für %s (%s) konnte nicht gespeichert werden; der maßgebliche Zustand bleibt unverändert.",
		"%s（%s）のプロジェクト設定を保存できませんでした。正規の状態は変更されていません。",
		"%s(%s)의 프로젝트 설정을 저장할 수 없습니다. 기준 상태는 변경되지 않았습니다.",
		"Не удалось сохранить настройку проекта для %s (%s); актуальное состояние не изменено.")
	add(KeySkillsMenuStatusRolledBack,
		"The live update for %s (%s) failed and the project setting was rolled back.",
		"%s（%s）的实时更新失败，项目设置已回滚。",
		"Die Live-Aktualisierung für %s (%s) ist fehlgeschlagen; die Projekteinstellung wurde zurückgesetzt.",
		"%s（%s）のライブ更新に失敗し、プロジェクト設定をロールバックしました。",
		"%s(%s)의 실시간 업데이트가 실패하여 프로젝트 설정을 롤백했습니다.",
		"Оперативное обновление %s (%s) завершилось ошибкой; настройка проекта откатана.")
	add(KeySkillsMenuStatusDegraded,
		"The update for %s (%s) and its rollback both failed. Refresh is required; the list shows the latest authoritative snapshot available.",
		"%s（%s）的更新和回滚均失败。需要刷新；列表显示当前可用的最新权威快照。",
		"Aktualisierung und Rücksetzung für %s (%s) sind fehlgeschlagen. Eine Aktualisierung ist erforderlich; die Liste zeigt den neuesten verfügbaren maßgeblichen Snapshot.",
		"%s（%s）の更新とロールバックの両方に失敗しました。再読み込みが必要です。一覧には取得できた最新の正規スナップショットを表示しています。",
		"%s(%s)의 업데이트와 롤백이 모두 실패했습니다. 새로 고침이 필요하며 목록에는 사용 가능한 최신 기준 스냅샷이 표시됩니다.",
		"Обновление %s (%s) и его откат завершились ошибкой. Требуется обновление; список показывает последний доступный актуальный снимок.")
	add(KeySkillsMenuStatusRefreshFailed,
		"The update for %s (%s) failed and the authoritative refresh also failed. Refresh is required before another toggle.",
		"%s（%s）的更新失败，权威刷新也失败。再次切换前必须刷新。",
		"Die Aktualisierung für %s (%s) und auch die maßgebliche Aktualisierung sind fehlgeschlagen. Vor dem nächsten Umschalten ist eine Aktualisierung erforderlich.",
		"%s（%s）の更新と正規状態の再読み込みに失敗しました。もう一度切り替える前に再読み込みが必要です。",
		"%s(%s)의 업데이트와 기준 새로 고침이 모두 실패했습니다. 다시 전환하기 전에 새로 고침이 필요합니다.",
		"Обновление %s (%s) и получение актуального состояния завершились ошибкой. Перед новым переключением требуется обновление.")
	add(KeySkillsMenuStatusRefreshed,
		"The authoritative catalog was refreshed to revision %d; Space may be used again.",
		"权威目录已刷新到修订号 %d；现在可以再次使用 Space。",
		"Der maßgebliche Katalog wurde auf Revision %d aktualisiert; die Leertaste kann wieder verwendet werden.",
		"正規のカタログをリビジョン %d に更新しました。Space を再び使用できます。",
		"기준 카탈로그를 리비전 %d(으)로 새로 고쳤습니다. 이제 Space를 다시 사용할 수 있습니다.",
		"Актуальный каталог обновлён до ревизии %d; Space снова доступен.")
	add(KeySkillsMenuStatusUnexpected,
		"The skill backend returned an unexpected result for %s: %v",
		"技能后端为 %s 返回了意外结果：%v",
		"Das Skill-Backend lieferte für %s ein unerwartetes Ergebnis: %v",
		"スキルバックエンドが %s に対して予期しない結果を返しました: %v",
		"스킬 백엔드가 %s에 대해 예상치 못한 결과를 반환했습니다: %v",
		"Сервер навыков вернул неожиданный результат для %s: %v")
}
