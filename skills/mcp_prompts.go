package skills

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"

	"github.com/agent-dance/luban/i18n"
)

const mcpCatalogLocatorScheme = "mcp-skill"

// MCPCatalogInput is the immutable discovery boundary between MCP resources
// and the authoritative revisioned Manager. ID, Locator, and Digest preserve
// source identity without reconstructing it from a display name.
type MCPCatalogInput struct {
	Skill   *Skill
	ID      SkillID
	Locator SkillLocator
	Digest  SkillDigest
}

// Validate checks source ownership, identity, canonical locator, and the exact
// registered markdown digest.
func (input MCPCatalogInput) Validate() error {
	if input.Skill == nil {
		return fmt.Errorf("invalid MCP catalog input: nil skill")
	}
	if input.Skill.Source != SourceMCP {
		return fmt.Errorf("invalid MCP catalog input: source %q", input.Skill.Source)
	}
	canonical, err := CanonicalVirtualSkillLocator(string(input.Locator))
	if err != nil || canonical != input.Locator {
		return ErrInvalidSkillLocator
	}
	expectedID, err := ComputeSkillID(SourceMCP, input.Locator)
	if err != nil {
		return err
	}
	if input.ID != expectedID {
		return fmt.Errorf("%w: MCP catalog locator does not match ID", ErrInvalidSkillID)
	}
	if err := input.Digest.Validate(); err != nil {
		return err
	}
	if input.Digest != ComputeSkillDigest(input.Skill.RawContent) {
		return fmt.Errorf("%w: MCP catalog content does not match digest", ErrInvalidSkillDigest)
	}
	return nil
}

// Clone returns a defensive copy suitable for crossing the MCP store boundary.
func (input MCPCatalogInput) Clone() MCPCatalogInput {
	input.Skill = cloneMCPSkill(input.Skill)
	return input
}

// mcpResourceCatalogLocator creates a stable virtual locator for one resource.
// The server is part of identity so two servers may publish the same skill://
// URI without colliding.
func mcpResourceCatalogLocator(serverName, resourceURI string) (SkillLocator, error) {
	serverName = strings.TrimSpace(serverName)
	resourceURI = strings.TrimSpace(resourceURI)
	if serverName == "" || hasControl(serverName) || !isMCPSkillResource(resourceURI) {
		return "", ErrInvalidSkillLocator
	}
	canonicalResource, err := CanonicalVirtualSkillLocator(resourceURI)
	if err != nil {
		return "", err
	}
	return mcpCatalogLocator("resource", serverName, string(canonicalResource))
}

// NewMCPPromptCatalogInput creates a stable catalog entry from an MCP prompt.
func NewMCPPromptCatalogInput(serverName, promptName, description string, argNames []string, body string) (MCPCatalogInput, error) {
	serverName = strings.TrimSpace(serverName)
	promptName = strings.TrimSpace(promptName)
	if serverName == "" || promptName == "" {
		return MCPCatalogInput{}, ErrInvalidSkillLocator
	}
	qualifiedName := serverName + ":" + promptName
	description = strings.TrimSpace(description)
	hasDescription := description != ""
	if description == "" {
		description = i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyAuxMCPPromptDescription, qualifiedName)
	}
	skill := &Skill{
		Name:                        qualifiedName,
		Description:                 description,
		Source:                      SourceMCP,
		RawContent:                  body,
		Content:                     body,
		ContentLength:               len(body),
		ArgNames:                    append([]string(nil), argNames...),
		WhenToUse:                   description,
		HasUserSpecifiedDescription: hasDescription,
	}
	locator, err := mcpCatalogLocator("prompt", serverName, promptName)
	if err != nil {
		return MCPCatalogInput{}, err
	}
	return newMCPCatalogInput(skill, locator)
}

type mcpCatalogStore struct {
	mu     sync.RWMutex
	inputs map[SkillID]MCPCatalogInput
}

func newMCPCatalogStore() *mcpCatalogStore {
	return &mcpCatalogStore{inputs: make(map[SkillID]MCPCatalogInput)}
}

func (s *mcpCatalogStore) replace(inputs []MCPCatalogInput) {
	next := make(map[SkillID]MCPCatalogInput, len(inputs))
	for _, input := range inputs {
		if err := input.Validate(); err != nil {
			continue
		}
		if _, duplicate := next[input.ID]; duplicate {
			continue
		}
		next[input.ID] = input.Clone()
	}
	s.mu.Lock()
	s.inputs = next
	s.mu.Unlock()
}

func (s *mcpCatalogStore) snapshot() []MCPCatalogInput {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]MCPCatalogInput, 0, len(s.inputs))
	for _, input := range s.inputs {
		out = append(out, input.Clone())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// MCPCatalogInputs returns the unified prompt- and resource-backed projection
// sorted by stable ID.
// Every Skill is defensively cloned.
func (m *Manager) MCPCatalogInputs() []MCPCatalogInput {
	store := m.currentMCPCatalogStore()
	if store == nil {
		return nil
	}
	return store.snapshot()
}

func cloneMCPSkill(in *Skill) *Skill {
	if in == nil {
		return nil
	}
	cp := *in
	cp.AllowedTools = append([]string(nil), in.AllowedTools...)
	cp.ArgNames = append([]string(nil), in.ArgNames...)
	cp.Paths = append([]string(nil), in.Paths...)
	if in.UserInvocable != nil {
		v := *in.UserInvocable
		cp.UserInvocable = &v
	}
	return &cp
}

func (m *Manager) ensureMCPCatalogStore() *mcpCatalogStore {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.mcpCatalog == nil {
		m.mcpCatalog = newMCPCatalogStore()
	}
	return m.mcpCatalog
}

func (m *Manager) currentMCPCatalogStore() *mcpCatalogStore {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.mcpCatalog
}

func newMCPCatalogInput(skill *Skill, locator SkillLocator) (MCPCatalogInput, error) {
	if skill == nil {
		return MCPCatalogInput{}, fmt.Errorf("invalid MCP catalog input: nil skill")
	}
	cp := cloneMCPSkill(skill)
	cp.Source = SourceMCP
	id, err := ComputeSkillID(SourceMCP, locator)
	if err != nil {
		return MCPCatalogInput{}, err
	}
	input := MCPCatalogInput{Skill: cp, ID: id, Locator: locator, Digest: ComputeSkillDigest(cp.RawContent)}
	if err := input.Validate(); err != nil {
		return MCPCatalogInput{}, err
	}
	return input, nil
}

func newMCPResourceCatalogInput(serverName string, skill *Skill) (MCPCatalogInput, error) {
	if skill == nil {
		return MCPCatalogInput{}, fmt.Errorf("invalid MCP resource catalog input")
	}
	locator, err := mcpResourceCatalogLocator(serverName, skill.FilePath)
	if err != nil {
		return MCPCatalogInput{}, err
	}
	return newMCPCatalogInput(skill, locator)
}

func mcpCatalogLocator(kind, serverName, objectIdentity string) (SkillLocator, error) {
	serverName = strings.TrimSpace(serverName)
	objectIdentity = strings.TrimSpace(objectIdentity)
	if serverName == "" || objectIdentity == "" || hasControl(serverName) || hasControl(objectIdentity) {
		return "", ErrInvalidSkillLocator
	}
	query := url.Values{}
	query.Set("identity", objectIdentity)
	query.Set("server", serverName)
	raw := (&url.URL{Scheme: mcpCatalogLocatorScheme, Host: kind, RawQuery: query.Encode()}).String()
	return CanonicalVirtualSkillLocator(raw)
}
