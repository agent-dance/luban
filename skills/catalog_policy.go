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
	CatalogPolicyReasonManagedDeny     CatalogPolicyReason = "managed-deny"
	CatalogPolicyReasonManagedReadOnly CatalogPolicyReason = "managed-read-only"
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
	DescriptionVisible bool
	UserInvocable      bool
	Executable         bool

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
		return deniedCatalogPolicyDecision(), nil
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
	source := SkillScopeDefault

	if input.FrontmatterDisableModelInvocation {
		modelVisible = false
		source = SkillScopeFrontmatter
	}
	if input.FrontmatterUserInvocable != nil {
		userInvocable = *input.FrontmatterUserInvocable
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
	executable := modelVisible || userInvocable
	return CatalogPolicyDecision{
		Visibility:         visibility,
		VisibilitySource:   source,
		ModelVisible:       modelVisible,
		DescriptionVisible: descriptionVisible,
		UserInvocable:      userInvocable,
		Executable:         executable,
	}
}

func applyCatalogVisibility(base CatalogPolicyDecision, visibility Visibility, scope SkillScope) CatalogPolicyDecision {
	switch visibility {
	case VisibilityAuto:
		base.Visibility = VisibilityAuto
		base.VisibilitySource = scope
		return base
	case VisibilityNameOnly:
		return CatalogPolicyDecision{
			Visibility:         visibility,
			VisibilitySource:   scope,
			ModelVisible:       true,
			DescriptionVisible: false,
			UserInvocable:      true,
			Executable:         true,
		}
	case VisibilityManualOnly:
		return CatalogPolicyDecision{
			Visibility:         visibility,
			VisibilitySource:   scope,
			ModelVisible:       false,
			DescriptionVisible: false,
			UserInvocable:      true,
			Executable:         true,
		}
	case VisibilityOff:
		return CatalogPolicyDecision{
			Visibility:       visibility,
			VisibilitySource: scope,
		}
	default:
		// Input validation makes this unreachable; keeping the default fail-closed
		// prevents future visibility values from becoming implicitly executable.
		return CatalogPolicyDecision{
			Visibility:       VisibilityOff,
			VisibilitySource: scope,
		}
	}
}

func deniedCatalogPolicyDecision() CatalogPolicyDecision {
	return CatalogPolicyDecision{
		Visibility:       VisibilityOff,
		VisibilitySource: SkillScopeManaged,
		Mutable:          false,
		ReadOnlyReason:   CatalogPolicyReasonManagedDeny,
	}
}
