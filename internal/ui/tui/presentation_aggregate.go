package tui

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/agent-dance/luban/i18n"
)

// ObservationAggregation is projection-only state. Hidden never means
// deleted: the underlying observation, evidence refs, and aggregate member
// index remain available for show-all, search, export, and audit.
type ObservationAggregation struct {
	GroupID        string `json:"group_id,omitempty"`
	MemberIndex    int    `json:"member_index,omitempty"`
	Representative bool   `json:"representative,omitempty"`
	Hidden         bool   `json:"hidden,omitempty"`
	Count          int    `json:"count,omitempty"`
	Summary        string `json:"summary,omitempty"`
}

type ObservationAggregate struct {
	ID            string        `json:"id"`
	Key           string        `json:"key"`
	SessionID     string        `json:"session_id"`
	TurnID        string        `json:"turn_id"`
	ActorID       string        `json:"actor_id,omitempty"`
	WorkUnitID    string        `json:"work_unit_id,omitempty"`
	Family        CommandFamily `json:"family"`
	Intent        string        `json:"intent"`
	MemberIDs     []string      `json:"member_ids"`
	ObjectCount   int           `json:"object_count,omitempty"`
	ObjectSamples []string      `json:"object_samples,omitempty"`
	EvidenceRefs  []DetailRef   `json:"evidence_refs,omitempty"`
	EvidenceCount int           `json:"evidence_count,omitempty"`
	Summary       string        `json:"summary"`
	Live          bool          `json:"live,omitempty"`
	Frozen        bool          `json:"frozen,omitempty"`
}

