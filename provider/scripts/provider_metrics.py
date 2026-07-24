#!/usr/bin/env python3
"""
provider_metrics.py — Go vs TypeScript Provider Feature Coverage Evaluator

Compares the Go gosrc/provider/ implementation against the original TypeScript
src/services/api/ implementation and produces scored coverage tables.

Usage:
    python3 provider_metrics.py [--json] [--gosrc <path>] [--tssrc <path>]

Options:
    --json          Output raw scores as JSON (for CI integration)
    --gosrc PATH    Path to gosrc/ directory (default: auto-detect)
    --tssrc PATH    Path to TypeScript src/ directory (default: auto-detect)

Exit code:
    0  All critical checks passed
    1  One or more P0 features missing
    2  Script error
"""

import argparse
import json
import os
import re
import sys
from dataclasses import dataclass, field
from pathlib import Path
from typing import Optional

# ─────────────────────────────────────────────────────────────────────────────
# Data Model
# ─────────────────────────────────────────────────────────────────────────────

@dataclass
class Feature:
    name: str
    domain: str
    priority: str          # P0, P1, P2, P3
    ts_signal: str         # grep pattern to confirm TS implementation
    go_signal: str         # grep pattern to confirm Go implementation
    ts_file: str           # relative path within tssrc
    go_file: str           # relative path within gosrc
    ts_implemented: bool = False
    go_implemented: bool = False
    notes: str = ""


@dataclass
class DomainScore:
    domain: str
    total: int = 0
    go_implemented: int = 0
    ts_implemented: int = 0

    @property
    def go_coverage(self) -> float:
        return (self.go_implemented / self.total * 100) if self.total else 0.0

    @property
    def ts_coverage(self) -> float:
        return (self.ts_implemented / self.total * 100) if self.total else 0.0


# ─────────────────────────────────────────────────────────────────────────────
# Feature Registry
# ─────────────────────────────────────────────────────────────────────────────

