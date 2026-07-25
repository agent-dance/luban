package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/agent-dance/luban/internal/contracts/stream"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/internal/runtime/loop"
	"github.com/agent-dance/luban/internal/store/session"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/sandbox"
	"github.com/agent-dance/luban/types"
)

const liveCacheLineageGate = "CACHE_LINEAGE_LIVE"

type liveCacheProviderSpec struct {
	name            string
	model           string
	protocol        string
	newProvider     func() provider.Provider
	slack           int
	requireCreation bool
}

type liveCacheProbeProvider struct {
	inner   provider.Provider
	mu      sync.Mutex
	hash    string
	last    string
	enabled bool
	lineage string
}

func (p *liveCacheProbeProvider) Name() string    { return p.inner.Name() }
func (p *liveCacheProbeProvider) ModelID() string { return p.inner.ModelID() }
func (p *liveCacheProbeProvider) Capabilities() provider.ProviderCapabilities {
	if capabilityProvider, ok := p.inner.(provider.CapabilityProvider); ok {
		return capabilityProvider.Capabilities()
	}
	return provider.ProviderCapabilities{}
}

func (p *liveCacheProbeProvider) CreateStream(ctx context.Context, params provider.Params) (<-chan types.StreamEvent, error) {
	requestHash, err := liveCacheParamsHash(params)
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	p.hash = requestHash
	p.enabled = params.UsePromptCache && params.PromptCacheKey != ""
	p.lineage = params.PromptCacheKey
	p.mu.Unlock()
	stream, err := p.inner.CreateStream(ctx, params)
	if err != nil {
		return nil, err
	}
	out := make(chan types.StreamEvent, 64)
	go func() {
		defer close(out)
		for event := range stream {
			if event.SystemFingerprint != "" {
				p.mu.Lock()
				p.last = event.SystemFingerprint
				p.mu.Unlock()
			}
			out <- event
		}
	}()
	return out, nil
}

func (p *liveCacheProbeProvider) Evidence() (string, string, bool, string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.hash, p.last, p.enabled, p.lineage
}

func liveCacheParamsHash(params provider.Params) (string, error) {
	// The actual lineage is intentionally excluded from recordings while the
	// presence and routing behavior remain represented by UsePromptCache.
	params.PromptCacheKey = "<cache-lineage>"
	payload, err := json.Marshal(params)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}

type liveCacheRunEvidence struct {
	Usage               types.Usage `json:"usage"`
	ParamsSHA256        string      `json:"params_sha256"`
	SystemFingerprint   string      `json:"system_fingerprint,omitempty"`
	CacheLineageEnabled bool        `json:"cache_lineage_enabled"`
}

type liveCacheLineageEvidence struct {
	Version               int                  `json:"version"`
	Provider              string               `json:"provider"`
	Model                 string               `json:"model"`
	Protocol              string               `json:"protocol"`
	LineageInherited      bool                 `json:"lineage_inherited"`
	RequestLineageMatched bool                 `json:"request_lineage_matched"`
	Cold                  liveCacheRunEvidence `json:"cold"`
	SourceHot             liveCacheRunEvidence `json:"source_hot"`
	Fork                  liveCacheRunEvidence `json:"fork"`
}

func TestLiveCacheLineageSourceHotFork(t *testing.T) {
	requested := liveCacheRequestedProviders(os.Getenv(liveCacheLineageGate))
	if len(requested) == 0 {
		t.Skip("set CACHE_LINEAGE_LIVE=deepseek,openai-responses,anthropic (or all) to run paid live cache verification")
	}

	for _, providerName := range requested {
		providerName := providerName
		t.Run(providerName, func(t *testing.T) {
			spec, ok := liveCacheSpecFromEnvironment(providerName)
			if !ok {
				t.Skipf("credentials for %s are not configured", providerName)
			}
			verifyLiveCacheLineage(t, spec)
		})
	}
}

