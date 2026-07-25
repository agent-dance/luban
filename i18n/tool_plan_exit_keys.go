package i18n

const (
	KeyToolPlanExitNotActive        Key = "tool.plan.exit.not_active"
	KeyToolPlanExitInvalidTypedData Key = "tool.plan.exit.invalid_typed_data"
	KeyToolPlanExitApproved         Key = "tool.plan.exit.approved"
	KeyToolPlanExitTeamHint         Key = "tool.plan.exit.team_hint"
	KeyToolPlanExitLabel            Key = "tool.plan.exit.label"
	KeyToolPlanExitEditedLabel      Key = "tool.plan.exit.edited_label"
	KeyToolPlanExitApprovedBody     Key = "tool.plan.exit.approved_body"
	KeyToolPlanExitRejected         Key = "tool.plan.exit.rejected"
	KeyToolPlanExitFeedback         Key = "tool.plan.exit.feedback"
	KeyToolPlanExitStateRequired    Key = "tool.plan.exit.state_required"
	KeyToolPlanExitPathMismatch     Key = "tool.plan.exit.path_mismatch"
	KeyToolPlanExitNoActiveFile     Key = "tool.plan.exit.no_active_file"
	KeyToolPlanExitReadFile         Key = "tool.plan.exit.read_file"
	KeyToolPlanExitMarshalInput     Key = "tool.plan.exit.marshal_input"
	KeyToolPlanExitInvalidInput     Key = "tool.plan.exit.invalid_input"
	KeyToolPlanExitInvalidPrompts   Key = "tool.plan.exit.invalid_prompts"
	KeyToolPlanExitReadBeforeExit   Key = "tool.plan.exit.read_before_exit"
	KeyToolPlanExitPersistEdited    Key = "tool.plan.exit.persist_edited"
	KeyToolPlanExitCommitRollback   Key = "tool.plan.exit.commit_rollback"
	KeyToolPlanExitCommit           Key = "tool.plan.exit.commit"
)

var toolPlanExitKeys = [...]Key{
	KeyToolPlanExitNotActive,
	KeyToolPlanExitInvalidTypedData,
	KeyToolPlanExitApproved,
	KeyToolPlanExitTeamHint,
	KeyToolPlanExitLabel,
	KeyToolPlanExitEditedLabel,
	KeyToolPlanExitApprovedBody,
	KeyToolPlanExitRejected,
	KeyToolPlanExitFeedback,
	KeyToolPlanExitStateRequired,
	KeyToolPlanExitPathMismatch,
	KeyToolPlanExitNoActiveFile,
	KeyToolPlanExitReadFile,
	KeyToolPlanExitMarshalInput,
	KeyToolPlanExitInvalidInput,
	KeyToolPlanExitInvalidPrompts,
	KeyToolPlanExitReadBeforeExit,
	KeyToolPlanExitPersistEdited,
	KeyToolPlanExitCommitRollback,
	KeyToolPlanExitCommit,
}

