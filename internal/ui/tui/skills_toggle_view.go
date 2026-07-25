package tui

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/skills"
)

// SkillsToggleNoticeKind keeps backend transaction truth separate from
// presentation copy. In particular, a returned error never turns a typed
// rollback or degraded result into an apparent commit.
type SkillsToggleNoticeKind uint8

const (
	skillsToggleNoticeNone SkillsToggleNoticeKind = iota
	skillsToggleNoticeLoading
	skillsToggleNoticeUpdating
	skillsToggleNoticeRefreshed
	skillsToggleNoticeLoadFailed
	skillsToggleNoticeBackendUnavailable
	skillsToggleNoticeSessionUnavailable
	skillsToggleNoticeInvalidResult
	skillsToggleNoticeReadOnly
	skillsToggleNoticeStale
	skillsToggleNoticeUnknown
	skillsToggleNoticeSessionOverride
	skillsToggleNoticePersistenceFailed
	skillsToggleNoticeRolledBack
	skillsToggleNoticeDegraded
	skillsToggleNoticeRefreshFailed
	skillsToggleNoticeUnexpected
)

// SkillsToggleNotice contains only typed state and raw values. Copy is
// resolved at render time from AppState.Language, so switching languages while
// the menu is open immediately updates every status line.
type SkillsToggleNotice struct {
	Kind           SkillsToggleNoticeKind
	SkillID        skills.SkillID
	Name           string
	Visibility     skills.Visibility
	Scope          skills.SkillScope
	ReadOnlyReason string
	Revision       skills.CatalogRevision
	Err            error
}

// SkillsToggleViewState is the searchable direct checklist. Snapshot is the last
// authoritative catalog accepted from Snapshot or ToggleProjectVisibility.
// It is never edited locally to predict a toggle.
type SkillsToggleViewState struct {
	HasSnapshot bool
	Snapshot    skills.CatalogSnapshot
	SessionID   string
	Query       string
	Filtered    []int
	Selected    int
	Loading     bool
	Refreshing  bool
	// RefreshRequired gates Space after a degraded transaction. Until a fresh
	// Snapshot succeeds, Space may retry the read but must never issue another
	// mutation against uncertain state.
	RefreshRequired bool
	Notice          SkillsToggleNotice
	operation       uint64
}

func (state SkillsToggleViewState) clone() SkillsToggleViewState {
	state.Snapshot = state.Snapshot.Clone()
	state.Filtered = append([]int(nil), state.Filtered...)
	return state
}

func (state *SkillsToggleViewState) cancelPending() {
	state.operation++
	state.Loading = false
	state.Refreshing = false
}

func (state *SkillsToggleViewState) beginLoad(sessionID string) uint64 {
	state.operation++
	state.Loading = true
	state.Refreshing = false
	state.SessionID = sessionID
	state.Notice = SkillsToggleNotice{Kind: skillsToggleNoticeLoading}
	return state.operation
}

func (state *SkillsToggleViewState) beginToggle(row skills.EffectiveSkill) uint64 {
	state.operation++
	state.Loading = true
	state.Refreshing = false
	state.Notice = SkillsToggleNotice{
		Kind:       skillsToggleNoticeUpdating,
		SkillID:    row.ID,
		Name:       row.Name,
		Visibility: row.Visibility,
		Scope:      row.VisibilitySource,
		Revision:   state.Snapshot.Revision,
	}
	return state.operation
}

func (state *SkillsToggleViewState) beginRefresh() uint64 {
	state.operation++
	state.Loading = true
	state.Refreshing = true
	state.RefreshRequired = true
	return state.operation
}

func (state *SkillsToggleViewState) acceptSnapshot(operation uint64, sessionID string, snapshot skills.CatalogSnapshot, loadErr error) bool {
	if state.operation != operation {
		return false
	}
	state.Loading = false
	state.Refreshing = false
	if loadErr != nil {
		state.Notice = SkillsToggleNotice{Kind: skillsToggleNoticeLoadFailed, Err: loadErr}
		return true
	}
	if err := snapshot.Validate(); err != nil {
		state.Notice = SkillsToggleNotice{Kind: skillsToggleNoticeLoadFailed, Err: err}
		return true
	}
	state.SessionID = sessionID
	state.installSnapshot(snapshot, "")
	state.RefreshRequired = false
	state.Notice = SkillsToggleNotice{}
	return true
}

