package skills

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"sort"
	"strings"

	svcmcp "github.com/agent-dance/luban/services/mcp"
)

const (
	// FeatureFlagMCPSkills mirrors the TypeScript feature('MCP_SKILLS') gate.
	FeatureFlagMCPSkills = "MCP_SKILLS"
)

// MCPSkillsFeatureEnabled reports whether resource-backed MCP skill discovery
// is enabled. The additional env names make local Go tests/runtime explicit
// while preserving the canonical MCP_SKILLS gate name.
func MCPSkillsFeatureEnabled() bool {
	for _, name := range []string{FeatureFlagMCPSkills, "CLAUDE_CODE_MCP_SKILLS", "CLAUDE_CODE_ENABLE_MCP_SKILLS"} {
		switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
		case "1", "true", "yes", "on", "enabled":
			return true
		case "0", "false", "no", "off", "disabled":
			return false
		}
	}
	return false
}

// DiscoverMCPSkills discovers skill:// resources from connected MCP servers
// and converts their SKILL.md content into SourceMCP skills. It intentionally
// reads the manager snapshot rather than connecting new servers; task_07 owns
// connection state and TS also fetches MCP skills only for connected clients.
func DiscoverMCPSkills(ctx context.Context, manager *svcmcp.Manager) ([]*Skill, error) {
	if manager == nil {
		return nil, errors.New("skills: nil MCP manager")
	}
	return DiscoverMCPSkillsFromConnections(ctx, manager.Snapshot())
}

// DiscoverMCPSkillsFromConnections is the testable core for resource-backed
// MCP skills. Only connected servers with resources capability and live clients
// participate.
func DiscoverMCPSkillsFromConnections(ctx context.Context, states []svcmcp.MCPServerConnection) ([]*Skill, error) {
	inputs, err := DiscoverMCPSkillCatalogInputsFromConnections(ctx, states)
	out := make([]*Skill, 0, len(inputs))
	for _, input := range inputs {
		out = append(out, cloneMCPSkill(input.Skill))
	}
	return out, err
}

