#!/usr/bin/env python3
"""
tool_parity_audit.py

Compare the model-facing tool surface from ../src/tools.ts with the Go
registry defined in registry_setup.go. The report focuses on:
  - tool presence in each implementation
  - model-facing tool names
  - top-level input schema fields

This intentionally audits the source of truth for each side instead of keeping
another hand-maintained registry that goes stale.
"""

from __future__ import annotations

import argparse
import json
import os
import re
import subprocess
import sys
import tempfile
import textwrap
from dataclasses import asdict, dataclass, field
from pathlib import Path
from typing import Iterable


@dataclass
class ToolSurface:
    symbol: str
    source: str
    name: str | None
    params: list[str]
    aliases: list[str] = field(default_factory=list)
    notes: list[str] = field(default_factory=list)


@dataclass
class ToolDiff:
    key: str
    ts_symbol: str | None
    go_symbol: str | None
    ts_name: str | None
    go_name: str | None
    ts_params: list[str]
    go_params: list[str]
    status: str
    notes: list[str] = field(default_factory=list)


SPECIAL_TS_TO_GO = {
    "ExitPlanModeV2Tool": "ExitPlanModeTool",
}

TS_INPUT_PARAM_OVERRIDES = {
    "/AgentTool/AgentTool.tsx": [
        "description",
        "prompt",
        "subagent_type",
        "model",
        "run_in_background",
        "name",
        "team_name",
        "mode",
        "isolation",
        "cwd",
    ],
    "/AskUserQuestionTool/AskUserQuestionTool.tsx": [
        "questions",
        "questions[].header",
        "questions[].question",
        "questions[].options",
        "questions[].options[].label",
        "questions[].options[].description",
        "questions[].options[].preview",
        "questions[].multiSelect",
    ],
    "/ExitPlanModeTool/ExitPlanModeV2Tool.ts": [
        "allowedPrompts",
        "allowedPrompts[].tool",
        "allowedPrompts[].prompt",
    ],
    "/SendMessageTool/SendMessageTool.ts": ["to", "summary", "message"],
    "/TaskUpdateTool/TaskUpdateTool.ts": [
        "taskId",
        "subject",
        "description",
        "activeForm",
        "status",
        "addBlocks",
        "addBlockedBy",
        "owner",
        "metadata",
    ],
    "/TodoWriteTool/TodoWriteTool.ts": [
        "todos",
        "todos[].content",
        "todos[].status",
        "todos[].activeForm",
    ],
}


def read_text(path: Path) -> str:
    return path.read_text(encoding="utf-8")


def strip_comments(text: str) -> str:
    text = re.sub(r"//.*", "", text)
    text = re.sub(r"/\*.*?\*/", "", text, flags=re.S)
    return text


def find_matching(text: str, start: int, open_char: str, close_char: str) -> int:
    depth = 0
    in_string: str | None = None
    escape = False
    i = start
    while i < len(text):
        ch = text[i]
        if in_string:
            if escape:
                escape = False
            elif ch == "\\":
                escape = True
            elif ch == in_string:
                in_string = None
            i += 1
            continue
        if ch in ("'", '"', "`"):
            in_string = ch
            i += 1
            continue
        if ch == open_char:
            depth += 1
        elif ch == close_char:
            depth -= 1
            if depth == 0:
                return i
        i += 1
    raise ValueError(f"unmatched {open_char}{close_char} starting at {start}")


def extract_top_level_keys(object_text: str) -> list[str]:
    text = object_text.strip()
    if not text.startswith("{") or not text.endswith("}"):
        return []
    inner = text[1:-1]
    keys: list[str] = []
    i = 0
    brace = bracket = paren = 0
    in_string: str | None = None
    escape = False
    while i < len(inner):
        ch = inner[i]
        if in_string:
            if escape:
                escape = False
            elif ch == "\\":
                escape = True
            elif ch == in_string:
                in_string = None
            i += 1
            continue
        if ch in ("'", '"', "`"):
            in_string = ch
            i += 1
            continue
        if ch == "/" and i + 1 < len(inner):
            if inner[i + 1] == "/":
                i = inner.find("\n", i)
                if i == -1:
                    break
                continue
            if inner[i + 1] == "*":
                end = inner.find("*/", i + 2)
                if end == -1:
                    break
                i = end + 2
                continue
        if ch == "{":
            brace += 1
            i += 1
            continue
        if ch == "}":
            brace -= 1
            i += 1
            continue
        if ch == "[":
            bracket += 1
            i += 1
            continue
        if ch == "]":
            bracket -= 1
            i += 1
            continue
        if ch == "(":
            paren += 1
            i += 1
            continue
        if ch == ")":
            paren -= 1
            i += 1
            continue
        if brace == bracket == paren == 0:
            ident_match = re.match(r"\s*([A-Za-z_][A-Za-z0-9_]*)\s*:", inner[i:])
            if ident_match:
                keys.append(ident_match.group(1))
                i += ident_match.end()
                continue
            string_match = re.match(r'\s*["\']([^"\']+)["\']\s*:', inner[i:])
            if string_match:
                keys.append(string_match.group(1))
                i += string_match.end()
                continue
        i += 1
    seen: set[str] = set()
    ordered: list[str] = []
    for key in keys:
        if key not in seen:
            ordered.append(key)
            seen.add(key)
    return ordered