func init() {
	addToolPlanExit(KeyToolPlanExitNotActive, "not in plan mode; call EnterPlanMode first", "当前不在 plan mode；请先调用 EnterPlanMode", "Nicht im Planungsmodus; zuerst EnterPlanMode aufrufen", "plan mode ではありません。先に EnterPlanMode を呼び出してください", "plan mode가 아닙니다. 먼저 EnterPlanMode를 호출하세요", "Сейчас не режим планирования; сначала вызовите EnterPlanMode")
	addToolPlanExit(KeyToolPlanExitInvalidTypedData, "ExitPlanMode returned invalid typed data", "ExitPlanMode 返回了无效的类型化数据", "ExitPlanMode hat ungültige typisierte Daten zurückgegeben", "ExitPlanMode が無効な型付きデータを返しました", "ExitPlanMode가 잘못된 형식의 데이터를 반환했습니다", "ExitPlanMode вернул недопустимые типизированные данные")
	addToolPlanExit(KeyToolPlanExitApproved, "User has approved exiting plan mode. You can now proceed.", "用户已批准退出 plan mode，现在可以继续。", "Der Benutzer hat das Verlassen des Planungsmodus genehmigt. Du kannst jetzt fortfahren.", "ユーザーが plan mode の終了を承認しました。続行できます。", "사용자가 plan mode 종료를 승인했습니다. 이제 계속 진행할 수 있습니다.", "Пользователь разрешил выйти из режима планирования. Можно продолжать.")
	addToolPlanExit(KeyToolPlanExitTeamHint, "\n\nIf this plan can be broken down into multiple independent tasks, consider using the TeamCreate tool to create a team and parallelize the work.", "\n\n如果此 Plan 可以拆分为多个独立任务，可考虑使用 TeamCreate 创建团队并行处理。", "\n\nWenn sich dieser Plan in mehrere unabhängige Aufgaben aufteilen lässt, erwäge mit TeamCreate ein Team zu erstellen und die Arbeit zu parallelisieren.", "\n\nこの Plan を複数の独立したタスクに分割できる場合は、TeamCreate でチームを作成して並行作業することを検討してください。", "\n\n이 Plan을 여러 독립 작업으로 나눌 수 있다면 TeamCreate로 팀을 만들어 병렬로 진행하는 것을 고려하세요.", "\n\nЕсли этот Plan можно разбить на независимые задачи, рассмотрите создание команды через TeamCreate для параллельной работы.")
	addToolPlanExit(KeyToolPlanExitLabel, "Approved Plan", "已批准的 Plan", "Genehmigter Plan", "承認済み Plan", "승인된 Plan", "Утверждённый Plan")
	addToolPlanExit(KeyToolPlanExitEditedLabel, "Approved Plan (edited by user)", "已批准的 Plan（经用户编辑）", "Genehmigter Plan (vom Benutzer bearbeitet)", "承認済み Plan（ユーザーが編集）", "승인된 Plan(사용자 편집)", "Утверждённый Plan (изменён пользователем)")
	addToolPlanExit(KeyToolPlanExitApprovedBody, "User has approved your plan. You can now start coding. Start by updating the task list if applicable\n\nYour plan has been saved to: %s\nYou can refer back to it if needed during implementation.%s\n\n## %s:\n%s", "用户已批准你的 Plan，现在可以开始编码。如适用，请先更新任务列表。\n\nPlan 已保存至：%s\n实现过程中可随时查阅。%s\n\n## %s：\n%s", "Der Benutzer hat deinen Plan genehmigt. Du kannst jetzt mit der Umsetzung beginnen. Aktualisiere gegebenenfalls zuerst die Aufgabenliste.\n\nDein Plan wurde hier gespeichert: %s\nDu kannst während der Umsetzung darauf zurückgreifen.%s\n\n## %s:\n%s", "ユーザーが Plan を承認しました。実装を開始できます。必要に応じて、まずタスクリストを更新してください。\n\nPlan の保存先: %s\n実装中に必要であれば参照できます。%s\n\n## %s:\n%s", "사용자가 Plan을 승인했습니다. 이제 코딩을 시작할 수 있습니다. 해당하는 경우 먼저 작업 목록을 업데이트하세요.\n\nPlan 저장 위치: %s\n구현 중 필요하면 다시 참고할 수 있습니다.%s\n\n## %s:\n%s", "Пользователь утвердил ваш Plan. Теперь можно приступать к реализации. При необходимости сначала обновите список задач.\n\nPlan сохранён здесь: %s\nПри необходимости обращайтесь к нему во время реализации.%s\n\n## %s:\n%s")
	addToolPlanExit(KeyToolPlanExitRejected, "User rejected the plan. Stay in plan mode and revise it before requesting approval again.", "用户驳回了 Plan。请保持 plan mode，修改后再申请批准。", "Der Benutzer hat den Plan abgelehnt. Bleibe im Planungsmodus und überarbeite ihn, bevor du erneut um Genehmigung bittest.", "ユーザーが Plan を却下しました。plan mode のまま修正し、改めて承認を求めてください。", "사용자가 Plan을 거절했습니다. plan mode를 유지하고 수정한 뒤 다시 승인을 요청하세요.", "Пользователь отклонил Plan. Оставайтесь в режиме планирования, доработайте его и снова запросите утверждение.")
	addToolPlanExit(KeyToolPlanExitFeedback, "\n\nFeedback: %s", "\n\n反馈：%s", "\n\nFeedback: %s", "\n\nフィードバック: %s", "\n\n피드백: %s", "\n\nКомментарий: %s")
	addToolPlanExit(KeyToolPlanExitStateRequired, "ExitPlanMode requires plan state", "ExitPlanMode 需要 plan state", "ExitPlanMode benötigt einen Planungsstatus", "ExitPlanMode には plan state が必要です", "ExitPlanMode에는 plan state가 필요합니다", "Для ExitPlanMode требуется plan state")
	addToolPlanExit(KeyToolPlanExitPathMismatch, "ExitPlanMode planFilePath does not match the active plan", "ExitPlanMode 的 planFilePath 与当前 Plan 不匹配", "Der planFilePath von ExitPlanMode stimmt nicht mit dem aktiven Plan überein", "ExitPlanMode の planFilePath が現在の Plan と一致しません", "ExitPlanMode planFilePath가 활성 Plan과 일치하지 않습니다", "planFilePath ExitPlanMode не соответствует активному Plan")
	addToolPlanExit(KeyToolPlanExitNoActiveFile, "ExitPlanMode has no active plan file", "ExitPlanMode 没有活动的 Plan 文件", "ExitPlanMode hat keine aktive Plan-Datei", "ExitPlanMode に有効な Plan ファイルがありません", "ExitPlanMode에 활성 Plan 파일이 없습니다", "У ExitPlanMode нет активного файла Plan")
	addToolPlanExit(KeyToolPlanExitReadFile, "read plan file %s: %v", "读取 Plan 文件 %s 失败：%v", "Plan-Datei %s konnte nicht gelesen werden: %v", "Plan ファイル %s を読み取れませんでした: %v", "Plan 파일 %s을(를) 읽지 못했습니다: %v", "Не удалось прочитать файл Plan %s: %v")
	addToolPlanExit(KeyToolPlanExitMarshalInput, "marshal ExitPlanMode input: %v", "序列化 ExitPlanMode 输入失败：%v", "ExitPlanMode-Eingabe konnte nicht serialisiert werden: %v", "ExitPlanMode の入力をシリアル化できませんでした: %v", "ExitPlanMode 입력을 직렬화하지 못했습니다: %v", "Не удалось сериализовать вход ExitPlanMode: %v")
	addToolPlanExit(KeyToolPlanExitInvalidInput, "invalid ExitPlanMode input: %v", "ExitPlanMode 输入无效：%v", "Ungültige ExitPlanMode-Eingabe: %v", "ExitPlanMode の入力が無効です: %v", "잘못된 ExitPlanMode 입력: %v", "Недопустимый вход ExitPlanMode: %v")
	addToolPlanExit(KeyToolPlanExitInvalidPrompts, "invalid ExitPlanMode allowedPrompts entry: tool must be Bash and prompt must be non-empty", "ExitPlanMode 的 allowedPrompts 条目无效：tool 必须为 Bash，且 prompt 不能为空", "Ungültiger allowedPrompts-Eintrag für ExitPlanMode: tool muss Bash sein und prompt darf nicht leer sein", "ExitPlanMode の allowedPrompts エントリが無効です: tool は Bash、prompt は空でない必要があります", "잘못된 ExitPlanMode allowedPrompts 항목입니다. tool은 Bash여야 하고 prompt는 비어 있으면 안 됩니다", "Недопустимая запись allowedPrompts ExitPlanMode: tool должен быть Bash, а prompt — непустым")
	addToolPlanExit(KeyToolPlanExitReadBeforeExit, "read plan file %s before exit: %v", "退出前读取 Plan 文件 %s 失败：%v", "Plan-Datei %s konnte vor dem Beenden nicht gelesen werden: %v", "終了前に Plan ファイル %s を読み取れませんでした: %v", "종료 전에 Plan 파일 %s을(를) 읽지 못했습니다: %v", "Не удалось прочитать файл Plan %s перед выходом: %v")
	addToolPlanExit(KeyToolPlanExitPersistEdited, "persist edited plan: %v", "保存用户编辑后的 Plan 失败：%v", "Bearbeiteter Plan konnte nicht gespeichert werden: %v", "編集済み Plan を保存できませんでした: %v", "편집된 Plan을 저장하지 못했습니다: %v", "Не удалось сохранить изменённый Plan: %v")
	addToolPlanExit(KeyToolPlanExitCommitRollback, "commit plan exit: %[2]v; restore original plan: %[1]v", "提交 Plan 退出失败：%[2]v；恢复原始 Plan 失败：%[1]v", "Beenden des Planungsmodus konnte nicht übernommen werden: %[2]v; ursprünglicher Plan konnte nicht wiederhergestellt werden: %[1]v", "Plan の終了を確定できませんでした: %[2]v。元の Plan の復元にも失敗しました: %[1]v", "Plan 종료를 반영하지 못했습니다: %[2]v. 원래 Plan 복원도 실패했습니다: %[1]v", "Не удалось применить выход из Plan: %[2]v; также не удалось восстановить исходный Plan: %[1]v")
	addToolPlanExit(KeyToolPlanExitCommit, "commit plan exit: %v", "提交 Plan 退出失败：%v", "Beenden des Planungsmodus konnte nicht übernommen werden: %v", "Plan の終了を確定できませんでした: %v", "Plan 종료를 반영하지 못했습니다: %v", "Не удалось применить выход из Plan: %v")
}

func addToolPlanExit(key Key, en, zh, de, ja, ko, ru string) {
	semanticTranslations[key] = map[Language]string{
		LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
	}
}
