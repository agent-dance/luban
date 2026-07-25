package mcp

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/agent-dance/luban/i18n"
)

const (
	mcpToolResultsDirEnv      = "MCP_TOOL_RESULTS_DIR"
	mcpLubanToolResultsDirEnv = "LUBAN_MCP_TOOL_RESULTS_DIR"
)

var unsafeMCPPersistIDChars = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

type mcpPersistedBinaryResult struct {
	Filepath string
	Size     int
	Ext      string
	Error    string
}

type mcpPersistedTextResult struct {
	Filepath     string
	OriginalSize int
	IsJSON       bool
	Error        string
}

func getMCPFormatDescription(resultType, schema string) string {
	switch resultType {
	case "structuredContent":
		if schema != "" {
			return i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyToolMCPFormatJSONSchema, schema)
		}
		return i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolMCPFormatJSON)
	case "contentArray", "structuredContent+contentArray":
		if schema != "" {
			return i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyToolMCPFormatJSONArraySchema, schema)
		}
		return i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolMCPFormatJSONArray)
	default:
		return i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolMCPFormatPlainText)
	}
}

func getMCPLargeOutputInstructions(rawOutputPath string, contentLength int, formatDescription string) string {
	return i18n.Format(
		i18n.DetectOrLoadLanguage(), i18n.KeyToolMCPLargeOutputStored,
		formatMCPInteger(contentLength),
		rawOutputPath,
		formatDescription,
		rawOutputPath,
	)
}

func extensionForMCPMimeType(mimeType string) string {
	switch normalizeMCPMime(mimeType) {
	case "application/pdf":
		return "pdf"
	case "application/json":
		return "json"
	case "text/csv":
		return "csv"
	case "text/plain":
		return "txt"
	case "text/html":
		return "html"
	case "text/markdown":
		return "md"
	case "application/zip":
		return "zip"
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		return "docx"
	case "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":
		return "xlsx"
	case "application/vnd.openxmlformats-officedocument.presentationml.presentation":
		return "pptx"
	case "application/msword":
		return "doc"
	case "application/vnd.ms-excel":
		return "xls"
	case "audio/mpeg":
		return "mp3"
	case "audio/wav":
		return "wav"
	case "audio/ogg":
		return "ogg"
	case "video/mp4":
		return "mp4"
	case "video/webm":
		return "webm"
	case "image/png":
		return "png"
	case "image/jpeg", "image/jpg":
		return "jpg"
	case "image/gif":
		return "gif"
	case "image/webp":
		return "webp"
	case "image/svg+xml":
		return "svg"
	default:
		return "bin"
	}
}

func persistMCPBinaryContent(bytes []byte, mimeType, persistID string) mcpPersistedBinaryResult {
	ext := extensionForMCPMimeType(mimeType)
	path, err := mcpToolResultPath(persistID, ext)
	if err != nil {
		return mcpPersistedBinaryResult{Error: err.Error()}
	}
	if err := writeMCPFileCreateOnce(path, bytes); err != nil {
		return mcpPersistedBinaryResult{Error: err.Error()}
	}
	return mcpPersistedBinaryResult{Filepath: path, Size: len(bytes), Ext: ext}
}

func persistMCPTextOutput(content, persistID string, isJSON bool) mcpPersistedTextResult {
	ext := "txt"
	if isJSON {
		ext = "json"
	}
	path, err := mcpToolResultPath(persistID, ext)
	if err != nil {
		return mcpPersistedTextResult{Error: err.Error()}
	}
	if err := writeMCPFileCreateOnce(path, []byte(content)); err != nil {
		return mcpPersistedTextResult{Error: err.Error()}
	}
	return mcpPersistedTextResult{Filepath: path, OriginalSize: len(content), IsJSON: isJSON}
}

func getMCPBinaryBlobSavedMessage(filepath, mimeType string, size int, sourceDescription string) string {
	return toolRuntimeFormat(i18n.KeyToolRuntimeMCPBinarySaved, sourceDescription, fallbackMCPMime(mimeType), formatMCPFileSize(size), filepath)
}

func newMCPPersistID(serverName, suffix string) string {
	parts := []string{"mcp"}
	if strings.TrimSpace(serverName) != "" {
		parts = append(parts, sanitizeMCPPersistID(serverName))
	}
	if strings.TrimSpace(suffix) != "" {
		parts = append(parts, sanitizeMCPPersistID(suffix))
	}
	parts = append(parts, fmt.Sprintf("%d", time.Now().UnixNano()), randomMCPSuffix())
	return strings.Join(parts, "-")
}

func mcpToolResultPath(persistID, ext string) (string, error) {
	dir := getMCPToolResultsDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	cleanDir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	filename := sanitizeMCPPersistID(persistID)
	if filename == "" {
		filename = "mcp-result"
	}
	ext = sanitizeMCPPersistID(ext)
	if ext == "" {
		ext = "bin"
	}
	path := filepath.Join(cleanDir, filename+"."+ext)
	cleanPath := filepath.Clean(path)
	if cleanPath != path || !strings.HasPrefix(cleanPath, cleanDir+string(os.PathSeparator)) {
		return "", i18n.NewError(i18n.KeyToolMCPUnsafeOutputPath)
	}
	return cleanPath, nil
}

func getMCPToolResultsDir() string {
	if dir := strings.TrimSpace(os.Getenv(mcpToolResultsDirEnv)); dir != "" {
		return dir
	}
	if dir := strings.TrimSpace(os.Getenv(mcpLubanToolResultsDirEnv)); dir != "" {
		return dir
	}
	if cwd, err := os.Getwd(); err == nil && strings.TrimSpace(cwd) != "" {
		return filepath.Join(cwd, ".luban-code", "tool-results")
	}
	return filepath.Join(os.TempDir(), "luban-code", "tool-results")
}

func writeMCPFileCreateOnce(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return err
	}
	return nil
}

func sanitizeMCPPersistID(value string) string {
	clean := unsafeMCPPersistIDChars.ReplaceAllString(strings.TrimSpace(value), "_")
	clean = strings.Trim(clean, "._-")
	if len(clean) > 96 {
		clean = strings.Trim(clean[:96], "._-")
	}
	return clean
}

func randomMCPSuffix() string {
	var buf [4]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "noentropy"
	}
	return hex.EncodeToString(buf[:])
}

func formatMCPFileSize(size int) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	}
	if size < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(size)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(size)/(1024*1024))
}

func formatMCPInteger(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var out []byte
	first := len(s) % 3
	if first == 0 {
		first = 3
	}
	out = append(out, s[:first]...)
	for i := first; i < len(s); i += 3 {
		out = append(out, ',')
		out = append(out, s[i:i+3]...)
	}
	return string(out)
}
