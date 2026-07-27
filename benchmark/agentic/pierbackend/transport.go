package pierbackend

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/agent-dance/luban/benchmark/agentic/evidenceproxy"
	"github.com/agent-dance/luban/benchmark/agentic/harness"
)

func (backend *Backend) RunAgent(ctx context.Context, invocation harness.AgentInvocation) (execution harness.AgentExecution, runErr error) {
	controllerStartedAt := time.Now().UTC()
	task, manifest, err := backend.resolvedTask(invocation.Task.ID)
	if err != nil {
		return harness.AgentExecution{}, err
	}
	bundle, err := backend.codexBundleSnapshot()
	if err != nil {
		return harness.AgentExecution{}, err
	}
	adapter, err := backend.pinnedAdapterSnapshot()
	if err != nil {
		return harness.AgentExecution{}, err
	}
	var canonicalCanary formalCodexCanonicalCanaryBinding
	if invocation.Agent.ID == "codex" {
		canonicalCanary, err = backend.codexCanonicalCanarySnapshot()
		if err != nil {
			return harness.AgentExecution{}, err
		}
	}
	sourceCommandArgvSHA, err := harness.HashCanonical(invocation.Agent.Command.Argv)
	if err != nil {
		return harness.AgentExecution{}, err
	}
	providerEndpoint, err := backend.providerEndpointSnapshot()
	if err != nil {
		return harness.AgentExecution{}, err
	}
	credential, err := exactEnvironmentValue(invocation.Environment, backend.config.ProviderCredentialEnv)
	if err != nil {
		return harness.AgentExecution{}, err
	}
	privateTask, cleanup, materializedSHA, err := backend.preparePrivateTask(task, manifest)
	if err != nil {
		return harness.AgentExecution{}, err
	}
	defer func() {
		backend.mu.RLock()
		development := backend.development
		backend.mu.RUnlock()
		if !development || runErr == nil {
			cleanup()
		}
	}()
	privateRoot := filepath.Dir(privateTask)
	access, err := randomHex(32)
	if err != nil {
		return harness.AgentExecution{}, err
	}
	runIdentity, err := randomHex(32)
	if err != nil {
		return harness.AgentExecution{}, err
	}
	lifecycle := harness.AttemptLifecycle{
		SchemaVersion: "agentic-bench/attempt-lifecycle-v1", RunIdentity: runIdentity,
		ControllerStartedAt: controllerStartedAt, ProviderAttemptState: "no_provider_attempt",
	}
	if err := harness.WriteJSONAtomic(filepath.Join(invocation.ArtifactDir, "attempt-lifecycle.json"), lifecycle, 0o600); err != nil {
		return harness.AgentExecution{}, err
	}
	rawEvidence := filepath.Join(invocation.ArtifactDir, "metrics", "provider-http.jsonl")
	readyPath := filepath.Join(privateRoot, "proxy.ready")
	proxyConfig := evidenceproxy.Config{
		ListenAddress: backend.config.ProxyListenAddress, Upstream: backend.config.ProviderUpstream,
		ApprovedOrigin:          providerEndpoint.ApprovedOrigin,
		EndpointSemanticsSHA256: providerEndpoint.SemanticsSHA256,
		RequireTLSPeerEvidence:  true,
		EvidencePath:            rawEvidence, ReadyPath: readyPath, AccessPath: "/" + access,
		Credential: credential, ExpectedModel: invocation.Agent.Model.Model,
		ExpectedEffort: invocation.Agent.Model.ReasoningEffort, AgentID: invocation.Agent.ID,
		RegisteredBinarySHA256:             invocation.Agent.BinarySHA256,
		FrozenBundleManifestSHA256:         bundle.ManifestSHA256,
		FrozenBundleTreeSHA256:             bundle.TreeSHA256,
		FrozenCanonicalCanaryReceiptSHA256: canonicalCanary.SHA256,
		AdapterSHA256:                      adapter.SHA256,
		AdapterVersion:                     PinnedAdapterVersion,
		SourceCommandArgvSHA256:            sourceCommandArgvSHA,
		RunIdentity:                        runIdentity,
		Transport:                          backend.config.ProviderTransport,
	}
	proxyConfig.ClientCanonicalizationStaticProofSHA256, err = evidenceproxy.ServiceTierCanonicalizationStaticProof(proxyConfig)
	if err != nil {
		return harness.AgentExecution{}, err
	}
	proxyContext, cancelProxy := context.WithCancel(ctx)
	proxyErrors := make(chan error, 1)
	go func() {
		proxyErrors <- evidenceproxy.Run(proxyContext, proxyConfig)
	}()
	address, err := waitProxyReady(ctx, readyPath, proxyErrors)
	if err != nil {
		cancelProxy()
		return harness.AgentExecution{}, err
	}
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		cancelProxy()
		return harness.AgentExecution{}, err
	}
	proxyOrigin := "http://" + net.JoinHostPort(backend.config.ProxyAdvertiseHost, port)
	proxyBaseURL := proxyOrigin + "/" + access + "/v1"
	proxyHealthURL := proxyOrigin + "/healthz"
	redaction, err := newPublicRedactionPolicy(invocation, backend.config, bundle, credential, access, proxyBaseURL, privateTask)
	if err != nil {
		cancelProxy()
		return harness.AgentExecution{}, err
	}
	// This guard runs after every later return, including protocol failures and
	// the final sealed-attempt write. A failed scan alone is insufficient: it
	// would leave the detected authority readable in a nominally public tree.
	// Sanitize first, prove that the remaining tree is clean, and invalidate any
	// attempt for which sanitization was necessary.
	defer func() {
		receiptPath := filepath.Join(invocation.ArtifactDir, "pier", "public-secret-scan.json")
		sanitizedFiles, sanitizeErr := redaction.sanitizePublicTree(invocation.ArtifactDir, receiptPath)
		scanErr := sanitizeErr
		if scanErr == nil {
			scanErr = redaction.scanAndWriteReceipt(invocation.ArtifactDir, receiptPath)
		}
		switch {
		case scanErr != nil:
			execution = harness.AgentExecution{}
			runErr = harness.AttemptProtocolError{Err: scanErr}
		case sanitizedFiles != 0:
			execution = harness.AgentExecution{}
			runErr = harness.AttemptProtocolError{Err: errors.New("public artifact sanitizer intercepted prohibited private authority")}
		}
	}()
	commandJSON, err := json.Marshal(invocation.Agent.Command.Argv)
	if err != nil {
		cancelProxy()
		return harness.AgentExecution{}, err
	}
	jobsRoot := filepath.Join(privateRoot, "jobs")
	jobName := invocation.PlanEntry.AgentID + "-" + task.ID
	args := commonPierArgs(privateTask, jobsRoot, jobName, manifest)
	kwargs := []string{
		"agent_kind=" + invocation.Agent.ID,
		"binary_path=" + invocation.Agent.Binary,
		"binary_sha256=" + invocation.Agent.BinarySHA256,
		"command_argv=" + string(commandJSON),
		"proxy_base_url=" + proxyBaseURL,
		"proxy_health_url=" + proxyHealthURL,
		"proxy_host=" + backend.config.ProxyAdvertiseHost,
		"reasoning_effort=" + invocation.Agent.Model.ReasoningEffort,
		"base_commit=" + task.BaseCommit,
		"binary_bundle_root=" + bundle.Root,
		"binary_bundle_manifest_path=" + bundle.ManifestPath,
		"binary_bundle_tree_sha256=" + bundle.TreeSHA256,
		"binary_bundle_manifest_sha256=" + bundle.ManifestSHA256,
		"adapter_sha256=" + adapter.SHA256,
		"adapter_version=" + PinnedAdapterVersion,
	}
	kwargs = append(kwargs, "source_command_argv_sha256="+sourceCommandArgvSHA)
	args = append(args,
		"--environment-kwarg", "private_proxy_port="+port,
		"--environment-kwarg", "egress_proxy_image="+backend.config.EgressProxyImage,
		"--environment-import-path", DockerEnvironmentImportPath,
		"--agent-import-path", AdapterImportPath,
		"--model", invocation.Agent.Model.Provider+"/"+invocation.Agent.Model.Model,
	)
	for _, kwarg := range kwargs {
		args = append(args, "--agent-kwarg", kwarg)
	}
	stdout, stderr, commandErr := backend.runPier(ctx, args, invocation.Environment, privateRoot)
	cancelProxy()
	proxyErr := waitProxyStop(proxyErrors)
	credential = ""
	redactedStdout := redaction.redact(stdout)
	redactedStderr := redaction.redact(stderr)
	if err := harness.WriteBytesAtomic(filepath.Join(invocation.ArtifactDir, "pier.stdout.log"), redactedStdout, 0o644); err != nil {
		return harness.AgentExecution{}, err
	}
	if err := harness.WriteBytesAtomic(filepath.Join(invocation.ArtifactDir, "pier.stderr.log"), redactedStderr, 0o644); err != nil {
		return harness.AgentExecution{}, err
	}
	earlyScanPath := filepath.Join(invocation.ArtifactDir, "pier", "public-secret-scan.json")
	if err := redaction.scanAndWriteReceipt(invocation.ArtifactDir, earlyScanPath); err != nil {
		return harness.AgentExecution{}, harness.AttemptProtocolError{Err: err}
	}
	if proxyErr != nil {
		return harness.AgentExecution{}, fmt.Errorf("provider meter stopped unexpectedly: %w", proxyErr)
	}
	if isRuntimeSourceIntegrityError(commandErr) {
		return harness.AgentExecution{}, commandErr
	}
	trialDir, locateErr := findSingleTrial(filepath.Join(jobsRoot, jobName))
	if locateErr != nil {
		return harness.AgentExecution{}, joinCommandError(commandErr, locateErr, redactedStdout, redactedStderr)
	}
	if err := archiveNonpublishedRawBundle(backend.config, invocation, runIdentity, trialDir, stdout, stderr); err != nil {
		return harness.AgentExecution{}, err
	}
	parsed, err := parseTrialResult(filepath.Join(trialDir, "result.json"))
	if err != nil {
		return harness.AgentExecution{}, harness.AttemptProtocolError{Err: err}
	}
	execution, err = backend.archiveAgentTrial(invocation, trialDir, parsed, rawEvidence, runIdentity, materializedSHA, proxyBaseURL, args, redaction)
	if err != nil {
		return harness.AgentExecution{}, err
	}
	lifecycle.ControllerFinishedAt = time.Now().UTC()
	lifecycle.ProviderAttemptState = "provider_attempt_sealed"
	lifecycle.ProviderAttemptCount = execution.ProviderEvidence.StartedAttemptCount
	execution.Lifecycle = lifecycle
	if err := harness.WriteJSONAtomic(filepath.Join(invocation.ArtifactDir, "attempt-lifecycle.json"), lifecycle, 0o600); err != nil {
		return harness.AgentExecution{}, err
	}
	if len(parsed.Rewards) == 0 {
		cause := fmt.Errorf("Pier separate verifier produced no reward (exception=%s)", parsed.ExceptionType)
		if commandErr != nil {
			cause = joinCommandError(commandErr, nil, redactedStdout, redactedStderr)
		}
		category, classified := classifyTrialInfrastructure(rawEvidence, parsed)
		if !classified {
			return harness.AgentExecution{}, fmt.Errorf("unclassified no-reward Pier trial: %w", cause)
		}
		if err := sealAgentAttempt(invocation, execution, category, cause.Error()); err != nil {
			return harness.AgentExecution{}, err
		}
		return execution, harness.AttemptInfrastructureError{Category: category, Err: cause}
	}
	if err := sealAgentAttempt(invocation, execution, harness.DeepSWEFailureNone, ""); err != nil {
		return harness.AgentExecution{}, err
	}
	return execution, nil
}

