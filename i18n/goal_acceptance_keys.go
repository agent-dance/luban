package i18n

// Semantic copy for versioned Goal acceptance conditions. Criterion IDs and
// text are user data and are always passed as format values.
const (
	KeyCommandGoalTransitionReport Key = "command.goal.transition_report"
	KeyCommandGoalCriteriaHeader   Key = "command.goal.criteria.header"
	KeyCommandGoalCriterionPending Key = "command.goal.criteria.pending"
	KeyCommandGoalCriterionMet     Key = "command.goal.criteria.met"
	KeyCommandGoalCriterionUnmet   Key = "command.goal.criteria.unmet"
	KeyCommandGoalCriterionReason  Key = "command.goal.criteria.reason"
	KeyCommandGoalCriterionAdded   Key = "command.goal.criteria.added"
	KeyCommandGoalCriterionUpdated Key = "command.goal.criteria.updated"
	KeyCommandGoalCriterionRemoved Key = "command.goal.criteria.removed"

	KeyRootGoalAcceptanceCriteriaRequired   Key = "root.goal.acceptance_criteria_required"
	KeyRootGoalAcceptanceCriteriaTooMany    Key = "root.goal.acceptance_criteria_too_many"
	KeyRootGoalAcceptanceCriterionRequired  Key = "root.goal.acceptance_criterion_required"
	KeyRootGoalAcceptanceCriterionTooLong   Key = "root.goal.acceptance_criterion_too_long"
	KeyRootGoalAcceptanceCriterionDuplicate Key = "root.goal.acceptance_criterion_duplicate"
	KeyRootGoalAcceptanceCriterionNotFound  Key = "root.goal.acceptance_criterion_not_found"
	KeyRootGoalCannotRemoveLastCriterion    Key = "root.goal.acceptance_criterion_last"
	KeyRootGoalAcceptanceCriteriaUnmet      Key = "root.goal.acceptance_criteria_unmet"
	KeyRootGoalAcceptanceEvaluationInvalid  Key = "root.goal.acceptance_evaluation_invalid"
	KeyRootGoalAcceptanceEvaluationStale    Key = "root.goal.acceptance_evaluation_stale"

	KeyToolGoalAcceptanceCriteriaHeader Key = "tool.goal.acceptance_criteria.header"
	KeyToolGoalAcceptanceCriterionItem  Key = "tool.goal.acceptance_criteria.item"
	KeyToolGoalAcceptanceCriterionMet   Key = "tool.goal.acceptance_criteria.met"
	KeyToolGoalAcceptanceCriterionUnmet Key = "tool.goal.acceptance_criteria.unmet"
	KeyToolGoalAcceptanceReason         Key = "tool.goal.acceptance_criteria.reason"
	KeyToolGoalRevisionRequired         Key = "tool.goal.acceptance_criteria.revision_required"
	KeyToolGoalRevisionStale            Key = "tool.goal.acceptance_criteria.revision_stale"
	KeyToolGoalRevisionFieldsUnexpected Key = "tool.goal.acceptance_criteria.revision_fields_unexpected"
	KeyPresentationGoalCriteriaCount    Key = "presentation.goal.acceptance_criteria.count"
	KeyPresentationGoalCriteriaProgress Key = "presentation.goal.acceptance_criteria.progress"
	KeyTUIGoalPanelTitle                Key = "tui.goal.acceptance.title"
	KeyTUIGoalCriterionPending          Key = "tui.goal.acceptance.criterion.pending"
	KeyTUIGoalCriterionMet              Key = "tui.goal.acceptance.criterion.met"
	KeyTUIGoalCriterionUnmet            Key = "tui.goal.acceptance.criterion.unmet"
	KeyTUIGoalCriteriaMore              Key = "tui.goal.acceptance.more"
	KeyTUIGoalStatusProgress            Key = "tui.goal.acceptance.status_progress"

	KeyLoopGoalEvaluatorMissingCriteria    Key = "loop.goal_evaluator.missing_criteria"
	KeyLoopGoalEvaluatorCriterionInvalid   Key = "loop.goal_evaluator.criterion_invalid"
	KeyLoopGoalEvaluatorCriteriaIncomplete Key = "loop.goal_evaluator.criteria_incomplete"
)

func goalAcceptanceCopy(en, zh, de, ja, ko, ru string) map[Language]string {
	return map[Language]string{LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru}
}

