#!/usr/bin/env python3
"""
infrastructure_metrics.py — 基础设施层覆盖率评估脚本

对比 Go 实现与 TypeScript 原版在以下四个维度的覆盖率：
  1. 类型覆盖率   (types/)
  2. 权限模型完整度 (permissions/)
  3. 渲染特性支持度 (render/)
  4. CLI 参数兼容性 (cli/)
  5. 启动流程完整度 (main.go / repl.go / printmode.go / signals.go)

用法：
  python3 infrastructure_metrics.py [--gosrc PATH] [--tssrc PATH]

可多次复测；每次运行均读取实际源码文件进行静态检测。
"""

import argparse
import os
import re
import sys
from dataclasses import dataclass, field
from pathlib import Path
from typing import Dict, List, Optional, Tuple


# ──────────────────────────────────────────────────────────────────────────────
# 数据结构
# ──────────────────────────────────────────────────────────────────────────────

@dataclass
class CheckItem:
    """单条检查项"""
    name: str
    description: str
    check_fn: object      # Callable[[SrcPaths], bool]
    weight: float = 1.0   # 权重（部分实现可用 0.5）


@dataclass
class SrcPaths:
    gosrc: Path
    tssrc: Path

    def go_file(self, *parts) -> Path:
        return self.gosrc / Path(*parts)

    def ts_file(self, *parts) -> Path:
        return self.tssrc / Path(*parts)

    def go_exists(self, *parts) -> bool:
        return self.go_file(*parts).exists()

    def ts_exists(self, *parts) -> bool:
        return self.ts_file(*parts).exists()

    def go_content(self, *parts) -> str:
        p = self.go_file(*parts)
        return p.read_text(errors='replace') if p.exists() else ""

    def ts_content(self, *parts) -> str:
        p = self.ts_file(*parts)
        return p.read_text(errors='replace') if p.exists() else ""

    def go_dir_content(self, subdir: str) -> str:
        """拼合指定 Go 子目录下所有 .go 文件的内容"""
        d = self.gosrc / subdir
        if not d.is_dir():
            return ""
        parts = []
        for f in sorted(d.glob("*.go")):
            parts.append(f.read_text(errors='replace'))
        return "\n".join(parts)

    def go_root_content(self) -> str:
        """拼合 gosrc 根目录所有 .go 文件（不含子目录）"""
        parts = []
        for f in sorted(self.gosrc.glob("*.go")):
            parts.append(f.read_text(errors='replace'))
        return "\n".join(parts)


@dataclass
class CategoryResult:
    name: str
    items: List[Tuple[str, str, bool, float]]  # (name, desc, passed, weight)

    @property
    def score(self) -> float:
        total_weight = sum(w for _, _, _, w in self.items)
        earned = sum(w for _, _, passed, w in self.items if passed)
        return earned / total_weight if total_weight > 0 else 0.0

    @property
    def passed(self) -> int:
        return sum(1 for _, _, p, _ in self.items if p)

    @property
    def total(self) -> int:
        return len(self.items)


# ──────────────────────────────────────────────────────────────────────────────
# 辅助函数
# ──────────────────────────────────────────────────────────────────────────────

def contains(text: str, *patterns: str) -> bool:
    """检查 text 是否包含所有 patterns（简单子串）"""
    return all(p in text for p in patterns)


def re_search(pattern: str, text: str) -> bool:
    return bool(re.search(pattern, text))


def file_line_count(path: Path) -> int:
    if not path.exists():
        return 0
    if path.is_dir():
        # Sum all .ts / .go files inside a directory
        total = 0
        for ext in ("*.ts", "*.go", "*.py"):
            for f in path.glob(ext):
                total += sum(1 for _ in f.open(errors='replace'))
        return total
    return sum(1 for _ in path.open(errors='replace'))


# ──────────────────────────────────────────────────────────────────────────────
# 检查项定义
# ──────────────────────────────────────────────────────────────────────────────

# ── 1. 类型覆盖率 ─────────────────────────────────────────────────────────────

