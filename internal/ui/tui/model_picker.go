package tui

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/provider"
)

// ModelPickerPhase tracks which level of the cascading picker is active.
type ModelPickerPhase int

const (
	// PickerPhaseProvider shows the provider list (first level).
	PickerPhaseProvider ModelPickerPhase = iota
	// PickerPhaseModel shows models for the selected provider (second level).
	PickerPhaseModel
	// PickerPhaseReasoning shows reasoning effort tiers for the selected model.
	PickerPhaseReasoning
	// PickerPhaseConnect shows a connection or reconnection flow (intermediate).
	PickerPhaseConnect
	// PickerPhaseEditLimits edits local model limit overrides.
	PickerPhaseEditLimits
	// PickerPhaseDeleteConfirm confirms deletion of a user-created provider.
	PickerPhaseDeleteConfirm
)

// ConnectInputField identifies the active custom-endpoint input.
type ConnectInputField int

const (
	ConnectInputAPIKey ConnectInputField = iota
	ConnectInputBaseURL
	ConnectInputProviderName
	ConnectInputAPIStyle
)

// ProviderPickerEntry represents a single provider in the cascading picker.
type ProviderPickerEntry struct {
	Name            string   // canonical name, e.g. "anthropic"
	DisplayName     string   // human-readable, e.g. "Anthropic"
	ModelCount      int      // number of models available
	IsActive        bool     // true if this is the currently active provider
	IsConnected     bool     // true if credentials are configured
	ConnectionState string   // typed state from provider.ConnectionState
	ConnectionLabel string   // human-readable status detail
	CanSelectModels bool     // true when Enter should open model selection
	CanConnect      bool     // true when Enter should open setup/connect flow
	SetupHint       string   // guidance for providers without inline auth
	AuthMethods     []string // e.g. ["api_key", "oauth_pkce", "device_code"]
	EnvKey          string   // primary environment variable for authentication
	BaseURL         string   // persisted custom endpoint, if configured
	DefaultBaseURL  string   // provider's default API base URL
	APIStyles       []provider.APIStyle
	APIStyle        provider.APIStyle
	DefaultBaseURLs map[provider.APIStyle]string
	DynamicModels   bool
	UserDefined     bool
	IsCreate        bool
}

// ModelPickerEntry represents a single model in the picker overlay.
type ModelPickerEntry struct {
	Provider          string // e.g. "anthropic"
	ModelID           string // e.g. "claude-sonnet-4-20250514"
	DisplayName       string // e.g. "Claude Sonnet 4"
	Description       string // one-line guidance shown beside the model ID
	ContextK          string // e.g. "200K"
	ContextTokens     int
	ContextOverridden bool
	CostIn            float64 // cost per 1M input tokens
	CostOut           float64 // cost per 1M output tokens
	CostCurrency      string  // ISO 4217 billing currency
	CanReason         bool
	CanSeeImages      bool
	ReasoningEfforts  []string // selectable effort tiers; empty = no second-level picker
	ReasoningEffort   string   // chosen effort tier when applying a selection
	IsDefault         bool
}

type reasoningEffortInfo struct {
	Label       string
	Description string
}

// ProviderConnectRequest is the complete connection form submitted by the
// model picker. Native providers use AuthMethod; compatible aggregate
// providers additionally use APIStyle and the optional custom display name.
type ProviderConnectRequest struct {
	ProviderName string
	DisplayName  string
	AuthMethod   string
	APIStyle     provider.APIStyle
	BaseURL      string
	APIKey       string
	UserDefined  bool
}

type ConnectOptions struct {
	APIStyles       []provider.APIStyle
	APIStyle        provider.APIStyle
	DefaultBaseURLs map[provider.APIStyle]string
	DynamicModels   bool
	UserDefined     bool
}

