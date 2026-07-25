package agent

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/agent-dance/luban/internal/gitutil"
)

const agentMemoryMaxBytes = 25_000
const agentMemoryMaxEntrypointLines = 200
const agentMemoryMaxSanitizedPathLength = 200
const agentMemorySnapshotBase = "agent-memory-snapshots"
const agentMemorySnapshotJSON = "snapshot.json"
const agentMemorySyncedJSON = ".snapshot-synced.json"

func loadAgentMemoryPrompt(agentType, scope, cwd string) string {
	scope = strings.ToLower(strings.TrimSpace(scope))
	if scope == "" {
		return ""
	}
	if !isAgentAutoMemoryEnabled(cwd) {
		return ""
	}
	memoryDir, err := agentMemoryDir(agentType, scope, cwd)
	if err != nil || memoryDir == "" {
		return ""
	}
	_ = os.MkdirAll(memoryDir, 0o755)
	snapshotUpdate := initializeAgentMemoryFromSnapshotIfNeeded(agentType, cwd, memoryDir)

	entrypoint := filepath.Join(memoryDir, "MEMORY.md")
	content := readAgentMemoryEntrypoint(entrypoint)

	scopeNote := ""
	switch scope {
	case "user":
		scopeNote = "Since this is user-scope memory, keep learnings general because they apply across projects."
	case "project":
		scopeNote = "Since this is project-scope memory, tailor learnings to this repository and keep team visibility in mind."
	case "local":
		scopeNote = "Since this is local-scope memory, tailor learnings to this project and machine."
	default:
		return ""
	}

	parts := []string{
		"# Persistent Agent Memory",
		fmt.Sprintf("You have persistent file-based memory at `%s`.", memoryDir),
		scopeNote,
		"Use `MEMORY.md` as an index. Store durable facts in focused Markdown files in the same directory, then add a concise pointer in `MEMORY.md`.",
		"Do not store short-lived task state in memory; use tasks or plans for current-session work.",
	}
	if strings.TrimSpace(content) != "" {
		parts = append(parts, fmt.Sprintf("Current `MEMORY.md` contents:\n\n%s", content))
	}
	if snapshotUpdate != "" {
		parts = append(parts, fmt.Sprintf("A newer project memory snapshot is available for this agent (updated at %s). Keep existing local memory unless the user explicitly asks to replace or sync it.", snapshotUpdate))
	}
	if extra := strings.TrimSpace(os.Getenv("LUBAN_COWORK_MEMORY_EXTRA_GUIDELINES")); extra != "" {
		parts = append(parts, extra)
	}
	return strings.Join(parts, "\n\n")
}

func agentMemoryDir(agentType, scope, cwd string) (string, error) {
	dirName := sanitizeAgentMemoryName(agentType)
	switch scope {
	case "user":
		base := agentMemoryBaseDir()
		if base == "" {
			return "", fmt.Errorf("could not resolve agent memory base directory")
		}
		return filepath.Join(base, "agent-memory", dirName), nil
	case "project":
		root := agentMemoryCWD(cwd)
		return filepath.Join(root, ".luban-code", "agent-memory", dirName), nil
	case "local":
		if remoteBase := strings.TrimSpace(os.Getenv("LUBAN_CODE_REMOTE_MEMORY_DIR")); remoteBase != "" {
			projectRoot := agentMemoryCanonicalProjectRoot(cwd)
			return filepath.Join(filepath.Clean(remoteBase), "projects", sanitizeMemoryProjectPath(projectRoot), "agent-memory-local", dirName), nil
		}
		root := agentMemoryCWD(cwd)
		return filepath.Join(root, ".luban-code", "agent-memory-local", dirName), nil
	default:
		return "", fmt.Errorf("unsupported memory scope %q", scope)
	}
}

func agentMemoryBaseDir() string {
	if remoteBase := strings.TrimSpace(os.Getenv("LUBAN_CODE_REMOTE_MEMORY_DIR")); remoteBase != "" {
		return filepath.Clean(remoteBase)
	}
	return agentConfigHomeDir()
}

