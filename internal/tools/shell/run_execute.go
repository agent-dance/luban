package shell

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/sandbox"
	"github.com/agent-dance/luban/types"
)

// These values are stable wire protocol identifiers, not user-facing copy.
const (
	runStatusSucceeded = "succeeded"
	runStatusFailed    = "failed"
	runStatusTimedOut  = "timed_out"
	runStatusCancelled = "cancelled"
	runStatusSkipped   = "skipped"
)

type runStepExecution struct {
	output        RunStepOutput
	stdout        runCaptureExcerpt
	stderr        runCaptureExcerpt
	contentBlocks []types.ContentBlock
}

const maxRunImageDataURIBytes = 2 * 1024 * 1024

type boundedRunImageCapture struct {
	mu       sync.Mutex
	data     []byte
	overflow bool
}

func (c *boundedRunImageCapture) Write(data []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	written := len(data)
	if c.overflow {
		return written, nil
	}
	remaining := maxRunImageDataURIBytes - len(c.data)
	if len(data) > remaining {
		c.overflow = true
		c.data = nil
		return written, nil
	}
	c.data = append(c.data, data...)
	return written, nil
}

func (c *boundedRunImageCapture) complete() ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.overflow {
		return nil, false
	}
	return append([]byte(nil), c.data...), true
}

type runCaptureExcerpt struct {
	leading  string
	trailing string
	omitted  int64
	total    int64
}

func (e runCaptureExcerpt) truncated() bool { return e.omitted > 0 }

func executeRunPlan(ctx context.Context, scope bashExecutionScope, plan *compiledRunPlan) types.ToolResult {
	logicalStarted := time.Now()
	planCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	results := make([]runStepExecution, len(plan.steps))
	done := make([]chan struct{}, len(plan.steps))
	for index := range done {
		done[index] = make(chan struct{})
	}
	parallelism := min(len(plan.steps), max(1, min(runtime.NumCPU(), 8)))
	processSlots := make(chan struct{}, parallelism)
	var workspace sync.RWMutex
	var failed atomic.Bool
	var wait sync.WaitGroup

	stepBudget := max(1, plan.maxChars/len(plan.steps))
	for index := range plan.steps {
		wait.Add(1)
		go func(stepIndex int) {
			defer wait.Done()
			defer close(done[stepIndex])
			step := plan.steps[stepIndex]
			for _, dependencyIndex := range step.dependsOn {
				<-done[dependencyIndex]
				if results[dependencyIndex].output.Status != runStatusSucceeded {
					results[stepIndex] = skippedRunStep(step)
					return
				}
			}
			if plan.failFast && failed.Load() {
				results[stepIndex] = skippedRunStep(step)
				return
			}
			select {
			case processSlots <- struct{}{}:
				defer func() { <-processSlots }()
			case <-planCtx.Done():
				results[stepIndex] = cancelledRunStep(step)
				return
			}

			if step.readOnly {
				workspace.RLock()
				defer workspace.RUnlock()
			} else {
				workspace.Lock()
				defer workspace.Unlock()
			}
			if planCtx.Err() != nil {
				results[stepIndex] = cancelledRunStep(step)
				return
			}
			results[stepIndex] = executeRunStep(planCtx, scope, step, plan.headLines, plan.tailLines, stepBudget, logicalStarted)
			if runStepFailed(results[stepIndex].output.Status) {
				failed.Store(true)
				if plan.failFast {
					cancel()
				}
			}
		}(index)
	}
	wait.Wait()

	return buildRunResult(results)
}

func runStepFailed(status string) bool {
	return status == runStatusFailed || status == runStatusTimedOut || status == runStatusCancelled
}

func skippedRunStep(step compiledRunStep) runStepExecution {
	return runStepExecution{output: RunStepOutput{
		ID: step.id, Status: runStatusSkipped, ExitCode: -1,
		Effect: step.effect, Resources: append([]string(nil), step.resources...),
	}}
}

func cancelledRunStep(step compiledRunStep) runStepExecution {
	return runStepExecution{output: RunStepOutput{
		ID: step.id, Status: runStatusCancelled, ExitCode: -1,
		Effect: step.effect, Resources: append([]string(nil), step.resources...),
	}}
}

