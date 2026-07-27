package shell

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

const (
	runVerificationNone           = ""
	runVerificationBuild          = "build"
	runVerificationStaticAnalysis = "static_analysis"
	runVerificationTargetedTest   = "targeted_test"
	runVerificationFullTest       = "full_test"
)

// runVerificationAttestation classifies the normalized plan actually accepted
// by Run. It never inspects process output or model prose. The plan binding is
// already a digest of argv/scripts, cwd, timeouts, dependencies, policy, and
// resources, so publishing its derived digest binds the evidence to that exact
// executor-owned verification configuration without exposing commands.
func runVerificationAttestation(plan *compiledRunPlan) (kind, configDigest string) {
	if plan == nil {
		return "", ""
	}
	for _, step := range plan.steps {
		candidate := classifyRunStepVerification(step)
		if runVerificationRank(candidate) > runVerificationRank(kind) {
			kind = candidate
		}
	}
	if kind == runVerificationNone || plan.bindingCode == "" {
		return "", ""
	}
	sum := sha256.Sum256([]byte(plan.bindingCode + "\x00" + kind))
	return kind, hex.EncodeToString(sum[:])
}

func classifyRunStepVerification(step compiledRunStep) string {
	if !step.useShell {
		if len(step.argv) == 0 || strings.ContainsAny(step.argv[0], `/\\`) {
			return runVerificationNone
		}
		return classifyRunVerificationArgv(step.argv)
	}
	argv, ok := strictSimpleVerificationShell(step.shellScript)
	if !ok {
		return runVerificationNone
	}
	return classifyRunVerificationArgv(argv)
}

func strictSimpleVerificationShell(script string) ([]string, bool) {
	program, err := syntax.NewParser(syntax.KeepComments(false)).Parse(strings.NewReader(script), "")
	if err != nil {
		return nil, false
	}
	if len(program.Stmts) != 1 {
		return nil, false
	}
	statement := program.Stmts[0]
	if statement.Negated || statement.Background || statement.Coprocess || statement.Disown || len(statement.Redirs) != 0 {
		return nil, false
	}
	call, ok := statement.Cmd.(*syntax.CallExpr)
	if !ok || len(call.Assigns) != 0 || len(call.Args) == 0 {
		return nil, false
	}
	argv := make([]string, len(call.Args))
	for index, word := range call.Args {
		literal, literalOK := strictVerificationLiteral(word)
		if !literalOK {
			return nil, false
		}
		argv[index] = literal
	}
	if argv[0] == "" || strings.ContainsAny(argv[0], `/\\`) {
		return nil, false
	}
	return argv, true
}

func strictVerificationLiteral(word *syntax.Word) (string, bool) {
	if word == nil || len(word.Parts) == 0 {
		return "", false
	}
	var value strings.Builder
	for _, part := range word.Parts {
		switch typed := part.(type) {
		case *syntax.Lit:
			value.WriteString(typed.Value)
		case *syntax.SglQuoted:
			value.WriteString(typed.Value)
		case *syntax.DblQuoted:
			for _, quoted := range typed.Parts {
				literal, ok := quoted.(*syntax.Lit)
				if !ok {
					return "", false
				}
				value.WriteString(literal.Value)
			}
		default:
			return "", false
		}
	}
	return value.String(), true
}

