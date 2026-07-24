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

// MCPPrompt is a prompt advertised by an MCP server's prompts/list endpoint.
// It is the unit of discovery used to expose MCP-supplied skills via the
// SkillTool. Mirrors the TS Command-with-loadedFrom=='mcp' shape but stays in
// the skills package so the loader and the tool agree on field names.
//
// Once registered with a Manager, MCP prompts are surfaced through Get/All/
// Names just like SKILL.md-loaded skills, with Source==SourceMCP and a
// "<server>:<name>" namespace so an MCP-supplied "commit" cannot collide
// with a user-authored "commit" skill.
type MCPPrompt struct {
	Server      string   // MCP server alias (e.g. "github", "issues")
	Name        string   // raw prompt name as advertised by the server
	Description string   // human-readable description
	WhenToUse   string   // optional richer guidance
	ArgNames    []string // declared argument names (mapped to $name placeholders)
	Body        string   // resolved prompt body — may be lazily populated
}

// MCPCatalogInput is the immutable discovery boundary between MCP stores and
// the authoritative revisioned Manager. Skill carries the legacy parsed value;
// ID, Locator, and Digest carry the source-aware inputs that must not be
// reconstructed from its display name.
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

// CatalogInput converts a prompt definition into its stable catalog inputs.
// The original server and prompt names are encoded separately; QualifiedName
// remains presentation-only and is never parsed back into identity parts.
func (p MCPPrompt) CatalogInput() (MCPCatalogInput, error) {
	skill := p.toSkill()
	if skill == nil {
		return MCPCatalogInput{}, fmt.Errorf("invalid MCP prompt catalog input")
	}
	locator, err := MCPPromptCatalogLocator(p.Server, p.Name)
	if err != nil {
		return MCPCatalogInput{}, err
	}
	return newMCPCatalogInput(skill, locator)
}

// MCPPromptCatalogLocator creates a stable virtual locator for one MCP prompt.
// Empty server names remain supported for the legacy single-prompt shim.
func MCPPromptCatalogLocator(serverName, promptName string) (SkillLocator, error) {
	return mcpCatalogLocator("prompt", serverName, promptName, true)
}

// MCPResourceCatalogLocator creates a stable virtual locator for one resource.
// The server is part of identity so two servers may publish the same skill://
// URI without colliding.
func MCPResourceCatalogLocator(serverName, resourceURI string) (SkillLocator, error) {
	serverName = strings.TrimSpace(serverName)
	resourceURI = strings.TrimSpace(resourceURI)
	if serverName == "" || hasControl(serverName) || !isMCPSkillResource(resourceURI) {
		return "", ErrInvalidSkillLocator
	}
	canonicalResource, err := CanonicalVirtualSkillLocator(resourceURI)
	if err != nil {
		return "", err
	}
	return mcpCatalogLocator("resource", serverName, string(canonicalResource), false)
}

// QualifiedName returns the namespaced skill name used inside the SkillTool
// (server:name). Both halves are required; an empty server falls back to
// just Name so callers can still register single-prompt MCP shims for tests.
func (p MCPPrompt) QualifiedName() string {
	server := strings.TrimSpace(p.Server)
	name := strings.TrimSpace(p.Name)
	if server == "" {
		return name
	}
	if name == "" {
		return ""
	}
	return server + ":" + name
}

// toSkill builds a Skill object suitable for caching in the Manager. The
// body is treated as already-stripped content (MCP servers return prompt
// templates without YAML frontmatter), so we can populate Content directly.
func (p MCPPrompt) toSkill() *Skill {
	qn := p.QualifiedName()
	if qn == "" {
		return nil
	}
	desc := strings.TrimSpace(p.Description)
	if desc == "" {
		desc = i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyAuxMCPPromptDescription, qn)
	}
	return &Skill{
		Name:                        qn,
		Description:                 desc,
		Source:                      SourceMCP,
		FilePath:                    "", // MCP prompts have no on-disk file
		SkillDir:                    "",
		RawContent:                  p.Body,
		Content:                     p.Body,
		ContentLength:               len(p.Body),
		ArgNames:                    append([]string(nil), p.ArgNames...),
		WhenToUse:                   strings.TrimSpace(p.WhenToUse),
		HasUserSpecifiedDescription: strings.TrimSpace(p.Description) != "",
	}
}

// mcpPromptStore is an internal, manager-attached registry that records the
// MCP-discovered prompts. Manager.RegisterMCPPrompts mutates this and the
// snapshot is consulted in populate() so on-disk skills do not need to be
// touched.
type mcpPromptStore struct {
	mu      sync.RWMutex
	prompts map[SkillID]MCPCatalogInput // prompt locator identity → input
	skills  map[SkillID]MCPCatalogInput // resource locator identity → input
}

