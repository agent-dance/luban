package tools

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
	"mvdan.cc/sh/v3/syntax"
)

// sedEditInvocation binds one syntactic sed -i call to the physical working
// directory in which that exact call will run. FilePaths remain the original
// argv spellings for compatibility; Targets are the immutable absolute paths
// consumed by validation, locking, and post-execution evidence refresh.
type sedEditInvocation struct {
	Plan         *SedEditPlan
	EffectiveCWD string
	Targets      []string
}

// sedEditExecution is deliberately produced once per Bash execution. A safe
// plan proves every in-place sed invocation, its ordering context, and its
// effective CWD. EvidenceSafe is false when a preceding/following command can
// mutate a target, a CWD transition has more than one outcome, or shell
// structure prevents a complete proof. Such commands may run only through the
// mandatory shell approval boundary and never publish Bash-as-Read evidence.
type sedEditExecution struct {
	Invocations  []sedEditInvocation
	HasInPlace   bool
	EvidenceSafe bool
}

type sedEditFlow struct {
	cwd   string
	exact bool
}

type sedEditAnalysisBuilder struct {
	policy             types.PolicyContext
	invocations        []sedEditInvocation
	potentialInPlace   int
	safe               bool
	nonSedSideEffect   bool
	unsupportedControl bool
	writeRedirect      bool
	asynchronous       bool
}

// analyzeSedEditExecution parses the command once and binds all trusted sed
// targets to their physical effective CWD. This compatibility entry point is
// also used by focused validation helpers; the policy analyzer calls the file
// variant directly so it does not parse the shell program twice.
func analyzeSedEditExecution(command, cwd string) sedEditExecution {
	policy := DefaultShellPolicyContext()
	if strings.TrimSpace(cwd) != "" {
		policy.CWD = cwd
	}
	return analyzeSedEditExecutionFileMustParse(command, policy)
}

func analyzeSedEditExecutionFileMustParse(command string, policy types.PolicyContext) sedEditExecution {
	prog, err := syntax.NewParser(syntax.KeepComments(false)).Parse(strings.NewReader(command), "")
	if err != nil {
		return sedEditExecution{}
	}
	return analyzeSedEditExecutionFile(prog, policy)
}

// analyzeBashCommandWithSedEvidencePolicy keeps permission preflight,
// execution, and rejection projection on the same evidence-boundary verdict.
// The unified analyzer also incorporates this rule; retaining the merge here
// is defense in depth for embedders that construct BashTool directly.
func analyzeBashCommandWithSedEvidencePolicy(command string, policy types.PolicyContext) (types.PolicyDecision, sedEditExecution) {
	decision := AnalyzeShellCommand(command, policy)
	execution := analyzeSedEditExecutionFileMustParse(command, policy)
	if execution.HasInPlace && !execution.EvidenceSafe {
		decision = strongerShellPolicyDecision(decision, unrestrictedCodePolicyDecision("sed"))
	}
	return decision, execution
}

// bindSedExecutionEnvironment makes the shell's CWD semantics match the
// physical plan. An inherited CDPATH can redirect `cd sub` to an unrelated
// directory; BASH_ENV/ENV can run an opaque startup hook first. An
// evidence-safe plan therefore disables all three inherited controls.
// Explicit assignments in the command are already evidence-ineligible.
func bindSedExecutionEnvironment(command *exec.Cmd, execution sedEditExecution) {
	if command == nil || !execution.EvidenceSafe {
		return
	}
	environment := command.Env
	if environment == nil {
		environment = os.Environ()
	}
	bound := make([]string, 0, len(environment)+3)
	for _, entry := range environment {
		name, _, _ := strings.Cut(entry, "=")
		if name == "CDPATH" || name == "BASH_ENV" || name == "ENV" {
			continue
		}
		bound = append(bound, entry)
	}
	command.Env = append(bound, "CDPATH=", "BASH_ENV=", "ENV=")
}

