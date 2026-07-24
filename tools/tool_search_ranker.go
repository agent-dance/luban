package tools

import (
	"math"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// BM25 hyper-parameters mirror the TS reference (k1=1.2, b=0.75).
const (
	bm25K1 = 1.2
	bm25B  = 0.75
)

// englishStopWords is a compact stopword list. Trimmed to terms that frequently
// appear in tool descriptions but carry no signal.
var englishStopWords = map[string]struct{}{
	"a": {}, "an": {}, "and": {}, "are": {}, "as": {}, "at": {},
	"be": {}, "by": {}, "for": {}, "from": {}, "has": {}, "have": {},
	"if": {}, "in": {}, "into": {}, "is": {}, "it": {}, "its": {},
	"of": {}, "on": {}, "or": {}, "that": {}, "the": {}, "to": {},
	"was": {}, "were": {}, "will": {}, "with": {}, "this": {},
	"these": {}, "those": {}, "but": {}, "not": {}, "no": {},
}

var bm25TokenPattern = regexp.MustCompile(`[a-z0-9]+`)

// Tokenise lowercases, strips non-alnum characters, and removes stopwords.
// Exported for tests + the embedding reranker.
func TokeniseForBM25(text string) []string {
	if text == "" {
		return nil
	}
	lower := strings.ToLower(text)
	raw := bm25TokenPattern.FindAllString(lower, -1)
	out := make([]string, 0, len(raw))
	for _, tok := range raw {
		if _, stop := englishStopWords[tok]; stop {
			continue
		}
		if len(tok) <= 1 {
			continue
		}
		out = append(out, tok)
	}
	return out
}

// BM25Document is the internal document representation used by the ranker.
type bm25Document struct {
	Name        string
	Tokens      []string
	TokenCounts map[string]int
	Length      int
}

// BM25Ranker scores tool entries against a query using the standard BM25
// formula. Construction is moderately expensive (O(n) tokenisation); call
// Rebuild whenever the underlying index changes.
type BM25Ranker struct {
	mu       sync.RWMutex
	docs     []bm25Document
	docFreq  map[string]int
	avgLen   float64
	idfCache map[string]float64
}

// NewBM25Ranker builds a ranker over the supplied entries.
func NewBM25Ranker(entries []ToolEntry) *BM25Ranker {
	r := &BM25Ranker{}
	r.Rebuild(entries)
	return r
}

// Rebuild replaces the corpus in place.
func (r *BM25Ranker) Rebuild(entries []ToolEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()

	docs := make([]bm25Document, 0, len(entries))
	docFreq := make(map[string]int)
	totalLen := 0

	for _, e := range entries {
		// Concatenate name + description + hint for richer signal.
		text := strings.Join([]string{
			expandIdentifier(e.Name),
			e.Description,
			e.SearchHint,
		}, " ")
		tokens := TokeniseForBM25(text)
		if len(tokens) == 0 {
			tokens = []string{strings.ToLower(e.Name)}
		}
		counts := make(map[string]int, len(tokens))
		for _, t := range tokens {
			counts[t]++
		}
		for term := range counts {
			docFreq[term]++
		}
		totalLen += len(tokens)
		docs = append(docs, bm25Document{
			Name:        e.Name,
			Tokens:      tokens,
			TokenCounts: counts,
			Length:      len(tokens),
		})
	}

	r.docs = docs
	r.docFreq = docFreq
	r.idfCache = make(map[string]float64, len(docFreq))
	if len(docs) == 0 {
		r.avgLen = 0
	} else {
		r.avgLen = float64(totalLen) / float64(len(docs))
	}
}

// expandIdentifier injects spaces around camelCase boundaries and underscores
// so the tokeniser can split "WebFetchTool" -> "web fetch tool".
func expandIdentifier(name string) string {
	name = strings.ReplaceAll(name, "__", " ")
	name = strings.ReplaceAll(name, "_", " ")
	return toolSearchCamelPattern.ReplaceAllString(name, "$1 $2")
}

// idf returns the IDF for a term, caching results.
func (r *BM25Ranker) idf(term string) float64 {
	if v, ok := r.idfCache[term]; ok {
		return v
	}
	n := float64(len(r.docs))
	df := float64(r.docFreq[term])
	v := math.Log(1 + (n-df+0.5)/(df+0.5))
	r.idfCache[term] = v
	return v
}

// ScoredMatch is a (name, score) pair returned by Rank.
type ScoredMatch struct {
	Name    string
	Score   float64
	Snippet string
}

// Rank scores every document and returns the top-K results sorted by descending
// score (ties broken by name for stability).
//
// TS-06: terms prefixed with `+` are treated as required — documents missing
// any required term are excluded entirely, matching the keyword-path
// semantics. Optional terms still contribute to scoring as before. The `+`
// prefix is stripped before scoring so BM25 weights compare against the bare
// token.
func (r *BM25Ranker) Rank(query string, k int) []ScoredMatch {
	r.mu.RLock()
	defer r.mu.RUnlock()

	rawTerms := splitQueryTerms(query)
	required, optional := partitionRequired(rawTerms)
	allForScoring := make([]string, 0, len(required)+len(optional))
	allForScoring = append(allForScoring, required...)
	allForScoring = append(allForScoring, optional...)
	if len(allForScoring) == 0 || len(r.docs) == 0 {
		return nil
	}
	if k <= 0 {
		k = 10
	}

	out := make([]ScoredMatch, 0, len(r.docs))
	for _, doc := range r.docs {
		// TS-06: hard-filter on required terms.
		if len(required) > 0 {
			missing := false
			for _, term := range required {
				if doc.TokenCounts[term] == 0 {
					missing = true
					break
				}
			}
			if missing {
				continue
			}
		}
		score := 0.0
		for _, term := range allForScoring {
			tf := float64(doc.TokenCounts[term])
			if tf == 0 {
				continue
			}
			idf := r.idf(term)
			normLen := float64(doc.Length)
			denom := tf + bm25K1*(1-bm25B+bm25B*normLen/r.avgLen)
			score += idf * (tf * (bm25K1 + 1) / denom)
		}
		if score > 0 {
			out = append(out, ScoredMatch{
				Name:    doc.Name,
				Score:   score,
				Snippet: snippetForMatch(doc, allForScoring),
			})
		}
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].Name < out[j].Name
		}
		return out[i].Score > out[j].Score
	})

	if len(out) > k {
		out = out[:k]
	}
	return out
}

