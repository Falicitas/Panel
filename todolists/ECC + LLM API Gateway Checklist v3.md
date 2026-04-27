# ECC + LLM API Gateway Checklist v3

从零开始配 Everything Claude Code，然后用它跑 LLM API Gateway 项目。
前提：Mac、Go 已装、Claude Code v2.1.88、Pro 订阅。

几个容易混的点先说清楚：
- ECC 有两套东西可以调：commands/（60 个）和 skills/（136 个），内容有重叠
- Plugin 装的 commands 带命名空间：`/everything-claude-code:plan`，跟 `/plan` 不是一回事
- `/plan`（不带命名空间）是 Claude Code 自带的 plan mode
- Skills 用短名调：`/tdd-workflow`、`/golang-patterns`，输入 `/` 后按字母补全
- Rules 必须手动装，plugin 系统不支持分发
- Hooks 从 ECC 仓库手动合并到 settings.json

---

## Phase 0 — 确认当前状态 (~3min)

之前折腾了一轮，先看现在什么情况。

- [x] Plugin 在位 — `/plugin list everything-claude-code@everything-claude-code`，已确认 ✔
- [x] Commands 在位 — `ls ~/.claude/commands/ | wc -l`，应该 60+
- [x] Skills 在位 — `ls ~/.claude/skills/ | wc -l`，应该 130+
- [x] Agents 在位 — `ls ~/.claude/agents/ | wc -l`，应该 28+
- [x] MCP 在位 — plugin 自带 github、context7、exa、memory、playwright、sequential-thinking，已确认 ✔
- [x] 验证命名空间调用 — `/everything-claude-code:plan` 已确认能跑 ✔

---

## Phase 1 — 手动安装 Rules (~5min)

Plugin 不能分发 rules，手动装。

- [x] 确认本地有 ECC 仓库 — `ls ~/Desktop/everything-claude-code/rules/`
  - 没有的话：`git clone https://github.com/affaan-m/everything-claude-code.git ~/Desktop/everything-claude-code`
- [x] 用 install.sh 安装 —

```bash
cd ~/Desktop/everything-claude-code && npm install && ./install.sh golang typescript
```

  - install.sh 报错就手动：`mkdir -p ~/.claude/rules && cp -r rules/common rules/golang rules/typescript ~/.claude/rules/`
- [x] 验证 — `ls ~/.claude/rules/`，应该看到 `common/`、`golang/`、`typescript/` 三个目录
- [x] 看一眼内容 — `cat ~/.claude/rules/golang/coding-style.md | head -30`

---

## Phase 2 — 配置 Hooks (~10min)

ECC 自带 hooks 在 `hooks/hooks.json`。之前 inline one-liner 出过问题，这次用外部脚本文件。

- [x] 看 ECC 自带了哪些 hooks — `cat ~/Desktop/everything-claude-code/hooks/hooks.json | head -80`
- [x] 清理 settings.json 里之前的坏 hooks — 让 Claude Code 删掉 settings.json 里整个 `"hooks"` 字段
- [x] 让 Claude Code 重新配 hooks — prompt：

> 查看 ~/Desktop/everything-claude-code/hooks/hooks.json 的内容。把合理的 hooks 合并到 ~/.claude/settings.json。要求：
> 1. hook command 全部用外部脚本文件（放 ~/.claude/hooks/），不要 inline one-liner
> 2. 脚本用 python3 从 stdin 读 JSON（jq 路径不在 hook 环境的 PATH 里）
> 3. PostToolUse hook 用 exit 0 + stdout 输出 feedback 给 Claude
> 4. PreToolUse hook 用 exit 2 + stderr 阻断操作
> 5. 额外加两个自定义 hook：PostToolUse 对 .go 文件检测 fmt.Println；PreToolUse 保护 go.mod、go.sum、yarn.lock
> 6. 每个脚本加文件日志（写 /tmp/hook_xxx.log）方便排查