func (backend *Backend) archiveAgentTrial(invocation harness.AgentInvocation, trialDir string, parsed sanitizedTrialResult, rawEvidence, runIdentity, materializedSHA, proxyBaseURL string, args []string, redaction publicRedactionPolicy) (harness.AgentExecution, error) {
	if parsed.TaskName != invocation.Task.ID && !strings.HasSuffix(parsed.TaskName, "/"+invocation.Task.ID) {
		return harness.AgentExecution{}, harness.AttemptProtocolError{Err: errors.New("Pier trial result belongs to a different task")}
	}
	if parsed.AgentName != "agentic-bench-pinned-cli" || parsed.AgentVersion != PinnedAdapterVersion || parsed.Provider != invocation.Agent.Model.Provider || parsed.Model != invocation.Agent.Model.Model {
		return harness.AgentExecution{}, harness.AttemptProtocolError{Err: errors.New("Pier trial result does not identify the frozen adapter and model")}
	}
	pierDir := filepath.Join(invocation.ArtifactDir, "pier")
	if err := os.MkdirAll(pierDir, 0o755); err != nil {
		return harness.AgentExecution{}, err
	}
	var verification *harness.VerificationResult
	if len(parsed.Rewards) > 0 {
		if parsed.VerifierStarted.IsZero() || parsed.VerifierEnded.IsZero() || parsed.VerifierEnded.Before(parsed.VerifierStarted) {
			return harness.AgentExecution{}, harness.AttemptProtocolError{Err: errors.New("Pier result lacks valid separate-verifier timing")}
		}
		result, err := exportVerification(trialDir, filepath.Join(invocation.ArtifactDir, "verifier"), parsed, redaction.redact)
		if err != nil {
			return harness.AgentExecution{}, err
		}
		verification = &result
	}
	adapter, err := backend.pinnedAdapterSnapshot()
	if err != nil {
		return harness.AgentExecution{}, err
	}
	bundle, err := backend.codexBundleSnapshot()
	if err != nil {
		return harness.AgentExecution{}, err
	}
	effective, effectiveReceiptSHA, err := readEffectiveArgvReceipt(
		filepath.Join(trialDir, "agent", "effective-argv.json"), invocation, adapter, bundle, proxyBaseURL,
	)
	if err != nil {
		return harness.AgentExecution{}, harness.AttemptProtocolError{Err: err}
	}
	agentArtifacts := []string{"stream.jsonl", "stderr.log", "sandbox-canary.json", "effective-argv.json", "workspace-capture.json", "exit.json", "pilot-guest-storage-receipt.json"}
	terminalReceiptPath := filepath.Join(trialDir, "agent", "terminal-evidence.json")
	terminalReceiptExists := false
	if parsed.ExceptionType != "AgentTimeoutError" {
		if _, statErr := os.Stat(terminalReceiptPath); statErr == nil {
			terminalReceiptExists = true
			agentArtifacts = append(agentArtifacts, "terminal-evidence.json")
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return harness.AgentExecution{}, harness.AttemptProtocolError{Err: fmt.Errorf("inspect adapter terminal evidence: %w", statErr)}
		}
	}
	for _, name := range agentArtifacts {
		source := filepath.Join(trialDir, "agent", name)
		if _, err := os.Stat(source); err != nil {
			return harness.AgentExecution{}, harness.AttemptProtocolError{Err: fmt.Errorf("Pier agent output lacks %s: %w", name, err)}
		}
		raw, err := os.ReadFile(source)
		if err != nil {
			return harness.AgentExecution{}, err
		}
		// Terminal evidence is regenerated below from the public redacted stream,
		// so its digest remains independently verifiable without private raw data.
		if name == "terminal-evidence.json" {
			continue
		}
		if err := harness.WriteBytesAtomic(filepath.Join(pierDir, "agent-"+name), redaction.redact(raw), 0o600); err != nil {
			return harness.AgentExecution{}, err
		}
	}
	sandboxCanaryPath := filepath.Join(trialDir, "agent", "sandbox-canary.json")
	if err := validateSandboxCanary(sandboxCanaryPath, invocation, adapter, bundle, effectiveReceiptSHA); err != nil {
		return harness.AgentExecution{}, harness.AttemptProtocolError{Err: err}
	}
	sandboxCanarySHA, err := harness.HashFile(sandboxCanaryPath)
	if err != nil {
		return harness.AgentExecution{}, err
	}
	fullPatch := filepath.Join(trialDir, "agent", "full-workspace.patch")
	committedPatch := filepath.Join(trialDir, "agent", "committed-workspace.patch")
	modelPatch := filepath.Join(trialDir, "artifacts", "model.patch")
	fullSHA, err := harness.HashFile(fullPatch)
	if err != nil {
		return harness.AgentExecution{}, err
	}
	modelSHA, err := harness.HashFile(modelPatch)
	if err != nil {
		return harness.AgentExecution{}, err
	}
	committedSHA, err := harness.HashFile(committedPatch)
	if err != nil {
		return harness.AgentExecution{}, err
	}
	if committedSHA != modelSHA {
		return harness.AgentExecution{}, harness.AttemptProtocolError{Err: errors.New("official model.patch differs from the pre-verifier committed-workspace capture")}
	}
	submissionPath := filepath.Join(invocation.ArtifactDir, "submission.patch")
	patchRaw, err := os.ReadFile(modelPatch)
	if err != nil {
		return harness.AgentExecution{}, err
	}
	if err := harness.WriteBytesAtomic(submissionPath, patchRaw, 0o644); err != nil {
		return harness.AgentExecution{}, err
	}
	auditPath := filepath.Join(invocation.ArtifactDir, "audit-workspace.patch")
	auditRaw, err := os.ReadFile(fullPatch)
	if err != nil {
		return harness.AgentExecution{}, err
	}
	if err := harness.WriteBytesAtomic(auditPath, auditRaw, 0o644); err != nil {
		return harness.AgentExecution{}, err
	}
	capture, err := readCapture(filepath.Join(trialDir, "agent", "workspace-capture.json"))
	if err != nil {
		return harness.AgentExecution{}, err
	}
	if capture.PatchSHA256 != modelSHA || capture.AuditPatchSHA256 != fullSHA ||
		capture.UncommittedChangesPresent != (fullSHA != modelSHA) || capture.BaseCommit != invocation.Task.BaseCommit {
		return harness.AgentExecution{}, harness.AttemptProtocolError{Err: errors.New("workspace capture receipt does not bind both official and audit patches")}
	}
	parsed.Capture = &capture
	rawPierResult, err := os.ReadFile(filepath.Join(trialDir, "result.json"))
	if err != nil {
		return harness.AgentExecution{}, err
	}
	publicPierResult := redaction.redact(rawPierResult)
	if err := harness.WriteBytesAtomic(filepath.Join(pierDir, "trial-result.raw.json"), publicPierResult, 0o600); err != nil {
		return harness.AgentExecution{}, err
	}
	if err := harness.WriteJSONAtomic(filepath.Join(pierDir, "trial-result.json"), parsed, 0o644); err != nil {
		return harness.AgentExecution{}, err
	}
	if err := harness.WriteBytesAtomic(filepath.Join(pierDir, "submission.sha256"), []byte(modelSHA+"\n"), 0o644); err != nil {
		return harness.AgentExecution{}, err
	}
	providerEndpoint, err := backend.providerEndpointSnapshot()
	if err != nil {
		return harness.AgentExecution{}, err
	}
	providerEndpointBinding, err := validateProviderEndpointEvidence(rawEvidence, runIdentity, providerEndpoint)
	if err != nil {
		return harness.AgentExecution{}, harness.AttemptProtocolError{Err: err}
	}
	var canonicalization *serviceTierCanonicalizationRunBinding
	if invocation.Agent.ID == "codex" {
		canonicalCanary, err := backend.codexCanonicalCanarySnapshot()
		if err != nil {
			return harness.AgentExecution{}, err
		}
		canonicalizationReceipt, canonicalizationReceiptSHA, err := writeServiceTierCanonicalizationReceipt(
			pierDir, rawEvidence, runIdentity, effectiveReceiptSHA, sandboxCanarySHA,
			invocation, adapter, bundle, canonicalCanary, effective,
		)
		if err != nil {
			return harness.AgentExecution{}, harness.AttemptProtocolError{Err: err}
		}
		canonicalization = &serviceTierCanonicalizationRunBinding{Receipt: canonicalizationReceipt, ReceiptSHA256: canonicalizationReceiptSHA}
	}
	receipt, err := backend.makeRunReceipt(redactArgs(args), materializedSHA, invocation.Agent.BinarySHA256, &effective, &providerEndpointBinding, canonicalization)
	if err != nil {
		return harness.AgentExecution{}, err
	}
	if err := harness.WriteJSONAtomic(filepath.Join(pierDir, "run-receipt.json"), receipt, 0o644); err != nil {
		return harness.AgentExecution{}, err
	}
	evidencePath := filepath.Join(invocation.ArtifactDir, filepath.FromSlash(invocation.Agent.RequestEvidence.RelativePath))
	canonicalizationBindingSHA := ""
	if canonicalization != nil {
		canonicalizationBindingSHA = canonicalization.Receipt.BindingSHA256
	}
	if err := normalizeProviderEvidence(rawEvidence, filepath.Join(trialDir, "agent", "stream.jsonl"), evidencePath, invocation.Agent, runIdentity, canonicalizationBindingSHA); err != nil {
		return harness.AgentExecution{}, harness.AttemptProtocolError{Err: err}
	}
	seal, err := evidenceproxy.ValidateEvidenceSeal(rawEvidence, runIdentity)
	if err != nil {
		return harness.AgentExecution{}, harness.AttemptProtocolError{Err: fmt.Errorf("revalidate provider evidence seal: %w", err)}
	}
	journalPath := evidenceproxy.AttemptJournalPath(rawEvidence)
	sealPath := evidenceproxy.EvidenceSealPath(rawEvidence)
	rawEvidenceSHA, err := harness.HashFile(rawEvidence)
	if err != nil {
		return harness.AgentExecution{}, err
	}
	journalSHA, err := harness.HashFile(journalPath)
	if err != nil {
		return harness.AgentExecution{}, err
	}
	sealSHA, err := harness.HashFile(sealPath)
	if err != nil {
		return harness.AgentExecution{}, err
	}
	providerEvidence := harness.ProviderEvidenceSeal{
		RawEvidencePath: rawEvidence, AttemptJournalPath: journalPath, SealPath: sealPath,
		RawEvidenceSHA256: rawEvidenceSHA, AttemptJournalSHA256: journalSHA, SealSHA256: sealSHA,
		StartedAttemptCount: seal.StartedAttemptCount, PersistedAttemptCount: seal.PersistedAttemptCount,
		RecordCount: seal.RecordCount, LastEvidenceHash: seal.LastEvidenceHash,
	}
	providerContextEvidence, providerContextFound, err := sealedProviderContextTerminal(rawEvidence, runIdentity)
	if err != nil {
		return harness.AgentExecution{}, harness.AttemptProtocolError{Err: err}
	}
	exitPath := filepath.Join(trialDir, "agent", "exit.json")
	exitCode, exitReceiptRaw, err := readExitReceipt(exitPath)
	if err != nil {
		return harness.AgentExecution{}, harness.AttemptProtocolError{Err: err}
	}
	var terminalEvidence harness.AgentTerminalEvidence
	exitClass := ""
	if parsed.ExceptionType == "AgentTimeoutError" {
		if exitCode != 124 {
			return harness.AgentExecution{}, harness.AttemptProtocolError{Err: errors.New("Pier agent timeout disagrees with the adapter exit receipt")}
		}
		exitClass, exitCode = "timeout", 124
		terminalEvidence = harness.AgentTerminalEvidence{
			SchemaVersion: "agentic-bench/terminal-evidence-v1",
			Source:        "pier_trial", Code: "agent_timeout", EvidenceSHA256: sha256Hex(publicPierResult),
		}
	} else if invocation.Agent.ID == "codex" && providerContextFound {
		if exitCode == 0 {
			return harness.AgentExecution{}, harness.AttemptProtocolError{Err: errors.New("sealed provider context failure conflicts with a zero Codex exit")}
		}
		rawStreamPath := filepath.Join(trialDir, "agent", "stream.jsonl")
		rawStream, err := os.ReadFile(rawStreamPath)
		if err != nil {
			return harness.AgentExecution{}, err
		}
		publicStream, err := os.ReadFile(filepath.Join(pierDir, "agent-stream.jsonl"))
		if err != nil {
			return harness.AgentExecution{}, err
		}
		if err := validateCodexProxyContextStream(rawStream, exitCode); err != nil {
			return harness.AgentExecution{}, harness.AttemptProtocolError{Err: err}
		}
		if err := validateCodexProxyContextStream(publicStream, exitCode); err != nil {
			return harness.AgentExecution{}, harness.AttemptProtocolError{Err: fmt.Errorf("public redaction changed Codex terminal semantics: %w", err)}
		}
		if terminalReceiptExists {
			adapterEvidence, err := readAndValidateTerminalEvidence(terminalReceiptPath, rawStreamPath, exitReceiptRaw, exitCode, "codex")
			if err != nil {
				return harness.AgentExecution{}, harness.AttemptProtocolError{Err: err}
			}
			if adapterEvidence.Source != "provider_event" || adapterEvidence.Code != "context_length_exceeded" {
				return harness.AgentExecution{}, harness.AttemptProtocolError{Err: errors.New("adapter and sealed provider context evidence disagree")}
			}
		}
		terminalEvidence = providerContextEvidence
		exitClass = "context_failure"
	} else {
		if !terminalReceiptExists {
			return harness.AgentExecution{}, harness.AttemptProtocolError{Err: errors.New("adapter terminal evidence is missing without a sealed Codex provider context failure")}
		}
		rawTerminalEvidence, err := readAndValidateTerminalEvidence(
			terminalReceiptPath,
			filepath.Join(trialDir, "agent", "stream.jsonl"), exitReceiptRaw, exitCode, invocation.Agent.ID,
		)
		if err != nil {
			return harness.AgentExecution{}, harness.AttemptProtocolError{Err: err}
		}
		publicStreamRaw, err := os.ReadFile(filepath.Join(pierDir, "agent-stream.jsonl"))
		if err != nil {
			return harness.AgentExecution{}, err
		}
		terminalEvidence, err = deriveTerminalEvidence(invocation.Agent.ID, publicStreamRaw, exitReceiptRaw, exitCode)
		if err != nil {
			return harness.AgentExecution{}, harness.AttemptProtocolError{Err: err}
		}
		if terminalEvidence.SchemaVersion != rawTerminalEvidence.SchemaVersion || terminalEvidence.Source != rawTerminalEvidence.Source || terminalEvidence.Code != rawTerminalEvidence.Code {
			return harness.AgentExecution{}, harness.AttemptProtocolError{Err: errors.New("public redaction changed structured terminal semantics")}
		}
		switch {
		case terminalEvidence.Source == "provider_event" && terminalEvidence.Code == "context_length_exceeded":
			exitClass = "context_failure"
		case terminalEvidence.Source == "process_exit" && terminalEvidence.Code == "completed":
			exitClass = "completed"
		case terminalEvidence.Source == "process_exit" && terminalEvidence.Code == "nonzero_exit":
			exitClass = "nonzero"
		default:
			return harness.AgentExecution{}, harness.AttemptProtocolError{Err: errors.New("adapter terminal evidence has no sealed execution mapping")}
		}
	}
	if err := harness.WriteJSONAtomic(filepath.Join(pierDir, "agent-terminal-evidence.json"), terminalEvidence, 0o600); err != nil {
		return harness.AgentExecution{}, err
	}
	if (exitClass == "nonzero" || exitClass == "context_failure") && parsed.ExceptionType != "NonZeroAgentExitCodeError" {
		return harness.AgentExecution{}, harness.AttemptProtocolError{Err: errors.New("nonzero agent exit disagrees with the structured Pier exception")}
	}
	if exitClass == "completed" && parsed.ExceptionType == "NonZeroAgentExitCodeError" {
		return harness.AgentExecution{}, harness.AttemptProtocolError{Err: errors.New("completed agent exit disagrees with the structured Pier exception")}
	}
	if parsed.AgentStartedAt.IsZero() || parsed.AgentFinishedAt.IsZero() || parsed.AgentFinishedAt.Before(parsed.AgentStartedAt) {
		return harness.AgentExecution{}, harness.AttemptProtocolError{Err: errors.New("Pier result lacks valid agent-only timing")}
	}
	if err := redaction.scanAndWriteReceipt(invocation.ArtifactDir, filepath.Join(pierDir, "public-secret-scan.json")); err != nil {
		return harness.AgentExecution{}, harness.AttemptProtocolError{Err: err}
	}
	canonicalizationEvidence := harness.ServiceTierCanonicalizationEvidence{}
	if canonicalization != nil {
		canonicalizationEvidence = harness.ServiceTierCanonicalizationEvidence{
			SchemaVersion:                harness.ServiceTierCanonicalizationEvidenceSchemaVersion,
			Representation:               canonicalization.Receipt.Representation,
			ReceiptRelativePath:          filepath.ToSlash(filepath.Join("pier", serviceTierCanonicalizationReceiptName)),
			ReceiptSHA256:                canonicalization.ReceiptSHA256,
			BindingSHA256:                canonicalization.Receipt.BindingSHA256,
			StaticProofSHA256:            canonicalization.Receipt.StaticProofSHA256,
			TransformationEvidenceSHA256: canonicalization.Receipt.TransformationEvidenceSHA256,
			TransformedRoundCount:        uint64(canonicalization.Receipt.TransformedProviderRoundCount),
		}
	}
	return harness.AgentExecution{
		ExitClass: exitClass, ExitCode: exitCode, StartedAt: parsed.AgentStartedAt, FinishedAt: parsed.AgentFinishedAt,
		TrialStartedAt: parsed.StartedAt, TrialFinishedAt: parsed.FinishedAt,
		SubmissionPatch: submissionPath, AuditWorkspacePatch: auditPath, Capture: capture, EvidencePath: evidencePath,
		EvidenceRunIdentity: runIdentity, ProviderEvidence: providerEvidence, ServiceTierCanonicalization: canonicalizationEvidence,
		TerminalEvidence: terminalEvidence, Verification: verification,
	}, nil
}

