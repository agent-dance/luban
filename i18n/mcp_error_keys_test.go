package i18n

import (
	"errors"
	"strings"
	"testing"
)

func TestServicesMCPErrorCatalogCoversEveryLanguage(t *testing.T) {
	const wantKeys = 99
	keys := make([]Key, 0, wantKeys)
	for key := range semanticTranslations {
		if strings.HasPrefix(string(key), "services.mcp.") {
			keys = append(keys, key)
		}
	}
	if len(keys) != wantKeys {
		t.Fatalf("services.mcp semantic key count = %d, want %d", len(keys), wantKeys)
	}
	for _, key := range keys {
		for _, lang := range AllLanguages() {
			copy := Text(lang, key)
			if copy == "" || strings.HasPrefix(copy, "[") {
				t.Errorf("Text(%s, %q) = %q", lang.Code(), key, copy)
			}
		}
	}
}

func TestServicesMCPWebSocketHeaderInvalidCoversEveryLanguage(t *testing.T) {
	for _, lang := range AllLanguages() {
		got := Text(lang, KeyServicesMCPWebSocketHeaderInvalid)
		if got == "" || strings.HasPrefix(got, "[") || strings.Contains(got, "%!") {
			t.Errorf("Text(%s, %q) = %q", lang.Code(), KeyServicesMCPWebSocketHeaderInvalid, got)
		}
	}
}

func TestServicesMCPJSONRPCConstructionKeysCoverEveryLanguage(t *testing.T) {
	keys := []Key{
		KeyServicesMCPJSONRPCRequestMethodMissing,
		KeyServicesMCPJSONRPCNotifyMethodMissing,
		KeyServicesMCPJSONRPCResultIDMissing,
		KeyServicesMCPJSONRPCErrorIDMissing,
		KeyServicesMCPJSONRPCEncodeRequestParams,
		KeyServicesMCPJSONRPCEncodeNotifyParams,
		KeyServicesMCPJSONRPCEncodeResult,
		KeyServicesMCPJSONRPCEncodeErrorData,
	}
	for _, key := range keys {
		for _, lang := range AllLanguages() {
			got := Text(lang, key)
			if got == "" || strings.HasPrefix(got, "[") || strings.Contains(got, "%!") {
				t.Errorf("Text(%s, %q) = %q", lang.Code(), key, got)
			}
		}
	}
}

func TestServicesMCPErrorCatalogPreservesRawProtocolValues(t *testing.T) {
	for _, lang := range AllLanguages() {
		got := Format(lang, KeyServicesMCPRemoteStatusDetail, "HTTP 599", "raw-remote-body-17")
		for _, raw := range []string{"HTTP 599", "raw-remote-body-17"} {
			if !strings.Contains(got, raw) {
				t.Errorf("%s omitted raw value %q: %q", lang.Code(), raw, got)
			}
		}
		if strings.Contains(got, "%!") {
			t.Errorf("%s has a formatting error: %q", lang.Code(), got)
		}
	}
}

func TestServicesMCPWrappedErrorsFollowRuntimeLanguageAndPreserveCause(t *testing.T) {
	previous := detectedLanguageCache.Load()
	t.Cleanup(func() { detectedLanguageCache.Store(previous) })

	cause := errors.New("raw-transport-cause-23")
	err := WrapError(KeyServicesMCPResolveAuthToken, cause)
	if !errors.Is(err, cause) {
		t.Fatal("MCP semantic error did not preserve its cause")
	}

	detectedLanguageCache.Store(int32(LangEN))
	english := err.Error()
	detectedLanguageCache.Store(int32(LangZH))
	chinese := err.Error()
	if english == chinese || !strings.Contains(chinese, cause.Error()) {
		t.Fatalf("runtime localization failed: en=%q zh=%q", english, chinese)
	}
}
