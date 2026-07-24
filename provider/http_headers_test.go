package provider

import (
	"reflect"
	"testing"
)

func TestParseHeaderLines(t *testing.T) {
	got := parseHeaderLines("X-Test: 1\nAuthorization: Bearer abc\r\nInvalid\n: nope\nAnother: two:parts\n")
	want := map[string]string{
		"X-Test":        "1",
		"Authorization": "Bearer abc",
		"Another":       "two:parts",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseHeaderLines() = %#v, want %#v", got, want)
	}
}
