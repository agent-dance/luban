package i18n

// Semantic copy used by commands/builtins.go. Command names, identifiers,
// paths, model/provider IDs, and raw downstream errors are format values.
const (
	KeyBuiltinActivityViewUnavailable           Key = "command.activity.view_unavailable"
	KeyBuiltinActivityActionsUnavailable        Key = "command.activity.actions_unavailable"
	KeyBuiltinActivityUsage                     Key = "command.activity.usage"
	KeyBuiltinDetailUsage                       Key = "command.detail.usage"
	KeyBuiltinSearchUsage                       Key = "command.search.usage"
	KeyBuiltinSearchUnavailable                 Key = "command.search.unavailable"
	KeyBuiltinExportUnavailable                 Key = "command.export.unavailable"
	KeyBuiltinExported                          Key = "command.export.completed"
	KeyBuiltinEditorDetailUsage                 Key = "command.editor.detail_usage"
	KeyBuiltinEditorUnavailable                 Key = "command.editor.unavailable"
	KeyBuiltinMouseUnavailable                  Key = "command.mouse.unavailable"
	KeyBuiltinMouseUsage                        Key = "command.mouse.usage"
	KeyBuiltinMouseState                        Key = "command.mouse.state"
	KeyBuiltinHelpCommands                      Key = "command.help.commands"
	KeyBuiltinHelpShortcuts                     Key = "command.help.shortcuts"
	KeyBuiltinHelpShortcutCycle                 Key = "command.help.shortcut.cycle"
	KeyBuiltinHelpShortcutToggle                Key = "command.help.shortcut.toggle"
	KeyBuiltinHelpShortcutJump                  Key = "command.help.shortcut.jump"
	KeyBuiltinHelpShortcutScroll                Key = "command.help.shortcut.scroll"
	KeyBuiltinHelpShortcutClose                 Key = "command.help.shortcut.close"
	KeyBuiltinClearViewUnavailable              Key = "command.clear.view_unavailable"
	KeyBuiltinClearConversationUnavailable      Key = "command.clear.conversation_unavailable"
	KeyBuiltinClearUsage                        Key = "command.clear.usage"
	KeyBuiltinCompactEmpty                      Key = "command.compact.empty"
	KeyBuiltinCompactUnavailable                Key = "command.compact.unavailable"
	KeyBuiltinCompactRunning                    Key = "command.compact.running"
	KeyBuiltinCompactComplete                   Key = "command.compact.complete"
	KeyBuiltinModelCurrent                      Key = "command.model.current"
	KeyBuiltinModelAvailable                    Key = "command.model.available"
	KeyBuiltinModelProvider                     Key = "command.model.provider"
	KeyBuiltinModelVision                       Key = "command.model.vision"
	KeyBuiltinModelText                         Key = "command.model.text"
	KeyBuiltinModelReasoningEfforts             Key = "command.model.reasoning_efforts"
	KeyBuiltinModelThinking                     Key = "command.model.thinking"
	KeyBuiltinModelDefault                      Key = "command.model.default"
	KeyBuiltinModelSwitchedPersistError         Key = "command.model.switched_persist_error"
	KeyBuiltinModelSwitchedSaved                Key = "command.model.switched_saved"
	KeyBuiltinModelSwitched                     Key = "command.model.switched"
	KeyBuiltinModelUnknownProvider              Key = "command.model.unknown_provider"
	KeyBuiltinModelProviderNotReady             Key = "command.model.provider_not_ready"
	KeyBuiltinModelLoadCredentialsError         Key = "command.model.load_credentials_error"
	KeyBuiltinModelCreateProviderError          Key = "command.model.create_provider_error"
	KeyBuiltinModelContext                      Key = "command.model.context"
	KeyBuiltinModelVisionBare                   Key = "command.model.vision_bare"
	KeyBuiltinModelTextOnly                     Key = "command.model.text_only"
	KeyBuiltinModelReasoningBare                Key = "command.model.reasoning_bare"
	KeyBuiltinModelProviderSwitchedPersistError Key = "command.model.provider_switched_persist_error"
	KeyBuiltinModelProviderSwitchedSaved        Key = "command.model.provider_switched_saved"
	KeyBuiltinModelProviderSwitched             Key = "command.model.provider_switched"
	KeyBuiltinSessionCurrentID                  Key = "command.session.current_id"
	KeyBuiltinSessionCurrent                    Key = "command.session.current"
	KeyBuiltinSessionCurrentError               Key = "command.session.current_error"
	KeyBuiltinSessionID                         Key = "command.session.id"
	KeyBuiltinSessionTitle                      Key = "command.session.title"
	KeyBuiltinSessionCreated                    Key = "command.session.created"
	KeyBuiltinSessionUpdated                    Key = "command.session.updated"
	KeyBuiltinSessionMessages                   Key = "command.session.messages"
	KeyBuiltinSessionGitBranch                  Key = "command.session.git_branch"
	KeyBuiltinSessionDirectory                  Key = "command.session.directory"
	KeyBuiltinSessionPreview                    Key = "command.session.preview"
	KeyBuiltinSessionStoreUnavailable           Key = "command.session.store_unavailable"
	KeyBuiltinSessionListError                  Key = "command.session.list_error"
	KeyBuiltinSessionNone                       Key = "command.session.none"
	KeyBuiltinSessionRecent                     Key = "command.session.recent"
	KeyBuiltinSessionListMessages               Key = "command.session.list_messages"
	KeyBuiltinSessionLoadUsage                  Key = "command.session.load_usage"
	KeyBuiltinSessionDeleteUsage                Key = "command.session.delete_usage"
	KeyBuiltinSessionDeleteUnavailable          Key = "command.session.delete_unavailable"
	KeyBuiltinSessionUsage                      Key = "command.session.usage"
)