type sealedAgentAttempt struct {
	SchemaVersion   string                         `json:"schema_version"`
	PairID          string                         `json:"pair_id"`
	TaskID          string                         `json:"task_id"`
	AgentID         string                         `json:"agent_id"`
	Repetition      int                            `json:"repetition"`
	Execution       harness.AgentExecution         `json:"execution"`
	FailureCategory harness.DeepSWEFailureCategory `json:"failure_category"`
	Failure         string                         `json:"failure,omitempty"`
}

func sealAgentAttempt(invocation harness.AgentInvocation, execution harness.AgentExecution, category harness.DeepSWEFailureCategory, failure string) error {
	return harness.WriteJSONAtomic(filepath.Join(invocation.ArtifactDir, "sealed-attempt.json"), sealedAgentAttempt{
		SchemaVersion: "agentic-bench/sealed-attempt-v1", PairID: invocation.PlanEntry.PairID,
		TaskID: invocation.Task.ID, AgentID: invocation.Agent.ID, Repetition: invocation.PlanEntry.Repetition,
		Execution: execution, FailureCategory: category, Failure: failure,
	}, 0o600)
}

func classifyTrialInfrastructure(rawEvidence string, parsed sanitizedTrialResult) (harness.DeepSWEFailureCategory, bool) {
	// A verifier that physically started but produced no reward is an evaluator
	// infrastructure gap. No exception text is interpreted.
	if !parsed.VerifierStarted.IsZero() {
		return harness.DeepSWEFailureVerifierInfrastructure, true
	}
	records, err := harness.ReadJSONLines[evidenceproxy.Record](rawEvidence)
	if err != nil || len(records) == 0 {
		return "", false
	}
	slices.SortFunc(records, func(left, right evidenceproxy.Record) int { return cmp.Compare(left.Round, right.Round) })
	terminal := records[len(records)-1]
	switch terminal.ErrorCode {
	case "upstream_transport", "upstream_read", "provider_usage_receipt_incomplete", "incomplete_server_evidence":
		return harness.DeepSWEFailureProviderInfrastructure, true
	case "downstream_write":
		return harness.DeepSWEFailureNetworkInfrastructure, true
	case "provider_http_error":
		if terminal.HTTPStatus == 408 || terminal.HTTPStatus == 429 || terminal.HTTPStatus >= 500 {
			return harness.DeepSWEFailureProviderInfrastructure, true
		}
	}
	return "", false
}

type canaryUsage struct {
	InputTokens           int64 `json:"input_tokens"`
	CachedInputTokens     int64 `json:"cached_input_tokens"`
	CacheWriteInputTokens int64 `json:"cache_write_input_tokens"`
	OutputTokens          int64 `json:"output_tokens"`
	ReasoningOutputTokens int64 `json:"reasoning_output_tokens"`
}

func (usage *canaryUsage) UnmarshalJSON(raw []byte) error {
	type wire canaryUsage
	decoded, err := strictDecodeCanaryObject[wire](raw, "canary usage", "input_tokens", "cached_input_tokens", "cache_write_input_tokens", "output_tokens", "reasoning_output_tokens")
	if err != nil {
		return err
	}
	*usage = canaryUsage(decoded)
	return nil
}

type canaryTool struct {
	Type string  `json:"type"`
	Name *string `json:"name"`
}

func (tool *canaryTool) UnmarshalJSON(raw []byte) error {
	type wire canaryTool
	decoded, err := strictDecodeCanaryObject[wire](raw, "canary tool", "type", "name")
	if err != nil {
		return err
	}
	*tool = canaryTool(decoded)
	return nil
}

type codexLiteCanaryRequest struct {
	RequestIndex                       int          `json:"request_index"`
	Model                              string       `json:"model"`
	Store                              *bool        `json:"store"`
	ReasoningEffort                    string       `json:"reasoning_effort"`
	ReasoningContext                   string       `json:"reasoning_context"`
	IncludeEncryptedReasoning          bool         `json:"include_encrypted_reasoning"`
	Stream                             bool         `json:"stream"`
	RequestServiceTierPresent          bool         `json:"request_service_tier_present"`
	RequestServiceTier                 *string      `json:"request_service_tier"`
	RequestServiceTierCanonical        string       `json:"request_service_tier_canonical"`
	RequestServiceTierSource           string       `json:"request_service_tier_source"`
	TopLevelToolCount                  int          `json:"top_level_tool_count"`
	ToolCatalog                        []canaryTool `json:"tool_catalog"`
	WebSearchToolPresent               bool         `json:"web_search_tool_present"`
	WebSearchToolCount                 int          `json:"web_search_tool_count"`
	CollaborationNamespacePresent      bool         `json:"collaboration_namespace_present"`
	SubagentToolPresent                bool         `json:"subagent_tool_present"`
	ExecCellWaitPresent                bool         `json:"exec_cell_wait_present"`
	WebSocketUpgradeCountBeforeRequest int          `json:"websocket_upgrade_count_before_request"`
	WebSocketUpgradeHeaderPresent      bool         `json:"websocket_upgrade_header_present"`
	WebSocketKeyHeaderPresent          bool         `json:"websocket_key_header_present"`
	ResponsesLiteHeaderPresent         bool         `json:"responses_lite_header_present"`
	AuthorizationHeaderPresent         bool         `json:"authorization_header_present"`
	Originator                         string       `json:"originator"`
	UserAgentPresent                   bool         `json:"user_agent_present"`
	PreviousResponseIDPresent          bool         `json:"previous_response_id_present"`
	CustomToolOutputCount              int          `json:"custom_tool_output_count"`
	ToolOutputExitCode                 *int         `json:"tool_output_exit_code"`
	ResponseModel                      string       `json:"response_model"`
	ResponseServiceTier                string       `json:"response_service_tier"`
	ResponseServiceTierCanonical       string       `json:"response_service_tier_canonical"`
	ResponseRequestIDPresent           bool         `json:"response_request_id_present"`
	ResponseUsage                      canaryUsage  `json:"response_usage"`
}

type lubanCanaryRequest struct {
	RequestIndex                 int             `json:"request_index"`
	Model                        string          `json:"model"`
	Store                        *bool           `json:"store"`
	ReasoningEffort              string          `json:"reasoning_effort"`
	ReasoningContext             string          `json:"reasoning_context"`
	RequestServiceTierPresent    bool            `json:"request_service_tier_present"`
	RequestServiceTier           *string         `json:"request_service_tier"`
	RequestServiceTierCanonical  string          `json:"request_service_tier_canonical"`
	RequestServiceTierSource     string          `json:"request_service_tier_source"`
	ToolNames                    []string        `json:"tool_names"`
	ResponsesLiteHeader          json.RawMessage `json:"responses_lite_header"`
	AdditionalToolsPrefixes      int             `json:"additional_tools_prefixes"`
	PreviousResponseIDPresent    bool            `json:"previous_response_id_present"`
	ResponseModel                string          `json:"response_model"`
	ResponseServiceTier          string          `json:"response_service_tier"`
	ResponseServiceTierCanonical string          `json:"response_service_tier_canonical"`
	ResponseRequestIDPresent     bool            `json:"response_request_id_present"`
}

type codexStandardCanaryRequest struct {
	RequestIndex                  int          `json:"request_index"`
	Model                         string       `json:"model"`
	Store                         *bool        `json:"store"`
	ReasoningEffort               string       `json:"reasoning_effort"`
	IncludeEncryptedReasoning     bool         `json:"include_encrypted_reasoning"`
	Stream                        bool         `json:"stream"`
	ResponsesLiteHeaderPresent    bool         `json:"responses_lite_header_present"`
	AuthorizationHeaderPresent    bool         `json:"authorization_header_present"`
	Originator                    string       `json:"originator"`
	RequestServiceTierPresent     bool         `json:"request_service_tier_present"`
	RequestServiceTier            *string      `json:"request_service_tier"`
	RequestServiceTierCanonical   string       `json:"request_service_tier_canonical"`
	RequestServiceTierSource      string       `json:"request_service_tier_source"`
	OrderedToolCatalog            []canaryTool `json:"ordered_tool_catalog"`
	WebSearchToolCount            int          `json:"web_search_tool_count"`
	WebSearchExternalAccess       []bool       `json:"web_search_external_access"`
	CollaborationNamespacePresent bool         `json:"collaboration_namespace_present"`
	MultiAgentNamespacePresent    bool         `json:"multi_agent_namespace_present"`
	SubagentToolPresent           bool         `json:"subagent_tool_present"`
	ConfigurationAccepted         bool         `json:"configuration_accepted"`
	ResponseModel                 string       `json:"response_model"`
	ResponseServiceTier           string       `json:"response_service_tier"`
	ResponseServiceTierCanonical  string       `json:"response_service_tier_canonical"`
	ResponseRequestIDPresent      bool         `json:"response_request_id_present"`
	ResponseUsage                 canaryUsage  `json:"response_usage"`
}

