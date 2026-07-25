package i18n

// Semantic copy used by the core slash-command status and goal surfaces.
// Command names, model/provider identifiers, paths, and raw errors are
// deliberately passed as format values rather than translated.
const (
	KeyCommandStatusDescription Key = "command.status.description"
	KeyCommandStatusUnknown     Key = "command.status.unknown"
	KeyCommandStatusNone        Key = "command.status.none"
	KeyCommandStatusReport      Key = "command.status.report"
	KeyCommandStatusAPIUsage    Key = "command.status.api_usage"
	KeyCommandStatusWebSearches Key = "command.status.web_searches"
	KeyCommandStatusCost        Key = "command.status.cost"
	KeyCommandStatusCostUnknown Key = "command.status.cost_unknown"

	KeyCommandContextDescription         Key = "command.context.description"
	KeyCommandContextUnknown             Key = "command.context.unknown"
	KeyCommandContextLoopTracker         Key = "command.context.source.loop_tracker"
	KeyCommandContextReport              Key = "command.context.report"
	KeyCommandContextUsageExact          Key = "command.context.usage.exact"
	KeyCommandContextUsageEstimated      Key = "command.context.usage.estimated"
	KeyCommandContextUsageLowerBound     Key = "command.context.usage.lower_bound"
	KeyCommandContextSourceProvider      Key = "command.context.source.provider"
	KeyCommandContextSourceEstimate      Key = "command.context.source.estimate"
	KeyCommandContextSourceLowerBound    Key = "command.context.source.lower_bound"
	KeyCommandContextRemaining           Key = "command.context.remaining"
	KeyCommandContextRemainingEstimated  Key = "command.context.remaining.estimated"
	KeyCommandContextRemainingUpperBound Key = "command.context.remaining.upper_bound"
	KeyCommandContextAutoCompact         Key = "command.context.state.auto_compact"
	KeyCommandContextBlocking            Key = "command.context.state.blocking"
	KeyCommandContextCritical            Key = "command.context.state.critical"
	KeyCommandContextLow                 Key = "command.context.state.low"
	KeyCommandContextUnavailable         Key = "command.context.unavailable"
	KeyCommandContextSource              Key = "command.context.source"

	KeyCommandGoalDescription    Key = "command.goal.description"
	KeyCommandGoalRuntimeMissing Key = "command.goal.runtime_missing"
	KeyCommandGoalUpdated        Key = "command.goal.updated"
	KeyCommandGoalPaused         Key = "command.goal.paused"
	KeyCommandGoalActive         Key = "command.goal.active"
	KeyCommandGoalSet            Key = "command.goal.set"
	KeyCommandGoalNoActive       Key = "command.goal.no_active"
	KeyCommandGoalCleared        Key = "command.goal.cleared"
	KeyCommandGoalNoActiveCreate Key = "command.goal.no_active_create"
	KeyCommandGoalReport         Key = "command.goal.report"
	KeyCommandGoalBudget         Key = "command.goal.budget"
	KeyCommandGoalUsage          Key = "command.goal.usage"
	KeyCommandGoalTurns          Key = "command.goal.turns"
	KeyCommandGoalLastEvaluation Key = "command.goal.last_evaluation"
	KeyCommandGoalSaveError      Key = "command.goal.save_error"
	KeyCommandGoalLoadError      Key = "command.goal.load_error"
	KeyCommandGoalUsageError     Key = "command.goal.usage_error"
)

