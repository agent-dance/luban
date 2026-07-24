package hooks

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sort"

	"github.com/agent-dance/luban/i18n"
)

// EventHookConfig holds per-event hook configuration as stored in
// .claude/settings.json under the "hooks" key.
//
// Schema (mirrors the TS HooksSettings type):
//
//	{
//	  "hooks": {
//	    "PreToolUse": [
//	      {
//	        "matcher": "Bash",
//	        "hooks": [
//	          {"type": "command", "command": "echo hi", "timeout": 5},
//	          {"type": "http",    "url": "https://example.com/hook"}
//	        ]
//	      }
//	    ],
//	    "Notification": [
//	      {
//	        "hooks": [{"type": "notification"}]
//	      }
//	    ]
//	  }
//	}
//
// The legacy flat array format is also supported for backward compatibility:
//
//	{"hooks": [{"type":"PreToolUse","command":"echo hi","timeout":5}]}

// HookMatcherConfig is a single matcher entry within an event config.
type HookMatcherConfig struct {
	// Matcher is an optional glob/string pattern for the tool name.
	Matcher string `json:"matcher,omitempty"`
	// Hooks is the list of hooks to run when the matcher matches.
	Hooks []HookConfig `json:"hooks"`
}

// HookConfig is the per-hook configuration used in the rich event-map format.
type HookConfig struct {
	// Type is the execution strategy: "command", "http", or "notification".
	Type string `json:"type"`

	// command fields
	Command string `json:"command,omitempty"`
	Timeout int    `json:"timeout,omitempty"`

	// http fields
	URL        string            `json:"url,omitempty"`
	Headers    map[string]string `json:"headers,omitempty"`
	RetryCount int               `json:"retry_count,omitempty"`

	// matcher (tool-name filter, propagated from HookMatcherConfig.Matcher)
	Matcher string `json:"matcher,omitempty"`
}

// rawSettings is used for flexible JSON unmarshalling that handles both the
// legacy flat-array format and the new event-map format.
type rawSettings struct {
	// Legacy: flat list of Hook structs keyed by "hooks"
	HooksFlat []Hook `json:"hooks"`
	// Rich: map from event type to list of matcher configs
	HooksMap map[string][]HookMatcherConfig `json:"-"`
}

func (rs *rawSettings) UnmarshalJSON(data []byte) error {
	// Probe the "hooks" value to detect array-of-Hook vs object format.
	var probe struct {
		Hooks json.RawMessage `json:"hooks"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return err
	}
	if len(probe.Hooks) == 0 {
		return nil
	}

	if probe.Hooks[0] == '[' {
		// Legacy flat array
		var legacy struct {
			Hooks []Hook `json:"hooks"`
		}
		if err := json.Unmarshal(data, &legacy); err != nil {
			return fmt.Errorf("%s: %w", i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyHookConfigLegacyParse), err)
		}
		rs.HooksFlat = legacy.Hooks
		return nil
	}

	if probe.Hooks[0] == '{' {
		// Rich event-map format
		var rich struct {
			Hooks map[string][]HookMatcherConfig `json:"hooks"`
		}
		if err := json.Unmarshal(data, &rich); err != nil {
			return fmt.Errorf("%s: %w", i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyHookConfigMapParse), err)
		}
		rs.HooksMap = rich.Hooks
		return nil
	}

	return fmt.Errorf("%s", i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyHookConfigUnexpected, probe.Hooks[0]))
}

// validHookTypes is the set of known HookType event names.
// C2: Unknown event names log a warning but are not rejected, preserving
// forward compatibility with future event types added server-side.
var validHookTypes = map[string]bool{
	string(HookPreToolUse):         true,
	string(HookPostToolUse):        true,
	string(HookPostToolUseFailure): true,
	string(HookSessionStart):       true,
	string(HookSessionEnd):         true,
	string(HookUserPromptSubmit):   true,
	string(HookStop):               true,
	string(HookPreQuery):           true,
	string(HookPostQuery):          true,
	string(HookPostSampling):       true,
	string(HookStopFailure):        true,
	string(HookNotification):       true,
	string(HookPreCompact):         true,
	string(HookPostCompact):        true,
	string(HookSubagentStart):      true,
	string(HookSubagentStop):       true,
	string(HookTeammateIdle):       true,
	string(HookTaskCreated):        true,
	string(HookTaskCompleted):      true,
}

// LoadConfig loads hook configuration from a settings.json file and returns a
// Runner that dispatches the appropriate hook kind for each event type.
//
// Both legacy (flat array) and rich (event-map) formats are supported.
func LoadConfig(settingsPath string) (*Runner, error) {
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &Runner{}, nil
		}
		return nil, err
	}
	return LoadConfigData(data, settingsPath)
}

// LoadConfigData parses hook configuration bytes. The source label is only used
// in error messages so callers can parse frontmatter hooks without a temp file.
func LoadConfigData(data []byte, source string) (*Runner, error) {
	var raw rawSettings
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyHookConfigSettingsParse, source), err)
	}

	// Legacy path: flat array already has all fields set.
	if len(raw.HooksFlat) > 0 {
		return NewRunner(raw.HooksFlat), nil
	}

	// Rich path: convert event-map to []Hook.
	var hooks []Hook
	eventNames := make([]string, 0, len(raw.HooksMap))
	for eventName := range raw.HooksMap {
		eventNames = append(eventNames, eventName)
	}
	sort.Strings(eventNames)
	for _, eventName := range eventNames {
		matchers := raw.HooksMap[eventName]
		// C2: Warn on unknown event type names; don't fail so that configs
		// written by a newer server version still load cleanly.
		if !validHookTypes[eventName] {
			slog.Warn(i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyLogHookUnknownEvent),
				"event", eventName, "settings", source)
			continue
		}
		hookType := HookType(eventName)
		for _, m := range matchers {
			for _, hc := range m.Hooks {
				h, err := hookConfigToHook(hookType, m.Matcher, hc)
				if err != nil {
					return nil, fmt.Errorf("%s: %w", i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyHookConfigEventInvalid, eventName), err)
				}
				hooks = append(hooks, h)
			}
		}
	}
	return NewRunner(hooks), nil
}

// hookConfigToHook converts a HookConfig into a Hook.
// C1: Returns an error for unrecognised hook kind values so callers get a
// clear message rather than silently dispatching to the wrong executor.
func hookConfigToHook(hookType HookType, matcher string, hc HookConfig) (Hook, error) {
	kind := HookKind(hc.Type)
	switch kind {
	case HookKindCommand, HookKindHTTP, HookKindNotification, "":
		// valid kinds; empty string is normalised to HookKindCommand below
	default:
		return Hook{}, fmt.Errorf("%s", i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyHookConfigKindUnknown, hc.Type))
	}
	if kind == "" {
		kind = HookKindCommand
	}
	h := Hook{
		Type:       hookType,
		Kind:       kind,
		Command:    hc.Command,
		Timeout:    hc.Timeout,
		URL:        hc.URL,
		Headers:    hc.Headers,
		RetryCount: hc.RetryCount,
		Matcher:    matcher,
	}
	if h.Matcher == "" {
		h.Matcher = hc.Matcher
	}
	return h, nil
}
