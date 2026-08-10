package tui

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

func TestCronPresentationLocalizesStableMetadataFallbacks(t *testing.T) {
	useDetailStoreTestLanguage(t, i18n.LangZH)
	result := &types.ToolResultBlock{Metadata: map[string]string{"next_fire": "unknown", "tz": "Local"}}
	formatted := FormatToolPresentation("CronCreate", nil, OutcomeSucceeded, result)
	if !strings.Contains(formatted.Summary, i18n.Text(i18n.LangZH, i18n.KeyPresentationCronNextUnknown)) ||
		!strings.Contains(formatted.Summary, i18n.Text(i18n.LangZH, i18n.KeyPresentationCronTimezoneLocal)) {
		t.Fatalf("cron fallback metadata was not localized: %q", formatted.Summary)
	}
}

func TestCronPresentationReadsOnlyCanonicalScheduleFields(t *testing.T) {
	useDetailStoreTestLanguage(t, i18n.LangEN)
	result := &types.ToolResultBlock{
		Data: map[string]any{
			"id":            "schedule-4",
			"humanSchedule": "legacy human schedule",
			"nextFire":      "legacy next fire",
			"timezone":      "legacy timezone",
		},
		Metadata: map[string]string{
			"humanSchedule":  "legacy metadata schedule",
			"human_schedule": "legacy snake schedule",
			"nextFire":       "legacy metadata next fire",
			"timezone":       "legacy metadata timezone",
			"next_fire":      "2026-07-26T01:00:00Z",
			"tz":             "UTC",
		},
	}
	formatted := FormatToolPresentation("CronCreate", map[string]any{"cron": "0 9 * * *"}, OutcomeSucceeded, result)
	for _, want := range []string{"schedule-4", "0 9 * * *", "2026-07-26T01:00:00Z", "UTC"} {
		if !strings.Contains(formatted.Summary, want) {
			t.Fatalf("canonical cron field %q missing from %q", want, formatted.Summary)
		}
	}
	for _, legacy := range []string{"legacy human schedule", "legacy metadata schedule", "legacy snake schedule", "legacy next fire", "legacy metadata next fire", "legacy timezone", "legacy metadata timezone"} {
		if strings.Contains(formatted.Summary, legacy) {
			t.Fatalf("legacy cron field %q leaked into %q", legacy, formatted.Summary)
		}
	}
}

var productionToolFamiliesForPresentationTest = map[string]CommandFamily{
	"Bash": FamilyShell, "PowerShell": FamilyShell,
	"Read": FamilyFileRead, "Write": FamilyFileWrite, "Edit": FamilyFileWrite, "NotebookEdit": FamilyFileWrite,
	"Glob": FamilySearch, "Grep": FamilySearch, "LSP": FamilySearch, "ToolSearch": FamilySearch,
	"WebFetch": FamilyWeb, "WebSearch": FamilyWeb,
	"ListMcpResourcesTool": FamilyMCP, "ReadMcpResourceTool": FamilyMCP,
	"Agent":      FamilyAgent,
	"TaskCreate": FamilyTask, "TaskList": FamilyTask, "TaskUpdate": FamilyTask, "TaskGet": FamilyTask,
	"TaskStop": FamilyTask, "TaskOutput": FamilyTask,
	"GetGoal": FamilyGoal, "CreateGoal": FamilyGoal, "UpdateGoal": FamilyGoal,
	"EnterPlanMode": FamilyDecision, "ExitPlanMode": FamilyDecision, "AskUserQuestion": FamilyDecision,
	"SendUserMessage": FamilyMessage, "SendMessage": FamilyMessage,
	"TeamCreate": FamilyTeam, "TeamDelete": FamilyTeam,
	"CronCreate": FamilyCron, "CronDelete": FamilyCron, "CronList": FamilyCron,
	"EnterWorktree": FamilyWorktree, "ExitWorktree": FamilyWorktree,
	"Config": FamilyConfig, "Skill": FamilySkill, "RemoteTrigger": FamilyRemote,
}

func TestProductionToolFamilyCoverage(t *testing.T) {
	production := productionToolFamiliesForPresentationTest

	if len(production) != 39 {
		t.Fatalf("production catalog has %d tools, want 39", len(production))
	}
	for name, family := range production {
		if got := CommandFamilyForTool(name); got != family {
			t.Errorf("CommandFamilyForTool(%q) = %q, want %q", name, got, family)
		}
	}
	for _, name := range []string{"mcp__github__search", "server_tool_web_search"} {
		if got := CommandFamilyForTool(name); got != FamilyMCP {
			t.Errorf("dynamic MCP family for %q = %q, want %q", name, got, FamilyMCP)
		}
	}
	if got := CommandFamilyForTool("FutureTool"); got != FamilyUnknown {
		t.Fatalf("unknown family = %q, want %q", got, FamilyUnknown)
	}
}

func TestEveryProductionFormatterCoversLifecycleWarningFailureAndExpansion(t *testing.T) {
	for toolName := range productionToolFamiliesForPresentationTest {
		t.Run(toolName, func(t *testing.T) {
			input := map[string]any{"file_path": "/workspace/item", "command": "true", "action": "get"}
			running := FormatToolPresentation(toolName, input, OutcomeRunning, nil)
			if running.Lifecycle != PresentationLifecycleRunning || running.Summary == "" {
				t.Fatalf("running contract=%+v", running)
			}
			success := FormatToolPresentation(toolName, input, OutcomeSucceeded, &types.ToolResultBlock{Outcome: types.ToolOutcomeSucceeded, Data: map[string]any{"success": true}})
			if success.Lifecycle != PresentationLifecycleCompleted || success.Summary == "" {
				t.Fatalf("success contract=%+v", success)
			}
			warning := FormatToolPresentation(toolName, input, OutcomeSucceeded, &types.ToolResultBlock{Outcome: types.ToolOutcomeSucceeded, Metadata: map[string]string{"warning": "true"}})
			if decision := DecidePresentation(warning.Facts(warning.Outcome)); !warning.Warning || decision.EffectiveLevel < PresentationStructured {
				t.Fatalf("warning contract=%+v decision=%+v", warning, decision)
			}
			failure := FormatToolPresentation(toolName, input, OutcomeFailed, &types.ToolResultBlock{Outcome: types.ToolOutcomeFailed, IsError: true, Content: "structured failure"})
			if decision := DecidePresentation(failure.Facts(failure.Outcome)); decision.EffectiveLevel < PresentationStructured {
				t.Fatalf("failure contract=%+v decision=%+v", failure, decision)
			}
			facts := success.Facts(success.Outcome)
			facts.Intent.Inspect = true
			if decision := DecidePresentation(facts); decision.EffectiveLevel < PresentationStructured {
				t.Fatalf("inspect contract=%+v", decision)
			}
			facts.Intent.Full = true
			if decision := DecidePresentation(facts); decision.EffectiveLevel != PresentationEvidence {
				t.Fatalf("full contract=%+v", decision)
			}
		})
	}
}

