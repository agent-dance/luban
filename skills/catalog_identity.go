package skills

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"strings"
)

const (
	skillIdentityDomain = "luban/skill-id/v1"
	skillRevisionDomain = "luban/skill-revision/v1"
)

// CanonicalSkillLocator canonicalizes raw according to the source's locator
// model. MCP skills use absolute virtual-resource URIs; every other current
// source is backed by a filesystem SKILL.md path.
func CanonicalSkillLocator(source SkillSource, raw string) (SkillLocator, error) {
	if err := source.Validate(); err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidSkillLocator, err)
	}
	if source == SourceMCP {
		return CanonicalVirtualSkillLocator(raw)
	}
	return canonicalFilesystemSkillLocator(raw)
}

// canonicalFilesystemSkillLocator returns an absolute, clean filesystem
// locator. Existing symlinks are resolved so a real SKILL.md and an alias to
// it share one identity. Resolution is intentionally best-effort: a deleted
// path must remain canonicalizable for persisted overrides and revokes.
func canonicalFilesystemSkillLocator(raw string) (SkillLocator, error) {
	if err := SkillLocator(raw).Validate(); err != nil {
		return "", err
	}

	// A relative string with a URI scheme is ambiguous. Callers with a virtual
	// resource must use CanonicalVirtualSkillLocator instead. filepath.IsAbs is
	// checked first so Windows drive paths are not mistaken for URI schemes on
	// Windows.
	if !filepath.IsAbs(raw) {
		if parsed, err := url.Parse(raw); err == nil && parsed.Scheme != "" {
			return "", ErrInvalidSkillLocator
		}
	}

	abs, err := filepath.Abs(raw)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidSkillLocator, err)
	}
	abs = filepath.Clean(abs)
	if resolved, resolveErr := filepath.EvalSymlinks(abs); resolveErr == nil {
		if resolvedAbs, absErr := filepath.Abs(resolved); absErr == nil {
			abs = filepath.Clean(resolvedAbs)
		}
	}
	locator := SkillLocator(abs)
	if err := locator.Validate(); err != nil {
		return "", err
	}
	return locator, nil
}

// CanonicalVirtualSkillLocator returns a canonical absolute virtual-resource
// URI. Scheme and host casing, dot path segments, and percent-escape hex case
// are normalized without reordering the query or decoding escaped separators.
// Fragments are rejected because they are client-side selectors rather than
// part of the resource locator and would make ownership ambiguous.
func CanonicalVirtualSkillLocator(raw string) (SkillLocator, error) {
	if err := SkillLocator(raw).Validate(); err != nil {
		return "", err
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Fragment != "" {
		return "", ErrInvalidSkillLocator
	}
	if parsed.Opaque == "" && parsed.Host == "" && parsed.Path == "" {
		return "", ErrInvalidSkillLocator
	}
	if !hasValidPercentEscapes(parsed.RawQuery) || !hasValidPercentEscapes(parsed.Opaque) {
		return "", ErrInvalidSkillLocator
	}

	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.RawQuery = canonicalPercentEscapes(parsed.RawQuery)
	if parsed.Opaque != "" {
		parsed.Opaque = canonicalPercentEscapes(parsed.Opaque)
	} else if parsed.Path != "" {
		escaped := cleanVirtualEscapedPath(parsed.EscapedPath())
		decoded, decodeErr := url.PathUnescape(escaped)
		if decodeErr != nil {
			return "", ErrInvalidSkillLocator
		}
		parsed.Path = decoded
		parsed.RawPath = escaped
	}

	locator := SkillLocator(parsed.String())
	if err := locator.Validate(); err != nil {
		return "", err
	}
	return locator, nil
}

// ComputeSkillID derives the stable source-qualified identity for a canonical
// locator. The display/invocation name is deliberately absent. Length-prefixed
// fields prevent concatenation boundary collisions.
func ComputeSkillID(source SkillSource, locator SkillLocator) (SkillID, error) {
	if err := source.Validate(); err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidSkillID, err)
	}
	if err := locator.Validate(); err != nil {
		return "", err
	}
	material := make([]byte, 0, len(skillIdentityDomain)+len(source)+len(locator)+24)
	material = appendIdentityField(material, skillIdentityDomain)
	material = appendIdentityField(material, string(source))
	material = appendIdentityField(material, string(locator))
	sum := sha256.Sum256(material)
	id := SkillID(skillIDPrefix + string(source) + ":" + hex.EncodeToString(sum[:]))
	if err := id.Validate(); err != nil {
		return "", err
	}
	return id, nil
}