func analyzeSedEditExecutionFile(prog *syntax.File, policy types.PolicyContext) sedEditExecution {
	if prog == nil {
		return sedEditExecution{}
	}
	physicalCWD, cwdOK := physicalSedCWD(policy.CWD)
	if cwdOK {
		policy.CWD = physicalCWD
	}
	builder := &sedEditAnalysisBuilder{
		policy: policy, safe: cwdOK,
		nonSedSideEffect: shellPolicyProgramTaintsExecutionEnvironment(prog),
	}

	// First establish the completeness envelope. The recursive flow evaluator
	// below supports lists, &&/||, subshells, brace groups, and literal wrappers.
	// All other control flow, writable redirections, and asynchronous execution
	// are fail-closed for evidence publication.
	syntax.Walk(prog, func(node syntax.Node) bool {
		switch value := node.(type) {
		case *syntax.CallExpr:
			if sedCallMayEditInPlace(value, policy) {
				builder.potentialInPlace++
			}
		case *syntax.Stmt:
			if value.Background || value.Coprocess || value.Disown {
				builder.asynchronous = true
			}
			for _, redirect := range value.Redirs {
				if shellPolicyRedirectWritesPath(redirect.Op) {
					builder.writeRedirect = true
				}
			}
		case *syntax.IfClause, *syntax.ForClause, *syntax.WhileClause,
			*syntax.CaseClause, *syntax.FuncDecl, *syntax.CmdSubst, *syntax.ProcSubst:
			builder.unsupportedControl = true
		}
		return true
	})

	builder.analyzeStatements(prog.Stmts, sedEditFlow{cwd: physicalCWD, exact: cwdOK})
	hasInPlace := builder.potentialInPlace > 0
	evidenceSafe := hasInPlace && builder.safe &&
		len(builder.invocations) == builder.potentialInPlace &&
		!builder.nonSedSideEffect && !builder.unsupportedControl &&
		!builder.writeRedirect && !builder.asynchronous
	return sedEditExecution{
		Invocations: append([]sedEditInvocation(nil), builder.invocations...),
		HasInPlace:  hasInPlace, EvidenceSafe: evidenceSafe,
	}
}

func physicalSedCWD(cwd string) (string, bool) {
	if strings.TrimSpace(cwd) == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return "", false
		}
	}
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return "", false
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", false
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", false
	}
	return canonicalPathForComparison(resolved), true
}

func sedCallMayEditInPlace(call *syntax.CallExpr, policy types.PolicyContext) bool {
	if call == nil || len(call.Args) == 0 {
		return false
	}
	name, args, _, _ := unwrapShellPolicyCall(call, map[string]shellPolicyValue{}, policy)
	if name != "sed" {
		return false
	}
	for _, word := range args {
		value := wordToString(word)
		if value == "-i" || value == "--in-place" ||
			strings.HasPrefix(value, "-i") && value != "-" ||
			strings.HasPrefix(value, "--in-place=") {
			return true
		}
	}
	return false
}

func (builder *sedEditAnalysisBuilder) analyzeStatements(statements []*syntax.Stmt, flow sedEditFlow) sedEditFlow {
	for _, statement := range statements {
		flow = builder.analyzeStatement(statement, flow)
	}
	return flow
}

