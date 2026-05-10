# everything on ClaudeCode

一个将 skills/command/hook 组织得更好的外挂 claude code 项目。

可以在一个单一 prompt 将 skills 和 commands 串起来。

![Image](https://pbs.twimg.com/media/G-0-_fZagAA9Kqk?format=jpg&name=900x900)

skills 的目录（其中一个 skill 例子）：~/.claude/skills/codemap-updater.md。也可以是一个含 README.md 的文件夹。

Commands 的目录：~/.claude/commands

## hook

1.  PreToolUse - Before a tool executes (validation, reminders)
2.  PostToolUse - After a tool finishes (formatting, feedback loops)
3.  UserPromptSubmit - When you send a message
4.  Stop - When Claude finishes responding
5.  PreCompact - Before context compaction
6.  Notification - Permission requests

Hook 的类别，以及配置代码：

```bash
{
  "PreToolUse": [
    {
      "matcher": "tool == \"Bash\" && tool_input.command matches \"(npm|pnpm|yarn|cargo|pytest)\"",
      "hooks": [
        {
          "type": "command",
          "command": "if [ -z \"$TMUX\" ]; then echo '[Hook] Consider tmux for session persistence' >&2; fi"
        }
      ]
    }
  ]
}
```

Pro tip: Use the `hookify` plugin to create hooks conversationally instead of writing JSON manually. Run /hookify and describe what you want.

连配置都不用写了吗，哈哈。

## 子 agent

`~/.claude/agents/`，这个目录下，一个 .md 就是一个子 agent。

Configure allowed tools, MCPs, and permissions per subagent for proper scoping.

核心目的：当一个主 agent 的上下文过多了，可以切割到子 agent 去使用。

```bash
# Example subagent structure
~/.claude/agents/
  planner.md           # Feature implementation planning
  architect.md         # System design decisions
  tdd-guide.md         # Test-driven development
  code-reviewer.md     # Quality/security review
  security-reviewer.md # Vulnerability analysis
  build-error-resolver.md
  e2e-runner.md
  refactor-cleaner.md
```



## rules

`~/.claude/rules/` 每次都会过一遍，比如说设置不要提交 console.logs。

```bash
~/.claude/rules/
  security.md      # No hardcoded secrets, validate inputs
  coding-style.md  # Immutability, file organization
  testing.md       # TDD workflow, 80% coverage
  git-workflow.md  # Commit format, PR process
  agents.md        # When to delegate to subagents
  performance.md   # Model selection, context management
```



## MCP

注意，可以操作数据库的。

Be picky with MCPs.

因为工具定义本身要占 token。200k 的窗口开太多 MCP 可能实际只剩 70k 给代码和对话。所以建议是配置里可以装 20-30 个，但同时启用的控制在 10 个以内、总工具数不超过 80。

## Plugin

skill 和 mcp，或者 hook/tools 结合。

先执行，然后 /plugins 选择：

```bash
claude plugin marketplace add https://github.com/mixedbread-ai/mgrep

# Open Claude, run /plugins, find new marketplace, install from there
```

比如有 lsp，可以让 claude code 像 vsc 一样诊断 ts 语法错误。

```bash
# Enabled plugins example
typescript-lsp@claude-plugins-official  # TypeScript intelligence
pyright-lsp@claude-plugins-official     # Python type checking
hookify@claude-plugins-official         # Create hooks conversationally
mgrep@Mixedbread-Grep                   # Better search than ripgrep
```

同样，注意上下文窗口。

## 并行工作

可以先不管。

## TODO

**第一步：基础（装完 CLI 立刻配）**

- [ ] `CLAUDE.md` — 项目技术栈、目录结构、开发规范
- [ ] `CODEMAP.md` — 代码地图，git 追踪
- [ ] `typescript-lsp` 插件 — TS 类型智能

**第二步：日常效率（用几天后加）**

- [ ] PostToolUse hook：编辑 TS/JS 后自动跑 Prettier
- [ ] PostToolUse hook：编辑 TS/TSX 后自动跑 `tsc --noEmit`
- [ ] PostToolUse hook：检查 `console.log` 残留
- [ ] PreToolUse hook：`git push` 前强制 review
- [ ] `hookify` 插件 — 用自然语言生成 hook 配置
- [ ] `/commands` — 高频操作写成 slash commands

**第三步：代码质量（流程稳定后加）**

- [ ] `~/.claude/rules/coding-style.md` — 编码规范
- [ ] `~/.claude/rules/git-workflow.md` — commit 规范
- [ ] `~/.claude/rules/testing.md` — 测试要求
- [ ] Stop hook：codemap 自动更新
- [ ] PreCompact hook：压缩前保存关键上下文
- [ ] `mgrep` 插件 — 增强代码搜索

**第四步：外部集成（有需求时加）**

- [ ] GitHub MCP — PR/Issue 操作
- [ ] Supabase MCP — 如果后端迁移到 Supabase
- [ ] `context7` 插件 — 实时文档查询
- [ ] `pyright-lsp` 插件 — FastAPI 后端 Python 类型检查

**第五步：进阶（瓶颈时加）**

- [ ] Subagents — 按职能拆分（code-reviewer、tdd-guide 等）
- [ ] `/fork` 并行工作流
- [ ] Git worktrees 多分支并行
- [ ] 自定义 statusline
- [ ] Sandbox 模式 / `--dangerously-skip-permissions`

