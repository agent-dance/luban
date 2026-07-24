package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"mime"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/agent-dance/luban/compact"
	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

func pdfErrorToolResult(err error) types.ToolResult {
	result := ErrorResponse(err)
	var typed *PDFError
	if errors.As(err, &typed) {
		result.Metadata = map[string]string{"pdfErrorReason": string(typed.Reason)}
	}
	return result
}

const (
	defaultReadMaxSizeBytes     = 256 * 1024
	defaultReadMaxTokens        = 25000
	pdfTargetRawSize            = 20 * 1024 * 1024
	pdfMaxExtractSize           = 100 * 1024 * 1024
	pdfAtMentionInlineThreshold = 10
	pdfMaxPagesPerRead          = 20
	pdfExtractSizeThreshold     = 3 * 1024 * 1024

	// PDFMaxPagesPerRead exposes the maximum number of pages a single PDF
	// Read operation may surface inline. Mirrors TS PDF_MAX_PAGES_PER_READ.
	PDFMaxPagesPerRead = pdfMaxPagesPerRead

	// PDFAtMentionInlineThreshold exposes the cutoff above which a PDF is
	// reported as variant=large-pdf rather than fully inlined.
	PDFAtMentionInlineThreshold = pdfAtMentionInlineThreshold
)

func (t *FileReadTool) pdfToolResultsDir() string {
	if t != nil && t.ToolResultsDirProvider != nil {
		if dir := strings.TrimSpace(t.ToolResultsDirProvider()); dir != "" {
			return dir
		}
	}
	if root := strings.TrimSpace(t.runtimeSnapshot().ProjectRoot); root != "" {
		return filepath.Join(root, ".claude", "tool-results")
	}
	return filepath.Join(os.TempDir(), "claude-code-go", "tool-results")
}

var (
	readTokenCounter = compact.NewTiktokenCounter()
	imageExtensions  = map[string]bool{
		".png":  true,
		".jpg":  true,
		".jpeg": true,
		".gif":  true,
		".webp": true,
	}
	binaryExtensions = map[string]bool{
		".png":     true,
		".jpg":     true,
		".jpeg":    true,
		".gif":     true,
		".bmp":     true,
		".ico":     true,
		".webp":    true,
		".tiff":    true,
		".tif":     true,
		".mp4":     true,
		".mov":     true,
		".avi":     true,
		".mkv":     true,
		".webm":    true,
		".wmv":     true,
		".flv":     true,
		".m4v":     true,
		".mpeg":    true,
		".mpg":     true,
		".mp3":     true,
		".wav":     true,
		".ogg":     true,
		".flac":    true,
		".aac":     true,
		".m4a":     true,
		".wma":     true,
		".aiff":    true,
		".opus":    true,
		".zip":     true,
		".tar":     true,
		".gz":      true,
		".bz2":     true,
		".7z":      true,
		".rar":     true,
		".xz":      true,
		".z":       true,
		".tgz":     true,
		".iso":     true,
		".exe":     true,
		".dll":     true,
		".so":      true,
		".dylib":   true,
		".bin":     true,
		".o":       true,
		".a":       true,
		".obj":     true,
		".lib":     true,
		".app":     true,
		".msi":     true,
		".deb":     true,
		".rpm":     true,
		".pdf":     true,
		".doc":     true,
		".docx":    true,
		".xls":     true,
		".xlsx":    true,
		".ppt":     true,
		".pptx":    true,
		".odt":     true,
		".ods":     true,
		".odp":     true,
		".ttf":     true,
		".otf":     true,
		".woff":    true,
		".woff2":   true,
		".eot":     true,
		".pyc":     true,
		".pyo":     true,
		".class":   true,
		".jar":     true,
		".war":     true,
		".ear":     true,
		".node":    true,
		".wasm":    true,
		".rlib":    true,
		".sqlite":  true,
		".sqlite3": true,
		".db":      true,
		".mdb":     true,
		".idx":     true,
		".psd":     true,
		".ai":      true,
		".eps":     true,
		".sketch":  true,
		".fig":     true,
		".xd":      true,
		".blend":   true,
		".3ds":     true,
		".max":     true,
		".swf":     true,
		".fla":     true,
		".lockb":   true,
		".dat":     true,
		".data":    true,
	}
	blockedDevicePaths = map[string]bool{
		"/dev/zero":    true,
		"/dev/random":  true,
		"/dev/urandom": true,
		"/dev/full":    true,
		"/dev/stdin":   true,
		"/dev/tty":     true,
		"/dev/console": true,
		"/dev/stdout":  true,
		"/dev/stderr":  true,
		"/dev/fd/0":    true,
		"/dev/fd/1":    true,
		"/dev/fd/2":    true,
	}
)