func newMCPPromptStore() *mcpPromptStore {
	return &mcpPromptStore{
		prompts: make(map[SkillID]MCPCatalogInput),
		skills:  make(map[SkillID]MCPCatalogInput),
	}
}

func (s *mcpPromptStore) replace(prompts []MCPPrompt) {
	next := make(map[SkillID]MCPCatalogInput, len(prompts))
	for _, p := range prompts {
		input, err := p.CatalogInput()
		if err != nil {
			continue
		}
		// First-write-wins inside a single replace call to preserve the
		// order MCP servers return prompts.
		if _, dup := next[input.ID]; dup {
			continue
		}
		next[input.ID] = input.Clone()
	}
	s.mu.Lock()
	s.prompts = next
	s.mu.Unlock()
}

func (s *mcpPromptStore) replaceSkills(skills []*Skill) {
	inputs := make([]MCPCatalogInput, 0, len(skills))
	for _, sk := range skills {
		if sk == nil || strings.TrimSpace(sk.Name) == "" {
			continue
		}
		cp := cloneMCPSkill(sk)
		cp.Source = SourceMCP
		serverName, _, ok := strings.Cut(cp.Name, ":")
		if !ok {
			continue
		}
		locator, err := MCPResourceCatalogLocator(serverName, cp.FilePath)
		if err != nil {
			continue
		}
		input, err := newMCPCatalogInput(cp, locator)
		if err == nil {
			inputs = append(inputs, input)
		}
	}
	s.replaceSkillInputs(inputs)
}

func (s *mcpPromptStore) replaceSkillInputs(inputs []MCPCatalogInput) {
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
	s.skills = next
	s.mu.Unlock()
}

func (s *mcpPromptStore) replaceAllInputs(inputs []MCPCatalogInput) {
	prompts := make(map[SkillID]MCPCatalogInput)
	resources := make(map[SkillID]MCPCatalogInput)
	for _, input := range inputs {
		kind, err := mcpCatalogInputKind(input)
		if err != nil {
			continue
		}
		target := resources
		if kind == "prompt" {
			target = prompts
		}
		if _, duplicate := target[input.ID]; duplicate {
			continue
		}
		target[input.ID] = input.Clone()
	}
	s.mu.Lock()
	s.prompts = prompts
	s.skills = resources
	s.mu.Unlock()
}