func (state *SkillsToggleViewState) acceptRefresh(operation uint64, sessionID string, snapshot skills.CatalogSnapshot, refreshErr error) bool {
	if state.operation != operation {
		return false
	}
	state.Loading = false
	state.Refreshing = false
	if refreshErr != nil {
		state.RefreshRequired = true
		state.Notice.Kind = skillsToggleNoticeRefreshFailed
		state.Notice.Err = refreshErr
		return true
	}
	if err := snapshot.Validate(); err != nil {
		state.RefreshRequired = true
		state.Notice.Kind = skillsToggleNoticeRefreshFailed
		state.Notice.Err = err
		return true
	}
	state.SessionID = sessionID
	state.installSnapshot(snapshot, state.Notice.SkillID)
	state.RefreshRequired = false
	state.Notice.Kind = skillsToggleNoticeRefreshed
	state.Notice.Revision = snapshot.Revision
	state.Notice.Err = nil
	return true
}

func (state *SkillsToggleViewState) acceptToggle(operation uint64, result skills.ProjectVisibilityToggleResult, toggleErr error) bool {
	if state.operation != operation {
		return false
	}
	state.Loading = false
	state.Refreshing = false
	if validationErr := result.Validate(); validationErr != nil {
		state.Notice = SkillsToggleNotice{
			Kind: skillsToggleNoticeInvalidResult,
			Err:  errors.Join(toggleErr, validationErr),
		}
		return true
	}

	// The selected stable ID is preserved when possible, but every displayed
	// field and the observed revision come from the authoritative result.
	state.installSnapshot(result.Snapshot, result.RequestedSkillID)
	state.RefreshRequired = false
	notice := SkillsToggleNotice{
		SkillID:  result.RequestedSkillID,
		Revision: result.CurrentRevision,
		Err:      toggleErr,
	}
	if result.Skill != nil {
		notice.Name = result.Skill.Name
		notice.Visibility = result.Skill.Visibility
		notice.Scope = result.Skill.VisibilitySource
		notice.ReadOnlyReason = result.Skill.ReadOnlyReason
	}
	switch result.Outcome {
	case skills.ProjectVisibilityToggleCommitted:
		// The authoritative checkbox redraw is sufficient success feedback.
		// Keep notices for failures and recovery actions only.
		notice = SkillsToggleNotice{}
	case skills.ProjectVisibilityToggleRejected:
		switch result.Reason {
		case skills.ProjectVisibilityToggleReasonStaleRevision:
			notice.Kind = skillsToggleNoticeStale
		case skills.ProjectVisibilityToggleReasonUnknownSkill:
			notice.Kind = skillsToggleNoticeUnknown
		case skills.ProjectVisibilityToggleReasonSessionOverride:
			notice.Kind = skillsToggleNoticeSessionOverride
		case skills.ProjectVisibilityToggleReasonReadOnly:
			notice.Kind = skillsToggleNoticeReadOnly
		case skills.ProjectVisibilityToggleReasonPersistenceFailed:
			notice.Kind = skillsToggleNoticePersistenceFailed
		case skills.ProjectVisibilityToggleReasonLiveApplyRolledBack:
			notice.Kind = skillsToggleNoticeRolledBack
		default:
			notice.Kind = skillsToggleNoticeUnexpected
		}
	case skills.ProjectVisibilityToggleDegraded:
		state.RefreshRequired = true
		if result.Reason == skills.ProjectVisibilityToggleReasonAuthoritativeRefresh {
			notice.Kind = skillsToggleNoticeRefreshFailed
		} else {
			notice.Kind = skillsToggleNoticeDegraded
		}
	default:
		notice.Kind = skillsToggleNoticeUnexpected
	}
	state.Notice = notice
	return true
}

