package shell

import (
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/permissions"
	"github.com/agent-dance/luban/types"
	"mvdan.cc/sh/v3/syntax"
)

// DefaultShellPolicyContext constructs the process-local authority used by
// shell analyzers. Runtime tools should add their exact AllowedDirs and
// sandbox state before calling AnalyzeShellCommand.
func DefaultShellPolicyContext() types.PolicyContext {
	cwd, _ := os.Getwd()
	home, _ := os.UserHomeDir()
	known := make(map[string]string)
	if home != "" {
		known["HOME"] = home
	}
	return types.PolicyContext{
		CWD: cwd, HomeDir: home, AllowedDirs: []string{cwd},
		TrustedTempRoots: []string{os.TempDir()}, KnownEnvironment: known,
	}
}

type shellPolicyValue struct {
	literal         string
	possibleLiteral string
	trustedTemp     bool
	trustedRoot     string
	dynamic         bool
	commandSub      bool
	variable        string
}

// AnalyzeShellCommand is the single shell safety analyzer shared by the hard
// safety gate, mandatory approval gate, Bash tool permission check, and Bash
// execution. PolicyAllow means only that this analyzer adds no stronger gate;
// normal permission rules and sandbox policy still apply.
func AnalyzeShellCommand(command string, policy types.PolicyContext) types.PolicyDecision {
	command = strings.TrimSpace(command)
	if command == "" {
		return allowShellPolicyDecision()
	}
	if policy.CWD == "" {
		policy.CWD, _ = os.Getwd()
	}
	if policy.HomeDir == "" {
		policy.HomeDir, _ = os.UserHomeDir()
	}
	if policy.KnownEnvironment == nil {
		policy.KnownEnvironment = make(map[string]string)
	}
	if policy.HomeDir != "" {
		if _, exists := policy.KnownEnvironment["HOME"]; !exists {
			policy.KnownEnvironment["HOME"] = policy.HomeDir
		}
	}

	// Apply the known non-rm block rules inside the shared analyzer. Recursive deletion is
	// excluded because it needs operand provenance, not regex-only severity.
	best := knownNonRMPolicyDecision(command)
	if violations := findCompoundCDViolations(command); len(violations) > 0 {
		best = strongerShellPolicyDecision(best, blockShellPolicyDecision(
			"shell.policy.block.compound_cd", i18n.KeyShellPolicyBlockKnownPattern,
			compoundCDViolationReasons(violations),
		))
	}

	prog, err := syntax.NewParser(syntax.KeepComments(false)).Parse(strings.NewReader(command), "")
	if err != nil {
		unknown := askShellPolicyDecision(
			"shell.policy.ask.parse_failure", i18n.KeyShellPolicyAskParseFailure,
		)
		unknown.PrivateCause = err
		return strongerShellPolicyDecision(best, unknown)
	}
	sedExecution := analyzeSedEditExecutionFile(prog, policy)
	if sedExecution.HasInPlace && !sedExecution.EvidenceSafe {
		best = strongerShellPolicyDecision(best, unrestrictedCodePolicyDecision("sed"))
	}
	best = strongerShellPolicyDecision(best, analyzeSequentialCWDPolicy(prog, policy))
	best = strongerShellPolicyDecision(best, analyzeUnmodeledCWDPolicy(prog, policy))

	complexControlFlow := false
	// Environment assignments are executable authority, not ordinary data,
	// when they affect command lookup, the dynamic loader, or a tool's delegate
	// configuration. Scan the whole program independently of control-flow
	// inference: a conditional/late mutation is uncertainty and must not restore
	// automatic approval for another external command in the same request.
	trustedProducerEnvironment := !shellPolicyProgramTaintsExecutionEnvironment(prog)
	for name, value := range policy.KnownEnvironment {
		if strings.TrimSpace(value) != "" && name != "PATH" && shellPolicyExecutionEnvironmentName(name) {
			trustedProducerEnvironment = false
			break
		}
	}
	syntax.Walk(prog, func(node syntax.Node) bool {
		switch value := node.(type) {
		case *syntax.IfClause, *syntax.ForClause, *syntax.WhileClause, *syntax.FuncDecl, *syntax.CaseClause, *syntax.Subshell, *syntax.BinaryCmd:
			complexControlFlow = true
		case *syntax.Stmt:
			if value.Background || value.Coprocess || value.Disown {
				complexControlFlow = true
			}
		case *syntax.Assign:
			if value.Name != nil && value.Name.Value == "PATH" {
				trustedProducerEnvironment = false
			}
		case *syntax.DeclClause:
			for _, assignment := range value.Args {
				if assignment.Name != nil && assignment.Name.Value == "PATH" || assignment.Value != nil && shellPolicyAssignmentName(assignment.Value) == "PATH" {
					trustedProducerEnvironment = false
				}
			}
		case *syntax.CallExpr:
			name := cmdName(value)
			if name == "hash" || name == "alias" || name == "enable" {
				trustedProducerEnvironment = false
			}
			if name == "export" || name == "readonly" || name == "declare" || name == "typeset" || name == "local" {
				for _, word := range value.Args[1:] {
					if strings.HasPrefix(wordToString(word), "PATH=") || shellPolicyAssignmentName(word) == "PATH" {
						trustedProducerEnvironment = false
					}
				}
			}
		}
		return !complexControlFlow
	})

	known := make(map[string]shellPolicyValue)
	hasRM := false
	syntax.Walk(prog, func(node syntax.Node) bool {
		if binary, ok := node.(*syntax.BinaryCmd); ok {
			if warning := checkPipeChain(binary); warning != "" {
				best = strongerShellPolicyDecision(best, blockShellPolicyDecision(
					"shell.policy.block.remote_pipe", i18n.KeyShellPolicyBlockKnownPattern, warning,
				))
				return false
			}
		}
		if statement, ok := node.(*syntax.Stmt); ok {
			best = strongerShellPolicyDecision(best, analyzeShellRedirectPolicy(statement, known, policy))
			if best.Disposition == types.PolicyBlock {
				return false
			}
		}
		if declaration, ok := node.(*syntax.DeclClause); ok {
			for _, assignment := range declaration.Args {
				if assignment.Name != nil {
					recordShellPolicyAssignment(known, assignment, policy, !complexControlFlow)
				} else if assignment.Value != nil {
					recordShellPolicyBuiltinAssignments(known, []*syntax.Word{assignment.Value}, policy, !complexControlFlow)
				}
			}
			return true
		}
		call, ok := node.(*syntax.CallExpr)
		if !ok {
			return true
		}
		if len(call.Args) == 0 {
			for _, assignment := range call.Assigns {
				recordShellPolicyAssignment(known, assignment, policy, !complexControlFlow && trustedProducerEnvironment)
			}
			return true
		}
		originalName := cmdName(call)
		if originalName == "export" || originalName == "readonly" || originalName == "declare" || originalName == "typeset" || originalName == "local" {
			for _, assignment := range call.Assigns {
				if assignment.Name != nil {
					recordShellPolicyAssignment(known, assignment, policy, !complexControlFlow)
				} else if assignment.Value != nil {
					recordShellPolicyBuiltinAssignments(known, []*syntax.Word{assignment.Value}, policy, !complexControlFlow)
				}
			}
		}

		name, args, wrapperDecision, callPolicy := unwrapShellPolicyCall(call, known, policy)
		best = strongerShellPolicyDecision(best, wrapperDecision)
		if name == "" {
			return true
		}
		best = strongerShellPolicyDecision(best, analyzeProtectedGlobOperands(args, known, callPolicy))
		if best.Disposition == types.PolicyBlock {
			return false
		}
		switch name {
		case "rm":
			hasRM = true
			best = strongerShellPolicyDecision(best, analyzeRMPolicy(args, known, callPolicy))
		case "bash", "sh", "zsh", "dash", "ash", "ksh":
			best = strongerShellPolicyDecision(best, analyzeNestedShellPolicy(args, known, callPolicy))
		case "busybox", "toybox":
			best = strongerShellPolicyDecision(best, analyzeAppletPolicy(args, known, callPolicy))
		case "dd":
			best = strongerShellPolicyDecision(best, analyzeDDPolicy(args, known, callPolicy))
		case "mkfs", "mkfs.ext4", "mkfs.xfs", "mkfs.btrfs":
			best = strongerShellPolicyDecision(best, blockShellPolicyDecision(
				"shell.policy.block.filesystem_format", i18n.KeyShellPolicyBlockKnownPattern,
				toolRuntimeText(i18n.KeyToolRuntimeDangerousFilesystemFormat),
			))
		case "eval":
			best = strongerShellPolicyDecision(best, analyzeEvalPolicy(args, known, callPolicy))
			applyEvalKnownEffects(args, known, callPolicy)
		case "xargs":
			best = strongerShellPolicyDecision(best, analyzeXargsPolicy(args, known, callPolicy))
		case "find":
			best = strongerShellPolicyDecision(best, analyzeFindExecPolicy(args, known, callPolicy))
		case ".", "source":
			best = strongerShellPolicyDecision(best, askShellPolicyDecision(
				"shell.policy.ask.dynamic_command", i18n.KeyShellPolicyAskDynamicTarget, name,
			))
		case "export", "readonly", "declare", "typeset", "local":
			recordShellPolicyBuiltinAssignments(known, args, callPolicy, !complexControlFlow)
		case "printf":
			updatePrintfAssignedVariable(known, args, callPolicy)
		case "read", "readarray", "mapfile":
			invalidateShellPolicyKnownValues(known)
		}
		best = strongerShellPolicyDecision(best, analyzeUnwrappedWritePolicy(name, args, known, callPolicy))
		authorityDecision := analyzeShellExecutionAuthority(name, args, known, callPolicy)
		if !trustedProducerEnvironment && !readBuiltins[name] {
			authorityDecision = strongerShellPolicyDecision(authorityDecision, unrestrictedCodePolicyDecision(name))
		}
		best = strongerShellPolicyDecision(best, authorityDecision)
		if name != "rm" && cmdName(call) == name {
			if warning := checkCallExpr(call); warning != "" {
				best = strongerShellPolicyDecision(best, blockShellPolicyDecision(
					"shell.policy.block.known_pattern", i18n.KeyShellPolicyBlockKnownPattern, warning,
				))
			}
		}
		return best.Disposition != types.PolicyBlock
	})

	if best.Disposition == types.PolicyAllow {
		if needed, key := bashPermissionApprovalReason(command); needed {
			best = askShellPolicyDecision("shell.policy.ask.structural", i18n.KeyShellPolicyAskStructural, toolPermissionText(key))
		} else if warning, fire := destructiveCommandWarning(shellPolicyStructuralSource(command)); fire && !hasRM {
			best = askShellPolicyDecision("shell.policy.ask.destructive", i18n.KeyShellPolicyAskDestructive, warning)
		}
	}
	return best
}

type sequentialCWDState struct {
	policy    types.PolicyContext
	changed   bool
	uncertain bool
}

func analyzeSequentialCWDPolicy(file *syntax.File, policy types.PolicyContext) types.PolicyDecision {
	if file == nil {
		return allowShellPolicyDecision()
	}
	_, best := analyzeSequentialCWDStatements(file.Stmts, []sequentialCWDState{{policy: policy}})
	return best
}

func analyzeSequentialCWDStatements(statements []*syntax.Stmt, states []sequentialCWDState) ([]sequentialCWDState, types.PolicyDecision) {
	best := allowShellPolicyDecision()
	for _, statement := range statements {
		var decision types.PolicyDecision
		states, decision = analyzeSequentialCWDStatement(statement, states)
		best = strongerShellPolicyDecision(best, decision)
		if best.Disposition == types.PolicyBlock {
			return states, best
		}
	}
	return states, best
}

func analyzeSequentialCWDStatement(statement *syntax.Stmt, states []sequentialCWDState) ([]sequentialCWDState, types.PolicyDecision) {
	if statement == nil || statement.Cmd == nil {
		return states, allowShellPolicyDecision()
	}
	if binary, ok := statement.Cmd.(*syntax.BinaryCmd); ok {
		leftStates, leftDecision := analyzeSequentialCWDStatement(binary.X, states)
		if binary.Op == syntax.Pipe || binary.Op == syntax.PipeAll {
			_, rightDecision := analyzeSequentialCWDStatement(binary.Y, states)
			return states, strongerShellPolicyDecision(leftDecision, rightDecision)
		}
		possible := mergeSequentialCWDStates(states, leftStates)
		rightStates, rightDecision := analyzeSequentialCWDStatement(binary.Y, possible)
		return mergeSequentialCWDStates(possible, rightStates), strongerShellPolicyDecision(leftDecision, rightDecision)
	}
	if subshell, ok := statement.Cmd.(*syntax.Subshell); ok {
		_, decision := analyzeSequentialCWDStatements(subshell.Stmts, states)
		// A subshell observes the caller cwd but never publishes its cd back to
		// the parent shell.
		return states, decision
	}
	if block, ok := statement.Cmd.(*syntax.Block); ok {
		// Brace groups execute in the current shell, so their final cwd remains
		// visible to following statements.
		return analyzeSequentialCWDStatements(block.Stmts, states)
	}
	call, ok := statement.Cmd.(*syntax.CallExpr)
	if !ok || len(call.Args) == 0 {
		return states, allowShellPolicyDecision()
	}
	if cdArgs, ok := sequentialCDArgs(call, states[0].policy); ok {
		return transitionSequentialCD(cdArgs, states), allowShellPolicyDecision()
	}
	best := allowShellPolicyDecision()
	var rendered strings.Builder
	if err := syntax.NewPrinter().Print(&rendered, statement); err != nil {
		return states, askShellPolicyDecision("shell.policy.ask.parse_failure", i18n.KeyShellPolicyAskParseFailure)
	}
	relativeMutation := sequentialStatementHasRelativeMutation(statement)
	for _, state := range states {
		if !state.changed {
			continue
		}
		best = strongerShellPolicyDecision(best, AnalyzeShellCommand(rendered.String(), state.policy))
		if state.uncertain && relativeMutation {
			best = strongerShellPolicyDecision(best, askShellPolicyDecision(
				"shell.policy.ask.dynamic_target", i18n.KeyShellPolicyAskDynamicTarget, "cd cwd",
			))
		}
		if best.Disposition == types.PolicyBlock {
			break
		}
	}
	return states, best
}

func sequentialCDArgs(call *syntax.CallExpr, policy types.PolicyContext) ([]*syntax.Word, bool) {
	if call == nil || len(call.Args) == 0 {
		return nil, false
	}
	name := filepath.Base(wordToString(call.Args[0]))
	if name == "cd" {
		return call.Args[1:], true
	}
	if name != "command" && name != "builtin" {
		return nil, false
	}
	unwrapped, args, decision, _ := unwrapShellPolicyCall(call, map[string]shellPolicyValue{}, policy)
	return args, unwrapped == "cd" && decision.Disposition == types.PolicyAllow
}

func transitionSequentialCD(args []*syntax.Word, states []sequentialCWDState) []sequentialCWDState {
	result := append([]sequentialCWDState(nil), states...)
	for _, state := range states {
		targetWord, dynamic := sequentialCDTarget(args, state.policy)
		if dynamic || targetWord == nil {
			uncertain := state
			uncertain.changed = true
			uncertain.uncertain = true
			result = append(result, uncertain)
			continue
		}
		value := shellPolicyWordValue(targetWord, map[string]shellPolicyValue{}, state.policy)
		childPolicy, decision := shellPolicyWithChildCWD(state.policy, value)
		child := sequentialCWDState{
			policy: childPolicy, changed: true,
			uncertain: state.uncertain || decision.Disposition != types.PolicyAllow,
		}
		result = append(result, child)
	}
	return mergeSequentialCWDStates(nil, result)
}

func sequentialCDTarget(args []*syntax.Word, policy types.PolicyContext) (*syntax.Word, bool) {
	endFlags := false
	for _, word := range args {
		value := shellPolicyWordValue(word, map[string]shellPolicyValue{}, policy)
		if value.dynamic || value.commandSub || value.possibleLiteral != "" {
			return nil, true
		}
		if !endFlags && value.literal == "--" {
			endFlags = true
			continue
		}
		if !endFlags && strings.HasPrefix(value.literal, "-") && value.literal != "-" {
			if value.literal == "-L" || value.literal == "-P" || value.literal == "-e" || value.literal == "-@" {
				continue
			}
			return nil, true
		}
		return word, false
	}
	if policy.HomeDir == "" {
		return nil, true
	}
	return &syntax.Word{Parts: []syntax.WordPart{&syntax.Lit{Value: policy.HomeDir}}}, false
}