type sandboxNegativeControl struct {
	SchemaVersion              string            `json:"schema_version"`
	SandboxPolicy              string            `json:"sandbox_policy"`
	ExpectedToolExitCode       int               `json:"expected_tool_exit_code"`
	MarkerWritten              bool              `json:"marker_written"`
	ValidSandboxReceiptEmitted bool              `json:"valid_sandbox_receipt_emitted"`
	ProviderCanaryRequests     []json.RawMessage `json:"provider_canary_requests"`
}

type websocketFallbackCanary struct {
	WebSocketUpgradeRequestCount   int    `json:"websocket_upgrade_request_count"`
	WebSocketUpgradeResponseStatus int    `json:"websocket_upgrade_response_status"`
	WebSocketGenerationPayloadSent bool   `json:"websocket_generation_payload_sent"`
	HTTPGenerationRequestCount     int    `json:"http_generation_request_count"`
	ExpectedLogicalGenerationCount int    `json:"expected_logical_generation_count"`
	DuplicateGenerationDetected    bool   `json:"duplicate_generation_detected"`
	FallbackTransport              string `json:"fallback_transport"`
}

type sandboxWorkspaceState struct {
	SchemaVersion               string `json:"schema_version"`
	Head                        string `json:"head"`
	ExpectedBaseCommit          string `json:"expected_base_commit"`
	HeadMatchesBaseCommit       bool   `json:"head_matches_base_commit"`
	IndexEntriesSHA256          string `json:"index_entries_sha256"`
	IndexMatchesHead            bool   `json:"index_matches_head"`
	TrackedWorktreeMatchesIndex bool   `json:"tracked_worktree_matches_index"`
	StatusPorcelainV1ZSHA256    string `json:"status_porcelain_v1_z_sha256"`
	StatusEntryCount            int    `json:"status_entry_count"`
	PositiveMarkerAbsent        bool   `json:"positive_marker_absent"`
	NegativeMarkerAbsent        bool   `json:"negative_marker_absent"`
}

type webSearchPositiveControl struct {
	EffectiveArgvSHA256 string          `json:"effective_argv_sha256"`
	ExpectedCLIExitCode int             `json:"expected_cli_exit_code"`
	ActualCLIExitCode   int             `json:"actual_cli_exit_code"`
	ValidReceiptEmitted bool            `json:"valid_receipt_emitted"`
	Request             json.RawMessage `json:"request"`
}

type webSearchNegativeControl struct {
	ConfigRemoved            string          `json:"config_removed"`
	OnlyRemovedConfig        bool            `json:"only_removed_config"`
	EffectiveArgvSHA256      string          `json:"effective_argv_sha256"`
	CounterfactualArgvSHA256 string          `json:"counterfactual_argv_sha256"`
	ExpectedCLIExitCode      string          `json:"expected_cli_exit_code"`
	ActualCLIExitCode        int             `json:"actual_cli_exit_code"`
	ValidReceiptEmitted      bool            `json:"valid_receipt_emitted"`
	Request                  json.RawMessage `json:"request"`
}

type serviceTierNegativeControl struct {
	ConfigReplaced           string          `json:"config_replaced"`
	ReplacementConfig        string          `json:"replacement_config"`
	OnlyReplacedConfigValue  bool            `json:"only_replaced_config_value"`
	EffectiveArgvSHA256      string          `json:"effective_argv_sha256"`
	CounterfactualArgvSHA256 string          `json:"counterfactual_argv_sha256"`
	ExpectedCLIExitCode      string          `json:"expected_cli_exit_code"`
	ActualCLIExitCode        int             `json:"actual_cli_exit_code"`
	ValidReceiptEmitted      bool            `json:"valid_receipt_emitted"`
	Request                  json.RawMessage `json:"request"`
}

type webSearchConfigurationCanary struct {
	SchemaVersion                  string          `json:"schema_version"`
	ProviderTransport              string          `json:"provider_transport"`
	Model                          string          `json:"model"`
	ReasoningEffort                string          `json:"reasoning_effort"`
	EffectiveConfig                string          `json:"effective_config"`
	AgentsEffectiveConfig          string          `json:"agents_effective_config"`
	ServiceTierEffectiveConfig     string          `json:"service_tier_effective_config"`
	ServiceTierDefaultWireEncoding string          `json:"service_tier_default_wire_encoding"`
	ServiceTierDefaultSource       string          `json:"service_tier_default_source"`
	ModelCatalogSHA256             string          `json:"model_catalog_sha256"`
	Positive                       json.RawMessage `json:"positive"`
	NegativeControl                json.RawMessage `json:"negative_control"`
	AgentsNegativeControl          json.RawMessage `json:"agents_negative_control"`
	ServiceTierNegativeControl     json.RawMessage `json:"service_tier_negative_control"`
}

type sandboxCanaryReceipt struct {
	SchemaVersion                string            `json:"schema_version"`
	AgentKind                    string            `json:"agent_kind"`
	BinarySHA256                 string            `json:"binary_sha256"`
	BaseCommit                   string            `json:"base_commit"`
	AdapterSHA256                string            `json:"adapter_sha256"`
	BundleManifestSHA256         string            `json:"bundle_manifest_sha256"`
	EffectiveArgvReceiptSHA256   string            `json:"effective_argv_receipt_sha256"`
	ControllerProxyReachable     bool              `json:"controller_proxy_reachable"`
	ToolProxyReachable           bool              `json:"tool_proxy_reachable"`
	CredentialInAgent            bool              `json:"credential_in_agent"`
	SourceBundleTreeSHA256       string            `json:"source_bundle_tree_sha256"`
	RuntimePayloadTreeSHA256     string            `json:"runtime_payload_tree_sha256"`
	ProviderCanaryRequests       []json.RawMessage `json:"provider_canary_requests"`
	ProviderCanaryTransport      string            `json:"provider_canary_transport,omitempty"`
	WebSocketFallback            json.RawMessage   `json:"websocket_fallback,omitempty"`
	SandboxNegativeControl       json.RawMessage   `json:"sandbox_negative_control,omitempty"`
	WebSearchConfigurationCanary json.RawMessage   `json:"web_search_configuration_canary,omitempty"`
	WorkspaceState               json.RawMessage   `json:"workspace_state,omitempty"`
}

func validateSandboxCanary(path string, invocation harness.AgentInvocation, adapter adapterBinding, bundle codexBundleBinding, effectiveReceiptSHA string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return validateSandboxCanaryReceipt(raw, invocation, adapter, bundle, effectiveReceiptSHA)
}