func TestFormatToolPresentationUsesRealStructuredResults(t *testing.T) {
	tests := []struct {
		name        string
		tool        string
		input       map[string]any
		data        any
		sideEffect  bool
		needsReview bool
		want        []string
	}{
		{
			name: "read discriminated union", tool: "Read",
			input: map[string]any{"file_path": "/workspace/main.go"},
			data: map[string]any{
				"type": "text",
				"file": map[string]any{"filePath": "/workspace/main.go", "numLines": 42, "startLine": 10, "totalLines": 100},
			},
			want: []string{"Read", "/workspace/main.go", "42 lines", "window 10..51/100"},
		},
		{
			name: "write receipt", tool: "Write",
			input:      map[string]any{"file_path": "/tmp/out.txt"},
			data:       map[string]any{"type": "create", "filePath": "/tmp/out.txt", "gitDiff": map[string]any{"additions": 5, "deletions": 2}},
			sideEffect: true,
			want:       []string{"Created", "/tmp/out.txt", "+5", "-2"},
		},
		{
			name: "grep counts", tool: "Grep",
			input: map[string]any{"pattern": "TODO", "path": "/workspace"},
			data:  map[string]any{"mode": "content", "numMatches": 9, "numFiles": 3, "filenames": []string{"a.go", "b.go", "c.go"}},
			want:  []string{"Grep", "TODO", "9 matches", "3 files"},
		},
		{
			name: "web fetch receipt", tool: "WebFetch",
			input: map[string]any{"url": "https://example.com", "prompt": "summarize"},
			data:  map[string]any{"code": 200, "codeText": "OK", "bytes": 4096, "durationMs": 1250, "url": "https://example.com", "result": "done"},
			want:  []string{"WebFetch", "https://example.com", "200", "4.0 KiB", "1.2s"},
		},
		{
			name: "agent completion metrics", tool: "Agent",
			input:       map[string]any{"description": "verify auth"},
			data:        map[string]any{"kind": "completed", "agentId": "agent-7", "agentType": "verifier", "durationMs": 2500, "totalTokens": 1200, "totalToolUseCount": 4, "transcriptPath": "/tmp/agent.jsonl"},
			needsReview: true,
			want:        []string{"Agent", "agent-7", "verifier", "4 tools", "1,200 tokens", "2.5s"},
		},
		{
			name: "nested task", tool: "TaskCreate",
			data:       map[string]any{"task": map[string]any{"id": "17", "subject": "Audit display"}},
			sideEffect: true,
			want:       []string{"TaskCreate", "17", "Audit display"},
		},
		{
			name: "task list status groups", tool: "TaskList",
			data: map[string]any{"tasks": []any{
				map[string]any{"id": "1", "status": "in_progress"},
				map[string]any{"id": "2", "status": "pending"},
				map[string]any{"id": "3", "status": "pending"},
			}},
			want: []string{"TaskList", "1 active", "2 pending"},
		},
		{
			name: "nested goal", tool: "CreateGoal",
			data: map[string]any{"goal": map[string]any{
				"objective": "Finish presentation policy", "status": "active", "tokens_used": 321,
				"acceptance_criteria": []any{
					map[string]any{"id": "AC-1", "text": "tests pass"},
					map[string]any{"id": "AC-2", "text": "copy is localized"},
				},
			}},
			sideEffect:  true,
			needsReview: true,
			want:        []string{"CreateGoal", "Finish presentation policy", "active", "2 acceptance criteria", "321 tokens"},
		},
		{
			name: "goal acceptance progress", tool: "GetGoal",
			data: map[string]any{"goal": map[string]any{
				"objective": "Finish presentation policy", "status": "active",
				"acceptance_criteria": []any{
					map[string]any{"id": "AC-1", "text": "tests pass"},
					map[string]any{"id": "AC-2", "text": "copy is localized"},
				},
				"last_acceptance_evaluation": map[string]any{"criteria": []any{
					map[string]any{"criterion_id": "AC-1", "met": true, "reason": "passed"},
					map[string]any{"criterion_id": "AC-2", "met": false, "reason": "missing"},
				}},
			}},
			want: []string{"GetGoal", "Finish presentation policy", "active", "1/2 criteria met"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatToolPresentation(tt.tool, tt.input, OutcomeSucceeded, &types.ToolResultBlock{
				Outcome: types.ToolOutcomeSucceeded,
				Data:    tt.data,
			})
			if got.Outcome != OutcomeSucceeded || got.SideEffect != tt.sideEffect || got.NeedsReview != tt.needsReview {
				t.Fatalf("flags/outcome = %+v", got)
			}
			presentation := got.Summary + "\n" + strings.Join(got.DetailLines, "\n")
			for _, want := range tt.want {
				if want == tt.tool {
					want = semanticToolActionInLanguage(i18n.LangEN, tt.tool)
					if got.Family == FamilyFileRead {
						want = i18n.Text(i18n.LangEN, i18n.KeyPresentationAggregateRead)
					} else if got.Family == FamilyAgent {
						want = i18n.Text(i18n.LangEN, i18n.KeyPresentationAgent)
					}
				}
				if !strings.Contains(presentation, want) {
					t.Fatalf("presentation missing %q: %+v", want, got)
				}
			}
		})
	}
}