def extract_object_property_value(object_text: str, key: str) -> str | None:
    text = object_text.strip()
    if not text.startswith("{") or not text.endswith("}"):
        return None
    inner = text[1:-1]
    i = 0
    brace = bracket = paren = 0
    in_string: str | None = None
    escape = False
    while i < len(inner):
        ch = inner[i]
        if in_string:
            if escape:
                escape = False
            elif ch == "\\":
                escape = True
            elif ch == in_string:
                in_string = None
            i += 1
            continue
        if ch in ("'", '"', "`"):
            in_string = ch
            i += 1
            continue
        if ch == "/" and i + 1 < len(inner):
            if inner[i + 1] == "/":
                i = inner.find("\n", i)
                if i == -1:
                    break
                continue
            if inner[i + 1] == "*":
                end = inner.find("*/", i + 2)
                if end == -1:
                    break
                i = end + 2
                continue
        if ch == "{":
            brace += 1
            i += 1
            continue
        if ch == "}":
            brace -= 1
            i += 1
            continue
        if ch == "[":
            bracket += 1
            i += 1
            continue
        if ch == "]":
            bracket -= 1
            i += 1
            continue
        if ch == "(":
            paren += 1
            i += 1
            continue
        if ch == ")":
            paren -= 1
            i += 1
            continue
        if brace == bracket == paren == 0:
            match = re.match(rf"\s*{re.escape(key)}\s*:", inner[i:])
            if match:
                value_start = i + match.end()
                j = value_start
                local_brace = local_bracket = local_paren = 0
                local_string: str | None = None
                local_escape = False
                while j < len(inner):
                    cj = inner[j]
                    if local_string:
                        if local_escape:
                            local_escape = False
                        elif cj == "\\":
                            local_escape = True
                        elif cj == local_string:
                            local_string = None
                        j += 1
                        continue
                    if cj in ("'", '"', "`"):
                        local_string = cj
                        j += 1
                        continue
                    if cj == "{":
                        local_brace += 1
                    elif cj == "}":
                        if local_brace == 0 and local_bracket == 0 and local_paren == 0:
                            break
                        local_brace -= 1
                    elif cj == "[":
                        local_bracket += 1
                    elif cj == "]":
                        local_bracket -= 1
                    elif cj == "(":
                        local_paren += 1
                    elif cj == ")":
                        local_paren -= 1
                    elif (
                        cj == ","
                        and local_brace == 0
                        and local_bracket == 0
                        and local_paren == 0
                    ):
                        break
                    j += 1
                return inner[value_start:j].strip()
        i += 1
    return None


def resolve_module(base_dir: Path, spec: str) -> Path | None:
    spec_path = spec.replace(".js", "")
    candidate = (base_dir / spec_path).resolve()
    for suffix in (".ts", ".tsx"):
        path = candidate.with_suffix(suffix)
        if path.exists():
            return path
    if candidate.exists():
        return candidate
    return None


def env_truthy(value: str | None) -> bool:
    if value is None:
        return False
    return value.strip().lower() in {"1", "true", "yes", "on"}


def env_defined_falsy(value: str | None) -> bool:
    if value is None:
        return False
    return value.strip().lower() in {"0", "false", "no", "off"}


def ts_has_embedded_search_tools() -> bool:
    if not env_truthy(os.environ.get("EMBEDDED_SEARCH_TOOLS")):
        return False
    entrypoint = os.environ.get("CLAUDE_CODE_ENTRYPOINT", "")
    return entrypoint not in {"sdk-ts", "sdk-py", "sdk-cli", "local-agent"}


def ts_is_agent_swarms_enabled() -> bool:
    if os.environ.get("USER_TYPE") == "ant":
        return True
    if env_truthy(os.environ.get("CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS")):
        return True
    return "--agent-teams" in sys.argv


def ts_is_tool_search_enabled_optimistic() -> bool:
    if env_truthy(os.environ.get("CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS")):
        return False
    value = os.environ.get("ENABLE_TOOL_SEARCH")
    if not value:
        return True
    if env_truthy(value):
        return True
    lowered = value.strip().lower()
    if env_defined_falsy(value) or lowered == "auto:100":
        return False
    return True


