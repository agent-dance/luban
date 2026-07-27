package i18n

// Semantic copy for the reproducible Agentic Coding HTML report and its
// renderer CLI. Benchmark names, agent/model/provider IDs, protocol values,
// paths, hashes, commands, and raw evidence remain untranslated parameters.
const (
	KeyAgenticReportEyebrow                 Key = "agentic.report.eyebrow"
	KeyAgenticReportTitle                   Key = "agentic.report.title"
	KeyAgenticReportSectionVerdict          Key = "agentic.report.section.verdict"
	KeyAgenticReportSectionMethodology      Key = "agentic.report.section.methodology"
	KeyAgenticReportSectionPublicReference  Key = "agentic.report.section.public_reference"
	KeyAgenticReportSectionExperiment       Key = "agentic.report.section.experiment"
	KeyAgenticReportSectionAggregateResults Key = "agentic.report.section.aggregate_results"
	KeyAgenticReportSectionExecutionOrder   Key = "agentic.report.section.execution_order"
	KeyAgenticReportSectionTaskResults      Key = "agentic.report.section.task_results"
	KeyAgenticReportSectionToolAnalysis     Key = "agentic.report.section.tool_analysis"
	KeyAgenticReportSectionToolWaterfall    Key = "agentic.report.section.tool_waterfall"
	KeyAgenticReportSectionOptimizations    Key = "agentic.report.section.optimizations"
	KeyAgenticReportSectionFailures         Key = "agentic.report.section.failures"
	KeyAgenticReportSectionReproduction     Key = "agentic.report.section.reproduction"
	KeyAgenticReportSectionLimitations      Key = "agentic.report.section.limitations"

	KeyAgenticReportClassFormalLabel           Key = "agentic.report.class.formal.label"
	KeyAgenticReportClassPilotLabel            Key = "agentic.report.class.pilot.label"
	KeyAgenticReportClassDiagnosticLabel       Key = "agentic.report.class.diagnostic.label"
	KeyAgenticReportClassPublicLabel           Key = "agentic.report.class.public.label"
	KeyAgenticReportClassFormalDescription     Key = "agentic.report.class.formal.description"
	KeyAgenticReportClassPilotDescription      Key = "agentic.report.class.pilot.description"
	KeyAgenticReportClassDiagnosticDescription Key = "agentic.report.class.diagnostic.description"
	KeyAgenticReportClassPublicDescription     Key = "agentic.report.class.public.description"
	KeyAgenticReportCoverageMissing            Key = "agentic.report.coverage.missing"
	KeyAgenticReportCoveragePartial            Key = "agentic.report.coverage.partial"
	KeyAgenticReportCoverageComplete           Key = "agentic.report.coverage.complete"
	KeyAgenticReportNotApplicable              Key = "agentic.report.not_applicable"
	KeyAgenticReportStatusPass                 Key = "agentic.report.status.pass"
	KeyAgenticReportStatusFail                 Key = "agentic.report.status.fail"
	KeyAgenticReportStatusUnknown              Key = "agentic.report.status.unknown"
	KeyAgenticReportStatusSatisfied            Key = "agentic.report.status.satisfied"
	KeyAgenticReportStatusNotSatisfied         Key = "agentic.report.status.not_satisfied"

	KeyAgenticReportHeaderBaseline           Key = "agentic.report.header.baseline"
	KeyAgenticReportHeaderContender          Key = "agentic.report.header.contender"
	KeyAgenticReportHeaderEvidenceAsOf       Key = "agentic.report.header.evidence_as_of"
	KeyAgenticReportHeaderAgent              Key = "agentic.report.header.agent"
	KeyAgenticReportHeaderTask               Key = "agentic.report.header.task"
	KeyAgenticReportHeaderRepetition         Key = "agentic.report.header.repetition"
	KeyAgenticReportHeaderStatus             Key = "agentic.report.header.status"
	KeyAgenticReportHeaderScore              Key = "agentic.report.header.score"
	KeyAgenticReportHeaderResult             Key = "agentic.report.header.result"
	KeyAgenticReportHeaderWallTime           Key = "agentic.report.header.wall_time"
	KeyAgenticReportHeaderProviderRounds     Key = "agentic.report.header.provider_rounds"
	KeyAgenticReportHeaderToolBearingRounds  Key = "agentic.report.header.tool_bearing_rounds"
	KeyAgenticReportHeaderToolInvocations    Key = "agentic.report.header.tool_invocations"
	KeyAgenticReportHeaderPhysicalOperations Key = "agentic.report.header.physical_operations"
	KeyAgenticReportHeaderToolErrors         Key = "agentic.report.header.tool_errors"
	KeyAgenticReportHeaderTokens             Key = "agentic.report.header.tokens"
	KeyAgenticReportHeaderCacheHit           Key = "agentic.report.header.cache_hit"
	KeyAgenticReportHeaderCost               Key = "agentic.report.header.cost"
	KeyAgenticReportHeaderEvidence           Key = "agentic.report.header.evidence"
	KeyAgenticReportHeaderBenchmarkVersion   Key = "agentic.report.header.benchmark_version"
	KeyAgenticReportHeaderModelContract      Key = "agentic.report.header.model_contract"
	KeyAgenticReportHeaderBinarySHA          Key = "agentic.report.header.binary_sha"
	KeyAgenticReportHeaderSourceRevision     Key = "agentic.report.header.source_revision"
	KeyAgenticReportHeaderWorkspace          Key = "agentic.report.header.workspace"
	KeyAgenticReportHeaderCoverage           Key = "agentic.report.header.coverage"
	KeyAgenticReportHeaderMetric             Key = "agentic.report.header.metric"
	KeyAgenticReportHeaderMean               Key = "agentic.report.header.mean"
	KeyAgenticReportHeaderPairedDifference   Key = "agentic.report.header.paired_difference"
	KeyAgenticReportHeaderRelativeChange     Key = "agentic.report.header.relative_change"
	KeyAgenticReportHeaderConfidenceInterval Key = "agentic.report.header.confidence_interval"
	KeyAgenticReportHeaderCategory           Key = "agentic.report.header.category"
	KeyAgenticReportHeaderSummary            Key = "agentic.report.header.summary"
	KeyAgenticReportHeaderCalls              Key = "agentic.report.header.calls"
	KeyAgenticReportHeaderDuration           Key = "agentic.report.header.duration"
	KeyAgenticReportHeaderTaskUniverse       Key = "agentic.report.header.task_universe"
	KeyAgenticReportHeaderResources          Key = "agentic.report.header.resources"
	KeyAgenticReportHeaderNetworkPolicy      Key = "agentic.report.header.network_policy"
	KeyAgenticReportHeaderPricing            Key = "agentic.report.header.pricing"
	KeyAgenticReportHeaderOutcome            Key = "agentic.report.header.outcome"

	KeyAgenticReportMetricPassRate              Key = "agentic.report.metric.pass_rate"
	KeyAgenticReportMetricWallTimeSeconds       Key = "agentic.report.metric.wall_time_seconds"
	KeyAgenticReportMetricTrialDurationSeconds  Key = "agentic.report.metric.trial_duration_seconds"
	KeyAgenticReportMetricLLMCallsStarted       Key = "agentic.report.metric.llm_calls_started"
	KeyAgenticReportMetricProviderRounds        Key = "agentic.report.metric.provider_rounds"
	KeyAgenticReportMetricProviderErrors        Key = "agentic.report.metric.provider_errors"
	KeyAgenticReportMetricToolBearingRounds     Key = "agentic.report.metric.tool_bearing_rounds"
	KeyAgenticReportMetricToolInvocations       Key = "agentic.report.metric.tool_invocations"
	KeyAgenticReportMetricPhysicalOperations    Key = "agentic.report.metric.physical_operations"
	KeyAgenticReportMetricNativeEvents          Key = "agentic.report.metric.native_events"
	KeyAgenticReportMetricToolErrors            Key = "agentic.report.metric.tool_errors"
	KeyAgenticReportMetricInputTokens           Key = "agentic.report.metric.input_tokens"
	KeyAgenticReportMetricCachedInputTokens     Key = "agentic.report.metric.cached_input_tokens"
	KeyAgenticReportMetricUncachedInputTokens   Key = "agentic.report.metric.uncached_input_tokens"
	KeyAgenticReportMetricCacheWriteInputTokens Key = "agentic.report.metric.cache_write_input_tokens"
	KeyAgenticReportMetricOutputTokens          Key = "agentic.report.metric.output_tokens"
	KeyAgenticReportMetricReasoningOutputTokens Key = "agentic.report.metric.reasoning_output_tokens"
	KeyAgenticReportMetricTokenWeightedCacheHit Key = "agentic.report.metric.token_weighted_cache_hit"
	KeyAgenticReportMetricRequestCacheHit       Key = "agentic.report.metric.request_cache_hit"
	KeyAgenticReportMetricCatalogCost           Key = "agentic.report.metric.catalog_cost"
	KeyAgenticReportMetricProviderReportedCost  Key = "agentic.report.metric.provider_reported_cost"
	KeyAgenticReportCostProviderNotAvailable    Key = "agentic.report.cost.provider_not_available"

	KeyAgenticReportMethodFormalOnly             Key = "agentic.report.method.formal_only"
	KeyAgenticReportMethodMissingNotZero         Key = "agentic.report.method.missing_not_zero"
	KeyAgenticReportMethodSameModel              Key = "agentic.report.method.same_model"
	KeyAgenticReportMethodSameTasks              Key = "agentic.report.method.same_tasks"
	KeyAgenticReportMethodSameEnvironment        Key = "agentic.report.method.same_environment"
	KeyAgenticReportMethodPairedSchedule         Key = "agentic.report.method.paired_schedule"
	KeyAgenticReportMethodIndependentEvaluation  Key = "agentic.report.method.independent_evaluation"
	KeyAgenticReportMethodMetricUnits            Key = "agentic.report.method.metric_units"
	KeyAgenticReportMethodPublicScoreDistinction Key = "agentic.report.method.public_score_distinction"
	KeyAgenticReportMethodComparativeInference   Key = "agentic.report.method.comparative_inference"
	KeyAgenticReportMethodGatewayEvidence        Key = "agentic.report.method.gateway_evidence"
	KeyAgenticReportMethodPhysicalAuxiliary      Key = "agentic.report.method.physical_auxiliary"
	KeyAgenticReportMethodSingleAgentFairness    Key = "agentic.report.method.single_agent_fairness"
	KeyAgenticReportMethodTransportAccounting    Key = "agentic.report.method.transport_accounting"
	KeyAgenticReportMethodPublicReferenceCost    Key = "agentic.report.method.public_reference_cost"
	KeyAgenticReportMethodStorageDeclaration     Key = "agentic.report.method.storage_declaration"
	KeyAgenticReportCostKnownLowerBound          Key = "agentic.report.cost.known_lower_bound"

	KeyAgenticReportGateClassification      Key = "agentic.report.gate.classification"
	KeyAgenticReportGateFormalScore         Key = "agentic.report.gate.formal_score"
	KeyAgenticReportGateArtifactIntegrity   Key = "agentic.report.gate.artifact_integrity"
	KeyAgenticReportGateScorecardRecomputed Key = "agentic.report.gate.scorecard_recomputed"
	KeyAgenticReportGatePairedSchedule      Key = "agentic.report.gate.paired_schedule"
	KeyAgenticReportGateModelContract       Key = "agentic.report.gate.model_contract"
	KeyAgenticReportGateSingleAgentFairness Key = "agentic.report.gate.single_agent_fairness"
	KeyAgenticReportGateNetworkIsolation    Key = "agentic.report.gate.network_isolation"
	KeyAgenticReportGateOracle              Key = "agentic.report.gate.oracle"
	KeyAgenticReportGateCompleteSpend       Key = "agentic.report.gate.complete_spend"
	KeyAgenticReportGateToolExecution       Key = "agentic.report.gate.tool_execution"
	KeyAgenticReportGateControllerDuration  Key = "agentic.report.gate.controller_duration"
	KeyAgenticReportGateExclusionSymmetry   Key = "agentic.report.gate.exclusion_symmetry"
	KeyAgenticReportGateStorageEvidence     Key = "agentic.report.gate.storage_evidence"
	KeyAgenticReportGateProjectionIntegrity Key = "agentic.report.gate.projection_integrity"
	KeyAgenticReportVerdictExceeds          Key = "agentic.report.verdict.exceeds"
	KeyAgenticReportVerdictNotExceeds       Key = "agentic.report.verdict.not_exceeds"
	KeyAgenticReportVerdictInsufficient     Key = "agentic.report.verdict.insufficient"

	KeyAgenticReportWaterfallHeadersWait       Key = "agentic.report.waterfall.headers_wait"
	KeyAgenticReportWaterfallTTFT              Key = "agentic.report.waterfall.ttft"
	KeyAgenticReportWaterfallStream            Key = "agentic.report.waterfall.stream"
	KeyAgenticReportWaterfallProviderError     Key = "agentic.report.waterfall.provider_error"
	KeyAgenticReportWaterfallToolCriticalPath  Key = "agentic.report.waterfall.tool_critical_path"
	KeyAgenticReportWaterfallOverlappingTotals Key = "agentic.report.waterfall.overlapping_totals"
	KeyAgenticReportWaterfallNoFabrication     Key = "agentic.report.waterfall.no_fabrication"
	KeyAgenticReportWaterfallAgentStart        Key = "agentic.report.waterfall.agent_start"
	KeyAgenticReportWaterfallAgentFinish       Key = "agentic.report.waterfall.agent_finish"
	KeyAgenticReportWaterfallDescription       Key = "agentic.report.waterfall.description"

	KeyAgenticReportOptimizationMechanism      Key = "agentic.report.optimization.mechanism"
	KeyAgenticReportOptimizationValue          Key = "agentic.report.optimization.value"
	KeyAgenticReportOptimizationRisk           Key = "agentic.report.optimization.risk"
	KeyAgenticReportOptimizationImplementation Key = "agentic.report.optimization.implementation"
	KeyAgenticReportOptimizationBefore         Key = "agentic.report.optimization.before"
	KeyAgenticReportOptimizationAfter          Key = "agentic.report.optimization.after"
	KeyAgenticReportOptimizationDesignDefect   Key = "agentic.report.optimization.design_defect"
	KeyAgenticReportOptimizationAttribution    Key = "agentic.report.optimization.attribution_scope"
	KeyAgenticReportOptimizationLayer          Key = "agentic.report.optimization.measurement_layer"
	KeyAgenticReportOptimizationEvidenceGrade  Key = "agentic.report.optimization.evidence_grade"
	KeyAgenticReportOptimizationExpectedEffect Key = "agentic.report.optimization.expected_effect"
	KeyAgenticReportOptimizationObservedEffect Key = "agentic.report.optimization.observed_effect"
	KeyAgenticReportOptimizationConfounders    Key = "agentic.report.optimization.confounders"
	KeyAgenticReportOptimizationAblation       Key = "agentic.report.optimization.ablation"
	KeyAgenticReportAblationMeasured           Key = "agentic.report.ablation.measured"
	KeyAgenticReportAblationNotRun             Key = "agentic.report.ablation.not_run"

	KeyAgenticReportFailureImplementation Key = "agentic.report.failure.implementation"
	KeyAgenticReportFailureIncomplete     Key = "agentic.report.failure.incomplete"
	KeyAgenticReportFailureRegression     Key = "agentic.report.failure.regression"
	KeyAgenticReportFailureValidation     Key = "agentic.report.failure.validation"
	KeyAgenticReportFailureTimeout        Key = "agentic.report.failure.timeout"
	KeyAgenticReportFailureInfrastructure Key = "agentic.report.failure.infrastructure"
	KeyAgenticReportFailureProtocol       Key = "agentic.report.failure.protocol"
	KeyAgenticReportFailureUnknown        Key = "agentic.report.failure.unknown"

	KeyAgenticReportReproductionObject         Key = "agentic.report.reproduction.object"
	KeyAgenticReportReproductionIdentity       Key = "agentic.report.reproduction.identity"
	KeyAgenticReportReproductionSource         Key = "agentic.report.reproduction.source"
	KeyAgenticReportReproductionMissingCommand Key = "agentic.report.reproduction.missing_command"
	KeyAgenticReportReproductionSafety         Key = "agentic.report.reproduction.safety"

	KeyAgenticReportLimitationFrozenCost           Key = "agentic.report.limitation.frozen_cost"
	KeyAgenticReportLimitationIncompatibleEvidence Key = "agentic.report.limitation.incompatible_evidence"

	KeyAgenticReportEmptyPublicReference Key = "agentic.report.empty.public_reference"
	KeyAgenticReportEmptyOptimizations   Key = "agentic.report.empty.optimizations"
	KeyAgenticReportEmptyFailures        Key = "agentic.report.empty.failures"
	KeyAgenticReportEmptyToolStats       Key = "agentic.report.empty.tool_stats"
	KeyAgenticReportDiagnosticWatermark  Key = "agentic.report.diagnostic.watermark"
	KeyAgenticReportDevelopmentWatermark Key = "agentic.report.development.watermark"
	KeyAgenticReportFooter               Key = "agentic.report.footer"
	KeyAgenticReportStatisticsSummary    Key = "agentic.report.statistics.summary"
	KeyAgenticReportPairedSummary        Key = "agentic.report.paired.summary"

	KeyAgenticReportCLIFlagInput  Key = "agentic.report.cli.flag.input"
	KeyAgenticReportCLIFlagOutput Key = "agentic.report.cli.flag.output"
	KeyAgenticReportCLIRequired   Key = "agentic.report.cli.required"
	KeyAgenticReportCLISuccess    Key = "agentic.report.cli.success"
	KeyAgenticReportCLIError      Key = "agentic.report.cli.error"
)