// ModelPickerState tracks the model picker modal with cascading selection.
// Phase 1 (PickerPhaseProvider): user picks a provider from Providers list.
// Phase 2 (PickerPhaseModel): user picks a model from the selected provider's models.
// Optional Phase 3 (PickerPhaseReasoning): user picks a reasoning effort tier.
// Connect phase (PickerPhaseConnect): user enters or refreshes provider credentials.
type ModelPickerState struct {
	Visible bool
	Phase   ModelPickerPhase

	// Provider-level state (Phase 1)
	Providers        []ProviderPickerEntry
	ProviderSelected int

	// Model-level state (Phase 2)
	SelectedProvider string             // the provider chosen in Phase 1
	Entries          []ModelPickerEntry // models for the selected provider
	Filtered         []int              // indices into Entries matching query
	Selected         int                // index into Filtered
	Query            string

	// Reasoning-level state (Phase 3 — PickerPhaseReasoning)
	ReasoningModel    ModelPickerEntry
	ReasoningSelected int

	// Connection-level state (PickerPhaseConnect)
	ConnectProvider          string   // provider being connected
	ConnectAuthMethods       []string // available auth methods for the provider
	ConnectSelectedAuth      int      // selected auth method index
	ConnectAPIKeyInput       string   // API key text input buffer
	ConnectBaseURLInput      string   // optional custom API endpoint
	ConnectDefaultBaseURL    string   // provider's default API base URL
	ConnectDefaultBaseURLs   map[provider.APIStyle]string
	ConnectAPIStyles         []provider.APIStyle
	ConnectSelectedStyle     int
	ConnectProviderNameInput string
	ConnectDynamicModels     bool
	ConnectUserDefined       bool
	ConnectInputField        ConnectInputField
	ConnectStatus            string // status message ("Connecting...", "✅ Connected", etc.)
	ConnectError             string // error message if connection failed
	ConnectHint              string // setup guidance for non-inline providers
	IsReconnect              bool   // true when refreshing an existing connection

	// Model limit edit state (PickerPhaseEditLimits)
	LimitEditModel ModelPickerEntry
	LimitEditInput string
	LimitEditError string

	DeleteProvider string
	DeleteError    string

	// Callbacks
	OnSelect      func(ModelPickerEntry)
	OnCancel      func()
	OnSaveLimits  func(ModelPickerEntry, *int) error
	EnterProvider func(providerName string)       // called when user selects a provider in Phase 1
	OnConnect     func(ProviderConnectRequest)    // called to perform connection
	OnDelete      func(providerName string) error // deletes a user-created provider
}

// clampProvider keeps ProviderSelected within range.
func (s *ModelPickerState) clampProvider() {
	if len(s.Providers) == 0 {
		s.ProviderSelected = 0
		return
	}
	if s.ProviderSelected < 0 {
		s.ProviderSelected = 0
	}
	if s.ProviderSelected >= len(s.Providers) {
		s.ProviderSelected = len(s.Providers) - 1
	}
}

// clamp keeps Selected within the filtered range.
func (s *ModelPickerState) clamp() {
	if len(s.Filtered) == 0 {
		s.Selected = 0
		return
	}
	if s.Selected < 0 {
		s.Selected = 0
	}
	if s.Selected >= len(s.Filtered) {
		s.Selected = len(s.Filtered) - 1
	}
}

func (s *ModelPickerState) clampReasoning() {
	n := len(s.ReasoningModel.ReasoningEfforts)
	if n == 0 {
		s.ReasoningSelected = 0
		return
	}
	if s.ReasoningSelected < 0 {
		s.ReasoningSelected = 0
	}
	if s.ReasoningSelected >= n {
		s.ReasoningSelected = n - 1
	}
}

// ApplyFilter rebuilds the Filtered index list based on Query.
// Matching is case-insensitive substring against model ID and display name.
func (s *ModelPickerState) ApplyFilter() {
	s.applyFilter()
}