// TestEveryProductionFormatterUsesToolSpecificStructuredFields is deliberately
// broader than a lifecycle smoke test: every registered production tool gets a
// fixture shaped like its actual input/result contract and must surface the
// fields that let a person identify the target, outcome, and relevant receipt.
func TestEveryProductionFormatterUsesToolSpecificStructuredFields(t *testing.T) {
	tests := map[string]struct {
		input    map[string]any
		data     any
		content  string
		blocks   []types.ContentBlock
		metadata map[string]string
		want     []string
	}{
		"Bash":         {input: map[string]any{"description": "run unit tests", "command": "go test ./..."}, data: map[string]any{"stdout": "ok", "stderr": "", "exitCode": 0, "rawOutputPath": "/tmp/bash-7.log", "interrupted": false}, want: []string{"Bash", "run unit tests", "completed", "exit 0", "Evidence ref: /tmp/bash-7.log"}},
		"PowerShell":   {input: map[string]any{"description": "inspect services", "command": "Get-Service"}, data: map[string]any{"stdout": "service", "stderr": "", "exitCode": 0, "durationMs": 800, "interrupted": false}, metadata: map[string]string{"semanticCategory": "read"}, want: []string{"PowerShell", "inspect services", "completed", "exit 0", "800ms"}},
		"Read":         {input: map[string]any{"file_path": "/workspace/main.go"}, data: map[string]any{"type": "text", "file": map[string]any{"filePath": "/workspace/main.go", "numLines": 42, "startLine": 10, "totalLines": 100}}, want: []string{"Read", "/workspace/main.go", "42 lines", "window 10..51/100"}},
		"Write":        {input: map[string]any{"file_path": "/workspace/new.go"}, data: map[string]any{"type": "create", "filePath": "/workspace/new.go", "content": "hello\n", "structuredPatch": []any{}, "originalFile": nil, "gitDiff": map[string]any{"additions": 12, "deletions": 0}}, want: []string{"Created", "/workspace/new.go", "6 B", "+12", "-0"}},
		"Edit":         {input: map[string]any{"file_path": "/workspace/edit.go"}, data: map[string]any{"filePath": "/workspace/edit.go", "status": "modified", "metadata": map[string]any{"occurrences": 2, "durationMs": 75}, "gitDiff": map[string]any{"additions": 3, "deletions": 2}}, want: []string{"Updated", "/workspace/edit.go", "2 replacements", "+3", "-2", "Duration: 75ms"}},
		"NotebookEdit": {input: map[string]any{"notebook_path": "/workspace/a.ipynb"}, data: map[string]any{"new_source": "print(1)", "cell_id": "cell-7", "cell_type": "code", "language": "python", "edit_mode": "insert", "notebook_path": "/workspace/a.ipynb", "original_file": "{}", "updated_file": "{}"}, want: []string{"Inserted cell", "/workspace/a.ipynb", "cell cell-7", "code", "python"}},
		"Glob":         {input: map[string]any{"pattern": "**/*.go", "path": "/workspace"}, data: map[string]any{"durationMs": 18, "numFiles": 14, "filenames": []any{"a.go"}, "truncated": false}, want: []string{"Glob", "**/*.go", "14 files", "Duration: 18ms"}},
		"Grep":         {input: map[string]any{"pattern": "TODO", "path": "/workspace"}, data: map[string]any{"mode": "content", "numMatches": 9, "numFiles": 3, "appliedLimit": 20, "appliedOffset": 5}, want: []string{"Grep", "TODO", "9 matches", "3 files", "mode content", "limit 20", "offset 5"}},
		"LSP":          {input: map[string]any{"operation": "hover", "filePath": "/workspace/main.go", "line": 12, "character": 4}, content: "symbol hover", want: []string{"LSP", "hover", "/workspace/main.go", "12:4"}},
		"ToolSearch": {input: map[string]any{"query": "calendar tools", "max_results": 4}, content: `Loaded 4 tool(s) for "calendar tools".`, blocks: []types.ContentBlock{
			types.ToolReferenceBlock{Type: types.ContentTypeToolReference, ToolName: "CalendarCreate"},
			types.ToolReferenceBlock{Type: types.ContentTypeToolReference, ToolName: "CalendarGet"},
			types.ToolReferenceBlock{Type: types.ContentTypeToolReference, ToolName: "CalendarList"},
			types.ToolReferenceBlock{Type: types.ContentTypeToolReference, ToolName: "CalendarDelete"},
		}, want: []string{"ToolSearch", "calendar tools", "4 tools"}},
		"WebFetch":             {input: map[string]any{"url": "https://example.com/docs"}, data: map[string]any{"code": 200, "bytes": 4096, "durationMs": 1250}, want: []string{"WebFetch", "https://example.com/docs", "200", "4.0 KiB", "1.2s"}},
		"WebSearch":            {input: map[string]any{"query": "Go release notes"}, data: map[string]any{"query": "Go release notes", "results": []any{map[string]any{"tool_use_id": "srv-1", "content": []any{map[string]any{"title": "Go", "url": "https://go.dev"}}}}, "durationSeconds": 0.9}, metadata: map[string]string{"results_count": "5", "durationSeconds": "0.9"}, want: []string{"WebSearch", "Go release notes", "5 sources", "900ms"}},
		"ListMcpResourcesTool": {input: map[string]any{"server": "docs"}, data: []any{map[string]any{"uri": "doc://a", "name": "A", "server": "docs"}, map[string]any{"uri": "doc://b", "name": "B", "server": "docs"}}, want: []string{"ListMcpResourcesTool", "docs", "2 resources"}},
		"ReadMcpResourceTool":  {input: map[string]any{"server": "docs", "uri": "doc://guide"}, data: map[string]any{"contents": []any{map[string]any{"uri": "doc://guide", "mimeType": "text/markdown", "text": "guide text"}}}, want: []string{"ReadMcpResourceTool", "docs/doc://guide", "1 contents", "text/markdown", "10 B"}},
		"Agent":                {input: map[string]any{"description": "verify display"}, data: map[string]any{"kind": "completed", "agentId": "agent-7", "agentType": "verifier", "totalToolUseCount": 4, "totalTokens": 1200, "durationMs": 2500, "transcriptPath": "/tmp/agent-7.jsonl"}, want: []string{"Agent", "agent-7", "completed", "verifier", "4 tools", "1,200 tokens", "2.5s"}},
		"TaskCreate":           {input: map[string]any{"subject": "Audit display"}, data: map[string]any{"task": map[string]any{"id": "17", "subject": "Audit display"}}, want: []string{"TaskCreate", "17", "Audit display"}},
		"TaskList":             {data: map[string]any{"tasks": []any{map[string]any{"id": "1", "status": "in_progress", "owner": "agent-1", "blockedBy": []any{}}, map[string]any{"id": "2", "status": "pending", "blockedBy": []any{"1"}}}}, want: []string{"TaskList", "1 active", "1 pending", "1 blocked"}},
		"TaskUpdate":           {input: map[string]any{"taskId": "17"}, data: map[string]any{"taskId": "17", "statusChange": map[string]any{"from": "pending", "to": "in_progress"}}, want: []string{"TaskUpdate", "17", "pending -> in_progress"}},
		"TaskGet":              {input: map[string]any{"taskId": "17"}, data: map[string]any{"task": map[string]any{"id": "17", "subject": "Audit display", "status": "in_progress"}}, want: []string{"TaskGet", "17", "Audit display", "in_progress"}},
		"TaskStop":             {input: map[string]any{"task_id": "agent-7"}, content: `{"task_id":"agent-7","task_type":"agent","command":"verify display"}`, want: []string{"TaskStop", "agent-7", "agent"}},
		"TaskOutput":           {input: map[string]any{"task_id": "agent-7", "block": true}, data: map[string]any{"task_id": "agent-7", "task_type": "local_agent", "retrieval_status": "success", "task_status": "completed", "output_bytes": 4096, "start_offset": 1024, "end_offset": 5120, "total_bytes": 8192, "was_truncated": true, "block": true, "exit_code": 0}, want: []string{"TaskOutput", "agent-7", "success", "completed", "4.0 KiB", "of 8.0 KiB", "offset 1024..5120", "follow", "exit 0", "truncated"}},
		"GetGoal":              {data: map[string]any{"goal": map[string]any{"objective": "Ship display policy", "status": "active", "tokensUsed": 320}}, want: []string{"GetGoal", "Ship display policy", "active", "320 tokens"}},
		"CreateGoal":           {input: map[string]any{"objective": "Ship display policy", "token_budget": 5000}, data: map[string]any{"goal": map[string]any{"objective": "Ship display policy", "status": "active", "tokensUsed": 0}}, want: []string{"CreateGoal", "Ship display policy", "active", "0 tokens"}},
		"UpdateGoal":           {input: map[string]any{"status": "complete"}, data: map[string]any{"goal": map[string]any{"objective": "Ship display policy", "status": "achieved", "tokensUsed": 900}}, want: []string{"UpdateGoal", "Ship display policy", "achieved", "900 tokens"}},
		"EnterPlanMode":        {data: map[string]any{"message": "plan mode enabled"}, want: []string{"EnterPlanMode", "completed"}},
		"ExitPlanMode":         {data: map[string]any{"filePath": "/workspace/plan.md", "isAgent": true, "planWasEdited": true, "awaitingLeaderApproval": true, "requestId": "decision-7"}, metadata: map[string]string{"exitPlanModeStatus": "awaiting_approval"}, want: []string{"ExitPlanMode", "completed", "/workspace/plan.md", "awaiting leader approval", "edited=true", "agent=true", "request decision-7", "awaiting_approval"}},
		"AskUserQuestion":      {input: map[string]any{"questions": []any{map[string]any{"question": "Choose", "options": []any{map[string]any{"label": "A"}, map[string]any{"label": "B"}}}}}, data: map[string]any{"questions": []any{map[string]any{"question": "Choose", "options": []any{map[string]any{"label": "A"}, map[string]any{"label": "B"}}}}, "answers": map[string]any{"Choose": "A"}, "annotations": map[string]any{}}, want: []string{"AskUserQuestion", "completed", "1 questions", "2 choices", "1 answers"}},
		"SendUserMessage":      {data: map[string]any{"message": "done", "attachments": []any{map[string]any{"path": "report.md", "size": 42}}, "sentAt": "2026-07-15T12:00:00.000Z"}, metadata: map[string]string{"messageStatus": "proactive"}, want: []string{"SendUserMessage", "completed", "1 attachments", "sent 2026-07-15T12:00:00.000Z", "proactive"}},
		"SendMessage":          {input: map[string]any{"to": "agent-7"}, data: map[string]any{"success": true, "target": "agent-7", "message": "sent", "request_id": "request-4", "routing": map[string]any{"sender": "lead", "target": "agent-7"}}, want: []string{"SendMessage", "completed", "to agent-7", "request request-4"}},
		"TeamCreate":           {input: map[string]any{"team_name": "display"}, content: `{"team_name":"display","team_file_path":"/tmp/display/config.json","lead_agent_id":"team-lead@display"}`, want: []string{"TeamCreate", "completed", "display", "lead team-lead@display", "config tmp/display/config.json"}},
		"TeamDelete":           {input: map[string]any{"team_name": "display"}, content: `{"team_name":"display","success":true}`, want: []string{"TeamDelete", "completed", "display"}},
		"CronCreate":           {input: map[string]any{"cron": "0 9 * * *"}, data: map[string]any{"id": "cron-4", "cron": "0 9 * * *", "next_fire": "2026-07-16T01:00:00Z", "recurring": true, "durable": true}, metadata: map[string]string{"next_fire": "2026-07-16T01:00:00Z", "tz": "UTC"}, want: []string{"CronCreate", "completed", "cron-4", "0 9 * * *", "next 2026-07-16T01:00:00Z", "UTC", "recurring=true", "durable=true"}},
		"CronDelete":           {data: map[string]any{"id": "cron-4"}, want: []string{"CronDelete", "completed", "cron-4"}},
		"CronList":             {data: map[string]any{"jobs": []any{map[string]any{"id": "cron-1", "recurring": true, "durable": true}, map[string]any{"id": "cron-2", "recurring": false, "durable": true}}}, want: []string{"CronList", "completed", "2 jobs", "1 recurring", "2 durable"}},
		"EnterWorktree":        {input: map[string]any{"name": "display"}, data: map[string]any{"worktreePath": "/workspace/.worktrees/display", "worktreeBranch": "codex/display"}, want: []string{"EnterWorktree", "completed", "workspace/.worktrees/display", "codex/display"}},
		"ExitWorktree":         {input: map[string]any{"action": "remove"}, data: map[string]any{"action": "remove", "originalCwd": "/workspace", "worktreePath": "/workspace/.worktrees/display", "worktreeBranch": "codex/display", "discardedFiles": 2, "discardedCommits": 1, "tmuxSessionName": "display"}, want: []string{"ExitWorktree", "completed", "remove", "/workspace/.worktrees/display", "codex/display", "2 files discarded", "1 commits discarded", "tmux display"}},
		"Config":               {input: map[string]any{"setting": "theme", "value": "dark"}, want: []string{"Config", "completed", "set", "theme"}},
		"Skill":                {input: map[string]any{"skill": "review"}, metadata: map[string]string{"commandName": "review", "status": "forked", "loadedFrom": "/workspace/skills/review/SKILL.md", "model": "gpt-5.5", "permissionDecision": "allowed"}, want: []string{"Skill", "review", "forked", "/workspace/skills/review/SKILL.md", "gpt-5.5", "allowed"}},
		"RemoteTrigger":        {input: map[string]any{"action": "get", "trigger_id": "trigger-9"}, data: map[string]any{"status": 200, "json": `{\"id\":\"trigger-9\"}`}, want: []string{"RemoteTrigger", "get", "completed", "HTTP 200", "trigger-9"}},
	}

	if len(tests) != len(productionToolFamiliesForPresentationTest) {
		t.Fatalf("real-field fixtures = %d, want %d", len(tests), len(productionToolFamiliesForPresentationTest))
	}
	for toolName := range productionToolFamiliesForPresentationTest {
		fixture, ok := tests[toolName]
		if !ok {
			t.Errorf("missing real-field fixture for production tool %q", toolName)
			continue
		}
		t.Run(toolName, func(t *testing.T) {
			got := FormatToolPresentation(toolName, fixture.input, OutcomeSucceeded, &types.ToolResultBlock{
				Outcome:       types.ToolOutcomeSucceeded,
				Content:       fixture.content,
				ContentBlocks: fixture.blocks,
				Data:          fixture.data,
				Metadata:      fixture.metadata,
			})
			presentation := got.Summary + "\n" + strings.Join(got.DetailLines, "\n")
			for _, want := range fixture.want {
				if want == toolName {
					want = semanticToolActionInLanguage(i18n.LangEN, toolName)
					if got.Family == FamilyFileRead {
						want = i18n.Text(i18n.LangEN, i18n.KeyPresentationAggregateRead)
					} else if got.Family == FamilyAgent {
						want = i18n.Text(i18n.LangEN, i18n.KeyPresentationAgent)
					}
				}
				if want == "completed" && got.Lifecycle == PresentationLifecycleCompleted && !strings.Contains(presentation, want) {
					continue
				}
				if !strings.Contains(presentation, want) {
					t.Errorf("presentation missing %q: %+v", want, got)
				}
			}
		})
	}
}

