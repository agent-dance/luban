// Package skillauthority validates the skill catalog authority pinned to one
// runtime-owned tool execution.
package skillauthority

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/agent-dance/luban/i18n"
	executioncontract "github.com/agent-dance/luban/internal/contracts/execution"
	"github.com/agent-dance/luban/skills"
	"github.com/agent-dance/luban/types"
)

// Authority is the immutable skill-project authority captured from an active,
// runtime-owned tool execution. Its fields are private so callers cannot forge
// a pinned generation from public session or path strings.
type Authority struct {
	identity             executioncontract.RuntimeOwnerIdentity
	canonicalProjectRoot string
	generation           skills.ProjectSourceGeneration
	enabled              bool
}

// Capture captures and validates the project generation owned by ctx. A nil
// manager disables skill authority checks and returns the zero Authority.
func Capture(ctx context.Context, manager *skills.Manager) (Authority, error) {
	if manager == nil {
		return Authority{}, nil
	}
	exec, ok := executioncontract.ToolExecutionContextFromContext(ctx)
	if !ok || !exec.IsRuntimeOwned() {
		return Authority{}, generationChangedError(skills.ErrSkillProjectGenerationChanged)
	}
	sessionID, sessionProjectDir, projectRoot, cwd, active := exec.ActiveRuntimeOwnerIdentity()
	if !active {
		return Authority{}, generationChangedError(skills.ErrSkillProjectGenerationChanged)
	}
	canonicalRoot, canonical := canonicalProjectRoot(projectRoot)
	if !canonical {
		return Authority{}, generationChangedError(skills.ErrSkillProjectGenerationChanged)
	}
	generationValue, pinned := exec.SkillProjectGeneration()
	if !pinned {
		return Authority{}, generationChangedError(skills.ErrSkillProjectGenerationChanged)
	}
	generation := skills.ProjectSourceGeneration(generationValue)
	if err := manager.ValidateProjectGeneration(generation); err != nil {
		return Authority{}, generationChangedError(err)
	}
	return Authority{
		identity: executioncontract.RuntimeOwnerIdentity{
			SessionID:         strings.TrimSpace(sessionID),
			SessionProjectDir: strings.TrimSpace(sessionProjectDir),
			ProjectRoot:       projectRoot,
			CWD:               cwd,
		},
		canonicalProjectRoot: canonicalRoot,
		generation:           generation,
		enabled:              true,
	}, nil
}

// ValidateRuntime rejects a mutable runtime snapshot that no longer belongs to
// the session and project root captured by Authority.
func (authority Authority) ValidateRuntime(runtime types.ToolRuntimeContext) error {
	if !authority.enabled {
		return nil
	}
	runtimeRoot, canonical := canonicalProjectRoot(runtime.ProjectRoot)
	if !canonical || authority.canonicalProjectRoot == "" ||
		strings.TrimSpace(runtime.SessionID) != authority.identity.SessionID ||
		runtimeRoot != authority.canonicalProjectRoot {
		return generationChangedError(skills.ErrSkillProjectGenerationChanged)
	}
	return nil
}

// ValidateContext proves that ctx still carries the active execution identity
// and skill generation captured by Authority. It fails closed after a run
// ends, a workspace is retargeted, or a symlink changes its destination.
func (authority Authority) ValidateContext(ctx context.Context) error {
	if !authority.enabled {
		return nil
	}
	exec, ok := executioncontract.ToolExecutionContextFromContext(ctx)
	if !ok || !exec.IsRuntimeOwned() {
		return generationChangedError(skills.ErrSkillProjectGenerationChanged)
	}
	sessionID, sessionProjectDir, projectRoot, cwd, active := exec.ActiveRuntimeOwnerIdentity()
	if !active || strings.TrimSpace(sessionID) != authority.identity.SessionID ||
		strings.TrimSpace(sessionProjectDir) != authority.identity.SessionProjectDir ||
		cwd != authority.identity.CWD {
		return generationChangedError(skills.ErrSkillProjectGenerationChanged)
	}
	runtimeRoot, canonical := canonicalProjectRoot(projectRoot)
	if !canonical || runtimeRoot != authority.canonicalProjectRoot {
		return generationChangedError(skills.ErrSkillProjectGenerationChanged)
	}
	generation, pinned := exec.SkillProjectGeneration()
	if !pinned || skills.ProjectSourceGeneration(generation) != authority.generation {
		return generationChangedError(skills.ErrSkillProjectGenerationChanged)
	}
	return nil
}

// ValidateOwner verifies durable retained-agent ownership without exposing the
// private authority identity to consumers.
func (authority Authority) ValidateOwner(sessionID, sessionProjectDir, projectRoot string, generation skills.ProjectSourceGeneration) error {
	if !authority.enabled || strings.TrimSpace(sessionID) == "" ||
		strings.TrimSpace(sessionID) != authority.identity.SessionID ||
		strings.TrimSpace(sessionProjectDir) == "" ||
		strings.TrimSpace(sessionProjectDir) != authority.identity.SessionProjectDir ||
		generation == 0 || generation != authority.generation {
		return generationChangedError(skills.ErrSkillProjectGenerationChanged)
	}
	ownerRoot, canonical := canonicalProjectRoot(projectRoot)
	if !canonical || ownerRoot != authority.canonicalProjectRoot {
		return generationChangedError(skills.ErrSkillProjectGenerationChanged)
	}
	return nil
}

// Enabled reports whether a manager-backed authority was captured.
func (authority Authority) Enabled() bool { return authority.enabled }

// Snapshot reads the exact session catalog pinned by Authority.
func (authority Authority) Snapshot(manager *skills.Manager) (skills.CatalogSnapshot, error) {
	if manager == nil || !authority.enabled {
		return skills.CatalogSnapshot{}, generationChangedError(skills.ErrSkillProjectGenerationChanged)
	}
	return manager.SnapshotAtGeneration(authority.identity.SessionID, authority.generation)
}

// ResolveLatest resolves against Authority's immutable session and project
// generation; consumer-supplied requests cannot retarget either field.
func (authority Authority) ResolveLatest(manager *skills.Manager, request skills.SkillResolveRequest, consume func(skills.ResolvedSkill) error) (skills.SkillResolveResult, error) {
	if manager == nil || !authority.enabled {
		return skills.SkillResolveResult{}, generationChangedError(skills.ErrSkillProjectGenerationChanged)
	}
	request.SessionID = authority.identity.SessionID
	request.ExpectedProjectGeneration = authority.generation
	return manager.ResolveLatest(request, consume)
}

// WithGenerationLease runs commit while Authority still owns manager's project
// generation. The callback must remain short and must not call back into manager.
func (authority Authority) WithGenerationLease(manager *skills.Manager, commit func() error) error {
	if commit == nil {
		return nil
	}
	if manager == nil {
		return commit()
	}
	if !authority.enabled {
		return generationChangedError(skills.ErrSkillProjectGenerationChanged)
	}
	return manager.WithProjectGenerationLease(authority.generation, commit)
}

func generationChangedError(cause error) error {
	return i18n.WrapInternalError(i18n.KeyLoopQueryValidateSkillGenerationFailed, cause)
}

func canonicalProjectRoot(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", false
	}
	resolved, err := filepath.EvalSymlinks(filepath.Clean(absolute))
	if err != nil {
		return "", false
	}
	resolved, err = filepath.Abs(filepath.Clean(resolved))
	if err != nil {
		return "", false
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", false
	}
	return filepath.Clean(resolved), true
}
