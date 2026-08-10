package i18n

// Semantic copy for the query-local Agentic V2 correctness flight. Runtime
// disposition, epoch, and blocker identifiers remain untranslated protocol
// values and are never used as display copy.
const (
	KeyLoopQueryFlightStateInvalid              Key = "loop.query.flight_state_invalid"
	KeyLoopQueryFlightRepeatedFailure           Key = "loop.query.flight_repeated_failure"
	KeyLoopVisibleFlightInvestigationNudge      Key = "loop.visible.flight_investigation_nudge"
	KeyLoopVisibleFlightVerificationConvergence Key = "loop.visible.flight_verification_convergence"
	KeyLoopVisibleFlightVerificationRequired    Key = "loop.visible.flight_verification_required"
	KeyLoopVisibleFlightWorkspaceUnknown        Key = "loop.visible.flight_workspace_unknown"
)

var loopFlightKeys = [...]Key{
	KeyLoopQueryFlightStateInvalid,
	KeyLoopQueryFlightRepeatedFailure,
	KeyLoopVisibleFlightInvestigationNudge,
	KeyLoopVisibleFlightVerificationConvergence,
	KeyLoopVisibleFlightVerificationRequired,
	KeyLoopVisibleFlightWorkspaceUnknown,
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
		KeyLoopVisibleFlightInvestigationNudge: {
			"The investigation has already gathered substantial evidence without changing the workspace. Reassess now: if the root cause and smallest complete fix are supported, use ApplyPatch next. Otherwise inspect only the missing decisive evidence; do not repeat broad discovery.",
			"当前调查已收集大量证据，但尚未修改工作区。请立即重新判断：若根因和最小完整修复已有依据，下一步请使用 ApplyPatch；否则只检查缺失的决定性证据，不要重复大范围探索。",
			"Die Untersuchung hat bereits umfangreiche Belege gesammelt, ohne den Workspace zu ändern. Bewerten Sie die Lage jetzt neu: Wenn Ursache und kleinste vollständige Korrektur belegt sind, verwenden Sie als Nächstes ApplyPatch. Andernfalls prüfen Sie nur den fehlenden entscheidenden Beleg und wiederholen Sie keine breit angelegte Suche.",
			"workspace を変更しないまま、調査ではすでに十分な根拠が集まっています。ここで再評価してください。根本原因と最小限の完全な修正に根拠があるなら、次に ApplyPatch を使用してください。そうでなければ、不足している決定的な根拠だけを確認し、広範な探索を繰り返さないでください。",
			"workspace를 변경하지 않은 채 조사에서 이미 충분한 근거를 수집했습니다. 지금 다시 판단하세요. 근본 원인과 최소한의 완전한 수정이 뒷받침된다면 다음 단계에서 ApplyPatch를 사용하세요. 그렇지 않다면 부족한 결정적 근거만 확인하고 광범위한 탐색을 반복하지 마세요.",
			"В ходе исследования уже собрано достаточно доказательств, но рабочая область не изменена. Пересмотрите ситуацию сейчас: если первопричина и минимальное полное исправление обоснованы, следующим шагом используйте ApplyPatch. Иначе проверьте только недостающее решающее доказательство и не повторяйте широкий поиск.",
		},
		KeyLoopVisibleFlightVerificationConvergence: {
			"Verification has already been attempted for the current change. Use its result to converge now: fix a code-related failure, but do not keep probing for unavailable package managers, dependencies, networks, or alternate build tools. If the implementation is complete and the remaining blocker is environmental, finish with that limitation and the evidence already collected.",
			"当前变更已经尝试过验证。请立即依据结果收敛：若失败与代码有关则修复，但不要继续探查不可用的包管理器、依赖、网络或替代构建工具。若实现已完整且剩余阻碍属于环境问题，请带着该限制和已有证据直接完成总结。",
			"Für die aktuelle Änderung wurde bereits eine Verifikation versucht. Führen Sie die Arbeit jetzt anhand des Ergebnisses zum Abschluss: Beheben Sie einen codebedingten Fehler, suchen Sie aber nicht weiter nach nicht verfügbaren Paketmanagern, Abhängigkeiten, Netzwerken oder alternativen Build-Werkzeugen. Ist die Implementierung vollständig und nur noch die Umgebung blockiert, schließen Sie mit dieser Einschränkung und den bereits gesammelten Belegen ab.",
			"現在の変更に対する検証はすでに試行済みです。その結果に基づいてここで収束してください。コードに起因する失敗は修正しますが、利用できないパッケージマネージャー、依存関係、ネットワーク、代替ビルドツールを探し続けないでください。実装が完了しており、残る阻害要因が環境だけなら、その制約と収集済みの根拠を示して完了してください。",
			"현재 변경 사항에 대한 검증은 이미 시도했습니다. 이제 그 결과를 바탕으로 마무리하세요. 코드로 인한 실패는 수정하되, 사용할 수 없는 패키지 관리자, 의존성, 네트워크 또는 대체 빌드 도구를 계속 탐색하지 마세요. 구현이 완료되었고 남은 문제가 환경뿐이라면 해당 제약과 이미 수집한 근거를 밝히고 완료하세요.",
			"Проверка текущего изменения уже была предпринята. Теперь завершите работу с учётом её результата: исправьте ошибку в коде, но не продолжайте искать недоступные менеджеры пакетов, зависимости, сеть или альтернативные средства сборки. Если реализация завершена и остаётся только ограничение среды, закончите работу, указав это ограничение и уже собранные доказательства.",
		},
		KeyLoopVisibleFlightVerificationRequired: {
			"The current coding action is not completion-ready. If the workspace changed, use Run for a relevant test, build, or static check against the current revision, then review the resulting diff before finalizing. If a mutation failed, correct it first. Do not claim that runtime verification proves the user's semantic requirements; you remain responsible for that judgment.",
			"当前 coding action 尚未达到可完成状态。如果 workspace 已变更，请使用 Run 针对当前 revision 执行相关测试、构建或静态检查，并在最终答复前复核生成的 diff；如果 mutation 失败，请先修正。不要声称 runtime verification 已证明用户的语义要求；这项判断仍由你负责。",
			"Die aktuelle Coding-Aktion ist noch nicht abschlussbereit. Wenn der Workspace geändert wurde, führen Sie mit Run einen relevanten Test, Build oder eine statische Prüfung für die aktuelle Revision aus und prüfen Sie anschließend den Diff. Beheben Sie zuerst eine fehlgeschlagene Mutation. Behaupten Sie nicht, die Runtime-Verifikation beweise die semantischen Anforderungen des Benutzers; für diese Beurteilung bleiben Sie verantwortlich.",
			"現在の coding action は完了可能な状態ではありません。workspace を変更した場合は、Run で現在の revision に対する関連テスト、ビルド、または静的チェックを実行し、最終回答の前に生成された diff を確認してください。mutation が失敗した場合は、まず修正してください。runtime verification がユーザーの意味上の要件を証明したとは主張しないでください。その判断には引き続きあなたが責任を負います。",
			"현재 coding action은 완료 가능한 상태가 아닙니다. workspace가 변경되었다면 Run으로 현재 revision에 대한 관련 테스트, 빌드 또는 정적 검사를 수행하고 최종 답변 전에 생성된 diff를 검토하세요. mutation이 실패했다면 먼저 수정하세요. runtime verification이 사용자의 의미적 요구 사항을 입증했다고 주장하지 마세요. 그 판단은 여전히 당신의 책임입니다.",
			"Текущее coding action ещё не готово к завершению. Если workspace был изменён, выполните через Run подходящий тест, сборку или статическую проверку для текущей revision, а затем просмотрите получившийся diff. Если mutation завершилась неудачно, сначала исправьте её. Не утверждайте, что runtime verification доказывает смысловые требования пользователя: ответственность за эту оценку остаётся за вами.",
		},
		KeyLoopVisibleFlightWorkspaceUnknown: {
			"The workspace revision is unsealed, and repeating Run cannot make it completion-ready. Inspect the current diff. Only a real ApplyPatch mutation can establish a new revision receipt; do not submit a no-op patch. If a source change remains, make it with ApplyPatch, then use a read-only Run for a relevant test, build, or static check. If no change remains, stop and report that this query lacks authority to adopt or seal the current state.",
			"当前工作区版本尚未封存，重复运行 Run 无法使其达到可完成状态。请检查当前 diff。只有实际产生变更的 ApplyPatch 才能建立新的版本回执；不要提交空操作补丁。若仍需修改源代码，请通过 ApplyPatch 完成，再用只读 Run 执行相关测试、构建或静态检查。若已无需修改，请停止并报告本次查询缺少采纳或封存当前状态的权限。",
			"Die Workspace-Revision ist nicht versiegelt; eine Wiederholung von Run macht sie nicht abschlussbereit. Prüfen Sie den aktuellen Diff. Nur eine echte Mutation durch ApplyPatch kann einen neuen Revisionsbeleg erstellen; reichen Sie keinen No-op-Patch ein. Falls noch eine Quelländerung nötig ist, führen Sie sie mit ApplyPatch aus und verwenden Sie danach einen schreibgeschützten Run für einen passenden Test, Build oder eine statische Prüfung. Falls keine Änderung mehr nötig ist, stoppen Sie und melden Sie, dass dieser Abfrage die Berechtigung zur Übernahme oder Versiegelung des aktuellen Zustands fehlt.",
			"workspace revision は seal されておらず、Run を繰り返しても完了可能な状態にはなりません。現在の diff を確認してください。新しい revision receipt を作成できるのは、実際に変更を行う ApplyPatch だけです。no-op patch は送信しないでください。ソース変更がまだ必要なら ApplyPatch で行い、その後に読み取り専用 Run で関連するテスト、ビルド、または静的チェックを実行してください。変更が不要なら停止し、この query には現在の状態を adopt または seal する権限がないことを報告してください。",
			"workspace revision이 seal되지 않았으므로 Run을 반복해도 완료 가능한 상태가 되지 않습니다. 현재 diff를 확인하세요. 실제 변경을 수행하는 ApplyPatch만 새 revision receipt를 만들 수 있으므로 no-op patch를 제출하지 마세요. 소스 변경이 남아 있다면 ApplyPatch로 수행한 다음 읽기 전용 Run으로 관련 테스트, 빌드 또는 정적 검사를 실행하세요. 변경이 남아 있지 않다면 중단하고 이 query에 현재 상태를 adopt하거나 seal할 권한이 없다고 보고하세요.",
			"Ревизия рабочей области не запечатана, и повтор Run не сделает её готовой к завершению. Просмотрите текущий diff. Новую квитанцию ревизии может создать только ApplyPatch с реальным изменением; не отправляйте пустой патч. Если ещё требуется изменение исходников, внесите его через ApplyPatch, затем выполните подходящий тест, сборку или статическую проверку с помощью Run только для чтения. Если изменений больше не требуется, остановитесь и сообщите, что у этого запроса нет полномочий принять или запечатать текущее состояние.",
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
