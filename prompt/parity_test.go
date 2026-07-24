package prompt

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/types"
)

const (
	parityTestdataDir = "testdata/parity"
	updateGoldensEnv  = "UPDATE_PROMPT_GOLDENS"
)

type promptParityGolden struct {
	Cases []promptParityCase `json:"cases"`
}

type promptParityCase struct {
	Name   string              `json:"name"`
	Tasks  []string            `json:"tasks"`
	Blocks []goldenPromptBlock `json:"blocks"`
}

type goldenPromptBlock struct {
	Name        string   `json:"name"`
	Source      string   `json:"source,omitempty"`
	Cache       bool     `json:"cache,omitempty"`
	CacheScope  string   `json:"cache_scope,omitempty"`
	Text        string   `json:"text"`
	SHA256      string   `json:"sha256"`
	MustContain []string `json:"must_contain,omitempty"`
}

func TestPromptParityGoldens(t *testing.T) {
	cases := []promptParityCase{
		buildEmptyDirectoryParityCase(t),
		buildGitRepositoryParityCase(t),
		buildMultiLevelMemoryParityCase(t),
		buildRulesParityCase(t),
		buildCustomAppendParityCase(t),
		buildMCPInstructionsParityCase(t),
	}
	assertP0ParityTaskCoverage(t, cases)
	assertPromptParityGolden(t, "prompt_blocks.json", promptParityGolden{Cases: cases})
}

func TestPromptParityContextInjectionGolden(t *testing.T) {
	userContext := UserContextBuilder{
		ClaudeMd: "Project instruction from CLAUDE.md.",
		Date:     time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC),
	}.Build()
	meta, ok := userContext.MetaMessage()
	if !ok {
		t.Fatal("expected user context meta message")
	}
	systemContext := SystemContextBuilder{
		GitStatus: "Current branch: parity\n\nStatus:\n M prompt/parity_test.go",
	}.Build()
	systemBlock, ok := systemContext.Block()
	if !ok {
		t.Fatal("expected system context block")
	}

	golden := promptParityGolden{Cases: []promptParityCase{
		{
			Name:  "user-context-prepend",
			Tasks: []string{"task_03"},
			Blocks: []goldenPromptBlock{snapshotBlock(SystemPromptBlock{
				Name:   "user_context",
				Source: "runtime",
				Text:   meta.GetText(),
			}, []string{
				"<system-reminder>",
				"# claudeMd",
				"# currentDate",
				"</system-reminder>",
			})},
		},
		{
			Name:  "system-context-append",
			Tasks: []string{"task_03", "task_07"},
			Blocks: []goldenPromptBlock{snapshotBlock(systemBlock, []string{
				"gitStatus: Current branch: parity",
				"Status:",
			})},
		},
	}}
	assertPromptParityGolden(t, "context_injection.json", golden)
}

func buildEmptyDirectoryParityCase(t *testing.T) promptParityCase {
	t.Helper()
	t.Setenv("SHELL", "/bin/zsh")
	root := t.TempDir()
	blocks := ApplyCacheScopes(BuildSystemPromptBlocks(parityTools(), Config{
		CWD:              root,
		AdditionalDirs:   []string{filepath.Join(root, "sibling")},
		ModelID:          "claude-opus-4-6",
		ModelDescription: "Claude Opus 4.6",
		KnowledgeCutoff:  "2026-01",
	}), CacheScopeOptions{GlobalSafe: true})
	blocks = normalizeBlocksForGolden(blocks, map[string]string{root: "$EMPTY_DIR"})
	return promptParityCase{
		Name:  "empty-directory-system-blocks",
		Tasks: []string{"task_01", "task_02", "task_08", "task_12"},
		Blocks: snapshotBlocks(blocks, map[string][]string{
			"static": {
				"You are LUBAN Code",
				"# System",
				"# Using your tools",
				"# Output efficiency",
			},
			"dynamic": {
				"You have been invoked in the following environment:",
				"Primary working directory: $EMPTY_DIR",
				"Additional working directories:",
				"Claude Opus 4.6",
			},
		}),
	}
}

