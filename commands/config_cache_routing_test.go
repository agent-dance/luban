package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/i18n"
)

func TestConfigSetCacheRoutingModeValidatesAndPersists(t *testing.T) {
	cwd := t.TempDir()
	var output strings.Builder
	ctx := &Context{CWD: cwd, Language: i18n.LangEN, OnEvent: func(value string) { output.WriteString(value) }}
	command := &configCmd{}

	if err := command.set(ctx, "cacheRoutingMode", "OFF"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(cwd, brand.ConfigDirName, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatal(err)
	}
	if got := settings["cacheRoutingMode"]; got != "off" {
		t.Fatalf("cacheRoutingMode = %#v, want off", got)
	}

	output.Reset()
	if err := command.set(ctx, "cacheRoutingMode", "sometimes"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "must be auto, on, or off") {
		t.Fatalf("validation output = %q", output.String())
	}
	data, err = os.ReadFile(filepath.Join(cwd, brand.ConfigDirName, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	settings = nil
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatal(err)
	}
	if got := settings["cacheRoutingMode"]; got != "off" {
		t.Fatalf("invalid value changed cacheRoutingMode to %#v", got)
	}
}
