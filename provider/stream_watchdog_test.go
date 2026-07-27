package provider

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/agent-dance/luban/types"
)

func TestResponsesStreamWatchdogBeforeFirstOutput(t *testing.T) {
	reader, writer := io.Pipe()
	watchdog := newStreamWatchdogBody(reader, streamWatchdogConfig{
		initialIdle: 30 * time.Millisecond,
		activeIdle:  10 * time.Millisecond,
	})
	t.Cleanup(func() {
		_ = watchdog.Close()
		_ = writer.Close()
	})

	events := make(chan types.StreamEvent, 8)
	go func() {
		(&ResponsesProvider{}).processResponsesStream(context.Background(), watchdog, events)
		close(events)
	}()

	// response.created is transport activity, but not a model output token. The
	// longer initial phase must remain in force until a non-empty delta arrives.
	if _, err := io.WriteString(writer, "event: response.created\ndata: {\"id\":\"resp_wait\"}\n\n"); err != nil {
		t.Fatalf("write response.created: %v", err)
	}

	started := time.Now()
	var gotStart bool
	var gotErr *types.APIError
	for event := range events {
		switch event.Type {
		case types.EventMessageStart:
			gotStart = true
		case types.EventError:
			gotErr = event.Error
		}
	}
	if !gotStart {
		t.Fatal("response.created was not projected as MessageStart")
	}
	if gotErr == nil || gotErr.Type != "stream_idle_timeout" {
		t.Fatalf("terminal error = %#v, want stream_idle_timeout", gotErr)
	}
	if elapsed := time.Since(started); elapsed < 15*time.Millisecond || elapsed > 250*time.Millisecond {
		t.Fatalf("initial watchdog elapsed = %s, want bounded idle deadline", elapsed)
	}
}

func TestResponsesStreamWatchdogAfterOutputUsesActiveDeadline(t *testing.T) {
	reader, writer := io.Pipe()
	watchdog := newStreamWatchdogBody(reader, streamWatchdogConfig{
		initialIdle: 300 * time.Millisecond,
		activeIdle:  35 * time.Millisecond,
	})
	t.Cleanup(func() {
		_ = watchdog.Close()
		_ = writer.Close()
	})

	events := make(chan types.StreamEvent, 16)
	go func() {
		(&ResponsesProvider{}).processResponsesStream(context.Background(), watchdog, events)
		close(events)
	}()

	streamPrefix := "event: response.created\ndata: {\"id\":\"resp_active\"}\n\n" +
		"event: response.output_item.added\ndata: {\"output_index\":0,\"item\":{\"type\":\"message\"}}\n\n" +
		"event: response.content_part.added\ndata: {\"output_index\":0,\"content_index\":0,\"part\":{\"type\":\"output_text\"}}\n\n" +
		"event: response.output_text.delta\ndata: {\"output_index\":0,\"content_index\":0,\"delta\":\"first token\"}\n\n"
	if _, err := io.WriteString(writer, streamPrefix); err != nil {
		t.Fatalf("write stream prefix: %v", err)
	}

	started := time.Now()
	var gotDelta bool
	var gotErr *types.APIError
	for event := range events {
		if event.Type == types.EventContentBlockDelta && event.Delta != nil && event.Delta.Text == "first token" {
			gotDelta = true
		}
		if event.Type == types.EventError {
			gotErr = event.Error
		}
	}
	if !gotDelta {
		t.Fatal("first output delta was not delivered before timeout")
	}
	if gotErr == nil || gotErr.Type != "stream_idle_timeout" {
		t.Fatalf("terminal error = %#v, want stream_idle_timeout", gotErr)
	}
	if elapsed := time.Since(started); elapsed >= 200*time.Millisecond {
		t.Fatalf("active watchdog elapsed = %s; initial deadline was used", elapsed)
	}
	if !IsRetryable(gotErr) {
		t.Fatal("stream_idle_timeout must consume the shared replay budget")
	}
}

func TestStreamWatchdogCountsSSEHeartbeatsAsProgress(t *testing.T) {
	reader, writer := io.Pipe()
	watchdog := newStreamWatchdogBody(reader, streamWatchdogConfig{
		initialIdle: 35 * time.Millisecond,
		activeIdle:  20 * time.Millisecond,
	})
	t.Cleanup(func() {
		_ = watchdog.Close()
		_ = writer.Close()
	})

	readDone := make(chan error, 1)
	go func() {
		_, err := io.Copy(io.Discard, watchdog)
		readDone <- err
	}()

	// The total stream duration exceeds initialIdle several times, but every raw
	// SSE comment is evidence that the transport is alive.
	for range 7 {
		if _, err := io.WriteString(writer, ": keep-alive\n\n"); err != nil {
			t.Fatalf("write heartbeat: %v", err)
		}
		time.Sleep(12 * time.Millisecond)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	select {
	case err := <-readDone:
		if err != nil {
			var idle *StreamIdleTimeoutError
			if errors.As(err, &idle) {
				t.Fatalf("healthy heartbeat stream timed out: %v", err)
			}
			t.Fatalf("read heartbeat stream: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("heartbeat stream did not finish")
	}
}

func TestResponsesClientHasNoRequestWideTimeout(t *testing.T) {
	p := NewResponses(Config{Timeout: 7})
	if p.client.Timeout != 0 {
		t.Fatalf("http.Client.Timeout = %s, want zero progress-based streaming", p.client.Timeout)
	}
	transport, ok := p.client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport = %T, want *http.Transport", p.client.Transport)
	}
	if transport.ResponseHeaderTimeout != 7*time.Second {
		t.Fatalf("ResponseHeaderTimeout = %s, want 7s", transport.ResponseHeaderTimeout)
	}
}

func TestResponsesStreamWatchdogConfigIsBounded(t *testing.T) {
	t.Setenv(responsesInitialIdleTimeoutEnv, "999999999")
	t.Setenv(responsesActiveIdleTimeoutEnv, "999999999")
	config := responsesStreamWatchdogConfig()
	if config.initialIdle != maxResponsesInitialIdleTimeout || config.activeIdle != maxResponsesActiveIdleTimeout {
		t.Fatalf("bounded config = %+v", config)
	}

	t.Setenv(responsesInitialIdleTimeoutEnv, "invalid")
	t.Setenv(responsesActiveIdleTimeoutEnv, "0")
	config = responsesStreamWatchdogConfig()
	if config.initialIdle != defaultResponsesInitialIdleTimeout || config.activeIdle != defaultResponsesActiveIdleTimeout {
		t.Fatalf("fallback config = %+v", config)
	}
}
