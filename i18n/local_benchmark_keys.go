package i18n

// Semantic copy for the repository-local Agentic Coding benchmark workflow.
// Agent, model, dataset and protocol identifiers remain untranslated.
const (
	KeyLocalBenchmarkUsage                       Key = "benchmark.local.usage"
	KeyLocalBenchmarkFlagTaskSize                Key = "benchmark.local.flag.task_size"
	KeyLocalBenchmarkFlagWithCodex               Key = "benchmark.local.flag.with_codex"
	KeyLocalBenchmarkFlagResultsRoot             Key = "benchmark.local.flag.results_root"
	KeyLocalBenchmarkFlagAgentTimeout            Key = "benchmark.local.flag.agent_timeout"
	KeyLocalBenchmarkFlagEvaluatorTimeout        Key = "benchmark.local.flag.evaluator_timeout"
	KeyLocalBenchmarkInvalidOptions              Key = "benchmark.local.invalid_options"
	KeyLocalBenchmarkTaskSizeRange               Key = "benchmark.local.task_size_range"
	KeyLocalBenchmarkPreparing                   Key = "benchmark.local.preparing"
	KeyLocalBenchmarkRunningAgent                Key = "benchmark.local.running_agent"
	KeyLocalBenchmarkEvaluating                  Key = "benchmark.local.evaluating"
	KeyLocalBenchmarkCompleted                   Key = "benchmark.local.completed"
	KeyLocalBenchmarkCompletedWithCodex          Key = "benchmark.local.completed_with_codex"
	KeyLocalBenchmarkPartial                     Key = "benchmark.local.partial"
	KeyLocalBenchmarkFailed                      Key = "benchmark.local.failed"
	KeyLocalBenchmarkBaselineRequired            Key = "benchmark.local.baseline_required"
	KeyLocalBenchmarkBaselineIncompatible        Key = "benchmark.local.baseline_incompatible"
	KeyLocalBenchmarkReportTitle                 Key = "benchmark.local.report.title"
	KeyLocalBenchmarkReportSubtitle              Key = "benchmark.local.report.subtitle"
	KeyLocalBenchmarkReportWatermark             Key = "benchmark.local.report.watermark"
	KeyLocalBenchmarkReportConclusion            Key = "benchmark.local.report.conclusion"
	KeyLocalBenchmarkReportMeasured              Key = "benchmark.local.report.measured"
	KeyLocalBenchmarkReportMeasuredAdjudicated   Key = "benchmark.local.report.measured_adjudicated"
	KeyLocalBenchmarkReportAdjudicationNotice    Key = "benchmark.local.report.adjudication_notice"
	KeyLocalBenchmarkReportStatusAdjudicatedPass Key = "benchmark.local.report.status.adjudicated_pass"
	KeyLocalBenchmarkReportSharedPass            Key = "benchmark.local.report.shared_pass"
	KeyLocalBenchmarkReportNoSharedPass          Key = "benchmark.local.report.no_shared_pass"
	KeyLocalBenchmarkReportLLMDefinition         Key = "benchmark.local.report.llm_definition"
	KeyLocalBenchmarkReportCostDefinition        Key = "benchmark.local.report.cost_definition"
	KeyLocalBenchmarkReportSelection             Key = "benchmark.local.report.selection"
	KeyLocalBenchmarkReportPartialCaveat         Key = "benchmark.local.report.partial_caveat"
	KeyLocalBenchmarkReportToolDiagnostic        Key = "benchmark.local.report.tool_diagnostic"
	KeyLocalBenchmarkReportConcurrency           Key = "benchmark.local.report.concurrency"
	KeyLocalBenchmarkReportHistoricalBaseline    Key = "benchmark.local.report.historical_baseline"
	KeyLocalBenchmarkReportBaselineRefreshed     Key = "benchmark.local.report.baseline_refreshed"
	KeyLocalBenchmarkReportBaselineReused        Key = "benchmark.local.report.baseline_reused"
	KeyLocalBenchmarkReportBinaryVersion         Key = "benchmark.local.report.binary_version"
	KeyLocalBenchmarkReportCodexVersion          Key = "benchmark.local.report.codex_version"
	KeyLocalBenchmarkReportGeneratedFrom         Key = "benchmark.local.report.generated_from"
	KeyLocalBenchmarkReportSectionShared         Key = "benchmark.local.report.section.shared"
	KeyLocalBenchmarkCodexReportTitle            Key = "benchmark.local.codex_report.title"
	KeyLocalBenchmarkCodexReportSubtitle         Key = "benchmark.local.codex_report.subtitle"
	KeyLocalBenchmarkCodexReportFrozenNotice     Key = "benchmark.local.codex_report.frozen_notice"
)

