package i18n

const (
	KeyEvidenceObservationHeader    Key = "evidence.observation.header"
	KeyEvidenceInput                Key = "evidence.input"
	KeyEvidenceResultBoundary       Key = "evidence.result.boundary"
	KeyEvidenceStructured           Key = "evidence.structured"
	KeyEvidenceObservationNotFound  Key = "evidence.observation.not_found"
	KeyEvidenceEncodeInputError     Key = "evidence.input.encode_error"
	KeyEvidenceReadResultError      Key = "evidence.result.read_error"
	KeyEvidenceReadStructuredError  Key = "evidence.structured.read_error"
	KeyTranscriptMissingStore       Key = "transcript.export.missing_store"
	KeyTranscriptUnsupportedFormat  Key = "transcript.export.unsupported_format"
	KeyTranscriptObservationHeader  Key = "transcript.observation.header"
	KeyTranscriptStructuredEvidence Key = "transcript.structured_evidence"
	KeyTranscriptPresentationHeader Key = "transcript.presentation.header"
	KeyTranscriptDecisionHeader     Key = "transcript.decision.header"
	KeyTranscriptRoleUser           Key = "transcript.role.user"
	KeyTranscriptRoleAssistant      Key = "transcript.role.assistant"
	KeyTranscriptRoleOther          Key = "transcript.role.other"
)

