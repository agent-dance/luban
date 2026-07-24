package swarm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/agent-dance/luban/brand"
)

// TeamConfig holds the persisted state of a swarm team.
type TeamConfig struct {
	Name          string       `json:"name"`
	Description   string       `json:"description,omitempty"`
	CreatedAt     int64        `json:"createdAt"`
	LeadAgentID   string       `json:"leadAgentId"`
	LeadSessionID string       `json:"leadSessionId,omitempty"`
	Members       []TeamMember `json:"members"`
	Revision      uint64       `json:"revision,omitempty"`
}

// TeamMember describes a single teammate in the swarm.
type TeamMember struct {
	AgentID       string   `json:"agentId"`
	Name          string   `json:"name"`
	AgentType     string   `json:"agentType,omitempty"`
	Model         string   `json:"model,omitempty"`
	JoinedAt      int64    `json:"joinedAt,omitempty"`
	Color         string   `json:"color,omitempty"`
	TmuxPaneID    string   `json:"tmuxPaneId"`
	BackendType   string   `json:"backendType,omitempty"`
	CWD           string   `json:"cwd"`
	WorktreePath  string   `json:"worktreePath,omitempty"`
	Subscriptions []string `json:"subscriptions"`
	IsActive      bool     `json:"isActive"`
	SpawnID       string   `json:"spawnId,omitempty"`
	Lifecycle     string   `json:"lifecycle,omitempty"`
}

var ErrTeamConfigExists = errors.New("team config already exists")

// TeamConfigPath returns ~/.luban-code/teams/{teamName}/team.json.
func TeamConfigPath(teamName string) (string, error) {
	if err := validateName(teamName, "team name"); err != nil {
		return "", err
	}
	home, err := userHomeDir()
	if err != nil {
		return "", fmt.Errorf("team config path: %w", err)
	}
	return filepath.Join(home, brand.ConfigDirName, "teams", teamName, "team.json"), nil
}

// TeamDir returns ~/.luban-code/teams/{teamName} after validating that the
// name cannot escape the teams root.
func TeamDir(teamName string) (string, error) {
	path, err := TeamConfigPath(teamName)
	if err != nil {
		return "", err
	}
	return filepath.Dir(path), nil
}

func userHomeDir() (string, error) {
	if home := os.Getenv("HOME"); home != "" {
		return home, nil
	}
	return os.UserHomeDir()
}

// LoadTeamConfig reads a team config from disk under an exclusive file lock
// to prevent reading a partially-written file during a concurrent save.
func LoadTeamConfig(teamName string) (*TeamConfig, error) {
	path, err := TeamConfigPath(teamName)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	unlock, err := lockFile(ctx, path+".lock")
	if err != nil {
		return nil, fmt.Errorf("load team config: lock: %w", err)
	}
	defer unlock()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("team %q not found", teamName)
		}
		return nil, fmt.Errorf("load team config: %w", err)
	}

	var cfg TeamConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("load team config: decode: %w", err)
	}
	return &cfg, nil
}

// SaveTeamConfig writes a team config to disk atomically under an exclusive
// file lock so concurrent savers do not corrupt the file.
func SaveTeamConfig(cfg *TeamConfig) error {
	if cfg == nil {
		return fmt.Errorf("save team config: nil config")
	}
	return SaveTeamConfigAs(cfg.Name, cfg)
}

// SaveTeamConfigAs writes cfg under the provided storage team name. This is
// used when the on-disk directory is a slug but cfg.Name preserves the display
// team name for TS-compatible readers.
func SaveTeamConfigAs(teamName string, cfg *TeamConfig) error {
	if cfg == nil {
		return fmt.Errorf("save team config: nil config")
	}
	path, err := TeamConfigPath(teamName)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("save team config: mkdir: %w", err)
	}

	ctx := context.Background()
	unlock, err := lockFile(ctx, path+".lock")
	if err != nil {
		return fmt.Errorf("save team config: lock: %w", err)
	}
	defer unlock()

	if currentData, readErr := os.ReadFile(path); readErr == nil {
		var current TeamConfig
		if json.Unmarshal(currentData, &current) == nil && cfg.Revision <= current.Revision {
			cfg.Revision = current.Revision + 1
		}
	} else if os.IsNotExist(readErr) && cfg.Revision == 0 {
		cfg.Revision = 1
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("save team config: encode: %w", err)
	}

	return atomicWrite(path, data)
}

