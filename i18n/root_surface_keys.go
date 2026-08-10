package i18n

// Semantic keys for user-visible details rendered by tui/root.go. Dynamic
// values (tool output, IDs, provider names, and paths) are intentionally kept
// as parameters so this catalog translates only product copy.
const (
	KeyTasksTitle                       Key = "tui.tasks.title"
	KeyTasksBlockedBy                   Key = "tui.tasks.blocked_by"
	KeyTasksMore                        Key = "tui.tasks.more"
	KeyObservationInput                 Key = "tui.observation.input"
	KeyObservationEvidenceUnavailable   Key = "tui.observation.evidence_unavailable"
	KeyObservationStructuredUnavailable Key = "tui.observation.structured_unavailable"
	KeyObservationStructuredEvidence    Key = "tui.observation.structured_evidence"
	KeyObservationEvidenceID            Key = "tui.observation.evidence_id"
	KeyObservationEvidenceIdentity      Key = "tui.observation.evidence_identity"
	KeyAggregateMembers                 Key = "tui.aggregate.members"
	KeyAggregateState                   Key = "tui.aggregate.state"
	KeyAggregateObjects                 Key = "tui.aggregate.objects"
	KeyAggregateMoreMembers             Key = "tui.aggregate.more_members"
	KeyAggregateEvidenceAvailable       Key = "tui.aggregate.evidence_available"
	KeyAggregateLive                    Key = "tui.aggregate.live"
	KeyAggregateFrozen                  Key = "tui.aggregate.frozen"
	KeyThinkingTitle                    Key = "tui.thinking.title"
	KeyActivityWorking                  Key = "tui.activity.working"
	KeyDecisionReceipt                  Key = "tui.decision.receipt"
	KeyActivitiesTitle                  Key = "tui.activities.title"
	KeyActivityUnassigned               Key = "tui.activity.unassigned"
	KeyActivityWorkActor                Key = "tui.activity.work_actor"
	KeyActivityActor                    Key = "tui.activity.actor"
	KeyActivityDetail                   Key = "tui.activity.detail"
	KeyActivityDescendants              Key = "tui.activity.descendants"
	KeyLLMRequestProblem                Key = "tui.llm_request.problem"
	KeyLLMRequestRetrying               Key = "tui.llm_request.retrying"
	KeyLLMRequestRequestRetrying        Key = "tui.llm_request.request_retrying"
	KeyLLMRequestReconnecting           Key = "tui.llm_request.reconnecting"
	KeyLLMRequestProblemDetail          Key = "tui.llm_request.problem_detail"
	KeyLLMRequestAttempt                Key = "tui.llm_request.attempt"
	KeyLLMRequestError                  Key = "tui.llm_request.error"
	KeyLLMRequestMetrics                Key = "tui.llm_request.metrics"
	KeyLLMRequestInterruptStatus        Key = "tui.llm_request.interrupt_status"
	KeyAssistantWorkedFor               Key = "tui.assistant.worked_for"
	KeySessionPickerTitle               Key = "tui.session_picker.title"
	KeySessionPickerQuery               Key = "tui.session_picker.query"
	KeySessionPickerEmpty               Key = "tui.session_picker.empty"
	KeySessionPickerMessages            Key = "tui.session_picker.messages"
	KeyForkPickerTitle                  Key = "tui.fork_picker.title"
	KeyForkPickerContextOnly            Key = "tui.fork_picker.context_only"
	KeyForkPickerEmpty                  Key = "tui.fork_picker.empty"
	KeyForkPickerReply                  Key = "tui.fork_picker.reply"
	KeyForkPickerShowing                Key = "tui.fork_picker.showing"
	KeyProviderPickerTitle              Key = "tui.provider_picker.title"
	KeyProviderPickerEmpty              Key = "tui.provider_picker.empty"
	KeyProviderPickerNotConnected       Key = "tui.provider_picker.not_connected"
	KeyProviderPickerCount              Key = "tui.provider_picker.count"
	KeyModelPickerTitle                 Key = "tui.model_picker.title"
	KeyModelPickerProviderHint          Key = "tui.model_picker.provider_hint"
	KeyModelPickerFilter                Key = "tui.model_picker.filter"
	KeyModelPickerEmpty                 Key = "tui.model_picker.empty"
	KeyModelPickerDefault               Key = "tui.model_picker.default"
	KeyModelPickerCount                 Key = "tui.model_picker.count"
	KeyModelPickerEffortHint            Key = "tui.model_picker.effort_hint"
	KeyModelLimitEditTitle              Key = "tui.model_limit_edit.title"
	KeyModelLimitEditCurrent            Key = "tui.model_limit_edit.current"
	KeyModelLimitEditInput              Key = "tui.model_limit_edit.input"
	KeyModelLimitEditHint               Key = "tui.model_limit_edit.hint"
	KeyModelLimitEditUnknown            Key = "tui.model_limit_edit.unknown"
	KeyModelLimitEditOverridden         Key = "tui.model_limit_edit.overridden"
	KeyReasoningPickerTitle             Key = "tui.reasoning_picker.title"
	KeyReasoningPickerEmpty             Key = "tui.reasoning_picker.empty"
	KeyReasoningPickerConfirm           Key = "tui.reasoning_picker.confirm"
	KeyClipboardCopyFailed              Key = "tui.clipboard.copy_failed"
	KeyClipboardCopied                  Key = "tui.clipboard.copied"
	KeyClipboardNothing                 Key = "tui.clipboard.nothing"
	KeyTranscriptSelectionHintOption    Key = "tui.transcript.selection_hint.option"
	KeyTranscriptSelectionHintGeneric   Key = "tui.transcript.selection_hint.generic"
	KeyImageUnsupported                 Key = "tui.image.unsupported"
	KeyImageCheckingClipboard           Key = "tui.image.checking_clipboard"
	KeyImageClipboardError              Key = "tui.image.clipboard_error"
	KeyImageClipboardEmpty              Key = "tui.image.clipboard_empty"
	KeyImagePasted                      Key = "tui.image.pasted"
	KeyImageOpenFailed                  Key = "tui.image.open_failed"
)