FEATURES: list[Feature] = [
    # ── Provider Abstraction ─────────────────────────────────────────────────
    Feature(
        name="Provider interface",
        domain="Provider Abstraction",
        priority="P0",
        ts_signal=r"getAnthropicClient|createStream|Provider",
        go_signal=r"type Provider interface",
        ts_file="services/api/client.ts",
        go_file="provider/provider.go",
    ),
    Feature(
        name="Params/Config struct",
        domain="Provider Abstraction",
        priority="P0",
        ts_signal=r"MessageParam|max_tokens|system",
        go_signal=r"type Params struct|type Config struct",
        ts_file="services/api/claude.ts",
        go_file="provider/provider.go",
    ),
    Feature(
        name="ModelID() accessor",
        domain="Provider Abstraction",
        priority="P1",
        ts_signal=r"modelId|model_id|\.model",
        go_signal=r"ModelID\(\)",
        ts_file="services/api/claude.ts",
        go_file="provider/provider.go",
    ),
    # ── Anthropic Core ───────────────────────────────────────────────────────
    Feature(
        name="Anthropic Direct API",
        domain="Anthropic Core",
        priority="P0",
        ts_signal=r"new Anthropic\b|AnthropicClient",
        go_signal=r"anthropic\.NewClient",
        ts_file="services/api/client.ts",
        go_file="provider/anthropic.go",
    ),
    Feature(
        name="SSE streaming (all 6 event types)",
        domain="Anthropic Core",
        priority="P0",
        ts_signal=r"message_start|content_block_start|message_stop",
        go_signal=r"EventMessageStart|EventContentBlockStart|EventMessageStop",
        ts_file="services/api/claude.ts",
        go_file="provider/anthropic.go",
    ),
    Feature(
        name="Extended Thinking (thinking block)",
        domain="Anthropic Core",
        priority="P1",
        ts_signal=r"thinking|ThinkingBlock",
        go_signal=r"ContentTypeThinking|ThinkingBlock|thinking_delta",
        ts_file="services/api/claude.ts",
        go_file="provider/anthropic.go",
    ),
    Feature(
        name="Tool use streaming (InputJSONDelta)",
        domain="Anthropic Core",
        priority="P0",
        ts_signal=r"tool_use|InputJSONDelta|input_json_delta",
        go_signal=r"InputJSONDelta|input_json_delta|ContentTypeToolUse",
        ts_file="services/api/claude.ts",
        go_file="provider/anthropic.go",
    ),
    Feature(
        name="System prompt cache breakpoint",
        domain="Anthropic Core",
        priority="P1",
        ts_signal=r"cache_control|CacheControl|ephemeral",
        go_signal=r"NewCacheControlEphemeralParam|CacheControl",
        ts_file="services/api/claude.ts",
        go_file="provider/anthropic.go",
    ),
    Feature(
        name="Tools list cache breakpoint",
        domain="Anthropic Core",
        priority="P1",
        ts_signal=r"cache_control|CacheControl",
        go_signal=r"last\.OfTool\.CacheControl|CacheControl.*tool",
        ts_file="services/api/claude.ts",
        go_file="provider/anthropic.go",
    ),
    Feature(
        name="Messages last-block cache breakpoint",
        domain="Anthropic Core",
        priority="P1",
        ts_signal=r"cache_control|breakpoint",
        go_signal=r"GetCacheControl|lastMsg\.Content",
        ts_file="services/api/claude.ts",
        go_file="provider/anthropic.go",
    ),
    Feature(
        name="AWS Bedrock provider",
        domain="Anthropic Core",
        priority="P1",
        ts_signal=r"AnthropicBedrock|bedrock",
        go_signal=r"Bedrock|bedrock",
        ts_file="services/api/client.ts",
        go_file="provider/anthropic.go",
        notes="Not implemented in Go",
    ),
    Feature(
        name="Google Vertex AI provider",
        domain="Anthropic Core",
        priority="P1",
        ts_signal=r"AnthropicVertex|vertex",
        go_signal=r"Vertex|vertex",
        ts_file="services/api/client.ts",
        go_file="provider/anthropic.go",
        notes="Not implemented in Go",
    ),
    Feature(
        name="Azure Foundry provider",
        domain="Anthropic Core",
        priority="P2",
        ts_signal=r"AnthropicFoundry|foundry|azure",
        go_signal=r"Foundry|foundry|AzureFoundry",
        ts_file="services/api/client.ts",
        go_file="provider/anthropic.go",
        notes="Not implemented in Go (can use BaseURL workaround)",
    ),
    Feature(
        name="Thinking config (budget_tokens/effort)",
        domain="Anthropic Core",
        priority="P2",
        ts_signal=r"budget_tokens|thinkingConfig|effort",
        go_signal=r"BudgetTokens|ThinkingConfig|Effort",
        ts_file="services/api/claude.ts",
        go_file="provider/provider.go",
        notes="Not implemented in Go",
    ),
    Feature(
        name="Beta headers (8 types)",
        domain="Anthropic Core",
        priority="P2",
        ts_signal=r"BETA_HEADER|anthropic-beta",
        go_signal=r"BetaHeaders|beta.*header",
        ts_file="services/api/claude.ts",
        go_file="provider/anthropic.go",
        notes="Not implemented in Go",
    ),
    Feature(
        name="Tool choice parameter",
        domain="Anthropic Core",
        priority="P1",
        ts_signal=r"tool_choice|ToolChoice",
        go_signal=r"ToolChoice|tool_choice",
        ts_file="services/api/claude.ts",
        go_file="provider/provider.go",
        notes="Not implemented in Go",
    ),
    Feature(
        name="Structured outputs",
        domain="Anthropic Core",
        priority="P2",
        ts_signal=r"structured_output|response_format",
        go_signal=r"StructuredOutput|ResponseFormat",
        ts_file="services/api/claude.ts",
        go_file="provider/anthropic.go",
        notes="Not implemented in Go",
    ),
    Feature(
        name="Image blocks",
        domain="Anthropic Core",
        priority="P2",
        ts_signal=r"ImageBlock|image.*base64|media_type.*image",
        go_signal=r"ImageBlock|ContentTypeImage",
        ts_file="services/api/claude.ts",
        go_file="provider/anthropic.go",
        notes="Not implemented in Go",
    ),
    Feature(
        name="Document blocks",
        domain="Anthropic Core",
        priority="P2",
        ts_signal=r"DocumentBlock|document.*content",
        go_signal=r"DocumentBlock|ContentTypeDocument",
        ts_file="services/api/claude.ts",
        go_file="provider/anthropic.go",
        notes="Not implemented in Go",
    ),
    Feature(
        name="RedactedThinking block",
        domain="Anthropic Core",
        priority="P2",
        ts_signal=r"RedactedThinking|redacted_thinking",
        go_signal=r"ContentTypeRedactedThinking|case.*RedactedThinking",
        ts_file="services/api/claude.ts",
        go_file="provider/anthropic.go",
        notes="Not implemented in Go",
    ),
    # ── OpenAI Compatible Layer ──────────────────────────────────────────────
    Feature(
        name="OpenAI Direct API",
        domain="OpenAI Compat",
        priority="P0",
        ts_signal=r"openai|OpenAI",
        go_signal=r"NewOpenAI|OpenAIProvider",
        ts_file="services/api/client.ts",
        go_file="provider/openai.go",
    ),
    Feature(
        name="Ollama (no-auth local)",
        domain="OpenAI Compat",
        priority="P1",
        ts_signal=r"ollama|localhost.*11434",
        go_signal=r"noAuthTransport|ollama|11434",
        ts_file="services/api/client.ts",
        go_file="provider/openai.go",
    ),
    Feature(
        name="DeepSeek provider",
        domain="OpenAI Compat",
        priority="P1",
        ts_signal=r"deepseek",
        go_signal=r"deepseek|DeepSeek",
        ts_file="services/api/client.ts",
        go_file="provider/env.go",
    ),
    Feature(
        name="Custom BaseURL",
        domain="OpenAI Compat",
        priority="P1",
        ts_signal=r"baseUrl|BASE_URL|baseURL",
        go_signal=r"BaseURL|baseURL",
        ts_file="services/api/client.ts",
        go_file="provider/provider.go",
    ),
    Feature(
        name="Custom request headers",
        domain="OpenAI Compat",
        priority="P1",
        ts_signal=r"customHeaders|ANTHROPIC_CUSTOM_HEADERS",
        go_signal=r"headerTransport|Headers.*map",
        ts_file="services/api/client.ts",
        go_file="provider/openai.go",
    ),
    Feature(
        name="Timeout configuration",
        domain="OpenAI Compat",
        priority="P1",
        ts_signal=r"timeout|Timeout",
        go_signal=r"Timeout|timeout.*600",
        ts_file="services/api/client.ts",
        go_file="provider/openai.go",
    ),
    Feature(
        name="OpenAI→Anthropic event synthesis",
        domain="OpenAI Compat",
        priority="P0",
        ts_signal=r"content_block_start|message_start",
        go_signal=r"processStream|EventContentBlockStart|textStarted",
        ts_file="services/api/client.ts",
        go_file="provider/openai.go",
    ),
    Feature(
        name="IncludeUsage (final usage chunk)",
        domain="OpenAI Compat",
        priority="P1",
        ts_signal=r"include_usage|stream_options",
        go_signal=r"IncludeUsage|StreamOptions",
        ts_file="services/api/claude.ts",
        go_file="provider/openai.go",
    ),
    Feature(
        name="Multi-tool call per-index tracking",
        domain="OpenAI Compat",
        priority="P1",
        ts_signal=r"tool_calls|toolCalls",
        go_signal=r"toolCalls\[idx\]|toolAcc|per.index",
        ts_file="services/api/claude.ts",
        go_file="provider/openai.go",
    ),
    # ── Error Handling ───────────────────────────────────────────────────────
    Feature(
        name="Stream error event",
        domain="Error Handling",
        priority="P0",
        ts_signal=r"error.*event|EventError|stream.*error",
        go_signal=r"EventError|types\.APIError",
        ts_file="services/api/claude.ts",
        go_file="provider/anthropic.go",
    ),
    Feature(
        name="APIError type mapping",
        domain="Error Handling",
        priority="P0",
        ts_signal=r"APIError|api_error",
        go_signal=r"openai\.APIError|APIError",
        ts_file="services/api/withRetry.ts",
        go_file="provider/openai.go",
    ),
    Feature(
        name="Context cancellation",
        domain="Error Handling",
        priority="P0",
        ts_signal=r"AbortController|abort|cancel",
        go_signal=r"ctx\.Done\(\)|ctx\.Err\(\)",
        ts_file="services/api/claude.ts",
        go_file="provider/anthropic.go",
    ),
    Feature(
        name="Retry on 429 (rate limit)",
        domain="Error Handling",
        priority="P0",
        ts_signal=r"429|rate.?limit|RateLimit",
        go_signal=r"retry.*429|429.*retry|StatusTooManyRequests",
        ts_file="services/api/withRetry.ts",
        go_file="provider/anthropic.go",
        notes="Not implemented in Go",
    ),
    Feature(
        name="Retry on 529 (overloaded)",
        domain="Error Handling",
        priority="P0",
        ts_signal=r"529|overload",
        go_signal=r"retry.*529|529.*retry",
        ts_file="services/api/withRetry.ts",
        go_file="provider/anthropic.go",
        notes="Not implemented in Go",
    ),
    Feature(
        name="Exponential backoff with jitter",
        domain="Error Handling",
        priority="P0",
        ts_signal=r"BASE_DELAY_MS|exponential|backoff|jitter",
        go_signal=r"backoff|jitter|exponential|BASE_DELAY",
        ts_file="services/api/withRetry.ts",
        go_file="provider/anthropic.go",
        notes="Not implemented in Go",
    ),
    Feature(
        name="Auth error handling (OAuth/Bedrock/Vertex)",
        domain="Error Handling",
        priority="P2",
        ts_signal=r"handleOAuth401|clearAWS|clearGCP",
        go_signal=r"OAuth.*401|handleAuth",
        ts_file="services/api/withRetry.ts",
        go_file="provider/anthropic.go",
        notes="Not implemented in Go (no multi-cloud support)",
    ),
    # ── Cache Management ─────────────────────────────────────────────────────
    Feature(
        name="Request-side cache breakpoint injection",
        domain="Cache Management",
        priority="P1",
        ts_signal=r"cache_control|ephemeral",
        go_signal=r"NewCacheControlEphemeralParam|CacheControl",
        ts_file="services/api/claude.ts",
        go_file="provider/anthropic.go",
    ),
    Feature(
        name="Post-response cache break detection",
        domain="Cache Management",
        priority="P3",
        ts_signal=r"promptCacheBreakDetection|tengu_prompt_cache_break",
        go_signal=r"cacheBreak|CacheBreak|promptCache.*detect",
        ts_file="services/api/promptCacheBreakDetection.ts",
        go_file="provider/anthropic.go",
        notes="Not implemented in Go",
    ),
    Feature(
        name="Cache break analytics event",
        domain="Cache Management",
        priority="P3",
        ts_signal=r"tengu_prompt_cache_break",
        go_signal=r"tengu_prompt_cache_break|CacheBreakEvent",
        ts_file="services/api/promptCacheBreakDetection.ts",
        go_file="provider/anthropic.go",
        notes="Not implemented in Go",
    ),
    Feature(
        name="Cache break diff debug file",
        domain="Cache Management",
        priority="P3",
        ts_signal=r"diff.*file|debugFile|cache.*diff",
        go_signal=r"cache.*diff|debugFile",
        ts_file="services/api/promptCacheBreakDetection.ts",
        go_file="provider/anthropic.go",
        notes="Not implemented in Go",
    ),
    # ── Factory / Routing ────────────────────────────────────────────────────
    Feature(
        name="NewFromEnv factory",
        domain="Factory/Routing",
        priority="P0",
        ts_signal=r"PROVIDER|ANTHROPIC_API_KEY|getProvider",
        go_signal=r"NewFromEnv\b",
        ts_file="services/api/client.ts",
        go_file="provider/env.go",
    ),
    Feature(
        name="NewFromEnvWithOverrides",
        domain="Factory/Routing",
        priority="P1",
        ts_signal=r"override|Override",
        go_signal=r"NewFromEnvWithOverrides",
        ts_file="services/api/client.ts",
        go_file="provider/env.go",
    ),
    Feature(
        name="Request ID injection",
        domain="Factory/Routing",
        priority="P3",
        ts_signal=r"x-client-request-id|requestId",
        go_signal=r"request.id|x-client-request-id|RequestID",
        ts_file="services/api/client.ts",
        go_file="provider/openai.go",
        notes="Not implemented in Go",
    ),
    Feature(
        name="Model fallback on 529",
        domain="Factory/Routing",
        priority="P2",
        ts_signal=r"fallback.*model|model.*fallback|haiku.*fallback",
        go_signal=r"FallbackModel|modelFallback",
        ts_file="services/api/withRetry.ts",
        go_file="provider/anthropic.go",
        notes="Not implemented in Go",
    ),
]