func sequentialStatementHasRelativeMutation(statement *syntax.Stmt) bool {
	for _, redirect := range statement.Redirs {
		if !shellPolicyRedirectWritesPath(redirect.Op) {
			continue
		}
		value := shellPolicyWordValue(redirect.Word, map[string]shellPolicyValue{}, types.PolicyContext{})
		if value.dynamic || value.commandSub || value.possibleLiteral != "" || !filepath.IsAbs(value.literal) {
			return true
		}
	}
	call, ok := statement.Cmd.(*syntax.CallExpr)
	if !ok {
		return false
	}
	return findActionHasRelativeMutationTarget(call.Args, map[string]shellPolicyValue{}, types.PolicyContext{})
}

func mergeSequentialCWDStates(groups ...[]sequentialCWDState) []sequentialCWDState {
	merged := make([]sequentialCWDState, 0)
	for _, states := range groups {
		for _, candidate := range states {
			duplicate := false
			for index := range merged {
				if merged[index].policy.CWD == candidate.policy.CWD && merged[index].uncertain == candidate.uncertain {
					merged[index].changed = merged[index].changed || candidate.changed
					duplicate = true
					break
				}
			}
			if !duplicate {
				merged = append(merged, candidate)
			}
		}
	}
	return merged
}

func analyzeUnmodeledCWDPolicy(file *syntax.File, policy types.PolicyContext) types.PolicyDecision {
	if file == nil {
		return allowShellPolicyDecision()
	}
	hasComplexControl, hasCD, hasUnmodeledTransition, hasRelativeMutation := false, false, false, false
	if _, exists := policy.KnownEnvironment["CDPATH"]; exists {
		hasUnmodeledTransition = true
	}
	syntax.Walk(file, func(node syntax.Node) bool {
		switch value := node.(type) {
		case *syntax.IfClause, *syntax.ForClause, *syntax.WhileClause, *syntax.CaseClause:
			hasComplexControl = true
		case *syntax.Assign:
			if value.Name != nil && value.Name.Value == "CDPATH" {
				hasUnmodeledTransition = true
			}
		case *syntax.Stmt:
			hasRelativeMutation = hasRelativeMutation || sequentialStatementHasRelativeMutation(value)
		case *syntax.CallExpr:
			if len(value.Args) == 0 {
				break
			}
			name := filepath.Base(wordToString(value.Args[0]))
			switch name {
			case "cd":
				hasCD = true
			case "pushd", "popd":
				hasUnmodeledTransition = true
			case "command", "builtin":
				unwrapped, _, _, _ := unwrapShellPolicyCall(value, map[string]shellPolicyValue{}, policy)
				hasCD = hasCD || unwrapped == "cd"
			}
		}
		return true
	})
	if hasRelativeMutation && (hasUnmodeledTransition || hasComplexControl && hasCD) {
		return askShellPolicyDecision(
			"shell.policy.ask.dynamic_target", i18n.KeyShellPolicyAskDynamicTarget, "CDPATH/cd/pushd",
		)
	}
	return allowShellPolicyDecision()
}

func invalidateShellPolicyKnownValues(known map[string]shellPolicyValue) {
	for name, previous := range known {
		uncertain := shellPolicyValue{dynamic: true, variable: name}
		if previous.literal != "" {
			uncertain.possibleLiteral = previous.literal
		} else {
			uncertain.possibleLiteral = previous.possibleLiteral
		}
		known[name] = uncertain
	}
}

func updatePrintfAssignedVariable(known map[string]shellPolicyValue, args []*syntax.Word, policy types.PolicyContext) {
	for index, word := range args {
		if wordToString(word) != "-v" || index+1 >= len(args) {
			continue
		}
		name := wordToString(args[index+1])
		if name == "" {
			invalidateShellPolicyKnownValues(known)
			return
		}
		if index+2 < len(args) {
			format := shellPolicyWordValue(args[index+2], known, policy)
			if !format.dynamic && !format.commandSub && format.possibleLiteral == "" {
				assigned := ""
				switch {
				case !strings.Contains(format.literal, "%"):
					assigned = format.literal
				case format.literal == "%s" && index+3 < len(args):
					argument := shellPolicyWordValue(args[index+3], known, policy)
					if !argument.dynamic && !argument.commandSub && argument.possibleLiteral == "" {
						assigned = argument.literal
					}
				}
				if assigned != "" {
					known[name] = shellPolicyValue{literal: assigned}
					return
				}
			}
		}
		previous := known[name]
		uncertain := shellPolicyValue{dynamic: true, variable: name, possibleLiteral: previous.literal}
		known[name] = uncertain
		return
	}
}

func applyEvalKnownEffects(args []*syntax.Word, known map[string]shellPolicyValue, policy types.PolicyContext) {
	args = evalPayloadWords(args)
	var command strings.Builder
	for index, word := range args {
		value := shellPolicyWordValue(word, known, policy)
		if value.dynamic || value.commandSub || value.possibleLiteral != "" {
			invalidateShellPolicyKnownValues(known)
			return
		}
		if index > 0 {
			command.WriteByte(' ')
		}
		command.WriteString(value.literal)
	}
	program, err := syntax.NewParser(syntax.KeepComments(false)).Parse(strings.NewReader(command.String()), "")
	if err != nil {
		invalidateShellPolicyKnownValues(known)
		return
	}
	deterministic := true
	syntax.Walk(program, func(node syntax.Node) bool {
		switch value := node.(type) {
		case *syntax.IfClause, *syntax.ForClause, *syntax.WhileClause, *syntax.FuncDecl, *syntax.CaseClause, *syntax.Subshell, *syntax.BinaryCmd:
			deterministic = false
		case *syntax.Stmt:
			if value.Background || value.Coprocess || value.Disown {
				deterministic = false
			}
		}
		return deterministic
	})
	if !deterministic {
		invalidateShellPolicyKnownValues(known)
		return
	}
	syntax.Walk(program, func(node syntax.Node) bool {
		switch value := node.(type) {
		case *syntax.Assign:
			recordShellPolicyAssignment(known, value, policy, true)
		case *syntax.DeclClause:
			for _, assignment := range value.Args {
				if assignment.Name != nil {
					recordShellPolicyAssignment(known, assignment, policy, true)
				} else if assignment.Value != nil {
					recordShellPolicyBuiltinAssignments(known, []*syntax.Word{assignment.Value}, policy, true)
				}
			}
			// DeclClause owns its assignment nodes. Do not descend and record the
			// same assignments a second time through the *syntax.Assign case.
			return false
		}
		return true
	})
}

func analyzeXargsPolicy(args []*syntax.Word, known map[string]shellPolicyValue, policy types.PolicyContext) types.PolicyDecision {
	valueFlags := map[string]bool{"-a": true, "--arg-file": true, "-d": true, "--delimiter": true, "-E": true, "--eof": true, "-I": true, "--replace": true, "-L": true, "--max-lines": true, "-n": true, "--max-args": true, "-P": true, "--max-procs": true, "-s": true, "--max-chars": true}
	for index := 0; index < len(args); index++ {
		value := shellPolicyWordValue(args[index], known, policy)
		if value.dynamic || value.commandSub || value.possibleLiteral != "" {
			return strongerShellPolicyDecision(
				askShellPolicyDecision("shell.policy.ask.dynamic_command", i18n.KeyShellPolicyAskDynamicTarget, value.variable),
				analyzeShellPolicySuffixes(args[index+1:], known, policy),
			)
		}
		literal := value.literal
		if literal == "--" {
			if index+1 >= len(args) {
				return allowShellPolicyDecision()
			}
			return strongerShellPolicyDecision(
				analyzeShellPolicySuffixes(args[index+1:], known, policy),
				askShellPolicyDecision("shell.policy.ask.dynamic_target", i18n.KeyShellPolicyAskDynamicTarget, "xargs stdin"),
			)
		}
		if valueFlags[literal] {
			index++
			if index >= len(args) {
				return askShellPolicyDecision("shell.policy.ask.parse_failure", i18n.KeyShellPolicyAskParseFailure)
			}
			continue
		}
		if isAttachedXargsValueFlag(literal) {
			continue
		}
		if strings.HasPrefix(literal, "--") && strings.Contains(literal, "=") || literal == "-0" || literal == "--null" || literal == "-r" || literal == "--no-run-if-empty" || literal == "-t" || literal == "--verbose" {
			continue
		}
		if strings.HasPrefix(literal, "-") {
			return strongerShellPolicyDecision(
				askShellPolicyDecision("shell.policy.ask.dynamic_flags", i18n.KeyShellPolicyAskDynamicFlags),
				analyzeShellPolicySuffixes(args[index+1:], known, policy),
			)
		}
		return strongerShellPolicyDecision(
			analyzeShellPolicyWords(args[index:], known, policy),
			askShellPolicyDecision("shell.policy.ask.dynamic_target", i18n.KeyShellPolicyAskDynamicTarget, "xargs stdin"),
		)
	}
	return allowShellPolicyDecision()
}

func isAttachedXargsValueFlag(value string) bool {
	if len(value) <= 2 || value[0] != '-' || value[1] == '-' {
		return false
	}
	switch value[1] {
	case 'a', 'd', 'E', 'I', 'L', 'n', 'P', 's':
		return true
	default:
		return false
	}
}

func analyzeFindExecPolicy(args []*syntax.Word, known map[string]shellPolicyValue, policy types.PolicyContext) types.PolicyDecision {
	best := analyzeFindOutputPolicy(args, known, policy)
	if best.Disposition == types.PolicyBlock {
		return best
	}
	rootAnalysis := analyzeFindSearchRoots(args, known, policy)
	rootDecision := rootAnalysis.decision
	for _, word := range args {
		value := shellPolicyWordValue(word, known, policy)
		if !value.dynamic && !value.commandSub && value.possibleLiteral == "" && value.literal == "-delete" {
			if rootDecision.Disposition == types.PolicyBlock {
				return rootDecision
			}
			best = strongerShellPolicyDecision(best, askShellPolicyDecision(
				"shell.policy.ask.destructive", i18n.KeyShellPolicyAskDestructive,
				toolRuntimeText(i18n.KeyToolRuntimeDestructiveFindDelete),
			))
			break
		}
	}
	for index := 0; index < len(args); index++ {
		value := shellPolicyWordValue(args[index], known, policy)
		if value.dynamic || value.commandSub || value.possibleLiteral != "" {
			best = strongerShellPolicyDecision(best, askShellPolicyDecision("shell.policy.ask.dynamic_command", i18n.KeyShellPolicyAskDynamicTarget, value.variable))
			continue
		}
		operator := value.literal
		if operator != "-exec" && operator != "-execdir" && operator != "-ok" && operator != "-okdir" {
			continue
		}
		start := index + 1
		end := start
		for end < len(args) {
			terminator := wordToString(args[end])
			if terminator == ";" || terminator == "+" {
				break
			}
			end++
		}
		if start == end {
			return askShellPolicyDecision("shell.policy.ask.parse_failure", i18n.KeyShellPolicyAskParseFailure)
		}
		actionWords := args[start:end]
		action := analyzeShellPolicyWords(actionWords, known, policy)
		isExecdir := operator == "-execdir" || operator == "-okdir"
		if isExecdir && findActionHasRelativeMutationTarget(actionWords, known, policy) {
			if rootDecision.Disposition == types.PolicyBlock {
				action = rootDecision
			} else {
				for _, root := range rootAnalysis.staticRoots {
					for _, childPolicy := range findExecdirCandidatePolicies(root, policy) {
						action = strongerShellPolicyDecision(action, analyzeShellPolicyWords(actionWords, known, childPolicy))
					}
				}
				// GNU find runs -execdir/-okdir in every matched entry's parent
				// directory. Static search roots bound the tree but cannot enumerate
				// all possible match directories, so a relative mutation always
				// retains a mandatory approval floor even when root and parent are
				// safe. A known protected/system root above still wins as Block.
				action = strongerShellPolicyDecision(action, askShellPolicyDecision(
					"shell.policy.ask.dynamic_target", i18n.KeyShellPolicyAskDynamicTarget, "find -execdir cwd",
				))
			}
		}
		usesPlaceholder := false
		for _, word := range args[start:end] {
			if strings.Contains(shellPolicyWordValue(word, known, policy).literal, "{}") {
				usesPlaceholder = true
				break
			}
		}
		if usesPlaceholder {
			if rootDecision.Disposition == types.PolicyBlock && (action.Disposition != types.PolicyAllow || findActionMayMutate(actionWords, known, policy)) {
				action = rootDecision
			} else {
				action = strongerShellPolicyDecision(action, askShellPolicyDecision("shell.policy.ask.dynamic_target", i18n.KeyShellPolicyAskDynamicTarget, "find {}"))
			}
		}
		best = strongerShellPolicyDecision(best, action)
		if best.Disposition == types.PolicyBlock {
			return best
		}
	}
	return best
}

func analyzeFindOutputPolicy(args []*syntax.Word, known map[string]shellPolicyValue, policy types.PolicyContext) types.PolicyDecision {
	best := allowShellPolicyDecision()
	for index := 0; index < len(args); index++ {
		operatorValue := shellPolicyWordValue(args[index], known, policy)
		if operatorValue.dynamic || operatorValue.commandSub || operatorValue.possibleLiteral != "" {
			continue
		}
		operator := operatorValue.literal
		if operator != "-fprint" && operator != "-fprint0" && operator != "-fprintf" && operator != "-fls" {
			continue
		}
		if index+1 >= len(args) {
			return strongerShellPolicyDecision(best, askShellPolicyDecision("shell.policy.ask.parse_failure", i18n.KeyShellPolicyAskParseFailure))
		}
		targetWord := args[index+1]
		target := shellPolicyWordValue(targetWord, known, policy)
		if target.possibleLiteral != "" {
			candidate := classifyDestructivePathWithGlob(target.possibleLiteral, policy, shellPolicyWordHasActiveUnquotedGlob(targetWord, target))
			if candidate.Disposition == types.PolicyBlock {
				return candidate
			}
		}
		if target.dynamic || target.commandSub {
			best = strongerShellPolicyDecision(best, askShellPolicyDecision("shell.policy.ask.dynamic_target", i18n.KeyShellPolicyAskDynamicTarget, target.variable))
		} else {
			best = strongerShellPolicyDecision(best, classifyDestructivePathWithGlob(target.literal, policy, shellPolicyWordHasActiveUnquotedGlob(targetWord, target)))
		}
		if best.Disposition == types.PolicyBlock {
			return best
		}
		if operator == "-fprintf" {
			if index+2 >= len(args) {
				return strongerShellPolicyDecision(best, askShellPolicyDecision("shell.policy.ask.parse_failure", i18n.KeyShellPolicyAskParseFailure))
			}
			index += 2 // output path and format
		} else {
			index++ // output path
		}
	}
	return best
}

type findSearchRootAnalysis struct {
	decision    types.PolicyDecision
	staticRoots []string
	uncertain   bool
}

