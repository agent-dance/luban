---
license: other
license_name: tencent
language:
- en
- zh
task_categories:
- text-generation
tags:
- code
- agent
- benchmark
- swe
- security
pretty_name: Tencent WorkBuddy Bench
size_categories:
- n<1K
---

# WorkBuddy Bench — Datasets

The task datasets for
[**WorkBuddy Bench**](https://github.com/Tencent/workbuddy-bench), a benchmark
for evaluating coding agents on real-world developer, PM, algo, QA, ops, and
security work. This repository hosts **only the task data**; the evaluation
framework, setup, and usage all live in the GitHub repository above.

### Dataset Summary

WorkBuddy Bench is built from real-world work tasks. Given a task and a
sandboxed workspace, an agent must produce the correct change — a patch, an
artifact, or a report — which is then graded against a test suite. The benchmark
ships four independent subsets, one `.tar.gz` archive each:

| Subset | Archive | Tasks | Domain |
|--------|---------|-------|--------|
| Code | `wb-bench-code-v1.0.tar.gz` | 80 | repo-level coding tasks |
| Web | `wb-bench-web-v1.0.tar.gz` | 70 | front-end / GUI (HTML/CSS/JS) |
| Office | `wb-bench-office-v1.0.tar.gz` | 50 | office data / file workflows |
| Security | `wb-bench-sec-v1.0.tar.gz` | 60 | security / vuln (red + blue) |

`SHA256SUMS` lists the checksum of every archive.

### Dataset Structure

Each archive extracts to `wb-bench-<subset>-v1.0/`, with one directory per task:

```
wb-bench-<subset>-v1.0/
  dataset.toml
  tasks/<task-id>/
    instruction.md                 # the task prompt
    task.toml                      # task metadata
    environment/                   # Dockerfile, docker-compose, scorer, workspace.tar.gz
    tests/                         # grading suite
```

Each task's `environment/workspace.tar.gz` stays packed; the task Dockerfile
extracts it into `/workspace` inside the container.

### Usage

These archives are consumed by the WorkBuddy Bench framework, not loaded
directly. See the [GitHub repository](https://github.com/Tencent/workbuddy-bench)
for how to download, extract, and run evaluations.