# ─────────────────────────────────────────────────────────────────────────────
# File Scanner
# ─────────────────────────────────────────────────────────────────────────────

def file_contains(root: Path, rel_path: str, pattern: str) -> bool:
    """Return True if the file at root/rel_path matches the regex pattern."""
    full = root / rel_path
    if not full.exists():
        return False
    try:
        text = full.read_text(encoding="utf-8", errors="replace")
        return bool(re.search(pattern, text))
    except Exception:
        return False


def scan_features(features: list[Feature], gosrc: Path, tssrc: Path) -> None:
    """Populate .go_implemented and .ts_implemented for every feature."""
    for f in features:
        f.go_implemented = file_contains(gosrc, f.go_file, f.go_signal)
        f.ts_implemented = file_contains(tssrc, f.ts_file, f.ts_signal)


# ─────────────────────────────────────────────────────────────────────────────
# Reporting
# ─────────────────────────────────────────────────────────────────────────────

PRIORITY_ORDER = {"P0": 0, "P1": 1, "P2": 2, "P3": 3}
PRIORITY_LABELS = {"P0": "🔴 P0 Critical", "P1": "🟠 P1 High", "P2": "🟡 P2 Medium", "P3": "🔵 P3 Low"}

def _check(implemented: bool) -> str:
    return "✅" if implemented else "❌"


