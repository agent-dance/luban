package i18n

// Semantic copy used only by the receipt-backed, local five-task Agentic
// Coding development report. Product names, tool names, protocol identifiers,
// formula symbols, and raw evidence remain untranslated parameters or terms.
const (
	KeyAgenticLocal5Watermark Key = "agentic.local5.watermark"
	KeyAgenticLocal5Subtitle  Key = "agentic.local5.subtitle"

	KeyAgenticLocal5SectionCurrentComparison       Key = "agentic.local5.section.current_comparison"
	KeyAgenticLocal5SectionBeforeAfter             Key = "agentic.local5.section.before_after"
	KeyAgenticLocal5SectionHistoricalToolRootCause Key = "agentic.local5.section.historical_tool_root_cause"
	KeyAgenticLocal5SectionKeyOptimizations        Key = "agentic.local5.section.key_optimizations"
	KeyAgenticLocal5SectionRawEvidence             Key = "agentic.local5.section.raw_evidence"
	KeyAgenticLocal5SectionConclusion              Key = "agentic.local5.section.conclusion"
	KeyAgenticLocal5SectionEfficiencyDiagnosis     Key = "agentic.local5.section.efficiency_diagnosis"
	KeyAgenticLocal5SectionOptimizationEvidence    Key = "agentic.local5.section.optimization_evidence"
	KeyAgenticLocal5SectionSharedPassComparison    Key = "agentic.local5.section.shared_pass_comparison"

	KeyAgenticLocal5CaveatSample5Nonofficial          Key = "agentic.local5.caveat.sample5_nonofficial"
	KeyAgenticLocal5CaveatToolDiagnosticNotComparable Key = "agentic.local5.caveat.tool_diagnostic_not_comparable"
	KeyAgenticLocal5CaveatCostFormulaEstimate         Key = "agentic.local5.caveat.cost_formula_estimate"
	KeyAgenticLocal5CaveatExactLLMDefinition          Key = "agentic.local5.caveat.exact_llm_definition"
	KeyAgenticLocal5CaveatIncompleteRefusal           Key = "agentic.local5.caveat.incomplete_refusal"
	KeyAgenticLocal5CaveatAttributionBundle           Key = "agentic.local5.caveat.attribution_bundle"
	KeyAgenticLocal5CaveatBaselineSecurityIncident    Key = "agentic.local5.caveat.baseline_security_incident"
	KeyAgenticLocal5CaveatSharedPassScope             Key = "agentic.local5.caveat.shared_pass_scope"
	KeyAgenticLocal5CaveatSharedPassToolTaxonomy      Key = "agentic.local5.caveat.shared_pass_tool_taxonomy"
	KeyAgenticLocal5SharedPassLongerSlower            Key = "agentic.local5.shared_pass.longer_slower"
	KeyAgenticLocal5SharedPassNormalizedNeutral       Key = "agentic.local5.shared_pass.normalized_neutral"

	KeyAgenticLocal5ConclusionNoComprehensiveSuperiority Key = "agentic.local5.conclusion.no_comprehensive_superiority"
	KeyAgenticLocal5ConclusionMeasured                   Key = "agentic.local5.conclusion.measured"
	KeyAgenticLocal5ConclusionCacheContext               Key = "agentic.local5.conclusion.cache_context"
	KeyAgenticLocal5ConclusionRawProjectionScope         Key = "agentic.local5.conclusion.raw_projection_scope"

	KeyAgenticLocal5EfficiencyPrimaryConclusion Key = "agentic.local5.efficiency.primary_conclusion"
	KeyAgenticLocal5EfficiencyCompletionTail    Key = "agentic.local5.efficiency.completion_tail"
	KeyAgenticLocal5EfficiencyTailUpperBound    Key = "agentic.local5.efficiency.tail_upper_bound"
	KeyAgenticLocal5EfficiencyFlightProof       Key = "agentic.local5.efficiency.flight_proof"

	KeyAgenticLocal5ScopeTrajectoryDiagnostic Key = "agentic.local5.scope.trajectory_diagnostic"
	KeyAgenticLocal5ScopeUnitFixture          Key = "agentic.local5.scope.unit_fixture"
	KeyAgenticLocal5ScopeSyntheticFixture     Key = "agentic.local5.scope.synthetic_fixture"
	KeyAgenticLocal5ScopeFieldDiagnostic      Key = "agentic.local5.scope.field_diagnostic"

	KeyAgenticLocal5LimitationEvaluatorSemantics   Key = "agentic.local5.limitation.evaluator_semantics"
	KeyAgenticLocal5LimitationExperimentalDesign   Key = "agentic.local5.limitation.experimental_design"
	KeyAgenticLocal5LimitationModelUnverified      Key = "agentic.local5.limitation.model_unverified"
	KeyAgenticLocal5LimitationMutableRawNoManifest Key = "agentic.local5.limitation.mutable_raw_no_manifest"
	KeyAgenticLocal5MetricMeterRecordedPOST        Key = "agentic.local5.metric.meter_recorded_post"
	KeyAgenticLocal5Footer                         Key = "agentic.local5.footer"

	KeyAgenticLocal5RootCauseFragmentedSurface   Key = "agentic.local5.root_cause.fragmented_surface"
	KeyAgenticLocal5RootCauseSequentialMutation  Key = "agentic.local5.root_cause.sequential_mutation"
	KeyAgenticLocal5RootCauseShellOverloading    Key = "agentic.local5.root_cause.shell_overloading"
	KeyAgenticLocal5RootCauseTelemetryConflation Key = "agentic.local5.root_cause.telemetry_conflation"

	KeyAgenticLocal5OptimizationInspectIntegration         Key = "agentic.local5.optimization.inspect_integration"
	KeyAgenticLocal5OptimizationApplyPatchAtomic           Key = "agentic.local5.optimization.apply_patch_atomic"
	KeyAgenticLocal5OptimizationRunVerification            Key = "agentic.local5.optimization.run_verification"
	KeyAgenticLocal5OptimizationThreeToolCatalog           Key = "agentic.local5.optimization.three_tool_catalog"
	KeyAgenticLocal5OptimizationContinuationCacheLineage   Key = "agentic.local5.optimization.continuation_cache_lineage"
	KeyAgenticLocal5OptimizationUnifiedAttemptRetry        Key = "agentic.local5.optimization.unified_attempt_retry"
	KeyAgenticLocal5OptimizationPreciseTelemetry           Key = "agentic.local5.optimization.precise_telemetry"
	KeyAgenticLocal5OptimizationPrintSessionQuartet        Key = "agentic.local5.optimization.print_session_quartet"
	KeyAgenticLocal5OptimizationInspectCursorCompatibility Key = "agentic.local5.optimization.inspect_cursor_compatibility"
)

type agenticLocal5CopyEntry struct {
	key  Key
	copy [6]string
}