func analyzeFindSearchRoots(args []*syntax.Word, known map[string]shellPolicyValue, policy types.PolicyContext) findSearchRootAnalysis {
	best := allowShellPolicyDecision()
	staticRoots := make([]string, 0, 2)
	foundRoot, uncertainLeading, parsingLeading := false, false, true
	for index := 0; index < len(args); index++ {
		word := args[index]
		value := shellPolicyWordValue(word, known, policy)
		if value.dynamic || value.commandSub || value.possibleLiteral != "" {
			uncertainLeading = true
			best = strongerShellPolicyDecision(best, askShellPolicyDecision("shell.policy.ask.dynamic_target", i18n.KeyShellPolicyAskDynamicTarget, value.variable))
			if value.possibleLiteral != "" {
				foundRoot = true
				staticRoots = append(staticRoots, value.possibleLiteral)
				best = strongerShellPolicyDecision(best, classifyDestructivePathWithGlob(
					value.possibleLiteral, policy, shellPolicyWordHasActiveUnquotedGlob(word, value),
				))
			}
			continue
		}
		literal := value.literal
		if parsingLeading {
			switch {
			case literal == "--":
				parsingLeading = false
				continue
			case literal == "-H" || literal == "-L" || literal == "-P":
				continue
			case literal == "-D" || literal == "-O":
				if index+1 >= len(args) {
					return findSearchRootAnalysis{
						decision:    strongerShellPolicyDecision(best, askShellPolicyDecision("shell.policy.ask.parse_failure", i18n.KeyShellPolicyAskParseFailure)),
						staticRoots: staticRoots, uncertain: true,
					}
				}
				index++
				continue
			case strings.HasPrefix(literal, "-D") && len(literal) > 2 || strings.HasPrefix(literal, "-O") && len(literal) > 2:
				continue
			case isFindExpressionOperator(literal):
				parsingLeading = false
			case strings.HasPrefix(literal, "-"):
				// Unknown leading options may or may not consume following words.
				// Preserve Ask but continue scanning every literal suffix for a
				// stronger root/system/protected-path Block.
				uncertainLeading = true
				best = strongerShellPolicyDecision(best, askShellPolicyDecision("shell.policy.ask.dynamic_flags", i18n.KeyShellPolicyAskDynamicFlags))
				continue
			default:
				parsingLeading = false
			}
		}
		if isFindExpressionOperator(literal) || literal == "!" || literal == "(" || literal == ")" {
			break
		}
		foundRoot = true
		staticRoots = append(staticRoots, literal)
		best = strongerShellPolicyDecision(best, classifyDestructivePathWithGlob(literal, policy, shellPolicyWordHasActiveUnquotedGlob(word, value)))
	}
	if !foundRoot {
		if uncertainLeading {
			return findSearchRootAnalysis{decision: best, uncertain: true}
		}
		return findSearchRootAnalysis{decision: classifyDestructivePath(".", policy), staticRoots: []string{"."}}
	}
	return findSearchRootAnalysis{decision: best, staticRoots: staticRoots, uncertain: uncertainLeading}
}

func findExecdirCandidatePolicies(root string, policy types.PolicyContext) []types.PolicyContext {
	if !filepath.IsAbs(root) {
		root = filepath.Join(policy.CWD, root)
	}
	root = filepath.Clean(root)
	if resolved, err := resolveExistingPolicyPath(root); err == nil {
		root = resolved
	}
	candidates := make([]types.PolicyContext, 0, 2)
	for _, cwd := range []string{filepath.Dir(root), root} {
		candidate := policy
		candidate.CWD = cwd
		duplicate := false
		for _, previous := range candidates {
			if previous.CWD == candidate.CWD {
				duplicate = true
				break
			}
		}
		if !duplicate {
			candidates = append(candidates, candidate)
		}
	}
	return candidates
}

func isFindExpressionOperator(value string) bool {
	switch value {
	case "-amin", "-anewer", "-atime", "-cmin", "-cnewer", "-ctime", "-delete", "-empty", "-exec", "-execdir", "-executable", "-false", "-fls", "-fprint", "-fprint0", "-fprintf", "-fstype", "-gid", "-group", "-ilname", "-iname", "-inum", "-ipath", "-iregex", "-links", "-lname", "-ls", "-maxdepth", "-mindepth", "-mmin", "-mount", "-mtime", "-name", "-newer", "-nogroup", "-nouser", "-ok", "-okdir", "-path", "-perm", "-print", "-print0", "-printf", "-prune", "-quit", "-readable", "-regex", "-samefile", "-size", "-true", "-type", "-uid", "-used", "-user", "-wholename", "-writable", "-xdev", "-xtype":
		return true
	default:
		return false
	}
}

func findActionMayMutate(words []*syntax.Word, known map[string]shellPolicyValue, policy types.PolicyContext) bool {
	var command strings.Builder
	for index, word := range words {
		value := shellPolicyWordValue(word, known, policy)
		if value.dynamic || value.commandSub || value.possibleLiteral != "" {
			return true
		}
		if index > 0 {
			command.WriteByte(' ')
		}
		command.WriteString(shellPolicyQuoteLiteral(value.literal))
	}
	semantics := ClassifyCommand(command.String())
	return semantics == SemanticWrite || semantics == SemanticDestructive
}

func analyzeShellPolicyWords(words []*syntax.Word, known map[string]shellPolicyValue, policy types.PolicyContext) types.PolicyDecision {
	var command strings.Builder
	for index, word := range words {
		value := shellPolicyWordValue(word, known, policy)
		if value.dynamic || value.commandSub || value.possibleLiteral != "" {
			return askShellPolicyDecision("shell.policy.ask.dynamic_command", i18n.KeyShellPolicyAskDynamicTarget, value.variable)
		}
		if index > 0 {
			command.WriteByte(' ')
		}
		command.WriteString(shellPolicyQuoteLiteral(value.literal))
	}
	return AnalyzeShellCommand(command.String(), policy)
}

func shellPolicyQuoteLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func shellPolicyAssignmentName(word *syntax.Word) string {
	if word == nil || len(word.Parts) == 0 {
		return ""
	}
	literal, ok := word.Parts[0].(*syntax.Lit)
	if !ok {
		return ""
	}
	name, _, found := strings.Cut(literal.Value, "=")
	if !found {
		return ""
	}
	return name
}

func recordShellPolicyBuiltinAssignments(known map[string]shellPolicyValue, args []*syntax.Word, policy types.PolicyContext, deterministic bool) {
	for _, word := range args {
		name := shellPolicyAssignmentName(word)
		if name == "" {
			continue
		}
		literal := wordToString(word)
		value := shellPolicyValue{dynamic: true, variable: name}
		if literal != "" {
			_, assigned, _ := strings.Cut(literal, "=")
			value = shellPolicyValue{literal: assigned}
		}
		if !deterministic {
			if previous, exists := known[name]; exists {
				for _, candidate := range []string{value.literal, previous.literal, previous.possibleLiteral} {
					if candidate != "" && classifyDestructivePath(candidate, policy).Disposition == types.PolicyBlock {
						value = shellPolicyValue{dynamic: true, variable: name, possibleLiteral: candidate}
						break
					}
				}
			}
		}
		known[name] = value
	}
}

func analyzeEvalPolicy(args []*syntax.Word, known map[string]shellPolicyValue, policy types.PolicyContext) types.PolicyDecision {
	args = evalPayloadWords(args)
	if len(args) == 0 {
		return allowShellPolicyDecision()
	}
	var command strings.Builder
	for index, word := range args {
		value := shellPolicyWordValue(word, known, policy)
		if value.dynamic || value.commandSub || value.possibleLiteral != "" {
			return strongerShellPolicyDecision(
				askShellPolicyDecision("shell.policy.ask.dynamic_command", i18n.KeyShellPolicyAskDynamicTarget, value.variable),
				analyzeEvalPolicy(args[index+1:], known, policy),
			)
		}
		if index > 0 {
			command.WriteByte(' ')
		}
		command.WriteString(value.literal)
	}
	return AnalyzeShellCommand(command.String(), policy)
}

func evalPayloadWords(args []*syntax.Word) []*syntax.Word {
	if len(args) > 0 && wordToString(args[0]) == "--" {
		return args[1:]
	}
	return args
}

func analyzeUnwrappedWritePolicy(name string, args []*syntax.Word, known map[string]shellPolicyValue, policy types.PolicyContext) types.PolicyDecision {
	switch name {
	case "tee", "sed", "cp", "install", "mv", "scp", "rsync", "dd", "truncate", "chmod", "chown", "ln", "touch", "mkdir":
	default:
		return allowShellPolicyDecision()
	}
	literals := make([]string, 0, len(args))
	for _, word := range args {
		value := shellPolicyWordValue(word, known, policy)
		if value.dynamic || value.commandSub || value.possibleLiteral != "" {
			return askShellPolicyDecision("shell.policy.ask.dynamic_target", i18n.KeyShellPolicyAskDynamicTarget, value.variable)
		}
		literals = append(literals, value.literal)
	}
	if warning := checkWriteCommandArgs(name, literals); warning != "" {
		return blockShellPolicyDecision("shell.policy.block.protected", i18n.KeyShellPolicyBlockKnownPattern, warning)
	}
	best := allowShellPolicyDecision()
	var targets []string
	switch name {
	case "tee", "touch", "mkdir", "chmod", "chown":
		targets = allNonFlagArgs(literals)
	case "cp", "install", "mv", "ln":
		targets = multiFileWriteTargets(name, literals)
	case "scp", "rsync":
		if target := lastNonFlagArg(literals); target != "" {
			targets = []string{target}
		}
	case "truncate":
		targets = truncateTargets(literals)
	case "sed":
		if hasFlag(literals, 'i') || hasLongFlag(literals, "--in-place") {
			targets = sedFileOperands(literals)
		}
	case "dd":
		for _, argument := range literals {
			if strings.HasPrefix(argument, "of=") {
				targets = append(targets, strings.TrimPrefix(argument, "of="))
			}
		}
	}
	for _, target := range targets {
		candidate := classifyDestructivePath(target, policy)
		if candidate.Disposition == types.PolicyBlock {
			return candidate
		}
	}
	return best
}

func multiFileWriteTargets(name string, args []string) []string {
	if name != "cp" && name != "install" && name != "mv" && name != "ln" {
		return nil
	}
	var targetDirectory string
	operands := make([]string, 0, len(args))
	endFlags := false
	for index := 0; index < len(args); index++ {
		argument := args[index]
		if !endFlags && argument == "--" {
			endFlags = true
			continue
		}
		if !endFlags && (argument == "-t" || argument == "--target-directory") {
			if index+1 < len(args) {
				index++
				targetDirectory = args[index]
			}
			continue
		}
		if !endFlags && strings.HasPrefix(argument, "--target-directory=") {
			targetDirectory = strings.TrimPrefix(argument, "--target-directory=")
			continue
		}
		if !endFlags && strings.HasPrefix(argument, "-t") && len(argument) > 2 {
			targetDirectory = strings.TrimPrefix(argument, "-t")
			continue
		}
		if !endFlags && strings.HasPrefix(argument, "-") && argument != "-" {
			continue
		}
		operands = append(operands, argument)
	}
	if targetDirectory == "" {
		if len(operands) == 0 {
			return nil
		}
		return []string{operands[len(operands)-1]}
	}
	targets := make([]string, 0, len(operands))
	for _, source := range operands {
		base := filepath.Base(filepath.Clean(source))
		if base == "." || base == string(filepath.Separator) {
			continue
		}
		targets = append(targets, filepath.Join(targetDirectory, base))
	}
	return targets
}

func findActionHasRelativeMutationTarget(words []*syntax.Word, known map[string]shellPolicyValue, policy types.PolicyContext) bool {
	if len(words) == 0 {
		return false
	}
	values := make([]string, 0, len(words))
	for _, word := range words {
		value := shellPolicyWordValue(word, known, policy)
		if value.dynamic || value.commandSub || value.possibleLiteral != "" {
			return true
		}
		values = append(values, value.literal)
	}
	name := filepath.Base(values[0])
	args := values[1:]
	var targets []string
	switch name {
	case "rm":
		endFlags := false
		for _, argument := range args {
			if !endFlags && argument == "--" {
				endFlags = true
				continue
			}
			if !endFlags && strings.HasPrefix(argument, "-") && argument != "-" {
				continue
			}
			targets = append(targets, argument)
		}
	case "tee", "touch", "mkdir", "chmod", "chown":
		targets = allNonFlagArgs(args)
	case "cp", "install", "mv", "ln":
		targets = multiFileWriteTargets(name, args)
	case "scp", "rsync":
		if target := lastNonFlagArg(args); target != "" && !strings.Contains(target, ":") {
			targets = []string{target}
		}
	case "truncate":
		targets = truncateTargets(args)
	case "sed":
		if hasFlag(args, 'i') || hasLongFlag(args, "--in-place") {
			targets = sedFileOperands(args)
		}
	case "dd":
		for _, argument := range args {
			if strings.HasPrefix(argument, "of=") {
				targets = append(targets, strings.TrimPrefix(argument, "of="))
			}
		}
	default:
		// Unknown destructive/write-capable commands retain the conservative
		// cwd dependency because their target grammar is not proven here.
		return findActionMayMutate(words, known, policy)
	}
	for _, target := range targets {
		if target == "" || strings.Contains(target, "{}") || !filepath.IsAbs(target) {
			return true
		}
	}
	return false
}

func allowShellPolicyDecision() types.PolicyDecision {
	return types.PolicyDecision{Disposition: types.PolicyAllow, Code: "shell.policy.allow"}
}

func blockShellPolicyDecision(code string, key i18n.Key, args ...any) types.PolicyDecision {
	return types.PolicyDecision{
		Disposition: types.PolicyBlock, Code: code, PublicKey: key, PublicArgs: args,
		RuleSource: "shell_policy", Risk: types.PolicyRiskCritical,
	}
}

func askShellPolicyDecision(code string, key i18n.Key, args ...any) types.PolicyDecision {
	return types.PolicyDecision{
		Disposition: types.PolicyRequiredAsk, Code: code, PublicKey: key, PublicArgs: args,
		RuleSource: "shell_policy", Risk: types.PolicyRiskHigh,
		Remediation: &types.PolicyRemediation{Action: "request_approval", PublicKey: i18n.KeyShellPolicyRemediationApprove},
	}
}

func unrestrictedCodePolicyDecision(executable string) types.PolicyDecision {
	decision := askShellPolicyDecision(
		types.PolicyCodeUnrestrictedCode,
		i18n.KeyShellPolicyAskUnrestrictedCode,
		executable,
	)
	decision.Risk = types.PolicyRiskUnrestrictedCode
	return decision
}

func strongerShellPolicyDecision(current, candidate types.PolicyDecision) types.PolicyDecision {
	strength := func(d types.PolicyDisposition) int {
		switch d {
		case types.PolicyBlock:
			return 2
		case types.PolicyRequiredAsk:
			return 1
		default:
			return 0
		}
	}
	if strength(candidate.Disposition) > strength(current.Disposition) {
		return candidate
	}
	if strength(candidate.Disposition) == strength(current.Disposition) &&
		candidate.Disposition == types.PolicyRequiredAsk &&
		candidate.Risk == types.PolicyRiskUnrestrictedCode && current.Risk != types.PolicyRiskUnrestrictedCode {
		return candidate
	}
	return current
}

func knownNonRMPolicyDecision(command string) types.PolicyDecision {
	structural := shellPolicyStructuralSource(command)
	for _, finding := range EvaluateBashSecurity(structural) {
		if finding.Severity < SeverityBlock || strings.HasPrefix(finding.Rule.Name, "rm-") {
			continue
		}
		return blockShellPolicyDecision(
			"shell.policy.block."+finding.Rule.Name,
			i18n.KeyShellPolicyBlockKnownPattern,
			toolRuntimeFormat(finding.Rule.ReasonKey, finding.Rule.ReasonArgs...),
		)
	}
	for _, dp := range dangerousPatterns {
		if dp.key == i18n.KeyToolRuntimeDangerousRootDelete {
			continue
		}
		if dp.pattern.MatchString(structural) {
			return blockShellPolicyDecision("shell.policy.block.known_pattern", i18n.KeyShellPolicyBlockKnownPattern, toolRuntimeText(dp.key))
		}
	}
	return allowShellPolicyDecision()
}

// shellPolicyStructuralSource removes quoted payloads and comments before the
// pattern rules run. AST-based checks still inspect actual argument
// values, but strings passed as data must not masquerade as shell structure.
func shellPolicyStructuralSource(command string) string {
	data := []byte(command)
	const (
		structuralNormal = iota
		structuralSingleQuote
		structuralDoubleQuote
		structuralBacktick
		structuralComment
	)
	state := structuralNormal
	for index := 0; index < len(data); index++ {
		character := data[index]
		switch state {
		case structuralComment:
			if character == '\n' {
				state = structuralNormal
			} else {
				data[index] = ' '
			}
		case structuralSingleQuote:
			data[index] = ' '
			if character == '\'' {
				state = structuralNormal
			}
		case structuralDoubleQuote:
			data[index] = ' '
			if character == '\\' && index+1 < len(data) {
				index++
				data[index] = ' '
			} else if character == '"' {
				state = structuralNormal
			}
		case structuralBacktick:
			data[index] = ' '
			if character == '`' {
				state = structuralNormal
			}
		default:
			switch character {
			case '\\':
				data[index] = ' '
				if index+1 < len(data) {
					index++
					data[index] = ' '
				}
			case '\'':
				data[index] = ' '
				state = structuralSingleQuote
			case '"':
				data[index] = ' '
				state = structuralDoubleQuote
			case '`':
				data[index] = ' '
				state = structuralBacktick
			case '#':
				if index == 0 || strings.ContainsRune(" \t\r\n;|&()", rune(data[index-1])) {
					data[index] = ' '
					state = structuralComment
				}
			}
		}
	}
	return string(data)
}