func TestFormatToolPresentationUsesTypedOutcomeWithoutPayloadInference(t *testing.T) {
	tests := []struct {
		name        string
		tool        string
		input       map[string]any
		initial     ObservationOutcome
		result      *types.ToolResultBlock
		wantOutcome ObservationOutcome
	}{
		{
			name: "task update success false", tool: "TaskUpdate", input: map[string]any{"taskId": "7"}, initial: OutcomeSucceeded,
			result:      &types.ToolResultBlock{Outcome: types.ToolOutcomeFailed, Data: map[string]any{"success": false, "error": "invalid transition"}},
			wantOutcome: OutcomeFailed,
		},
		{
			name: "team delete JSON content", tool: "TeamDelete", initial: OutcomeSucceeded,
			result:      &types.ToolResultBlock{Outcome: types.ToolOutcomeFailed, Content: `{"success":false,"error":"team is busy"}`},
			wantOutcome: OutcomeFailed,
		},
		{
			name: "remote HTTP failure", tool: "RemoteTrigger", input: map[string]any{"action": "create"}, initial: OutcomeSucceeded,
			result:      &types.ToolResultBlock{Outcome: types.ToolOutcomeFailed, Data: map[string]any{"status": 500, "json": `{}`}},
			wantOutcome: OutcomeFailed,
		},
		{
			name: "web HTTP failure", tool: "WebFetch", input: map[string]any{"url": "https://example.com/missing"}, initial: OutcomeSucceeded,
			result:      &types.ToolResultBlock{Outcome: types.ToolOutcomeFailed, Data: map[string]any{"code": 404, "codeText": "Not Found"}},
			wantOutcome: OutcomeFailed,
		},
		{
			name: "generic result error", tool: "Bash", initial: OutcomeSucceeded,
			result:      &types.ToolResultBlock{Outcome: types.ToolOutcomeFailed, IsError: true, Content: "command failed"},
			wantOutcome: OutcomeFailed,
		},
		{
			name: "explicit denial preserved", tool: "Write", initial: OutcomeDenied,
			result:      &types.ToolResultBlock{Outcome: types.ToolOutcomeDenied, Data: map[string]any{"success": true}},
			wantOutcome: OutcomeDenied,
		},
		{
			name: "typed denial refines generic failure", tool: "Read", initial: OutcomeFailed,
			result:      &types.ToolResultBlock{Outcome: types.ToolOutcomeDenied, IsError: true, Content: "policy denied access"},
			wantOutcome: OutcomeDenied,
		},
		{
			name: "prose is never status input", tool: "PowerShell", initial: OutcomeSucceeded,
			result:      &types.ToolResultBlock{Outcome: types.ToolOutcomeSucceeded, Content: "success=false HTTP 500 error=imaginary"},
			wantOutcome: OutcomeSucceeded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatToolPresentation(tt.tool, tt.input, tt.initial, tt.result)
			if got.Outcome != tt.wantOutcome {
				t.Fatalf("Outcome = %q, want %q; presentation=%+v", got.Outcome, tt.wantOutcome, got)
			}
			if got.Facts(tt.initial).Outcome != tt.wantOutcome {
				t.Fatalf("Facts outcome = %q, want %q", got.Facts(tt.initial).Outcome, tt.wantOutcome)
			}
			promoted := tt.wantOutcome != tt.initial
			if got.Warning != promoted {
				t.Fatalf("Warning = %t, want %t for %+v", got.Warning, promoted, got)
			}
		})
	}
}