func isAgentAutoMemoryEnabled(cwd string) bool {
	if isTruthyAgentEnv(os.Getenv("LUBAN_CODE_DISABLE_AUTO_MEMORY")) {
		return false
	}
	if isDefinedFalsyAgentEnv(os.Getenv("LUBAN_CODE_DISABLE_AUTO_MEMORY")) {
		return true
	}
	if isTruthyAgentEnv(os.Getenv("LUBAN_CODE_SIMPLE")) {
		return false
	}
	if isTruthyAgentEnv(os.Getenv("LUBAN_CODE_REMOTE")) && strings.TrimSpace(os.Getenv("LUBAN_CODE_REMOTE_MEMORY_DIR")) == "" {
		return false
	}
	if setting, ok := readAutoMemorySetting(cwd); ok {
		return setting
	}
	return true
}

func isDefinedFalsyAgentEnv(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "0", "false", "no", "off":
		return true
	default:
		return false
	}
}

func readAutoMemorySetting(cwd string) (bool, bool) {
	var got bool
	var set bool
	for _, path := range pluginSettingsPaths(cwd) {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var raw struct {
			AutoMemoryEnabled *bool `json:"autoMemoryEnabled"`
		}
		if err := json.Unmarshal(data, &raw); err != nil || raw.AutoMemoryEnabled == nil {
			continue
		}
		got = *raw.AutoMemoryEnabled
		set = true
	}
	return got, set
}

func agentMemoryCWD(cwd string) string {
	if strings.TrimSpace(cwd) != "" {
		return filepath.Clean(cwd)
	}
	if current, err := os.Getwd(); err == nil {
		return current
	}
	return "."
}

func agentMemoryCanonicalProjectRoot(cwd string) string {
	start := agentMemoryCWD(cwd)
	if commonDir, err := gitutil.Run(start, "rev-parse", "--path-format=absolute", "--git-common-dir"); err == nil {
		commonDir = strings.TrimSpace(commonDir)
		if commonDir != "" {
			if filepath.Base(commonDir) != ".git" {
				return filepath.Clean(commonDir)
			}
			return filepath.Dir(commonDir)
		}
	}
	if topLevel, err := gitutil.Run(start, "rev-parse", "--show-toplevel"); err == nil && strings.TrimSpace(topLevel) != "" {
		return filepath.Clean(strings.TrimSpace(topLevel))
	}
	return start
}