func strictDecodeCanaryObject[T any](raw []byte, label string, expectedFields ...string) (T, error) {
	var zero T
	if err := validateStrictJSON(raw); err != nil {
		return zero, fmt.Errorf("%s is not strict JSON: %w", label, err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return zero, fmt.Errorf("%s is not a JSON object", label)
	}
	wanted := make(map[string]struct{}, len(expectedFields))
	for _, field := range expectedFields {
		wanted[field] = struct{}{}
	}
	if len(fields) != len(wanted) {
		return zero, fmt.Errorf("%s has unexpected or missing fields", label)
	}
	for field := range fields {
		if _, ok := wanted[field]; !ok {
			return zero, fmt.Errorf("%s has unexpected field %q", label, field)
		}
	}
	var value T
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return zero, fmt.Errorf("decode %s: %w", label, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return zero, fmt.Errorf("%s contains trailing JSON", label)
	}
	return value, nil
}

func validateSandboxCanaryReceipt(raw []byte, invocation harness.AgentInvocation, adapter adapterBinding, bundle codexBundleBinding, effectiveReceiptSHA string) error {
	commonFields := []string{
		"schema_version", "agent_kind", "binary_sha256", "base_commit", "controller_proxy_reachable",
		"tool_proxy_reachable", "credential_in_agent", "adapter_sha256", "bundle_manifest_sha256",
		"effective_argv_receipt_sha256", "source_bundle_tree_sha256", "runtime_payload_tree_sha256",
		"provider_canary_requests",
	}
	expectedRuntimeTree := CodexBundleTreeSHA256
	expectedTierEncoding := harness.ServiceTierEncodingExplicitDefault
	switch invocation.Agent.ID {
	case "codex":
		expectedTierEncoding = harness.ServiceTierEncodingClientCanonical
		commonFields = append(commonFields, "provider_canary_transport", "websocket_fallback", "sandbox_negative_control", "web_search_configuration_canary", "workspace_state")
	case "luban":
		expectedRuntimeTree = LubanRuntimeTreeSHA256
	default:
		return fmt.Errorf("sandbox canary has unsupported agent kind %q", invocation.Agent.ID)
	}
	receipt, err := strictDecodeCanaryObject[sandboxCanaryReceipt](raw, "sandbox canary", commonFields...)
	if err != nil {
		return err
	}
	if receipt.SchemaVersion != "agentic-bench/sandbox-canary-v3" || receipt.AgentKind != invocation.Agent.ID ||
		receipt.BinarySHA256 != invocation.Agent.BinarySHA256 || receipt.BaseCommit != invocation.Task.BaseCommit ||
		receipt.AdapterSHA256 != adapter.SHA256 || receipt.BundleManifestSHA256 != bundle.ManifestSHA256 ||
		receipt.EffectiveArgvReceiptSHA256 != effectiveReceiptSHA || !receipt.ControllerProxyReachable ||
		receipt.ToolProxyReachable || receipt.CredentialInAgent || receipt.SourceBundleTreeSHA256 != CodexBundleTreeSHA256 ||
		receipt.RuntimePayloadTreeSHA256 != expectedRuntimeTree || invocation.Agent.Model.ServiceTier != "default" ||
		invocation.Agent.Model.ServiceTierRequestEncoding != expectedTierEncoding {
		return errors.New("sandbox canary does not prove controller-only provider access and the frozen model contract")
	}
	if invocation.Agent.ID == "luban" {
		return validateLubanCanaryRequests(receipt.ProviderCanaryRequests, invocation.Agent.Model)
	}
	if receipt.ProviderCanaryTransport != "responses-lite-websocket-426-http-sse-diagnostic" {
		return errors.New("Codex sandbox canary has an unsupported provider transport")
	}
	if err := validateCodexLiteCanaryRequests(receipt.ProviderCanaryRequests, invocation.Agent.Model, 0); err != nil {
		return fmt.Errorf("validate Codex positive sandbox canary: %w", err)
	}
	fallback, err := strictDecodeCanaryObject[websocketFallbackCanary](
		receipt.WebSocketFallback, "Codex WebSocket fallback canary",
		"websocket_upgrade_request_count", "websocket_upgrade_response_status", "websocket_generation_payload_sent",
		"http_generation_request_count", "expected_logical_generation_count", "duplicate_generation_detected", "fallback_transport",
	)
	if err != nil {
		return err
	}
	if fallback.WebSocketUpgradeRequestCount != 1 || fallback.WebSocketUpgradeResponseStatus != http.StatusUpgradeRequired ||
		fallback.WebSocketGenerationPayloadSent || fallback.HTTPGenerationRequestCount != 2 ||
		fallback.ExpectedLogicalGenerationCount != 2 || fallback.DuplicateGenerationDetected || fallback.FallbackTransport != "http-sse" {
		return errors.New("Codex sandbox canary does not prove one safe WebSocket-to-HTTP fallback")
	}
	negative, err := strictDecodeCanaryObject[sandboxNegativeControl](
		receipt.SandboxNegativeControl, "Codex sandbox negative control",
		"schema_version", "sandbox_policy", "expected_tool_exit_code", "marker_written", "valid_sandbox_receipt_emitted", "provider_canary_requests",
	)
	if err != nil {
		return err
	}
	if negative.SchemaVersion != "agentic-bench/sandbox-negative-control-v1" || negative.SandboxPolicy != "danger-full-access" ||
		negative.ExpectedToolExitCode != 91 || negative.MarkerWritten || negative.ValidSandboxReceiptEmitted {
		return errors.New("Codex sandbox negative control has invalid execution semantics")
	}
	if err := validateCodexLiteCanaryRequests(negative.ProviderCanaryRequests, invocation.Agent.Model, 91); err != nil {
		return fmt.Errorf("validate Codex sandbox negative-control requests: %w", err)
	}
	workspace, err := strictDecodeCanaryObject[sandboxWorkspaceState](
		receipt.WorkspaceState, "Codex canary workspace state",
		"schema_version", "head", "expected_base_commit", "head_matches_base_commit", "index_entries_sha256", "index_matches_head",
		"tracked_worktree_matches_index", "status_porcelain_v1_z_sha256", "status_entry_count", "positive_marker_absent", "negative_marker_absent",
	)
	if err != nil {
		return err
	}
	if workspace.SchemaVersion != "agentic-bench/sandbox-workspace-state-v1" || workspace.Head != invocation.Task.BaseCommit ||
		workspace.ExpectedBaseCommit != invocation.Task.BaseCommit || !workspace.HeadMatchesBaseCommit || !lowerHexSHA256(workspace.IndexEntriesSHA256) ||
		!workspace.IndexMatchesHead || !workspace.TrackedWorktreeMatchesIndex || workspace.StatusPorcelainV1ZSHA256 != sha256Hex(nil) ||
		workspace.StatusEntryCount != 0 || !workspace.PositiveMarkerAbsent || !workspace.NegativeMarkerAbsent {
		return errors.New("Codex canaries did not preserve the frozen workspace")
	}
	return validateCodexWebSearchCanary(receipt.WebSearchConfigurationCanary, invocation.Agent.Model)
}

func validateCodexLiteCanaryRequests(rawRequests []json.RawMessage, model harness.ModelRequestSpec, expectedToolExit int) error {
	if len(rawRequests) != 2 {
		return errors.New("Codex Responses Lite canary must contain exactly two provider requests")
	}
	wantUsage := canaryUsage{InputTokens: 11, CachedInputTokens: 3, CacheWriteInputTokens: 2, OutputTokens: 5, ReasoningOutputTokens: 1}
	wantTools := []struct{ kind, name string }{{"custom", "exec"}, {"function", "wait"}, {"function", "request_user_input"}}
	fields := []string{
		"request_index", "model", "store", "reasoning_effort", "reasoning_context", "include_encrypted_reasoning", "stream",
		"request_service_tier_present", "request_service_tier", "request_service_tier_canonical", "request_service_tier_source", "top_level_tool_count", "tool_catalog", "web_search_tool_present",
		"web_search_tool_count", "collaboration_namespace_present", "subagent_tool_present", "exec_cell_wait_present",
		"websocket_upgrade_count_before_request", "websocket_upgrade_header_present", "websocket_key_header_present",
		"responses_lite_header_present", "authorization_header_present", "originator", "user_agent_present",
		"previous_response_id_present", "custom_tool_output_count", "tool_output_exit_code", "response_model",
		"response_service_tier", "response_service_tier_canonical", "response_request_id_present", "response_usage",
	}
	for index, raw := range rawRequests {
		request, err := strictDecodeCanaryObject[codexLiteCanaryRequest](raw, fmt.Sprintf("Codex Responses Lite canary request %d", index), fields...)
		if err != nil {
			return err
		}
		if request.RequestIndex != index || request.Model != model.Model || request.Store == nil || *request.Store ||
			request.ReasoningEffort != model.ReasoningEffort || request.ReasoningContext != "all_turns" || !request.IncludeEncryptedReasoning || !request.Stream ||
			request.RequestServiceTierPresent || request.RequestServiceTier != nil || request.RequestServiceTierCanonical != "default" || request.RequestServiceTierSource != "client_canonicalized_default" || request.TopLevelToolCount != 0 ||
			request.WebSearchToolPresent || request.WebSearchToolCount != 0 || request.CollaborationNamespacePresent || request.SubagentToolPresent ||
			!request.ExecCellWaitPresent || request.WebSocketUpgradeCountBeforeRequest != 1 || !request.WebSocketUpgradeHeaderPresent || !request.WebSocketKeyHeaderPresent ||
			!request.ResponsesLiteHeaderPresent || !request.AuthorizationHeaderPresent || request.Originator != "codex_exec" || !request.UserAgentPresent ||
			request.PreviousResponseIDPresent || request.ResponseModel != model.Model || request.ResponseServiceTier != "default" || request.ResponseServiceTierCanonical != "default" ||
			!request.ResponseRequestIDPresent || request.ResponseUsage != wantUsage || len(request.ToolCatalog) != len(wantTools) {
			return fmt.Errorf("Codex Responses Lite canary request %d violates the frozen wire contract", index)
		}
		for toolIndex, want := range wantTools {
			got := request.ToolCatalog[toolIndex]
			if got.Name == nil || got.Type != want.kind || *got.Name != want.name {
				return fmt.Errorf("Codex Responses Lite canary request %d changed the exec/wait tool distinction", index)
			}
		}
		if index == 0 {
			if request.CustomToolOutputCount != 0 || request.ToolOutputExitCode != nil {
				return errors.New("first Codex Responses Lite request already contains a tool result")
			}
		} else if request.CustomToolOutputCount != 1 || request.ToolOutputExitCode == nil || *request.ToolOutputExitCode != expectedToolExit {
			return errors.New("second Codex Responses Lite request is not bound to the expected exec result")
		}
	}
	return nil
}

func validateLubanCanaryRequests(rawRequests []json.RawMessage, model harness.ModelRequestSpec) error {
	if len(rawRequests) != 2 {
		return errors.New("Luban canary must contain exactly two provider requests")
	}
	fields := []string{
		"request_index", "model", "store", "reasoning_effort", "reasoning_context", "request_service_tier_present", "request_service_tier", "request_service_tier_canonical", "request_service_tier_source",
		"tool_names", "responses_lite_header", "additional_tools_prefixes", "previous_response_id_present", "response_model",
		"response_service_tier", "response_service_tier_canonical", "response_request_id_present",
	}
	for index, raw := range rawRequests {
		request, err := strictDecodeCanaryObject[lubanCanaryRequest](raw, fmt.Sprintf("Luban canary request %d", index), fields...)
		if err != nil {
			return err
		}
		if request.RequestIndex != index || request.Model != model.Model || request.Store == nil || *request.Store || request.ReasoningEffort != model.ReasoningEffort ||
			(request.ReasoningContext != "" && request.ReasoningContext != "all_turns") || !request.RequestServiceTierPresent || request.RequestServiceTier == nil || *request.RequestServiceTier != "default" || request.RequestServiceTierCanonical != "default" || request.RequestServiceTierSource != "wire_explicit_default" ||
			!slices.Equal(request.ToolNames, []string{"ApplyPatch", "Inspect", "Run"}) || string(request.ResponsesLiteHeader) != "null" ||
			request.AdditionalToolsPrefixes != 0 || request.PreviousResponseIDPresent || request.ResponseModel != model.Model ||
			request.ResponseServiceTier != "default" || request.ResponseServiceTierCanonical != "default" || !request.ResponseRequestIDPresent {
			return fmt.Errorf("Luban canary request %d violates the frozen public Responses contract", index)
		}
	}
	return nil
}

func validateCodexWebSearchCanary(raw json.RawMessage, model harness.ModelRequestSpec) error {
	web, err := strictDecodeCanaryObject[webSearchConfigurationCanary](
		raw, "Codex web-search configuration canary",
		"schema_version", "provider_transport", "model", "reasoning_effort", "effective_config", "agents_effective_config",
		"service_tier_effective_config", "service_tier_default_wire_encoding", "service_tier_default_source",
		"model_catalog_sha256", "positive", "negative_control", "agents_negative_control", "service_tier_negative_control",
	)
	if err != nil {
		return err
	}
	if web.SchemaVersion != "agentic-bench/fairness-configuration-canary-v2" || web.ProviderTransport != "responses-http-sse-standard-diagnostic" ||
		web.Model != model.Model || web.ReasoningEffort != model.ReasoningEffort || web.EffectiveConfig != `web_search="disabled"` ||
		web.AgentsEffectiveConfig != "agents.enabled=false" || web.ServiceTierEffectiveConfig != `service_tier="default"` ||
		web.ServiceTierDefaultWireEncoding != "omitted" || web.ServiceTierDefaultSource != "client_canonicalized_default" || !lowerHexSHA256(web.ModelCatalogSHA256) {
		return errors.New("Codex web-search canary metadata does not bind the frozen configuration")
	}
	positive, err := strictDecodeCanaryObject[webSearchPositiveControl](
		web.Positive, "Codex web-search positive control", "effective_argv_sha256", "expected_cli_exit_code", "actual_cli_exit_code", "valid_receipt_emitted", "request",
	)
	if err != nil {
		return err
	}
	negative, err := strictDecodeCanaryObject[webSearchNegativeControl](
		web.NegativeControl, "Codex web-search negative control", "config_removed", "only_removed_config", "effective_argv_sha256", "counterfactual_argv_sha256", "expected_cli_exit_code", "actual_cli_exit_code", "valid_receipt_emitted", "request",
	)
	if err != nil {
		return err
	}
	agentsNegative, err := strictDecodeCanaryObject[webSearchNegativeControl](
		web.AgentsNegativeControl, "Codex agents negative control", "config_removed", "only_removed_config", "effective_argv_sha256", "counterfactual_argv_sha256", "expected_cli_exit_code", "actual_cli_exit_code", "valid_receipt_emitted", "request",
	)
	if err != nil {
		return err
	}
	serviceTierNegative, err := strictDecodeCanaryObject[serviceTierNegativeControl](
		web.ServiceTierNegativeControl, "Codex web-search negative control", "config_replaced", "replacement_config", "only_replaced_config_value",
		"effective_argv_sha256", "counterfactual_argv_sha256", "expected_cli_exit_code", "actual_cli_exit_code", "valid_receipt_emitted", "request",
	)
	if err != nil {
		return err
	}
	if !lowerHexSHA256(positive.EffectiveArgvSHA256) || positive.ExpectedCLIExitCode != 0 || positive.ActualCLIExitCode != 0 || !positive.ValidReceiptEmitted {
		return errors.New("Codex web-search positive control did not succeed exactly once")
	}
	for _, control := range []struct {
		name          string
		value         webSearchNegativeControl
		removedConfig string
	}{
		{"web-search", negative, `web_search="disabled"`},
		{"agents", agentsNegative, "agents.enabled=false"},
	} {
		if control.value.ConfigRemoved != control.removedConfig || !control.value.OnlyRemovedConfig ||
			!lowerHexSHA256(control.value.EffectiveArgvSHA256) || control.value.CounterfactualArgvSHA256 != control.value.EffectiveArgvSHA256 ||
			control.value.ExpectedCLIExitCode != "nonzero" || control.value.ActualCLIExitCode == 0 || control.value.ValidReceiptEmitted {
			return fmt.Errorf("Codex %s negative control did not fail closed", control.name)
		}
	}
	if positive.EffectiveArgvSHA256 == negative.EffectiveArgvSHA256 || positive.EffectiveArgvSHA256 == agentsNegative.EffectiveArgvSHA256 ||
		negative.EffectiveArgvSHA256 == agentsNegative.EffectiveArgvSHA256 {
		return errors.New("Codex web-search counterfactual argv is not distinct from the positive control")
	}
	if serviceTierNegative.ConfigReplaced != `service_tier="default"` || serviceTierNegative.ReplacementConfig != `service_tier="priority"` ||
		!serviceTierNegative.OnlyReplacedConfigValue || !lowerHexSHA256(serviceTierNegative.EffectiveArgvSHA256) ||
		serviceTierNegative.CounterfactualArgvSHA256 != serviceTierNegative.EffectiveArgvSHA256 ||
		serviceTierNegative.ExpectedCLIExitCode != "nonzero" || serviceTierNegative.ActualCLIExitCode == 0 || serviceTierNegative.ValidReceiptEmitted ||
		positive.EffectiveArgvSHA256 == serviceTierNegative.EffectiveArgvSHA256 || negative.EffectiveArgvSHA256 == serviceTierNegative.EffectiveArgvSHA256 ||
		agentsNegative.EffectiveArgvSHA256 == serviceTierNegative.EffectiveArgvSHA256 {
		return fmt.Errorf("Codex %s negative control did not fail closed", "service-tier")
	}
	if err := validateCodexStandardCanaryRequest(positive.Request, model, 0, false, false, true, ""); err != nil {
		return fmt.Errorf("validate Codex web-search positive request: %w", err)
	}
	if err := validateCodexStandardCanaryRequest(negative.Request, model, 1, true, false, false, ""); err != nil {
		return fmt.Errorf("validate Codex web-search negative request: %w", err)
	}
	if err := validateCodexStandardCanaryRequest(agentsNegative.Request, model, 2, false, true, false, ""); err != nil {
		return fmt.Errorf("validate Codex agents negative request: %w", err)
	}
	if err := validateCodexStandardCanaryRequest(serviceTierNegative.Request, model, 3, false, false, false, "priority"); err != nil {
		return fmt.Errorf("validate Codex web-search negative request: %w", err)
	}
	return nil
}

func validateCodexStandardCanaryRequest(raw json.RawMessage, model harness.ModelRequestSpec, index int, wantWebSearch, wantMultiAgent, wantAccepted bool, explicitServiceTier string) error {
	fields := []string{
		"request_index", "model", "store", "reasoning_effort", "include_encrypted_reasoning", "stream", "responses_lite_header_present",
		"authorization_header_present", "originator", "request_service_tier_present", "request_service_tier", "request_service_tier_canonical", "request_service_tier_source", "ordered_tool_catalog",
		"web_search_tool_count", "web_search_external_access", "collaboration_namespace_present", "multi_agent_namespace_present", "subagent_tool_present",
		"configuration_accepted", "response_model", "response_service_tier", "response_service_tier_canonical", "response_request_id_present", "response_usage",
	}
	request, err := strictDecodeCanaryObject[codexStandardCanaryRequest](raw, fmt.Sprintf("Codex standard Responses canary request %d", index), fields...)
	if err != nil {
		return err
	}
	wantUsage := canaryUsage{InputTokens: 7, CachedInputTokens: 2, CacheWriteInputTokens: 1, OutputTokens: 3, ReasoningOutputTokens: 1}
	wantTools := []canaryToolIdentity{
		{Type: "function", Name: "exec_command", NamePresent: true},
		{Type: "function", Name: "write_stdin", NamePresent: true},
		{Type: "function", Name: "update_plan", NamePresent: true},
		{Type: "function", Name: "request_user_input", NamePresent: true},
		{Type: "function", Name: "view_image", NamePresent: true},
	}
	if wantWebSearch {
		wantTools = append(wantTools, canaryToolIdentity{Type: "web_search"})
	}
	if wantMultiAgent {
		wantTools = append(wantTools, canaryToolIdentity{Type: "namespace", Name: "multi_agent_v1", NamePresent: true})
	}
	derivedWebSearch := 0
	derivedCollaboration := false
	derivedMultiAgent := false
	derivedSubagent := false
	for _, tool := range request.OrderedToolCatalog {
		name := ""
		if tool.Name != nil {
			name = strings.ToLower(strings.ReplaceAll(*tool.Name, "-", "_"))
		}
		kind := strings.ToLower(strings.ReplaceAll(tool.Type, "-", "_"))
		if strings.Contains(kind, "web_search") || name == "web_search" {
			derivedWebSearch++
		}
		if kind == "namespace" && name == "collaboration" {
			derivedCollaboration = true
		}
		if kind == "namespace" && name == "multi_agent_v1" {
			derivedMultiAgent = true
		}
		if isCanarySubagentTool(kind, name) {
			derivedSubagent = true
		}
	}
	wantServiceTierPresent := explicitServiceTier != ""
	wantServiceTierCanonical := explicitServiceTier
	wantServiceTierSource := "wire_explicit"
	if !wantServiceTierPresent {
		wantServiceTierCanonical = "default"
		wantServiceTierSource = "client_canonicalized_default"
	}
	serviceTierValueMatches := request.RequestServiceTier == nil
	if wantServiceTierPresent {
		serviceTierValueMatches = request.RequestServiceTier != nil && *request.RequestServiceTier == explicitServiceTier
	}
	if request.RequestIndex != index || request.Model != model.Model || request.Store == nil || *request.Store || request.ReasoningEffort != model.ReasoningEffort ||
		!request.IncludeEncryptedReasoning || !request.Stream || request.ResponsesLiteHeaderPresent || !request.AuthorizationHeaderPresent || request.Originator != "codex_exec" ||
		request.RequestServiceTierPresent != wantServiceTierPresent || !serviceTierValueMatches || request.RequestServiceTierCanonical != wantServiceTierCanonical || request.RequestServiceTierSource != wantServiceTierSource ||
		request.ResponseModel != model.Model || request.ResponseServiceTier != "default" || request.ResponseServiceTierCanonical != "default" ||
		!request.ResponseRequestIDPresent || request.ResponseUsage != wantUsage || !matchesCanaryToolCatalog(request.OrderedToolCatalog, wantTools) || request.WebSearchToolCount != derivedWebSearch ||
		request.CollaborationNamespacePresent != derivedCollaboration || request.MultiAgentNamespacePresent != derivedMultiAgent || request.SubagentToolPresent != derivedSubagent ||
		derivedCollaboration || (derivedWebSearch > 0) != wantWebSearch || derivedMultiAgent != wantMultiAgent || derivedSubagent != wantMultiAgent || request.ConfigurationAccepted != wantAccepted {
		return errors.New("Codex standard Responses canary request violates its wire contract")
	}
	if wantWebSearch {
		if !slices.Equal(request.WebSearchExternalAccess, []bool{false}) {
			return errors.New("Codex web-search negative control changed external access semantics")
		}
	} else if len(request.WebSearchExternalAccess) != 0 {
		return errors.New("Codex standard Responses canary has unexplained web-search access evidence")
	}
	return nil
}

type canaryToolIdentity struct {
	Type        string
	Name        string
	NamePresent bool
}

func matchesCanaryToolCatalog(got []canaryTool, want []canaryToolIdentity) bool {
	if len(got) != len(want) {
		return false
	}
	for index, expected := range want {
		actual := got[index]
		if actual.Type != expected.Type || (actual.Name != nil) != expected.NamePresent {
			return false
		}
		if expected.NamePresent && *actual.Name != expected.Name {
			return false
		}
	}
	return true
}

func isCanarySubagentTool(kind, name string) bool {
	if kind == "namespace" && (name == "collaboration" || name == "multi_agent_v1") {
		return true
	}
	switch name {
	case "spawn_agent", "wait_agent", "send_message", "followup_task", "interrupt_agent", "list_agents":
		return true
	default:
		return false
	}
}

func normalizeProviderEvidence(rawPath, streamPath, outputPath string, agent harness.AgentSpec, expectedRunIdentity, canonicalizationBindingSHA string) error {
	if _, err := evidenceproxy.ValidateEvidenceSeal(rawPath, expectedRunIdentity); err != nil {
		return fmt.Errorf("validate sealed provider evidence: %w", err)
	}
	if agent.ID == "codex" && !lowerHexSHA256(canonicalizationBindingSHA) {
		return errors.New("sealed Codex provider evidence lacks its final service-tier canonicalization binding")
	}
	if agent.ID != "codex" && canonicalizationBindingSHA != "" {
		return errors.New("non-Codex provider evidence has a Codex service-tier canonicalization binding")
	}
	return normalizeProviderEvidenceUnsealedBound(rawPath, streamPath, outputPath, agent, expectedRunIdentity, canonicalizationBindingSHA)
}

// normalizeProviderEvidenceUnsealed exists only for isolated parser fixtures.
// Production paths must call normalizeProviderEvidence so an unsealed or
// truncated provider ledger can never be normalized into scoreable evidence.
func normalizeProviderEvidenceUnsealed(rawPath, streamPath, outputPath string, agent harness.AgentSpec, expectedRunIdentity string) error {
	return normalizeProviderEvidenceUnsealedBound(rawPath, streamPath, outputPath, agent, expectedRunIdentity, "")
}

func normalizeProviderEvidenceUnsealedBound(rawPath, streamPath, outputPath string, agent harness.AgentSpec, expectedRunIdentity, canonicalizationBindingSHA string) error {
	if !lowerHexSHA256(expectedRunIdentity) {
		return errors.New("provider evidence run identity is invalid")
	}
	records, err := harness.ReadJSONLines[evidenceproxy.Record](rawPath)
	if err != nil {
		return err
	}
	if len(records) == 0 {
		return errors.New("provider meter captured no requests")
	}
	trace, err := parseToolTrace(streamPath)
	if err != nil {
		return err
	}
	rounds, resultReceipts, err := projectProviderRecords(records, agent, expectedRunIdentity, canonicalizationBindingSHA)
	if err != nil {
		return err
	}
	// Correlation is defined by provider-request start order, but the sealed
	// ledger is appended in completion order. Temporarily derive the former
	// view, then restore evidence-sequence order before writing so the
	// normalized ledger retains the raw hash-chain order under concurrency.
	slices.SortFunc(rounds, func(left, right harness.ProviderRoundEvidence) int { return cmp.Compare(left.Round, right.Round) })
	if err := correlateToolEvidence(rounds, resultReceipts, trace); err != nil {
		return err
	}
	slices.SortFunc(rounds, func(left, right harness.ProviderRoundEvidence) int {
		return cmp.Compare(left.EvidenceSequence, right.EvidenceSequence)
	})
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(outputPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	for _, round := range rounds {
		if err := encoder.Encode(round); err != nil {
			return err
		}
	}
	return file.Sync()
}
func admissibleProviderError(code string) bool {
	switch code {
	case "upstream_transport", "provider_http_error", "upstream_read", "downstream_write", "provider_usage_receipt_incomplete", "incomplete_server_evidence":
		return true
	default:
		return false
	}
}

type tracedTool struct {
	IDHash      string
	Kind        string
	Error       *bool
	OutputBytes *int64
	DurationMS  *int64
}

type tracedRound struct {
	Logical        int
	Physical       int
	CriticalPathMS int64
	TotalLatencyMS int64
	QueueMS        int64
}

type parsedTrace struct {
	byID        map[string]int
	tools       []tracedTool
	rounds      []tracedRound
	duplicateID bool
}

func parseToolTrace(path string) (parsedTrace, error) {
	file, err := os.Open(path)
	if err != nil {
		return parsedTrace{}, err
	}
	defer file.Close()
	result := parsedTrace{byID: map[string]int{}}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 32*1024*1024)
	for scanner.Scan() {
		var event map[string]any
		if json.Unmarshal(scanner.Bytes(), &event) != nil {
			continue
		}
		kind, _ := event["type"].(string)
		if kind == "tool_result" {
			id, _ := event["tool_use_id"].(string)
			metrics, _ := event["metrics"].(map[string]any)
			isError, _ := event["is_error"].(bool)
			if id != "" {
				tool := tracedTool{IDHash: hashID(id), Kind: "luban_tool_result", Error: boolPointer(isError)}
				if outputBytes, ok := optionalIntValue(metrics["content_bytes"]); ok {
					tool.OutputBytes = &outputBytes
				}
				if duration, ok := optionalIntValue(metrics["duration_ms"]); ok {
					tool.DurationMS = &duration
				}
				result.addTool(tool)
			}
		}
		if kind == "item.completed" {
			item, _ := event["item"].(map[string]any)
			itemID, _ := item["id"].(string)
			itemKind, _ := item["type"].(string)
			tool := tracedTool{IDHash: hashID(itemID), Kind: itemKind}
			switch itemKind {
			case "command_execution":
				status, _ := item["status"].(string)
				exitCode, hasExitCode := optionalIntValue(item["exit_code"])
				failed := status != "completed" || (hasExitCode && exitCode != 0)
				tool.Error = boolPointer(failed)
				if output, ok := item["aggregated_output"].(string); ok {
					bytes := int64(len([]byte(output)))
					tool.OutputBytes = &bytes
				}
			case "file_change":
				status, _ := item["status"].(string)
				tool.Error = boolPointer(status != "completed")
				if changes, exists := item["changes"]; exists {
					raw, marshalErr := json.Marshal(changes)
					if marshalErr == nil {
						bytes := int64(len(raw))
						tool.OutputBytes = &bytes
					}
				}
			case "mcp_tool_call":
				status, _ := item["status"].(string)
				_, hasError := item["error"]
				tool.Error = boolPointer(status != "completed" || hasError)
				if value, exists := item["result"]; exists {
					raw, marshalErr := json.Marshal(value)
					if marshalErr == nil {
						bytes := int64(len(raw))
						tool.OutputBytes = &bytes
					}
				}
			default:
				continue
			}
			if duration, ok := optionalIntValue(item["duration_ms"]); ok {
				tool.DurationMS = &duration
			}
			if itemID != "" {
				result.addTool(tool)
			}
		}
		if kind == "agentic_metrics" && event["metric"] == "tool_round" {
			round, ok := event["tool_round"].(map[string]any)
			if !ok {
				return parsedTrace{}, errors.New("agent tool-round metric is not an object")
			}
			logical, logicalOK := optionalIntValue(round["logical_model_visible_calls"])
			physical, physicalOK := optionalIntValue(round["physical_child_operations"])
			critical, criticalOK := optionalIntValue(round["critical_path_ms"])
			total, totalOK := optionalIntValue(round["total_child_latency_ms"])
			queue, queueOK := optionalIntValue(round["queue_ms"])
			if !logicalOK || !physicalOK || !criticalOK || !totalOK || !queueOK || logical < 0 || physical < 0 || critical < 0 || total < 0 || queue < 0 {
				return parsedTrace{}, errors.New("agent tool-round metric is incomplete or invalid")
			}
			result.rounds = append(result.rounds, tracedRound{
				Logical: int(logical), Physical: int(physical), CriticalPathMS: critical,
				TotalLatencyMS: total, QueueMS: queue,
			})
		}
	}
	if err := scanner.Err(); err != nil {
		return parsedTrace{}, err
	}
	if result.duplicateID {
		return parsedTrace{}, errors.New("agent trace contains a duplicate tool identity")
	}
	return result, nil
}

func (trace *parsedTrace) addTool(tool tracedTool) {
	if tool.IDHash != "" {
		if _, duplicate := trace.byID[tool.IDHash]; duplicate {
			trace.duplicateID = true
		} else {
			trace.byID[tool.IDHash] = len(trace.tools)
		}
	}
	trace.tools = append(trace.tools, tool)
}

type toolLocation struct {
	round int
	call  int
}

type providerToolResultReceipt struct {
	Kind        string
	PayloadHash string
	OutputBytes int64
}

func correlateToolEvidence(rounds []harness.ProviderRoundEvidence, results map[string]providerToolResultReceipt, trace parsedTrace) error {
	locations := make([]toolLocation, 0)
	byID := map[string]toolLocation{}
	for roundIndex := range rounds {
		for callIndex := range rounds[roundIndex].ToolCalls {
			call := &rounds[roundIndex].ToolCalls[callIndex]
			location := toolLocation{round: roundIndex, call: callIndex}
			if _, duplicate := byID[call.ID]; duplicate {
				return fmt.Errorf("provider tool call ID %s is duplicated", call.ID)
			}
			byID[call.ID] = location
			locations = append(locations, location)
			if result, ok := results[call.ID]; ok {
				if strings.TrimSuffix(result.Kind, "_output") != call.Kind {
					return fmt.Errorf("provider result kind %s does not match call kind %s", result.Kind, call.Kind)
				}
				call.OutputBytes = int64Pointer(result.OutputBytes)
			}
		}
	}
	for id := range results {
		if _, ok := byID[id]; !ok {
			return fmt.Errorf("provider request contains a result for unknown tool call %s", id)
		}
	}
	usedTrace := make([]bool, len(trace.tools))
	matchedCall := map[toolLocation]bool{}
	traceForCall := map[toolLocation]int{}
	for _, location := range locations {
		call := &rounds[location.round].ToolCalls[location.call]
		traceIndex, ok := trace.byID[call.ID]
		if !ok {
			continue
		}
		if isCodexExecCall(*call) && !codexExecTraceCompatible(trace.tools[traceIndex]) {
			return fmt.Errorf("Codex custom exec %s has incompatible agent trace kind %s", call.ID, trace.tools[traceIndex].Kind)
		}
		applyTrace(call, trace.tools[traceIndex], "id")
		usedTrace[traceIndex] = true
		matchedCall[location] = true
		traceForCall[location] = traceIndex
	}
	correlateCodexExecByOrder(rounds, locations, trace, usedTrace, matchedCall, traceForCall)
	// Correlate remaining monomorphic tool families by compatible kind and
	// stable provider round/call ordinal only when the complete cardinalities
	// agree. Codex's polymorphic custom exec is intentionally excluded here and
	// handled by the stricter matcher above. Requiring a provider-visible result
	// prevents an aborted proposal from consuming a later execution receipt.
	for _, family := range []string{"command", "file_change", "mcp"} {
		candidateCalls := make([]toolLocation, 0)
		for _, location := range locations {
			if matchedCall[location] {
				continue
			}
			call := rounds[location.round].ToolCalls[location.call]
			if call.OutputBytes != nil && toolCallFamily(call.Name) == family {
				candidateCalls = append(candidateCalls, location)
			}
		}
		candidateTraces := make([]int, 0)
		for traceIndex, tool := range trace.tools {
			if !usedTrace[traceIndex] && tracedToolFamily(tool.Kind) == family {
				candidateTraces = append(candidateTraces, traceIndex)
			}
		}
		if len(candidateCalls) == 0 || len(candidateCalls) != len(candidateTraces) {
			continue
		}
		for index, location := range candidateCalls {
			traceIndex := candidateTraces[index]
			applyTrace(&rounds[location.round].ToolCalls[location.call], trace.tools[traceIndex], "ordered_kind")
			matchedCall[location] = true
			usedTrace[traceIndex] = true
			traceForCall[location] = traceIndex
		}
	}
	roundMetricIndex := 0
	for index := range rounds {
		round := &rounds[index]
		if len(round.ToolCalls) == 0 || round.Outcome != "success" || roundMetricIndex >= len(trace.rounds) {
			continue
		}
		operational := trace.rounds[roundMetricIndex]
		if operational.Logical != len(round.ToolCalls) {
			return fmt.Errorf("tool-round metric %d reports %d logical calls, provider emitted %d", roundMetricIndex, operational.Logical, len(round.ToolCalls))
		}
		roundMetricIndex++
		round.PhysicalToolOperations = intPointer(operational.Physical)
		round.ToolCriticalPathMS = int64Pointer(operational.CriticalPathMS)
		round.ToolTotalLatencyMS = int64Pointer(operational.TotalLatencyMS)
		round.ToolQueueMS = int64Pointer(operational.QueueMS)
	}
	if roundMetricIndex != len(trace.rounds) {
		return fmt.Errorf("agent trace contains %d unmatched tool-round metrics", len(trace.rounds)-roundMetricIndex)
	}
	return nil
}

// correlateCodexExecByOrder binds Codex's provider-visible custom exec calls
// to its content-free agent trace. A custom exec is deliberately polymorphic:
// Codex reports a shell execution as command_execution and an apply_patch
// execution as file_change, while both are named exec on the provider wire.
//
// The fallback never inspects tool input or output. It only commits a binding
// when every remaining result-bearing exec has exactly one remaining
// command/file-change receipt, their ordinal pairing is type-compatible, and
// that pairing preserves the order established by exact-ID matches. A missing
// receipt, an extra receipt, or an order conflict leaves the whole residual set
// unmatched; choosing a subset would make attribution ambiguous.
func correlateCodexExecByOrder(
	rounds []harness.ProviderRoundEvidence,
	locations []toolLocation,
	trace parsedTrace,
	usedTrace []bool,
	matchedCall map[toolLocation]bool,
	traceForCall map[toolLocation]int,
) {
	callOrdinal := make(map[toolLocation]int, len(locations))
	candidateCalls := make([]toolLocation, 0)
	for ordinal, location := range locations {
		callOrdinal[location] = ordinal
		if matchedCall[location] {
			continue
		}
		call := rounds[location.round].ToolCalls[location.call]
		if call.OutputBytes != nil && isCodexExecCall(call) {
			candidateCalls = append(candidateCalls, location)
		}
	}
	candidateTraces := make([]int, 0)
	for index, tool := range trace.tools {
		if !usedTrace[index] && codexExecTraceCompatible(tool) {
			candidateTraces = append(candidateTraces, index)
		}
	}
	if len(candidateCalls) == 0 || len(candidateCalls) != len(candidateTraces) {
		return
	}

	proposed := make(map[toolLocation]int, len(candidateCalls))
	for index, location := range candidateCalls {
		traceIndex := candidateTraces[index]
		if !codexExecTraceCompatible(trace.tools[traceIndex]) {
			return
		}
		proposed[location] = traceIndex
	}
	for location, traceIndex := range proposed {
		for anchor, anchorTraceIndex := range traceForCall {
			if compareOrdinal(callOrdinal[location], callOrdinal[anchor]) != compareOrdinal(traceIndex, anchorTraceIndex) {
				return
			}
		}
	}

	for _, location := range candidateCalls {
		traceIndex := proposed[location]
		applyTrace(&rounds[location.round].ToolCalls[location.call], trace.tools[traceIndex], "ordered_kind")
		matchedCall[location] = true
		usedTrace[traceIndex] = true
		traceForCall[location] = traceIndex
	}
}

func isCodexExecCall(call harness.ToolCallEvidence) bool {
	return call.Kind == "custom_tool_call" && strings.EqualFold(call.Name, "exec")
}

func codexExecTraceCompatible(tool tracedTool) bool {
	return tool.Kind == "command_execution" || tool.Kind == "file_change"
}

func compareOrdinal(left, right int) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}

