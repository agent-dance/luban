package compact

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"unicode/utf8"
)

const (
	progressiveInspectRewriteSchema   = "progressive-inspect-rewrite/v1"
	progressiveInspectIndexSchema     = "progressive-inspect-index/v1"
	progressiveInspectRewriteMaxBytes = 6 * 1024
	progressiveInspectIndexMaxBytes   = 6 * 1024
	progressiveInspectChunkMaxBytes   = 384
)

type progressiveInspectSource struct {
	Requests        []progressiveInspectSourceRequest  `json:"requests"`
	Evidence        []progressiveInspectSourceEvidence `json:"evidence"`
	HasMoreView     bool                               `json:"has_more_view"`
	SourceTruncated bool                               `json:"source_truncated"`
	Cursor          string                             `json:"cursor"`
}

type progressiveInspectSourceRequest struct {
	ID               string                          `json:"id"`
	Kind             string                          `json:"kind"`
	Path             string                          `json:"path"`
	Matches          []progressiveInspectSourceMatch `json:"matches"`
	SourceTruncated  bool                            `json:"source_truncated"`
	TruncationReason string                          `json:"truncation_reason"`
}

type progressiveInspectSourceMatch struct {
	Path  string `json:"path"`
	Items []struct {
		Line int `json:"line"`
	} `json:"items"`
}

type progressiveInspectSourceEvidence struct {
	Path   string `json:"path"`
	Chunks []struct {
		Lines   []int  `json:"lines"`
		Content string `json:"content"`
	} `json:"chunks"`
}

type progressiveInspectRewrite struct {
	Schema          string                              `json:"schema"`
	ContentSHA256   string                              `json:"content_sha256"`
	OriginalBytes   int                                 `json:"original_bytes"`
	Proof           json.RawMessage                     `json:"proof"`
	Requests        []progressiveInspectRewriteRequest  `json:"requests,omitempty"`
	Evidence        []progressiveInspectRewriteEvidence `json:"evidence,omitempty"`
	OmittedChunks   int                                 `json:"omitted_chunks,omitempty"`
	HasMoreView     bool                                `json:"has_more_view,omitempty"`
	SourceTruncated bool                                `json:"source_truncated,omitempty"`
	Cursor          string                              `json:"cursor,omitempty"`
}

type progressiveInspectRewriteRequest struct {
	ID               string                           `json:"id,omitempty"`
	Kind             string                           `json:"kind,omitempty"`
	Path             string                           `json:"path,omitempty"`
	Matches          []progressiveInspectRewriteMatch `json:"matches,omitempty"`
	SourceTruncated  bool                             `json:"source_truncated,omitempty"`
	TruncationReason string                           `json:"truncation_reason,omitempty"`
}

type progressiveInspectRewriteMatch struct {
	Path      string `json:"path"`
	Items     int    `json:"items"`
	FirstLine int    `json:"first_line,omitempty"`
	LastLine  int    `json:"last_line,omitempty"`
}

type progressiveInspectRewriteEvidence struct {
	Path   string                           `json:"path"`
	Chunks []progressiveInspectRewriteChunk `json:"chunks,omitempty"`
}

type progressiveInspectRewriteChunk struct {
	Lines   []int  `json:"lines,omitempty"`
	Content string `json:"content,omitempty"`
}

// progressiveInspectRewriteContent retains a bounded, extractive source map.
// Unlike the content-free proof, it preserves paths, line ranges, exact head
// and tail text for selected snippets, pagination state, and the deterministic
// proof. Non-JSON or oversized skeletons fail closed instead of discarding
// source semantics.
func progressiveInspectRewriteContent(original, proof string) (string, bool) {
	source, rewrite, ok := newProgressiveInspectProjection(original, proof, progressiveInspectRewriteSchema)
	if !ok {
		return "", false
	}
	for _, evidence := range source.Evidence {
		rewrite.Evidence = append(rewrite.Evidence, progressiveInspectRewriteEvidence{Path: evidence.Path})
		for _, chunk := range evidence.Chunks {
			candidate := progressiveInspectRewriteChunk{
				Lines: append([]int(nil), chunk.Lines...), Content: boundedInspectChunk(chunk.Content),
			}
			last := len(rewrite.Evidence) - 1
			rewrite.Evidence[last].Chunks = append(rewrite.Evidence[last].Chunks, candidate)
			encoded, err := json.Marshal(rewrite)
			if err != nil {
				return "", false
			}
			if len(encoded) > progressiveInspectRewriteMaxBytes {
				rewrite.Evidence[last].Chunks = rewrite.Evidence[last].Chunks[:len(rewrite.Evidence[last].Chunks)-1]
				rewrite.OmittedChunks++
			}
		}
	}
	encoded, err := json.Marshal(rewrite)
	if err != nil || len(encoded) > progressiveInspectRewriteMaxBytes || len(encoded) >= len(original) {
		return "", false
	}
	return string(encoded), true
}

