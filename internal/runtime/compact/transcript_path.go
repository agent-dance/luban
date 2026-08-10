package compact

import (
	"io"
	"os"
	"strings"
)

const compactTranscriptProbeBytes = 4096

func (s *SummaryCompactor) usableTranscriptPath() string {
	path := s.TranscriptPath
	if s.TranscriptPathResolver != nil {
		path = s.TranscriptPathResolver()
	}
	return usableCompactTranscriptPath(path)
}

// usableCompactTranscriptPath returns a recovery path only when it currently
// resolves to a readable, non-empty regular file. A compaction may hold an old
// content-addressed genesis path while the session publishes a newer audit
// artifact; exposing that empty placeholder gives the model a false recovery
// capability.
func usableCompactTranscriptPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 {
		return ""
	}
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	probe := make([]byte, compactTranscriptProbeBytes)
	n, readErr := file.Read(probe)
	if readErr != nil && readErr != io.EOF {
		return ""
	}
	if strings.TrimSpace(string(probe[:n])) == "" {
		return ""
	}
	return path
}
