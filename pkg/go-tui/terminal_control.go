package tui

import (
	"errors"
	"sync"
)

// TerminalControlSink is the minimal interface for emitting terminal control
// sequences without bypassing the active terminal owner.
type TerminalControlSink interface {
	WriteTerminalControl([]byte) error
}

// TerminalControlRejection is a bounded owner-channel failure reason. It does
// not carry the attempted control bytes or any application identity.
type TerminalControlRejection string

const (
	TerminalControlNoOwner     TerminalControlRejection = "no_owner"
	TerminalControlUnavailable TerminalControlRejection = "owner_unavailable"
	TerminalControlWriteError  TerminalControlRejection = "write_error"
)

// TerminalControlObserver receives rejected owner-channel writes. Callbacks
// run after terminal/lease locks are released and must remain non-blocking.
type TerminalControlObserver func(TerminalControlRejection)

var (
	// ErrNoTerminalControlOwner indicates that no component currently owns the
	// terminal control channel.
	ErrNoTerminalControlOwner = errors.New("tui: no terminal control owner")

	// ErrTerminalControlUnavailable indicates that the owning app has handed
	// the terminal to another process, is suspended, or is stopping.
	ErrTerminalControlUnavailable = errors.New("tui: terminal control unavailable")
)

type terminalControlLease struct {
	id   uint64
	sink TerminalControlSink
}

var terminalControls struct {
	sync.RWMutex
	nextID uint64
	leases []terminalControlLease
}

type terminalControlObserverLease struct {
	id       uint64
	observer TerminalControlObserver
}

var terminalControlObservers struct {
	sync.RWMutex
	nextID uint64
	leases []terminalControlObserverLease
}

// InstallTerminalControlObserver installs a temporary process observer and
// returns an idempotent release function. All installed observers receive each
// rejection, so a diagnostic adapter cannot suppress process metrics.
func InstallTerminalControlObserver(observer TerminalControlObserver) func() {
	if observer == nil {
		return func() {}
	}
	terminalControlObservers.Lock()
	terminalControlObservers.nextID++
	lease := terminalControlObserverLease{id: terminalControlObservers.nextID, observer: observer}
	terminalControlObservers.leases = append(terminalControlObservers.leases, lease)
	terminalControlObservers.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			terminalControlObservers.Lock()
			defer terminalControlObservers.Unlock()
			for i := len(terminalControlObservers.leases) - 1; i >= 0; i-- {
				if terminalControlObservers.leases[i].id == lease.id {
					terminalControlObservers.leases = append(
						terminalControlObservers.leases[:i],
						terminalControlObservers.leases[i+1:]...,
					)
					return
				}
			}
		})
	}
}

func observeTerminalControlRejection(reason TerminalControlRejection) {
	terminalControlObservers.RLock()
	observers := append([]terminalControlObserverLease(nil), terminalControlObservers.leases...)
	terminalControlObservers.RUnlock()
	for _, lease := range observers {
		func(observer TerminalControlObserver) {
			defer func() { _ = recover() }()
			observer(reason)
		}(lease.observer)
	}
}

// InstallTerminalControlSink installs sink as the current terminal-control
// owner and returns an idempotent release function. Leases form a stack so a
// temporary injected sink can be removed without losing the previous owner.
func InstallTerminalControlSink(sink TerminalControlSink) func() {
	if sink == nil {
		return func() {}
	}

	terminalControls.Lock()
	terminalControls.nextID++
	lease := terminalControlLease{id: terminalControls.nextID, sink: sink}
	terminalControls.leases = append(terminalControls.leases, lease)
	terminalControls.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			terminalControls.Lock()
			defer terminalControls.Unlock()
			for i := len(terminalControls.leases) - 1; i >= 0; i-- {
				if terminalControls.leases[i].id != lease.id {
					continue
				}
				terminalControls.leases = append(
					terminalControls.leases[:i],
					terminalControls.leases[i+1:]...,
				)
				return
			}
		})
	}
}

// WriteTerminalControl emits one complete control sequence through the active
// owner. It never falls back to stdout or stderr.
func WriteTerminalControl(sequence []byte) error {
	if len(sequence) == 0 {
		return nil
	}

	terminalControls.RLock()
	if len(terminalControls.leases) == 0 {
		terminalControls.RUnlock()
		observeTerminalControlRejection(TerminalControlNoOwner)
		return ErrNoTerminalControlOwner
	}
	sink := terminalControls.leases[len(terminalControls.leases)-1].sink
	terminalControls.RUnlock()
	err := sink.WriteTerminalControl(sequence)
	if err != nil {
		if _, appSink := sink.(*App); !appSink {
			observeTerminalControlRejection(TerminalControlWriteError)
		}
	}
	return err
}

// WriteTerminalControl serializes a control sequence with rendering and
// terminal lifecycle transitions. Calls fail closed while the App does not
// own a usable terminal.
func (a *App) WriteTerminalControl(sequence []byte) error {
	if len(sequence) == 0 {
		return nil
	}

	a.terminalMu.Lock()
	if a.terminal == nil || !a.opened.Load() || a.externalActive.Load() ||
		a.selfSuspended.Load() || a.terminalSuspended.Load() || a.stopping.Load() {
		a.terminalMu.Unlock()
		observeTerminalControlRejection(TerminalControlUnavailable)
		return ErrTerminalControlUnavailable
	}
	_, err := a.terminal.WriteDirect(sequence)
	a.terminalMu.Unlock()
	if err != nil {
		observeTerminalControlRejection(TerminalControlWriteError)
	}
	return err
}

func (a *App) installTerminalControlSink() {
	a.terminalControlLeaseMu.Lock()
	defer a.terminalControlLeaseMu.Unlock()
	if a.releaseTerminalControl != nil {
		return
	}
	a.releaseTerminalControl = InstallTerminalControlSink(a)
}

func (a *App) releaseTerminalControlSink() {
	a.terminalControlLeaseMu.Lock()
	release := a.releaseTerminalControl
	a.releaseTerminalControl = nil
	a.terminalControlLeaseMu.Unlock()
	if release != nil {
		release()
	}
}
