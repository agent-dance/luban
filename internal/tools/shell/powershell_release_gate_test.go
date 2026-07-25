package shell

import (
	"context"
	"reflect"
	"testing"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

func TestPowerShellSchemaUsesLocalizedStrictContract(t *testing.T) {
	tool := &PowerShellTool{}
	schema := tool.Schema()
	if !schema.RejectsUnknownFields() {
		t.Fatalf("PowerShell schema must reject unknown fields: %#v", schema.AdditionalProperties)
	}
	if !reflect.DeepEqual(schema.Required, []string{"command"}) {
		t.Fatalf("PowerShell required fields = %#v", schema.Required)
	}

	lang := i18n.DetectOrLoadLanguage()
	if got, want := tool.Description(), i18n.Text(lang, i18n.KeyToolPromptPowerShellDescription); got != want {
		t.Fatalf("Description() = %q, want %q", got, want)
	}
	wantDescriptions := map[string]i18n.Key{
		"command":           i18n.KeyToolPromptPowerShellCommand,
		"timeout":           i18n.KeyToolPromptPowerShellTimeout,
		"description":       i18n.KeyToolPromptPowerShellSummary,
		"run_in_background": i18n.KeyToolPromptPowerShellRunInBackground,
	}
	for field, key := range wantDescriptions {
		property, ok := schema.Properties[field].(map[string]any)
		if !ok {
			t.Fatalf("schema property %q = %#v", field, schema.Properties[field])
		}
		if got, want := property["description"], i18n.Text(lang, key); got != want {
			t.Fatalf("schema property %q description = %q, want %q", field, got, want)
		}
	}
}

func TestPowerShellExecuteRejectsUnknownInputBeforeProcessLaunch(t *testing.T) {
	result, err := (&PowerShellTool{}).Execute(context.Background(), map[string]any{
		"command":          "Write-Output ok",
		"undeclared_field": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || result.Outcome != types.ToolOutcomeFailed {
		t.Fatalf("unknown input result = %#v", result)
	}
}

func TestPowerShellToolMetadataClassifiesSchedulingRisk(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    types.ToolMetadata
	}{
		{
			name: "read", command: "Get-Content README.md",
			want: types.ToolMetadata{ReadOnly: true, ConcurrencySafe: true, MaxResultSizeChars: 30_000},
		},
		{
			name: "process read", command: "Get-Process",
			want: types.ToolMetadata{ReadOnly: true, ConcurrencySafe: true, MaxResultSizeChars: 30_000},
		},
		{
			name: "write", command: "Set-Content out.txt value",
			want: types.ToolMetadata{Write: true, MaxResultSizeChars: 30_000},
		},
		{
			name: "network", command: "Invoke-WebRequest https://example.com",
			want: types.ToolMetadata{Write: true, MaxResultSizeChars: 30_000},
		},
		{
			name: "destructive", command: "Remove-Item -Recurse target",
			want: types.ToolMetadata{Write: true, Destructive: true, MaxResultSizeChars: 30_000},
		},
		{
			name: "unknown fails closed", command: "$scriptBlock.Invoke()",
			want: types.ToolMetadata{Write: true, MaxResultSizeChars: 30_000},
		},
	}
	tool := &PowerShellTool{}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := tool.ToolMetadata(map[string]any{"command": test.command}); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("ToolMetadata(%q) = %#v, want %#v", test.command, got, test.want)
			}
		})
	}
}

type powerShellActivePlanGate struct {
	calls int
}

func (g *powerShellActivePlanGate) IsActive() bool {
	g.calls++
	return true
}

func (g *powerShellActivePlanGate) AllowedPromptMatches(string, string) bool { return false }

func TestRegisteredPowerShellRequiresOneTimeRegistryDispatch(t *testing.T) {
	reg := registry.New()
	planGate := &powerShellActivePlanGate{}
	tool := &PowerShellTool{PlanState: planGate}
	reg.Register(tool)

	input := map[string]any{"command": "Write-Output ok"}
	registered, ok := reg.Get(tool.Name()).(*PowerShellTool)
	if !ok {
		t.Fatalf("registered PowerShell type = %T", reg.Get(tool.Name()))
	}
	direct, err := registered.Execute(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	wantPermission := i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolPermissionPowerShellDispatch)
	if !direct.IsError || direct.Content != wantPermission {
		t.Fatalf("direct registered execution = %#v, want %q", direct, wantPermission)
	}
	if planGate.calls != 0 {
		t.Fatalf("direct execution crossed registry dispatch gate: plan calls = %d", planGate.calls)
	}

	preflight, err := reg.CheckToolPermissions(context.Background(), tool.Name(), input, types.ToolPermissionRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if preflight.ExecutionPolicyCode != "" {
		t.Fatalf("PowerShell permission policy code = %q, want empty registry policy binding", preflight.ExecutionPolicyCode)
	}
	reg.RevokePermissionGrant(preflight.PermissionGrant)

	dispatched, err := reg.ExecuteToolWithError(context.Background(), tool.Name(), input)
	if err != nil {
		t.Fatal(err)
	}
	wantPlanBlock := i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolRuntimePowerShellPlanModeBlocked)
	if !dispatched.IsError || dispatched.Content != wantPlanBlock {
		t.Fatalf("registry dispatch = %#v, want plan block %q", dispatched, wantPlanBlock)
	}
	if planGate.calls != 1 {
		t.Fatalf("registry dispatch did not cross the permission receipt gate: plan calls = %d", planGate.calls)
	}

	replayedDirect, err := registered.Execute(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !replayedDirect.IsError || replayedDirect.Content != wantPermission || planGate.calls != 1 {
		t.Fatalf("direct execution reused consumed dispatch receipt: result=%#v plan calls=%d", replayedDirect, planGate.calls)
	}
}