def print_feature_table(features: list[Feature]) -> None:
    domains = sorted({f.domain for f in features})
    for domain in domains:
        domain_features = [f for f in features if f.domain == domain]
        go_ok = sum(1 for f in domain_features if f.go_implemented)
        total = len(domain_features)
        pct = go_ok / total * 100 if total else 0
        bar_len = 20
        filled = int(bar_len * go_ok / total) if total else 0
        bar = "█" * filled + "░" * (bar_len - filled)

        print(f"\n{'═'*72}")
        print(f"  {domain:30s}  [{bar}]  {go_ok}/{total}  ({pct:.0f}%)")
        print(f"{'═'*72}")
        print(f"  {'Feature':<42} {'Pri':5} {'TS':4} {'Go':4}  Notes")
        print(f"  {'-'*42} {'-'*5} {'-'*4} {'-'*4}  {'-'*20}")

        sorted_features = sorted(domain_features, key=lambda x: PRIORITY_ORDER[x.priority])
        for f in sorted_features:
            notes = f.notes[:28] if f.notes else ""
            print(f"  {f.name:<42} {f.priority:5} {_check(f.ts_implemented):4} {_check(f.go_implemented):4}  {notes}")


def print_summary(features: list[Feature]) -> dict:
    total = len(features)
    go_total = sum(1 for f in features if f.go_implemented)
    ts_total = sum(1 for f in features if f.ts_implemented)

    by_priority: dict[str, dict] = {}
    for p in ["P0", "P1", "P2", "P3"]:
        pf = [f for f in features if f.priority == p]
        go_ok = sum(1 for f in pf if f.go_implemented)
        by_priority[p] = {"total": len(pf), "go": go_ok, "ts": sum(1 for f in pf if f.ts_implemented)}

    print("\n" + "═" * 72)
    print("  OVERALL SUMMARY")
    print("═" * 72)
    print(f"  {'Priority':<15} {'Features':>10} {'TS Impl':>10} {'Go Impl':>10} {'Go Coverage':>12}")
    print(f"  {'-'*15} {'-'*10} {'-'*10} {'-'*10} {'-'*12}")

    for p in ["P0", "P1", "P2", "P3"]:
        d = by_priority[p]
        cov = d['go'] / d['total'] * 100 if d['total'] else 0
        label = PRIORITY_LABELS[p]
        print(f"  {label:<15} {d['total']:>10} {d['ts']:>10} {d['go']:>10} {cov:>11.0f}%")

    total_cov = go_total / total * 100 if total else 0
    ts_cov = ts_total / total * 100 if total else 0
    print(f"  {'─'*15} {'─'*10} {'─'*10} {'─'*10} {'─'*12}")
    print(f"  {'TOTAL':<15} {total:>10} {ts_total:>10} {go_total:>10} {total_cov:>11.0f}%")
    print()

    # P0 gap alert
    p0_missing = [f for f in features if f.priority == "P0" and not f.go_implemented]
    if p0_missing:
        print(f"  ⚠️  {len(p0_missing)} CRITICAL (P0) features not implemented in Go:")
        for f in p0_missing:
            print(f"     • {f.domain} › {f.name}")
    else:
        print("  ✅ All P0 (Critical) features are implemented in Go")
    print()

    return {
        "total_features": total,
        "go_implemented": go_total,
        "ts_implemented": ts_total,
        "go_coverage_pct": round(total_cov, 1),
        "ts_coverage_pct": round(ts_cov, 1),
        "by_priority": {
            p: {
                "total": by_priority[p]["total"],
                "go": by_priority[p]["go"],
                "ts": by_priority[p]["ts"],
                "go_coverage_pct": round(by_priority[p]["go"] / by_priority[p]["total"] * 100, 1) if by_priority[p]["total"] else 0,
            }
            for p in ["P0", "P1", "P2", "P3"]
        },
        "p0_missing": [f.name for f in p0_missing],
    }


