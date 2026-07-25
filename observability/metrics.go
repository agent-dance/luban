// Package observability provides bounded, process-local incident metrics.
//
// The collector deliberately accepts typed, low-cardinality observations
// instead of arbitrary label maps. Raw commands, paths, session IDs, tool-use
// IDs, model output, and error strings never enter a metric series.
package observability

import (
	"sort"
	"strings"
	"sync"
)

// MetricName is a stable machine-readable metric identifier.
type MetricName string

const (
	MetricCompactionRuns            MetricName = "runtime.compaction.runs_total"
	MetricCompactionOutputRatio     MetricName = "runtime.compaction.output_ratio"
	MetricCompactionReductionRatio  MetricName = "runtime.compaction.reduction_ratio"
	MetricCompactionSemanticResults MetricName = "runtime.compaction.semantic_results_total"
	MetricShellPolicyDecisions      MetricName = "runtime.shell.policy_decisions_total"
	MetricTerminalControlRejected   MetricName = "runtime.terminal.control_write_rejected_total"
	MetricActivityOrphans           MetricName = "runtime.activity.orphans_total"
	MetricActivityStaleDrops        MetricName = "runtime.activity.stale_drops_total"
	MetricGenerationDrops           MetricName = "runtime.generation.drops_total"
)

type label struct {
	key   string
	value string
}

type aggregate struct {
	name   MetricName
	labels []label
	count  uint64
	sum    float64
	last   float64
	min    float64
	max    float64
}

// Collector is safe for concurrent producers.
type Collector struct {
	mu sync.Mutex

	series map[string]*aggregate
}

// NewCollector creates an empty collector.
func NewCollector() *Collector {
	return &Collector{
		series: make(map[string]*aggregate),
	}
}

var process = NewCollector()

func (c *Collector) record(name MetricName, value float64, labels ...label) {
	sort.Slice(labels, func(i, j int) bool { return labels[i].key < labels[j].key })
	var key strings.Builder
	key.WriteString(string(name))
	for _, item := range labels {
		key.WriteByte(0)
		key.WriteString(item.key)
		key.WriteByte('=')
		key.WriteString(item.value)
	}
	seriesKey := key.String()
	c.mu.Lock()
	metric := c.series[seriesKey]
	if metric == nil {
		metric = &aggregate{name: name, labels: append([]label(nil), labels...), min: value, max: value}
		c.series[seriesKey] = metric
	}
	metric.count++
	metric.sum += value
	metric.last = value
	if value < metric.min {
		metric.min = value
	}
	if value > metric.max {
		metric.max = value
	}
	c.mu.Unlock()
}

// CompactionTrigger is a bounded compaction cause.
type CompactionTrigger string

const (
	CompactionTriggerManual   CompactionTrigger = "manual"
	CompactionTriggerAuto     CompactionTrigger = "auto"
	CompactionTriggerReactive CompactionTrigger = "reactive"
	CompactionTriggerUnknown  CompactionTrigger = "unknown"
)

// CompactionOutcome is the terminal lifecycle result of one attempt.
type CompactionOutcome string

const (
	CompactionOutcomeSuccess   CompactionOutcome = "success"
	CompactionOutcomeFailure   CompactionOutcome = "failure"
	CompactionOutcomeCancelled CompactionOutcome = "cancelled"
	CompactionOutcomeNoop      CompactionOutcome = "noop"
)

// CompactionObservation contains counts only; it cannot carry summary text or
// transcript content.
type CompactionObservation struct {
	Trigger        CompactionTrigger
	Outcome        CompactionOutcome
	BeforeTokens   int
	AfterTokens    int
	BeforeMessages int
	AfterMessages  int
}

// RecordCompaction records one terminal compaction outcome and, when known,
// its output/input token ratio and semantic reduction result.
func RecordCompaction(observation CompactionObservation) { process.RecordCompaction(observation) }

