package ui

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/agent-dance/luban/internal/contracts/interaction"
)

func screenReaderAskUserRequest(id string, questions ...interaction.QuestionSpec) interaction.AskUserInteractionRequest {
	return interaction.AskUserInteractionRequest{RequestID: id, SessionID: "session", TurnID: "turn", ToolUseID: "tool", Questions: questions}
}

func TestScreenReaderAskUserUsesSingleInputOwnerForMultiSelectCustomAndCommandResume(t *testing.T) {
	reader, writer := io.Pipe()
	defer writer.Close()
	output := newScreenReaderTestBuffer()
	renderer := NewScreenReaderRenderer(output, reader)
	t.Cleanup(func() { _ = renderer.Close() })
	command := make(chan string, 1)
	go func() {
		line, _ := renderer.ReadCommand(context.Background())
		command <- line
	}()
	waitForScreenReaderOutput(t, output, "Input:")

	question := interaction.QuestionSpec{
		Question: "Choose several?", Header: "Choice", MultiSelect: true,
		Options: []interaction.OptionSpec{{Label: "Alpha", Description: "First"}, {Label: "Beta", Description: "Second"}},
	}
	returned := make(chan interaction.AskUserInteractionResponse, 1)
	errs := make(chan error, 1)
	go func() {
		response, err := renderer.AskUserQuestions(context.Background(), screenReaderAskUserRequest("ask-screen", question))
		returned <- response
		errs <- err
	}()
	waitForScreenReaderOutput(t, output, "decision ask-screen:1")
	if _, err := io.WriteString(writer, "decision ask-screen:1 Alpha,Other:Zig build n:trusted note\n"); err != nil {
		t.Fatal(err)
	}
	if err := <-errs; err != nil {
		t.Fatal(err)
	}
	response := <-returned
	answer := response.Answers[question.Question]
	if response.Outcome != interaction.AskUserInteractionCompleted || strings.Join(answer.Selection, ",") != "Alpha" || answer.OtherText != "Zig build" {
		t.Fatalf("screen-reader answer = %+v response=%+v", answer, response)
	}
	if response.Annotations[question.Question].Notes != "trusted note" {
		t.Fatalf("screen-reader notes = %+v", response.Annotations)
	}
	waitForScreenReaderOutput(t, output, "Command input resumed")
	if _, err := io.WriteString(writer, "next command\n"); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-command:
		if got != "next command" {
			t.Fatalf("resumed command = %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("command input did not resume after AskUser")
	}
}

func TestScreenReaderAskUserCancellationAndUntrustedControlsAreSafe(t *testing.T) {
	reader, writer := io.Pipe()
	defer writer.Close()
	output := newScreenReaderTestBuffer()
	renderer := NewScreenReaderRenderer(output, reader)
	t.Cleanup(func() { _ = renderer.Close() })
	question := interaction.QuestionSpec{
		Question: "Choose\x1b]0;bad\a?", Header: "Head\u009b", MultiSelect: false,
		Options: []interaction.OptionSpec{{Label: "Alpha\x00", Description: "First\r"}, {Label: "Beta", Description: "Second", Preview: "Preview\x1b[31m"}},
	}
	returned := make(chan interaction.AskUserInteractionResponse, 1)
	go func() {
		response, _ := renderer.AskUserQuestions(context.Background(), screenReaderAskUserRequest("ask-control", question))
		returned <- response
	}()
	waitForScreenReaderOutput(t, output, "decision ask-control:1")
	text := output.String()
	for _, raw := range []rune{'\x00', '\x07', '\x1b', '\x9b', '\r'} {
		if strings.ContainsRune(text, raw) {
			t.Fatalf("screen-reader AskUser retained control U+%04X: %q", raw, text)
		}
	}
	for _, escaped := range []string{`\x00`, `\x07`, `\x1b`, `\u009b`, `\r`} {
		if !strings.Contains(text, escaped) {
			t.Fatalf("screen-reader AskUser omitted safe control %q: %q", escaped, text)
		}
	}
	if _, err := io.WriteString(writer, "decision ask-control:1 escape\n"); err != nil {
		t.Fatal(err)
	}
	select {
	case response := <-returned:
		if response.Outcome != interaction.AskUserInteractionCancelled || len(response.Answers) != 0 {
			t.Fatalf("cancelled screen-reader AskUser = %+v", response)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled screen-reader AskUser leaked")
	}
}

func TestScreenReaderAskUserRejectsWrongSessionBeforeWriting(t *testing.T) {
	output := newScreenReaderTestBuffer()
	renderer := NewScreenReaderRenderer(output, strings.NewReader(""))
	t.Cleanup(func() { _ = renderer.Close() })
	renderer.SetSessionIdentityResolver(func() string { return "visible-session" })
	request := screenReaderAskUserRequest("wrong-session", interaction.QuestionSpec{
		Question: "Should stay private?", Header: "Private", Options: []interaction.OptionSpec{{Label: "Alpha", Description: "First"}, {Label: "Beta", Description: "Second"}},
	})
	request.SessionID = "old-session"
	response, err := renderer.AskUserQuestions(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if response.Outcome != interaction.AskUserInteractionStale || output.String() != "" {
		t.Fatalf("wrong-session response=%+v output=%q", response, output.String())
	}
}
