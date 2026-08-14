package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/agent-dance/luban/types"
)

func TestOpenAIPublicResponsesSanitizesExact3InspectSchemaOnWire(t *testing.T) {
	localSchema := exact3V2InspectSchemaForResponsesTest()
	localBefore, err := json.Marshal(localSchema)
	if err != nil {
		t.Fatal(err)
	}

	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		raw, _ := io.ReadAll(request.Body)
		if err := json.Unmarshal(raw, &requestBody); err != nil {
			t.Errorf("decode request: %v", err)
		}
		writer.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(writer, buildSSEStream([]sseEvent{
			{Type: "response.created", Data: `{"response":{"id":"resp_schema","model":"gpt-5.6-sol"}}`},
			{Type: "response.completed", Data: `{"response":{"id":"resp_schema","model":"gpt-5.6-sol","status":"completed","usage":{"input_tokens":1,"output_tokens":1},"output":[]}}`},
		}))
	}))
	defer server.Close()

	responses := NewResponses(Config{
		ProviderName:       "openai",
		ResponsesSemantics: ResponsesSemanticsOpenAIPublic,
		APIKey:             "test-key",
		BaseURL:            server.URL,
		Model:              "gpt-5.6-sol",
	})
	stream, err := responses.CreateStream(context.Background(), Params{
		Messages: []types.Message{types.UserMessage("inspect")},
		Tools: []types.ToolDefinition{{
			Name: "Inspect", Description: "Inspect repository files in batches.",
			InputSchema: localSchema, Strict: true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for range stream {
	}

	toolsValue := requestBody["tools"]
	if toolsValue == nil {
		for _, item := range requestBody["input"].([]any) {
			inputItem, _ := item.(map[string]any)
			if inputItem["type"] == "additional_tools" {
				toolsValue = inputItem["tools"]
				break
			}
		}
	}
	tools, ok := toolsValue.([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("wire tools = %#v", toolsValue)
	}
	tool, _ := tools[0].(map[string]any)
	if strict, _ := tool["strict"].(bool); !strict {
		t.Fatalf("wire tool is not strict: %#v", tool)
	}
	parameters, ok := tool["parameters"].(map[string]any)
	if !ok {
		t.Fatalf("wire parameters = %#v", tool["parameters"])
	}
	wire, err := json.Marshal(parameters)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{`"_semantic"`, `"integer"`, `"default"`} {
		if strings.Contains(string(wire), forbidden) {
			t.Fatalf("wire schema retained %s: %s", forbidden, wire)
		}
	}
	assertOpenAIResponsesStrictObjects(t, parameters, "parameters")

	maxChars := parameters["properties"].(map[string]any)["max_chars"].(map[string]any)
	maxChars = assertNullableResponsesSchema(t, maxChars, "number")
	if maxChars["minimum"] != float64(8_192) || maxChars["description"] == "" {
		t.Fatalf("supported Inspect constraints were lost: %#v", maxChars)
	}
	properties := parameters["properties"].(map[string]any)
	assertNullableResponsesSchema(t, properties["cursor"].(map[string]any), "string")
	requests := assertNullableResponsesSchema(t, properties["requests"].(map[string]any), "array")
	request := requests["items"].(map[string]any)
	requestProperties := request["properties"].(map[string]any)
	assertResponsesSchemaNotNullable(t, requestProperties["id"].(map[string]any), "string")
	assertResponsesSchemaNotNullable(t, requestProperties["kind"].(map[string]any), "string")
	assertNullableResponsesSchema(t, requestProperties["path"].(map[string]any), "string")
	assertNullableResponsesSchema(t, requestProperties["ranges"].(map[string]any), "array")
	localAfter, err := json.Marshal(localSchema)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(localBefore, localAfter) {
		t.Fatalf("provider mutated registry-owned schema\nbefore: %s\n after: %s", localBefore, localAfter)
	}
	if !strings.Contains(string(localAfter), `"_semantic"`) || !strings.Contains(string(localAfter), `"integer"`) || !strings.Contains(string(localAfter), `"default"`) {
		t.Fatalf("local Inspect annotations were unexpectedly removed: %s", localAfter)
	}
}

func exact3V2InspectSchemaForResponsesTest() types.JSONSchema {
	semanticNumber := func(description string, minimum int) map[string]any {
		return map[string]any{
			"type": "number", "description": description, "minimum": minimum,
			"integer": true, "_semantic": "number",
		}
	}
	rangeSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"start": semanticNumber("Start line.", 1),
			"end":   semanticNumber("End line.", 1),
		},
		"required": []string{"start", "end"}, "additionalProperties": false,
	}
	requestSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id":          map[string]any{"type": "string", "description": "Request identifier."},
			"kind":        map[string]any{"type": "string", "enum": []string{"read", "search", "glob"}},
			"path":        map[string]any{"type": "string", "description": "Repository path."},
			"pattern":     map[string]any{"type": "string", "description": "Search pattern."},
			"ranges":      map[string]any{"type": "array", "items": rangeSchema},
			"context":     semanticNumber("Context lines.", 0),
			"max_results": semanticNumber("Maximum results.", 1),
		},
		"required": []string{"id", "kind"}, "additionalProperties": false,
	}
	maxChars := semanticNumber("Maximum returned characters.", 8_192)
	maxChars["default"] = 12_000
	return types.StrictObjectSchema(map[string]any{
		"requests":    map[string]any{"type": "array", "items": requestSchema},
		"cursor":      map[string]any{"type": "string", "description": "Pagination cursor."},
		"max_chars":   maxChars,
		"max_files":   semanticNumber("Maximum files.", 1),
		"max_matches": semanticNumber("Maximum matches.", 1),
	})
}