func (builder *sedEditAnalysisBuilder) analyzeStatement(statement *syntax.Stmt, flow sedEditFlow) sedEditFlow {
	if statement == nil || statement.Cmd == nil {
		return flow
	}
	switch command := statement.Cmd.(type) {
	case *syntax.BinaryCmd:
		switch command.Op {
		case syntax.Pipe, syntax.PipeAll:
			if statementContainsInPlaceSed(command.X, builder.policy) || statementContainsInPlaceSed(command.Y, builder.policy) {
				builder.safe = false
			}
			return flow
		case syntax.AndStmt, syntax.OrStmt:
			if changed, ok := exactSedCDSuccess(command.X, flow, builder.policy); ok {
				if command.Op == syntax.AndStmt {
					builder.analyzeStatement(command.Y, changed)
				} else {
					// The right side of cd TARGET || ... runs only when cd
					// failed, so it retains the caller's exact CWD.
					builder.analyzeStatement(command.Y, flow)
				}
				// After the binary expression either CWD is possible; a
				// later relative sed cannot be bound to one physical target.
				return sedEditFlow{cwd: flow.cwd, exact: false}
			}
			if statementContainsCD(command.X, builder.policy) || statementContainsCD(command.Y, builder.policy) {
				builder.safe = false
				return sedEditFlow{cwd: flow.cwd, exact: false}
			}
			builder.analyzeStatement(command.X, flow)
			builder.analyzeStatement(command.Y, flow)
			return flow
		}
	case *syntax.Subshell:
		builder.analyzeStatements(command.Stmts, flow)
		return flow
	case *syntax.Block:
		return builder.analyzeStatements(command.Stmts, flow)
	case *syntax.CallExpr:
		if changed, ok := exactSedCDCallSuccess(command, flow, builder.policy); ok {
			// In an unconditional list, a failed cd leaves the old CWD and
			// a successful cd installs the new one. Only an enclosing &&/||
			// can select a single outcome for a following sed.
			return sedEditFlow{cwd: changed.cwd, exact: false}
		}
		builder.analyzeCall(command, flow)
		return flow
	default:
		if statementContainsInPlaceSed(statement, builder.policy) {
			builder.safe = false
		}
		builder.unsupportedControl = true
	}
	return flow
}

func exactSedCDSuccess(statement *syntax.Stmt, flow sedEditFlow, policy types.PolicyContext) (sedEditFlow, bool) {
	if statement == nil || statement.Cmd == nil || len(statement.Redirs) != 0 ||
		statement.Background || statement.Coprocess || statement.Disown {
		return sedEditFlow{}, false
	}
	call, ok := statement.Cmd.(*syntax.CallExpr)
	if !ok {
		return sedEditFlow{}, false
	}
	return exactSedCDCallSuccess(call, flow, policy)
}

func exactSedCDCallSuccess(call *syntax.CallExpr, flow sedEditFlow, policy types.PolicyContext) (sedEditFlow, bool) {
	if call == nil || !flow.exact {
		return sedEditFlow{}, false
	}
	for _, assignment := range call.Assigns {
		if assignment.Name != nil && (assignment.Name.Value == "CDPATH" || assignment.Name.Value == "PATH") {
			return sedEditFlow{}, false
		}
	}
	localPolicy := policy
	localPolicy.CWD = flow.cwd
	args, ok := sequentialCDArgs(call, localPolicy)
	if !ok {
		return sedEditFlow{}, false
	}
	target, dynamic := sequentialCDTarget(args, localPolicy)
	if dynamic || target == nil {
		return sedEditFlow{}, false
	}
	child, decision := shellPolicyWithChildCWD(localPolicy, shellPolicyWordValue(target, map[string]shellPolicyValue{}, localPolicy))
	if decision.Disposition != types.PolicyAllow {
		return sedEditFlow{}, false
	}
	physical, ok := physicalSedCWD(child.CWD)
	if !ok {
		return sedEditFlow{}, false
	}
	return sedEditFlow{cwd: physical, exact: true}, true
}

func statementContainsCD(node syntax.Node, policy types.PolicyContext) bool {
	found := false
	syntax.Walk(node, func(current syntax.Node) bool {
		call, ok := current.(*syntax.CallExpr)
		if !ok {
			return true
		}
		if _, yes := sequentialCDArgs(call, policy); yes {
			found = true
			return false
		}
		return true
	})
	return found
}

func statementContainsInPlaceSed(node syntax.Node, policy types.PolicyContext) bool {
	found := false
	syntax.Walk(node, func(current syntax.Node) bool {
		if call, ok := current.(*syntax.CallExpr); ok && sedCallMayEditInPlace(call, policy) {
			found = true
			return false
		}
		return true
	})
	return found
}