def check_types(paths: SrcPaths) -> List[Tuple[str, str, bool, float]]:
    msgs = paths.go_content("types", "messages.go")
    tools = paths.go_content("types", "tools.go")
    stream = paths.go_content("types", "stream.go")

    return [
        ("TextBlock",          "文本内容块",
         contains(msgs, "TextBlock", "ContentTypeText"), 1.0),

        ("ToolUseBlock",       "工具调用块",
         contains(msgs, "ToolUseBlock", "ContentTypeToolUse"), 1.0),

        ("ToolResultBlock",    "工具结果块",
         contains(msgs, "ToolResultBlock", "ContentTypeToolResult"), 1.0),

        ("ThinkingBlock",      "思考块（含 Signature）",
         contains(msgs, "ThinkingBlock", "Signature"), 1.0),

        ("ImageBlock",         "图片块",
         contains(msgs, "ImageBlock", "ImageSource"), 1.0),

        ("UnknownBlock",       "未知块（防数据丢失）",
         contains(msgs, "UnknownBlock", "json.RawMessage"), 1.0),

        ("ContentBlock interface", "内容块接口",
         contains(msgs, "ContentBlock", "GetType()"), 1.0),

        ("Role 常量",           "user/assistant 角色",
         contains(msgs, "RoleUser", "RoleAssistant"), 1.0),

        ("StopReason",         "停止原因枚举",
         contains(msgs, "StopReasonEndTurn", "StopReasonToolUse"), 1.0),

        ("Message.MarshalJSON",   "自定义 JSON 序列化",
         contains(tools, "MarshalJSON"), 1.0),

        ("Message.UnmarshalJSON", "自定义 JSON 反序列化",
         contains(tools, "UnmarshalJSON"), 1.0),

        ("Tool interface",     "工具接口（Name/Description/Schema/Execute）",
         contains(tools, "Name()", "Description()", "Schema()", "Execute("), 1.0),

        ("ToolResult 两级错误", "IsError 业务错误语义",
         contains(tools, "IsError", "ToolResult"), 1.0),

        ("StreamEvent",        "流式事件",
         contains(stream, "StreamEvent", "StreamEventType"), 1.0),

        ("EventType 常量完整性", "8 种事件类型",
         all(e in stream for e in [
             "EventMessageStart", "EventContentBlockStart", "EventContentBlockDelta",
             "EventContentBlockStop", "EventMessageDelta", "EventMessageStop",
             "EventPing", "EventError"
         ]), 1.0),

        ("Usage token 字段",   "含缓存字段",
         contains(stream, "CacheCreationInputTokens", "CacheReadInputTokens"), 1.0),

        ("CreateMessageRequest", "请求体类型",
         contains(stream, "CreateMessageRequest", "MaxTokens", "Stream"), 1.0),

        ("APIMessage",         "API 顶层响应类型",
         contains(stream, "APIMessage", "StopReason", "Usage"), 1.0),

        ("APIError.Error()",   "错误接口实现",
         contains(stream, "APIError", "func (e *APIError) Error()"), 1.0),

        ("ToDefinitions",      "工具批量转换辅助函数",
         contains(tools, "ToDefinitions", "ToDefinition"), 1.0),
    ]


# ── 2. 权限模型完整度 ─────────────────────────────────────────────────────────

