package i18n

import (
	"errors"
	"strings"
	"testing"
)

type toolPDFHelperTestCause struct{}

func (*toolPDFHelperTestCause) Error() string { return "raw-pdf-cause-42" }

func TestToolPDFHelperKeysCoverEveryLanguage(t *testing.T) {
	for _, key := range toolPDFHelperKeys {
		for _, lang := range AllLanguages() {
			got := Text(lang, key)
			if got == "" || got == "["+string(key)+"]" {
				t.Errorf("%s is missing for %s: %q", key, lang.Code(), got)
			}
		}
	}
	if err := ValidateSemanticCatalog(); err != nil {
		t.Fatal(err)
	}
}

func TestToolPDFHelperEnglishCompatibility(t *testing.T) {
	cause := errors.New("raw-cause")
	cases := []struct {
		key  Key
		args []any
		want string
	}{
		{KeyToolPDFHelperReadFileFailed, []any{cause}, "failed to read file: raw-cause"},
		{KeyToolPDFHelperFileEmpty, []any{"/tmp/empty.pdf"}, "PDF file is empty: /tmp/empty.pdf"},
		{KeyToolPDFHelperFileTooLarge, []any{"20MB"}, "PDF file exceeds maximum allowed size of 20MB."},
		{KeyToolPDFHelperInvalidHeader, []any{"/tmp/bad.pdf"}, "File is not a valid PDF (missing %PDF- header): /tmp/bad.pdf"},
		{KeyToolPDFHelperExtractionFileTooLarge, []any{"100MB"}, "PDF file exceeds maximum allowed size for text extraction (100MB)."},
		{KeyToolPDFHelperRendererUnavailable, nil, "pdftoppm is not installed. Install poppler-utils (e.g. `brew install poppler` or `apt-get install poppler-utils`) to enable PDF page rendering."},
		{KeyToolPDFHelperCreateResultsDirectory, []any{cause}, "failed to create tool results directory: raw-cause"},
		{KeyToolPDFHelperCreateExtractionDirectory, []any{cause}, "failed to create PDF extraction directory: raw-cause"},
		{KeyToolPDFHelperReadExtractionOutput, []any{cause}, "failed to read PDF extraction output: raw-cause"},
		{KeyToolPDFHelperNoOutputPages, nil, "pdftoppm produced no output pages. The PDF may be invalid."},
		{KeyToolPDFHelperReadExtractedPageImage, []any{cause}, "failed to read extracted PDF page image: raw-cause"},
		{KeyToolPDFHelperPasswordProtected, nil, "PDF is password-protected. Please provide an unprotected version."},
		{KeyToolPDFHelperCorrupted, nil, "PDF file is corrupted or invalid."},
		{KeyToolPDFHelperPDFToPPMFailed, []any{"raw stderr"}, "pdftoppm failed: raw stderr"},
	}

	for _, tc := range cases {
		if got := Format(LangEN, tc.key, tc.args...); got != tc.want {
			t.Errorf("Format(LangEN, %s) = %q, want %q", tc.key, got, tc.want)
		}
	}
}

func TestToolPDFHelperErrorsUseActiveLanguageAndPreserveCause(t *testing.T) {
	previous := detectedLanguageCache.Load()
	t.Cleanup(func() { detectedLanguageCache.Store(previous) })

	cause := &toolPDFHelperTestCause{}
	err := WrapError(KeyToolPDFHelperReadFileFailed, cause)
	if !errors.Is(err, cause) {
		t.Fatal("PDF helper error did not preserve its underlying cause")
	}
	var typedCause *toolPDFHelperTestCause
	if !errors.As(err, &typedCause) || typedCause != cause {
		t.Fatal("PDF helper error did not preserve its typed underlying cause")
	}

	detectedLanguageCache.Store(int32(LangEN))
	english := err.Error()
	if english != "failed to read file: raw-pdf-cause-42" {
		t.Fatalf("English compatibility changed: %q", english)
	}
	detectedLanguageCache.Store(int32(LangZH))
	chinese := err.Error()
	if english == chinese || !strings.Contains(chinese, "raw-pdf-cause-42") {
		t.Fatalf("runtime localization failed: en=%q zh=%q", english, chinese)
	}
}