func TestOpenAIResponsesSchemaSanitizerPreservesNamedSchemaStructure(t *testing.T) {
	schema := types.StrictObjectSchema(map[string]any{
		"default": map[string]any{"type": "string", "title": "Property named default"},
		"choice": map[string]any{
			"description": "choice description",
			"oneOf": []any{
				map[string]any{"$ref": "#/$defs/text"},
				map[string]any{"anyOf": []any{map[string]any{"type": "number"}, map[string]any{"type": "null"}}},
			},
			"allOf": []any{map[string]any{"title": "retained title"}},
		},
	})
	schema.Defs = map[string]any{
		"text": map[string]any{"type": "string", "enum": []any{"a", "b"}, "default": "a"},
	}

	clean := canonicalOpenAIResponsesToolSchema(schema, true)
	properties := clean["properties"].(map[string]any)
	if _, ok := properties["default"]; !ok {
		t.Fatalf("property name was confused with a schema annotation: %#v", properties)
	}
	choice := properties["choice"].(map[string]any)
	if !openAIResponsesSchemaAcceptsNull(choice) {
		t.Fatalf("already nullable composed schema lost null support: %#v", choice)
	}
	for _, key := range []string{"description", "oneOf", "allOf"} {
		if _, ok := choice[key]; !ok {
			t.Fatalf("supported structure %q was removed: %#v", key, choice)
		}
	}
	defs := clean["$defs"].(map[string]any)
	text := defs["text"].(map[string]any)
	if text["type"] != "string" || text["enum"] == nil {
		t.Fatalf("$defs/$ref structure was not preserved: %#v", clean)
	}
	if _, ok := text["default"]; ok {
		t.Fatalf("nested default annotation was not removed: %#v", text)
	}
}

func TestDeepSeekResponsesDisablesStrictForOptionalInspectSchema(t *testing.T) {
	localSchema := exact3V2InspectSchemaForResponsesTest()
	converted := convertToolsToResponsesAPIForSemantics([]types.ToolDefinition{{
		Name: "Inspect", Description: "Inspect repository files in batches.",
		InputSchema: localSchema, Strict: true,
	}}, true, ResponsesSemanticsDeepSeek)
	if len(converted) != 1 {
		t.Fatalf("converted tools = %#v", converted)
	}
	tool := converted[0]
	if strict, present := tool["strict"].(bool); !present || strict {
		t.Fatalf("DeepSeek optional Inspect did not declare selective strict downgrade: %#v", tool)
	}
	parameters := tool["parameters"].(map[string]any)
	properties := parameters["properties"].(map[string]any)
	if properties["cursor"].(map[string]any)["type"] != "string" {
		t.Fatalf("DeepSeek cursor was rewritten instead of preserved as optional: %#v", properties["cursor"])
	}
	if required, present := parameters["required"]; present && len(required.([]any)) != 0 {
		t.Fatalf("DeepSeek root optional fields became required: %#v", required)
	}
	request := properties["requests"].(map[string]any)["items"].(map[string]any)
	if !reflect.DeepEqual(request["required"], []any{"id", "kind"}) {
		t.Fatalf("DeepSeek nested required fields changed: %#v", request["required"])
	}
	wire, err := json.Marshal(parameters)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{`"_semantic"`, `"integer"`, `"default"`} {
		if strings.Contains(string(wire), forbidden) {
			t.Fatalf("DeepSeek non-strict schema retained %s: %s", forbidden, wire)
		}
	}
}

