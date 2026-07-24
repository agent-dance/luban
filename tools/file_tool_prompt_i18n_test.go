package tools

import (
	"testing"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

func TestFileToolPromptsUseActiveRuntimeLanguage(t *testing.T) {
	previous := i18n.DetectOrLoadLanguage()
	t.Cleanup(func() { _ = i18n.SaveLanguage(previous) })
	read, edit, write := &FileReadTool{}, &FileEditTool{}, &FileWriteTool{}

	if err := i18n.SaveLanguage(i18n.LangZH); err != nil {
		t.Fatal(err)
	}
	if got := read.Description(); got != i18n.Text(i18n.LangZH, i18n.KeyToolFileReadDescription) {
		t.Fatalf("Read description = %q", got)
	}
	if got := edit.Description(); got != i18n.Text(i18n.LangZH, i18n.KeyToolFileEditDescription) {
		t.Fatalf("Edit description = %q", got)
	}
	if got := write.Description(); got != i18n.Text(i18n.LangZH, i18n.KeyToolFileWriteDescription) {
		t.Fatalf("Write description = %q", got)
	}
	definitions := types.ToDefinitions([]types.Tool{read, edit, write})
	if definitions[0].Description != read.Description() || definitions[1].Description != edit.Description() || definitions[2].Description != write.Description() {
		t.Fatalf("API definitions did not retain localized rich descriptions: %#v", definitions)
	}

	if err := i18n.SaveLanguage(i18n.LangJA); err != nil {
		t.Fatal(err)
	}
	if read.Description() == definitions[0].Description {
		t.Fatal("description was cached across runtime language change")
	}
}

func TestFileToolSchemaDescriptionsUseSemanticKeys(t *testing.T) {
	previous := i18n.DetectOrLoadLanguage()
	t.Cleanup(func() { _ = i18n.SaveLanguage(previous) })
	if err := i18n.SaveLanguage(i18n.LangZH); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		schema types.JSONSchema
		field  string
		key    i18n.Key
	}{
		{(&FileReadTool{}).Schema(), "file_path", i18n.KeyToolFileReadInputFilePathDescription},
		{(&FileReadTool{}).Schema(), "offset", i18n.KeyToolFileReadInputOffsetDescription},
		{(&FileReadTool{}).Schema(), "limit", i18n.KeyToolFileReadInputLimitDescription},
		{(&FileReadTool{}).Schema(), "pages", i18n.KeyToolFileReadInputPagesDescription},
		{(&FileEditTool{}).Schema(), "file_path", i18n.KeyToolFileEditInputFilePathDescription},
		{(&FileEditTool{}).Schema(), "old_string", i18n.KeyToolFileEditInputOldStringDescription},
		{(&FileEditTool{}).Schema(), "new_string", i18n.KeyToolFileEditInputNewStringDescription},
		{(&FileEditTool{}).Schema(), "replace_all", i18n.KeyToolFileEditInputReplaceAllDescription},
		{(&FileWriteTool{}).Schema(), "file_path", i18n.KeyToolFileWriteInputFilePathDescription},
		{(&FileWriteTool{}).Schema(), "content", i18n.KeyToolFileWriteInputContentDescription},
	}
	for _, tc := range cases {
		property, ok := tc.schema.Properties[tc.field].(map[string]any)
		if !ok || property["description"] != i18n.Text(i18n.LangZH, tc.key) {
			t.Errorf("schema field %s = %#v", tc.field, tc.schema.Properties[tc.field])
		}
	}
}
