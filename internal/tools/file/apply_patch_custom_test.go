package file

import (
	"strings"
	"testing"

	"github.com/agent-dance/luban/types"
)

func TestApplyPatchCustomContractIsExplicitAndDefaultsOff(t *testing.T) {
	tool := &ApplyPatchTool{}
	functionDefinition := types.ToDefinition(tool)
	if functionDefinition.IsCustom() || functionDefinition.Format != nil || !functionDefinition.Strict {
		t.Fatalf("default ApplyPatch definition = %#v", functionDefinition)
	}

	tool.CustomToolInput = true
	customDefinition := types.ToDefinition(tool)
	if !customDefinition.IsCustom() || customDefinition.Strict || customDefinition.Format == nil {
		t.Fatalf("custom ApplyPatch definition = %#v", customDefinition)
	}
	format := customDefinition.Format
	if format.Type != "grammar" || format.Syntax != "lark" || format.Definition != applyPatchCustomGrammar {
		t.Fatalf("custom grammar format = %#v", format)
	}
	for _, forbidden := range []string{"%import", "%ignore", "%declare", "(?=", "(?!"} {
		if strings.Contains(format.Definition, forbidden) {
			t.Fatalf("grammar contains unsupported construct %q", forbidden)
		}
	}
	for _, required := range []string{`BEGIN: "*** Begin Patch"`, `END: "*** End Patch"`, "add_file", "update_file", "delete_file"} {
		if !strings.Contains(format.Definition, required) {
			t.Fatalf("grammar missing %q", required)
		}
	}
}

func TestApplyPatchCustomInputProjectsExactBytesToLocalParser(t *testing.T) {
	tool := &ApplyPatchTool{CustomToolInput: true}
	patch := strings.Join([]string{
		"*** Begin Patch",
		"*** Add File: new.txt",
		"+hello \\ world",
		"*** End Patch",
	}, "\n")
	input, err := tool.DecodeCustomToolInput(patch)
	if err != nil {
		t.Fatal(err)
	}
	if input["patch"] != patch {
		t.Fatalf("projected patch = %#v", input["patch"])
	}
	parsed, parseErr := parseApplyPatch(input["patch"].(string))
	if parseErr != nil || len(parsed.Files) != 1 || parsed.Files[0].Path != "new.txt" {
		t.Fatalf("local parser rejected custom grammar payload: parsed=%#v err=%v", parsed, parseErr)
	}
}
