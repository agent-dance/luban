package i18n

const (
	KeyLLMActivityPreparing                  Key = "tui.llm_activity.preparing"
	KeyLLMActivityWaitingFirstToken          Key = "tui.llm_activity.waiting_first_token"
	KeyLLMActivityThinking                   Key = "tui.llm_activity.thinking"
	KeyLLMActivityGeneratingToolInput        Key = "tui.llm_activity.generating_tool_input"
	KeyLLMActivityGeneratingToolInputGeneric Key = "tui.llm_activity.generating_tool_input_generic"
	KeyLLMActivityToolInputReceived          Key = "tui.llm_activity.tool_input_received"
	KeyLLMActivityRunningTools               Key = "tui.llm_activity.running_tools"
	KeyLLMActivityWaitingAfterTools          Key = "tui.llm_activity.waiting_after_tools"
	KeyLLMActivityGeneratingResponse         Key = "tui.llm_activity.generating_response"
	KeyLLMActivityStageElapsed               Key = "tui.llm_activity.stage_elapsed"
	KeyLLMActivitySlowStage                  Key = "tui.llm_activity.slow_stage"
)

var llmActivityKeys = [...]Key{
	KeyLLMActivityPreparing,
	KeyLLMActivityWaitingFirstToken,
	KeyLLMActivityThinking,
	KeyLLMActivityGeneratingToolInput,
	KeyLLMActivityGeneratingToolInputGeneric,
	KeyLLMActivityToolInputReceived,
	KeyLLMActivityRunningTools,
	KeyLLMActivityWaitingAfterTools,
	KeyLLMActivityGeneratingResponse,
	KeyLLMActivityStageElapsed,
	KeyLLMActivitySlowStage,
}

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}
	add(KeyLLMActivityPreparing,
		"Preparing model request", "准备模型请求", "Modellanfrage wird vorbereitet", "モデルリクエストを準備中", "모델 요청 준비 중", "Подготовка запроса к модели")
	add(KeyLLMActivityWaitingFirstToken,
		"Waiting for first token", "等待首 token", "Warten auf das erste Token", "最初の token を待機中", "첫 token 대기 중", "Ожидание первого token")
	add(KeyLLMActivityThinking,
		"Model is thinking", "模型正在思考", "Modell denkt nach", "モデルが思考中", "모델이 생각 중", "Модель размышляет")
	add(KeyLLMActivityGeneratingToolInput,
		"Generating input for %s", "正在生成 %s 的工具输入", "Eingabe für %s wird erstellt", "%s のツール入力を生成中", "%s 도구 입력 생성 중", "Создание входных данных для %s")
	add(KeyLLMActivityGeneratingToolInputGeneric,
		"Generating tool input", "正在生成工具输入", "Werkzeugeingabe wird erstellt", "ツール入力を生成中", "도구 입력 생성 중", "Создание входных данных инструмента")
	add(KeyLLMActivityToolInputReceived,
		"%s · received %s of tool input", "%s · 已接收工具输入 %s", "%s · %s Werkzeugeingabe empfangen", "%s · ツール入力を %s 受信", "%s · 도구 입력 %s 수신", "%s · получено %s входных данных инструмента")
	add(KeyLLMActivityRunningTools,
		"Running tools", "正在执行工具", "Werkzeuge werden ausgeführt", "ツールを実行中", "도구 실행 중", "Выполнение инструментов")
	add(KeyLLMActivityWaitingAfterTools,
		"Waiting for model after tools", "工具完成，等待模型继续", "Warten auf das Modell nach den Werkzeugen", "ツール完了後のモデルを待機中", "도구 완료 후 모델 대기 중", "Ожидание модели после инструментов")
	add(KeyLLMActivityGeneratingResponse,
		"Generating response", "正在生成答复", "Antwort wird erstellt", "回答を生成中", "응답 생성 중", "Создание ответа")
	add(KeyLLMActivityStageElapsed,
		"stage %s", "阶段 %s", "Phase %s", "段階 %s", "단계 %s", "этап %s")
	add(KeyLLMActivitySlowStage,
		"this stage is taking a while", "此阶段耗时较长", "diese Phase dauert länger", "この段階には時間がかかっています", "이 단계가 오래 걸리고 있습니다", "этот этап занимает много времени")
}
