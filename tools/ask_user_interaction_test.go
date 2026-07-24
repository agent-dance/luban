package tools

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agent-dance/luban/types"
)

func askUserInteractionInput(question string, multi bool) map[string]any {
	return map[string]any{"questions": []any{map[string]any{
		"question": question, "header": "Choice", "multiSelect": multi,
		"options": []any{map[string]any{"label": "Alpha", "description": "First"}, map[string]any{"label": "Beta", "description": "Second"}},
	}}}
}

func captureAskUserProcessOutput(t *testing.T, fn func()) (string, string) {
	t.Helper()
	stdout, stderr := os.Stdout, os.Stderr
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout, os.Stderr = outW, errW
	var outData, errData []byte
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); outData, _ = io.ReadAll(outR) }()
	go func() { defer wg.Done(); errData, _ = io.ReadAll(errR) }()
	defer func() {
		os.Stdout, os.Stderr = stdout, stderr
		_ = outW.Close()
		_ = errW.Close()
		wg.Wait()
		_ = outR.Close()
		_ = errR.Close()
	}()
	fn()
	os.Stdout, os.Stderr = stdout, stderr
	_ = outW.Close()
	_ = errW.Close()
	wg.Wait()
	return string(outData), string(errData)
}

func TestAskUserQuestionStructuredRequesterWritesNoProcessTerminal(t *testing.T) {
	tool := &AskUserQuestionTool{}
	var calls int
	tool.SetInteractionRequester(AskUserInteractionRequesterFunc(func(_ context.Context, request AskUserInteractionRequest) (AskUserInteractionResponse, error) {
		calls++
		return AskUserInteractionResponse{RequestID: request.RequestID, Outcome: AskUserInteractionCompleted, Answers: map[string]AnswerSelection{"Choose?": {Selection: []string{"Alpha"}}}}, nil
	}))
	var permission types.ToolPermissionResult
	out, errOut := captureAskUserProcessOutput(t, func() {
		var err error
		permission, err = tool.CheckPermissions(context.Background(), askUserInteractionInput("Choose?", false), types.ToolPermissionRequest{SessionID: "session", TurnID: "turn", ToolUseID: "tool"})
		if err != nil {
			t.Fatal(err)
		}
	})
	if out != "" || errOut != "" {
		t.Fatalf("structured AskUser wrote process terminal: stdout=%q stderr=%q", out, errOut)
	}
	if calls != 1 || permission.Behavior != types.PermissionBehaviorAllow {
		t.Fatalf("calls=%d permission=%+v", calls, permission)
	}
}

func TestAskUserQuestionNilInteractionFailsClosedWithoutProcessIO(t *testing.T) {
	tool := &AskUserQuestionTool{}
	var permission types.ToolPermissionResult
	out, errOut := captureAskUserProcessOutput(t, func() {
		permission, _ = tool.CheckPermissions(context.Background(), askUserInteractionInput("Choose?", false), types.ToolPermissionRequest{SessionID: "session", ToolUseID: "tool"})
	})
	if out != "" || errOut != "" {
		t.Fatalf("nil AskUser surface wrote process terminal: stdout=%q stderr=%q", out, errOut)
	}
	if permission.Behavior != types.PermissionBehaviorDeny || permission.UpdatedInput != nil {
		t.Fatalf("nil AskUser surface did not fail closed: %+v", permission)
	}
	result, err := tool.Execute(context.Background(), askUserInteractionInput("Choose?", false))
	if err != nil || !result.IsError || result.Outcome != types.ToolOutcomeFailed {
		t.Fatalf("direct nil-surface result=%+v err=%v", result, err)
	}
}