// RecordCompaction records one terminal compaction outcome.
func (c *Collector) RecordCompaction(observation CompactionObservation) {
	trigger := normalizeCompactionTrigger(observation.Trigger)
	outcome := normalizeCompactionOutcome(observation.Outcome)
	c.record(MetricCompactionRuns, 1,
		label{key: "trigger", value: string(trigger)},
		label{key: "outcome", value: string(outcome)},
	)

	semantic := "unknown"
	terminalSuccess := outcome == CompactionOutcomeSuccess || outcome == CompactionOutcomeNoop
	if terminalSuccess && observation.BeforeTokens > 0 && observation.AfterTokens > 0 {
		ratio := float64(observation.AfterTokens) / float64(observation.BeforeTokens)
		c.record(MetricCompactionOutputRatio, ratio,
			label{key: "trigger", value: string(trigger)},
			label{key: "outcome", value: string(outcome)},
		)
		c.record(MetricCompactionReductionRatio, 1-ratio,
			label{key: "trigger", value: string(trigger)},
			label{key: "outcome", value: string(outcome)},
		)
		if observation.AfterTokens < observation.BeforeTokens {
			semantic = "reduced"
		} else {
			semantic = "not_reduced"
		}
	} else if terminalSuccess && observation.BeforeMessages > 0 && observation.AfterMessages > 0 {
		if observation.AfterMessages < observation.BeforeMessages {
			semantic = "reduced"
		} else {
			semantic = "not_reduced"
		}
	}
	c.record(MetricCompactionSemanticResults, 1,
		label{key: "trigger", value: string(trigger)},
		label{key: "outcome", value: string(outcome)},
		label{key: "result", value: semantic},
	)
}

func normalizeCompactionTrigger(trigger CompactionTrigger) CompactionTrigger {
	switch trigger {
	case CompactionTriggerManual, CompactionTriggerAuto, CompactionTriggerReactive:
		return trigger
	default:
		return CompactionTriggerUnknown
	}
}

func normalizeCompactionOutcome(outcome CompactionOutcome) CompactionOutcome {
	switch outcome {
	case CompactionOutcomeSuccess, CompactionOutcomeFailure, CompactionOutcomeCancelled, CompactionOutcomeNoop:
		return outcome
	default:
		return CompactionOutcomeFailure
	}
}

// RecordShellPolicy records a consumed, structured shell policy decision. The
// code is collapsed to a fixed reason class before it is used as a label.
func RecordShellPolicy(disposition, code string) { process.RecordShellPolicy(disposition, code) }

// RecordShellPolicy records a structured decision without command content.
func (c *Collector) RecordShellPolicy(disposition, code string) {
	decision := normalizeShellDisposition(disposition)
	reason := shellReasonClass(code)
	c.record(MetricShellPolicyDecisions, 1,
		label{key: "decision", value: decision},
		label{key: "reason_class", value: reason},
	)
}

func normalizeShellDisposition(disposition string) string {
	switch strings.ToLower(strings.TrimSpace(disposition)) {
	case "allow", "allowed":
		return "allow"
	case "ask", "prompt", "approval_required", "required_ask":
		return "ask"
	case "block", "blocked":
		return "block"
	case "deny", "denied":
		return "deny"
	default:
		return "unknown"
	}
}

func shellReasonClass(code string) string {
	code = strings.ToLower(strings.TrimSpace(code))
	switch {
	case strings.Contains(code, "root"):
		return "root"
	case strings.Contains(code, "home"):
		return "home"
	case strings.Contains(code, "raw_device"), strings.Contains(code, "device"):
		return "raw_device"
	case strings.Contains(code, "protected"), strings.Contains(code, "system"):
		return "protected_path"
	case strings.Contains(code, "dynamic"), strings.Contains(code, "unproven"):
		return "dynamic_target"
	case strings.Contains(code, "parse"), strings.Contains(code, "structural"):
		return "parse_or_structure"
	case strings.Contains(code, "destructive"), strings.Contains(code, "delete"):
		return "destructive"
	case code == "", strings.Contains(code, "allow"):
		return "none"
	default:
		return "other"
	}
}

// TerminalWriteReason is a bounded terminal-control rejection class.
type TerminalWriteReason string

