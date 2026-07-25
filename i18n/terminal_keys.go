package i18n

// Semantic copy used by terminal and screen-reader renderers.
const (
	KeyTerminalErrorPrefix        Key = "terminal.error_prefix"
	KeyTerminalCacheUsage         Key = "terminal.cache_usage"
	KeyTerminalProvider           Key = "terminal.provider"
	KeyTerminalModel              Key = "terminal.model"
	KeyTerminalSession            Key = "terminal.session"
	KeyTerminalTools              Key = "terminal.tools"
	KeyTerminalTaskHint           Key = "terminal.task_hint"
	KeyTerminalGoodbye            Key = "terminal.goodbye"
	KeyTerminalCostSummary        Key = "terminal.cost_summary"
	KeyTerminalContextBar         Key = "terminal.context_bar"
	KeyTerminalRunning            Key = "terminal.running"
	KeyScreenReaderStopped        Key = "screen_reader.input_stopped"
	KeyScreenReaderCommandResume  Key = "screen_reader.command_resumed"
	KeyScreenReaderCommandPause   Key = "screen_reader.command_suspended"
	KeyScreenReaderQueueFull      Key = "screen_reader.command_queue_full"
	KeyScreenReaderReservedEarly  Key = "screen_reader.command_reserved_early"
	KeyScreenReaderReservedScope  Key = "screen_reader.command_reserved_scope"
	KeyScreenReaderInfo           Key = "screen_reader.info"
	KeyScreenReaderSuccess        Key = "screen_reader.success"
	KeyScreenReaderWarning        Key = "screen_reader.warning"
	KeyScreenReaderClosed         Key = "screen_reader.closed"
	KeyScreenReaderInput          Key = "screen_reader.input"
	KeyScreenReaderBanner         Key = "screen_reader.banner"
	KeyScreenReaderTools          Key = "screen_reader.tools"
	KeyScreenReaderHelp           Key = "screen_reader.help"
	KeyScreenReaderToolStarted    Key = "screen_reader.tool_started"
	KeyScreenReaderToolInput      Key = "screen_reader.tool_input"
	KeyScreenReaderToolFinished   Key = "screen_reader.tool_finished"
	KeyScreenReaderEvidence       Key = "screen_reader.tool_evidence"
	KeyScreenReaderHookFinished   Key = "screen_reader.hook_finished"
	KeyScreenReaderHookSummary    Key = "screen_reader.hook_summary"
	KeyScreenReaderHookEvidence   Key = "screen_reader.hook_evidence"
	KeyScreenReaderRuntimeError   Key = "screen_reader.runtime_error"
	KeyScreenReaderErrorEvidence  Key = "screen_reader.runtime_error_evidence"
	KeyScreenReaderErrorMetadata  Key = "screen_reader.runtime_error_metadata"
	KeyScreenReaderUsage          Key = "screen_reader.token_usage"
	KeyScreenReaderCost           Key = "screen_reader.cost"
	KeyScreenReaderContext        Key = "screen_reader.context_usage"
	KeyScreenReaderDecision       Key = "screen_reader.decision_required"
	KeyScreenReaderExecution      Key = "screen_reader.execution_session"
	KeyScreenReaderAction         Key = "screen_reader.decision_action"
	KeyScreenReaderReviewBody     Key = "screen_reader.review_body"
	KeyScreenReaderReviewDetail   Key = "screen_reader.review_detail"
	KeyScreenReaderPostMode       Key = "screen_reader.post_mode"
	KeyScreenReaderChoice         Key = "screen_reader.choice"
	KeyScreenReaderPrompt         Key = "screen_reader.decision_prompt"
	KeyScreenReaderAskUserPrompt  Key = "screen_reader.ask_user.prompt"
	KeyScreenReaderAskUserInvalid Key = "screen_reader.ask_user.invalid"
	KeyScreenReaderInvalidChoice  Key = "screen_reader.invalid_choice"
	KeyScreenReaderAuditFailed    Key = "screen_reader.audit_failed"
	KeyScreenReaderAuditBlocked   Key = "screen_reader.audit_blocked"
	KeyScreenReaderReceipt        Key = "screen_reader.receipt"
)

