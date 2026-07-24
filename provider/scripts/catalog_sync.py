#!/usr/bin/env python3
"""Sync provider/provider_catalog.json.

This script owns the generated remote model catalog payload that is embedded by Go.
The runtime does not depend on generated Go source; it only embeds the JSON data file.
"""

from __future__ import annotations

import json
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
OUT = ROOT / "provider_catalog.json"
CATALOG_VERIFIED_AT = "2026-07-18"
FAMILY_HISTORY_LIMIT = 4

# Effective context windows returned by Codex app-server token-usage events.
# Verified against Codex CLI 0.144.5 model presets on 2026-07-18. Keep API-only models on their
# public API limits instead of inventing a Codex subscription limit for them.
OPENAI_CODEX_CONTEXT_WINDOWS = {
    "gpt-5.6-sol": 353_400,
    "gpt-5.6-terra": 353_400,
    "gpt-5.6-luna": 353_400,
    "gpt-5.5": 258_400,
    "gpt-5.4": 258_400,
    "gpt-5.4-mini": 258_400,
}


REMOTE_MODELS = [
    # Anthropic model IDs, limits, and pricing:
    # https://platform.claude.com/docs/en/about-claude/models/overview.md
    # https://platform.claude.com/docs/en/about-claude/pricing.md
    {"Provider": "anthropic", "ID": "claude-fable-5", "Name": "Claude Fable 5", "ContextWindow": 1000000, "MaxOutput": 128000, "CostPer1MIn": 10.0, "CostPer1MOut": 50.0, "CacheReadPer1M": 1.0, "CacheCreatePer1M": 12.5, "CanReason": True, "CanUseTools": True, "CanSeeImages": True, "CacheControl": True, "APIFormat": "messages", "_family": "fable", "_rank": 1},
    {"Provider": "anthropic", "ID": "claude-opus-4-8", "Name": "Claude Opus 4.8", "ContextWindow": 1000000, "MaxOutput": 128000, "CostPer1MIn": 5.0, "CostPer1MOut": 25.0, "CacheReadPer1M": 0.5, "CacheCreatePer1M": 6.25, "CanReason": True, "CanUseTools": True, "CanSeeImages": True, "CacheControl": True, "APIFormat": "messages", "_family": "opus", "_rank": 5},
    {"Provider": "anthropic", "ID": "claude-opus-4-7", "Name": "Claude Opus 4.7", "ContextWindow": 1000000, "MaxOutput": 128000, "CostPer1MIn": 5.0, "CostPer1MOut": 25.0, "CacheReadPer1M": 0.5, "CacheCreatePer1M": 6.25, "CanReason": True, "CanUseTools": True, "CanSeeImages": True, "CacheControl": True, "APIFormat": "messages", "_family": "opus", "_rank": 4},
    # Sonnet 5 uses introductory $2/$10 pricing through 2026-08-31.
    {"Provider": "anthropic", "ID": "claude-sonnet-5", "Name": "Claude Sonnet 5", "ContextWindow": 1000000, "MaxOutput": 128000, "CostPer1MIn": 2.0, "CostPer1MOut": 10.0, "CacheReadPer1M": 0.2, "CacheCreatePer1M": 2.5, "CanReason": True, "CanUseTools": True, "CanSeeImages": True, "CacheControl": True, "APIFormat": "messages", "IsDefault": True, "_family": "sonnet", "_rank": 5},
    {"Provider": "anthropic", "ID": "claude-sonnet-4-6", "Name": "Claude Sonnet 4.6", "ContextWindow": 1000000, "MaxOutput": 64000, "CostPer1MIn": 3.0, "CostPer1MOut": 15.0, "CacheReadPer1M": 0.3, "CacheCreatePer1M": 3.75, "CanReason": True, "CanUseTools": True, "CanSeeImages": True, "CacheControl": True, "APIFormat": "messages", "_family": "sonnet", "_rank": 4},
    {"Provider": "anthropic", "ID": "claude-haiku-4-5-20251001", "Name": "Claude Haiku 4.5", "ContextWindow": 200000, "MaxOutput": 64000, "CostPer1MIn": 1.0, "CostPer1MOut": 5.0, "CacheReadPer1M": 0.1, "CacheCreatePer1M": 1.25, "CanReason": True, "CanUseTools": True, "CanSeeImages": True, "CacheControl": True, "APIFormat": "messages", "_family": "haiku", "_rank": 3},

    # OpenAI model IDs, limits, and pricing:
    # https://developers.openai.com/api/docs/guides/latest-model.md
    # https://developers.openai.com/api/docs/pricing
    # The dedicated Luna model page reports 1.05M context; the migration guide's
    # 400K statement conflicts with that model reference, so the model page wins.
    {"Provider": "openai", "ID": "gpt-5.6-sol", "Aliases": ["gpt-5.6"], "Name": "GPT-5.6 Sol", "ContextWindow": 1050000, "MaxOutput": 128000, "CostPer1MIn": 5.0, "CostPer1MOut": 30.0, "CacheReadPer1M": 0.5, "CacheCreatePer1M": 6.25, "CanReason": True, "ReasoningEfforts": ["low", "medium", "high", "xhigh", "max"], "CanUseTools": True, "CanSeeImages": True, "APIFormat": "responses", "IsDefault": True, "_family": "gpt-main", "_rank": 5},
    {"Provider": "openai", "ID": "gpt-5.6-terra", "Name": "GPT-5.6 Terra", "ContextWindow": 1050000, "MaxOutput": 128000, "CostPer1MIn": 2.5, "CostPer1MOut": 15.0, "CacheReadPer1M": 0.25, "CacheCreatePer1M": 3.125, "CanReason": True, "ReasoningEfforts": ["low", "medium", "high", "xhigh", "max"], "CanUseTools": True, "CanSeeImages": True, "APIFormat": "responses", "_family": "gpt-mini", "_rank": 4},
    {"Provider": "openai", "ID": "gpt-5.6-luna", "Name": "GPT-5.6 Luna", "ContextWindow": 1050000, "MaxOutput": 128000, "CostPer1MIn": 1.0, "CostPer1MOut": 6.0, "CacheReadPer1M": 0.1, "CacheCreatePer1M": 1.25, "CanReason": True, "ReasoningEfforts": ["low", "medium", "high", "xhigh", "max"], "CanUseTools": True, "CanSeeImages": True, "APIFormat": "responses", "_family": "gpt-nano", "_rank": 2},
    {"Provider": "openai", "ID": "gpt-5.5", "Name": "GPT-5.5", "ContextWindow": 1050000, "MaxOutput": 128000, "CostPer1MIn": 5.0, "CostPer1MOut": 30.0, "CacheReadPer1M": 0.5, "CanReason": True, "ReasoningEfforts": ["low", "medium", "high", "xhigh"], "CanUseTools": True, "CanSeeImages": True, "APIFormat": "responses", "_family": "gpt-main", "_rank": 4},
    {"Provider": "openai", "ID": "gpt-5.4", "Name": "GPT-5.4", "ContextWindow": 1050000, "MaxOutput": 128000, "CostPer1MIn": 2.5, "CostPer1MOut": 15.0, "CacheReadPer1M": 0.25, "CanReason": True, "ReasoningEfforts": ["low", "medium", "high", "xhigh"], "CanUseTools": True, "CanSeeImages": True, "APIFormat": "responses", "_family": "gpt-main", "_rank": 3},
    {"Provider": "openai", "ID": "gpt-5.4-mini", "Name": "GPT-5.4 mini", "ContextWindow": 400000, "MaxOutput": 128000, "CostPer1MIn": 0.75, "CostPer1MOut": 4.5, "CacheReadPer1M": 0.075, "CanReason": True, "ReasoningEfforts": ["low", "medium", "high", "xhigh"], "CanUseTools": True, "CanSeeImages": True, "APIFormat": "responses", "_family": "gpt-mini", "_rank": 3},

    # Gemini model limits and standard paid-tier pricing:
    # https://ai.google.dev/gemini-api/docs/models
    # https://ai.google.dev/gemini-api/docs/pricing
    {"Provider": "gemini", "ID": "gemini-3.5-flash", "Name": "Gemini 3.5 Flash", "ContextWindow": 1048576, "MaxOutput": 65536, "CostPer1MIn": 1.5, "CostPer1MOut": 9.0, "CacheReadPer1M": 0.15, "CanReason": True, "CanUseTools": True, "CanSeeImages": True, "CacheControl": True, "APIFormat": "chat-completions", "IsDefault": True, "_family": "flash", "_rank": 3},
    {"Provider": "gemini", "ID": "gemini-3.1-pro-preview", "Name": "Gemini 3.1 Pro Preview", "ContextWindow": 1048576, "MaxOutput": 65536, "CostPer1MIn": 2.0, "CostPer1MOut": 12.0, "CacheReadPer1M": 0.2, "CanReason": True, "CanUseTools": True, "CanSeeImages": True, "CacheControl": True, "APIFormat": "chat-completions", "_family": "pro", "_rank": 3},
    {"Provider": "gemini", "ID": "gemini-3.1-flash-lite", "Name": "Gemini 3.1 Flash-Lite", "ContextWindow": 1048576, "MaxOutput": 65536, "CostPer1MIn": 0.25, "CostPer1MOut": 1.5, "CacheReadPer1M": 0.025, "CanReason": True, "CanUseTools": True, "CanSeeImages": True, "CacheControl": True, "APIFormat": "chat-completions", "_family": "flash-lite", "_rank": 3},
    {"Provider": "gemini", "ID": "gemini-2.5-pro", "Name": "Gemini 2.5 Pro", "ContextWindow": 1048576, "MaxOutput": 65536, "CostPer1MIn": 1.25, "CostPer1MOut": 10.0, "CacheReadPer1M": 0.125, "CanReason": True, "CanUseTools": True, "CanSeeImages": True, "CacheControl": True, "APIFormat": "chat-completions", "_family": "pro", "_rank": 1},
    {"Provider": "gemini", "ID": "gemini-2.5-flash", "Name": "Gemini 2.5 Flash", "ContextWindow": 1048576, "MaxOutput": 65536, "CostPer1MIn": 0.3, "CostPer1MOut": 2.5, "CacheReadPer1M": 0.03, "CanReason": True, "CanUseTools": True, "CanSeeImages": True, "CacheControl": True, "APIFormat": "chat-completions", "_family": "flash", "_rank": 1},
    {"Provider": "gemini", "ID": "gemini-2.5-flash-lite", "Name": "Gemini 2.5 Flash-Lite", "ContextWindow": 1048576, "MaxOutput": 65536, "CostPer1MIn": 0.1, "CostPer1MOut": 0.4, "CacheReadPer1M": 0.01, "CanReason": True, "CanUseTools": True, "CanSeeImages": True, "CacheControl": True, "APIFormat": "chat-completions", "_family": "flash-lite", "_rank": 1},

    {"Provider": "deepseek", "ID": "deepseek-v4-flash", "Name": "DeepSeek V4 Flash", "ContextWindow": 1048576, "MaxOutput": 393216, "CostCurrency": "CNY", "CostPer1MIn": 1.0, "CostPer1MOut": 2.0, "CacheReadPer1M": 0.02, "CanReason": True, "CanUseTools": True, "CanSeeImages": False, "APIFormat": "chat-completions", "IsDefault": True, "_family": "v4", "_rank": 3},
    {"Provider": "deepseek", "ID": "deepseek-v4-pro", "Name": "DeepSeek V4 Pro", "ContextWindow": 1048576, "MaxOutput": 393216, "CostCurrency": "CNY", "CostPer1MIn": 3.0, "CostPer1MOut": 6.0, "CacheReadPer1M": 0.025, "CanReason": True, "CanUseTools": True, "CanSeeImages": False, "APIFormat": "chat-completions", "_family": "v4", "_rank": 2},

    # Current Groq rows and prices reverified at https://groq.com/pricing and
    # https://console.groq.com/docs/prompt-caching.
    # Newer rows are omitted until their capability flags and output limits can
    # be read from official model cards instead of inferred from model names.
    {"Provider": "groq", "ID": "llama-3.3-70b-versatile", "Name": "Llama 3.3 70B", "ContextWindow": 131072, "MaxOutput": 32768, "CostPer1MIn": 0.59, "CostPer1MOut": 0.79, "CanReason": False, "CanUseTools": True, "CanSeeImages": False, "APIFormat": "chat-completions", "IsDefault": True, "_family": "llama", "_rank": 2},
    {"Provider": "groq", "ID": "llama-3.1-8b-instant", "Name": "Llama 3.1 8B", "ContextWindow": 131072, "MaxOutput": 8192, "CostPer1MIn": 0.05, "CostPer1MOut": 0.08, "CanReason": False, "CanUseTools": True, "CanSeeImages": False, "APIFormat": "chat-completions", "_family": "llama", "_rank": 1},
    {"Provider": "groq", "ID": "openai/gpt-oss-120b", "Name": "GPT-OSS 120B", "ContextWindow": 131072, "MaxOutput": 65536, "CostPer1MIn": 0.15, "CostPer1MOut": 0.6, "CacheReadPer1M": 0.075, "CanReason": True, "CanUseTools": True, "CanSeeImages": False, "CacheControl": True, "APIFormat": "chat-completions", "_family": "gpt-oss", "_rank": 2},
    {"Provider": "groq", "ID": "openai/gpt-oss-20b", "Name": "GPT-OSS 20B", "ContextWindow": 131072, "MaxOutput": 65536, "CostPer1MIn": 0.075, "CostPer1MOut": 0.3, "CacheReadPer1M": 0.0375, "CanReason": True, "CanUseTools": True, "CanSeeImages": False, "CacheControl": True, "APIFormat": "chat-completions", "_family": "gpt-oss", "_rank": 1},

    # xAI model limits and capabilities:
    # https://docs.x.ai/developers/models
    # Pricing stays unknown because the catalog cannot express the >=200K tier.
    {"Provider": "xai", "ID": "grok-4.5", "Name": "Grok 4.5", "ContextWindow": 500000, "MaxOutput": 0, "CanReason": True, "ReasoningEfforts": ["low", "medium", "high", "xhigh"], "CanUseTools": True, "CanSeeImages": True, "APIFormat": "responses", "IsDefault": True, "_family": "grok", "_rank": 1},

    # Mistral model cards and pricing:
    # https://docs.mistral.ai/models/overview
    # https://mistral.ai/pricing/api/
    # https://docs.mistral.ai/studio-api/conversations/advanced/prompt-caching
    {"Provider": "mistral", "ID": "mistral-large-2512", "Aliases": ["mistral-large-latest"], "Name": "Mistral Large 3", "ContextWindow": 256000, "MaxOutput": 0, "CostPer1MIn": 0.5, "CostPer1MOut": 1.5, "CacheReadPer1M": 0.05, "CanReason": False, "CanUseTools": True, "CanSeeImages": True, "CacheControl": True, "APIFormat": "chat-completions", "IsDefault": True, "_family": "large", "_rank": 2},
    {"Provider": "mistral", "ID": "mistral-medium-3-5", "Aliases": ["mistral-medium-3", "mistral-medium-latest"], "Name": "Mistral Medium 3.5", "ContextWindow": 256000, "MaxOutput": 0, "CostPer1MIn": 1.5, "CostPer1MOut": 7.5, "CacheReadPer1M": 0.15, "CanReason": True, "CanUseTools": True, "CanSeeImages": True, "CacheControl": True, "APIFormat": "chat-completions", "_family": "medium", "_rank": 3},
    # Medium 3.1 and Medium 3 are deprecated and scheduled to retire on 2026-08-31.
    {"Provider": "mistral", "ID": "mistral-medium-2508", "Name": "Mistral Medium 3.1", "ContextWindow": 128000, "MaxOutput": 0, "CostPer1MIn": 0.4, "CostPer1MOut": 2.0, "CanReason": False, "CanUseTools": True, "CanSeeImages": False, "APIFormat": "chat-completions", "_family": "medium", "_rank": 2},
    {"Provider": "mistral", "ID": "mistral-medium-2505", "Name": "Mistral Medium 3", "ContextWindow": 128000, "MaxOutput": 0, "CostPer1MIn": 0.4, "CostPer1MOut": 2.0, "CanReason": False, "CanUseTools": True, "CanSeeImages": False, "APIFormat": "chat-completions", "_family": "medium", "_rank": 1},
    {"Provider": "mistral", "ID": "mistral-small-2603", "Name": "Mistral Small 4", "ContextWindow": 256000, "MaxOutput": 0, "CostPer1MIn": 0.15, "CostPer1MOut": 0.6, "CacheReadPer1M": 0.015, "CanReason": True, "CanUseTools": True, "CanSeeImages": True, "CacheControl": True, "APIFormat": "chat-completions", "_family": "small", "_rank": 1},
    {"Provider": "mistral", "ID": "codestral-2508", "Name": "Codestral", "ContextWindow": 128000, "MaxOutput": 0, "CostPer1MIn": 0.3, "CostPer1MOut": 0.9, "CacheReadPer1M": 0.03, "CanReason": False, "CanUseTools": True, "CanSeeImages": False, "CacheControl": True, "APIFormat": "chat-completions", "_family": "codestral", "_rank": 1},

    # Zhipu standard prices below use the <=32K input tier. The current price
    # table is valid through 2026-08-31 and the catalog cannot express tiers.
    # https://docs.bigmodel.cn/cn/guide/start/model-overview
    # https://open.bigmodel.cn/pricing
    {"Provider": "zhipu", "ID": "glm-5.2", "Name": "GLM-5.2", "ContextWindow": 1000000, "MaxOutput": 128000, "CostCurrency": "CNY", "CostPer1MIn": 8.0, "CostPer1MOut": 28.0, "CacheReadPer1M": 2.0, "CanReason": True, "CanUseTools": True, "CanSeeImages": False, "CacheControl": True, "APIFormat": "chat-completions", "IsDefault": True, "_family": "glm-main", "_rank": 4},
    {"Provider": "zhipu", "ID": "glm-5.1", "Name": "GLM-5.1", "ContextWindow": 200000, "MaxOutput": 128000, "CostCurrency": "CNY", "CostPer1MIn": 6.0, "CostPer1MOut": 24.0, "CacheReadPer1M": 1.3, "CanReason": True, "CanUseTools": True, "CanSeeImages": False, "CacheControl": True, "APIFormat": "chat-completions", "_family": "glm-main", "_rank": 3},
    {"Provider": "zhipu", "ID": "glm-5", "Name": "GLM-5", "ContextWindow": 200000, "MaxOutput": 128000, "CostCurrency": "CNY", "CostPer1MIn": 4.0, "CostPer1MOut": 18.0, "CacheReadPer1M": 1.0, "CanReason": True, "CanUseTools": True, "CanSeeImages": False, "CacheControl": True, "APIFormat": "chat-completions", "_family": "glm-main", "_rank": 2},
    {"Provider": "zhipu", "ID": "glm-4.7", "Name": "GLM-4.7", "ContextWindow": 200000, "MaxOutput": 128000, "CanReason": True, "CanUseTools": True, "CanSeeImages": False, "APIFormat": "chat-completions", "_family": "glm-main", "_rank": 1},
    {"Provider": "zhipu", "ID": "glm-5-turbo", "Name": "GLM-5 Turbo", "ContextWindow": 200000, "MaxOutput": 128000, "CostCurrency": "CNY", "CostPer1MIn": 5.0, "CostPer1MOut": 22.0, "CacheReadPer1M": 1.2, "CanReason": True, "CanUseTools": True, "CanSeeImages": False, "CacheControl": True, "APIFormat": "chat-completions", "_family": "glm-turbo", "_rank": 1},
    {"Provider": "zhipu", "ID": "glm-4.7-flashx", "Name": "GLM-4.7 FlashX", "ContextWindow": 200000, "MaxOutput": 128000, "CostCurrency": "CNY", "CostPer1MIn": 0.5, "CostPer1MOut": 3.0, "CacheReadPer1M": 0.1, "CanReason": True, "CanUseTools": True, "CanSeeImages": False, "CacheControl": True, "APIFormat": "chat-completions", "_family": "glm-flashx", "_rank": 1},

    # MiniMax prices use the <=512K input tier; larger M3 prompts cost 2x.
    # https://platform.minimax.io/docs/guides/models-intro
    # https://platform.minimax.io/docs/guides/pricing-paygo
    {"Provider": "minimax", "ID": "MiniMax-M3", "Name": "MiniMax M3", "ContextWindow": 1000000, "MaxOutput": 524288, "CostPer1MIn": 0.3, "CostPer1MOut": 1.2, "CacheReadPer1M": 0.06, "CanReason": True, "CanUseTools": True, "CanSeeImages": True, "CacheControl": True, "APIFormat": "chat-completions", "IsDefault": True, "_family": "m-main", "_rank": 4},
    {"Provider": "minimax", "ID": "MiniMax-M2.7", "Name": "MiniMax M2.7", "ContextWindow": 204800, "MaxOutput": 204800, "CostPer1MIn": 0.3, "CostPer1MOut": 1.2, "CacheReadPer1M": 0.06, "CacheCreatePer1M": 0.375, "CanReason": True, "CanUseTools": True, "CanSeeImages": False, "CacheControl": True, "APIFormat": "chat-completions", "_family": "m-main", "_rank": 3},
    {"Provider": "minimax", "ID": "MiniMax-M2.5", "Name": "MiniMax M2.5", "ContextWindow": 204800, "MaxOutput": 80000, "CanReason": True, "CanUseTools": True, "CanSeeImages": False, "APIFormat": "chat-completions", "_family": "m-main", "_rank": 2},
    {"Provider": "minimax", "ID": "MiniMax-M2.1", "Name": "MiniMax M2.1", "ContextWindow": 204800, "MaxOutput": 80000, "CanReason": True, "CanUseTools": True, "CanSeeImages": False, "APIFormat": "chat-completions", "_family": "m-main", "_rank": 1},
    {"Provider": "minimax", "ID": "MiniMax-M2.7-highspeed", "Name": "MiniMax M2.7 HighSpeed", "ContextWindow": 204800, "MaxOutput": 204800, "CostPer1MIn": 0.6, "CostPer1MOut": 2.4, "CacheReadPer1M": 0.06, "CacheCreatePer1M": 0.375, "CanReason": True, "CanUseTools": True, "CanSeeImages": False, "CacheControl": True, "APIFormat": "chat-completions", "_family": "m-highspeed", "_rank": 3},
    {"Provider": "minimax", "ID": "MiniMax-M2.5-highspeed", "Name": "MiniMax M2.5 HighSpeed", "ContextWindow": 204800, "MaxOutput": 80000, "CanReason": False, "CanUseTools": True, "CanSeeImages": False, "APIFormat": "chat-completions", "_family": "m-highspeed", "_rank": 2},
    {"Provider": "minimax", "ID": "MiniMax-M2.1-highspeed", "Name": "MiniMax M2.1 HighSpeed", "ContextWindow": 204800, "MaxOutput": 80000, "CanReason": False, "CanUseTools": True, "CanSeeImages": False, "APIFormat": "chat-completions", "_family": "m-highspeed", "_rank": 1},

    # Kimi model limits and native CNY pricing:
    # https://platform.kimi.com/docs/models
    # https://platform.kimi.com/docs/pricing/chat-k26
    # https://platform.kimi.com/docs/pricing/chat-k27-code
    {"Provider": "kimi", "ID": "kimi-k3", "Name": "Kimi K3", "ContextWindow": 1000000, "MaxOutput": 0, "CanReason": True, "ReasoningEfforts": ["max"], "CanUseTools": True, "CanSeeImages": True, "CacheControl": True, "APIFormat": "chat-completions", "IsDefault": True, "_family": "kimi-main", "_rank": 4},
    {"Provider": "kimi", "ID": "kimi-k2.6", "Name": "Kimi K2.6", "ContextWindow": 262144, "MaxOutput": 0, "CostCurrency": "CNY", "CostPer1MIn": 6.5, "CostPer1MOut": 27.0, "CacheReadPer1M": 1.1, "CanReason": True, "CanUseTools": True, "CanSeeImages": True, "CacheControl": True, "APIFormat": "chat-completions", "_family": "kimi-main", "_rank": 3},
    {"Provider": "kimi", "ID": "kimi-k2.5", "Name": "Kimi K2.5", "ContextWindow": 262144, "MaxOutput": 0, "CostCurrency": "CNY", "CostPer1MIn": 4.0, "CostPer1MOut": 21.0, "CacheReadPer1M": 0.7, "CanReason": True, "CanUseTools": True, "CanSeeImages": True, "CacheControl": True, "APIFormat": "chat-completions", "_family": "kimi-main", "_rank": 2},
    {"Provider": "kimi", "ID": "kimi-k2.7-code", "Name": "Kimi K2.7 Code", "ContextWindow": 262144, "MaxOutput": 0, "CostCurrency": "CNY", "CostPer1MIn": 6.5, "CostPer1MOut": 27.0, "CacheReadPer1M": 1.3, "CanReason": True, "CanUseTools": True, "CanSeeImages": True, "CacheControl": True, "APIFormat": "chat-completions", "_family": "kimi-code", "_rank": 2},
    {"Provider": "kimi", "ID": "kimi-k2.7-code-highspeed", "Name": "Kimi K2.7 Code HighSpeed", "ContextWindow": 262144, "MaxOutput": 0, "CostCurrency": "CNY", "CostPer1MIn": 13.0, "CostPer1MOut": 54.0, "CacheReadPer1M": 2.6, "CanReason": True, "CanUseTools": True, "CanSeeImages": True, "CacheControl": True, "APIFormat": "chat-completions", "_family": "kimi-code", "_rank": 1},
    {"Provider": "kimi", "ID": "moonshot-v1-128k", "Name": "Moonshot V1 128K", "ContextWindow": 128000, "MaxOutput": 0, "CostCurrency": "CNY", "CostPer1MIn": 10.0, "CostPer1MOut": 30.0, "CanReason": False, "CanUseTools": True, "CanSeeImages": False, "APIFormat": "chat-completions", "_family": "moonshot-v1", "_rank": 1},

    # Partner regional pricing can differ; these rows use the global base prices.
    {"Provider": "bedrock", "ID": "anthropic.claude-fable-5", "Name": "Claude Fable 5 (Bedrock)", "ContextWindow": 1000000, "MaxOutput": 128000, "CostPer1MIn": 10.0, "CostPer1MOut": 50.0, "CacheReadPer1M": 1.0, "CacheCreatePer1M": 12.5, "CanReason": True, "CanUseTools": True, "CanSeeImages": True, "CacheControl": True, "APIFormat": "messages", "_family": "fable", "_rank": 1},
    {"Provider": "bedrock", "ID": "anthropic.claude-opus-4-8", "Name": "Claude Opus 4.8 (Bedrock)", "ContextWindow": 1000000, "MaxOutput": 128000, "CostPer1MIn": 5.0, "CostPer1MOut": 25.0, "CacheReadPer1M": 0.5, "CacheCreatePer1M": 6.25, "CanReason": True, "CanUseTools": True, "CanSeeImages": True, "CacheControl": True, "APIFormat": "messages", "_family": "opus", "_rank": 5},
    {"Provider": "bedrock", "ID": "anthropic.claude-sonnet-5", "Name": "Claude Sonnet 5 (Bedrock)", "ContextWindow": 1000000, "MaxOutput": 128000, "CostPer1MIn": 2.0, "CostPer1MOut": 10.0, "CacheReadPer1M": 0.2, "CacheCreatePer1M": 2.5, "CanReason": True, "CanUseTools": True, "CanSeeImages": True, "CacheControl": True, "APIFormat": "messages", "IsDefault": True, "_family": "sonnet", "_rank": 5},
    {"Provider": "bedrock", "ID": "anthropic.claude-sonnet-4-6", "Name": "Claude Sonnet 4.6 (Bedrock)", "ContextWindow": 1000000, "MaxOutput": 64000, "CostPer1MIn": 3.0, "CostPer1MOut": 15.0, "CacheReadPer1M": 0.3, "CacheCreatePer1M": 3.75, "CanReason": True, "CanUseTools": True, "CanSeeImages": True, "CacheControl": True, "APIFormat": "messages", "_family": "sonnet", "_rank": 4},
    {"Provider": "bedrock", "ID": "anthropic.claude-opus-4-7", "Name": "Claude Opus 4.7 (Bedrock)", "ContextWindow": 1000000, "MaxOutput": 128000, "CostPer1MIn": 5.0, "CostPer1MOut": 25.0, "CacheReadPer1M": 0.5, "CacheCreatePer1M": 6.25, "CanReason": True, "CanUseTools": True, "CanSeeImages": True, "CacheControl": True, "APIFormat": "messages", "_family": "opus", "_rank": 4},
    {"Provider": "bedrock", "ID": "anthropic.claude-haiku-4-5-20251001-v1:0", "Name": "Claude Haiku 4.5 (Bedrock)", "ContextWindow": 200000, "MaxOutput": 64000, "CostPer1MIn": 1.0, "CostPer1MOut": 5.0, "CacheReadPer1M": 0.1, "CacheCreatePer1M": 1.25, "CanReason": True, "CanUseTools": True, "CanSeeImages": True, "CacheControl": True, "APIFormat": "messages", "_family": "haiku", "_rank": 3},

    {"Provider": "vertex", "ID": "claude-fable-5", "Name": "Claude Fable 5 (Vertex)", "ContextWindow": 1000000, "MaxOutput": 128000, "CostPer1MIn": 10.0, "CostPer1MOut": 50.0, "CacheReadPer1M": 1.0, "CacheCreatePer1M": 12.5, "CanReason": True, "CanUseTools": True, "CanSeeImages": True, "CacheControl": True, "APIFormat": "messages", "_family": "fable", "_rank": 1},
    {"Provider": "vertex", "ID": "claude-opus-4-8", "Name": "Claude Opus 4.8 (Vertex)", "ContextWindow": 1000000, "MaxOutput": 128000, "CostPer1MIn": 5.0, "CostPer1MOut": 25.0, "CacheReadPer1M": 0.5, "CacheCreatePer1M": 6.25, "CanReason": True, "CanUseTools": True, "CanSeeImages": True, "CacheControl": True, "APIFormat": "messages", "_family": "opus", "_rank": 5},
    {"Provider": "vertex", "ID": "claude-sonnet-5", "Name": "Claude Sonnet 5 (Vertex)", "ContextWindow": 1000000, "MaxOutput": 128000, "CostPer1MIn": 2.0, "CostPer1MOut": 10.0, "CacheReadPer1M": 0.2, "CacheCreatePer1M": 2.5, "CanReason": True, "CanUseTools": True, "CanSeeImages": True, "CacheControl": True, "APIFormat": "messages", "IsDefault": True, "_family": "sonnet", "_rank": 5},
    {"Provider": "vertex", "ID": "claude-sonnet-4-6", "Name": "Claude Sonnet 4.6 (Vertex)", "ContextWindow": 1000000, "MaxOutput": 64000, "CostPer1MIn": 3.0, "CostPer1MOut": 15.0, "CacheReadPer1M": 0.3, "CacheCreatePer1M": 3.75, "CanReason": True, "CanUseTools": True, "CanSeeImages": True, "CacheControl": True, "APIFormat": "messages", "_family": "sonnet", "_rank": 4},
    {"Provider": "vertex", "ID": "claude-opus-4-7", "Name": "Claude Opus 4.7 (Vertex)", "ContextWindow": 1000000, "MaxOutput": 128000, "CostPer1MIn": 5.0, "CostPer1MOut": 25.0, "CacheReadPer1M": 0.5, "CacheCreatePer1M": 6.25, "CanReason": True, "CanUseTools": True, "CanSeeImages": True, "CacheControl": True, "APIFormat": "messages", "_family": "opus", "_rank": 4},
    {"Provider": "vertex", "ID": "claude-haiku-4-5@20251001", "Name": "Claude Haiku 4.5 (Vertex)", "ContextWindow": 200000, "MaxOutput": 64000, "CostPer1MIn": 1.0, "CostPer1MOut": 5.0, "CacheReadPer1M": 0.1, "CacheCreatePer1M": 1.25, "CanReason": True, "CanUseTools": True, "CanSeeImages": True, "CacheControl": True, "APIFormat": "messages", "_family": "haiku", "_rank": 3},
]


