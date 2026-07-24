package tools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/google/uuid"

	"github.com/agent-dance/luban/compact"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/loop"
	"github.com/agent-dance/luban/skills"
	"github.com/agent-dance/luban/types"
)

// -----------------------------------------------------------------------
// SkillTool – the types.Tool implementation
// -----------------------------------------------------------------------

// skillCtxKey is a context-value key used by callers (the loop) to inject
// the current session id without coupling to a global. It is unexported so
// callers must go through WithSkillSessionID.
type skillCtxKey struct{}

// skillToolUseCtxKey carries the current tool_use id from the harness so
// the tool can tag its messages for compaction state. Optional.
type skillToolUseCtxKey struct{}

var errSkillInvocationRejected = errors.New("skill invocation rejected")

// SkillInvocationRequest is the transport-neutral entry point used by
// explicit user surfaces. Execute adapts model tool input to the same path.
type SkillInvocationRequest struct {
	SessionID                 string
	Selector                  string
	ExpectedRevision          skills.SkillRevision
	ExpectedProjectGeneration skills.ProjectSourceGeneration
	Origin                    skills.InvocationOrigin
	Arguments                 *string
}

// SkillLoadedLedgerState is one atomic view of the current context epoch and
// the body digest previously proven visible in that epoch. A mismatched or
// zero LoadedContextEpoch is treated as not loaded.
type SkillLoadedLedgerState struct {
	ContextEpoch       uint64
	LoadedContextEpoch uint64
	ContentDigest      skills.SkillDigest
	PayloadDigest      skills.InvocationPayloadDigest
}

// SkillLoadedLedgerResolver returns an immutable ledger view for one stable
// skill ID. SkillTool only reads this state; task_19 commits receipts after the
// corresponding tool result enters visible history.
type SkillLoadedLedgerResolver func(context.Context, string, skills.SkillID) SkillLoadedLedgerState

// WithSkillSessionID returns a child context that carries the given session
// id for the SkillTool. SkillTool.Execute reads the value via this key.
func WithSkillSessionID(ctx context.Context, sessionID string) context.Context {
	return context.WithValue(ctx, skillCtxKey{}, sessionID)
}

// WithSkillToolUseID returns a child context that carries the model's
// tool_use id for the current Skill invocation. The SkillTool records this
// in the per-session "invoked skills" log so /compact can preserve which
// skills ran. Mirrors TS tagMessagesWithToolUseID.
func WithSkillToolUseID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, skillToolUseCtxKey{}, id)
}

// SkillTool wires skills.Manager into the tool executor interface.
// It consumes the skills.Manager from the skills/ package for loading,
// parsing, and discovering skills, while handling tool execution here.
type SkillTool struct {
	Manager *skills.Manager

	// LoadedLedgerResolver supplies the visible-context ledger used to choose
	// full, already-loaded, or superseding invocation envelopes. When unset or
	// invalid, SkillTool safely emits a full envelope without a receipt.
	LoadedLedgerResolver SkillLoadedLedgerResolver

	// LanguageResolver returns the active runtime language for user-visible
	// tool results. It is injected by runtime composition; isolated legacy
	// callers retain the historical English fallback.
	LanguageResolver func(context.Context) i18n.Language

	// SessionIDResolver, when set, takes priority over the context value
	// and the CLAUDE_SESSION_ID env var. When the resolver returns an empty
	// string the next fallback is consulted.
	SessionIDResolver func(context.Context) string

	// FallbackSessionID is used when no other source yields a non-empty id.
	// When unset, a process-wide UUID is generated lazily.
	FallbackSessionID string

	// ParentModelResolver returns the parent session's current model name
	// (e.g. "sonnet[1m]") so a skill's `model:` frontmatter override can
	// preserve any tier suffix. Optional; when nil or returning "", the
	// skill model is used verbatim.
	ParentModelResolver func(context.Context) string

	// AllowRules / DenyRules are evaluated against the requested skill
	// name BEFORE execution. Rules support exact-name and "plugin:*"
	// prefix forms (see MatchSkillRule). Deny rules win.
	//
	// In practice the harness populates these from the user's permission
	// settings; tests set them directly.
	AllowRules []string
	DenyRules  []string

	// AllowedToolsApplier, if set, is invoked before content rendering with
	// the skill's allowed-tools list so the harness can register them as
	// session-scoped allow rules. The returned cleanup runs after the tool
	// returns. When nil, allowed-tools are still surfaced via Metadata for
	// observers (legacy behavior preserved).
	AllowedToolsApplier func(ctx context.Context, sessionID, skillName string, tools []string) (cleanup func())

	// UsageStore records per-skill invocation counts when set. Wired by
	// registry_setup; tests can pass an in-memory store.
	UsageStore *SkillUsageStore

	// inflightMu guards the inflight maps. inflight is the legacy
	// per-process flat shape (skill name → present), preserved for backwards
	// compatibility with existing tests that initialise this field directly.
	// inflightOwners records which sessionID currently holds the slot for
	// each skill name and powers the InflightSession() observer (skill-05).
	inflightMu     sync.Mutex
	inflight       map[string]struct{}
	inflightOwners map[string]string

	// invokedSkills records the (sessionID, skillName, toolUseID) triples
	// for skills that have run in this session, so /compact can preserve a
	// summary in the compaction state. Mirrors TS addInvokedSkill.
	invokedMu     sync.Mutex
	invokedSkills map[string][]InvokedSkillRecord

	// fallbackOnce guards lazy initialisation of the per-process fallback
	// session id when neither the resolver, env var nor explicit override
	// produced a value.
	fallbackOnce sync.Once
	fallbackID   string
}

