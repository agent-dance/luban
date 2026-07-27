// Package cacheevidence projects prompt-cache request policy without retaining
// prompt text, cache keys, or other request content.
package cacheevidence

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"
)

// RequestPolicy is a content-free projection of one decoded provider request.
// Presence fields distinguish an omitted protocol option from an explicit
// value. ShapeValid reports only JSON/type validity; it does not claim that an
// upstream accepted or honored the policy.
type RequestPolicy struct {
	Observed                       bool     `json:"observed"`
	ShapeValid                     bool     `json:"shape_valid"`
	PromptCacheKeyPresent          bool     `json:"prompt_cache_key_present"`
	PromptCacheKeySHA256           string   `json:"prompt_cache_key_sha256,omitempty"`
	PromptCacheOptionsPresent      bool     `json:"prompt_cache_options_present"`
	PromptCacheOptionsMode         string   `json:"prompt_cache_options_mode,omitempty"`
	PromptCacheOptionsTTLPresent   bool     `json:"prompt_cache_options_ttl_present"`
	PromptCacheOptionsTTL          string   `json:"prompt_cache_options_ttl,omitempty"`
	PromptCacheOptionsTTLSeconds   *int64   `json:"prompt_cache_options_ttl_seconds,omitempty"`
	PromptCacheRetentionPresent    bool     `json:"prompt_cache_retention_present"`
	PromptCacheRetention           string   `json:"prompt_cache_retention,omitempty"`
	PromptCacheBreakpointCount     int      `json:"prompt_cache_breakpoint_count"`
	PromptCacheBreakpointPositions []string `json:"prompt_cache_breakpoint_position_hashes,omitempty"`
}

// InspectRequest decodes one JSON request and returns only cache-policy
// metadata. The returned boolean is false when the request is not one complete
// JSON object. A syntactically valid request can still have ShapeValid=false.
func InspectRequest(body []byte) (RequestPolicy, bool) {
	request, ok := decodeUniqueJSONObject(body)
	if !ok {
		return RequestPolicy{}, false
	}

	policy := RequestPolicy{Observed: true, ShapeValid: true}
	if raw, present := request["prompt_cache_key"]; present {
		key, ok := raw.(string)
		if !ok || key == "" {
			policy.ShapeValid = false
		} else {
			policy.PromptCacheKeyPresent = true
			policy.PromptCacheKeySHA256 = hashDomain("prompt-cache-key", key)
		}
	}

	if raw, present := request["prompt_cache_options"]; present {
		policy.PromptCacheOptionsPresent = true
		options, ok := raw.(map[string]any)
		if !ok {
			policy.ShapeValid = false
		} else {
			if rawMode, exists := options["mode"]; exists {
				mode, valid := rawMode.(string)
				if !valid || mode == "" {
					policy.ShapeValid = false
				} else {
					policy.PromptCacheOptionsMode = mode
				}
			}
			if rawTTL, exists := options["ttl"]; exists {
				policy.PromptCacheOptionsTTLPresent = true
				ttl, valid := rawTTL.(string)
				duration, durationErr := time.ParseDuration(ttl)
				if !valid || ttl == "" || durationErr != nil || duration <= 0 || duration%time.Second != 0 {
					policy.ShapeValid = false
				} else {
					seconds := int64(duration / time.Second)
					policy.PromptCacheOptionsTTL = ttl
					policy.PromptCacheOptionsTTLSeconds = &seconds
				}
			}
		}
	}

	if raw, present := request["prompt_cache_retention"]; present {
		policy.PromptCacheRetentionPresent = true
		retention, ok := raw.(string)
		if !ok || retention == "" {
			policy.ShapeValid = false
		} else {
			policy.PromptCacheRetention = retention
		}
	}

	positions, valid := breakpointPositions(request)
	policy.ShapeValid = policy.ShapeValid && valid
	policy.PromptCacheBreakpointCount = len(positions)
	policy.PromptCacheBreakpointPositions = make([]string, 0, len(positions))
	for _, position := range positions {
		policy.PromptCacheBreakpointPositions = append(policy.PromptCacheBreakpointPositions, hashDomain("prompt-cache-breakpoint-position", position))
	}
	return policy, true
}

// LineageSummary describes cache-key continuity within one physical run. It
// deliberately does not label a run cold or warm: that state cannot be proved
// from a request key or a gateway cache-token receipt alone.
type LineageSummary struct {
	ObservedRequests   int    `json:"observed_requests"`
	InvalidRequests    int    `json:"invalid_requests"`
	KeyPresentRequests int    `json:"key_present_requests"`
	UniqueKeyCount     int    `json:"unique_key_count"`
	KeyTransitions     int    `json:"key_transitions"`
	FirstKeySHA256     string `json:"first_key_sha256,omitempty"`
	Stable             bool   `json:"stable"`
}