def should_include_ts_symbol(symbol: str) -> bool:
    if symbol in {"ConfigTool", "TungstenTool", "REPLTool"}:
        return os.environ.get("USER_TYPE") == "ant"
    if symbol == "LSPTool":
        return env_truthy(os.environ.get("ENABLE_LSP_TOOL"))
    if symbol in {"CronCreateTool", "CronDeleteTool", "CronListTool"}:
        return env_truthy(os.environ.get("AGENT_TRIGGERS"))
    if symbol == "RemoteTriggerTool":
        return env_truthy(os.environ.get("AGENT_TRIGGERS_REMOTE"))
    if symbol in {"TeamCreateTool", "TeamDeleteTool"}:
        return ts_is_agent_swarms_enabled()
    if symbol == "TestingPermissionTool":
        return os.environ.get("NODE_ENV") == "test"
    if symbol in {"GlobTool", "GrepTool"} and ts_has_embedded_search_tools():
        return False
    if symbol == "ToolSearchTool":
        return ts_is_tool_search_enabled_optimistic()
    return True


def find_exported_constant_literal(base_dir: Path, const_name: str) -> str | None:
    for path in base_dir.glob("*.ts*"):
        text = read_text(path)
        literal = re.search(
            rf"\bexport\s+const\s+{re.escape(const_name)}\s*=\s*['\"]([^'\"]+)['\"]",
            text,
        )
        if literal:
            return literal.group(1)
    return None


def parse_ts_imports(text: str, base_dir: Path) -> dict[str, Path]:
    imports: dict[str, Path] = {}

    for match in re.finditer(
        r"import\s*\{([^}]+)\}\s*from\s*['\"]([^'\"]+)['\"]", text
    ):
        names = [part.strip().split(" as ")[0].strip() for part in match.group(1).split(",")]
        path = resolve_module(base_dir, match.group(2))
        if path is None:
            continue
        for name in names:
            imports[name] = path

    for match in re.finditer(
        r"require\(\s*['\"]([^'\"]+)['\"]\s*\)\.(\w+)", text, flags=re.S
    ):
        path = resolve_module(base_dir, match.group(1))
        if path is None:
            continue
        imports[match.group(2)] = path

    return imports


def parse_ts_helper_defs(text: str) -> dict[str, str]:
    helpers: dict[str, str] = {}
    for match in re.finditer(r"\bconst\s+(\w+)\s*=", text):
        name = match.group(1)
        start = match.end()
        next_match = re.search(r"\n(?:const|export\s+function)\s+\w+", text[start:])
        end = start + next_match.start() if next_match else len(text)
        helpers[name] = text[start:end]
    return helpers


def extract_get_all_base_tools_block(text: str) -> str:
    text = strip_comments(text)
    anchor = "export function getAllBaseTools(): Tools"
    start = text.find(anchor)
    if start == -1:
        raise ValueError("getAllBaseTools not found")
    brace_start = text.find("{", start)
    brace_end = find_matching(text, brace_start, "{", "}")
    return text[brace_start:brace_end + 1]


def expand_ts_symbols(block: str, known_symbols: set[str], helper_defs: dict[str, str]) -> list[str]:
    found: list[str] = []
    seen: set[str] = set()
    visiting: set[str] = set()

    def add_symbol(symbol: str) -> None:
        if symbol in seen:
            return
        seen.add(symbol)
        found.append(symbol)

    def walk(text: str) -> None:
        for symbol in sorted(known_symbols, key=len, reverse=True):
            if re.search(rf"\b{re.escape(symbol)}\b", text):
                if symbol.endswith("Tool"):
                    add_symbol(symbol)
                elif symbol in helper_defs:
                    if symbol in visiting:
                        continue
                    visiting.add(symbol)
                    walk(helper_defs[symbol])
                    visiting.remove(symbol)
        for helper in helper_defs:
            if re.search(rf"\b{re.escape(helper)}\b", text):
                if helper in visiting:
                    continue
                visiting.add(helper)
                walk(helper_defs[helper])
                visiting.remove(helper)

    walk(block)
    return found


