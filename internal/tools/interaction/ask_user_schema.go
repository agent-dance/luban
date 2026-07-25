package interaction

import (
	"fmt"
	"strings"

	"github.com/agent-dance/luban/i18n"
	interactioncontract "github.com/agent-dance/luban/internal/contracts/interaction"
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
func ValidateAskUserQuestions(questions []interactioncontract.QuestionSpec) error {
	if len(questions) < askUserMinQuestions {
		return fmt.Errorf("%s", i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyToolRuntimeAskUserMinQuestions, askUserMinQuestions))
	}
	if len(questions) > askUserMaxQuestions {
		return fmt.Errorf("%s", i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyToolRuntimeAskUserMaxQuestions, askUserMaxQuestions, len(questions)))
	}
	seen := make(map[string]struct{}, len(questions))
	for i, q := range questions {
		if err := validateAskUserQuestion(q); err != nil {
			return fmt.Errorf("%s: %w", i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyToolRuntimeAskUserQuestionContext, i+1, q.Question), err)
		}
		key := strings.ToLower(strings.TrimSpace(q.Question))
		if _, dup := seen[key]; dup {
			return fmt.Errorf("%s", i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyToolRuntimeAskUserDuplicateQuestion, i+1, q.Question))
		}
		seen[key] = struct{}{}
	}
	return nil
}

func validateAskUserQuestion(q interactioncontract.QuestionSpec) error {
	text := strings.TrimSpace(q.Question)
	if text == "" {
		return fmt.Errorf("%s", i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolRuntimeAskUserQuestionRequired))
	}
	if !strings.HasSuffix(text, "?") {
		return fmt.Errorf("%s", i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolRuntimeAskUserQuestionMarkRequired))
	}

	header := strings.TrimSpace(q.Header)
	if header == "" {
		return fmt.Errorf("%s", i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolRuntimeAskUserHeaderRequired))
	}
	if len([]rune(header)) > askUserMaxHeaderLength {
		return fmt.Errorf("%s", i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyToolRuntimeAskUserHeaderTooLong, header, askUserMaxHeaderLength))
	}

	if len(q.Options) < askUserMinOptions {
		return fmt.Errorf("%s", i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyToolRuntimeAskUserMinOptions, askUserMinOptions, len(q.Options)))
	}
	if len(q.Options) > askUserMaxOptions {
		return fmt.Errorf("%s", i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyToolRuntimeAskUserMaxOptions, askUserMaxOptions, len(q.Options)))
	}

	labels := make(map[string]struct{}, len(q.Options))
	for j, opt := range q.Options {
		if err := validateAskUserOption(opt, q.MultiSelect); err != nil {
			return fmt.Errorf("%s: %w", i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyToolRuntimeAskUserOptionContext, j+1, opt.Label), err)
		}
		labelKey := strings.ToLower(strings.TrimSpace(opt.Label))
		if _, dup := labels[labelKey]; dup {
			return fmt.Errorf("%s", i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyToolRuntimeAskUserDuplicateOption, j+1, opt.Label))
		}
		labels[labelKey] = struct{}{}
	}
	return nil
}

func validateAskUserOption(opt interactioncontract.OptionSpec, multiSelect bool) error {
	label := strings.TrimSpace(opt.Label)
	if label == "" {
		return fmt.Errorf("%s", i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolRuntimeAskUserLabelRequired))
	}
	if words := len(strings.Fields(label)); words > askUserMaxLabelWords {
		return fmt.Errorf("%s", i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyToolRuntimeAskUserLabelTooLong, words))
	}
	if strings.EqualFold(label, askUserOtherLabel) {
		return fmt.Errorf("%s", i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolRuntimeAskUserOtherReserved))
	}

	if opt.Preview != "" {
		if multiSelect {
			return fmt.Errorf("%s", i18n.Text(i18n.DetectOrLoadLanguage(), i18n.KeyToolRuntimeAskUserPreviewMultiSelect))
		}
		if len(opt.Preview) > askUserMaxPreviewLength {
			return fmt.Errorf("%s", i18n.Format(i18n.DetectOrLoadLanguage(), i18n.KeyToolRuntimeAskUserPreviewTooLong, askUserMaxPreviewLength))
		}
	}
	return nil
}