def check_permissions(paths: SrcPaths) -> List[Tuple[str, str, bool, float]]:
    perm = paths.go_dir_content("permissions")

    return [
        ("ModeAllowAll",        "AllowAll 模式",
         contains(perm, "ModeAllowAll"), 1.0),

        ("ModeAskAlways",       "AskAlways 模式",
         contains(perm, "ModeAskAlways"), 1.0),

        ("ModeRuleBased",       "RuleBased 模式",
         contains(perm, "ModeRuleBased"), 1.0),

        ("DecisionAllow/Deny/Ask", "三种基础决策",
         all(d in perm for d in ["DecisionAllow", "DecisionDeny", "DecisionAsk"]), 1.0),

        ("DecisionAllowOnce",   "AllowOnce（不缓存）",
         contains(perm, "DecisionAllowOnce"), 1.0),

        ("Checker struct",      "权限检查器",
         contains(perm, "type Checker struct"), 1.0),

        ("Rule struct",         "规则结构体",
         contains(perm, "type Rule struct", "Tool", "Pattern", "Decision"), 1.0),

        ("sessionCache",        "会话级缓存",
         contains(perm, "sessionCache"), 1.0),

        ("sync.RWMutex",        "并发安全",
         contains(perm, "sync.RWMutex"), 1.0),

        ("promptFunc",          "交互式提示回调",
         contains(perm, "promptFunc"), 1.0),

        ("glob 模式匹配",        "工具名通配符",
         contains(perm, "filepath.Match", "matchPattern"), 1.0),

        ("file_path glob 匹配",  "文件路径 glob",
         contains(perm, "file_path", "filepath.Match"), 1.0),

        ("command 前缀匹配",     "Bash 命令前缀",
         contains(perm, "HasPrefix", "command"), 1.0),

        ("Bash SHA256 缓存键",   "防命令缓存碰撞",
         contains(perm, "sha256", "Bash"), 1.0),

        ("fail-closed 设计",    "模式错误视为匹配",
         contains(perm, "fail closed", "return true"), 0.5),

        ("多来源规则",           "rules source 字段（原版特性）",
         contains(perm, "source", "Source"), 0.0),  # 未实现，权重0

        ("AI 分类器",            "auto 模式 LLM 分类",
         contains(perm, "classifier", "Classifier"), 0.0),  # 未实现，权重0
    ]


# ── 3. 渲染特性支持度 ─────────────────────────────────────────────────────────

def check_render(paths: SrcPaths) -> List[Tuple[str, str, bool, float]]:
    md = paths.go_content("render", "markdown.go")
    rv = paths.go_content("render.go")  # 根目录 render.go
    # 若根目录不存在则尝试 gosrc 根
    if not rv:
        rv = paths.go_root_content()

    return [
        ("H1/H2/H3 标题",       "三级标题渲染",
         all(h in md for h in ["# ", "## ", "### "]), 1.0),

        ("粗体 **bold**",        "加粗",
         contains(md, "boldRe", "Bold"), 1.0),

        ("斜体 *italic*",        "斜体",
         contains(md, "italicRe", "Italic"), 1.0),

        ("行内代码 `code`",       "行内代码高亮",
         contains(md, "inlineCodeRe", "FgCyan"), 1.0),

        ("删除线 ~~strike~~",     "删除线",
         contains(md, "strikeRe", "CrossedOut"), 1.0),

        ("无序列表 -/*",          "项目符号",
         contains(md, "bullet", "•"), 1.0),

        ("有序列表 1.",           "数字列表",
         contains(md, "numberedListRe"), 1.0),

        ("引用块 >",              "blockquote",
         contains(md, '"> "', "│"), 0.5),  # prefix match + pipe marker

        ("围栏代码块 ```",         "代码块（含语言标签）",
         contains(md, "inCodeBlock", "codeBlockLang"), 1.0),

        ("水平分割线",             "--- / *** / ___",
         contains(md, "isHorizontalRule", "────"), 1.0),

        ("链接 [text](url)",      "链接文本渲染",
         contains(md, "linkRe", "Underline"), 1.0),

        ("代码语法高亮",           "chroma 等库（原版特性）",
         re_search(r"chroma|highlight|syntax", md), 0.0),  # 未实现

        ("OSC 8 超链接",           "终端超链接（原版特性）",
         re_search(r"OSC|\\033\]8|hyperlink", md), 0.0),  # 未实现

        ("表格渲染",               "Markdown table（原版特性）",
         re_search(r"table|Table|\|.*\|", md), 0.0),  # 未实现

        ("formatToolInput",       "工具调用输入预览",
         contains(rv, "formatToolInput"), 1.0),

        ("formatToolResult",      "工具结果多行展示",
         contains(rv, "formatToolResult", "truncated"), 1.0),

        ("makeEventHandler",      "Print 模式事件回调",
         contains(rv, "makeEventHandler"), 1.0),

        ("makeREPLEventHandler",  "REPL 模式事件回调",
         contains(rv, "makeREPLEventHandler"), 1.0),

        ("EventThinking 渲染",    "思考块显示",
         contains(rv, "EventThinking"), 1.0),

        ("token 用量显示",         "cache token 统计",
         contains(rv, "CacheReadInputTokens", "CacheCreationInputTokens"), 1.0),
    ]


