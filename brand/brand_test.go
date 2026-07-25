package brand

import (
	"reflect"
	"strings"
	"testing"
)

func TestProductIdentity(t *testing.T) {
	if DisplayName != "LUBAN Code" || RuntimeName != DisplayName {
		t.Fatalf("display/runtime identity = %q/%q, want LUBAN Code", DisplayName, RuntimeName)
	}
	if CommandName != "luban-code" {
		t.Fatalf("command name = %q, want luban-code", CommandName)
	}
	if ConfigDirName != ".luban-code" {
		t.Fatalf("config directory = %q, want .luban-code", ConfigDirName)
	}
	if InstructionsFile != "LUBAN.md" {
		t.Fatalf("instructions file = %q, want LUBAN.md", InstructionsFile)
	}
}

func TestDeepSeekProviderIdentityIsNotProductBranding(t *testing.T) {
	if DefaultProvider != "deepseek" || DeepSeekProvider != "deepseek" {
		t.Fatalf("DeepSeek provider identity changed: default=%q provider=%q", DefaultProvider, DeepSeekProvider)
	}
	if DeepSeekDefaultModel != "deepseek-v4-flash" || DeepSeekProModel != "deepseek-v4-pro" {
		t.Fatalf("DeepSeek model identity changed: default=%q pro=%q", DeepSeekDefaultModel, DeepSeekProModel)
	}
	if DeepSeekBaseURL != "https://api.deepseek.com/v1" {
		t.Fatalf("DeepSeek base URL changed: %q", DeepSeekBaseURL)
	}
}

func TestTerminalLogoUsesLUBANIdentityAndReturnsCopy(t *testing.T) {
	wide := TerminalLogoLines(80)
	wantWide := []string{
		"█      █    █ █████   ████  █    █",
		"█      █    █ █    █ █    █ ██   █",
		"█      █    █ █████  ██████ █ █  █",
		"█      █    █ █    █ █    █ █  ███",
		"██████  ████  █████  █    █ █    █",
	}
	if !reflect.DeepEqual(wide, wantWide) {
		t.Fatalf("wide logo = %q, want %q", wide, wantWide)
	}
	if strings.Contains(strings.Join(wide, "\n"), "DEEPSEEK") {
		t.Fatalf("wide logo does not represent LUBAN identity: %q", wide)
	}
	original := wide[0]
	wide[0] = "mutated"
	if got := TerminalLogoLines(80)[0]; got != original {
		t.Fatal("TerminalLogoLines returned mutable shared brand data")
	}
	if got := TerminalLogoLines(10); len(got) != 1 || got[0] != "LUBAN" {
		t.Fatalf("narrow logo = %q, want LUBAN", got)
	}
}
