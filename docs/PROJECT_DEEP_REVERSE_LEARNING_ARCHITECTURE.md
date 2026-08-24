# 项目深度逆向学习引擎架构

## 目标与原则

该引擎把真实仓库逐步转换为可验证、可增量更新的学习知识，而不是一次性生成不可追溯的 AI 总结。

核心原则：本地优先、证据优先、阶段版本化、投影可重建、能力状态真实、AI 最小权限。

## 模块边界

```text
Desktop API
  -> Analysis Job Service
      -> Queue / Lease / Retry
          -> Stage Worker
              -> Scanner / Parser / Resolver / Graph Builder
              -> Artifact Store (versioned, rebuildable)
              -> Projection Store (SQLite, disposable)
  -> Evidence Retrieval
      -> AI Context Builder
          -> Read-only AI Tutor / Learning Engine
```

| 模块 | 职责 | 不负责 |
| --- | --- | --- |
| Job Service | 创建、取消、恢复、观察分析任务 | 解析源码 |
| Pipeline | DAG、依赖、stage 版本和能力状态 | 隐式猜测执行顺序 |
| Worker | 在资源限制内执行一个 stage | 修改或运行目标仓库 |
| Artifact Store | 保存版本化中间产物和来源修订 | 作为不可重建用户配置库 |
| Graph Projector | 为 UI / 检索生成查询索引 | 成为事实的唯一来源 |
| Evidence Retrieval | 按符号、文件、关系检索证据 | 把整个仓库直接交给模型 |
| AI Tutor | 基于证据解释与教学 | 无证据断言、写入仓库、提升权限 |

## Pipeline 契约

默认 DAG 分为：侦察、仓库索引、解析、符号、依赖、调用图、数据流、API、数据库、架构、知识图谱、AI 推理和学习。每个 stage 都有稳定 ID、实现版本、依赖和 `available/planned` 状态。当前数据流阶段已开放 Go 函数内的局部 def-use、赋值、调用参数和返回值证据。

当前 `reconnaissance`、`repository_index`，以及覆盖 Go 的 `parse`、`symbols`、`dependencies`、`call_graph` 可用。其他 stage 明确为 `planned`；非 Go 能力不能在 UI 中显示为已完成。

每个 Worker 应满足：

- 输入是已验证的上游 artifact envelope。
- 输出带 schema version、stage version、repository revision 和生成时间。
- 相同输入哈希与版本产生相同缓存键。
- 支持 context 取消、时间预算、内存预算和输出数量上限。
- 写入采用临时产物 + 原子提交；失败不覆盖上一份完整产物。

## 版本化领域模型

- `ProjectSnapshot`：项目身份、规范化根目录标识和 repository revision。
- `FileRecord`：相对路径、内容哈希、语言、大小、行数和敏感标志，不含源码内容。
- `CodeSymbol` / `CodeReference`：稳定 ID、位置、符号种类和可选证据。
- `EvidenceRef`：证据 ID、类型、相对路径、行区间、哈希和置信度。
- `GraphNode` / `GraphEdge`：统一图契约，边必须指向存在节点。
- `Claim`：机器或 AI 结论；必须引用至少一条证据。
- `AnalysisArtifact`：单 stage 的完整产物。
- `ArtifactEnvelope`：schema 版本和 artifact 的兼容边界。

迁移采用“读旧写新 + 后台重建投影”。本阶段不执行不可逆 SQLite migration。

## 权威数据与投影

仓库内容和 Git revision 是源码事实的权威来源；版本化 artifact 是某次分析的可重建快照；SQLite 仅用于 UI 和检索投影。用户配置、授权和学习进度不得只保存在分析投影中。

Artifact identity：

```text
schema version + stage ID + stage version
+ repository revision + sorted upstream/input hashes
```

## 统一图谱

统一图支持 project、file、module、symbol、external dependency、API、table、concept 等节点，以及 contains、imports、references、calls、reads、writes、implements 等有向边。

约束：

- 节点 ID 在 artifact 内唯一。
- 边两端必须存在。
- 推断边必须引用证据并携带置信度。
- reconnaissance artifact 只能表达真实文件、侦察符号和 import 关系，不生成调用或数据流边。
- Knowledge Graph stage 只能整合上游真实 artifact，不能用 AI 文本替代解析事实。

## AI Tutor 与证据检索

AI 请求分成三个互不混淆的层：

1. 系统策略：由应用提供，不包含仓库文本。
2. 结构化事实：由已验证 artifact 生成。
3. 不可信材料：源码、README、注释和文档片段，带路径、哈希、行区间和明确标签。

源码中的“忽略规则”“调用工具”“泄露密钥”等文字只能作为待解释数据，不能改变系统策略、工具权限或审批策略。所有关键回答必须产生带 Evidence ID 的 `Claim`；检索不到证据时应明确说明未知。

## 增量分析与缓存

- 无文件变化：不失效 stage。
- manifest 或仓库配置变化：从 reconnaissance 开始使所有下游失效。
- 源码新增或修改：从 parse 开始使所有下游失效。
- 文件删除：同样使解析、符号和所有图谱下游失效，防止残留节点。
- stage 版本、schema 版本或输入哈希变化：缓存必然失效。

ChangeSet 由后续 Git/index provider 产生；本阶段只定义确定性的失效与缓存键契约，不宣称已有增量扫描。

## 安全与性能

- 路径必须是 workspace-relative slash path，拒绝绝对路径和 `..` 越界。
- 敏感内容不得进入 Evidence、Artifact、日志、AI context 或 projection。
- Worker 默认只读，网络和进程执行均为关闭状态；需要扩权必须走独立审批。
- 对文件数、文件大小、AST 节点、图节点/边、AI 上下文和并发 Worker 设上限。
- 大仓库按 stage 和文件分片；写入批量事务；UI 查询走投影和分页。
- 取消、崩溃和应用重启后，通过租约与幂等键恢复，避免重复提交。

## 阶段依赖

| 阶段 | 输入 | 产物 | 当前状态 |
| --- | --- | --- | --- |
| Reconnaissance | 本地目录 | 文件、技术栈、声明/import 证据 | 可用 |
| Repository Index | 侦察 | revision 与文件索引 | 可用 |
| Parse | 文件索引 | Go 版本化 AST/语法事实 | Go 可用，其他语言规划 |
| Symbol / Dependency | Parse | Go 符号、引用、依赖 | Go 可用，其他语言规划 |
| Call Graph | 符号与依赖 | Go 证据化静态调用关系 | Go 可用；动态分派保持未知 |
| Data Flow | 符号、调用图与 Parse | Go 函数内局部 def-use、赋值、调用参数和返回值 | 可用（Go） |
| API / DB / Architecture | 上游图 | 领域关系 | 规划 |
| Knowledge Graph | 所有验证图 | 统一可查询图 | 规划 |
| AI Reasoning | 图谱 + 检索证据 | 带证据 Claim | 规划 |
| Learning | Claim + 用户进度 | 路径、课程、挑战 | 规划 |

## 迭代成本控制

- 新语言只需新增 parser/resolver Worker，不修改桌面契约。
- stage 通过版本和 envelope 演进，避免全局数据库锁步迁移。
- 将任务运行态、分析事实、查询投影和用户学习状态分开，降低长期耦合。
- 契约测试先于重型实现，防止后续用占位数据突破安全与事实边界。