func estimateReadTokens(text string) int {
	return readTokenCounter.Count(text)
}

func fileReadTokenLimitError(filePath string, tokenCount, maxTokens int) types.ToolResult {
	result := toolRuntimeErrorf(
		i18n.KeyToolReadTokenLimit,
		tokenCount,
		maxTokens,
	)
	result = structuredFileError(result.Content, fileErrorReadTokenLimit, filePath, true, nil, readFileRetry(filePath, 1, 2000))
	result.Completeness = types.ToolResultCompleteness{Source: types.ToolResultCompletenessCaptureDropped}
	return result
}

func fileReadSizeLimitError(filePath string, size, maxSize int64) types.ToolResult {
	result := toolRuntimeErrorf(
		i18n.KeyToolReadSizeLimit,
		formatReadSize(size),
		formatReadSize(maxSize),
	)
	result = structuredFileError(result.Content, fileErrorReadSizeLimit, filePath, true, nil, readFileRetry(filePath, 1, 2000))
	result.Completeness = types.ToolResultCompleteness{Source: types.ToolResultCompletenessCaptureDropped}
	return result
}

func formatReadSize(size int64) string {
	kb := float64(size) / 1024
	if kb < 1 {
		return toolRuntimeFormat(i18n.KeyToolReadBytes, size)
	}
	if kb < 1024 {
		return trimTrailingZero(kb) + "KB"
	}
	mb := kb / 1024
	if mb < 1024 {
		return trimTrailingZero(mb) + "MB"
	}
	gb := mb / 1024
	return trimTrailingZero(gb) + "GB"
}

func parsePDFPageRange(raw string) (first int, last int, ok bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, 0, false
	}
	if strings.HasSuffix(raw, "-") {
		first, err := strconv.Atoi(strings.TrimSpace(strings.TrimSuffix(raw, "-")))
		if err != nil || first <= 0 {
			return 0, 0, false
		}
		return first, math.MaxInt, true
	}
	if !strings.Contains(raw, "-") {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			return 0, 0, false
		}
		return n, n, true
	}
	parts := strings.SplitN(raw, "-", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	first, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	last, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil || first <= 0 || last < first {
		return 0, 0, false
	}
	return first, last, true
}

// PDFPageSet is the parsed result of the TS Read PDF page selector grammar:
// a single page ("3"), a bounded range ("1-5"), or an open-ended range
// ("5-"). Open-ended ranges are accepted syntactically so validation can
// reject them as over the per-read page cap before any file I/O.
type PDFPageSet struct {
	// Pages is the contiguous 1-indexed page list for non-open-ended
	// selectors. Empty when OpenEnded is true.
	Pages []int
	// OpenEnded is true if the selector ends with "-".
	OpenEnded bool
	// OpenStart, when OpenEnded is true, is the starting page number.
	OpenStart int
	// First and Last bracket the explicit selection.
	First int
	Last  int
}

// parsePDFPageSelector mirrors TS parsePDFPageRange. It intentionally rejects
// comma-separated selectors in the default Read path.
func parsePDFPageSelector(raw string) (PDFPageSet, bool) {
	first, last, ok := parsePDFPageRange(raw)
	if !ok {
		return PDFPageSet{}, false
	}
	set := PDFPageSet{First: first, Last: last}
	if last == math.MaxInt {
		set.OpenEnded = true
		set.OpenStart = first
		return set, true
	}
	set.Pages = make([]int, 0, last-first+1)
	for page := first; page <= last; page++ {
		set.Pages = append(set.Pages, page)
	}
	return set, true
}

func trimTrailingZero(value float64) string {
	text := fmt.Sprintf("%.1f", value)
	return strings.TrimSuffix(strings.TrimSuffix(text, "0"), ".")
}