var agenticLocal5Copy = [...]agenticLocal5CopyEntry{
	{KeyAgenticLocal5Watermark, [6]string{
		"LOCAL 5-TASK PRELIMINARY NON-OFFICIAL EVALUATION · NOT A PUBLIC BENCHMARK SCORE",
		"本机5题初步非官方评估 · 不代表公开测评分数",
		"LOKALE VORLÄUFIGE NICHT OFFIZIELLE BEWERTUNG MIT 5 AUFGABEN · KEIN ÖFFENTLICHER BENCHMARKWERT",
		"ローカル 5 タスク予備非公式評価 · 公開ベンチマークスコアではありません",
		"로컬 5개 작업 예비 비공식 평가 · 공개 벤치마크 점수가 아님",
		"ЛОКАЛЬНАЯ ПРЕДВАРИТЕЛЬНАЯ НЕОФИЦИАЛЬНАЯ ОЦЕНКА ПО 5 ЗАДАНИЯМ · НЕ ЯВЛЯЕТСЯ ПУБЛИЧНОЙ ОЦЕНКОЙ БЕНЧМАРКА",
	}},
	{KeyAgenticLocal5Subtitle, [6]string{
		"A receipt-backed development comparison of Codex and Luban on five representative Agentic Coding tasks, with observed deltas, candidate mechanisms, and raw evidence.",
		"基于回执的开发阶段对比：Codex 与 Luban 在 5 道代表性 Agentic Coding 题上的表现，并附观察差值、候选机制与原始证据。",
		"Ein beleggestützter Entwicklungsvergleich von Codex und Luban anhand von fünf repräsentativen Agentic-Coding-Aufgaben, einschließlich Vorher-Nachher-Zuordnung und Rohbelegen.",
		"代表的な 5 つの Agentic Coding タスクで Codex と Luban を比較した、受領記録に基づく開発レポートです。最適化前後の帰属分析と生の証拠を含みます。",
		"대표적인 Agentic Coding 작업 5개에서 Codex와 Luban을 비교한 영수증 기반 개발 보고서로, 최적화 전후의 기여 분석과 원시 증거를 포함합니다.",
		"Подкреплённое квитанциями сравнение Codex и Luban на этапе разработки по пяти репрезентативным задачам Agentic Coding, с атрибуцией изменений до и после оптимизации и исходными доказательствами.",
	}},

	{KeyAgenticLocal5SectionCurrentComparison, [6]string{
		"Current five-task comparison", "当前 5 题对比", "Aktueller Vergleich mit fünf Aufgaben", "現行の 5 タスク比較", "현재 5개 작업 비교", "Текущее сравнение по пяти задачам",
	}},
	{KeyAgenticLocal5SectionBeforeAfter, [6]string{
		"Luban before and after", "Luban 优化前后", "Luban vor und nach der Optimierung", "Luban の最適化前後", "Luban 최적화 전후", "Luban до и после оптимизации",
	}},
	{KeyAgenticLocal5SectionHistoricalToolRootCause, [6]string{
		"Why historical tool calls multiplied", "历史工具调用为何成倍增加", "Warum sich historische Tool-Aufrufe vervielfachten", "従来のツール呼び出しが増幅した理由", "기존 도구 호출이 급증한 원인", "Почему ранее число вызовов инструментов многократно росло",
	}},
	{KeyAgenticLocal5SectionKeyOptimizations, [6]string{
		"Key harness optimizations", "Harness 关键优化", "Wesentliche Harness-Optimierungen", "Harness の主要な最適化", "Harness 핵심 최적화", "Ключевые оптимизации Harness",
	}},
	{KeyAgenticLocal5SectionRawEvidence, [6]string{
		"Raw evidence and integrity", "原始证据与完整性", "Rohbelege und Integrität", "生の証拠と完全性", "원시 증거 및 무결성", "Исходные доказательства и целостность",
	}},
	{KeyAgenticLocal5SectionConclusion, [6]string{
		"Current conclusion", "当前结论", "Aktuelles Fazit", "現時点の結論", "현재 결론", "Текущий вывод",
	}},
	{KeyAgenticLocal5SectionEfficiencyDiagnosis, [6]string{
		"Efficiency diagnosis", "效率诊断", "Effizienzdiagnose", "効率診断", "효율 진단", "Диагностика эффективности",
	}},
	{KeyAgenticLocal5SectionOptimizationEvidence, [6]string{
		"Optimization evidence ledger", "优化证据台账", "Nachweisregister der Optimierungen", "最適化の証拠台帳", "최적화 증거 대장", "Реестр доказательств оптимизаций",
	}},
	{KeyAgenticLocal5SectionSharedPassComparison, [6]string{
		"Efficiency on tasks passed by both agents", "双方共同通过题目的效率对比", "Effizienz bei Aufgaben, die beide Agents bestanden haben", "両 Agent がともに合格したタスクの効率比較", "두 Agent가 모두 통과한 작업의 효율 비교", "Сравнение эффективности на задачах, пройденных обоими агентами",
	}},

	{KeyAgenticLocal5CaveatSample5Nonofficial, [6]string{
		"This is a development pilot with five locally selected tasks. It cannot be extrapolated to a full or published benchmark score and does not establish a formal winner.",
		"这是仅含 5 道本地选题的开发 pilot；结果不可外推为完整或公开测评分数，也不能据此确立正式胜者。",
		"Dies ist ein Entwicklungspilot mit fünf lokal ausgewählten Aufgaben. Die Ergebnisse lassen sich weder auf einen vollständigen oder veröffentlichten Benchmarkwert hochrechnen noch begründen sie einen formellen Sieger.",
		"これはローカルで選んだ 5 タスクによる開発 pilot です。完全版または公開ベンチマークのスコアには外挿できず、正式な勝者を確定するものでもありません。",
		"로컬에서 선정한 작업 5개로 진행한 개발 pilot입니다. 전체 또는 공개 벤치마크 점수로 일반화할 수 없으며 공식 승자를 확정하지 않습니다.",
		"Это пилотный прогон для разработки на пяти локально выбранных задачах. Его результаты нельзя экстраполировать на полную или опубликованную оценку бенчмарка и считать формальным определением победителя.",
	}},
	{KeyAgenticLocal5CaveatToolDiagnosticNotComparable, [6]string{
		"Historical and current tool-event figures use different catalogs, lifecycle stages, or instrumentation. They are diagnostic context, not like-for-like headline evidence.",
		"历史与当前工具事件数字采用不同的工具目录、生命周期阶段或埋点口径，仅用于诊断背景，不属于可直接同口径对比的核心证据。",
		"Historische Zahlen zu Tool-Aufrufen stammen aus unterschiedlichen Tool-Katalogen oder Messverfahren. Sie dienen als Diagnosekontext und nicht als direkt vergleichbarer Hauptnachweis.",
		"従来のツール呼び出し数は、異なるツールカタログまたは計測方式から得られたものです。診断用の背景情報であり、同一条件で比較できる主要証拠ではありません。",
		"기존 도구 호출 수치는 서로 다른 도구 카탈로그나 계측 방식에서 나온 값입니다. 진단 맥락일 뿐, 동일 조건의 핵심 비교 증거가 아닙니다.",
		"Исторические показатели вызовов инструментов получены при разных каталогах инструментов или способах инструментирования. Это диагностический контекст, а не напрямую сопоставимое основное доказательство.",
	}},
	{KeyAgenticLocal5CaveatCostFormulaEstimate, [6]string{
		"The deduplicated comparable estimate charges each visible all-transport input class once at frozen gateway rates; the legacy runner receipt is shown separately because it adds the cache-write premium to an uncached base that already contains W. Neither value is a provider invoice.",
		"去重后的可比估算按冻结网关费率对各类全传输可见输入 token 各计费一次；旧 runner 回执另行展示，因为它在已包含 W 的未缓存基数上再次追加缓存写入费率。两者都不是 Provider 账单。",
		"Die deduplizierte vergleichbare Schätzung berechnet jede sichtbare Input-Klasse über alle Transporte genau einmal zu fixierten Gateway-Tarifen. Der alte Runner-Beleg wird separat gezeigt, weil er den Cache-Write-Aufschlag auf eine ungecachte Basis addiert, die W bereits enthält. Keiner der Werte ist eine Provider-Rechnung.",
		"重複排除した比較用見積もりは、全 transport で観測できる各 input 区分を固定 gateway レートで一度だけ課金します。旧 runner の回収値は、W を既に含む未キャッシュ基数に cache-write 料金を再加算するため別表示です。どちらも Provider の請求書ではありません。",
		"중복 제거 비교 추정치는 모든 transport에서 관측된 각 입력 분류를 고정 gateway 요율로 한 번씩만 계산합니다. 기존 runner 영수증은 W가 이미 포함된 미캐시 기준에 cache-write 요율을 다시 더하므로 별도로 표시합니다. 둘 다 Provider 청구서는 아닙니다.",
		"Дедуплицированная сопоставимая оценка учитывает каждый видимый класс входных токенов по всем transport ровно один раз по фиксированным тарифам gateway. Старое значение runner показано отдельно, поскольку оно добавляет надбавку за запись в кэш к некэшированной базе, уже содержащей W. Ни одно значение не является счётом Provider.",
	}},
	{KeyAgenticLocal5CaveatExactLLMDefinition, [6]string{
		"The local exact metric is each meter-recorded outbound HTTP POST /responses attempt. The report recomputes it from provider-requests.jsonl and separates HTTP 2xx from non-2xx; 2xx does not prove a completed model response, and the meter is not a start-time WAL.",
		"本机精确口径是 meter 已记录的每次出站 HTTP POST /responses 尝试。报告从 provider-requests.jsonl 重新计算，并区分 HTTP 2xx 与非 2xx；2xx 不证明模型响应完整，且该 meter 不是请求启动时 WAL。",
		"Die lokale exakte Metrik zählt jeden vom Meter aufgezeichneten ausgehenden HTTP-POST-/responses-Versuch. Der Bericht berechnet sie aus provider-requests.jsonl neu und trennt HTTP 2xx von Nicht-2xx. 2xx beweist keine vollständig abgeschlossene Modellantwort, und das Meter ist kein WAL zum Startzeitpunkt.",
		"ローカルの正確な指標は、meter に記録された外向き HTTP POST /responses 試行の各件です。レポートは provider-requests.jsonl から再計算し、HTTP 2xx と非 2xx を分離します。2xx はモデル応答の完了を証明せず、この meter は開始時 WAL ではありません。",
		"로컬 정확 지표는 meter에 기록된 각 외부 HTTP POST /responses 시도입니다. 보고서는 provider-requests.jsonl에서 이를 재계산하고 HTTP 2xx와 비 2xx를 분리합니다. 2xx는 모델 응답 완료를 증명하지 않으며 meter는 시작 시점 WAL이 아닙니다.",
		"Локальная точная метрика — каждая зарегистрированная счётчиком исходящая попытка HTTP POST /responses. Отчёт пересчитывает её по provider-requests.jsonl и разделяет HTTP 2xx и не-2xx. Код 2xx не доказывает завершённость ответа модели, а счётчик не является WAL на момент старта.",
	}},
	{KeyAgenticLocal5CaveatIncompleteRefusal, [6]string{
		"Missing, malformed, unsealed, or incompletely paired evidence is reported as N/A and blocks a comparative conclusion; it is never converted to zero or imputed.",
		"缺失、格式错误、未封存或配对不完整的证据一律报告为 N/A，并阻断比较结论；绝不会转写为零或进行插补。",
		"Fehlende, fehlerhafte, unversiegelte oder unvollständig gepaarte Nachweise werden als N/A ausgewiesen und verhindern eine vergleichende Schlussfolgerung; sie werden niemals als null behandelt oder imputiert.",
		"欠落、不正、未 seal、またはペアが不完全な証拠は N/A として報告し、比較結論を出しません。ゼロへの置換や補完は行いません。",
		"누락되었거나 형식이 잘못되었거나 seal되지 않았거나 쌍이 불완전한 증거는 N/A로 보고하고 비교 결론을 차단합니다. 0으로 바꾸거나 대체 추정하지 않습니다.",
		"Отсутствующие, некорректные, незапечатанные или не полностью попарные доказательства отмечаются как N/A и блокируют сравнительный вывод; они никогда не заменяются нулём и не восстанавливаются расчётно.",
	}},
	{KeyAgenticLocal5CaveatAttributionBundle, [6]string{
		"Historical mechanism cards are source-backed hypotheses. Observed before-and-after deltas apply to the optimization bundle and do not isolate each change without a controlled A/B ablation.",
		"历史机制卡片是有源码支持的假设；已观察到的优化前后差值归属于整套优化 bundle，没有受控 A/B 消融时不能单独归因到每项变更。",
		"Die historischen Mechanismuskarten sind durch Quellcode gestützte Hypothesen. Beobachtete Vorher-Nachher-Differenzen gelten für das gesamte Optimierungsbündel und isolieren ohne kontrollierte A/B-Ablation keine einzelne Änderung.",
		"過去のメカニズムカードはソースに裏付けられた仮説です。観測された前後差分は最適化 bundle 全体に対するもので、統制 A/B アブレーションなしに各変更を個別帰属できません。",
		"기존 메커니즘 카드는 소스에 근거한 가설입니다. 관측된 전후 차이는 최적화 bundle 전체에 해당하며 통제된 A/B 절제 실험 없이는 개별 변경 효과를 분리할 수 없습니다.",
		"Исторические карточки механизмов — гипотезы, подтверждённые исходным кодом. Наблюдаемые различия до и после относятся ко всему пакету оптимизаций и без контролируемой A/B-абляции не изолируют отдельные изменения.",
	}},
	{KeyAgenticLocal5CaveatBaselineSecurityIncident, [6]string{
		"The historical baseline is a contaminated diagnostic baseline: its Fabric Luban trajectory exposed 12 inherited credential values to the model endpoint, followed by redaction across 18 delivered files and an environment allowlist. Its metrics remain visible but cannot support causal or formal improvement claims.",
		"历史基线属于受污染的诊断基线：其中 Fabric 的 Luban 轨迹把 12 个继承凭据值发送到了模型端点，随后对 18 个交付文件进行了脱敏并启用环境变量 allowlist。相关数字保留展示，但不能支撑因果或正式提升结论。",
		"Die historische Basis ist eine kontaminierte Diagnosebasis: Im Fabric-Luban-Lauf gelangten 12 geerbte Zugangswerte an den Modellendpunkt; danach wurden 18 ausgelieferte Dateien bereinigt und eine Umgebungs-Allowlist eingeführt. Die Werte bleiben sichtbar, tragen aber keine kausalen oder formellen Verbesserungsbehauptungen.",
		"過去のベースラインは汚染された診断用基準です。Fabric の Luban 軌跡では継承された認証情報 12 件がモデル endpoint に送られ、その後 18 個の配布ファイルを編集し環境 allowlist を導入しました。数値は表示しますが、因果的または正式な改善主張の根拠にはできません。",
		"기존 기준선은 오염된 진단 기준선입니다. Fabric Luban 궤적에서 상속된 자격 증명 값 12개가 모델 endpoint로 전송되었고, 이후 전달 파일 18개를 수정하고 환경 allowlist를 도입했습니다. 수치는 공개하지만 인과적·공식 개선 주장을 뒷받침하지 못합니다.",
		"Историческая база загрязнена и годится лишь для диагностики: в траектории Fabric Luban 12 унаследованных значений учётных данных попали на endpoint модели; затем данные были удалены из 18 файлов и введён allowlist окружения. Метрики показаны, но не подтверждают причинное или формальное улучшение.",
	}},
	{KeyAgenticLocal5CaveatSharedPassScope, [6]string{
		"This section is conditioned on quality: it includes only the %d tasks that both agents passed under the strict evaluator, with one observed run per agent–task pair. It does not estimate unconditional benchmark efficiency or statistical superiority.",
		"本节以质量为前提：仅纳入双方均经严格评测器判定通过的 %d 道题，每个 Agent–题目组合只有 1 次观测运行。因此不能据此估计无条件的整体测评效率或统计显著优势。",
		"Dieser Abschnitt ist qualitätskonditioniert: Er umfasst nur die %d Aufgaben, die beide Agents nach der strengen Bewertung bestanden haben, mit genau einem beobachteten Lauf je Agent-Aufgaben-Paar. Daraus lassen sich weder die unbedingte Benchmark-Effizienz noch statistische Überlegenheit ableiten.",
		"この節は品質を条件とし、厳格な評価で両 Agent がともに合格した %d タスクのみを対象にします。観測は Agent–タスクの各組み合わせにつき 1 回だけです。ベンチマーク全体の無条件の効率や統計的優位性を推定するものではありません。",
		"이 절은 품질을 조건으로 하며, 엄격한 평가에서 두 Agent가 모두 통과한 %d개 작업만 포함합니다. 관측은 Agent–작업 쌍당 1회뿐입니다. 전체 벤치마크의 무조건적 효율이나 통계적 우위를 추정하지 않습니다.",
		"Этот раздел содержит сравнение при условии качества: включены только %d задач, которые оба агента прошли по строгой оценке, по одному наблюдаемому запуску на каждую пару агент–задача. Эти данные не позволяют оценивать безусловную эффективность по всему бенчмарку или статистическое превосходство.",
	}},
	{KeyAgenticLocal5CaveatSharedPassToolTaxonomy, [6]string{
		"Codex and Luban expose different tool catalogs and event taxonomies. Tool-call totals and per-tool counts in this section are diagnostic only; they are not like-for-like efficiency evidence.",
		"Codex 与 Luban 暴露的工具目录和事件分类口径不同。本节的工具调用总数与分工具计数仅供诊断，不属于可同口径比较的效率证据。",
		"Codex und Luban verwenden unterschiedliche Tool-Kataloge und Ereignistaxonomien. Gesamtzahlen und Aufschlüsselungen der Tool-Aufrufe in diesem Abschnitt dienen nur der Diagnose und sind kein direkt vergleichbarer Effizienznachweis.",
		"Codex と Luban ではツールカタログとイベント分類が異なります。この節のツール呼び出し総数とツール別件数は診断専用であり、同一条件の効率証拠ではありません。",
		"Codex와 Luban은 서로 다른 도구 카탈로그와 이벤트 분류 체계를 사용합니다. 이 절의 도구 호출 합계와 도구별 수치는 진단용일 뿐, 동일 기준의 효율 근거가 아닙니다.",
		"Codex и Luban используют разные каталоги инструментов и таксономии событий. Общее число и разбивка вызовов инструментов в этом разделе предназначены только для диагностики и не являются сопоставимым доказательством эффективности.",
	}},
	{KeyAgenticLocal5SharedPassLongerSlower, [6]string{
		"Input per POST is close (Codex %s, Luban %s; %s), while Luban uses more provider time per POST (Codex %s, Luban %s; %s) and more output tokens per POST (Codex %s, Luban %s; %s). With solved quality held constant, this two-task sample associates Luban's extra time and cost with longer, slower model responses; it does not establish causation.",
		"每次 POST 的输入量接近（Codex %s，Luban %s；%s），但 Luban 每次 POST 的 Provider 耗时更高（Codex %s，Luban %s；%s），输出 token 也更多（Codex %s，Luban %s；%s）。在解题质量保持一致后，这个两题样本显示 Luban 的额外耗时与费用伴随更长、更慢的模型响应，但不能据此确立因果关系。",
		"Die Eingabemenge je POST ist ähnlich (Codex %s, Luban %s; %s), während Luban mehr Provider-Zeit je POST (Codex %s, Luban %s; %s) und mehr Ausgabe-Token je POST benötigt (Codex %s, Luban %s; %s). Bei konstant gehaltener Lösungsqualität bringt diese Zwei-Aufgaben-Stichprobe Lubans zusätzliche Zeit und Kosten mit längeren, langsameren Modellantworten in Verbindung; Kausalität ist damit nicht belegt.",
		"POST 当たりの入力量は近い値です（Codex %s、Luban %s、%s）。一方、Luban は POST 当たりの Provider 時間（Codex %s、Luban %s、%s）と出力 token（Codex %s、Luban %s、%s）が多くなっています。解決品質を一定にしたこの 2 タスク標本では、Luban の追加時間と費用はより長く遅いモデル応答に伴っていますが、因果関係を示すものではありません。",
		"POST당 입력량은 비슷하지만(Codex %s, Luban %s, %s), Luban은 POST당 Provider 시간(Codex %s, Luban %s, %s)과 출력 token(Codex %s, Luban %s, %s)이 더 많습니다. 해결 품질을 동일하게 둔 이 2개 작업 표본에서는 Luban의 추가 시간과 비용이 더 길고 느린 모델 응답과 함께 나타나지만 인과관계를 입증하지는 않습니다.",
		"Объём входа на POST близок (Codex %s, Luban %s; %s), однако Luban расходует больше времени Provider на POST (Codex %s, Luban %s; %s) и выдаёт больше выходных токенов на POST (Codex %s, Luban %s; %s). При одинаковом качестве решения эта выборка из двух задач связывает дополнительные время и стоимость Luban с более длинными и медленными ответами модели, но не доказывает причинность.",
	}},
	{KeyAgenticLocal5SharedPassNormalizedNeutral, [6]string{
		"Per-solved-task and per-POST normalizations are derived dynamically. They separate task-count effects from call intensity, but one run per pair cannot establish a stable mechanism.",
		"每道已解决题目与每次 POST 的归一化指标均动态派生；它们可区分题目数量影响与调用强度，但每个配对只有一次运行，无法确立稳定机制。",
		"Die Normalisierungen je gelöster Aufgabe und je POST werden dynamisch abgeleitet. Sie trennen Effekte der Aufgabenanzahl von der Aufrufintensität, doch ein Lauf je Paar kann keinen stabilen Mechanismus belegen.",
		"解決タスク当たりおよび POST 当たりの正規化値は動的に導出されます。タスク数の影響と呼び出し強度を分離できますが、各ペア 1 回の実行では安定した仕組みを立証できません。",
		"해결 작업당 및 POST당 정규화 값은 동적으로 산출됩니다. 작업 수 효과와 호출 강도를 구분하지만 쌍당 1회 실행만으로는 안정적인 메커니즘을 입증할 수 없습니다.",
		"Нормировки на решённую задачу и на POST вычисляются динамически. Они отделяют влияние числа задач от интенсивности вызовов, но один запуск на пару не позволяет установить устойчивый механизм.",
	}},

	{KeyAgenticLocal5ConclusionNoComprehensiveSuperiority, [6]string{
		"Luban has not demonstrated comprehensive superiority over Codex.", "Luban 尚未证明全面超越 Codex。", "Luban hat keine umfassende Überlegenheit gegenüber Codex nachgewiesen.", "Luban が Codex を全面的に上回ることは実証されていません。", "Luban이 Codex를 전면적으로 능가한다는 점은 입증되지 않았습니다.", "Всестороннее превосходство Luban над Codex не доказано.",
	}},
	{KeyAgenticLocal5ConclusionMeasured, [6]string{
		"Strict raw local projection: Luban %d/%d versus Codex %d/%d; meter-recorded POSTs %d versus %d (%s), task duration %s, and comparable estimated cost %s.",
		"本机严格原始投影：Luban %d/%d，Codex %d/%d；meter 已记录 POST 为 %d 对 %d（%s），任务耗时 %s，可比估算费用 %s。",
		"Strikte lokale Rohprojektion: Luban %d/%d gegenüber Codex %d/%d; aufgezeichnete POSTs %d gegenüber %d (%s), Aufgabendauer %s und vergleichbare geschätzte Kosten %s.",
		"厳密なローカル生投影：Luban %d/%d、Codex %d/%d。記録済み POST は %d 対 %d（%s）、タスク所要時間は %s、比較用推定費用は %s です。",
		"엄격한 로컬 원시 투영: Luban %d/%d, Codex %d/%d; 기록된 POST는 %d 대 %d(%s), 작업 소요 시간은 %s, 비교 추정 비용은 %s입니다.",
		"Строгая локальная сырая проекция: Luban %d/%d против Codex %d/%d; записанные POST — %d против %d (%s), время выполнения задачи — %s, сопоставимая оценка стоимости — %s.",
	}},
	{KeyAgenticLocal5ConclusionCacheContext, [6]string{
		"Cache ratio is %s in Luban's favor, but cached-token volume is also higher (%d versus %d) because Luban sends substantially more input; this is not an efficiency win by itself.",
		"缓存率高出 %s，但 Luban 的缓存 token 绝对量也更高（%d 对 %d），原因是其输入显著更多；这本身不能视为效率胜出。",
		"Die Cache-Quote liegt um %s zugunsten von Luban, doch wegen deutlich mehr Input ist auch das Cache-Token-Volumen höher (%d gegenüber %d); allein ist das kein Effizienzgewinn.",
		"cache 率は Luban が %s 高い一方、入力が大幅に多いため cache token 絶対量も多く（%d 対 %d）、それだけでは効率上の勝利ではありません。",
		"cache 비율은 Luban이 %s 높지만 입력량이 훨씬 많아 cached token 절대량도 더 큽니다(%d 대 %d). 이것만으로 효율 우위는 아닙니다.",
		"Доля кэша выше у Luban на %s, но из-за большего входа выше и объём кэшированных токенов (%d против %d); само по себе это не выигрыш в эффективности.",
	}},
	{KeyAgenticLocal5ConclusionRawProjectionScope, [6]string{
		"The 2/5 and 3/5 figures are strict projections from five of five complete local evaluator partitions. They remain non-official pilot results, not a public benchmark score.",
		"2/5 与 3/5 来自 5/5 完整本地 evaluator 分区的严格投影；它们仍是非官方 pilot 结果，不是公开测评分数。",
		"Die Werte 2/5 und 3/5 sind strikte Projektionen aus fünf von fünf vollständigen lokalen Evaluator-Partitionen. Sie bleiben nicht offizielle Pilotergebnisse und sind kein öffentlicher Benchmarkwert.",
		"2/5 と 3/5 は 5/5 の完全なローカル evaluator 分割から得た厳密な投影です。非公式 pilot の結果であり、公開ベンチマークスコアではありません。",
		"2/5와 3/5는 5/5 완전한 로컬 evaluator 분할의 엄격한 투영입니다. 여전히 비공식 pilot 결과이며 공개 벤치마크 점수가 아닙니다.",
		"Значения 2/5 и 3/5 — строгая проекция пяти из пяти полных локальных разделов evaluator. Это по-прежнему неофициальные результаты пилота, а не публичная оценка бенчмарка.",
	}},

	{KeyAgenticLocal5EfficiencyPrimaryConclusion, [6]string{
		"Tool wrappers fell, but model rounds, output, task duration, and cost rose. The new harness improved packaging, not end-to-end efficiency in this pilot.",
		"工具 wrapper 数下降了，但模型轮次、输出、任务耗时与费用均上升；本 pilot 改善的是包装层，而不是端到端效率。",
		"Die Tool-Wrapper nahmen ab, Modellrunden, Output, Aufgabendauer und Kosten jedoch zu. In diesem Pilot verbesserte das neue Harness die Verpackung, nicht die Ende-zu-Ende-Effizienz.",
		"ツール wrapper 数は減りましたが、モデル回数、出力、タスク所要時間、費用は増えました。この pilot で改善したのは包装層であり、end-to-end 効率ではありません。",
		"tool wrapper 수는 줄었지만 모델 라운드, 출력, 작업 소요 시간, 비용은 늘었습니다. 이 pilot에서 새 harness는 포장 계층만 개선했고 end-to-end 효율은 개선하지 못했습니다.",
		"Число wrapper инструментов снизилось, но выросли раунды модели, вывод, время выполнения задачи и стоимость. В этом пилоте улучшилась упаковка, а не сквозная эффективность.",
	}},
	{KeyAgenticLocal5EfficiencyCompletionTail, [6]string{
		"Upper-bound completion-shaped tail: %d calls, %.0f seconds, $%.4f, with %d completion rejections. This segment is correlated with rejected completion candidates, but cannot be assumed wholly wasted.",
		"completion-shaped tail 上界：%d 次调用、%.0f 秒、$%.4f，并出现 %d 次 completion rejection。该区段与被拒绝的完成候选相关，但不能假设全部都是浪费。",
		"Obergrenze des completion-förmigen Tails: %d Aufrufe, %.0f Sekunden, $%.4f und %d Completion-Ablehnungen. Der Abschnitt korreliert mit abgelehnten Abschlusskandidaten, ist aber nicht vollständig als Verschwendung anzusetzen.",
		"completion-shaped tail の上限は %d 回、%.0f 秒、$%.4f、completion rejection %d 回です。拒否された完了候補と相関しますが、全てが無駄とは仮定できません。",
		"completion-shaped tail 상한은 %d회, %.0f초, $%.4f이며 completion rejection은 %d회입니다. 거부된 완료 후보와 연관되지만 전부 낭비라고 가정할 수 없습니다.",
		"Верхняя граница completion-shaped tail: %d вызовов, %.0f секунд, $%.4f и %d отклонений завершения. Сегмент связан с отклонёнными кандидатами, но не весь является потерями.",
	}},
	{KeyAgenticLocal5EfficiencyTailUpperBound, [6]string{
		"This tail is an attribution upper bound, not a causal waste estimate: productive inspection, patching, verification, startup, I/O, and orchestration may be inside it.",
		"该 tail 只是归因上界，不是因果浪费估算；其中可能包含有产出的检查、修改、验证、启动、I/O 与编排。",
		"Dieser Tail ist eine Attributionsobergrenze, keine kausale Verlustschätzung; produktive Prüfung, Änderung, Verifikation, Start, I/O und Orchestrierung können enthalten sein.",
		"この tail は帰属の上限であり、因果的な無駄の推定ではありません。生産的な検査、変更、検証、起動、I/O、オーケストレーションを含み得ます。",
		"이 tail은 귀속 상한이지 인과적 낭비 추정치가 아닙니다. 생산적인 검사, 수정, 검증, 시작, I/O, 오케스트레이션이 포함될 수 있습니다.",
		"Этот tail — верхняя граница атрибуции, а не причинная оценка потерь; он может включать полезные проверки, изменения, верификацию, запуск, I/O и оркестрацию.",
	}},
	{KeyAgenticLocal5EfficiencyFlightProof, [6]string{
		"Flight rejects completion after writes until revision-bound verification exists. This guards stale completion, but every write path must emit a correct write-effect receipt; exhaustive coverage remains a proof obligation. In this pilot only 5/43 Run results were revision_bound and 37/43 were committed_unverified.",
		"Flight 会在写入后拒绝 completion，直到取得绑定 revision 的验证；这能防止过期完成，但每条写路径都必须正确发出 write-effect 回执，完备覆盖仍是待证明义务。本 pilot 的 43 次 Run 中仅 5 次为 revision_bound，37 次为 committed_unverified。",
		"Flight lehnt Abschlüsse nach Schreibvorgängen bis zur revisionsgebundenen Verifikation ab. Das schützt vor veralteten Abschlüssen, verlangt aber korrekte Write-Effect-Belege für jeden Schreibpfad; vollständige Abdeckung bleibt zu beweisen. Nur 5/43 Run-Ergebnisse waren revision_bound, 37/43 committed_unverified.",
		"Flight は書込み後、revision に結び付いた検証まで completion を拒否します。陳腐化した完了を防ぎますが、全書込み経路が正しい write-effect receipt を出す必要があり、網羅性は未証明です。本 pilot では Run 43 件中 revision_bound は 5 件、committed_unverified は 37 件でした。",
		"Flight는 쓰기 후 revision 결합 검증이 있을 때까지 completion을 거부합니다. 오래된 완료를 막지만 모든 쓰기 경로가 올바른 write-effect receipt를 내야 하며 완전한 커버리지는 입증 과제입니다. 이 pilot에서 Run 43건 중 revision_bound는 5건, committed_unverified는 37건입니다.",
		"Flight отклоняет завершение после записи до проверки, связанной с revision. Это защищает от устаревшего завершения, но каждый путь записи должен выдавать корректную квитанцию write-effect; полнота остаётся обязанностью доказательства. В пилоте revision_bound были 5/43 Run, committed_unverified — 37/43.",
	}},

	{KeyAgenticLocal5ScopeTrajectoryDiagnostic, [6]string{"Trajectory diagnostic; taxonomy differs", "轨迹诊断；taxonomy 不同", "Trajektoriendiagnose; Taxonomie verschieden", "軌跡診断、taxonomy は異なる", "궤적 진단; taxonomy 상이", "Диагностика траектории; таксономии различаются"}},
	{KeyAgenticLocal5ScopeUnitFixture, [6]string{"Deterministic unit fixture", "确定性 unit fixture", "Deterministisches Unit-Fixture", "決定的 unit fixture", "결정적 unit fixture", "Детерминированный unit fixture"}},
	{KeyAgenticLocal5ScopeSyntheticFixture, [6]string{"Synthetic compaction fixture", "合成 compaction fixture", "Synthetisches Compaction-Fixture", "合成 compaction fixture", "합성 compaction fixture", "Синтетический compaction fixture"}},
	{KeyAgenticLocal5ScopeFieldDiagnostic, [6]string{"Single field observation; no ablation", "单次现场观察；无消融", "Einzelbeobachtung; keine Ablation", "単一の現場観測、ablation なし", "단일 현장 관측; ablation 없음", "Единичное полевое наблюдение; без абляции"}},

	{KeyAgenticLocal5LimitationEvaluatorSemantics, [6]string{
		"Evaluator semantics are RepoLaunch-compatible: a partially applied candidate patch may still score if production hunks apply; a nonzero test process may pass after parsed test projection; and the Skim timeout still scores the patch produced before timeout. These states must not be read as ordinary clean exits.",
		"Evaluator 采用 RepoLaunch 兼容语义：candidate patch 即使仅部分应用，只要生产代码 hunk 生效仍可能计分；测试进程非 0 时也可能依据解析后的测试投影通过；Skim 超时仍会评估超时前产生的 patch。这些状态不能当作普通的干净退出。",
		"Der Evaluator folgt RepoLaunch-kompatibler Semantik: Ein teilweise angewandter Kandidaten-Patch kann zählen, wenn Produktions-Hunks greifen; ein Testprozess ungleich null kann nach Testprojektion bestehen; beim Skim-Timeout wird der zuvor erzeugte Patch bewertet. Das sind keine normalen sauberen Exits.",
		"Evaluator は RepoLaunch 互換の意味論です。production hunk が適用されれば candidate patch の部分適用でも採点され、test process が非 0 でも解析済み test 投影で合格し得ます。Skim の timeout もそれ以前の patch を採点します。通常の clean exit と解釈できません。",
		"Evaluator는 RepoLaunch 호환 의미론을 사용합니다. production hunk가 적용되면 candidate patch가 부분 적용되어도 채점될 수 있고, test process가 0이 아니어도 파싱된 test 투영으로 통과할 수 있으며, Skim timeout도 그 전의 patch를 채점합니다. 일반적인 clean exit로 읽으면 안 됩니다.",
		"Evaluator использует семантику RepoLaunch: частично применённый patch может быть засчитан при применении production-hunk; ненулевой тестовый процесс может пройти по разобранной проекции; timeout Skim оценивает созданный до него patch. Это не обычные чистые завершения.",
	}},
	{KeyAgenticLocal5LimitationExperimentalDesign, [6]string{
		"Tasks were locally selected, not random or held out. There is one run per agent–task pair, and local concurrency and execution order were not paired or counterbalanced; variance, order effects, and statistical superiority are unknown.",
		"题目由本地选取，并非随机或 held-out；每个 agent–task 组合仅运行 1 次，本机并发与执行顺序也未配对或交叉平衡，因此方差、顺序效应与统计优越性均未知。",
		"Die Aufgaben wurden lokal ausgewählt, nicht zufällig oder held-out. Pro Agent-Aufgabe gibt es einen Lauf; lokale Parallelität und Reihenfolge waren weder gepaart noch gegenbalanciert. Varianz, Reihenfolgeeffekte und statistische Überlegenheit sind unbekannt.",
		"タスクはローカル選択で、ランダムでも held-out でもありません。agent–task ごとに 1 回のみで、ローカル並行性と実行順序もペア化・カウンターバランスされていません。分散、順序効果、統計的優位性は不明です。",
		"작업은 로컬 선택이며 무작위나 held-out이 아닙니다. agent–task 쌍당 1회만 실행했고 로컬 동시성과 실행 순서도 페어링·교차 균형되지 않았습니다. 분산, 순서 효과, 통계적 우월성은 알 수 없습니다.",
		"Задачи выбраны локально, не случайно и не held-out. На пару agent–task есть один запуск; локальная параллельность и порядок не были попарно сбалансированы. Дисперсия, эффект порядка и статистическое превосходство неизвестны.",
	}},
	{KeyAgenticLocal5LimitationModelUnverified, [6]string{
		"The report verifies request-side model slug, effort, and pinned client binaries, but local receipts do not attest the Provider-served model or weight snapshot. gpt-5.6-sol / xhigh is a request contract, not independent served-model proof.",
		"报告验证了请求侧 model slug、effort 与固定 client binary，但本地回执未证明 Provider 实际服务的模型或权重 snapshot；gpt-5.6-sol / xhigh 是请求契约，不是独立的服务模型证明。",
		"Der Bericht prüft Modell-Slug, Effort und fixierte Client-Binaries auf Anfrageseite, attestiert aber weder das bereitgestellte Modell noch den Gewichts-Snapshot. gpt-5.6-sol / xhigh ist ein Anfragevertrag, kein unabhängiger Nachweis.",
		"レポートは要求側 model slug、effort、固定 client binary を検証しますが、Provider が提供したモデルや weight snapshot は証明しません。gpt-5.6-sol / xhigh は要求契約であり、独立した提供モデル証明ではありません。",
		"보고서는 요청 측 model slug, effort, 고정 client binary를 검증하지만 Provider 제공 모델이나 weight snapshot을 증명하지 않습니다. gpt-5.6-sol / xhigh는 요청 계약이지 독립적인 제공 모델 증명이 아닙니다.",
		"Отчёт проверяет slug, effort и бинарные файлы клиента на стороне запроса, но не удостоверяет обслуживавшую модель или snapshot весов. gpt-5.6-sol / xhigh — контракт запроса, не независимое доказательство модели.",
	}},
	{KeyAgenticLocal5LimitationMutableRawNoManifest, [6]string{
		"Raw and source links point to mutable local files and are not bound by an immutable SHA-256 manifest or candidate-binary source/build receipt. They aid inspection but do not prove snapshot consistency or end-to-end provenance.",
		"raw 与源码链接指向可变的本地文件，且没有不可变 SHA-256 manifest 或 candidate binary 的源码/构建回执绑定；它们便于核查，但不能证明 snapshot 一致性或端到端来源链。",
		"Raw- und Quelllinks zeigen auf veränderbare lokale Dateien und sind weder durch ein unveränderliches SHA-256-Manifest noch einen Source/Build-Beleg des Kandidaten gebunden. Sie helfen bei der Prüfung, beweisen aber keine Snapshot-Konsistenz oder Provenienz.",
		"raw と source のリンクは変更可能なローカルファイルを指し、不変 SHA-256 manifest や candidate binary の source/build receipt に結び付いていません。確認には使えますが、snapshot 一貫性や end-to-end 来歴は証明しません。",
		"raw 및 source 링크는 변경 가능한 로컬 파일을 가리키며 불변 SHA-256 manifest나 candidate binary source/build receipt에 결합되지 않습니다. 검토에는 도움이 되지만 snapshot 일관성이나 end-to-end 출처를 증명하지 않습니다.",
		"Ссылки raw и source ведут на изменяемые локальные файлы и не связаны неизменяемым SHA-256 manifest или квитанцией source/build бинарного кандидата. Они помогают проверке, но не доказывают согласованность snapshot и происхождение.",
	}},
	{KeyAgenticLocal5MetricMeterRecordedPOST, [6]string{
		"Meter-recorded POST /responses", "Meter 已记录的 POST /responses", "Vom Meter aufgezeichnete POST /responses", "meter 記録済み POST /responses", "meter 기록 POST /responses", "Записанные meter POST /responses",
	}},
	{KeyAgenticLocal5Footer, [6]string{
		"Generated as a self-contained local HTML snapshot · evidence as of %s · linked raw/source files are mutable and not a content-addressed manifest",
		"作为本机自包含 HTML snapshot 生成 · 证据截至 %s · 所链接 raw/source 文件可变，且不构成内容寻址 manifest",
		"Als eigenständiger lokaler HTML-Snapshot erzeugt · Evidenzstand %s · verknüpfte Raw-/Quelldateien sind veränderbar und kein inhaltsadressiertes Manifest",
		"自己完結型ローカル HTML snapshot として生成 · 証拠基準日時 %s · リンク先 raw/source は変更可能で content-addressed manifest ではありません",
		"독립형 로컬 HTML snapshot으로 생성 · 증거 기준 %s · 연결된 raw/source 파일은 변경 가능하며 content-addressed manifest가 아님",
		"Создан как автономный локальный HTML snapshot · данные на %s · связанные raw/source изменяемы и не являются content-addressed manifest",
	}},

	{KeyAgenticLocal5RootCauseFragmentedSurface, [6]string{
		"The old read, search, glob, edit, and shell surface split one coding intent across many schemas and model turns, forcing repeated discovery and replanning.",
		"旧的读取、搜索、glob、编辑和 shell 工具把一次编码意图拆散到多个 schema 与模型轮次中，迫使模型重复探索和重新规划。",
		"Die frühere Oberfläche für Lesen, Suchen, Glob, Bearbeiten und Shell verteilte eine Programmierabsicht auf viele Schemas und Modellrunden und erzwang wiederholte Erkundung und Neuplanung.",
		"従来の read、search、glob、edit、shell は、1 つの実装意図を多数の schema とモデルターンに分断し、探索と再計画の反復を招いていました。",
		"기존 read, search, glob, edit, shell 표면은 하나의 코딩 의도를 여러 schema와 모델 턴으로 분할해 탐색과 재계획을 반복하게 했습니다.",
		"Прежний набор средств чтения, поиска, glob, редактирования и shell дробил единый замысел изменения кода между множеством schema и ходов модели, вынуждая повторять исследование и перепланирование.",
	}},
	{KeyAgenticLocal5RootCauseSequentialMutation, [6]string{
		"Per-file sequential edits exposed partial workspace states. A late conflict could trigger repair calls, rereads, and repeated validation for changes that should have been one transaction.",
		"逐文件串行编辑会暴露部分完成的工作区状态；后期冲突可能触发修复调用、重新读取和重复验证，而这些变更本应属于同一事务。",
		"Sequenzielle Änderungen pro Datei legten partielle Workspace-Zustände offen. Ein später Konflikt konnte Reparaturaufrufe, erneutes Lesen und wiederholte Validierung für Änderungen auslösen, die eine einzige Transaktion sein sollten.",
		"ファイル単位の逐次編集では、途中状態の workspace が露出していました。後段の競合により、本来 1 つの transaction であるべき変更に修復呼び出し、再読込、再検証が発生していました。",
		"파일별 순차 편집은 일부만 변경된 workspace 상태를 노출했습니다. 뒤늦은 충돌로 인해 하나의 transaction이어야 할 변경에 복구 호출, 재읽기, 반복 검증이 발생할 수 있었습니다.",
		"Последовательное редактирование отдельных файлов оставляло workspace в частично изменённом состоянии. Поздний конфликт мог вызвать исправляющие вызовы, повторное чтение и повторную проверку изменений, которые должны были быть одной транзакцией.",
	}},
	{KeyAgenticLocal5RootCauseShellOverloading, [6]string{
		"A general shell mixed exploration, mutation, and verification behind opaque commands and noisy output, so dependencies were implicit and redundant checks were common.",
		"通用 shell 把探索、修改和验证混在不透明的命令与嘈杂输出之后，使依赖关系只能隐式表达，也更容易出现重复检查。",
		"Eine allgemeine Shell vermischte Erkundung, Änderung und Validierung hinter undurchsichtigen Befehlen und verrauschter Ausgabe. Dadurch blieben Abhängigkeiten implizit und redundante Prüfungen waren häufig.",
		"汎用 shell は探索・変更・検証を不透明なコマンドとノイズの多い出力の背後に混在させていたため、依存関係が暗黙的になり、重複確認が頻発していました。",
		"범용 shell은 탐색, 변경, 검증을 불투명한 명령과 잡음이 많은 출력 뒤에 섞어 두어 의존 관계가 암묵적이었고 중복 검사가 자주 발생했습니다.",
		"Универсальный shell скрывал исследование, изменение и проверку за непрозрачными командами и шумным выводом, поэтому зависимости оставались неявными, а проверки часто дублировались.",
	}},
	{KeyAgenticLocal5RootCauseTelemetryConflation, [6]string{
		"Earlier telemetry conflated provider requests, completed responses, retries, tool invocations, and physical operations. That obscured whether a reported 2× increase was model behavior, retry amplification, or counting semantics.",
		"早期 telemetry 混淆了 Provider 请求、完成响应、重试、工具调用和物理操作，因而无法判断所谓 2× 增长究竟来自模型行为、重试放大，还是计数语义。",
		"Frühere Telemetrie vermischte Provider-Anfragen, abgeschlossene Antworten, Wiederholungsversuche, Tool-Aufrufe und physische Operationen. Dadurch blieb unklar, ob ein gemeldeter Anstieg um 2× auf Modellverhalten, Retry-Verstärkung oder Zählsemantik zurückging.",
		"以前の telemetry は Provider request、完了応答、retry、ツール呼び出し、物理操作を混同していました。そのため、報告された 2× 増加がモデルの挙動、retry の増幅、計数定義のどれによるものか判別できませんでした。",
		"이전 telemetry는 Provider 요청, 완료 응답, 재시도, 도구 호출, 물리 작업을 혼합해 집계했습니다. 따라서 보고된 2× 증가가 모델 행동, 재시도 증폭, 집계 정의 중 무엇 때문인지 구분하기 어려웠습니다.",
		"Ранее телеметрия смешивала запросы Provider, завершённые ответы, повторные попытки, вызовы инструментов и физические операции. Поэтому было непонятно, чем вызван заявленный рост в 2×: поведением модели, усилением из-за повторов или правилами подсчёта.",
	}},

	{KeyAgenticLocal5OptimizationInspectIntegration, [6]string{
		"Inspect supports typed batches for bounded reading, search, and globbing. This reduces wrapper events, but the pilot's 54 partial results show that fewer model round trips are not yet demonstrated.",
		"Inspect 支持对有界读取、搜索和 glob 进行类型化批处理；它减少了 wrapper 事件，但本 pilot 的 54 次 partial 结果表明，模型往返是否减少尚未得到证明。",
		"Inspect bündelt begrenztes Lesen, Suchen und Globbing in einem typisierten Batch. Das reduziert Erkundungsrunden und bewahrt zugleich eindeutige Nachweise für jedes Ergebnis.",
		"Inspect は範囲制限付きの読込・検索・glob を型付きの 1 バッチに統合し、各結果の明示的な証拠を保ちながら探索の往復を削減します。",
		"Inspect는 범위가 제한된 읽기, 검색, glob을 하나의 typed batch로 통합해 각 결과의 명시적 증거를 유지하면서 탐색 왕복을 줄입니다.",
		"Inspect объединяет ограниченные чтение, поиск и glob в один типизированный пакет, сокращая циклы исследования и сохраняя явное доказательство для каждого результата.",
	}},
	{KeyAgenticLocal5OptimizationApplyPatchAtomic, [6]string{
		"ApplyPatch prevalidates a multi-file, multi-hunk patch, then commits files with best-effort rollback on failure. It narrows partial-state exposure but is not crash-atomic or externally invisible.",
		"ApplyPatch 会先校验完整的多文件、多 hunk patch，再逐文件提交并在失败时尽力回滚；它缩小了部分状态暴露，但并非崩溃原子或对外不可见的原子事务。",
		"ApplyPatch validiert und committet Änderungen über mehrere Dateien und Hunks atomar gegen Workspace-Snapshots. Partielle Änderungen und der Großteil der Reparaturschleifen entfallen.",
		"ApplyPatch は複数ファイル・複数 hunk の変更を workspace snapshot と照合し、原子的に検証・commit します。途中状態の編集と修復の反復をほぼ排除します。",
		"ApplyPatch는 여러 파일과 hunk의 변경을 workspace snapshot에 대조해 검증하고 원자적으로 commit하여 부분 편집 상태와 대부분의 복구 반복을 없앱니다.",
		"ApplyPatch проверяет изменения в нескольких файлах и hunk по snapshot workspace и фиксирует их атомарно, устраняя частичные правки и большую часть циклов исправления.",
	}},
	{KeyAgenticLocal5OptimizationRunVerification, [6]string{
		"Run expresses validation as structured steps and can parallelize independent checks. Revision binding is conditional: only 5/43 pilot results were revision_bound, while 37/43 were committed_unverified.",
		"Run 以结构化步骤表达验证并可并行独立检查；revision 绑定是有条件的：本 pilot 仅 5/43 次结果为 revision_bound，37/43 为 committed_unverified。",
		"Run beschreibt abhängigkeitsbewusste Validierung als strukturierte Schritte, kann unabhängige Prüfungen parallelisieren und liefert begrenzte Ergebnisse, die an die committete Revision gebunden sind.",
		"Run は依存関係を認識した検証を構造化ステップで表現し、独立した確認を並列化できます。結果は範囲制限され、commit 済み revision に結び付けられます。",
		"Run은 의존성을 인식하는 검증을 구조화된 단계로 표현하고 독립적인 검사를 병렬화할 수 있으며, commit된 revision에 연결된 제한된 결과를 반환합니다.",
		"Run задаёт проверку с учётом зависимостей в виде структурированных шагов, может выполнять независимые проверки параллельно и возвращает ограниченные результаты, привязанные к зафиксированной revision.",
	}},
	{KeyAgenticLocal5OptimizationThreeToolCatalog, [6]string{
		"The model-visible catalog is exactly Inspect, ApplyPatch, and Run with stable definitions. Fewer schemas reduce prompt weight, tool-choice entropy, and accidental orchestration branches.",
		"模型可见目录严格限定为定义稳定的 Inspect、ApplyPatch 和 Run；更少的 schema 降低了 prompt 负担、工具选择熵和意外编排分支。",
		"Der für das Modell sichtbare Katalog besteht exakt aus Inspect, ApplyPatch und Run mit stabilen Definitionen. Weniger Schemas verringern Prompt-Last, Entropie bei der Tool-Auswahl und unbeabsichtigte Orchestrierungszweige.",
		"モデルに見える catalog は、定義が安定した Inspect、ApplyPatch、Run の 3 つだけです。schema の削減により prompt の負荷、ツール選択のエントロピー、意図しないオーケストレーション分岐を減らします。",
		"모델에 보이는 catalog는 정의가 안정적인 Inspect, ApplyPatch, Run 세 가지로 정확히 제한됩니다. schema 수를 줄여 prompt 부담, 도구 선택 엔트로피, 의도하지 않은 오케스트레이션 분기를 낮춥니다.",
		"Видимый модели catalog состоит ровно из Inspect, ApplyPatch и Run со стабильными определениями. Меньшее число schema снижает объём prompt, неопределённость выбора инструментов и случайные ветви оркестрации.",
	}},
	{KeyAgenticLocal5OptimizationContinuationCacheLineage, [6]string{
		"Continuation identity, stable prompt prefixes, and cache lineage are bound across turns to prevent accidental forks and aim to improve prefix reuse; the pilot cache ratio nevertheless fell by 0.5 percentage points versus the old Luban run.",
		"跨轮绑定 continuation 身份、稳定 prompt 前缀与缓存 lineage，旨在防止意外分叉并提高前缀复用；但本 pilot 相比旧 Luban 的缓存率仍下降了 0.5 个百分点。",
		"Continuation-Identität, stabile Prompt-Präfixe und Cache-Lineage werden über Runden hinweg gebunden. Das verhindert unbeabsichtigte Kontextabzweigungen und verbessert Cache-Treffer für wiederverwendbare Präfixe.",
		"continuation の同一性、安定した prompt prefix、cache lineage をターン間で結び付け、意図しないコンテキスト分岐を防ぎ、再利用可能な prefix の cache hit を改善します。",
		"continuation 식별자, 안정적인 prompt prefix, cache lineage를 턴 사이에 결합해 의도하지 않은 컨텍스트 분기를 막고 재사용 가능한 prefix의 cache hit를 높입니다.",
		"Идентичность continuation, стабильные префиксы prompt и lineage кэша связываются между ходами, предотвращая случайное ветвление контекста и повышая попадания кэша для повторно используемых префиксов.",
	}},
	{KeyAgenticLocal5OptimizationUnifiedAttemptRetry, [6]string{
		"The attempt controller enforces a shared retry budget and typed transport outcomes. The cited code does not yet prove one end-to-end identity through downstream tool effects; this pilot observed zero transport retries.",
		"attempt controller 统一约束重试预算并记录类型化 transport 结果；所引用源码尚不能证明身份贯穿到下游工具影响，本 pilot 观察到的 transport retry 为 0。",
		"Eine einheitliche Attempt-Identität verknüpft ausgehende Anfrage, Provider-Ergebnis, Retry, Continuation und nachgelagerte Tool-Effekte. Das ermöglicht Deduplizierung und eine exakte Messung der Retry-Verstärkung.",
		"単一の attempt identity で外向き request、Provider の結果、retry、continuation、下流のツール効果を関連付け、重複排除と retry 増幅の正確な測定を可能にします。",
		"하나의 attempt identity가 외부 요청, Provider 결과, 재시도, continuation, 후속 도구 효과를 연결하여 중복 제거와 재시도 증폭의 정확한 측정을 가능하게 합니다.",
		"Единая идентичность attempt связывает исходящий запрос, результат Provider, retry, continuation и последующие эффекты инструментов, позволяя устранять дубликаты и точно измерять усиление из-за повторов.",
	}},
	{KeyAgenticLocal5OptimizationPreciseTelemetry, [6]string{
		"The formal telemetry schema separates attempts, outcomes, logical calls, and physical operations. This local pilot still uses a completion-time HTTP meter rather than a request-start WAL, so only the reported local projection is claimed here.",
		"正式 telemetry schema 会区分 attempt、结果、逻辑调用与物理操作；但本地 pilot 仍使用请求结束时记录的 HTTP meter，而非请求启动 WAL，因此这里只主张已披露的本地投影。",
		"Die Telemetrie erfasst gestartete LLM-Aufrufe, abgeschlossene Antworten, Retries, Tool-Aufrufe, physische Operationen sowie Token-, Cache-, Zeit- und Kostennachweise über alle Transporte getrennt.",
		"telemetry は開始済み LLM 呼び出し、完了応答、retry、ツール呼び出し、物理操作に加え、全 transport の token、cache、時間、費用の証拠を個別に記録します。",
		"telemetry는 시작된 LLM 호출, 완료 응답, 재시도, 도구 호출, 물리 작업과 모든 transport의 token, cache, 시간, 비용 증거를 각각 기록합니다.",
		"Телеметрия раздельно фиксирует начатые вызовы LLM, завершённые ответы, retry, вызовы инструментов, физические операции, а также доказательства по токенам, кэшу, времени и стоимости для всех transport.",
	}},
	{KeyAgenticLocal5OptimizationPrintSessionQuartet, [6]string{
		"Print mode now carries session ID, session project directory, project root, and CWD as one startup identity quartet. Final-candidate validation reduced observed Inspect root-denied failures to zero.",
		"Print mode 现在把 session ID、session project directory、project root 与 CWD 作为同一个启动身份四元组传递；最终候选验证中观察到的 Inspect root-denied 失败已降为 0。",
		"Der Print-Modus übergibt Session-ID, Session-Projektverzeichnis, Projektwurzel und CWD nun als ein gemeinsames Startidentitäts-Quartett. In der Validierung des finalen Kandidaten sanken die beobachteten Inspect-root-denied-Fehler auf null.",
		"Print mode は session ID、session project directory、project root、CWD を 1 つの起動 identity quartet として渡すようになりました。最終候補の検証では、観測された Inspect root-denied 失敗が 0 になりました。",
		"Print mode는 session ID, session project directory, project root, CWD를 하나의 시작 identity quartet로 전달합니다. 최종 후보 검증에서 관측된 Inspect root-denied 실패가 0으로 줄었습니다.",
		"Режим Print теперь передаёт ID сессии, каталог проекта сессии, корень проекта и CWD как единый квартет стартовой идентичности. При проверке финального кандидата число наблюдаемых ошибок Inspect root-denied снизилось до нуля.",
	}},
	{KeyAgenticLocal5OptimizationInspectCursorCompatibility, [6]string{
		"Inspect continuation accepts model-shaped requests:[] with max_* placeholders while keeping the server cursor authoritative. The observed two-call zero-progress pattern fell from two calls to zero; one later invalid/consumed cursor recovered on the next turn, so cursor errors did not fall to zero.",
		"Inspect continuation 接受模型生成的 requests:[] 与 max_* 占位字段，同时仍以服务端 cursor 为权威；观察到的两次调用零进展模式从 2 次降为 0，但之后仍有 1 次 invalid/consumed cursor 并在下一轮恢复，因此不能声称所有 cursor 错误归零。",
		"Inspect-Continuation akzeptiert nun modellgeformte requests:[] mit max_*-Platzhaltern, während der serverseitige Cursor-Snapshot maßgeblich bleibt. Eine beobachtete Schleife mit zwei LLM-Aufrufen ohne Fortschritt fiel nach der Korrektur auf null.",
		"Inspect continuation は、サーバー側 cursor snapshot を正として維持しつつ、モデル形式の requests:[] と max_* placeholder を受け入れます。観測された 2 回連続の LLM ゼロ進捗ループ 1 件は、修正後 0 になりました。",
		"Inspect continuation은 서버 측 cursor snapshot을 기준으로 유지하면서 모델 형태의 requests:[]와 max_* placeholder를 허용합니다. 관측된 2회 연속 LLM 무진행 루프 한 건은 수정 후 0으로 줄었습니다.",
		"Продолжение Inspect теперь принимает сформированные моделью requests:[] с заполнителями max_*, сохраняя серверный snapshot cursor источником истины. Один наблюдаемый цикл из двух вызовов LLM без прогресса после исправления сократился до нуля.",
	}},
}

var agenticLocal5Keys = func() []Key {
	keys := make([]Key, 0, len(agenticLocal5Copy))
	for _, entry := range agenticLocal5Copy {
		keys = append(keys, entry.key)
	}
	return keys
}()

func init() {
	for _, entry := range agenticLocal5Copy {
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