func init() {
	addTerminal := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru}
	}
	addTerminal(KeyTerminalErrorPrefix, "Error: ", "错误：", "Fehler: ", "エラー: ", "오류: ", "Ошибка: ")
	addTerminal(KeyTerminalCacheUsage, "[cache: %dK read / %dK created / %dK uncached]", "[缓存：读取 %dK / 创建 %dK / 未缓存 %dK]", "[Cache: %dK gelesen / %dK erstellt / %dK nicht zwischengespeichert]", "[キャッシュ: 読み取り %dK / 作成 %dK / 未キャッシュ %dK]", "[캐시: 읽기 %dK / 생성 %dK / 미캐시 %dK]", "[кэш: прочитано %dK / создано %dK / без кэша %dK]")
	addTerminal(KeyTerminalProvider, "Provider  %s", "服务商  %s", "Anbieter  %s", "プロバイダー  %s", "제공자  %s", "Провайдер  %s")
	addTerminal(KeyTerminalModel, "Model     %s", "模型      %s", "Modell    %s", "モデル      %s", "모델      %s", "Модель     %s")
	addTerminal(KeyTerminalSession, "Session: %s", "会话：%s", "Sitzung: %s", "セッション: %s", "세션: %s", "Сеанс: %s")
	addTerminal(KeyTerminalTools, "Tools: %s", "工具：%s", "Tools: %s", "ツール: %s", "도구: %s", "Инструменты: %s")
	addTerminal(KeyTerminalTaskHint, "Type a task. Use /help for commands, or 'exit' to quit.", "输入任务。使用 /help 查看命令，或输入“exit”退出。", "Gib eine Aufgabe ein. Nutze /help für Befehle oder 'exit' zum Beenden.", "タスクを入力してください。コマンドは /help、終了は 'exit' です。", "작업을 입력하세요. 명령은 /help, 종료는 'exit'를 사용하세요.", "Введите задачу. Используйте /help для команд или 'exit' для выхода.")
	addTerminal(KeyTerminalGoodbye, "Goodbye!", "再见！", "Auf Wiedersehen!", "さようなら！", "안녕히 가세요!", "До свидания!")
	addTerminal(KeyTerminalCostSummary, "💰 Turn: %s%.4f | Session: %s%.4f | Tokens: %s in / %s out", "💰 本轮：%s%.4f | 会话：%s%.4f | Token：输入 %s / 输出 %s", "💰 Runde: %s%.4f | Sitzung: %s%.4f | Token: %s ein / %s aus", "💰 ターン: %s%.4f | セッション: %s%.4f | トークン: 入力 %s / 出力 %s", "💰 턴: %s%.4f | 세션: %s%.4f | 토큰: 입력 %s / 출력 %s", "💰 Ход: %s%.4f | Сеанс: %s%.4f | Токены: %s вход / %s выход")
	addTerminal(KeyTerminalContextBar, "[Context: %s %.0f%% (%s/%s)]", "[上下文：%s %.0f%%（%s/%s）]", "[Kontext: %s %.0f%% (%s/%s)]", "[コンテキスト: %s %.0f%%（%s/%s）]", "[컨텍스트: %s %.0f%% (%s/%s)]", "[Контекст: %s %.0f%% (%s/%s)]")
	addTerminal(KeyTerminalRunning, "⚡ Running %s...", "⚡ 正在运行 %s...", "⚡ %s wird ausgeführt...", "⚡ %s を実行中...", "⚡ %s 실행 중...", "⚡ Выполняется %s...")
	addTerminal(KeyScreenReaderStopped, "screen reader input did not stop", "屏幕阅读器输入未停止", "Screenreader-Eingabe wurde nicht beendet", "スクリーンリーダー入力が停止しませんでした", "스크린 리더 입력이 중지되지 않았습니다", "Ввод программы чтения с экрана не остановился")
	addTerminal(KeyScreenReaderCommandResume, "\nDecision input completed. Command input resumed.\n", "\n决策输入已完成。命令输入已恢复。\n", "\nEntscheidungseingabe abgeschlossen. Befehlseingabe fortgesetzt.\n", "\n決定入力が完了しました。コマンド入力を再開します。\n", "\n결정 입력이 완료되었습니다. 명령 입력을 재개합니다.\n", "\nВвод решения завершён. Ввод команд возобновлён.\n")
	addTerminal(KeyScreenReaderCommandPause, "\nCommand input suspended while a decision requires attention.\n", "\n决策需要处理时，命令输入已暂停。\n", "\nDie Befehlseingabe ist pausiert, solange eine Entscheidung Aufmerksamkeit erfordert.\n", "\n決定への対応中はコマンド入力を一時停止します。\n", "\n결정에 주의가 필요한 동안 명령 입력이 일시 중지되었습니다.\n", "\nВвод команд приостановлен, пока решение требует внимания.\n")
	addTerminal(KeyScreenReaderQueueFull, "Warning: queued command input limit reached; excess pre-decision input was discarded.\n", "警告：命令输入队列已达上限；决策前的多余输入已丢弃。\n", "Warnung: Das Limit für wartende Befehlseingaben ist erreicht; überschüssige Eingaben vor der Entscheidung wurden verworfen.\n", "警告: 保留できるコマンド入力の上限に達したため、決定前の余分な入力を破棄しました。\n", "경고: 대기 중인 명령 입력 한도에 도달하여 결정 전의 초과 입력을 버렸습니다.\n", "Предупреждение: достигнут лимит очереди команд; лишний ввод до решения отброшен.\n")
	addTerminal(KeyScreenReaderReservedEarly, "Input was reserved as a command because it arrived before decision %s became active.\n", "该输入在决策 %s 激活前到达，已保留为命令。\n", "Die Eingabe wurde als Befehl reserviert, weil sie vor Aktivierung der Entscheidung %s einging.\n", "決定 %s が有効になる前に届いたため、入力はコマンドとして保留されました。\n", "결정 %s가 활성화되기 전에 도착한 입력은 명령으로 보류되었습니다.\n", "Ввод зарезервирован как команда, так как поступил до активации решения %s.\n")
	addTerminal(KeyScreenReaderReservedScope, "Input was reserved as a command because it did not identify decision %s. Enter decision %s followed by a choice number or name.\n", "该输入未指定决策 %s，已保留为命令。请输入 decision %s，后跟选项编号或名称。\n", "Die Eingabe wurde als Befehl reserviert, weil sie die Entscheidung %s nicht identifizierte. Gib decision %s gefolgt von einer Auswahlnummer oder einem Namen ein.\n", "入力が決定 %s を特定しなかったため、コマンドとして保留されました。decision %s の後に選択番号または名前を入力してください。\n", "입력이 결정 %s를 식별하지 않아 명령으로 보류되었습니다. decision %s 뒤에 선택 번호 또는 이름을 입력하세요.\n", "Ввод зарезервирован как команда, так как не указал решение %s. Введите decision %s, затем номер или имя выбора.\n")
	addTerminal(KeyScreenReaderInfo, "Info: %s\n", "信息：%s\n", "Info: %s\n", "情報: %s\n", "정보: %s\n", "Сведения: %s\n")
	addTerminal(KeyScreenReaderSuccess, "Success: %s\n", "成功：%s\n", "Erfolg: %s\n", "成功: %s\n", "성공: %s\n", "Успешно: %s\n")
	addTerminal(KeyScreenReaderWarning, "Warning: %s\n", "警告：%s\n", "Warnung: %s\n", "警告: %s\n", "경고: %s\n", "Предупреждение: %s\n")
	addTerminal(KeyScreenReaderClosed, "Session closed.", "会话已关闭。", "Sitzung geschlossen.", "セッションを閉じました。", "세션이 종료되었습니다.", "Сеанс закрыт.")
	addTerminal(KeyScreenReaderInput, "Input: ", "输入：", "Eingabe: ", "入力: ", "입력: ", "Ввод: ")
	addTerminal(KeyScreenReaderBanner, "LUBAN Code screen reader mode. Provider: %s. Model: %s.\n", "LUBAN Code 屏幕阅读器模式。服务商：%s。模型：%s。\n", "LUBAN Code Screenreader-Modus. Anbieter: %s. Modell: %s.\n", "LUBAN Code スクリーンリーダーモード。プロバイダー: %s。モデル: %s。\n", "LUBAN Code 스크린 리더 모드. 제공자: %s. 모델: %s.\n", "Режим чтения с экрана LUBAN Code. Провайдер: %s. Модель: %s.\n")
	addTerminal(KeyScreenReaderTools, "Available tools: %s.\n", "可用工具：%s。\n", "Verfügbare Tools: %s.\n", "利用可能なツール: %s。\n", "사용 가능한 도구: %s.\n", "Доступные инструменты: %s.\n")
	addTerminal(KeyScreenReaderHelp, "Help is available with /help. Exit with /exit.\n", "使用 /help 查看帮助，使用 /exit 退出。\n", "Hilfe mit /help. Beenden mit /exit.\n", "ヘルプは /help、終了は /exit です。\n", "도움말은 /help, 종료는 /exit를 사용하세요.\n", "Справка доступна через /help. Выход через /exit.\n")
	addTerminal(KeyScreenReaderToolStarted, "Tool started: %s. Tool use ID: %s. Session: %s. Project root: %s. Turn: %s. Work unit: %s. Actor: %s. Actor type: %s.\n", "工具已启动：%s。工具调用 ID：%s。会话：%s。项目根目录：%s。轮次：%s。工作单元：%s。执行者：%s。执行者类型：%s。\n", "Tool gestartet: %s. Tool-Nutzungs-ID: %s. Sitzung: %s. Projektwurzel: %s. Runde: %s. Arbeitseinheit: %s. Akteur: %s. Akteurstyp: %s.\n", "ツール開始: %s。ツール使用 ID: %s。セッション: %s。プロジェクトルート: %s。ターン: %s。作業単位: %s。実行者: %s。実行者タイプ: %s。\n", "도구 시작: %s. 도구 사용 ID: %s. 세션: %s. 프로젝트 루트: %s. 턴: %s. 작업 단위: %s. 실행자: %s. 실행자 유형: %s.\n", "Инструмент запущен: %s. ID вызова: %s. Сеанс: %s. Корень проекта: %s. Ход: %s. Рабочая единица: %s. Исполнитель: %s. Тип исполнителя: %s.\n")
	addTerminal(KeyScreenReaderToolInput, "Tool input: %s\n", "工具输入：%s\n", "Tooleingabe: %s\n", "ツール入力: %s\n", "도구 입력: %s\n", "Вход инструмента: %s\n")
	addTerminal(KeyScreenReaderToolFinished, "Tool finished. Tool use ID: %s. Outcome: %s. Session: %s. Project root: %s. Turn: %s. Work unit: %s. Actor: %s. Actor type: %s.\n", "工具已完成。工具调用 ID：%s。结果：%s。会话：%s。项目根目录：%s。轮次：%s。工作单元：%s。执行者：%s。执行者类型：%s。\n", "Tool beendet. Tool-Nutzungs-ID: %s. Ergebnis: %s. Sitzung: %s. Projektwurzel: %s. Runde: %s. Arbeitseinheit: %s. Akteur: %s. Akteurstyp: %s.\n", "ツール完了。ツール使用 ID: %s。結果: %s。セッション: %s。プロジェクトルート: %s。ターン: %s。作業単位: %s。実行者: %s。実行者タイプ: %s。\n", "도구 완료. 도구 사용 ID: %s. 결과: %s. 세션: %s. 프로젝트 루트: %s. 턴: %s. 작업 단위: %s. 실행자: %s. 실행자 유형: %s.\n", "Инструмент завершён. ID вызова: %s. Результат: %s. Сеанс: %s. Корень проекта: %s. Ход: %s. Рабочая единица: %s. Исполнитель: %s. Тип исполнителя: %s.\n")
	addTerminal(KeyScreenReaderEvidence, "Tool evidence begins.\n%s\nTool evidence ends.\n", "工具证据开始。\n%s\n工具证据结束。\n", "Tool-Belege beginnen.\n%s\nTool-Belege enden.\n", "ツールの証拠開始。\n%s\nツールの証拠終了。\n", "도구 증거 시작.\n%s\n도구 증거 끝.\n", "Начало сведений инструмента.\n%s\nКонец сведений инструмента.\n")
	addTerminal(KeyScreenReaderHookFinished, "Hook finished: %s. Execution ID: %s. Source tool use ID: %s. Status: %s. Session: %s. Project root: %s. Turn: %s. Work unit: %s. Actor: %s. Actor type: %s.\n", "钩子已完成：%s。执行 ID：%s。源工具调用 ID：%s。状态：%s。会话：%s。项目根目录：%s。轮次：%s。工作单元：%s。执行者：%s。执行者类型：%s。\n", "Hook beendet: %s. Ausführungs-ID: %s. Quell-Tool-Nutzungs-ID: %s. Status: %s. Sitzung: %s. Projektwurzel: %s. Runde: %s. Arbeitseinheit: %s. Akteur: %s. Akteurstyp: %s.\n", "フック完了: %s。実行 ID: %s。元ツール使用 ID: %s。状態: %s。セッション: %s。プロジェクトルート: %s。ターン: %s。作業単位: %s。実行者: %s。実行者タイプ: %s。\n", "훅 완료: %s. 실행 ID: %s. 원본 도구 사용 ID: %s. 상태: %s. 세션: %s. 프로젝트 루트: %s. 턴: %s. 작업 단위: %s. 실행자: %s. 실행자 유형: %s.\n", "Хук завершён: %s. ID выполнения: %s. ID исходного вызова: %s. Статус: %s. Сеанс: %s. Корень проекта: %s. Ход: %s. Рабочая единица: %s. Исполнитель: %s. Тип исполнителя: %s.\n")
	addTerminal(KeyScreenReaderHookSummary, "Hook summary: %s\n", "Hook 摘要：%s\n", "Hook-Zusammenfassung: %s\n", "フック概要: %s\n", "훅 요약: %s\n", "Сводка хука: %s\n")
	addTerminal(KeyScreenReaderHookEvidence, "Hook evidence: %s\n", "钩子证据：%s\n", "Hook-Belege: %s\n", "フックの証拠: %s\n", "훅 증거: %s\n", "Сведения хука: %s\n")
	addTerminal(KeyScreenReaderRuntimeError, "Runtime error. %s\n", "运行时错误。%s\n", "Laufzeitfehler. %s\n", "実行時エラー。%s\n", "런타임 오류. %s\n", "Ошибка выполнения. %s\n")
	addTerminal(KeyScreenReaderErrorEvidence, "Runtime error evidence: %s\n", "运行时错误证据：%s\n", "Laufzeitfehler-Beleg: %s\n", "実行時エラーの証拠: %s\n", "런타임 오류 증거: %s\n", "Сведения об ошибке выполнения: %s\n")
	addTerminal(KeyScreenReaderErrorMetadata, "Runtime error metadata: %s\n", "运行时错误元数据：%s\n", "Laufzeitfehler-Metadaten: %s\n", "実行時エラーのメタデータ: %s\n", "런타임 오류 메타데이터: %s\n", "Метаданные ошибки выполнения: %s\n")
	addTerminal(KeyScreenReaderUsage, "Token usage: %d input, %d output, %d cache read, %d cache created.\n", "Token 用量：输入 %d，输出 %d，缓存读取 %d，缓存创建 %d。\n", "Token-Nutzung: %d Eingabe, %d Ausgabe, %d Cache gelesen, %d Cache erstellt.\n", "トークン使用量: 入力 %d、出力 %d、キャッシュ読み取り %d、キャッシュ作成 %d。\n", "토큰 사용량: 입력 %d, 출력 %d, 캐시 읽기 %d, 캐시 생성 %d.\n", "Использование токенов: вход %d, выход %d, кэш прочитан %d, кэш создан %d.\n")
	addTerminal(KeyScreenReaderCost, "Cost: turn %.4f USD, session %.4f USD. Tokens: %d input, %d output.\n", "费用：本轮 %.4f USD，会话 %.4f USD。Token：输入 %d，输出 %d。\n", "Kosten: Runde %.4f USD, Sitzung %.4f USD. Token: %d Eingabe, %d Ausgabe.\n", "料金: ターン %.4f USD、セッション %.4f USD。トークン: 入力 %d、出力 %d。\n", "비용: 턴 %.4f USD, 세션 %.4f USD. 토큰: 입력 %d, 출력 %d.\n", "Стоимость: ход %.4f USD, сеанс %.4f USD. Токены: вход %d, выход %d.\n")
	addTerminal(KeyScreenReaderContext, "Context usage: %d of %d tokens, %d percent.\n", "上下文用量：已使用 %d / 共 %d 个 Token，%d%%。\n", "Kontextnutzung: %d von %d Token, %d Prozent.\n", "コンテキスト使用量: %d 中 %d トークン、%d パーセント。\n", "컨텍스트 사용량: %d개 중 %d개 토큰, %d퍼센트.\n", "Использование контекста: %d из %d токенов, %d процентов.\n")
	addTerminal(KeyScreenReaderDecision, "Decision required. ID: %s. Kind: %s. Actor: %s. Actor type: %s. Work unit: %s.\n", "需要决策。ID：%s。类型：%s。执行者：%s。执行者类型：%s。工作单元：%s。\n", "Entscheidung erforderlich. ID: %s. Art: %s. Akteur: %s. Akteurstyp: %s. Arbeitseinheit: %s.\n", "決定が必要です。ID: %s。種類: %s。実行者: %s。実行者タイプ: %s。作業単位: %s。\n", "결정이 필요합니다. ID: %s. 종류: %s. 실행자: %s. 실행자 유형: %s. 작업 단위: %s.\n", "Требуется решение. ID: %s. Вид: %s. Исполнитель: %s. Тип исполнителя: %s. Рабочая единица: %s.\n")
	addTerminal(KeyScreenReaderExecution, "Execution session: %s.\n", "执行会话：%s。\n", "Ausführungssitzung: %s.\n", "実行セッション: %s。\n", "실행 세션: %s.\n", "Сеанс выполнения: %s.\n")
	addTerminal(KeyScreenReaderAction, "Action: %s. Target: %s. Impact: %s. Risk level: %d. Risk reason: %s. Rule source: %s. Approval scope: %s.\n", "操作：%s。目标：%s。影响：%s。风险等级：%d。风险原因：%s。规则来源：%s。批准范围：%s。\n", "Aktion: %s. Ziel: %s. Auswirkung: %s. Risikostufe: %d. Risikogrund: %s. Regelquelle: %s. Genehmigungsumfang: %s.\n", "操作: %s。対象: %s。影響: %s。リスクレベル: %d。理由: %s。ルールの出所: %s。承認範囲: %s。\n", "작업: %s. 대상: %s. 영향: %s. 위험 수준: %d. 위험 사유: %s. 규칙 출처: %s. 승인 범위: %s.\n", "Действие: %s. Цель: %s. Воздействие: %s. Уровень риска: %d. Причина риска: %s. Источник правила: %s. Область одобрения: %s.\n")
	addTerminal(KeyScreenReaderReviewBody, "Review body begins.\n%s\nReview body ends.\n", "审查正文开始。\n%s\n审查正文结束。\n", "Prüfungstext beginnt.\n%s\nPrüfungstext endet.\n", "レビュー本文の開始。\n%s\nレビュー本文の終了。\n", "검토 본문 시작.\n%s\n검토 본문 끝.\n", "Начало текста проверки.\n%s\nКонец текста проверки.\n")
	addTerminal(KeyScreenReaderReviewDetail, "Review detail %d: %s\n", "审查详情 %d：%s\n", "Prüfdetail %d: %s\n", "レビュー詳細 %d: %s\n", "검토 세부정보 %d: %s\n", "Деталь проверки %d: %s\n")
	addTerminal(KeyScreenReaderPostMode, "After approval, permission mode will be %s.\n", "批准后，权限模式将变为 %s。\n", "Nach der Genehmigung wird der Berechtigungsmodus %s sein.\n", "承認後、権限モードは %s になります。\n", "승인 후 권한 모드는 %s가 됩니다.\n", "После одобрения режим разрешений будет %s.\n")
	addTerminal(KeyScreenReaderChoice, "Choice %d: %s.\n", "选项 %d：%s。\n", "Auswahl %d: %s.\n", "選択肢 %d: %s。\n", "선택 %d: %s.\n", "Выбор %d: %s.\n")
	addTerminal(KeyScreenReaderPrompt, "Decision choice: type decision %s followed by a choice number or name: ", "决策选项：请输入 decision %s，后跟选项编号或名称：", "Entscheidungsauswahl: Gib decision %s gefolgt von einer Auswahlnummer oder einem Namen ein: ", "決定の選択: decision %s の後に選択番号または名前を入力してください: ", "결정 선택: decision %s 뒤에 선택 번호 또는 이름을 입력하세요: ", "Выбор решения: введите decision %s, затем номер или имя выбора: ")
	addTerminal(KeyScreenReaderAskUserPrompt, "Answer: type decision %s followed by an option number, exact label, or Other:<text>: ", "回答：请输入 decision %s，后跟选项编号、准确标签或 Other:<文本>：", "Antwort: Gib decision %s gefolgt von einer Optionsnummer, der genauen Bezeichnung oder Other:<Text> ein: ", "回答: decision %s の後に選択番号、正確なラベル、または Other:<テキスト> を入力してください: ", "답변: decision %s 뒤에 옵션 번호, 정확한 레이블 또는 Other:<텍스트>를 입력하세요: ", "Ответ: введите decision %s, затем номер варианта, точную метку или Other:<текст>: ")
	addTerminal(KeyScreenReaderAskUserInvalid, "Invalid answer. Enter decision %s followed by valid option values or escape.\n%s", "答案无效。请输入 decision %s，后跟有效选项值或 escape。\n%s", "Ungültige Antwort. Gib decision %s gefolgt von gültigen Optionswerten oder escape ein.\n%s", "無効な回答です。decision %s の後に有効な選択値または escape を入力してください。\n%s", "잘못된 답변입니다. decision %s 뒤에 유효한 옵션 값 또는 escape를 입력하세요.\n%s", "Недопустимый ответ. Введите decision %s, затем допустимые варианты или escape.\n%s")
	addTerminal(KeyScreenReaderInvalidChoice, "Invalid decision choice. Enter decision %s followed by a choice number, exact choice name, y, n, a, or escape.\n%s", "无效的决策选项。请输入 decision %s，后跟选项编号、准确名称、y、n、a 或 escape。\n%s", "Ungültige Entscheidungsauswahl. Gib decision %s gefolgt von Auswahlnummer, genauem Namen, y, n, a oder escape ein.\n%s", "無効な決定選択です。decision %s の後に選択番号、正確な名前、y、n、a、または escape を入力してください。\n%s", "잘못된 결정 선택입니다. decision %s 뒤에 선택 번호, 정확한 이름, y, n, a 또는 escape를 입력하세요.\n%s", "Недопустимый выбор решения. Введите decision %s, затем номер, точное имя, y, n, a или escape.\n%s")
	addTerminal(KeyScreenReaderAuditFailed, "Warning: decision audit persistence failed: %s.\n", "警告：决策审计记录失败：%s。\n", "Warnung: Das Speichern des Entscheidungs-Audits ist fehlgeschlagen: %s.\n", "警告: 決定監査の保存に失敗しました: %s。\n", "경고: 결정 감사 저장에 실패했습니다: %s.\n", "Предупреждение: не удалось сохранить аудит решения: %s.\n")
	addTerminal(KeyScreenReaderAuditBlocked, "Decision approval blocked because its audit record was not durably stored.\n", "由于审计记录未被持久保存，决策批准已被阻止。\n", "Die Genehmigung wurde blockiert, weil ihr Audit-Eintrag nicht dauerhaft gespeichert wurde.\n", "監査記録が永続的に保存されなかったため、決定の承認をブロックしました。\n", "감사 기록이 영구 저장되지 않아 결정 승인이 차단되었습니다.\n", "Одобрение решения заблокировано, так как запись аудита не была надёжно сохранена.\n")
	addTerminal(KeyScreenReaderReceipt, "Decision receipt. ID: %s. Outcome: %s. Choice: %s.\n", "决策回执。ID：%s。结果：%s。选项：%s。\n", "Entscheidungsbeleg. ID: %s. Ergebnis: %s. Auswahl: %s.\n", "決定の受領。ID: %s。結果: %s。選択: %s。\n", "결정 영수증. ID: %s. 결과: %s. 선택: %s.\n", "Квитанция решения. ID: %s. Результат: %s. Выбор: %s.\n")
}