func (state *SkillsToggleViewState) rejectReadOnly(row skills.EffectiveSkill) {
	state.Notice = SkillsToggleNotice{
		Kind:           skillsToggleNoticeReadOnly,
		SkillID:        row.ID,
		Name:           row.Name,
		Visibility:     row.Visibility,
		Scope:          row.VisibilitySource,
		ReadOnlyReason: row.ReadOnlyReason,
		Revision:       state.Snapshot.Revision,
	}
}

func (state *SkillsToggleViewState) installSnapshot(snapshot skills.CatalogSnapshot, preferred skills.SkillID) {
	if preferred == "" {
		if selected := state.selectedRow(); selected != nil {
			preferred = selected.ID
		}
	}
	state.HasSnapshot = true
	state.Snapshot = snapshot.Clone()
	state.applyFilter(preferred)
}

func (state *SkillsToggleViewState) applyFilter(preferred skills.SkillID) {
	query := strings.ToLower(strings.TrimSpace(state.Query))
	indices := make([]int, 0, len(state.Snapshot.Skills))
	for index, row := range state.Snapshot.Skills {
		if query == "" || strings.Contains(strings.ToLower(skillsToggleSearchText(row)), query) {
			indices = append(indices, index)
		}
	}
	sort.SliceStable(indices, func(left, right int) bool {
		leftRow, rightRow := state.Snapshot.Skills[indices[left]], state.Snapshot.Skills[indices[right]]
		leftName, rightName := strings.ToLower(leftRow.Name), strings.ToLower(rightRow.Name)
		if leftName != rightName {
			return leftName < rightName
		}
		return leftRow.ID < rightRow.ID
	})
	state.Filtered = indices
	if preferred != "" {
		for visibleIndex, rowIndex := range state.Filtered {
			if state.Snapshot.Skills[rowIndex].ID == preferred {
				state.Selected = visibleIndex
				state.clamp()
				return
			}
		}
	}
	state.clamp()
}

func skillsToggleSearchText(row skills.EffectiveSkill) string {
	return strings.Join([]string{
		string(row.ID), row.Name, row.Summary, string(row.Source), string(row.Locator),
		string(row.Visibility), string(row.VisibilitySource), row.ReadOnlyReason,
	}, " ")
}

func (state *SkillsToggleViewState) appendQuery(char rune) {
	if !unicode.IsPrint(char) || unicode.IsSpace(char) {
		return
	}
	preferred := skills.SkillID("")
	if selected := state.selectedRow(); selected != nil {
		preferred = selected.ID
	}
	state.Query += string(char)
	state.applyFilter(preferred)
}

func (state *SkillsToggleViewState) backspaceQuery() bool {
	if state.Query == "" {
		return false
	}
	preferred := skills.SkillID("")
	if selected := state.selectedRow(); selected != nil {
		preferred = selected.ID
	}
	runes := []rune(state.Query)
	state.Query = string(runes[:len(runes)-1])
	state.applyFilter(preferred)
	return true
}

func (state *SkillsToggleViewState) move(delta int) {
	state.Selected += delta
	state.clamp()
}

func (state *SkillsToggleViewState) clamp() {
	if len(state.Filtered) == 0 {
		state.Selected = 0
		return
	}
	if state.Selected < 0 {
		state.Selected = 0
	}
	if state.Selected >= len(state.Filtered) {
		state.Selected = len(state.Filtered) - 1
	}
}

func (state *SkillsToggleViewState) selectedRow() *skills.EffectiveSkill {
	if len(state.Filtered) == 0 || state.Selected < 0 || state.Selected >= len(state.Filtered) {
		return nil
	}
	index := state.Filtered[state.Selected]
	if index < 0 || index >= len(state.Snapshot.Skills) {
		return nil
	}
	row := state.Snapshot.Skills[index]
	return &row
}