func (builder *sedEditAnalysisBuilder) analyzeCall(call *syntax.CallExpr, flow sedEditFlow) {
	localPolicy := builder.policy
	localPolicy.CWD = flow.cwd
	name, args, wrapperDecision, callPolicy := unwrapShellPolicyCall(call, map[string]shellPolicyValue{}, localPolicy)
	if name == "sed" {
		literals, static := shellPolicyStaticArguments(args, map[string]shellPolicyValue{}, callPolicy)
		for _, word := range args {
			value := shellPolicyWordValue(word, map[string]shellPolicyValue{}, callPolicy)
			if shellPolicyWordHasActiveUnquotedGlob(word, value) || strings.HasPrefix(wordToString(word), "~") {
				static = false
				break
			}
		}
		plan := (*SedEditPlan)(nil)
		if static {
			plan = parseSedArgs(literals)
		}
		if plan != nil {
			if !flow.exact || wrapperDecision.Disposition != types.PolicyAllow ||
				!shellPolicySafeSedInvocation(args, map[string]shellPolicyValue{}, callPolicy) || len(plan.FilePaths) == 0 {
				builder.safe = false
				return
			}
			effectiveCWD, ok := physicalSedCWD(callPolicy.CWD)
			if !ok {
				builder.safe = false
				return
			}
			targets := make([]string, 0, len(plan.FilePaths))
			for _, file := range plan.FilePaths {
				if strings.TrimSpace(file) == "" {
					builder.safe = false
					return
				}
				targets = append(targets, resolveSedTarget(file, effectiveCWD))
			}
			builder.invocations = append(builder.invocations, sedEditInvocation{
				Plan: plan, EffectiveCWD: effectiveCWD, Targets: targets,
			})
			return
		}
	}

	// Any non-sed command in the same shell program must be proven incapable
	// of filesystem mutation. Otherwise it can replace a validated target (or
	// replace it after sed but before evidence refresh), so the entire command
	// is approval-only and evidence-ineligible.
	if !sedSideEffectFreeCall(name, args, wrapperDecision, callPolicy) {
		builder.nonSedSideEffect = true
	}
	for _, assignment := range call.Assigns {
		if assignment.Name != nil && (assignment.Name.Value == "PATH" || assignment.Name.Value == "CDPATH") {
			builder.nonSedSideEffect = true
		}
	}
}

func sedSideEffectFreeCall(name string, args []*syntax.Word, wrapperDecision types.PolicyDecision, policy types.PolicyContext) bool {
	if name == "" || wrapperDecision.Disposition != types.PolicyAllow {
		return false
	}
	switch name {
	case ":", "true", "false", "test", "[", "[[", "pwd", "echo", "printf":
		return true
	}
	literals, static := shellPolicyStaticArguments(args, map[string]shellPolicyValue{}, policy)
	if !static {
		return false
	}
	if analyzeShellExecutionAuthority(name, args, map[string]shellPolicyValue{}, policy).Disposition != types.PolicyAllow {
		return false
	}
	return classifyExternal(name, literals) == SemanticRead
}

// SedReadStateTracker mirrors the FileEditTool "must read first" semantics
// for in-place sed invocations. Its compatibility API records the identity and
// digest from one opened descriptor; timestamps alone are never accepted as
// mutation authority.
//
// This is intentionally minimal: when the tracker is nil, validation is a
// no-op so existing call sites continue to work.
type SedReadStateTracker struct {
	mu      sync.Mutex
	entries map[string]sedReadTrackerEntry
}

type sedReadTrackerEntry struct {
	identity os.FileInfo
	digest   string
}

// NewSedReadStateTracker returns an empty tracker.
func NewSedReadStateTracker() *SedReadStateTracker {
	return &SedReadStateTracker{entries: make(map[string]sedReadTrackerEntry)}
}