// applyFilter rebuilds the Filtered index list based on Query.
// Matching is case-insensitive substring against model ID and display name.
// Always allocates a new slice to avoid sharing the underlying array with any
// previous Filtered value that the render goroutine may still be reading.
func (s *ModelPickerState) applyFilter() {
	if s.Query == "" {
		s.Filtered = make([]int, len(s.Entries))
		for i := range s.Entries {
			s.Filtered[i] = i
		}
	} else {
		q := strings.ToLower(s.Query)
		filtered := make([]int, 0, len(s.Entries))
		for i, e := range s.Entries {
			haystack := strings.ToLower(e.ModelID + " " + e.DisplayName)
			if strings.Contains(haystack, q) {
				filtered = append(filtered, i)
			}
		}
		s.Filtered = filtered
	}
	s.clamp()
}

// appendQuery adds a rune to the search query and re-filters.
func (s *ModelPickerState) appendQuery(r rune) {
	if unicode.IsPrint(r) {
		s.Query += string(r)
		s.applyFilter()
	}
}

// backspaceQuery removes the last rune from the query and re-filters.
func (s *ModelPickerState) backspaceQuery() {
	if s.Query == "" {
		return
	}
	runes := []rune(s.Query)
	s.Query = string(runes[:len(runes)-1])
	s.applyFilter()
}

// selectedEntry returns the currently highlighted entry, or nil if none.
func (s *ModelPickerState) selectedEntry() *ModelPickerEntry {
	if len(s.Filtered) == 0 || s.Selected >= len(s.Filtered) {
		return nil
	}
	idx := s.Filtered[s.Selected]
	if idx >= len(s.Entries) {
		return nil
	}
	return &s.Entries[idx]
}

// GoBack returns from model/connect phase to provider phase, resetting state.
func (s *ModelPickerState) GoBack() {
	s.Phase = PickerPhaseProvider
	s.SelectedProvider = ""
	s.Entries = nil
	s.Filtered = nil
	s.Selected = 0
	s.Query = ""
	s.ReasoningModel = ModelPickerEntry{}
	s.ReasoningSelected = 0
	// Reset connect state
	s.ConnectProvider = ""
	s.ConnectAuthMethods = nil
	s.ConnectSelectedAuth = 0
	s.ConnectAPIKeyInput = ""
	s.ConnectBaseURLInput = ""
	s.ConnectDefaultBaseURL = ""
	s.ConnectDefaultBaseURLs = nil
	s.ConnectAPIStyles = nil
	s.ConnectSelectedStyle = 0
	s.ConnectProviderNameInput = ""
	s.ConnectDynamicModels = false
	s.ConnectUserDefined = false
	s.ConnectInputField = ConnectInputAPIKey
	s.ConnectStatus = ""
	s.ConnectError = ""
	s.ConnectHint = ""
	s.IsReconnect = false
	s.LimitEditModel = ModelPickerEntry{}
	s.LimitEditInput = ""
	s.LimitEditError = ""
	s.DeleteProvider = ""
	s.DeleteError = ""
}

// EnterReasoning transitions from model selection to reasoning-effort selection.
func (s *ModelPickerState) EnterReasoning(entry ModelPickerEntry) {
	s.Phase = PickerPhaseReasoning
	s.ReasoningModel = entry
	s.ReasoningSelected = 0
	if entry.ReasoningEffort == "" {
		entry.ReasoningEffort = DefaultReasoningEffort(entry.ReasoningEfforts)
		s.ReasoningModel = entry
	}
	for i, effort := range entry.ReasoningEfforts {
		if effort == entry.ReasoningEffort {
			s.ReasoningSelected = i
			break
		}
	}
	s.clampReasoning()
}

func (s *ModelPickerState) selectedReasoningEffort() string {
	if len(s.ReasoningModel.ReasoningEfforts) == 0 {
		return ""
	}
	s.clampReasoning()
	return s.ReasoningModel.ReasoningEfforts[s.ReasoningSelected]
}

func DefaultReasoningEffort(efforts []string) string {
	return provider.DefaultReasoningEffort(efforts)
}