// InvokedSkillRecord captures one skill invocation for compaction-state
// purposes. ToolUseID matches the model's tool_use block id; SkillName is
// the resolved name (post-leading-slash strip).
type InvokedSkillRecord struct {
	SkillName string
	ToolUseID string
	Source    skills.SkillSource
}

// NewSkillTool creates a SkillTool using the default skill directories.
func NewSkillTool() *SkillTool {
	return &SkillTool{
		Manager:        skills.NewManager(skills.DefaultDirs()...),
		inflight:       make(map[string]struct{}),
		inflightOwners: make(map[string]string),
	}
}

func (t *SkillTool) Name() string { return "Skill" }

func (t *SkillTool) Description() string {
	return skills.GetSkillToolPrompt()
}

func (t *SkillTool) Schema() types.JSONSchema {
	return types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"skill": map[string]any{
				"type":        "string",
				"description": `Stable skill ID from the latest catalog, or an unambiguous skill name for compatibility. Do not include a leading slash.`,
			},
			"revision": map[string]any{
				"type":        "integer",
				"minimum":     1,
				"description": "Optional per-skill revision observed in the latest catalog. A changed revision is rejected instead of executing stale content.",
			},
			"args": map[string]any{
				"type":        "string",
				"description": "Optional arguments forwarded to the skill body. The string is substituted into the skill's argument-hint placeholders ($ARGUMENTS, $0, named args from the skill's `arguments:` frontmatter); when no placeholder is present the value is appended as `ARGUMENTS: <args>` so the skill always sees what the caller forwarded.",
			},
		},
		Required: []string{"skill"},
	}
}

// validateSkillName enforces the contract documented for Claude:
//   - if the name contains ":", it must be a plugin namespace ("plugin:skill")
//     with both sides non-empty and exactly one colon
//   - reject path-traversal segments ("..", "/", "\")
//   - otherwise the name must be non-empty
//
// Note: a leading "/" is NOT a hard error — the caller (Execute) is
// expected to call normalizeSkillName first, which strips the slash and
// records a tengu_skill_leading_slash analytics event. This mirrors TS
// behavior where slash-prefixed names recover gracefully instead of
// erroring out the model.
func validateSkillName(name string) error {
	if name == "" {
		return fmt.Errorf("'skill' parameter is required")
	}
	if strings.HasPrefix(name, "/") {
		return fmt.Errorf("skill name %q must not include a leading slash", name)
	}
	// skill-leading-slash-strip-with-event: TS strips a leading slash and
	// emits a tengu_skill_leading_slash analytics event rather than
	// hard-failing. This recovers gracefully from a typo like "/commit".
	// Stripping is done in the entry path (Execute / normalizeSkillName);
	// this validator now only rejects the lingering edge case where the
	// strip didn't happen (defensive only). Path-traversal still fails.
	if strings.ContainsAny(name, `\/`) {
		return fmt.Errorf("skill name %q must not contain path separators", name)
	}
	for _, segment := range strings.Split(name, ":") {
		if segment == ".." || strings.HasPrefix(segment, "../") || strings.Contains(segment, "/..") {
			return fmt.Errorf("skill name %q must not contain path traversal segments", name)
		}
	}
	if strings.Contains(name, ":") {
		parts := strings.Split(name, ":")
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return fmt.Errorf("plugin-namespaced skill name %q must be of the form \"plugin:skill\" with both sides non-empty", name)
		}
	}
	return nil
}