// ComputeSkillDigest hashes the exact effective SKILL.md string used for
// invocation. It performs no trimming, newline normalization, or frontmatter
// stripping, so every byte-level content change produces a different digest.
func ComputeSkillDigest(content string) SkillDigest {
	sum := sha256.Sum256([]byte(content))
	return SkillDigest("sha256:" + hex.EncodeToString(sum[:]))
}

// skillRevisionInput returns a deterministic encoding of every effective
// catalog field except Revision itself. A registry can compare or hash this
// material to decide whether to advance an entry's monotonic SkillRevision.
// The returned slice is newly allocated and safe for the caller to retain.
func skillRevisionInput(skill EffectiveSkill) ([]byte, error) {
	// Validate all effective-state invariants without requiring a revision that
	// is, by definition, derived from this material.
	candidate := skill
	candidate.Revision = 1
	if err := candidate.Validate(); err != nil {
		return nil, err
	}

	fields := []string{
		skillRevisionDomain,
		string(skill.ID),
		skill.Name,
		skill.Summary,
		boolRevisionField(skill.SummaryGenerated),
		string(skill.Source),
		string(skill.Locator),
		string(skill.Digest),
		string(skill.Visibility),
		string(skill.VisibilitySource),
		boolRevisionField(skill.ModelVisible),
		boolRevisionField(skill.DescriptionVisible),
		boolRevisionField(skill.UserInvocable),
		boolRevisionField(skill.Executable),
		boolRevisionField(skill.Mutable),
		skill.ReadOnlyReason,
		string(skill.ShadowedBy),
	}
	var size int
	for _, field := range fields {
		size += 8 + len(field)
	}
	material := make([]byte, 0, size)
	for _, field := range fields {
		material = appendIdentityField(material, field)
	}
	return material, nil
}

// skillRevisionFingerprint hashes skillRevisionInput into lowercase SHA-256
// hex. It is not a SkillDigest: the value fingerprints effective metadata,
// while SkillDigest exclusively identifies exact invoked SKILL.md content.
func skillRevisionFingerprint(skill EffectiveSkill) (string, error) {
	material, err := skillRevisionInput(skill)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(material)
	return hex.EncodeToString(sum[:]), nil
}

func appendIdentityField(dst []byte, value string) []byte {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(value)))
	dst = append(dst, size[:]...)
	return append(dst, value...)
}

func boolRevisionField(value bool) string {
	if value {
		return "1"
	}
	return "0"
}

func cleanVirtualEscapedPath(escaped string) string {
	if escaped == "" {
		return ""
	}
	escaped = canonicalPercentEscapes(escaped)
	hadTrailingSlash := strings.HasSuffix(escaped, "/") && escaped != "/"
	cleaned := path.Clean(escaped)
	if strings.HasPrefix(escaped, "/") && !strings.HasPrefix(cleaned, "/") {
		cleaned = "/" + cleaned
	}
	if hadTrailingSlash && cleaned != "/" {
		cleaned += "/"
	}
	return cleaned
}

func canonicalPercentEscapes(value string) string {
	if !strings.Contains(value, "%") {
		return value
	}
	var builder strings.Builder
	builder.Grow(len(value))
	for index := 0; index < len(value); index++ {
		if value[index] != '%' || index+2 >= len(value) {
			builder.WriteByte(value[index])
			continue
		}
		first, second := value[index+1], value[index+2]
		if !isHexByte(first) || !isHexByte(second) {
			builder.WriteByte(value[index])
			continue
		}
		decoded := hexByteValue(first)<<4 | hexByteValue(second)
		if isURIUnreserved(decoded) {
			builder.WriteByte(decoded)
			index += 2
			continue
		}
		builder.WriteByte('%')
		builder.WriteByte(toUpperHex(first))
		builder.WriteByte(toUpperHex(second))
		index += 2
	}
	return builder.String()
}

func hasValidPercentEscapes(value string) bool {
	for index := 0; index < len(value); index++ {
		if value[index] != '%' {
			continue
		}
		if index+2 >= len(value) || !isHexByte(value[index+1]) || !isHexByte(value[index+2]) {
			return false
		}
		index += 2
	}
	return true
}

func hexByteValue(value byte) byte {
	switch {
	case value >= '0' && value <= '9':
		return value - '0'
	case value >= 'a' && value <= 'f':
		return value - 'a' + 10
	default:
		return value - 'A' + 10
	}
}

func isURIUnreserved(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' ||
		value >= '0' && value <= '9' || value == '-' || value == '.' || value == '_' || value == '~'
}

func isHexByte(value byte) bool {
	return value >= '0' && value <= '9' || value >= 'a' && value <= 'f' || value >= 'A' && value <= 'F'
}

func toUpperHex(value byte) byte {
	if value >= 'a' && value <= 'f' {
		return value - ('a' - 'A')
	}
	return value
}