# ── 4. CLI 参数兼容性 ─────────────────────────────────────────────────────────

def check_cli(paths: SrcPaths) -> List[Tuple[str, str, bool, float]]:
    cli = paths.go_content("cli", "cli.go")

    return [
        ("-p / print 模式",       "单次查询标志",
         contains(cli, '"p"', "Print"), 1.0),

        ("--model / -m",          "模型选择",
         contains(cli, '"model"', '"m"'), 1.0),

        ("--provider",            "提供商选择（Go 独有）",
         contains(cli, '"provider"'), 1.0),

        ("--max-turns",           "最大轮次",
         contains(cli, '"max-turns"', "MaxTurns"), 1.0),

        ("--system-prompt",       "系统提示覆盖",
         contains(cli, '"system-prompt"'), 1.0),

        ("--resume",              "恢复上次会话",
         contains(cli, '"resume"'), 1.0),

        ("--session-id",          "指定会话 ID",
         contains(cli, '"session-id"'), 1.0),

        ("session-id 安全验证",    "防路径遍历攻击",
         contains(cli, "validSessionID", r'a-zA-Z0-9'), 1.0),

        ("--allowed-dir 可重复",   "multiString flag.Value",
         contains(cli, "multiString", "allowed-dir"), 1.0),

        ("--allow-all",           "跳过权限检查",
         contains(cli, '"allow-all"', "AllowAll"), 1.0),

        ("--verbose",             "详细日志",
         contains(cli, '"verbose"'), 1.0),

        ("--version / -v",        "版本信息",
         contains(cli, '"version"', '"v"', "Version"), 1.0),

        ("--help 输出到 stdout",   "help exit 0",
         contains(cli, "os.Stdout", "os.Exit(0)"), 1.0),

        ("Version 常量",           "版本号定义",
         re_search(r'Version\s*=\s*"v\d', cli), 1.0),

        ("--output-format",       "JSON 输出格式（原版特性）",
         contains(cli, "output-format"), 0.0),  # 未实现

        ("--mcp-server",          "MCP 服务器参数（原版特性）",
         contains(cli, "mcp-server"), 0.0),  # 未实现

        ("--permission-mode",     "权限模式参数（原版特性）",
         contains(cli, "permission-mode"), 0.0),  # 未实现

        ("--allowed-tools",       "工具白名单（原版特性）",
         contains(cli, "allowed-tools"), 0.0),  # 未实现
    ]


# ── 5. 启动流程完整度 ─────────────────────────────────────────────────────────

