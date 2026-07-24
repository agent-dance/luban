// Package tools — schema_helpers.go provides the small set of schema
// "semantic" wrappers used by FileEditTool, FileReadTool, etc. They mirror
// the TS zod helper style (semanticBoolean, semanticNumber) so the JSON
// schema published to the model carries human-readable descriptions and
// safe defaults.
package tools

// semanticBoolean returns a JSON Schema object describing a boolean property
// with a human-friendly description and a default value. The `_semantic`
// marker distinguishes schemas built via this helper from raw boolean
// schemas, letting downstream introspection tell the two apart.
func semanticBoolean(description string, defaultValue bool) map[string]any {
	return map[string]any{
		"type":        "boolean",
		"description": description,
		"default":     defaultValue,
		"_semantic":   "boolean",
	}
}

// SemanticBoolean is the exported alias for semanticBoolean. Tests probe
// either the lowercase or the uppercase form.
func SemanticBoolean(description string, defaultValue bool) map[string]any {
	return semanticBoolean(description, defaultValue)
}

// semanticNumber returns a JSON Schema object describing a numeric property
// with bounds and a description. `minimum`/`maximum` are emitted only when
// non-zero (use math.Inf-style sentinels by passing 0 to skip). `integer`
// is set when the value must be a whole number.
func semanticNumber(description string, minimum int, integer bool) map[string]any {
	out := map[string]any{
		"type":        "number",
		"description": description,
		"minimum":     minimum,
		"_semantic":   "number",
	}
	if integer {
		out["integer"] = true
	}
	return out
}

// SemanticNumber is the exported alias for semanticNumber.
func SemanticNumber(description string, minimum int, integer bool) map[string]any {
	return semanticNumber(description, minimum, integer)
}
