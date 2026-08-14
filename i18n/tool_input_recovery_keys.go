package i18n

const (
	KeyRuntimeToolInputRecoveryRetry            Key = "runtime.tool.input_recovery.retry"
	KeyRuntimeToolInputRecoveryFailed           Key = "runtime.tool.input_recovery.failed"
	KeyRuntimeToolInputRecoveryRepeated         Key = "runtime.tool.input_recovery.repeated"
	KeyRuntimeToolInputRecoveryAbandoned        Key = "runtime.tool.input_recovery.abandoned"
	KeyLoopVisibleToolInputRecovery             Key = "loop.visible.tool_input_recovery"
	KeyLoopVisibleToolInputRecoveryMissingValue Key = "loop.visible.tool_input_recovery.missing_value"
	KeyLoopVisibleToolInputRecoveryAtOffset     Key = "loop.visible.tool_input_recovery.at_offset"
	KeyLoopToolInputRecoveryFailed              Key = "loop.tool_input_recovery.failed"
	KeyLoopToolInputRecoveryRepeated            Key = "loop.tool_input_recovery.repeated"
	KeyLoopToolInputRecoveryAbandoned           Key = "loop.tool_input_recovery.abandoned"
	KeyTUIInvalidToolUse                        Key = "tui.tool.invalid_input"
	KeyThinkingCollapsedHint                    Key = "tui.thinking.collapsed_hint"
)

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}
	add(KeyRuntimeToolInputRecoveryRetry,
		"Invalid tool input for %s; no tool was run. Asking the model to correct it (%d/%d).",
		"工具 %s 的输入无效，未执行任何工具。正在要求模型修正（%d/%d）。",
		"Ungültige Tool-Eingabe für %s; kein Tool wurde ausgeführt. Das Modell wird zur Korrektur aufgefordert (%d/%d).",
		"ツール %s の入力が無効なため、ツールは実行されませんでした。モデルに修正を求めています（%d/%d）。",
		"도구 %s의 입력이 잘못되어 도구를 실행하지 않았습니다. 모델에 수정을 요청합니다(%d/%d).",
		"Некорректный ввод для инструмента %s; инструмент не запускался. Модели предложено исправить ввод (%d/%d).")
	add(KeyRuntimeToolInputRecoveryFailed,
		"Tool input for %s remained invalid after %d correction attempt(s); no tool was run.",
		"工具 %s 的输入在 %d 次修正后仍无效，未执行任何工具。",
		"Die Tool-Eingabe für %s blieb nach %d Korrekturversuch(en) ungültig; kein Tool wurde ausgeführt.",
		"ツール %s の入力は %d 回修正しても無効なままで、ツールは実行されませんでした。",
		"도구 %s의 입력이 %d회 수정 후에도 잘못되어 도구를 실행하지 않았습니다.",
		"Ввод для инструмента %s остался некорректным после %d попыток исправления; инструмент не запускался.")
	add(KeyRuntimeToolInputRecoveryRepeated,
		"The model repeated the same invalid input for %s after %d correction attempt(s); no tool was run.",
		"模型对工具 %s 的输入在 %d 次修正后仍完全相同且无效；未执行任何工具。",
		"Das Modell hat für %s nach %d Korrekturversuch(en) dieselbe ungültige Eingabe wiederholt; kein Tool wurde ausgeführt.",
		"モデルはツール %s に対して %d 回の修正後も同じ無効な入力を繰り返しました。ツールは実行されませんでした。",
		"모델이 도구 %s에 대해 %d회 수정 후에도 동일한 잘못된 입력을 반복했습니다. 도구는 실행되지 않았습니다.",
		"Модель повторила тот же некорректный ввод для инструмента %s после %d попыток исправления; инструмент не запускался.")
	add(KeyRuntimeToolInputRecoveryAbandoned,
		"The model did not return a corrected call for %s; no tool was run and the response was not accepted as complete.",
		"模型没有重新发出对工具 %s 的有效调用；未执行任何工具，也未将该响应视为完成。",
		"Das Modell hat keinen korrigierten Aufruf für %s zurückgegeben; kein Tool wurde ausgeführt und die Antwort wurde nicht als abgeschlossen akzeptiert.",
		"モデルはツール %s の修正済み呼び出しを返しませんでした。ツールは実行されず、この応答は完了として受理されませんでした。",
		"모델이 도구 %s의 수정된 호출을 반환하지 않았습니다. 도구를 실행하지 않았으며 응답을 완료로 인정하지 않았습니다.",
		"Модель не вернула исправленный вызов инструмента %s; инструмент не запускался, а ответ не принят как завершённый.")
	add(KeyLoopVisibleToolInputRecovery,
		"The previous tool call for %s contained invalid input and was not executed. Emit the tool call again with complete valid input. Do not claim that the tool ran or that the task is complete until a valid tool result is returned.",
		"上一次对工具 %s 的调用输入无效，因此没有执行。请使用完整且有效的输入重新发出工具调用。在收到有效工具结果前，不要声称工具已执行或任务已完成。",
		"Der vorherige Aufruf von %s enthielt ungültige Eingaben und wurde nicht ausgeführt. Gib den Tool-Aufruf erneut mit vollständiger, gültiger Eingabe aus. Behaupte erst nach einem gültigen Tool-Ergebnis, dass das Tool ausgeführt oder die Aufgabe abgeschlossen wurde.",
		"直前のツール %s の呼び出しは入力が無効だったため実行されませんでした。完全で有効な入力を使ってツール呼び出しを再度出力してください。有効なツール結果が返るまで、実行済みまたはタスク完了とはしないでください。",
		"이전 도구 %s 호출은 입력이 잘못되어 실행되지 않았습니다. 완전하고 유효한 입력으로 도구 호출을 다시 출력하세요. 유효한 도구 결과가 반환되기 전에는 도구가 실행되었거나 작업이 완료되었다고 하지 마세요.",
		"Предыдущий вызов инструмента %s содержал некорректный ввод и не выполнялся. Повторите вызов с полным корректным вводом. Не утверждайте, что инструмент выполнен или задача завершена, пока не получен корректный результат инструмента.")
	add(KeyLoopVisibleToolInputRecoveryMissingValue,
		"The previous tool call for %s was not executed because field %s was missing a JSON value near byte %d. Regenerate the complete tool call from the current schema; do not copy the previous arguments. Do not claim that the tool ran or that the task is complete until a valid tool result is returned.",
		"上一次对工具 %s 的调用未执行：字段 %s 在第 %d 字节附近缺少 JSON 值。请根据当前工具 schema 重新生成完整调用，不要复制先前的参数。在收到有效工具结果前，不要声称工具已执行或任务已完成。",
		"Der vorherige Aufruf von %s wurde nicht ausgeführt, weil dem Feld %s nahe Byte %d ein JSON-Wert fehlte. Erzeuge den vollständigen Tool-Aufruf anhand des aktuellen Schemas neu und kopiere nicht die vorherigen Argumente. Behaupte erst nach einem gültigen Tool-Ergebnis, dass das Tool ausgeführt oder die Aufgabe abgeschlossen wurde.",
		"直前のツール %s の呼び出しは、フィールド %s の JSON 値がバイト %d 付近で欠けていたため実行されませんでした。現在のスキーマから完全なツール呼び出しを生成し直し、以前の引数をコピーしないでください。有効なツール結果が返るまで、実行済みまたはタスク完了とはしないでください。",
		"이전 도구 %s 호출은 필드 %s의 JSON 값이 %d바이트 부근에서 누락되어 실행되지 않았습니다. 현재 스키마를 기준으로 전체 도구 호출을 다시 생성하고 이전 인수를 복사하지 마세요. 유효한 도구 결과가 반환되기 전에는 도구가 실행되었거나 작업이 완료되었다고 하지 마세요.",
		"Предыдущий вызов инструмента %s не выполнялся: у поля %s отсутствовало JSON-значение около байта %d. Создайте полный вызов заново по текущей схеме и не копируйте прежние аргументы. Не утверждайте, что инструмент выполнен или задача завершена, пока не получен корректный результат.")
	add(KeyLoopVisibleToolInputRecoveryAtOffset,
		"The previous tool call for %s was not executed because its JSON input was malformed near byte %d. Regenerate the complete tool call from the current schema; do not copy the previous arguments. Do not claim that the tool ran or that the task is complete until a valid tool result is returned.",
		"上一次对工具 %s 的调用未执行：其 JSON 输入在第 %d 字节附近格式错误。请根据当前工具 schema 重新生成完整调用，不要复制先前的参数。在收到有效工具结果前，不要声称工具已执行或任务已完成。",
		"Der vorherige Aufruf von %s wurde nicht ausgeführt, weil seine JSON-Eingabe nahe Byte %d fehlerhaft war. Erzeuge den vollständigen Tool-Aufruf anhand des aktuellen Schemas neu und kopiere nicht die vorherigen Argumente. Behaupte erst nach einem gültigen Tool-Ergebnis, dass das Tool ausgeführt oder die Aufgabe abgeschlossen wurde.",
		"直前のツール %s の呼び出しは、JSON 入力がバイト %d 付近で不正だったため実行されませんでした。現在のスキーマから完全なツール呼び出しを生成し直し、以前の引数をコピーしないでください。有効なツール結果が返るまで、実行済みまたはタスク完了とはしないでください。",
		"이전 도구 %s 호출은 JSON 입력이 %d바이트 부근에서 잘못되어 실행되지 않았습니다. 현재 스키마를 기준으로 전체 도구 호출을 다시 생성하고 이전 인수를 복사하지 마세요. 유효한 도구 결과가 반환되기 전에는 도구가 실행되었거나 작업이 완료되었다고 하지 마세요.",
		"Предыдущий вызов инструмента %s не выполнялся: его JSON-ввод был некорректен около байта %d. Создайте полный вызов заново по текущей схеме и не копируйте прежние аргументы. Не утверждайте, что инструмент выполнен или задача завершена, пока не получен корректный результат.")
	add(KeyLoopToolInputRecoveryFailed,
		"Tool input recovery failed for %s after %d correction attempt(s)",
		"工具 %s 的输入在 %d 次修正后仍未恢复",
		"Wiederherstellung der Tool-Eingabe für %s nach %d Korrekturversuch(en) fehlgeschlagen",
		"ツール %s の入力は %d 回修正しても復旧できませんでした",
		"도구 %s의 입력을 %d회 수정한 후에도 복구하지 못했습니다",
		"Не удалось исправить ввод для инструмента %s после %d попыток")
	add(KeyLoopToolInputRecoveryRepeated,
		"The model repeated the same invalid input for %s after %d correction attempt(s)",
		"模型对工具 %s 的输入在 %d 次修正后仍完全相同且无效",
		"Das Modell hat für %s nach %d Korrekturversuch(en) dieselbe ungültige Eingabe wiederholt",
		"モデルはツール %s に対して %d 回の修正後も同じ無効な入力を繰り返しました",
		"모델이 도구 %s에 대해 %d회 수정 후에도 동일한 잘못된 입력을 반복했습니다",
		"Модель повторила тот же некорректный ввод для инструмента %s после %d попыток исправления")
	add(KeyLoopToolInputRecoveryAbandoned,
		"The model did not provide the required corrected call for %s",
		"模型没有按要求重新发出对工具 %s 的有效调用",
		"Das Modell hat den erforderlichen korrigierten Aufruf für %s nicht bereitgestellt",
		"モデルは必要なツール %s の修正済み呼び出しを返しませんでした",
		"모델이 필요한 도구 %s의 수정된 호출을 제공하지 않았습니다",
		"Модель не предоставила требуемый исправленный вызов инструмента %s")
	add(KeyTUIInvalidToolUse,
		"Tool %s was not run: invalid input",
		"工具 %s 未执行：输入无效",
		"Tool %s wurde nicht ausgeführt: ungültige Eingabe",
		"ツール %s は実行されませんでした: 入力が無効です",
		"도구 %s 미실행: 잘못된 입력",
		"Инструмент %s не запускался: некорректный ввод")
	add(KeyThinkingCollapsedHint,
		"Reasoning collapsed · Alt+O to show all",
		"思考内容已收起 · 按 Alt+O 显示全部",
		"Gedankengang eingeklappt · Alt+O zeigt alles",
		"思考内容を折りたたみました · Alt+O ですべて表示",
		"추론 접힘 · Alt+O로 모두 표시",
		"Рассуждение свёрнуто · Alt+O — показать всё")
}