const (
	TerminalWriteNoOwner     TerminalWriteReason = "no_owner"
	TerminalWriteUnavailable TerminalWriteReason = "owner_unavailable"
	TerminalWriteFailure     TerminalWriteReason = "write_failure"
)

// RecordTerminalControlRejected records a control write rejected by the owner
// API. It is not a claim that a direct stdout/stderr bypass occurred.
func RecordTerminalControlRejected(reason TerminalWriteReason) {
	process.RecordTerminalControlRejected(reason)
}

// RecordTerminalControlRejected records a bounded owner-channel rejection.
func (c *Collector) RecordTerminalControlRejected(reason TerminalWriteReason) {
	switch reason {
	case TerminalWriteNoOwner, TerminalWriteUnavailable, TerminalWriteFailure:
	default:
		reason = TerminalWriteFailure
	}
	c.record(MetricTerminalControlRejected, 1, label{key: "reason", value: string(reason)})
}

// ActivityMetricSource is a bounded activity reconciliation surface.
type ActivityMetricSource string

const (
	ActivitySourceRestoreReconcile ActivityMetricSource = "restore_reconcile"
	ActivitySourceScopeFence       ActivityMetricSource = "scope_fence"
	ActivitySourceSequenceFence    ActivityMetricSource = "sequence_fence"
	ActivitySourceTerminalFence    ActivityMetricSource = "terminal_fence"
	ActivitySourceProjection       ActivityMetricSource = "projection"
)

// RecordActivityOrphans records durable rows that had no authoritative live run.
func RecordActivityOrphans(count int, source ActivityMetricSource) {
	process.RecordActivityOrphans(count, source)
}

// RecordActivityOrphans records a bounded orphan delta.
func (c *Collector) RecordActivityOrphans(count int, source ActivityMetricSource) {
	if count <= 0 {
		return
	}
	if source != ActivitySourceRestoreReconcile && source != ActivitySourceProjection {
		source = ActivitySourceRestoreReconcile
	}
	c.record(MetricActivityOrphans, float64(count), label{key: "source", value: string(source)})
}

// RecordActivityStaleDrop records a rejected activity update.
func RecordActivityStaleDrop(source ActivityMetricSource) {
	process.RecordActivityStaleDrop(source)
}

// RecordActivityStaleDrop records a bounded stale-update rejection.
func (c *Collector) RecordActivityStaleDrop(source ActivityMetricSource) {
	if source != ActivitySourceScopeFence && source != ActivitySourceSequenceFence &&
		source != ActivitySourceTerminalFence && source != ActivitySourceProjection {
		source = ActivitySourceScopeFence
	}
	c.record(MetricActivityStaleDrops, 1, label{key: "source", value: string(source)})
}

// GenerationDropSurface is a bounded stale-event fence.
type GenerationDropSurface string

const (
	GenerationSurfaceTUIEpoch              GenerationDropSurface = "tui_epoch"
	GenerationSurfaceTUITool               GenerationDropSurface = "tui_tool"
	GenerationSurfaceTUIDecision           GenerationDropSurface = "tui_decision"
	GenerationSurfaceNotificationSession   GenerationDropSurface = "notification_session"
	GenerationSurfaceCoordinatorCompletion GenerationDropSurface = "coordinator_completion"
)

// RecordGenerationDrop records a stale event rejected by an epoch/session/run fence.
func RecordGenerationDrop(surface GenerationDropSurface) {
	process.RecordGenerationDrop(surface)
}

// RecordGenerationDrop records a bounded generation-fence rejection.
func (c *Collector) RecordGenerationDrop(surface GenerationDropSurface) {
	switch surface {
	case GenerationSurfaceTUIEpoch, GenerationSurfaceTUITool, GenerationSurfaceTUIDecision,
		GenerationSurfaceNotificationSession, GenerationSurfaceCoordinatorCompletion:
	default:
		surface = GenerationSurfaceTUIEpoch
	}
	c.record(MetricGenerationDrops, 1, label{key: "surface", value: string(surface)})
}