func executeRunStep(ctx context.Context, scope bashExecutionScope, step compiledRunStep, headLines, tailLines, charBudget int, logicalStarted time.Time) runStepExecution {
	started := time.Now()
	result := runStepExecution{output: RunStepOutput{
		ID: step.id, Status: runStatusFailed, ExitCode: -1,
		Effect: step.effect, Resources: append([]string(nil), step.resources...),
	}}
	finish := func() runStepExecution {
		result.output.DurationMS = time.Since(started).Milliseconds()
		result.output.StdoutBytes = result.stdout.total
		result.output.StderrBytes = result.stderr.total
		result.output.StdoutChars = result.stdout.total
		result.output.StderrChars = result.stderr.total
		result.output.Truncated = result.stdout.truncated() || result.stderr.truncated()
		return result
	}

	mutationCoordinator := scope.fileMutations
	mutationTargets := sedEditExecutionMutationTargets(step.sed)
	releaseMutationLocks := func() {}
	if mutationCoordinator != nil && len(mutationTargets) > 0 {
		releaseMutationLocks = mutationCoordinator.Lock(mutationTargets)
		defer releaseMutationLocks()
		if err := mutationCoordinator.ValidateFullRead(ctx, mutationTargets); err != nil {
			result.stderr = literalRunExcerpt(err.Error())
			return finish()
		}
	}

	commandContext, cancel := context.WithTimeout(ctx, step.timeout)
	defer cancel()
	command, err := buildRunCommand(commandContext, scope, step)
	if err != nil {
		result.stderr = literalRunExcerpt(toolRuntimeFormat(i18n.KeyToolRunCommandBuildFailed, step.id))
		return finish()
	}
	bindSedExecutionEnvironment(command, step.sed)

	devNull, openErr := os.Open(os.DevNull)
	if openErr == nil {
		command.Stdin = devNull
		defer devNull.Close()
	}
	stdoutBudget := (charBudget + 1) / 2
	stderrBudget := charBudget / 2
	stdout := newBoundedRunCapture(stdoutBudget, headLines, tailLines)
	stderr := newBoundedRunCapture(stderrBudget, headLines, tailLines)
	var imageCapture *boundedRunImageCapture
	if step.imageOutput {
		imageCapture = &boundedRunImageCapture{}
		command.Stdout = io.MultiWriter(stdout, imageCapture)
	} else {
		command.Stdout = stdout
	}
	command.Stderr = stderr

	processStarted := time.Now()
	runErr := command.Start()
	if runErr == nil {
		// Job-object attachment is a best-effort hardening on Windows. Some
		// hosts place this process in a non-breakaway parent job; falling back to
		// direct-process cancellation is safer than rejecting a process that has
		// already started.
		_ = commandStarted(command)
		defer commandFinished(command)
	}
	if runErr == nil {
		result.output.Invoked = true
		result.output.StartedOffsetMS = max(int64(0), processStarted.Sub(logicalStarted).Milliseconds())
		runErr = command.Wait()
		processEnded := time.Now()
		result.output.EndedOffsetMS = max(result.output.StartedOffsetMS, processEnded.Sub(logicalStarted).Milliseconds())
		result.output.ProcessDurationMS = max(int64(0), processEnded.Sub(processStarted).Milliseconds())
	}
	result.stdout = stdout.excerpt(headLines, tailLines)
	result.stderr = stderr.excerpt(headLines, tailLines)
	if command.ProcessState != nil {
		result.output.ExitCode = command.ProcessState.ExitCode()
	}

	if mutationCoordinator != nil && len(mutationTargets) > 0 {
		if runErr != nil || result.output.ExitCode != 0 {
			mutationCoordinator.Invalidate(ctx, mutationTargets)
		} else if err := mutationCoordinator.Commit(ctx, mutationTargets, "Run"); err != nil {
			mutationCoordinator.Invalidate(ctx, mutationTargets)
		}
	}

	switch {
	case errors.Is(commandContext.Err(), context.DeadlineExceeded):
		result.output.Status = runStatusTimedOut
	case errors.Is(commandContext.Err(), context.Canceled) || errors.Is(ctx.Err(), context.Canceled):
		result.output.Status = runStatusCancelled
	case runErr == nil:
		result.output.Status = runStatusSucceeded
	default:
		if step.verificationKind == runVerificationNone {
			if interpretation, ok := interpretCommandResult(firstSegmentCommand(step.command), result.output.ExitCode); ok && interpretation.TreatAsSuccess {
				result.output.Status = runStatusSucceeded
				break
			}
		}
		result.output.Status = runStatusFailed
	}
	if result.output.Status == runStatusSucceeded && step.imageOutput {
		raw, complete := imageCapture.complete()
		imageResult, valid := buildImageToolResult(string(raw), "")
		if !complete || !valid {
			result.output.Status = runStatusFailed
			result.stderr = literalRunExcerpt(toolRuntimeText(i18n.KeyToolRunImageOutputInvalid))
		} else {
			result.contentBlocks = append(result.contentBlocks, imageResult.ContentBlocks...)
			result.stdout.leading = imageResult.Content
			result.stdout.trailing = ""
			result.stdout.omitted = 0
		}
	}
	return finish()
}

