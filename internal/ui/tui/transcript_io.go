package tui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/internal/messagecontrol"
	"github.com/agent-dance/luban/types"
)

type TranscriptScrollAnchor struct {
	ObservationID string
	RowOffset     int
}

type TranscriptViewState struct {
	FocusTarget  string
	ScrollAnchor TranscriptScrollAnchor
}

type TranscriptSearchMatch struct {
	ObservationID string
	EvidenceRef   DetailRef
	ByteOffset    int
}

type TranscriptSearchController struct {
	mu               sync.Mutex
	observations     *ObservationStore
	details          DetailStore
	messages         []Message
	matches          []TranscriptSearchMatch
	current          int
	returnTo         TranscriptViewState
	returnDisclosure map[string]DisclosureState
}

func NewTranscriptSearchController(observations *ObservationStore, details DetailStore, messageSets ...[]Message) *TranscriptSearchController {
	var messages []Message
	if len(messageSets) > 0 {
		messages = append([]Message(nil), messageSets[0]...)
	}
	return &TranscriptSearchController{observations: observations, details: details, messages: messages, current: -1}
}

func (s *TranscriptSearchController) Open(query string, returnTo TranscriptViewState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.returnTo = returnTo
	s.returnDisclosure = make(map[string]DisclosureState)
	s.matches = nil
	s.current = -1
	needle := []byte(query)
	if len(needle) == 0 || s.observations == nil || s.details == nil {
		return nil
	}
	for _, observation := range s.observations.Snapshot() {
		before := len(s.matches)
		refs := append(append([]DetailRef(nil), observation.ResultRefs...), observation.EnvelopeRefs...)
		for _, ref := range refs {
			evidence, err := s.details.Get(ref)
			if err != nil {
				return err
			}
			for offset := 0; offset <= len(evidence)-len(needle); {
				relative := bytes.Index(evidence[offset:], needle)
				if relative < 0 {
					break
				}
				matchOffset := offset + relative
				s.matches = append(s.matches, TranscriptSearchMatch{ObservationID: observation.ID, EvidenceRef: ref, ByteOffset: matchOffset})
				offset = matchOffset + len(needle)
			}
		}
		identity, err := json.Marshal(struct {
			ID, SessionID, TurnID, WorkUnitID, ActorID, ToolUseID, ToolName string
			Input                                                           map[string]any
		}{observation.ID, observation.SessionID, observation.TurnID, observation.WorkUnitID, observation.ActorID, observation.ToolUseID, observation.ToolName, observation.ToolInput})
		if err == nil {
			appendTranscriptMatches(&s.matches, observation.ID, DetailRef{}, identity, needle)
		}
		if len(s.matches) > before {
			s.returnDisclosure[observation.ID] = observation.Disclosure
		}
	}
	for index, message := range s.messages {
		before := len(s.matches)
		anchor := message.ObservationID
		if anchor == "" {
			anchor = fmt.Sprintf("message:%06d", index)
		}
		for _, ref := range message.DetailRefs {
			content, err := s.details.Get(ref)
			if err != nil {
				return err
			}
			appendTranscriptMatches(&s.matches, anchor, ref, content, needle)
		}
		text := message.Text
		if message.Brief != nil && message.Brief.Message != "" {
			text = message.Brief.Message
		}
		content := []byte(text)
		if len(content) == 0 {
			continue
		}
		appendTranscriptMatches(&s.matches, anchor, DetailRef{}, content, needle)
		if len(s.matches) > before {
			if observation, ok := s.observations.Get(anchor); ok {
				s.returnDisclosure[anchor] = observation.Disclosure
			}
		}
	}
	if len(s.matches) > 0 {
		s.current = 0
	}
	return nil
}

func appendTranscriptMatches(matches *[]TranscriptSearchMatch, anchor string, ref DetailRef, content, needle []byte) {
	for offset := 0; offset <= len(content)-len(needle); {
		relative := bytes.Index(content[offset:], needle)
		if relative < 0 {
			return
		}
		matchOffset := offset + relative
		*matches = append(*matches, TranscriptSearchMatch{ObservationID: anchor, EvidenceRef: ref, ByteOffset: matchOffset})
		offset = matchOffset + len(needle)
	}
}