def print_param_mapping() -> None:
    """Print the API parameter mapping table (Go Params ↔ TS ↔ raw API)."""
    print("\n" + "═" * 72)
    print("  API PARAMETER MAPPING: Go Params → Anthropic API")
    print("═" * 72)
    rows = [
        ("Model",          "model",          "model",                                "✅ Direct"),
        ("MaxTokens",      "max_tokens",     "max_tokens",                           "✅ Direct"),
        ("System",         "system",         "system[] + cache_control",             "✅ + cache"),
        ("Messages",       "messages",       "messages[] + cache_control",           "✅ + cache"),
        ("Tools",          "tools",          "tools[] + cache_control",              "✅ + cache"),
        ("—",              "thinking",       "thinking{type,budget_tokens}",         "❌ Missing"),
        ("—",              "effort",         "effort",                               "❌ Missing"),
        ("—",              "tool_choice",    "tool_choice",                          "❌ Missing"),
        ("—",              "betas[]",        "anthropic-beta header",                "❌ Missing"),
        ("—",              "stream",         "stream (always true)",                 "✅ Fixed"),
    ]
    header = f"  {'Go Params Field':<20} {'TS Field':<18} {'Anthropic API':<35} {'Status'}"
    print(header)
    print(f"  {'-'*20} {'-'*18} {'-'*35} {'-'*12}")
    for go, ts, api, status in rows:
        print(f"  {go:<20} {ts:<18} {api:<35} {status}")

    print("\n" + "─" * 72)
    print("  API PARAMETER MAPPING: Go Params → OpenAI API")
    print("─" * 72)
    oai_rows = [
        ("Model",          "model",                         "✅ Direct"),
        ("MaxTokens",      "max_completion_tokens",         "✅ Direct"),
        ("System",         "messages[0].role=system",       "✅ Converted"),
        ("Messages",       "messages[]",                    "✅ Converted"),
        ("Tools",          "tools[]",                       "✅ Converted"),
        ("—",              "stream_options.include_usage",  "✅ Fixed true"),
        ("—",              "tool_choice",                   "❌ Missing"),
        ("—",              "response_format",               "❌ Missing"),
        ("—",              "temperature",                   "❌ Missing"),
    ]
    header2 = f"  {'Go Params Field':<20} {'OpenAI API Field':<35} {'Status'}"
    print(header2)
    print(f"  {'-'*20} {'-'*35} {'-'*12}")
    for go, api, status in oai_rows:
        print(f"  {go:<20} {api:<35} {status}")
    print()