func (s *mcpPromptStore) inputsSnapshot() []MCPCatalogInput {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]MCPCatalogInput, 0, len(s.prompts)+len(s.skills))
	for _, input := range s.prompts {
		out = append(out, input.Clone())
	}
	for _, input := range s.skills {
		out = append(out, input.Clone())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (s *mcpPromptStore) skillsInputSnapshot() []MCPCatalogInput {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]MCPCatalogInput, 0, len(s.skills))
	for _, input := range s.skills {
		out = append(out, input.Clone())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (s *mcpPromptStore) snapshot() []*Skill {
	inputs := s.inputsSnapshot()
	out := make([]*Skill, 0, len(inputs))
	for _, input := range inputs {
		out = append(out, input.Skill)
	}
	return out
}

func (s *mcpPromptStore) skillsSnapshot() []*Skill {
	inputs := s.skillsInputSnapshot()
	out := make([]*Skill, 0, len(inputs))
	for _, input := range inputs {
		out = append(out, input.Skill)
	}
	return out
}

// RegisterMCPPrompts replaces the manager's known MCP prompt set. Callers
// (typically the MCP client adapter) invoke this whenever the connected
// servers' prompts/list output changes; the manager invalidates its cache
// so the next Get/All/Names re-merges them with on-disk skills.
//
// Passing an empty slice clears all MCP-supplied skills.
func (m *Manager) RegisterMCPPrompts(prompts []MCPPrompt) {
	if m == nil {
		return
	}
	store := m.ensureMCPPromptStore()
	store.replace(prompts)

	m.mu.Lock()
	m.cache = make(map[string]*Skill)
	m.populated = false
	m.mu.Unlock()
}

// RegisterMCPSkills replaces the manager's skill:// resource-backed MCP skill
// set. It shares the same merge path as MCP prompts while keeping independent
// storage so prompts/list_changed and resources/list_changed can invalidate
// their caches separately.
func (m *Manager) RegisterMCPSkills(skills []*Skill) {
	if m == nil {
		return
	}
	store := m.ensureMCPPromptStore()
	store.replaceSkills(skills)

	m.mu.Lock()
	m.cache = make(map[string]*Skill)
	m.populated = false
	m.mu.Unlock()
}

// RegisterMCPSkillCatalogInputs replaces the resource-backed MCP catalog using
// stable inputs produced during discovery. It avoids reconstructing a server
// identity from the presentation name while preserving RegisterMCPSkills for
// legacy callers.
func (m *Manager) RegisterMCPSkillCatalogInputs(inputs []MCPCatalogInput) {
	if m == nil {
		return
	}
	store := m.ensureMCPPromptStore()
	store.replaceSkillInputs(inputs)

	m.mu.Lock()
	m.cache = make(map[string]*Skill)
	m.populated = false
	m.mu.Unlock()
}

// MCPCatalogInputs returns prompt- and resource-backed catalog inputs sorted by
// stable ID. Every Skill is defensively cloned.
func (m *Manager) MCPCatalogInputs() []MCPCatalogInput {
	store := m.currentMCPPromptStore()
	if store == nil {
		return nil
	}
	return store.inputsSnapshot()
}

// MCPSkillCatalogInputs returns only resource-backed MCP catalog inputs.
func (m *Manager) MCPSkillCatalogInputs() []MCPCatalogInput {
	store := m.currentMCPPromptStore()
	if store == nil {
		return nil
	}
	return store.skillsInputSnapshot()
}

// MCPPrompts returns a snapshot of all currently registered MCP-supplied
// prompt-like skills. It includes both legacy prompts/list shims and
// resource-backed MCP skills so existing SkillTool call sites keep one MCP
// source surface.
func (m *Manager) MCPPrompts() []*Skill {
	store := m.currentMCPPromptStore()
	if store == nil {
		return nil
	}
	return store.snapshot()
}

// MCPSkills returns only skill:// resource-backed MCP skills.
func (m *Manager) MCPSkills() []*Skill {
	store := m.currentMCPPromptStore()
	if store == nil {
		return nil
	}
	return store.skillsSnapshot()
}

// mergeMCPPromptsLocked is invoked by populate() under m.mu's write lock.
// It overlays the registered MCP prompts on top of on-disk skills with
// first-match-wins semantics: an on-disk skill of the same qualified name
// wins (so users can locally override a server-provided prompt).
func (m *Manager) mergeMCPPromptsLocked() {
	if m.mcpPrompts == nil {
		return
	}
	for _, sk := range m.mcpPrompts.snapshot() {
		if sk == nil || sk.Name == "" {
			continue
		}
		if _, exists := m.cache[sk.Name]; exists {
			continue
		}
		m.cache[sk.Name] = sk
	}
}

func cloneMCPSkill(in *Skill) *Skill {
	if in == nil {
		return nil
	}
	cp := *in
	cp.AllowedTools = append([]string(nil), in.AllowedTools...)
	cp.ArgNames = append([]string(nil), in.ArgNames...)
	cp.Paths = append([]string(nil), in.Paths...)
	cp.Aliases = append([]string(nil), in.Aliases...)
	if in.UserInvocable != nil {
		v := *in.UserInvocable
		cp.UserInvocable = &v
	}
	return &cp
}

func (m *Manager) ensureMCPPromptStore() *mcpPromptStore {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.mcpPrompts == nil {
		m.mcpPrompts = newMCPPromptStore()
	}
	return m.mcpPrompts
}

func (m *Manager) currentMCPPromptStore() *mcpPromptStore {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.mcpPrompts
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
	input := MCPCatalogInput{
		Skill:   cp,
		ID:      id,
		Locator: locator,
		Digest:  ComputeSkillDigest(cp.RawContent),
	}
	if err := input.Validate(); err != nil {
		return MCPCatalogInput{}, err
	}
	return input, nil
}

func newMCPResourceCatalogInput(serverName string, skill *Skill) (MCPCatalogInput, error) {
	if skill == nil {
		return MCPCatalogInput{}, fmt.Errorf("invalid MCP resource catalog input")
	}
	locator, err := MCPResourceCatalogLocator(serverName, skill.FilePath)
	if err != nil {
		return MCPCatalogInput{}, err
	}
	return newMCPCatalogInput(skill, locator)
}

func mcpCatalogLocator(kind, serverName, objectIdentity string, allowEmptyServer bool) (SkillLocator, error) {
	serverName = strings.TrimSpace(serverName)
	objectIdentity = strings.TrimSpace(objectIdentity)
	if (!allowEmptyServer && serverName == "") || objectIdentity == "" || hasControl(serverName) || hasControl(objectIdentity) {
		return "", ErrInvalidSkillLocator
	}
	query := url.Values{}
	query.Set("identity", objectIdentity)
	query.Set("server", serverName)
	raw := (&url.URL{Scheme: mcpCatalogLocatorScheme, Host: kind, RawQuery: query.Encode()}).String()
	return CanonicalVirtualSkillLocator(raw)
}