// DiscoverMCPSkillCatalogInputsFromConnections is the stable-ID discovery
// core. It preserves the server name alongside each resource URI so two MCP
// servers can publish identical skill:// locators without colliding.
func DiscoverMCPSkillCatalogInputsFromConnections(ctx context.Context, states []svcmcp.MCPServerConnection) ([]MCPCatalogInput, error) {
	if !MCPSkillsFeatureEnabled() {
		return nil, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var out []MCPCatalogInput
	var errs []error
	for _, state := range states {
		if state.Type != svcmcp.MCPStateConnected || state.Client == nil || !mcpCapabilityExists(state.Capabilities, "resources") {
			continue
		}
		resources := append([]svcmcp.Resource(nil), state.Resources...)
		if len(resources) == 0 {
			result, err := state.Client.ListResourcesResult(ctx)
			if err != nil {
				errs = append(errs, fmt.Errorf("skills: list MCP resources for %s: %w", state.Name, err))
				continue
			}
			resources = result.Resources
		}
		for _, resource := range resources {
			if !isMCPSkillResource(resource.URI) {
				continue
			}
			result, err := state.Client.ReadResourceResult(ctx, resource.URI)
			if err != nil {
				errs = append(errs, fmt.Errorf("skills: read MCP skill %s from %s: %w", resource.URI, state.Name, err))
				continue
			}
			markdown, ok := mcpSkillMarkdown(result.Contents)
			if !ok {
				continue
			}
			if skill := skillFromMCPResource(state.Name, resource, markdown); skill != nil {
				input, inputErr := newMCPResourceCatalogInput(state.Name, skill)
				if inputErr != nil {
					errs = append(errs, fmt.Errorf("skills: catalog MCP skill %s from %s: %w", resource.URI, state.Name, inputErr))
					continue
				}
				out = append(out, input)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, errors.Join(errs...)
}

// RegisterDiscoveredMCPSkills refreshes the skill manager's MCP skill set from
// the current MCP connection snapshot.
func RegisterDiscoveredMCPSkills(ctx context.Context, skillManager *Manager, mcpManager *svcmcp.Manager) error {
	if skillManager == nil {
		return errors.New("skills: nil skill manager")
	}
	if mcpManager == nil {
		return errors.New("skills: nil MCP manager")
	}
	return RefreshMCPSkillCatalogFromConnections(ctx, skillManager, mcpManager.Snapshot())
}

// RefreshMCPSkillCatalogFromConnections atomically replaces resource-backed
// inputs only after a fully successful discovery. A transient list/read error
// therefore retains the last authoritative set instead of becoming a false
// delete; a successful disconnected or feature-gated empty set clears it.
func RefreshMCPSkillCatalogFromConnections(ctx context.Context, skillManager *Manager, states []svcmcp.MCPServerConnection) error {
	if skillManager == nil {
		return errors.New("skills: nil skill manager")
	}
	inputs, err := DiscoverMCPSkillCatalogInputsFromConnections(ctx, states)
	if err != nil {
		return err
	}
	skillManager.RegisterMCPSkillCatalogInputs(inputs)
	return nil
}

// InvalidateMCPSkills is the resources/list_changed hook target for task_13.
func InvalidateMCPSkills(skillManager *Manager) {
	if skillManager == nil {
		return
	}
	skillManager.RegisterMCPSkills(nil)
}

func isMCPSkillResource(uri string) bool {
	parsed, err := url.Parse(strings.TrimSpace(uri))
	return err == nil && strings.EqualFold(parsed.Scheme, "skill")
}

func mcpSkillMarkdown(contents []svcmcp.ResourceContent) (string, bool) {
	var parts []string
	for _, content := range contents {
		if strings.TrimSpace(content.Text) != "" {
			parts = append(parts, content.Text)
			continue
		}
		if strings.TrimSpace(content.Blob) != "" {
			decoded, err := base64.StdEncoding.DecodeString(stripMCPResourceBase64(content.Blob))
			if err == nil {
				parts = append(parts, string(decoded))
			}
		}
	}
	if len(parts) == 0 {
		return "", false
	}
	return strings.Join(parts, "\n"), true
}

func skillFromMCPResource(serverName string, resource svcmcp.Resource, markdown string) *Skill {
	baseName := mcpSkillResourceName(resource)
	if baseName == "" || strings.TrimSpace(serverName) == "" {
		return nil
	}
	qualifiedName := strings.TrimSpace(serverName) + ":" + baseName
	parsed := parseFrontmatter(markdown, resource.URI)
	description := strings.TrimSpace(resource.Description)
	hasUserSpecifiedDescription := false
	if parsed.Frontmatter.Description != nil && strings.TrimSpace(*parsed.Frontmatter.Description) != "" {
		description = strings.TrimSpace(*parsed.Frontmatter.Description)
		hasUserSpecifiedDescription = true
	}
	if description == "" {
		description = "MCP skill: " + qualifiedName
	}
	skill := &Skill{
		Name:                        qualifiedName,
		Description:                 description,
		Source:                      SourceMCP,
		FilePath:                    resource.URI,
		SkillDir:                    "",
		RawContent:                  markdown,
		Content:                     parsed.Content,
		ContentLength:               len(parsed.Content),
		HasUserSpecifiedDescription: hasUserSpecifiedDescription,
		HasGeneratedDescription:     !hasUserSpecifiedDescription && strings.TrimSpace(resource.Description) == "",
	}
	applyMCPFrontmatter(skill, parsed.Frontmatter)
	return skill
}

func applyMCPFrontmatter(skill *Skill, fm rawFrontmatter) {
	if skill == nil {
		return
	}
	if len(fm.AllowedTools) > 0 {
		skill.AllowedTools = append([]string(nil), fm.AllowedTools...)
	}
	if fm.ArgumentHint != nil {
		skill.ArgumentHint = *fm.ArgumentHint
	}
	if len(fm.Arguments) > 0 {
		skill.ArgNames = ParseArgumentNames(fm.Arguments)
	}
	if fm.WhenToUse != nil {
		skill.WhenToUse = *fm.WhenToUse
	}
	if fm.Version != nil {
		skill.Version = *fm.Version
	}
	if fm.Model != nil && strings.TrimSpace(*fm.Model) != "inherit" {
		skill.Model = *fm.Model
	}
	if fm.DisableModelInvocation != nil {
		skill.DisableModelInvocation = parseBoolString(*fm.DisableModelInvocation)
	}
	if fm.UserInvocable != nil {
		v := parseBoolString(*fm.UserInvocable)
		skill.UserInvocable = &v
	}
	if fm.Context != nil && *fm.Context == "fork" {
		skill.Context = ContextFork
	}
	if fm.Agent != nil {
		skill.Agent = *fm.Agent
	}
	if fm.Effort != nil {
		skill.Effort = *fm.Effort
	}
	if len(fm.Paths) > 0 {
		skill.Paths = append([]string(nil), fm.Paths...)
	}
	if fm.Shell != nil {
		shell := strings.ToLower(strings.TrimSpace(*fm.Shell))
		if shell == "bash" || shell == "powershell" {
			skill.Shell = shell
		}
	}
}

func mcpSkillResourceName(resource svcmcp.Resource) string {
	if name := sanitizeMCPSkillName(resource.Name); name != "" {
		return name
	}
	parsed, err := url.Parse(strings.TrimSpace(resource.URI))
	if err != nil {
		return ""
	}
	resourcePath := strings.Trim(parsed.Path, "/")
	candidates := []string{
		path.Base(resourcePath),
		path.Base(path.Dir(resourcePath)),
		strings.TrimSpace(parsed.Host),
	}
	for _, candidate := range candidates {
		candidate = strings.TrimSuffix(candidate, ".md")
		if candidate == "SKILL" || candidate == "." || candidate == "/" {
			continue
		}
		if name := sanitizeMCPSkillName(candidate); name != "" {
			return name
		}
	}
	return ""
}

func sanitizeMCPSkillName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.TrimSuffix(name, "/")
	name = strings.TrimSuffix(name, ".md")
	if name == "." || name == "/" {
		return ""
	}
	name = strings.ReplaceAll(name, " ", "-")
	return name
}

func mcpCapabilityExists(caps svcmcp.ServerCapabilities, key string) bool {
	if caps == nil {
		return false
	}
	value, ok := caps[key]
	if !ok || value == nil {
		return false
	}
	if enabled, ok := value.(bool); ok {
		return enabled
	}
	return true
}

func stripMCPResourceBase64(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '\n', '\r', '\t', ' ':
			return -1
		default:
			return r
		}
	}, s)
}
