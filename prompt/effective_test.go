package prompt

import (
	"reflect"
	"testing"
)

func TestBuildEffectiveSystemPromptPrecedence(t *testing.T) {
	defaultPrompt := SystemPrompt{
		{Text: "default static", Source: "built_in", Name: "static", Cache: true, CacheScope: "ephemeral"},
		{Text: "default dynamic", Source: "runtime", Name: "dynamic"},
	}

	tests := []struct {
		name  string
		input EffectiveSystemPromptInput
		want  []string
	}{
		{
			name: "override replaces every other prompt including append",
			input: EffectiveSystemPromptInput{
				Override:    "override",
				Coordinator: "coordinator",
				Agent:       "agent",
				Custom:      "custom",
				Default:     defaultPrompt,
				Append:      "append",
			},
			want: []string{"override"},
		},
		{
			name: "coordinator wins over agent custom and default but keeps append",
			input: EffectiveSystemPromptInput{
				Coordinator: "coordinator",
				Agent:       "agent",
				Custom:      "custom",
				Default:     defaultPrompt,
				Append:      "append",
			},
			want: []string{"coordinator", "append"},
		},
		{
			name: "agent wins over custom and default but keeps append",
			input: EffectiveSystemPromptInput{
				Agent:   "agent",
				Custom:  "custom",
				Default: defaultPrompt,
				Append:  "append",
			},
			want: []string{"agent", "append"},
		},
		{
			name: "custom replaces default but keeps append",
			input: EffectiveSystemPromptInput{
				Custom:  "custom",
				Default: defaultPrompt,
				Append:  "append",
			},
			want: []string{"custom", "append"},
		},
		{
			name: "default is used when no higher precedence prompt exists",
			input: EffectiveSystemPromptInput{
				Default: defaultPrompt,
			},
			want: []string{"default static", "default dynamic"},
		},
		{
			name: "append is added to default",
			input: EffectiveSystemPromptInput{
				Default: defaultPrompt,
				Append:  "append",
			},
			want: []string{"default static", "default dynamic", "append"},
		},
		{
			name: "blank higher precedence prompts are ignored",
			input: EffectiveSystemPromptInput{
				Override: " \n\t ",
				Agent:    " ",
				Custom:   "custom",
				Default:  defaultPrompt,
				Append:   " ",
			},
			want: []string{"custom"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildEffectiveSystemPrompt(tt.input)
			if !reflect.DeepEqual(got.Texts(), tt.want) {
				t.Fatalf("Texts() = %#v, want %#v", got.Texts(), tt.want)
			}
		})
	}
}

func TestBuildEffectiveSystemPromptClonesDefaultBlocks(t *testing.T) {
	defaultPrompt := SystemPrompt{{Text: "default", Source: "built_in"}}
	got := BuildEffectiveSystemPrompt(EffectiveSystemPromptInput{Default: defaultPrompt})
	got[0].Text = "mutated"

	if defaultPrompt[0].Text != "default" {
		t.Fatalf("default prompt was mutated: %#v", defaultPrompt)
	}
}
