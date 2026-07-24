package skills

import (
	"errors"
	"fmt"
)

// ErrInvalidCatalogPolicy identifies malformed or cross-skill policy input.
var ErrInvalidCatalogPolicy = errors.New("invalid skill catalog policy")

// CatalogPolicyReason is a stable, non-localized explanation for one policy
// decision. Presentation layers must translate these semantic codes instead of
// rendering them directly.
type CatalogPolicyReason string

const (
	CatalogPolicyReasonDefault                  CatalogPolicyReason = "default"
	CatalogPolicyReasonFrontmatter              CatalogPolicyReason = "frontmatter"
	CatalogPolicyReasonUserOverride             CatalogPolicyReason = "user-override"
	CatalogPolicyReasonProjectOverride          CatalogPolicyReason = "project-override"
	CatalogPolicyReasonSessionOverride          CatalogPolicyReason = "session-override"
	CatalogPolicyReasonManagedDeny              CatalogPolicyReason = "managed-deny"
	CatalogPolicyReasonManagedReadOnly          CatalogPolicyReason = "managed-read-only"
	CatalogPolicyReasonNameOnly                 CatalogPolicyReason = "visibility-name-only"
	CatalogPolicyReasonManualOnly               CatalogPolicyReason = "visibility-manual-only"
	CatalogPolicyReasonOff                      CatalogPolicyReason = "visibility-off"
	CatalogPolicyReasonFrontmatterModelDisabled CatalogPolicyReason = "frontmatter-model-disabled"
	CatalogPolicyReasonFrontmatterUserDisabled  CatalogPolicyReason = "frontmatter-user-disabled"
	CatalogPolicyReasonNoInvocationOrigin       CatalogPolicyReason = "no-invocation-origin"
)

// CatalogPolicyInput contains only the policy facts needed to evaluate one
// stable skill. It deliberately excludes prompt history: previously announced
// text is never an authorization input.
type CatalogPolicyInput struct {
	SkillID SkillID

	// Defaults describe source-level capabilities before frontmatter or
	// visibility overrides. For example, a plugin without discovery metadata
	// can default to not model-visible while remaining user-invocable.
	DefaultModelVisible  bool
	DefaultUserInvocable bool

	// FrontmatterDisableModelInvocation corresponds to
	// disable-model-invocation. FrontmatterUserInvocable is nil when the field
	// is absent and therefore inherits DefaultUserInvocable.
	FrontmatterDisableModelInvocation bool
	FrontmatterUserInvocable          *bool

	// Persistent layers remain separate so project policy can override user
	// policy without destroying either stored value.
	UserOverride    *VisibilityOverride
	ProjectOverride *VisibilityOverride
	SessionOverride *VisibilityOverride

	// A managed deny is the only visibility layer that lower scopes can never
	// relax. ManagedReadOnly can independently make an otherwise usable skill
	// immutable to local management surfaces.
	ManagedDeny     bool
	ManagedReadOnly bool
}

// CatalogPolicyDecision is the complete pure evaluation result consumed by
// registry producers. Executable means at least one permitted invocation
// origin remains; callers must still check ModelVisible versus UserInvocable
// for the actual invocation origin.
type CatalogPolicyDecision struct {
	Visibility       Visibility
	VisibilitySource SkillScope

	ModelVisible       bool
	ModelReason        CatalogPolicyReason
	DescriptionVisible bool
	DescriptionReason  CatalogPolicyReason
	UserInvocable      bool
	UserReason         CatalogPolicyReason
	Executable         bool
	ExecutionReason    CatalogPolicyReason

	Mutable        bool
	ReadOnlyReason CatalogPolicyReason
}

// EvaluateCatalogPolicy resolves managed policy, session state, persistent
// project/user state, frontmatter, and source defaults in that order. The
// function is deterministic and has no side effects.
func EvaluateCatalogPolicy(input CatalogPolicyInput) (CatalogPolicyDecision, error) {
	if err := input.SkillID.Validate(); err != nil {
		return CatalogPolicyDecision{}, fmt.Errorf("%w: %v", ErrInvalidCatalogPolicy, err)
	}
	// Do not let a malformed lower-scope record delay a newly arrived managed
	// deny. The stable target still has to be valid, but lower policy is
	// irrelevant once the authoritative hard boundary is known.
	if input.ManagedDeny {
		return deniedCatalogPolicyDecision(CatalogPolicyReasonManagedDeny), nil
	}
	for _, candidate := range []struct {
		scope    SkillScope
		override *VisibilityOverride
	}{
		{scope: SkillScopeUser, override: input.UserOverride},
		{scope: SkillScopeProject, override: input.ProjectOverride},
		{scope: SkillScopeSession, override: input.SessionOverride},
	} {
		if err := validateCatalogPolicyOverride(input.SkillID, candidate.scope, candidate.override); err != nil {
			return CatalogPolicyDecision{}, err
		}
	}

	base := evaluateCatalogPolicyBase(input)
	decision := base
	if override := input.UserOverride; override != nil {
		decision = applyCatalogVisibility(base, override.Visibility, SkillScopeUser)
	}
	if override := input.ProjectOverride; override != nil {
		decision = applyCatalogVisibility(base, override.Visibility, SkillScopeProject)
	}
	if override := input.SessionOverride; override != nil {
		decision = applyCatalogVisibility(base, override.Visibility, SkillScopeSession)
	}

	decision.Mutable = true
	if input.ManagedReadOnly {
		decision.Mutable = false
		decision.ReadOnlyReason = CatalogPolicyReasonManagedReadOnly
	}
	return decision, nil
}

