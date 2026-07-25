package skill

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	executioncontract "github.com/agent-dance/luban/internal/contracts/execution"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/tools/toolbase"
	"github.com/agent-dance/luban/skills"
	"github.com/agent-dance/luban/types"
)

// -----------------------------------------------------------------------
// SkillTool – the types.Tool implementation
// -----------------------------------------------------------------------

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
	// tool results. It is injected by runtime composition.
	LanguageResolver func(context.Context) i18n.Language

	// inflightOwners records which sessionID currently holds each skill slot.
	inflightMu     sync.Mutex
	inflightOwners map[string]string
}

// NewSkillTool creates a SkillTool using the default skill directories.
func NewSkillTool() *SkillTool {
	return &SkillTool{
		Manager:        skills.NewManager(skills.DefaultDirs()...),
		inflightOwners: make(map[string]string),
	}
}

func (t *SkillTool) Name() string { return "Skill" }

func (t *SkillTool) Description() string {
	return i18n.Text(t.language(context.Background()), i18n.KeyToolSkillDescription)
}

func (t *SkillTool) ToolMetadata(map[string]any) types.ToolMetadata {
	return types.ToolMetadata{}
}

func (t *SkillTool) Schema() types.JSONSchema {
	lang := t.language(context.Background())
	return types.JSONSchema{
		Type: "object",
		Properties: map[string]any{
			"skill": map[string]any{
				"type":        "string",
				"description": i18n.Text(lang, i18n.KeyToolSkillInputSelectorDescription),
			},
			"revision": map[string]any{
				"type":        "integer",
				"minimum":     1,
				"description": i18n.Text(lang, i18n.KeyToolSkillInputRevisionDescription),
			},
			"args": map[string]any{
				"type":        "string",
				"description": i18n.Text(lang, i18n.KeyToolSkillInputArgumentsDescription),
			},
		},
		Required: []string{"skill"},
	}
}