type agenticReportCopyEntry struct {
	key  Key
	copy [6]string
}

var agenticReportCopy = [...]agenticReportCopyEntry{
	{KeyAgenticReportEyebrow, [6]string{
		"Agentic Coding · Reproducible Scorecard",
		"Agentic Coding · 可复现计分卡",
		"Agentic Coding · Reproduzierbare Scorecard",
		"Agentic Coding · 再現可能なスコアカード",
		"Agentic Coding · 재현 가능한 스코어카드",
		"Agentic Coding · Воспроизводимая оценочная карта",
	}},
	{KeyAgenticReportTitle, [6]string{
		"Agentic Coding Benchmark Report",
		"Agentic Coding 测评报告",
		"Agentic-Coding-Benchmarkbericht",
		"Agentic Coding ベンチマークレポート",
		"Agentic Coding 벤치마크 보고서",
		"Отчёт о тестировании Agentic Coding",
	}},
	{KeyAgenticReportSectionVerdict, [6]string{"Evidence-gated conclusion", "证据门控结论", "Evidenzgebundene Schlussfolgerung", "証拠ゲート付き結論", "증거 게이트 결론", "Вывод с проверкой доказательств"}},
	{KeyAgenticReportSectionMethodology, [6]string{"Methodology and evidence hierarchy", "方法与证据分级", "Methodik und Evidenzhierarchie", "方法と証拠階層", "방법론 및 증거 계층", "Методика и иерархия доказательств"}},
	{KeyAgenticReportSectionPublicReference, [6]string{"Published reference scores", "公开参考分数", "Veröffentlichte Referenzwerte", "公開参考スコア", "공개 참고 점수", "Опубликованные справочные оценки"}},
	{KeyAgenticReportSectionExperiment, [6]string{"Experiment contract", "实验契约", "Versuchsvertrag", "実験契約", "실험 계약", "Контракт эксперимента"}},
	{KeyAgenticReportSectionAggregateResults, [6]string{"Aggregate results", "汇总结果", "Aggregierte Ergebnisse", "集計結果", "집계 결과", "Сводные результаты"}},
	{KeyAgenticReportSectionExecutionOrder, [6]string{"Execution-order stratification", "执行顺序分层", "Stratifizierung nach Ausführungsreihenfolge", "実行順序別の層化", "실행 순서 층화", "Стратификация по порядку выполнения"}},
	{KeyAgenticReportSectionTaskResults, [6]string{"Task-level results", "逐题结果", "Ergebnisse je Aufgabe", "タスク別結果", "작업별 결과", "Результаты по заданиям"}},
	{KeyAgenticReportSectionToolAnalysis, [6]string{"Tool-call analysis", "工具调用分析", "Analyse der Tool-Aufrufe", "ツール呼び出し分析", "도구 호출 분석", "Анализ вызовов инструментов"}},
	{KeyAgenticReportSectionToolWaterfall, [6]string{"Provider and tool waterfall", "Provider 与工具瀑布", "Provider- und Tool-Wasserfall", "Provider とツールのウォーターフォール", "Provider 및 도구 워터폴", "Водопад Provider и инструментов"}},
	{KeyAgenticReportSectionOptimizations, [6]string{"Optimizations and ablations", "关键优化与消融", "Optimierungen und Ablationen", "最適化とアブレーション", "최적화 및 절제 실험", "Оптимизации и абляции"}},
	{KeyAgenticReportSectionFailures, [6]string{"Failure analysis", "失败分析", "Fehleranalyse", "失敗分析", "실패 분석", "Анализ сбоев"}},
	{KeyAgenticReportSectionReproduction, [6]string{"Reproduction and integrity", "复现与完整性", "Reproduktion und Integrität", "再現と完全性", "재현 및 무결성", "Воспроизведение и целостность"}},
	{KeyAgenticReportSectionLimitations, [6]string{"Limitations", "限制与边界", "Einschränkungen", "制約事項", "제한 사항", "Ограничения"}},

	{KeyAgenticReportClassFormalLabel, [6]string{"Formal", "正式全量", "Formell", "正式", "정식", "Формальный"}},
	{KeyAgenticReportClassPilotLabel, [6]string{"Pilot", "Pilot 子集", "Pilot", "Pilot", "Pilot", "Пилот"}},
	{KeyAgenticReportClassDiagnosticLabel, [6]string{"Diagnostic", "诊断 Canary", "Diagnose", "診断 Canary", "진단 Canary", "Диагностика"}},
	{KeyAgenticReportClassPublicLabel, [6]string{"Published reference", "公开参考", "Veröffentlichte Referenz", "公開参考", "공개 참고", "Опубликованный ориентир"}},
	{KeyAgenticReportClassFormalDescription, [6]string{
		"The only evidence class eligible for the final score; it requires the full selection, complete pairing, immutable artifacts, and a complete ledger.",
		"唯一可用于最终计分的证据等级；必须覆盖全量题集、完整配对、不可变工件和完整 ledger。",
		"Die einzige für die Endwertung zugelassene Evidenzklasse; erforderlich sind die vollständige Auswahl, vollständige Paarung, unveränderliche Artefakte und ein vollständiges Ledger.",
		"最終スコアに使用できる唯一の証拠クラスです。全選択、完全なペアリング、不変の成果物、完全な ledger が必要です。",
		"최종 점수에 사용할 수 있는 유일한 증거 등급입니다. 전체 선택, 완전한 페어링, 변경 불가능한 산출물, 완전한 ledger가 필요합니다.",
		"Единственный класс доказательств, пригодный для итоговой оценки; требуются полная выборка, полное попарное сопоставление, неизменяемые артефакты и полный ledger.",
	}},
	{KeyAgenticReportClassPilotDescription, [6]string{
		"Uses the formal protocol on a preregistered subset, but is not a published leaderboard score.",
		"在预注册子集上遵守正式协议，但不能作为公开榜单分数。",
		"Verwendet das formelle Protokoll für eine vorab registrierte Teilmenge, ist aber kein veröffentlichter Bestenlistenwert.",
		"事前登録したサブセットで正式プロトコルを使用しますが、公開リーダーボードのスコアではありません。",
		"사전 등록된 하위 집합에 정식 프로토콜을 적용하지만 공개 리더보드 점수는 아닙니다.",
		"Использует формальный протокол на заранее зарегистрированном подмножестве, но не является опубликованной оценкой рейтинга.",
	}},
	{KeyAgenticReportClassDiagnosticDescription, [6]string{
		"Useful for locating mechanisms only; it is excluded from final scores, confidence intervals, and winner claims.",
		"仅用于定位机制；不得计入最终分数、置信区间或胜出结论。",
		"Nur zur Ursachenlokalisierung geeignet; von Endwerten, Konfidenzintervallen und Siegbehauptungen ausgeschlossen.",
		"メカニズムの特定にのみ使用し、最終スコア、信頼区間、勝利の主張からは除外します。",
		"메커니즘 파악에만 사용하며 최종 점수, 신뢰 구간, 승자 주장에서는 제외합니다.",
		"Используется только для поиска механизмов; исключается из итоговых оценок, доверительных интервалов и заявлений о победе.",
	}},
	{KeyAgenticReportClassPublicDescription, [6]string{
		"Reported under the publisher's methodology; without task-level protocol equivalence it is contextual evidence, not a paired comparison.",
		"采用发布方口径；若无逐题协议等价证据，只能作为背景参考，不能进行 paired comparison。",
		"Nach der Methodik des Herausgebers berichtet; ohne Protokollgleichheit je Aufgabe ist dies Kontext und kein gepaarter Vergleich.",
		"公開元の方法で報告された値です。タスク単位のプロトコル同等性がなければ、ペア比較ではなく参考情報です。",
		"게시자의 방법론으로 보고된 값입니다. 작업별 프로토콜 동등성이 없으면 페어 비교가 아닌 참고 증거입니다.",
		"Приведено по методике издателя; без эквивалентности протокола на уровне заданий это контекст, а не попарное сравнение.",
	}},
	{KeyAgenticReportCoverageMissing, [6]string{"Not provided", "未提供", "Nicht angegeben", "未提供", "제공되지 않음", "Не предоставлено"}},
	{KeyAgenticReportCoveragePartial, [6]string{"Partial coverage", "部分覆盖", "Teilweise Abdeckung", "部分的なカバレッジ", "부분 커버리지", "Частичное покрытие"}},
	{KeyAgenticReportCoverageComplete, [6]string{"Complete coverage", "完整覆盖", "Vollständige Abdeckung", "完全なカバレッジ", "완전한 커버리지", "Полное покрытие"}},
	{KeyAgenticReportNotApplicable, [6]string{"Not applicable", "不适用", "Nicht anwendbar", "該当なし", "해당 없음", "Не применимо"}},
	{KeyAgenticReportStatusPass, [6]string{"Pass", "通过", "Bestanden", "成功", "통과", "Пройдено"}},
	{KeyAgenticReportStatusFail, [6]string{"Fail", "失败", "Fehlgeschlagen", "失敗", "실패", "Не пройдено"}},
	{KeyAgenticReportStatusUnknown, [6]string{"Unknown", "未知", "Unbekannt", "不明", "알 수 없음", "Неизвестно"}},
	{KeyAgenticReportStatusSatisfied, [6]string{"Satisfied", "已满足", "Erfüllt", "満たす", "충족", "Выполнено"}},
	{KeyAgenticReportStatusNotSatisfied, [6]string{"Not satisfied", "未满足", "Nicht erfüllt", "満たさない", "미충족", "Не выполнено"}},

	{KeyAgenticReportHeaderBaseline, [6]string{"Baseline", "基线", "Baseline", "ベースライン", "기준선", "Базовый вариант"}},
	{KeyAgenticReportHeaderContender, [6]string{"Contender", "挑战者", "Kandidat", "比較対象", "도전자", "Претендент"}},
	{KeyAgenticReportHeaderEvidenceAsOf, [6]string{"Evidence as of", "证据截至", "Evidenzstand", "証拠の基準日時", "증거 기준 시각", "Данные по состоянию на"}},
	{KeyAgenticReportHeaderAgent, [6]string{"Agent", "Agent", "Agent", "Agent", "Agent", "Agent"}},
	{KeyAgenticReportHeaderTask, [6]string{"Task", "题目", "Aufgabe", "タスク", "작업", "Задание"}},
	{KeyAgenticReportHeaderRepetition, [6]string{"Run", "重复轮次", "Lauf", "実行回", "실행", "Запуск"}},
	{KeyAgenticReportHeaderStatus, [6]string{"Status", "状态", "Status", "状態", "상태", "Статус"}},
	{KeyAgenticReportHeaderScore, [6]string{"Score", "分数", "Punktzahl", "スコア", "점수", "Оценка"}},
	{KeyAgenticReportHeaderResult, [6]string{"Result", "结果", "Ergebnis", "結果", "결과", "Результат"}},
	{KeyAgenticReportHeaderWallTime, [6]string{"Wall time", "墙钟耗时", "Wandzeit", "実時間", "경과 시간", "Фактическое время"}},
	{KeyAgenticReportHeaderProviderRounds, [6]string{"Successful provider rounds", "成功 Provider rounds", "Erfolgreiche Provider-Runden", "成功した Provider ラウンド", "성공한 Provider 라운드", "Успешные раунды Provider"}},
	{KeyAgenticReportHeaderToolBearingRounds, [6]string{"Tool-bearing rounds", "含工具 rounds", "Runden mit Tools", "ツールを含むラウンド", "도구 포함 라운드", "Раунды с инструментами"}},
	{KeyAgenticReportHeaderToolInvocations, [6]string{"Provider-committed logical tool calls", "Provider 已提交逻辑工具调用", "Vom Provider bestätigte logische Tool-Aufrufe", "Provider が確定した論理ツール呼び出し", "Provider 확정 논리 도구 호출", "Зафиксированные Provider логические вызовы инструментов"}},
	{KeyAgenticReportHeaderPhysicalOperations, [6]string{"Adapter-reported operations (mixed legacy)", "Adapter 上报操作（混合旧口径）", "Vom Adapter gemeldete Operationen (gemischte Altmetrik)", "Adapter 報告オペレーション（旧方式混在）", "Adapter 보고 작업(혼합 레거시)", "Операции по данным Adapter (смешанная устаревшая метрика)"}},
	{KeyAgenticReportHeaderToolErrors, [6]string{"Observed tool errors", "已观察工具错误", "Beobachtete Tool-Fehler", "観測されたツールエラー", "관측된 도구 오류", "Наблюдаемые ошибки инструментов"}},
	{KeyAgenticReportHeaderTokens, [6]string{"Input / cached / output / reasoning tokens", "Input / cached / output / reasoning tokens", "Input-/Cached-/Output-/Reasoning-Tokens", "Input / cached / output / reasoning tokens", "Input / cached / output / reasoning tokens", "Токены input / cached / output / reasoning"}},
	{KeyAgenticReportHeaderCacheHit, [6]string{"Cache hit rate", "缓存命中率", "Cache-Trefferrate", "キャッシュヒット率", "캐시 적중률", "Доля попаданий в кэш"}},
	{KeyAgenticReportHeaderCost, [6]string{"Cost", "费用", "Kosten", "費用", "비용", "Стоимость"}},
	{KeyAgenticReportHeaderEvidence, [6]string{"Evidence", "证据", "Evidenz", "証拠", "증거", "Доказательства"}},
	{KeyAgenticReportHeaderBenchmarkVersion, [6]string{"Benchmark / version", "基准 / 版本", "Benchmark / Version", "ベンチマーク / バージョン", "벤치마크 / 버전", "Бенчмарк / версия"}},
	{KeyAgenticReportHeaderModelContract, [6]string{"Provider / model / effort", "Provider / 模型 / 推理强度", "Provider / Modell / Reasoning-Stufe", "Provider / モデル / 推論強度", "Provider / 모델 / 추론 강도", "Provider / модель / уровень рассуждений"}},
	{KeyAgenticReportHeaderBinarySHA, [6]string{"Binary SHA-256", "二进制 SHA-256", "Binärdatei-SHA-256", "バイナリ SHA-256", "바이너리 SHA-256", "SHA-256 бинарного файла"}},
	{KeyAgenticReportHeaderSourceRevision, [6]string{"Source revision", "源码版本", "Quellcode-Revision", "ソースリビジョン", "소스 리비전", "Ревизия исходного кода"}},
	{KeyAgenticReportHeaderWorkspace, [6]string{"Workspace", "工作区", "Arbeitsbereich", "ワークスペース", "워크스페이스", "Рабочая область"}},
	{KeyAgenticReportHeaderCoverage, [6]string{"Coverage", "覆盖率", "Abdeckung", "カバレッジ", "커버리지", "Покрытие"}},
	{KeyAgenticReportHeaderMetric, [6]string{"Metric", "指标", "Metrik", "指標", "지표", "Метрика"}},
	{KeyAgenticReportHeaderMean, [6]string{"Mean", "均值", "Mittelwert", "平均", "평균", "Среднее"}},
	{KeyAgenticReportHeaderPairedDifference, [6]string{"Paired difference", "配对差", "Gepaarte Differenz", "ペア差", "대응 차이", "Парная разница"}},
	{KeyAgenticReportHeaderRelativeChange, [6]string{"Relative change", "相对变化", "Relative Änderung", "相対変化", "상대 변화", "Относительное изменение"}},
	{KeyAgenticReportHeaderConfidenceInterval, [6]string{"Confidence interval", "置信区间", "Konfidenzintervall", "信頼区間", "신뢰 구간", "Доверительный интервал"}},
	{KeyAgenticReportHeaderCategory, [6]string{"Category", "类别", "Kategorie", "カテゴリ", "범주", "Категория"}},
	{KeyAgenticReportHeaderSummary, [6]string{"Summary", "摘要", "Zusammenfassung", "要約", "요약", "Резюме"}},
	{KeyAgenticReportHeaderCalls, [6]string{"Calls", "调用次数", "Aufrufe", "呼び出し回数", "호출 수", "Вызовы"}},
	{KeyAgenticReportHeaderDuration, [6]string{"Duration", "耗时", "Dauer", "所要時間", "소요 시간", "Длительность"}},
	{KeyAgenticReportHeaderTaskUniverse, [6]string{"Task universe", "题目全集", "Aufgabengrundgesamtheit", "タスク母集団", "전체 작업 집합", "Совокупность заданий"}},
	{KeyAgenticReportHeaderResources, [6]string{"Resources", "资源", "Ressourcen", "リソース", "리소스", "Ресурсы"}},
	{KeyAgenticReportHeaderNetworkPolicy, [6]string{"Network policy", "网络策略", "Netzwerkrichtlinie", "ネットワークポリシー", "네트워크 정책", "Сетевая политика"}},
	{KeyAgenticReportHeaderPricing, [6]string{"Pricing", "定价", "Preisgrundlage", "価格設定", "요금", "Тарифы"}},
	{KeyAgenticReportHeaderOutcome, [6]string{"Outcome", "结果", "Ergebnis", "結果", "결과", "Исход"}},

	{KeyAgenticReportMetricPassRate, [6]string{"Task pass rate", "任务通过率", "Aufgaben-Erfolgsquote", "タスク成功率", "작업 통과율", "Доля пройденных заданий"}},
	{KeyAgenticReportMetricWallTimeSeconds, [6]string{"Agent wall time", "Agent 墙钟耗时", "Agent-Wandzeit", "Agent 実時間", "Agent 경과 시간", "Фактическое время Agent"}},
	{KeyAgenticReportMetricTrialDurationSeconds, [6]string{"End-to-end trial time", "端到端试验耗时", "End-to-End-Versuchszeit", "エンドツーエンド試行時間", "엔드투엔드 평가 시간", "Сквозное время испытания"}},
	{KeyAgenticReportMetricLLMCallsStarted, [6]string{"LLM calls (all started generate=true inference)", "LLM 调用（全部已开始且 generate=true 的推理）", "LLM-Aufrufe (alle gestarteten Inferenzen mit generate=true)", "LLM 呼び出し（開始済み generate=true 推論の全件）", "LLM 호출(시작된 모든 generate=true 추론)", "Вызовы LLM (все начатые инференсы с generate=true)"}},
	{KeyAgenticReportMetricProviderRounds, [6]string{"Successful Provider rounds", "成功 Provider 轮次", "Erfolgreiche Provider-Runden", "成功した Provider ラウンド", "성공한 Provider 라운드", "Успешные раунды Provider"}},
	{KeyAgenticReportMetricProviderErrors, [6]string{"Failed Provider requests", "失败 Provider 请求", "Fehlgeschlagene Provider-Anfragen", "失敗した Provider リクエスト", "실패한 Provider 요청", "Неудачные запросы Provider"}},
	{KeyAgenticReportMetricToolBearingRounds, [6]string{"Tool-bearing rounds", "含工具轮次", "Runden mit Tools", "ツールを含むラウンド", "도구 포함 라운드", "Раунды с инструментами"}},
	{KeyAgenticReportMetricToolInvocations, [6]string{"Provider-committed logical tool calls", "Provider 已提交逻辑工具调用", "Vom Provider bestätigte logische Tool-Aufrufe", "Provider が確定した論理ツール呼び出し", "Provider 확정 논리 도구 호출", "Зафиксированные Provider логические вызовы инструментов"}},
	{KeyAgenticReportMetricPhysicalOperations, [6]string{"Adapter-reported operations (mixed legacy)", "Adapter 上报操作（混合旧口径）", "Vom Adapter gemeldete Operationen (gemischte Altmetrik)", "Adapter 報告オペレーション（旧方式混在）", "Adapter 보고 작업(혼합 레거시)", "Операции по данным Adapter (смешанная устаревшая метрика)"}},
	{KeyAgenticReportMetricNativeEvents, [6]string{"Native CLI events", "CLI 原生事件", "Native CLI-Ereignisse", "CLI ネイティブイベント", "CLI 네이티브 이벤트", "Нативные события CLI"}},
	{KeyAgenticReportMetricToolErrors, [6]string{"Observed tool errors", "已观察工具错误", "Beobachtete Tool-Fehler", "観測されたツールエラー", "관측된 도구 오류", "Наблюдаемые ошибки инструментов"}},
	{KeyAgenticReportMetricInputTokens, [6]string{"Input tokens", "输入 tokens", "Input-Tokens", "入力 tokens", "입력 tokens", "Входные токены"}},
	{KeyAgenticReportMetricCachedInputTokens, [6]string{"Cached input tokens", "缓存输入 tokens", "Gecachte Input-Tokens", "キャッシュ済み入力 tokens", "캐시된 입력 tokens", "Кэшированные входные токены"}},
	{KeyAgenticReportMetricUncachedInputTokens, [6]string{"All-transport ordinary uncached input tokens (I−C−W)", "全部传输的普通未缓存输入 tokens（I−C−W）", "Gewöhnliche ungecachete Input-Tokens aller Transporte (I−C−W)", "全伝送の通常未キャッシュ入力 tokens（I−C−W）", "전체 전송의 일반 미캐시 입력 tokens(I−C−W)", "Обычные некэшированные входные токены всех транспортных попыток (I−C−W)"}},
	{KeyAgenticReportMetricCacheWriteInputTokens, [6]string{"Cache-write input tokens", "缓存写入输入 tokens", "Cache-Write-Input-Tokens", "キャッシュ書き込み入力 tokens", "캐시 쓰기 입력 tokens", "Входные токены записи в кэш"}},
	{KeyAgenticReportMetricOutputTokens, [6]string{"Output tokens", "输出 tokens", "Output-Tokens", "出力 tokens", "출력 tokens", "Выходные токены"}},
	{KeyAgenticReportMetricReasoningOutputTokens, [6]string{"Reasoning output tokens", "推理输出 tokens", "Reasoning-Output-Tokens", "推論出力 tokens", "추론 출력 tokens", "Токены вывода рассуждений"}},
	{KeyAgenticReportMetricTokenWeightedCacheHit, [6]string{"Token-weighted cache hit rate", "按 token 加权的缓存命中率", "Tokengewichtete Cache-Trefferrate", "token 加重キャッシュヒット率", "token 가중 캐시 적중률", "Взвешенная по токенам доля попаданий в кэш"}},
	{KeyAgenticReportMetricRequestCacheHit, [6]string{"Request cache hit rate", "请求缓存命中率", "Cache-Trefferrate je Anfrage", "リクエストキャッシュヒット率", "요청 캐시 적중률", "Доля запросов с попаданием в кэш"}},
	{KeyAgenticReportMetricCatalogCost, [6]string{"Frozen gateway-comparable rate-card estimate", "冻结的网关可比费率卡估算", "Fixierte gateway-vergleichbare Rate-Card-Schätzung", "固定 gateway 比較用 rate-card 見積もり", "고정 gateway 비교 rate-card 추정치", "Оценка по фиксированной сопоставимой для gateway тарифной карте"}},
	{KeyAgenticReportMetricProviderReportedCost, [6]string{"Provider-reported cost", "Provider 报告费用", "Vom Provider gemeldete Kosten", "Provider 報告費用", "Provider 보고 비용", "Стоимость по данным Provider"}},
	{KeyAgenticReportCostProviderNotAvailable, [6]string{"N/A — gateway does not emit per-response cost; excluded from verdict", "不适用 — 网关不提供逐响应费用；不纳入结论", "N/V — das Gateway gibt keine Kosten pro Antwort aus; von der Bewertung ausgeschlossen", "該当なし — gateway は応答ごとの費用を出力しないため、判定から除外", "해당 없음 — gateway가 응답별 비용을 내보내지 않으므로 판정에서 제외", "Н/Д — gateway не выдаёт стоимость каждого ответа; исключено из вывода"}},

	{KeyAgenticReportMethodFormalOnly, [6]string{
		"Only a complete local formal run is eligible for final scoring.",
		"只有本地完整正式测评可进入最终计分。",
		"Nur ein vollständiger lokaler formeller Lauf ist für die Endwertung zugelassen.",
		"ローカルで完了した正式実行のみが最終スコアの対象です。",
		"로컬에서 완료된 정식 실행만 최종 점수에 포함됩니다.",
		"К итоговой оценке допускается только полный локальный формальный запуск.",
	}},
	{KeyAgenticReportMethodMissingNotZero, [6]string{
		"Missing observations remain unknown and are never treated as zero.",
		"缺失观测始终保持未知，绝不按 0 处理。",
		"Fehlende Beobachtungen bleiben unbekannt und werden nie als null behandelt.",
		"欠測値は不明のまま扱い、0 とみなしません。",
		"누락된 관측값은 알 수 없음으로 유지하며 0으로 처리하지 않습니다.",
		"Отсутствующие наблюдения остаются неизвестными и никогда не считаются нулевыми.",
	}},
	{KeyAgenticReportMethodSameModel, [6]string{
		"Both agents must use the same pinned model, reasoning effort, and effective default service tier, with fallback rejected. Wire representation is disclosed separately: Codex 0.145 uses a source-proven client-canonicalized default, while Luban emits an explicit default; this is semantic parity, not byte-identical request JSON.",
		"两个 Agent 必须使用相同的固定模型、推理强度和有效 default service tier，并拒绝 fallback。wire 表示单独披露：Codex 0.145 使用源码证明的客户端规范化 default，Luban 显式发送 default；这是语义一致，不代表请求 JSON 字节相同。",
		"Beide Agents müssen dasselbe fixierte Modell, dieselbe Reasoning-Stufe und denselben effektiven Default-Service-Tier verwenden; Fallback ist ausgeschlossen. Die Wire-Darstellung wird separat ausgewiesen: Codex 0.145 nutzt einen quellcodebelegten, clientseitig kanonisierten Default, Luban sendet Default explizit. Das ist semantische Parität, nicht byteidentisches Request-JSON.",
		"両 Agent は同じ固定モデル、推論強度、有効な default service tier を使用し、fallback を拒否します。wire 表現は別に開示します。Codex 0.145 はソースで証明されたクライアント正規化 default、Luban は明示的 default を送ります。これは意味上の同等性であり、リクエスト JSON のバイト同一性ではありません。",
		"두 Agent는 동일하게 고정된 모델, 추론 강도, 유효한 default service tier를 사용하고 fallback을 거부해야 합니다. wire 표현은 별도로 공개합니다. Codex 0.145는 소스로 입증된 클라이언트 정규화 default를 사용하고 Luban은 default를 명시적으로 전송합니다. 이는 의미상 동등성이지 요청 JSON 바이트 동일성이 아닙니다.",
		"Оба агента должны использовать одну закреплённую модель, одинаковый уровень рассуждений и эффективный service tier default, без fallback. Представление в wire раскрывается отдельно: Codex 0.145 использует подтверждённый исходным кодом канонический default клиента, а Luban передаёт default явно. Это семантический паритет, а не побайтово одинаковый JSON запроса.",
	}},
	{KeyAgenticReportMethodSameTasks, [6]string{
		"The compared runs must use the same immutable task selection and evaluator revision.",
		"对比运行必须使用相同的不可变题目选择和 evaluator 版本。",
		"Die verglichenen Läufe müssen dieselbe unveränderliche Aufgabenauswahl und Evaluator-Revision verwenden.",
		"比較する実行では、同じ不変のタスク選択と evaluator revision を使用する必要があります。",
		"비교 실행은 동일한 변경 불가능 작업 선택과 evaluator revision을 사용해야 합니다.",
		"Сравниваемые запуски должны использовать одну неизменяемую выборку заданий и ревизию evaluator.",
	}},
	{KeyAgenticReportMethodSameEnvironment, [6]string{
		"Resource limits, network policy, sandboxing, timeouts, and cache policy must be equivalent.",
		"资源限制、网络策略、sandbox、超时和缓存策略必须等价。",
		"Ressourcenlimits, Netzrichtlinie, Sandbox, Timeouts und Cache-Richtlinie müssen gleichwertig sein.",
		"リソース制限、ネットワークポリシー、sandbox、タイムアウト、キャッシュポリシーは同等でなければなりません。",
		"리소스 제한, 네트워크 정책, sandbox, 시간 제한 및 캐시 정책이 동등해야 합니다.",
		"Ограничения ресурсов, сетевая политика, sandbox, тайм-ауты и политика кэша должны быть эквивалентны.",
	}},
	{KeyAgenticReportMethodPairedSchedule, [6]string{
		"Execution order is paired and counterbalanced so task and time effects do not favor either agent.",
		"执行顺序采用配对与反平衡设计，避免题目和时间效应偏向任一 Agent。",
		"Die Ausführungsreihenfolge ist gepaart und ausbalanciert, damit Aufgaben- und Zeiteffekte keinen Agent bevorzugen.",
		"実行順序はペア化して均衡させ、タスクや時間の影響が一方の Agent に偏らないようにします。",
		"실행 순서를 페어링하고 균형화하여 작업 및 시간 효과가 어느 Agent에도 유리하지 않게 합니다.",
		"Порядок запусков попарно сбалансирован, чтобы эффекты задания и времени не давали преимущества ни одному агенту.",
	}},
	{KeyAgenticReportMethodIndependentEvaluation, [6]string{
		"Task completion is determined only by the frozen independent evaluator, never by the agent's self-report.",
		"任务完成情况仅由冻结的独立 evaluator 判定，绝不采用 Agent 自报结果。",
		"Die Aufgabenerfüllung wird ausschließlich vom fixierten unabhängigen Evaluator bestimmt, nie durch die Selbstauskunft des Agents.",
		"タスクの完了は固定された独立 evaluator のみが判定し、Agent の自己申告は使用しません。",
		"작업 완료 여부는 고정된 독립 evaluator만 판정하며 Agent의 자체 보고는 사용하지 않습니다.",
		"Выполнение задания определяет только зафиксированный независимый evaluator, а не самоотчёт агента.",
	}},
	{KeyAgenticReportMethodMetricUnits, [6]string{
		"All-started generate=true inference LLM calls, completed LLM responses, identity-bound retry amplification, provider-committed logical tool calls, execution-matched tool calls, and physical operations are distinct units. Retry amplification is unknown without complete logical-generation identity. Headline efficiency uses LLM calls; tool metrics are diagnostic.",
		"全部已开始且 generate=true 的推理 LLM 调用、已完成 LLM 响应、基于完整身份绑定的重试放大率、Provider 已提交逻辑工具调用、执行匹配工具调用和物理操作是不同计量单位。缺少完整逻辑生成身份时，重试放大率保持未知。头部效率采用 LLM 调用；工具指标仅用于诊断。",
		"Alle gestarteten Inferenz-LLM-Aufrufe mit generate=true, abgeschlossene LLM-Antworten, identitätsgebundene Retry-Amplifikation, vom Provider bestätigte logische Tool-Aufrufe, ausführungszugeordnete Tool-Aufrufe und physische Operationen sind verschiedene Einheiten. Ohne vollständige Identität der logischen Generierung bleibt die Retry-Amplifikation unbekannt. Die Haupteffizienz nutzt LLM-Aufrufe; Tool-Metriken sind diagnostisch.",
		"開始済みの generate=true 推論 LLM 呼び出し、完了した LLM 応答、ID に結び付いた再試行増幅率、Provider 確定の論理ツール呼び出し、実行照合済みツール呼び出し、物理オペレーションは別の単位です。論理生成 ID が完全でなければ、再試行増幅率は不明です。主要効率は LLM 呼び出しを用い、ツール指標は診断専用です。",
		"시작된 모든 generate=true 추론 LLM 호출, 완료된 LLM 응답, 식별자에 결합된 재시도 증폭률, Provider 확정 논리 도구 호출, 실행 매칭 도구 호출, 물리 작업은 서로 다른 단위입니다. 논리 생성 식별자가 완전하지 않으면 재시도 증폭률은 알 수 없음입니다. 주요 효율은 LLM 호출을 사용하며 도구 지표는 진단용입니다.",
		"Все начатые инференс-вызовы LLM с generate=true, завершённые ответы LLM, связанное с идентификаторами усиление из-за повторов, зафиксированные Provider логические вызовы инструментов, сопоставленные с исполнением вызовы и физические операции — разные единицы. Без полной идентификации логической генерации усиление из-за повторов остаётся неизвестным. Основная эффективность использует вызовы LLM; метрики инструментов диагностические.",
	}},
	{KeyAgenticReportMethodPublicScoreDistinction, [6]string{
		"live_pooled is the public CI primary score over included trials. task_macro is the paper-style per-task audit score. They use different denominators and are never substituted for one another.",
		"live_pooled 是基于纳入 trials 的公开 CI 主分；task_macro 是论文口径的逐题审计分。两者分母不同，绝不互相替代。",
		"live_pooled ist der primäre öffentliche CI-Wert über eingeschlossene Trials. task_macro ist der aufgabenweise Auditwert nach Paper-Methodik. Beide haben unterschiedliche Nenner und werden nie gegeneinander ausgetauscht.",
		"live_pooled は採用 trials 全体に対する公開 CI の主要スコアです。task_macro は論文方式のタスク別監査スコアです。分母が異なるため、相互に置き換えません。",
		"live_pooled는 포함된 trials 전체의 공개 CI 기본 점수입니다. task_macro는 논문 방식의 작업별 감사 점수입니다. 분모가 다르므로 서로 대체하지 않습니다.",
		"live_pooled — основной публичный CI-показатель по включённым trials. task_macro — по-заданий аудит в стиле статьи. У них разные знаменатели, и они не подменяют друг друга.",
	}},
	{KeyAgenticReportMethodComparativeInference, [6]string{
		"The following aggregate and paired-bootstrap tables are local comparative inference. They complement the public scorecard and never replace its scoring or confidence interval.",
		"以下汇总表和配对 bootstrap 表属于本地比较推断，仅补充公开计分卡，绝不替代其计分或置信区间。",
		"Die folgenden Aggregat- und Paired-Bootstrap-Tabellen sind lokale Vergleichsinferenz. Sie ergänzen die öffentliche Scorecard und ersetzen weder deren Wertung noch Konfidenzintervall.",
		"以下の集計表と paired bootstrap 表はローカルな比較推論です。公開スコアカードを補足するもので、その採点や信頼区間を置き換えません。",
		"다음 집계 및 paired bootstrap 표는 로컬 비교 추론입니다. 공개 스코어카드를 보완할 뿐 점수 계산이나 신뢰구간을 대체하지 않습니다.",
		"Следующие агрегированные таблицы и paired bootstrap — локальный сравнительный вывод. Они дополняют публичную оценку и не заменяют её правила или доверительный интервал.",
	}},
	{KeyAgenticReportMethodGatewayEvidence, [6]string{
		"Provider and served-model identity is observed by the configured benchmark gateway. The model slug is evidence as of the recorded time, not an OpenAI-signed attestation or a cryptographically immutable weight identity.",
		"Provider 与实际服务模型身份由已配置的测评网关观测。模型 slug 仅代表证据记录时点，并非 OpenAI 签名证明，也不是密码学不可变的权重身份。",
		"Provider- und Served-Model-Identität werden vom konfigurierten Benchmark-Gateway beobachtet. Der Modell-Slug ist Evidenz zum aufgezeichneten Zeitpunkt, keine von OpenAI signierte Bestätigung und keine kryptografisch unveränderliche Gewichtsidentität.",
		"Provider と実際に提供されたモデルの同一性は、設定済みベンチマークゲートウェイで観測されます。モデル slug は記録時点の証拠であり、OpenAI の署名付き証明でも、暗号学的に不変な重みの同一性でもありません。",
		"Provider 및 실제 제공 모델 식별 정보는 구성된 벤치마크 게이트웨이가 관측합니다. 모델 slug는 기록 시점의 증거일 뿐 OpenAI 서명 증명이나 암호학적으로 불변인 가중치 식별자가 아닙니다.",
		"Идентификаторы Provider и фактически обслуживавшей модели наблюдаются настроенным шлюзом бенчмарка. Slug модели — свидетельство на момент записи, а не подписанная OpenAI аттестация и не криптографически неизменный идентификатор весов.",
	}},
	{KeyAgenticReportMethodPhysicalAuxiliary, [6]string{
		"Adapter-reported operations are auxiliary and nullable: legacy wrappers, process starts, reads, searches, and file commits are not a homogeneous OS-operation metric. They do not determine the superiority verdict until typed child-operation coverage is complete and symmetric.",
		"Adapter 上报操作是辅助且可空的：旧 wrapper、进程启动、读取、搜索与文件提交并非同构的 OS 操作指标。在类型化子操作覆盖完整且对称之前，它不参与全面超越结论。",
		"Vom Adapter gemeldete Operationen sind ergänzend und nullable: Legacy-Wrapper, Prozessstarts, Lese-, Such- und Datei-Commit-Vorgänge bilden keine homogene OS-Metrik. Bis zur vollständigen und symmetrischen Abdeckung typisierter Unteroperationen entscheiden sie nicht über die Überlegenheit.",
		"Adapter 報告オペレーションは補助的かつ nullable です。旧 wrapper、プロセス開始、読み取り、検索、ファイル commit は同質の OS オペレーション指標ではありません。型付き子操作のカバレッジが完全かつ対称になるまで、優越判定には使いません。",
		"Adapter 보고 작업은 보조 지표이며 nullable입니다. 레거시 wrapper, 프로세스 시작, 읽기, 검색, 파일 커밋은 동질적인 OS 작업 지표가 아닙니다. 형식화된 하위 작업 커버리지가 완전하고 대칭적이기 전에는 우월성 판정에 사용하지 않습니다.",
		"Операции по данным Adapter — вспомогательная nullable-метрика: устаревшие wrapper, запуски процессов, чтения, поиски и фиксации файлов не образуют однородную метрику ОС. До полного и симметричного покрытия типизированных дочерних операций они не влияют на вывод о превосходстве.",
	}},
	{KeyAgenticReportMethodSingleAgentFairness, [6]string{
		"Both harnesses are restricted to one coding agent, with exactly two fixed benchmark agents and one shared model budget. Nested Agent/Team tools are forbidden by configuration and the observed tool catalog. This is an experimental fairness control, not a performance optimization.",
		"两个 harness 均限制为单个 coding agent，测评中固定为两个 agent 并共享同一模型预算。配置与实际观测到的工具目录都禁止嵌套 Agent/Team 工具。这是实验公平控制，不是性能优化。",
		"Beide Harnesses sind auf je einen Coding-Agenten beschränkt, bei genau zwei festen Benchmark-Agenten und demselben Modellbudget. Verschachtelte Agent-/Team-Tools sind durch Konfiguration und beobachteten Tool-Katalog verboten. Dies ist eine Fairnesskontrolle, keine Leistungsoptimierung.",
		"両 harness は単一の coding agent に制限され、固定された2つのベンチマーク agent が同一のモデル予算を使います。設定と観測済みツールカタログの両方で入れ子の Agent/Team ツールを禁止します。これは実験上の公平性制御であり、性能最適化ではありません。",
		"두 harness 모두 하나의 coding agent로 제한되며, 고정된 두 벤치마크 agent가 동일한 모델 예산을 사용합니다. 설정과 관측된 도구 카탈로그 모두 중첩 Agent/Team 도구를 금지합니다. 이는 실험 공정성 통제이며 성능 최적화가 아닙니다.",
		"Оба harness ограничены одним coding agent; в тесте фиксированы ровно два агента с единым бюджетом модели. Вложенные инструменты Agent/Team запрещены конфигурацией и наблюдаемым каталогом инструментов. Это контроль справедливости эксперимента, а не оптимизация производительности.",
	}},
	{KeyAgenticReportMethodTransportAccounting, [6]string{
		"Transport accounting uses two fixed universes: all started transport attempts for spend, usage, and cache coverage; inference-only requests for quality, model requests, and tool metrics. WebSocket connections, prewarm attempts, and billable-inference classifications are never inferred from one another.",
		"传输核算固定使用两套口径：所有已启动传输尝试用于费用、usage 与缓存覆盖；仅推理请求用于质量、模型请求与工具指标。WebSocket 连接、预热尝试和可计费推理分类绝不互相推断。",
		"Die Transportabrechnung verwendet zwei feste Grundgesamtheiten: alle gestarteten Transportversuche für Kosten-, Nutzungs- und Cache-Abdeckung sowie ausschließlich Inferenzanfragen für Qualität, Modellanfragen und Tool-Metriken. WebSocket-Verbindungen, Prewarm-Versuche und als abrechenbar klassifizierte Inferenzen werden nie voneinander abgeleitet.",
		"伝送集計では2つの固定母集団を使います。開始済みの全伝送試行は費用・usage・キャッシュのカバレッジに、推論リクエストのみは品質・モデルリクエスト・ツール指標に用います。WebSocket 接続、prewarm 試行、課金対象推論の分類を相互に推定しません。",
		"전송 집계는 두 개의 고정 모집단을 사용합니다. 시작된 모든 전송 시도는 비용·usage·캐시 커버리지에, 추론 요청만 품질·모델 요청·도구 지표에 사용합니다. WebSocket 연결, prewarm 시도, 과금 가능 추론 분류를 서로 추정하지 않습니다.",
		"Для транспортного учёта используются две фиксированные совокупности: все начатые попытки — для расходов, usage и покрытия кэша; только запросы вывода — для качества, запросов модели и инструментов. Соединения WebSocket, попытки prewarm и классификация оплачиваемого вывода не выводятся друг из друга.",
	}},
	{KeyAgenticReportMethodPublicReferenceCost, [6]string{
		"Published reference cost is source-reported historical context. Its cache-write, per-request long-context, and service-tier billing basis cannot be independently reconstructed, so it is excluded from the headline cost verdict and local cost differences.",
		"公开参考费用仅是来源方上报的历史背景。其 cache-write、逐请求长上下文和 service-tier 计费基础无法独立重建，因此不进入核心费用结论或本地费用差值。",
		"Veröffentlichte Referenzkosten sind lediglich vom Ursprung gemeldeter historischer Kontext. Ihre Abrechnungsgrundlage für Cache-Write, Long-Context je Anfrage und Service-Tier ist nicht unabhängig rekonstruierbar; deshalb fließen sie weder in das zentrale Kostenurteil noch in lokale Kostendifferenzen ein.",
		"公開参考費用は出典側が報告した過去の参考情報です。cache-write、リクエスト単位の長文コンテキスト、service-tier の課金根拠を独立再構築できないため、主要な費用判定やローカル費用差には含めません。",
		"공개 참고 비용은 출처가 보고한 과거 참고 정보입니다. cache-write, 요청별 장문 컨텍스트, service-tier 과금 근거를 독립적으로 재구성할 수 없으므로 핵심 비용 판정과 로컬 비용 차이에서 제외합니다.",
		"Опубликованная справочная стоимость — лишь исторический контекст по данным источника. Основание тарификации cache-write, длинного контекста по запросам и service-tier независимо не восстанавливается, поэтому эти данные не входят в основной вывод о стоимости и локальные разницы.",
	}},
	{KeyAgenticReportMethodStorageDeclaration, [6]string{
		"Declared storage: %d MB; Pier 0.3 local Docker does not enforce it as a task quota. Formal evidence separately reports the host and guest guards and receipts.",
		"声明存储：%d MB；Pier 0.3 本地 Docker 不会将其强制为任务配额。正式证据会分别报告主机与 Guest 的保护阈值和回执。",
		"Deklarierter Speicher: %d MB; Pier 0.3 erzwingt ihn in lokalem Docker nicht als Aufgabenquote. Formelle Nachweise weisen Host- und Guest-Schutzwerte sowie Belege getrennt aus.",
		"宣言ストレージ: %d MB。Pier 0.3 のローカル Docker はタスク quota として強制しません。正式証拠では Host と Guest の guard と receipt を別々に示します。",
		"선언된 저장 공간: %d MB. Pier 0.3 로컬 Docker는 이를 작업 quota로 강제하지 않습니다. 정식 증거는 Host와 Guest의 guard 및 receipt를 각각 보고합니다.",
		"Заявленное хранилище: %d МБ; локальный Docker в Pier 0.3 не применяет его как квоту задания. Формальные доказательства отдельно показывают проверки и подтверждения Host и Guest.",
	}},
	{KeyAgenticReportCostKnownLowerBound, [6]string{
		"Known lower bound: %s · cost receipts %d/%d all transport attempts · unknown-cost attempts %d · cache-write receipts %d/%d.",
		"已知下界：%s · 费用回执 %d/%d 次全部传输尝试 · 未知费用尝试 %d 次 · cache-write 回执 %d/%d。",
		"Bekannte Untergrenze: %s · Kostenbelege %d/%d aller Transportversuche · Versuche mit unbekannten Kosten %d · Cache-Write-Belege %d/%d.",
		"既知の下限: %s · 費用 receipt は全伝送試行の %d/%d · 費用不明の試行 %d · cache-write receipt %d/%d。",
		"알려진 하한: %s · 전체 전송 시도 비용 receipt %d/%d · 비용 불명 시도 %d · cache-write receipt %d/%d.",
		"Известная нижняя граница: %s · подтверждения стоимости %d/%d всех транспортных попыток · попыток с неизвестной стоимостью %d · подтверждения cache-write %d/%d.",
	}},

	{KeyAgenticReportGateClassification, [6]string{"Evidence classification", "证据分类", "Evidenzklassifikation", "証拠分類", "증거 분류", "Классификация доказательств"}},
	{KeyAgenticReportGateFormalScore, [6]string{"Formal scoring eligibility", "正式计分资格", "Formelle Wertungsberechtigung", "正式採点資格", "정식 채점 자격", "Допуск к формальной оценке"}},
	{KeyAgenticReportGateArtifactIntegrity, [6]string{"Artifact integrity", "工件完整性", "Artefaktintegrität", "成果物の完全性", "산출물 무결성", "Целостность артефактов"}},
	{KeyAgenticReportGateScorecardRecomputed, [6]string{"Scorecard recomputed", "计分卡重算", "Scorecard neu berechnet", "スコアカード再計算", "스코어카드 재계산", "Пересчёт оценочной карты"}},
	{KeyAgenticReportGatePairedSchedule, [6]string{"Paired schedule", "成对调度", "Gepaarter Ablaufplan", "ペア実行計画", "페어링 일정", "Попарный график"}},
	{KeyAgenticReportGateModelContract, [6]string{"Model contract", "模型契约", "Modellvertrag", "モデル契約", "모델 계약", "Контракт модели"}},
	{KeyAgenticReportGateSingleAgentFairness, [6]string{"Single-agent fairness control", "单 Agent 公平控制", "Fairnesskontrolle für Einzelagenten", "単一 Agent の公平性制御", "단일 Agent 공정성 통제", "Контроль справедливости одного Agent"}},
	{KeyAgenticReportGateNetworkIsolation, [6]string{"Network isolation", "网络隔离", "Netzwerkisolation", "ネットワーク分離", "네트워크 격리", "Сетевая изоляция"}},
	{KeyAgenticReportGateOracle, [6]string{"Oracle verification", "Oracle 验证", "Oracle-Prüfung", "Oracle 検証", "Oracle 검증", "Проверка Oracle"}},
	{KeyAgenticReportGateCompleteSpend, [6]string{"Complete spend coverage", "完整花费覆盖", "Vollständige Kostenabdeckung", "費用の完全なカバレッジ", "비용 완전 커버리지", "Полное покрытие расходов"}},
	{KeyAgenticReportGateToolExecution, [6]string{"Logical tool execution coverage", "逻辑工具执行覆盖", "Abdeckung der logischen Werkzeugausführung", "論理ツール実行のカバレッジ", "논리 도구 실행 커버리지", "Покрытие выполнения логических инструментов"}},
	{KeyAgenticReportGateControllerDuration, [6]string{"Controller end-to-end duration", "Controller 端到端耗时", "Controller-End-to-End-Dauer", "Controller のエンドツーエンド所要時間", "Controller 종단 간 소요 시간", "Сквозная длительность Controller"}},
	{KeyAgenticReportGateExclusionSymmetry, [6]string{"Infrastructure-exclusion symmetry", "基础设施排除对称性", "Symmetrie der Infrastruktur-Ausschlüsse", "インフラ除外の対称性", "인프라 제외 대칭성", "Симметрия инфраструктурных исключений"}},
	{KeyAgenticReportGateStorageEvidence, [6]string{"Host and guest storage evidence", "主机与 Guest 存储证据", "Host- und Guest-Speichernachweise", "Host と Guest のストレージ証拠", "Host 및 Guest 스토리지 증거", "Доказательства хранилища Host и Guest"}},
	{KeyAgenticReportGateProjectionIntegrity, [6]string{"Raw-to-normalized projection integrity", "Raw 到 normalized 投影完整性", "Integrität der Raw-zu-Normalized-Projektion", "Raw から normalized への投影整合性", "Raw-to-normalized 투영 무결성", "Целостность проекции Raw-to-normalized"}},
	{KeyAgenticReportVerdictExceeds, [6]string{
		"Formal evidence satisfies every superiority gate.",
		"正式证据满足全部“全面超越”门槛。",
		"Die formelle Evidenz erfüllt alle Überlegenheitskriterien.",
		"正式な証拠がすべての優越条件を満たしています。",
		"정식 증거가 모든 우위 기준을 충족합니다.",
		"Формальные доказательства удовлетворяют всем критериям превосходства.",
	}},
	{KeyAgenticReportVerdictNotExceeds, [6]string{
		"Formal evidence does not satisfy every superiority gate.",
		"正式证据未满足全部“全面超越”门槛。",
		"Die formelle Evidenz erfüllt nicht alle Überlegenheitskriterien.",
		"正式な証拠はすべての優越条件を満たしていません。",
		"정식 증거가 모든 우위 기준을 충족하지 못했습니다.",
		"Формальные доказательства удовлетворяют не всем критериям превосходства.",
	}},
	{KeyAgenticReportVerdictInsufficient, [6]string{
		"Evidence is insufficient; no winner may be declared.",
		"证据不足，禁止宣告胜出。",
		"Die Evidenz reicht nicht aus; es darf kein Sieger erklärt werden.",
		"証拠が不十分なため、勝者を宣言できません。",
		"증거가 불충분하여 승자를 선언할 수 없습니다.",
		"Доказательств недостаточно; объявлять победителя нельзя.",
	}},

	{KeyAgenticReportWaterfallHeadersWait, [6]string{"Request to response headers", "请求至响应 headers", "Anfrage bis Response-Header", "リクエストから response headers", "요청부터 response headers", "От запроса до заголовков ответа"}},
	{KeyAgenticReportWaterfallTTFT, [6]string{"Headers to first byte", "Headers 至首字节", "Header bis zum ersten Byte", "Headers から最初のバイト", "Headers부터 첫 바이트", "От заголовков до первого байта"}},
	{KeyAgenticReportWaterfallStream, [6]string{"Response stream", "响应流", "Antwortstream", "レスポンスストリーム", "응답 스트림", "Поток ответа"}},
	{KeyAgenticReportWaterfallProviderError, [6]string{"Provider error", "Provider 错误", "Provider-Fehler", "Provider エラー", "Provider 오류", "Ошибка Provider"}},
	{KeyAgenticReportWaterfallToolCriticalPath, [6]string{"Round-level tool critical path", "Round 级工具关键路径", "Kritischer Tool-Pfad je Runde", "ラウンド単位のツールクリティカルパス", "라운드 단위 도구 임계 경로", "Критический путь инструментов в раунде"}},
	{KeyAgenticReportWaterfallOverlappingTotals, [6]string{
		"Total tool latency is the sum of child operations and can overlap in parallel; do not stack it onto the critical path.",
		"工具总耗时是子操作之和，可能并行重叠；不能与关键路径堆叠相加。",
		"Die gesamte Tool-Latenz ist die Summe der Unteroperationen und kann sich bei Parallelität überlappen; sie darf nicht auf den kritischen Pfad addiert werden.",
		"ツール総レイテンシは子操作の合計で、並列実行時には重複します。クリティカルパスに積み上げてはいけません。",
		"도구 총 지연 시간은 하위 작업의 합이며 병렬 실행 시 겹칠 수 있으므로 임계 경로에 더하면 안 됩니다.",
		"Общая задержка инструментов — сумма дочерних операций, которые могут перекрываться; её нельзя складывать с критическим путём.",
	}},
	{KeyAgenticReportWaterfallNoFabrication, [6]string{
		"When per-tool timestamps are absent, the report does not fabricate an exact tool waterfall.",
		"缺少单工具时间戳时，报告不会伪造精确工具瀑布。",
		"Fehlen Zeitstempel je Tool, erfindet der Bericht keinen exakten Tool-Wasserfall.",
		"ツール別のタイムスタンプがない場合、正確なツールウォーターフォールを捏造しません。",
		"도구별 타임스탬프가 없으면 보고서는 정밀한 도구 워터폴을 만들어 내지 않습니다.",
		"При отсутствии временных меток отдельных инструментов отчёт не выдумывает точный водопад.",
	}},
	{KeyAgenticReportWaterfallAgentStart, [6]string{"Agent start", "Agent 开始", "Agent-Start", "Agent 開始", "Agent 시작", "Начало Agent"}},
	{KeyAgenticReportWaterfallAgentFinish, [6]string{"Agent finish", "Agent 结束", "Agent-Ende", "Agent 終了", "Agent 종료", "Завершение Agent"}},
	{KeyAgenticReportWaterfallDescription, [6]string{
		"Each row is one actual Provider request on the agent wall-time axis. Tool critical-path, total-latency, and queue values remain numeric because no tool span timestamps are available to place them honestly.",
		"每行表示 Agent 墙钟时间轴上的一个实际 Provider 请求。由于没有可诚实定位的工具 span 时间戳，工具关键路径、总耗时和排队时间仅以数值展示。",
		"Jede Zeile entspricht einer tatsächlichen Provider-Anfrage auf der Agent-Wandzeitachse. Kritischer Pfad, Gesamtlatenz und Warteschlangenzeit der Tools bleiben numerisch, weil keine Tool-Span-Zeitstempel für eine ehrliche Positionierung vorliegen.",
		"各行は Agent の実時間軸上の実際の Provider リクエストです。ツール span の時刻がなく正確に配置できないため、クリティカルパス、総レイテンシ、キュー時間は数値のみで表示します。",
		"각 행은 Agent 경과 시간축의 실제 Provider 요청 하나입니다. 정직하게 배치할 도구 span 타임스탬프가 없으므로 임계 경로, 총 지연 시간, 대기 시간은 수치로만 표시합니다.",
		"Каждая строка — фактический запрос Provider на шкале времени Agent. Критический путь, суммарная задержка и очередь инструментов остаются числовыми, поскольку нет временных меток span для достоверного размещения.",
	}},

	{KeyAgenticReportOptimizationMechanism, [6]string{"Mechanism", "机制", "Mechanismus", "メカニズム", "메커니즘", "Механизм"}},
	{KeyAgenticReportOptimizationValue, [6]string{"Why it matters", "为什么有价值", "Warum es wichtig ist", "価値がある理由", "가치가 있는 이유", "Почему это важно"}},
	{KeyAgenticReportOptimizationRisk, [6]string{"Risks", "风险", "Risiken", "リスク", "위험", "Риски"}},
	{KeyAgenticReportOptimizationImplementation, [6]string{"Production implementation", "生产化落点", "Produktionsimplementierung", "本番実装", "프로덕션 구현", "Реализация для эксплуатации"}},
	{KeyAgenticReportOptimizationBefore, [6]string{"Before", "优化前", "Vorher", "変更前", "개선 전", "До"}},
	{KeyAgenticReportOptimizationAfter, [6]string{"After", "优化后", "Nachher", "変更後", "개선 후", "После"}},
	{KeyAgenticReportOptimizationDesignDefect, [6]string{"Design defect", "设计缺陷", "Entwurfsfehler", "設計上の欠陥", "설계 결함", "Дефект проектирования"}},
	{KeyAgenticReportOptimizationAttribution, [6]string{"Attribution scope", "归因范围", "Attributionsumfang", "帰属範囲", "귀속 범위", "Область атрибуции"}},
	{KeyAgenticReportOptimizationLayer, [6]string{"Measurement layer", "测量层级", "Messebene", "測定レイヤー", "측정 계층", "Уровень измерения"}},
	{KeyAgenticReportOptimizationEvidenceGrade, [6]string{"Evidence grade", "证据等级", "Evidenzgrad", "証拠等級", "증거 등급", "Уровень доказательности"}},
	{KeyAgenticReportOptimizationExpectedEffect, [6]string{"Expected effect", "预期效果", "Erwarteter Effekt", "期待される効果", "예상 효과", "Ожидаемый эффект"}},
	{KeyAgenticReportOptimizationObservedEffect, [6]string{"Observed effect", "观察到的效果", "Beobachteter Effekt", "観測された効果", "관측된 효과", "Наблюдаемый эффект"}},
	{KeyAgenticReportOptimizationConfounders, [6]string{"Confounders", "混杂因素", "Störfaktoren", "交絡要因", "교란 요인", "Смешивающие факторы"}},
	{KeyAgenticReportOptimizationAblation, [6]string{"Ablation", "消融", "Ablation", "アブレーション", "절제 실험", "Абляция"}},
	{KeyAgenticReportAblationMeasured, [6]string{"Measured ablation", "已测消融", "Gemessene Ablation", "測定済みアブレーション", "측정된 절제 실험", "Измеренная абляция"}},
	{KeyAgenticReportAblationNotRun, [6]string{"Ablation not run", "未运行消融", "Ablation nicht ausgeführt", "アブレーション未実行", "절제 실험 미실행", "Абляция не выполнена"}},

	{KeyAgenticReportFailureImplementation, [6]string{"Implementation error", "实现错误", "Implementierungsfehler", "実装エラー", "구현 오류", "Ошибка реализации"}},
	{KeyAgenticReportFailureIncomplete, [6]string{"Incomplete implementation", "实现不完整", "Unvollständige Implementierung", "実装不完了", "불완전한 구현", "Неполная реализация"}},
	{KeyAgenticReportFailureRegression, [6]string{"Regression", "回归", "Regression", "リグレッション", "회귀", "Регрессия"}},
	{KeyAgenticReportFailureValidation, [6]string{"Insufficient validation", "验证不足", "Unzureichende Validierung", "検証不足", "검증 부족", "Недостаточная проверка"}},
	{KeyAgenticReportFailureTimeout, [6]string{"Timeout", "超时", "Zeitüberschreitung", "タイムアウト", "시간 초과", "Тайм-аут"}},
	{KeyAgenticReportFailureInfrastructure, [6]string{"Infrastructure failure", "基础设施故障", "Infrastrukturfehler", "インフラ障害", "인프라 장애", "Сбой инфраструктуры"}},
	{KeyAgenticReportFailureProtocol, [6]string{"Protocol violation", "协议违规", "Protokollverstoß", "プロトコル違反", "프로토콜 위반", "Нарушение протокола"}},
	{KeyAgenticReportFailureUnknown, [6]string{"Unclassified failure", "未判定失败", "Nicht klassifizierter Fehler", "未分類の失敗", "분류되지 않은 실패", "Неклассифицированный сбой"}},

	{KeyAgenticReportReproductionObject, [6]string{"Object", "对象", "Objekt", "対象", "대상", "Объект"}},
	{KeyAgenticReportReproductionIdentity, [6]string{"SHA-256 / identity", "SHA-256 / identity", "SHA-256 / Identität", "SHA-256 / identity", "SHA-256 / identity", "SHA-256 / идентификатор"}},
	{KeyAgenticReportReproductionSource, [6]string{"Source", "来源", "Quelle", "出典", "출처", "Источник"}},
	{KeyAgenticReportReproductionMissingCommand, [6]string{"No reproduction command was registered.", "未登记复现命令。", "Es wurde kein Reproduktionsbefehl registriert.", "再現コマンドが登録されていません。", "재현 명령이 등록되지 않았습니다.", "Команда воспроизведения не зарегистрирована."}},
	{KeyAgenticReportReproductionSafety, [6]string{
		"Commands are stored as argv and POSIX-quoted for display. The report generator does not execute them and does not accept credentials, raw prompts, or raw tool output as display input.",
		"命令以 argv 存储并按 POSIX 规则转义后展示。报告生成器不会执行命令，也不接受 credentials、raw prompt 或 raw tool output 作为展示输入。",
		"Befehle werden als argv gespeichert und zur Anzeige nach POSIX quotiert. Der Berichtsgenerator führt sie nicht aus und akzeptiert weder Zugangsdaten noch rohe Prompts oder rohe Tool-Ausgaben als Anzeigeeingabe.",
		"コマンドは argv として保存し、POSIX 形式で引用して表示します。レポート生成器はコマンドを実行せず、credentials、raw prompt、raw tool output を表示入力として受け付けません。",
		"명령은 argv로 저장하고 POSIX 규칙으로 인용해 표시합니다. 보고서 생성기는 명령을 실행하지 않으며 credentials, raw prompt, raw tool output을 표시 입력으로 받지 않습니다.",
		"Команды хранятся как argv и для показа экранируются по POSIX. Генератор отчёта не выполняет их и не принимает credentials, сырые prompt или вывод инструментов для отображения.",
	}},

	{KeyAgenticReportLimitationFrozenCost, [6]string{
		"Comparable cost is a same-gateway frozen-rate estimate from visible all-transport input, cached-input, and output tokens. It excludes cache-write premiums and is not an official invoice; provider-reported cost is excluded from the verdict.",
		"可比费用是基于同一网关冻结费率、按全部传输中可见的 input、cached-input 与 output tokens 估算的结果。它不含 cache-write 溢价，也不是官方账单；Provider 报告费用不进入结论。",
		"Die vergleichbaren Kosten sind eine Schätzung mit fixierten Raten desselben Gateways aus sichtbaren Input-, Cached-Input- und Output-Tokens aller Transporte. Cache-Write-Aufschläge sind ausgeschlossen; es handelt sich nicht um eine offizielle Rechnung, und Provider-Kosten fließen nicht in die Bewertung ein.",
		"比較費用は、同一 gateway の固定レートを使い、全伝送で観測できた input、cached-input、output tokens から算出した見積もりです。cache-write の追加料金は含まず、公式請求書でもありません。Provider 報告費用は判定から除外します。",
		"비교 비용은 동일 gateway의 고정 요율로 전체 전송에서 관측된 input, cached-input, output tokens를 계산한 추정치입니다. cache-write 추가 요금은 제외하며 공식 청구서가 아닙니다. Provider 보고 비용은 판정에서 제외합니다.",
		"Сопоставимая стоимость — оценка по фиксированным тарифам одного gateway на основе видимых токенов input, cached-input и output всех передач. Надбавка за cache-write не включена, это не официальный счёт, а стоимость по данным Provider исключена из вывода.",
	}},
	{KeyAgenticReportLimitationIncompatibleEvidence, [6]string{
		"Runs with lost evidence, partial usage, a different evaluator, or a different reasoning effort cannot enter the formal comparison.",
		"丢失证据、usage 不完整、evaluator 不同或推理强度不同的运行不能进入正式比较。",
		"Läufe mit verlorener Evidenz, unvollständiger Usage, anderem Evaluator oder anderer Reasoning-Stufe dürfen nicht in den formellen Vergleich eingehen.",
		"証拠の欠落、usage の不完全、異なる evaluator、異なる推論強度がある実行は、正式比較に含められません。",
		"증거 유실, 불완전한 usage, 다른 evaluator 또는 다른 추론 강도의 실행은 정식 비교에 포함할 수 없습니다.",
		"Запуски с потерянными доказательствами, неполной usage, другим evaluator или уровнем рассуждений нельзя включать в формальное сравнение.",
	}},

	{KeyAgenticReportEmptyPublicReference, [6]string{
		"No published reference was provided. Local diagnostic data is never used to invent a published score.",
		"未提供公开参考；报告绝不会用本地诊断数据推造公开分数。",
		"Es wurde keine veröffentlichte Referenz angegeben. Aus lokalen Diagnosedaten wird niemals ein veröffentlichter Wert abgeleitet.",
		"公開参考値は提供されていません。ローカル診断データから公開スコアを作り出すことはありません。",
		"공개 참고 점수가 제공되지 않았습니다. 로컬 진단 데이터로 공개 점수를 만들어 내지 않습니다.",
		"Опубликованный ориентир не предоставлен. Локальные диагностические данные никогда не используются для выдумывания опубликованной оценки.",
	}},
	{KeyAgenticReportEmptyOptimizations, [6]string{
		"The optimization ledger has no entries; the report does not invent optimization gains.",
		"优化 ledger 没有条目；报告不会虚构优化收益。",
		"Das Optimierungs-Ledger enthält keine Einträge; der Bericht erfindet keine Optimierungsgewinne.",
		"最適化 ledger に項目がありません。レポートは最適化効果を作り出しません。",
		"최적화 ledger에 항목이 없습니다. 보고서는 최적화 효과를 만들어 내지 않습니다.",
		"В ledger оптимизаций нет записей; отчёт не выдумывает прирост от оптимизации.",
	}},
	{KeyAgenticReportEmptyFailures, [6]string{
		"No failed task is present in the loaded evidence. Failure annotations are a formal-output gate: any unclassified failure prevents report generation.",
		"已加载证据中没有失败题。失败标注是正式输出门禁：任何未分类失败都会阻止报告生成。",
		"Die geladenen Nachweise enthalten keine fehlgeschlagene Aufgabe. Fehlerannotationen sind eine Ausgabevoraussetzung: Jeder nicht klassifizierte Fehler verhindert die Berichtserstellung.",
		"読み込んだ証拠に失敗タスクはありません。失敗注釈は正式出力のゲートであり、未分類の失敗があるとレポートは生成されません。",
		"불러온 증거에 실패한 작업이 없습니다. 실패 주석은 정식 출력 게이트이며, 분류되지 않은 실패가 하나라도 있으면 보고서를 생성하지 않습니다.",
		"В загруженных доказательствах нет проваленных заданий. Аннотации сбоев обязательны для формального вывода: любой неклассифицированный сбой блокирует создание отчёта.",
	}},
	{KeyAgenticReportEmptyToolStats, [6]string{
		"No like-for-like tool breakdown was provided; tool-call counts are not inferred from native events.",
		"未提供同口径工具分解；报告不会从原生事件推导工具调用数。",
		"Es wurde keine vergleichbare Tool-Aufschlüsselung angegeben; Tool-Aufrufe werden nicht aus nativen Ereignissen abgeleitet.",
		"同一口径のツール内訳は提供されていません。ネイティブイベントからツール呼び出し数を推定しません。",
		"동일 기준의 도구 분석이 제공되지 않았습니다. 네이티브 이벤트에서 도구 호출 수를 추론하지 않습니다.",
		"Сопоставимая детализация по инструментам не предоставлена; число их вызовов не выводится из нативных событий.",
	}},
	{KeyAgenticReportDiagnosticWatermark, [6]string{
		"DIAGNOSTIC · NOT SCOREABLE",
		"仅诊断 · 不可计分",
		"DIAGNOSE · NICHT WERTBAR",
		"診断用 · 採点対象外",
		"진단용 · 채점 불가",
		"ДИАГНОСТИКА · ВНЕ ЗАЧЁТА",
	}},
	{KeyAgenticReportDevelopmentWatermark, [6]string{
		"5-TASK DEVELOPMENT BENCHMARK · NOT FORMAL · formal_compatible=false",
		"5 题开发测评 · 非正式 · formal_compatible=false",
		"ENTWICKLUNGSTEST MIT 5 AUFGABEN · NICHT FORMELL · formal_compatible=false",
		"5 タスク開発ベンチマーク · 非公式 · formal_compatible=false",
		"5개 작업 개발 평가 · 비공식 · formal_compatible=false",
		"РАЗРАБОТЧЕСКИЙ ТЕСТ ИЗ 5 ЗАДАНИЙ · НЕОФИЦИАЛЬНО · formal_compatible=false",
	}},
	{KeyAgenticReportFooter, [6]string{
		"Generated by the content-addressed Agentic report pipeline · evidence as of %s · self-contained file with no external runtime dependencies",
		"由内容寻址的 Agentic 报告流水线生成 · 证据截至 %s · 单文件，无外部运行时依赖",
		"Erzeugt durch die inhaltsadressierte Agentic-Berichtspipeline · Evidenzstand %s · eigenständige Datei ohne externe Laufzeitabhängigkeiten",
		"コンテンツアドレス方式の Agentic レポートパイプラインで生成 · 証拠基準日時 %s · 外部ランタイム依存のない自己完結型ファイル",
		"콘텐츠 주소 기반 Agentic 보고서 파이프라인에서 생성 · 증거 기준 시각 %s · 외부 런타임 의존성이 없는 독립 실행형 파일",
		"Создано конвейером Agentic-отчётов с адресацией по содержимому · данные по состоянию на %s · автономный файл без внешних зависимостей времени выполнения",
	}},
	{KeyAgenticReportStatisticsSummary, [6]string{
		"Method: %s · confidence: %s · resamples: %d · seed: %d.",
		"方法：%s · 置信度：%s · 重采样次数：%d · 随机种子：%d。",
		"Methode: %s · Konfidenz: %s · Resamples: %d · Seed: %d.",
		"手法: %s · 信頼度: %s · リサンプル回数: %d · seed: %d。",
		"방법: %s · 신뢰도: %s · 재표본 추출 횟수: %d · seed: %d.",
		"Метод: %s · доверительная вероятность: %s · повторных выборок: %d · seed: %d.",
	}},
	{KeyAgenticReportPairedSummary, [6]string{
		"Matched %d tasks / %d pairs · quality wins / losses / ties: %d / %d / %d.",
		"匹配 %d 道题 / %d 个配对 · 质量胜 / 负 / 平：%d / %d / %d。",
		"Zugeordnet: %d Aufgaben / %d Paare · Qualitätsgewinne / -verluste / Gleichstände: %d / %d / %d.",
		"対応済み: %d タスク / %d ペア · 品質の勝ち / 負け / 引き分け: %d / %d / %d。",
		"매칭: 작업 %d개 / 페어 %d개 · 품질 승 / 패 / 무: %d / %d / %d.",
		"Сопоставлено: %d заданий / %d пар · победы / поражения / ничьи по качеству: %d / %d / %d.",
	}},

	{KeyAgenticReportCLIFlagInput, [6]string{"Path to the report input JSON.", "报告输入 JSON 的路径。", "Pfad zur JSON-Eingabe des Berichts.", "レポート入力 JSON のパス。", "보고서 입력 JSON 경로입니다.", "Путь к входному JSON отчёта."}},
	{KeyAgenticReportCLIFlagOutput, [6]string{"Path for the generated self-contained HTML.", "生成的自包含 HTML 路径。", "Pfad für das erzeugte eigenständige HTML.", "生成する自己完結型 HTML のパス。", "생성할 독립 실행형 HTML 경로입니다.", "Путь для создаваемого автономного HTML."}},
	{KeyAgenticReportCLIRequired, [6]string{"Both --input and --output are required.", "必须同时提供 --input 和 --output。", "Sowohl --input als auch --output sind erforderlich.", "--input と --output の両方が必要です。", "--input과 --output을 모두 지정해야 합니다.", "Необходимо указать и --input, и --output."}},
	{KeyAgenticReportCLISuccess, [6]string{"Report written to %s.", "报告已写入 %s。", "Bericht wurde nach %s geschrieben.", "レポートを %s に書き込みました。", "보고서를 %s에 기록했습니다.", "Отчёт записан в %s."}},
	{KeyAgenticReportCLIError, [6]string{"Could not generate the report: %v", "无法生成报告：%v", "Der Bericht konnte nicht erzeugt werden: %v", "レポートを生成できませんでした: %v", "보고서를 생성하지 못했습니다: %v", "Не удалось создать отчёт: %v"}},
}

var agenticReportKeys = func() []Key {
	keys := make([]Key, 0, len(agenticReportCopy))
	for _, entry := range agenticReportCopy {
		keys = append(keys, entry.key)
	}
	return keys
}()

func init() {
	for _, entry := range agenticReportCopy {
		semanticTranslations[entry.key] = map[Language]string{
			LangEN: entry.copy[0],
			LangZH: entry.copy[1],
			LangDE: entry.copy[2],
			LangJA: entry.copy[3],
			LangKO: entry.copy[4],
			LangRU: entry.copy[5],
		}
	}
}
