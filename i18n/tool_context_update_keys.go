package i18n

const (
	KeyToolContextUpdateDescription      Key = "tool.context_update.description"
	KeyToolContextUpdateInputTargetIndex Key = "tool.context_update.input.target_index"
	KeyToolContextUpdateInputTargetTool  Key = "tool.context_update.input.target_tool"
	KeyToolContextUpdateInputAction      Key = "tool.context_update.input.action"
	KeyToolContextUpdateInputReasonCode  Key = "tool.context_update.input.reason_code"
	KeyToolContextUpdateInputConfidence  Key = "tool.context_update.input.confidence"
	KeyToolContextUpdateInvalid          Key = "tool.context_update.invalid"
)

var toolContextUpdateKeys = [...]Key{
	KeyToolContextUpdateDescription, KeyToolContextUpdateInputTargetIndex, KeyToolContextUpdateInputTargetTool, KeyToolContextUpdateInputAction,
	KeyToolContextUpdateInputReasonCode, KeyToolContextUpdateInputConfidence,
	KeyToolContextUpdateInvalid,
}

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru}
	}
	add(KeyToolContextUpdateDescription,
		"After consuming the previous tool-result batch, call this only alongside another tool action, once per assessed non-ContextUpdate result; omit it before a final answer and never add a turn for assessment; classify only because the runtime computes and validates replacements",
		"消费上一批工具结果后，只能在执行另一个工具动作的同时，对每个要评估的非 ContextUpdate 结果调用本工具一次；final answer 前省略，绝不能为评估单独增加一轮；只做分类，替代内容由运行时计算并验证",
		"Rufen Sie dieses Tool nach der Verarbeitung des vorherigen Ergebnisstapels nur zusammen mit einer anderen Toolaktion einmal je bewertetem Nicht-ContextUpdate-Ergebnis auf; lassen Sie es vor einer finalen Antwort weg und fügen Sie nie eine eigene Bewertungsrunde hinzu; klassifizieren Sie nur, die Laufzeit berechnet und prüft Ersetzungen",
		"直前のツール結果群を処理したら、別のツールアクションと同時にのみ、評価する ContextUpdate 以外の結果ごとに本ツールを一度呼び出してください。final answer の前は省略し、評価だけのターンを追加してはいけません。分類だけを行い、置換はランタイムが計算・検証します",
		"이전 도구 결과 묶음을 사용한 뒤 다른 도구 작업과 함께만 평가할 ContextUpdate 외 결과마다 이 도구를 한 번 호출하세요. final answer 전에는 생략하고 평가만을 위한 turn을 추가하지 마세요. 분류만 수행하며 대체는 런타임이 계산하고 검증합니다",
		"После обработки предыдущей группы результатов вызывайте этот инструмент только вместе с другим действием инструмента, по одному разу для каждого оцениваемого результата, кроме ContextUpdate; пропускайте его перед final answer и не добавляйте отдельный ход ради оценки; выполняйте только классификацию, замены вычисляет и проверяет среда")
	add(KeyToolContextUpdateInputTargetIndex,
		"Zero-based position in the complete immediately previous tool-result batch; positions containing ContextUpdate are invalid",
		"在紧邻的上一批完整工具结果中的从 0 开始位置；指向 ContextUpdate 的位置无效",
		"Nullbasierte Position im vollständigen unmittelbar vorherigen Tool-Ergebnisstapel; Positionen mit ContextUpdate sind ungültig",
		"直前の完全なツール結果群における 0 始まりの位置。ContextUpdate を指す位置は無効です",
		"바로 이전의 전체 도구 결과 묶음에서 0부터 시작하는 위치이며 ContextUpdate가 있는 위치는 유효하지 않습니다",
		"Позиция с нуля в полной непосредственно предыдущей группе результатов инструментов; позиции с ContextUpdate недопустимы")
	add(KeyToolContextUpdateInputTargetTool,
		"Exact tool name at target_index, used as a selector cross-check",
		"target_index 位置上的精确工具名，用于交叉校验选择器",
		"Exakter Toolname an target_index zur Gegenprüfung des Selektors",
		"セレクター照合に使う target_index 位置の正確なツール名",
		"선택자 교차 검증에 사용하는 target_index 위치의 정확한 도구 이름",
		"Точное имя инструмента в target_index для перекрёстной проверки селектора")
	add(KeyToolContextUpdateInputAction,
		"Retention action: KEEP failures, diagnostics, and still-needed evidence; REWRITE partial value; INDEX uncertain recoverable value; DROP only a successful result fully superseded by a deterministic receipt",
		"保留动作：失败、诊断信息和仍需使用的证据选 KEEP；部分有价值选 REWRITE；价值不确定但可恢复选 INDEX；只有被确定性回执完全取代的成功结果才选 DROP",
		"Aufbewahrungsaktion: KEEP für Fehler, Diagnosen und weiterhin benötigte Belege; REWRITE für teilweise wertvolle Inhalte; INDEX für unsichere, wiederherstellbare Inhalte; DROP nur für erfolgreiche Ergebnisse, die vollständig durch einen deterministischen Beleg ersetzt wurden",
		"保持アクション：失敗、診断情報、まだ必要な証拠は KEEP、部分的に価値がある場合は REWRITE、不確実でも復元可能な場合は INDEX、決定的な受領情報で完全に置き換えられた成功結果だけを DROP にします",
		"보존 동작: 실패, 진단 정보, 아직 필요한 증거는 KEEP, 일부 가치가 있으면 REWRITE, 불확실하지만 복구 가능하면 INDEX, 결정적 영수증으로 완전히 대체된 성공 결과만 DROP을 사용합니다",
		"Действие хранения: KEEP для ошибок, диагностики и всё ещё нужных доказательств; REWRITE для частично ценных данных; INDEX для неопределённых, но восстанавливаемых данных; DROP только для успешного результата, полностью заменённого детерминированной квитанцией")
	add(KeyToolContextUpdateInputReasonCode,
		"Stable protocol reason code, not free-form prose", "稳定的协议原因码，不使用自由文本", "Stabiler Protokoll-Grundcode, kein Freitext", "自由文ではなく安定したプロトコル理由コード", "자유 형식 문장이 아닌 안정적인 프로토콜 사유 코드", "Стабильный код причины протокола, не произвольный текст")
	add(KeyToolContextUpdateInputConfidence,
		"Confidence from 0 to 1", "0 到 1 的置信度", "Konfidenz von 0 bis 1", "0 から 1 の信頼度", "0에서 1 사이의 신뢰도", "Уверенность от 0 до 1")
	add(KeyToolContextUpdateInvalid,
		"The context update proposal is invalid", "上下文更新建议无效", "Der Kontextaktualisierungsvorschlag ist ungültig", "コンテキスト更新の提案が無効です", "컨텍스트 업데이트 제안이 올바르지 않습니다", "Предложение обновления контекста недействительно")
}