func liveCacheRequestedProviders(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	seen := make(map[string]struct{})
	for _, item := range strings.Split(raw, ",") {
		name := strings.ToLower(strings.TrimSpace(item))
		if name == "all" {
			return []string{"anthropic", "deepseek", "openai-responses"}
		}
		if name == "openai" {
			name = "openai-responses"
		}
		switch name {
		case "anthropic", "deepseek", "openai-responses":
			seen[name] = struct{}{}
		}
	}
	providers := make([]string, 0, len(seen))
	for name := range seen {
		providers = append(providers, name)
	}
	sort.Strings(providers)
	return providers
}

func liveCacheSpecFromEnvironment(providerName string) (liveCacheProviderSpec, bool) {
	switch providerName {
	case "deepseek":
		apiKey := firstLiveCacheEnv("DEEPSEEK_LIVE_CACHE_API_KEY", "DEEPSEEK_API_KEY")
		if apiKey == "" {
			return liveCacheProviderSpec{}, false
		}
		model := firstLiveCacheValue(os.Getenv("DEEPSEEK_LIVE_CACHE_MODEL"), os.Getenv("DEEPSEEK_MODEL"), brand.DeepSeekDefaultModel)
		baseURL := firstLiveCacheValue(os.Getenv("DEEPSEEK_BASE_URL"), brand.DeepSeekBaseURL)
		return liveCacheProviderSpec{name: providerName, model: model, protocol: "deepseek_user_id", newProvider: func() provider.Provider {
			raw := provider.NewOpenAI(provider.Config{ProviderName: "deepseek", APIKey: apiKey, BaseURL: baseURL, Model: model, MaxTokens: 64, Timeout: 120})
			return provider.NewRetryProvider(raw, provider.DefaultRetryConfig())
		}, slack: 128}, true
	case "openai-responses":
		apiKey := firstLiveCacheEnv("OPENAI_LIVE_CACHE_API_KEY", "OPENAI_API_KEY")
		if apiKey == "" {
			return liveCacheProviderSpec{}, false
		}
		model := firstLiveCacheValue(os.Getenv("OPENAI_LIVE_CACHE_MODEL"), os.Getenv("OPENAI_MODEL"), provider.CatalogDefaultModel("openai", "gpt-5.4-mini"))
		baseURL := firstLiveCacheValue(os.Getenv("OPENAI_BASE_URL"), "https://api.openai.com/v1")
		return liveCacheProviderSpec{name: providerName, model: model, protocol: "prompt_cache_key", newProvider: func() provider.Provider {
			raw := provider.NewResponses(provider.Config{APIKey: apiKey, BaseURL: baseURL, Model: model, MaxTokens: 64, Timeout: 120})
			return provider.NewRetryProvider(raw, provider.DefaultRetryConfig())
		}, slack: 256}, true
	case "anthropic":
		apiKey := firstLiveCacheEnv("ANTHROPIC_LIVE_CACHE_API_KEY", "ANTHROPIC_API_KEY")
		authToken := strings.TrimSpace(os.Getenv("ANTHROPIC_AUTH_TOKEN"))
		if apiKey == "" && authToken == "" {
			return liveCacheProviderSpec{}, false
		}
		model := firstLiveCacheValue(os.Getenv("ANTHROPIC_LIVE_CACHE_MODEL"), os.Getenv("ANTHROPIC_MODEL"), provider.CatalogDefaultModel("anthropic", "claude-sonnet-5"))
		baseURL := strings.TrimSpace(os.Getenv("ANTHROPIC_BASE_URL"))
		return liveCacheProviderSpec{name: providerName, model: model, protocol: "cache_control", newProvider: func() provider.Provider {
			raw := provider.NewAnthropic(provider.Config{APIKey: apiKey, AuthToken: authToken, BaseURL: baseURL, Model: model, MaxTokens: 64, Timeout: 120})
			return provider.NewRetryProvider(raw, provider.DefaultRetryConfig())
		}, slack: 256, requireCreation: true}, true
	default:
		return liveCacheProviderSpec{}, false
	}
}