def extract_ts_tool_name(path: Path) -> tuple[str | None, list[str]]:
    text = read_text(path)
    stripped = strip_comments(text)
    imports = parse_ts_imports(text, path.parent)
    notes: list[str] = []

    build_tool_pos = stripped.find("buildTool({")
    if build_tool_pos != -1:
        nearby = stripped[build_tool_pos : build_tool_pos + 1600]
        nearby_match = re.search(
            r"\bname\s*:\s*(?:['\"]([^'\"]+)['\"]|([A-Z][A-Z0-9_]+))",
            nearby,
        )
        if nearby_match:
            if nearby_match.group(1):
                return nearby_match.group(1), notes
            const_name = nearby_match.group(2)
            if const_name:
                import_path = imports.get(const_name)
                if import_path:
                    const_text = read_text(import_path)
                    literal = re.search(
                        rf"\bexport\s+const\s+{re.escape(const_name)}\s*=\s*['\"]([^'\"]+)['\"]",
                        const_text,
                    )
                    if literal:
                        return literal.group(1), notes
                fallback_literal = find_exported_constant_literal(path.parent, const_name)
                if fallback_literal is not None:
                    return fallback_literal, notes
                notes.append(f"unresolved name constant {const_name}")
        try:
            brace_start = stripped.find("{", build_tool_pos)
            brace_end = find_matching(stripped, brace_start, "{", "}")
            build_obj = stripped[brace_start:brace_end + 1]
            value = extract_object_property_value(build_obj, "name")
            if value:
                value = value.strip()
                quoted = re.fullmatch(r"['\"]([^'\"]+)['\"]", value)
                if quoted:
                    return quoted.group(1), notes
                const_match = re.fullmatch(r"([A-Z][A-Z0-9_]+)", value)
                if const_match:
                    const_name = const_match.group(1)
                    import_path = imports.get(const_name)
                    if import_path:
                        const_text = read_text(import_path)
                        literal = re.search(
                            rf"\bexport\s+const\s+{re.escape(const_name)}\s*=\s*['\"]([^'\"]+)['\"]",
                            const_text,
                        )
                        if literal:
                            return literal.group(1), notes
                    fallback_literal = find_exported_constant_literal(path.parent, const_name)
                    if fallback_literal is not None:
                        return fallback_literal, notes
                    notes.append(f"unresolved name constant {const_name}")
        except ValueError:
            notes.append("unable to parse buildTool() object")

    direct_patterns = [
        r"\breadonly\s+name\s*=\s*'([^']+)'",
        r'\breadonly\s+name\s*=\s*"([^"]+)"',
        r"(?<![\w.])name\s*=\s*'([^']+)'",
        r'(?<![\w.])name\s*=\s*"([^"]+)"',
    ]
    for pattern in direct_patterns:
        match = re.search(pattern, text)
        if match:
            return match.group(1), notes

    notes.append("unable to resolve model-facing name")
    return None, notes


def extract_ts_schema_body(text: str, schema_name: str) -> str | None:
    match = re.search(rf"(?:export\s+)?const\s+{re.escape(schema_name)}\s*=\s*lazySchema\(\s*\(\)\s*=>", text)
    if not match:
        return None
    call_start = text.find("(", match.end() - 1)
    if call_start == -1:
        return None
    call_end = find_matching(text, call_start, "(", ")")
    return text[match.end():call_end]


def extract_top_level_return_expression(block_expr: str) -> str | None:
    text = block_expr.strip()
    if not text.startswith("{") or not text.endswith("}"):
        return text

    inner = text[1:-1]
    i = 0
    brace = bracket = paren = 0
    in_string: str | None = None
    escape = False
    while i < len(inner):
        ch = inner[i]
        if in_string:
            if escape:
                escape = False
            elif ch == "\\":
                escape = True
            elif ch == in_string:
                in_string = None
            i += 1
            continue
        if ch in ("'", '"', "`"):
            in_string = ch
            i += 1
            continue
        if ch == "{":
            brace += 1
            i += 1
            continue
        if ch == "}":
            brace -= 1
            i += 1
            continue
        if ch == "[":
            bracket += 1
            i += 1
            continue
        if ch == "]":
            bracket -= 1
            i += 1
            continue
        if ch == "(":
            paren += 1
            i += 1
            continue
        if ch == ")":
            paren -= 1
            i += 1
            continue
        if brace == bracket == paren == 0 and inner.startswith("return", i):
            value_start = i + len("return")
            j = value_start
            local_brace = local_bracket = local_paren = 0
            local_string: str | None = None
            local_escape = False
            while j < len(inner):
                cj = inner[j]
                if local_string:
                    if local_escape:
                        local_escape = False
                    elif cj == "\\":
                        local_escape = True
                    elif cj == local_string:
                        local_string = None
                    j += 1
                    continue
                if cj in ("'", '"', "`"):
                    local_string = cj
                    j += 1
                    continue
                if cj == "{":
                    local_brace += 1
                elif cj == "}":
                    local_brace -= 1
                elif cj == "[":
                    local_bracket += 1
                elif cj == "]":
                    local_bracket -= 1
                elif cj == "(":
                    local_paren += 1
                elif cj == ")":
                    local_paren -= 1
                elif cj == ";" and local_brace == local_bracket == local_paren == 0:
                    break
                j += 1
            return inner[value_start:j].strip()
        i += 1
    return None


def unwrap_ts_schema_expr(expr: str) -> str:
    return extract_top_level_return_expression(expr) or expr.strip()


def extract_omit_keys(expr: str) -> list[str]:
    omit_keys: list[str] = []
    for match in re.finditer(r"\.omit\(\s*\{", expr):
        brace_start = expr.find("{", match.start())
        brace_end = find_matching(expr, brace_start, "{", "}")
        omit_keys.extend(extract_top_level_keys(expr[brace_start:brace_end + 1]))
    return omit_keys