var localBenchmarkKeys = []Key{
	KeyLocalBenchmarkUsage, KeyLocalBenchmarkFlagTaskSize, KeyLocalBenchmarkFlagWithCodex, KeyLocalBenchmarkFlagResultsRoot,
	KeyLocalBenchmarkFlagAgentTimeout, KeyLocalBenchmarkFlagEvaluatorTimeout,
	KeyLocalBenchmarkInvalidOptions, KeyLocalBenchmarkTaskSizeRange, KeyLocalBenchmarkPreparing,
	KeyLocalBenchmarkRunningAgent, KeyLocalBenchmarkEvaluating, KeyLocalBenchmarkCompleted, KeyLocalBenchmarkCompletedWithCodex,
	KeyLocalBenchmarkPartial, KeyLocalBenchmarkFailed, KeyLocalBenchmarkBaselineRequired, KeyLocalBenchmarkBaselineIncompatible, KeyLocalBenchmarkReportTitle,
	KeyLocalBenchmarkReportSubtitle, KeyLocalBenchmarkReportWatermark,
	KeyLocalBenchmarkReportConclusion, KeyLocalBenchmarkReportMeasured, KeyLocalBenchmarkReportMeasuredAdjudicated,
	KeyLocalBenchmarkReportAdjudicationNotice, KeyLocalBenchmarkReportStatusAdjudicatedPass,
	KeyLocalBenchmarkReportSharedPass, KeyLocalBenchmarkReportNoSharedPass,
	KeyLocalBenchmarkReportLLMDefinition, KeyLocalBenchmarkReportCostDefinition,
	KeyLocalBenchmarkReportSelection, KeyLocalBenchmarkReportPartialCaveat,
	KeyLocalBenchmarkReportToolDiagnostic, KeyLocalBenchmarkReportConcurrency,
	KeyLocalBenchmarkReportHistoricalBaseline, KeyLocalBenchmarkReportBaselineRefreshed,
	KeyLocalBenchmarkReportBaselineReused, KeyLocalBenchmarkReportBinaryVersion,
	KeyLocalBenchmarkReportCodexVersion, KeyLocalBenchmarkCodexReportTitle,
	KeyLocalBenchmarkCodexReportSubtitle, KeyLocalBenchmarkCodexReportFrozenNotice,
	KeyLocalBenchmarkReportGeneratedFrom,
	KeyLocalBenchmarkReportSectionShared,
}

