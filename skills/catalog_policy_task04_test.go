package skills

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
)

const catalogPolicyTestSkillID SkillID = "skill:project:/repo/.agents/skills/review/SKILL.md"

func TestCatalogPolicyPrecedenceMatrix(t *testing.T) {
	t.Parallel()

	type overrideCase struct {
		name       string
		visibility *Visibility
	}
	overrideCases := []overrideCase{{name: "absent"}}
	for _, visibility := range []Visibility{
		VisibilityAuto,
		VisibilityNameOnly,
		VisibilityManualOnly,
		VisibilityOff,
	} {
		visibility := visibility
		overrideCases = append(overrideCases, overrideCase{name: string(visibility), visibility: &visibility})
	}

	frontmatterCases := []struct {
		name         string
		disableModel bool
		user         *bool
	}{
		{name: "defaults"},
		{name: "model-disabled", disableModel: true},
		{name: "user-disabled", user: catalogPolicyBool(false)},
		{name: "both-disabled", disableModel: true, user: catalogPolicyBool(false)},
	}

	cases := 0
	for _, managedDeny := range []bool{false, true} {
		for _, frontmatter := range frontmatterCases {
			for _, user := range overrideCases {
				for _, project := range overrideCases {
					for _, session := range overrideCases {
						name := fmt.Sprintf("managed=%t/frontmatter=%s/user=%s/project=%s/session=%s",
							managedDeny, frontmatter.name, user.name, project.name, session.name)
						t.Run(name, func(t *testing.T) {
							input := CatalogPolicyInput{
								SkillID:                           catalogPolicyTestSkillID,
								DefaultModelVisible:               true,
								DefaultUserInvocable:              true,
								FrontmatterDisableModelInvocation: frontmatter.disableModel,
								FrontmatterUserInvocable:          frontmatter.user,
								UserOverride:                      catalogPolicyOverride(SkillScopeUser, user.visibility),
								ProjectOverride:                   catalogPolicyOverride(SkillScopeProject, project.visibility),
								SessionOverride:                   catalogPolicyOverride(SkillScopeSession, session.visibility),
								ManagedDeny:                       managedDeny,
							}

							got, err := EvaluateCatalogPolicy(input)
							if err != nil {
								t.Fatal(err)
							}
							again, err := EvaluateCatalogPolicy(input)
							if err != nil {
								t.Fatal(err)
							}
							if !reflect.DeepEqual(got, again) {
								t.Fatalf("evaluation is not deterministic:\nfirst  %#v\nsecond %#v", got, again)
							}

							want := catalogPolicyExpected(input)
							assertCatalogPolicyCore(t, got, want)
						})
						cases++
					}
				}
			}
		}
	}
	if cases != 2*4*5*5*5 {
		t.Fatalf("matrix covered %d cases, want 1000", cases)
	}
}

func TestCatalogPolicyVisibilitySemantics(t *testing.T) {
	t.Parallel()

	// Both lower-level invocation paths are disabled so this table proves that
	// explicit non-auto overrides really have higher precedence than
	// frontmatter, while auto deliberately returns to the lower policy.
	tests := []struct {
		visibility         Visibility
		modelVisible       bool
		descriptionVisible bool
		userInvocable      bool
		executable         bool
	}{
		{visibility: VisibilityAuto},
		{visibility: VisibilityNameOnly, modelVisible: true, userInvocable: true, executable: true},
		{visibility: VisibilityManualOnly, userInvocable: true, executable: true},
		{visibility: VisibilityOff},
	}

	for _, test := range tests {
		t.Run(string(test.visibility), func(t *testing.T) {
			input := CatalogPolicyInput{
				SkillID:                           catalogPolicyTestSkillID,
				DefaultModelVisible:               true,
				DefaultUserInvocable:              true,
				FrontmatterDisableModelInvocation: true,
				FrontmatterUserInvocable:          catalogPolicyBool(false),
				SessionOverride: catalogPolicyOverride(
					SkillScopeSession,
					catalogPolicyVisibility(test.visibility),
				),
			}
			got, err := EvaluateCatalogPolicy(input)
			if err != nil {
				t.Fatal(err)
			}
			if got.Visibility != test.visibility || got.VisibilitySource != SkillScopeSession ||
				got.ModelVisible != test.modelVisible || got.DescriptionVisible != test.descriptionVisible ||
				got.UserInvocable != test.userInvocable || got.Executable != test.executable {
				t.Fatalf("decision = %#v", got)
			}
			if got.DescriptionVisible && !got.ModelVisible {
				t.Fatal("description was exposed without model visibility")
			}
			if test.visibility == VisibilityNameOnly && got.DescriptionVisible {
				t.Fatal("name-only exposed a description")
			}
			if test.visibility == VisibilityOff && got.Executable {
				t.Fatal("off remained executable")
			}
			if test.visibility == VisibilityManualOnly && (got.ModelVisible || !got.UserInvocable) {
				t.Fatal("manual-only was not user-only")
			}
		})
	}
}

