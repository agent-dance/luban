package inspect

import "testing"

func BenchmarkInspectPagination500Matches(b *testing.B) {
	matches := make([]Match, 500)
	for index := range matches {
		matches[index] = Match{
			Path: "pkg/file-" + integerString(index%100) + ".go",
			Line: index + 1,
			Text: "compact matching source line",
		}
	}
	batch := batchResult{
		generation: "benchmark-generation",
		requests: []completedRequest{{result: RequestResult{
			ID: "matches", Kind: KindSearch, Path: ".", Matches: matches,
		}}},
	}
	pageLimits := limits{maxChars: maximumMaxChars, maxFiles: maximumMaxFiles, maxMatches: maximumMaxMatches}
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		state := newPaginationState(batch, pageLimits)
		page, _, _ := state.nextPage(batch.generation)
		if len(page.Requests) != 1 || len(page.Requests[0].Matches) != len(matches) {
			b.Fatalf("pagination lost matches: %#v", page.Stats)
		}
	}
}

func BenchmarkInspectModelEvidenceProjection(b *testing.B) {
	snippets := make([]Snippet, 0, 256)
	for index := 0; index < 256; index++ {
		snippets = append(snippets, Snippet{
			ID: "s_" + integerString(index), Path: "pkg/cache.go",
			StartLine: index*8 + 1, EndLine: index*8 + 8,
			Content: "line one\nline two\nline three\nline four\nline five\nline six\nline seven\nline eight",
		})
	}
	result := Result{
		Requests: []RequestResult{{ID: "cache", Kind: KindRead, Path: "pkg/cache.go"}},
		Snippets: snippets, Stats: ResultStats{Requests: 1, Files: 1, Snippets: len(snippets)},
	}
	view := &evidenceView{seen: make(map[string]struct{})}
	_, observations, err := marshalModelResult(result, view)
	if err != nil {
		b.Fatal(err)
	}
	view.observe(observations)
	b.ReportAllocs()
	b.SetBytes(int64(len(snippets) * 8))
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		encoded, _, encodeErr := marshalModelResult(result, view)
		if encodeErr != nil || len(encoded) == 0 {
			b.Fatalf("warm evidence projection failed: bytes=%d err=%v", len(encoded), encodeErr)
		}
	}
}