// progressiveInspectIndexContent is the uncertainty fallback for an old
// source read whose rich extractive rewrite cannot prevent immediate semantic
// compaction. It retains every repository path and chunk line range, exact
// search cardinalities, pagination state, the original digest, and the typed
// proof, but omits source text. The model can therefore re-read a precise
// location instead of relying on an invented summary.
func progressiveInspectIndexContent(original, proof string) (string, bool) {
	source, index, ok := newProgressiveInspectProjection(original, proof, progressiveInspectIndexSchema)
	if !ok {
		return "", false
	}
	for _, evidence := range source.Evidence {
		indexed := progressiveInspectRewriteEvidence{Path: evidence.Path}
		for _, chunk := range evidence.Chunks {
			indexed.Chunks = append(indexed.Chunks, progressiveInspectRewriteChunk{Lines: append([]int(nil), chunk.Lines...)})
		}
		index.Evidence = append(index.Evidence, indexed)
	}
	encoded, err := json.Marshal(index)
	if err != nil || len(encoded) > progressiveInspectIndexMaxBytes || len(encoded) >= len(original) {
		return "", false
	}
	return string(encoded), true
}

func newProgressiveInspectProjection(original, proof, schema string) (progressiveInspectSource, progressiveInspectRewrite, bool) {
	var source progressiveInspectSource
	if json.Unmarshal([]byte(original), &source) != nil || len(source.Evidence) == 0 {
		return progressiveInspectSource{}, progressiveInspectRewrite{}, false
	}
	var proofJSON json.RawMessage
	if json.Unmarshal([]byte(proof), &proofJSON) != nil {
		return progressiveInspectSource{}, progressiveInspectRewrite{}, false
	}
	digest := sha256.Sum256([]byte(original))
	rewrite := progressiveInspectRewrite{
		Schema: schema, ContentSHA256: hex.EncodeToString(digest[:]),
		OriginalBytes: len(original), Proof: proofJSON,
		HasMoreView: source.HasMoreView, SourceTruncated: source.SourceTruncated, Cursor: source.Cursor,
	}
	for _, request := range source.Requests {
		item := progressiveInspectRewriteRequest{
			ID: request.ID, Kind: request.Kind, Path: request.Path,
			SourceTruncated: request.SourceTruncated, TruncationReason: request.TruncationReason,
		}
		for _, match := range request.Matches {
			summary := progressiveInspectRewriteMatch{Path: match.Path, Items: len(match.Items)}
			if len(match.Items) > 0 {
				summary.FirstLine = match.Items[0].Line
				summary.LastLine = match.Items[len(match.Items)-1].Line
			}
			item.Matches = append(item.Matches, summary)
		}
		rewrite.Requests = append(rewrite.Requests, item)
	}
	return source, rewrite, true
}

func boundedInspectChunk(value string) string {
	if len(value) <= progressiveInspectChunkMaxBytes {
		return value
	}
	half := (progressiveInspectChunkMaxBytes - len("\n<snip/>\n")) / 2
	head := value[:half]
	for len(head) > 0 && !utf8.ValidString(head) {
		head = head[:len(head)-1]
	}
	tail := value[len(value)-half:]
	for len(tail) > 0 && !utf8.ValidString(tail) {
		tail = tail[1:]
	}
	return head + "\n<snip/>\n" + tail
}