// splitQueryTerms tokenises the query for BM25 ranking, preserving the leading
// `+` marker so partitionRequired can separate required terms.
func splitQueryTerms(query string) []string {
	if query == "" {
		return nil
	}
	out := make([]string, 0)
	for _, raw := range strings.Fields(strings.ToLower(query)) {
		required := strings.HasPrefix(raw, "+")
		body := raw
		if required {
			body = strings.TrimPrefix(body, "+")
		}
		toks := TokeniseForBM25(body)
		for _, tok := range toks {
			if required {
				out = append(out, "+"+tok)
			} else {
				out = append(out, tok)
			}
		}
	}
	return out
}

// partitionRequired splits a marker-prefixed term list into (required,
// optional) slices with the `+` markers stripped.
func partitionRequired(terms []string) (required, optional []string) {
	for _, t := range terms {
		if strings.HasPrefix(t, "+") {
			required = append(required, strings.TrimPrefix(t, "+"))
		} else {
			optional = append(optional, t)
		}
	}
	return required, optional
}

// snippetForMatch builds a short context fragment showing matched terms.
func snippetForMatch(doc bm25Document, terms []string) string {
	if len(doc.Tokens) == 0 {
		return ""
	}
	matched := make(map[string]struct{}, len(terms))
	for _, t := range terms {
		matched[t] = struct{}{}
	}
	matchedInDoc := make([]string, 0, len(matched))
	for _, t := range doc.Tokens {
		if _, ok := matched[t]; ok {
			matchedInDoc = append(matchedInDoc, t)
		}
	}
	if len(matchedInDoc) == 0 {
		return ""
	}
	return strings.Join(matchedInDoc, ", ")
}
