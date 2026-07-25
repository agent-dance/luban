package file

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

const (
	pdfInfoTimeout   = 20 * time.Second
	pdfRenderTimeout = 120 * time.Second
)

// PDFErrorReason mirrors the structured failure reasons in the TS PDF helper.
// Read still returns tool-level error text, while callers can inspect the
// reason without parsing that text.
type PDFErrorReason string

const (
	PDFErrorEmpty             PDFErrorReason = "empty"
	PDFErrorTooLarge          PDFErrorReason = "too_large"
	PDFErrorPasswordProtected PDFErrorReason = "password_protected"
	PDFErrorCorrupted         PDFErrorReason = "corrupted"
	PDFErrorUnknown           PDFErrorReason = "unknown"
	PDFErrorUnavailable       PDFErrorReason = "unavailable"
)

type PDFError struct {
	Reason  PDFErrorReason
	Message string
	display error
	cause   error
}

func (e *PDFError) Error() string {
	if e == nil {
		return ""
	}
	if e.display != nil {
		return e.display.Error()
	}
	return e.Message
}

func (e *PDFError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func newPDFError(reason PDFErrorReason, key i18n.Key, args ...any) error {
	display := i18n.NewError(key, args...)
	return &PDFError{Reason: reason, Message: display.Error(), display: display}
}

func wrapPDFError(reason PDFErrorReason, key i18n.Key, cause error, args ...any) error {
	display := i18n.WrapError(key, cause, args...)
	return &PDFError{Reason: reason, Message: display.Error(), display: display, cause: cause}
}

// pdfErrorWithCause preserves a renderer or subprocess cause without adding
// it to stable user-facing copy. Raw output that belongs in the message must
// be passed separately as a semantic-key argument.
func pdfErrorWithCause(reason PDFErrorReason, key i18n.Key, cause error, args ...any) error {
	display := i18n.NewError(key, args...)
	return &PDFError{Reason: reason, Message: display.Error(), display: display, cause: cause}
}

var (
	pdfInfoPageCountPattern = regexp.MustCompile(`(?m)^Pages:\s+(\d+)\s*$`)
)

func getPDFPageCount(filePath string) (int, bool) {
	return getPDFPageCountWithPDFInfo(filePath)
}

func readPDFFile(filePath string) ([]byte, int64, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, 0, wrapPDFError(PDFErrorUnknown, i18n.KeyToolPDFHelperReadFileFailed, err)
	}
	if info.Size() == 0 {
		return nil, 0, newPDFError(PDFErrorEmpty, i18n.KeyToolPDFHelperFileEmpty, filePath)
	}
	if info.Size() > pdfTargetRawSize {
		return nil, 0, newPDFError(PDFErrorTooLarge, i18n.KeyToolPDFHelperFileTooLarge, formatReadSize(pdfTargetRawSize))
	}

	file, err := os.Open(filePath)
	if err != nil {
		return nil, 0, wrapPDFError(PDFErrorUnknown, i18n.KeyToolPDFHelperReadFileFailed, err)
	}
	defer file.Close()

	header := make([]byte, 5)
	n, err := io.ReadFull(file, header)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return nil, 0, wrapPDFError(PDFErrorUnknown, i18n.KeyToolPDFHelperReadFileFailed, err)
	}
	if !bytes.Equal(header[:n], []byte("%PDF-")) {
		return nil, 0, newPDFError(PDFErrorCorrupted, i18n.KeyToolPDFHelperInvalidHeader, filePath)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, 0, wrapPDFError(PDFErrorUnknown, i18n.KeyToolPDFHelperReadFileFailed, err)
	}
	return data, info.Size(), nil
}

type pdfPageExtraction struct {
	Images       []types.ImageBlock
	Count        int
	OriginalSize int64
	OutputDir    string
}

func extractPDFPageImages(filePath string, firstPage, lastPage int, toolResultsDir string) (result pdfPageExtraction, err error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return result, wrapPDFError(PDFErrorUnknown, i18n.KeyToolPDFHelperReadFileFailed, err)
	}
	if info.Size() == 0 {
		return result, newPDFError(PDFErrorEmpty, i18n.KeyToolPDFHelperFileEmpty, filePath)
	}
	if info.Size() > pdfMaxExtractSize {
		return result, newPDFError(
			PDFErrorTooLarge,
			i18n.KeyToolPDFHelperExtractionFileTooLarge,
			formatReadSize(pdfMaxExtractSize),
		)
	}
	result.OriginalSize = info.Size()

	// Strict %PDF- magic byte check before invoking the renderer
	// subprocess. Mirrors TS pdf.ts which rejects HTML-as-PDF / ZIP-as-PDF
	// poisoning with a clear error rather than the opaque "corrupt"
	// message pdftoppm produces. See fr-pdf-magic-byte-strict.
	if err := verifyPDFMagicHeader(filePath); err != nil {
		return result, err
	}
	if !hasPDFToPPM() {
		return result, newPDFError(PDFErrorUnavailable, i18n.KeyToolPDFHelperRendererUnavailable)
	}
	if strings.TrimSpace(toolResultsDir) == "" {
		toolResultsDir = filepath.Join(os.TempDir(), "luban-code", "tool-results")
	}
	if err := os.MkdirAll(toolResultsDir, 0o755); err != nil {
		return result, i18n.WrapError(i18n.KeyToolPDFHelperCreateResultsDirectory, err)
	}
	dir, err := os.MkdirTemp(toolResultsDir, "pdf-*")
	if err != nil {
		return result, i18n.WrapError(i18n.KeyToolPDFHelperCreateExtractionDirectory, err)
	}
	result.OutputDir = dir
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(dir)
			result.OutputDir = ""
		}
	}()

	if err := extractPDFPagesWithPDFToPPM(filePath, dir, firstPage, lastPage); err != nil {
		return result, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return result, i18n.WrapError(i18n.KeyToolPDFHelperReadExtractionOutput, err)
	}

	imageNames := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(strings.ToLower(name), ".jpg") {
			imageNames = append(imageNames, name)
		}
	}
	sort.Strings(imageNames)
	if len(imageNames) == 0 {
		return result, i18n.NewError(i18n.KeyToolPDFHelperNoOutputPages)
	}

	images := make([]types.ImageBlock, 0, len(imageNames))
	for _, name := range imageNames {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return result, i18n.WrapError(i18n.KeyToolPDFHelperReadExtractedPageImage, err)
		}
		images = append(images, types.ImageBlock{
			Type: types.ContentTypeImage,
			Source: &types.ImageSource{
				Type:      "base64",
				MediaType: "image/jpeg",
				Data:      encodeBase64(data),
			},
		})
	}

	result.Images = images
	result.Count = len(images)
	cleanup = false
	return result, nil
}

