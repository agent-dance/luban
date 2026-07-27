package i18n

// Semantic copy for the query-local Agentic V2 correctness flight. Runtime
// disposition, epoch, and blocker identifiers remain untranslated protocol
// values and are never used as display copy.
const (
	KeyLoopQueryFlightStateInvalid           Key = "loop.query.flight_state_invalid"
	KeyLoopQueryFlightRepeatedFailure        Key = "loop.query.flight_repeated_failure"
	KeyLoopVisibleFlightVerificationRequired Key = "loop.visible.flight_verification_required"
)

var loopFlightKeys = [...]Key{
	KeyLoopQueryFlightStateInvalid,
	KeyLoopQueryFlightRepeatedFailure,
	KeyLoopVisibleFlightVerificationRequired,
}

func init() {
	entries := map[Key][6]string{
		KeyLoopQueryFlightStateInvalid: {
			"The coding-flight state could not be validated.",
			"无法验证当前 coding flight 状态。",
			"Der Zustand des Coding-Flights konnte nicht validiert werden.",
			"coding flight の状態を検証できませんでした。",
			"coding flight 상태를 검증하지 못했습니다.",
			"Не удалось проверить состояние coding flight.",
		},
		KeyLoopQueryFlightRepeatedFailure: {
			"Stopped after the same deterministic tool failure repeated against an unchanged workspace. Change the prerequisite, input, hypothesis, or workspace state before retrying.",
			"同一确定性工具失败在未变化的工作区上重复出现，已停止执行。再次尝试前，请改变前置条件、输入、假设或工作区状态。",
			"Die Ausführung wurde beendet, nachdem derselbe deterministische Tool-Fehler im unveränderten Workspace wiederholt auftrat. Ändern Sie vor einem erneuten Versuch die Voraussetzung, Eingabe, Hypothese oder den Workspace-Zustand.",
			"変更されていない workspace で同じ決定的なツールエラーが繰り返されたため、実行を停止しました。再試行する前に、前提条件、入力、仮説、または workspace の状態を変更してください。",
			"변경되지 않은 workspace에서 동일한 결정적 도구 오류가 반복되어 실행을 중단했습니다. 다시 시도하기 전에 전제 조건, 입력, 가설 또는 workspace 상태를 변경하세요.",
			"Выполнение остановлено: в неизменённой рабочей области повторилась та же детерминированная ошибка инструмента. Перед новой попыткой измените предпосылку, входные данные, гипотезу или состояние рабочей области.",
		},
		KeyLoopVisibleFlightVerificationRequired: {
			"The current coding action is not completion-ready. If the workspace changed, use Run for a relevant test, build, or static check against the current revision, then review the resulting diff before finalizing. If a mutation failed, correct it first. Do not claim that runtime verification proves the user's semantic requirements; you remain responsible for that judgment.",
			"当前 coding action 尚未达到可完成状态。如果 workspace 已变更，请使用 Run 针对当前 revision 执行相关测试、构建或静态检查，并在最终答复前复核生成的 diff；如果 mutation 失败，请先修正。不要声称 runtime verification 已证明用户的语义要求；这项判断仍由你负责。",
			"Die aktuelle Coding-Aktion ist noch nicht abschlussbereit. Wenn der Workspace geändert wurde, führen Sie mit Run einen relevanten Test, Build oder eine statische Prüfung für die aktuelle Revision aus und prüfen Sie anschließend den Diff. Beheben Sie zuerst eine fehlgeschlagene Mutation. Behaupten Sie nicht, die Runtime-Verifikation beweise die semantischen Anforderungen des Benutzers; für diese Beurteilung bleiben Sie verantwortlich.",
			"現在の coding action は完了可能な状態ではありません。workspace を変更した場合は、Run で現在の revision に対する関連テスト、ビルド、または静的チェックを実行し、最終回答の前に生成された diff を確認してください。mutation が失敗した場合は、まず修正してください。runtime verification がユーザーの意味上の要件を証明したとは主張しないでください。その判断には引き続きあなたが責任を負います。",
			"현재 coding action은 완료 가능한 상태가 아닙니다. workspace가 변경되었다면 Run으로 현재 revision에 대한 관련 테스트, 빌드 또는 정적 검사를 수행하고 최종 답변 전에 생성된 diff를 검토하세요. mutation이 실패했다면 먼저 수정하세요. runtime verification이 사용자의 의미적 요구 사항을 입증했다고 주장하지 마세요. 그 판단은 여전히 당신의 책임입니다.",
			"Текущее coding action ещё не готово к завершению. Если workspace был изменён, выполните через Run подходящий тест, сборку или статическую проверку для текущей revision, а затем просмотрите получившийся diff. Если mutation завершилась неудачно, сначала исправьте её. Не утверждайте, что runtime verification доказывает смысловые требования пользователя: ответственность за эту оценку остаётся за вами.",
		},
	}
	for key, translations := range entries {
		semanticTranslations[key] = map[Language]string{
			LangEN: translations[0],
			LangZH: translations[1],
			LangDE: translations[2],
			LangJA: translations[3],
			LangKO: translations[4],
			LangRU: translations[5],
		}
	}
}