func skillsToggleVisibleRange(total, selected, limit int) (int, int) {
	if total <= 0 || limit <= 0 {
		return 0, 0
	}
	if total <= limit {
		return 0, total
	}
	start := selected - limit/2
	if start < 0 {
		start = 0
	}
	if start > total-limit {
		start = total - limit
	}
	return start, start + limit
}

func formatSkillsToggleNotice(lang i18n.Language, notice SkillsToggleNotice) string {
	name := strings.TrimSpace(notice.Name)
	if name == "" {
		name = string(notice.SkillID)
	}
	switch notice.Kind {
	case skillsToggleNoticeNone:
		return ""
	case skillsToggleNoticeLoading:
		return i18n.Text(lang, i18n.KeySkillsMenuLoading)
	case skillsToggleNoticeUpdating:
		return i18n.Format(lang, i18n.KeySkillsMenuUpdating, name, notice.SkillID)
	case skillsToggleNoticeRefreshed:
		return i18n.Format(lang, i18n.KeySkillsMenuStatusRefreshed, notice.Revision)
	case skillsToggleNoticeLoadFailed:
		return i18n.Format(lang, i18n.KeySkillsMenuLoadFailed, notice.Err)
	case skillsToggleNoticeBackendUnavailable:
		return i18n.Text(lang, i18n.KeySkillsMenuBackendUnavailable)
	case skillsToggleNoticeSessionUnavailable:
		return i18n.Text(lang, i18n.KeySkillsMenuSessionUnavailable)
	case skillsToggleNoticeInvalidResult:
		return i18n.Format(lang, i18n.KeySkillsMenuInvalidResult, notice.Err)
	case skillsToggleNoticeReadOnly:
		reason := skillsReadOnlyReasonInLanguage(lang, notice.ReadOnlyReason)
		if reason == "" {
			reason = i18n.Text(lang, i18n.KeySkillsMenuReadOnlyUnspecified)
		}
		return i18n.Format(lang, i18n.KeySkillsMenuStatusReadOnly, name, notice.SkillID, reason)
	case skillsToggleNoticeStale:
		return i18n.Format(lang, i18n.KeySkillsMenuStatusStale, notice.SkillID, notice.Revision)
	case skillsToggleNoticeUnknown:
		return i18n.Format(lang, i18n.KeySkillsMenuStatusUnknown, notice.SkillID)
	case skillsToggleNoticeSessionOverride:
		command := fmt.Sprintf("/skills reset %s --scope session", notice.SkillID)
		return i18n.Format(lang, i18n.KeySkillsMenuStatusSessionOverride, name, notice.SkillID, command)
	case skillsToggleNoticePersistenceFailed:
		return i18n.Format(lang, i18n.KeySkillsMenuStatusPersistenceFailed, name, notice.SkillID)
	case skillsToggleNoticeRolledBack:
		return i18n.Format(lang, i18n.KeySkillsMenuStatusRolledBack, name, notice.SkillID)
	case skillsToggleNoticeDegraded:
		return i18n.Format(lang, i18n.KeySkillsMenuStatusDegraded, name, notice.SkillID)
	case skillsToggleNoticeRefreshFailed:
		return i18n.Format(lang, i18n.KeySkillsMenuStatusRefreshFailed, name, notice.SkillID)
	case skillsToggleNoticeUnexpected:
		return i18n.Format(lang, i18n.KeySkillsMenuStatusUnexpected, notice.SkillID, notice.Err)
	default:
		return ""
	}
}

func skillsReadOnlyReasonInLanguage(lang i18n.Language, reason string) string {
	trimmed := strings.TrimSpace(reason)
	switch skills.CatalogPolicyReason(trimmed) {
	case skills.CatalogPolicyReasonManagedReadOnly:
		return i18n.Text(lang, i18n.KeyCommandSkillsReadOnlyManaged)
	case skills.CatalogPolicyReasonManagedDeny:
		return i18n.Text(lang, i18n.KeyCommandSkillsReadOnlyDenied)
	default:
		return trimmed
	}
}