func TestDeepSeekResponsesOmitsStrictWithoutSelectiveDowngrade(t *testing.T) {
	schema := exact3V2InspectSchemaForResponsesTest()
	tests := []struct {
		name       string
		strictMode bool
		toolStrict bool
	}{
		{name: "tool never requested strict", strictMode: true, toolStrict: false},
		{name: "request disabled strict mode", strictMode: false, toolStrict: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			converted := convertToolsToResponsesAPIForSemantics([]types.ToolDefinition{{
				Name: "Inspect", InputSchema: schema, Strict: test.toolStrict,
			}}, test.strictMode, ResponsesSemanticsDeepSeek)
			if _, present := converted[0]["strict"]; present {
				t.Fatalf("ordinary non-strict tool received an explicit flag: %#v", converted[0])
			}
		})
	}
}

func TestOpenAIResponsesNullableProjectionPreservesEnumAndConst(t *testing.T) {
	schema := types.StrictObjectSchema(map[string]any{
		"enum_value": map[string]any{
			"type": "string", "enum": []any{"read", "glob"},
		},
		"const_value": map[string]any{
			"type": "string", "const": "fixed",
		},
		"already_nullable_enum": map[string]any{
			"type": []any{"string", "null"}, "enum": []any{"read", nil},
		},
		"type_nullable_enum_rejects_null": map[string]any{
			"type": []any{"string", "null"}, "enum": []any{"read"},
		},
	})
	clean := canonicalOpenAIResponsesToolSchema(schema, true)
	properties := clean["properties"].(map[string]any)

	enumBranch := assertNullableResponsesSchema(t, properties["enum_value"].(map[string]any), "string")
	if !reflect.DeepEqual(enumBranch["enum"], []any{"read", "glob"}) {
		t.Fatalf("enum constraint changed while adding nullable branch: %#v", properties["enum_value"])
	}
	if responsesSchemaValuesIncludeNull(enumBranch["enum"].([]any)) {
		t.Fatalf("null was incorrectly inserted into original enum: %#v", enumBranch)
	}
	constBranch := assertNullableResponsesSchema(t, properties["const_value"].(map[string]any), "string")
	if constBranch["const"] != "fixed" {
		t.Fatalf("const constraint changed while adding nullable branch: %#v", properties["const_value"])
	}
	alreadyNullable := properties["already_nullable_enum"].(map[string]any)
	if _, wrapped := alreadyNullable["anyOf"]; wrapped {
		t.Fatalf("already nullable enum received a duplicate wrapper: %#v", alreadyNullable)
	}
	assertNullableResponsesSchema(t, alreadyNullable, "string")
	rejectingEnumBranch := assertNullableResponsesSchema(t, properties["type_nullable_enum_rejects_null"].(map[string]any), "string")
	if !reflect.DeepEqual(rejectingEnumBranch["enum"], []any{"read"}) {
		t.Fatalf("type-nullable enum constraint changed: %#v", properties["type_nullable_enum_rejects_null"])
	}
}

