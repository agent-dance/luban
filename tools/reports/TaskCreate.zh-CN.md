# TaskCreate 一致性报告

- 原版: `src/tools/TaskCreateTool/TaskCreateTool.ts`
- Go版: `tools/tasks.go`

## 结论

- 摘要: 接口完全对齐；创建契约已对齐，Go 现在也已经是在持久化、scope-aware、带锁的后端之上创建任务，但原版运行时仍然更丰富。

## 名称与描述

- 名称一致性: 完全一致。
- 描述一致性: 两边都把这个工具描述为用于在共享任务系统中创建一个任务条目。 除“关键差异”外，措辞差别不影响核心语义。

## 参数与类型

- 类型签名: subject: string，description?: string，activeForm?: string，metadata?: object。
- 参数一致性: 顶层参数名与原版审计结果一致；字段顺序差异不构成语义差异。

## 实现概要

- 核心能力: 在共享任务系统中创建一个任务条目。
- 典型场景: 适用于工作需要被显式跟踪，而不是隐含在自由推理文本里。
- 核心痛点: 它解决的是多步骤工作的可见性和协同问题，避免模型只能靠自由文本硬记整套计划。
- 主要挑战: 难点在于持久化、依赖边、阻塞语义，以及跨工具、跨会话保持任务状态一致。
- 实现思路一致性: 部分一致。两边都围绕共享的持久化任务系统展开，Go 现在也已镜像更多原版的 scope、加锁和 runtime-task 底座，但原版仍然有更丰富、带 hook 的 app-state 运行时。

## 流程图与决策地图

### 如何阅读这张图

- 先读蓝色主路径，用它理解共享的 happy path。
- 再看黄色菱形，它们是决定工具走哪条分支的判断条件。
- 最后看红色节点，它们标出 Go 与 `../src` 真正分叉的位置，或被有意排除的范围。

```mermaid
flowchart TD
    A(["开始"])
    B["解析任务字段"]
    C["解析任务列表作用域"]
    D["创建任务记录"]
    E["持久化任务"]
    F(["返回已创建任务"])
    X1["原版只会在 task-v2 打开时暴露这个工具"]
    X2["原版还会运行 task-created hooks，并做更丰富的 UI 展开"]
    A --> B
    B --> C
    C --> D
    D --> E
    E --> F
    B -.-> X1
    D -.-> X2
    classDef start fill:#e8f4fd,stroke:#1d4ed8,color:#0f172a;
    classDef step fill:#eff6ff,stroke:#60a5fa,color:#0f172a;
    classDef decision fill:#fef3c7,stroke:#d97706,color:#7c2d12;
    classDef gap fill:#fee2e2,stroke:#dc2626,color:#7f1d1d;
    classDef result fill:#dcfce7,stroke:#16a34a,color:#14532d;
    class A start;
    class B,C,D,E step;
    class X1,X2 gap;
    class F result;
```

### 决策点

- 这个工具最重要的隐含决策是作用域解析：新任务到底归属于当前 session / team 上下文中的哪一个任务列表。

### 差异热点

- 共享 happy path 就是创建并持久化。
- 原版会在这条路径外包上 task-v2 gate、hook 执行，以及更丰富的 UI / runtime 副作用，而 Go 还没有镜像这些行为。


## 输出与格式

- 输出对比: 原版返回结构化任务结果；Go 返回基于新持久化任务底座的纯文本创建摘要。

## 关键差异

- 剩余差距主要集中在 hook/app-state 集成、更丰富的 typed 结果，以及更宽的原版 teammate/runtime 生命周期。
