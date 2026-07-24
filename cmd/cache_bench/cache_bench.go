// cache_bench sends multiple rounds through the real QueryLoop to measure
// actual prompt cache hit rates against a live OpenAI-compatible backend.
//
// Usage:
//
//	PROVIDER=openai-responses \
//	OPENAI_API_KEY="sk-..." OPENAI_BASE_URL="http://..." OPENAI_MODEL="gpt-5.4" \
//	  go run cmd/cache_bench/cache_bench.go
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/agent-dance/luban/i18n"
	"github.com/agent-dance/luban/loop"
	"github.com/agent-dance/luban/provider"
	"github.com/agent-dance/luban/registry"
	"github.com/agent-dance/luban/types"
)

func main() {
	lang := i18n.DetectOrLoadLanguage()
	provName := os.Getenv("PROVIDER")
	if provName == "" {
		provName = "openai-responses"
	}

	p, err := provider.NewFromEnvWithOverrides(provName, "")
	if err != nil {
		fmt.Fprintln(os.Stderr, i18n.Format(lang, i18n.KeyCacheBenchProviderInitFailed, err))
		os.Exit(1)
	}
	fmt.Println(i18n.Format(lang, i18n.KeyCacheBenchProvider, p.Name(), p.ModelID()))

	reg := registry.New()

	ql := loop.New(p, reg, loop.Config{
		MaxTurns:  1, // one turn per Run() so we control rounds
		System:    "You are a distributed systems expert. Always give detailed technical answers with concrete examples. Cover trade-offs and alternatives.",
		SessionID: fmt.Sprintf("cache-bench-%d", time.Now().Unix()),
	})

	questions := []string{
		"Design a high-availability event-driven order processing system with Kafka. Cover exactly-once semantics, multi-region failover, and CQRS with PostgreSQL.",
		"Deep dive into the transactional outbox pattern. How does Debezium CDC work with PostgreSQL WAL?",
		"Explain the multi-region failover procedure step by step. How to prevent split-brain?",
		"Design the observability stack: distributed tracing, metrics, and SLO-based alerting.",
		"Top 5 failure modes and how each gets auto-detected and recovered?",
		"Design CI/CD with ArgoCD, canary deployments, and automated rollback.",
	}

	fmt.Println("\n" + i18n.Text(lang, i18n.KeyCacheBenchHeader))
	fmt.Println(strings.Repeat("-", 65))

	for i, q := range questions {
		var lastUsage *types.Usage

		err := ql.Run(context.Background(), q, func(evt loop.Event) {
			switch evt.Type {
			case loop.EventTurnEnd:
				if evt.Usage != nil {
					lastUsage = evt.Usage
				}
			case loop.EventError:
				fmt.Fprintf(os.Stderr, "  [%s]\n", evt.Text)
			}
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, i18n.Format(lang, i18n.KeyCacheBenchRoundFailed, i+1, err))
			continue
		}

		if lastUsage != nil {
			u := lastUsage
			uncached := u.UncachedInputTokens()
			hitPct := 0.0
			if u.InputTokens > 0 {
				hitPct = float64(u.CacheReadInputTokens) / float64(u.InputTokens) * 100
			}
			fmt.Printf("%-7d %10d %10d %10d %10d %7.1f%%\n",
				i+1, u.InputTokens, u.CacheReadInputTokens, u.CacheCreationInputTokens, uncached, hitPct)
		} else {
			fmt.Printf("%-7d %10s\n", i+1, i18n.Text(lang, i18n.KeyCacheBenchNoUsage))
		}
	}

	fmt.Println(strings.Repeat("-", 65))
}