func TestCatalogPolicyFrontmatterAndDefaults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		input         CatalogPolicyInput
		visibility    Visibility
		source        SkillScope
		modelVisible  bool
		userInvocable bool
		executable    bool
	}{
		{
			name: "default model and user",
			input: CatalogPolicyInput{
				DefaultModelVisible: true, DefaultUserInvocable: true,
			},
			visibility: VisibilityAuto, source: SkillScopeDefault,
			modelVisible: true, userInvocable: true, executable: true,
		},
		{
			name: "frontmatter model disable keeps user execution",
			input: CatalogPolicyInput{
				DefaultModelVisible: true, DefaultUserInvocable: true,
				FrontmatterDisableModelInvocation: true,
			},
			visibility: VisibilityManualOnly, source: SkillScopeFrontmatter,
			userInvocable: true, executable: true,
		},
		{
			name: "frontmatter user disable keeps model execution",
			input: CatalogPolicyInput{
				DefaultModelVisible: true, DefaultUserInvocable: true,
				FrontmatterUserInvocable: catalogPolicyBool(false),
			},
			visibility: VisibilityAuto, source: SkillScopeFrontmatter,
			modelVisible: true, executable: true,
		},
		{
			name: "all invocation origins disabled",
			input: CatalogPolicyInput{
				DefaultModelVisible: true, DefaultUserInvocable: true,
				FrontmatterDisableModelInvocation: true,
				FrontmatterUserInvocable:          catalogPolicyBool(false),
			},
			visibility: VisibilityOff, source: SkillScopeFrontmatter,
		},
		{
			name: "frontmatter can explicitly enable a default-disabled user path",
			input: CatalogPolicyInput{
				DefaultModelVisible: false, DefaultUserInvocable: false,
				FrontmatterUserInvocable: catalogPolicyBool(true),
			},
			visibility: VisibilityManualOnly, source: SkillScopeFrontmatter,
			userInvocable: true, executable: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := test.input
			input.SkillID = catalogPolicyTestSkillID
			got, err := EvaluateCatalogPolicy(input)
			if err != nil {
				t.Fatal(err)
			}
			if got.Visibility != test.visibility || got.VisibilitySource != test.source ||
				got.ModelVisible != test.modelVisible || got.DescriptionVisible != test.modelVisible ||
				got.UserInvocable != test.userInvocable || got.Executable != test.executable {
				t.Fatalf("decision = %#v", got)
			}
		})
	}
}

func TestCatalogPolicyManagedRestrictionsAndReadOnly(t *testing.T) {
	t.Parallel()

	lower := CatalogPolicyInput{
		SkillID:              catalogPolicyTestSkillID,
		DefaultModelVisible:  true,
		DefaultUserInvocable: true,
		UserOverride:         catalogPolicyOverride(SkillScopeUser, catalogPolicyVisibility(VisibilityNameOnly)),
		ProjectOverride:      catalogPolicyOverride(SkillScopeProject, catalogPolicyVisibility(VisibilityOff)),
		SessionOverride:      catalogPolicyOverride(SkillScopeSession, catalogPolicyVisibility(VisibilityManualOnly)),
		ManagedReadOnly:      true,
	}
	readOnly, err := EvaluateCatalogPolicy(lower)
	if err != nil {
		t.Fatal(err)
	}
	if readOnly.Visibility != VisibilityManualOnly || readOnly.VisibilitySource != SkillScopeSession ||
		readOnly.ModelVisible || !readOnly.UserInvocable || !readOnly.Executable {
		t.Fatalf("managed read-only changed lower visibility: %#v", readOnly)
	}
	if readOnly.Mutable || readOnly.ReadOnlyReason != CatalogPolicyReasonManagedReadOnly {
		t.Fatalf("managed read-only mutability = %#v", readOnly)
	}

	lower.ManagedDeny = true
	denied, err := EvaluateCatalogPolicy(lower)
	if err != nil {
		t.Fatal(err)
	}
	if denied.Visibility != VisibilityOff || denied.VisibilitySource != SkillScopeManaged ||
		denied.ModelVisible || denied.DescriptionVisible || denied.UserInvocable || denied.Executable || denied.Mutable {
		t.Fatalf("managed deny was relaxed: %#v", denied)
	}
	if denied.ReadOnlyReason != CatalogPolicyReasonManagedDeny {
		t.Fatalf("managed deny reason = %q", denied.ReadOnlyReason)
	}
}

func TestCatalogPolicyManagedDenyIgnoresMalformedLowerScopes(t *testing.T) {
	t.Parallel()

	got, err := EvaluateCatalogPolicy(CatalogPolicyInput{
		SkillID:     catalogPolicyTestSkillID,
		ManagedDeny: true,
		SessionOverride: &VisibilityOverride{
			SkillID:    "different-invalid-target",
			Scope:      SkillScopeProject,
			Visibility: "unknown",
		},
	})
	if err != nil {
		t.Fatalf("managed deny was delayed by lower invalid state: %v", err)
	}
	if got.Visibility != VisibilityOff || got.VisibilitySource != SkillScopeManaged || got.Executable || got.Mutable {
		t.Fatalf("managed deny decision = %#v", got)
	}
}