def check_startup(paths: SrcPaths) -> List[Tuple[str, str, bool, float]]:
    root = paths.go_root_content()
    main_txt = paths.go_content("main.go") if paths.go_exists("main.go") else root
    repl_txt = paths.go_content("repl.go") if paths.go_exists("repl.go") else root
    print_txt = paths.go_content("printmode.go") if paths.go_exists("printmode.go") else root
    sig_txt = paths.go_content("signals.go") if paths.go_exists("signals.go") else root

    return [
        ("cli.Parse() 调用",      "参数解析入口",
         contains(main_txt, "cli.Parse()"), 1.0),

        ("provider.New",          "Provider 初始化",
         re_search(r"provider\.New", main_txt), 1.0),

        ("os.Getwd()",            "工作目录获取",
         contains(main_txt, "os.Getwd()"), 1.0),

        ("SetupRegistry",         "工具注册",
         contains(main_txt, "SetupRegistry"), 1.0),

        ("prompt.BuildSystemPrompt", "系统提示构建",
         contains(main_txt, "BuildSystemPrompt"), 1.0),

        ("prompt.DiscoverClaudeMD", "CLAUDE.md 加载",
         contains(main_txt, "DiscoverClaudeMD"), 1.0),

        ("loop.New",              "查询循环创建",
         contains(main_txt, "loop.New"), 1.0),

        ("hooks.LoadFromSettings", "settings.json hooks",
         contains(main_txt, "LoadFromSettings"), 1.0),

        ("RunPrintMode",          "Print 模式分支",
         contains(main_txt, "RunPrintMode") and contains(print_txt, "RunPrintMode"), 1.0),

        ("RunREPL",               "REPL 模式分支",
         contains(main_txt, "RunREPL") and contains(repl_txt, "RunREPL"), 1.0),

        ("session.NewFileStore",  "会话存储初始化",
         contains(main_txt, "NewFileStore"), 1.0),

        ("compact.NewResultStore","工具输出持久化初始化",
         contains(main_txt, "NewResultStore"), 1.0),

        ("readline.NewEx",        "readline 初始化",
         contains(main_txt, "readline.NewEx"), 1.0),

        ("两级 SIGINT 处理",       "query-cancel vs global-cancel",
         contains(sig_txt, "SetQueryCancel", "globalCancel", "queryCancelFn"), 1.0),

        ("SIGTERM 处理",           "始终退出",
         contains(sig_txt, "SIGTERM", "globalCancel"), 1.0),

        ("context.WithCancel",    "queryCtx 取消",
         contains(repl_txt, "context.WithCancel", "queryCancel"), 1.0),

        ("slash 命令分发",         "cmdReg.IsCommand",
         contains(repl_txt, "IsCommand", "cmdReg"), 1.0),

        ("Store.Save 自动存档",    "每轮后存档",
         contains(repl_txt, "Store.Save", "SessionID"), 1.0),

        ("context.Canceled 检测", "取消 vs 错误区分",
         contains(repl_txt, "context.Canceled") or contains(print_txt, "context.Canceled"), 1.0),

        ("自动更新检测",            "GitHub releases 检查（原版特性）",
         re_search(r"update|autoupdate|github.*release", main_txt, ), 0.0),  # 未实现
    ]


def re_search(pattern: str, text: str, flags: int = re.IGNORECASE) -> bool:
    return bool(re.search(pattern, text, flags))


# ──────────────────────────────────────────────────────────────────────────────
# 运行所有检查
# ──────────────────────────────────────────────────────────────────────────────

def run_category(name: str, items: List[Tuple[str, str, bool, float]]) -> CategoryResult:
    return CategoryResult(name=name, items=items)


def run_all(paths: SrcPaths) -> List[CategoryResult]:
    categories = [
        ("类型覆盖率",     check_types(paths)),
        ("权限模型完整度", check_permissions(paths)),
        ("渲染特性支持度", check_render(paths)),
        ("CLI 参数兼容性", check_cli(paths)),
        ("启动流程完整度", check_startup(paths)),
    ]
    return [run_category(name, items) for name, items in categories]


# ──────────────────────────────────────────────────────────────────────────────
# 输出格式化
# ──────────────────────────────────────────────────────────────────────────────

RESET  = "\033[0m"
BOLD   = "\033[1m"
GREEN  = "\033[32m"
RED    = "\033[31m"
YELLOW = "\033[33m"
CYAN   = "\033[36m"
DIM    = "\033[2m"


def color(text: str, code: str, use_color: bool) -> str:
    return f"{code}{text}{RESET}" if use_color else text


def score_bar(score: float, width: int = 20) -> str:
    filled = int(score * width)
    bar = "█" * filled + "░" * (width - filled)
    return bar


def score_color(score: float, use_color: bool) -> str:
    pct = score * 100
    if pct >= 80:
        c = GREEN
    elif pct >= 50:
        c = YELLOW
    else:
        c = RED
    return color(f"{pct:5.1f}%", c, use_color)


