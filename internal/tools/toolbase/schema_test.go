package toolbase

import "testing"

func TestSemanticBooleanContract(t *testing.T) {
	property := SemanticBoolean("flag", true)
	if property["type"] != "boolean" || property["default"] != true || property["_semantic"] != "boolean" {
		t.Fatalf("semantic boolean = %#v", property)
	}
}

func TestSemanticNumberContract(t *testing.T) {
	property := SemanticNumber("count", 1, true)
	if property["type"] != "number" || property["minimum"] != 1 || property["integer"] != true || property["_semantic"] != "number" {
		t.Fatalf("semantic number = %#v", property)
	}
}
