package inspect

import (
	"encoding/json"
	"strconv"

	"github.com/agent-dance/luban/i18n"
)

// partialFailure is a bounded, structured local-presentation diagnostic. Raw
// request identifiers, kinds, and repository-relative paths remain fields;
// localized copy is confined to Message. The model wire receives the same
// safe read diagnostics through error_details.
type partialFailure struct {
	RequestID string `json:"request_id"`
	Kind      string `json:"kind"`
	Path      string `json:"path,omitempty"`
	Code      string `json:"code"`
	Message   string `json:"message"`
}

func inspectPartialDiagnosticMetadata(batch batchResult) map[string]string {
	failures := make([]partialFailure, 0)
	succeeded := 0
	failedRequests := 0
	lang := i18n.DetectOrLoadLanguage()
	for _, completed := range batch.requests {
		request := completed.result
		if len(request.Errors) == 0 && !request.SourcePartial {
			succeeded++
			continue
		}
		failedRequests++
		for _, requestError := range request.Errors {
			failures = append(failures, partialFailure{
				RequestID: request.ID, Kind: request.Kind, Path: request.Path,
				Code: requestError.Code, Message: requestError.Message,
			})
		}
		if len(request.Errors) == 0 && request.SourcePartial {
			reason := request.PartialReason
			if reason == "" {
				reason = "source_truncated"
			}
			failures = append(failures, partialFailure{
				RequestID: request.ID, Kind: request.Kind, Path: request.Path, Code: reason,
				Message: i18n.Format(lang, i18n.KeyToolInspectPartialReason, reason),
			})
		}
	}
	if len(failures) == 0 {
		return nil
	}
	encoded, err := json.Marshal(failures)
	if err != nil {
		return nil
	}
	return map[string]string{
		"inspect.partial_failures":         string(encoded),
		"inspect.partial_failure_count":    strconv.Itoa(failedRequests),
		"inspect.successful_request_count": strconv.Itoa(succeeded),
	}
}