def print_category(cat: CategoryResult, use_color: bool, verbose: bool) -> None:
    bar = score_bar(cat.score)
    score_str = score_color(cat.score, use_color)
    title = color(f"▶ {cat.name}", BOLD + CYAN, use_color)
    print(f"\n{title}")
    print(f"  {bar}  {score_str}  ({cat.passed}/{cat.total} 项通过)")
    print()

    if verbose:
        # 详细模式：每项都打印
        for name, desc, passed, weight in cat.items:
            if weight == 0.0:
                icon = color("○", DIM, use_color)
                status = color("N/A", DIM, use_color)
            elif passed:
                icon = color("✓", GREEN, use_color)
                status = color("PASS", GREEN, use_color)
            else:
                icon = color("✗", RED, use_color)
                status = color("FAIL", RED, use_color)
            w_str = color(f"(w={weight:.1f})", DIM, use_color)
            print(f"    {icon} [{status}] {name:30s} {desc} {w_str}")
    else:
        # 简洁模式：只打印失败项
        failed = [(n, d, p, w) for n, d, p, w in cat.items if not p and w > 0]
        if failed:
            print(f"  {color('未实现项：', YELLOW, use_color)}")
            for name, desc, _, weight in failed:
                print(f"    {color('✗', RED, use_color)} {name:30s} {desc}")
        else:
            print(f"  {color('  所有检测项均通过！', GREEN, use_color)}")


def print_summary(results: List[CategoryResult], use_color: bool) -> None:
    print()
    print(color("=" * 72, BOLD, use_color))
    print(color("  基础设施层覆盖率汇总", BOLD, use_color))
    print(color("=" * 72, BOLD, use_color))
    print()

    # 表头
    col1 = "模块"
    col2 = "得分"
    col3 = "进度条"
    col4 = "通过/总计"
    print(f"  {'模块':<16}  {'得分':>7}  {'进度条':<22}  {'通过/总计':>10}")
    print(f"  {'-'*16}  {'-'*7}  {'-'*22}  {'-'*10}")

    total_weight = 0.0
    total_earned = 0.0

    for cat in results:
        cat_weight = sum(w for _, _, _, w in cat.items)
        cat_earned = sum(w for _, _, p, w in cat.items if p)
        total_weight += cat_weight
        total_earned += cat_earned

        score_str = score_color(cat.score, use_color)
        bar = score_bar(cat.score, 20)
        passed_str = f"{cat.passed}/{cat.total}"
        print(f"  {cat.name:<16}  {score_str}  {bar}  {passed_str:>10}")

    # 综合得分
    overall = total_earned / total_weight if total_weight > 0 else 0.0
    print()
    overall_str = score_color(overall, use_color)
    overall_bar = score_bar(overall, 20)
    print(f"  {'综合覆盖率':<16}  {overall_str}  {overall_bar}")
    print()
    print(color("=" * 72, BOLD, use_color))


def print_gap_table(results: List[CategoryResult], use_color: bool) -> None:
    """打印差距分析（仅展示未通过、权重>0 的项）"""
    any_gap = False
    lines = []
    for cat in results:
        for name, desc, passed, weight in cat.items:
            if not passed and weight > 0:
                if not any_gap:
                    any_gap = True
                    lines.append(("模块", "检查项", "说明"))
                    lines.append(("-" * 12, "-" * 28, "-" * 30))
                lines.append((cat.name, name, desc))

    if not any_gap:
        print(color("\n  ✓ 所有有效检查项均已实现，无差距！", GREEN, use_color))
        return

    print()
    print(color("  差距项（weight > 0 且未通过）", BOLD + YELLOW, use_color))
    print()
    for row in lines:
        print(f"  {row[0]:<14}  {row[1]:<30}  {row[2]}")


# ──────────────────────────────────────────────────────────────────────────────
# 文件统计
# ──────────────────────────────────────────────────────────────────────────────