// normalizeSkillName trims whitespace and strips a single leading slash if
// present. It returns the normalized name and a flag indicating whether a
// slash was stripped (callers may emit a tengu_skill_leading_slash event).
// This mirrors TS validateInput which logs the event and proceeds.
func normalizeSkillName(name string) (string, bool) {
	trimmed := strings.TrimSpace(name)
	if strings.HasPrefix(trimmed, "/") {
		return strings.TrimPrefix(trimmed, "/"), true
	}
	return trimmed, false
}

// resolveSessionID returns the session id used to scope inflight bookkeeping
// and to substitute ${CLAUDE_SESSION_ID} in the skill body. The resolution
// order is:
//
//  1. Context value set via WithSkillSessionID
//  2. SessionIDResolver, if non-nil
//  3. CLAUDE_SESSION_ID env var
//  4. FallbackSessionID, if non-empty
//  5. A lazily-generated per-process UUID (so the variable is never blank)
func (t *SkillTool) resolveSessionID(ctx context.Context) string {
	if ctx != nil {
		if v, ok := ctx.Value(skillCtxKey{}).(string); ok {
			if trimmed := strings.TrimSpace(v); trimmed != "" {
				return trimmed
			}
		}
	}
	if t.SessionIDResolver != nil {
		if v := strings.TrimSpace(t.SessionIDResolver(ctx)); v != "" {
			return v
		}
	}
	if v := strings.TrimSpace(os.Getenv("CLAUDE_SESSION_ID")); v != "" {
		return v
	}
	if v := strings.TrimSpace(t.FallbackSessionID); v != "" {
		return v
	}
	t.fallbackOnce.Do(func() {
		t.fallbackID = "skill-" + uuid.NewString()
	})
	return t.fallbackID
}

// InflightSession returns the session id currently holding the inflight
// guard for the given skill name, or "" if no session is running it.
// Per-session, not per-process (skill-05).
func (t *SkillTool) InflightSession(skill string) string {
	t.inflightMu.Lock()
	defer t.inflightMu.Unlock()
	if t.inflightOwners == nil {
		// Legacy callers may have populated `inflight` directly without an
		// owner. Fall back to "unknown" so the slot is still observable.
		if _, busy := t.inflight[skill]; busy {
			return "unknown"
		}
		return ""
	}
	if owner, ok := t.inflightOwners[skill]; ok {
		return owner
	}
	if _, busy := t.inflight[skill]; busy {
		return "unknown"
	}
	return ""
}

// claimInflight marks (sessionID, name) as running. Returns the sessionID
// already holding the slot, or "" if the claim succeeded. The legacy flat
// `inflight` map is updated in lockstep with the per-session owners map so
// existing call-sites that pre-populate `inflight` continue to work.
func (t *SkillTool) claimInflight(sessionID, name string) string {
	t.inflightMu.Lock()
	defer t.inflightMu.Unlock()
	if t.inflight == nil {
		t.inflight = make(map[string]struct{})
	}
	if t.inflightOwners == nil {
		t.inflightOwners = make(map[string]string)
	}
	if _, busy := t.inflight[name]; busy {
		if owner, ok := t.inflightOwners[name]; ok {
			return owner
		}
		return "unknown"
	}
	t.inflight[name] = struct{}{}
	t.inflightOwners[name] = sessionID
	return ""
}

func (t *SkillTool) releaseInflight(sessionID, name string) {
	t.inflightMu.Lock()
	defer t.inflightMu.Unlock()
	if t.inflightOwners != nil {
		if owner, ok := t.inflightOwners[name]; ok && owner == sessionID {
			delete(t.inflightOwners, name)
			delete(t.inflight, name)
			return
		}
	}
	// Best-effort cleanup for legacy paths that never set an owner.
	delete(t.inflight, name)
}

type skillToolInput struct {
	Skill    string `json:"skill"`
	Args     string `json:"args"`
	Revision uint64 `json:"revision,omitempty"`
}

