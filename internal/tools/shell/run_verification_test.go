package shell

import "testing"

func TestRunVerificationClassifier(t *testing.T) {
	tests := []struct {
		name string
		step compiledRunStep
		want string
	}{
		{name: "full go tests", step: compiledRunStep{argv: []string{"go", "test", "./..."}}, want: runVerificationFullTest},
		{name: "targeted go tests", step: compiledRunStep{argv: []string{"go", "test", "./internal/app"}}, want: runVerificationTargetedTest},
		{name: "go vet", step: compiledRunStep{argv: []string{"go", "vet", "./..."}}, want: runVerificationStaticAnalysis},
		{name: "go build", step: compiledRunStep{argv: []string{"go", "build", "./..."}}, want: runVerificationBuild},
		{name: "gofmt diff", step: compiledRunStep{argv: []string{"gofmt", "-d", "source.go"}}, want: runVerificationStaticAnalysis},
		{name: "gofmt write", step: compiledRunStep{argv: []string{"gofmt", "-w", "source.go"}}, want: runVerificationNone},
		{name: "pytest", step: compiledRunStep{argv: []string{"pytest", "tests/test_api.py"}}, want: runVerificationTargetedTest},
		{name: "node syntax check", step: compiledRunStep{argv: []string{"node", "--check", "verify.js"}}, want: runVerificationStaticAnalysis},
		{name: "node short syntax check", step: compiledRunStep{argv: []string{"nodejs", "-c", "verify.js"}}, want: runVerificationStaticAnalysis},
		{name: "node script execution", step: compiledRunStep{argv: []string{"node", "verify.js"}}, want: runVerificationNone},
		{name: "node eval is not syntax check", step: compiledRunStep{argv: []string{"node", "--check", "verify.js", "--eval", "process.exit()"}}, want: runVerificationNone},
		{name: "package lint", step: compiledRunStep{argv: []string{"npm", "run", "lint"}}, want: runVerificationStaticAnalysis},
		{name: "cargo check", step: compiledRunStep{argv: []string{"cargo", "check"}}, want: runVerificationStaticAnalysis},
		{name: "project Maven wrapper", step: compiledRunStep{argv: []string{"./mvnw", "test"}}, want: runVerificationTargetedTest},
		{name: "project Gradle wrapper", step: compiledRunStep{argv: []string{"./gradlew", "check"}}, want: runVerificationTargetedTest},
		{name: "wrapped tests", step: compiledRunStep{argv: []string{"timeout", "30", "go", "test", "./..."}}, want: runVerificationFullTest},
		{name: "strict shell tests", step: compiledRunStep{useShell: true, shellScript: "go test ./..."}, want: runVerificationFullTest},
		{name: "compound shell tests", step: compiledRunStep{useShell: true, shellScript: "cd internal/app && go test ./..."}, want: runVerificationNone},
		{name: "pwd observation", step: compiledRunStep{argv: []string{"pwd"}}, want: runVerificationNone},
		{name: "git status observation", step: compiledRunStep{argv: []string{"git", "status"}}, want: runVerificationNone},
		{name: "argument is not command", step: compiledRunStep{argv: []string{"echo", "pytest"}}, want: runVerificationNone},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := classifyRunStepVerification(test.step); got != test.want {
				t.Fatalf("classification = %q, want %q", got, test.want)
			}
		})
	}
}

func TestRunVerificationClassifierRejectsMaskedExitStatus(t *testing.T) {
	for _, script := range []string{
		"go test ./... || true",
		"go test ./...; true",
		"go test ./...\ntrue",
		"! go test ./...",
		"go test ./... &",
		"(go test ./...)",
		"{ go test ./...; }",
		"go test ./... | cat",
		"echo $(go test ./...)",
	} {
		t.Run(script, func(t *testing.T) {
			if got := classifyRunStepVerification(compiledRunStep{useShell: true, shellScript: script}); got != runVerificationNone {
				t.Fatalf("masked shell classified as %q", got)
			}
		})
	}
	if got := classifyRunStepVerification(compiledRunStep{argv: []string{"/tmp/fake/go", "test", "./..."}}); got != runVerificationNone {
		t.Fatalf("path-selected verifier classified as %q", got)
	}
	if got := classifyRunStepVerification(compiledRunStep{argv: []string{"../mvnw", "test"}}); got != runVerificationNone {
		t.Fatalf("parent-selected wrapper classified as %q", got)
	}
}

func TestRunVerificationAttestationBindsExactPlan(t *testing.T) {
	step := compiledRunStep{argv: []string{"go", "test", "./internal/app"}}
	firstKind, firstDigest := runVerificationAttestation(&compiledRunPlan{steps: []compiledRunStep{step}, bindingCode: "plan-binding-a"})
	secondKind, secondDigest := runVerificationAttestation(&compiledRunPlan{steps: []compiledRunStep{step}, bindingCode: "plan-binding-b"})
	if firstKind != runVerificationTargetedTest || secondKind != firstKind || firstDigest == "" || secondDigest == "" || firstDigest == secondDigest {
		t.Fatalf("attestations = (%q, %q), (%q, %q)", firstKind, firstDigest, secondKind, secondDigest)
	}
	if kind, digest := runVerificationAttestation(&compiledRunPlan{
		steps: []compiledRunStep{{argv: []string{"pwd"}}}, bindingCode: "observation-binding",
	}); kind != "" || digest != "" {
		t.Fatalf("observation attestation = (%q, %q)", kind, digest)
	}
}
