package prompt

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// PromptDump is a machine-readable prompt construction snapshot. Text fields
// are redacted before hashes are computed.
type PromptDump struct {
	Blocks  []PromptDumpBlock   `json:"blocks"`
	Context []PromptDumpContext `json:"context,omitempty"`
}

// PromptDumpBlock is a rendered system prompt block plus stable metadata.
type PromptDumpBlock struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Source     string `json:"source"`
	CacheScope string `json:"cache_scope,omitempty"`
	TextHash   string `json:"text_hash"`
	Text       string `json:"text"`
}

// PromptDumpContext is model-visible context outside the base system blocks.
type PromptDumpContext struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Source   string `json:"source"`
	TextHash string `json:"text_hash"`
	Text     string `json:"text"`
}

// BuildPromptDump renders a redacted prompt dump from already-constructed
// system blocks and context values.
func BuildPromptDump(blocks []SystemPromptBlock, userCtx UserContext, systemCtx SystemContext) PromptDump {
	dump := PromptDump{
		Blocks: make([]PromptDumpBlock, 0, len(blocks)),
	}
	for i, block := range blocks {
		text := RedactPromptDumpText(block.Text)
		name := firstNonEmpty(block.Name, fmt.Sprintf("block_%d", i))
		source := firstNonEmpty(block.Source, "unknown")
		dump.Blocks = append(dump.Blocks, PromptDumpBlock{
			ID:         fmt.Sprintf("system.%s.%d", stableIDPart(name), i),
			Name:       name,
			Source:     source,
			CacheScope: block.CacheScope,
			TextHash:   promptDumpSHA256Hex(text),
			Text:       text,
		})
	}
	for _, entry := range userCtx.Entries() {
		text := RedactPromptDumpText(entry.Value)
		dump.Context = append(dump.Context, PromptDumpContext{
			ID:       "user_context." + stableIDPart(entry.Key),
			Name:     entry.Key,
			Source:   "user_context",
			TextHash: promptDumpSHA256Hex(text),
			Text:     text,
		})
	}
	for _, entry := range systemCtx.Entries() {
		text := RedactPromptDumpText(entry.Value)
		dump.Context = append(dump.Context, PromptDumpContext{
			ID:       "system_context." + stableIDPart(entry.Key),
			Name:     entry.Key,
			Source:   "system_context",
			TextHash: promptDumpSHA256Hex(text),
			Text:     text,
		})
	}
	return dump
}

// WritePromptDumpJSON writes a pretty JSON prompt dump.
func WritePromptDumpJSON(w io.Writer, dump PromptDump) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(dump)
}

// WritePromptDumpText writes a human-readable prompt dump.
func WritePromptDumpText(w io.Writer, dump PromptDump) error {
	for _, block := range dump.Blocks {
		if _, err := fmt.Fprintf(w, "## %s (%s)\nsource: %s\ncache_scope: %s\ntext_hash: %s\n\n%s\n\n",
			block.ID, block.Name, block.Source, block.CacheScope, block.TextHash, block.Text); err != nil {
			return err
		}
	}
	if len(dump.Context) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(w, "## context"); err != nil {
		return err
	}
	for _, ctx := range dump.Context {
		if _, err := fmt.Fprintf(w, "\n### %s (%s)\nsource: %s\ntext_hash: %s\n\n%s\n",
			ctx.ID, ctx.Name, ctx.Source, ctx.TextHash, ctx.Text); err != nil {
			return err
		}
	}
	return nil
}

var promptDumpRedactors = []struct {
	re   *regexp.Regexp
	repl string
}{
	{regexp.MustCompile(`(?im)(^\s*authorization\s*:\s*)(?:bearer|basic)?\s*[^\s,;]+`), `${1}[REDACTED]`},
	{regexp.MustCompile(`(?im)(^\s*(?:x-api-key|api-key|proxy-authorization|cookie|set-cookie)\s*:\s*)[^\r\n]+`), `${1}[REDACTED]`},
	{regexp.MustCompile(`(?i)\b((?:api[_-]?key|access[_-]?token|auth[_-]?token|secret|password)\s*[:=]\s*)("[^"]+"|'[^']+'|[^\s,;]+)`), `${1}[REDACTED]`},
	{regexp.MustCompile(`(?i)\b(bearer\s+)[a-z0-9._~+/=-]{16,}`), `${1}[REDACTED]`},
	{regexp.MustCompile(`(?i)\b(sk-[a-z0-9]{16,})\b`), `[REDACTED]`},
}

// RedactPromptDumpText removes obvious secrets and auth header values from
// prompt dumps. It is intentionally conservative and deterministic.
func RedactPromptDumpText(text string) string {
	redacted := text
	for _, redactor := range promptDumpRedactors {
		redacted = redactor.re.ReplaceAllString(redacted, redactor.repl)
	}
	return redacted
}

func promptDumpSHA256Hex(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func stableIDPart(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastUnderscore := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastUnderscore = false
		default:
			if !lastUnderscore {
				b.WriteByte('_')
				lastUnderscore = true
			}
		}
	}
	return strings.Trim(b.String(), "_")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