func TestFormatToolPresentationTreatsAsyncAgentAsBackgroundNotFailure(t *testing.T) {
	got := FormatToolPresentation("Agent", map[string]any{"description": "audit"}, OutcomeSucceeded, &types.ToolResultBlock{
		Outcome: types.ToolOutcomeSucceeded,
		Data: map[string]any{
			"kind": "partial", "isAsync": true, "agentId": "agent-bg", "outputFile": "/tmp/agent-bg/output.txt",
		},
	})
	if got.Outcome != OutcomeSucceeded || !got.Background || got.Warning || got.NeedsReview {
		t.Fatalf("async agent was misclassified: %+v", got)
	}
}

func TestFormatToolPresentationSafeFallbackIsDeterministicBoundedAndOpaque(t *testing.T) {
	first := map[string]any{}
	first["zeta"] = "last-value"
	first["api_key"] = "must-not-leak"
	first["alpha"] = "first-value"
	second := map[string]any{}
	second["alpha"] = "first-value"
	second["api_key"] = "must-not-leak"
	second["zeta"] = "last-value"
	content := strings.Repeat("retained evidence line\n", 200)

	one := FormatToolPresentation("FutureTool", first, OutcomeSucceeded, &types.ToolResultBlock{Content: content})
	two := FormatToolPresentation("FutureTool", second, OutcomeSucceeded, &types.ToolResultBlock{Content: content})
	if one.Summary != two.Summary {
		t.Fatalf("fallback is map-order dependent: %q != %q", one.Summary, two.Summary)
	}
	for _, want := range []string{"FutureTool", "input keys: alpha,api_key,zeta", "201 lines", "details available"} {
		if !strings.Contains(one.Summary, want) {
			t.Fatalf("fallback summary %q missing %q", one.Summary, want)
		}
	}
	all := one.Summary + "\n" + strings.Join(one.DetailLines, "\n")
	for _, forbidden := range []string{"must-not-leak", "first-value", "last-value", "retained evidence line"} {
		if strings.Contains(all, forbidden) {
			t.Fatalf("fallback leaked %q: %s", forbidden, all)
		}
	}
	if !one.Sensitive || !one.HasMore || one.Object != "" {
		t.Fatalf("fallback safety flags = %+v", one)
	}
	if utf8.RuneCountInString(one.Summary) > maxPresentationSummaryRunes {
		t.Fatalf("summary has %d runes", utf8.RuneCountInString(one.Summary))
	}
	for _, line := range one.DetailLines {
		if utf8.RuneCountInString(line) > maxPresentationDetailRunes {
			t.Fatalf("detail has %d runes: %q", utf8.RuneCountInString(line), line)
		}
	}
}

