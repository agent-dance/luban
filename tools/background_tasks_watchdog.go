package tools

import (
	"os"
	"sync"
	"time"
)

// TK-04: BackgroundTaskWatchdog scans the live task list and flags tasks
// that have been "running" longer than HardDeadline, or that haven't
// produced new stdout for IdleTimeout. Detected tasks have a warning
// recorded in their Error field; if AutoStop is true, they are
// terminated with status="failed".
//
// The watchdog is opt-in: callers must invoke Start to launch the
// monitoring goroutine. Stop ends the goroutine and is safe to call
// concurrently. Watchdog state is independent of the rest of
// BackgroundTaskManager — it only reads/writes BackgroundTask fields
// through their existing accessors and the shared output file mtime.

// BackgroundTaskWatchdogConfig tunes the watchdog. Zero values disable
// the corresponding check.
type BackgroundTaskWatchdogConfig struct {
	// IdleTimeout is the maximum allowed gap between stdout writes for
	// a running shell task. When the output file's mtime is older than
	// IdleTimeout the task is flagged. Set to 0 to disable.
	IdleTimeout time.Duration
	// HardDeadline is the maximum total runtime for any task. Tasks that
	// have been "running" for longer than HardDeadline are flagged. Set
	// to 0 to disable.
	HardDeadline time.Duration
	// AutoStop, when true, calls Manager.Stop on flagged tasks. When
	// false the watchdog only records the diagnostic in the task's
	// Error field so operators can investigate.
	AutoStop bool
	// PollInterval is how often the watchdog scans the task list.
	// Defaults to 1 minute when zero.
	PollInterval time.Duration
}

// BackgroundTaskWatchdog is the runtime monitor.
type BackgroundTaskWatchdog struct {
	mgr   *BackgroundTaskManager
	cfg   BackgroundTaskWatchdogConfig
	mu    sync.Mutex
	stop  chan struct{}
	done  chan struct{}
	clock func() time.Time
}

// NewBackgroundTaskWatchdog returns an unstarted watchdog.
func NewBackgroundTaskWatchdog(mgr *BackgroundTaskManager, cfg BackgroundTaskWatchdogConfig) *BackgroundTaskWatchdog {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = time.Minute
	}
	return &BackgroundTaskWatchdog{
		mgr:   mgr,
		cfg:   cfg,
		clock: time.Now,
	}
}

// SetClock overrides the time source (test-only).
func (w *BackgroundTaskWatchdog) SetClock(fn func() time.Time) {
	if fn == nil {
		return
	}
	w.mu.Lock()
	w.clock = fn
	w.mu.Unlock()
}

// Start launches the watchdog loop in a goroutine. Calling Start twice
// without Stop is a no-op.
func (w *BackgroundTaskWatchdog) Start() {
	w.mu.Lock()
	if w.stop != nil {
		w.mu.Unlock()
		return
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	w.stop = stop
	w.done = done
	interval := w.cfg.PollInterval
	w.mu.Unlock()

	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				w.Scan()
			}
		}
	}()
}

// Stop terminates the watchdog and waits for the goroutine to exit.
func (w *BackgroundTaskWatchdog) Stop() {
	w.mu.Lock()
	stop := w.stop
	done := w.done
	w.stop = nil
	w.done = nil
	w.mu.Unlock()
	if stop != nil {
		close(stop)
	}
	if done != nil {
		<-done
	}
}

// Scan inspects every running task and flags / stops those exceeding the
// configured thresholds. Returns the IDs of tasks flagged on this scan.
func (w *BackgroundTaskWatchdog) Scan() []string {
	if w.mgr == nil {
		return nil
	}
	w.mu.Lock()
	clock := w.clock
	cfg := w.cfg
	w.mu.Unlock()

	now := clock()
	flagged := make([]string, 0)

	w.mgr.mu.Lock()
	tasks := make([]*BackgroundTask, 0, len(w.mgr.tasks))
	for _, t := range w.mgr.tasks {
		tasks = append(tasks, t)
	}
	w.mgr.mu.Unlock()

	for _, t := range tasks {
		t.mu.RLock()
		status := t.Status
		startedAt := t.StartedAt
		outputPath := t.OutputPath
		err := t.Error
		t.mu.RUnlock()

		if status != "running" {
			continue
		}

		var diagnostic string
		// Hard-deadline check.
		if cfg.HardDeadline > 0 && !startedAt.IsZero() &&
			now.Sub(startedAt) > cfg.HardDeadline {
			diagnostic = "watchdog: hard deadline exceeded after " +
				now.Sub(startedAt).Round(time.Second).String()
		}
		// Idle-output check (file mtime).
		if diagnostic == "" && cfg.IdleTimeout > 0 && outputPath != "" {
			if info, statErr := os.Stat(outputPath); statErr == nil {
				if mt := info.ModTime(); !mt.IsZero() && now.Sub(mt) > cfg.IdleTimeout {
					diagnostic = "watchdog: no stdout for " +
						now.Sub(mt).Round(time.Second).String()
				}
			}
		}

		if diagnostic == "" {
			continue
		}

		t.mu.Lock()
		if err == "" {
			t.Error = diagnostic
		}
		t.mu.Unlock()
		flagged = append(flagged, t.ID)

		if cfg.AutoStop {
			_, _ = w.mgr.Stop(t.ID)
		}
	}
	return flagged
}