func applyTrace(call *harness.ToolCallEvidence, tool tracedTool, match string) {
	call.Error = tool.Error
	call.DurationMS = tool.DurationMS
	call.AgentTraceOutputBytes = tool.OutputBytes
	call.TraceKind = tool.Kind
	call.TraceMatch = match
}

func toolCallFamily(name string) string {
	normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(name, "_", ""), "-", ""))
	switch {
	case normalized == "exec":
		// Codex custom exec is polymorphic and must be correlated by the
		// dedicated content-free ordered matcher above.
		return ""
	case strings.Contains(normalized, "shell"), strings.Contains(normalized, "bash"), strings.Contains(normalized, "exec"), normalized == "run":
		return "command"
	case strings.Contains(normalized, "applypatch"), strings.Contains(normalized, "filechange"), normalized == "patch":
		return "file_change"
	case strings.Contains(normalized, "mcp"):
		return "mcp"
	}
	return ""
}

func tracedToolFamily(kind string) string {
	switch kind {
	case "command_execution", "luban_tool_result":
		return "command"
	case "file_change":
		return "file_change"
	case "mcp_tool_call":
		return "mcp"
	}
	return ""
}

func boolPointer(value bool) *bool { return &value }

func int64Pointer(value int64) *int64 { return &value }

