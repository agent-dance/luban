package tui

import (
	"sync/atomic"
)

var skillsMenuSequence atomic.Uint64

// SkillsMenuState is the isolated state for the /skills overlay. Request
// callbacks are deliberately retained as live resolvers: session changes are
// checked again before every catalog read or mutation.
type SkillsMenuState struct {
	Visible bool
	Request SkillsMenuOpenRequest
	Toggle  SkillsToggleViewState
	token   uint64
}

func newSkillsMenuState(request SkillsMenuOpenRequest) *SkillsMenuState {
	return &SkillsMenuState{
		Visible: true,
		Request: request,
		token:   skillsMenuSequence.Add(1),
	}
}

func (state *SkillsMenuState) clone() *SkillsMenuState {
	if state == nil {
		return nil
	}
	copy := *state
	copy.Toggle = state.Toggle.clone()
	return &copy
}

func (state *SkillsMenuState) currentSessionID() string {
	if state == nil || state.Request.SessionID == nil {
		return ""
	}
	return state.Request.SessionID()
}