func recordShellPolicyAssignment(known map[string]shellPolicyValue, assignment *syntax.Assign, policy types.PolicyContext, trustProvenance bool) {
	if assignment == nil || assignment.Name == nil {
		return
	}
	name := assignment.Name.Value
	if assignment.Append || assignment.Value == nil {
		delete(known, name)
		return
	}
	value := shellPolicyWordValue(assignment.Value, known, policy)
	if value.trustedTemp && value.literal != "" {
		value = shellPolicyValue{dynamic: true, variable: name}
	}
	if !trustProvenance {
		uncertain := shellPolicyValue{dynamic: true, variable: name}
		candidates := []string{value.literal, value.possibleLiteral}
		if previous, exists := known[name]; exists {
			candidates = append(candidates, previous.literal, previous.possibleLiteral)
		}
		for _, candidate := range candidates {
			if candidate == "" {
				continue
			}
			if classifyDestructivePath(candidate, policy).Disposition == types.PolicyBlock {
				uncertain.possibleLiteral = candidate
				break
			}
		}
		value = uncertain
	}
	known[name] = value
}

func shellPolicyProgramTaintsExecutionEnvironment(prog *syntax.File) bool {
	if prog == nil {
		return false
	}
	tainted := false
	syntax.Walk(prog, func(node syntax.Node) bool {
		if tainted {
			return false
		}
		switch value := node.(type) {
		case *syntax.Assign:
			if value.Name != nil && shellPolicyExecutionEnvironmentName(value.Name.Value) {
				tainted = true
				return false
			}
		case *syntax.DeclClause:
			for _, assignment := range value.Args {
				name := ""
				if assignment.Name != nil {
					name = assignment.Name.Value
				} else if assignment.Value != nil {
					name = shellPolicyAssignmentName(assignment.Value)
				}
				if shellPolicyExecutionEnvironmentName(name) {
					tainted = true
					return false
				}
			}
		case *syntax.CallExpr:
			name := cmdName(value)
			switch name {
			case "export", "readonly", "declare", "typeset", "local", "sudo", "doas":
				for _, word := range value.Args[1:] {
					if shellPolicyExecutionEnvironmentName(shellPolicyEnvironmentWordName(word)) {
						tainted = true
						return false
					}
				}
			case "env":
				if shellPolicyEnvWordsTaintExecutionEnvironment(value.Args[1:]) {
					tainted = true
					return false
				}
			case "unset", "read", "readarray", "mapfile", "getopts":
				for _, word := range value.Args[1:] {
					literal := strings.TrimSpace(wordToString(word))
					if !strings.HasPrefix(literal, "-") && shellPolicyExecutionEnvironmentName(literal) {
						tainted = true
						return false
					}
				}
			case "printf":
				for index, word := range value.Args[1:] {
					if wordToString(word) == "-v" && index+2 < len(value.Args) &&
						shellPolicyExecutionEnvironmentName(wordToString(value.Args[index+2])) {
						tainted = true
						return false
					}
				}
			}
		}
		return true
	})
	return tainted
}

func shellPolicyEnvironmentWordName(word *syntax.Word) string {
	if name := shellPolicyAssignmentName(word); name != "" {
		return name
	}
	literal := strings.TrimSpace(wordToString(word))
	if literal == "" || strings.HasPrefix(literal, "-") || strings.Contains(literal, "=") {
		return ""
	}
	return literal
}

func shellPolicyEnvWordsTaintExecutionEnvironment(words []*syntax.Word) bool {
	for index := 0; index < len(words); index++ {
		value := strings.TrimSpace(wordToString(words[index]))
		if value == "--" {
			words = words[index+1:]
			break
		}
		switch value {
		case "-i", "--ignore-environment", "-0", "--null", "-v", "--debug":
			continue
		case "-u", "--unset", "-C", "--chdir":
			index++
			continue
		case "-S", "--split-string":
			// The recursively parsed split payload owns the exact decision.
			return false
		}
		if strings.HasPrefix(value, "--unset=") || strings.HasPrefix(value, "--chdir=") ||
			strings.HasPrefix(value, "-C") && len(value) > 2 {
			continue
		}
		if strings.HasPrefix(value, "--split-string=") {
			return false
		}
		if strings.HasPrefix(value, "-") {
			return false
		}
		words = words[index:]
		break
	}
	for _, word := range words {
		name := shellPolicyAssignmentName(word)
		if name == "" {
			break
		}
		if shellPolicyExecutionEnvironmentName(name) {
			return true
		}
	}
	return false
}

func shellPolicyExecutionEnvironmentName(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	if name == "PATH" || name == "LIBPATH" || name == "SHLIB_PATH" ||
		strings.HasPrefix(name, "LD_") || strings.HasPrefix(name, "DYLD_") {
		return true
	}
	// Git exposes command-bearing configuration through both direct delegate
	// variables and the GIT_CONFIG_COUNT/KEY_n/VALUE_n family. Treat the whole
	// namespace as authority so a newly added delegate knob fails closed.
	if strings.HasPrefix(name, "GIT_") {
		return true
	}
	switch name {
	case "PAGER", "SSH_ASKPASS", "GIT_ASKPASS", "BASH_ENV", "ENV",
		"GOENV", "GOFLAGS", "GOTOOLCHAIN", "GOROOT", "GCCGO", "CC", "CXX",
		"RIPGREP_CONFIG_PATH":
		return true
	default:
		return false
	}
}

func unwrapShellPolicyCall(call *syntax.CallExpr, known map[string]shellPolicyValue, policy types.PolicyContext) (string, []*syntax.Word, types.PolicyDecision, types.PolicyContext) {
	words := call.Args
	decision := allowShellPolicyDecision()
	childPolicy := policy
	for len(words) > 0 {
		wrappedArgs := words[1:]
		first := shellPolicyWordValue(words[0], known, childPolicy)
		if shellPolicyWordHasUnquotedExpansion(words[0]) {
			suffixDecision := analyzeShellPolicySuffixes(words[1:], known, childPolicy)
			if strings.TrimSpace(first.literal) != "" {
				nested := AnalyzeShellCommand(first.literal, childPolicy)
				if nested.Disposition == types.PolicyBlock {
					return "", nil, nested, childPolicy
				}
			}
			return "", nil, strongerShellPolicyDecision(
				askShellPolicyDecision("shell.policy.ask.dynamic_command", i18n.KeyShellPolicyAskDynamicTarget, first.variable),
				suffixDecision,
			), childPolicy
		}
		if first.dynamic || first.commandSub {
			key := i18n.KeyShellPolicyAskDynamicTarget
			if first.commandSub {
				key = i18n.KeyShellPolicyAskCommandSubst
			}
			return "", nil, strongerShellPolicyDecision(
				askShellPolicyDecision("shell.policy.ask.dynamic_command", key, first.variable),
				analyzeShellPolicySuffixes(words[1:], known, childPolicy),
			), childPolicy
		}
		name := filepath.Base(first.literal)
		switch name {
		case "command":
			words = skipCommandWrapper(words[1:])
		case "builtin":
			words = skipBuiltinWrapper(words[1:])
		case "exec":
			words = skipExecWrapper(words[1:])
		case "env":
			var envDecision types.PolicyDecision
			var envComplete bool
			words, childPolicy, envDecision, envComplete = skipEnvWrapper(words[1:], known, childPolicy)
			decision = strongerShellPolicyDecision(decision, envDecision)
			if envComplete {
				return "", nil, decision, childPolicy
			}
		case "sudo":
			var sudoDecision types.PolicyDecision
			words, childPolicy, sudoDecision = skipSudoWrapper(words[1:], known, childPolicy)
			decision = strongerShellPolicyDecision(decision, sudoDecision)
		case "doas":
			var terminal bool
			var wrapperDecision types.PolicyDecision
			words, wrapperDecision, terminal = skipDoasWrapper(words[1:])
			decision = strongerShellPolicyDecision(decision, wrapperDecision)
			if terminal {
				return "", nil, decision, childPolicy
			}
		case "nohup":
			words = skipNohupWrapper(words[1:])
		case "time":
			var timeDecision types.PolicyDecision
			words, timeDecision = skipTimeWrapper(words[1:], known, childPolicy)
			decision = strongerShellPolicyDecision(decision, timeDecision)
		case "timeout":
			words = skipTimeoutWrapper(words[1:])
		case "nice":
			words = skipNiceWrapper(words[1:])
		case "stdbuf":
			words = skipStdbufWrapper(words[1:])
		case "ionice":
			var terminal bool
			var wrapperDecision types.PolicyDecision
			words, wrapperDecision, terminal = skipIoniceWrapper(words[1:])
			decision = strongerShellPolicyDecision(decision, wrapperDecision)
			if terminal {
				return "", nil, decision, childPolicy
			}
		case "unbuffer":
			words = skipUnbufferWrapper(words[1:])
		case "taskset":
			var terminal bool
			var wrapperDecision types.PolicyDecision
			words, wrapperDecision, terminal = skipTasksetWrapper(words[1:])
			decision = strongerShellPolicyDecision(decision, wrapperDecision)
			if terminal {
				return "", nil, decision, childPolicy
			}
		case "chrt":
			var terminal bool
			var wrapperDecision types.PolicyDecision
			words, wrapperDecision, terminal = skipChrtWrapper(words[1:])
			decision = strongerShellPolicyDecision(decision, wrapperDecision)
			if terminal {
				return "", nil, decision, childPolicy
			}
		default:
			if shellPolicyExecutablePathNeedsApproval(first.literal, name) {
				decision = strongerShellPolicyDecision(decision, unrestrictedCodePolicyDecision(name))
			}
			return name, words[1:], decision, childPolicy
		}
		if words == nil {
			// Unknown or dynamic wrapper flags make the child boundary
			// ambiguous. Analyze every literal suffix and take the worst result
			// so a protected/root/raw-device operation cannot be downgraded from
			// Block to an approvable parse warning.
			decision = strongerShellPolicyDecision(decision, analyzeShellPolicySuffixes(wrappedArgs, known, childPolicy))
		}
		if len(words) == 0 {
			return "", nil, strongerShellPolicyDecision(
				decision,
				askShellPolicyDecision("shell.policy.ask.dynamic_command", i18n.KeyShellPolicyAskParseFailure),
			), childPolicy
		}
	}
	return "", nil, decision, childPolicy
}

