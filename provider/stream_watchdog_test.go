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

	// response.created is a complete SSE event and therefore renews the stream
	// activity deadline even though it is not a model output token.
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
	if elapsed := time.Since(started); elapsed < 5*time.Millisecond || elapsed > 250*time.Millisecond {
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
		t.Fatal("stream_idle_timeout must consume the stream-reconnect budget")
	}
}

func TestStreamWatchdogHeartbeatsDoNotExtendFirstOutputDeadline(t *testing.T) {
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

	started := time.Now()
	if _, err := io.WriteString(writer, ": keep-alive\n\n"); err != nil {
		t.Fatalf("write first heartbeat: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	if _, err := io.WriteString(writer, ": keep-alive\n\n"); err != nil {
		t.Fatalf("write second heartbeat: %v", err)
	}
	select {
	case err := <-readDone:
		var idle *StreamIdleTimeoutError
		if !errors.As(err, &idle) || idle.Phase != streamWatchdogAwaitingOutput {
			t.Fatalf("heartbeat stream error = %#v, want initial idle timeout", err)
		}
		if elapsed := time.Since(started); elapsed < 25*time.Millisecond || elapsed > 250*time.Millisecond {
			t.Fatalf("heartbeat deadline elapsed = %s, want original initial deadline", elapsed)
		}
	case <-time.After(time.Second):
		t.Fatal("heartbeats extended the first-output deadline")
	}
}

func TestStreamWatchdogHeartbeatsDoNotExtendActiveOutputDeadline(t *testing.T) {
	reader, writer := io.Pipe()
	watchdog := newStreamWatchdogBody(reader, streamWatchdogConfig{
		initialIdle: 300 * time.Millisecond,
		activeIdle:  50 * time.Millisecond,
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

	watchdog.markActivity()
	started := time.Now()
	if _, err := io.WriteString(writer, ": keep-alive\n\n"); err != nil {
		t.Fatalf("write first active heartbeat: %v", err)
	}
	time.Sleep(35 * time.Millisecond)
	if _, err := io.WriteString(writer, ": keep-alive\n\n"); err != nil {
		t.Fatalf("write second active heartbeat: %v", err)
	}
	select {
	case err := <-readDone:
		var idle *StreamIdleTimeoutError
		if !errors.As(err, &idle) || idle.Phase != streamWatchdogActive {
			t.Fatalf("active heartbeat stream error = %#v, want active idle timeout", err)
		}
		if elapsed := time.Since(started); elapsed < 40*time.Millisecond || elapsed > 80*time.Millisecond {
			t.Fatalf("active heartbeat deadline elapsed = %s, want original stream-activity deadline", elapsed)
		}
	case <-time.After(time.Second):
		t.Fatal("heartbeats extended the active-output deadline")
	}
}

func TestResponsesStreamWatchdogRenewsOnEveryCompleteEvent(t *testing.T) {
	reader, writer := io.Pipe()
	watchdog := newStreamWatchdogBody(reader, streamWatchdogConfig{
		initialIdle: 300 * time.Millisecond,
		activeIdle:  50 * time.Millisecond,
	})
	t.Cleanup(func() {
		_ = watchdog.Close()
		_ = writer.Close()
	})

	events := make(chan types.StreamEvent, 32)
	go func() {
		(&ResponsesProvider{}).processResponsesStream(context.Background(), watchdog, events)
		close(events)
	}()

	prefix := "event: response.created\ndata: {\"id\":\"resp_progress\"}\n\n"
	if _, err := io.WriteString(writer, prefix); err != nil {
		t.Fatalf("write first lifecycle event: %v", err)
	}
	started := time.Now()
	time.Sleep(35 * time.Millisecond)
	if _, err := io.WriteString(writer, "event: response.in_progress\ndata: {\"type\":\"response.in_progress\"}\n\n"); err != nil {
		t.Fatalf("write second lifecycle event: %v", err)
	}

	var gotErr *types.APIError
	for event := range events {
		if event.Type == types.EventError {
			gotErr = event.Error
		}
	}
	if gotErr == nil || gotErr.Type != "stream_idle_timeout" {
		t.Fatalf("terminal error = %#v, want stream_idle_timeout", gotErr)
	}
	if elapsed := time.Since(started); elapsed < 70*time.Millisecond || elapsed > 250*time.Millisecond {
		t.Fatalf("renewed activity deadline elapsed = %s, want second event to renew active timeout", elapsed)
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

func TestResponsesClientUsesBoundedDefaultResponseHeaderTimeout(t *testing.T) {
	p := NewResponses(Config{})
	transport, ok := p.client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport = %T, want *http.Transport", p.client.Transport)
	}
	if transport.ResponseHeaderTimeout != defaultResponsesInitialIdleTimeout {
		t.Fatalf("ResponseHeaderTimeout = %s, want %s", transport.ResponseHeaderTimeout, defaultResponsesInitialIdleTimeout)
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

func TestResponsesStreamWatchdogUnifiedConfigMatchesCodexPolicy(t *testing.T) {
	t.Setenv(responsesStreamIdleTimeoutEnv, "1750")
	t.Setenv(responsesInitialIdleTimeoutEnv, "100")
	t.Setenv(responsesActiveIdleTimeoutEnv, "200")
	config := responsesStreamWatchdogConfig()
	if config.initialIdle != 1750*time.Millisecond || config.activeIdle != 1750*time.Millisecond {
		t.Fatalf("unified stream idle config = %+v, want 1.75s for both phases", config)
	}
}