func init() {
	for key, values := range map[Key][6]string{
		KeyBuiltinActivityViewUnavailable:      {"activity view is not configured", "活动视图尚未配置", "Aktivitätsansicht ist nicht konfiguriert", "アクティビティ表示が設定されていません", "활동 보기가 구성되지 않았습니다", "Представление активности не настроено"},
		KeyBuiltinActivityActionsUnavailable:   {"activity actions are not configured", "活动操作尚未配置", "Aktivitätsaktionen sind nicht konfiguriert", "アクティビティ操作が設定されていません", "활동 작업이 구성되지 않았습니다", "Действия активности не настроены"},
		KeyBuiltinActivityUsage:                {"usage: /activity [list|close|<id> cancel|jump|details|acknowledge]", "用法：/activity [list|close|<id> cancel|jump|details|acknowledge]", "Verwendung: /activity [list|close|<id> cancel|jump|details|acknowledge]", "使い方: /activity [list|close|<id> cancel|jump|details|acknowledge]", "사용법: /activity [list|close|<id> cancel|jump|details|acknowledge]", "Использование: /activity [list|close|<id> cancel|jump|details|acknowledge]"},
		KeyBuiltinDetailUsage:                  {"usage: /detail <observation-id> [summary|detail|evidence|next]", "用法：/detail <observation-id> [summary|detail|evidence|next]", "Verwendung: /detail <observation-id> [summary|detail|evidence|next]", "使い方: /detail <observation-id> [summary|detail|evidence|next]", "사용법: /detail <observation-id> [summary|detail|evidence|next]", "Использование: /detail <observation-id> [summary|detail|evidence|next]"},
		KeyBuiltinSearchUsage:                  {"usage: /search <query|--next|--previous|--close>", "用法：/search <query|--next|--previous|--close>", "Verwendung: /search <query|--next|--previous|--close>", "使い方: /search <query|--next|--previous|--close>", "사용법: /search <query|--next|--previous|--close>", "Использование: /search <query|--next|--previous|--close>"},
		KeyBuiltinSearchUnavailable:            {"transcript search is not configured", "对话记录搜索尚未配置", "Transkriptsuche ist nicht konfiguriert", "会話履歴検索が設定されていません", "대화 기록 검색이 구성되지 않았습니다", "Поиск по расшифровке не настроен"},
		KeyBuiltinExportUnavailable:            {"transcript export is not configured", "对话记录导出尚未配置", "Transkriptexport ist nicht konfiguriert", "会話履歴のエクスポートが設定されていません", "대화 기록 내보내기가 구성되지 않았습니다", "Экспорт расшифровки не настроен"},
		KeyBuiltinExported:                     {"Exported transcript: %s", "已导出对话记录：%s", "Transkript exportiert: %s", "会話履歴をエクスポートしました: %s", "대화 기록을 내보냈습니다: %s", "Расшифровка экспортирована: %s"},
		KeyBuiltinEditorDetailUsage:            {"usage: /editor detail <observation-id>", "用法：/editor detail <observation-id>", "Verwendung: /editor detail <observation-id>", "使い方: /editor detail <observation-id>", "사용법: /editor detail <observation-id>", "Использование: /editor detail <observation-id>"},
		KeyBuiltinEditorUnavailable:            {"transcript editor is not configured", "对话记录编辑器尚未配置", "Transkript-Editor ist nicht konfiguriert", "会話履歴エディターが設定されていません", "대화 기록 편집기가 구성되지 않았습니다", "Редактор расшифровки не настроен"},
		KeyBuiltinMouseUnavailable:             {"mouse capture control is not configured", "鼠标捕获控制尚未配置", "Maussteuerung ist nicht konfiguriert", "マウスキャプチャ制御が設定されていません", "마우스 캡처 제어가 구성되지 않았습니다", "Управление захватом мыши не настроено"},
		KeyBuiltinMouseUsage:                   {"usage: /mouse [on|off|toggle]", "用法：/mouse [on|off|toggle]", "Verwendung: /mouse [on|off|toggle]", "使い方: /mouse [on|off|toggle]", "사용법: /mouse [on|off|toggle]", "Использование: /mouse [on|off|toggle]"},
		KeyBuiltinMouseState:                   {"Mouse capture: %t", "鼠标捕获：%t", "Mausaufnahme: %t", "マウスキャプチャ: %t", "마우스 캡처: %t", "Захват мыши: %t"},
		KeyBuiltinHelpCommands:                 {"Available commands:\n", "可用命令：\n", "Verfügbare Befehle:\n", "利用可能なコマンド:\n", "사용 가능한 명령:\n", "Доступные команды:\n"},
		KeyBuiltinHelpShortcuts:                {"Fullscreen shortcuts:\n", "全屏快捷键：\n", "Vollbild-Kurzbefehle:\n", "全画面ショートカット:\n", "전체 화면 단축키:\n", "Полноэкранные сочетания клавиш:\n"},
		KeyBuiltinHelpShortcutCycle:            {"  Ctrl+O  Cycle focused observation summary/detail/evidence\n", "  Ctrl+O  循环切换焦点观察项的摘要/详情/证据\n", "  Ctrl+O  Fokusbeobachtung: Zusammenfassung/Details/Belege wechseln\n", "  Ctrl+O  注目中の観測の要約/詳細/根拠を切替\n", "  Ctrl+O  선택한 관찰의 요약/상세/근거 전환\n", "  Ctrl+O  Переключать сводку/детали/доказательства выбранного наблюдения\n"},
		KeyBuiltinHelpShortcutToggle:           {"  Alt+O   Toggle all transcript evidence without changing local disclosure\n", "  Alt+O   切换所有对话记录证据，不改变本地披露级别\n", "  Alt+O   Alle Transkriptbelege umschalten, ohne lokale Offenlegung zu ändern\n", "  Alt+O   ローカル表示を変えずに全会話履歴の根拠を切替\n", "  Alt+O   로컬 공개 수준을 바꾸지 않고 모든 대화 근거 전환\n", "  Alt+O   Переключать все доказательства расшифровки без изменения локального уровня\n"},
		KeyBuiltinHelpShortcutJump:             {"  Ctrl+Home / Ctrl+End  Jump to transcript start/end\n", "  Ctrl+Home / Ctrl+End  跳到对话记录开头/结尾\n", "  Ctrl+Home / Ctrl+End  Zum Anfang/Ende des Transkripts springen\n", "  Ctrl+Home / Ctrl+End  会話履歴の先頭/末尾へ移動\n", "  Ctrl+Home / Ctrl+End  대화 기록 처음/끝으로 이동\n", "  Ctrl+Home / Ctrl+End  Перейти к началу/концу расшифровки\n"},
		KeyBuiltinHelpShortcutScroll:           {"  PageUp / PageDown  Scroll transcript or active decision\n", "  PageUp / PageDown  滚动对话记录或当前决策\n", "  PageUp / PageDown  Transkript oder aktive Entscheidung scrollen\n", "  PageUp / PageDown  会話履歴または現在の判断をスクロール\n", "  PageUp / PageDown  대화 기록 또는 활성 결정을 스크롤\n", "  PageUp / PageDown  Прокручивать расшифровку или активное решение\n"},
		KeyBuiltinHelpShortcutClose:            {"  Escape  Close the current overlay and restore input focus\n", "  Escape  关闭当前覆盖层并恢复输入焦点\n", "  Escape  Aktuelles Overlay schließen und Eingabefokus wiederherstellen\n", "  Escape  現在のオーバーレイを閉じて入力フォーカスを戻す\n", "  Escape  현재 오버레이를 닫고 입력 포커스 복원\n", "  Escape  Закрыть текущий оверлей и вернуть фокус ввода\n"},
		KeyBuiltinClearViewUnavailable:         {"clear view is not configured", "清空视图尚未配置", "Ansicht löschen ist nicht konfiguriert", "ビューのクリアが設定されていません", "보기 지우기가 구성되지 않았습니다", "Очистка представления не настроена"},
		KeyBuiltinClearConversationUnavailable: {"clear conversation is not configured", "清空对话尚未配置", "Konversation löschen ist nicht konfiguriert", "会話のクリアが設定されていません", "대화 지우기가 구성되지 않았습니다", "Очистка беседы не настроена"},
		KeyBuiltinClearUsage:                   {"usage: /clear [view|conversation]", "用法：/clear [view|conversation]", "Verwendung: /clear [view|conversation]", "使い方: /clear [view|conversation]", "사용법: /clear [view|conversation]", "Использование: /clear [view|conversation]"},
		KeyBuiltinCompactEmpty:                 {"Nothing to compact — conversation history is empty.\n", "无需压缩——对话历史为空。\n", "Nichts zu komprimieren — der Gesprächsverlauf ist leer.\n", "圧縮する内容はありません。会話履歴が空です。\n", "압축할 내용이 없습니다. 대화 기록이 비어 있습니다.\n", "Сжимать нечего — история беседы пуста.\n"},
		KeyBuiltinCompactUnavailable:           {"compaction is not configured", "压缩功能尚未配置", "Komprimierung ist nicht konfiguriert", "圧縮機能が設定されていません", "압축 기능이 구성되지 않았습니다", "Сжатие не настроено"},
		KeyBuiltinCompactRunning:               {"Compacting context with LLM summary...\n", "正在使用 LLM 摘要压缩上下文…\n", "Kontext wird mit einer LLM-Zusammenfassung komprimiert …\n", "LLM要約でコンテキストを圧縮しています…\n", "LLM 요약으로 컨텍스트를 압축하는 중…\n", "Сжатие контекста с помощью сводки LLM…\n"},
		KeyBuiltinCompactComplete:              {"Context compacted: %d → %d messages (LLM summary).\n", "上下文已压缩：%d → %d 条消息（LLM 摘要）。\n", "Kontext komprimiert: %d → %d Nachrichten (LLM-Zusammenfassung).\n", "コンテキストを圧縮しました: %d → %d 件のメッセージ（LLM要約）。\n", "컨텍스트를 압축했습니다: 메시지 %d개 → %d개(LLM 요약).\n", "Контекст сжат: %d → %d сообщений (сводка LLM).\n"},
		KeyBuiltinModelCurrent:                 {"Current: %s/%s\n", "当前：%s/%s\n", "Aktuell: %s/%s\n", "現在: %s/%s\n", "현재: %s/%s\n", "Текущий: %s/%s\n"},
		KeyBuiltinModelAvailable:               {"\nAvailable models:\n", "\n可用模型：\n", "\nVerfügbare Modelle:\n", "\n利用可能なモデル:\n", "\n사용 가능한 모델:\n", "\nДоступные модели:\n"},
		KeyBuiltinModelProvider:                {"  %s (%s):\n", "  %s（%s）：\n", "  %s (%s):\n", "  %s（%s）:\n", "  %s(%s):\n", "  %s (%s):\n"},
		KeyBuiltinModelVision:                  {" (vision)", "（视觉）", " (Bildverarbeitung)", "（画像対応）", " (이미지)", " (зрение)"}, KeyBuiltinModelText: {" (text)", "（文本）", " (Text)", "（テキスト）", " (텍스트)", " (текст)"}, KeyBuiltinModelReasoningEfforts: {" (reasoning: %s)", "（推理：%s）", " (Denken: %s)", "（推論: %s）", " (추론: %s)", " (рассуждение: %s)"}, KeyBuiltinModelThinking: {" (thinking)", "（思考）", " (Denken)", "（思考）", " (추론)", " (рассуждение)"}, KeyBuiltinModelDefault: {" [default]", "［默认］", " [Standard]", "［既定］", " [기본값]", " [по умолчанию]"},
		KeyBuiltinModelSwitchedPersistError: {"Model switched to: %s\nWarning: failed to persist model: %v\n", "模型已切换为：%s\n警告：无法保存模型：%v\n", "Modell gewechselt zu: %s\nWarnung: Modell konnte nicht gespeichert werden: %v\n", "モデルを切り替えました: %s\n警告: モデルを保存できませんでした: %v\n", "모델을 전환했습니다: %s\n경고: 모델을 저장하지 못했습니다: %v\n", "Модель переключена на: %s\nПредупреждение: не удалось сохранить модель: %v\n"},
		KeyBuiltinModelSwitchedSaved:        {"Model switched to: %s\nSaved model in %s\n", "模型已切换为：%s\n模型已保存到 %s\n", "Modell gewechselt zu: %s\nModell in %s gespeichert\n", "モデルを切り替えました: %s\nモデルを %s に保存しました\n", "모델을 전환했습니다: %s\n모델을 %s에 저장했습니다\n", "Модель переключена на: %s\nМодель сохранена в %s\n"},
		KeyBuiltinModelSwitched:             {"Model switched to: %s\n", "模型已切换为：%s\n", "Modell gewechselt zu: %s\n", "モデルを切り替えました: %s\n", "모델을 전환했습니다: %s\n", "Модель переключена на: %s\n"},
		KeyBuiltinModelUnknownProvider:      {"unknown provider %q — available: %s", "未知 Provider %q——可用：%s", "unbekannter Anbieter %q — verfügbar: %s", "不明なProvider %q — 利用可能: %s", "알 수 없는 Provider %q — 사용 가능: %s", "неизвестный провайдер %q — доступные: %s"},
		KeyBuiltinModelProviderNotReady:     {"provider %q is not ready: %s", "Provider %q 尚未就绪：%s", "Anbieter %q ist nicht bereit: %s", "Provider %q の準備ができていません: %s", "Provider %q가 준비되지 않았습니다: %s", "провайдер %q не готов: %s"},
		KeyBuiltinModelLoadCredentialsError: {"failed to load credentials for provider %q: %w", "无法加载 Provider %q 的凭据：%w", "Anmeldedaten für Anbieter %q konnten nicht geladen werden: %w", "Provider %q の認証情報を読み込めませんでした: %w", "Provider %q의 자격 증명을 불러오지 못했습니다: %w", "не удалось загрузить учетные данные провайдера %q: %w"},
		KeyBuiltinModelCreateProviderError:  {"failed to create provider %q: %w", "无法创建 Provider %q：%w", "Anbieter %q konnte nicht erstellt werden: %w", "Provider %q を作成できませんでした: %w", "Provider %q를 만들지 못했습니다: %w", "не удалось создать провайдера %q: %w"},
		KeyBuiltinModelContext:              {"%s context", "%s 上下文", "%s Kontext", "%s コンテキスト", "%s 컨텍스트", "%s контекста"}, KeyBuiltinModelVisionBare: {"vision", "视觉", "Bildverarbeitung", "画像対応", "이미지", "зрение"}, KeyBuiltinModelTextOnly: {"text-only", "仅文本", "nur Text", "テキストのみ", "텍스트 전용", "только текст"}, KeyBuiltinModelReasoningBare: {"reasoning", "推理", "Denken", "推論", "추론", "рассуждение"},
		KeyBuiltinModelProviderSwitchedPersistError: {"✅ Switched to %s/%s%s\nWarning: failed to persist provider/model: %v\n", "✅ 已切换到 %s/%s%s\n警告：无法保存 Provider/模型：%v\n", "✅ Gewechselt zu %s/%s%s\nWarnung: Anbieter/Modell konnte nicht gespeichert werden: %v\n", "✅ %s/%s%s に切り替えました\n警告: Provider/モデルを保存できませんでした: %v\n", "✅ %s/%s%s(으)로 전환했습니다\n경고: Provider/모델을 저장하지 못했습니다: %v\n", "✅ Переключено на %s/%s%s\nПредупреждение: не удалось сохранить провайдера/модель: %v\n"},
		KeyBuiltinModelProviderSwitchedSaved:        {"✅ Switched to %s/%s%s\nSaved provider/model in %s\n", "✅ 已切换到 %s/%s%s\nProvider/模型已保存到 %s\n", "✅ Gewechselt zu %s/%s%s\nAnbieter/Modell in %s gespeichert\n", "✅ %s/%s%s に切り替えました\nProvider/モデルを %s に保存しました\n", "✅ %s/%s%s(으)로 전환했습니다\nProvider/모델을 %s에 저장했습니다\n", "✅ Переключено на %s/%s%s\nПровайдер/модель сохранены в %s\n"},
		KeyBuiltinModelProviderSwitched:             {"✅ Switched to %s/%s%s\n", "✅ 已切换到 %s/%s%s\n", "✅ Gewechselt zu %s/%s%s\n", "✅ %s/%s%s に切り替えました\n", "✅ %s/%s%s(으)로 전환했습니다\n", "✅ Переключено на %s/%s%s\n"},
		KeyBuiltinSessionCurrentID:                  {"Current session: %s\n", "当前会话：%s\n", "Aktuelle Sitzung: %s\n", "現在のセッション: %s\n", "현재 세션: %s\n", "Текущий сеанс: %s\n"}, KeyBuiltinSessionCurrent: {"Current session\n", "当前会话\n", "Aktuelle Sitzung\n", "現在のセッション\n", "현재 세션\n", "Текущий сеанс\n"},
		KeyBuiltinSessionCurrentError: {"Unable to load current session details: %s\nCurrent session ID: %s\n", "无法加载当前会话详情：%s\n当前会话 ID：%s\n", "Details der aktuellen Sitzung konnten nicht geladen werden: %s\nID der aktuellen Sitzung: %s\n", "現在のセッションの詳細を読み込めませんでした: %s\n現在のセッション ID: %s\n", "현재 세션 세부 정보를 불러올 수 없습니다: %s\n현재 세션 ID: %s\n", "Не удалось загрузить сведения о текущем сеансе: %s\nID текущего сеанса: %s\n"},
		KeyBuiltinSessionID:           {"  ID:            %s\n", "  ID：           %s\n", "  ID:            %s\n", "  ID:            %s\n", "  ID:            %s\n", "  ID:            %s\n"}, KeyBuiltinSessionTitle: {"  Title:         %s\n", "  标题：         %s\n", "  Titel:         %s\n", "  タイトル:      %s\n", "  제목:          %s\n", "  Заголовок:     %s\n"}, KeyBuiltinSessionCreated: {"  Created:       %s\n", "  创建时间：     %s\n", "  Erstellt:      %s\n", "  作成日時:      %s\n", "  생성됨:        %s\n", "  Создан:        %s\n"}, KeyBuiltinSessionUpdated: {"  Updated:       %s\n", "  更新时间：     %s\n", "  Aktualisiert:  %s\n", "  更新日時:      %s\n", "  업데이트됨:    %s\n", "  Обновлен:      %s\n"}, KeyBuiltinSessionMessages: {"  Messages:      %d\n", "  消息数：       %d\n", "  Nachrichten:   %d\n", "  メッセージ:    %d\n", "  메시지:        %d\n", "  Сообщений:     %d\n"}, KeyBuiltinSessionGitBranch: {"  Git branch:    %s\n", "  Git 分支：     %s\n", "  Git-Branch:    %s\n", "  Git ブランチ:  %s\n", "  Git 브랜치:    %s\n", "  Ветка Git:     %s\n"}, KeyBuiltinSessionDirectory: {"  Directory:     %s\n", "  目录：         %s\n", "  Verzeichnis:   %s\n", "  ディレクトリ:  %s\n", "  디렉터리:      %s\n", "  Каталог:       %s\n"}, KeyBuiltinSessionPreview: {"  Preview:       %s\n", "  预览：         %s\n", "  Vorschau:      %s\n", "  プレビュー:    %s\n", "  미리보기:      %s\n", "  Предпросмотр:  %s\n"},
		KeyBuiltinSessionStoreUnavailable: {"Session store not available.\n", "会话存储不可用。\n", "Sitzungsspeicher nicht verfügbar.\n", "セッションストアは利用できません。\n", "세션 저장소를 사용할 수 없습니다.\n", "Хранилище сеансов недоступно.\n"}, KeyBuiltinSessionListError: {"Error listing sessions: %s\n", "列出会话时出错：%s\n", "Fehler beim Auflisten der Sitzungen: %s\n", "セッション一覧の取得エラー: %s\n", "세션 목록 오류: %s\n", "Ошибка при выводе списка сеансов: %s\n"}, KeyBuiltinSessionNone: {"No saved sessions found.\n", "未找到已保存的会话。\n", "Keine gespeicherten Sitzungen gefunden.\n", "保存済みのセッションが見つかりません。\n", "저장된 세션을 찾을 수 없습니다.\n", "Сохраненные сеансы не найдены.\n"}, KeyBuiltinSessionRecent: {"Recent sessions:\n", "最近会话：\n", "Letzte Sitzungen:\n", "最近のセッション:\n", "최근 세션:\n", "Недавние сеансы:\n"}, KeyBuiltinSessionListMessages: {", %d msgs", "，%d 条消息", ", %d Nachr.", "、%d 件", ", 메시지 %d개", ", %d сообщ."}, KeyBuiltinSessionLoadUsage: {"Usage: /session load <id-or-title>\n", "用法：/session load <id-or-title>\n", "Verwendung: /session load <id-or-title>\n", "使い方: /session load <id-or-title>\n", "사용법: /session load <id-or-title>\n", "Использование: /session load <id-or-title>\n"}, KeyBuiltinSessionDeleteUsage: {"usage: /session delete <session-id>", "用法：/session delete <session-id>", "Verwendung: /session delete <session-id>", "使い方: /session delete <session-id>", "사용법: /session delete <session-id>", "Использование: /session delete <session-id>"}, KeyBuiltinSessionDeleteUnavailable: {"session history deletion is not configured", "会话历史删除功能尚未配置", "Löschen des Sitzungsverlaufs ist nicht konfiguriert", "セッション履歴の削除が設定されていません", "세션 기록 삭제가 구성되지 않았습니다", "Удаление истории сеансов не настроено"}, KeyBuiltinSessionUsage: {"Usage: /session [current|list [query]|load <id-or-title>|rename <name>|delete <session-id>]\n", "用法：/session [current|list [query]|load <id-or-title>|rename <name>|delete <session-id>]\n", "Verwendung: /session [current|list [query]|load <id-or-title>|rename <name>|delete <session-id>]\n", "使い方: /session [current|list [query]|load <id-or-title>|rename <name>|delete <session-id>]\n", "사용법: /session [current|list [query]|load <id-or-title>|rename <name>|delete <session-id>]\n", "Использование: /session [current|list [query]|load <id-or-title>|rename <name>|delete <session-id>]\n"},
	} {
		semanticTranslations[key] = commandBuiltins(values[0], values[1], values[2], values[3], values[4], values[5])
	}
}

func commandBuiltins(en, zh, de, ja, ko, ru string) map[Language]string {
	return map[Language]string{LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru}
}