def extract_ts_input_params(path: Path) -> tuple[list[str], list[str]]:
    text = read_text(path)
    imports = parse_ts_imports(text, path.parent)
    notes: list[str] = []

    normalized_path = str(path).replace("\\", "/")
    for suffix, params in TS_INPUT_PARAM_OVERRIDES.items():
        if normalized_path.endswith(suffix):
            return params, notes
    if normalized_path.endswith("/AgentTool/AgentTool.tsx"):
        full_body = extract_ts_schema_body(text, "fullInputSchema")
        if full_body is not None:
            return extract_ts_params_from_expr(full_body, text, imports, path.parent)
    if normalized_path.endswith("/BashTool/BashTool.tsx"):
        full_body = extract_ts_schema_body(text, "fullInputSchema")
        if full_body is not None:
            params, extra_notes = extract_ts_params_from_expr(full_body, text, imports, path.parent)
            return [param for param in params if param != "_simulatedSedEdit"], extra_notes

    body = extract_ts_schema_body(text, "inputSchema")
    if body is None:
        import_match = re.search(r"import\s*\{[^}]*\binputSchema\b[^}]*\}\s*from\s*['\"]([^'\"]+)['\"]", text)
        if import_match:
            imported = resolve_module(path.parent, import_match.group(1))
            if imported is not None and imported != path:
                return extract_ts_input_params(imported)
        notes.append("inputSchema not found")
        return [], notes

    body = unwrap_ts_schema_expr(body)

    if "fullInputSchema()" in body:
        full_body = extract_ts_schema_body(text, "fullInputSchema")
        if full_body:
            params, extra_notes = extract_ts_params_from_expr(full_body, text, imports, path.parent)
            omit_keys = set(extract_omit_keys(body))
            if params:
                return [param for param in params if param not in omit_keys], notes + extra_notes

    return extract_ts_params_from_expr(body, text, imports, path.parent)


def resolve_ts_schema_reference(
    ref: str,
    expr: str,
    file_text: str,
    imports: dict[str, Path],
    base_dir: Path,
) -> tuple[list[str], list[str]]:
    local_match = re.search(
        rf"\bconst\s+{re.escape(ref)}\s*=\s*z\.(?:strictObject|object)\(\{{",
        expr,
    )
    if local_match:
        brace_start = expr.find("{", local_match.end() - 1)
        brace_end = find_matching(expr, brace_start, "{", "}")
        return extract_top_level_keys(expr[brace_start:brace_end + 1]), []

    imported = imports.get(ref)
    if imported is not None:
        return extract_ts_input_params(imported)

    nested_body = extract_ts_schema_body(file_text, ref)
    if nested_body is not None:
        return extract_ts_params_from_expr(nested_body, file_text, imports, base_dir)

    return [], [f"unable to resolve schema reference {ref}"]


def extract_ts_params_from_expr(
    expr: str,
    file_text: str,
    imports: dict[str, Path],
    base_dir: Path,
) -> tuple[list[str], list[str]]:
    expr = unwrap_ts_schema_expr(expr)
    notes: list[str] = []
    for needle in ("z.strictObject({", "z.object({"):
        pos = expr.find(needle)
        if pos != -1:
            brace_start = expr.find("{", pos)
            brace_end = find_matching(expr, brace_start, "{", "}")
            return extract_top_level_keys(expr[brace_start:brace_end + 1]), notes

    merge_extend = re.search(
        r"return\s+(\w+)\(\)\.merge\((\w+)\)\.extend\(\{",
        expr,
    )
    if merge_extend:
        left_ref, right_ref = merge_extend.group(1), merge_extend.group(2)
        left_params, left_notes = resolve_ts_schema_reference(left_ref, expr, file_text, imports, base_dir)
        right_params, right_notes = resolve_ts_schema_reference(right_ref, expr, file_text, imports, base_dir)
        brace_start = expr.find("{", merge_extend.end() - 1)
        brace_end = find_matching(expr, brace_start, "{", "}")
        extend_params = extract_top_level_keys(expr[brace_start:brace_end + 1])
        ordered: list[str] = []
        for param in [*left_params, *right_params, *extend_params]:
            if param not in ordered:
                ordered.append(param)
        return ordered, notes + left_notes + right_notes

    ref_match = re.search(r"(\w+)\(\)", expr)
    if ref_match:
        ref = ref_match.group(1)
        if ref == "inputSchema":
            notes.append("recursive inputSchema reference")
            return [], notes
        return resolve_ts_schema_reference(ref, expr, file_text, imports, base_dir)

    notes.append("unable to resolve input schema fields")
    return [], notes


def normalize_ts_symbol(symbol: str) -> str:
    return SPECIAL_TS_TO_GO.get(symbol, symbol)