func buildGitRepositoryParityCase(t *testing.T) promptParityCase {
	t.Helper()
	root := t.TempDir()
	runGit(t, root, "init", "-b", "main")
	runGit(t, root, "config", "user.name", "Parity Tester")
	runGit(t, root, "config", "user.email", "parity@example.test")
	writeFile(t, filepath.Join(root, "tracked.txt"), "tracked\n")
	runGit(t, root, "add", "tracked.txt")
	runGit(t, root, "commit", "-m", "initial parity fixture")
	writeFile(t, filepath.Join(root, "tracked.txt"), "tracked\nchanged\n")
	writeFile(t, filepath.Join(root, "untracked.txt"), "untracked\n")

	gitStatus := LoadGitContextWithOptions(GitContextOptions{CWD: root, Timeout: 2 * time.Second})
	if strings.TrimSpace(gitStatus) == "" {
		t.Skip("git executable unavailable or fixture repo could not be inspected")
	}
	gitStatus = normalizeGitContextForGolden(gitStatus)
	blocks := SystemContextBuilder{GitStatus: gitStatus}.Build().AppendTo(nil)
	blocks = normalizeBlocksForGolden(blocks, map[string]string{root: "$GIT_REPO"})
	return promptParityCase{
		Name:  "git-repository-system-context",
		Tasks: []string{"task_03", "task_07", "task_12"},
		Blocks: snapshotBlocks(blocks, map[string][]string{
			"system_context": {
				"gitStatus: This is the git status at the start of the conversation.",
				"Current branch: main",
				"Git user: Parity Tester",
				"Status:",
				"Recent commits:",
			},
		}),
	}
}

func buildMultiLevelMemoryParityCase(t *testing.T) promptParityCase {
	t.Helper()
	root := copyFixtureToTemp(t, "memory_tree")
	project := filepath.Join(root, "project")
	leaf := filepath.Join(project, "child", "leaf")
	user := filepath.Join(root, "user")
	files := discoverMemoryFiles(leaf, memoryPaths{userDir: user})
	text := normalizeTextForGolden(FormatMemoryFiles(files), map[string]string{root: "$MEMORY_TREE"})
	return promptParityCase{
		Name:  "multi-level-memory-files",
		Tasks: []string{"task_03", "task_04", "task_12"},
		Blocks: []goldenPromptBlock{snapshotBlock(SystemPromptBlock{
			Name:   "memory",
			Source: "runtime",
			Text:   text,
		}, []string{
			memoryInstructionPrompt,
			"Contents of $MEMORY_TREE/user/CLAUDE.md",
			"Contents of $MEMORY_TREE/project/CLAUDE.md",
			"Contents of $MEMORY_TREE/project/.claude/CLAUDE.md",
			"Contents of $MEMORY_TREE/project/child/CLAUDE.md",
			"Contents of $MEMORY_TREE/project/child/leaf/CLAUDE.local.md",
		})},
	}
}

