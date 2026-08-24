# Reasonix 当前系统架构

## 目的

本文记录项目深度逆向学习引擎接入前的真实系统边界。它是后续架构演进的基线，不把规划能力描述成已实现能力。

## 当前技术栈

- 桌面壳：Wails v2，Go 后端通过绑定接口服务 React 前端。
- 前端：React、TypeScript、Vite，使用轮询读取分析任务状态。
- 本地分析：`internal/projectanalysis`，只读遍历工作区并生成侦察结果。
- 本地投影：`internal/projectiondb` + SQLite，数据可删除、可由项目重新分析生成。
- 任务基础设施：`internal/taskmonitor` 提供版本快照、CAS、审计、幂等和运行租约，但项目分析任务尚未接入。
- 解析基础：Go 官方 AST 已接入版本化 Parse Pipeline；仓库包含可选 Tree-sitter 及 JavaScript、TypeScript、Python、Rust grammar，但默认构建尚未启用这些语言。

## 当前真实数据流

```text
本地项目目录
  -> 路径与安全预检
  -> 文件遍历（上限 30,000，单文件上限 2 MB）
  -> 敏感文件名隔离（不读取内容）
  -> manifest / 技术栈侦察
  -> Go AST + 其他语言正则声明侦察
  -> import 依赖侦察
  -> Result + Evidence
  -> Repository Index -> Go Parse Artifact
  -> Go Symbol / Dependency -> Call Graph -> Function-local Data Flow
  -> SQLite 可重建投影
  -> Wails API
  -> React 项目学习工作台
```

## 已实现能力

- 本地只读扫描、取消和进度上报。
- 文件、语言、manifest、技术栈、声明和 import 级依赖侦察。
- 证据来源文件、行号和置信度。
- `.env`、私钥、证书和凭据类文件隔离。
- 跳过依赖、构建产物、版本控制目录。
- 结果投影缓存与桌面端可视化。
- 版本化仓库索引、Go 正式 AST、跨文件符号/依赖、证据化静态调用图、函数内数据流和桌面摘要。

## 尚未实现

- TypeScript、JavaScript、Python、Rust 等语言的正式 AST、统一符号解析和调用图。
- Go 接收者方法调用的完整解析、动态分派、跨函数数据流、API 与数据库模型，以及非 Go 语言的数据流模型。
- 架构推断、项目知识图谱、增量 Git 分析。
- 基于证据的 AI Tutor、课程、挑战和实验室。
- 可恢复的持久化 Analysis Job / Worker Queue。

## 可复用模块

| 模块 | 用途 | 约束 |
| --- | --- | --- |
| `projectanalysis.Analyze` | 侦察阶段适配器 | 保留现有返回契约，不升级为“正式 parser” |
| `projectiondb` | 查询投影和大体积可重建索引 | 不是用户业务数据的唯一权威来源 |
| `taskmonitor` | 后续 Job 快照、CAS、租约、审计 | 通过适配层接入，不直接耦合分析领域模型 |
| Tree-sitter 依赖 | 后续多语言 parser | 每种 grammar 需独立测试、版本和资源限制 |
| `agent` 只读子代理 | 后续 AI Reasoning | 只能消费经过策略过滤的证据包 |

## 不应修改的边界

- 扫描源码、README 和注释都是不可信输入，不能进入系统提示或改变工具权限。
- 扫描阶段不运行项目脚本、Hook、安装、构建、测试或仓库内 MCP。
- 敏感文件只记录存在性，不读取或缓存内容。
- `internal/memory` 不是项目知识图谱存储。
- SQLite 投影丢失不能导致用户权威状态丢失。
- 现有第一阶段 `Result` 是桌面 UI 的兼容边界，后续以适配器生成版本化 Artifact。

## 插入点

1. `Analyze` 结果通过 reconnaissance adapter 转成版本化 `AnalysisArtifact`。
2. Pipeline 调度器按 stage DAG 派发有版本的 Worker。
3. Worker 只读取上游 artifact，通过 envelope 写入可重建 artifact store。
4. Graph projector 将经过验证的节点和边写入查询投影。
5. AI context builder 从图谱和源码证据中检索最小上下文，并保持系统策略与不可信内容结构隔离。
