package skills

import "testing"

func TestGetCharBudget(t *testing.T) {
	tests := []struct {
		tokens int
		want   int
	}{
		{tokens: 0, want: defaultCharBudget},
		{tokens: 100_000, want: 4_000},
		{tokens: 200_000, want: 8_000},
	}
	for _, test := range tests {
		if got := GetCharBudget(test.tokens); got != test.want {
			t.Errorf("GetCharBudget(%d) = %d, want %d", test.tokens, got, test.want)
		}
	}
}

func TestIsUserInvocable(t *testing.T) {
	explicitTrue := true
	explicitFalse := false
	tests := []struct {
		name  string
		skill *Skill
		want  bool
	}{
		{name: "default", skill: &Skill{}, want: true},
		{name: "explicit true", skill: &Skill{UserInvocable: &explicitTrue}, want: true},
		{name: "explicit false", skill: &Skill{UserInvocable: &explicitFalse}, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.skill.IsUserInvocable(); got != test.want {
				t.Fatalf("IsUserInvocable() = %t, want %t", got, test.want)
			}
		})
	}
}