func validateCatalogPolicyOverride(target SkillID, scope SkillScope, override *VisibilityOverride) error {
	if override == nil {
		return nil
	}
	if err := override.Validate(); err != nil {
		return fmt.Errorf("%w: %s override: %v", ErrInvalidCatalogPolicy, scope, err)
	}
	if override.SkillID != target {
		return fmt.Errorf("%w: %s override targets a different skill ID", ErrInvalidCatalogPolicy, scope)
	}
	if override.Scope != scope {
		return fmt.Errorf("%w: %s override has scope %s", ErrInvalidCatalogPolicy, scope, override.Scope)
	}
	return nil
}

func evaluateCatalogPolicyBase(input CatalogPolicyInput) CatalogPolicyDecision {
	modelVisible := input.DefaultModelVisible
	userInvocable := input.DefaultUserInvocable
	modelReason := CatalogPolicyReasonDefault
	userReason := CatalogPolicyReasonDefault
	source := SkillScopeDefault

	if input.FrontmatterDisableModelInvocation {
		modelVisible = false
		modelReason = CatalogPolicyReasonFrontmatterModelDisabled
		source = SkillScopeFrontmatter
	}
	if input.FrontmatterUserInvocable != nil {
		userInvocable = *input.FrontmatterUserInvocable
		if userInvocable {
			userReason = CatalogPolicyReasonFrontmatter
		} else {
			userReason = CatalogPolicyReasonFrontmatterUserDisabled
		}
		source = SkillScopeFrontmatter
	}

	visibility := VisibilityAuto
	switch {
	case !modelVisible && !userInvocable:
		visibility = VisibilityOff
	case !modelVisible && userInvocable:
		visibility = VisibilityManualOnly
	}

	descriptionVisible := modelVisible
	descriptionReason := modelReason
	executable := modelVisible || userInvocable
	executionReason := policyExecutionReason(modelVisible, modelReason, userInvocable, userReason)
	return CatalogPolicyDecision{
		Visibility:         visibility,
		VisibilitySource:   source,
		ModelVisible:       modelVisible,
		ModelReason:        modelReason,
		DescriptionVisible: descriptionVisible,
		DescriptionReason:  descriptionReason,
		UserInvocable:      userInvocable,
		UserReason:         userReason,
		Executable:         executable,
		ExecutionReason:    executionReason,
	}
}

func applyCatalogVisibility(base CatalogPolicyDecision, visibility Visibility, scope SkillScope) CatalogPolicyDecision {
	reason := catalogPolicyScopeReason(scope)
	switch visibility {
	case VisibilityAuto:
		base.Visibility = VisibilityAuto
		base.VisibilitySource = scope
		if base.ModelVisible {
			base.ModelReason = reason
			base.DescriptionReason = reason
		}
		if base.UserInvocable {
			base.UserReason = reason
		}
		if base.Executable {
			base.ExecutionReason = reason
		}
		return base
	case VisibilityNameOnly:
		return CatalogPolicyDecision{
			Visibility:         visibility,
			VisibilitySource:   scope,
			ModelVisible:       true,
			ModelReason:        reason,
			DescriptionVisible: false,
			DescriptionReason:  CatalogPolicyReasonNameOnly,
			UserInvocable:      true,
			UserReason:         reason,
			Executable:         true,
			ExecutionReason:    reason,
		}
	case VisibilityManualOnly:
		return CatalogPolicyDecision{
			Visibility:         visibility,
			VisibilitySource:   scope,
			ModelVisible:       false,
			ModelReason:        CatalogPolicyReasonManualOnly,
			DescriptionVisible: false,
			DescriptionReason:  CatalogPolicyReasonManualOnly,
			UserInvocable:      true,
			UserReason:         reason,
			Executable:         true,
			ExecutionReason:    reason,
		}
	case VisibilityOff:
		return CatalogPolicyDecision{
			Visibility:        visibility,
			VisibilitySource:  scope,
			ModelReason:       CatalogPolicyReasonOff,
			DescriptionReason: CatalogPolicyReasonOff,
			UserReason:        CatalogPolicyReasonOff,
			ExecutionReason:   CatalogPolicyReasonOff,
		}
	default:
		// Input validation makes this unreachable; keeping the default fail-closed
		// prevents future visibility values from becoming implicitly executable.
		return CatalogPolicyDecision{
			Visibility:        VisibilityOff,
			VisibilitySource:  scope,
			ModelReason:       CatalogPolicyReasonOff,
			DescriptionReason: CatalogPolicyReasonOff,
			UserReason:        CatalogPolicyReasonOff,
			ExecutionReason:   CatalogPolicyReasonOff,
		}
	}
}

func deniedCatalogPolicyDecision(reason CatalogPolicyReason) CatalogPolicyDecision {
	return CatalogPolicyDecision{
		Visibility:        VisibilityOff,
		VisibilitySource:  SkillScopeManaged,
		ModelReason:       reason,
		DescriptionReason: reason,
		UserReason:        reason,
		ExecutionReason:   reason,
		Mutable:           false,
		ReadOnlyReason:    reason,
	}
}

func catalogPolicyScopeReason(scope SkillScope) CatalogPolicyReason {
	switch scope {
	case SkillScopeSession:
		return CatalogPolicyReasonSessionOverride
	case SkillScopeProject:
		return CatalogPolicyReasonProjectOverride
	case SkillScopeUser:
		return CatalogPolicyReasonUserOverride
	case SkillScopeFrontmatter:
		return CatalogPolicyReasonFrontmatter
	default:
		return CatalogPolicyReasonDefault
	}
}

func policyExecutionReason(modelVisible bool, modelReason CatalogPolicyReason, userInvocable bool, userReason CatalogPolicyReason) CatalogPolicyReason {
	if modelVisible {
		return modelReason
	}
	if userInvocable {
		return userReason
	}
	return CatalogPolicyReasonNoInvocationOrigin
}