func TestCatalogPolicyRejectsInvalidOverrides(t *testing.T) {
	t.Parallel()

	otherID := SkillID("skill:user:/home/example/.agents/skills/review/SKILL.md")
	tests := []struct {
		name  string
		input CatalogPolicyInput
	}{
		{name: "invalid target ID", input: CatalogPolicyInput{SkillID: "review"}},
		{
			name: "override targets another stable ID",
			input: CatalogPolicyInput{
				SkillID: catalogPolicyTestSkillID,
				SessionOverride: &VisibilityOverride{
					SkillID: otherID, Scope: SkillScopeSession, Visibility: VisibilityOff,
				},
			},
		},
		{
			name: "override stored in wrong scope slot",
			input: CatalogPolicyInput{
				SkillID: catalogPolicyTestSkillID,
				ProjectOverride: &VisibilityOverride{
					SkillID: catalogPolicyTestSkillID, Scope: SkillScopeUser, Visibility: VisibilityOff,
				},
			},
		},
		{
			name: "invalid visibility",
			input: CatalogPolicyInput{
				SkillID: catalogPolicyTestSkillID,
				UserOverride: &VisibilityOverride{
					SkillID: catalogPolicyTestSkillID, Scope: SkillScopeUser, Visibility: "sometimes",
				},
			},
		},
		{
			name: "last non-off attached to non-off visibility",
			input: CatalogPolicyInput{
				SkillID: catalogPolicyTestSkillID,
				UserOverride: &VisibilityOverride{
					SkillID: catalogPolicyTestSkillID, Scope: SkillScopeUser,
					Visibility: VisibilityAuto, LastNonOff: catalogPolicyVisibility(VisibilityManualOnly),
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := EvaluateCatalogPolicy(test.input)
			if !errors.Is(err, ErrInvalidCatalogPolicy) {
				t.Fatalf("error = %v, want ErrInvalidCatalogPolicy", err)
			}
			if got != (CatalogPolicyDecision{}) {
				t.Fatalf("invalid input returned a usable decision: %#v", got)
			}
		})
	}
}

type catalogPolicyExpectedDecision struct {
	visibility         Visibility
	source             SkillScope
	modelVisible       bool
	descriptionVisible bool
	userInvocable      bool
	executable         bool
	mutable            bool
}

func catalogPolicyExpected(input CatalogPolicyInput) catalogPolicyExpectedDecision {
	if input.ManagedDeny {
		return catalogPolicyExpectedDecision{visibility: VisibilityOff, source: SkillScopeManaged}
	}

	modelVisible := input.DefaultModelVisible && !input.FrontmatterDisableModelInvocation
	userInvocable := input.DefaultUserInvocable
	if input.FrontmatterUserInvocable != nil {
		userInvocable = *input.FrontmatterUserInvocable
	}
	visibility := VisibilityAuto
	source := SkillScopeDefault
	if input.FrontmatterDisableModelInvocation || input.FrontmatterUserInvocable != nil {
		source = SkillScopeFrontmatter
	}
	switch {
	case !modelVisible && !userInvocable:
		visibility = VisibilityOff
	case !modelVisible:
		visibility = VisibilityManualOnly
	}

	selected := (*VisibilityOverride)(nil)
	if input.UserOverride != nil {
		selected = input.UserOverride
	}
	if input.ProjectOverride != nil {
		selected = input.ProjectOverride
	}
	if input.SessionOverride != nil {
		selected = input.SessionOverride
	}
	if selected != nil {
		visibility = selected.Visibility
		source = selected.Scope
		switch visibility {
		case VisibilityAuto:
			// Explicit auto masks lower overrides and inherits frontmatter/default.
		case VisibilityNameOnly:
			modelVisible = true
			userInvocable = true
		case VisibilityManualOnly:
			modelVisible = false
			userInvocable = true
		case VisibilityOff:
			modelVisible = false
			userInvocable = false
		}
	}

	descriptionVisible := modelVisible && visibility != VisibilityNameOnly
	return catalogPolicyExpectedDecision{
		visibility: visibility, source: source,
		modelVisible: modelVisible, descriptionVisible: descriptionVisible,
		userInvocable: userInvocable, executable: modelVisible || userInvocable,
		mutable: true,
	}
}

func assertCatalogPolicyCore(t *testing.T, got CatalogPolicyDecision, want catalogPolicyExpectedDecision) {
	t.Helper()
	if got.Visibility != want.visibility || got.VisibilitySource != want.source ||
		got.ModelVisible != want.modelVisible || got.DescriptionVisible != want.descriptionVisible ||
		got.UserInvocable != want.userInvocable || got.Executable != want.executable || got.Mutable != want.mutable {
		t.Fatalf("decision core = %#v, want %#v", got, want)
	}
}

func catalogPolicyOverride(scope SkillScope, visibility *Visibility) *VisibilityOverride {
	if visibility == nil {
		return nil
	}
	return &VisibilityOverride{
		SkillID:    catalogPolicyTestSkillID,
		Scope:      scope,
		Visibility: *visibility,
	}
}

func catalogPolicyVisibility(visibility Visibility) *Visibility { return &visibility }

func catalogPolicyBool(value bool) *bool { return &value }