// SummarizeLineage counts unique keys and transitions without retaining keys.
func SummarizeLineage(policies []RequestPolicy) LineageSummary {
	summary := LineageSummary{}
	unique := make(map[string]struct{})
	previous := ""
	previousPresent := false
	for _, policy := range policies {
		if !policy.Observed {
			continue
		}
		summary.ObservedRequests++
		if !policy.ShapeValid {
			summary.InvalidRequests++
		}
		present := policy.PromptCacheKeyPresent && policy.PromptCacheKeySHA256 != ""
		if present {
			summary.KeyPresentRequests++
			if summary.FirstKeySHA256 == "" {
				summary.FirstKeySHA256 = policy.PromptCacheKeySHA256
			}
			unique[policy.PromptCacheKeySHA256] = struct{}{}
		}
		if summary.ObservedRequests > 1 && (present != previousPresent || present && policy.PromptCacheKeySHA256 != previous) {
			summary.KeyTransitions++
		}
		previous = policy.PromptCacheKeySHA256
		previousPresent = present
	}
	summary.UniqueKeyCount = len(unique)
	summary.Stable = summary.ObservedRequests > 0 && summary.InvalidRequests == 0 && summary.KeyPresentRequests == summary.ObservedRequests && summary.UniqueKeyCount == 1 && summary.KeyTransitions == 0
	return summary
}

func breakpointPositions(request map[string]any) ([]string, bool) {
	positions := make([]string, 0)
	allowed := allowedBreakpointPositions(request)
	valid := true
	var walk func(any, string)
	walk = func(value any, pointer string) {
		switch typed := value.(type) {
		case map[string]any:
			keys := make([]string, 0, len(typed))
			for key := range typed {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				childPointer := pointer + "/" + escapeJSONPointerToken(key)
				if key == "prompt_cache_breakpoint" {
					marker, ok := typed[key].(map[string]any)
					mode, modeOK := marker["mode"].(string)
					if !ok || !modeOK || mode != "explicit" || len(marker) != 1 {
						valid = false
					}
					positions = append(positions, childPointer)
					if _, ok := allowed[childPointer]; !ok {
						valid = false
					}
				}
				walk(typed[key], childPointer)
			}
		case []any:
			for index, child := range typed {
				walk(child, pointer+"/"+strconv.Itoa(index))
			}
		}
	}
	walk(request, "")
	sort.Strings(positions)
	return positions, valid
}

func allowedBreakpointPositions(request map[string]any) map[string]struct{} {
	allowed := make(map[string]struct{})
	for _, root := range []string{"input", "messages"} {
		items, _ := request[root].([]any)
		for itemIndex, rawItem := range items {
			item, _ := rawItem.(map[string]any)
			content, _ := item["content"].([]any)
			for contentIndex, rawBlock := range content {
				if _, ok := rawBlock.(map[string]any); ok {
					pointer := "/" + root + "/" + strconv.Itoa(itemIndex) + "/content/" + strconv.Itoa(contentIndex) + "/prompt_cache_breakpoint"
					allowed[pointer] = struct{}{}
				}
			}
		}
	}
	return allowed
}

func escapeJSONPointerToken(value string) string {
	value = strings.ReplaceAll(value, "~", "~0")
	return strings.ReplaceAll(value, "/", "~1")
}

func hashDomain(domain, value string) string {
	digest := sha256.Sum256([]byte("agentic-bench/cache-evidence/v1\x00" + domain + "\x00" + value))
	return hex.EncodeToString(digest[:])
}

func decodeUniqueJSONObject(raw []byte) (map[string]any, bool) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	value, ok := decodeUniqueJSONValue(decoder)
	if !ok {
		return nil, false
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, false
	}
	object, ok := value.(map[string]any)
	return object, ok
}

func decodeUniqueJSONValue(decoder *json.Decoder) (any, bool) {
	token, err := decoder.Token()
	if err != nil {
		return nil, false
	}
	delimiter, composite := token.(json.Delim)
	if !composite {
		return token, true
	}
	switch delimiter {
	case '{':
		result := make(map[string]any)
		for decoder.More() {
			keyToken, err := decoder.Token()
			key, keyOK := keyToken.(string)
			if err != nil || !keyOK {
				return nil, false
			}
			if _, duplicate := result[key]; duplicate {
				return nil, false
			}
			value, ok := decodeUniqueJSONValue(decoder)
			if !ok {
				return nil, false
			}
			result[key] = value
		}
		closing, err := decoder.Token()
		return result, err == nil && closing == json.Delim('}')
	case '[':
		result := make([]any, 0)
		for decoder.More() {
			value, ok := decodeUniqueJSONValue(decoder)
			if !ok {
				return nil, false
			}
			result = append(result, value)
		}
		closing, err := decoder.Token()
		return result, err == nil && closing == json.Delim(']')
	default:
		return nil, false
	}
}