def build_payload() -> list[dict]:
    grouped: dict[tuple[str, str], list[dict]] = {}
    for model in REMOTE_MODELS:
        family = model["_family"]
        grouped.setdefault((model["Provider"], family), []).append(model)

    final: list[dict] = []
    for _, models in grouped.items():
        models.sort(key=lambda item: item["_rank"], reverse=True)
        final.extend(models[:FAMILY_HISTORY_LIMIT])

    final.sort(key=lambda item: (item["Provider"], not item.get("IsDefault", False), item["ID"]))
    stripped = []
    for item in final:
        clean = {k: v for k, v in item.items() if not k.startswith("_")}
        # The OpenAI API can expose larger windows than a ChatGPT-backed Codex
        # session. Prefer the verified Codex window when that model is offered
        # by the subscription; otherwise retain the public API metadata.
        if clean["Provider"] == "openai":
            clean["ContextWindow"] = OPENAI_CODEX_CONTEXT_WINDOWS.get(
                clean["ID"], clean["ContextWindow"]
            )
        stripped.append(clean)
    return stripped


def validate_payload(payload: list[dict]) -> None:
    identifiers: dict[tuple[str, str], str] = {}
    defaults: dict[str, int] = {}
    valid_formats = {"messages", "chat-completions", "responses"}

    for model in payload:
        provider = model["Provider"]
        model_id = model["ID"]
        if model["ContextWindow"] <= 0:
            raise ValueError(f"{provider}/{model_id}: ContextWindow must be positive")
        if model["APIFormat"] not in valid_formats:
            raise ValueError(f"{provider}/{model_id}: invalid APIFormat {model['APIFormat']!r}")
        defaults[provider] = defaults.get(provider, 0) + int(model.get("IsDefault", False))

        for identifier in [model_id, *model.get("Aliases", [])]:
            key = (provider, identifier)
            if owner := identifiers.get(key):
                raise ValueError(f"{provider}/{identifier}: identifier already owned by {owner}")
            identifiers[key] = model_id

    xai_models = [model["ID"] for model in payload if model["Provider"] == "xai"]
    if xai_models != ["grok-4.5"]:
        raise ValueError(
            f"xai: expected only the latest stable model grok-4.5, found {xai_models}"
        )

    for provider, count in defaults.items():
        if count != 1:
            raise ValueError(f"{provider}: expected exactly one default model, found {count}")


def main() -> None:
    payload = build_payload()
    validate_payload(payload)
    OUT.write_text(json.dumps(payload, ensure_ascii=False, indent=2, sort_keys=True) + "\n")
    print(f"wrote {OUT} with {len(payload)} models (verified {CATALOG_VERIFIED_AT})")


if __name__ == "__main__":
    main()
