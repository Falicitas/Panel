# GitNexus 上手 Checklist

从零到能用 CLI + MCP 给 Claude Code 注入代码库的结构化上下文。

前置：Node.js 18+，一个你熟悉的中等规模 repo（方便验证结果对不对）。

---

## Phase 0 — Web UI 快速体验 (~10min)

先不装 CLI，用浏览器版感受一下。

- [ ] 打开 gitnexus.vercel.app
- [ ] 拖一个 ZIP 进去（或填 GitHub repo URL）— 建议用你自己的项目，比如桃源天下或 LLM Gateway
- [ ] 等 indexing 完成，看知识图谱的可视化
- [ ] 点几个节点，看 file → function → dependency 的关系链
- [ ] 试一下内置的 Graph RAG Agent — 问它"这个项目的入口在哪"之类的问题
  - 🔍 Web UI 跑在浏览器里（Tree-sitter WASM + LadybugDB WASM），大 repo 会吃内存。CLI 版本没这个限制

---

## Phase 1 — CLI 安装 + 本地 indexing (~15min)

- [ ] 安装 — `npm install -g gitnexus`
- [ ] cd 到你的项目目录，执行 `gitnexus analyze`
  - index 存在 `.gitnexus/` 目录下（已 gitignore）
  - 全局 registry 在 `~/.gitnexus/`
- [ ] `gitnexus status` — 看 index 状态：文件数、节点数、边数
- [ ] `gitnexus list` — 确认 repo 出现在已索引列表里

---

## Phase 2 — MCP 集成 Claude Code (~20min)

这是 GitNexus 的实用价值所在。

- [ ] `gitnexus setup` — 自动配置 MCP server 到你的编辑器（Claude Code / Cursor）
- [ ] `gitnexus mcp` — 启动 MCP server（stdio 模式）
- [ ] 在 Claude Code 里测试 — 问一个关于你项目架构的问题，看看返回的上下文是否包含了 call chain 和 dependency 信息
- [ ] 对比测试 — 同样的问题，关掉 GitNexus MCP 再问一次，看回答质量差异
  - 🔍 Claude Code 有最深的集成：MCP tools + agent skills + PreToolUse hooks（搜索时注入图谱上下文）+ PostToolUse hooks（commit 后自动重建索引）

---

## Phase 3 — 进阶功能 (~15min)

- [ ] `gitnexus analyze --skills` — 从检测到的 communities 生成 repo-specific skill files
- [ ] `gitnexus wiki` — 从知识图谱生成 wiki 文档，看看质量怎么样
- [ ] `gitnexus serve` — 启动 HTTP server，然后在浏览器打开 web UI 连接本地 backend（比直接拖 ZIP 更快，支持多 repo）
- [ ] `gitnexus analyze --force` — 强制全量重建索引，对比增量 vs 全量耗时
- [ ] 💡 `gitnexus analyze --embeddings` — 开启 embedding 生成（更慢但搜索质量更好）

---

## Phase 4 — 在 LLM Gateway 项目上实测 (~15min)

- [ ] 在 LLM Gateway monorepo 根目录跑 `gitnexus analyze`
- [ ] 用 Claude Code + GitNexus MCP 做一次真实开发任务 — 比如加一个新的 provider adapter
- [ ] 评估：Claude Code 是否减少了"漏掉依赖"或"改了 A 没改 B"的情况？
- [ ] 🤖 让 Claude Code 自己用 GitNexus 的 Cypher query 工具查一下 "哪些文件 import 了 router 模块"

---

## 品味校准 (~5min)

- [ ] 判断：GitNexus 会成为 AI coding 的标配 MCP 还是会被 IDE 原生功能替代？
- [ ] 它解决的问题是真痛点吗？纯 vector search 的 RAG 够用吗，还是 graph RAG 确实有质的提升？
- [ ] 核心设计决策：用知识图谱（Cypher query）而不是纯 embedding 来表示代码结构。这带来了什么好处和限制？
- [ ] DX 打分（1-5）：安装摩擦、indexing 速度、MCP 集成流畅度、输出质量
- [ ] 一句话总结：推荐还是劝退？
- [ ] 它能替代你现有工具链的哪一环？

---
---