func (s *ObservationStore) AggregateSnapshot() []ObservationAggregate {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]ObservationAggregate, 0, len(s.aggregates))
	for _, group := range s.aggregates {
		if len(group.MemberIDs) < 2 {
			continue
		}
		clone := *group
		clone.MemberIDs = append([]string(nil), group.MemberIDs...)
		clone.ObjectSamples = append([]string(nil), group.ObjectSamples...)
		clone.EvidenceRefs = append([]DetailRef(nil), group.EvidenceRefs...)
		out = append(out, clone)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func cloneObservationAggregates(groups []ObservationAggregate) []ObservationAggregate {
	if groups == nil {
		return nil
	}
	out := make([]ObservationAggregate, len(groups))
	for index := range groups {
		out[index] = groups[index]
		out[index].MemberIDs = append([]string(nil), groups[index].MemberIDs...)
		out[index].ObjectSamples = append([]string(nil), groups[index].ObjectSamples...)
		out[index].EvidenceRefs = append([]DetailRef(nil), groups[index].EvidenceRefs...)
	}
	return out
}

// restoreAggregatesLocked installs the exact checkpointed aggregate surface.
// It deliberately does not re-run current presentation policy: doing so would
// let a future policy change alter already-settled resume/fork history.
func (s *ObservationStore) restoreAggregatesLocked(groups []ObservationAggregate) bool {
	s.aggregates = make(map[string]*ObservationAggregate, len(groups))
	s.aggregateKeyByObservation = make(map[string]string)
	for _, source := range groups {
		if source.ID == "" || source.Key == "" || len(source.MemberIDs) < 2 {
			return false
		}
		group := cloneObservationAggregates([]ObservationAggregate{source})[0]
		if _, duplicate := s.aggregates[group.Key]; duplicate {
			return false
		}
		for _, memberID := range group.MemberIDs {
			index, exists := s.byID[memberID]
			if !exists || s.observations[index].Aggregation.GroupID != group.ID {
				return false
			}
			if _, duplicate := s.aggregateKeyByObservation[memberID]; duplicate {
				return false
			}
			s.aggregateKeyByObservation[memberID] = group.Key
		}
		s.aggregates[group.Key] = &group
	}
	return true
}

func (s *ObservationStore) Aggregate(id string) (ObservationAggregate, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, group := range s.aggregates {
		if group.ID != id || len(group.MemberIDs) < 2 {
			continue
		}
		clone := *group
		clone.MemberIDs = append([]string(nil), group.MemberIDs...)
		clone.ObjectSamples = append([]string(nil), group.ObjectSamples...)
		clone.EvidenceRefs = append([]DetailRef(nil), group.EvidenceRefs...)
		return clone, true
	}
	return ObservationAggregate{}, false
}

func (s *ObservationStore) rebuildAggregatesLocked() {
	s.aggregates = make(map[string]*ObservationAggregate)
	s.aggregateKeyByObservation = make(map[string]string)
	for index := range s.observations {
		s.observations[index].Aggregation = ObservationAggregation{}
		restoreObservationPresentationLevel(&s.observations[index])
		s.observations[index].Decision.Reasons = removePresentationReason(s.observations[index].Decision.Reasons, ReasonAggregateMember)
	}
	for index := range s.observations {
		if observationAggregationBoundary(s.observations[index]) {
			s.rotateObservationAggregateLocked(observationAggregateKey(s.observations[index]))
		} else if observationAggregationCandidate(s.observations[index]) {
			s.addObservationToAggregateLocked(index, observationAggregateKey(s.observations[index]))
		}
	}
	for _, group := range s.aggregates {
		group.Live = false
		group.Frozen = true
	}
}

// FreezeAggregates closes live groups at the deterministic turn boundary.
// Late output cannot mutate a frozen summary; it remains independently visible
// and can form a later group if a new turn supplies a different key.
func (s *ObservationStore) FreezeAggregates(sessionID, turnID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	frozen := 0
	for _, group := range s.aggregates {
		if !group.Live || group.SessionID != sessionID || group.TurnID != turnID {
			continue
		}
		group.Live = false
		group.Frozen = true
		frozen++
	}
	return frozen
}

func (s *ObservationStore) updateObservationAggregateLocked(index int) {
	if index < 0 || index >= len(s.observations) {
		return
	}
	observation := &s.observations[index]
	previousKey := s.aggregateKeyByObservation[observation.ID]
	nextKey := ""
	baseKey := observationAggregateKey(*observation)
	if observationAggregationBoundary(*observation) {
		s.rotateObservationAggregateLocked(baseKey)
	} else if observationAggregationCandidate(*observation) {
		nextKey = s.currentObservationAggregateKeyLocked(baseKey)
	}
	if previousKey == nextKey && nextKey != "" {
		return
	}
	if previousKey != "" {
		s.removeObservationFromAggregateLocked(observation.ID, previousKey)
	}
	observation.Aggregation = ObservationAggregation{}
	restoreObservationPresentationLevel(observation)
	observation.Decision.Reasons = removePresentationReason(observation.Decision.Reasons, ReasonAggregateMember)
	if nextKey == "" {
		return
	}
	s.addObservationToAggregateLocked(index, nextKey)
}

func (s *ObservationStore) addObservationToAggregateLocked(index int, key string) {
	observation := &s.observations[index]
	key = s.currentObservationAggregateKeyLocked(key)
	group := s.aggregates[key]
	if group == nil {
		group = &ObservationAggregate{
			ID: observationAggregateID(key), Key: key,
			SessionID: observation.SessionID, TurnID: observation.TurnID,
			ActorID: observation.ActorID, WorkUnitID: observation.WorkUnitID,
			Family: observation.Presentation.Family, Intent: observation.Presentation.AggregationIntent,
			Live: true,
		}
		s.aggregates[key] = group
	}
	if group.Frozen {
		return
	}
	if existingKey := s.aggregateKeyByObservation[observation.ID]; existingKey != "" {
		return
	}
	previousCount := len(group.MemberIDs)
	group.MemberIDs = append(group.MemberIDs, observation.ID)
	s.aggregateKeyByObservation[observation.ID] = key
	s.appendAggregateEvidenceLocked(group, *observation)
	group.Summary = observationAggregateSummaryInLanguage(observationPresentationLanguage(*observation), group.Family, group.Intent, len(group.MemberIDs))
	if previousCount == 0 {
		return
	}
	previousRepresentativeID := group.MemberIDs[previousCount-1]
	if previousIndex, ok := s.byID[previousRepresentativeID]; ok {
		s.setAggregateMemberProjectionLocked(&s.observations[previousIndex], group, previousCount, false)
	}
	s.setAggregateMemberProjectionLocked(observation, group, previousCount+1, true)
}

const observationAggregateRotationPrefix = "\x00presentation-aggregate-rotation\x00"

func observationAggregateRotationMarkerKey(baseKey string) string {
	return observationAggregateRotationPrefix + observationAggregateID(baseKey)
}

func (s *ObservationStore) currentObservationAggregateKeyLocked(baseKey string) string {
	if marker := s.aggregates[observationAggregateRotationMarkerKey(baseKey)]; marker != nil && marker.Key != "" {
		return marker.Key
	}
	return baseKey
}

// rotateObservationAggregateLocked makes an exceptional terminal event a hard
// ordering boundary. The frozen group remains auditable while the next safe
// success starts a distinct segment under the same domain intent.
func (s *ObservationStore) rotateObservationAggregateLocked(baseKey string) {
	if strings.TrimSpace(baseKey) == "" {
		return
	}
	markerKey := observationAggregateRotationMarkerKey(baseKey)
	currentKey := s.currentObservationAggregateKeyLocked(baseKey)
	group := s.aggregates[currentKey]
	if group == nil || !group.Live {
		return
	}
	group.Live = false
	group.Frozen = true
	generation := 1
	if marker := s.aggregates[markerKey]; marker != nil {
		generation = marker.ObjectCount + 1
	}
	nextKey := fmt.Sprintf("%s\x00segment:%d", baseKey, generation)
	s.aggregates[markerKey] = &ObservationAggregate{Key: nextKey, ObjectCount: generation}
}

func (s *ObservationStore) setAggregateMemberProjectionLocked(member *Observation, group *ObservationAggregate, memberIndex int, representative bool) {
	if member == nil || group == nil {
		return
	}
	member.Aggregation = ObservationAggregation{
		GroupID: group.ID, MemberIndex: memberIndex, Representative: representative, Hidden: !representative,
	}
	if representative {
		member.Aggregation.Count = len(group.MemberIDs)
		member.Aggregation.Summary = group.Summary
		member.Decision.EffectiveLevel = PresentationFolded
		member.Decision.Reasons = removePresentationReason(member.Decision.Reasons, ReasonAggregateMember)
		return
	}
	member.Decision.EffectiveLevel = PresentationHiddenMember
	member.Decision.Reasons = appendPresentationReason(member.Decision.Reasons, ReasonAggregateMember)
}

func (s *ObservationStore) appendAggregateEvidenceLocked(group *ObservationAggregate, observation Observation) {
	if object := strings.TrimSpace(observation.Presentation.Object); object != "" {
		group.ObjectCount++
		const maxObjectSamples = 20
		if len(group.ObjectSamples) < maxObjectSamples {
			group.ObjectSamples = append(group.ObjectSamples, object)
		}
	}
	refs := append(append([]DetailRef(nil), observation.ResultRefs...), observation.EnvelopeRefs...)
	group.EvidenceCount += len(refs)
	group.EvidenceRefs = append(group.EvidenceRefs, refs...)
}

func (s *ObservationStore) removeObservationFromAggregateLocked(observationID, key string) {
	group := s.aggregates[key]
	delete(s.aggregateKeyByObservation, observationID)
	if group == nil {
		return
	}
	for index, id := range group.MemberIDs {
		if id == observationID {
			group.MemberIDs = append(group.MemberIDs[:index], group.MemberIDs[index+1:]...)
			break
		}
	}
	if len(group.MemberIDs) == 0 {
		delete(s.aggregates, key)
		return
	}
	s.refreshAggregateMembersLocked(group)
}

func (s *ObservationStore) refreshAggregateMembersLocked(group *ObservationAggregate) {
	if group == nil {
		return
	}
	lang := i18n.DetectOrLoadLanguage()
	if len(group.MemberIDs) > 0 {
		if index, ok := s.byID[group.MemberIDs[0]]; ok {
			lang = observationPresentationLanguage(s.observations[index])
		}
	}
	group.Summary = observationAggregateSummaryInLanguage(lang, group.Family, group.Intent, len(group.MemberIDs))
	group.ObjectCount = 0
	group.ObjectSamples = nil
	group.EvidenceCount = 0
	group.EvidenceRefs = nil
	for _, id := range group.MemberIDs {
		if index, ok := s.byID[id]; ok {
			s.appendAggregateEvidenceLocked(group, s.observations[index])
		}
	}
	if len(group.MemberIDs) < 2 {
		for _, id := range group.MemberIDs {
			if index, ok := s.byID[id]; ok {
				member := &s.observations[index]
				member.Aggregation = ObservationAggregation{}
				restoreObservationPresentationLevel(member)
				member.Decision.Reasons = removePresentationReason(member.Decision.Reasons, ReasonAggregateMember)
			}
		}
		return
	}
	representativeID := group.MemberIDs[len(group.MemberIDs)-1]
	for memberIndex, id := range group.MemberIDs {
		index, ok := s.byID[id]
		if !ok {
			continue
		}
		member := &s.observations[index]
		representative := id == representativeID
		s.setAggregateMemberProjectionLocked(member, group, memberIndex+1, representative)
	}
}

func restoreObservationPresentationLevel(observation *Observation) {
	if observation == nil {
		return
	}
	level := observation.Decision.DefaultLevel
	defaultDisclosure := PresentationDecision{EffectiveLevel: observation.Decision.DefaultLevel}.DisclosureLevel()
	if observation.Disclosure.UserPinned || observation.Disclosure.Level != defaultDisclosure {
		level = presentationLevelForDisclosure(observation.Disclosure.Level)
	}
	observation.Decision.EffectiveLevel = level
}

func observationAggregationCandidate(observation Observation) bool {
	return observation.Decision.AggregationEligible && observation.Outcome == OutcomeSucceeded &&
		observation.Disclosure.Level == DisclosureSummary && !observation.Disclosure.UserPinned &&
		observation.Presentation.AggregationIntent != ""
}

func observationAggregationBoundary(observation Observation) bool {
	if observation.Presentation.AggregationIntent == "" ||
		observation.Outcome == OutcomeUnknown || observation.Outcome == OutcomeRunning {
		return false
	}
	return observation.Outcome != OutcomeSucceeded || observation.Presentation.Warning ||
		observation.Presentation.RequiresDecision || observation.Presentation.SideEffect
}

func observationAggregateKey(observation Observation) string {
	values := []string{
		observation.SessionID,
		observation.TurnID,
		observation.ActorID,
		observation.WorkUnitID,
		string(observation.Presentation.Family),
		observation.Presentation.AggregationIntent,
		CanonicalAggregationDomainIntent(observation.Presentation.Family, observation.ToolName, observation.ToolInput),
	}
	var builder strings.Builder
	for _, value := range values {
		fmt.Fprintf(&builder, "%d:%s|", len(value), value)
	}
	return builder.String()
}

// CanonicalAggregationDomainIntent returns a non-reversible digest of the
// fields that distinguish operations within one broad formatter intent. It
// keeps search expressions, web targets, and MCP routing identity out of the
// visible aggregate key while preventing semantically unrelated calls from
// being folded together.
func CanonicalAggregationDomainIntent(family CommandFamily, toolName string, input map[string]any) string {
	components := make([]string, 0, 10)
	appendInput := func(label string, keys ...string) {
		for _, key := range keys {
			if value := canonicalAggregationInputValue(input[key]); value != "" {
				components = append(components, label+"="+value)
				return
			}
		}
	}
	switch family {
	case FamilySearch:
		if strings.EqualFold(strings.TrimSpace(toolName), "LSP") {
			appendInput("operation", "operation")
			appendInput("file_path", "filePath", "file_path")
			appendInput("line", "line")
			appendInput("character", "character")
		} else {
			appendInput("query", "pattern", "query", "symbol", "glob")
		}
	case FamilyWeb:
		appendInput("query", "query", "search_query")
		for _, key := range []string{"url", "uri"} {
			if value := canonicalAggregationInputValue(input[key]); value != "" {
				components = append(components, key+"="+canonicalAggregationLocator(value))
			}
		}
	case FamilyMCP:
		appendInput("server", "server", "server_name", "serverName")
		appendInput("capability", "tool_name", "toolName", "capability", "method", "prompt")
		appendInput("uri", "uri", "resource_uri", "resourceUri")
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(toolName)), "mcp__") {
			parts := strings.SplitN(strings.TrimSpace(toolName)[len("mcp__"):], "__", 2)
			if len(parts) > 0 && strings.TrimSpace(parts[0]) != "" {
				components = append(components, "dynamic_server="+strings.TrimSpace(parts[0]))
			}
			if len(parts) == 2 && strings.TrimSpace(parts[1]) != "" {
				components = append(components, "dynamic_capability="+strings.TrimSpace(parts[1]))
			}
		}
	}
	if len(components) == 0 {
		return ""
	}
	var canonical strings.Builder
	for _, component := range components {
		fmt.Fprintf(&canonical, "%d:%s|", len(component), component)
	}
	digest := sha256.Sum256([]byte("presentation-domain-intent-v1\x00" + canonical.String()))
	return hex.EncodeToString(digest[:16])
}