func hasPDFToPPM() bool {
	pdftoppmAvailableOnce.Do(func() {
		_, err := exec.LookPath("pdftoppm")
		pdftoppmAvailableCache = err == nil
	})
	return pdftoppmAvailableCache
}

// pdftoppmAvailableOnce caches the LookPath result for the process
// lifetime. Mirrors TS pdftoppmAvailable which is also memoised. Calling
// LookPath on every PDF read is wasteful in long-lived sessions.
var (
	pdftoppmAvailableOnce  sync.Once
	pdftoppmAvailableCache bool
)

// verifyPDFMagicHeader reads the first 5 bytes and confirms they match
// the PDF magic prefix "%PDF-". Used by paths that bypass readPDFFile
// (e.g. extractPDFPageImages) so non-PDF inputs are rejected with a clear
// message before any renderer subprocess is spawned. Mirrors TS pdf.ts.
func verifyPDFMagicHeader(filePath string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return wrapPDFError(PDFErrorUnknown, i18n.KeyToolPDFHelperReadFileFailed, err)
	}
	defer f.Close()
	header := make([]byte, 5)
	n, err := io.ReadFull(f, header)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return wrapPDFError(PDFErrorUnknown, i18n.KeyToolPDFHelperReadFileFailed, err)
	}
	if !bytes.Equal(header[:n], []byte("%PDF-")) {
		return newPDFError(PDFErrorCorrupted, i18n.KeyToolPDFHelperInvalidHeader, filePath)
	}
	return nil
}

func getPDFPageCountWithPDFInfo(filePath string) (int, bool) {
	if _, err := exec.LookPath("pdfinfo"); err != nil {
		return 0, false
	}

	ctx, cancel := context.WithTimeout(context.Background(), pdfInfoTimeout)
	defer cancel()

	output, err := exec.CommandContext(ctx, "pdfinfo", filePath).CombinedOutput()
	if err != nil {
		return 0, false
	}
	match := pdfInfoPageCountPattern.FindStringSubmatch(string(output))
	if len(match) != 2 {
		return 0, false
	}
	count, err := strconv.Atoi(match[1])
	if err != nil {
		return 0, false
	}
	return count, true
}

func extractPDFPagesWithPDFToPPM(filePath, outputDir string, firstPage, lastPage int) error {
	args := []string{"-jpeg", "-r", "100"}
	if firstPage > 0 {
		args = append(args, "-f", strconv.Itoa(firstPage))
	}
	if lastPage > 0 && lastPage != math.MaxInt {
		args = append(args, "-l", strconv.Itoa(lastPage))
	}
	args = append(args, filePath, filepath.Join(outputDir, "page"))

	ctx, cancel := context.WithTimeout(context.Background(), pdfRenderTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "pdftoppm", args...)
	cmd.Env = pdfRendererEnvironment()
	output, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	stderr := strings.TrimSpace(string(output))
	if strings.Contains(strings.ToLower(stderr), "password") {
		return pdfErrorWithCause(PDFErrorPasswordProtected, i18n.KeyToolPDFHelperPasswordProtected, err)
	}
	if strings.Contains(strings.ToLower(stderr), "damaged") ||
		strings.Contains(strings.ToLower(stderr), "corrupt") ||
		strings.Contains(strings.ToLower(stderr), "invalid") {
		return pdfErrorWithCause(PDFErrorCorrupted, i18n.KeyToolPDFHelperCorrupted, err)
	}
	if stderr == "" {
		stderr = err.Error()
	}
	return pdfErrorWithCause(PDFErrorUnknown, i18n.KeyToolPDFHelperPDFToPPMFailed, err, stderr)
}

func pdfRendererEnvironment() []string {
	environment := os.Environ()
	if strings.TrimSpace(os.Getenv("FONTCONFIG_FILE")) != "" || strings.TrimSpace(os.Getenv("FONTCONFIG_PATH")) != "" {
		return environment
	}
	for _, candidate := range []string{
		"/opt/homebrew/etc/fonts/fonts.conf",
		"/usr/local/etc/fonts/fonts.conf",
		"/etc/fonts/fonts.conf",
	} {
		if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
			return append(environment, "FONTCONFIG_FILE="+candidate)
		}
	}
	return environment
}

func encodeBase64(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}