func shellPolicyExecutablePathNeedsApproval(literal, executable string) bool {
	rawLiteral := strings.TrimSpace(literal)
	if rawLiteral == "" || rawLiteral == executable {
		return false
	}
	literal = filepath.Clean(rawLiteral)
	if literal == filepath.Join(string(filepath.Separator), "bin", executable) ||
		literal == filepath.Join(string(filepath.Separator), "usr", "bin", executable) {
		return false
	}
	// A slash-bearing relative path or a binary outside immutable system
	// locations is custom executable authority even when its basename happens
	// to match a safe allowlisted command.
	return strings.ContainsAny(rawLiteral, `/\`)
}

func analyzeShellPolicySuffixes(words []*syntax.Word, known map[string]shellPolicyValue, policy types.PolicyContext) types.PolicyDecision {
	best := allowShellPolicyDecision()
	for index := range words {
		candidate := analyzeShellPolicyWords(words[index:], known, policy)
		best = strongerShellPolicyDecision(best, candidate)
		if best.Disposition == types.PolicyBlock {
			return best
		}
	}
	return best
}

func skipCommandWrapper(words []*syntax.Word) []*syntax.Word {
	for len(words) > 0 {
		value := wordToString(words[0])
		if value == "--" {
			return words[1:]
		}
		if !strings.HasPrefix(value, "-") || value == "-" {
			return words
		}
		if value != "-p" {
			return nil
		}
		words = words[1:]
	}
	return words
}

func skipBuiltinWrapper(words []*syntax.Word) []*syntax.Word {
	for len(words) > 0 {
		value := wordToString(words[0])
		if value == "--" {
			return words[1:]
		}
		if !strings.HasPrefix(value, "-") || value == "-" {
			return words
		}
		switch value {
		case "-a", "-d", "-n", "-p", "-s":
			words = words[1:]
		default:
			return nil
		}
	}
	return nil
}

func skipExecWrapper(words []*syntax.Word) []*syntax.Word {
	for len(words) > 0 {
		value := wordToString(words[0])
		if value == "--" {
			return words[1:]
		}
		if !strings.HasPrefix(value, "-") || value == "-" {
			return words
		}
		if value == "-a" {
			if len(words) < 2 {
				return nil
			}
			words = words[2:]
			continue
		}
		if value == "-c" || value == "-l" || value == "-cl" || value == "-lc" {
			words = words[1:]
			continue
		}
		return nil
	}
	return nil
}

func skipEnvWrapper(words []*syntax.Word, known map[string]shellPolicyValue, policy types.PolicyContext) ([]*syntax.Word, types.PolicyContext, types.PolicyDecision, bool) {
	decision := allowShellPolicyDecision()
	childPolicy := policy
	for len(words) > 0 {
		resolved := shellPolicyWordValue(words[0], known, childPolicy)
		value := resolved.literal
		if value == "" || resolved.dynamic || resolved.commandSub || resolved.possibleLiteral != "" {
			return nil, childPolicy, askShellPolicyDecision("shell.policy.ask.dynamic_flags", i18n.KeyShellPolicyAskDynamicFlags), false
		}
		if value == "--" {
			words = words[1:]
			break
		}
		switch value {
		case "-i", "--ignore-environment", "-0", "--null", "-v", "--debug":
			words = words[1:]
			continue
		case "-u", "--unset":
			if len(words) < 2 {
				return nil, childPolicy, askShellPolicyDecision("shell.policy.ask.parse_failure", i18n.KeyShellPolicyAskParseFailure), false
			}
			words = words[2:]
			continue
		case "-C", "--chdir":
			if len(words) < 2 {
				return nil, childPolicy, askShellPolicyDecision("shell.policy.ask.parse_failure", i18n.KeyShellPolicyAskParseFailure), false
			}
			var cwdDecision types.PolicyDecision
			childPolicy, cwdDecision = shellPolicyWithChildCWD(childPolicy, shellPolicyWordValue(words[1], known, childPolicy))
			decision = strongerShellPolicyDecision(decision, cwdDecision)
			words = words[2:]
			continue
		case "-S", "--split-string":
			if len(words) < 2 {
				return nil, childPolicy, askShellPolicyDecision("shell.policy.ask.parse_failure", i18n.KeyShellPolicyAskParseFailure), false
			}
			payload := shellPolicyWordValue(words[1], known, childPolicy)
			if payload.dynamic || payload.commandSub || payload.possibleLiteral != "" {
				return nil, childPolicy, askShellPolicyDecision("shell.policy.ask.dynamic_command", i18n.KeyShellPolicyAskDynamicTarget, payload.variable), false
			}
			decision = strongerShellPolicyDecision(decision, analyzeEnvSplitCommand(payload.literal, words[2:], known, childPolicy))
			return nil, childPolicy, decision, true
		}
		if strings.HasPrefix(value, "--split-string=") {
			decision = strongerShellPolicyDecision(decision, analyzeEnvSplitCommand(strings.TrimPrefix(value, "--split-string="), words[1:], known, childPolicy))
			return nil, childPolicy, decision, true
		}
		if strings.HasPrefix(value, "--chdir=") || strings.HasPrefix(value, "-C") && len(value) > 2 {
			prefix := "-C"
			if strings.HasPrefix(value, "--chdir=") {
				prefix = "--chdir="
			}
			var cwdDecision types.PolicyDecision
			childPolicy, cwdDecision = shellPolicyWithChildCWD(childPolicy, shellPolicyValueTrimPrefix(resolved, prefix))
			decision = strongerShellPolicyDecision(decision, cwdDecision)
			words = words[1:]
			continue
		}
		if strings.HasPrefix(value, "--unset=") {
			words = words[1:]
			continue
		}
		if strings.HasPrefix(value, "-") {
			return nil, childPolicy, askShellPolicyDecision("shell.policy.ask.dynamic_flags", i18n.KeyShellPolicyAskDynamicFlags), false
		}
		break
	}
	for len(words) > 0 {
		resolved := shellPolicyWordValue(words[0], known, childPolicy)
		value := resolved.literal
		if resolved.dynamic || resolved.commandSub || resolved.possibleLiteral != "" || shellPolicyWordHasUnquotedExpansion(words[0]) {
			return nil, childPolicy, strongerShellPolicyDecision(
				decision, unrestrictedCodePolicyDecision("env"),
			), false
		}
		if !strings.Contains(value, "=") || strings.HasPrefix(value, "=") {
			return words, childPolicy, decision, false
		}
		if assignmentName, _, _ := strings.Cut(value, "="); shellPolicyExecutionEnvironmentName(assignmentName) {
			decision = strongerShellPolicyDecision(decision, unrestrictedCodePolicyDecision("env"))
		}
		words = words[1:]
	}
	return words, childPolicy, decision, false
}

func shellPolicyValueTrimPrefix(value shellPolicyValue, prefix string) shellPolicyValue {
	value.literal = strings.TrimPrefix(value.literal, prefix)
	if value.possibleLiteral != "" {
		value.possibleLiteral = strings.TrimPrefix(value.possibleLiteral, prefix)
	}
	return value
}

func shellPolicyWithChildCWD(policy types.PolicyContext, value shellPolicyValue) (types.PolicyContext, types.PolicyDecision) {
	decision := allowShellPolicyDecision()
	target := value.literal
	if value.possibleLiteral != "" {
		target = value.possibleLiteral
		decision = askShellPolicyDecision("shell.policy.ask.dynamic_target", i18n.KeyShellPolicyAskDynamicTarget, value.variable)
	} else if value.dynamic || value.commandSub {
		return policy, askShellPolicyDecision("shell.policy.ask.dynamic_target", i18n.KeyShellPolicyAskDynamicTarget, value.variable)
	}
	if strings.TrimSpace(target) == "" {
		return policy, askShellPolicyDecision("shell.policy.ask.dynamic_target", i18n.KeyShellPolicyAskDynamicTarget, value.variable)
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(policy.CWD, target)
	}
	resolved, err := resolveExistingPolicyPath(filepath.Clean(target))
	if err != nil {
		return policy, strongerShellPolicyDecision(decision, askShellPolicyDecision(
			"shell.policy.ask.unproven_target", i18n.KeyShellPolicyAskUnprovenTarget, target,
		))
	}
	policy.CWD = resolved
	return policy, decision
}

func analyzeEnvSplitCommand(payload string, trailing []*syntax.Word, known map[string]shellPolicyValue, policy types.PolicyContext) types.PolicyDecision {
	var command strings.Builder
	command.WriteString(payload)
	for _, word := range trailing {
		value := shellPolicyWordValue(word, known, policy)
		if value.dynamic || value.commandSub || value.possibleLiteral != "" {
			return askShellPolicyDecision("shell.policy.ask.dynamic_command", i18n.KeyShellPolicyAskDynamicTarget, value.variable)
		}
		command.WriteByte(' ')
		command.WriteString(shellPolicyQuoteLiteral(value.literal))
	}
	return AnalyzeShellCommand(command.String(), policy)
}

func skipSudoWrapper(words []*syntax.Word, known map[string]shellPolicyValue, policy types.PolicyContext) ([]*syntax.Word, types.PolicyContext, types.PolicyDecision) {
	decision := allowShellPolicyDecision()
	childPolicy := policy
	valueFlags := map[string]bool{
		"-u": true, "--user": true, "-g": true, "--group": true,
		"-h": true, "--host": true, "-p": true, "--prompt": true,
		"-r": true, "--role": true, "-t": true, "--type": true,
		"-C": true, "--close-from": true,
	}
	for len(words) > 0 {
		resolved := shellPolicyWordValue(words[0], known, childPolicy)
		value := resolved.literal
		if value == "" || resolved.dynamic || resolved.commandSub || resolved.possibleLiteral != "" {
			return nil, childPolicy, strongerShellPolicyDecision(decision, askShellPolicyDecision("shell.policy.ask.dynamic_flags", i18n.KeyShellPolicyAskDynamicFlags))
		}
		if value == "--" {
			return words[1:], childPolicy, decision
		}
		if !strings.HasPrefix(value, "-") || value == "-" {
			if shellPolicyAssignmentName(words[0]) != "" {
				words = words[1:]
				continue
			}
			return words, childPolicy, decision
		}
		if value == "-D" || value == "--chdir" {
			if len(words) < 2 {
				return nil, childPolicy, strongerShellPolicyDecision(decision, askShellPolicyDecision("shell.policy.ask.parse_failure", i18n.KeyShellPolicyAskParseFailure))
			}
			var cwdDecision types.PolicyDecision
			childPolicy, cwdDecision = shellPolicyWithChildCWD(childPolicy, shellPolicyWordValue(words[1], known, childPolicy))
			decision = strongerShellPolicyDecision(decision, cwdDecision)
			words = words[2:]
			continue
		}
		if strings.HasPrefix(value, "--chdir=") || strings.HasPrefix(value, "-D") && len(value) > 2 {
			prefix := "-D"
			if strings.HasPrefix(value, "--chdir=") {
				prefix = "--chdir="
			}
			var cwdDecision types.PolicyDecision
			childPolicy, cwdDecision = shellPolicyWithChildCWD(childPolicy, shellPolicyValueTrimPrefix(resolved, prefix))
			decision = strongerShellPolicyDecision(decision, cwdDecision)
			words = words[1:]
			continue
		}
		if valueFlags[value] {
			if len(words) < 2 {
				return nil, childPolicy, strongerShellPolicyDecision(decision, askShellPolicyDecision("shell.policy.ask.parse_failure", i18n.KeyShellPolicyAskParseFailure))
			}
			words = words[2:]
			continue
		}
		if strings.Contains(value, "=") || value == "-n" || value == "--non-interactive" || value == "-E" || value == "--preserve-env" || value == "-H" || value == "--set-home" {
			words = words[1:]
			continue
		}
		return nil, childPolicy, strongerShellPolicyDecision(decision, askShellPolicyDecision("shell.policy.ask.dynamic_flags", i18n.KeyShellPolicyAskDynamicFlags))
	}
	return nil, childPolicy, decision
}

func skipDoasWrapper(words []*syntax.Word) ([]*syntax.Word, types.PolicyDecision, bool) {
	for len(words) > 0 {
		value := wordToString(words[0])
		if value == "" {
			return nil, askShellPolicyDecision("shell.policy.ask.dynamic_flags", i18n.KeyShellPolicyAskDynamicFlags), false
		}
		if value == "--" {
			return words[1:], allowShellPolicyDecision(), false
		}
		switch value {
		case "-L":
			return nil, allowShellPolicyDecision(), true
		case "-s":
			return nil, askShellPolicyDecision("shell.policy.ask.dynamic_command", i18n.KeyShellPolicyAskDynamicTarget, "doas shell"), true
		case "-n":
			words = words[1:]
			continue
		case "-a", "-C", "-u":
			if len(words) < 2 {
				return nil, askShellPolicyDecision("shell.policy.ask.parse_failure", i18n.KeyShellPolicyAskParseFailure), false
			}
			terminal := value == "-C"
			words = words[2:]
			if terminal {
				return nil, allowShellPolicyDecision(), true
			}
			continue
		}
		if strings.HasPrefix(value, "-a") || strings.HasPrefix(value, "-u") {
			words = words[1:]
			continue
		}
		if strings.HasPrefix(value, "-") {
			return nil, askShellPolicyDecision("shell.policy.ask.dynamic_flags", i18n.KeyShellPolicyAskDynamicFlags), false
		}
		return words, allowShellPolicyDecision(), false
	}
	return nil, askShellPolicyDecision("shell.policy.ask.parse_failure", i18n.KeyShellPolicyAskParseFailure), false
}

func skipIoniceWrapper(words []*syntax.Word) ([]*syntax.Word, types.PolicyDecision, bool) {
	processMode := false
	for len(words) > 0 {
		value := wordToString(words[0])
		if value == "" {
			return nil, askShellPolicyDecision("shell.policy.ask.dynamic_flags", i18n.KeyShellPolicyAskDynamicFlags), false
		}
		if value == "--" {
			words = words[1:]
			break
		}
		switch value {
		case "-h", "--help", "-V", "--version":
			return nil, allowShellPolicyDecision(), true
		case "-t", "--ignore":
			words = words[1:]
			continue
		case "-c", "--class", "-n", "--classdata", "-p", "--pid", "-P", "--pgid", "-u", "--uid":
			if len(words) < 2 {
				return nil, askShellPolicyDecision("shell.policy.ask.parse_failure", i18n.KeyShellPolicyAskParseFailure), false
			}
			processMode = processMode || value == "-p" || value == "--pid" || value == "-P" || value == "--pgid" || value == "-u" || value == "--uid"
			words = words[2:]
			continue
		}
		if strings.HasPrefix(value, "--class=") || strings.HasPrefix(value, "--classdata=") {
			words = words[1:]
			continue
		}
		if strings.HasPrefix(value, "--pid=") || strings.HasPrefix(value, "--pgid=") || strings.HasPrefix(value, "--uid=") {
			processMode = true
			words = words[1:]
			continue
		}
		if (strings.HasPrefix(value, "-c") || strings.HasPrefix(value, "-n")) && len(value) > 2 {
			words = words[1:]
			continue
		}
		if strings.HasPrefix(value, "-") {
			return nil, askShellPolicyDecision("shell.policy.ask.dynamic_flags", i18n.KeyShellPolicyAskDynamicFlags), false
		}
		break
	}
	if processMode {
		return nil, allowShellPolicyDecision(), true
	}
	if len(words) == 0 {
		return nil, askShellPolicyDecision("shell.policy.ask.parse_failure", i18n.KeyShellPolicyAskParseFailure), false
	}
	return words, allowShellPolicyDecision(), false
}

func skipUnbufferWrapper(words []*syntax.Word) []*syntax.Word {
	if len(words) == 0 {
		return nil
	}
	value := wordToString(words[0])
	if value == "" {
		return nil
	}
	if value == "-p" {
		if len(words) < 2 {
			return nil
		}
		return words[1:]
	}
	// unbuffer only recognizes -p; notably, -- is the child executable rather
	// than a conventional option terminator.
	return words
}

func skipTasksetWrapper(words []*syntax.Word) ([]*syntax.Word, types.PolicyDecision, bool) {
	processMode := false
	for len(words) > 0 {
		value := wordToString(words[0])
		if value == "" {
			return nil, askShellPolicyDecision("shell.policy.ask.dynamic_flags", i18n.KeyShellPolicyAskDynamicFlags), false
		}
		if value == "--" {
			words = words[1:]
			break
		}
		switch value {
		case "-h", "--help", "-V", "--version":
			return nil, allowShellPolicyDecision(), true
		case "-a", "--all-tasks", "-c", "--cpu-list":
			words = words[1:]
			continue
		case "-p", "--pid":
			processMode = true
			words = words[1:]
			continue
		}
		if strings.HasPrefix(value, "-") && len(value) > 1 {
			cluster := strings.TrimPrefix(value, "-")
			valid := true
			for _, flag := range cluster {
				if flag != 'a' && flag != 'c' && flag != 'p' {
					valid = false
					break
				}
				processMode = processMode || flag == 'p'
			}
			if !valid {
				return nil, askShellPolicyDecision("shell.policy.ask.dynamic_flags", i18n.KeyShellPolicyAskDynamicFlags), false
			}
			words = words[1:]
			continue
		}
		break
	}
	if processMode {
		return nil, allowShellPolicyDecision(), true
	}
	if len(words) < 2 { // mask/cpu-list followed by command
		return nil, askShellPolicyDecision("shell.policy.ask.parse_failure", i18n.KeyShellPolicyAskParseFailure), false
	}
	return words[1:], allowShellPolicyDecision(), false
}

func skipChrtWrapper(words []*syntax.Word) ([]*syntax.Word, types.PolicyDecision, bool) {
	noPriority := false
	for len(words) > 0 {
		value := wordToString(words[0])
		if value == "" {
			return nil, askShellPolicyDecision("shell.policy.ask.dynamic_flags", i18n.KeyShellPolicyAskDynamicFlags), false
		}
		if value == "--" {
			words = words[1:]
			break
		}
		switch value {
		case "-h", "--help", "-V", "--version", "-m", "--max":
			return nil, allowShellPolicyDecision(), true
		case "-p", "--pid":
			return nil, allowShellPolicyDecision(), true
		case "-o", "--other", "-b", "--batch", "-i", "--idle", "-d", "--deadline", "-e", "--ext":
			noPriority = true
			words = words[1:]
			continue
		case "-a", "--all-tasks", "-f", "--fifo", "-r", "--rr", "-R", "--reset-on-fork", "-v", "--verbose":
			words = words[1:]
			continue
		case "-D", "--sched-deadline", "-P", "--sched-period", "-T", "--sched-runtime", "-U", "--sched-util-min", "-X", "--sched-util-max":
			if len(words) < 2 {
				return nil, askShellPolicyDecision("shell.policy.ask.parse_failure", i18n.KeyShellPolicyAskParseFailure), false
			}
			words = words[2:]
			continue
		}
		if strings.HasPrefix(value, "--sched-") && strings.Contains(value, "=") || strings.HasPrefix(value, "-D") && len(value) > 2 || strings.HasPrefix(value, "-P") && len(value) > 2 || strings.HasPrefix(value, "-T") && len(value) > 2 {
			words = words[1:]
			continue
		}
		if strings.HasPrefix(value, "-") {
			return nil, askShellPolicyDecision("shell.policy.ask.dynamic_flags", i18n.KeyShellPolicyAskDynamicFlags), false
		}
		break
	}
	if len(words) == 0 {
		return nil, askShellPolicyDecision("shell.policy.ask.parse_failure", i18n.KeyShellPolicyAskParseFailure), false
	}
	if noPriority {
		if len(words) >= 2 && isShellPolicyNumericWord(words[0]) {
			return words[1:], allowShellPolicyDecision(), false
		}
		return words, allowShellPolicyDecision(), false
	}
	if len(words) < 2 {
		return nil, askShellPolicyDecision("shell.policy.ask.parse_failure", i18n.KeyShellPolicyAskParseFailure), false
	}
	return words[1:], allowShellPolicyDecision(), false
}

func isShellPolicyNumericWord(word *syntax.Word) bool {
	value := wordToString(word)
	if value == "" {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func skipTimeoutWrapper(words []*syntax.Word) []*syntax.Word {
	for len(words) > 0 {
		value := wordToString(words[0])
		if value == "" {
			return nil
		}
		if value == "--" {
			words = words[1:]
			break
		}
		if value == "-s" || value == "--signal" || value == "-k" || value == "--kill-after" {
			if len(words) < 2 {
				return nil
			}
			words = words[2:]
			continue
		}
		if strings.HasPrefix(value, "--signal=") || strings.HasPrefix(value, "--kill-after=") || value == "--foreground" || value == "--preserve-status" || value == "--verbose" {
			words = words[1:]
			continue
		}
		if strings.HasPrefix(value, "-") {
			return nil
		}
		break
	}
	if len(words) < 2 { // duration followed by command
		return nil
	}
	return words[1:]
}

func skipTimeWrapper(words []*syntax.Word, known map[string]shellPolicyValue, policy types.PolicyContext) ([]*syntax.Word, types.PolicyDecision) {
	decision := allowShellPolicyDecision()
	for len(words) > 0 {
		resolved := shellPolicyWordValue(words[0], known, policy)
		value := resolved.literal
		if value == "" || resolved.dynamic || resolved.commandSub || resolved.possibleLiteral != "" {
			return nil, strongerShellPolicyDecision(decision, askShellPolicyDecision("shell.policy.ask.dynamic_flags", i18n.KeyShellPolicyAskDynamicFlags))
		}
		if value == "--" {
			return words[1:], decision
		}
		if !strings.HasPrefix(value, "-") || value == "-" {
			return words, decision
		}
		if value == "-o" || value == "--output" {
			if len(words) < 2 {
				return nil, strongerShellPolicyDecision(decision, askShellPolicyDecision("shell.policy.ask.parse_failure", i18n.KeyShellPolicyAskParseFailure))
			}
			decision = strongerShellPolicyDecision(decision, analyzeWrapperOutputTarget(words[1], known, policy))
			words = words[2:]
			continue
		}
		if value == "-f" || value == "--format" {
			if len(words) < 2 {
				return nil, strongerShellPolicyDecision(decision, askShellPolicyDecision("shell.policy.ask.parse_failure", i18n.KeyShellPolicyAskParseFailure))
			}
			words = words[2:]
			continue
		}
		if strings.HasPrefix(value, "--output=") || strings.HasPrefix(value, "-o") && len(value) > 2 {
			prefix := "-o"
			if strings.HasPrefix(value, "--output=") {
				prefix = "--output="
			}
			decision = strongerShellPolicyDecision(decision, analyzeWrapperOutputValue(shellPolicyValueTrimPrefix(resolved, prefix), policy))
			words = words[1:]
			continue
		}
		if strings.HasPrefix(value, "--format=") || strings.HasPrefix(value, "-f") && len(value) > 2 || value == "-a" || value == "--append" || value == "-p" || value == "--portability" || value == "-v" || value == "--verbose" {
			words = words[1:]
			continue
		}
		return nil, strongerShellPolicyDecision(decision, askShellPolicyDecision("shell.policy.ask.dynamic_flags", i18n.KeyShellPolicyAskDynamicFlags))
	}
	return nil, decision
}

func analyzeWrapperOutputTarget(word *syntax.Word, known map[string]shellPolicyValue, policy types.PolicyContext) types.PolicyDecision {
	return analyzeWrapperOutputValue(shellPolicyWordValue(word, known, policy), policy)
}

func analyzeWrapperOutputValue(value shellPolicyValue, policy types.PolicyContext) types.PolicyDecision {
	best := allowShellPolicyDecision()
	if value.possibleLiteral != "" {
		best = strongerShellPolicyDecision(best, classifyDestructivePath(value.possibleLiteral, policy))
	}
	if value.dynamic || value.commandSub {
		return strongerShellPolicyDecision(best, askShellPolicyDecision("shell.policy.ask.dynamic_target", i18n.KeyShellPolicyAskDynamicTarget, value.variable))
	}
	return strongerShellPolicyDecision(best, classifyDestructivePath(value.literal, policy))
}

func skipNohupWrapper(words []*syntax.Word) []*syntax.Word {
	if len(words) > 0 && wordToString(words[0]) == "--" {
		return words[1:]
	}
	return words
}

func skipNiceWrapper(words []*syntax.Word) []*syntax.Word {
	for len(words) > 0 {
		value := wordToString(words[0])
		if value == "-n" || value == "--adjustment" {
			if len(words) < 2 {
				return nil
			}
			words = words[2:]
			continue
		}
		if strings.HasPrefix(value, "--adjustment=") || (strings.HasPrefix(value, "-") && len(value) > 1) {
			words = words[1:]
			continue
		}
		return words
	}
	return nil
}

func skipStdbufWrapper(words []*syntax.Word) []*syntax.Word {
	for len(words) > 0 {
		value := wordToString(words[0])
		if value == "--" {
			return words[1:]
		}
		if value == "-i" || value == "-o" || value == "-e" || value == "--input" || value == "--output" || value == "--error" {
			if len(words) < 2 {
				return nil
			}
			words = words[2:]
			continue
		}
		if strings.HasPrefix(value, "--input=") || strings.HasPrefix(value, "--output=") || strings.HasPrefix(value, "--error=") ||
			strings.HasPrefix(value, "-i") || strings.HasPrefix(value, "-o") || strings.HasPrefix(value, "-e") {
			words = words[1:]
			continue
		}
		return words
	}
	return nil
}

func analyzeRMPolicy(args []*syntax.Word, known map[string]shellPolicyValue, policy types.PolicyContext) types.PolicyDecision {
	recursive, force, endFlags := false, false, false
	targets := make([]*syntax.Word, 0, len(args))
	best := allowShellPolicyDecision()
	for _, word := range args {
		value := shellPolicyWordValue(word, known, policy)
		if shellPolicyWordHasUnquotedExpansion(word) {
			best = strongerShellPolicyDecision(best, askShellPolicyDecision("shell.policy.ask.dynamic_flags", i18n.KeyShellPolicyAskDynamicFlags))
			for _, expanded := range strings.Fields(value.literal) {
				if strings.HasPrefix(expanded, "-") {
					continue
				}
				if candidate := classifyDestructivePath(expanded, policy); candidate.Disposition == types.PolicyBlock {
					return candidate
				}
			}
			continue
		}
		if !endFlags && (value.dynamic || value.commandSub) {
			best = strongerShellPolicyDecision(best, askShellPolicyDecision("shell.policy.ask.dynamic_flags", i18n.KeyShellPolicyAskDynamicFlags))
			targets = append(targets, word)
			continue
		}
		literal := value.literal
		if !endFlags && literal == "--" {
			endFlags = true
			continue
		}
		if !endFlags && strings.HasPrefix(literal, "--") {
			switch strings.SplitN(literal, "=", 2)[0] {
			case "--recursive":
				recursive = true
			case "--force":
				force = true
			}
			continue
		}
		if !endFlags && strings.HasPrefix(literal, "-") && literal != "-" {
			if strings.ContainsAny(literal[1:], "rR") {
				recursive = true
			}
			if strings.ContainsRune(literal[1:], 'f') {
				force = true
			}
			continue
		}
		targets = append(targets, word)
	}
	trustedCleanup := len(targets) > 0
	for _, target := range targets {
		trustedCleanup = trustedCleanup && shellPolicyWordValue(target, known, policy).trustedTemp
		best = strongerShellPolicyDecision(best, analyzeDestructiveTarget(target, known, policy))
	}
	if best.Disposition != types.PolicyAllow {
		return best
	}
	if recursive && !trustedCleanup {
		return askShellPolicyDecision(
			"shell.policy.ask.destructive", i18n.KeyShellPolicyAskDestructive,
			toolRuntimeText(i18n.KeyToolRuntimeDestructiveRmRecursive),
		)
	}
	_ = force // force changes shell behavior but target class owns hard policy.
	return best
}

func analyzeNestedShellPolicy(args []*syntax.Word, known map[string]shellPolicyValue, policy types.PolicyContext) types.PolicyDecision {
	for i, word := range args {
		flag := wordToString(word)
		commandFlag := flag == "--command" || strings.HasPrefix(flag, "-") && !strings.HasPrefix(flag, "--") && strings.ContainsRune(strings.TrimPrefix(flag, "-"), 'c')
		if !commandFlag {
			continue
		}
		if i+1 >= len(args) {
			return askShellPolicyDecision("shell.policy.ask.dynamic_command", i18n.KeyShellPolicyAskParseFailure)
		}
		value := shellPolicyWordValue(args[i+1], known, policy)
		if value.dynamic || value.commandSub {
			return askShellPolicyDecision("shell.policy.ask.dynamic_command", i18n.KeyShellPolicyAskDynamicTarget, value.variable)
		}
		return AnalyzeShellCommand(value.literal, policy)
	}
	return askShellPolicyDecision("shell.policy.ask.dynamic_command", i18n.KeyShellPolicyAskDynamicTarget, "script")
}

func analyzeAppletPolicy(args []*syntax.Word, known map[string]shellPolicyValue, policy types.PolicyContext) types.PolicyDecision {
	if len(args) == 0 {
		return askShellPolicyDecision("shell.policy.ask.dynamic_command", i18n.KeyShellPolicyAskDynamicTarget, "applet")
	}
	applet := shellPolicyWordValue(args[0], known, policy)
	if applet.dynamic || applet.commandSub || applet.possibleLiteral != "" || applet.literal == "" {
		return askShellPolicyDecision("shell.policy.ask.dynamic_command", i18n.KeyShellPolicyAskDynamicTarget, applet.variable)
	}
	switch filepath.Base(applet.literal) {
	case "bash", "sh", "zsh", "dash", "ash", "ksh":
		return analyzeNestedShellPolicy(args[1:], known, policy)
	default:
		return analyzeShellPolicyWords(args, known, policy)
	}
}

func analyzeDDPolicy(args []*syntax.Word, known map[string]shellPolicyValue, policy types.PolicyContext) types.PolicyDecision {
	for _, word := range args {
		value := shellPolicyWordValue(word, known, policy)
		if strings.HasPrefix(value.possibleLiteral, "of=") {
			target := strings.TrimPrefix(value.possibleLiteral, "of=")
			if decision := classifyDestructivePathWithGlob(target, policy, shellPolicyWordHasActiveUnquotedGlob(word, value)); decision.Disposition == types.PolicyBlock {
				return decision
			}
		}
		if value.dynamic || value.commandSub {
			if shellPolicyWordContainsLiteral(word, "of=") {
				return askShellPolicyDecision("shell.policy.ask.dynamic_target", i18n.KeyShellPolicyAskDynamicTarget, value.variable)
			}
			continue
		}
		if strings.HasPrefix(value.literal, "of=") {
			target := strings.TrimPrefix(value.literal, "of=")
			if isRawDevicePath(target) {
				return blockShellPolicyDecision("shell.policy.block.raw_device", i18n.KeyShellPolicyBlockRawDevice, target)
			}
			if decision := classifyDestructivePathWithGlob(target, policy, shellPolicyWordHasActiveUnquotedGlob(word, value)); decision.Disposition == types.PolicyBlock {
				return decision
			}
		}
	}
	return allowShellPolicyDecision()
}

// analyzeProtectedGlobOperands applies to both read and write commands. A
// shell expands unquoted path globs before the child process sees its argv, so
// command-specific write-target parsing alone cannot protect a matching input
// operand. Quoted patterns remain literal and are deliberately excluded.
func analyzeProtectedGlobOperands(args []*syntax.Word, known map[string]shellPolicyValue, policy types.PolicyContext) types.PolicyDecision {
	for _, word := range args {
		value := shellPolicyWordValue(word, known, policy)
		if value.possibleLiteral != "" && shellPolicyWordHasActiveUnquotedGlob(word, value) && shellPolicyGlobMayMatchProtectedPath(value.possibleLiteral) {
			return blockShellPolicyDecision("shell.policy.block.protected", i18n.KeyShellPolicyBlockProtected, value.possibleLiteral)
		}
		if !shellPolicyWordHasActiveUnquotedGlob(word, value) || !shellPolicyGlobMayMatchProtectedPath(value.literal) {
			continue
		}
		return blockShellPolicyDecision("shell.policy.block.protected", i18n.KeyShellPolicyBlockProtected, value.literal)
	}
	return allowShellPolicyDecision()
}

func analyzeShellRedirectPolicy(statement *syntax.Stmt, known map[string]shellPolicyValue, policy types.PolicyContext) types.PolicyDecision {
	if statement == nil {
		return allowShellPolicyDecision()
	}
	best := allowShellPolicyDecision()
	for _, redirect := range statement.Redirs {
		writesPath := shellPolicyRedirectWritesPath(redirect.Op)
		readsPath := redirect.Op == syntax.RdrIn
		if !writesPath && !readsPath {
			continue
		}
		value := shellPolicyWordValue(redirect.Word, known, policy)
		if value.possibleLiteral != "" {
			candidate := classifyDestructivePathWithGlob(value.possibleLiteral, policy, shellPolicyWordHasActiveUnquotedGlob(redirect.Word, value))
			if candidate.Disposition == types.PolicyBlock {
				return candidate
			}
		}
		if value.dynamic || value.commandSub {
			best = strongerShellPolicyDecision(best, askShellPolicyDecision(
				"shell.policy.ask.dynamic_target", i18n.KeyShellPolicyAskDynamicTarget, value.variable,
			))
			continue
		}
		if readsPath {
			if shellPolicyWordHasActiveUnquotedGlob(redirect.Word, value) && shellPolicyGlobMayMatchProtectedPath(value.literal) {
				return blockShellPolicyDecision("shell.policy.block.protected", i18n.KeyShellPolicyBlockProtected, value.literal)
			}
			continue
		}
		if len(FilterBashPathScopeExemptions([]string{value.literal})) == 0 {
			continue
		}
		best = strongerShellPolicyDecision(best, classifyDestructivePathWithGlob(value.literal, policy, shellPolicyWordHasActiveUnquotedGlob(redirect.Word, value)))
	}
	return best
}

func shellPolicyRedirectWritesPath(operator syntax.RedirOperator) bool {
	switch operator {
	case syntax.RdrOut, syntax.AppOut, syntax.RdrInOut, syntax.RdrClob, syntax.AppClob,
		syntax.RdrAll, syntax.RdrAllClob, syntax.AppAll, syntax.AppAllClob:
		return true
	default:
		return false
	}
}

func analyzeDestructiveTarget(word *syntax.Word, known map[string]shellPolicyValue, policy types.PolicyContext) types.PolicyDecision {
	value := shellPolicyWordValue(word, known, policy)
	hasGlob := shellPolicyWordHasActiveUnquotedGlob(word, value)
	if value.possibleLiteral != "" {
		if possible := classifyDestructivePathWithGlob(value.possibleLiteral, policy, hasGlob); possible.Disposition == types.PolicyBlock {
			return possible
		}
	}
	if value.commandSub {
		return askShellPolicyDecision("shell.policy.ask.command_substitution", i18n.KeyShellPolicyAskCommandSubst)
	}
	if value.dynamic {
		name := value.variable
		if name == "" {
			name = "?"
		}
		return askShellPolicyDecision("shell.policy.ask.dynamic_target", i18n.KeyShellPolicyAskDynamicTarget, name)
	}
	if value.trustedTemp {
		if !shellPolicyWordIsExactQuotedParam(word) {
			return askShellPolicyDecision("shell.policy.ask.dynamic_target", i18n.KeyShellPolicyAskDynamicTarget, value.variable)
		}
		if value.trustedRoot != "" {
			for _, trustedRoot := range policy.TrustedTempRoots {
				resolvedValueRoot, err := resolveExistingPolicyPath(value.trustedRoot)
				if err == nil && validTrustedTempRoot(trustedRoot) && policyPathWithin(resolvedValueRoot, trustedRoot) {
					return allowShellPolicyDecision()
				}
			}
			if rootDecision := classifyDestructivePath(value.trustedRoot, policy); rootDecision.Disposition == types.PolicyBlock {
				return rootDecision
			}
		}
		return allowShellPolicyDecision()
	}
	return classifyDestructivePathWithGlob(value.literal, policy, hasGlob)
}

func shellPolicyWordIsExactQuotedParam(word *syntax.Word) bool {
	if word == nil || len(word.Parts) != 1 {
		return false
	}
	quoted, ok := word.Parts[0].(*syntax.DblQuoted)
	if !ok || len(quoted.Parts) != 1 {
		return false
	}
	_, ok = quoted.Parts[0].(*syntax.ParamExp)
	return ok
}

func shellPolicyWordHasUnquotedExpansion(word *syntax.Word) bool {
	if word == nil {
		return false
	}
	for _, part := range word.Parts {
		switch part.(type) {
		case *syntax.ParamExp, *syntax.CmdSubst:
			return true
		}
	}
	return false
}

func shellPolicyWordHasUnquotedGlob(word *syntax.Word) bool {
	if word == nil {
		return false
	}
	for _, part := range word.Parts {
		if literal, ok := part.(*syntax.Lit); ok && strings.ContainsAny(literal.Value, "*?[") {
			return true
		}
	}
	return false
}

func shellPolicyWordHasActiveUnquotedGlob(word *syntax.Word, value shellPolicyValue) bool {
	if shellPolicyWordHasUnquotedGlob(word) {
		return true
	}
	// Unquoted parameter expansion is followed by pathname expansion. Preserve
	// that fact when a deterministic variable value contains glob metacharacters;
	// the same value inside double quotes is not considered active.
	return shellPolicyWordHasUnquotedExpansion(word) && strings.ContainsAny(value.literal+value.possibleLiteral, "*?[")
}

// shellPolicyGlobMayMatchProtectedPath conservatively checks whether an
// unquoted pathname pattern can resolve to the canonical protected-path set.
// Directory entries may occur at any depth, basename entries apply only to the
// final component, and exact multi-component entries are suffix matched. This
// mirrors permissions.IsProtectedPath without relying on the current contents
// of the filesystem (which are mutable between policy and execution).
func shellPolicyGlobMayMatchProtectedPath(pattern string) bool {
	components := shellPolicyPathPatternComponents(pattern)
	if len(components) == 0 {
		return false
	}
	for _, protected := range permissions.GetProtectedPaths() {
		protected = strings.TrimPrefix(filepath.ToSlash(protected), "/")
		switch {
		case strings.HasSuffix(protected, "/"):
			name := strings.TrimSuffix(protected, "/")
			for _, component := range components {
				if shellPolicyGlobComponentMayMatch(component, name) {
					return true
				}
			}
		case !strings.Contains(protected, "/"):
			if shellPolicyGlobComponentMayMatch(components[len(components)-1], protected) {
				return true
			}
		default:
			protectedComponents := strings.Split(protected, "/")
			if len(components) < len(protectedComponents) {
				continue
			}
			start := len(components) - len(protectedComponents)
			matched := true
			for index := range protectedComponents {
				if !shellPolicyGlobComponentMayMatch(components[start+index], protectedComponents[index]) {
					matched = false
					break
				}
			}
			if matched {
				return true
			}
		}
	}
	return false
}

func shellPolicyPathPatternComponents(pattern string) []string {
	cleaned := filepath.ToSlash(filepath.Clean(strings.TrimSpace(pattern)))
	parts := strings.Split(cleaned, "/")
	components := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" && part != "." && part != ".." {
			components = append(components, part)
		}
	}
	return components
}

func shellPolicyGlobComponentMayMatch(pattern, protected string) bool {
	if pattern == protected {
		return true
	}
	if strings.HasPrefix(protected, ".") && !shellPolicyGlobExplicitlyTargetsHidden(pattern) {
		// With the default shell pathname-expansion rules, '*' and '?' do not
		// implicitly select a leading dot. This keeps ordinary globs such as
		// src/*.go out of the hard-block path.
		return false
	}
	if matched, err := path.Match(pattern, protected); err == nil && matched {
		return true
	}
	// Conservatively treat the raw payload of a bracket expression as another
	// literal alternative. Besides normal character-class matching above, this
	// closes obfuscations such as .g[it] against the protected component .git.
	if flattened, ok := shellPolicyFlattenBracketPayloads(pattern); ok && flattened == protected {
		return true
	}
	return false
}

func shellPolicyGlobExplicitlyTargetsHidden(pattern string) bool {
	if strings.HasPrefix(pattern, ".") {
		return true
	}
	if !strings.HasPrefix(pattern, "[") {
		return false
	}
	end := strings.IndexByte(pattern[1:], ']')
	if end < 0 {
		return false
	}
	class := pattern[1 : end+1]
	matched, err := path.Match("["+class+"]", ".")
	return err == nil && matched
}

func shellPolicyFlattenBracketPayloads(pattern string) (string, bool) {
	var flattened strings.Builder
	found := false
	for index := 0; index < len(pattern); index++ {
		if pattern[index] != '[' {
			flattened.WriteByte(pattern[index])
			continue
		}
		endOffset := strings.IndexByte(pattern[index+1:], ']')
		if endOffset < 0 {
			return "", false
		}
		end := index + 1 + endOffset
		payload := pattern[index+1 : end]
		if payload == "" || strings.ContainsAny(payload, "!^-") {
			return "", false
		}
		flattened.WriteString(payload)
		found = true
		index = end
	}
	return flattened.String(), found
}

func classifyDestructivePath(target string, policy types.PolicyContext) types.PolicyDecision {
	return classifyDestructivePathWithGlob(target, policy, false)
}

func classifyDestructivePathWithGlob(target string, policy types.PolicyContext, unquotedGlob bool) types.PolicyDecision {
	target = strings.TrimSpace(target)
	if target == "" {
		return askShellPolicyDecision("shell.policy.ask.dynamic_target", i18n.KeyShellPolicyAskDynamicTarget, "?")
	}
	if target == "~" {
		return blockShellPolicyDecision("shell.policy.block.home", i18n.KeyShellPolicyBlockHome)
	}
	if strings.HasPrefix(target, "~/") && policy.HomeDir != "" {
		target = filepath.Join(policy.HomeDir, strings.TrimPrefix(target, "~/"))
	}
	if unquotedGlob && shellPolicyGlobMayMatchProtectedPath(target) {
		return blockShellPolicyDecision("shell.policy.block.protected", i18n.KeyShellPolicyBlockProtected, target)
	}
	resolved := target
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(policy.CWD, resolved)
	}
	resolved = filepath.Clean(resolved)
	if unquotedGlob && (resolved == string(filepath.Separator)+"*" || filepath.Dir(resolved) == string(filepath.Separator) && strings.ContainsAny(filepath.Base(resolved), "*?[")) {
		return blockShellPolicyDecision("shell.policy.block.root", i18n.KeyShellPolicyBlockRoot)
	}
	physical, err := resolveExistingPolicyPath(resolved)
	if err != nil {
		return askShellPolicyDecision("shell.policy.ask.unproven_target", i18n.KeyShellPolicyAskUnprovenTarget, target)
	}
	resolved = physical
	root := string(filepath.Separator)
	if resolved == root {
		return blockShellPolicyDecision("shell.policy.block.root", i18n.KeyShellPolicyBlockRoot)
	}
	resolvedHome, _ := resolveExistingPolicyPath(policy.HomeDir)
	if resolvedHome != "" && resolved == resolvedHome {
		return blockShellPolicyDecision("shell.policy.block.home", i18n.KeyShellPolicyBlockHome)
	}
	if isRawDevicePath(resolved) {
		return blockShellPolicyDecision("shell.policy.block.raw_device", i18n.KeyShellPolicyBlockRawDevice, resolved)
	}
	if permissions.IsProtectedPath(resolved) || permissions.IsProtectedPath(target) {
		return blockShellPolicyDecision("shell.policy.block.protected", i18n.KeyShellPolicyBlockProtected, target)
	}
	// Platform temporary roots can live below nominal system prefixes (for
	// example /var/folders on macOS). The registered root is narrow authority;
	// it does not make the whole system prefix writable.
	for _, trustedRoot := range policy.TrustedTempRoots {
		if validTrustedTempRoot(trustedRoot) && policyPathWithin(resolved, trustedRoot) {
			return allowShellPolicyDecision()
		}
	}
	for _, systemRoot := range []string{"/Applications", "/Library", "/System", "/bin", "/boot", "/dev", "/etc", "/lib", "/lib32", "/lib64", "/opt", "/proc", "/root", "/sbin", "/sys", "/usr", "/var"} {
		if policyPathWithin(resolved, systemRoot) {
			return blockShellPolicyDecision("shell.policy.block.system", i18n.KeyShellPolicyBlockSystem, resolved)
		}
	}
	roots := append([]string(nil), policy.AllowedDirs...)
	if policy.CWD != "" {
		roots = append(roots, policy.CWD)
	}
	for _, allowed := range roots {
		if policyPathWithin(resolved, allowed) {
			return allowShellPolicyDecision()
		}
	}
	return askShellPolicyDecision("shell.policy.ask.unproven_target", i18n.KeyShellPolicyAskUnprovenTarget, target)
}

func resolveExistingPolicyPath(path string) (string, error) {
	current := filepath.Clean(path)
	tail := make([]string, 0, 4)
	for {
		if _, err := os.Lstat(current); err == nil {
			resolved, resolveErr := filepath.EvalSymlinks(current)
			if resolveErr != nil {
				return "", resolveErr
			}
			for index := len(tail) - 1; index >= 0; index-- {
				resolved = filepath.Join(resolved, tail[index])
			}
			return filepath.Clean(resolved), nil
		} else if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return filepath.Clean(path), nil
		}
		tail = append(tail, filepath.Base(current))
		current = parent
	}
}

func policyPathWithin(path, root string) bool {
	if strings.TrimSpace(root) == "" {
		return false
	}
	resolvedRoot, err := resolveExistingPolicyPath(root)
	if err != nil {
		return false
	}
	return pathWithin(path, resolvedRoot)
}

func validTrustedTempRoot(root string) bool {
	resolved, err := resolveExistingPolicyPath(root)
	if err != nil || resolved == string(filepath.Separator) {
		return false
	}
	for _, base := range []string{"/tmp", "/var/tmp", "/var/folders", "/private/var/folders"} {
		resolvedBase, baseErr := resolveExistingPolicyPath(base)
		if baseErr == nil && pathWithin(resolved, resolvedBase) {
			return true
		}
	}
	// Non-POSIX platforms place temporary directories outside the protected
	// Unix system prefixes; a narrow absolute root remains valid there.
	return filepath.Separator != '/' && filepath.IsAbs(resolved)
}

func pathWithin(path, root string) bool {
	if strings.TrimSpace(root) == "" {
		return false
	}
	path = filepath.Clean(path)
	root = filepath.Clean(root)
	if path == root {
		return true
	}
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func isRawDevicePath(path string) bool {
	clean := filepath.Clean(path)
	for _, prefix := range []string{"/dev/sd", "/dev/hd", "/dev/vd", "/dev/xvd", "/dev/nvme", "/dev/mmcblk", "/dev/disk", "/dev/rdisk", "/dev/mapper/", "/dev/dm-", "/dev/loop"} {
		if strings.HasPrefix(clean, prefix) {
			return true
		}
	}
	return false
}

func shellPolicyWordValue(word *syntax.Word, known map[string]shellPolicyValue, policy types.PolicyContext) shellPolicyValue {
	if word == nil {
		return shellPolicyValue{dynamic: true}
	}
	var result shellPolicyValue
	var appendPart func(syntax.WordPart)
	appendPart = func(part syntax.WordPart) {
		switch value := part.(type) {
		case *syntax.Lit:
			result.literal += value.Value
			if result.possibleLiteral != "" {
				result.possibleLiteral += value.Value
			}
		case *syntax.SglQuoted:
			result.literal += value.Value
			if result.possibleLiteral != "" {
				result.possibleLiteral += value.Value
			}
		case *syntax.DblQuoted:
			for _, quoted := range value.Parts {
				appendPart(quoted)
			}
		case *syntax.ParamExp:
			name := ""
			if value.Param != nil {
				name = value.Param.Value
			}
			if value.Exp != nil && value.Exp.Word != nil && shellPolicyExpansionCanProduceWord(value.Exp.Op) {
				fallback := shellPolicyWordValue(value.Exp.Word, known, policy)
				result.possibleLiteral = result.literal + fallback.literal
				result.dynamic = true
				result.commandSub = fallback.commandSub
				result.variable = name
				return
			}
			if name == "HOME" {
				result.literal += policy.HomeDir
				return
			}
			if knownValue, exists := known[name]; exists {
				result.literal += knownValue.literal
				result.trustedTemp = result.trustedTemp || knownValue.trustedTemp
				if result.trustedRoot == "" {
					result.trustedRoot = knownValue.trustedRoot
				}
				if result.possibleLiteral == "" {
					result.possibleLiteral = knownValue.possibleLiteral
				}
				result.dynamic = result.dynamic || knownValue.dynamic
				result.commandSub = result.commandSub || knownValue.commandSub
				result.variable = knownValue.variable
				if result.variable == "" && knownValue.trustedTemp {
					result.variable = name
				}
				return
			}
			if environment, exists := policy.KnownEnvironment[name]; exists {
				result.literal += environment
				return
			}
			result.dynamic = true
			result.variable = name
		case *syntax.CmdSubst:
			if root, ok := shellPolicyCmdSubstMktempDir(value, known, policy); ok {
				result.trustedTemp = true
				result.trustedRoot = root
				return
			}
			result.dynamic = true
			result.commandSub = true
		default:
			result.dynamic = true
		}
	}
	for _, part := range word.Parts {
		appendPart(part)
	}
	return result
}

func shellPolicyExpansionCanProduceWord(operator syntax.ParExpOperator) bool {
	switch operator {
	case syntax.AlternateUnset, syntax.AlternateUnsetOrNull,
		syntax.DefaultUnset, syntax.DefaultUnsetOrNull,
		syntax.AssignUnset, syntax.AssignUnsetOrNull:
		return true
	default:
		return false
	}
}

func shellPolicyCmdSubstMktempDir(substitution *syntax.CmdSubst, known map[string]shellPolicyValue, policy types.PolicyContext) (string, bool) {
	if substitution == nil || len(substitution.Stmts) != 1 {
		return "", false
	}
	call, ok := substitution.Stmts[0].Cmd.(*syntax.CallExpr)
	if !ok || len(call.Args) == 0 {
		return "", false
	}
	executable := wordToString(call.Args[0])
	if executable != "mktemp" && executable != "/usr/bin/mktemp" && executable != "/bin/mktemp" {
		return "", false
	}
	if _, pathWasAssigned := known["PATH"]; pathWasAssigned {
		return "", false
	}
	directory := false
	root := ""
	var template string
	for index := 1; index < len(call.Args); index++ {
		value := shellPolicyWordValue(call.Args[index], known, policy)
		if value.dynamic || value.commandSub || value.possibleLiteral != "" {
			return "", false
		}
		arg := value.literal
		if arg == "-d" || arg == "--directory" || strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") && strings.ContainsRune(arg, 'd') {
			directory = true
		}
		if arg == "-p" || arg == "--tmpdir" {
			if index+1 >= len(call.Args) {
				return "", false
			}
			index++
			rootValue := shellPolicyWordValue(call.Args[index], known, policy)
			if rootValue.dynamic || rootValue.commandSub || rootValue.possibleLiteral != "" || rootValue.literal == "" {
				return "", false
			}
			root = rootValue.literal
			continue
		}
		if strings.HasPrefix(arg, "--tmpdir=") {
			root = strings.TrimPrefix(arg, "--tmpdir=")
			if root == "" {
				return "", false
			}
		}
		if !strings.HasPrefix(arg, "-") && arg != root {
			template = arg
		}
	}
	if !directory {
		return "", false
	}
	if root == "" && template != "" && strings.ContainsRune(template, filepath.Separator) {
		root = filepath.Dir(template)
	}
	if root == "" {
		if value, exists := known["TMPDIR"]; exists {
			if value.dynamic || value.commandSub || value.possibleLiteral != "" || value.literal == "" {
				return "", false
			}
			root = value.literal
		} else if value, exists := policy.KnownEnvironment["TMPDIR"]; exists {
			root = value
		} else if len(policy.TrustedTempRoots) > 0 {
			root = policy.TrustedTempRoots[0]
		} else {
			root = os.TempDir()
		}
	}
	if root != "" && !filepath.IsAbs(root) {
		root = filepath.Join(policy.CWD, root)
	}
	return filepath.Clean(root), true
}

func shellPolicyWordContainsLiteral(word *syntax.Word, needle string) bool {
	found := false
	syntax.Walk(word, func(node syntax.Node) bool {
		if lit, ok := node.(*syntax.Lit); ok && strings.Contains(lit.Value, needle) {
			found = true
			return false
		}
		return !found
	})
	return found
}

// astDeepCheck parses the command as shell script and walks the AST
// looking for structurally dangerous patterns that regexes miss.
func astDeepCheck(command string) string {
	prog, err := syntax.NewParser(syntax.KeepComments(false)).Parse(strings.NewReader(command), "")
	if err != nil {
		return "" // unparseable — let bash handle it
	}

	var warning string
	syntax.Walk(prog, func(node syntax.Node) bool {
		if warning != "" {
			return false
		}
		switch n := node.(type) {
		case *syntax.CallExpr:
			warning = checkCallExpr(n)
		case *syntax.BinaryCmd:
			warning = checkPipeChain(n)
		case *syntax.Stmt:
			warning = checkRedirects(n)
		}
		return warning == ""
	})
	return warning
}

// cmdName extracts the literal command name from a CallExpr, or "" if it's
// a variable expansion or other dynamic expression.
func cmdName(call *syntax.CallExpr) string {
	if len(call.Args) == 0 {
		return ""
	}
	parts := call.Args[0].Parts
	if len(parts) != 1 {
		return ""
	}
	if lit, ok := parts[0].(*syntax.Lit); ok {
		// Strip path prefix: /usr/bin/rm → rm
		name := lit.Value
		if idx := strings.LastIndex(name, "/"); idx >= 0 {
			name = name[idx+1:]
		}
		return name
	}
	return ""
}

// argLiterals extracts all literal string arguments from a CallExpr (skipping the command name).
func argLiterals(call *syntax.CallExpr) []string {
	if len(call.Args) <= 1 {
		return nil
	}
	var args []string
	for _, word := range call.Args[1:] {
		args = append(args, wordToString(word))
	}
	return args
}

// wordToString converts a syntax.Word to its literal string representation.
// Returns "" for words containing expansions or other non-literal parts.
func wordToString(w *syntax.Word) string {
	var sb strings.Builder
	for _, part := range w.Parts {
		switch p := part.(type) {
		case *syntax.Lit:
			sb.WriteString(p.Value)
		case *syntax.SglQuoted:
			sb.WriteString(p.Value)
		case *syntax.DblQuoted:
			// Only handle fully-literal double-quoted strings
			for _, qp := range p.Parts {
				if lit, ok := qp.(*syntax.Lit); ok {
					sb.WriteString(lit.Value)
				} else {
					return "" // contains expansion
				}
			}
		default:
			return "" // non-literal part (variable, subshell, etc.)
		}
	}
	return sb.String()
}

// hasFlag checks if any argument starts with "-" and contains the given flag character.
func hasFlag(args []string, flag byte) bool {
	for _, a := range args {
		if strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") {
			if strings.ContainsRune(a, rune(flag)) {
				return true
			}
		}
	}
	return false
}

// hasRootPath checks if any non-flag argument is "/" or starts with "/".
func hasRootPath(args []string) bool {
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			continue
		}
		if a == "/" || a == "/*" {
			return true
		}
	}
	return false
}

// checkCallExpr inspects a single command invocation.
func checkCallExpr(call *syntax.CallExpr) string {
	name := cmdName(call)
	if name == "" {
		return ""
	}
	args := argLiterals(call)

	switch name {
	case "rm":
		if hasFlag(args, 'r') && hasFlag(args, 'f') && hasRootPath(args) {
			return toolRuntimeText(i18n.KeyToolRuntimeDangerousRootDelete)
		}
		// Warn if rm -rf has variable arguments (can't verify safety)
		if hasFlag(args, 'r') && hasFlag(args, 'f') && hasParamExpansion(call) {
			return toolRuntimeText(i18n.KeyToolRuntimeDangerousVariableDelete)
		}
	case "mkfs", "mkfs.ext4", "mkfs.xfs", "mkfs.btrfs":
		return toolRuntimeText(i18n.KeyToolRuntimeDangerousFilesystemFormat)
	case "dd":
		for _, a := range args {
			if strings.HasPrefix(a, "of=/dev/") {
				return toolRuntimeText(i18n.KeyToolRuntimeDangerousDirectDiskWrite)
			}
		}
		// dd with variable in of= target
		if hasParamExpansion(call) && hasWordContaining(call, "of=") {
			return toolRuntimeText(i18n.KeyToolRuntimeDangerousVariableDiskWrite)
		}
	case "chmod":
		if hasFlag(args, 'R') || !strings.HasPrefix(strings.Join(args, " "), "-") {
			for _, a := range args {
				if a == "777" {
					if hasRootPath(args) {
						return toolRuntimeText(i18n.KeyToolRuntimeDangerousChmodRoot)
					}
				}
			}
		}
	case "python", "python3", "python2":
		return checkScriptOneLiners(name, args, "shutil.rmtree", "os.remove", "os.system")
	case "perl":
		return checkScriptOneLiners(name, args, "unlink", "rmtree", "system")
	case "ruby":
		return checkScriptOneLiners(name, args, "FileUtils.rm_rf", "system")
	case "eval":
		// eval with any argument is suspicious when combined with other patterns
		if len(args) > 0 {
			// Recursively check what eval would run
			return astDeepCheck(strings.Join(args, " "))
		}
	}

	// Check for process substitution arguments: bash <(curl ...)
	if name == "bash" || name == "sh" {
		for _, word := range call.Args[1:] {
			for _, part := range word.Parts {
				if ps, ok := part.(*syntax.ProcSubst); ok {
					if containsDownloader(ps.Stmts) {
						return toolRuntimeText(i18n.KeyToolRuntimeDangerousProcessSubstitution)
					}
				}
			}
		}
	}

	// Check for commands that write to files via arguments (not redirects).
	// These bypass checkRedirects() but can still modify protected paths.
	return checkWriteCommandArgs(name, args)
}

// checkScriptOneLiners checks if a scripting language one-liner contains dangerous operations.
func checkScriptOneLiners(lang string, args []string, dangerous ...string) string {
	for i, a := range args {
		if a == "-c" && i+1 < len(args) {
			code := args[i+1]
			for _, d := range dangerous {
				if strings.Contains(code, d) {
					return toolRuntimeFormat(i18n.KeyToolRuntimeDangerousLanguageOneLiner, lang)
				}
			}
		}
	}
	return ""
}

// checkPipeChain inspects pipe chains for dangerous patterns like curl|bash.
func checkPipeChain(bin *syntax.BinaryCmd) string {
	if bin.Op != syntax.Pipe {
		return ""
	}

	// Get left and right commands
	leftCmd := extractCmdName(bin.X)
	rightCmd := extractCmdName(bin.Y)

	// curl/wget piped to bash/sh/sudo
	if isDownloader(leftCmd) && isShellOrSudo(rightCmd) {
		return toolRuntimeFormat(i18n.KeyToolRuntimeDangerousRemotePipe, leftCmd, rightCmd)
	}

	// base64 -d piped to bash/sh
	if leftCmd == "base64" && isShellOrSudo(rightCmd) {
		return toolRuntimeFormat(i18n.KeyToolRuntimeDangerousBase64Pipe, rightCmd)
	}

	return ""
}

// checkRedirects inspects statement redirects for dangerous targets.
func checkRedirects(stmt *syntax.Stmt) string {
	for _, redir := range stmt.Redirs {
		if shellPolicyRedirectWritesPath(redir.Op) {
			target := wordToString(redir.Word)
			// Block redirects to raw block devices
			if strings.HasPrefix(target, "/dev/sd") || strings.HasPrefix(target, "/dev/hd") ||
				strings.HasPrefix(target, "/dev/nvme") || strings.HasPrefix(target, "/dev/vd") {
				return toolRuntimeText(i18n.KeyToolRuntimeDangerousRawDeviceRedirect)
			}
			// Block redirects to protected paths
			if isProtectedBashTarget(target) || shellPolicyWordHasUnquotedGlob(redir.Word) && shellPolicyGlobMayMatchProtectedPath(target) {
				return toolRuntimeFormat(i18n.KeyToolRuntimeDangerousProtectedWrite, target)
			}
		}
	}
	return ""
}

// checkWriteCommandArgs detects commands that write to files via arguments
// rather than redirects. This catches: tee, sed -i, cp, mv, install, scp,
// rsync, dd (of=), truncate, and similar commands that can modify protected paths.
// W5 fix: added rsync, dd of=<file>, truncate.
func checkWriteCommandArgs(name string, args []string) string {
	switch name {
	case "tee":
		// tee writes to all listed file arguments (non-flag args).
		for _, a := range args {
			if strings.HasPrefix(a, "-") {
				continue
			}
			if isProtectedBashTarget(a) {
				return toolRuntimeFormat(i18n.KeyToolRuntimeDangerousTeeProtectedWrite, a)
			}
		}
	case "sed":
		// W2 fix: improved sed -i detection.
		// Only flag when -i (in-place) is used; without -i, sed writes to stdout.
		if hasFlag(args, 'i') || hasLongFlag(args, "--in-place") {
			// Parse sed arguments more carefully:
			// -e SCRIPT and -f FILE consume the next argument, so those should
			// not be treated as file operands. Everything else that doesn't
			// start with "-" and doesn't look like a sed script is a file operand.
			fileArgs := sedFileOperands(args)
			for _, f := range fileArgs {
				if isProtectedBashTarget(f) {
					return toolRuntimeFormat(i18n.KeyToolRuntimeDangerousSedProtectedEdit, f)
				}
			}
		}
	case "cp", "install":
		// The last non-flag argument is the destination.
		dest := lastNonFlagArg(args)
		if dest != "" && isProtectedBashTarget(dest) {
			return toolRuntimeFormat(i18n.KeyToolRuntimeDangerousCommandProtectedWrite, name, dest)
		}
	case "mv":
		// The last non-flag argument is the destination.
		dest := lastNonFlagArg(args)
		if dest != "" && isProtectedBashTarget(dest) {
			return toolRuntimeFormat(i18n.KeyToolRuntimeDangerousMoveProtectedWrite, dest)
		}
		// Also check source (moving away from protected path = destructive).
		nonFlags := allNonFlagArgs(args)
		if len(nonFlags) >= 2 {
			for _, src := range nonFlags[:len(nonFlags)-1] {
				if isProtectedBashTarget(src) {
					return toolRuntimeFormat(i18n.KeyToolRuntimeDangerousMoveProtectedSource, src)
				}
			}
		}
	case "scp":
		// scp target is the last argument. Check if it refers to a local protected path.
		dest := lastNonFlagArg(args)
		if dest != "" && !strings.Contains(dest, ":") && isProtectedBashTarget(dest) {
			return toolRuntimeFormat(i18n.KeyToolRuntimeDangerousSCPProtectedWrite, dest)
		}
	case "rsync":
		// W5 fix: rsync destination is the last non-flag argument.
		// Only check local destinations (no ":" means local).
		dest := lastNonFlagArg(args)
		if dest != "" && !strings.Contains(dest, ":") && isProtectedBashTarget(dest) {
			return toolRuntimeFormat(i18n.KeyToolRuntimeDangerousRsyncProtectedWrite, dest)
		}
	case "dd":
		// W5 fix: dd with of=<protected_path> writes to that path.
		for _, a := range args {
			if strings.HasPrefix(a, "of=") {
				target := strings.TrimPrefix(a, "of=")
				if isProtectedBashTarget(target) {
					return toolRuntimeFormat(i18n.KeyToolRuntimeDangerousDDProtectedWrite, target)
				}
			}
		}
	case "truncate":
		// W5 fix: truncate -s <size> <file> modifies the target file.
		// Check all non-flag, non-size arguments.
		skipNext := false
		for _, a := range args {
			if skipNext {
				skipNext = false
				continue
			}
			if a == "-s" || a == "--size" {
				skipNext = true // next arg is the size value
				continue
			}
			if strings.HasPrefix(a, "-s") || strings.HasPrefix(a, "--size=") {
				continue
			}
			if strings.HasPrefix(a, "-") {
				continue
			}
			if isProtectedBashTarget(a) {
				return toolRuntimeFormat(i18n.KeyToolRuntimeDangerousTruncateProtected, a)
			}
		}
	}
	return ""
}

// hasLongFlag checks if any argument matches a long-form flag (e.g. "--in-place").
func hasLongFlag(args []string, flag string) bool {
	for _, a := range args {
		if a == flag || strings.HasPrefix(a, flag+"=") {
			return true
		}
	}
	return false
}

// lastNonFlagArg returns the last argument that doesn't start with "-".
func lastNonFlagArg(args []string) string {
	var last string
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			last = a
		}
	}
	return last
}

// allNonFlagArgs returns all arguments that don't start with "-".
func allNonFlagArgs(args []string) []string {
	var result []string
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			result = append(result, a)
		}
	}
	return result
}

// sedFileOperands extracts the file operands from sed arguments, skipping:
// - flags (anything starting with "-")
// - arguments consumed by -e (script) and -f (script-file)
// - inline scripts (arguments that look like sed substitution expressions)
// W2 fix: this is more accurate than the previous simple filter.
func sedFileOperands(args []string) []string {
	var files []string
	skipNext := false
	seenScript := false // track if we've seen at least one explicit -e script
	for i, a := range args {
		if skipNext {
			skipNext = false
			continue
		}
		// -e SCRIPT: next arg is a script, skip it
		if a == "-e" || a == "--expression" {
			skipNext = true
			seenScript = true
			continue
		}
		// -eINLINE (merged form)
		if strings.HasPrefix(a, "-e") && !strings.HasPrefix(a, "--") {
			seenScript = true
			continue
		}
		if strings.HasPrefix(a, "--expression=") {
			seenScript = true
			continue
		}
		// -f FILE: next arg is a script file, skip it
		if a == "-f" || a == "--file" {
			skipNext = true
			seenScript = true
			continue
		}
		if strings.HasPrefix(a, "-f") && !strings.HasPrefix(a, "--") {
			seenScript = true
			continue
		}
		if strings.HasPrefix(a, "--file=") {
			seenScript = true
			continue
		}
		// Skip -i, -i.bak, --in-place, --in-place=.bak, and other flags
		if strings.HasPrefix(a, "-") {
			continue
		}
		// First non-flag, non-consumed argument could be an inline script
		// (when no -e was given). After the first script, the rest are files.
		if !seenScript && i > 0 && isSedScript(a) {
			seenScript = true
			continue
		}
		// This is a file operand.
		seenScript = true // any non-flag after script position is a file
		files = append(files, a)
	}
	return files
}

// isSedScript returns true if the string looks like a sed script expression.
func isSedScript(s string) bool {
	// Common sed script starters: s/ s| y/ a\ i\ d p q etc.
	if len(s) == 0 {
		return false
	}
	// Single-char commands
	if len(s) == 1 && strings.ContainsRune("dpqx", rune(s[0])) {
		return true
	}
	// Substitution: s/.../ or s|...|
	if len(s) >= 2 && (s[0] == 's' || s[0] == 'y') && (s[1] == '/' || s[1] == '|' || s[1] == '#' || s[1] == ',') {
		return true
	}
	// Address + command: /pattern/d, /pattern/p, etc.
	if s[0] == '/' {
		return true
	}
	return false
}

// isProtectedBashTarget checks if a bash redirect target is a protected path.
// Delegates to permissions.IsProtectedPath() which is the single source of truth
// with full path normalization including filepath.Abs() traversal prevention.
func isProtectedBashTarget(target string) bool {
	if target == "" {
		return false
	}
	return permissions.IsProtectedPath(target)
}

// extractCmdName gets the command name from a statement node.
func extractCmdName(stmt *syntax.Stmt) string {
	if stmt == nil || stmt.Cmd == nil {
		return ""
	}
	if call, ok := stmt.Cmd.(*syntax.CallExpr); ok {
		return cmdName(call)
	}
	return ""
}

// isDownloader returns true for commands that fetch remote content.
func isDownloader(cmd string) bool {
	return cmd == "curl" || cmd == "wget"
}

// isShellOrSudo returns true for shell interpreters and privilege escalation.
func isShellOrSudo(cmd string) bool {
	return cmd == "bash" || cmd == "sh" || cmd == "sudo" || cmd == "zsh"
}

// containsDownloader checks if a list of statements contains a curl/wget call.
func containsDownloader(stmts []*syntax.Stmt) bool {
	for _, stmt := range stmts {
		if stmt.Cmd == nil {
			continue
		}
		if call, ok := stmt.Cmd.(*syntax.CallExpr); ok {
			if isDownloader(cmdName(call)) {
				return true
			}
		}
		// Check nested pipes
		if bin, ok := stmt.Cmd.(*syntax.BinaryCmd); ok {
			leftName := extractCmdName(bin.X)
			if isDownloader(leftName) {
				return true
			}
		}
	}
	return false
}

// hasParamExpansion checks if any argument in a CallExpr contains a variable
// expansion ($VAR, ${VAR}, etc.). When dangerous commands use variables as
// targets, we can't statically verify safety.
func hasParamExpansion(call *syntax.CallExpr) bool {
	if len(call.Args) <= 1 {
		return false
	}
	for _, word := range call.Args[1:] {
		if wordHasParamExp(word) {
			return true
		}
	}
	return false
}

// hasWordContaining checks if any argument word contains a literal substring.
// Used to detect patterns like "of=" in dd arguments that also have variable parts.
func hasWordContaining(call *syntax.CallExpr, substr string) bool {
	if len(call.Args) <= 1 {
		return false
	}
	for _, word := range call.Args[1:] {
		for _, part := range word.Parts {
			if lit, ok := part.(*syntax.Lit); ok {
				if strings.Contains(lit.Value, substr) {
					return true
				}
			}
		}
	}
	return false
}

// wordHasParamExp recursively checks if a word contains any parameter expansion.
func wordHasParamExp(w *syntax.Word) bool {
	for _, part := range w.Parts {
		switch p := part.(type) {
		case *syntax.ParamExp:
			return true
		case *syntax.DblQuoted:
			for _, qp := range p.Parts {
				if _, ok := qp.(*syntax.ParamExp); ok {
					return true
				}
			}
		case *syntax.CmdSubst:
			// $(cmd) — conservatively flag this too
			return true
		}
	}
	return false
}
