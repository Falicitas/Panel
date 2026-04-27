# qmd 上手 Checklist

从安装到把个人笔记/文档变成可搜索的本地知识库，接入 agent 工作流。

前置：Node.js 18+ 或 Bun。有一些 markdown 文件（笔记、文档、会议记录之类的）。

---

## Phase 0 — 安装 + 建第一个 collection (~10min)

- [ ] 安装 — `npm install -g @tobilu/qmd`
- [ ] 创建 collection — `qmd collection add ~/notes --name notes --mask "**/*.md"`
- [ ] 再加一个 — `qmd collection add ~/Documents/work --name work`
- [ ] `qmd collection list` — 确认 collections 创建成功，看文件数
- [ ] `qmd status` — 看 index 状态

---

## Phase 1 — BM25 关键词搜索 (~10min)

不需要 embedding，装完就能搜。

- [ ] `qmd search "你知道的某个关键词"` — 看返回结果的格式（score、file path、snippet）
- [ ] `qmd search "关键词" -c notes` — 限定在某个 collection 搜
- [ ] `qmd search "关键词" --json -n 10` — JSON 输出，给 LLM 用的格式
- [ ] `qmd get "path/to/file.md"` — 用搜索结果里的 path 获取完整文档
- [ ] `qmd get "#abc123"` — 用 docid 获取（docid 在搜索结果里有）

---

## Phase 2 — Embedding + 语义搜索 (~15min)

- [ ] `qmd embed` — 生成 vector embeddings（首次会下载 GGUF 模型到 `~/.cache/qmd/models/`）
  - 🔍 默认用 embeddinggemma-300M。你做中文内容比较多，考虑换模型：`export QMD_EMBED_MODEL="hf:Qwen/Qwen3-Embedding-0.6B-GGUF/Qwen3-Embedding-0.6B-Q8_0.gguf"` 然后 `qmd embed -f` 重建
- [ ] `qmd vsearch "语义化的问题"` — 纯 vector 搜索，对比 BM25 结果差异
- [ ] `qmd query "自然语言问题"` — hybrid 模式（BM25 + vector + reranking），这是推荐的搜索方式
- [ ] 对比三种搜索的结果质量 — search vs vsearch vs query，感受 reranking 的作用

---

## Phase 3 — Context 系统 (~10min)

Context 让搜索结果附带路径级别的说明，LLM 能更好理解文档含义。

- [ ] `qmd context add qmd://notes "个人笔记和想法"`
- [ ] `qmd context add qmd://work "工作文档"`
- [ ] `qmd context check` — 检查哪些 collection 缺 context
- [ ] 再搜一次，看返回结果里是否带上了 context 信息

---

## Phase 4 — MCP Server 接入 agent (~15min)

- [ ] 启动 MCP — `qmd mcp --http --daemon`（默认 8181 端口）
- [ ] 验证 — `curl -X POST http://localhost:8181/query -H "Content-Type: application/json" -d '{"searches": [{"type": "lex", "query": "test"}]}'`
- [ ] 配置到 Claude Code — 在 MCP 配置里加上 qmd 的 HTTP endpoint
- [ ] 实测：在 Claude Code 里问"我之前写的关于 XX 的笔记说了什么"，看它是否能通过 qmd 找到
- [ ] `qmd mcp stop` — 用完记得关

---

## Phase 5 — AST-aware chunking + 代码搜索 (~10min)

如果你的 collection 里有代码文件。

- [ ] `qmd embed --chunk-strategy auto` — 开启 AST-aware chunking（Tree-sitter 按函数/类/import 边界切分）
- [ ] 搜一下代码相关的内容 — `qmd query "auth flow" --chunk-strategy auto`
- [ ] 💡 对比 auto vs 默认 chunking 的搜索结果差异

---

## 品味校准 (~5min)

- [ ] 判断：qmd 会成为本地知识搜索的事实标准还是会被更大的产品吃掉？（作者是 Shopify 创始人 Tobi Lütke）
- [ ] 它解决的问题是真痛点吗？Obsidian search、ripgrep、Alfred 已经存在，qmd 的差异在哪？
- [ ] 核心设计决策：BM25 + vector + reranking 三级混合，全部本地 GGUF 模型。跟云端 embedding API 比，这个 tradeoff 你接受吗？
- [ ] DX 打分（1-5）：安装摩擦、indexing 速度、搜索质量、MCP 集成
- [ ] 一句话总结：推荐还是劝退？
- [ ] 能替代你现有工具链的哪一环？跟 OpenClaw 的 memory 系统有什么互补？

---
---