// validateSkillName enforces the Skill selector contract:
//   - if the name contains ":", it must be a plugin namespace ("plugin:skill")
//     with both sides non-empty and exactly one colon
//   - reject path-traversal segments ("..", "/", "\")
//   - otherwise the name must be non-empty
func validateSkillName(name string) error {
	if name == "" {
		return fmt.Errorf("'skill' parameter is required")
	}
	if strings.ContainsAny(name, `\/`) {
		return fmt.Errorf("skill name %q must not contain path separators", name)
	}
	for _, segment := range strings.Split(name, ":") {
		if segment == ".." {
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

// resolveSessionID returns the active QueryLoop-owned session. Exported
// ToolExecutionContext fields are not trusted because callers can construct
// them without the private owner capability.
func (t *SkillTool) resolveSessionID(ctx context.Context) string {
	if exec, ok := executioncontract.ToolExecutionContextFromContext(ctx); ok {
		if sessionID, _, _, _, active := exec.ActiveRuntimeOwnerIdentity(); active {
			return strings.TrimSpace(sessionID)
		}
	}
	return ""
}

// claimInflight marks (sessionID, name) as running. Returns the sessionID
// already holding the slot, or "" if the claim succeeded.
func (t *SkillTool) claimInflight(sessionID, name string) string {
	t.inflightMu.Lock()
	defer t.inflightMu.Unlock()
	if t.inflightOwners == nil {
		t.inflightOwners = make(map[string]string)
	}
	if owner, busy := t.inflightOwners[name]; busy {
		return owner
	}
	t.inflightOwners[name] = sessionID
	return ""
}

func (t *SkillTool) releaseInflight(sessionID, name string) {
	t.inflightMu.Lock()
	defer t.inflightMu.Unlock()
	if owner, ok := t.inflightOwners[name]; ok && owner == sessionID {
		delete(t.inflightOwners, name)
	}
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
	in, err := toolbase.ParseInput[skillToolInput](input)
	if err != nil {
		return skillErrorResult(skillErrInvalidFormat, i18n.Text(lang, i18n.KeySkillToolInvalidInput), "", nil), nil
	}
	_, revisionProvided := input["revision"]
	if revisionProvided && in.Revision == 0 {
		return skillErrorResult(skillErrInvalidFormat, i18n.Text(lang, i18n.KeySkillToolInvalidInput), in.Skill, nil), nil
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
	if exec, ok := executioncontract.ToolExecutionContextFromContext(ctx); ok {
		if generation, pinned := exec.SkillProjectGeneration(); pinned {
			request.ExpectedProjectGeneration = skills.ProjectSourceGeneration(generation)
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
			skillErrInvalidFormat,
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
	lang := t.language(ctx)
	if t == nil || t.Manager == nil {
		return skillErrorResult(skillErrLoadFailure, i18n.Text(lang, i18n.KeySkillToolUnavailable), request.Selector, nil), nil
	}
	if request.SessionID == "" {
		request.SessionID = t.resolveSessionID(ctx)
	}
	selector := strings.TrimSpace(request.Selector)
	if selector == "" {
		return skillErrorResult(skillErrInvalidFormat, i18n.Text(lang, i18n.KeySkillToolRequired), "", nil), nil
	}
	if !skills.SkillID(selector).IsValid() {
		if err := validateSkillName(selector); err != nil {
			return skillErrorResult(skillErrInvalidFormat, i18n.Text(lang, i18n.KeySkillToolInvalidSelector), selector, nil), nil
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
		if holder := t.claimInflight(request.SessionID, name); holder != "" {
			result := skillErrorResult(skillErrInvalidFormat, i18n.Format(lang, i18n.KeySkillToolRecursive, name, holder), name, nil)
			rejected = &result
			return errSkillInvocationRejected
		}
		claimedName = name

		body := skills.PrepareSkillContent(resolved.Skill, request.Arguments, request.SessionID)
		if len(resolved.Skill.AllowedTools) > 0 {
			body += "\n\n" + i18n.Format(lang, i18n.KeyToolSkillAllowedTools, strings.Join(resolved.Skill.AllowedTools, ", "))
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
		if skillHasOnlySafeProperties(resolved.Skill) {
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
		result := skillErrorResult(skillErrLoadFailure, i18n.Format(lang, i18n.KeySkillToolRegistryFailure, selector), selector, nil)
		mergeSkillResolveMetadata(result.Metadata, resolveResult, request.Origin)
		return result, nil
	}
	if resolveResult.Outcome != skills.SkillResolveResolved {
		return t.resolveRejection(lang, request, resolveResult), nil
	}
	defer t.releaseInflight(request.SessionID, claimedName)

	skill := prepared.resolved.Skill
	effective := prepared.resolved.Effective
	status := "inline"
	if skill.Context == skills.ContextFork {
		status = "forked"
	}
	resolvedModel := skill.Model

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
	metadata["projectGeneration"] = strconv.FormatUint(uint64(request.ExpectedProjectGeneration), 10)
	for key, value := range prepared.receiptMetadata {
		metadata[key] = value
	}
	if resolvedModel != "" {
		metadata["model"] = resolvedModel
	}
	if skill.Effort != "" {
		metadata["effort"] = skill.Effort
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

	var code skillErrorCode
	var message string
	switch result.Outcome {
	case skills.SkillResolveNotFound:
		code = skillErrUnknownSkill
		message = i18n.Format(lang, i18n.KeySkillToolNotFound, request.Selector)
		available := t.availableSkillNames(request.SessionID, request.Origin)
		if len(available) > 0 {
			message += i18n.Format(lang, i18n.KeySkillToolAvailable, strings.Join(available, ", "))
		} else {
			message += i18n.Text(lang, i18n.KeySkillToolNoneInstalled)
		}
	case skills.SkillResolveAmbiguous:
		code = skillErrInvalidFormat
		message = i18n.Format(lang, i18n.KeySkillToolAmbiguous, request.Selector, joinSkillIDs(result.Candidates))
	case skills.SkillResolveShadowed:
		code = skillErrDisableModelInvoke
		shadowedBy := skills.SkillID("")
		if result.Resolved != nil {
			shadowedBy = result.Resolved.Effective.ShadowedBy
		}
		message = i18n.Format(lang, i18n.KeySkillToolShadowed, name, shadowedBy)
	case skills.SkillResolveStale:
		code = skillErrInvalidFormat
		var revision skills.SkillRevision
		if result.Resolved != nil {
			revision = result.Resolved.Effective.Revision
		}
		message = i18n.Format(lang, i18n.KeySkillToolStale, name, revision)
	case skills.SkillResolvePolicyDenied:
		code = skillErrDisableModelInvoke
		extra["availability"] = "disabled"
		if request.Origin == skills.InvocationOriginUser {
			message = i18n.Format(lang, i18n.KeySkillToolPolicyDeniedUser, name)
		} else {
			message = i18n.Format(lang, i18n.KeySkillToolPolicyDeniedModel, name)
		}
	default:
		code = skillErrLoadFailure
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
// numeric errorCode plus a stable label.
func skillErrorResult(code skillErrorCode, msg, name string, extra map[string]string) types.ToolResult {
	md := map[string]string{
		"success":     "false",
		"errorCode":   fmt.Sprintf("%d", int(code)),
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

func boolToString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