func init() {
	rootSurface := func(en, zh, de, ja, ko, ru string) map[Language]string {
		return map[Language]string{LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru}
	}
	for key, t := range map[Key]map[Language]string{
		KeyTasksTitle: rootSurface(" Tasks (%d)", " 任务（%d）", " Aufgaben (%d)", " タスク（%d）", " 작업 (%d)", " Задачи (%d)"), KeyTasksBlockedBy: rootSurface(" (blocked by %s)", "（被 %s 阻塞）", " (blockiert durch %s)", "（%s によりブロック）", " (%s에 의해 차단됨)", " (заблокировано: %s)"), KeyTasksMore: rootSurface(" ... %d more", " ... 另有 %d 项", " ... %d weitere", " ... ほか %d 件", " ... %d개 더", " ... ещё %d"),
		KeyObservationInput: rootSurface("  Input:\n%s", "  输入：\n%s", "  Eingabe:\n%s", "  入力:\n%s", "  입력:\n%s", "  Ввод:\n%s"), KeyObservationEvidenceUnavailable: rootSurface("  Evidence unavailable: %s", "  证据不可用：%s", "  Belege nicht verfügbar: %s", "  証拠を利用できません: %s", "  증거를 사용할 수 없습니다: %s", "  Доказательства недоступны: %s"), KeyObservationStructuredUnavailable: rootSurface("  Structured evidence unavailable: %s", "  结构化证据不可用：%s", "  Strukturierte Belege nicht verfügbar: %s", "  構造化された証拠を利用できません: %s", "  구조화된 증거를 사용할 수 없습니다: %s", "  Структурированные доказательства недоступны: %s"), KeyObservationStructuredEvidence: rootSurface("  Structured evidence:\n%s", "  结构化证据：\n%s", "  Strukturierte Belege:\n%s", "  構造化された証拠:\n%s", "  구조화된 증거:\n%s", "  Структурированные доказательства:\n%s"), KeyObservationEvidenceID: rootSurface("  Evidence ID: %s", "  证据 ID：%s", "  Beleg-ID: %s", "  証拠 ID: %s", "  증거 ID: %s", "  ID доказательства: %s"),
		KeyObservationEvidenceIdentity: rootSurface(" — actor %s — work unit %s", " — 执行者 %s — 工作单元 %s", " — Akteur %s — Arbeitseinheit %s", " — 実行者 %s — 作業単位 %s", " — 실행자 %s — 작업 단위 %s", " — исполнитель %s — рабочая единица %s"), KeyAggregateMembers: rootSurface("Aggregate members: %d", "聚合成员：%d", "Aggregierte Mitglieder: %d", "集約メンバー: %d", "집계 구성원: %d", "Агрегированные участники: %d"), KeyAggregateState: rootSurface("Aggregate state: %s — %d objects — %d evidence refs", "聚合状态：%s — %d 个对象 — %d 个证据引用", "Aggregatzustand: %s — %d Objekte — %d Belegverweise", "集約状態: %s — %d オブジェクト — %d 証拠参照", "집계 상태: %s — %d개 객체 — %d개 증거 참照", "Состояние агрегата: %s — %d объектов — %d ссылок на доказательства"), KeyAggregateObjects: rootSurface("Objects: %s", "对象：%s", "Objekte: %s", "オブジェクト: %s", "객체: %s", "Объекты: %s"), KeyAggregateMoreMembers: rootSurface("… %d more members; use show-all, search, or export", "… 还有 %d 个成员；可使用显示全部、搜索或导出", "… %d weitere Mitglieder; nutze alle anzeigen, Suche oder Export", "… ほか %d メンバー。すべて表示、検索、またはエクスポートを使用", "… 구성원 %d명 더 있음. 모두 표시, 검색 또는 내보내기 사용", "… ещё %d участников; используйте показать всё, поиск или экспорт"), KeyAggregateEvidenceAvailable: rootSurface("evidence available", "证据可用", "Belege verfügbar", "証拠あり", "증거 있음", "доказательства доступны"), KeyAggregateLive: rootSurface("live", "实时", "live", "ライブ", "실시간", "активно"), KeyAggregateFrozen: rootSurface("frozen", "冻结", "eingefroren", "固定", "고정됨", "заморожено"),
		KeyThinkingTitle:   rootSurface("💭 Thinking:", "💭 思考中：", "💭 Denkt nach:", "💭 思考中:", "💭 생각 중:", "💭 Размышляет:"),
		KeyActivityWorking: rootSurface("Working", "工作中", "In Arbeit", "作業中", "작업 중", "В работе"), KeyActivityRunning: rootSurface("%d running", "%d 项运行中", "%d laufen", "%d 件実行中", "%d개 실행 중", "%d выполняются"), KeyDecisionReceipt: rootSurface("  Decision receipt: %s", "  决策回执：%s", "  Entscheidungsbeleg: %s", "  決定の記録: %s", "  결정 영수증: %s", "  Квитанция решения: %s"), KeyActivitiesTitle: rootSurface("Activities", "活动", "Aktivitäten", "アクティビティ", "활동", "Действия"), KeyActivityUnassigned: rootSurface("unassigned", "未分配", "nicht zugewiesen", "未割り当て", "미할당", "не назначено"), KeyActivityWorkActor: rootSurface("work=%s actor=%s > ", "工作=%s 执行者=%s > ", "Arbeit=%s Akteur=%s > ", "作業=%s 実行者=%s > ", "작업=%s 실행자=%s > ", "работа=%s исполнитель=%s > "), KeyActivityActor: rootSurface("  actor=%s > ", "  执行者=%s > ", "  Akteur=%s > ", "  実行者=%s > ", "  실행자=%s > ", "  исполнитель=%s > "), KeyActivityDetail: rootSurface("%s %s%s  %s/%s outcome=%s  %s", "%s %s%s  %s/%s 结果=%s  %s", "%s %s%s  %s/%s Ergebnis=%s  %s", "%s %s%s  %s/%s 結果=%s  %s", "%s %s%s  %s/%s 결과=%s  %s", "%s %s%s  %s/%s результат=%s  %s"), KeyActivityDescendants: rootSurface("  descendants=%d worst=%s", "  后代=%d 最差=%s", "  Nachkommen=%d schlechtester=%s", "  子孫=%d 最悪=%s", "  하위=%d 최악=%s", "  потомков=%d худший=%s"),
		KeyLLMRequestProblem:         rootSurface("LLM API issue", "LLM API 请求出错", "LLM-API-Problem", "LLM API の問題", "LLM API 문제", "Проблема LLM API"),
		KeyLLMRequestRetrying:        rootSurface("Retry %d/%d in %s", "第 %d/%d 次重试 · %s 后继续", "Wiederholung %d/%d in %s", "%d/%d 回目の再試行 · %s 後", "재시도 %d/%d · %s 후", "Повтор %d/%d через %s"),
		KeyLLMRequestRequestRetrying: rootSurface("Request retry %d/%d in %s", "请求重试 %d/%d · %s 后继续", "Anfragewiederholung %d/%d in %s", "リクエスト再試行 %d/%d · %s 後", "요청 재시도 %d/%d · %s 후", "Повтор запроса %d/%d через %s"),
		KeyLLMRequestReconnecting:    rootSurface("Reconnecting %d/%d in %s", "正在重连 %d/%d · %s 后继续", "Neu verbinden %d/%d in %s", "再接続 %d/%d · %s 後", "재연결 %d/%d · %s 후", "Переподключение %d/%d через %s"),
		KeyLLMRequestProblemDetail:   rootSurface("Problem: %s", "问题：%s", "Problem: %s", "問題: %s", "문제: %s", "Проблема: %s"),
		KeyLLMRequestAttempt:         rootSurface("Attempt %d/%d", "尝试 %d/%d", "Versuch %d/%d", "試行 %d/%d", "시도 %d/%d", "Попытка %d/%d"),
		KeyLLMRequestError:           rootSurface("Error: %s", "错误：%s", "Fehler: %s", "エラー: %s", "오류: %s", "Ошибка: %s"),
		KeyLLMRequestMetrics:         rootSurface("Connection %s · First token %s", "建立连接 %s · 首 token %s", "Verbindung %s · Erstes Token %s", "接続 %s · 最初の token %s", "연결 %s · 첫 token %s", "Соединение %s · Первый token %s"),
		KeyLLMRequestInterruptStatus: rootSurface("(%s • Ctrl+C to interrupt)", "(%s • Ctrl+C 中断)", "(%s • Ctrl+C zum Unterbrechen)", "(%s • Ctrl+C で中断)", "(%s • Ctrl+C로 중단)", "(%s • Ctrl+C — прервать)"),
		KeySessionPickerTitle:        rootSurface("Resume session — ↑/↓ select, Enter apply, Esc close", "恢复会话 — ↑/↓ 选择，Enter 应用，Esc 关闭", "Sitzung fortsetzen — ↑/↓ wählen, Enter anwenden, Esc schließen", "セッションを再開 — ↑/↓で選択、Enterで適用、Escで閉じる", "세션 재개 — ↑/↓ 선택, Enter 적용, Esc 닫기", "Продолжить сеанс — ↑/↓ выбрать, Enter применить, Esc закрыть"), KeySessionPickerQuery: rootSurface("Query: %s", "查询：%s", "Abfrage: %s", "検索: %s", "검색: %s", "Запрос: %s"), KeySessionPickerEmpty: rootSurface("No sessions found.", "未找到会话。", "Keine Sitzungen gefunden.", "セッションが見つかりません。", "세션을 찾을 수 없습니다.", "Сеансы не найдены."), KeySessionPickerMessages: rootSurface("  (%d messages)", "  （%d 条消息）", "  (%d Nachrichten)", "  （%d 件のメッセージ）", "  (메시지 %d개)", "  (%d сообщений)"),
		KeyForkPickerTitle: rootSurface("Available fork points (newest first) — ↑/↓ select, Enter fork, Esc close", "可用分叉点（最新优先）— ↑/↓ 选择，Enter 分叉，Esc 关闭", "Verfügbare Fork-Punkte (neueste zuerst) — ↑/↓ wählen, Enter forken, Esc schließen", "利用可能なフォーク地点（新しい順）— ↑/↓で選択、Enterでフォーク、Escで閉じる", "사용 가능한 포크 지점(최신순) — ↑/↓ 선택, Enter 포크, Esc 닫기", "Доступные точки ветвления (сначала новые) — ↑/↓ выбрать, Enter создать ветку, Esc закрыть"), KeyForkPickerContextOnly: rootSurface("Only turns retained in the active model context are listed.", "仅列出当前模型上下文中保留的轮次。", "Es werden nur im aktiven Modellkontext erhaltene Züge aufgelistet.", "アクティブなモデルコンテキストに残っているターンのみ表示します。", "활성 모델 컨텍스트에 유지된 턴만 표시됩니다.", "Показаны только ходы, сохранённые в активном контексте модели."), KeyForkPickerEmpty: rootSurface("No conversation turns available to fork.", "没有可分叉的对话轮次。", "Keine Gesprächszüge zum Forken verfügbar.", "フォーク可能な会話ターンはありません。", "포크할 수 있는 대화 턴이 없습니다.", "Нет доступных ходов диалога для ветвления."), KeyForkPickerReply: rootSurface("    Reply: %s", "    回复：%s", "    Antwort: %s", "    返信: %s", "    답변: %s", "    Ответ: %s"), KeyForkPickerShowing: rootSurface("    Showing %d–%d of %d", "    显示第 %d–%d 项，共 %d 项", "    Zeige %d–%d von %d", "    %d～%d / %d を表示", "    %d–%d / %d 표시", "    Показано %d–%d из %d"),
		KeyProviderPickerTitle: rootSurface("Switch Model — Select Provider  %s", "切换模型 — 选择提供商  %s", "Modell wechseln — Anbieter auswählen  %s", "モデルを切り替え — プロバイダーを選択  %s", "모델 전환 — 공급자 선택  %s", "Сменить модель — выбрать провайдера  %s"), KeyProviderPickerEmpty: rootSurface("No providers available.", "没有可用提供商。", "Keine Anbieter verfügbar.", "利用可能なプロバイダーはありません。", "사용 가능한 공급자가 없습니다.", "Нет доступных провайдеров."), KeyProviderPickerNotConnected: rootSurface(" — not connected", " — 未连接", " — nicht verbunden", " — 未接続", " — 연결되지 않음", " — не подключён"), KeyProviderPickerCount: rootSurface("  (%d/%d providers)", "  （%d/%d 个提供商）", "  (%d/%d Anbieter)", "  （%d/%d プロバイダー）", "  (공급자 %d/%d)", "  (%d/%d провайдеров)"),
		KeyModelPickerTitle: rootSurface("Select Model and Effort", "选择模型和推理强度", "Modell und Denkaufwand auswählen", "モデルと推論レベルを選択", "모델 및 추론 수준 선택", "Выберите модель и уровень рассуждений"), KeyModelPickerProviderHint: rootSurface("Provider: %s. Press Enter to select reasoning effort, or Esc to go back.", "提供商：%s。按 Enter 选择推理强度，或按 Esc 返回。", "Anbieter: %s. Enter wählt den Denkaufwand, Esc geht zurück.", "プロバイダー: %s。Enterで推論レベルを選択、Escで戻ります。", "공급자: %s. Enter로 추론 수준 선택, Esc로 돌아가기.", "Провайдер: %s. Нажмите Enter для выбора уровня рассуждений или Esc для возврата."), KeyModelPickerFilter: rootSurface("Filter: %s", "筛选：%s", "Filter: %s", "フィルター: %s", "필터: %s", "Фильтр: %s"), KeyModelPickerEmpty: rootSurface("No matching models.", "没有匹配的模型。", "Keine passenden Modelle.", "一致するモデルはありません。", "일치하는 모델이 없습니다.", "Нет подходящих моделей."), KeyModelPickerDefault: rootSurface(" (default)", "（默认）", " (Standard)", "（デフォルト）", " (기본값)", " (по умолчанию)"), KeyModelPickerCount: rootSurface("  (%d/%d models)", "  （%d/%d 个模型）", "  (%d/%d Modelle)", "  （%d/%d モデル）", "  (모델 %d/%d)", "  (%d/%d моделей)"), KeyModelPickerEffortHint: rootSurface("Press Enter to select reasoning effort, or Esc to go back.", "按 Enter 选择推理强度，或按 Esc 返回。", "Enter wählt den Denkaufwand, Esc geht zurück.", "Enterで推論レベルを選択、Escで戻ります。", "Enter로 추론 수준 선택, Esc로 돌아가기.", "Нажмите Enter для выбора уровня рассуждений или Esc для возврата."), KeyModelLimitEditTitle: rootSurface("Edit model limits: %s/%s", "编辑模型限制：%s/%s", "Modellgrenzen bearbeiten: %s/%s", "モデル制限を編集: %s/%s", "모델 제한 편집: %s/%s", "Изменить лимиты модели: %s/%s"), KeyModelLimitEditCurrent: rootSurface("  Current context: %s", "  当前上下文：%s", "  Aktueller Kontext: %s", "  現在のコンテキスト: %s", "  현재 컨텍스트: %s", "  Текущий контекст: %s"), KeyModelLimitEditInput: rootSurface("→ Context window tokens: %s", "→ 上下文窗口 token 数：%s", "→ Kontextfenster-Token: %s", "→ コンテキストウィンドウ token 数: %s", "→ 컨텍스트 창 토큰: %s", "→ Токены окна контекста: %s"), KeyModelLimitEditHint: rootSurface("  Enter save · R reset to catalog default · Esc cancel", "  Enter 保存 · R 重置为 catalog 默认值 · Esc 取消", "  Enter speichert · R setzt auf Katalogstandard zurück · Esc bricht ab", "  Enterで保存 · Rでcatalog既定値に戻す · Escでキャンセル", "  Enter 저장 · R catalog 기본값으로 재설정 · Esc 취소", "  Enter сохранить · R сбросить к значению каталога · Esc отмена"), KeyReasoningPickerTitle: rootSurface("Select Reasoning Level for %s", "选择 %s 的推理等级", "Denkstufe für %s auswählen", "%s の推論レベルを選択", "%s의 추론 수준 선택", "Выберите уровень рассуждений для %s"), KeyReasoningPickerEmpty: rootSurface("No selectable reasoning efforts.", "没有可选择的推理强度。", "Keine auswählbaren Denkaufwände.", "選択可能な推論レベルはありません。", "선택 가능한 추론 수준이 없습니다.", "Нет доступных уровней рассуждений."), KeyReasoningPickerConfirm: rootSurface("Press Enter to confirm or Esc to go back.", "按 Enter 确认，或按 Esc 返回。", "Enter zum Bestätigen, Esc zum Zurückgehen.", "Enterで確認、Escで戻ります。", "Enter로 확인, Esc로 돌아가기.", "Нажмите Enter для подтверждения или Esc для возврата."),
		KeyModelLimitEditUnknown:    rootSurface("unknown", "未知", "unbekannt", "不明", "알 수 없음", "неизвестно"),
		KeyModelLimitEditOverridden: rootSurface("%s (overridden)", "%s（已覆盖）", "%s (überschrieben)", "%s（上書き済み）", "%s(재정의됨)", "%s (переопределено)"),
		KeyClipboardCopyFailed:      rootSurface("❌ Copy failed: %s", "❌ 复制失败：%s", "❌ Kopieren fehlgeschlagen: %s", "❌ コピーに失敗しました: %s", "❌ 복사 실패: %s", "❌ Не удалось скопировать: %s"), KeyClipboardCopied: rootSurface("📋 Copied! %s", "📋 已复制！%s", "📋 Kopiert! %s", "📋 コピーしました！%s", "📋 복사됨! %s", "📋 Скопировано! %s"), KeyClipboardNothing: rootSurface("📋 Nothing to copy", "📋 没有可复制的内容", "📋 Nichts zum Kopieren", "📋 コピーするものがありません", "📋 복사할 내용이 없습니다", "📋 Нечего копировать"), KeyImageUnsupported: rootSurface("📷 Current model does not support image input", "📷 当前模型不支持图片输入", "📷 Aktuelles Modell unterstützt keine Bildeingabe", "📷 現在のモデルは画像入力に対応していません", "📷 현재 모델은 이미지 입력을 지원하지 않습니다", "📷 Текущая модель не поддерживает ввод изображений"), KeyImageCheckingClipboard: rootSurface("📋 Checking clipboard for image…", "📋 正在检查剪贴板中的图片…", "📋 Zwischenablage wird auf Bild geprüft…", "📋 クリップボード内の画像を確認中…", "📋 클립보드에서 이미지 확인 중…", "📋 Проверяем буфер обмена на изображение…"), KeyImageClipboardError: rootSurface("❌ Clipboard error: %s", "❌ 剪贴板错误：%s", "❌ Zwischenablagefehler: %s", "❌ クリップボードエラー: %s", "❌ 클립보드 오류: %s", "❌ Ошибка буфера обмена: %s"), KeyImageClipboardEmpty: rootSurface("📋 No image in clipboard", "📋 剪贴板中没有图片", "📋 Kein Bild in der Zwischenablage", "📋 クリップボードに画像がありません", "📋 클립ボード에 이미지가 없습니다", "📋 В буфере обмена нет изображения"), KeyImagePasted: rootSurface("📷 Image #%d pasted (%s, %s) — press Enter to send", "📷 已粘贴图片 #%d（%s，%s）— 按 Enter 发送", "📷 Bild #%d eingefügt (%s, %s) — Enter zum Senden", "📷 画像 #%d を貼り付けました（%s、%s）— Enterで送信", "📷 이미지 #%d 붙여넣음 (%s, %s) — Enter로 전송", "📷 Изображение #%d вставлено (%s, %s) — нажмите Enter для отправки"),
	} {
		semanticTranslations[key] = t
	}
	semanticTranslations[KeyAssistantWorkedFor] = rootSurface(
		"Worked for %s",
		"工作耗时 %s",
		"Arbeitszeit: %s",
		"作業時間 %s",
		"작업 시간 %s",
		"Время работы: %s",
	)
	semanticTranslations[KeyImageOpenFailed] = rootSurface(
		"Could not open image: %s",
		"无法打开图片：%s",
		"Bild konnte nicht geöffnet werden: %s",
		"画像を開けませんでした: %s",
		"이미지를 열 수 없습니다: %s",
		"Не удалось открыть изображение: %s",
	)
	semanticTranslations[KeyTranscriptSelectionHintOption] = rootSurface(
		"Tip: hold Option (Alt) and drag to select text",
		"提示：按住 Option（Alt）并拖动以选择文字",
		"Tipp: Option (Alt) gedrückt halten und zum Auswählen ziehen",
		"ヒント: Option（Alt）を押しながらドラッグしてテキストを選択",
		"팁: Option(Alt)을 누른 채 드래그하여 텍스트 선택",
		"Подсказка: удерживайте Option (Alt) и перетаскивайте для выделения текста",
	)
	semanticTranslations[KeyTranscriptSelectionHintGeneric] = rootSurface(
		"Tip: hold Shift (Option/Alt in iTerm2) and drag to select text",
		"提示：按住 Shift（iTerm2 中为 Option/Alt）并拖动以选择文字",
		"Tipp: Shift gedrückt halten (in iTerm2 Option/Alt) und zum Auswählen ziehen",
		"ヒント: Shift（iTerm2 では Option/Alt）を押しながらドラッグしてテキストを選択",
		"팁: Shift(iTerm2에서는 Option/Alt)를 누른 채 드래그하여 텍스트 선택",
		"Подсказка: удерживайте Shift (Option/Alt в iTerm2) и перетаскивайте для выделения текста",
	)
}
