package i18n

import (
	"errors"
	"strings"
	"testing"
)

func TestToolWebValidationKeysCoverEveryLanguage(t *testing.T) {
	for _, key := range toolWebValidationKeys {
		for _, lang := range AllLanguages() {
			got := Text(lang, key)
			if got == "" || got == "["+string(key)+"]" {
				t.Errorf("Text(%s, %q) = %q", lang.Code(), key, got)
			}
		}
	}
}

func TestToolWebValidationEnglishContractContracts(t *testing.T) {
	cause := errors.New("raw-cause")
	tests := []struct {
		key  Key
		args []any
		want string
	}{
		{KeyToolWebValidationURLTooLong, []any{2000}, "URL exceeds max length of 2000 characters"},
		{KeyToolWebValidationInvalidURL, []any{cause}, "invalid URL: raw-cause"},
		{KeyToolWebValidationUnsupportedScheme, []any{"ftp"}, `unsupported scheme "ftp": only http and https are allowed`},
		{KeyToolWebValidationUserinfoForbidden, nil, "URL must not contain userinfo (user:password@)"},
		{KeyToolWebValidationHostnameMissing, nil, "URL has no hostname"},
		{KeyToolWebValidationHostnameNotPublic, []any{"intranet"}, `URL hostname "intranet" is not a public domain`},
		{KeyToolWebValidationResolveHostname, []any{"raw.example", cause}, `failed to resolve hostname "raw.example": raw-cause`},
		{KeyToolWebValidationLoopbackAddress, []any{"127.0.0.1"}, "URL resolves to loopback address 127.0.0.1"},
		{KeyToolWebValidationPrivateAddress, []any{"10.0.0.1"}, "URL resolves to private address 10.0.0.1"},
		{KeyToolWebValidationLinkLocalAddress, []any{"169.254.1.1"}, "URL resolves to link-local address 169.254.1.1"},
		{KeyToolWebValidationUnspecifiedAddress, []any{"0.0.0.0"}, "URL resolves to unspecified address 0.0.0.0"},
		{KeyToolWebValidationCloudMetadataAddress, []any{"169.254.169.254"}, "URL resolves to cloud metadata endpoint 169.254.169.254"},
		{KeyToolWebValidationRedirectLimit, []any{10}, "stopped after 10 redirects"},
		{KeyToolWebDomainSafetyCheckFailed, []any{"raw.example"}, "Unable to verify if domain raw.example is safe to fetch. This may be due to network restrictions or enterprise security policies blocking claude.ai."},
		{KeyToolWebDomainEmptyHostname, nil, "empty hostname"},
		{KeyToolWebDomainBuildRequest, []any{cause}, "domain_info preflight: build request: raw-cause"},
		{KeyToolWebDomainRequest, []any{cause}, "domain_info request: raw-cause"},
		{KeyToolWebDomainStatus, []any{503}, "domain check returned status 503"},
		{KeyToolWebDomainDecodeResponse, []any{cause}, "decode domain_info response: raw-cause"},
		{KeyToolWebDomainMissingCanFetch, nil, "domain_info response missing boolean can_fetch"},
	}

	for _, tt := range tests {
		if got := Format(LangEN, tt.key, tt.args...); got != tt.want {
			t.Errorf("Format(LangEN, %q) = %q, want %q", tt.key, got, tt.want)
		}
	}
}

func TestToolWebValidationTranslationsPreserveRawNetworkValues(t *testing.T) {
	cause := errors.New(`lookup raw.dns.example at 192.0.2.53: raw DNS cause`)
	tests := []struct {
		key  Key
		args []any
		raw  []string
	}{
		{KeyToolWebValidationInvalidURL, []any{errors.New(`parse "https://raw.example/%zz": invalid URL escape`)}, []string{"https://raw.example/%zz", "invalid URL escape"}},
		{KeyToolWebValidationUnsupportedScheme, []any{"raw-scheme"}, []string{"raw-scheme"}},
		{KeyToolWebValidationHostnameNotPublic, []any{"raw-host"}, []string{"raw-host"}},
		{KeyToolWebValidationResolveHostname, []any{"raw-host", cause}, []string{"raw-host", "raw.dns.example", "192.0.2.53", "raw DNS cause"}},
		{KeyToolWebValidationPrivateAddress, []any{"10.23.45.67"}, []string{"10.23.45.67"}},
		{KeyToolWebDomainSafetyCheckFailed, []any{"raw-domain.example"}, []string{"raw-domain.example", "claude.ai"}},
		{KeyToolWebDomainStatus, []any{599}, []string{"599"}},
		{KeyToolWebDomainRequest, []any{cause}, []string{"raw.dns.example", "192.0.2.53", "raw DNS cause"}},
	}

	for _, lang := range AllLanguages() {
		for _, tt := range tests {
			got := Format(lang, tt.key, tt.args...)
			if strings.Contains(got, "%!") {
				t.Errorf("Format(%s, %q) has a formatting error: %q", lang.Code(), tt.key, got)
			}
			for _, raw := range tt.raw {
				if !strings.Contains(got, raw) {
					t.Errorf("Format(%s, %q) omitted raw value %q: %q", lang.Code(), tt.key, raw, got)
				}
			}
		}
	}
}

type typedWebValidationCause struct{}

func (*typedWebValidationCause) Error() string { return "typed-raw-cause" }

func TestToolWebValidationSemanticErrorLocalizesAtRenderAndPreservesCause(t *testing.T) {
	previous := detectedLanguageCache.Load()
	t.Cleanup(func() { detectedLanguageCache.Store(previous) })

	cause := &typedWebValidationCause{}
	err := WrapError(KeyToolWebValidationResolveHostname, cause, "raw.example")
	if !errors.Is(err, cause) {
		t.Fatal("WrapError did not preserve errors.Is")
	}
	var typed *typedWebValidationCause
	if !errors.As(err, &typed) {
		t.Fatal("WrapError did not preserve errors.As")
	}

	detectedLanguageCache.Store(int32(LangEN))
	english := err.Error()
	detectedLanguageCache.Store(int32(LangZH))
	chinese := err.Error()
	if english == chinese {
		t.Fatalf("error did not follow the runtime language: en=%q zh=%q", english, chinese)
	}
	for _, got := range []string{english, chinese} {
		for _, raw := range []string{"raw.example", "typed-raw-cause"} {
			if !strings.Contains(got, raw) {
				t.Fatalf("localized error omitted %q: %q", raw, got)
			}
		}
	}
}
