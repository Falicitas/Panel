# OpenCode 上手配置 Checklist（Mac mini M2 / macOS）

从零装好 OpenCode，接上免费模型和云端付费模型，能在终端里用 AI 写代码。

前提：macOS，Homebrew 已装，有终端使用经验。不跑本地模型（16GB 内存不值得折腾）。

---

## Phase 0 — 安装 (~5min)

装 OpenCode CLI。

- [x] 装 OpenCode — `brew install anomalyco/tap/opencode`
  - 这个 tap 是 OpenCode 团队自己维护的，更新最快。官方 `brew install opencode` 由 Homebrew 团队维护，更新慢
  - 验证：`opencode --version`

---

## Phase 1 — 接免费模型跑通 (~10min)

用 OpenCode Zen 的免费模型验证链路。

- [x] 启动 OpenCode — `cd` 到任意一个有代码的项目目录，运行 `opencode`
- [ ] 连接 Zen — 在 TUI 里输入 `/connect`，选 OpenCode Zen，去 opencode.ai/auth 注册拿 API key，粘贴
- [ ] 选免费模型 — `/models` 查看可用模型，选一个标了 "Free" 的（目前有 Big Pickle、MiniMax M2.5 Free 等，轮换的）
  - 🔍 免费模型是模型厂商限时提供的，用你的使用数据换免费调用。质量参差不齐，适合试手
- [ ] 测试 — 随便问个问题，比如 "explain the project structure"，确认能出响应
- [ ] 🌟🌟🌟🌟 诚实的说预型 3 和 4 做了，最近在研究 hermes。

---

## Phase 2 — 接云端付费模型 (~10min)

免费模型质量不够日常用，接真正好用的模型。

- [ ] 方案 A：Zen 按量付费 — 在 Zen 控制台充 $20，按 token 计费，支持 GPT-5 Codex、Claude Sonnet 等
  - `/models` 里选对应模型就行
  - 🔍 Zen 会自动 $5 以下余额时充 $20，记得去控制台设 monthly limit 防意外扣费
- [ ] 方案 B：Go 订阅 — $5 首月 / $10 月，包含 GLM-5、Kimi K2.5、MiniMax M2.5 等开源模型
  - 适合量大但不需要顶级模型的场景
- [ ] 方案 C：直接接 provider — `/connect` 选 OpenAI / Google 等，粘自己的 API key
  - 🔍 Anthropic 在 2026 年 1 月封了 OpenCode 对 Claude 的直接调用。用 Zen 可以绕过，或者用 OpenRouter
- [ ] 💡 方案 D：接你自己的 LLM Gateway — 在 `~/.config/opencode/opencode.json` 里加 custom provider，baseURL 指向你的 gateway 服务
  ```json
  {
    "$schema": "https://opencode.ai/config.json",
    "provider": {
      "my-gateway": {
        "npm": "@ai-sdk/openai-compatible",
        "name": "My LLM Gateway",
        "options": {
          "baseURL": "https://your-gateway.com/v1"
        },
        "models": {
          "deepseek-v3": { "name": "DeepSeek V3" },
          "qwen-plus": { "name": "Qwen Plus" }
        }
      }
    }
  }
  ```
  - 这就是你的 gateway 项目的 dogfooding 场景

---

## Phase 3 — 给不同任务配不同模型 (~10min)

OpenCode 的 agent 系统支持给 plan 和 build 配不同模型。

- [ ] 理解 Plan 和 Build — Plan 模式只读分析不改文件，Build 模式能改文件跑命令。按 Tab 键切换
- [ ] 给 plan 配便宜模型，给 build 配好模型 — 在 `opencode.json` 里：
  ```json
  {
    "agent": {
      "plan": {
        "model": "opencode/kimi-k2.5"
      },
      "build": {
        "model": "opencode/gpt-5.3-codex"
      }
    }
  }
  ```
  - 🔍 这就是你说的 Resource Aware Optimization 的雏形：plan 用便宜快速的模型做分析，build 用贵但准的模型改代码
- [ ] 设默认模型 — 在 config 顶层加 `"model": "opencode/kimi-k2.5"` 作为全局 fallback

---

## Phase 4 — 项目初始化和 AGENTS.md (~10min)

让 OpenCode 理解你的项目。

- [ ] 在 gateway 项目目录跑 `/init` — OpenCode 会扫描项目结构，生成 AGENTS.md
  - AGENTS.md 会包含：build/test 命令、项目架构、代码规范
  - 这个文件会被注入到每次对话的 system prompt 里
- [ ] 检查生成的 AGENTS.md — 看看它理解得对不对，手动补充你的 Go backend 的编码规范、目录结构说明
- [ ] 提交到 git — `git add AGENTS.md && git commit -m "add AGENTS.md"`
- [ ] 在桃源天下项目也跑一遍 `/init`

---

## Phase 5 — Skills 和自定义命令 (~15min)

把你在 Claude Code 里积累的 skills 迁移过来。

- [ ] 创建全局 skills 目录 — `mkdir -p ~/.config/opencode/skills`
  - 🔍 OpenCode 的 skill 格式和 Claude Code 的 skill 格式兼容，都是 `skills/<name>/SKILL.md`，带 YAML frontmatter