func (t *SkillTool) Execute(ctx context.Context, input map[string]any) (types.ToolResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	lang := t.language(ctx)
	in, err := parseInput[skillToolInput](input)
	if err != nil {
		return skillErrorResult(SkillErrInvalidFormat, i18n.Text(lang, i18n.KeySkillToolInvalidInput), "", nil), nil
	}
	_, revisionProvided := input["revision"]
	if revisionProvided && in.Revision == 0 {
		return skillErrorResult(SkillErrInvalidFormat, i18n.Text(lang, i18n.KeySkillToolInvalidInput), in.Skill, nil), nil
	}

	var arguments *string
	if _, provided := input["args"]; provided {
		value := in.Args
		arguments = &value
	}
	request := SkillInvocationRequest{
		SessionID:        t.resolveSessionID(ctx),
		Selector:         in.Skill,
		ExpectedRevision: skills.SkillRevision(in.Revision),
		Origin:           skills.InvocationOriginModel,
		Arguments:        arguments,
	}
	if exec, ok := loop.ToolExecutionContextFromContext(ctx); ok {
		if generation, pinned := exec.SkillProjectGeneration(); pinned {
			request.ExpectedProjectGeneration = generation
		}
	}
	return t.invoke(ctx, request)
}

// Invoke executes one explicit user request against the latest registry
// state. Keeping Origin on the request makes omission and confused-deputy
// adapters fail closed instead of silently upgrading them to user authority.
func (t *SkillTool) Invoke(ctx context.Context, request SkillInvocationRequest) (types.ToolResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	lang := t.language(ctx)
	if request.Origin != skills.InvocationOriginUser {
		return skillErrorResult(
			SkillErrInvalidFormat,
			i18n.Text(lang, i18n.KeySkillToolExplicitUserOrigin),
			request.Selector,
			map[string]string{"invocationOrigin": string(request.Origin)},
		), nil
	}
	return t.invoke(ctx, request)
}