// ReasoningEffortInfoInLanguage returns localized first-party labels while
// preserving unknown Provider-defined effort identifiers.
func ReasoningEffortInfoInLanguage(lang i18n.Language, effort string) reasoningEffortInfo {
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case "low":
		return reasoningEffortInfo{Label: i18n.Text(lang, i18n.KeyReasoningLabelLow), Description: i18n.Text(lang, i18n.KeyReasoningDescriptionLow)}
	case "medium":
		return reasoningEffortInfo{Label: i18n.Text(lang, i18n.KeyReasoningLabelMedium), Description: i18n.Text(lang, i18n.KeyReasoningDescriptionMedium)}
	case "high":
		return reasoningEffortInfo{Label: i18n.Text(lang, i18n.KeyReasoningLabelHigh), Description: i18n.Text(lang, i18n.KeyReasoningDescriptionHigh)}
	case "xhigh", "extra-high", "extra_high":
		return reasoningEffortInfo{Label: i18n.Text(lang, i18n.KeyReasoningLabelExtraHigh), Description: i18n.Text(lang, i18n.KeyReasoningDescriptionExtraHigh)}
	case "max":
		return reasoningEffortInfo{Label: i18n.Text(lang, i18n.KeyReasoningLabelMax), Description: i18n.Text(lang, i18n.KeyReasoningDescriptionMax)}
	default:
		label := strings.TrimSpace(effort)
		if label == "" {
			label = i18n.Text(lang, i18n.KeyReasoningLabelDefault)
		} else {
			label = titleWords(label)
		}
		return reasoningEffortInfo{Label: label, Description: i18n.Text(lang, i18n.KeyReasoningDescriptionProviderDefined)}
	}
}

// ModelPickerDescriptionInLanguage localizes built-in fallback copy. A
// Provider-supplied Description is external content and is returned unchanged.
func ModelPickerDescriptionInLanguage(lang i18n.Language, entry ModelPickerEntry) string {
	if entry.Description != "" {
		return entry.Description
	}
	id := strings.ToLower(entry.ModelID)
	key := i18n.KeyModelDescriptionGeneral
	switch {
	case id == "gpt-5.5":
		key = i18n.KeyModelDescriptionGPT55
	case id == "gpt-5.4":
		key = i18n.KeyModelDescriptionGPT54
	case id == "gpt-5.4-mini":
		key = i18n.KeyModelDescriptionGPT54Mini
	case id == "gpt-5.4-nano":
		key = i18n.KeyModelDescriptionGPT54Nano
	case id == "gpt-5.3-codex":
		key = i18n.KeyModelDescriptionGPT53Codex
	case id == "gpt-5.2":
		key = i18n.KeyModelDescriptionGPT52
	case id == "gpt-5":
		key = i18n.KeyModelDescriptionGPT5
	case id == "gpt-5-mini":
		key = i18n.KeyModelDescriptionGPT5Mini
	case id == "gpt-4o":
		key = i18n.KeyModelDescriptionGPT4O
	case id == "gpt-4o-mini":
		key = i18n.KeyModelDescriptionGPT4OMini
	case strings.Contains(id, "codex-spark"):
		key = i18n.KeyModelDescriptionCodexSpark
	case strings.Contains(id, "codex"):
		key = i18n.KeyModelDescriptionCodex
	case strings.Contains(id, "deepseek-v4-pro"):
		key = i18n.KeyModelDescriptionDeepSeekPro
	case strings.Contains(id, "deepseek-v4-flash"):
		key = i18n.KeyModelDescriptionDeepSeekFlash
	case strings.Contains(id, "sonnet"):
		key = i18n.KeyModelDescriptionClaudeSonnet
	case strings.Contains(id, "opus"):
		key = i18n.KeyModelDescriptionClaudeOpus
	case strings.Contains(id, "haiku"):
		key = i18n.KeyModelDescriptionClaudeHaiku
	case strings.Contains(id, "gemini-2.5-pro"):
		key = i18n.KeyModelDescriptionGeminiPro
	case strings.Contains(id, "gemini-2.5-flash-lite"):
		key = i18n.KeyModelDescriptionGeminiFlashLite
	case strings.Contains(id, "gemini-2.5-flash"):
		key = i18n.KeyModelDescriptionGeminiFlash
	case entry.CanReason:
		key = i18n.KeyModelDescriptionReasoning
	case entry.CanSeeImages:
		key = i18n.KeyModelDescriptionMultimodal
	}
	return i18n.Text(lang, key)
}

