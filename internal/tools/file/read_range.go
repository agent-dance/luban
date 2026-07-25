package file

import (
	"bytes"
	"fmt"
)

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