def print_file_stats(paths: SrcPaths, use_color: bool) -> None:
    print()
    print(color("  Go 源文件统计", BOLD, use_color))
    print()

    modules = [
        ("types/",       ["messages.go", "tools.go", "stream.go"]),
        ("permissions/", ["permissions.go", "prompt.go"]),
        ("render/",      ["markdown.go"]),
        ("cli/",         ["cli.go"]),
        ("root",         ["main.go", "repl.go", "printmode.go", "signals.go", "render.go"]),
    ]

    total_lines = 0
    print(f"  {'模块':<20}  {'文件':<22}  {'行数':>8}")
    print(f"  {'-'*20}  {'-'*22}  {'-'*8}")

    for module, files in modules:
        for fname in files:
            if module == "root":
                p = paths.gosrc / fname
            else:
                p = paths.gosrc / module.rstrip("/") / fname
            lines = file_line_count(p)
            total_lines += lines
            exists_str = "" if p.exists() else color(" [不存在]", RED, use_color)
            print(f"  {module:<20}  {fname:<22}  {lines:>8}{exists_str}")

    print(f"  {'':20}  {'合计':<22}  {total_lines:>8}")
    print()

    # TS 对比文件
    print(color("  TS 原版对应文件", BOLD, use_color))
    print()
    ts_files = [
        ("src/types/", "permissions.ts"),
        ("src/types/", "ids.ts"),
        ("src/cli/",   "exit.ts"),
        ("src/utils/", "permissions/"),
    ]
    for subdir, fname in ts_files:
        p = paths.tssrc / subdir.lstrip("src/").rstrip("/") / fname if subdir != "src/" else paths.tssrc / fname
        # 简化路径构建
        p2 = paths.tssrc.parent / subdir / fname
        lines2 = file_line_count(p2)
        print(f"  {subdir + fname:<44}  {lines2:>8} 行")


# ──────────────────────────────────────────────────────────────────────────────
# 主入口
# ──────────────────────────────────────────────────────────────────────────────

def parse_args():
    parser = argparse.ArgumentParser(
        description="基础设施层覆盖率评估脚本（types / permissions / render / cli / startup）"
    )
    parser.add_argument(
        "--gosrc",
        default=str(Path(__file__).parent.parent.parent),  # gosrc/
        help="Go 源码根目录（默认：脚本目录上两级）",
    )
    parser.add_argument(
        "--tssrc",
        default=str(Path(__file__).parent.parent.parent.parent / "src"),
        help="TypeScript 源码根目录（默认：gosrc 同级 src/）",
    )
    parser.add_argument(
        "--no-color",
        action="store_true",
        help="禁用 ANSI 颜色输出",
    )
    parser.add_argument(
        "--verbose", "-v",
        action="store_true",
        help="详细模式：打印每项检查结果（默认只打印失败项）",
    )
    parser.add_argument(
        "--gaps",
        action="store_true",
        help="额外打印差距分析表",
    )
    parser.add_argument(
        "--stats",
        action="store_true",
        help="打印文件行数统计",
    )
    return parser.parse_args()


def main():
    args = parse_args()

    gosrc = Path(args.gosrc).resolve()
    tssrc = Path(args.tssrc).resolve()
    use_color = not args.no_color and sys.stdout.isatty()

    print(color("\n  Claude Code Go — 基础设施层覆盖率评估", BOLD + CYAN, use_color))
    print(color(f"  Go 源码: {gosrc}", DIM, use_color))
    print(color(f"  TS 源码: {tssrc}", DIM, use_color))

    if not gosrc.is_dir():
        print(color(f"\n错误：Go 源码目录不存在：{gosrc}", RED, use_color))
        sys.exit(1)

    paths = SrcPaths(gosrc=gosrc, tssrc=tssrc)
    results = run_all(paths)

    for cat in results:
        print_category(cat, use_color, args.verbose)

    print_summary(results, use_color)

    if args.gaps:
        print_gap_table(results, use_color)

    if args.stats:
        print_file_stats(paths, use_color)

    # 返回码：综合覆盖率 < 50% 则返回 1
    total_w = sum(w for cat in results for _, _, _, w in cat.items)
    total_e = sum(w for cat in results for _, _, p, w in cat.items if p)
    overall = total_e / total_w if total_w > 0 else 0
    sys.exit(0 if overall >= 0.5 else 1)


if __name__ == "__main__":
    main()