// CreateTeamConfigAs publishes a new team only when no config already exists.
// The existence check and atomic write share the same durable lock domain.
func CreateTeamConfigAs(teamName string, cfg *TeamConfig) error {
	if cfg == nil {
		return fmt.Errorf("create team config: nil config")
	}
	path, err := TeamConfigPath(teamName)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create team config: mkdir: %w", err)
	}
	unlock, err := lockFile(context.Background(), path+".lock")
	if err != nil {
		return fmt.Errorf("create team config: lock: %w", err)
	}
	defer unlock()
	if _, err := os.Lstat(path); err == nil {
		return fmt.Errorf("create team config: %w: %s", ErrTeamConfigExists, teamName)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("create team config: inspect: %w", err)
	}
	copyConfig := cloneTeamConfig(cfg)
	copyConfig.Revision = 1
	data, err := json.MarshalIndent(copyConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("create team config: encode: %w", err)
	}
	if err := atomicWrite(path, data); err != nil {
		return fmt.Errorf("create team config: write: %w", err)
	}
	*cfg = *copyConfig
	return nil
}

// UpdateTeamConfig performs a locked read-modify-write and increments the
// durable revision. The mutator must not retain the supplied pointer.
func UpdateTeamConfig(ctx context.Context, teamName string, mutate func(*TeamConfig) error) (*TeamConfig, error) {
	if mutate == nil {
		return nil, fmt.Errorf("update team config: nil mutator")
	}
	path, err := TeamConfigPath(teamName)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("update team config: mkdir: %w", err)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	unlock, err := lockFile(ctx, path+".lock")
	if err != nil {
		return nil, fmt.Errorf("update team config: lock: %w", err)
	}
	defer unlock()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("update team config: read: %w", err)
	}
	var current TeamConfig
	if err := json.Unmarshal(data, &current); err != nil {
		return nil, fmt.Errorf("update team config: decode: %w", err)
	}
	next := cloneTeamConfig(&current)
	if err := mutate(next); err != nil {
		return nil, err
	}
	next.Revision = current.Revision + 1
	encoded, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("update team config: encode: %w", err)
	}
	if err := atomicWrite(path, encoded); err != nil {
		return nil, fmt.Errorf("update team config: write: %w", err)
	}
	return cloneTeamConfig(next), nil
}

func cloneTeamConfig(cfg *TeamConfig) *TeamConfig {
	if cfg == nil {
		return nil
	}
	cloned := *cfg
	cloned.Members = append([]TeamMember(nil), cfg.Members...)
	for index := range cloned.Members {
		if cfg.Members[index].Subscriptions != nil {
			cloned.Members[index].Subscriptions = append([]string{}, cfg.Members[index].Subscriptions...)
		}
	}
	return &cloned
}

// DeleteTeamConfig removes the team config while retaining the stable lock
// inode used to serialize mixed readers, writers, and deletion retries.
func DeleteTeamConfig(teamName string) error {
	path, err := TeamConfigPath(teamName)
	if err != nil {
		return err
	}

	unlock, err := lockFile(context.Background(), path+".lock")
	if err != nil {
		return fmt.Errorf("delete team config: lock: %w", err)
	}
	defer unlock()
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete team config: %w", err)
	}
	return nil
}

// DeleteTeamDirectory recursively removes all durable state for a team,
// including config, locks, inboxes, and other team-local files. It is a no-op
// when the directory is already gone.
func DeleteTeamDirectory(teamName string) error {
	dir, err := TeamDir(teamName)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("delete team directory: %w", err)
	}
	return nil
}