func sanitizeMemoryProjectPath(name string) string {
	var b strings.Builder
	for _, r := range name {
		if r <= unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	sanitized := b.String()
	if len(sanitized) <= agentMemoryMaxSanitizedPathLength {
		return sanitized
	}
	return sanitized[:agentMemoryMaxSanitizedPathLength] + "-" + strconv.FormatInt(int64(math.Abs(float64(djb2Hash32(name)))), 36)
}

func djb2Hash32(value string) int32 {
	var hash int32
	for _, r := range value {
		hash = (hash << 5) - hash + int32(r)
	}
	return hash
}

func sanitizeAgentMemoryName(agentType string) string {
	trimmed := strings.TrimSpace(agentType)
	if trimmed == "" {
		return "general-purpose"
	}
	var b strings.Builder
	for _, r := range trimmed {
		switch {
		case r == ':' || r == '-' || r == '_' || r == '.':
			b.WriteRune('-')
		case r <= unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	name := strings.Trim(b.String(), "-")
	if name == "" {
		return "general-purpose"
	}
	return name
}

func readAgentMemoryEntrypoint(path string) string {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return ""
	}
	truncated, warning := truncateAgentMemoryEntrypoint(trimmed)
	if warning != "" {
		return truncated + "\n\n" + warning
	}
	return truncated
}

func truncateAgentMemoryEntrypoint(content string) (string, string) {
	lines := strings.Split(content, "\n")
	originalLineCount := len(lines)
	originalByteCount := len(content)
	lineTruncated := originalLineCount > agentMemoryMaxEntrypointLines
	byteTruncated := originalByteCount > agentMemoryMaxBytes
	if lineTruncated {
		lines = lines[:agentMemoryMaxEntrypointLines]
		content = strings.Join(lines, "\n")
	}
	if len(content) > agentMemoryMaxBytes {
		cut := strings.LastIndex(content[:agentMemoryMaxBytes], "\n")
		if cut <= 0 {
			cut = agentMemoryMaxBytes
		}
		content = content[:cut]
	}
	if !lineTruncated && !byteTruncated {
		return content, ""
	}
	reason := "size limit"
	if lineTruncated && !byteTruncated {
		reason = fmt.Sprintf("%d lines (limit: %d)", originalLineCount, agentMemoryMaxEntrypointLines)
	} else if byteTruncated && !lineTruncated {
		reason = fmt.Sprintf("%d bytes (limit: %d)", originalByteCount, agentMemoryMaxBytes)
	}
	return content, fmt.Sprintf("> WARNING: MEMORY.md exceeded %s. Only part of it was loaded. Keep index entries concise and move detail into topic files.", reason)
}

type agentMemorySnapshotMeta struct {
	UpdatedAt string `json:"updatedAt"`
}

type agentMemorySyncedMeta struct {
	SyncedFrom string `json:"syncedFrom"`
}

func initializeAgentMemoryFromSnapshotIfNeeded(agentType, cwd, memoryDir string) string {
	snapshotDir := agentMemorySnapshotDir(agentType, cwd)
	meta := readAgentMemorySnapshotMeta(filepath.Join(snapshotDir, agentMemorySnapshotJSON))
	if strings.TrimSpace(meta.UpdatedAt) == "" {
		return ""
	}
	if !agentMemoryDirHasMarkdown(memoryDir) {
		copyAgentMemorySnapshot(snapshotDir, memoryDir)
		writeAgentMemorySyncedMeta(memoryDir, meta.UpdatedAt)
		return ""
	}
	synced := readAgentMemorySyncedMeta(filepath.Join(memoryDir, agentMemorySyncedJSON))
	if strings.TrimSpace(synced.SyncedFrom) == "" || snapshotTimestampAfter(meta.UpdatedAt, synced.SyncedFrom) {
		return meta.UpdatedAt
	}
	return ""
}

func agentMemorySnapshotDir(agentType, cwd string) string {
	root := agentMemoryCWD(cwd)
	return filepath.Join(root, ".luban-code", agentMemorySnapshotBase, sanitizeAgentMemoryName(agentType))
}

func readAgentMemorySnapshotMeta(path string) agentMemorySnapshotMeta {
	var meta agentMemorySnapshotMeta
	readAgentMemoryJSON(path, &meta)
	return meta
}

func readAgentMemorySyncedMeta(path string) agentMemorySyncedMeta {
	var meta agentMemorySyncedMeta
	readAgentMemoryJSON(path, &meta)
	return meta
}

func readAgentMemoryJSON(path string, dest any) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, dest)
}

func agentMemoryDirHasMarkdown(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
			return true
		}
	}
	return false
}

func copyAgentMemorySnapshot(snapshotDir, memoryDir string) {
	entries, err := os.ReadDir(snapshotDir)
	if err != nil {
		return
	}
	_ = os.MkdirAll(memoryDir, 0o755)
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == agentMemorySnapshotJSON {
			continue
		}
		src := filepath.Join(snapshotDir, entry.Name())
		dst := filepath.Join(memoryDir, entry.Name())
		data, err := os.ReadFile(src)
		if err != nil {
			continue
		}
		_ = os.WriteFile(dst, data, 0o644)
	}
}

func writeAgentMemorySyncedMeta(memoryDir, updatedAt string) {
	data, err := json.Marshal(agentMemorySyncedMeta{SyncedFrom: updatedAt})
	if err != nil {
		return
	}
	_ = os.MkdirAll(memoryDir, 0o755)
	_ = os.WriteFile(filepath.Join(memoryDir, agentMemorySyncedJSON), data, 0o644)
}

func snapshotTimestampAfter(candidate, baseline string) bool {
	candidateTime, candidateErr := time.Parse(time.RFC3339Nano, candidate)
	baselineTime, baselineErr := time.Parse(time.RFC3339Nano, baseline)
	if candidateErr == nil && baselineErr == nil {
		return candidateTime.After(baselineTime)
	}
	return strings.Compare(candidate, baseline) > 0
}