// invoke is the shared authoritative path. Execute is the only model-origin
// adapter; Invoke is the only exported explicit-user adapter.
func (t *SkillTool) invoke(ctx context.Context, request SkillInvocationRequest) (types.ToolResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	lang := t.language(ctx)
	if t == nil || t.Manager == nil {
		return skillErrorResult(SkillErrLoadFailure, i18n.Text(lang, i18n.KeySkillToolUnavailable), request.Selector, nil), nil
	}
	if request.SessionID == "" {
		request.SessionID = t.resolveSessionID(ctx)
	}
	selector, slashStripped := normalizeSkillName(request.Selector)
	if selector == "" {
		return skillErrorResult(SkillErrInvalidFormat, i18n.Text(lang, i18n.KeySkillToolRequired), "", nil), nil
	}
	if !skills.SkillID(selector).IsValid() {
		if err := validateSkillName(selector); err != nil {
			return skillErrorResult(SkillErrInvalidFormat, i18n.Text(lang, i18n.KeySkillToolInvalidSelector), selector, nil), nil
		}
	}
	if request.Arguments != nil {
		value := *request.Arguments
		request.Arguments = &value
	}
	request.Selector = selector

	type preparedInvocation struct {
		resolved           skills.ResolvedSkill
		envelope           string
		envelopeKind       skills.InvocationEnvelopeKind
		payloadDigest      skills.InvocationPayloadDigest
		receiptMetadata    map[string]string
		permissionDecision string
	}
	var prepared preparedInvocation
	var rejected *types.ToolResult
	claimedName := ""
	resolveResult, resolveErr := t.Manager.ResolveLatest(skills.SkillResolveRequest{
		SessionID:                 request.SessionID,
		Selector:                  selector,
		ExpectedRevision:          request.ExpectedRevision,
		ExpectedProjectGeneration: request.ExpectedProjectGeneration,
		Origin:                    request.Origin,
	}, func(resolved skills.ResolvedSkill) error {
		if err := resolved.Validate(); err != nil {
			return err
		}
		name := resolved.Effective.Name
		if matched := FirstMatchingSkillRule(t.DenyRules, name); matched != "" {
			result := skillErrorResult(SkillErrInvalidFormat, i18n.Format(lang, i18n.KeySkillToolDenyRule, name, matched), name, map[string]string{
				"permissionDecision": "deny",
				"matchedRule":        matched,
			})
			rejected = &result
			return errSkillInvocationRejected
		}
		if holder := t.claimInflight(request.SessionID, name); holder != "" {
			result := skillErrorResult(SkillErrInvalidFormat, i18n.Format(lang, i18n.KeySkillToolRecursive, name, holder), name, nil)
			rejected = &result
			return errSkillInvocationRejected
		}
		claimedName = name

		body := skills.PrepareSkillContent(resolved.Skill, request.Arguments, request.SessionID)
		if len(resolved.Skill.AllowedTools) > 0 {
			body += "\n\nAllowed tools: " + strings.Join(resolved.Skill.AllowedTools, ", ")
		}
		arguments := skills.NewInvocationArguments(request.Arguments)
		payloadDigest := skills.DigestInvocationPayload(body)
		kind := skills.InvocationEnvelopeFull
		envelope := ""
		ledger := SkillLoadedLedgerState{}
		if t.LoadedLedgerResolver != nil {
			ledger = t.LoadedLedgerResolver(ctx, request.SessionID, resolved.Effective.ID)
		}
		loadedCurrent := ledger.ContextEpoch != 0 && ledger.LoadedContextEpoch == ledger.ContextEpoch &&
			ledger.ContentDigest.Validate() == nil && ledger.PayloadDigest.Validate() == nil
		var err error
		switch {
		case loadedCurrent && ledger.ContentDigest == resolved.Effective.Digest && ledger.PayloadDigest == payloadDigest:
			kind = skills.InvocationEnvelopeAlreadyLoaded
			envelope, err = skills.RenderLoadedDigestAcknowledgement(
				resolved.Effective, ledger.ContentDigest, ledger.PayloadDigest, body, arguments,
			)
		case loadedCurrent && ledger.ContentDigest != resolved.Effective.Digest:
			kind = skills.InvocationEnvelopeSuperseding
			envelope, err = skills.RenderSupersedingInvocationEnvelope(resolved.Effective, ledger.ContentDigest, body, arguments)
		default:
			envelope, err = skills.RenderFullInvocationEnvelope(resolved.Effective, body, arguments)
		}
		if err != nil {
			return err
		}

		var receiptMetadata map[string]string
		if ledger.ContextEpoch != 0 {
			receiptMetadata, err = skills.EncodeSkillExecutionReceiptMetadata(skills.SkillExecutionReceipt{
				ContextEpoch:            ledger.ContextEpoch,
				SkillID:                 resolved.Effective.ID,
				ContentDigest:           resolved.Effective.Digest,
				InvocationPayloadDigest: payloadDigest,
				InvocationEnvelopeKind:  kind,
			})
			if err != nil {
				return err
			}
		}

		permissionDecision := "ask"
		if FirstMatchingSkillRule(t.AllowRules, name) != "" || skillHasOnlySafeProperties(resolved.Skill) {
			permissionDecision = "allow"
		}
		prepared = preparedInvocation{
			resolved: resolved, envelope: envelope, envelopeKind: kind,
			payloadDigest: payloadDigest, receiptMetadata: receiptMetadata,
			permissionDecision: permissionDecision,
		}
		return nil
	})

	if resolveErr != nil || resolveResult.Outcome != skills.SkillResolveResolved {
		if claimedName != "" {
			t.releaseInflight(request.SessionID, claimedName)
			claimedName = ""
		}
	}
	if errors.Is(resolveErr, errSkillInvocationRejected) && rejected != nil {
		mergeSkillResolveMetadata(rejected.Metadata, resolveResult, request.Origin)
		return *rejected, nil
	}
	if resolveErr != nil {
		result := skillErrorResult(SkillErrLoadFailure, i18n.Format(lang, i18n.KeySkillToolRegistryFailure, selector), selector, nil)
		mergeSkillResolveMetadata(result.Metadata, resolveResult, request.Origin)
		return result, nil
	}
	if resolveResult.Outcome != skills.SkillResolveResolved {
		return t.resolveRejection(lang, request, resolveResult), nil
	}
	defer t.releaseInflight(request.SessionID, claimedName)

	skill := prepared.resolved.Skill
	effective := prepared.resolved.Effective
	if len(skill.AllowedTools) > 0 && t.AllowedToolsApplier != nil {
		if cleanup := t.AllowedToolsApplier(ctx, request.SessionID, effective.Name, skill.AllowedTools); cleanup != nil {
			defer cleanup()
		}
	}

	status := "inline"
	if skill.Context == skills.ContextFork {
		status = "forked"
	}
	resolvedModel := skill.Model
	if resolvedModel != "" {
		parent := ""
		if t.ParentModelResolver != nil {
			parent = t.ParentModelResolver(ctx)
		}
		if parent == "" {
			parent = strings.TrimSpace(os.Getenv("CLAUDE_MAIN_MODEL"))
		}
		resolvedModel = resolveSkillModelOverride(skill.Model, parent)
	}

	metadata := map[string]string{
		"success":            "true",
		"commandName":        effective.Name,
		"status":             status,
		"allowedTools":       strings.Join(skill.AllowedTools, ", "),
		"argumentsProvided":  boolToString(request.Arguments != nil),
		"sessionID":          request.SessionID,
		"permissionDecision": prepared.permissionDecision,
		"loadedFrom":         string(effective.Source),
		"registryOutcome":    string(resolveResult.Outcome),
		"catalogRevision":    strconv.FormatUint(uint64(resolveResult.CatalogRevision), 10),
		"skillID":            string(effective.ID),
		"skillRevision":      strconv.FormatUint(uint64(effective.Revision), 10),
		"skillDigest":        string(effective.Digest),
		"invocationOrigin":   string(request.Origin),
		"envelopeKind":       string(prepared.envelopeKind),
		"payloadDigest":      string(prepared.payloadDigest),
	}
	if request.ExpectedProjectGeneration != 0 {
		metadata["projectGeneration"] = strconv.FormatUint(uint64(request.ExpectedProjectGeneration), 10)
	}
	for key, value := range prepared.receiptMetadata {
		metadata[key] = value
	}
	if resolvedModel != "" {
		metadata["model"] = resolvedModel
	}
	if skill.Effort != "" {
		metadata["effort"] = skill.Effort
	}
	if slashStripped {
		metadata["leadingSlashStripped"] = "true"
	}
	toolUseID := ""
	if value, ok := ctx.Value(skillToolUseCtxKey{}).(string); ok {
		toolUseID = value
		metadata["toolUseID"] = value
	}

	t.recordInvocation(request.SessionID, InvokedSkillRecord{
		SkillName: effective.Name,
		ToolUseID: toolUseID,
		Source:    effective.Source,
	})
	if t.UsageStore != nil {
		t.UsageStore.Record(effective.Name)
	}
	return types.ToolResult{Content: prepared.envelope, Metadata: metadata}, nil
}