func TestFormatToolPresentationFailureHasBoundedRootCause(t *testing.T) {
	content := "root cause\n" + strings.Repeat("failure detail ", 300)
	got := FormatToolPresentation("Bash", map[string]any{"command": "go test ./..."}, OutcomeFailed, &types.ToolResultBlock{
		Content: content, IsError: true, Metadata: map[string]string{"exit_code": "1", "duration_ms": "1250"},
	})
	joined := strings.Join(got.DetailLines, "\n")
	for _, want := range []string{"Outcome: failed", "exit code 1", "1.2s", "Cause: root cause"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("failure details omitted %q: %v", want, got.DetailLines)
		}
	}
	for _, line := range got.DetailLines {
		if utf8.RuneCountInString(line) > maxPresentationDetailRunes {
			t.Fatalf("bounded detail grew to %d runes", utf8.RuneCountInString(line))
		}
	}
}

func TestFormatToolPresentationSuccessfulResultPreviewIsBoundedAndKeepsTail(t *testing.T) {
	content := "first retained line\n" + strings.Repeat("middle detail\n", 200) + "DETAIL_SENTINEL"
	got := FormatToolPresentation("Read", map[string]any{"file_path": "/workspace/main.go"}, OutcomeSucceeded, &types.ToolResultBlock{
		Outcome: types.ToolOutcomeSucceeded,
		Content: content,
	})
	joined := strings.Join(got.DetailLines, "\n")
	for _, want := range []string{"Result: first retained line", "DETAIL_SENTINEL"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("successful detail omitted %q: %v", want, got.DetailLines)
		}
	}
	for _, line := range got.DetailLines {
		if utf8.RuneCountInString(line) > maxPresentationDetailRunes {
			t.Fatalf("bounded result preview grew to %d runes", utf8.RuneCountInString(line))
		}
	}
}

func TestFormatToolPresentationAgentDetailUsesTypedContentWithoutProtocolTrailer(t *testing.T) {
	useDetailStoreTestLanguage(t, i18n.LangEN)
	cjkLongLine := strings.Repeat("完整的中文结论不应被截断。", 40)
	body := "The focused audit completed successfully.\n\n" +
		"The typed result is the only display source.\n" + cjkLongLine + "\n" +
		"FINAL_CONCLUSION: every typed conclusion line remains visible."
	protocolTrailer := "agentId: agent-internal (use SendMessage with to: 'agent-internal' to continue this agent)\n<usage>total_tokens: 145555\ntool_uses: 17\nduration_ms: 54090</usage>"
	result := &types.ToolResultBlock{
		Outcome: types.ToolOutcomeSucceeded,
		ContentBlocks: []types.ContentBlock{
			types.TextBlock{Type: types.ContentTypeText, Text: body},
			types.TextBlock{Type: types.ContentTypeText, Text: protocolTrailer},
		},
		Data: map[string]any{
			"kind":           "completed",
			"agentId":        "agent-internal",
			"transcriptPath": "/tmp/private/agent-internal.jsonl",
			"content": []any{
				map[string]any{"type": "text", "text": body},
			},
		},
	}
	if providerText := result.TextContent(); !strings.Contains(providerText, "<usage>") || !strings.Contains(providerText, "agentId: agent-internal") {
		t.Fatalf("provider-compatible text was unexpectedly changed: %q", providerText)
	}

	got := FormatToolPresentation("Agent", map[string]any{"description": "focused audit"}, OutcomeSucceeded, result)
	details := strings.Join(got.DetailLines, "\n")
	for _, forbidden := range []string{"agentId:", "<usage>", "tool_uses:", "duration_ms:", "/tmp/private/agent-internal.jsonl"} {
		if strings.Contains(details, forbidden) {
			t.Fatalf("Agent detail leaked provider protocol field %q: %q", forbidden, details)
		}
	}
	if got := strings.Count(details, "The focused audit completed successfully."); got != 1 {
		t.Fatalf("typed Agent result appeared %d times, want once: %q", got, details)
	}
	if got.HasMore {
		t.Fatalf("Agent presentation still advertises complete run evidence: %+v", got)
	}
	var resultLine string
	for _, line := range got.DetailLines {
		if strings.Contains(line, "The focused audit completed successfully.") {
			resultLine = line
			break
		}
	}
	if resultLine == "" {
		t.Fatalf("Agent detail omitted typed result: %q", details)
	}
	if !strings.Contains(resultLine, body) {
		t.Fatalf("Agent result did not preserve the complete typed conclusion:\nwant body: %q\nresult: %q", body, resultLine)
	}
	for _, want := range []string{
		"The focused audit completed successfully.",
		"\n\n",
		cjkLongLine,
		"FINAL_CONCLUSION: every typed conclusion line remains visible.",
	} {
		if !strings.Contains(resultLine, want) {
			t.Fatalf("Agent result omitted complete typed content %q: %q", want, resultLine)
		}
	}
}

func TestFormatToolPresentationAgentFailureDoesNotFallbackToProtocolTrailer(t *testing.T) {
	useDetailStoreTestLanguage(t, i18n.LangEN)
	protocolTrailer := "agentId: agent-internal\noutcome: failed\n<usage>total_tokens: 100\ntool_uses: 3\nduration_ms: 900</usage>"
	got := FormatToolPresentation("Agent", map[string]any{"description": "focused audit"}, OutcomeFailed, &types.ToolResultBlock{
		Outcome: types.ToolOutcomeFailed,
		IsError: true,
		Content: protocolTrailer,
		Data: map[string]any{
			"kind":    "error",
			"message": "provider request failed",
		},
	})
	details := strings.Join(got.DetailLines, "\n")
	if !strings.Contains(details, "Cause: provider request failed") {
		t.Fatalf("Agent failure omitted typed cause: %q", details)
	}
	for _, forbidden := range []string{"agentId:", "<usage>", "tool_uses:", "duration_ms:"} {
		if strings.Contains(details, forbidden) {
			t.Fatalf("Agent failure leaked provider protocol field %q: %q", forbidden, details)
		}
	}
}

func TestPresentationRedactionDistinguishesUsageMetricsFromCredentials(t *testing.T) {
	metrics := map[string]any{"totalTokens": 1200, "input_tokens": 800, "token_budget": 5000, "token_count": 4, "maxTokens": 8192}
	got := FormatToolPresentation("Agent", nil, OutcomeSucceeded, &types.ToolResultBlock{
		Outcome: types.ToolOutcomeSucceeded,
		Data:    metrics,
	})
	if got.Sensitive {
		t.Fatalf("usage metrics were treated as credentials: %+v", got)
	}
	redactedMetrics := RedactPresentationValue(metrics).(map[string]any)
	for key, value := range metrics {
		if redactedMetrics[key] != value {
			t.Fatalf("metric %q was redacted: %#v", key, redactedMetrics[key])
		}
	}

	credentials := map[string]any{
		"token": "one", "access_token": "two", "accessToken": "two-camel", "authorization": "Bearer three",
		"nested": map[string]any{"client_secret": "four"},
	}
	redacted := RedactPresentationValue(credentials).(map[string]any)
	encoded := presentationString(redacted)
	if strings.Contains(encoded, "one") || strings.Contains(encoded, "two") || strings.Contains(encoded, "three") || strings.Contains(encoded, "four") {
		t.Fatalf("credential value leaked after structural redaction: %s", encoded)
	}
	if !presentationValueHasSensitiveKey(credentials) {
		t.Fatal("credential-shaped keys were not detected")
	}
}

