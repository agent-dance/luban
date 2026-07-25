package compact

import (
	"context"
	"reflect"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/types"
)

// ReactiveCompactOptions controls the prompt-overflow recovery compaction path.
type ReactiveCompactOptions struct {
	Compactor    Compactor
	HasAttempted bool
	MediaStrip   bool
	KeepRecent   int
	Trigger      string
}

// ReactiveCompactorUnavailableError indicates that overflow recovery could not
// perform semantic compaction. It is intentionally typed so callers can
// distinguish fail-closed recovery from a provider or summarizer failure.
type ReactiveCompactorUnavailableError struct{}

func (*ReactiveCompactorUnavailableError) Error() string {
	return i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyCompactReactiveCompactorUnavailable)
}

// TryReactiveCompact attempts a single recovery transformation after the
// provider rejects a request for context/media size reasons. The caller owns the
// HasAttempted guard so repeated provider failures surface instead of looping.
func TryReactiveCompact(ctx context.Context, messages []types.Message, opts ReactiveCompactOptions) (*CompactionResult, bool, error) {
	if opts.HasAttempted {
		return nil, false, nil
	}
	compactor := opts.Compactor
	if compactor == nil {
		// Missing recovery configuration fails closed and leaves the
		// caller-owned history untouched.
		return nil, true, &ReactiveCompactorUnavailableError{}
	}
	trigger := opts.Trigger
	if trigger == "" {
		trigger = "reactive"
	}

	compactionInput := messages
	if opts.MediaStrip {
		stripped := StripImagesFromMessages(messages)
		if reflect.DeepEqual(stripped, messages) {
			return nil, false, nil
		}
		compactionInput = stripped
	}

	var (
		result *CompactionResult
		err    error
	)
	if triggered, ok := compactor.(TriggeredCompactor); ok {
		result, err = triggered.CompactWithTrigger(ctx, compactionInput, opts.KeepRecent, trigger)
	} else {
		result, err = compactor.Compact(ctx, compactionInput, opts.KeepRecent)
	}
	if err != nil {
		return nil, true, err
	}
	if result == nil {
		return nil, false, nil
	}
	if opts.MediaStrip {
		result = restoreMediaInPreservedMessages(result, compactionInput, messages)
	}
	if reflect.DeepEqual(BuildPostCompactMessages(result), messages) {
		return nil, false, nil
	}
	return result, true, nil
}

func restoreMediaInPreservedMessages(result *CompactionResult, stripped, original []types.Message) *CompactionResult {
	if result == nil || len(result.MessagesToKeep) == 0 || len(stripped) != len(original) {
		return result
	}

	restored := *result
	restored.MessagesToKeep = restoreMessagesFromOriginal(result.MessagesToKeep, stripped, original)
	return &restored
}

func restoreMessagesFromOriginal(kept, stripped, original []types.Message) []types.Message {
	if len(kept) == 0 {
		return nil
	}
	out := make([]types.Message, len(kept))
	copy(out, kept)

	if start, ok := findContiguousMessages(kept, stripped); ok {
		copy(out, original[start:start+len(kept)])
		return out
	}

	used := make([]bool, len(stripped))
	for i, msg := range kept {
		for j, candidate := range stripped {
			if used[j] || !reflect.DeepEqual(msg, candidate) {
				continue
			}
			out[i] = original[j]
			used[j] = true
			break
		}
	}
	return out
}

func findContiguousMessages(needle, haystack []types.Message) (int, bool) {
	if len(needle) == 0 || len(needle) > len(haystack) {
		return 0, false
	}
	for start := 0; start <= len(haystack)-len(needle); start++ {
		if reflect.DeepEqual(needle, haystack[start:start+len(needle)]) {
			return start, true
		}
	}
	return 0, false
}