func titleWords(s string) string {
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == '-' || r == '_' || unicode.IsSpace(r)
	})
	for i, part := range parts {
		if part == "" {
			continue
		}
		runes := []rune(strings.ToLower(part))
		runes[0] = unicode.ToUpper(runes[0])
		parts[i] = string(runes)
	}
	return strings.Join(parts, " ")
}

func (s *ModelPickerState) EnterEditLimits(entry ModelPickerEntry) {
	s.Phase = PickerPhaseEditLimits
	s.LimitEditModel = entry
	s.LimitEditError = ""
	if entry.ContextTokens > 0 {
		s.LimitEditInput = fmt.Sprintf("%d", entry.ContextTokens)
	} else {
		s.LimitEditInput = ""
	}
}

// EnterConnect transitions from provider phase to connect phase for an unconnected provider.
func (s *ModelPickerState) EnterConnect(providerName string, authMethods []string, envKey, baseURL, defaultBaseURL, hint string, options ...ConnectOptions) {
	s.Phase = PickerPhaseConnect
	s.ConnectProvider = providerName
	s.ConnectAuthMethods = authMethods
	s.ConnectSelectedAuth = 0
	s.ConnectAPIKeyInput = ""
	s.ConnectBaseURLInput = baseURL
	s.ConnectDefaultBaseURL = defaultBaseURL
	s.ConnectDefaultBaseURLs = nil
	s.ConnectAPIStyles = nil
	s.ConnectSelectedStyle = 0
	s.ConnectProviderNameInput = ""
	s.ConnectDynamicModels = false
	s.ConnectUserDefined = false
	s.ConnectInputField = ConnectInputAPIKey
	s.ConnectStatus = ""
	s.ConnectError = ""
	s.ConnectHint = hint
	s.IsReconnect = false
	if len(options) > 0 {
		option := options[0]
		s.ConnectDefaultBaseURLs = clonePickerBaseURLs(option.DefaultBaseURLs)
		s.ConnectAPIStyles = append([]provider.APIStyle(nil), option.APIStyles...)
		if len(s.ConnectAPIStyles) == 0 && option.DynamicModels {
			s.ConnectAPIStyles = []provider.APIStyle{provider.APIStyleOpenAI, provider.APIStyleAnthropic}
		}
		selectedStyle := provider.ParseAPIStyle(string(option.APIStyle))
		for index, style := range s.ConnectAPIStyles {
			if style == selectedStyle {
				s.ConnectSelectedStyle = index
				break
			}
		}
		s.ConnectDynamicModels = option.DynamicModels
		s.ConnectUserDefined = option.UserDefined
		if option.DynamicModels {
			s.ConnectInputField = ConnectInputAPIStyle
		}
	}
}

// EnterReconnect opens the connection flow for an already connected provider.
func (s *ModelPickerState) EnterReconnect(provider ProviderPickerEntry) {
	s.EnterProviderConnect(provider)
	s.IsReconnect = true
}

func (s *ModelPickerState) EnterProviderConnect(entry ProviderPickerEntry) {
	s.EnterConnect(entry.Name, entry.AuthMethods, entry.EnvKey, entry.BaseURL, entry.DefaultBaseURL, entry.SetupHint, ConnectOptions{
		APIStyles: entry.APIStyles, APIStyle: entry.APIStyle,
		DefaultBaseURLs: entry.DefaultBaseURLs, DynamicModels: entry.DynamicModels,
		UserDefined: entry.UserDefined || entry.IsCreate,
	})
	if entry.UserDefined && !entry.IsCreate {
		s.ConnectProviderNameInput = entry.DisplayName
	}
}