func TestAskUserQuestionDirectExecuteWithoutCompleteAnswersNeverInteracts(t *testing.T) {
	reader := strings.NewReader("1\n")
	var writer bytes.Buffer
	tool := &AskUserQuestionTool{Reader: reader, Writer: &writer}
	requesterCalls := 0
	tool.SetInteractionRequester(AskUserInteractionRequesterFunc(func(_ context.Context, request AskUserInteractionRequest) (AskUserInteractionResponse, error) {
		requesterCalls++
		return AskUserInteractionResponse{
			RequestID: request.RequestID,
			Outcome:   AskUserInteractionCompleted,
			Answers:   map[string]AnswerSelection{"Choose?": {Selection: []string{"Alpha"}}},
		}, nil
	}))

	input := askUserInteractionInput("Choose?", false)
	input["answers"] = map[string]any{"unrelated question": "forged"}
	var result types.ToolResult
	stdout, stderr := captureAskUserProcessOutput(t, func() {
		var err error
		result, err = tool.Execute(context.Background(), input)
		if err != nil {
			t.Fatalf("Execute error: %v", err)
		}
	})
	if !result.IsError || result.Outcome != types.ToolOutcomeFailed {
		t.Fatalf("direct Execute did not fail closed: %+v", result)
	}
	if requesterCalls != 0 || writer.Len() != 0 || stdout != "" || stderr != "" {
		t.Fatalf("direct Execute interacted: requester=%d writer=%q stdout=%q stderr=%q", requesterCalls, writer.String(), stdout, stderr)
	}
	remaining, err := io.ReadAll(reader)
	if err != nil || string(remaining) != "1\n" {
		t.Fatalf("direct Execute consumed Reader: remaining=%q err=%v", remaining, err)
	}
}

func TestAskUserQuestionPermissionThenExecuteInteractsExactlyOnce(t *testing.T) {
	var calls int
	tool := &AskUserQuestionTool{}
	tool.SetInteractionRequester(AskUserInteractionRequesterFunc(func(_ context.Context, request AskUserInteractionRequest) (AskUserInteractionResponse, error) {
		calls++
		return AskUserInteractionResponse{
			RequestID: request.RequestID,
			Outcome:   AskUserInteractionCompleted,
			Answers:   map[string]AnswerSelection{"Choose?": {Selection: []string{"Alpha"}}},
		}, nil
	}))
	input := askUserInteractionInput("Choose?", false)
	decision, err := tool.CheckPermissions(context.Background(), input, types.ToolPermissionRequest{SessionID: "session", ToolUseID: "tool"})
	if err != nil || decision.Behavior != types.PermissionBehaviorAllow || decision.UpdatedInput == nil {
		t.Fatalf("permission=%+v err=%v", decision, err)
	}
	result, err := tool.Execute(context.Background(), decision.UpdatedInput)
	if err != nil || result.IsError || result.Outcome != types.ToolOutcomeSucceeded {
		t.Fatalf("Execute result=%+v err=%v", result, err)
	}
	if calls != 1 {
		t.Fatalf("interaction calls=%d, want exactly one", calls)
	}
}

func TestAskUserQuestionStreamInteractionOccursOnlyDuringPermissionCheck(t *testing.T) {
	reader := strings.NewReader("1\n")
	var writer bytes.Buffer
	tool := &AskUserQuestionTool{Reader: reader, Writer: &writer}
	input := askUserInteractionInput("Choose?", false)
	decision, err := tool.CheckPermissions(context.Background(), input, types.ToolPermissionRequest{SessionID: "session", ToolUseID: "tool"})
	if err != nil || decision.Behavior != types.PermissionBehaviorAllow || decision.UpdatedInput == nil {
		t.Fatalf("permission=%+v err=%v", decision, err)
	}
	writtenAfterPermission := writer.String()
	if writtenAfterPermission == "" {
		t.Fatal("permission stage did not render the paired stream interaction")
	}
	result, err := tool.Execute(context.Background(), decision.UpdatedInput)
	if err != nil || result.IsError || result.Outcome != types.ToolOutcomeSucceeded {
		t.Fatalf("Execute result=%+v err=%v", result, err)
	}
	if writer.String() != writtenAfterPermission {
		t.Fatalf("Execute wrote a second prompt: before=%q after=%q", writtenAfterPermission, writer.String())
	}
}