def build_ts_surfaces(ts_root: Path) -> dict[str, ToolSurface]:
    tools_ts = ts_root / "tools.ts"
    text = read_text(tools_ts)
    imports = parse_ts_imports(text, ts_root)
    helper_defs = parse_ts_helper_defs(text)
    block = extract_get_all_base_tools_block(text)
    eligible_symbols = {
        name
        for name in set(imports) | set(helper_defs)
        if (
            (
                name in helper_defs
                and (
                    name.endswith("Tool")
                    or name.endswith("Tools")
                    or (name.startswith("get") and name.endswith("Tool"))
                )
            )
            or (
                name in imports
                and "/src/tools/" in str(imports[name]).replace("\\", "/")
                and (
                    name.endswith("Tool")
                    or name.endswith("Tools")
                    or (name.startswith("get") and name.endswith("Tool"))
                )
            )
        )
    }
    symbols = expand_ts_symbols(block, eligible_symbols, helper_defs)

    surfaces: dict[str, ToolSurface] = {}
    for symbol in symbols:
        target_symbol = symbol
        path = imports.get(symbol)
        if path is None and symbol in helper_defs:
            helper_text = helper_defs[symbol]
            helper_match = re.search(r"\.(\w+Tool)\b", helper_text)
            if helper_match is None:
                helper_match = re.search(
                    r"\b(getSendMessageTool|getTeamCreateTool|getTeamDeleteTool)\b",
                    symbol,
                )
            if helper_match:
                target_symbol = {
                    "getSendMessageTool": "SendMessageTool",
                    "getTeamCreateTool": "TeamCreateTool",
                    "getTeamDeleteTool": "TeamDeleteTool",
                }.get(helper_match.group(1), helper_match.group(1))
                path = imports.get(target_symbol)
        if path is None:
            continue
        if not should_include_ts_symbol(target_symbol):
            continue
        key = normalize_ts_symbol(target_symbol)
        name, name_notes = extract_ts_tool_name(path)
        params, param_notes = extract_ts_input_params(path)
        notes = [*name_notes, *param_notes]
        surfaces[key] = ToolSurface(
            symbol=target_symbol,
            source=str(path.relative_to(ts_root.parent)),
            name=name,
            params=params,
            notes=notes,
        )

    lazy_required_tools = {
        "SendMessageTool": "./tools/SendMessageTool/SendMessageTool",
        "TeamCreateTool": "./tools/TeamCreateTool/TeamCreateTool",
        "TeamDeleteTool": "./tools/TeamDeleteTool/TeamDeleteTool",
    }
    for symbol, spec_path in lazy_required_tools.items():
        if symbol in surfaces or not should_include_ts_symbol(symbol):
            continue
        tool_path = resolve_module(ts_root, spec_path)
        if tool_path is None:
            continue
        name, name_notes = extract_ts_tool_name(tool_path)
        params, param_notes = extract_ts_input_params(tool_path)
        surfaces[symbol] = ToolSurface(
            symbol=symbol,
            source=str(tool_path.relative_to(ts_root.parent)),
            name=name,
            params=params,
            notes=[*name_notes, *param_notes],
        )
    return surfaces


def parse_go_registry_symbols(registry_setup: Path) -> list[str]:
    lines = read_text(registry_setup).splitlines()
    vars_to_symbols: dict[str, str] = {}
    symbols: list[str] = []

    def add(symbol: str) -> None:
        if symbol not in symbols:
            symbols.append(symbol)

    for line in lines:
        if match := re.search(r"(\w+)\s*:=\s*&tools\.(\w+)\{", line):
            vars_to_symbols[match.group(1)] = match.group(2)
        elif match := re.search(r"(\w+)\s*:=\s*tools\.(New\w+)\(", line):
            vars_to_symbols[match.group(1)] = match.group(2)

        if match := re.search(r"reg\.Register\(&tools\.(\w+)\{", line):
            add(match.group(1))
        elif match := re.search(r"reg\.Register\(tools\.(New\w+)\(", line):
            add(match.group(1))
        elif match := re.search(r"reg\.Register\((\w+)\)", line):
            symbol = vars_to_symbols.get(match.group(1))
            if symbol:
                add(symbol)
    return symbols


def build_go_indexes(go_tools_dir: Path) -> tuple[dict[str, Path], dict[str, tuple[str, Path]]]:
    type_index: dict[str, Path] = {}
    ctor_index: dict[str, tuple[str, Path]] = {}
    for path in go_tools_dir.glob("*.go"):
        text = read_text(path)
        for match in re.finditer(r"\btype\s+(\w+)\s+struct\b", text):
            type_index[match.group(1)] = path
        for match in re.finditer(r"\bfunc\s+(New\w+)\([^)]*\)\s+\*(\w+)\b", text):
            ctor_index[match.group(1)] = (match.group(2), path)
    return type_index, ctor_index


def extract_go_name(text: str, type_name: str) -> tuple[str | None, list[str]]:
    notes: list[str] = []
    match = re.search(
        rf"func\s+\(.*\*\s*{re.escape(type_name)}\)\s+Name\(\)\s+string\s*\{{\s*return\s+\"([^\"]+)\"",
        text,
    )
    if match:
        return match.group(1), notes
    notes.append("Name() literal not found")
    return None, notes


def extract_go_aliases(text: str, type_name: str) -> list[str]:
    match = re.search(
        rf"func\s+\(.*\*\s*{re.escape(type_name)}\)\s+Aliases\(\)\s+\[\]string\s*\{{\s*return\s+\[\]string\{{([^}}]*)\}}",
        text,
        flags=re.S,
    )
    if not match:
        return []
    return re.findall(r'"([^"]+)"', match.group(1))