func clonePickerBaseURLs(values map[provider.APIStyle]string) map[provider.APIStyle]string {
	result := make(map[provider.APIStyle]string, len(values))
	for style, value := range values {
		result[style] = value
	}
	return result
}

func (s *ModelPickerState) EnterDeleteConfirm(providerName string) {
	s.Phase = PickerPhaseDeleteConfirm
	s.DeleteProvider = providerName
	s.DeleteError = ""
}

func (s *ModelPickerState) toggleConnectInputField() {
	fields := s.connectInputFields()
	for index, field := range fields {
		if field == s.ConnectInputField {
			s.ConnectInputField = fields[(index+1)%len(fields)]
			return
		}
	}
	s.ConnectInputField = fields[0]
}

func (s *ModelPickerState) connectInputFields() []ConnectInputField {
	if !s.ConnectDynamicModels {
		return []ConnectInputField{ConnectInputAPIKey, ConnectInputBaseURL}
	}
	fields := []ConnectInputField{ConnectInputAPIStyle}
	if s.ConnectUserDefined {
		fields = append(fields, ConnectInputProviderName)
	}
	return append(fields, ConnectInputBaseURL, ConnectInputAPIKey)
}

func (s *ModelPickerState) selectedAPIStyle() provider.APIStyle {
	if s.ConnectSelectedStyle >= 0 && s.ConnectSelectedStyle < len(s.ConnectAPIStyles) {
		return provider.ParseAPIStyle(string(s.ConnectAPIStyles[s.ConnectSelectedStyle]))
	}
	return provider.APIStyleOpenAI
}

func (s *ModelPickerState) changeAPIStyle(delta int) {
	if len(s.ConnectAPIStyles) < 2 {
		return
	}
	s.ConnectSelectedStyle = (s.ConnectSelectedStyle + delta) % len(s.ConnectAPIStyles)
	if s.ConnectSelectedStyle < 0 {
		s.ConnectSelectedStyle += len(s.ConnectAPIStyles)
	}
}

func (s *ModelPickerState) selectedDefaultBaseURL() string {
	if value := s.ConnectDefaultBaseURLs[s.selectedAPIStyle()]; value != "" {
		return value
	}
	return s.ConnectDefaultBaseURL
}

func (s *ModelPickerState) appendConnectInput(r rune) {
	if unicode.IsPrint(r) {
		if s.ConnectInputField == ConnectInputBaseURL {
			s.ConnectBaseURLInput += string(r)
		} else if s.ConnectInputField == ConnectInputProviderName {
			s.ConnectProviderNameInput += string(r)
		} else if s.ConnectInputField == ConnectInputAPIStyle {
			return
		} else {
			s.ConnectAPIKeyInput += string(r)
		}
	}
}

// appendConnectPaste inserts a bracketed-paste payload into the active
// connection input. Control characters are ignored so pasted newlines cannot
// submit or corrupt provider credentials.
func (s *ModelPickerState) appendConnectPaste(text string) {
	for _, r := range text {
		s.appendConnectInput(r)
	}
}

func (s *ModelPickerState) backspaceConnectInput() {
	input := &s.ConnectAPIKeyInput
	if s.ConnectInputField == ConnectInputBaseURL {
		input = &s.ConnectBaseURLInput
	} else if s.ConnectInputField == ConnectInputProviderName {
		input = &s.ConnectProviderNameInput
	} else if s.ConnectInputField == ConnectInputAPIStyle {
		return
	}
	if *input == "" {
		return
	}
	runes := []rune(*input)
	*input = string(runes[:len(runes)-1])
}

// maskedAPIKey returns a masked representation of the API key for display.
func (s *ModelPickerState) maskedAPIKey() string {
	n := len(s.ConnectAPIKeyInput)
	if n == 0 {
		return ""
	}
	if n <= 4 {
		return strings.Repeat("•", n)
	}
	return strings.Repeat("•", n-4) + s.ConnectAPIKeyInput[n-4:]
}