- [ ] 迁移你的 de-ai-writing skill — 把 `~/.claude/skills/de-ai-writing/SKILL.md` 复制到 `~/.config/opencode/skills/de-ai-writing/SKILL.md`
- [ ] 迁移 tech-doc-style 和 learning-todolist skill — 同上
- [ ] 创建自定义命令 — `mkdir -p ~/.config/opencode/commands`，写一个 `test.md`：
  ```markdown
  ---
  description: Run tests and show failures
  ---
  Run the full test suite, show any failures, and suggest fixes.
  ```
  - 之后在 TUI 里 `/test` 就能触发
- [ ] 💡 试 `/init` 之外的 slash commands — `/undo`（撤销改动）、`/redo`、`/share`（分享 session 链接）

---

## Phase 6 — oh-my-opencode 插件 (~15min)

OpenCode 的增强插件，提供多 agent 编排、后台并行任务、LSP/AST 工具、自动 todo 追踪等功能。

- [ ] 安装 — 在 OpenCode TUI 里直接粘贴这段话让 agent 自己装：
  ```
  Install and configure oh-my-opencode by following the instructions here:
  https://raw.githubusercontent.com/code-yeongyu/oh-my-opencode/refs/heads/master/docs/guide/installation.md
  ```
  - 或者手动：`bunx oh-my-opencode install`（交互式）/ `npx oh-my-opencode install`
  - 安装时会问你有没有 Claude Pro/Max、ChatGPT、Gemini、GitHub Copilot 订阅，根据实际情况选
  - 你没有这些订阅的话全选 no：`bunx oh-my-opencode install --no-tui --claude=no --chatgpt=no --gemini=no --copilot=no`
- [ ] 验证安装 — `cat ~/.config/opencode/opencode.json`，确认 plugin 数组里有 `"oh-my-opencode"`
- [ ] 了解核心 agent 分工：
  - Sisyphus — 主 agent，负责写代码，推荐 Claude Opus 4.5（没订阅就用 Zen 模型替代）
  - Oracle — 架构设计和 debug（GPT 5.2）
  - Librarian — 查文档、搜开源实现（Claude Sonnet 4.5）
  - Explore — 快速 codebase grep（Grok Code，免费）
  - 🔍 这些 agent 自动协作：主 agent 写代码时把搜索丢给 Explore 后台跑，把 debug 丢给 Oracle，不用手动调度
- [ ] 试 ultrawork 模式 — prompt 里加 `ultrawork`（或 `ulw`），插件自动编排多 agent 并行完成任务
- [ ] 💡 自定义 agent 模型 — 在 `~/.config/opencode/oh-my-opencode.json` 里改：
  ```json
  {
    "agents": {
      "sisyphus": { "model": "opencode/gpt-5.3-codex" },
      "oracle": { "model": "opencode/kimi-k2.5" },
      "explore": { "model": "opencode/grok-code" }
    }
  }
  ```
  - 没有 Claude/GPT 订阅也能用，把模型指向 Zen 或你自己的 gateway
- [ ] 💡 内置 MCP — 装完自带 Exa（网页搜索）、Context7（官方文档）、Grep.app（GitHub 代码搜索），agent 自动调用

---

## Phase 7 — 日常工作流 (~10min)

跑几个实际任务验证体验。

- [ ] 试 Plan → Build 流程 — 在 gateway 项目里，Plan 模式下让它分析一个功能需求，确认方案后 Tab 切 Build 让它写代码
- [ ] 试 @ 引用文件 — 输入 `@` 然后模糊搜索项目里的文件，把文件拉进上下文
- [ ] 试 subagent — 输入 `@security-auditor review this code` 这种方式调用子 agent（如果配了的话）
- [ ] 💡 装 VS Code 插件 — 在 VS Code 扩展市场搜 OpenCode 装上，快捷键 Cmd+Esc 打开 OpenCode 面板
  - 插件和 TUI 共享配置和 session
- [ ] 💡 试 GitHub 集成 — 在 PR/Issue 评论里 `/opencode` 或 `/oc` 可以触发 OpenCode 在 GitHub Actions 里跑任务

---

## 品味校准 (~5min)

- [ ] 判断：OpenCode 会成为 Claude Code 的替代品还是会一直活在它的阴影下？
- [ ] OpenCode Zen 作为 API gateway 的商业模式可持续吗？跟 OpenRouter 比有什么独特优势？
- [ ] 设计决策：OpenCode 选了 "provider-agnostic + 开源 TUI" 路线，Claude Code 选了 "深度绑定 Anthropic 模型"。哪条路走得通？
- [ ] DX 打分（1-5）：安装摩擦、模型切换便利性、TUI 交互体验、agent/skill 生态
- [ ] 一句话总结：推荐/劝退别人从 Claude Code 切换到 OpenCode，你会怎么说？
- [ ] 追加问题：OpenCode 的 custom provider 配置对你的 gateway 项目来说是天然的分发渠道吗？用户在 opencode.json 里配你的 gateway 地址就能用，获客成本几乎为零