func init() {
	entries := map[Key]map[Language]string{
		KeyCommandGoalTransitionReport: goalAcceptanceCopy(
			"%s\n%s", "%s\n%s", "%s\n%s", "%s\n%s", "%s\n%s", "%s\n%s"),
		KeyCommandGoalCriteriaHeader: goalAcceptanceCopy(
			"\nAcceptance criteria (revision %d):", "\n验收条件（版本 %d）：", "\nAbnahmekriterien (Revision %d):", "\n受け入れ条件（リビジョン %d）:", "\n인수 조건(리비전 %d):", "\nКритерии приёмки (версия %d):"),
		KeyCommandGoalCriterionPending: goalAcceptanceCopy(
			"\n  [ ] %s: %s", "\n  [ ] %s：%s", "\n  [ ] %s: %s", "\n  [ ] %s: %s", "\n  [ ] %s: %s", "\n  [ ] %s: %s"),
		KeyCommandGoalCriterionMet: goalAcceptanceCopy(
			"\n  [x] %s: %s", "\n  [x] %s：%s", "\n  [x] %s: %s", "\n  [x] %s: %s", "\n  [x] %s: %s", "\n  [x] %s: %s"),
		KeyCommandGoalCriterionUnmet: goalAcceptanceCopy(
			"\n  [!] %s: %s", "\n  [!] %s：%s", "\n  [!] %s: %s", "\n  [!] %s: %s", "\n  [!] %s: %s", "\n  [!] %s: %s"),
		KeyCommandGoalCriterionReason: goalAcceptanceCopy(
			"\n      Reason: %s", "\n      原因：%s", "\n      Grund: %s", "\n      理由: %s", "\n      이유: %s", "\n      Причина: %s"),
		KeyCommandGoalCriterionAdded: goalAcceptanceCopy(
			"Acceptance criterion added", "验收条件已添加", "Abnahmekriterium hinzugefügt", "受け入れ条件を追加しました", "인수 조건을 추가했습니다", "Критерий приёмки добавлен"),
		KeyCommandGoalCriterionUpdated: goalAcceptanceCopy(
			"Acceptance criterion updated", "验收条件已更新", "Abnahmekriterium aktualisiert", "受け入れ条件を更新しました", "인수 조건을 업데이트했습니다", "Критерий приёмки обновлён"),
		KeyCommandGoalCriterionRemoved: goalAcceptanceCopy(
			"Acceptance criterion removed", "验收条件已移除", "Abnahmekriterium entfernt", "受け入れ条件を削除しました", "인수 조건을 삭제했습니다", "Критерий приёмки удалён"),

		KeyRootGoalAcceptanceCriteriaRequired: goalAcceptanceCopy(
			"At least one explicit acceptance criterion is required", "至少需要一条明确的验收条件", "Mindestens ein ausdrückliches Abnahmekriterium ist erforderlich", "明確な受け入れ条件が1つ以上必要です", "명확한 인수 조건이 하나 이상 필요합니다", "Необходим хотя бы один явный критерий приёмки"),
		KeyRootGoalAcceptanceCriteriaTooMany: goalAcceptanceCopy(
			"A goal may have at most %d acceptance criteria", "一个目标最多可有 %d 条验收条件", "Ein Ziel darf höchstens %d Abnahmekriterien haben", "1つの目標に設定できる受け入れ条件は最大 %d 件です", "하나의 목표에는 인수 조건을 최대 %d개까지 설정할 수 있습니다", "У цели может быть не более %d критериев приёмки"),
		KeyRootGoalAcceptanceCriterionRequired: goalAcceptanceCopy(
			"Acceptance criterion text is required", "必须填写验收条件", "Der Text des Abnahmekriteriums ist erforderlich", "受け入れ条件を入力してください", "인수 조건 내용을 입력해야 합니다", "Необходимо указать текст критерия приёмки"),
		KeyRootGoalAcceptanceCriterionTooLong: goalAcceptanceCopy(
			"An acceptance criterion must not exceed %d characters", "验收条件不得超过 %d 个字符", "Ein Abnahmekriterium darf höchstens %d Zeichen lang sein", "受け入れ条件は %d 文字以内で入力してください", "인수 조건은 %d자를 초과할 수 없습니다", "Критерий приёмки не должен превышать %d символов"),
		KeyRootGoalAcceptanceCriterionDuplicate: goalAcceptanceCopy(
			"Acceptance criteria must be unique", "验收条件不能重复", "Abnahmekriterien müssen eindeutig sein", "受け入れ条件は重複できません", "인수 조건은 중복될 수 없습니다", "Критерии приёмки не должны повторяться"),
		KeyRootGoalAcceptanceCriterionNotFound: goalAcceptanceCopy(
			"The acceptance criterion was not found", "未找到该验收条件", "Das Abnahmekriterium wurde nicht gefunden", "受け入れ条件が見つかりません", "인수 조건을 찾을 수 없습니다", "Критерий приёмки не найден"),
		KeyRootGoalCannotRemoveLastCriterion: goalAcceptanceCopy(
			"A goal must keep at least one acceptance criterion", "目标必须保留至少一条验收条件", "Ein Ziel muss mindestens ein Abnahmekriterium behalten", "目標には受け入れ条件を1つ以上残す必要があります", "목표에는 인수 조건을 하나 이상 남겨야 합니다", "У цели должен оставаться хотя бы один критерий приёмки"),
		KeyRootGoalAcceptanceCriteriaUnmet: goalAcceptanceCopy(
			"The current goal revision cannot complete until every acceptance criterion is verified", "当前目标版本必须验证全部验收条件后才能完成", "Die aktuelle Zielrevision kann erst abgeschlossen werden, wenn alle Abnahmekriterien verifiziert sind", "現在の目標リビジョンは、すべての受け入れ条件が検証されるまで完了できません", "현재 목표 리비전은 모든 인수 조건이 검증되어야 완료할 수 있습니다", "Текущую версию цели нельзя завершить, пока не проверены все критерии приёмки"),
		KeyRootGoalAcceptanceEvaluationInvalid: goalAcceptanceCopy(
			"The acceptance evaluation does not contain one valid result for every criterion", "验收评估未包含每条条件各自唯一且有效的结果", "Die Abnahmebewertung enthält nicht für jedes Kriterium genau ein gültiges Ergebnis", "受け入れ評価に各条件の有効な結果が1件ずつ含まれていません", "인수 평가에 각 조건의 유효한 결과가 하나씩 포함되어 있지 않습니다", "Оценка приёмки не содержит по одному корректному результату для каждого критерия"),
		KeyRootGoalAcceptanceEvaluationStale: goalAcceptanceCopy(
			"The goal changed while its acceptance criteria were being evaluated", "验收条件评估期间目标已发生变化", "Das Ziel wurde während der Bewertung seiner Abnahmekriterien geändert", "受け入れ条件の評価中に目標が変更されました", "인수 조건을 평가하는 동안 목표가 변경되었습니다", "Цель изменилась во время оценки критериев приёмки"),

		KeyToolGoalAcceptanceCriteriaHeader: goalAcceptanceCopy(
			"Acceptance criteria (revision %d):", "验收条件（版本 %d）：", "Abnahmekriterien (Revision %d):", "受け入れ条件（リビジョン %d）:", "인수 조건(리비전 %d):", "Критерии приёмки (версия %d):"),
		KeyToolGoalAcceptanceCriterionItem: goalAcceptanceCopy(
			"[ ] %s: %s", "[ ] %s：%s", "[ ] %s: %s", "[ ] %s: %s", "[ ] %s: %s", "[ ] %s: %s"),
		KeyToolGoalAcceptanceCriterionMet: goalAcceptanceCopy(
			"[x] %s: %s", "[x] %s：%s", "[x] %s: %s", "[x] %s: %s", "[x] %s: %s", "[x] %s: %s"),
		KeyToolGoalAcceptanceCriterionUnmet: goalAcceptanceCopy(
			"[!] %s: %s", "[!] %s：%s", "[!] %s: %s", "[!] %s: %s", "[!] %s: %s", "[!] %s: %s"),
		KeyToolGoalAcceptanceReason: goalAcceptanceCopy(
			"  Reason: %s", "  原因：%s", "  Grund: %s", "  理由: %s", "  이유: %s", "  Причина: %s"),
		KeyToolGoalRevisionRequired: goalAcceptanceCopy(
			"expected_revision must be a positive integer when status is revise", "status 为 revise 时，expected_revision 必须为正整数", "Bei status revise muss expected_revision eine positive Ganzzahl sein", "status が revise の場合、expected_revision は正の整数にしてください", "status가 revise이면 expected_revision은 양의 정수여야 합니다", "При status=revise поле expected_revision должно быть положительным целым числом"),
		KeyToolGoalRevisionStale: goalAcceptanceCopy(
			"The goal revision changed; reload the goal before revising its acceptance criteria", "目标版本已变化；修改验收条件前请重新加载目标", "Die Zielrevision wurde geändert; lade das Ziel vor der Überarbeitung der Abnahmekriterien neu", "目標リビジョンが変更されました。受け入れ条件を修正する前に目標を再読み込みしてください", "목표 리비전이 변경되었습니다. 인수 조건을 수정하기 전에 목표를 다시 불러오세요", "Версия цели изменилась; перед изменением критериев приёмки загрузите цель заново"),
		KeyToolGoalRevisionFieldsUnexpected: goalAcceptanceCopy(
			"acceptance_criteria and expected_revision are only valid when status is revise", "acceptance_criteria 和 expected_revision 仅可在 status 为 revise 时使用", "acceptance_criteria und expected_revision sind nur bei status revise zulässig", "acceptance_criteria と expected_revision は status が revise の場合にのみ使用できます", "acceptance_criteria와 expected_revision은 status가 revise일 때만 사용할 수 있습니다", "Поля acceptance_criteria и expected_revision допустимы только при status=revise"),
		KeyPresentationGoalCriteriaCount: goalAcceptanceCopy(
			"%d acceptance criteria", "%d 条验收条件", "%d Abnahmekriterien", "受け入れ条件 %d 件", "인수 조건 %d개", "%d критериев приёмки"),
		KeyPresentationGoalCriteriaProgress: goalAcceptanceCopy(
			"%d/%d criteria met", "已满足 %d/%d 条条件", "%d/%d Kriterien erfüllt", "%d/%d 件の条件を達成", "조건 %d/%d 충족", "Выполнено критериев: %d/%d"),
		KeyTUIGoalPanelTitle: goalAcceptanceCopy(
			"Goal acceptance · %s · revision %d · %d/%d met — %s", "目标验收 · %s · 版本 %d · 已满足 %d/%d — %s", "Zielabnahme · %s · Revision %d · %d/%d erfüllt — %s", "目標の受け入れ · %s · リビジョン %d · %d/%d 達成 — %s", "목표 인수 · %s · 리비전 %d · %d/%d 충족 — %s", "Приёмка цели · %s · версия %d · выполнено %d/%d — %s"),
		KeyTUIGoalCriterionPending: goalAcceptanceCopy(
			"  [pending] %s: %s", "  [待验收] %s：%s", "  [ausstehend] %s: %s", "  [未評価] %s: %s", "  [대기] %s: %s", "  [ожидает] %s: %s"),
		KeyTUIGoalCriterionMet: goalAcceptanceCopy(
			"  [met] %s: %s", "  [已通过] %s：%s", "  [erfüllt] %s: %s", "  [達成] %s: %s", "  [충족] %s: %s", "  [выполнен] %s: %s"),
		KeyTUIGoalCriterionUnmet: goalAcceptanceCopy(
			"  [not met] %s: %s", "  [未通过] %s：%s", "  [nicht erfüllt] %s: %s", "  [未達成] %s: %s", "  [미충족] %s: %s", "  [не выполнен] %s: %s"),
		KeyTUIGoalCriteriaMore: goalAcceptanceCopy(
			"  … %d more acceptance criteria", "  … 另有 %d 条验收条件", "  … %d weitere Abnahmekriterien", "  … 他 %d 件の受け入れ条件", "  … 인수 조건 %d개 더 있음", "  … ещё %d критериев приёмки"),
		KeyTUIGoalStatusProgress: goalAcceptanceCopy(
			" · %d/%d met", " · 已满足 %d/%d", " · %d/%d erfüllt", " · %d/%d 達成", " · %d/%d 충족", " · выполнено %d/%d"),

		KeyLoopGoalEvaluatorMissingCriteria: goalAcceptanceCopy(
			"goal evaluator response is missing acceptance criterion results", "目标评估响应缺少验收条件结果", "In der Antwort der Zielauswertung fehlen Ergebnisse der Abnahmekriterien", "目標評価の応答に受け入れ条件の結果がありません", "목표 평가 응답에 인수 조건 결과가 없습니다", "В ответе оценщика цели отсутствуют результаты критериев приёмки"),
		KeyLoopGoalEvaluatorCriterionInvalid: goalAcceptanceCopy(
			"goal evaluator returned an invalid acceptance criterion result", "目标评估器返回了无效的验收条件结果", "Die Zielauswertung hat ein ungültiges Ergebnis für ein Abnahmekriterium geliefert", "目標評価が無効な受け入れ条件の結果を返しました", "목표 평가기가 잘못된 인수 조건 결과를 반환했습니다", "Оценщик цели вернул некорректный результат критерия приёмки"),
		KeyLoopGoalEvaluatorCriteriaIncomplete: goalAcceptanceCopy(
			"goal evaluator did not return exactly one result for every acceptance criterion", "目标评估器未对每条验收条件各返回一个结果", "Die Zielauswertung hat nicht genau ein Ergebnis für jedes Abnahmekriterium geliefert", "目標評価が各受け入れ条件に対して1件ずつ結果を返しませんでした", "목표 평가기가 각 인수 조건에 대해 정확히 하나의 결과를 반환하지 않았습니다", "Оценщик цели не вернул ровно по одному результату для каждого критерия приёмки"),
	}
	for key, translations := range entries {
		semanticTranslations[key] = translations
	}
}