func init() {
	add := func(key Key, en, zh, de, ja, ko, ru string) {
		semanticTranslations[key] = map[Language]string{
			LangEN: en, LangZH: zh, LangDE: de, LangJA: ja, LangKO: ko, LangRU: ru,
		}
	}
	add(KeyLocalBenchmarkUsage,
		"Usage: benchmark --task-size=N [--with-codex] [--results-root PATH] [--agent-timeout SECONDS] [--evaluator-timeout SECONDS]",
		"用法：benchmark --task-size=N [--with-codex] [--results-root 路径] [--agent-timeout 秒] [--evaluator-timeout 秒]",
		"Aufruf: benchmark --task-size=N [--with-codex] [--results-root PFAD] [--agent-timeout SEKUNDEN] [--evaluator-timeout SEKUNDEN]",
		"使用法: benchmark --task-size=N [--with-codex] [--results-root パス] [--agent-timeout 秒] [--evaluator-timeout 秒]",
		"사용법: benchmark --task-size=N [--with-codex] [--results-root 경로] [--agent-timeout 초] [--evaluator-timeout 초]",
		"Использование: benchmark --task-size=N [--with-codex] [--results-root ПУТЬ] [--agent-timeout СЕКУНДЫ] [--evaluator-timeout СЕКУНДЫ]")
	add(KeyLocalBenchmarkFlagTaskSize, "Number of representative tasks to run.", "要运行的代表性题目数量。", "Anzahl der auszuführenden repräsentativen Aufgaben.", "実行する代表タスク数。", "실행할 대표 작업 수입니다.", "Число запускаемых репрезентативных задач.")
	add(KeyLocalBenchmarkFlagWithCodex, "Run Codex and replace the frozen Codex baseline.", "运行 Codex 并替换已冻结的 Codex 基线。", "Codex ausführen und die fixierte Codex-Basis ersetzen.", "Codex を実行して固定済み Codex ベースラインを置き換えます。", "Codex를 실행하고 고정된 Codex 기준선을 교체합니다.", "Запустить Codex и заменить зафиксированную базовую линию Codex.")
	add(KeyLocalBenchmarkFlagResultsRoot, "Directory that receives date-grouped reports.", "用于保存按日期分组报告的目录。", "Verzeichnis für nach Datum gruppierte Berichte.", "日付別レポートの保存先。", "날짜별 보고서를 저장할 디렉터리입니다.", "Каталог для отчётов, сгруппированных по дате.")
	add(KeyLocalBenchmarkFlagAgentTimeout, "Per-agent task timeout in seconds.", "每个 Agent 每道题的超时秒数。", "Zeitlimit je Agent-Aufgabe in Sekunden.", "Agent ごとのタスク制限時間（秒）。", "Agent별 작업 제한 시간(초)입니다.", "Лимит времени на задачу для каждого агента в секундах.")
	add(KeyLocalBenchmarkFlagEvaluatorTimeout, "Per-evaluation timeout in seconds.", "每次评测的超时秒数。", "Zeitlimit je Auswertung in Sekunden.", "評価ごとの制限時間（秒）。", "평가별 제한 시간(초)입니다.", "Лимит времени на одну проверку в секундах.")
	add(KeyLocalBenchmarkInvalidOptions, "The benchmark options are invalid; use --help for the supported form.", "测评参数无效；请使用 --help 查看支持的形式。", "Die Benchmark-Optionen sind ungültig; --help zeigt die unterstützte Form.", "ベンチマークのオプションが無効です。--help で使用法を確認してください。", "벤치마크 옵션이 잘못되었습니다. --help로 지원 형식을 확인하세요.", "Параметры теста недопустимы; поддерживаемый формат указан в --help.")
	add(KeyLocalBenchmarkTaskSizeRange, "task-size must be between 1 and %d for the frozen representative catalog.", "对于已固化的代表性题库，task-size 必须在 1 到 %d 之间。", "Für den fixierten repräsentativen Katalog muss task-size zwischen 1 und %d liegen.", "固定済み代表カタログでは task-size は 1 から %d の範囲で指定してください。", "고정된 대표 카탈로그에서 task-size는 1에서 %d 사이여야 합니다.", "Для зафиксированного репрезентативного каталога task-size должен быть от 1 до %d.")
	add(KeyLocalBenchmarkPreparing, "Preparing benchmark run: %s", "正在准备测评：%s", "Benchmark-Lauf wird vorbereitet: %s", "ベンチマーク実行を準備中: %s", "벤치마크 실행 준비 중: %s", "Подготовка запуска теста: %s")
	add(KeyLocalBenchmarkRunningAgent, "Running %s on %s", "正在运行 %s：%s", "%s wird für %s ausgeführt", "%s を %s で実行中", "%s 실행 중: %s", "Запуск %s для %s")
	add(KeyLocalBenchmarkEvaluating, "Evaluating %s on %s", "正在评测 %s：%s", "%s wird für %s ausgewertet", "%s を %s で評価中", "%s 평가 중: %s", "Проверка %s для %s")
	add(KeyLocalBenchmarkCompleted, "Benchmark completed; report: %s", "测评完成；报告：%s", "Benchmark abgeschlossen; Bericht: %s", "ベンチマーク完了。レポート: %s", "벤치마크 완료. 보고서: %s", "Тест завершён; отчёт: %s")
	add(KeyLocalBenchmarkCompletedWithCodex,
		"Benchmark completed; comparison report: %s; new frozen Codex report: %s",
		"测评完成；对比报告：%s；新的冻结 Codex 报告：%s",
		"Benchmark abgeschlossen; Vergleichsbericht: %s; neuer fixierter Codex-Bericht: %s",
		"ベンチマーク完了。比較レポート: %s、新しい固定 Codex レポート: %s",
		"벤치마크 완료. 비교 보고서: %s, 새로 고정된 Codex 보고서: %s",
		"Тест завершён; сравнительный отчёт: %s; новый зафиксированный отчёт Codex: %s")
	add(KeyLocalBenchmarkPartial, "Benchmark finished with incomplete evidence; partial report: %s", "测评结束，但证据不完整；部分报告：%s", "Benchmark mit unvollständiger Evidenz beendet; Teilbericht: %s", "ベンチマークは証拠不完全で終了しました。部分レポート: %s", "벤치마크가 불완전한 근거로 종료되었습니다. 부분 보고서: %s", "Тест завершён с неполными данными; частичный отчёт: %s")
	add(KeyLocalBenchmarkFailed, "Benchmark setup failed; diagnostic log: %s", "测评准备失败；诊断日志：%s", "Benchmark-Vorbereitung fehlgeschlagen; Diagnoselog: %s", "ベンチマークの準備に失敗しました。診断ログ: %s", "벤치마크 준비에 실패했습니다. 진단 로그: %s", "Подготовка теста не удалась; диагностический журнал: %s")
	add(KeyLocalBenchmarkBaselineRequired,
		"No frozen Codex baseline is available. Run benchmark once with --with-codex.",
		"当前没有可用的冻结 Codex 基线，请先使用 --with-codex 运行一次 benchmark。",
		"Es ist keine fixierte Codex-Basis verfügbar. Führen Sie benchmark einmal mit --with-codex aus.",
		"固定済み Codex ベースラインがありません。まず --with-codex で benchmark を実行してください。",
		"사용 가능한 고정 Codex 기준선이 없습니다. 먼저 --with-codex로 benchmark를 한 번 실행하세요.",
		"Зафиксированная базовая линия Codex отсутствует. Один раз запустите benchmark с --with-codex.")
	add(KeyLocalBenchmarkBaselineIncompatible,
		"The frozen Codex baseline is missing required coverage or does not match this benchmark configuration; refresh it with --with-codex.",
		"冻结 Codex 基线的题目覆盖不足或与本次测评配置不一致；请使用 --with-codex 刷新。",
		"Die fixierte Codex-Basis deckt die Aufgaben nicht ausreichend ab oder passt nicht zu dieser Benchmark-Konfiguration; aktualisieren Sie sie mit --with-codex.",
		"固定済み Codex ベースラインのタスク範囲が不足しているか、今回の設定と一致しません。--with-codex で更新してください。",
		"고정된 Codex 기준선의 작업 범위가 부족하거나 이번 벤치마크 구성과 일치하지 않습니다. --with-codex로 갱신하세요.",
		"Зафиксированная базовая линия Codex не покрывает нужные задачи или не соответствует конфигурации теста; обновите её с --with-codex.")
	add(KeyLocalBenchmarkReportTitle, "Codex and Luban Agentic Coding comparison", "Codex 与 Luban Agentic Coding 对比", "Agentic-Coding-Vergleich von Codex und Luban", "Codex と Luban の Agentic Coding 比較", "Codex와 Luban Agentic Coding 비교", "Сравнение Agentic Coding: Codex и Luban")
	add(KeyLocalBenchmarkReportSubtitle, "Local comparison on %d representative public tasks, generated from structured evidence.", "基于 %d 道代表性公开题目的本机对比，由结构化证据自动生成。", "Lokaler Vergleich von %d repräsentativen öffentlichen Aufgaben, automatisch aus strukturierten Nachweisen erzeugt.", "%d 件の代表的な公開タスクによるローカル比較を構造化証拠から自動生成しました。", "%d개 대표 공개 작업에 대한 로컬 비교이며 구조화된 근거에서 자동 생성했습니다.", "Локальное сравнение на %d репрезентативных публичных задачах, автоматически собранное из структурированных данных.")
	add(KeyLocalBenchmarkReportWatermark, "UNOFFICIAL LOCAL SAMPLE", "非官方本机样本", "INOFFIZIELLE LOKALE STICHPROBE", "非公式ローカル標本", "비공식 로컬 표본", "НЕОФИЦИАЛЬНАЯ ЛОКАЛЬНАЯ ВЫБОРКА")
	add(KeyLocalBenchmarkReportConclusion, "Measured conclusion", "实测结论", "Gemessenes Fazit", "実測結果", "실측 결론", "Вывод по измерениям")
	add(KeyLocalBenchmarkReportMeasured, "Strict local score: Codex %d/%d; Luban %d/%d. LLM calls: %d versus %d. Task duration: %s versus %s. Comparable estimated cost: %s versus %s.", "本机严格得分：Codex %d/%d，Luban %d/%d。LLM 调用：%d 对 %d。任务耗时：%s 对 %s。可比估算费用：%s 对 %s。", "Strikter lokaler Wert: Codex %d/%d; Luban %d/%d. LLM-Aufrufe: %d gegenüber %d. Aufgabendauer: %s gegenüber %s. Vergleichbare Kostenschätzung: %s gegenüber %s.", "厳密なローカル得点: Codex %d/%d、Luban %d/%d。LLM 呼び出し: %d 対 %d。タスク所要時間: %s 対 %s。比較用推定費用: %s 対 %s。", "엄격한 로컬 점수: Codex %d/%d, Luban %d/%d. LLM 호출: %d 대 %d. 작업 소요 시간: %s 대 %s. 비교 추정 비용: %s 대 %s.", "Строгий локальный результат: Codex %d/%d; Luban %d/%d. Вызовы LLM: %d против %d. Время выполнения задач: %s против %s. Сопоставимая оценка стоимости: %s против %s.")
	add(KeyLocalBenchmarkReportMeasuredAdjudicated, "Adjudicated local score: Codex %d/%d; Luban %d/%d. LLM calls: %d versus %d. Task duration: %s versus %s. Comparable estimated cost: %s versus %s.", "本机裁决后得分：Codex %d/%d，Luban %d/%d。LLM 调用：%d 对 %d。任务耗时：%s 对 %s。可比估算费用：%s 对 %s。", "Lokaler Wert nach Beurteilung: Codex %d/%d; Luban %d/%d. LLM-Aufrufe: %d gegenüber %d. Aufgabendauer: %s gegenüber %s. Vergleichbare Kostenschätzung: %s gegenüber %s.", "裁定後のローカル得点: Codex %d/%d、Luban %d/%d。LLM 呼び出し: %d 対 %d。タスク所要時間: %s 対 %s。比較用推定費用: %s 対 %s。", "판정 반영 로컬 점수: Codex %d/%d, Luban %d/%d. LLM 호출: %d 대 %d. 작업 소요 시간: %s 대 %s. 비교 추정 비용: %s 대 %s.", "Локальный результат после оценки: Codex %d/%d; Luban %d/%d. Вызовы LLM: %d против %d. Время выполнения задач: %s против %s. Сопоставимая оценка стоимости: %s против %s.")
	add(KeyLocalBenchmarkReportAdjudicationNotice,
		"The Codex skim-rs__skim-1044 result is a disclosed diagnostic adjudication: its production patch passes the target test and all 587 regression tests after excluding only the colliding candidate test. The original evaluator failure remains linked and unchanged.",
		"Codex 的 skim-rs__skim-1044 结果采用已披露的诊断裁决：仅排除发生名称冲突的候选测试后，其生产代码补丁通过目标测试及全部 587 个回归测试。原始 evaluator 失败结果保持不变并继续提供链接。",
		"Das Codex-Ergebnis für skim-rs__skim-1044 ist eine offengelegte diagnostische Beurteilung: Nach dem alleinigen Ausschluss des kollidierenden Kandidatentests besteht der Produktionspatch den Zieltest und alle 587 Regressionstests. Der ursprüngliche Evaluator-Fehler bleibt unverändert verlinkt.",
		"Codex の skim-rs__skim-1044 結果は開示済みの診断裁定です。衝突した候補テストだけを除外すると、本番コードパッチは対象テストと 587 件すべての回帰テストに合格します。元の evaluator 失敗結果は変更せずリンクを保持します。",
		"Codex의 skim-rs__skim-1044 결과는 공개된 진단 판정입니다. 이름이 충돌한 후보 테스트만 제외하면 프로덕션 코드 패치가 대상 테스트와 회귀 테스트 587개를 모두 통과합니다. 기존 evaluator 실패 결과는 변경하지 않고 링크를 유지합니다.",
		"Результат Codex для skim-rs__skim-1044 является раскрытой диагностической оценкой: после исключения только конфликтующего кандидатного теста производственный патч проходит целевой тест и все 587 регрессионных тестов. Исходный сбой evaluator остаётся без изменений и доступен по ссылке.")
	add(KeyLocalBenchmarkReportStatusAdjudicatedPass, "Pass (diagnostic adjudication)", "通过（诊断裁决）", "Bestanden (diagnostische Beurteilung)", "合格（診断裁定）", "통과(진단 판정)", "Пройдено (диагностическая оценка)")
	add(KeyLocalBenchmarkReportSharedPass, "Efficiency on the %d tasks passed by both agents; this quality-conditioned slice is not an overall score.", "双方都通过的 %d 道题上的效率对比；这是以质量为条件的切片，不是整体得分。", "Effizienz bei den %d von beiden Agents bestandenen Aufgaben; dieser qualitätsbedingte Ausschnitt ist kein Gesamtwert.", "両 Agent が合格した %d タスクでの効率です。品質条件付きの切片であり、総合得点ではありません。", "두 Agent가 모두 통과한 %d개 작업의 효율입니다. 품질 조건부 구간이며 전체 점수가 아닙니다.", "Эффективность на %d задачах, решённых обоими агентами; этот срез при условии качества не является общей оценкой.")
	add(KeyLocalBenchmarkReportNoSharedPass, "No complete task was passed by both agents, so a quality-conditioned efficiency comparison is unavailable.", "没有双方均通过且证据完整的题目，因此无法进行以质量为条件的效率对比。", "Keine vollständige Aufgabe wurde von beiden Agents bestanden; ein qualitätsbedingter Effizienzvergleich ist daher nicht verfügbar.", "両 Agent がともに合格した完全なタスクがないため、品質条件付き効率比較は利用できません。", "두 Agent가 모두 통과한 완전한 작업이 없어 품질 조건부 효율 비교를 제공할 수 없습니다.", "Нет полной задачи, решённой обоими агентами, поэтому сравнение эффективности при условии качества недоступно.")
	add(KeyLocalBenchmarkReportLLMDefinition, "LLM calls are counted as exact HTTP POST requests to the Responses endpoint; parallel tool calls inside one response do not increase this count.", "LLM 调用按发往 Responses 端点的实际 HTTP POST 次数计数；单次响应中的并行工具调用不会增加该计数。", "LLM-Aufrufe sind exakte HTTP-POST-Anfragen an den Responses-Endpunkt; parallele Tool-Aufrufe innerhalb einer Antwort erhöhen den Wert nicht.", "LLM 呼び出しは Responses endpoint への実際の HTTP POST 数です。1 応答内の並列ツール呼び出しは増分しません。", "LLM 호출은 Responses endpoint로 전송된 실제 HTTP POST 수입니다. 한 응답 안의 병렬 도구 호출은 이 수를 늘리지 않습니다.", "Вызовы LLM считаются как точные HTTP POST на endpoint Responses; параллельные вызовы инструментов внутри одного ответа счётчик не увеличивают.")
	add(KeyLocalBenchmarkReportCostDefinition, "Cost is a same-gateway, non-billing estimate from the frozen token rate card; it excludes local compute and evaluator cost.", "费用是在同一网关下按固化 token 价目表计算的非账单估算；不含本机算力和评测器成本。", "Die Kosten sind eine nicht abrechnungsrelevante Schätzung für dasselbe Gateway anhand der fixierten Token-Preisliste; lokale Rechen- und Auswertungskosten sind ausgeschlossen.", "費用は同一 gateway と固定 token 価格表による非請求用推定で、ローカル計算資源と評価器の費用は含みません。", "비용은 동일 gateway와 고정 token 요금표를 사용한 비청구 추정치이며 로컬 연산 및 평가기 비용은 제외합니다.", "Стоимость — небиллинговая оценка для одного gateway по зафиксированному тарифу токенов; локальные вычисления и проверка не учитываются.")
	add(KeyLocalBenchmarkReportSelection, "The task catalog is the frozen representative subset from SWE-bench-Live/MultiLang; task-size selects its first N preregistered tasks.", "题库是从 SWE-bench-Live/MultiLang 固化的代表性子集；task-size 选择其中预注册顺序的前 N 道题。", "Der Aufgabenkatalog ist die fixierte repräsentative Teilmenge von SWE-bench-Live/MultiLang; task-size wählt die ersten N vorregistrierten Aufgaben.", "タスクカタログは SWE-bench-Live/MultiLang から固定した代表サブセットで、task-size は事前登録順の先頭 N 件を選びます。", "작업 카탈로그는 SWE-bench-Live/MultiLang에서 고정한 대표 하위 집합이며 task-size는 사전 등록 순서의 앞 N개를 선택합니다.", "Каталог задач — зафиксированное репрезентативное подмножество SWE-bench-Live/MultiLang; task-size выбирает первые N заранее зарегистрированных задач.")
	add(KeyLocalBenchmarkReportPartialCaveat, "Some run or evaluation evidence is missing. Missing values remain unavailable and no comparative conclusion is inferred from them.", "部分运行或评测证据缺失；缺失值保持不可用，报告不会据此推断对比结论。", "Einige Lauf- oder Auswertungsnachweise fehlen. Fehlende Werte bleiben nicht verfügbar; daraus wird kein Vergleichsfazit abgeleitet.", "実行または評価の証拠が一部不足しています。欠損値は利用不可のままとし、比較結論には使用しません。", "일부 실행 또는 평가 근거가 누락되었습니다. 누락 값은 사용할 수 없는 상태로 유지하며 비교 결론에 사용하지 않습니다.", "Часть данных запуска или проверки отсутствует. Пропуски остаются недоступными и не используются для сравнительных выводов.")
	add(KeyLocalBenchmarkReportToolDiagnostic, "Agent tool events are retained only for diagnosis because Codex and Luban expose different tool catalogs; the primary efficiency metric is LLM calls.", "由于 Codex 与 Luban 的工具目录不同，Agent 工具事件仅保留用于诊断；主要效率指标是 LLM 调用次数。", "Agent-Tool-Ereignisse dienen wegen unterschiedlicher Tool-Kataloge von Codex und Luban nur der Diagnose; primäre Effizienzmetrik sind LLM-Aufrufe.", "Codex と Luban ではツールカタログが異なるため、Agent ツールイベントは診断専用です。主要な効率指標は LLM 呼び出し回数です。", "Codex와 Luban의 도구 카탈로그가 달라 Agent 도구 이벤트는 진단용으로만 유지합니다. 주요 효율 지표는 LLM 호출 수입니다.", "События инструментов агента сохраняются только для диагностики, поскольку каталоги Codex и Luban различаются; основная метрика эффективности — вызовы LLM.")
	add(KeyLocalBenchmarkReportConcurrency, "Codex and Luban run concurrently within each task pair to shorten the pilot. Task duration therefore includes shared host and gateway contention and is not an isolated latency measurement.", "为缩短 pilot，每道题内的 Codex 与 Luban 会并发运行；因此任务耗时包含共享主机与网关竞争，不能视为隔离环境下的延迟。", "Codex und Luban laufen innerhalb jedes Aufgabenpaars parallel, um den Pilot zu verkürzen. Die Aufgabendauer enthält daher Konkurrenz um Host und Gateway und ist keine isolierte Latenzmessung.", "pilot を短縮するため、各タスクペア内で Codex と Luban を並行実行します。そのためタスク所要時間には共有ホストと gateway の競合が含まれ、隔離レイテンシではありません。", "pilot 시간을 줄이기 위해 각 작업 쌍에서 Codex와 Luban을 동시에 실행합니다. 따라서 작업 소요 시간에는 공유 호스트와 gateway 경합이 포함되며 격리된 지연 측정이 아닙니다.", "Чтобы сократить пилот, Codex и Luban запускаются параллельно внутри каждой пары задач. Поэтому время задачи включает конкуренцию за общий хост и gateway и не является изолированным измерением задержки.")
	add(KeyLocalBenchmarkReportHistoricalBaseline,
		"Only Luban ran in this benchmark. Codex metrics come from the identified frozen historical baseline, so task durations were not observed during the same wall-clock interval.",
		"本次测评只运行 Luban；Codex 指标来自报告中明确标识的冻结历史基线，因此双方任务耗时并非在同一墙钟区间观测。",
		"In diesem Benchmark lief nur Luban. Die Codex-Metriken stammen aus der ausgewiesenen fixierten historischen Basis; die Aufgabendauern wurden daher nicht im selben Zeitfenster beobachtet.",
		"今回のベンチマークで実行したのは Luban のみです。Codex 指標は明記された固定済み履歴ベースラインから取得しており、タスク所要時間は同じ実時間区間で観測されたものではありません。",
		"이번 벤치마크에서는 Luban만 실행했습니다. Codex 지표는 명시된 고정 과거 기준선에서 가져왔으므로 작업 소요 시간은 같은 실제 시간 구간에서 관측된 값이 아닙니다.",
		"В этом тесте запускался только Luban. Метрики Codex взяты из указанной зафиксированной исторической базы, поэтому время задач измерялось не в одном и том же интервале.")
	add(KeyLocalBenchmarkReportBaselineRefreshed,
		"Codex baseline refreshed in this run: %s; source %s; captured %s. This snapshot becomes the default for future comparisons.",
		"本次运行已刷新 Codex 基线：%s；来源 %s；固化时间 %s。该快照将自动成为后续对比的默认基线。",
		"Codex-Basis in diesem Lauf aktualisiert: %s; Quelle %s; erfasst %s. Dieser Snapshot wird automatisch zur Standardbasis künftiger Vergleiche.",
		"今回の実行で Codex ベースラインを更新しました: %s、取得元 %s、固定日時 %s。このスナップショットが今後の比較の既定値になります。",
		"이번 실행에서 Codex 기준선을 갱신했습니다: %s, 출처 %s, 고정 시각 %s. 이 스냅샷이 이후 비교의 기본 기준선이 됩니다.",
		"Базовая линия Codex обновлена в этом запуске: %s; источник %s; время фиксации %s. Этот снимок станет базой по умолчанию для последующих сравнений.")
	add(KeyLocalBenchmarkReportBaselineReused,
		"Codex was not rerun. This report reuses the frozen baseline: %s; source %s; captured %s.",
		"本次未重新运行 Codex；报告复用冻结基线：%s；来源 %s；固化时间 %s。",
		"Codex wurde nicht erneut ausgeführt. Dieser Bericht verwendet die fixierte Basis: %s; Quelle %s; erfasst %s.",
		"Codex は再実行していません。このレポートは固定済みベースライン %s（取得元 %s、固定日時 %s）を再利用しています。",
		"Codex를 다시 실행하지 않았습니다. 이 보고서는 고정된 기준선 %s(출처 %s, 고정 시각 %s)을 재사용합니다.",
		"Codex повторно не запускался. В отчёте повторно используется зафиксированная база: %s; источник %s; время фиксации %s.")
	add(KeyLocalBenchmarkReportBinaryVersion, "Binary version", "二进制版本", "Binärversion", "バイナリ版", "바이너리 버전", "Версия бинарного файла")
	add(KeyLocalBenchmarkReportCodexVersion, "Codex version", "Codex 版本", "Codex-Version", "Codex バージョン", "Codex 버전", "Версия Codex")
	add(KeyLocalBenchmarkCodexReportTitle, "Frozen Codex benchmark baseline", "冻结 Codex 测评基线", "Fixierte Codex-Benchmark-Basis", "固定 Codex ベンチマーク基準", "고정 Codex 벤치마크 기준선", "Зафиксированная базовая линия теста Codex")
	add(KeyLocalBenchmarkCodexReportSubtitle,
		"Reusable Codex evidence from %d representative public tasks.",
		"来自 %d 道代表性公开题目、可复用的 Codex 测评证据。",
		"Wiederverwendbare Codex-Nachweise aus %d repräsentativen öffentlichen Aufgaben.",
		"%d 件の代表的な公開タスクから得た再利用可能な Codex 証拠です。",
		"%d개 대표 공개 작업에서 얻은 재사용 가능한 Codex 근거입니다.",
		"Повторно используемые данные Codex по %d репрезентативным публичным задачам.")
	add(KeyLocalBenchmarkCodexReportFrozenNotice,
		"Frozen baseline for %s from %s, captured at %s. It remains in use until benchmark is run again with --with-codex.",
		"这是 %s 的冻结基线，来源为 %s，固化于 %s；在再次使用 --with-codex 运行 benchmark 前始终生效。",
		"Fixierte Basis für %s aus %s, erfasst am %s. Sie bleibt bis zum nächsten benchmark-Lauf mit --with-codex aktiv.",
		"%s の固定ベースラインです。取得元は %s、固定日時は %s です。benchmark を --with-codex で再実行するまで使用されます。",
		"%s의 고정 기준선이며 출처는 %s, 고정 시각은 %s입니다. benchmark를 --with-codex로 다시 실행할 때까지 사용됩니다.",
		"Зафиксированная база для %s из %s, время фиксации %s. Она используется до следующего запуска benchmark с --with-codex.")
	add(KeyLocalBenchmarkReportGeneratedFrom, "This HTML was generated from %s with the checked-in report template.", "本 HTML 由 %s 与仓库内报告模板自动生成。", "Dieses HTML wurde aus %s mit der eingecheckten Berichtsvorlage erzeugt.", "この HTML は %s とリポジトリ内のレポートテンプレートから生成されました。", "이 HTML은 %s와 저장소의 보고서 템플릿으로 생성되었습니다.", "Этот HTML создан из %s с помощью шаблона отчёта в репозитории.")
	add(KeyLocalBenchmarkReportSectionShared, "Efficiency when both agents pass", "同题双通过时的效率", "Effizienz bei Erfolg beider Agents", "両 Agent 合格時の効率", "두 Agent 모두 통과한 경우의 효율", "Эффективность при успехе обоих агентов")
}