func firstLiveCacheEnv(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

func firstLiveCacheValue(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func verifyLiveCacheLineage(t *testing.T, spec liveCacheProviderSpec) {
	t.Helper()
	workspace := t.TempDir()
	projectDir := filepath.Join(workspace, "project")
	if err := os.MkdirAll(projectDir, 0o700); err != nil {
		t.Fatal(err)
	}
	repository := session.NewRepository(filepath.Join(workspace, "sessions"))
	sourceRef := session.Ref{ID: uuid.NewString(), ProjectDir: projectDir}
	nonce := uuid.NewString()
	lineageID := sourceRef.ID
	systemPrefix := "Cache verification nonce: " + nonce + ". This nonce must remain the first system prefix."

	newRuntime := func(t *testing.T, sessionID, cacheLineageID string) (*loop.QueryLoop, *liveCacheProbeProvider, string) {
		t.Helper()
		observed := &liveCacheProbeProvider{inner: spec.newProvider()}
		ref := provider.NewProviderRef(observed)
		deps := SetupRegistry(ref, projectDir, []string{projectDir}, sandbox.NoopBackend{}, nil, true)
		t.Cleanup(func() {
			stopScheduleForTest(t, deps)
			deps.StopWebFetchCache()
			stopMCPRuntimeBridgeForTest(t, deps)
		})
		if err := prepareInitialRegistryRuntime(deps, projectDir, []string{projectDir}); err != nil {
			t.Fatal(err)
		}
		deps.BindSessionIdentity(sessionID)
		system := systemPrefix + "\n\n" + buildSystemPromptForCWD("", deps.Registry, projectDir)
		deps.AgentTool.System = system
		query := loop.New(ref, deps.Registry, loop.Config{
			Model:            spec.model,
			System:           system,
			MaxTurns:         1,
			MaxTokens:        64,
			MaxContextTokens: 1_000_000,
			SessionID:        sessionID,
			CacheLineageID:   cacheLineageID,
			ProjectRoot:      projectDir,
			CWD:              projectDir,
			SkillManager:     deps.SkillManager,
		})
		return query, observed, system
	}

	run := func(t *testing.T, query *loop.QueryLoop, observed *liveCacheProbeProvider, prompt string) (liveCacheRunEvidence, string) {
		t.Helper()
		var usage types.Usage
		if err := query.Run(context.Background(), prompt, func(event stream.Event) {
			if event.Type == stream.EventTurnEnd && event.Usage != nil {
				usage = *event.Usage
			}
		}); err != nil {
			t.Fatal(err)
		}
		paramsHash, fingerprint, cacheLineageEnabled, requestLineage := observed.Evidence()
		return liveCacheRunEvidence{Usage: usage, ParamsSHA256: paramsHash, SystemFingerprint: fingerprint, CacheLineageEnabled: cacheLineageEnabled}, requestLineage
	}

	coldQuery, coldProvider, sourceSystem := newRuntime(t, sourceRef.ID, lineageID)
	cold, coldRequestLineage := run(t, coldQuery, coldProvider, "Do not call tools. Reply with exactly CACHE_SEEDED.")
	seedHistory := coldQuery.Messages()
	if got := liveSkillCatalogMessageCount(seedHistory); got != 1 {
		t.Fatalf("seed history catalog snapshots = %d, want 1", got)
	}
	store := repository.StoreForProjectDir(projectDir)
	if err := store.Save(sourceRef.ID, seedHistory); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveMeta(sourceRef.ID, session.SessionMeta{CacheLineageID: lineageID, CWD: projectDir, Provider: spec.name, Model: spec.model}); err != nil {
		t.Fatal(err)
	}
	forkRef, err := repository.Fork(sourceRef, seedHistory)
	if err != nil {
		t.Fatal(err)
	}
	sourceMeta, err := store.GetMeta(sourceRef.ID)
	if err != nil {
		t.Fatal(err)
	}
	forkMeta, err := store.GetMeta(forkRef.ID)
	if err != nil {
		t.Fatal(err)
	}
	forkHistory, err := repository.Load(forkRef)
	if err != nil {
		t.Fatal(err)
	}

	settle := 2 * time.Second
	if configured := strings.TrimSpace(os.Getenv("CACHE_LINEAGE_SETTLE")); configured != "" {
		if parsed, parseErr := time.ParseDuration(configured); parseErr == nil && parsed >= 0 {
			settle = parsed
		}
	}
	time.Sleep(settle)

	const branchPrompt = "Do not call tools. Reply with exactly CACHE_BRANCH_OK."
	sourceHotQuery, sourceHotProvider, sourceHotSystem := newRuntime(t, sourceRef.ID, sourceMeta.CacheLineageID)
	sourceHotQuery.SetMessages(seedHistory)
	sourceHot, sourceHotRequestLineage := run(t, sourceHotQuery, sourceHotProvider, branchPrompt)
	forkQuery, forkProvider, forkSystem := newRuntime(t, forkRef.ID, forkMeta.CacheLineageID)
	forkQuery.SetMessages(forkHistory)
	fork, forkRequestLineage := run(t, forkQuery, forkProvider, branchPrompt)

	evidence := liveCacheLineageEvidence{
		Version:          1,
		Provider:         spec.name,
		Model:            spec.model,
		Protocol:         spec.protocol,
		LineageInherited: forkRef.ID != sourceRef.ID && forkMeta.CacheLineageID == sourceMeta.CacheLineageID,
		RequestLineageMatched: coldRequestLineage == sourceMeta.CacheLineageID &&
			sourceHotRequestLineage == sourceMeta.CacheLineageID &&
			forkRequestLineage == forkMeta.CacheLineageID &&
			sourceHotRequestLineage == forkRequestLineage,
		Cold:      cold,
		SourceHot: sourceHot,
		Fork:      fork,
	}
	if sourceSystem != sourceHotSystem || sourceSystem != forkSystem {
		t.Fatal("source and fork runtimes rebuilt different system prompts")
	}
	if err := validateLiveCacheLineageEvidence(evidence, spec.slack, spec.requireCreation); err != nil {
		t.Fatalf("live cache verification failed: %v; evidence=%+v", err, evidence)
	}
	if recordDir := strings.TrimSpace(os.Getenv("CACHE_LINEAGE_RECORD_DIR")); recordDir != "" {
		if err := recordLiveCacheLineageEvidence(recordDir, evidence); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("live cache verified: provider=%s model=%s cold=%+v source_hot=%+v fork=%+v", spec.name, spec.model, cold.Usage, sourceHot.Usage, fork.Usage)
}

func validateLiveCacheLineageEvidence(evidence liveCacheLineageEvidence, slack int, requireCreation bool) error {
	if !evidence.LineageInherited {
		return fmt.Errorf("fork did not inherit the source cache lineage")
	}
	if !evidence.RequestLineageMatched {
		return fmt.Errorf("provider requests did not use the inherited cache lineage")
	}
	if evidence.SourceHot.ParamsSHA256 == "" || evidence.Fork.ParamsSHA256 == "" || evidence.SourceHot.ParamsSHA256 != evidence.Fork.ParamsSHA256 {
		return fmt.Errorf("source-hot and fork request envelopes differ")
	}
	if !evidence.Cold.CacheLineageEnabled || !evidence.SourceHot.CacheLineageEnabled || !evidence.Fork.CacheLineageEnabled {
		return fmt.Errorf("cold, source-hot, or fork request omitted cache lineage routing")
	}
	for name, run := range map[string]liveCacheRunEvidence{"cold": evidence.Cold, "source-hot": evidence.SourceHot, "fork": evidence.Fork} {
		if run.Usage.InputTokens <= 0 {
			return fmt.Errorf("%s request did not report input usage", name)
		}
	}
	if requireCreation && evidence.Cold.Usage.CacheCreationInputTokens <= 0 {
		return fmt.Errorf("cold request did not report cache creation")
	}
	if evidence.SourceHot.Usage.CacheReadInputTokens <= 0 || evidence.Fork.Usage.CacheReadInputTokens <= 0 {
		return fmt.Errorf("source-hot or fork request did not report a cache read")
	}
	expected := evidence.Cold.Usage.InputTokens
	if requireCreation && evidence.Cold.Usage.CacheCreationInputTokens > 0 {
		expected = evidence.Cold.Usage.CacheCreationInputTokens
	}
	minimum := expected - slack
	if minimum < 1 {
		minimum = 1
	}
	if evidence.SourceHot.Usage.CacheReadInputTokens < minimum || evidence.Fork.Usage.CacheReadInputTokens < minimum {
		return fmt.Errorf("cache read below minimum %d tokens", minimum)
	}
	drift := evidence.SourceHot.Usage.CacheReadInputTokens - evidence.Fork.Usage.CacheReadInputTokens
	if drift > slack || drift < -slack {
		return fmt.Errorf("source-hot/fork cache-read drift %d exceeds %d", drift, slack)
	}
	if evidence.SourceHot.SystemFingerprint != "" && evidence.Fork.SystemFingerprint != "" && evidence.SourceHot.SystemFingerprint != evidence.Fork.SystemFingerprint {
		return fmt.Errorf("source-hot/fork system fingerprints differ")
	}
	return nil
}

func recordLiveCacheLineageEvidence(recordDir string, evidence liveCacheLineageEvidence) error {
	if err := os.MkdirAll(recordDir, 0o700); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		return err
	}
	name := strings.NewReplacer("/", "-", "\\", "-").Replace(evidence.Provider) + ".json"
	return os.WriteFile(filepath.Join(recordDir, name), append(payload, '\n'), 0o600)
}

func TestValidateLiveCacheLineageEvidence(t *testing.T) {
	valid := liveCacheLineageEvidence{
		LineageInherited:      true,
		RequestLineageMatched: true,
		Cold:                  liveCacheRunEvidence{Usage: types.Usage{InputTokens: 1000, CacheCreationInputTokens: 900}, CacheLineageEnabled: true},
		SourceHot:             liveCacheRunEvidence{Usage: types.Usage{InputTokens: 1100, CacheReadInputTokens: 900}, ParamsSHA256: "same", CacheLineageEnabled: true},
		Fork:                  liveCacheRunEvidence{Usage: types.Usage{InputTokens: 1100, CacheReadInputTokens: 890}, ParamsSHA256: "same", CacheLineageEnabled: true},
	}
	if err := validateLiveCacheLineageEvidence(valid, 128, true); err != nil {
		t.Fatalf("valid evidence rejected: %v", err)
	}
	for _, mutate := range []func(*liveCacheLineageEvidence){
		func(value *liveCacheLineageEvidence) { value.LineageInherited = false },
		func(value *liveCacheLineageEvidence) { value.RequestLineageMatched = false },
		func(value *liveCacheLineageEvidence) { value.Fork.ParamsSHA256 = "different" },
		func(value *liveCacheLineageEvidence) { value.Fork.Usage.CacheReadInputTokens = 0 },
		func(value *liveCacheLineageEvidence) { value.Cold.Usage.CacheCreationInputTokens = 0 },
	} {
		candidate := valid
		mutate(&candidate)
		if err := validateLiveCacheLineageEvidence(candidate, 128, true); err == nil {
			t.Fatalf("invalid evidence accepted: %+v", candidate)
		}
	}
	recordDir := t.TempDir()
	valid.Provider = "openai-responses"
	valid.Model = "cache-test-model"
	valid.Protocol = "prompt_cache_key"
	if err := recordLiveCacheLineageEvidence(recordDir, valid); err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(filepath.Join(recordDir, "openai-responses.json"))
	if err != nil {
		t.Fatal(err)
	}
	var recorded liveCacheLineageEvidence
	if err := json.Unmarshal(payload, &recorded); err != nil {
		t.Fatal(err)
	}
	if recorded.Provider != valid.Provider || recorded.SourceHot.ParamsSHA256 != "same" || !recorded.LineageInherited || !recorded.RequestLineageMatched {
		t.Fatalf("recorded evidence = %+v", recorded)
	}
}

func liveSkillCatalogMessageCount(messages []types.Message) int {
	count := 0
	for _, message := range messages {
		if message.DeveloperMetadata != nil && message.DeveloperMetadata.Kind == types.DeveloperMessageKindSkillCatalogSnapshot {
			count++
		}
	}
	return count
}