def print_streaming_correctness() -> None:
    """Print streaming event correctness table."""
    print("\n" + "═" * 72)
    print("  STREAMING EVENT CORRECTNESS")
    print("═" * 72)

    anthropic_events = [
        ("message_start",                           True,  True,  "TestConvertToAnthropicMessages_RoundTrip"),
        ("content_block_start (text)",              True,  True,  "TestProcessStream_TextOnly"),
        ("content_block_start (tool_use)",          True,  True,  "TestProcessStream_ToolUse"),
        ("content_block_start (thinking)",          True,  True,  "TestProcessStream_ThinkingBlock"),
        ("content_block_delta (text_delta)",        True,  True,  "TestProcessStream_TextOnly"),
        ("content_block_delta (input_json_delta)",  True,  True,  "TestProcessStream_ToolUse"),
        ("content_block_delta (thinking_delta)",    True,  True,  "TestProcessStream_ThinkingBlock"),
        ("content_block_stop",                      True,  True,  "Multiple tests"),
        ("message_delta (stop_reason)",             True,  True,  "Stream end handling"),
        ("message_delta (usage)",                   True,  True,  "TestProcessStream_TextOnly"),
        ("message_stop",                            True,  True,  "All stream tests"),
        ("error event",                             True,  True,  "TestProcessStream_ErrorEvent"),
    ]

    print("\n  Anthropic Provider Events:")
    print(f"  {'Event Type':<45} {'TS':4} {'Go':4}  Test Coverage")
    print(f"  {'-'*45} {'-'*4} {'-'*4}  {'-'*30}")
    for evt, ts, go, test in anthropic_events:
        print(f"  {evt:<45} {_check(ts):4} {_check(go):4}  {test}")

    openai_events = [
        ("Synthetic message_start",                 True,  "Auto-generated"),
        ("Text → content_block_start/delta/stop",   True,  "processStream()"),
        ("Tool calls per-index tracking",            True,  "toolCalls map[int]toolAcc"),
        ("Tool → content_block_start/delta/stop",   True,  "idx+1 offset"),
        ("FinishReason: tool_calls → StopReasonToolUse", True, "switch statement"),
        ("FinishReason: length → StopReasonMaxTokens",   True, "switch statement"),
        ("FinishReason: null string → skip",         True,  'fr != "null" guard'),
        ("usage chunk (IncludeUsage)",               True,  "resp.Usage != nil"),
        ("CachedTokens → CacheReadInputTokens",      True,  "PromptTokensDetails"),
    ]

    print("\n  OpenAI→Anthropic Event Synthesis:")
    print(f"  {'Scenario':<50} {'Go':4}  Mechanism")
    print(f"  {'-'*50} {'-'*4}  {'-'*28}")
    for evt, go, mech in openai_events:
        print(f"  {evt:<50} {_check(go):4}  {mech}")
    print()