func literalRunExcerpt(value string) runCaptureExcerpt {
	return runCaptureExcerpt{leading: value, total: int64(len(value))}
}

func buildRunCommand(ctx context.Context, scope bashExecutionScope, step compiledRunStep) (*exec.Cmd, error) {
	if scope.forceSandbox && (!scope.sandboxAvailable || scope.sandboxName == "none") {
		return nil, i18n.NewError(i18n.KeyToolRunSandboxUnavailable)
	}
	environment, managedRoot, environmentErr := runVerificationEnvironment(scope, step)
	if environmentErr != nil {
		return nil, i18n.WrapInternalError(i18n.KeyToolRunCommandBuildFailed, environmentErr, step.id)
	}
	name, arguments := runCommandArgv(step)
	useSandbox := scope.forceSandbox || scope.sandboxAvailable && scope.sandboxName != "none" && ShouldUseSandbox(step.command, step.semantics)
	if useSandbox {
		capability, available := sandbox.Snapshot(scope.sandbox)
		if !available || capability.ID() != scope.sandboxCapability {
			return nil, i18n.NewError(i18n.KeyToolRunSandboxUnavailable)
		}
		readWritePaths := runSandboxWritePaths(scope, step.cwd, managedRoot)
		configuration := sandbox.Config{
			ReadWritePaths: readWritePaths, WorkDir: step.cwd, Environment: environment,
		}
		command, err := scope.sandbox.Command(ctx, configuration, name, arguments...)
		if err != nil {
			return nil, i18n.WrapInternalError(i18n.KeyToolRunCommandBuildFailed, err, step.id)
		}
		command.Dir = step.cwd
		environment.Apply(command)
		configureCommandCancellation(command)
		return command, nil
	}
	command := exec.CommandContext(ctx, name, arguments...)
	command.Dir = step.cwd
	environment.Apply(command)
	configureCommandCancellation(command)
	return command, nil
}

func runCommandArgv(step compiledRunStep) (string, []string) {
	if !step.useShell {
		return step.argv[0], append([]string(nil), step.argv[1:]...)
	}
	// The script is policy-analyzed before execution. pipefail is enabled both
	// as a startup option and immediately in the script prelude; startup files
	// are disabled so ambient BASH_ENV/profile state cannot widen execution.
	return "bash", []string{"--noprofile", "--norc", "-o", "pipefail", "-c", "set -o pipefail\n" + step.shellScript}
}

func runSandboxWritePaths(scope bashExecutionScope, stepCWD, managedRoot string) []string {
	candidates := append(append([]string(nil), scope.allowedDirs...), scope.cwd, stepCWD)
	if managedRoot != "" {
		candidates = append(candidates, managedRoot)
	}
	if !scope.forceSandbox && len(scope.allowedDirs) == 0 {
		root := string(filepath.Separator)
		if volume := filepath.VolumeName(stepCWD); volume != "" {
			root = volume + string(filepath.Separator)
		}
		candidates = []string{root}
	}
	seen := make(map[string]struct{}, len(candidates))
	paths := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate) == "" {
			continue
		}
		cleaned := filepath.Clean(candidate)
		if _, exists := seen[cleaned]; exists {
			continue
		}
		seen[cleaned] = struct{}{}
		paths = append(paths, cleaned)
	}
	sort.Strings(paths)
	return paths
}

