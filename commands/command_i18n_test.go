package commands

import (
	"strings"
	"testing"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/runtime/goal"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/types"
)

type commandI18NQueryLoop struct{}

func (commandI18NQueryLoop) SetMessagesPreservingToolUseLedger([]types.Message) {}
func (commandI18NQueryLoop) Messages() []types.Message                          { return nil }
func (commandI18NQueryLoop) Model() string                                      { return "model-test" }
func (commandI18NQueryLoop) SetModel(string)                                    {}
func (commandI18NQueryLoop) ContextUsage() (int, int)                           { return 1000, 250 }
func (commandI18NQueryLoop) SetProvider(provider.Provider)                      {}

type commandI18NGoalRuntime struct{ current *goal.Goal }

func (r commandI18NGoalRuntime) LoadGoal() (*goal.Goal, error) { return r.current, nil }
func (commandI18NGoalRuntime) SaveGoal(goal.Goal) error        { return nil }

func TestCoreCommandsUseContextLanguage(t *testing.T) {
	goalState := &goal.Goal{Objective: "验证本地化", Status: goal.StatusActive}
	cases := []struct {
		name string
		cmd  Command
		want string
	}{
		{name: "status", cmd: &statusCmd{}, want: "会话状态"},
		{name: "context", cmd: &contextCmd{}, want: "当前上下文使用量"},
		{name: "goal", cmd: &goalCmd{}, want: "目标状态"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var output string
			ctx := &Context{
				Language:    i18n.LangZH,
				QueryLoop:   commandI18NQueryLoop{},
				GoalRuntime: commandI18NGoalRuntime{current: goalState},
				OnEvent:     func(value string) { output += value },
			}
			if err := tc.cmd.Execute(ctx, ""); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(output, tc.want) {
				t.Fatalf("output = %q, want localized text %q", output, tc.want)
			}
		})
	}
}

func TestBuiltinsUseContextLanguage(t *testing.T) {
	cases := []struct {
		name string
		cmd  Command
		args string
		ctx  func(*Context)
		want string
	}{
		{
			name: "model switch", cmd: &modelCmd{}, args: "model-test",
			ctx:  func(ctx *Context) { ctx.QueryLoop = commandI18NQueryLoop{} },
			want: "模型已切换为",
		},
		{
			name: "session usage", cmd: &sessionCmd{}, args: "invalid",
			ctx: func(*Context) {}, want: "用法：/session",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var output string
			ctx := &Context{Language: i18n.LangZH, OnEvent: func(value string) { output += value }}
			tc.ctx(ctx)
			if err := tc.cmd.Execute(ctx, tc.args); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(output, tc.want) {
				t.Fatalf("output = %q, want localized text %q", output, tc.want)
			}
		})
	}
}
