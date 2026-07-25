package collaboration

import (
	"crypto/rand"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/swarm"
)

const teamLeadName = "team-lead"

var teamNameAdjectives = []string{
	"bright", "calm", "clever", "cozy", "curious", "eager", "gentle", "joyful",
	"lively", "mellow", "nimble", "polished", "radiant", "steady", "swift", "witty",
}

var teamNameNouns = []string{
	"atlas", "beacon", "brook", "cipher", "ember", "harbor", "meadow", "nexus",
	"orchard", "quill", "signal", "spruce", "star", "summit", "willow", "zephyr",
}

func sanitizeSwarmName(name, fallback string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = fallback
	}
	var builder strings.Builder
	lastDash := false
	for _, value := range name {
		switch {
		case unicode.IsLetter(value) || unicode.IsDigit(value):
			builder.WriteRune(value)
			lastDash = false
		case value == '_' || value == '-':
			builder.WriteRune(value)
			lastDash = false
		case unicode.IsSpace(value):
			if !lastDash {
				builder.WriteByte('-')
				lastDash = true
			}
		default:
			if !lastDash {
				builder.WriteByte('-')
				lastDash = true
			}
		}
	}
	sanitized := strings.Trim(builder.String(), "-_")
	for sanitized != "" {
		value := rune(sanitized[0])
		if unicode.IsLetter(value) || unicode.IsDigit(value) {
			break
		}
		sanitized = sanitized[1:]
	}
	if sanitized == "" {
		sanitized = fallback
	}
	if len(sanitized) > 64 {
		sanitized = strings.TrimRight(sanitized[:64], "-_")
	}
	if sanitized == "" {
		return "team"
	}
	return sanitized
}

func teamStorageName(teamName string) string {
	teamName = strings.TrimSpace(teamName)
	if teamName == "" {
		teamName = "team"
	}
	var builder strings.Builder
	for _, value := range teamName {
		if value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9' {
			builder.WriteRune(unicode.ToLower(value))
			continue
		}
		builder.WriteByte('-')
	}
	sanitized := strings.Trim(builder.String(), "-")
	if sanitized == "" {
		sanitized = "team"
	}
	if len(sanitized) > 64 {
		sanitized = strings.TrimRight(sanitized[:64], "-")
	}
	if sanitized == "" {
		return "team"
	}
	return sanitized
}

func uniqueTeamName(requested string) (string, error) {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return "", i18n.NewError(i18n.KeyToolRuntimeRequiredFieldMissing, "team_name")
	}
	if !teamConfigExists(teamStorageName(requested)) {
		return requested, nil
	}
	for range 64 {
		candidate := randomTeamSlug()
		if !teamConfigExists(candidate) {
			return candidate, nil
		}
	}
	return "", i18n.NewError(i18n.KeyToolRuntimeTeamUniqueNameGenerationFailed, "team")
}

func teamConfigExists(storageName string) bool {
	path, err := swarm.TeamConfigPath(storageName)
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

func randomTeamSlug() string {
	var values [2]byte
	if _, err := rand.Read(values[:]); err != nil {
		return fmt.Sprintf("team-%d", time.Now().UnixNano())
	}
	adjective := teamNameAdjectives[int(values[0])%len(teamNameAdjectives)]
	noun := teamNameNouns[int(values[1])%len(teamNameNouns)]
	return sanitizeSwarmName(adjective+"-"+noun, "team")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