def extract_go_schema_params(text: str, type_name: str) -> tuple[list[str], list[str]]:
    notes: list[str] = []
    schema_match = re.search(
        rf"func\s+\(.*\*\s*{re.escape(type_name)}\)\s+Schema\(\)\s+types\.JSONSchema\s*\{{",
        text,
    )
    if not schema_match:
        notes.append("Schema() not found")
        return [], notes
    schema_start = text.find("{", schema_match.end() - 1)
    schema_end = find_matching(text, schema_start, "{", "}")
    schema_block = text[schema_start:schema_end + 1]
    props_match = re.search(r"Properties\s*:\s*map\[string\]any\s*\{", schema_block)
    if not props_match:
        notes.append("schema properties not found")
        return [], notes
    brace_start = schema_block.find("{", props_match.end() - 1)
    brace_end = find_matching(schema_block, brace_start, "{", "}")
    return extract_top_level_keys(schema_block[brace_start:brace_end + 1]), notes


def probe_go_surfaces(go_root: Path) -> dict[str, dict[str, object]]:
    probe_source = textwrap.dedent(
        """
        package main

        import (
        	"encoding/json"
        	"os"
        	"reflect"
        	"sort"

        	"github.com/anthropics/claude-code-go/provider"
        	"github.com/anthropics/claude-code-go/sandbox"
        	"github.com/anthropics/claude-code-go/types"
        )

        type aliasedTool interface {
        	Aliases() []string
        }

        type surface struct {
        	Symbol  string   `json:"symbol"`
        	Name    string   `json:"name"`
        	Params  []string `json:"params"`
        	Aliases []string `json:"aliases"`
        }

        func appendNestedSchemaKeys(prefix string, value any, out *[]string) {
        	obj, ok := value.(map[string]any)
        	if !ok {
        		return
        	}

        	if rawItems, ok := obj["items"].(map[string]any); ok {
        		if rawProps, ok := rawItems["properties"].(map[string]any); ok {
        			for key, raw := range rawProps {
        				name := prefix + "[]." + key
        				*out = append(*out, name)
        				appendNestedSchemaKeys(name, raw, out)
        			}
        		}
        	}

        	if rawProps, ok := obj["properties"].(map[string]any); ok {
        		for key, raw := range rawProps {
        			name := prefix + "." + key
        			*out = append(*out, name)
        			appendNestedSchemaKeys(name, raw, out)
        		}
        	}
        }

        func schemaKeys(schema types.JSONSchema) []string {
        	keys := make([]string, 0, len(schema.Properties))
        	for key, raw := range schema.Properties {
        		keys = append(keys, key)
        		appendNestedSchemaKeys(key, raw, &keys)
        	}
        	sort.Strings(keys)
        	deduped := make([]string, 0, len(keys))
        	for _, key := range keys {
        		if len(deduped) == 0 || deduped[len(deduped)-1] != key {
        			deduped = append(deduped, key)
        		}
        	}
        	return deduped
        }

        func main() {
        	deps := SetupRegistry(&provider.ProviderRef{}, ".", nil, sandbox.NoopBackend{}, nil)
        	surfaces := map[string]surface{}
        	for _, tool := range deps.Registry.All() {
        		typ := reflect.TypeOf(tool)
        		if typ.Kind() == reflect.Ptr {
        			typ = typ.Elem()
        		}
        		item := surface{
        			Symbol: typ.Name(),
        			Name:   tool.Name(),
        			Params: schemaKeys(tool.Schema()),
        		}
        		if aliased, ok := tool.(aliasedTool); ok {
        			item.Aliases = aliased.Aliases()
        		}
        		surfaces[item.Symbol] = item
        	}
        	_ = json.NewEncoder(os.Stdout).Encode(surfaces)
        }
        """
    ).strip()

    temp_file = tempfile.NamedTemporaryFile(
        mode="w",
        suffix="_tool_parity_probe.go",
        dir=go_root,
        delete=False,
        encoding="utf-8",
    )
    try:
        temp_path = Path(temp_file.name)
        temp_file.write(probe_source)
        temp_file.close()
        proc = subprocess.run(
            ["go", "run", temp_path.name, "registry_setup.go"],
            cwd=go_root,
            check=True,
            capture_output=True,
            text=True,
        )
        return json.loads(proc.stdout)
    finally:
        try:
            Path(temp_file.name).unlink()
        except FileNotFoundError:
            pass