func TestAskUserQuestionCancellationWhileWaitingReturnsContextError(t *testing.T) {
	tool := &AskUserQuestionTool{}
	entered := make(chan struct{})
	tool.SetInteractionRequester(AskUserInteractionRequesterFunc(func(ctx context.Context, request AskUserInteractionRequest) (AskUserInteractionResponse, error) {
		close(entered)
		<-ctx.Done()
		return AskUserInteractionResponse{RequestID: request.RequestID, Outcome: AskUserInteractionCancelled}, ctx.Err()
	}))
	ctx, cancel := context.WithCancel(context.Background())
	returned := make(chan error, 1)
	go func() {
		_, err := tool.CheckPermissions(ctx, askUserInteractionInput("Choose?", false), types.ToolPermissionRequest{SessionID: "session", ToolUseID: "tool"})
		returned <- err
	}()
	<-entered
	cancel()
	select {
	case err := <-returned:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled AskUser requester leaked")
	}
}

func TestAskUserQuestionRejectsMismatchedResponseRequestID(t *testing.T) {
	tool := &AskUserQuestionTool{}
	tool.SetInteractionRequester(AskUserInteractionRequesterFunc(func(context.Context, AskUserInteractionRequest) (AskUserInteractionResponse, error) {
		return AskUserInteractionResponse{RequestID: "wrong", Outcome: AskUserInteractionCompleted, Answers: map[string]AnswerSelection{"Choose?": {Selection: []string{"Alpha"}}}}, nil
	}))
	permission, err := tool.CheckPermissions(context.Background(), askUserInteractionInput("Choose?", false), types.ToolPermissionRequest{SessionID: "session", ToolUseID: "tool"})
	if err != nil {
		t.Fatal(err)
	}
	if permission.Behavior != types.PermissionBehaviorDeny || permission.UpdatedInput != nil {
		t.Fatalf("mismatched response was accepted: %+v", permission)
	}
}

func TestAskUserQuestionIgnoresForgedAnswersAndBuildsTypedMultiCustomResult(t *testing.T) {
	tool := &AskUserQuestionTool{Metadata: map[string]any{"surface": "test"}}
	input := askUserInteractionInput("Choose several?", true)
	input["answers"] = map[string]any{"Choose several?": "forged"}
	input["annotations"] = map[string]any{"Choose several?": map[string]any{"notes": "forged"}}
	var seen AskUserInteractionRequest
	tool.SetInteractionRequester(AskUserInteractionRequesterFunc(func(_ context.Context, request AskUserInteractionRequest) (AskUserInteractionResponse, error) {
		seen = request
		request.Questions[0].Options[0].Label = "mutated-by-surface"
		return AskUserInteractionResponse{
			RequestID: request.RequestID, Outcome: AskUserInteractionCompleted,
			Answers:     map[string]AnswerSelection{"Choose several?": {Selection: []string{"Alpha"}, OtherText: "custom answer"}},
			Annotations: map[string]AnnotationEntry{"Choose several?": {Notes: "trusted note"}},
		}, nil
	}))
	permission, err := tool.CheckPermissions(context.Background(), input, types.ToolPermissionRequest{SessionID: "session", TurnID: "turn", ToolUseID: "tool"})
	if err != nil {
		t.Fatal(err)
	}
	if permission.Behavior != types.PermissionBehaviorAllow || seen.SessionID != "session" || seen.TurnID != "turn" || seen.ToolUseID != "tool" {
		t.Fatalf("permission=%+v request=%+v", permission, seen)
	}
	answers, _ := permission.UpdatedInput["answers"].(map[string]any)
	if answers["Choose several?"] != "Alpha, custom answer" || strings.Contains(answers["Choose several?"].(string), "forged") {
		t.Fatalf("trusted answers = %#v", answers)
	}
	result, err := tool.Execute(context.Background(), permission.UpdatedInput)
	if err != nil || result.IsError || result.Outcome != types.ToolOutcomeSucceeded {
		t.Fatalf("Execute result=%+v err=%v", result, err)
	}
	output, ok := result.Data.(AskUserQuestionOutput)
	if !ok || output.Answers["Choose several?"] != "Alpha, custom answer" || output.Annotations["Choose several?"].Notes != "trusted note" {
		t.Fatalf("typed output = %#v", result.Data)
	}
	questions, _ := permission.UpdatedInput["questions"].([]any)
	if len(questions) == 0 {
		t.Fatalf("updated input lost questions: %#v", permission.UpdatedInput)
	}
}
