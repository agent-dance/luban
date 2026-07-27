package report

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublishedSchemasAreValidJSON(t *testing.T) {
	for _, path := range []string{
		"schema/report-input.schema.json",
		"schema/optimization-ledger.schema.json",
	} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", path, err)
		}
		var document map[string]any
		if err := json.Unmarshal(raw, &document); err != nil {
			t.Fatalf("Unmarshal(%s): %v", path, err)
		}
		if document["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
			t.Fatalf("%s does not declare JSON Schema 2020-12", path)
		}
		if document["additionalProperties"] != false {
			t.Fatalf("%s root is not closed to unknown properties", path)
		}
	}
}

func TestCanonicalFuturePathResolvesSymlinkedParent(t *testing.T) {
	root := t.TempDir()
	aliasParent := t.TempDir()
	alias := filepath.Join(aliasParent, "artifact-alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	candidate, err := canonicalFuturePath(filepath.Join(alias, "new", "report.html"))
	if err != nil {
		t.Fatalf("canonicalFuturePath: %v", err)
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	if !pathContains(canonicalRoot, candidate) {
		t.Fatalf("candidate %q did not resolve inside root %q", candidate, canonicalRoot)
	}
}

func TestRenderResolvesEveryTemplateKeyInEveryLanguage(t *testing.T) {
	data, err := Compile(diagnosticInputFixture)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	tests := []struct {
		code     string
		htmlLang string
	}{
		{code: "en", htmlLang: "en"},
		{code: "zh-CN", htmlLang: "zh-CN"},
		{code: "de", htmlLang: "de"},
		{code: "ja", htmlLang: "ja"},
		{code: "ko", htmlLang: "ko"},
		{code: "ru", htmlLang: "ru"},
	}
	for _, test := range tests {
		t.Run(test.code, func(t *testing.T) {
			localized := data
			localized.Meta.Language = test.code
			var output bytes.Buffer
			if err := Render(&output, localized); err != nil {
				t.Fatalf("Render: %v", err)
			}
			rendered := output.String()
			if !strings.Contains(rendered, `<html lang="`+test.htmlLang+`">`) {
				t.Fatalf("rendered report does not declare html lang %q", test.htmlLang)
			}
			if strings.Contains(rendered, "[agentic.report.") {
				t.Fatal("rendered report contains an unresolved semantic copy key")
			}
		})
	}
}