func buildRunResult(executions []runStepExecution) types.ToolResult {
	counts := make(map[string]int)
	output := RunOutput{Steps: make([]RunStepOutput, len(executions)), LogicalExecutionCommitted: true}
	metadata := map[string]string{"stepCount": strconv.Itoa(len(executions))}
	for index, execution := range executions {
		output.Steps[index] = execution.output
		output.contentBlocks = append(output.contentBlocks, execution.contentBlocks...)
		counts[execution.output.Status]++
	}
	metadata["succeeded"] = strconv.Itoa(counts[runStatusSucceeded])
	metadata["failed"] = strconv.Itoa(counts[runStatusFailed])
	metadata["timedOut"] = strconv.Itoa(counts[runStatusTimedOut])
	metadata["cancelled"] = strconv.Itoa(counts[runStatusCancelled])
	metadata["skipped"] = strconv.Itoa(counts[runStatusSkipped])

	var model strings.Builder
	model.WriteString(toolRuntimeFormat(
		i18n.KeyToolRunSummary,
		counts[runStatusSucceeded], counts[runStatusFailed], counts[runStatusTimedOut],
		counts[runStatusCancelled], counts[runStatusSkipped],
	))
	for _, execution := range executions {
		step := execution.output
		model.WriteString("\n")
		model.WriteString(toolRuntimeFormat(
			i18n.KeyToolRunStepResult,
			step.ID, step.Status, step.ExitCode, step.DurationMS, step.Truncated, step.Effect,
		))
		appendRunStream(&model, i18n.KeyToolRunStdout, execution.stdout)
		appendRunStream(&model, i18n.KeyToolRunStderr, execution.stderr)
	}
	output.modelText = model.String()

	isError := counts[runStatusFailed]+counts[runStatusTimedOut]+counts[runStatusCancelled] > 0
	outcome := types.ToolOutcomeSucceeded
	if counts[runStatusTimedOut] > 0 {
		outcome = types.ToolOutcomeTimedOut
	} else if counts[runStatusCancelled] > 0 {
		outcome = types.ToolOutcomeCancelled
	} else if counts[runStatusFailed] > 0 {
		outcome = types.ToolOutcomeFailed
	}
	return types.ToolResult{
		Content: output.modelText, Data: &output, IsError: isError,
		Metadata: metadata, Outcome: outcome,
	}
}

func appendRunStream(builder *strings.Builder, heading i18n.Key, excerpt runCaptureExcerpt) {
	if excerpt.total == 0 {
		return
	}
	builder.WriteString("\n")
	builder.WriteString(toolRuntimeText(heading))
	if excerpt.leading != "" {
		builder.WriteString("\n")
		builder.WriteString(excerpt.leading)
	}
	if excerpt.omitted > 0 {
		builder.WriteString("\n")
		builder.WriteString(toolRuntimeFormat(i18n.KeyToolRunOutputOmitted, excerpt.omitted))
	}
	if excerpt.trailing != "" {
		builder.WriteString("\n")
		builder.WriteString(excerpt.trailing)
	}
}

// boundedRunCapture retains a fixed first-byte region and a fixed circular
// last-byte region. Write never allocates in proportion to process output.
type boundedRunCapture struct {
	mu sync.Mutex

	capacity int
	headCap  int
	tailCap  int
	total    int64
	head     []byte
	tail     []byte
	tailPos  int
	tailLen  int
}

func newBoundedRunCapture(capacity, headLines, tailLines int) *boundedRunCapture {
	if capacity < 0 {
		capacity = 0
	}
	headCap, tailCap := 0, 0
	weight := headLines + tailLines
	if weight > 0 {
		headCap = capacity * headLines / weight
		tailCap = capacity - headCap
	}
	return &boundedRunCapture{
		capacity: capacity, headCap: headCap, tailCap: tailCap,
		head: make([]byte, 0, headCap), tail: make([]byte, tailCap),
	}
}

