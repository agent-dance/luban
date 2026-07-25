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

	"github.com/agent-dance/luban/internal/mcp/catalog"
	mcpmanager "github.com/agent-dance/luban/internal/mcp/manager"
)

const (
	// FeatureFlagMCPSkills is the canonical runtime gate for MCP-backed skills.
	FeatureFlagMCPSkills = "LUBAN_CODE_MCP_SKILLS"
)

// mcpSkillsFeatureEnabled reports whether MCP-backed skill discovery is enabled.
func mcpSkillsFeatureEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(FeatureFlagMCPSkills))) {
	case "1", "true", "yes", "on", "enabled":
		return true
	case "0", "false", "no", "off", "disabled":
		return false
	}
	return false
}

// discoverMCPResourceCatalogInputsFromConnections is the stable-ID discovery
// core. It preserves the server name alongside each resource URI so two MCP
// servers can publish identical skill:// locators without colliding.
func discoverMCPResourceCatalogInputsFromConnections(ctx context.Context, states []mcpmanager.MCPServerConnection) ([]MCPCatalogInput, error) {
	if !mcpSkillsFeatureEnabled() {
		return nil, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var out []MCPCatalogInput
	var errs []error
	for _, state := range states {
		if state.Type != mcpmanager.MCPStateConnected || state.Client == nil || !mcpCapabilityExists(state.Capabilities, "resources") {
			continue
		}
		resources := append([]catalog.Resource(nil), state.Resources...)
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

func isMCPSkillResource(uri string) bool {
	parsed, err := url.Parse(strings.TrimSpace(uri))
	return err == nil && strings.EqualFold(parsed.Scheme, "skill")
}

func mcpSkillMarkdown(contents []catalog.ResourceContent) (string, bool) {
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

func skillFromMCPResource(serverName string, resource catalog.Resource, markdown string) *Skill {
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
		skill.ArgNames = parseArgumentNames(fm.Arguments)
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

func mcpSkillResourceName(resource catalog.Resource) string {
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

func mcpCapabilityExists(caps catalog.ServerCapabilities, key string) bool {
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