func hasBinaryExtension(filePath string) bool {
	ext := strings.ToLower(filepath.Ext(filePath))
	return binaryExtensions[ext]
}

func isBlockedDevicePath(filePath string) bool {
	if blockedDevicePaths[filePath] {
		return true
	}
	if strings.HasPrefix(filePath, "/proc/") {
		return strings.HasSuffix(filePath, "/fd/0") ||
			strings.HasSuffix(filePath, "/fd/1") ||
			strings.HasSuffix(filePath, "/fd/2")
	}
	return false
}

func imageMediaTypeForPath(path string) string {
	mediaType := mime.TypeByExtension(strings.ToLower(filepath.Ext(path)))
	if mediaType == "" {
		return "image/png"
	}
	if idx := strings.IndexByte(mediaType, ';'); idx >= 0 {
		mediaType = strings.TrimSpace(mediaType[:idx])
	}
	return mediaType
}

func (t *FileReadTool) executeRichRead(ctx context.Context, filePath string, in FileReadInput, limits FileReadingLimits) (types.ToolResult, bool, error) {
	if err := ctx.Err(); err != nil {
		return types.ToolResult{}, true, err
	}
	ext := strings.ToLower(filepath.Ext(filePath))
	switch {
	case ext == ".ipynb":
		stat, err := os.Stat(filePath)
		if err != nil {
			return types.ToolResult{}, false, err
		}
		if stat.Size() > limits.MaxSizeBytes {
			return fileReadSizeLimitError(filePath, stat.Size(), limits.MaxSizeBytes), true, nil
		}
		data, err := os.ReadFile(filePath)
		if err != nil {
			return toolRuntimeErrorf(i18n.KeyToolReadFileFailed, err), true, nil
		}
		cells, err := readNotebookCells(data, filePath)
		if err != nil {
			return ErrorResponse(err), true, nil
		}
		cellsJSON, _ := json.Marshal(cells)
		if tokens, _ := t.tieredReadTokenCount(ctx, string(cellsJSON), limits.MaxTokens); tokens > limits.MaxTokens {
			return fileReadTokenLimitError(filePath, tokens, limits.MaxTokens), true, nil
		}
		output := FileReadOutput{
			Type: FileReadVariantNotebook,
			File: FileReadOutputFile{FilePath: filePath, Cells: cells},
		}
		return types.ToolResult{
			Content:       toolRuntimeFormat(i18n.KeyToolReadNotebookSummary, filePath, len(cells)),
			ContentBlocks: notebookCellsToContentBlocks(cells),
			Data:          output,
			Metadata:      readVariantMetadata(output.Type),
		}, true, nil

	case imageExtensions[ext]:
		data, err := os.ReadFile(filePath)
		if err != nil {
			return toolRuntimeErrorf(i18n.KeyToolReadFileFailed, err), true, nil
		}
		imgBudget := limits.MaxTokens
		if liveBudget := t.tokenBudget(); liveBudget > 0 && liveBudget < imgBudget {
			imgBudget = liveBudget
		}
		prepared, err := prepareImageForRead(filePath, data, imgBudget)
		if err != nil {
			return ErrorResponse(err), true, nil
		}
		encoded := base64.StdEncoding.EncodeToString(prepared.Data)
		output := FileReadOutput{
			Type: FileReadVariantImage,
			File: FileReadOutputFile{
				Base64: encoded, MediaType: prepared.MediaType,
				OriginalSize: int64(len(data)), Dimensions: fileReadImageDimensions(prepared.Dimensions),
			},
		}
		result := types.ToolResult{
			Content:       toolRuntimeFormat(i18n.KeyToolReadImageSummary, filePath, formatReadSize(int64(len(data)))),
			ContentBlocks: []types.ContentBlock{newImageBlockFromBase64(encoded, prepared.MediaType)},
			Data:          output,
			Metadata:      readVariantMetadata(output.Type),
		}
		if metadataText := createImageMetadataText(prepared.Dimensions); metadataText != "" {
			result.NewMessages = []types.Message{{
				Role: types.RoleUser, Content: []types.ContentBlock{newTextBlock(metadataText)},
			}}
		}
		return result, true, nil

	case ext == ".pdf":
		if in.Pages != "" {
			set, ok := parsePDFPageSelector(in.Pages)
			if !ok {
				return toolRuntimeErrorf(i18n.KeyToolReadPagesInvalid, in.Pages), true, nil
			}
			if set.OpenEnded || len(set.Pages) > pdfMaxPagesPerRead {
				return toolRuntimeErrorf(i18n.KeyToolReadPageRangeTooWide, in.Pages, pdfMaxPagesPerRead), true, nil
			}
			extraction, err := extractPDFPageImages(filePath, set.First, set.Last, t.pdfToolResultsDir())
			if err != nil {
				return pdfErrorToolResult(err), true, nil
			}
			budget := limits.MaxTokens
			if liveBudget := t.tokenBudget(); liveBudget > 0 && liveBudget < budget {
				budget = liveBudget
			}
			totalTokens := 0
			content := make([]types.ContentBlock, 0, len(extraction.Images))
			for _, image := range extraction.Images {
				if image.Source != nil {
					totalTokens += int(math.Ceil(float64(len(image.Source.Data)) * 0.125))
				}
				content = append(content, image)
			}
			if budget > 0 && totalTokens > budget {
				_ = os.RemoveAll(extraction.OutputDir)
				return toolRuntimeErrorf(i18n.KeyToolReadPDFBudget, totalTokens, budget), true, nil
			}
			t.emitAnalytics("tengu_pdf_page_extraction", map[string]any{
				"success": true, "pageCount": extraction.Count,
				"fileSize": extraction.OriginalSize, "hasPageRange": true,
			})
			output := FileReadOutput{
				Type: FileReadVariantParts,
				File: FileReadOutputFile{
					FilePath: filePath, OriginalSize: extraction.OriginalSize,
					Count: extraction.Count, OutputDir: extraction.OutputDir,
				},
			}
			return types.ToolResult{
				Content: fmtPDFPartsSummary(filePath, extraction.OriginalSize, extraction.Count),
				Data:    output, Metadata: readVariantMetadata(output.Type),
				NewMessages: []types.Message{{Role: types.RoleUser, Content: content}},
			}, true, nil
		}

		if pageCount, ok := getPDFPageCount(filePath); ok && pageCount > pdfAtMentionInlineThreshold {
			return toolRuntimeErrorf(
				i18n.KeyToolReadPDFTooManyPages,
				pageCount, pdfMaxPagesPerRead,
			), true, nil
		}
		stat, err := os.Stat(filePath)
		if err != nil {
			return ErrorResponse(err), true, nil
		}
		if !t.isPDFSupported() || stat.Size() > pdfExtractSizeThreshold {
			extraction, extractErr := extractPDFPageImages(filePath, 0, 0, t.pdfToolResultsDir())
			if extractErr == nil {
				t.emitAnalytics("tengu_pdf_page_extraction", map[string]any{
					"success": true, "pageCount": extraction.Count, "fileSize": extraction.OriginalSize,
				})
			} else {
				t.emitAnalytics("tengu_pdf_page_extraction", map[string]any{
					"success": false, "available": pdfPageExtractionAvailable(), "fileSize": stat.Size(),
				})
			}
		}
		if !t.isPDFSupported() {
			return toolRuntimeErrorf(i18n.KeyToolReadPDFFullUnsupported, pdfMaxPagesPerRead), true, nil
		}
		data, originalSize, err := readPDFFile(filePath)
		if err != nil {
			return pdfErrorToolResult(err), true, nil
		}
		encoded := base64.StdEncoding.EncodeToString(data)
		output := FileReadOutput{
			Type: FileReadVariantPDF,
			File: FileReadOutputFile{FilePath: filePath, Base64: encoded, OriginalSize: originalSize},
		}
		return types.ToolResult{
			Content: fmtPDFReadSummary(filePath, originalSize), Data: output,
			Metadata: readVariantMetadata(output.Type),
			NewMessages: []types.Message{{
				Role: types.RoleUser,
				Content: []types.ContentBlock{types.DocumentBlock{
					Type:   types.ContentTypeDocument,
					Source: &types.DocumentSource{Type: "base64", MediaType: "application/pdf", Data: encoded},
				}},
			}},
		}, true, nil
	}
	return types.ToolResult{}, false, nil
}