func (t *SkillTool) language(ctx context.Context) i18n.Language {
	if t != nil && t.LanguageResolver != nil {
		return t.LanguageResolver(ctx)
	}
	// Runtime composition injects the active language. Standalone embedders use
	// the persisted or detected runtime language as their fallback.
	return i18n.DetectOrLoadLanguage()
}

func (t *SkillTool) resolveRejection(lang i18n.Language, request SkillInvocationRequest, result skills.SkillResolveResult) types.ToolResult {
	name := request.Selector
	if result.Resolved != nil && result.Resolved.Effective.Name != "" {
		name = result.Resolved.Effective.Name
	}
	extra := map[string]string{"status": "inline"}
	mergeSkillResolveMetadata(extra, result, request.Origin)

	var code SkillErrorCode
	var message string
	switch result.Outcome {
	case skills.SkillResolveNotFound:
		code = SkillErrUnknownSkill
		message = i18n.Format(lang, i18n.KeySkillToolNotFound, request.Selector)
		available := t.availableSkillNames(request.SessionID, request.Origin)
		if len(available) > 0 {
			message += i18n.Format(lang, i18n.KeySkillToolAvailable, strings.Join(available, ", "))
		} else {
			message += i18n.Text(lang, i18n.KeySkillToolNoneInstalled)
		}
	case skills.SkillResolveAmbiguous:
		code = SkillErrInvalidFormat
		message = i18n.Format(lang, i18n.KeySkillToolAmbiguous, request.Selector, joinSkillIDs(result.Candidates))
	case skills.SkillResolveShadowed:
		code = SkillErrDisableModelInvoke
		shadowedBy := skills.SkillID("")
		if result.Resolved != nil {
			shadowedBy = result.Resolved.Effective.ShadowedBy
		}
		message = i18n.Format(lang, i18n.KeySkillToolShadowed, name, shadowedBy)
	case skills.SkillResolveStale:
		code = SkillErrInvalidFormat
		var revision skills.SkillRevision
		if result.Resolved != nil {
			revision = result.Resolved.Effective.Revision
		}
		message = i18n.Format(lang, i18n.KeySkillToolStale, name, revision)
	case skills.SkillResolvePolicyDenied:
		code = SkillErrDisableModelInvoke
		extra["availability"] = "disabled"
		if request.Origin == skills.InvocationOriginUser {
			message = i18n.Format(lang, i18n.KeySkillToolPolicyDeniedUser, name)
		} else {
			message = i18n.Format(lang, i18n.KeySkillToolPolicyDeniedModel, name)
		}
	default:
		code = SkillErrLoadFailure
		message = i18n.Format(lang, i18n.KeySkillToolRegistryFailure, name)
	}
	return skillErrorResult(code, message, name, extra)
}