func buildRulesParityCase(t *testing.T) promptParityCase {
	t.Helper()
	root := t.TempDir()
	project := filepath.Join(root, "project")
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(root, "claude-user"))
	t.Setenv("DEEPSEEK_CODE_CONFIG_DIR", filepath.Join(root, "deepseek-user"))
	t.Setenv("LUBAN_CODE_CONFIG_DIR", filepath.Join(root, "luban-user"))
	t.Setenv("USER_TYPE", "ant")
	t.Setenv("CLAUDE_CODE_MANAGED_SETTINGS_PATH", filepath.Join(root, "managed"))
	rules := filepath.Join(project, ".claude", "rules")
	writeFile(t, filepath.Join(rules, "01-base.md"), "Base rule fixture.\n\n@./base-extra.txt\n")
	writeFile(t, filepath.Join(rules, "base-extra.txt"), "Base rule include.")
	writeFile(t, filepath.Join(rules, "02-go.md"), "---\npaths: src/**/*.go\n---\nGo rule fixture.\n\n@./go-extra.txt\n")
	writeFile(t, filepath.Join(rules, "go-extra.txt"), "Go rule include.")
	writeFile(t, filepath.Join(rules, "nested", "03-all.md"), "---\npaths: **\n---\nNested match-all conditional rule.")
	target := filepath.Join(project, "src", "main.go")
	files := DiscoverMemoryFilesForTarget(project, target)
	text := normalizeTextForGolden(FormatMemoryFiles(files), map[string]string{root: "$RULES_TREE"})
	return promptParityCase{
		Name:  "rules-and-frontmatter-paths",
		Tasks: []string{"task_04", "task_05", "task_12"},
		Blocks: []goldenPromptBlock{snapshotBlock(SystemPromptBlock{
			Name:   "memory_rules",
			Source: "runtime",
			Text:   text,
		}, []string{
			"Contents of $RULES_TREE/project/.claude/rules/01-base.md",
			"Base rule fixture.",
			"Contents of $RULES_TREE/project/.claude/rules/base-extra.txt",
			"Contents of $RULES_TREE/project/.claude/rules/02-go.md",
			"Go rule fixture.",
			"Contents of $RULES_TREE/project/.claude/rules/go-extra.txt",
			"Nested match-all conditional rule.",
		})},
	}
}

func buildCustomAppendParityCase(t *testing.T) promptParityCase {
	t.Helper()
	defaultPrompt := SystemPrompt{
		{Text: "default static", Source: "built_in", Name: "static", Cache: true, CacheScope: CacheScopeEphemeral},
		{Text: "default dynamic", Source: "runtime", Name: "dynamic"},
	}
	blocks := BuildEffectiveSystemPrompt(EffectiveSystemPromptInput{
		Custom:  "Custom prompt fixture.",
		Default: defaultPrompt,
		Append:  "Append prompt fixture.",
	})
	return promptParityCase{
		Name:  "custom-and-append-prompt-precedence",
		Tasks: []string{"task_06", "task_12"},
		Blocks: snapshotBlocks(blocks, map[string][]string{
			"custom": {"Custom prompt fixture."},
			"append": {"Append prompt fixture."},
		}),
	}
}

func buildMCPInstructionsParityCase(t *testing.T) promptParityCase {
	t.Helper()
	fixture := filepath.Join(parityTestdataDir, "fixtures", "mcp_instructions", "servers.txt")
	data, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	text := renderMCPInstructionsFixture(string(data))
	return promptParityCase{
		Name:  "mcp-instructions-section",
		Tasks: []string{"task_12"},
		Blocks: []goldenPromptBlock{snapshotBlock(SystemPromptBlock{
			Name:   "mcp_instructions",
			Source: "runtime",
			Text:   text,
		}, []string{
			"# MCP Server Instructions",
			"## filesystem",
			"Prefer the Read and Write tools",
			"## postgres",
			"Query only read-only replicas",
		})},
	}
}

func snapshotBlocks(blocks []SystemPromptBlock, contains map[string][]string) []goldenPromptBlock {
	out := make([]goldenPromptBlock, 0, len(blocks))
	for _, block := range blocks {
		out = append(out, snapshotBlock(block, contains[block.Name]))
	}
	return out
}

func snapshotBlock(block SystemPromptBlock, contains []string) goldenPromptBlock {
	return goldenPromptBlock{
		Name:        block.Name,
		Source:      block.Source,
		Cache:       block.Cache,
		CacheScope:  block.CacheScope,
		Text:        block.Text,
		SHA256:      testSHA256Hex(block.Text),
		MustContain: contains,
	}
}