func (s *TranscriptSearchController) Matches() []TranscriptSearchMatch {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]TranscriptSearchMatch(nil), s.matches...)
}

func (s *TranscriptSearchController) Current() (TranscriptSearchMatch, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current < 0 || s.current >= len(s.matches) {
		return TranscriptSearchMatch{}, false
	}
	return s.matches[s.current], true
}

func (s *TranscriptSearchController) Next() (TranscriptSearchMatch, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.matches) == 0 {
		return TranscriptSearchMatch{}, false
	}
	s.current = (s.current + 1) % len(s.matches)
	return s.matches[s.current], true
}

func (s *TranscriptSearchController) Previous() (TranscriptSearchMatch, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.matches) == 0 {
		return TranscriptSearchMatch{}, false
	}
	s.current--
	if s.current < 0 {
		s.current = len(s.matches) - 1
	}
	return s.matches[s.current], true
}

func (s *TranscriptSearchController) Close() TranscriptViewState {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.observations != nil {
		for id, disclosure := range s.returnDisclosure {
			_ = s.observations.SetDisclosure(id, disclosure)
		}
	}
	return s.returnTo
}

type TranscriptExportFormat string

const (
	TranscriptExportHumanReadable TranscriptExportFormat = "human_readable"
	TranscriptExportRawAuditJSON  TranscriptExportFormat = "raw_audit_json"
)

type TranscriptExporter struct {
	observations    *ObservationStore
	details         DetailStore
	messages        []types.Message
	decisions       []DecisionRecord
	presentation    []Message
	language        i18n.Language
	controlScope    messagecontrol.Scope
	controlScopeSet bool
}

func (e *TranscriptExporter) WithPresentation(messages []Message) *TranscriptExporter {
	e.presentation = append([]Message(nil), messages...)
	return e
}

// WithDecisions includes structured permission and plan decisions in both
// human-readable exports and the raw audit artifact.
func (e *TranscriptExporter) WithDecisions(decisions []DecisionRecord) *TranscriptExporter {
	e.decisions = append([]DecisionRecord(nil), decisions...)
	return e
}

func (e *TranscriptExporter) WithLanguage(lang i18n.Language) *TranscriptExporter {
	e.language = lang
	return e
}

func (e *TranscriptExporter) WithInternalControlScope(capability messagecontrol.Capability, scope messagecontrol.Scope) *TranscriptExporter {
	e.controlScope = messagecontrol.Scope{}
	e.controlScopeSet = false
	if capability.Valid() && scope.Bound() {
		e.controlScope = scope
		e.controlScopeSet = true
	}
	return e
}

func NewTranscriptExporter(observations *ObservationStore, details DetailStore, messageSets ...[]types.Message) *TranscriptExporter {
	var messages []types.Message
	if len(messageSets) > 0 {
		messages = append([]types.Message(nil), messageSets[0]...)
	}
	return &TranscriptExporter{observations: observations, details: details, messages: messages, language: i18n.DetectOrLoadLanguage()}
}

