package search

import (
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/agent-dance/luban/types"
)

type testRuntimeScope struct {
	projectRoot string
	allowedDirs []string
	deniedRules []types.PermissionRuleValue
}

func NewRuntimeScope(projectRoot string, _ bool) *testRuntimeScope {
	return &testRuntimeScope{projectRoot: projectRoot}
}

func (s *testRuntimeScope) SetAllowedDirs(dirs []string) {
	s.allowedDirs = append([]string(nil), dirs...)
}

func (s *testRuntimeScope) SetDeniedTools(specs []string) {
	s.deniedRules = s.deniedRules[:0]
	for _, spec := range specs {
		open := strings.IndexByte(spec, '(')
		if open <= 0 || !strings.HasSuffix(spec, ")") {
			continue
		}
		s.deniedRules = append(s.deniedRules, types.PermissionRuleValue{
			ToolName:    strings.TrimSpace(spec[:open]),
			RuleContent: strings.TrimSpace(spec[open+1 : len(spec)-1]),
		})
	}
}

func (s *testRuntimeScope) ToolRuntimeContext() types.ToolRuntimeContext {
	return types.ToolRuntimeContext{
		ProjectRoot: s.projectRoot,
		AllowedDirs: append([]string(nil), s.allowedDirs...),
		DeniedRules: append([]types.PermissionRuleValue(nil), s.deniedRules...),
	}
}

func resetRipgrepLocationForTest() {
	locateRipgrepMu.Lock()
	defer locateRipgrepMu.Unlock()
	locateRipgrepOnce = sync.Once{}
	locatedRipgrepPath = ""
	locatedRipgrepErr = nil
	codesignOnce = sync.Once{}
	codesignErr = nil
}

func withUnavailableRipgrep(t testing.TB) {
	t.Helper()
	t.Setenv("LUBAN_RG_PATH", filepath.Join(t.TempDir(), "missing-rg"))
	resetRipgrepLocationForTest()
	t.Cleanup(resetRipgrepLocationForTest)
}