- [x] 手动验证脚本 — 对每个脚本跑 `echo '{"tool_input":{"file_path":"test.go"}}' | python3 ~/.claude/hooks/xxx.py`，看输出对不对
- [x] 加 memory persistence hooks — ECC 自带 session 生命周期 hooks（longform guide 里讲的），让 Claude Code 配：

> 查看 ~/Desktop/everything-claude-code/hooks/memory-persistence/ 目录。把 SessionStart、PreCompact、Stop 三个 hook 加到 settings.json。这三个 hook 的作用：
> - SessionStart：新 session 启动时加载最近 7 天的 session 文件
> - PreCompact：compact 前保存当前状态到 session 文件
> - Stop：session 结束时持久化到 ~/.claude/sessions/YYYY-MM-DD-topic.tmp
> 同样用外部脚本文件，不要 inline。

  - 🔍 这套 hooks 让跨 session 的上下文能续上。没有它，每次新开 session 都要重新说一遍背景。参考 longform guide "Context & Memory Management" 章节
- [x] 重启 Claude Code 让 hooks 生效

---

## Phase 3 — 感受 ECC 的三层结构 (~15min)

搞清楚 commands、skills、agents、rules 各是什么，怎么调。

### Commands vs Skills

ECC 同时装了 commands/（60 个）和 skills/（136 个），功能有重叠。

- [x] 对比同一功能的两种调用方式：
  1. `/everything-claude-code:plan 实现 API key 验证中间件` — 这是 commands/ 里的 plan.md
  2. 输入 `/` 打 `plan`，看 skills 补全列表里有没有 plan 相关的 skill
  - 🔍 Commands 通过命名空间前缀调用，是 prompt 模板；Skills 是 SKILL.md 格式，可能支持更多 frontmatter 功能（auto-invocation、allowed-tools 等）
- [x] 看 command 原文 — `cat ~/.claude/commands/plan.md`
- [ ] 看对应 skill 原文 — `cat ~/.claude/skills/blueprint/SKILL.md`（或补全出来的实际名字）
- [ ] 用下来哪个顺手用哪个

### Agents

- [ ] 看 agent 列表 — `ls ~/.claude/agents/`
- [ ] 看一个 agent 原文 — `cat ~/.claude/agents/go-reviewer.md`，注意 frontmatter 里的 tools 和 model
- [ ] 试 delegate — "delegate to go-reviewer to review backend/cmd/server/main.go"
  - 🔍 子 agent 有独立 context window，不占主 session。审查、文档更新这类任务适合 delegate 出去

### Rules

- [ ] 让 Claude Code 写一个 Go HTTP handler — "写一个 Go HTTP handler，GET /health 返回 200"
- [ ] 看代码风格 — 有 rules 时应该遵循 error wrapping、context propagation、不 panic 等约定
- [ ] 💡 可选：临时 `mv ~/.claude/rules/golang ~/.claude/rules/golang.bak`，重启 Claude Code，再写同样的 handler 对比差异，然后改回来

---

## Phase 4 — 用 ECC 跑 Gateway 开发 (~1h)

在 `llm-gateway/` 根目录跑 Claude Code。

### Backend 侧

- [ ] ECC plan — `/everything-claude-code:plan 实现 backend/ 的 API key 验证中间件 + 请求转发到 DeepSeek chat/completions 端点，支持 streaming SSE`
  - 看 planner agent 生成的 blueprint 质量（requirements restatement、phases、risks、complexity 估算）
  - 确认后再让 Claude Code 写代码
- [ ] TDD — `/everything-claude-code:tdd`（commands 版）或 `/tdd-workflow`（skills 版），对 backend/internal/auth/ 写测试
- [ ] Go 专属 — `/everything-claude-code:go-test`（commands）或 `/golang-testing`（skills），对比哪个更合适
- [ ] 代码审查 — "delegate to go-reviewer to review backend/internal/proxy/"
- [ ] 也试 command 版 — `/everything-claude-code:code-review backend/internal/proxy/`，跟 delegate agent 的审查结果比一下

