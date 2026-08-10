package app

import (
	"context"
	"sync"

	"github.com/agent-dance/luban/internal/ui/tui"
)

type tuiInputSubmission struct {
	text           string
	images         []tui.ImageAttachment
	imagesCaptured bool
	steering       bool
	ctx            context.Context
	lifecycleCtx   context.Context
	abort          func()
}

// prepareTUIInputAdmission gives the foreground handler a cancelable context
// while retaining the longer-lived TUI context for actions (such as picker
// selections) that intentionally run after the handler returns.
func prepareTUIInputAdmission(runtimeCtx context.Context, state *tui.AppState, submission *tuiInputSubmission) bool {
	admissionCtx, admissionCancel := context.WithCancel(runtimeCtx)
	generation, admitted := state.TryReserveQuery(admissionCancel)
	if !admitted {
		admissionCancel()
		return false
	}
	submission.ctx = withTUIInputAdmission(admissionCtx, generation)
	submission.lifecycleCtx = runtimeCtx
	submission.abort = func() {
		admissionCancel()
		state.ClearQueryCancel(generation)
	}
	return true
}

// tuiInputScheduler serializes interactive submissions. The active handler
// owns the foreground turn; later submissions wait in FIFO order until it
// reaches a terminal boundary.
type tuiInputScheduler struct {
	mu sync.Mutex

	active  bool
	pending []tuiInputSubmission

	launch         func(func()) bool
	prepare        func(*tuiInputSubmission) bool
	run            func(tuiInputSubmission)
	cancelActive   func() bool
	onQueueChanged func([]string)
}

func newTUIInputScheduler(
	launch func(func()) bool,
	prepare func(*tuiInputSubmission) bool,
	run func(tuiInputSubmission),
	cancelActive func() bool,
	onQueueChanged func([]string),
) *tuiInputScheduler {
	return &tuiInputScheduler{
		launch: launch, prepare: prepare, run: run, cancelActive: cancelActive, onQueueChanged: onQueueChanged,
	}
}

// TrySubmit synchronously decides whether a submission owns execution. Callers
// choose whether a busy foreground admits the submission into the FIFO.
func (s *tuiInputScheduler) TrySubmit(text string, captureImages func() []tui.ImageAttachment, allowQueue bool) (accepted, queued bool) {
	if s == nil {
		return false, false
	}
	submission := tuiInputSubmission{text: text}
	s.mu.Lock()
	if s.active {
		if !allowQueue {
			s.mu.Unlock()
			return false, false
		}
		if captureImages != nil {
			submission.imagesCaptured = true
			submission.images = append([]tui.ImageAttachment(nil), captureImages()...)
		}
		s.pending = append(s.pending, submission)
		queued := s.queuedTextsLocked()
		s.mu.Unlock()
		s.publishQueue(queued)
		return true, true
	}
	s.active = true
	if captureImages != nil {
		submission.imagesCaptured = true
		submission.images = append([]tui.ImageAttachment(nil), captureImages()...)
	}
	s.mu.Unlock()
	if !s.launchSubmission(submission) {
		s.stopAndDiscard()
		return false, false
	}
	return true, false
}

// PromoteQueuedToSteering marks the oldest queued message as guidance and
// interrupts the active query. It stays at the front of the FIFO and starts as
// soon as the interrupted turn has committed its terminal state.
func (s *tuiInputScheduler) PromoteQueuedToSteering() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	if !s.active || len(s.pending) == 0 || s.pending[0].steering {
		s.mu.Unlock()
		return false
	}
	s.pending[0].steering = true
	if s.cancelActive == nil || !s.cancelActive() {
		if len(s.pending) > 0 {
			s.pending[0].steering = false
		}
		s.mu.Unlock()
		return false
	}
	s.mu.Unlock()
	return true
}

func (s *tuiInputScheduler) launchSubmission(submission tuiInputSubmission) bool {
	if s.launch == nil || s.run == nil {
		return false
	}
	if s.prepare != nil && !s.prepare(&submission) {
		return false
	}
	launched := s.launch(func() {
		defer s.finishSubmission()
		s.run(submission)
	})
	if !launched && submission.abort != nil {
		submission.abort()
	}
	return launched
}

func (s *tuiInputScheduler) finishSubmission() {
	s.mu.Lock()
	if len(s.pending) == 0 {
		s.active = false
		s.mu.Unlock()
		s.publishQueue(nil)
		return
	}
	next := s.pending[0]
	s.pending = append([]tuiInputSubmission(nil), s.pending[1:]...)
	queued := s.queuedTextsLocked()
	s.mu.Unlock()
	s.publishQueue(queued)
	if !s.launchSubmission(next) {
		s.stopAndDiscard()
	}
}

func (s *tuiInputScheduler) stopAndDiscard() {
	s.mu.Lock()
	s.active = false
	s.pending = nil
	s.mu.Unlock()
	s.publishQueue(nil)
}

func (s *tuiInputScheduler) queuedTextsLocked() []string {
	queued := make([]string, len(s.pending))
	for index, submission := range s.pending {
		queued[index] = submission.text
	}
	return queued
}

func (s *tuiInputScheduler) publishQueue(queued []string) {
	if s.onQueueChanged != nil {
		s.onQueueChanged(queued)
	}
}