func intPointer(value int) *int { return &value }

func optionalIntValue(value any) (int64, bool) {
	if value == nil {
		return 0, false
	}
	switch typed := value.(type) {
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) || math.Trunc(typed) != typed || typed < math.MinInt64 || typed > math.MaxInt64 {
			return 0, false
		}
		return int64(typed), true
	case json.Number:
		parsed, err := typed.Int64()
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseInt(typed, 10, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func readCapture(path string) (harness.SubmissionCaptureEvidence, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return harness.SubmissionCaptureEvidence{}, err
	}
	var envelope struct {
		SchemaVersion string `json:"schema_version"`
		harness.SubmissionCaptureEvidence
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		return harness.SubmissionCaptureEvidence{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return harness.SubmissionCaptureEvidence{}, errors.New("workspace capture receipt contains trailing JSON")
	}
	if envelope.SchemaVersion != "agentic-bench/workspace-capture-v2" || envelope.Method != "official-git-diff+temporary-index-audit-v2" ||
		!lowerHexSHA256(envelope.PatchSHA256) || !lowerHexSHA256(envelope.AuditPatchSHA256) ||
		!envelope.IncludesTracked || !envelope.IncludesUntracked || !envelope.IncludesBinary {
		return harness.SubmissionCaptureEvidence{}, errors.New("workspace capture receipt is incomplete")
	}
	return envelope.SubmissionCaptureEvidence, nil
}

func sealedProviderContextTerminal(rawEvidence, expectedRunIdentity string) (harness.AgentTerminalEvidence, bool, error) {
	if _, err := evidenceproxy.ValidateEvidenceSeal(rawEvidence, expectedRunIdentity); err != nil {
		return harness.AgentTerminalEvidence{}, false, fmt.Errorf("validate provider context evidence seal: %w", err)
	}
	records, err := harness.ReadJSONLines[evidenceproxy.Record](rawEvidence)
	if err != nil {
		return harness.AgentTerminalEvidence{}, false, err
	}
	return deriveProviderContextTerminal(records, expectedRunIdentity)
}

func deriveProviderContextTerminal(records []evidenceproxy.Record, expectedRunIdentity string) (harness.AgentTerminalEvidence, bool, error) {
	if len(records) == 0 || !lowerHexSHA256(expectedRunIdentity) {
		return harness.AgentTerminalEvidence{}, false, errors.New("provider context evidence has no valid run identity or rounds")
	}
	records = slices.Clone(records)
	slices.SortFunc(records, func(left, right evidenceproxy.Record) int { return cmp.Compare(left.Round, right.Round) })
	terminalInferenceIndex := -1
	for index, record := range records {
		if record.SchemaVersion != "agentic-bench/provider-http-v6" || record.Round != index || record.RunIdentity != expectedRunIdentity {
			return harness.AgentTerminalEvidence{}, false, errors.New("provider context evidence is not bound to one contiguous run")
		}
		switch record.ProviderAttemptKind {
		case "inference":
			terminalInferenceIndex = index
		case "prewarm":
		default:
			return harness.AgentTerminalEvidence{}, false, fmt.Errorf("provider round %d has an unknown attempt kind", record.Round)
		}
		failurePresent := record.ResponseFailureCode != "" || record.ResponseFailureEventSHA256 != ""
		if !failurePresent {
			continue
		}
		if record.ResponseFailureCode != "context_length_exceeded" || !lowerHexSHA256(record.ResponseFailureEventSHA256) ||
			record.ProviderAttemptKind != "inference" || record.Disposition != "agent_context_failure" ||
			record.ErrorCode != "provider_context_failure" || record.ProtocolValid || !record.ProviderAttemptStarted ||
			record.ResponseCompleted || (record.ResponseStatus != "" && record.ResponseStatus != "failed") {
			return harness.AgentTerminalEvidence{}, false, fmt.Errorf("provider round %d has unknown or unsealed response-failure evidence", record.Round)
		}
	}
	if terminalInferenceIndex < 0 {
		return harness.AgentTerminalEvidence{}, false, nil
	}
	terminal := records[terminalInferenceIndex]
	if terminal.ResponseFailureCode == "" && terminal.ResponseFailureEventSHA256 == "" {
		return harness.AgentTerminalEvidence{}, false, nil
	}
	return harness.AgentTerminalEvidence{
		SchemaVersion:  "agentic-bench/terminal-evidence-v1",
		Source:         "provider_event",
		Code:           "context_length_exceeded",
		EvidenceSHA256: terminal.ResponseFailureEventSHA256,
	}, true, nil
}

// validateCodexProxyContextStream proves only that the fixed Codex CLI ended
// in a nonzero failed turn. It deliberately does not infer a failure class
// from ThreadErrorEvent.message: Codex 0.145's exec JSON projection drops the
// app-server codexErrorInfo field, so classification authority lives solely in
// the sealed provider response above.
func validateCodexProxyContextStream(streamRaw []byte, exitCode int) error {
	if exitCode == 0 {
		return errors.New("Codex context stream conflicts with a zero process exit")
	}
	lines := bytes.Split(streamRaw, []byte{'\n'})
	if len(lines) > 0 && len(lines[len(lines)-1]) == 0 {
		lines = lines[:len(lines)-1]
	}
	failedTurns := 0
	completedTurns := 0
	for index, line := range lines {
		line = bytes.TrimSuffix(line, []byte{'\r'})
		if len(line) == 0 {
			return fmt.Errorf("Codex machine stream contains an empty record at line %d", index+1)
		}
		if err := validateStrictJSON(line); err != nil {
			return fmt.Errorf("Codex machine stream contains invalid JSON at line %d: %w", index+1, err)
		}
		var event map[string]any
		if err := json.Unmarshal(line, &event); err != nil {
			return fmt.Errorf("decode Codex machine event at line %d: %w", index+1, err)
		}
		eventType, ok := event["type"].(string)
		if !ok || eventType == "" {
			return fmt.Errorf("Codex machine stream contains an untyped event at line %d", index+1)
		}
		switch eventType {
		case "turn.completed":
			completedTurns++
		case "turn.failed":
			failedTurns++
			if code := structuredProviderFailureCode(event); code != "" && code != "context_length_exceeded" {
				return fmt.Errorf("Codex failed turn has unsupported structured code %q", code)
			}
		case "error", "response.failed":
			if code := structuredProviderFailureCode(event); code != "" && code != "context_length_exceeded" {
				return fmt.Errorf("Codex terminal event has unsupported structured code %q", code)
			}
		}
	}
	if failedTurns != 1 || completedTurns != 0 {
		return errors.New("Codex stream does not prove one unambiguous failed turn")
	}
	return nil
}

func readExitReceipt(path string) (int, []byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, nil, err
	}
	if err := validateStrictJSON(raw); err != nil {
		return 0, nil, fmt.Errorf("validate agent exit receipt JSON: %w", err)
	}
	var receipt struct {
		SchemaVersion string `json:"schema_version"`
		ExitCode      int    `json:"exit_code"`
		StartedAt     string `json:"started_at"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&receipt); err != nil {
		return 0, nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return 0, nil, errors.New("agent exit receipt contains trailing JSON")
	}
	if receipt.SchemaVersion != "agentic-bench/agent-exit-v1" || receipt.StartedAt == "" {
		return 0, nil, errors.New("unsupported or incomplete agent exit receipt")
	}
	return receipt.ExitCode, raw, nil
}

func sha256Hex(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// validateStrictJSON applies the same JSON authority rules as the pinned
// Python adapter. encoding/json otherwise accepts duplicate object keys by
// silently keeping the last value, which would let the adapter and the
// independent archiver interpret the same evidence bytes differently.
func validateStrictJSON(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := validateStrictJSONValue(decoder); err != nil {
		return err
	}
	if token, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err != nil {
			return err
		}
		return fmt.Errorf("unexpected trailing JSON token %v", token)
	}
	return nil
}

func validateStrictJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("JSON object key is not a string")
			}
			if _, duplicate := seen[key]; duplicate {
				return fmt.Errorf("duplicate JSON key %q", key)
			}
			seen[key] = struct{}{}
			if err := validateStrictJSONValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil {
			return err
		}
		if closing != json.Delim('}') {
			return errors.New("JSON object has an invalid closing delimiter")
		}
	case '[':
		for decoder.More() {
			if err := validateStrictJSONValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil {
			return err
		}
		if closing != json.Delim(']') {
			return errors.New("JSON array has an invalid closing delimiter")
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delimiter)
	}
	return nil
}

type parsedTerminalEvent struct {
	kind string
	code string
	raw  []byte
}

func nestedEventString(event map[string]any, path ...string) string {
	var current any = event
	for _, component := range path {
		object, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current = object[component]
	}
	value, _ := current.(string)
	return value
}

func structuredProviderFailureCode(event map[string]any) string {
	switch event["type"] {
	case "response.failed":
		return nestedEventString(event, "response", "error", "code")
	case "turn.failed":
		return nestedEventString(event, "error", "code")
	case "error":
		if code := nestedEventString(event, "code"); code != "" {
			return code
		}
		return nestedEventString(event, "error", "code")
	default:
		return ""
	}
}

func parseStructuredTerminalEvent(agentKind string, event map[string]any, raw []byte) (parsedTerminalEvent, bool, error) {
	kind, _ := event["type"].(string)
	if kind == "turn.completed" {
		if agentKind != "codex" {
			return parsedTerminalEvent{}, false, errors.New("non-Codex stream emitted a Codex terminal event")
		}
		return parsedTerminalEvent{kind: "completed", code: "completed", raw: raw}, true, nil
	}
	if kind != "response.failed" && kind != "turn.failed" && kind != "error" {
		return parsedTerminalEvent{}, false, nil
	}
	if code := structuredProviderFailureCode(event); code != "" {
		return parsedTerminalEvent{kind: "provider_failure", code: code, raw: raw}, true, nil
	}
	if agentKind == "luban" && kind == "error" &&
		event["schema_version"] == "runtime-event/v2" && event["kind"] == "runtime_error" && event["outcome"] == "failed" {
		if code, ok := event["code"].(string); ok && code != "" {
			return parsedTerminalEvent{kind: "provider_failure", code: code, raw: raw}, true, nil
		}
	}
	return parsedTerminalEvent{}, false, fmt.Errorf("%s terminal event lacks a supported structured code", agentKind)
}

func deriveTerminalEvidence(agentKind string, streamRaw, exitReceiptRaw []byte, exitCode int) (harness.AgentTerminalEvidence, error) {
	if agentKind != "codex" && agentKind != "luban" {
		return harness.AgentTerminalEvidence{}, fmt.Errorf("unsupported agent kind %q", agentKind)
	}
	lines := bytes.Split(streamRaw, []byte{'\n'})
	if len(lines) > 0 && len(lines[len(lines)-1]) == 0 {
		lines = lines[:len(lines)-1]
	}
	var terminals []parsedTerminalEvent
	for index, line := range lines {
		line = bytes.TrimSuffix(line, []byte{'\r'})
		if len(line) == 0 {
			return harness.AgentTerminalEvidence{}, fmt.Errorf("machine stream contains an empty record at line %d", index+1)
		}
		if err := validateStrictJSON(line); err != nil {
			return harness.AgentTerminalEvidence{}, fmt.Errorf("machine stream contains invalid JSON at line %d: %w", index+1, err)
		}
		var event map[string]any
		if err := json.Unmarshal(line, &event); err != nil {
			return harness.AgentTerminalEvidence{}, fmt.Errorf("machine stream contains invalid JSON at line %d: %w", index+1, err)
		}
		if _, ok := event["type"].(string); !ok {
			return harness.AgentTerminalEvidence{}, fmt.Errorf("machine stream contains an untyped event at line %d", index+1)
		}
		terminal, found, err := parseStructuredTerminalEvent(agentKind, event, line)
		if err != nil {
			return harness.AgentTerminalEvidence{}, err
		}
		if found {
			terminals = append(terminals, terminal)
		}
	}
	var contexts, completed []parsedTerminalEvent
	for _, terminal := range terminals {
		switch {
		case terminal.kind == "provider_failure" && terminal.code == "context_length_exceeded":
			contexts = append(contexts, terminal)
		case terminal.kind == "provider_failure":
			return harness.AgentTerminalEvidence{}, errors.New("machine stream contains an unsupported structured provider failure")
		case terminal.kind == "completed":
			completed = append(completed, terminal)
		}
	}
	if len(contexts) > 0 {
		if len(contexts) != 1 || len(completed) != 0 || exitCode == 0 {
			return harness.AgentTerminalEvidence{}, errors.New("context failure conflicts with another terminal outcome")
		}
		return harness.AgentTerminalEvidence{
			SchemaVersion: "agentic-bench/terminal-evidence-v1", Source: "provider_event",
			Code: "context_length_exceeded", EvidenceSHA256: sha256Hex(contexts[0].raw),
		}, nil
	}
	if len(completed) > 0 && (len(completed) != 1 || exitCode != 0) {
		return harness.AgentTerminalEvidence{}, errors.New("Codex turn completion conflicts with the process exit")
	}
	code := "completed"
	if exitCode != 0 {
		code = "nonzero_exit"
	}
	return harness.AgentTerminalEvidence{
		SchemaVersion: "agentic-bench/terminal-evidence-v1", Source: "process_exit",
		Code: code, EvidenceSHA256: sha256Hex(exitReceiptRaw),
	}, nil
}

func readAndValidateTerminalEvidence(path, streamPath string, exitReceiptRaw []byte, exitCode int, agentKind string) (harness.AgentTerminalEvidence, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return harness.AgentTerminalEvidence{}, err
	}
	if err := validateStrictJSON(raw); err != nil {
		return harness.AgentTerminalEvidence{}, fmt.Errorf("validate adapter terminal evidence JSON: %w", err)
	}
	var receipt harness.AgentTerminalEvidence
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&receipt); err != nil {
		return harness.AgentTerminalEvidence{}, fmt.Errorf("decode adapter terminal evidence: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return harness.AgentTerminalEvidence{}, errors.New("adapter terminal evidence contains trailing JSON")
	}
	streamRaw, err := os.ReadFile(streamPath)
	if err != nil {
		return harness.AgentTerminalEvidence{}, err
	}
	expected, err := deriveTerminalEvidence(agentKind, streamRaw, exitReceiptRaw, exitCode)
	if err != nil {
		return harness.AgentTerminalEvidence{}, err
	}
	if receipt != expected {
		return harness.AgentTerminalEvidence{}, errors.New("adapter terminal evidence disagrees with the raw structured event")
	}
	return receipt, nil
}

func exactEnvironmentValue(environment []string, name string) (string, error) {
	value := ""
	found := false
	for _, item := range environment {
		key, candidate, ok := strings.Cut(item, "=")
		if key != name || !ok {
			continue
		}
		if found {
			return "", fmt.Errorf("provider credential %s is duplicated", name)
		}
		value, found = candidate, true
	}
	if !found || value == "" {
		return "", fmt.Errorf("provider credential %s is missing", name)
	}
	return value, nil
}

func waitProxyReady(ctx context.Context, readyPath string, failures <-chan error) (string, error) {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if raw, err := os.ReadFile(readyPath); err == nil {
			return strings.TrimSpace(string(raw)), nil
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case err := <-failures:
			return "", fmt.Errorf("provider meter failed before readiness: %w", err)
		case <-ticker.C:
		}
	}
}

func waitProxyStop(failures <-chan error) error {
	select {
	case err := <-failures:
		return err
	case <-time.After(10 * time.Second):
		return errors.New("provider meter did not stop")
	}
}

func redactArgs(args []string) []string {
	result := make([]string, len(args))
	for index, value := range args {
		if strings.HasPrefix(value, "proxy_base_url=") || strings.HasPrefix(value, "proxy_health_url=") {
			digest := sha256.Sum256([]byte(value))
			result[index] = strings.SplitN(value, "=", 2)[0] + "_sha256=" + hex.EncodeToString(digest[:])
		} else {
			result[index] = value
		}
	}
	return result
}

func hashID(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
