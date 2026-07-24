package tools

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/agent-dance/luban/i18n"
)

const fastReadPathMaxSizeBytes int64 = 10 * 1024 * 1024

// ReadFileRangeResult contains the selected range plus metadata about the full file.
type ReadFileRangeResult struct {
	Content          string
	LineCount        int
	TotalLines       int
	TotalBytes       int64
	ReadBytes        int64
	ModTimeMs        int64
	ModTimeNs        int64
	TruncatedByBytes bool
}

// FileTooLargeError mirrors the original read-range failure when an unrestricted
// read exceeds the configured byte ceiling.
type FileTooLargeError struct {
	SizeInBytes int64
	MaxSize     int64
}

func (e *FileTooLargeError) Error() string {
	return fmt.Sprintf(
		"File content (%s) exceeds maximum allowed size (%s). Use offset and limit parameters to read specific portions of the file, or search for specific content instead of reading the whole file.",
		formatReadSize(e.SizeInBytes),
		formatReadSize(e.MaxSize),
	)
}

type ReadFileRangeOptions struct {
	TruncateOnByteLimit bool
}

func readFileInRange(
	ctx context.Context,
	filePath string,
	offset int,
	maxLines *int,
	maxBytes *int64,
	options ReadFileRangeOptions,
) (ReadFileRangeResult, error) {
	if err := ctx.Err(); err != nil {
		return ReadFileRangeResult{}, err
	}

	info, err := os.Stat(filePath)
	if err != nil {
		return ReadFileRangeResult{}, err
	}
	if info.IsDir() {
		return ReadFileRangeResult{}, i18n.NewError(i18n.KeyToolSourceSinkReadDirectory, filePath)
	}

	if info.Mode().IsRegular() && info.Size() < fastReadPathMaxSizeBytes {
		if !options.TruncateOnByteLimit && maxBytes != nil && info.Size() > *maxBytes {
			return ReadFileRangeResult{}, &FileTooLargeError{
				SizeInBytes: info.Size(),
				MaxSize:     *maxBytes,
			}
		}
		data, err := os.ReadFile(filePath)
		if err != nil {
			return ReadFileRangeResult{}, err
		}
		if err := ctx.Err(); err != nil {
			return ReadFileRangeResult{}, err
		}
		return readFileInRangeFast(
			data,
			info.ModTime().UnixMilli(),
			info.ModTime().UnixNano(),
			offset,
			maxLines,
			maxBytes,
			options,
		), nil
	}

	return readFileInRangeStreaming(ctx, filePath, offset, maxLines, maxBytes, options)
}

func readFileInRangeFast(
	raw []byte,
	mtimeMs int64,
	mtimeNs int64,
	offset int,
	maxLines *int,
	maxBytes *int64,
	options ReadFileRangeOptions,
) ReadFileRangeResult {
	if len(raw) == 0 {
		return ReadFileRangeResult{
			Content:    "",
			LineCount:  0,
			TotalLines: 0,
			TotalBytes: 0,
			ReadBytes:  0,
			ModTimeMs:  mtimeMs,
			ModTimeNs:  mtimeNs,
		}
	}

	text := raw
	if bytes.HasPrefix(text, []byte{0xEF, 0xBB, 0xBF}) {
		text = text[3:]
	}

	endLine := int(^uint(0) >> 1)
	if maxLines != nil {
		endLine = offset + *maxLines
	}

	selectedLines := make([]string, 0)
	var selectedBytes int64
	truncatedByBytes := false
	lineIndex := 0
	start := 0

	tryPush := func(line []byte) {
		if truncatedByBytes {
			return
		}
		if options.TruncateOnByteLimit && maxBytes != nil {
			sep := int64(0)
			if len(selectedLines) > 0 {
				sep = 1
			}
			nextBytes := selectedBytes + sep + int64(len(line))
			if nextBytes > *maxBytes {
				truncatedByBytes = true
				return
			}
			selectedBytes = nextBytes
		}
		selectedLines = append(selectedLines, string(line))
	}

	for start <= len(text) {
		newline := bytes.IndexByte(text[start:], '\n')
		var line []byte
		if newline < 0 {
			line = text[start:]
		} else {
			line = text[start : start+newline]
		}
		if len(line) > 0 && line[len(line)-1] == '\r' {
			line = line[:len(line)-1]
		}
		if lineIndex >= offset && lineIndex < endLine {
			tryPush(line)
		}
		lineIndex++
		if newline < 0 {
			break
		}
		start += newline + 1
		if start == len(text) {
			if lineIndex >= offset && lineIndex < endLine {
				tryPush([]byte{})
			}
			lineIndex++
			break
		}
	}

	content := stringsJoinWithNewlines(selectedLines)
	return ReadFileRangeResult{
		Content:          content,
		LineCount:        len(selectedLines),
		TotalLines:       lineIndex,
		TotalBytes:       int64(len(text)),
		ReadBytes:        int64(len(content)),
		ModTimeMs:        mtimeMs,
		ModTimeNs:        mtimeNs,
		TruncatedByBytes: truncatedByBytes,
	}
}

