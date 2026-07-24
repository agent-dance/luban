#!/usr/bin/env python3
"""
extension_metrics.py — Go vs TypeScript Extension System Coverage Evaluator

Evaluates the coverage of the Go replication of Claude Code's extension system
(MCP, Skills, Registry, Commands) against the original TypeScript implementation.

Usage:
    python3 extension_metrics.py [--dir <gosrc_root>] [--json] [--verbose]

Re-runnable: scans source files dynamically on each invocation.
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
# Reference data: TypeScript ground-truth counts/features
# ─────────────────────────────────────────────────────────────────────────────

TS_MCP = {
    "transports": ["stdio", "sse", "streamable_http", "websocket"],
    "rpc_methods": [
        "initialize", "initialized", "tools/list", "tools/call",
        "resources/list", "resources/read", "prompts/list", "prompts/get",
        "elicit/request", "sampling/createMessage",
    ],
    "config_types": [
        "McpStdioServerConfig", "McpSSEServerConfig",
        "McpHTTPServerConfig", "McpWebSocketServerConfig",
    ],
    "config_scopes": ["global", "project", "policy"],
    "advanced_features": ["oauth", "elicitation", "progress", "image_resize",
                          "batch_connect", "analytics", "plugin_mcp"],
}

TS_SKILLS = {
    "source_types": ["bundled", "userSettings", "projectSettings",
                     "policySettings", "plugin", "mcp"],
    "frontmatter_fields": [
        "description", "allowed-tools", "argument-hint", "arguments",
        "when_to_use", "version", "model", "disable-model-invocation",
        "user-invocable", "hooks", "context", "agent", "effort",
        "paths", "shell",
    ],
    "advanced_features": [
        "conditional_paths", "dynamic_discovery", "shell_exec",
        "env_var_substitution", "named_args", "symlink_dedup",
        "memoized_cache", "dynamic_reload_callback",
        "skills_lock_json",
    ],
}

# Unconditional + feature-flagged TS commands
TS_COMMANDS_UNCONDITIONAL = [
    "add-dir", "autofix-pr", "btw", "clear", "color", "commit", "compact",
    "config", "context", "cost", "diff", "doctor", "memory", "help", "ide",
    "init", "keybindings", "login", "logout", "mcp", "mobile", "onboarding",
    "pr_comments", "release-notes", "rename", "resume", "review", "session",
    "share", "skills", "status", "tasks", "teleport", "security-review",
    "bughunter", "terminalSetup", "usage", "theme", "vim",
]
TS_COMMANDS_FLAGGED = [
    "proactive", "brief", "assistant", "bridge", "remoteControlServer",
    "voice", "forceSnip", "workflows", "web", "subscribePr", "ultraplan",
    "torch", "peers", "fork", "buddy",
]
TS_COMMANDS_ALL = TS_COMMANDS_UNCONDITIONAL + TS_COMMANDS_FLAGGED

TS_REGISTRY = {
    "estimated_tools": 60,
    "features": ["permissions", "thread_safe", "clone", "dynamic_register",
                 "audit_log"],
}

# ─────────────────────────────────────────────────────────────────────────────
# Go source analysis helpers
# ─────────────────────────────────────────────────────────────────────────────

def read_file(path: Path) -> str:
    try:
        return path.read_text(encoding="utf-8")
    except Exception:
        return ""


def find_go_files(directory: Path, subdir: str) -> list:
    target = directory / subdir
    if not target.exists():
        return []
    return sorted(target.rglob("*.go"))


def grep(pattern: str, text: str, flags: int = re.IGNORECASE) -> list:
    return re.findall(pattern, text, flags)


def contains(pattern: str, text: str) -> bool:
    return bool(re.search(pattern, text, re.IGNORECASE))


# ─────────────────────────────────────────────────────────────────────────────
# MCP analysis
# ─────────────────────────────────────────────────────────────────────────────

@dataclass
class MCPMetrics:
    transports_found: list = field(default_factory=list)
    rpc_methods_found: list = field(default_factory=list)
    config_types_found: list = field(default_factory=list)
    config_scopes_found: list = field(default_factory=list)
    advanced_features_found: list = field(default_factory=list)
    tool_name_prefix: bool = False
    cursor_pagination: bool = False
    graceful_shutdown: bool = False
    protocol_version: Optional[str] = None


def analyze_mcp(gosrc: Path) -> MCPMetrics:
    m = MCPMetrics()
    files = find_go_files(gosrc, "mcp")
    combined = "\n".join(read_file(f) for f in files)

    # Transports
    if contains(r"stdin|stdout|exec\.Command|StdioTransport|newline", combined):
        m.transports_found.append("stdio")
    if contains(r"sse|server.sent|EventSource", combined):
        m.transports_found.append("sse")
    if contains(r"StreamableHTTP|streamable.http", combined):
        m.transports_found.append("streamable_http")
    if contains(r"websocket|WebSocket|gorilla/websocket|nhooyr\.io/websocket", combined):
        m.transports_found.append("websocket")

    # RPC methods
    rpc_patterns = {
        "initialize":             r'"initialize"',
        "initialized":            r'"initialized"',
        "tools/list":             r'"tools/list"',
        "tools/call":             r'"tools/call"',
        "resources/list":         r'"resources/list"',
        "resources/read":         r'"resources/read"',
        "prompts/list":           r'"prompts/list"',
        "prompts/get":            r'"prompts/get"',
        "elicit/request":         r'"elicit/request"|elicit',
        "sampling/createMessage": r'"sampling/createMessage"|createMessage',
    }
    for method, pat in rpc_patterns.items():
        if contains(pat, combined):
            m.rpc_methods_found.append(method)

    # Config types
    if contains(r"ServerConfig|StdioConfig", combined):
        m.config_types_found.append("McpStdioServerConfig")
    for t in ["SSE", "HTTP", "WebSocket"]:
        if contains(t, combined):
            m.config_types_found.append(f"Mcp{t}ServerConfig")

    # Config scopes
    for scope in ["global", "project", "policy"]:
        if contains(scope, combined):
            m.config_scopes_found.append(scope)

    # Advanced features
    feat_patterns = {
        "oauth":         r"oauth|OAuth|token.*refresh|refresh.*token",
        "elicitation":   r"elicit",
        "progress":      r"progress|Progress",
        "image_resize":  r"resize|image.*resize|ImgResize",
        "batch_connect": r"batch.*connect|connection.*batch",
        "analytics":     r"logEvent|analytics|telemetry",
        "plugin_mcp":    r"plugin.*mcp|mcp.*plugin",
    }
    for feat, pat in feat_patterns.items():
        if contains(pat, combined):
            m.advanced_features_found.append(feat)

    # Quality indicators
    m.tool_name_prefix = contains(r'mcp_|"mcp_%s"', combined)
    m.cursor_pagination = contains(r"[Cc]ursor|nextCursor|NextCursor", combined)
    m.graceful_shutdown = contains(r"SIGTERM|Kill|5.*second|timeout.*kill", combined)
    ver = re.search(r'"(20\d\d-\d\d-\d\d)"', combined)
    m.protocol_version = ver.group(1) if ver else None

    return m


# ─────────────────────────────────────────────────────────────────────────────
# Skills analysis
# ─────────────────────────────────────────────────────────────────────────────

@dataclass
class SkillsMetrics:
    file_format_loading: bool = False
    dir_format_loading: bool = False
    trigger_detection: bool = False
    case_insensitive_regex: bool = False
    skill_tool_adapter: bool = False
    placeholder_substitution: bool = False
    frontmatter_parsing: bool = False
    frontmatter_fields_found: list = field(default_factory=list)
    source_types_found: list = field(default_factory=list)
    advanced_features_found: list = field(default_factory=list)
    skills_lock_loaded: bool = False


def analyze_skills(gosrc: Path) -> SkillsMetrics:
    m = SkillsMetrics()
    files = find_go_files(gosrc, "skills")
    combined = "\n".join(read_file(f) for f in files)

    # Core loading
    m.file_format_loading    = contains(r'\.md|\.MD|filepath\.Ext', combined)
    m.dir_format_loading     = contains(r'SKILL\.md|skill\.md', combined)
    m.trigger_detection      = contains(r'Trigger|trigger|DetectTrigger', combined)
    m.case_insensitive_regex = contains(r'\(\?i\)', combined) or "(?i)" in combined
    m.skill_tool_adapter     = contains(r'SkillTool|skillTool', combined)
    m.placeholder_substitution = contains(r'\{\{ARGS\}\}|\{\{PROMPT\}\}|ReplaceAll', combined)

    # Frontmatter
    m.frontmatter_parsing = contains(r'yaml|frontmatter|---\n', combined)
    for fname in TS_SKILLS["frontmatter_fields"]:
        if contains(re.escape(fname), combined):
            m.frontmatter_fields_found.append(fname)

    # Source types
    for src in TS_SKILLS["source_types"]:
        if contains(src, combined):
            m.source_types_found.append(src)

    # Advanced features
    feat_patterns = {
        "conditional_paths":       r'paths|\.Paths|PathFilter',
        "dynamic_discovery":       r'discoverSkill|dynamic.*skill|walkUp|filepath\.Walk',
        "shell_exec":              r'exec\.Command|ShellExec|shell.*exec',
        "env_var_substitution":    r'CLAUDE_SKILL_DIR|CLAUDE_SESSION_ID|os\.Getenv',
        "named_args":              r'namedArg|named.*arg|argument.*name',
        "symlink_dedup":           r'Lstat|EvalSymlinks|realpath|symlink',
        "memoized_cache":          r'sync\.Once|memoize|cache|Cache',
        "dynamic_reload_callback": r'callback|OnLoad|onDynamic|Reload',
        "skills_lock_json":        r'skills.lock|skillslock|SkillsLock',
    }
    for feat, pat in feat_patterns.items():
        if contains(pat, combined):
            m.advanced_features_found.append(feat)

    # skills-lock.json: file exists but code must actually read it
    lock_file = gosrc / "skills-lock.json"
    if lock_file.exists():
        m.skills_lock_loaded = contains(r'skills-lock\.json|skillslock|SkillsLock', combined)

    return m


# ─────────────────────────────────────────────────────────────────────────────
# Registry analysis
# ─────────────────────────────────────────────────────────────────────────────

@dataclass
class RegistryMetrics:
    tool_count: int = 0
    thread_safe: bool = False
    clone_support: bool = False
    error_separation: bool = False
    order_preserved: bool = False
    duplicate_overwrite: bool = False
    permissions_system: bool = False
    audit_log: bool = False
    dynamic_register: bool = False


def analyze_registry(gosrc: Path) -> RegistryMetrics:
    m = RegistryMetrics()

    # Count Register( calls in registry_setup.go
    setup_files = list(gosrc.glob("registry_setup.go")) + list(gosrc.glob("**/registry_setup.go"))
    setup_text = "\n".join(read_file(f) for f in setup_files)
    m.tool_count = len(re.findall(r'\.Register\(', setup_text))

    # Analyze registry package
    files = find_go_files(gosrc, "registry")
    combined = "\n".join(read_file(f) for f in files) + "\n" + setup_text

    m.thread_safe         = contains(r'sync\.RWMutex|RLock|RUnlock|Lock\(\)', combined)
    m.clone_support       = contains(r'func.*Clone|\.Clone\(\)', combined)
    m.error_separation    = contains(r'ExecuteToolWithError|IsError', combined)
    m.order_preserved     = contains(r'order\s*\[\]string|r\.order', combined)
    m.duplicate_overwrite = contains(r'exists.*replace|silent.*overwrite|already.*registered', combined)
    m.permissions_system  = contains(r'Permission|PermissionLevel|permission_level', combined)
    m.audit_log           = contains(r'audit|AuditLog|logTool|tool.*log', combined)
    m.dynamic_register    = contains(r'Register.*runtime|runtime.*register|hot.*register', combined)

    return m


# ─────────────────────────────────────────────────────────────────────────────
# Commands analysis
# ─────────────────────────────────────────────────────────────────────────────

@dataclass
class CommandsMetrics:
    commands_found: list = field(default_factory=list)
    aliases_support: bool = False
    slash_parsing: bool = False
    err_exit_sentinel: bool = False
    subcommand_tree: bool = False
    autocomplete: bool = False
    dynamic_register: bool = False
    panic_on_duplicate: bool = False


def analyze_commands(gosrc: Path) -> CommandsMetrics:
    m = CommandsMetrics()
    files = find_go_files(gosrc, "commands")
    combined = "\n".join(read_file(f) for f in files)

    # Detect by string literals for each TS command name
    for cmd in TS_COMMANDS_ALL:
        safe = re.escape(cmd)
        if contains(rf'"{safe}"|`{safe}`', combined):
            m.commands_found.append(cmd)

    # Detect by struct names
    struct_to_cmd = {
        "helpCmd": "help", "exitCmd": "exit", "clearCmd": "clear",
        "compactCmd": "compact", "modelCmd": "model", "costCmd": "cost",
        "versionCmd": "version", "sessionCmd": "session",
    }
    for struct, cmd in struct_to_cmd.items():
        if contains(struct, combined) and cmd not in m.commands_found:
            m.commands_found.append(cmd)

    m.commands_found = sorted(set(m.commands_found))

    m.aliases_support    = contains(r'Aliases\(\)|alias|Alias', combined)
    m.slash_parsing      = contains(r'IsCommand|HasPrefix.*"/"|strings\.TrimPrefix.*"/"', combined)
    m.err_exit_sentinel  = contains(r'ErrExit|errExit', combined)
    m.subcommand_tree    = contains(r'subcommand|SubCommand|sub_command|sub.*cmd', combined)
    m.autocomplete       = contains(r'autocomplete|AutoComplete|Complete\(\)', combined)
    m.dynamic_register   = contains(r'dynamic.*register|runtime.*register', combined)
    m.panic_on_duplicate = contains(r'panic.*duplicate|already.*registered.*panic|panic\(', combined)

    return m


# ─────────────────────────────────────────────────────────────────────────────
# Score computation
# ─────────────────────────────────────────────────────────────────────────────

def pct(found: int, total: int) -> float:
    return round(found / total * 100, 1) if total > 0 else 0.0


def compute_mcp_score(m: MCPMetrics) -> dict:
    transport_cov   = pct(len(m.transports_found),        len(TS_MCP["transports"]))
    rpc_cov         = pct(len(m.rpc_methods_found),       len(TS_MCP["rpc_methods"]))
    config_type_cov = pct(len(m.config_types_found),      len(TS_MCP["config_types"]))
    scope_cov       = pct(len(m.config_scopes_found),     len(TS_MCP["config_scopes"]))
    feat_cov        = pct(len(m.advanced_features_found), len(TS_MCP["advanced_features"]))

    bonus = sum([
        5 if m.tool_name_prefix  else 0,
        5 if m.cursor_pagination else 0,
        5 if m.graceful_shutdown else 0,
        3 if m.protocol_version  else 0,
    ])

    # Weighted: transport×30, rpc×25, config×15, scope×10, feat×20
    weighted = (transport_cov * 0.30 + rpc_cov * 0.25 + config_type_cov * 0.15
                + scope_cov * 0.10 + feat_cov * 0.20)
    overall = min(100.0, round(weighted + bonus * 0.3, 1))

    return {
        "transport_coverage":         transport_cov,
        "rpc_method_coverage":        rpc_cov,
        "config_type_coverage":       config_type_cov,
        "config_scope_coverage":      scope_cov,
        "advanced_feature_coverage":  feat_cov,
        "quality_bonus_pts":          bonus,
        "weighted_overall":           overall,
        "transports_found":           m.transports_found,
        "rpc_found":                  m.rpc_methods_found,
        "config_types_found":         m.config_types_found,
        "config_scopes_found":        m.config_scopes_found,
        "advanced_features_found":    m.advanced_features_found,
        "protocol_version":           m.protocol_version,
        "cursor_pagination":          m.cursor_pagination,
        "tool_name_prefix":           m.tool_name_prefix,
        "graceful_shutdown":          m.graceful_shutdown,
    }


def compute_skills_score(m: SkillsMetrics) -> dict:
    source_cov   = pct(len(m.source_types_found),      len(TS_SKILLS["source_types"]))
    fm_field_cov = pct(len(m.frontmatter_fields_found), len(TS_SKILLS["frontmatter_fields"]))
    adv_cov      = pct(len(m.advanced_features_found), len(TS_SKILLS["advanced_features"]))

    core_pts = sum([
        20 if m.file_format_loading      else 0,
        20 if m.dir_format_loading       else 0,
        15 if m.trigger_detection        else 0,
        10 if m.case_insensitive_regex   else 0,
        15 if m.skill_tool_adapter       else 0,
        10 if m.placeholder_substitution else 0,
        10 if m.frontmatter_parsing      else 0,
    ])
    core_cov = min(100.0, float(core_pts))

    # Weighted: core×50, source×10, frontmatter×20, advanced×20
    weighted = (core_cov * 0.50 + source_cov * 0.10
                + fm_field_cov * 0.20 + adv_cov * 0.20)

    return {
        "core_loading_score":         core_cov,
        "source_type_coverage":       source_cov,
        "frontmatter_field_coverage": fm_field_cov,
        "advanced_feature_coverage":  adv_cov,
        "weighted_overall":           round(weighted, 1),
        "source_types_found":         m.source_types_found,
        "frontmatter_fields_found":   m.frontmatter_fields_found,
        "advanced_features_found":    m.advanced_features_found,
        "frontmatter_parsing":        m.frontmatter_parsing,
        "skills_lock_loaded":         m.skills_lock_loaded,
    }


def compute_registry_score(m: RegistryMetrics) -> dict:
    ts_tool_count = TS_REGISTRY["estimated_tools"]
    tool_cov = pct(m.tool_count, ts_tool_count)

    feature_pts = sum([
        25 if m.thread_safe         else 0,
        20 if m.clone_support       else 0,
        20 if m.error_separation    else 0,
        10 if m.order_preserved     else 0,
        10 if m.duplicate_overwrite else 0,
        10 if m.permissions_system  else 0,
        5  if m.audit_log           else 0,
    ])
    feature_cov = min(100.0, float(feature_pts))

    # Weighted: tools×40, features×60
    weighted = tool_cov * 0.40 + feature_cov * 0.60

    return {
        "tool_count_go":      m.tool_count,
        "tool_count_ts_est":  ts_tool_count,
        "tool_coverage":      tool_cov,
        "feature_score":      feature_cov,
        "weighted_overall":   round(weighted, 1),
        "thread_safe":        m.thread_safe,
        "clone_support":      m.clone_support,
        "error_separation":   m.error_separation,
        "order_preserved":    m.order_preserved,
        "permissions_system": m.permissions_system,
        "audit_log":          m.audit_log,
    }


def compute_commands_score(m: CommandsMetrics) -> dict:
    unconditional_found = [c for c in m.commands_found if c in TS_COMMANDS_UNCONDITIONAL]
    flagged_found       = [c for c in m.commands_found if c in TS_COMMANDS_FLAGGED]

    unconditional_cov = pct(len(unconditional_found), len(TS_COMMANDS_UNCONDITIONAL))
    flagged_cov       = pct(len(flagged_found),       len(TS_COMMANDS_FLAGGED))
    total_cov         = pct(len(m.commands_found),    len(TS_COMMANDS_ALL))

    feature_pts = sum([
        20 if m.aliases_support   else 0,
        20 if m.slash_parsing     else 0,
        15 if m.err_exit_sentinel else 0,
        20 if m.subcommand_tree   else 0,
        15 if m.autocomplete      else 0,
        10 if m.dynamic_register  else 0,
    ])
    feature_cov = min(100.0, float(feature_pts))

    # Weighted: unconditional×40, total×20, features×40
    weighted = unconditional_cov * 0.40 + total_cov * 0.20 + feature_cov * 0.40

    return {
        "unconditional_commands_found": len(unconditional_found),
        "unconditional_commands_total": len(TS_COMMANDS_UNCONDITIONAL),
        "unconditional_coverage":       unconditional_cov,
        "flagged_commands_found":       len(flagged_found),
        "flagged_commands_total":       len(TS_COMMANDS_FLAGGED),
        "flagged_coverage":             flagged_cov,
        "total_coverage":               total_cov,
        "feature_score":                feature_cov,
        "weighted_overall":             round(weighted, 1),
        "commands_found":               sorted(m.commands_found),
        "missing_unconditional": sorted(
            set(TS_COMMANDS_UNCONDITIONAL) - set(unconditional_found)
        ),
        "aliases_support":  m.aliases_support,
        "slash_parsing":    m.slash_parsing,
        "subcommand_tree":  m.subcommand_tree,
    }


# ─────────────────────────────────────────────────────────────────────────────
# Report rendering
# ─────────────────────────────────────────────────────────────────────────────

def bar(pct_val: float, width: int = 20) -> str:
    filled = round(pct_val / 100 * width)
    return "█" * filled + "░" * (width - filled)


def print_table(rows: list, headers: tuple, col_widths: list):
    sep = "  "
    header_line = sep.join(h.ljust(w) for h, w in zip(headers, col_widths))
    print(header_line)
    print("─" * (sum(col_widths) + len(sep) * (len(col_widths) - 1)))
    for row in rows:
        line = sep.join(str(cell).ljust(w) for cell, w in zip(row, col_widths))
        print(line)


def render_report(mcp: dict, skills: dict, registry: dict, commands: dict,
                  verbose: bool = False):
    print()
    print("╔══════════════════════════════════════════════════════════════╗")
    print("║        Extension System Coverage Report  (Go vs TS)         ║")
    print("╚══════════════════════════════════════════════════════════════╝")
    print()

    # ── Summary ──────────────────────────────────────────────────────────────
    overall = round(
        mcp["weighted_overall"]      * 0.30 +
        skills["weighted_overall"]   * 0.25 +
        commands["weighted_overall"] * 0.20 +
        registry["weighted_overall"] * 0.25,
        1
    )

    summary_rows = [
        ("MCP",      f"{mcp['weighted_overall']:5.1f}%",      bar(mcp["weighted_overall"]),      "weight 30%"),
        ("Skills",   f"{skills['weighted_overall']:5.1f}%",   bar(skills["weighted_overall"]),   "weight 25%"),
        ("Commands", f"{commands['weighted_overall']:5.1f}%", bar(commands["weighted_overall"]), "weight 20%"),
        ("Registry", f"{registry['weighted_overall']:5.1f}%", bar(registry["weighted_overall"]), "weight 25%"),
    ]
    print("  Subsystem Coverage")
    print_table(summary_rows, ("Subsystem", "Score", "Progress", "Note"),
                [12, 8, 22, 14])
    print()
    print(f"  {'─'*56}")
    print(f"  {'OVERALL WEIGHTED':40s}  {overall:5.1f}%  {bar(overall)}")
    print()

    # ── MCP detail ───────────────────────────────────────────────────────────
    print("━━━  MCP  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
    mcp_rows = [
        ("Transports",
         f"{len(mcp['transports_found'])}/4",
         bar(mcp["transport_coverage"]),
         f"{mcp['transport_coverage']:.0f}%"),
        ("RPC Methods",
         f"{len(mcp['rpc_found'])}/10",
         bar(mcp["rpc_method_coverage"]),
         f"{mcp['rpc_method_coverage']:.0f}%"),
        ("Config Types",
         f"{len(mcp['config_types_found'])}/4",
         bar(mcp["config_type_coverage"]),
         f"{mcp['config_type_coverage']:.0f}%"),
        ("Config Scopes",
         f"{len(mcp['config_scopes_found'])}/3",
         bar(mcp["config_scope_coverage"]),
         f"{mcp['config_scope_coverage']:.0f}%"),
        ("Advanced Features",
         f"{len(mcp['advanced_features_found'])}/7",
         bar(mcp["advanced_feature_coverage"]),
         f"{mcp['advanced_feature_coverage']:.0f}%"),
    ]
    print_table(mcp_rows, ("Dimension", "Count", "Bar", "Pct"), [20, 7, 22, 6])

    quality = []
    if mcp["tool_name_prefix"]:  quality.append("✔ tool-name-prefix")
    if mcp["cursor_pagination"]: quality.append("✔ cursor-pagination")
    if mcp["graceful_shutdown"]: quality.append("✔ graceful-shutdown")
    if mcp["protocol_version"]:  quality.append(f"✔ protocol-v{mcp['protocol_version']}")
    print(f"\n  Quality : {', '.join(quality) or '—'}")

    missing_t = [t for t in TS_MCP["transports"] if t not in mcp["transports_found"]]
    if missing_t:
        print(f"  Missing transports : {', '.join(missing_t)}")

    if verbose:
        missing_rpc = [r for r in TS_MCP["rpc_methods"] if r not in mcp["rpc_found"]]
        if missing_rpc:
            print(f"  Missing RPC methods: {', '.join(missing_rpc)}")
        if mcp["advanced_features_found"]:
            print(f"  Adv features found : {', '.join(mcp['advanced_features_found'])}")
    print()

    # ── Skills detail ─────────────────────────────────────────────────────────
    print("━━━  Skills  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
    sk_rows = [
        ("Core Loading",
         f"{skills['core_loading_score']:.0f}/100",
         bar(skills["core_loading_score"]),
         f"{skills['core_loading_score']:.0f}%"),
        ("Source Types",
         f"{len(skills['source_types_found'])}/6",
         bar(skills["source_type_coverage"]),
         f"{skills['source_type_coverage']:.0f}%"),
        ("Frontmatter Fields",
         f"{len(skills['frontmatter_fields_found'])}/15",
         bar(skills["frontmatter_field_coverage"]),
         f"{skills['frontmatter_field_coverage']:.0f}%"),
        ("Advanced Features",
         f"{len(skills['advanced_features_found'])}/9",
         bar(skills["advanced_feature_coverage"]),
         f"{skills['advanced_feature_coverage']:.0f}%"),
    ]
    print_table(sk_rows, ("Dimension", "Count", "Bar", "Pct"), [20, 7, 22, 6])

    fm   = "✔ parsed"   if skills["frontmatter_parsing"] else "✘ missing"
    lock = "✔ consumed" if skills["skills_lock_loaded"]   else "✘ not consumed (file exists)"
    print(f"\n  Frontmatter parsing : {fm}")
    print(f"  skills-lock.json    : {lock}")
    if verbose and skills["advanced_features_found"]:
        print(f"  Adv features found  : {', '.join(skills['advanced_features_found'])}")
    print()

    # ── Commands detail ───────────────────────────────────────────────────────
    print("━━━  Commands  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
    cmd_rows = [
        ("Unconditional Cmds",
         f"{commands['unconditional_commands_found']}/{commands['unconditional_commands_total']}",
         bar(commands["unconditional_coverage"]),
         f"{commands['unconditional_coverage']:.0f}%"),
        ("Feature-Flagged Cmds",
         f"{commands['flagged_commands_found']}/{commands['flagged_commands_total']}",
         bar(commands["flagged_coverage"]),
         f"{commands['flagged_coverage']:.0f}%"),
        ("Total",
         f"{len(commands['commands_found'])}/{len(TS_COMMANDS_ALL)}",
         bar(commands["total_coverage"]),
         f"{commands['total_coverage']:.0f}%"),
        ("Feature Score",
         f"{commands['feature_score']:.0f}/100",
         bar(commands["feature_score"]),
         f"{commands['feature_score']:.0f}%"),
    ]
    print_table(cmd_rows, ("Dimension", "Count", "Bar", "Pct"), [22, 7, 22, 6])

    found = commands["commands_found"]
    print(f"\n  Implemented : {', '.join(found) if found else '—'}")
    missing = commands["missing_unconditional"]
    if missing:
        chunks = [missing[i:i+6] for i in range(0, len(missing), 6)]
        label = "  Missing (unc): "
        for i, chunk in enumerate(chunks):
            prefix = label if i == 0 else " " * len(label)
            print(f"{prefix}{', '.join(chunk)}")
    print()

    # ── Registry detail ───────────────────────────────────────────────────────
    print("━━━  Registry  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
    reg_rows = [
        ("Tool Count",
         f"{registry['tool_count_go']}/{registry['tool_count_ts_est']}",
         bar(registry["tool_coverage"]),
         f"{registry['tool_coverage']:.0f}%"),
        ("Feature Score",
         f"{registry['feature_score']:.0f}/100",
         bar(registry["feature_score"]),
         f"{registry['feature_score']:.0f}%"),
    ]
    print_table(reg_rows, ("Dimension", "Count", "Bar", "Pct"), [16, 7, 22, 6])

    feats = []
    for label, val in [
        ("thread_safe",     registry["thread_safe"]),
        ("clone",           registry["clone_support"]),
        ("err_separation",  registry["error_separation"]),
        ("order_preserved", registry["order_preserved"]),
        ("permissions",     registry["permissions_system"]),
        ("audit_log",       registry["audit_log"]),
    ]:
        feats.append(f"{'✔' if val else '✘'} {label}")
    print(f"\n  Features: {', '.join(feats)}")
    print()

    # ── Gap priorities ────────────────────────────────────────────────────────
    print("━━━  Top Gaps (Recommended Priority Order)  ━━━━━━━━━━━━━━━━━━━")
    gaps = []
    if mcp["transport_coverage"] < 50:
        gaps.append(("P1", "MCP: Add SSE + StreamableHTTP transports",
                     f"Transport {mcp['transport_coverage']:.0f}% → est 75%"))
    if not skills["frontmatter_parsing"]:
        gaps.append(("P1", "Skills: Implement YAML frontmatter parsing",
                     "0/15 frontmatter fields supported"))
    if not skills["skills_lock_loaded"]:
        gaps.append(("P1", "Skills: Consume skills-lock.json",
                     "Lock file exists but Go code ignores it"))
    if commands["unconditional_coverage"] < 30:
        gaps.append(("P2", "Commands: Add /mcp /skills /config /doctor /memory",
                     f"Uncond coverage {commands['unconditional_coverage']:.0f}%"))
    if "conditional_paths" not in skills["advanced_features_found"]:
        gaps.append(("P2", "Skills: Conditional paths filtering",
                     "Per-file skill activation missing"))
    if "oauth" not in mcp["advanced_features_found"]:
        gaps.append(("P2", "MCP: OAuth token refresh",
                     "Required for remote MCP servers"))
    if not registry["permissions_system"]:
        gaps.append(("P3", "Registry: PermissionLevel system",
                     "Security layer for tool execution"))
    if "dynamic_discovery" not in skills["advanced_features_found"]:
        gaps.append(("P3", "Skills: Dynamic skill discovery",
                     "Walk up dir tree to find skill dirs"))

    print_table(gaps, ("Pri", "Gap", "Detail"), [4, 46, 38])
    print()


# ─────────────────────────────────────────────────────────────────────────────
# Main
# ─────────────────────────────────────────────────────────────────────────────

def main():
    parser = argparse.ArgumentParser(
        description="Evaluate Go vs TypeScript extension system coverage"
    )
    parser.add_argument(
        "--dir", default=None,
        help="Path to gosrc root (default: auto-detected from script location)"
    )
    parser.add_argument("--json",    action="store_true", help="Output raw JSON")
    parser.add_argument("--verbose", action="store_true", help="Show extra detail")
    args = parser.parse_args()

    # Resolve gosrc root: scripts/ → mcp/ → gosrc/
    if args.dir:
        gosrc = Path(args.dir).resolve()
    else:
        gosrc = Path(__file__).parent.parent.parent.resolve()

    if not gosrc.exists():
        print(f"ERROR: gosrc directory not found: {gosrc}", file=sys.stderr)
        sys.exit(1)

    if not args.json:
        print(f"Scanning: {gosrc}")

    # Run analyses
    mcp_m = analyze_mcp(gosrc)
    sk_m  = analyze_skills(gosrc)
    reg_m = analyze_registry(gosrc)
    cmd_m = analyze_commands(gosrc)

    # Compute scores
    mcp_s = compute_mcp_score(mcp_m)
    sk_s  = compute_skills_score(sk_m)
    reg_s = compute_registry_score(reg_m)
    cmd_s = compute_commands_score(cmd_m)

    if args.json:
        output = {
            "gosrc": str(gosrc),
            "mcp":      mcp_s,
            "skills":   sk_s,
            "registry": reg_s,
            "commands": cmd_s,
            "overall_weighted": round(
                mcp_s["weighted_overall"] * 0.30 +
                sk_s["weighted_overall"]  * 0.25 +
                cmd_s["weighted_overall"] * 0.20 +
                reg_s["weighted_overall"] * 0.25,
                1
            ),
        }
        print(json.dumps(output, indent=2, ensure_ascii=False))
    else:
        render_report(mcp_s, sk_s, reg_s, cmd_s, verbose=args.verbose)


if __name__ == "__main__":
    main()