### Dashboard 侧

- [ ] ECC plan — `/everything-claude-code:plan 实现 dashboard/ 的登录页和 API key 管理页，用 shadcn/ui 组件`
- [ ] 写完后审查 — "delegate to typescript-reviewer to review dashboard/src/"

### Landing 侧（后做）

- [ ] ECC plan — `/everything-claude-code:plan 实现 landing/ 首页：hero + features + pricing。Lenis 平滑滚动，clip-path 不规则形状`

---

## Phase 5 — Hooks 验证 (~10min)

- [ ] fmt.Println 检测 — 让 Claude Code 在 .go 文件写 `fmt.Println("debug")`，看回复里是否提到 hook feedback
  - 🔍 PostToolUse exit 0 的 stdout 是传给 Claude 的 feedback，Claude 会在回复中提到，不是 UI 弹窗
- [ ] lockfile 保护 — 让 Claude Code 改 go.mod，看 PreToolUse exit 2 是否阻断了
- [ ] console.log 检测 — 让 Claude Code 在 .ts 文件写 `console.log`，看有没有对应 hook
- [ ] secret 检测 — 让 Claude Code 把 `sk-test123456` 写到代码里，看是否被拦

---

## Phase 6 — 工作流固化（项目跑起来之后）

### 基础

- [ ] 迭代 CLAUDE.md — 加 codemap、已验证的架构决策、踩过的坑
- [ ] 自定义 agent — 写 `~/.claude/agents/api-compat-checker.md`（检查 OpenAI schema 兼容性）
- [ ] `/fork` 并行 — backend 和 frontend 问题分开讨论
- [ ] Git worktree — `git worktree add ../gateway-dashboard dashboard`，并行开发

### Session Memory（longform guide 里的东西）

- [ ] 确认 session 文件在生成 — 跑完一个 session 后 `ls ~/.claude/sessions/`，应该有 `.tmp` 文件
- [ ] 看 session 文件内容 — `cat ~/.claude/sessions/最新的文件.tmp`，应包含：完成了什么、哪些方法有效、哪些没效、下一步做什么
- [ ] 新 session 验证加载 — 开新 session，看 Claude 是否自动提到上次的进展
- [ ] 💡 手动兜底 — 自动 hook 没生效的话，session 结束前手动说 "总结当前进展，保存到 ~/.claude/sessions/gateway-YYYY-MM-DD.tmp"

### Token 优化（longform guide）

- [ ] Context window 管理 — `/mcp` 检查启用的 MCP 数量，保持 <10 个、<80 个 tools
- [ ] 定期 compact — 研究完切到实现前跑 `/compact`，别等自动 compact
  - 🔍 compact 时机：探索完 → 开始实现前；完成一个 milestone → 开始下一个前；debug 完 → 继续 feature 前。实现过程中别 compact，会丢变量名、文件路径等中间状态
- [ ] 考虑 CLI 替代 MCP — 不常用的 MCP（如 playwright）可以 disable 掉，用 `npx playwright test` 通过 `!` 命令直接跑，省 context window
- [ ] Subagent model 分级 — agent frontmatter 里设 `model: haiku` 给简单任务（文件搜索、格式检查），`model: sonnet` 给主力任务，`model: opus` 只给架构决策
  - 加到 settings.json：`"env": { "CLAUDE_CODE_SUBAGENT_MODEL": "haiku" }`

### Continuous Learning（longform guide）

- [ ] 试 /learn — 解决了一个有点复杂的问题后，跑 `/everything-claude-code:learn`，让 Claude 提取 pattern 保存为 skill
- [ ] 看学到了什么 — `/everything-claude-code:instinct-status`，看已提取的 patterns
- [ ] 💡 Stop hook 自动学习 — ECC 的 `evaluate-session.sh` 可以在 session 结束时自动提取 patterns，进阶配置，先手动 `/learn` 试试

### 安全（security guide）