func (e *TranscriptExporter) Export(path string, format TranscriptExportFormat) error {
	data, err := e.serialize(format)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(dir, ".transcript-export-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	ok := false
	defer func() {
		_ = temporary.Close()
		if !ok {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	ok = true
	return nil
}

func (e *TranscriptExporter) serialize(format TranscriptExportFormat) ([]byte, error) {
	if e.observations == nil || e.details == nil {
		return nil, fmt.Errorf("%s", i18n.Text(e.language, i18n.KeyTranscriptMissingStore))
	}
	type auditObservation struct {
		ID         string   `json:"id"`
		SessionID  string   `json:"session_id"`
		TurnID     string   `json:"turn_id"`
		WorkUnitID string   `json:"work_unit_id"`
		ActorID    string   `json:"actor_id"`
		ToolUseID  string   `json:"tool_use_id"`
		ToolName   string   `json:"tool_name"`
		Evidence   []string `json:"evidence"`
		Envelopes  []string `json:"structured_evidence,omitempty"`
	}
	observations := e.observations.Snapshot()
	audit := make([]auditObservation, 0, len(observations))
	for _, observation := range observations {
		entry := auditObservation{
			ID: observation.ID, SessionID: observation.SessionID, TurnID: observation.TurnID,
			WorkUnitID: observation.WorkUnitID, ActorID: observation.ActorID,
			ToolUseID: observation.ToolUseID, ToolName: observation.ToolName,
		}
		for _, ref := range observation.ResultRefs {
			content, err := e.details.Get(ref)
			if err != nil {
				return nil, err
			}
			entry.Evidence = append(entry.Evidence, string(content))
		}
		for _, ref := range observation.EnvelopeRefs {
			content, err := e.details.Get(ref)
			if err != nil {
				return nil, err
			}
			entry.Envelopes = append(entry.Envelopes, string(content))
		}
		audit = append(audit, entry)
	}
	if format == TranscriptExportRawAuditJSON {
		return json.MarshalIndent(struct {
			SchemaVersion int                `json:"schema_version"`
			Messages      []types.Message    `json:"messages"`
			Observations  []auditObservation `json:"observations"`
			Decisions     []DecisionRecord   `json:"decisions,omitempty"`
			Presentation  []Message          `json:"presentation,omitempty"`
		}{SchemaVersion: 3, Messages: e.messages, Observations: audit, Decisions: e.decisions, Presentation: e.presentation}, "", "  ")
	}
	if format != TranscriptExportHumanReadable {
		return nil, fmt.Errorf("%s", i18n.Format(e.language, i18n.KeyTranscriptUnsupportedFormat, format))
	}
	var output strings.Builder
	for index, message := range e.messages {
		internal := message.IsInternalRuntimeMessageForScope(messagecontrol.Scope{})
		if e.controlScopeSet {
			internal = message.IsInternalRuntimeMessageForScope(e.controlScope)
		}
		if internal {
			continue
		}
		text := message.GetText()
		if text == "" {
			continue
		}
		role := message.Role
		trustedDeveloper := message.IsTrustedDeveloperMessageForScope(messagecontrol.Scope{})
		if e.controlScopeSet {
			trustedDeveloper = message.IsTrustedDeveloperMessageForScope(e.controlScope)
		}
		if role == types.RoleDeveloper && !trustedDeveloper {
			role = types.RoleUser
		}
		fmt.Fprintf(&output, "[%06d] %s\n", index, i18n.TranscriptRoleLabel(e.language, string(role)))
		output.WriteString(text)
		if !strings.HasSuffix(text, "\n") {
			output.WriteByte('\n')
		}
		output.WriteByte('\n')
	}
	for _, observation := range audit {
		fmt.Fprint(&output, i18n.Format(e.language, i18n.KeyTranscriptObservationHeader, observation.ID, observation.ToolName, observation.ToolUseID, observation.SessionID, observation.TurnID, observation.WorkUnitID, observation.ActorID))
		for _, evidence := range observation.Evidence {
			output.WriteString(evidence)
			if !strings.HasSuffix(evidence, "\n") {
				output.WriteByte('\n')
			}
		}
		for _, envelope := range observation.Envelopes {
			output.WriteString(i18n.Text(e.language, i18n.KeyTranscriptStructuredEvidence))
			output.WriteString(envelope)
			if !strings.HasSuffix(envelope, "\n") {
				output.WriteByte('\n')
			}
		}
		output.WriteByte('\n')
	}
	for index, message := range e.presentation {
		if message.Text == "" && len(message.DetailRefs) == 0 {
			continue
		}
		fmt.Fprint(&output, i18n.Format(e.language, i18n.KeyTranscriptPresentationHeader, index, message.Kind, message.Text))
		for _, ref := range message.DetailRefs {
			content, err := e.details.Get(ref)
			if err != nil {
				return nil, err
			}
			output.Write(content)
			if len(content) > 0 && content[len(content)-1] != '\n' {
				output.WriteByte('\n')
			}
		}
		output.WriteByte('\n')
	}
	for _, decision := range e.decisions {
		fmt.Fprint(&output, i18n.Format(e.language, i18n.KeyTranscriptDecisionHeader,
			decision.Prompt.DecisionID, decision.Prompt.Kind, decision.Prompt.ActorID,
			decision.Prompt.Action, decision.Prompt.Target, decision.Prompt.Impact,
			decision.Prompt.RiskReason, decision.Prompt.RuleSource, decision.Prompt.ApprovalScope,
			decision.Response.Outcome, decision.Response.Choice))
	}
	return []byte(output.String()), nil
}