func assertPromptParityGolden(t *testing.T, filename string, got promptParityGolden) {
	t.Helper()
	for i := range got.Cases {
		for j := range got.Cases[i].Blocks {
			block := &got.Cases[i].Blocks[j]
			block.SHA256 = testSHA256Hex(block.Text)
			for _, want := range block.MustContain {
				if !strings.Contains(block.Text, want) {
					t.Fatalf("case %q block %q field text missing required substring %q", got.Cases[i].Name, block.Name, want)
				}
			}
		}
	}

	path := filepath.Join(parityTestdataDir, "golden", filename)
	if os.Getenv(updateGoldensEnv) != "" {
		writeGolden(t, path, got)
		return
	}

	var want promptParityGolden
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v\nRun %s=1 go test ./prompt to intentionally create/update it.", path, err, updateGoldensEnv)
	}
	if err := json.Unmarshal(data, &want); err != nil {
		t.Fatalf("parse golden %s: %v", path, err)
	}
	if diff := diffPromptParityGolden(want, got); diff != "" {
		t.Fatalf("prompt parity golden drift in %s:\n%s\nRun %s=1 go test ./prompt only after reviewing the prompt change.", path, diff, updateGoldensEnv)
	}
}

func diffPromptParityGolden(want, got promptParityGolden) string {
	if len(want.Cases) != len(got.Cases) {
		return fmt.Sprintf("case count: want %d got %d", len(want.Cases), len(got.Cases))
	}
	for i := range want.Cases {
		wc, gc := want.Cases[i], got.Cases[i]
		caseName := wc.Name
		if caseName == "" {
			caseName = gc.Name
		}
		if wc.Name != gc.Name {
			return fmt.Sprintf("case[%d] field name: want %q got %q", i, wc.Name, gc.Name)
		}
		if !reflect.DeepEqual(wc.Tasks, gc.Tasks) {
			return fmt.Sprintf("case %q field tasks: want %#v got %#v", caseName, wc.Tasks, gc.Tasks)
		}
		if len(wc.Blocks) != len(gc.Blocks) {
			return fmt.Sprintf("case %q block count: want %d got %d", caseName, len(wc.Blocks), len(gc.Blocks))
		}
		for j := range wc.Blocks {
			if diff := diffPromptBlock(caseName, j, wc.Blocks[j], gc.Blocks[j]); diff != "" {
				return diff
			}
		}
	}
	return ""
}

func diffPromptBlock(caseName string, index int, want, got goldenPromptBlock) string {
	blockName := want.Name
	if blockName == "" {
		blockName = got.Name
	}
	prefix := fmt.Sprintf("case %q block[%d] %q", caseName, index, blockName)
	if want.Name != got.Name {
		return fmt.Sprintf("%s field name: want %q got %q", prefix, want.Name, got.Name)
	}
	if want.Source != got.Source {
		return fmt.Sprintf("%s field source: want %q got %q", prefix, want.Source, got.Source)
	}
	if want.Cache != got.Cache {
		return fmt.Sprintf("%s field cache: want %v got %v", prefix, want.Cache, got.Cache)
	}
	if want.CacheScope != got.CacheScope {
		return fmt.Sprintf("%s field cache_scope: want %q got %q", prefix, want.CacheScope, got.CacheScope)
	}
	if want.SHA256 != got.SHA256 {
		return fmt.Sprintf("%s field text_sha256: want %s got %s\n%s", prefix, want.SHA256, got.SHA256, textMismatchSnippet(want.Text, got.Text))
	}
	if want.Text != got.Text {
		return fmt.Sprintf("%s field text: sha matched but text differs\n%s", prefix, textMismatchSnippet(want.Text, got.Text))
	}
	if !reflect.DeepEqual(want.MustContain, got.MustContain) {
		return fmt.Sprintf("%s field must_contain: want %#v got %#v", prefix, want.MustContain, got.MustContain)
	}
	return ""
}

func textMismatchSnippet(want, got string) string {
	wantLines := strings.Split(want, "\n")
	gotLines := strings.Split(got, "\n")
	limit := len(wantLines)
	if len(gotLines) > limit {
		limit = len(gotLines)
	}
	for i := 0; i < limit; i++ {
		var wl, gl string
		if i < len(wantLines) {
			wl = wantLines[i]
		}
		if i < len(gotLines) {
			gl = gotLines[i]
		}
		if wl != gl {
			return fmt.Sprintf("first differing line %d:\n- %s\n+ %s", i+1, wl, gl)
		}
	}
	return "text length differs"
}