func (t *SkillTool) availableSkillNames(sessionID string, origin skills.InvocationOrigin) []string {
	snapshot, err := t.Manager.Snapshot(sessionID)
	if err != nil {
		return nil
	}
	unique := make(map[string]struct{})
	for _, candidate := range snapshot.Skills {
		allowed := candidate.Executable && candidate.ShadowedBy == ""
		if origin == skills.InvocationOriginModel {
			allowed = allowed && candidate.ModelVisible
		} else if origin == skills.InvocationOriginUser {
			allowed = allowed && candidate.UserInvocable
		} else {
			allowed = false
		}
		if allowed {
			unique[candidate.Name] = struct{}{}
		}
	}
	names := make([]string, 0, len(unique))
	for name := range unique {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func mergeSkillResolveMetadata(metadata map[string]string, result skills.SkillResolveResult, origin skills.InvocationOrigin) {
	if metadata == nil {
		return
	}
	metadata["registryOutcome"] = string(result.Outcome)
	metadata["catalogRevision"] = strconv.FormatUint(uint64(result.CatalogRevision), 10)
	metadata["invocationOrigin"] = string(origin)
	if len(result.Candidates) > 0 {
		metadata["candidateSkillIDs"] = joinSkillIDs(result.Candidates)
	}
	if result.Resolved != nil {
		metadata["skillID"] = string(result.Resolved.Effective.ID)
		metadata["skillRevision"] = strconv.FormatUint(uint64(result.Resolved.Effective.Revision), 10)
		metadata["skillDigest"] = string(result.Resolved.Effective.Digest)
	}
}

func joinSkillIDs(ids []skills.SkillID) string {
	values := make([]string, len(ids))
	for index, id := range ids {
		values[index] = string(id)
	}
	return strings.Join(values, ", ")
}

// skillErrorResult builds a structured error ToolResult that includes the
// numeric errorCode plus a stable label, mirroring TS errorCode behavior.
func skillErrorResult(code SkillErrorCode, msg, name string, extra map[string]string) types.ToolResult {
	md := map[string]string{
		"success":     "false",
		"errorCode":   fmt.Sprintf("%d", code.Int()),
		"errorReason": code.String(),
	}
	if name != "" {
		md["commandName"] = name
	}
	for k, v := range extra {
		md[k] = v
	}
	return types.ToolResult{Content: msg, IsError: true, Metadata: md}
}

// recordInvocation appends a record to the per-session invoked-skill log.
func (t *SkillTool) recordInvocation(sessionID string, rec InvokedSkillRecord) {
	if sessionID == "" || rec.SkillName == "" {
		return
	}
	t.invokedMu.Lock()
	defer t.invokedMu.Unlock()
	if t.invokedSkills == nil {
		t.invokedSkills = make(map[string][]InvokedSkillRecord)
	}
	t.invokedSkills[sessionID] = append(t.invokedSkills[sessionID], rec)
}

// InvokedSkills returns the recorded invocations for the given session, in
// insertion order. Used by /compact to preserve which skills ran across a
// summarisation boundary.
func (t *SkillTool) InvokedSkills(sessionID string) []InvokedSkillRecord {
	t.invokedMu.Lock()
	defer t.invokedMu.Unlock()
	if recs, ok := t.invokedSkills[sessionID]; ok {
		out := make([]InvokedSkillRecord, len(recs))
		copy(out, recs)
		return out
	}
	return nil
}

func (t *SkillTool) PostCompactInvokedSkills(sessionID string) []compact.InvokedSkillSnapshot {
	records := t.InvokedSkills(sessionID)
	if len(records) == 0 {
		return nil
	}
	out := make([]compact.InvokedSkillSnapshot, 0, len(records))
	for _, record := range records {
		out = append(out, compact.InvokedSkillSnapshot{
			Name:      record.SkillName,
			ToolUseID: record.ToolUseID,
			Source:    string(record.Source),
		})
	}
	return out
}

func boolToString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
