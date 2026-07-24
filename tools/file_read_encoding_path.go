// Package tools — file_read_encoding_path.go wires the encoding-detection
// helpers into FileReadTool.Execute. When the file is detected as a
// non-UTF-8 text encoding (UTF-16 LE/BE or Latin-1), this path performs a
// full read + decode + render, returning a numbered-line result that
// matches the standard text path. The detected encoding metadata is
// stamped into ReadFileState so FileWrite can preserve it on overwrite.
package tools

import (
	"context"
	"os"
	"strings"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

// peekFileHead returns up to maxBytes from the start of filePath without
// reading the entire file. Best-effort: returns nil on error.
func peekFileHead(filePath string, maxBytes int) []byte {
	f, err := os.Open(filePath)
	if err != nil {
		return nil
	}
	defer f.Close()
	buf := make([]byte, maxBytes)
	n, err := f.Read(buf)
	if err != nil && n == 0 {
		return nil
	}
	return buf[:n]
}

// readNonUTF8Text handles the rendering of a UTF-16 / Latin-1 file by
// reading the bytes, decoding to UTF-8, and applying the same offset/limit
// logic as the standard text path. Returns (result, handled, err).
func (t *FileReadTool) readNonUTF8Text(
	ctx context.Context,
	filePath, normalizedPath string,
	in FileReadInput,
	input map[string]any,
	det EncodingDetectResult,
	mtimeMs, mtimeNs, totalBytes int64,
	ext string,
	limits FileReadingLimits,
) (types.ToolResult, bool, error) {
	if err := ctx.Err(); err != nil {
		return types.ToolResult{}, true, err
	}
	state := t.readState()

	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrorResponse(fileReadNotFoundRuntimeError(filePath)), true, nil
		}
		return ErrorResponsef("%s", toolRuntimeFormat(i18n.KeyToolLegacyAFileReadFailed, err)), true, nil
	}

	// Reject too-large non-UTF-8 reads to match the UTF-8 size guard.
	_, limitSpecified := input["limit"]
	_, offsetSpecified := input["offset"]
	if int64(len(data)) > limits.MaxSizeBytes && !limitSpecified {
		return structuredReadSizeError(filePath, int64(len(data)), limits.MaxSizeBytes), true, nil
	}

	decoded := decodeFileBytes(data, det)
	requestedOffset := 1
	if offsetSpecified {
		requestedOffset = int(in.Offset)
	}
	requestedLimit := 0
	if limitSpecified {
		requestedLimit = int(in.Limit)
	}
	limit := (*int)(nil)
	if limitSpecified {
		limit = &requestedLimit
	}

	if decoded == "" {
		state.RecordReadForContext(ctx, normalizedPath, ReadFileEntry{
			TimestampMs: mtimeMs, Offset: requestedOffset, Limit: requestedLimit,
			MtimeNs: mtimeNs, TotalBytes: totalBytes,
			OffsetSpecified: offsetSpecified, LimitSpecified: limitSpecified,
			CoverageKnown: true, CoverageComplete: true, FullSnapshot: true,
			IsPartialView: false, LastTool: "Read", DedupEligible: true,
			Encoding: det.Encoding, BOM: det.BOM,
		})
		t.afterSuccessfulTextRead(normalizedPath, "", mtimeMs, false, fileReadSuccessMetrics{
			TotalLines: 0, ReadLines: 0, TotalBytes: int64(len(data)), ReadBytes: 0,
			Offset: requestedOffset, Limit: limit, Ext: ext, MessageID: fileReadMessageID(ctx),
		})
		result := t.newTextReadResult(FileReadOutputFile{
			FilePath: filePath, Content: "", NumLines: 0, StartLine: requestedOffset, TotalLines: 0,
		})
		result.Metadata["encoding"] = string(det.Encoding)
		return result, true, nil
	}

	allLines := strings.Split(decoded, "\n")
	totalLines := len(allLines)
	startLine := requestedOffset
	zeroBased := startLine
	if startLine > 0 {
		zeroBased = startLine - 1
	}
	if zeroBased >= totalLines {
		state.RecordReadForContext(ctx, normalizedPath, ReadFileEntry{
			TimestampMs: mtimeMs, Offset: requestedOffset, Limit: requestedLimit,
			MtimeNs: mtimeNs, TotalBytes: totalBytes, TotalLines: totalLines,
			OffsetSpecified: offsetSpecified, LimitSpecified: limitSpecified,
			CoverageKnown: true, IsPartialView: false, LastTool: "Read", DedupEligible: true,
			Encoding: det.Encoding, BOM: det.BOM,
		})
		t.afterSuccessfulTextRead(normalizedPath, "", mtimeMs, true, fileReadSuccessMetrics{
			TotalLines: totalLines, ReadLines: 0, TotalBytes: int64(len(data)), ReadBytes: 0,
			Offset: requestedOffset, Limit: limit, Ext: ext, MessageID: fileReadMessageID(ctx),
		})
		result := t.newTextReadResult(FileReadOutputFile{
			FilePath: filePath, Content: "", NumLines: 0, StartLine: startLine, TotalLines: totalLines,
		})
		result.Metadata["encoding"] = string(det.Encoding)
		return result, true, nil
	}

	endLine := totalLines
	if requestedLimit > 0 && zeroBased+requestedLimit < endLine {
		endLine = zeroBased + requestedLimit
	}
	selected := strings.Join(allLines[zeroBased:endLine], "\n")
	if tokens, _ := t.tieredReadTokenCount(ctx, selected, limits.MaxTokens); tokens > limits.MaxTokens {
		return fileReadTokenLimitError(filePath, tokens, limits.MaxTokens), true, nil
	}
	coverage, coverageComplete := readObservationCoverage(startLine, endLine-zeroBased, totalLines)
	state.RecordReadForContext(ctx, normalizedPath, ReadFileEntry{
		TimestampMs: mtimeMs, Offset: requestedOffset, Limit: requestedLimit,
		MtimeNs: mtimeNs, TotalBytes: totalBytes,
		OffsetSpecified: offsetSpecified, LimitSpecified: limitSpecified,
		CoverageKnown: true, Coverage: coverage, TotalLines: totalLines,
		CoverageComplete: coverageComplete, FullSnapshot: coverageComplete,
		IsPartialView: false, Content: selected, LastTool: "Read",
		DedupEligible: true, Encoding: det.Encoding, BOM: det.BOM,
	})
	t.afterSuccessfulTextRead(normalizedPath, selected, mtimeMs, !coverageComplete, fileReadSuccessMetrics{
		TotalLines: totalLines, ReadLines: endLine - zeroBased,
		TotalBytes: int64(len(data)), ReadBytes: int64(len(selected)),
		Offset: requestedOffset, Limit: limit, Ext: ext, MessageID: fileReadMessageID(ctx),
	})
	result := t.newTextReadResult(FileReadOutputFile{
		FilePath: filePath, Content: selected, NumLines: endLine - zeroBased,
		StartLine: startLine, TotalLines: totalLines,
	})
	result.Metadata["encoding"] = string(det.Encoding)
	return result, true, nil
}