func (c *boundedRunCapture) Write(data []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	written := len(data)
	c.total += int64(written)
	if remaining := c.headCap - len(c.head); remaining > 0 {
		amount := min(remaining, len(data))
		c.head = append(c.head, data[:amount]...)
	}
	if c.tailCap == 0 {
		return written, nil
	}
	if len(data) >= c.tailCap {
		copy(c.tail, data[len(data)-c.tailCap:])
		c.tailPos = 0
		c.tailLen = c.tailCap
		return written, nil
	}
	for len(data) > 0 {
		amount := min(len(data), c.tailCap-c.tailPos)
		copy(c.tail[c.tailPos:c.tailPos+amount], data[:amount])
		c.tailPos = (c.tailPos + amount) % c.tailCap
		c.tailLen = min(c.tailCap, c.tailLen+amount)
		data = data[amount:]
	}
	return written, nil
}

func (c *boundedRunCapture) excerpt(headLines, tailLines int) runCaptureExcerpt {
	c.mu.Lock()
	defer c.mu.Unlock()
	tail := c.orderedTail()
	head := append([]byte(nil), c.head...)
	if c.total <= int64(c.capacity) {
		overlap := len(head) + len(tail) - int(c.total)
		if overlap > 0 && overlap <= len(tail) {
			tail = tail[overlap:]
		}
		full := append(head, tail...)
		return selectRunLines(full, c.total, headLines, tailLines)
	}
	leading := firstRunLines(head, headLines)
	trailing := lastRunLines(tail, tailLines)
	omitted := c.total - int64(len(leading)+len(trailing))
	if omitted < 0 {
		omitted = 0
	}
	return runCaptureExcerpt{
		leading: validRunText(leading), trailing: validRunText(trailing),
		omitted: omitted, total: c.total,
	}
}

func (c *boundedRunCapture) orderedTail() []byte {
	if c.tailLen == 0 {
		return nil
	}
	output := make([]byte, c.tailLen)
	start := c.tailPos - c.tailLen
	if start < 0 {
		start += c.tailCap
	}
	first := min(c.tailLen, c.tailCap-start)
	copy(output, c.tail[start:start+first])
	copy(output[first:], c.tail[:c.tailLen-first])
	return output
}

func selectRunLines(data []byte, total int64, headLines, tailLines int) runCaptureExcerpt {
	if len(data) == 0 {
		return runCaptureExcerpt{total: total, omitted: total}
	}
	lines := splitRunLines(data)
	if len(lines) <= headLines+tailLines || len(lines) <= headLines || len(lines) <= tailLines {
		return runCaptureExcerpt{leading: validRunText(data), total: total}
	}
	leading := joinRunLines(lines[:min(headLines, len(lines))])
	trailingStart := max(0, len(lines)-tailLines)
	if trailingStart < headLines {
		trailingStart = headLines
	}
	trailing := joinRunLines(lines[trailingStart:])
	omitted := total - int64(len(leading)+len(trailing))
	if omitted < 0 {
		omitted = 0
	}
	return runCaptureExcerpt{
		leading: validRunText(leading), trailing: validRunText(trailing),
		omitted: omitted, total: total,
	}
}

func splitRunLines(data []byte) [][]byte {
	var lines [][]byte
	for len(data) > 0 {
		index := bytes.IndexByte(data, '\n')
		if index < 0 {
			lines = append(lines, data)
			break
		}
		lines = append(lines, data[:index+1])
		data = data[index+1:]
	}
	return lines
}

func joinRunLines(lines [][]byte) []byte {
	size := 0
	for _, line := range lines {
		size += len(line)
	}
	joined := make([]byte, 0, size)
	for _, line := range lines {
		joined = append(joined, line...)
	}
	return joined
}

func firstRunLines(data []byte, count int) []byte {
	if count <= 0 {
		return nil
	}
	lines := splitRunLines(data)
	return joinRunLines(lines[:min(count, len(lines))])
}

func lastRunLines(data []byte, count int) []byte {
	if count <= 0 {
		return nil
	}
	lines := splitRunLines(data)
	return joinRunLines(lines[max(0, len(lines)-count):])
}

func validRunText(data []byte) string {
	if utf8.Valid(data) {
		return string(data)
	}
	return strings.ToValidUTF8(string(data), "�")
}