// RecordRead stores a descriptor-bound identity and digest for path.
func (t *SedReadStateTracker) RecordRead(path string) {
	if t == nil || path == "" {
		return
	}
	abs := normalizeSedPath(path)
	if abs == "" {
		return
	}
	snapshot, err := readEditTarget(abs, nil)
	if err != nil {
		t.Forget(abs)
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.entries[abs] = sedReadTrackerEntry{identity: snapshot.Info, digest: snapshot.ContentDigest}
}

// HasFresh reports whether path still identifies the exact observed file and
// its descriptor-bound digest remains unchanged.
func (t *SedReadStateTracker) HasFresh(path string) bool {
	if t == nil {
		return false
	}
	abs := normalizeSedPath(path)
	if abs == "" {
		return false
	}
	t.mu.Lock()
	previous, ok := t.entries[abs]
	t.mu.Unlock()
	if !ok || previous.identity == nil || previous.digest == "" {
		return false
	}
	snapshot, err := readEditTarget(abs, previous.identity)
	return err == nil && snapshot.ContentDigest == previous.digest
}

// Forget removes any cached entry for `path`.
func (t *SedReadStateTracker) Forget(path string) {
	if t == nil {
		return
	}
	abs := normalizeSedPath(path)
	if abs == "" {
		return
	}
	t.mu.Lock()
	delete(t.entries, abs)
	t.mu.Unlock()
}

func normalizeSedPath(p string) string {
	if p == "" {
		return ""
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return filepath.Clean(p)
	}
	return filepath.Clean(abs)
}

// ValidateSedEdit applies the "must Read first" gate to an in-place sed
// invocation. Returns nil if the command is not a sed -i edit, or if the
// tracker is nil. Returns a typed error mirroring the FileEditTool message
// when the file has not been read or has been externally modified since the
// last recorded mtime.
func ValidateSedEdit(cmd, cwd string, tracker *SedReadStateTracker) error {
	if tracker == nil {
		return nil
	}
	execution := analyzeSedEditExecution(cmd, cwd)
	return validateSedEditExecution(execution, tracker)
}

func validateSedEditExecution(execution sedEditExecution, tracker *SedReadStateTracker) error {
	if tracker == nil {
		return nil
	}
	if !execution.HasInPlace {
		return nil
	}
	if !execution.EvidenceSafe {
		return i18n.NewError(i18n.KeyShellPolicyAskUnrestrictedCode, "sed")
	}
	for _, invocation := range execution.Invocations {
		for _, path := range invocation.Targets {
			if _, err := os.Lstat(path); err != nil {
				return i18n.NewError(i18n.KeyToolRuntimeBashSedReadRequired, path)
			}
			if !tracker.HasFresh(path) {
				return i18n.NewError(i18n.KeyToolRuntimeBashSedReadRequired, path)
			}
		}
	}
	return nil
}

// MarkSedEditComplete updates the tracker after a successful sed run so
// subsequent Edit calls see the new mtime as the baseline.
func MarkSedEditComplete(cmd, cwd string, tracker *SedReadStateTracker) {
	if tracker == nil {
		return
	}
	execution := analyzeSedEditExecution(cmd, cwd)
	markSedEditExecutionComplete(execution, tracker)
}

func markSedEditExecutionComplete(execution sedEditExecution, tracker *SedReadStateTracker) {
	if tracker == nil {
		return
	}
	if !execution.EvidenceSafe {
		return
	}
	for _, invocation := range execution.Invocations {
		for _, path := range invocation.Targets {
			tracker.RecordRead(path)
		}
	}
}

// ValidateSedEditReadState applies the sed gate to the same ReadFileState used
// by Read/Edit/Write. A partial or stale read is not sufficient because sed -i
// replaces the complete file.
func ValidateSedEditReadState(cmd, cwd string, state *ReadFileState) error {
	return ValidateSedEditReadStateForContext(context.Background(), cmd, cwd, state)
}

func ValidateSedEditReadStateForContext(ctx context.Context, cmd, cwd string, state *ReadFileState) error {
	if state == nil {
		return nil
	}
	execution := analyzeSedEditExecution(cmd, cwd)
	if !execution.HasInPlace {
		return nil
	}
	return validateSedEditExecutionReadState(ctx, execution, state)
}

func validateSedEditExecutionReadState(ctx context.Context, execution sedEditExecution, state *ReadFileState) error {
	if state == nil || !execution.HasInPlace {
		return nil
	}
	if !execution.EvidenceSafe {
		return i18n.NewError(i18n.KeyShellPolicyAskUnrestrictedCode, "sed")
	}
	for _, invocation := range execution.Invocations {
		for _, path := range invocation.Targets {
			info, err := os.Lstat(path)
			if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
				return i18n.NewError(i18n.KeyToolRuntimeBashSedReadRequired, path)
			}
			entry, found := state.GetForContext(ctx, path)
			if !found || entry.IsPartialView || !readEntryHasFullSnapshot(entry) || !sedReadStateIsFresh(path, info, entry) {
				return i18n.NewError(i18n.KeyToolRuntimeBashSedReadRequired, path)
			}
		}
	}
	return nil
}

