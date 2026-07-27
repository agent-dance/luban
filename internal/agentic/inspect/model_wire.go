package inspect

import (
	"encoding/json"
	"sort"
	"strings"
)

// modelResult is the compact, stable Inspect wire protocol. The rich Result
// remains available locally as typed Data; the provider only needs request
// routing, grouped matches, source evidence, completeness, and a cursor.
type modelResult struct {
	Requests          []modelRequest      `json:"requests"`
	Evidence          []modelEvidenceFile `json:"evidence,omitempty"`
	HasMoreView       bool                `json:"has_more_view"`
	SourceTruncated   bool                `json:"source_truncated"`
	OmittedRequestIDs []string            `json:"omitted_request_ids,omitempty"`
	Cursor            string              `json:"cursor,omitempty"`
	Stats             modelResultStats    `json:"stats"`
}

type modelRequest struct {
	ID               string            `json:"id"`
	Kind             string            `json:"kind"`
	Path             string            `json:"path,omitempty"`
	Files            []string          `json:"files,omitempty"`
	Matches          []modelMatchGroup `json:"matches,omitempty"`
	Errors           []string          `json:"errors,omitempty"`
	SourceTruncated  bool              `json:"source_truncated,omitempty"`
	TruncationReason string            `json:"truncation_reason,omitempty"`
}

type modelMatchGroup struct {
	Path  string           `json:"path"`
	Items []modelMatchItem `json:"items"`
}

type modelMatchItem struct {
	Line int    `json:"line"`
	Text string `json:"text,omitempty"`
}

type modelEvidenceFile struct {
	Path   string               `json:"path"`
	Chunks []modelEvidenceChunk `json:"chunks"`
}

type modelEvidenceChunk struct {
	Lines   [2]int  `json:"lines"`
	Columns []int   `json:"columns,omitempty"`
	Content *string `json:"content,omitempty"`
	Seen    string  `json:"seen,omitempty"`
}

type modelResultStats struct {
	Requests    int `json:"requests"`
	Files       int `json:"files"`
	Matches     int `json:"matches"`
	Evidence    int `json:"evidence"`
	NewChars    int `json:"new_chars"`
	ReusedChars int `json:"reused_chars"`
}

type modelLine struct {
	path        string
	line        int
	startColumn int
	endColumn   int
	content     string
	key         string
	seen        bool
}

func marshalModelResult(result Result, view *evidenceView) ([]byte, []evidenceObservation, error) {
	wire, observations := projectModelResult(result, view)
	encoded, err := json.Marshal(wire)
	return encoded, observations, err
}

func projectModelResult(result Result, view *evidenceView) (modelResult, []evidenceObservation) {
	wire := modelResult{
		Requests:    make([]modelRequest, 0, len(result.Requests)),
		HasMoreView: result.HasMoreView, SourceTruncated: result.SourceTruncated,
		OmittedRequestIDs: append([]string(nil), result.OmittedRequestIDs...), Cursor: result.Cursor,
		Stats: modelResultStats{
			Requests: result.Stats.Requests, Files: result.Stats.Files,
			Matches: result.Stats.Matches,
		},
	}
	for _, request := range result.Requests {
		projected := modelRequest{
			ID: request.ID, Kind: request.Kind,
			Files: append([]string(nil), request.Files...),
		}
		for _, requestError := range request.Errors {
			projected.Errors = append(projected.Errors, requestError.Code)
		}
		if request.SourcePartial {
			projected.SourceTruncated = true
			projected.TruncationReason = request.PartialReason
			if projected.TruncationReason == "" {
				projected.TruncationReason = "source_truncated"
			}
		}
		projected.Matches = groupModelMatches(request.Matches)
		wire.Requests = append(wire.Requests, projected)
	}

	lines := collectModelLines(result.Snippets, view)
	wire.Evidence = groupModelEvidence(lines)
	wire.Stats.Evidence = len(lines)
	observations := make([]evidenceObservation, 0, len(lines))
	for _, line := range lines {
		observations = append(observations, evidenceObservation{key: line.key})
		if line.seen {
			wire.Stats.ReusedChars += len(line.content)
		} else {
			wire.Stats.NewChars += len(line.content)
		}
	}
	return wire, observations
}