func canonicalAggregationInputValue(value any) string {
	return strings.TrimSpace(presentationString(value))
}

func canonicalAggregationLocator(value string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme == "" {
		return strings.TrimSpace(value)
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.User = nil
	query := parsed.Query()
	for key := range query {
		if isSensitivePresentationLocatorKey(key) {
			query.Del(key)
		}
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func observationAggregateID(key string) string {
	digest := sha256.Sum256([]byte("presentation-aggregate-v1\x00" + key))
	return "aggregate:" + hex.EncodeToString(digest[:12])
}

func observationAggregateSummaryInLanguage(lang i18n.Language, family CommandFamily, intent string, count int) string {
	label := string(family)
	switch family {
	case FamilyFileRead:
		label = i18n.Text(lang, i18n.KeyPresentationAggregateRead)
	case FamilySearch:
		label = i18n.Text(lang, i18n.KeyPresentationAggregateSearch)
	case FamilyWeb:
		label = i18n.Text(lang, i18n.KeyPresentationAggregateWeb)
	case FamilyMCP:
		label = "MCP"
	}
	return i18n.Format(lang, i18n.KeyPresentationAggregateOperations, label, formatPresentationInt(int64(count)))
}

func observationPresentationLanguage(observation Observation) i18n.Language {
	switch strings.ToLower(strings.TrimSpace(observation.Presentation.Language)) {
	case "zh":
		return i18n.LangZH
	case "de":
		return i18n.LangDE
	case "ja":
		return i18n.LangJA
	case "ko":
		return i18n.LangKO
	case "ru":
		return i18n.LangRU
	case "en":
		return i18n.LangEN
	default:
		return i18n.DetectOrLoadLanguage()
	}
}

func removePresentationReason(reasons []PresentationReason, remove PresentationReason) []PresentationReason {
	out := reasons[:0]
	for _, reason := range reasons {
		if reason != remove {
			out = append(out, reason)
		}
	}
	return out
}