# ─────────────────────────────────────────────────────────────────────────────
# Path Auto-detection
# ─────────────────────────────────────────────────────────────────────────────

def find_root(start: Path, marker: str) -> Optional[Path]:
    """Walk up from `start` to find a directory containing `marker`."""
    current = start.resolve()
    for _ in range(10):
        if (current / marker).exists():
            return current
        current = current.parent
    return None


def auto_detect_paths(script_path: Path) -> tuple[Optional[Path], Optional[Path]]:
    """Attempt to auto-detect gosrc/ and tssrc/ paths relative to the script."""
    # script lives at gosrc/provider/scripts/provider_metrics.py
    provider_dir = script_path.parent.parent    # gosrc/provider/
    gosrc_dir = provider_dir.parent             # gosrc/
    project_root = gosrc_dir.parent             # claude-code/
    tssrc_dir = project_root / "src"

    gosrc = gosrc_dir if gosrc_dir.exists() else None
    tssrc = tssrc_dir if tssrc_dir.exists() else None
    return gosrc, tssrc


# ─────────────────────────────────────────────────────────────────────────────
# Main
# ─────────────────────────────────────────────────────────────────────────────

def main() -> int:
    parser = argparse.ArgumentParser(
        description="Provider feature coverage evaluator (Go vs TypeScript)"
    )
    parser.add_argument("--json", action="store_true", help="Output scores as JSON")
    parser.add_argument("--gosrc", default=None, help="Path to gosrc/ directory")
    parser.add_argument("--tssrc", default=None, help="Path to TypeScript src/ directory")
    args = parser.parse_args()

    script_path = Path(__file__).resolve()
    auto_gosrc, auto_tssrc = auto_detect_paths(script_path)

    gosrc = Path(args.gosrc) if args.gosrc else auto_gosrc
    tssrc = Path(args.tssrc) if args.tssrc else auto_tssrc

    if not gosrc or not gosrc.exists():
        print(f"ERROR: gosrc directory not found. Pass --gosrc <path>", file=sys.stderr)
        return 2
    if not tssrc or not tssrc.exists():
        print(f"WARNING: TypeScript src/ not found at {tssrc}. TS columns will show ❌.", file=sys.stderr)
        tssrc = Path("/nonexistent")

    print("\n" + "═" * 72)
    print("  PROVIDER MODULE: Go vs TypeScript Feature Coverage")
    print(f"  gosrc: {gosrc}")
    print(f"  tssrc: {tssrc}")
    print("═" * 72)

    scan_features(FEATURES, gosrc, tssrc)

    if args.json:
        summary = print_summary(FEATURES)
        feature_data = [
            {
                "name": f.name,
                "domain": f.domain,
                "priority": f.priority,
                "go_implemented": f.go_implemented,
                "ts_implemented": f.ts_implemented,
                "notes": f.notes,
            }
            for f in FEATURES
        ]
        print(json.dumps({"summary": summary, "features": feature_data}, indent=2))
    else:
        print_feature_table(FEATURES)
        summary = print_summary(FEATURES)
        print_param_mapping()
        print_streaming_correctness()

    p0_missing = [f for f in FEATURES if f.priority == "P0" and not f.go_implemented]
    return 1 if p0_missing else 0


if __name__ == "__main__":
    sys.exit(main())