func init() {
	semanticTranslations[KeyCommandStatusDescription] = commandCore("Show session identity, unique input, API processed usage, and cost", "显示会话标识、唯一输入、API 已处理用量和费用", "Sitzungskennung, eindeutige Eingaben, API-Nutzung und Kosten anzeigen", "セッション識別子、一意の入力、API処理使用量、コストを表示", "세션 식별자, 고유 입력, API 처리 사용량 및 비용을 표시", "Показать идентификатор сеанса, уникальный ввод, обработанное API использование и стоимость")
	semanticTranslations[KeyCommandStatusUnknown] = commandCore("(unknown)", "（未知）", "(unbekannt)", "（不明）", "(알 수 없음)", "(неизвестно)")
	semanticTranslations[KeyCommandStatusNone] = commandCore("(none)", "（无）", "(keine)", "（なし）", "(없음)", "(нет)")
	semanticTranslations[KeyCommandStatusReport] = commandCore("Session Status\n──────────────────────────────────────────\n  Model:           %s\n  Session ID:      %s\n  Working dir:     %s\n  Messages:        %d\n  New input≈:      %s\n", "会话状态\n──────────────────────────────────────────\n  模型：           %s\n  会话 ID：        %s\n  工作目录：       %s\n  消息数：         %d\n  新输入≈：        %s\n", "Sitzungsstatus\n──────────────────────────────────────────\n  Modell:          %s\n  Sitzungs-ID:     %s\n  Arbeitsordner:   %s\n  Nachrichten:     %d\n  Neue Eingabe≈:   %s\n", "セッションの状態\n──────────────────────────────────────────\n  モデル:          %s\n  セッション ID:   %s\n  作業ディレクトリ: %s\n  メッセージ:      %d\n  新規入力≈:       %s\n", "세션 상태\n──────────────────────────────────────────\n  모델:            %s\n  세션 ID:         %s\n  작업 디렉터리:   %s\n  메시지:          %d\n  새 입력≈:        %s\n", "Статус сеанса\n──────────────────────────────────────────\n  Модель:          %s\n  ID сеанса:       %s\n  Рабочая папка:   %s\n  Сообщения:       %d\n  Новый ввод≈:     %s\n")
	semanticTranslations[KeyCommandStatusAPIUsage] = commandCore("\nAPI processed usage:\n  Input tokens:    %s\n  Output tokens:   %s\n  Cache read:      %s\n  Cache created:   %s\n", "\nAPI 已处理用量：\n  输入 token：     %s\n  输出 token：     %s\n  缓存读取：       %s\n  缓存创建：       %s\n", "\nVon der API verarbeitete Nutzung:\n  Eingabetoken:    %s\n  Ausgabetoken:    %s\n  Cache gelesen:   %s\n  Cache erstellt:  %s\n", "\nAPI 処理使用量:\n  入力トークン:    %s\n  出力トークン:    %s\n  キャッシュ読取:  %s\n  キャッシュ作成:  %s\n", "\nAPI 처리 사용량:\n  입력 토큰:       %s\n  출력 토큰:       %s\n  캐시 읽기:       %s\n  캐시 생성:       %s\n", "\nИспользование, обработанное API:\n  Входные токены:  %s\n  Выходные токены: %s\n  Чтение кэша:     %s\n  Создано кэша:    %s\n")
	semanticTranslations[KeyCommandStatusWebSearches] = commandCore("  Web searches:    %s\n", "  Web 搜索：       %s\n", "  Websuchen:       %s\n", "  Web検索:         %s\n", "  웹 검색:         %s\n", "  Веб-поиски:      %s\n")
	semanticTranslations[KeyCommandStatusCost] = commandCore("  Session cost:    %s%.4f\n", "  会话费用：       %s%.4f\n", "  Sitzungskosten:  %s%.4f\n", "  セッション費用:  %s%.4f\n", "  세션 비용:       %s%.4f\n", "  Стоимость сеанса: %s%.4f\n")
	semanticTranslations[KeyCommandStatusCostUnknown] = commandCore("  Session cost:    (unknown model pricing)\n", "  会话费用：       （未知模型定价）\n", "  Sitzungskosten:  (unbekannte Modellpreise)\n", "  セッション費用:  （モデル価格不明）\n", "  세션 비용:       (알 수 없는 모델 가격)\n", "  Стоимость сеанса: (неизвестная цена модели)\n")

	semanticTranslations[KeyCommandContextDescription] = commandCore("Show current model context occupancy", "显示当前模型上下文占用", "Aktuelle Modellkontextbelegung anzeigen", "現在のモデルコンテキスト使用量を表示", "현재 모델 컨텍스트 사용량 표시", "Показать текущее использование контекста модели")
	semanticTranslations[KeyCommandContextUnknown] = semanticTranslations[KeyCommandStatusUnknown]
	semanticTranslations[KeyCommandContextLoopTracker] = commandCore("query loop context tracker", "查询循环上下文跟踪器", "Kontext-Tracker der Abfrageschleife", "クエリループのコンテキスト追跡", "쿼리 루프 컨텍스트 추적기", "Трекер контекста цикла запросов")
	semanticTranslations[KeyCommandContextSourceProvider] = commandCore("provider usage", "Provider 精确用量", "Anbieternutzung", "プロバイダー使用量", "Provider 사용량", "Данные провайдера")
	semanticTranslations[KeyCommandContextSourceEstimate] = commandCore("complete local estimate", "完整本地估算", "vollständige lokale Schätzung", "完全なローカル推定", "완전한 로컬 추정치", "Полная локальная оценка")
	semanticTranslations[KeyCommandContextSourceLowerBound] = commandCore("incomplete local estimate (lower bound)", "不完整本地估算（下界）", "unvollständige lokale Schätzung (Untergrenze)", "不完全なローカル推定（下限）", "불완전한 로컬 추정치(하한)", "Неполная локальная оценка (нижняя граница)")
	semanticTranslations[KeyCommandContextReport] = commandCore("Current Context Usage\n──────────────────────────────────────────\n  Model:                  %s\n  Conversation messages:  %d\n", "当前上下文使用量\n──────────────────────────────────────────\n  模型：                   %s\n  对话消息：               %d\n", "Aktuelle Kontextnutzung\n──────────────────────────────────────────\n  Modell:                  %s\n  Gesprächsnachrichten:    %d\n", "現在のコンテキスト使用量\n──────────────────────────────────────────\n  モデル:                  %s\n  会話メッセージ:          %d\n", "현재 컨텍스트 사용량\n──────────────────────────────────────────\n  모델:                    %s\n  대화 메시지:             %d\n", "Текущее использование контекста\n──────────────────────────────────────────\n  Модель:                  %s\n  Сообщения диалога:       %d\n")
	semanticTranslations[KeyCommandContextUsageExact] = commandCore("  Context:                 %s / %s tokens (%.1f%% used)\n", "  上下文：                  %s / %s 个 token（已用 %.1f%%）\n", "  Kontext:                 %s / %s Token (%.1f%% genutzt)\n", "  コンテキスト:             %s / %s トークン（%.1f%% 使用）\n", "  컨텍스트:                %s / %s 토큰 (%.1f%% 사용)\n", "  Контекст:                %s / %s токенов (использовано %.1f%%)\n")
	semanticTranslations[KeyCommandContextUsageEstimated] = commandCore("  Context:                 ≈%s / %s tokens (≈%.1f%% used)\n", "  上下文：                  ≈%s / %s 个 token（约已用 %.1f%%）\n", "  Kontext:                 ≈%s / %s Token (≈%.1f%% genutzt)\n", "  コンテキスト:             ≈%s / %s トークン（約 %.1f%% 使用）\n", "  컨텍스트:                ≈%s / %s 토큰 (약 %.1f%% 사용)\n", "  Контекст:                ≈%s / %s токенов (использовано ≈%.1f%%)\n")
	semanticTranslations[KeyCommandContextUsageLowerBound] = commandCore("  Context:                 ≥%s / %s tokens (≥%.1f%% used)\n", "  上下文：                  ≥%s / %s 个 token（至少已用 %.1f%%）\n", "  Kontext:                 ≥%s / %s Token (≥%.1f%% genutzt)\n", "  コンテキスト:             ≥%s / %s トークン（%.1f%% 以上使用）\n", "  컨텍스트:                ≥%s / %s 토큰 (최소 %.1f%% 사용)\n", "  Контекст:                ≥%s / %s токенов (использовано ≥%.1f%%)\n")
	semanticTranslations[KeyCommandContextRemaining] = commandCore("  Remaining:              %d%% (%s tokens)\n", "  剩余：                   %d%%（%s 个 token）\n", "  Verbleibend:             %d%% (%s Token)\n", "  残り:                    %d%%（%s トークン）\n", "  남음:                    %d%% (%s 토큰)\n", "  Осталось:                %d%% (%s токенов)\n")
	semanticTranslations[KeyCommandContextRemainingEstimated] = commandCore("  Remaining:              ≈%d%% (≈%s tokens)\n", "  剩余：                   约 %d%%（约 %s 个 token）\n", "  Verbleibend:             ≈%d%% (≈%s Token)\n", "  残り:                    約 %d%%（約 %s トークン）\n", "  남음:                    약 %d%% (약 %s 토큰)\n", "  Осталось:                ≈%d%% (≈%s токенов)\n")
	semanticTranslations[KeyCommandContextRemainingUpperBound] = commandCore("  Remaining:              ≤%d%% (≤%s tokens)\n", "  剩余：                   ≤%d%%（≤%s 个 token）\n", "  Verbleibend:             ≤%d%% (≤%s Token)\n", "  残り:                    ≤%d%%（≤%s トークン）\n", "  남음:                    ≤%d%% (≤%s 토큰)\n", "  Осталось:                ≤%d%% (≤%s токенов)\n")
	semanticTranslations[KeyCommandContextAutoCompact] = commandCore("  State:                  auto-compact threshold reached\n", "  状态：                   已达到自动压缩阈值\n", "  Status:                  Schwelle für automatische Komprimierung erreicht\n", "  状態:                    自動圧縮のしきい値に到達\n", "  상태:                    자동 압축 임계값에 도달\n", "  Состояние:               достигнут порог автосжатия\n")
	semanticTranslations[KeyCommandContextBlocking] = commandCore("  State:                  blocking limit reached; run /compact\n", "  状态：                   已达到阻止上限；请运行 /compact\n", "  Status:                  Sperrlimit erreicht; /compact ausführen\n", "  状態:                    制限に到達しました。/compact を実行してください\n", "  상태:                    차단 한도에 도달했습니다. /compact를 실행하세요\n", "  Состояние:               достигнут блокирующий предел; выполните /compact\n")
	semanticTranslations[KeyCommandContextCritical] = commandCore("  State:                  context critically low\n", "  状态：                   上下文严重不足\n", "  Status:                  Kontext kritisch knapp\n", "  状態:                    コンテキストが極端に不足\n", "  상태:                    컨텍스트가 심각하게 부족함\n", "  Состояние:               контекст критически мал\n")
	semanticTranslations[KeyCommandContextLow] = commandCore("  State:                  context low\n", "  状态：                   上下文不足\n", "  Status:                  Kontext knapp\n", "  状態:                    コンテキストが不足\n", "  상태:                    컨텍스트가 부족함\n", "  Состояние:               мало контекста\n")
	semanticTranslations[KeyCommandContextUnavailable] = commandCore("  Context:                 unknown (no reliable usage data)\n", "  上下文：                  未知（无可靠用量数据）\n", "  Kontext:                 unbekannt (keine verlässlichen Nutzungsdaten)\n", "  コンテキスト:             不明（信頼できる使用量データなし）\n", "  컨텍스트:                알 수 없음 (신뢰할 수 있는 사용량 데이터 없음)\n", "  Контекст:                неизвестен (нет надёжных данных об использовании)\n")
	semanticTranslations[KeyCommandContextSource] = commandCore("  Source:                 %s\n", "  来源：                   %s\n", "  Quelle:                  %s\n", "  情報源:                  %s\n", "  출처:                    %s\n", "  Источник:                %s\n")

	semanticTranslations[KeyCommandGoalDescription] = commandCore("Set or manage the session goal", "设置或管理会话目标", "Sitzungsziel festlegen oder verwalten", "セッション目標を設定または管理", "세션 목표를 설정하거나 관리", "Установить или управлять целью сеанса")
	semanticTranslations[KeyCommandGoalRuntimeMissing] = commandCore("goal runtime is not configured", "目标运行时尚未配置", "Ziel-Laufzeit ist nicht konfiguriert", "目標ランタイムが設定されていません", "목표 런타임이 구성되지 않았습니다", "Среда выполнения цели не настроена")
	semanticTranslations[KeyCommandGoalUpdated] = commandCore("Goal updated", "目标已更新", "Ziel aktualisiert", "目標を更新しました", "목표를 업데이트했습니다", "Цель обновлена")
	semanticTranslations[KeyCommandGoalPaused] = commandCore("Goal paused", "目标已暂停", "Ziel pausiert", "目標を一時停止しました", "목표를 일시 중지했습니다", "Цель приостановлена")
	semanticTranslations[KeyCommandGoalActive] = commandCore("Goal active", "目标已激活", "Ziel aktiv", "目標を有効にしました", "목표가 활성화되었습니다", "Цель активна")
	semanticTranslations[KeyCommandGoalSet] = commandCore("Goal set", "目标已设置", "Ziel gesetzt", "目標を設定しました", "목표를 설정했습니다", "Цель установлена")
	semanticTranslations[KeyCommandGoalNoActive] = commandCore("No active goal is set.", "未设置活动目标。", "Kein aktives Ziel ist gesetzt.", "アクティブな目標は設定されていません。", "활성 목표가 설정되지 않았습니다.", "Активная цель не установлена.")
	semanticTranslations[KeyCommandGoalCleared] = commandCore("Goal cleared: %s", "目标已清除：%s", "Ziel gelöscht: %s", "目標を削除しました: %s", "목표를 지웠습니다: %s", "Цель очищена: %s")
	semanticTranslations[KeyCommandGoalNoActiveCreate] = commandCore("No active goal is set. Use /goal <objective> to create one.", "未设置活动目标。使用 /goal <objective> 创建目标。", "Kein aktives Ziel ist gesetzt. Mit /goal <objective> erstellst du eines.", "アクティブな目標は設定されていません。/goal <objective> で作成してください。", "활성 목표가 설정되지 않았습니다. /goal <objective>로 만드세요.", "Активная цель не установлена. Создайте её через /goal <objective>.")
	semanticTranslations[KeyCommandGoalReport] = commandCore("Goal status: %s\nObjective: %s", "目标状态：%s\n目标：%s", "Zielstatus: %s\nZiel: %s", "目標の状態: %s\n目標: %s", "목표 상태: %s\n목표: %s", "Статус цели: %s\nЦель: %s")
	semanticTranslations[KeyCommandGoalBudget] = commandCore("\nToken budget: %d", "\nToken 预算：%d", "\nTokenbudget: %d", "\nトークン予算: %d", "\n토큰 예산: %d", "\nБюджет токенов: %d")
	semanticTranslations[KeyCommandGoalUsage] = commandCore("\nUsage: %d", "\n用量：%d", "\nNutzung: %d", "\n使用量: %d", "\n사용량: %d", "\nИспользование: %d")
	semanticTranslations[KeyCommandGoalTurns] = commandCore("\nTurns: %d", "\n轮次：%d", "\nRunden: %d", "\nターン: %d", "\n턴: %d", "\nХоды: %d")
	semanticTranslations[KeyCommandGoalLastEvaluation] = commandCore("\nLast evaluation: %s", "\n上次评估：%s", "\nLetzte Bewertung: %s", "\n前回の評価: %s", "\n마지막 평가: %s", "\nПоследняя оценка: %s")
	semanticTranslations[KeyCommandGoalSaveError] = commandCore("save goal: %w", "保存目标：%w", "Ziel speichern: %w", "目標を保存: %w", "목표 저장: %w", "сохранение цели: %w")
	semanticTranslations[KeyCommandGoalLoadError] = commandCore("load goal: %w", "加载目标：%w", "Ziel laden: %w", "目標を読み込み: %w", "목표 로드: %w", "загрузка цели: %w")
	semanticTranslations[KeyCommandGoalUsageError] = commandCore("usage: /goal [status|view|set <objective> [--accept <criterion>...]|edit <objective>|criteria add <text>|criteria edit <id> <text>|criteria remove <id>|pause|resume|clear]", "用法：/goal [status|view|set <objective> [--accept <criterion>...]|edit <objective>|criteria add <text>|criteria edit <id> <text>|criteria remove <id>|pause|resume|clear]", "Verwendung: /goal [status|view|set <objective> [--accept <criterion>...]|edit <objective>|criteria add <text>|criteria edit <id> <text>|criteria remove <id>|pause|resume|clear]", "使い方: /goal [status|view|set <objective> [--accept <criterion>...]|edit <objective>|criteria add <text>|criteria edit <id> <text>|criteria remove <id>|pause|resume|clear]", "사용법: /goal [status|view|set <objective> [--accept <criterion>...]|edit <objective>|criteria add <text>|criteria edit <id> <text>|criteria remove <id>|pause|resume|clear]", "Использование: /goal [status|view|set <objective> [--accept <criterion>...]|edit <objective>|criteria add <text>|criteria edit <id> <text>|criteria remove <id>|pause|resume|clear]")
}

func commandCore(en, zh, de, ja, ko, ru string) map[Language]string {
	return map[Language]string{LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru}
}