func TestDeepSeekResponsesKeepsStrictForFullyRequiredNestedSchema(t *testing.T) {
	nested := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id":   map[string]any{"type": "string"},
			"kind": map[string]any{"type": "string"},
		},
		"required": []string{"id", "kind"}, "additionalProperties": false,
	}
	schema := types.StrictObjectSchema(map[string]any{
		"request": nested,
		"limit":   map[string]any{"type": "number"},
	}, "request", "limit")
	converted := convertToolsToResponsesAPIForSemantics([]types.ToolDefinition{{
		Name: "FullyRequired", InputSchema: schema, Strict: true,
	}}, true, ResponsesSemanticsDeepSeek)
	tool := converted[0]
	if strict, _ := tool["strict"].(bool); !strict {
		t.Fatalf("fully-required DeepSeek tool lost strict mode: %#v", tool)
	}
	parameters := tool["parameters"].(map[string]any)
	assertOpenAIResponsesStrictObjects(t, parameters, "parameters")
	properties := parameters["properties"].(map[string]any)
	assertResponsesSchemaNotNullable(t, properties["limit"].(map[string]any), "number")
	request := properties["request"].(map[string]any)
	assertResponsesSchemaNotNullable(t, request["properties"].(map[string]any)["id"].(map[string]any), "string")
}

func assertNullableResponsesSchema(t *testing.T, node map[string]any, nonNullType string) map[string]any {
	t.Helper()
	if nonNullType == "" && openAIResponsesSchemaAcceptsNull(node) {
		return node
	}
	if types, ok := node["type"].([]any); ok {
		hasNull := false
		hasNonNull := nonNullType == ""
		for _, candidate := range types {
			hasNull = hasNull || candidate == "null"
			hasNonNull = hasNonNull || candidate == nonNullType
		}
		if hasNull && hasNonNull {
			return node
		}
	}
	alternatives, ok := node["anyOf"].([]any)
	if !ok {
		t.Fatalf("schema is not nullable: %#v", node)
	}
	var nonNull map[string]any
	hasNull := false
	for _, alternative := range alternatives {
		candidate, _ := alternative.(map[string]any)
		if openAIResponsesSchemaAcceptsNull(candidate) {
			hasNull = true
			continue
		}
		if nonNull == nil {
			nonNull = candidate
		}
	}
	if !hasNull || nonNull == nil || (nonNullType != "" && !openAIResponsesSchemaIncludesType(nonNull["type"], nonNullType)) {
		t.Fatalf("schema has invalid nullable alternatives: %#v", node)
	}
	return nonNull
}

func assertResponsesSchemaNotNullable(t *testing.T, node map[string]any, expectedType string) {
	t.Helper()
	if !openAIResponsesSchemaIncludesType(node["type"], expectedType) || openAIResponsesSchemaIncludesType(node["type"], "null") {
		t.Fatalf("required schema type = %#v, want non-nullable %q", node["type"], expectedType)
	}
	if alternatives, ok := node["anyOf"].([]any); ok {
		for _, alternative := range alternatives {
			candidate, _ := alternative.(map[string]any)
			if openAIResponsesSchemaIncludesType(candidate["type"], "null") {
				t.Fatalf("required schema unexpectedly accepts null: %#v", node)
			}
		}
	}
}

func assertOpenAIResponsesStrictObjects(t *testing.T, value any, path string) {
	t.Helper()
	switch node := value.(type) {
	case map[string]any:
		if openAIResponsesSchemaIncludesType(node["type"], "object") {
			properties, ok := node["properties"].(map[string]any)
			if !ok {
				t.Fatalf("%s properties = %#v", path, node["properties"])
			}
			if additional, ok := node["additionalProperties"].(bool); !ok || additional {
				t.Fatalf("%s additionalProperties = %#v", path, node["additionalProperties"])
			}
			required, ok := node["required"].([]any)
			if !ok {
				// Direct sanitizer calls retain []string; HTTP decoding yields []any.
				if stringsRequired, stringsOK := node["required"].([]string); stringsOK {
					required = make([]any, len(stringsRequired))
					for index := range stringsRequired {
						required[index] = stringsRequired[index]
					}
				} else {
					t.Fatalf("%s required = %#v", path, node["required"])
				}
			}
			seen := make(map[string]bool, len(required))
			for _, item := range required {
				name, _ := item.(string)
				seen[name] = true
			}
			for name := range properties {
				if !seen[name] {
					t.Fatalf("%s required does not cover property %q: %#v", path, name, required)
				}
			}
		}
		for key, child := range node {
			assertOpenAIResponsesStrictObjects(t, child, path+"."+key)
		}
	case []any:
		for index, child := range node {
			assertOpenAIResponsesStrictObjects(t, child, path+"[]")
			_ = index
		}
	}
}