- [ ] 跑一次 AgentShield 扫描 — `npx ecc-agentshield scan`，检查 CLAUDE.md、settings.json、hooks、MCP 配置有没有问题
- [ ] 确认 --dangerously-skip-permissions 没开 — 有真实数据的项目里不要用这个
- [ ] 审查 MCP 权限 — 6 个 MCP 里 playwright 和 exa 有网络访问能力，确认你需要它们；不需要就 disable

---

## Claude Code 快捷键

- [ ] Shift+Tab — 循环切换 normal → auto-accept → plan mode
- [ ] Tab — 切换 thinking 显示（sticky across sessions）
- [ ] Ctrl+O — verbose 模式，看完整思考和 tool call
- [ ] Ctrl+Z — 挂起到后台，`fg` 恢复
- [ ] Ctrl+V — 粘贴截图
- [ ] Option+P — 切换模型（Sonnet ↔ Opus）
- [ ] `!` — 跑 bash，如 `! go version`
- [ ] `@` — 引用文件，如 `@backend/internal/proxy/`
- [ ] `/btw` — 问不相关问题不打断上下文
- [ ] `&` — 后台执行，如 `& go build ./...`
- [ ] `/compact` — 手动 compact，在逻辑断点处用
- [ ] `/cost` — 查看当前 session token 消耗
- [ ] `/rewind` — 回退到之前的状态（代码 + 对话都回退）

---

## 认知备忘

- ECC = ~/.claude/ 下的 .md 文件集合 — commands/（prompt 模板，命名空间调用）、skills/（SKILL.md 格式，短名调用）、agents/（subagent system prompt）、rules/（自动加载的编码约束）
- 两套命令并存 — `/everything-claude-code:plan`（ECC command）和 `/plan`（原生 plan mode）是不同的东西
- Skills 比 commands 多 — 136 vs 60，skills 是 ECC 的主力，很多功能只有 skill 版没有 command 版
- Rules 手动装 — `./install.sh golang typescript`，plugin 不能分发
- Hooks 用外部脚本 — 不要 inline one-liner，从 stdin 读 JSON，用 python3 不要 jq（PATH 问题）
- PostToolUse feedback 传给 Claude（exit 0 + stdout），不是 UI 弹窗
- PreToolUse 阻断用 exit 2 + stderr
- Context window 是有限的 — MCP <10、tools <80，研究完就 `/compact`
- Compact 时机：探索完→实现前、milestone 完→下一个前、debug 完→继续前。实现中途别 compact
- Session memory 靠 hooks — SessionStart 加载上次上下文，Stop 保存本次成果，PreCompact 在 compact 前保存状态。文件格式 `~/.claude/sessions/YYYY-MM-DD-topic.tmp`
- Subagent 省 context — delegate 给子 agent 不占主 session，审查/文档类任务优先 delegate
- Token 优化：Sonnet 做 90% 的活（省 60%），Haiku 跑子 agent，`/compact` 在逻辑断点手动触发
- 安全：`npx ecc-agentshield scan` 定期跑，不用 `--dangerously-skip-permissions`，不需要的 MCP 就 disable
- 三个子项目顺序 — backend 先 → dashboard 其次 → landing 最后

---

## 原文参考

要深入某个主题时查：

- Shorthand Guide（setup 基础）— 覆盖 skills/commands/hooks/agents/MCPs/plugins 的配置和用法
- Longform Guide（进阶优化）— token 优化策略、session memory persistence hooks、compact 时机、continuous learning（/learn 自动提取 patterns）、verification loops/evals、parallelization（git worktree + /fork + cascade method）、subagent orchestration（iterative retrieval pattern）
- Security Guide（安全）— Claude Code CVEs（CVE-2025-59536, CVE-2026-21852）、MCP 攻击面、prompt injection 通过 PDF/PR/email 进入 context、sandboxing（Docker + network isolation）、AgentShield 扫描、kill switch 和 heartbeat 模式