func writeGolden(t *testing.T, path string, golden promptParityGolden) {
	t.Helper()
	data, err := json.MarshalIndent(golden, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertP0ParityTaskCoverage(t *testing.T, cases []promptParityCase) {
	t.Helper()
	taskFiles, err := filepath.Glob(filepath.Join("parity_tasks", "task_*.json"))
	if err != nil {
		t.Fatal(err)
	}
	covered := make(map[string]bool)
	for _, tc := range cases {
		for _, task := range tc.Tasks {
			covered[task] = true
		}
	}
	for _, taskFile := range taskFiles {
		data, err := os.ReadFile(taskFile)
		if err != nil {
			t.Fatal(err)
		}
		var meta struct {
			ID       string `json:"id"`
			Priority string `json:"priority"`
		}
		if err := json.Unmarshal(data, &meta); err != nil {
			t.Fatal(err)
		}
		if meta.Priority == "P0" && !covered[meta.ID] {
			t.Fatalf("P0 prompt parity task %s has no golden assertion coverage", meta.ID)
		}
	}
}

func copyFixtureToTemp(t *testing.T, name string) string {
	t.Helper()
	src := filepath.Join(parityTestdataDir, "fixtures", name)
	dst := filepath.Join(t.TempDir(), name)
	copyDir(t, src, dst)
	return dst
}

func copyDir(t *testing.T, src, dst string) {
	t.Helper()
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			copyDir(t, srcPath, dstPath)
			continue
		}
		data, err := os.ReadFile(srcPath)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dstPath, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func normalizeBlocksForGolden(blocks []SystemPromptBlock, replacements map[string]string) []SystemPromptBlock {
	out := make([]SystemPromptBlock, len(blocks))
	for i, block := range blocks {
		block.Text = normalizeTextForGolden(block.Text, replacements)
		out[i] = block
	}
	return out
}

func normalizeTextForGolden(text string, replacements map[string]string) string {
	type replacement struct {
		from string
		to   string
	}
	var ordered []replacement
	for from, to := range replacements {
		ordered = append(ordered, replacement{from: filepath.ToSlash(from), to: to})
		ordered = append(ordered, replacement{from: from, to: to})
	}
	sort.Slice(ordered, func(i, j int) bool {
		return len(ordered[i].from) > len(ordered[j].from)
	})
	text = filepath.ToSlash(text)
	for _, r := range ordered {
		text = strings.ReplaceAll(text, filepath.ToSlash(r.from), r.to)
	}
	text = platformLineRe.ReplaceAllString(text, " - Platform: $$PLATFORM")
	text = osVersionLineRe.ReplaceAllString(text, " - OS version: $$OS_VERSION")
	return text
}

var (
	recentCommitLineRe = regexp.MustCompile(`(?m)^[0-9a-f]{7,40} initial parity fixture$`)
	platformLineRe     = regexp.MustCompile(`(?m)^ - Platform: .+$`)
	osVersionLineRe    = regexp.MustCompile(`(?m)^ - OS version: .+$`)
)

func normalizeGitContextForGolden(text string) string {
	return recentCommitLineRe.ReplaceAllString(text, "$COMMIT initial parity fixture")
}

func testSHA256Hex(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func parityTools() []types.Tool {
	return []types.Tool{
		&mockTool{name: "Bash", desc: "Run commands"},
		&mockTool{name: "Read", desc: "Read files"},
		&mockTool{name: "Edit", desc: "Edit files"},
		&mockTool{name: "Write", desc: "Write files"},
		&mockTool{name: "Glob", desc: "Find files"},
		&mockTool{name: "Grep", desc: "Search files"},
		&mockTool{name: "TodoWrite", desc: "Track todos"},
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func renderMCPInstructionsFixture(raw string) string {
	var blocks []string
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		name, instructions, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		blocks = append(blocks, "## "+strings.TrimSpace(name)+"\n"+strings.TrimSpace(instructions))
	}
	if len(blocks) == 0 {
		return ""
	}
	return "# MCP Server Instructions\n\nThe following MCP servers have provided instructions for how to use their tools and resources:\n\n" + strings.Join(blocks, "\n\n")
}