func TestPresentationSanitizesCredentialBearingURLsAndURIs(t *testing.T) {
	tests := []struct {
		name  string
		tool  string
		input map[string]any
		want  []string
	}{
		{
			name:  "web URL userinfo query and fragment",
			tool:  "WebFetch",
			input: map[string]any{"url": "https://alice:password@example.com/safe/path?view=compact&access_token=access-secret#callback?code=oauth-secret&tab=summary"},
			want:  []string{"https://example.com/safe/path", "view=compact", "tab=summary"},
		},
		{
			name:  "MCP URI API key and signature",
			tool:  "ReadMcpResourceTool",
			input: map[string]any{"server": "docs", "uri": "memo://catalog/report?api_key=api-secret&format=json#signature=signed-secret"},
			want:  []string{"docs/memo://catalog/report", "format=json"},
		},
		{
			name:  "remote URL target",
			tool:  "RemoteTrigger",
			input: map[string]any{"action": "get", "target": "https://remote.example/run?key=target-secret&safe=yes"},
			want:  []string{"https://remote.example/run", "safe=yes"},
		},
		{
			name:  "message recipient URL",
			tool:  "SendMessage",
			input: map[string]any{"to": "https://hooks.example/send?signature=recipient-secret&channel=ops"},
			want:  []string{"https://hooks.example/send", "channel=ops"},
		},
		{
			name:  "credential URL nested under a generic field",
			tool:  "WebFetch",
			input: map[string]any{"url": "https://example.com/safe", "options": map[string]any{"endpoint": "https://api.example/run?token=nested-secret&mode=read"}},
			want:  []string{"https://example.com/safe", "mode=read"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatToolPresentation(tt.tool, tt.input, OutcomeRunning, nil)
			visible := got.Object + "\n" + got.Summary + "\n" + strings.Join(got.DetailLines, "\n")
			for _, secret := range []string{"password", "access-secret", "oauth-secret", "api-secret", "signed-secret", "target-secret", "recipient-secret", "nested-secret", "alice"} {
				if strings.Contains(visible, secret) {
					t.Fatalf("presentation leaked %q: %+v", secret, got)
				}
			}
			for _, want := range tt.want {
				if !strings.Contains(visible, want) {
					t.Fatalf("sanitized presentation omitted safe locator component %q: %+v", want, got)
				}
			}
			if !got.Sensitive {
				t.Fatalf("sanitized locator did not mark the projection sensitive: %+v", got)
			}
		})
	}
}

func TestPresentationLocatorSanitizerLeavesSafePathsAndURLsUseful(t *testing.T) {
	path := FormatToolPresentation("Read", map[string]any{"file_path": "/workspace/a file.go"}, OutcomeRunning, nil)
	if path.Object != "/workspace/a file.go" {
		t.Fatalf("file path was URL-encoded: %+v", path)
	}
	web := FormatToolPresentation("WebFetch", map[string]any{"url": "https://example.com/a/b?lang=zh&view=full#section"}, OutcomeRunning, nil)
	for _, want := range []string{"https://example.com/a/b", "lang=zh", "view=full", "section"} {
		if !strings.Contains(web.Object, want) {
			t.Fatalf("safe URL omitted %q: %+v", want, web)
		}
	}
	if web.Sensitive {
		t.Fatalf("safe URL was marked sensitive: %+v", web)
	}
}

func TestRedactPresentationTextIsStructuralAndBounded(t *testing.T) {
	jsonText := RedactPresentationText(`{"totalTokens":321,"token":"must-not-leak","nested":{"client_secret":"also-secret"}}`, 200)
	if !strings.Contains(jsonText, `"totalTokens":321`) || !strings.Contains(jsonText, `"token":"[REDACTED]"`) {
		t.Fatalf("JSON text redaction = %q", jsonText)
	}
	if strings.Contains(jsonText, "must-not-leak") || strings.Contains(jsonText, "also-secret") {
		t.Fatalf("JSON text leaked credentials: %q", jsonText)
	}

	prose := RedactPresentationText("first line\ntoken: must-not-leak\n"+strings.Repeat("x", 500), 80)
	if !strings.Contains(prose, "[REDACTED sensitive detail]") || strings.Contains(prose, "must-not-leak") {
		t.Fatalf("prose redaction = %q", prose)
	}
	if utf8.RuneCountInString(prose) > 80 {
		t.Fatalf("prose redaction has %d runes", utf8.RuneCountInString(prose))
	}
}

func TestFormatToolPresentationStableSummaryByCoreFamily(t *testing.T) {
	tests := []struct {
		tool  string
		input map[string]any
		data  map[string]any
		want  string
	}{
		{"Bash", map[string]any{"command": "go test ./tui"}, map[string]any{"exitCode": 0}, "Run command · go test ./tui · exit 0"},
		{"mcp__github__search", map[string]any{}, map[string]any{}, "Use MCP tool · github/search"},
		{"Config", map[string]any{"action": "get", "key": "theme"}, map[string]any{}, "Read configuration · get · theme"},
		{"CronList", nil, map[string]any{"jobs": []any{}}, "No schedules"},
	}
	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			got := FormatToolPresentation(tt.tool, tt.input, OutcomeSucceeded, &types.ToolResultBlock{
				Outcome: types.ToolOutcomeSucceeded,
				Data:    tt.data,
			})
			if got.Summary != tt.want {
				t.Fatalf("Summary = %q, want %q", got.Summary, tt.want)
			}
		})
	}
}