func sedReadStateIsFresh(path string, info os.FileInfo, entry ReadFileEntry) bool {
	if entry.ContentDigest == "" || entry.FileIdentity == nil || info == nil || !os.SameFile(entry.FileIdentity, info) {
		return false
	}
	if !readEntryMatchesModTime(entry, info.ModTime()) {
		return false
	}
	// Hash and identify the exact same opened descriptor. This remains safe
	// when content is changed and size/mtime are deliberately rolled back.
	snapshot, err := readEditTarget(path, entry.FileIdentity)
	return err == nil && snapshot.ContentDigest == entry.ContentDigest
}

// MarkSedEditReadState refreshes shared read state after a successful sed edit
// so a following Edit sees the exact post-sed content and mtime.
func MarkSedEditReadState(cmd, cwd string, state *ReadFileState) {
	MarkSedEditReadStateForContext(context.Background(), cmd, cwd, state)
}

func MarkSedEditReadStateForContext(ctx context.Context, cmd, cwd string, state *ReadFileState) {
	if state == nil {
		return
	}
	execution := analyzeSedEditExecution(cmd, cwd)
	markSedEditExecutionReadState(ctx, execution, state)
}

func markSedEditExecutionReadState(ctx context.Context, execution sedEditExecution, state *ReadFileState) {
	if state == nil || !execution.EvidenceSafe {
		return
	}
	for _, invocation := range execution.Invocations {
		for _, path := range invocation.Targets {
			snapshot, err := readEditTarget(path, nil)
			if err != nil {
				state.ClearForContext(ctx, path)
				continue
			}
			detected := detectFileEncoding(snapshot.Raw)
			decoded := decodeFileBytes(snapshot.Raw, detected)
			totalLines := readStateTotalLines(decoded)
			coverage, _ := readObservationCoverage(1, totalLines, totalLines)
			state.SetForContext(ctx, path, ReadFileEntry{
				TimestampMs:   snapshot.Info.ModTime().UnixMilli(),
				MtimeNs:       snapshot.Info.ModTime().UnixNano(),
				TotalBytes:    snapshot.Info.Size(),
				ContentDigest: snapshot.ContentDigest,
				FileIdentity:  snapshot.Info,
				TotalLines:    totalLines,
				Coverage:      coverage,
				CoverageKnown: true, CoverageComplete: true, FullSnapshot: true,
				Content:  decoded,
				LastTool: "Bash",
				Encoding: detected.Encoding,
				BOM:      append([]byte(nil), detected.BOM...),
			})
		}
	}
}

// sedEditMutationTargets returns canonical absolute targets for the recognized
// sed -i command. Callers use the set both for deterministic mutation locking
// and for the post-command evidence refresh.
func sedEditMutationTargets(cmd, cwd string) []string {
	return sedEditExecutionMutationTargets(analyzeSedEditExecution(cmd, cwd))
}

func sedEditExecutionMutationTargets(execution sedEditExecution) []string {
	if !execution.EvidenceSafe {
		return nil
	}
	var targets []string
	for _, invocation := range execution.Invocations {
		for _, path := range invocation.Targets {
			if path == "" {
				continue
			}
			targets = append(targets, canonicalFileEditLockPath(path))
			if invocation.Plan.BackupExt != "" {
				targets = append(targets, canonicalFileEditLockPath(path+invocation.Plan.BackupExt))
			}
		}
	}
	return targets
}

func resolveSedTarget(path, cwd string) string {
	if !filepath.IsAbs(path) && cwd != "" {
		path = filepath.Join(cwd, path)
	}
	return filepath.Clean(path)
}
