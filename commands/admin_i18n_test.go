package commands_test

import (
	"strings"
	"testing"

	"github.com/agent-dance/luban/commands"
	"github.com/agent-dance/luban/i18n"
)

func TestAdminCommandsUseContextLanguage(t *testing.T) {
	registry := commands.NewRegistry()
	commands.RegisterBuiltins(registry)

	tests := []struct {
		command string
		args    string
		want    string
	}{
		{command: "config", args: "get", want: "用法：/config get <key>"},
		{command: "init", want: "已初始化项目结构"},
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			var output strings.Builder
			ctx := &commands.Context{
				CWD:      t.TempDir(),
				Language: i18n.LangZH,
				OnEvent:  func(s string) { output.WriteString(s) },
			}
			if err := registry.Find(tt.command).Execute(ctx, tt.args); err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if got := output.String(); !strings.Contains(got, tt.want) {
				t.Fatalf("output missing %q:\n%s", tt.want, got)
			}
		})
	}
}

func TestDoctorUsesContextLanguage(t *testing.T) {
	ctx, output := newDoctorCtx("anthropic", "claude-sonnet-4-6")
	ctx.Language = i18n.LangZH
	registry := commands.NewRegistry()
	commands.RegisterBuiltins(registry)
	if err := registry.Find("doctor").Execute(ctx, ""); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := output.String(); !strings.Contains(got, "凭据") || !strings.Contains(got, "模型") {
		t.Fatalf("doctor output did not use Chinese labels:\n%s", got)
	}
}
