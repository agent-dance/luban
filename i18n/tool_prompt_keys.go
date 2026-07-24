package i18n

const (
	KeyAskUserPermission        Key = "tool.ask_user.permission"
	KeyAskUserPreviewSideBySide Key = "tool.ask_user.preview.side_by_side"
	KeyAskUserMultiPrompt       Key = "tool.ask_user.prompt.multi"
	KeyAskUserSinglePrompt      Key = "tool.ask_user.prompt.single"
	KeyAskUserCustomPrompt      Key = "tool.ask_user.prompt.custom"
	KeyAskUserOtherOption       Key = "tool.ask_user.option.other"
	KeyAskUserTUISingleHint     Key = "tool.ask_user.tui.single_hint"
	KeyAskUserTUIMultiHint      Key = "tool.ask_user.tui.multi_hint"
	KeyAskUserTUICustomHint     Key = "tool.ask_user.tui.custom_hint"
	KeyAskUserProgress          Key = "tool.ask_user.progress"
	KeyAskUserTUINotesPrompt    Key = "tool.ask_user.tui.notes_prompt"
	KeyAskUserTUINotesHint      Key = "tool.ask_user.tui.notes_hint"
	KeyAskUserTUINotesAvailable Key = "tool.ask_user.tui.notes_available"
	KeyBashSandboxBuildError    Key = "tool.bash.sandbox.build_error"
	KeyBashSandboxFallback      Key = "tool.bash.sandbox.fallback"
)