def build_go_surfaces(go_root: Path) -> dict[str, ToolSurface]:
    type_index, ctor_index = build_go_indexes(go_root / "tools")
    runtime_surfaces = probe_go_surfaces(go_root)
    surfaces: dict[str, ToolSurface] = {}

    for symbol, runtime in runtime_surfaces.items():
        path = type_index.get(symbol)
        if path is None:
            for ctor_type, ctor_path in ctor_index.values():
                if ctor_type == symbol:
                    path = ctor_path
                    break
        source = str(path.relative_to(go_root)) if path is not None else ""
        surfaces[symbol] = ToolSurface(
            symbol=symbol,
            source=source,
            name=runtime.get("name"),
            params=sorted(runtime.get("params", [])),
            aliases=runtime.get("aliases", []),
            notes=[],
        )
    return surfaces


def compare_surfaces(
    ts_surfaces: dict[str, ToolSurface], go_surfaces: dict[str, ToolSurface]
) -> list[ToolDiff]:
    diffs: list[ToolDiff] = []
    all_keys = sorted(set(ts_surfaces) | set(go_surfaces))
    for key in all_keys:
        ts = ts_surfaces.get(key)
        go = go_surfaces.get(key)
        notes: list[str] = []
        if ts is None:
            status = "go_only"
        elif go is None:
            status = "ts_only"
        else:
            name_match = ts.name == go.name or (ts.name is not None and ts.name in go.aliases)
            params_match = set(ts.params) == set(go.params)
            if name_match and params_match:
                status = "match"
            elif name_match:
                status = "param_mismatch"
            elif params_match:
                status = "name_mismatch"
            else:
                status = "name_and_param_mismatch"
            notes.extend(ts.notes)
            notes.extend(go.notes)
        diffs.append(
            ToolDiff(
                key=key,
                ts_symbol=ts.symbol if ts else None,
                go_symbol=go.symbol if go else None,
                ts_name=ts.name if ts else None,
                go_name=go.name if go else None,
                ts_params=ts.params if ts else [],
                go_params=go.params if go else [],
                status=status,
                notes=sorted(set(filter(None, notes))),
            )
        )
    return diffs


def render_markdown(diffs: list[ToolDiff], ts_count: int, go_count: int) -> str:
    status_counts: dict[str, int] = {}
    for diff in diffs:
        status_counts[diff.status] = status_counts.get(diff.status, 0) + 1

    lines = [
        "# Tool Parity Audit",
        "",
        f"- TS base tools: {ts_count}",
        f"- Go registered tools: {go_count}",
        f"- Exact matches: {status_counts.get('match', 0)}",
        f"- Name mismatches: {status_counts.get('name_mismatch', 0)}",
        f"- Param mismatches: {status_counts.get('param_mismatch', 0)}",
        f"- Name+param mismatches: {status_counts.get('name_and_param_mismatch', 0)}",
        f"- TS only: {status_counts.get('ts_only', 0)}",
        f"- Go only: {status_counts.get('go_only', 0)}",
        "",
        "| Tool | Status | TS name | Go name | TS params | Go params |",
        "| --- | --- | --- | --- | --- | --- |",
    ]

    for diff in diffs:
        lines.append(
            "| {tool} | {status} | {ts_name} | {go_name} | `{ts_params}` | `{go_params}` |".format(
                tool=diff.key,
                status=diff.status,
                ts_name=diff.ts_name or "-",
                go_name=diff.go_name or "-",
                ts_params=", ".join(diff.ts_params),
                go_params=", ".join(diff.go_params),
            )
        )

    noteworthy = [
        diff for diff in diffs if diff.status not in {"match"} and diff.notes
    ]
    if noteworthy:
        lines.extend(["", "## Notes", ""])
        for diff in noteworthy:
            lines.append(f"- {diff.key}: {'; '.join(diff.notes)}")

    return "\n".join(lines)


def main(argv: list[str]) -> int:
    parser = argparse.ArgumentParser(description="Audit TS vs Go tool parity")
    parser.add_argument("--json", action="store_true", help="Emit JSON instead of markdown")
    parser.add_argument(
        "--go-root",
        default=str(Path(__file__).resolve().parents[2]),
        help="Path to gosrc root",
    )
    parser.add_argument(
        "--ts-root",
        default=str(Path(__file__).resolve().parents[3] / "src"),
        help="Path to TypeScript src root",
    )
    args = parser.parse_args(argv)

    go_root = Path(args.go_root).resolve()
    ts_root = Path(args.ts_root).resolve()

    ts_surfaces = build_ts_surfaces(ts_root)
    go_surfaces = build_go_surfaces(go_root)
    diffs = compare_surfaces(ts_surfaces, go_surfaces)

    payload = {
        "summary": {
            "ts_base_tools": len(ts_surfaces),
            "go_registered_tools": len(go_surfaces),
        },
        "ts_tools": {k: asdict(v) for k, v in ts_surfaces.items()},
        "go_tools": {k: asdict(v) for k, v in go_surfaces.items()},
        "diffs": [asdict(diff) for diff in diffs],
    }

    if args.json:
        json.dump(payload, sys.stdout, indent=2, ensure_ascii=False)
        sys.stdout.write("\n")
    else:
        sys.stdout.write(render_markdown(diffs, len(ts_surfaces), len(go_surfaces)))
        sys.stdout.write("\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
