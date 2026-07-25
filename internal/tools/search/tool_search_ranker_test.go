package search

import (
	"fmt"
	"sync"
	"testing"
)

func TestBM25RankerConcurrentDistinctQueries(t *testing.T) {
	const queryCount = 32

	entries := make([]toolEntry, 0, queryCount)
	for i := 0; i < queryCount; i++ {
		entries = append(entries, toolEntry{
			Name:        fmt.Sprintf("Tool%d", i),
			Description: fmt.Sprintf("operation keyword%d", i),
		})
	}
	ranker := newBM25Ranker(entries)

	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < queryCount; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for range 100 {
				matches := ranker.rank(fmt.Sprintf("keyword%d", i), 1)
				if len(matches) != 1 || matches[0].Name != fmt.Sprintf("Tool%d", i) {
					t.Errorf("query %d returned %#v", i, matches)
					return
				}
			}
		}()
	}
	close(start)
	wg.Wait()
}