func init() {
	addToolPrompt(KeyAskUserPermission, "Answer questions?", "回答这些问题？", "Fragen beantworten?", "質問に回答しますか？", "질문에 답할까요?", "Ответить на вопросы?")
	addToolPrompt(KeyAskUserPreviewSideBySide, "(preview: side by side)\n", "（预览：并排）\n", "(Vorschau: nebeneinander)\n", "（プレビュー: 横並び）\n", "(미리 보기: 나란히)\n", "(предпросмотр: рядом)\n")
	addToolPrompt(KeyAskUserMultiPrompt, "Enter comma-separated numbers (for example, 1,3) or 'o:<text>' for Other; append ' n:<notes>' to add notes: ", "输入以逗号分隔的编号（例如 1,3），或输入 'o:<文本>' 选择“其他”；可追加 ' n:<备注>' 添加备注：", "Gib kommagetrennte Nummern ein (z. B. 1,3) oder 'o:<Text>' für Sonstiges; mit ' n:<Notizen>' kannst du Notizen anhängen: ", "カンマ区切りの番号（例: 1,3）を入力するか、「その他」は 'o:<テキスト>' を入力してください。' n:<メモ>' を末尾に付けるとメモを追加できます: ", "쉼표로 구분한 번호(예: 1,3)를 입력하거나 기타 항목은 'o:<텍스트>'를 입력하세요. 메모는 뒤에 ' n:<메모>'를 붙이세요: ", "Введите номера через запятую (например, 1,3) или 'o:<текст>' для другого ответа; для заметки добавьте ' n:<заметка>': ")
	addToolPrompt(KeyAskUserSinglePrompt, "Enter a number (1-%d) or 'o' for Other; append ' n:<notes>' to add notes: ", "输入编号（1-%d），或输入 'o' 选择“其他”；可追加 ' n:<备注>' 添加备注：", "Gib eine Nummer (1-%d) oder 'o' für Sonstiges ein; mit ' n:<Notizen>' kannst du Notizen anhängen: ", "番号（1-%d）または「その他」の 'o' を入力してください。' n:<メモ>' を末尾に付けるとメモを追加できます: ", "번호(1-%d) 또는 기타 항목을 뜻하는 'o'를 입력하세요. 메모는 뒤에 ' n:<메모>'를 붙이세요: ", "Введите номер (1-%d) или 'o' для другого ответа; для заметки добавьте ' n:<заметка>': ")
	addToolPrompt(KeyAskUserCustomPrompt, "Enter your custom answer: ", "输入自定义答案：", "Gib deine eigene Antwort ein: ", "回答を入力してください: ", "직접 답변을 입력하세요: ", "Введите свой ответ: ")
	addToolPrompt(KeyAskUserOtherOption, "Other (type an answer)", "其他（输入答案）", "Sonstiges (Antwort eingeben)", "その他（回答を入力）", "기타(답변 입력)", "Другое (ввести ответ)")
	addToolPrompt(KeyAskUserTUISingleHint, "Use ↑/↓ to choose, Enter to submit, or Esc to cancel.", "使用 ↑/↓ 选择，按 Enter 提交，或按 Esc 取消。", "Mit ↑/↓ auswählen, mit Enter absenden oder mit Esc abbrechen.", "↑/↓ で選択し、Enter で送信、Esc でキャンセルします。", "↑/↓로 선택하고 Enter로 제출하거나 Esc로 취소하세요.", "Выберите вариант клавишами ↑/↓, нажмите Enter для отправки или Esc для отмены.")
	addToolPrompt(KeyAskUserTUIMultiHint, "Use ↑/↓ to move, Space to toggle, Enter to submit, or Esc to cancel.", "使用 ↑/↓ 移动，按空格切换选择，按 Enter 提交，或按 Esc 取消。", "Mit ↑/↓ navigieren, mit Leertaste umschalten, mit Enter absenden oder mit Esc abbrechen.", "↑/↓ で移動し、Space で切り替え、Enter で送信、Esc でキャンセルします。", "↑/↓로 이동하고 Space로 선택을 전환한 뒤 Enter로 제출하거나 Esc로 취소하세요.", "Перемещайтесь клавишами ↑/↓, отмечайте пробелом, отправляйте Enter или отменяйте Esc.")
	addToolPrompt(KeyAskUserTUICustomHint, "Type an answer, press Enter to submit, or Esc to return to the choices.", "输入答案，按 Enter 提交，或按 Esc 返回选项。", "Antwort eingeben, mit Enter absenden oder mit Esc zu den Optionen zurückkehren.", "回答を入力し、Enter で送信するか、Esc で選択肢に戻ります。", "답변을 입력하고 Enter로 제출하거나 Esc로 선택 항목으로 돌아가세요.", "Введите ответ, нажмите Enter для отправки или Esc для возврата к вариантам.")
	addToolPrompt(KeyAskUserProgress, "Question %d of %d", "第 %d 个问题，共 %d 个", "Frage %d von %d", "質問 %d / %d", "질문 %d/%d", "Вопрос %d из %d")
	addToolPrompt(KeyAskUserTUINotesPrompt, "Notes: ", "备注：", "Notizen: ", "メモ: ", "메모: ", "Примечания: ")
	addToolPrompt(KeyAskUserTUINotesHint, "Type optional notes and press Enter to submit the answer, or Esc to return.", "输入可选备注并按 Enter 提交答案，或按 Esc 返回。", "Optionale Notizen eingeben und mit Enter die Antwort absenden oder mit Esc zurückkehren.", "任意のメモを入力して Enter で回答を送信するか、Esc で戻ります。", "선택적 메모를 입력하고 Enter로 답변을 제출하거나 Esc로 돌아가세요.", "Введите необязательные примечания и нажмите Enter для отправки ответа или Esc для возврата.")
	addToolPrompt(KeyAskUserTUINotesAvailable, "Press N to add optional notes.", "按 N 添加可选备注。", "Mit N optionale Notizen hinzufügen.", "N で任意のメモを追加できます。", "N을 눌러 선택적 메모를 추가하세요.", "Нажмите N, чтобы добавить необязательные примечания.")
	addToolPrompt(KeyBashSandboxBuildError, "Could not build the isolated shell sandbox", "无法构建隔离的 shell sandbox", "Die isolierte Shell-Sandbox konnte nicht erstellt werden", "隔離された shell sandbox を構築できませんでした", "격리된 shell sandbox를 구성할 수 없습니다", "Не удалось создать изолированную sandbox для shell")
	addToolPrompt(KeyBashSandboxFallback, "sandbox: Warning: could not build a sandboxed command (%v); running without sandboxing\n", "sandbox：警告：无法构建 sandbox 命令（%v）；将不使用 sandbox 运行\n", "sandbox: Warnung: Sandbox-Befehl konnte nicht erstellt werden (%v); Ausführung ohne Sandbox\n", "sandbox: 警告: sandbox コマンドを構築できませんでした（%v）。sandbox なしで実行します\n", "sandbox: 경고: sandbox 명령을 구성할 수 없습니다(%v). sandbox 없이 실행합니다\n", "sandbox: Предупреждение: не удалось создать команду в sandbox (%v); запуск без sandbox\n")
}

func addToolPrompt(key Key, en, zh, de, ja, ko, ru string) {
	semanticTranslations[key] = map[Language]string{LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru}
}