func init() {
	addEvidence(KeyEvidenceObservationHeader, "Observation: %s\nSession: %s\nTurn: %s\nWork unit: %s\nActor: %s\nTool: %s\nOutcome: %s\n", "观察记录：%s\n会话：%s\n轮次：%s\n工作单元：%s\n执行者：%s\n工具：%s\n结果：%s\n", "Beobachtung: %s\nSitzung: %s\nRunde: %s\nArbeitseinheit: %s\nAkteur: %s\nTool: %s\nErgebnis: %s\n", "観測: %s\nセッション: %s\nターン: %s\n作業単位: %s\n実行者: %s\nツール: %s\n結果: %s\n", "관찰: %s\n세션: %s\n턴: %s\n작업 단위: %s\n실행자: %s\n도구: %s\n결과: %s\n", "Наблюдение: %s\nСеанс: %s\nХод: %s\nРабочая единица: %s\nИсполнитель: %s\nИнструмент: %s\nРезультат: %s\n")
	addEvidence(KeyEvidenceInput, "\nInput:\n%s\n", "\n输入：\n%s\n", "\nEingabe:\n%s\n", "\n入力:\n%s\n", "\n입력:\n%s\n", "\nВход:\n%s\n")
	addEvidence(KeyEvidenceResultBoundary, "\nResult evidence %d begins.\n%s\nResult evidence %d ends.\n", "\n结果证据 %d 开始。\n%s\n结果证据 %d 结束。\n", "\nErgebnisbeleg %d beginnt.\n%s\nErgebnisbeleg %d endet.\n", "\n結果エビデンス %d 開始。\n%s\n結果エビデンス %d 終了。\n", "\n결과 증거 %d 시작.\n%s\n결과 증거 %d 끝.\n", "\nНачало сведений о результате %d.\n%s\nКонец сведений о результате %d.\n")
	addEvidence(KeyEvidenceStructured, "\nStructured evidence %d:\n%s\n", "\n结构化证据 %d：\n%s\n", "\nStrukturierter Beleg %d:\n%s\n", "\n構造化エビデンス %d:\n%s\n", "\n구조화된 증거 %d:\n%s\n", "\nСтруктурированные сведения %d:\n%s\n")
	addEvidence(KeyEvidenceObservationNotFound, "observation %q was not found", "找不到观察记录 %q", "Beobachtung %q wurde nicht gefunden", "観測 %q が見つかりません", "관찰 %q을(를) 찾을 수 없습니다", "Наблюдение %q не найдено")
	addEvidence(KeyEvidenceEncodeInputError, "could not encode observation input", "无法编码观察记录输入", "Beobachtungseingabe konnte nicht kodiert werden", "観測入力をエンコードできませんでした", "관찰 입력을 인코딩할 수 없습니다", "Не удалось закодировать вход наблюдения")
	addEvidence(KeyEvidenceReadResultError, "could not read result evidence %d", "无法读取结果证据 %d", "Ergebnisbeleg %d konnte nicht gelesen werden", "結果エビデンス %d を読み込めませんでした", "결과 증거 %d을(를) 읽을 수 없습니다", "Не удалось прочитать сведения о результате %d")
	addEvidence(KeyEvidenceReadStructuredError, "could not read structured evidence %d", "无法读取结构化证据 %d", "Strukturierter Beleg %d konnte nicht gelesen werden", "構造化エビデンス %d を読み込めませんでした", "구조화된 증거 %d을(를) 읽을 수 없습니다", "Не удалось прочитать структурированные сведения %d")
	addEvidence(KeyTranscriptMissingStore, "transcript export requires an observation store and a detail store", "对话记录导出需要观察记录存储和详情存储", "Für den Transkriptexport werden ein Beobachtungs- und ein Detailspeicher benötigt", "トランスクリプトのエクスポートには観測ストアと詳細ストアが必要です", "대화 기록을 내보내려면 관찰 저장소와 세부 정보 저장소가 필요합니다", "Для экспорта стенограммы нужны хранилища наблюдений и деталей")
	addEvidence(KeyTranscriptUnsupportedFormat, "unsupported transcript export format %q", "不支持对话记录导出格式 %q", "Nicht unterstütztes Transkript-Exportformat %q", "未対応のトランスクリプト出力形式 %q", "지원하지 않는 대화 기록 내보내기 형식 %q", "Неподдерживаемый формат экспорта стенограммы %q")
	addEvidence(KeyTranscriptObservationHeader, "[%s] %s (%s)\nSession: %s\nTurn: %s\nWork: %s\nActor: %s\n", "[%s] %s（%s）\n会话：%s\n轮次：%s\n工作单元：%s\n执行者：%s\n", "[%s] %s (%s)\nSitzung: %s\nRunde: %s\nArbeit: %s\nAkteur: %s\n", "[%s] %s（%s）\nセッション: %s\nターン: %s\n作業: %s\n実行者: %s\n", "[%s] %s (%s)\n세션: %s\n턴: %s\n작업: %s\n실행자: %s\n", "[%s] %s (%s)\nСеанс: %s\nХод: %s\nРабота: %s\nИсполнитель: %s\n")
	addEvidence(KeyTranscriptStructuredEvidence, "Structured evidence: ", "结构化证据：", "Strukturierter Beleg: ", "構造化エビデンス: ", "구조화된 증거: ", "Структурированные сведения: ")
	addEvidence(KeyTranscriptPresentationHeader, "[presentation:%06d] %d\n%s\n", "[展示:%06d] %d\n%s\n", "[Darstellung:%06d] %d\n%s\n", "[表示:%06d] %d\n%s\n", "[표시:%06d] %d\n%s\n", "[представление:%06d] %d\n%s\n")
	addEvidence(KeyTranscriptDecisionHeader, "[decision:%s] %s\nActor: %s\nAction: %s\nTarget: %s\nImpact: %s\nRisk: %s\nRule: %s\nScope: %s\nOutcome: %s\nChoice: %s\n\n", "[决策:%s] %s\n执行者：%s\n操作：%s\n目标：%s\n影响：%s\n风险：%s\n规则：%s\n范围：%s\n结果：%s\n选择：%s\n\n", "[Entscheidung:%s] %s\nAkteur: %s\nAktion: %s\nZiel: %s\nAuswirkung: %s\nRisiko: %s\nRegel: %s\nUmfang: %s\nErgebnis: %s\nAuswahl: %s\n\n", "[決定:%s] %s\n実行者: %s\n操作: %s\n対象: %s\n影響: %s\nリスク: %s\nルール: %s\n範囲: %s\n結果: %s\n選択: %s\n\n", "[결정:%s] %s\n실행자: %s\n작업: %s\n대상: %s\n영향: %s\n위험: %s\n규칙: %s\n범위: %s\n결과: %s\n선택: %s\n\n", "[решение:%s] %s\nИсполнитель: %s\nДействие: %s\nЦель: %s\nВоздействие: %s\nРиск: %s\nПравило: %s\nОбласть: %s\nРезультат: %s\nВыбор: %s\n\n")
	addEvidence(KeyTranscriptRoleUser, "User", "用户", "Benutzer", "ユーザー", "사용자", "Пользователь")
	addEvidence(KeyTranscriptRoleAssistant, "Assistant", "Assistant", "Assistant", "Assistant", "Assistant", "Assistant")
	addEvidence(KeyTranscriptRoleOther, "%s", "%s", "%s", "%s", "%s", "%s")
}

func TranscriptRoleLabel(lang Language, role string) string {
	switch role {
	case "user":
		return Text(lang, KeyTranscriptRoleUser)
	case "assistant":
		return Text(lang, KeyTranscriptRoleAssistant)
	default:
		return Format(lang, KeyTranscriptRoleOther, role)
	}
}

func addEvidence(key Key, en, zh, de, ja, ko, ru string) {
	semanticTranslations[key] = map[Language]string{LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru}
}