func groupModelMatches(matches []Match) []modelMatchGroup {
	if len(matches) == 0 {
		return nil
	}
	byPath := make(map[string][]modelMatchItem)
	for _, match := range matches {
		byPath[match.Path] = append(byPath[match.Path], modelMatchItem{Line: match.Line, Text: match.Text})
	}
	paths := make([]string, 0, len(byPath))
	for path := range byPath {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	groups := make([]modelMatchGroup, 0, len(paths))
	for _, path := range paths {
		items := byPath[path]
		sort.SliceStable(items, func(i, j int) bool {
			if items[i].Line == items[j].Line {
				return items[i].Text < items[j].Text
			}
			return items[i].Line < items[j].Line
		})
		groups = append(groups, modelMatchGroup{Path: path, Items: items})
	}
	return groups
}

func collectModelLines(snippets []Snippet, view *evidenceView) []modelLine {
	byKey := make(map[string]modelLine)
	for _, snippet := range snippets {
		if snippet.StartColumn > 0 || snippet.EndColumn > 0 {
			line := newModelLine(snippet.Path, snippet.StartLine, snippet.StartColumn, snippet.EndColumn, snippet.Content, view)
			byKey[line.key] = line
			continue
		}
		for offset, content := range strings.Split(snippet.Content, "\n") {
			line := newModelLine(snippet.Path, snippet.StartLine+offset, 0, 0, strings.TrimSuffix(content, "\r"), view)
			byKey[line.key] = line
		}
	}
	lines := make([]modelLine, 0, len(byKey))
	for _, line := range byKey {
		lines = append(lines, line)
	}
	sort.SliceStable(lines, func(i, j int) bool {
		if lines[i].path != lines[j].path {
			return lines[i].path < lines[j].path
		}
		if lines[i].line != lines[j].line {
			return lines[i].line < lines[j].line
		}
		if lines[i].startColumn != lines[j].startColumn {
			return lines[i].startColumn < lines[j].startColumn
		}
		return lines[i].key < lines[j].key
	})
	return lines
}

func newModelLine(path string, line, startColumn, endColumn int, content string, view *evidenceView) modelLine {
	key := evidenceSpanKey(path, line, startColumn, endColumn, content)
	return modelLine{
		path: path, line: line, startColumn: startColumn, endColumn: endColumn,
		content: content, key: key, seen: view != nil && view.contains(key),
	}
}

func groupModelEvidence(lines []modelLine) []modelEvidenceFile {
	if len(lines) == 0 {
		return nil
	}
	files := make([]modelEvidenceFile, 0)
	for index := 0; index < len(lines); {
		path := lines[index].path
		end := index + 1
		for end < len(lines) && lines[end].path == path {
			end++
		}
		files = append(files, modelEvidenceFile{Path: path, Chunks: groupModelChunks(lines[index:end])})
		index = end
	}
	return files
}

func groupModelChunks(lines []modelLine) []modelEvidenceChunk {
	chunks := make([]modelEvidenceChunk, 0, len(lines))
	for index := 0; index < len(lines); {
		first := lines[index]
		if first.startColumn > 0 || first.endColumn > 0 {
			chunks = append(chunks, modelChunk(lines[index:index+1]))
			index++
			continue
		}
		end := index + 1
		for end < len(lines) && lines[end].startColumn == 0 && lines[end].endColumn == 0 &&
			lines[end].line == lines[end-1].line+1 && lines[end].seen == first.seen {
			end++
		}
		chunks = append(chunks, modelChunk(lines[index:end]))
		index = end
	}
	return chunks
}

func modelChunk(lines []modelLine) modelEvidenceChunk {
	first := lines[0]
	last := lines[len(lines)-1]
	chunk := modelEvidenceChunk{Lines: [2]int{first.line, last.line}}
	if first.startColumn > 0 || first.endColumn > 0 {
		chunk.Columns = []int{first.startColumn, first.endColumn}
	}
	keys := make([]string, 0, len(lines))
	contents := make([]string, 0, len(lines))
	for _, line := range lines {
		keys = append(keys, line.key)
		contents = append(contents, line.content)
	}
	if first.seen {
		chunk.Seen = evidenceReference(first.path, first.line, last.line, first.startColumn, first.endColumn, keys)
		return chunk
	}
	content := strings.Join(contents, "\n")
	chunk.Content = &content
	return chunk
}
