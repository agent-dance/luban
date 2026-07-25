package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/agent-dance/luban/brand"
	"github.com/agent-dance/luban/internal/ui/theme"
)

// configureTerminalTheme reads the existing "theme" setting with the same
// precedence as other startup settings. Every terminal surface consumes the
// resulting shared palette.
func configureTerminalTheme(cwd string) error {
	paths := make([]string, 0, 3)
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		paths = append(paths, filepath.Join(home, brand.ConfigDirName, "settings.json"))
	}
	if cwd != "" {
		paths = append(paths,
			filepath.Join(cwd, brand.ConfigDirName, "settings.json"),
			filepath.Join(cwd, brand.ConfigDirName, "settings.local.json"),
		)
	}

	name := ""
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		var settings map[string]any
		if err := json.Unmarshal(data, &settings); err != nil {
			return err
		}
		if value, ok := settings["theme"].(string); ok && strings.TrimSpace(value) != "" {
			name = value
		}
	}
	theme.Configure(name)
	return nil
}
