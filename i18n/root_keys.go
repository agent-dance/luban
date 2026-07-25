package i18n

// Semantic keys for the core TUI shell.
const (
	KeyInputPlaceholder        Key = "tui.input.placeholder"
	KeyPastedText              Key = "tui.input.pasted_text"
	KeyPromptHistoryNotSaved   Key = "tui.input.history_not_saved"
	KeyUserPrefix              Key = "tui.message.user_prefix"
	KeyImageAttachment         Key = "tui.message.image_attachment"
	KeyErrorPrefix             Key = "common.error_prefix"
	KeyModeAuto                Key = "mode.auto"
	KeyModeAsk                 Key = "mode.ask"
	KeyModePlan                Key = "mode.plan"
	KeyModeLabel               Key = "mode.label"
	KeyWebSearchCount          Key = "status.web_search_count"
	KeyShowAllEvidence         Key = "evidence.show_all"
	KeyGoalPrefix              Key = "goal.prefix"
	KeyGoalPausedPrefix        Key = "goal.paused_prefix"
	KeySlashCommandsTitle      Key = "slash_commands.title"
	KeyPermissionAllowOnce     Key = "permission.allow_once"
	KeyPermissionAlwaysAllow   Key = "permission.always_allow"
	KeyPermissionExecute       Key = "permission.execute"
	KeyPermissionStayInPlan    Key = "permission.stay_in_plan"
	KeyPermissionReject        Key = "permission.reject"
	KeyRiskMedium              Key = "risk.medium"
	KeyRiskHigh                Key = "risk.high"
	KeyPermissionDecision      Key = "permission.decision"
	KeyPlanDecision            Key = "plan.decision"
	KeyDecisionActor           Key = "decision.actor"
	KeyDecisionAgentSession    Key = "decision.agent_session"
	KeyDecisionAction          Key = "decision.action"
	KeyDecisionTarget          Key = "decision.target"
	KeyDecisionImpact          Key = "decision.impact"
	KeyDecisionRisk            Key = "decision.risk"
	KeyDecisionScope           Key = "decision.scope"
	KeyDecisionInput           Key = "decision.input"
	KeyDecisionAfterApproval   Key = "decision.after_approval"
	KeyGoodbye                 Key = "common.goodbye"
	KeyForkNoConversationTurns Key = "fork.no_conversation_turns"
)

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}
	add(KeyInputPlaceholder, "Type a message... (Ctrl+D to exit)", "输入消息... (Ctrl+D 退出)", "Nachricht eingeben... (Strg+D zum Beenden)", "メッセージを入力... (Ctrl+Dで終了)", "메시지 입력... (Ctrl+D 종료)", "Введите сообщение... (Ctrl+D для выхода)")
	add(KeyPastedText, "[Pasted text #%d +%d lines]", "[粘贴文本 #%d 共%d行]", "[Eingefügter Text #%d +%d Zeilen]", "[貼り付けたテキスト #%d +%d行]", "[붙여넣은 텍스트 #%d +%d줄]", "[Вставленный текст #%d +%d строк]")
	add(KeyPromptHistoryNotSaved, "⚠ Prompt history not saved", "⚠ 提示历史未保存", "⚠ Verlauf nicht gespeichert", "⚠ プロンプト履歴が保存されていません", "⚠ 프롬프트 기록이 저장되지 않았습니다", "⚠ История запросов не сохранена")
	add(KeyUserPrefix, "You: ", "你：", "Sie: ", "あなた：", "나: ", "Вы: ")
	add(KeyImageAttachment, "📷 [Image #%d] (%s)", "📷 [图片 #%d]（%s）", "📷 [Bild #%d] (%s)", "📷 [画像 #%d]（%s）", "📷 [이미지 #%d] (%s)", "📷 [Изображение #%d] (%s)")
	add(KeyErrorPrefix, "Error: ", "错误：", "Fehler: ", "エラー：", "오류: ", "Ошибка: ")
	add(KeyModeAuto, "Auto", "自动", "Auto", "自動", "자동", "Авто")
	add(KeyModeAsk, "Ask", "询问", "Fragen", "確認", "확인", "Спрашивать")
	add(KeyModePlan, "Plan", "计划", "Planen", "計画", "계획", "План")
	add(KeyModeLabel, "%s mode", "%s模式", "%s-Modus", "%sモード", "%s 모드", "режим %s")
	add(KeyWebSearchCount, "%d web search", "%d 次网络搜索", "%d Websuche", "%d 回ウェブ検索", "%d회 웹 검색", "%d поисков в вебе")
	add(KeyShowAllEvidence, "show all evidence", "显示所有证据", "alle Beweise anzeigen", "すべての証拠を表示", "모든 증거 표시", "показать все доказательства")
	add(KeyGoalPrefix, "Goal: ", "目标：", "Ziel: ", "目標：", "목표: ", "Цель: ")
	add(KeyGoalPausedPrefix, "Goal paused: ", "目标已暂停：", "Ziel pausiert: ", "目標一時停止：", "목표 일시 중지: ", "Цель приостановлена: ")
	add(KeySlashCommandsTitle, "Slash Commands — Up/Down move, Tab complete, Enter run, Esc close", "斜杠命令 — 上下键移动，Tab补全，回车执行，Esc关闭", "Slash-Befehle — Hoch/Runter bewegen, Tab vervollständigen, Enter ausführen, Esc schließen", "スラッシュコマンド — 上下で移動、Tabで補完、Enterで実行、Escで閉じる", "슬래시 명령 — 위/아래 이동, Tab 완성, Enter 실행, Esc 닫기", "Slash-команды — Вверх/Вниз движение, Tab завершение, Enter запуск, Esc закрыть")
	add(KeyPermissionAllowOnce, "Allow once", "允许一次", "Einmal erlauben", "一度だけ許可", "한 번 허용", "Разрешить один раз")
	add(KeyPermissionAlwaysAllow, "Always allow", "始终允许", "Immer erlauben", "常に許可", "항상 허용", "Всегда разрешать")
	add(KeyPermissionExecute, "Execute", "执行", "Ausführen", "実行", "실행", "Выполнить")
	add(KeyPermissionStayInPlan, "Stay in Plan", "停留在计划模式", "Im Planmodus bleiben", "計画モードに留まる", "계획 모드 유지", "Оставаться в плане")
	add(KeyPermissionReject, "Reject", "拒绝", "Ablehnen", "拒否", "거부", "Отклонить")
	add(KeyRiskMedium, "medium", "中", "mittel", "中", "중간", "средний")
	add(KeyRiskHigh, "high", "高", "hoch", "高", "높음", "высокий")
	add(KeyPermissionDecision, "Permission Decision", "权限决定", "Berechtigungsentscheidung", "許可の決定", "권한 결정", "Решение о разрешении")
	add(KeyPlanDecision, "Plan Decision", "计划决定", "Planungsentscheidung", "計画の決定", "계획 결정", "Решение плана")
	add(KeyDecisionActor, "Actor: %s (%s)  Work: %s", "执行者：%s（%s）工作单元：%s", "Akteur: %s (%s)  Arbeit: %s", "実行者：%s（%s）作業：%s", "실행자: %s(%s) 작업: %s", "Исполнитель: %s (%s)  Работа: %s")
	add(KeyDecisionAgentSession, "Agent execution session: ", "代理执行会话：", "Agenten-Ausführungssitzung: ", "エージェント実行セッション：", "에이전트 실행 세션: ", "Сессия выполнения агента: ")
	add(KeyDecisionAction, "Action: ", "操作：", "Aktion: ", "操作：", "작업: ", "Действие: ")
	add(KeyDecisionTarget, "Target: ", "目标：", "Ziel: ", "対象：", "대상: ", "Цель: ")
	add(KeyDecisionImpact, "Impact: ", "影响：", "Auswirkung: ", "影響：", "영향: ", "Влияние: ")
	add(KeyDecisionRisk, "Risk: ", "风险：", "Risiko: ", "リスク：", "위험: ", "Риск: ")
	add(KeyDecisionScope, "Scope: ", "范围：", "Umfang: ", "範囲：", "범위: ", "Область: ")
	add(KeyDecisionInput, "Input: ", "输入：", "Eingabe: ", "入力：", "입력: ", "Ввод: ")
	add(KeyDecisionAfterApproval, "After approval: permission mode ", "批准后权限模式：", "Nach Genehmigung: Berechtigungsmodus ", "承認後の許可モード：", "승인 후 권한 모드: ", "После утверждения: режим разрешений ")
	add(KeyGoodbye, "Goodbye!", "再见！", "Auf Wiedersehen!", "さようなら！", "안녕히 가세요!", "До свидания!")
	add(KeyForkNoConversationTurns, "No conversation turns are available to fork.", "当前没有可用于分叉的对话轮次。", "Es sind keine Gesprächsrunden zum Verzweigen verfügbar.", "フォークできる会話ターンがありません。", "포크할 수 있는 대화 턴이 없습니다.", "Нет доступных реплик диалога для ответвления.")
}
