package tools

import (
	"fmt"
	"strings"

	"github.com/agent-dance/luban/i18n"
)

// AskUser schema constants — mirror the TS schema in
// src/tools/AskUserQuestionTool/schema.ts.
const (
	askUserMinQuestions     = 1
	askUserMaxQuestions     = 4
	askUserMinOptions       = 2
	askUserMaxOptions       = 4
	askUserMaxHeaderLength  = 12
	askUserMaxLabelWords    = 5
	askUserMaxPreviewLength = 4000
	askUserOtherSentinel    = "Other:"
	askUserOtherLabel       = "Other"
)

// ValidateAskUserQuestions performs strict schema validation matching the TS
// reference. Returns nil when the questions are well-formed.
func ValidateAskUserQuestions(questions []QuestionSpec) error {
	if len(questions) < askUserMinQuestions {
		return fmt.Errorf("%s", toolRuntimeFormat(i18n.KeyToolRuntimeAskUserMinQuestions, askUserMinQuestions))
	}
	if len(questions) > askUserMaxQuestions {
		return fmt.Errorf("%s", toolRuntimeFormat(i18n.KeyToolRuntimeAskUserMaxQuestions, askUserMaxQuestions, len(questions)))
	}
	seen := make(map[string]struct{}, len(questions))
	for i, q := range questions {
		if err := validateAskUserQuestion(q); err != nil {
			return fmt.Errorf("%s: %w", toolRuntimeFormat(i18n.KeyToolRuntimeAskUserQuestionContext, i+1, q.Question), err)
		}
		key := strings.ToLower(strings.TrimSpace(q.Question))
		if _, dup := seen[key]; dup {
			return fmt.Errorf("%s", toolRuntimeFormat(i18n.KeyToolRuntimeAskUserDuplicateQuestion, i+1, q.Question))
		}
		seen[key] = struct{}{}
	}
	// Mirror src/tools/AskUserQuestionTool/AskUserQuestionTool.tsx:158-181:
	// when the harness has opted into html preview rendering, run the same
	// fragment-shape sanity check the TS validator performs. The format is
	// resolved from the package-level setter or the
	// CLAUDE_ASKUSER_PREVIEW_FORMAT env override; default is "" (no check).
	if format := ResolveAskUserPreviewFormat(); format != "" {
		if err := ValidateHtmlPreviewForQuestions(questions, format); err != nil {
			return err
		}
	}
	return nil
}

func validateAskUserQuestion(q QuestionSpec) error {
	text := strings.TrimSpace(q.Question)
	if text == "" {
		return fmt.Errorf("%s", toolRuntimeText(i18n.KeyToolRuntimeAskUserQuestionRequired))
	}
	if !strings.HasSuffix(text, "?") {
		return fmt.Errorf("%s", toolRuntimeText(i18n.KeyToolRuntimeAskUserQuestionMarkRequired))
	}

	header := strings.TrimSpace(q.Header)
	if header == "" {
		return fmt.Errorf("%s", toolRuntimeText(i18n.KeyToolRuntimeAskUserHeaderRequired))
	}
	if len([]rune(header)) > askUserMaxHeaderLength {
		return fmt.Errorf("%s", toolRuntimeFormat(i18n.KeyToolRuntimeAskUserHeaderTooLong, header, askUserMaxHeaderLength))
	}

	if len(q.Options) < askUserMinOptions {
		return fmt.Errorf("%s", toolRuntimeFormat(i18n.KeyToolRuntimeAskUserMinOptions, askUserMinOptions, len(q.Options)))
	}
	if len(q.Options) > askUserMaxOptions {
		return fmt.Errorf("%s", toolRuntimeFormat(i18n.KeyToolRuntimeAskUserMaxOptions, askUserMaxOptions, len(q.Options)))
	}

	labels := make(map[string]struct{}, len(q.Options))
	for j, opt := range q.Options {
		if err := validateAskUserOption(opt, q.MultiSelect); err != nil {
			return fmt.Errorf("%s: %w", toolRuntimeFormat(i18n.KeyToolRuntimeAskUserOptionContext, j+1, opt.Label), err)
		}
		labelKey := strings.ToLower(strings.TrimSpace(opt.Label))
		if _, dup := labels[labelKey]; dup {
			return fmt.Errorf("%s", toolRuntimeFormat(i18n.KeyToolRuntimeAskUserDuplicateOption, j+1, opt.Label))
		}
		labels[labelKey] = struct{}{}
	}
	return nil
}

func validateAskUserOption(opt OptionSpec, multiSelect bool) error {
	label := strings.TrimSpace(opt.Label)
	if label == "" {
		return fmt.Errorf("%s", toolRuntimeText(i18n.KeyToolRuntimeAskUserLabelRequired))
	}
	if words := len(strings.Fields(label)); words > askUserMaxLabelWords {
		return fmt.Errorf("%s", toolRuntimeFormat(i18n.KeyToolRuntimeAskUserLabelTooLong, words))
	}
	if strings.EqualFold(label, askUserOtherLabel) {
		return fmt.Errorf("%s", toolRuntimeText(i18n.KeyToolRuntimeAskUserOtherReserved))
	}

	if opt.Preview != "" {
		if multiSelect {
			return fmt.Errorf("%s", toolRuntimeText(i18n.KeyToolRuntimeAskUserPreviewMultiSelect))
		}
		if len(opt.Preview) > askUserMaxPreviewLength {
			return fmt.Errorf("%s", toolRuntimeFormat(i18n.KeyToolRuntimeAskUserPreviewTooLong, askUserMaxPreviewLength))
		}
	}
	return nil
}

// QuestionHasPreview reports whether any option in the question has a preview
// payload (used by the UI to decide between stacked and side-by-side rendering).
func QuestionHasPreview(q QuestionSpec) bool {
	for _, o := range q.Options {
		if o.Preview != "" {
			return true
		}
	}
	return false
}