func classifyRunVerificationArgv(argv []string) string {
	if len(argv) == 0 {
		return runVerificationNone
	}
	name := strings.ToLower(filepathBase(strings.TrimSpace(argv[0])))
	args := argv[1:]
	switch name {
	case "env", "command", "builtin", "exec", "nice", "nohup", "time", "timeout", "stdbuf", "sudo", "doas":
		return classifyWrappedRunVerification(args)
	case "bash", "sh", "zsh", "dash", "ksh":
		for index, argument := range args {
			if argument == "-c" && index+1 < len(args) {
				argv, ok := strictSimpleVerificationShell(args[index+1])
				if !ok {
					return runVerificationNone
				}
				return classifyRunVerificationArgv(argv)
			}
		}
	case "go":
		subcommand := firstNonFlag(args)
		switch subcommand {
		case "test":
			if containsRunArgument(args, "./...") {
				return runVerificationFullTest
			}
			return runVerificationTargetedTest
		case "vet":
			return runVerificationStaticAnalysis
		case "build":
			return runVerificationBuild
		}
	case "pytest", "py.test", "nosetests", "nosetests3", "jest", "vitest", "mocha", "ava", "rspec", "ctest", "tox":
		return runVerificationTargetedTest
	case "python", "python3", "pypy", "pypy3":
		for index := 0; index+1 < len(args); index++ {
			if args[index] != "-m" {
				continue
			}
			switch strings.ToLower(args[index+1]) {
			case "pytest", "unittest", "nose", "tox":
				return runVerificationTargetedTest
			case "compileall":
				return runVerificationBuild
			}
		}
	case "cargo":
		switch firstNonFlag(args) {
		case "test":
			return runVerificationTargetedTest
		case "check", "clippy", "fmt":
			return runVerificationStaticAnalysis
		case "build":
			return runVerificationBuild
		}
	case "dotnet":
		switch firstNonFlag(args) {
		case "test":
			return runVerificationTargetedTest
		case "build":
			return runVerificationBuild
		case "format":
			return runVerificationStaticAnalysis
		}
	case "npm", "pnpm", "yarn", "bun":
		return classifyPackageScriptVerification(args)
	case "uv", "poetry", "bundle", "npx":
		return classifyVerificationRunnerSuffix(args)
	case "make", "gmake", "gradle", "gradlew", "mvn", "mvnw", "rake":
		return classifyBuildTargetVerification(args)
	case "eslint", "ruff", "mypy", "pyright", "pylint", "golangci-lint", "staticcheck", "shellcheck", "clang-tidy", "rubocop", "biome":
		return runVerificationStaticAnalysis
	case "tsc", "rustc", "javac", "gcc", "g++", "clang", "clang++", "swiftc":
		return runVerificationBuild
	case "git":
		if firstNonFlag(args) == "diff" && containsRunArgument(args, "--check") {
			return runVerificationStaticAnalysis
		}
	}
	return runVerificationNone
}

func classifyWrappedRunVerification(args []string) string {
	for index, argument := range args {
		if argument == "--" && index+1 < len(args) {
			return classifyRunVerificationArgv(args[index+1:])
		}
		if strings.Contains(argument, "=") && !strings.HasPrefix(argument, "-") {
			continue
		}
		if strings.HasPrefix(argument, "-") || isDecimalRunArgument(argument) {
			continue
		}
		return classifyRunVerificationArgv(args[index:])
	}
	return runVerificationNone
}

func classifyPackageScriptVerification(args []string) string {
	for _, argument := range args {
		switch strings.ToLower(strings.TrimSpace(argument)) {
		case "test", "test:unit", "test:integration", "spec":
			return runVerificationTargetedTest
		case "lint", "typecheck", "type-check", "check", "format:check":
			return runVerificationStaticAnalysis
		case "build", "compile":
			return runVerificationBuild
		}
	}
	return runVerificationNone
}

func classifyVerificationRunnerSuffix(args []string) string {
	for index, argument := range args {
		name := strings.ToLower(filepathBase(argument))
		switch name {
		case "pytest", "py.test", "unittest", "jest", "vitest", "mocha", "rspec", "rake", "ctest", "tox":
			return runVerificationTargetedTest
		case "eslint", "ruff", "mypy", "pyright", "pylint", "tsc", "golangci-lint", "staticcheck":
			return runVerificationStaticAnalysis
		case "go", "cargo", "dotnet", "make", "gradle", "mvn":
			return classifyRunVerificationArgv(args[index:])
		}
	}
	return runVerificationNone
}

func classifyBuildTargetVerification(args []string) string {
	for _, argument := range args {
		normalized := strings.ToLower(strings.TrimLeft(strings.TrimSpace(argument), ":"))
		switch normalized {
		case "test", "tests", "check", "verify", "spec":
			return runVerificationTargetedTest
		case "lint", "clippy", "fmt", "format", "typecheck", "type-check":
			return runVerificationStaticAnalysis
		case "build", "compile", "assemble", "package":
			return runVerificationBuild
		}
	}
	return runVerificationNone
}

func containsRunArgument(args []string, wanted string) bool {
	for _, argument := range args {
		if argument == wanted {
			return true
		}
	}
	return false
}

func isDecimalRunArgument(value string) bool {
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

func runVerificationRank(kind string) int {
	switch kind {
	case runVerificationStaticAnalysis:
		return 1
	case runVerificationBuild:
		return 2
	case runVerificationTargetedTest:
		return 3
	case runVerificationFullTest:
		return 4
	default:
		return 0
	}
}