func readFileInRangeStreaming(
	ctx context.Context,
	filePath string,
	offset int,
	maxLines *int,
	maxBytes *int64,
	options ReadFileRangeOptions,
) (ReadFileRangeResult, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return ReadFileRangeResult{}, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return ReadFileRangeResult{}, err
	}

	reader := bufio.NewReaderSize(f, 64*1024)
	endLine := int(^uint(0) >> 1)
	if maxLines != nil {
		endLine = offset + *maxLines
	}

	selectedLines := make([]string, 0)
	var selectedBytes int64
	var totalBytes int64
	truncatedByBytes := false
	lineIndex := 0
	firstChunk := true
	var partial []byte

	tryPush := func(line []byte) {
		if truncatedByBytes {
			return
		}
		if options.TruncateOnByteLimit && maxBytes != nil {
			sep := int64(0)
			if len(selectedLines) > 0 {
				sep = 1
			}
			nextBytes := selectedBytes + sep + int64(len(line))
			if nextBytes > *maxBytes {
				truncatedByBytes = true
				return
			}
			selectedBytes = nextBytes
		}
		selectedLines = append(selectedLines, string(line))
	}

	processLine := func(line []byte) {
		if len(line) > 0 && line[len(line)-1] == '\r' {
			line = line[:len(line)-1]
		}
		if lineIndex >= offset && lineIndex < endLine {
			tryPush(line)
		}
		lineIndex++
	}

	for {
		if err := ctx.Err(); err != nil {
			return ReadFileRangeResult{}, err
		}

		chunk, readErr := reader.ReadSlice('\n')
		if len(chunk) > 0 {
			totalBytes += int64(len(chunk))
			if firstChunk {
				firstChunk = false
				chunk = bytes.TrimPrefix(chunk, []byte{0xEF, 0xBB, 0xBF})
			}
			if !options.TruncateOnByteLimit && maxBytes != nil && totalBytes > *maxBytes {
				return ReadFileRangeResult{}, &FileTooLargeError{
					SizeInBytes: totalBytes,
					MaxSize:     *maxBytes,
				}
			}
			partial = append(partial, chunk...)
		}

		switch {
		case errors.Is(readErr, bufio.ErrBufferFull):
			continue
		case errors.Is(readErr, io.EOF):
			if totalBytes == 0 {
				return ReadFileRangeResult{
					Content:    "",
					LineCount:  0,
					TotalLines: 0,
					TotalBytes: 0,
					ReadBytes:  0,
					ModTimeMs:  info.ModTime().UnixMilli(),
					ModTimeNs:  info.ModTime().UnixNano(),
				}, nil
			}
			if len(partial) > 0 && partial[len(partial)-1] == '\n' {
				partial = partial[:len(partial)-1]
				processLine(partial)
				if lineIndex >= offset && lineIndex < endLine {
					tryPush([]byte{})
				}
				lineIndex++
			} else {
				processLine(partial)
			}
			content := stringsJoinWithNewlines(selectedLines)
			return ReadFileRangeResult{
				Content:          content,
				LineCount:        len(selectedLines),
				TotalLines:       lineIndex,
				TotalBytes:       totalBytes,
				ReadBytes:        int64(len(content)),
				ModTimeMs:        info.ModTime().UnixMilli(),
				ModTimeNs:        info.ModTime().UnixNano(),
				TruncatedByBytes: truncatedByBytes,
			}, nil
		case readErr != nil:
			return ReadFileRangeResult{}, readErr
		default:
			if len(partial) > 0 && partial[len(partial)-1] == '\n' {
				partial = partial[:len(partial)-1]
			}
			processLine(partial)
			partial = partial[:0]
		}
	}
}

func stringsJoinWithNewlines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	var buf bytes.Buffer
	for i, line := range lines {
		if i > 0 {
			buf.WriteByte('\n')
		}
		buf.WriteString(line)
	}
	return buf.String()
}