func TestShellPresentationUsesStructuredExecutionSemantics(t *testing.T) {
	tests := []struct {
		name        string
		metadata    map[string]string
		wantRisk    PresentationRisk
		wantSide    bool
		wantWarning bool
		wantLevel   PresentationLevel
	}{
		{name: "read remains folded", metadata: map[string]string{"semanticCategory": "read", "wasReadOnly": "true"}, wantRisk: RiskLow, wantLevel: PresentationFolded},
		{name: "write receipt is structured", metadata: map[string]string{"semanticCategory": "write"}, wantRisk: RiskMedium, wantSide: true, wantLevel: PresentationStructured},
		{name: "network receipt is structured", metadata: map[string]string{"semanticCategory": "network"}, wantRisk: RiskMedium, wantSide: true, wantLevel: PresentationStructured},
		{name: "destructive receipt is structured", metadata: map[string]string{"semanticCategory": "destructive", "destructiveWarning": "confirm deletion"}, wantRisk: RiskDestructive, wantSide: true, wantWarning: true, wantLevel: PresentationStructured},
		{name: "security warning is structured", metadata: map[string]string{"semanticCategory": "read", "securityWarn": "suspicious expansion"}, wantRisk: RiskLow, wantWarning: true, wantLevel: PresentationStructured},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatted := FormatToolPresentation("Bash", map[string]any{"command": "structured command"}, OutcomeSucceeded, &types.ToolResultBlock{
				Outcome: types.ToolOutcomeSucceeded, Metadata: tt.metadata,
			})
			if formatted.Risk != tt.wantRisk || formatted.SideEffect != tt.wantSide || formatted.Warning != tt.wantWarning {
				t.Fatalf("formatted facts=%+v", formatted)
			}
			decision := DecidePresentation(formatted.Facts(formatted.Outcome))
			if decision.EffectiveLevel != tt.wantLevel {
				t.Fatalf("decision=%+v, want %s", decision, tt.wantLevel)
			}
		})
	}
}

func TestFormatterMapsAgentPlanAndScopeFactsWithoutParsingProse(t *testing.T) {
	agent := FormatToolPresentation("Agent", nil, OutcomeSucceeded, &types.ToolResultBlock{
		Outcome: types.ToolOutcomeSucceeded, Data: map[string]any{"kind": "completed", "agentId": "agent-1"},
	})
	agentDecision := DecidePresentation(agent.Facts(agent.Outcome))
	if !agent.TerminalAgent || !containsPresentationReason(agentDecision.Reasons, ReasonTerminalAgent) || agentDecision.EffectiveLevel < PresentationStructured {
		t.Fatalf("agent terminal facts=%+v decision=%+v", agent, agentDecision)
	}
	plan := FormatToolPresentation("ExitPlanMode", nil, OutcomeSucceeded, &types.ToolResultBlock{Outcome: types.ToolOutcomeSucceeded})
	planDecision := DecidePresentation(plan.Facts(plan.Outcome))
	if !plan.PlanGate || !containsPresentationReason(planDecision.Reasons, ReasonPlanGate) {
		t.Fatalf("plan gate facts=%+v decision=%+v", plan, planDecision)
	}
	scope := FormatToolPresentation("Read", nil, OutcomeSucceeded, &types.ToolResultBlock{
		Outcome: types.ToolOutcomeSucceeded, Metadata: map[string]string{"scope_expanded": "true"},
	})
	scopeDecision := DecidePresentation(scope.Facts(scope.Outcome))
	if !scope.ScopeExpanded || !containsPresentationReason(scopeDecision.Reasons, ReasonScopeExpanded) || scopeDecision.EffectiveLevel < PresentationStructured {
		t.Fatalf("scope facts=%+v decision=%+v", scope, scopeDecision)
	}
}

func TestApplyPatchPresentationSeparatesParameterChangesAndReceipt(t *testing.T) {
	formatted := FormatToolPresentationInLanguage(i18n.LangZH, "ApplyPatch", map[string]any{"patch": "raw"}, OutcomeSucceeded, &types.ToolResultBlock{
		Outcome: types.ToolOutcomeSucceeded,
		Metadata: map[string]string{
			"apply_patch.parameter_bytes": "2048",
			"apply_patch.changed_files":   "3",
			"apply_patch.additions":       "17",
			"apply_patch.deletions":       "4",
			"apply_patch.receipt_bytes":   "71",
		},
	})
	for _, want := range []string{"参数 2.0 KiB", "变更 3 个文件 / +17 -4", "回执 71 B"} {
		if !strings.Contains(formatted.Summary, want) {
			t.Fatalf("ApplyPatch summary %q missing %q", formatted.Summary, want)
		}
	}
}

func TestApplyPatchCompletedPresentationCoversParameterScalesAndMultiFileDiffstat(t *testing.T) {
	for _, test := range []struct {
		name     string
		metadata map[string]string
		want     []string
	}{
		{
			name: "one byte parameter",
			metadata: map[string]string{
				"apply_patch.parameter_bytes": "1", "apply_patch.changed_files": "1",
				"apply_patch.additions": "1", "apply_patch.deletions": "0", "apply_patch.receipt_bytes": "71",
			},
			want: []string{"参数 1 B", "变更 1 个文件 / +1 -0", "回执 71 B"},
		},
		{
			name: "ten kibibyte multi file parameter",
			metadata: map[string]string{
				"apply_patch.parameter_bytes": "10240", "apply_patch.changed_files": "4",
				"apply_patch.additions": "120", "apply_patch.deletions": "17", "apply_patch.receipt_bytes": "512",
			},
			want: []string{"参数 10.0 KiB", "变更 4 个文件 / +120 -17", "回执 512 B"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			formatted := FormatToolPresentationInLanguage(i18n.LangZH, "ApplyPatch", nil, OutcomeSucceeded, &types.ToolResultBlock{
				Outcome: types.ToolOutcomeSucceeded, Metadata: test.metadata,
			})
			for _, want := range test.want {
				if !strings.Contains(formatted.Summary, want) {
					t.Fatalf("completed ApplyPatch summary %q missing %q", formatted.Summary, want)
				}
			}
			if strings.Contains(formatted.Summary, "raw") {
				t.Fatalf("completed ApplyPatch summary leaked parameter content: %q", formatted.Summary)
			}
		})
	}
}

func TestInspectPresentationSurfacesStructuredPartialCause(t *testing.T) {
	failures := `[{"request_id":"root","kind":"read","path":".","code":"read_is_directory","message":"目录不能作为文件读取"}]`
	formatted := FormatToolPresentationInLanguage(i18n.LangZH, "Inspect", nil, OutcomePartial, &types.ToolResultBlock{
		Outcome: types.ToolOutcomePartial,
		Metadata: map[string]string{
			"inspect.partial_failures":         failures,
			"inspect.partial_failure_count":    "1",
			"inspect.successful_request_count": "2",
		},
	})
	if !strings.Contains(formatted.Summary, "1 项请求失败") || !strings.Contains(formatted.Summary, "另有 2 项请求成功") {
		t.Fatalf("Inspect summary = %q", formatted.Summary)
	}
	details := strings.Join(formatted.DetailLines, "\n")
	for _, want := range []string{"root", "read", "read_is_directory", "目录不能作为文件读取"} {
		if !strings.Contains(details, want) {
			t.Fatalf("Inspect details %q missing %q", details, want)
		}
	}
}
