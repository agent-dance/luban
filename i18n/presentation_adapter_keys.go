package i18n

func init() {
	adapter := func(en, zh, de, ja, ko, ru string) map[Language]string {
		return map[Language]string{LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru}
	}
	for key, translations := range map[Key]map[Language]string{
		KeyAdapterAggregateAction:        adapter("group routine operations", "聚合常规操作", "Routinevorgänge gruppieren", "定型操作をグループ化", "일반 작업 그룹화", "сгруппировать обычные операции"),
		KeyAdapterAggregateMembers:       adapter("Member IDs: %s", "成员 ID：%s", "Mitglieds-IDs: %s", "メンバー ID: %s", "구성원 ID: %s", "ID участников: %s"),
		KeyAdapterAggregateSummary:       adapter("%s - %d operations", "%s - %d 次操作", "%s - %d Vorgänge", "%s - %d 件の操作", "%s - 작업 %d개", "%s - %d операций"),
		KeyAdapterActionShell:            adapter("run command", "运行命令", "Befehl ausführen", "コマンドを実行", "명령 실행", "выполнить команду"),
		KeyAdapterActionFileRead:         adapter("read", "读取", "lesen", "読み取り", "읽기", "прочитать"),
		KeyAdapterActionFileWrite:        adapter("change file", "修改文件", "Datei ändern", "ファイルを変更", "파일 변경", "изменить файл"),
		KeyAdapterActionSearch:           adapter("search", "搜索", "suchen", "検索", "검색", "найти"),
		KeyAdapterActionWeb:              adapter("access web", "访问网页", "Webzugriff", "Web にアクセス", "웹 접근", "обратиться к веб-ресурсу"),
		KeyAdapterActionMCP:              adapter("call MCP", "调用 MCP", "MCP aufrufen", "MCP を呼び出す", "MCP 호출", "вызвать MCP"),
		KeyAdapterActionAgent:            adapter("delegate", "委派", "delegieren", "委任", "위임", "делегировать"),
		KeyAdapterActionDecision:         adapter("request decision", "请求决策", "Entscheidung anfordern", "判断を要求", "결정 요청", "запросить решение"),
		KeyAdapterActionMessage:          adapter("send message", "发送消息", "Nachricht senden", "メッセージを送信", "메시지 전송", "отправить сообщение"),
		KeyAdapterCommandRunning:         adapter("Command /%s: running %s%s.", "命令 /%s：正在%s%s。", "Befehl /%s: %s%s läuft.", "コマンド /%s: %s%s を実行中です。", "명령 /%s: %s%s 실행 중.", "Команда /%s: выполняется %s%s."),
		KeyAdapterCommandUnstructured:    adapter("completed; domain outcome not structured", "已完成；领域结果未结构化", "abgeschlossen; Domänenergebnis nicht strukturiert", "完了。ドメイン結果は構造化されていません", "완료됨; 도메인 결과가 구조화되지 않음", "завершено; доменный результат не структурирован"),
		KeyAdapterCommandTerminal:        adapter("Command /%s: %s.", "命令 /%s：%s。", "Befehl /%s: %s.", "コマンド /%s: %s。", "명령 /%s: %s.", "Команда /%s: %s."),
		KeyAdapterCommandDisplayRisk:     adapter(" Display: %s. Risk: %s.", " 展示：%s。风险：%s。", " Anzeige: %s. Risiko: %s.", " 表示: %s。リスク: %s。", " 표시: %s. 위험: %s.", " Представление: %s. Риск: %s."),
		KeyAdapterCommandNext:            adapter(" Next: %s", " 下一步：%s", " Nächster Schritt: %s", " 次: %s", " 다음: %s", " Далее: %s"),
		KeyAdapterCommandEvidenceRefs:    adapter(" Evidence references: %s.", " 证据引用：%s。", " Belegreferenzen: %s.", " 証拠参照: %s。", " 근거 참조: %s.", " Ссылки на данные: %s."),
		KeyAdapterCommandMoreRetained:    adapter(" More detail is retained.", " 已保留更多详情。", " Weitere Details wurden beibehalten.", " 追加の詳細が保持されています。", " 추가 세부 정보가 보존되었습니다.", " Дополнительные сведения сохранены."),
		KeyAdapterCommandSensitiveHidden: adapter(" Sensitive values were redacted.", " 敏感值已脱敏。", " Vertrauliche Werte wurden geschwärzt.", " 機密値はマスキングされています。", " 민감한 값이 마스킹되었습니다.", " Конфиденциальные значения скрыты."),
		KeyAdapterReviewNext:             adapter("review the result and retained evidence", "检查结果和保留的证据", "Ergebnis und beibehaltene Belege prüfen", "結果と保持された証拠を確認", "결과와 보존된 근거 검토", "проверить результат и сохранённые данные"),
	} {
		semanticTranslations[key] = translations
	}
}
