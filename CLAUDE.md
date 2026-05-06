# Panel — CC 操作约束

<!--
本文件 CC 每次新对话 / 压缩后自动加载到 context。遵循 Anthropic 官方建议:
- 目标 < 200 行
- 具体可验证指令 ("Run X" > "Test it")
- 不放短期状态 (测试期标志这种, 进 PRD 或 panel-construction.md)
- 详细背景看 PRD.md, 这里只放 CC 反复需要的硬规则
-->

项目 = Mac 本地个人指挥中枢 (SwiftUI + Go 后端 + SQLite)。详见 PRD.md。

---

## Git 工作流 (硬规则)

每个 feat/fix/refactor/docs 推进**走 issue 分支**, **不直接 commit 到 dev**。

```bash
# 起分支 (N 单调递增, 看 git branch 找下一个空号)
git checkout dev && git checkout -b issue-N

# 在 issue-N 上做改动 + commit (一个推进可多 commit)
git add ... && git commit -m "feat(...): ..."
```

**做完 STOP — 不自己 merge 到 dev**。把 commit 留在 issue branch 等用户验证后合。

合的命令 (用户执行) — head 已在 issue branch, todolists/ 笔记一并 chore commit:

```bash
git add todolists/ && \
  git commit -m "chore(todolists): snapshot" 2>/dev/null; \
  git checkout dev && git merge --no-ff issue-N
```

无 todolists/ 改动时 commit 静默失败 (2>/dev/null), `;` 不阻断后续 checkout + merge。
CC 给合命令必须用这格式, **不能**只给 `git merge --no-ff issue-N` 一句。

**同一功能的多个修复留在同一个 issue branch** (不开新 branch):
- 用户上次纠正过两次 (issue-14/15 拆错, issue-17/18 拆错) — cherry-pick 到原 branch 上
- 如果 issue 已被用户 merge, 才开新 issue branch

## Conventional Commits

格式 `<type>(<area>-<module>[-<feature>]): ...`:
- `feat(backend-保健品-提醒推送): ...`
- `feat(mac-todolist-聚焦卡片): ...`
- `fix(mac-财务): ...`
- 跨层 `feat(both-记录): ...`

type: `feat / fix / refactor / docs / test / chore / perf / ci`。**粒度细** — 一个 logical change 一个 commit。

## 数据保护 (硬规则)

`todolists/*.md` 是变化最频繁的文件 (panel autosave + 用户随时写笔记), 跟 commit 节奏不同步。

- **默认避免 `git reset --hard`**, 优先 `git revert` 或 fix-forward
- 必须 reset --hard 时先 stash:
  ```bash
  git stash push -m "save todolists" -- todolists/
  git reset --hard <commit>
  git stash pop
  ```
- 单文件回退用 `git checkout <commit> -- <path>`
- 已 gitignore: `data/` `mac/build/` `todolists/micro/`

---

## 常用命令

```bash
# Backend 改完 (Go) — launchd 自动 respawn
cd /Users/kinesis/panel/backend
go build -o ~/panel/bin/panel-backend ./
kill -TERM $(pgrep -f "panel/bin/panel-backend")

# Mac 改完 (Swift) — 加新文件后必须先 xcodegen
cd /Users/kinesis/panel/mac
xcodegen generate                                                    # 仅当增删文件
xcodebuild -scheme Panel -configuration Debug -derivedDataPath build build
pkill -9 -f "Panel.app/Contents/MacOS/Panel"
open /Users/kinesis/panel/mac/build/Build/Products/Debug/Panel.app

# 测试
cd /Users/kinesis/panel/backend && go test ./... -race
```

## 测试沉淀 (硬规则)

**每个 feat/fix 的复现路径必须沉淀成测试代码**, 不能 bash+curl 验完就消失 —
context 压缩后 LLM 拿不到测试方法 / 容易重复踩坑。

布局:
- 单测: 紧挨实现, `<pkg>/<code>_test.go` (如 `store/supplements_test.go`)
- 集成测试: `backend/tests/integration/`
  - **必读索引**: `backend/tests/integration/README.md` — 列每个文件覆盖的 case +
    历史 commit ref + user 原话复现步骤
  - 基础设施: `helpers.go` 提供 `newTestEnv` / `assertNotifBody` 等

何时进集成测试 (而不是仅单测):
- 跨包状态机 (store + server + supplements/english 三包以上)
- 通知 lifecycle (dismiss/undismiss/cascade/idx 越界)
- 任何 user 在 issue 推进时 bash+curl 验过的复现路径

写测试约定 (issue-27 起):
1. **fix 完同步加 test** — issue 上一个 commit 修 bug, 紧跟一个 commit 加 test
   (commit type 用 `test(...): ...`)
2. test func 注释里带 历史 commit hash + user 原话复现步骤,
   利于压缩后 grep 找回上下文
3. 加 / 改 test 时同步更新 `backend/tests/integration/README.md` 索引表

## 关键路径

- backend HTTP `:8089` (launchd label `com.kinesis.panel`)
- DB `~/panel/data/panel.db`
- mac app `/Users/kinesis/panel/mac/build/Build/Products/Debug/Panel.app`
- KeyStore (UserDefaults `com.kinesis.panel`):
  - `voice.openrouter_api_key` (F12 听写)
  - `voice.groq_api_key` (英文精听 STT 直连)

---

## 命令格式 (iTerm2 wrap 防护, 硬规则)

CC 输出的代码块在 iTerm2 / 大多数终端里会按窗口宽度 wrap 成多物理行 — 用户复制
粘贴时, wrap 后的尾部 token 常被当成下一行命令独立执行 (例如把 `issue-27` 单独
当命令), 导致行为错乱。

**任何超过 ~80 字符的 shell 命令必须用 `\` 续行**, 每物理行 < 80 字符。

❌ 错误 (单行过长, iTerm2 wrap 后复制易断):
```bash
git add todolists/ && git commit -m "chore(todolists): snapshot" 2>/dev/null; git checkout dev && git merge --no-ff issue-N
```

✅ 正确 (反斜杠续行, wrap 后整段复制仍正确):
```bash
git add todolists/ && \
  git commit -m "chore(todolists): snapshot" 2>/dev/null; \
  git checkout dev && git merge --no-ff issue-N
```

✅ 也可 (拆多行执行, 失去 short-circuit 但更直观):
```bash
git add todolists/
git commit -m "chore(todolists): snapshot" 2>/dev/null
git checkout dev && git merge --no-ff issue-N
```

适用于所有 CC 输出给用户复制的命令 (curl / sqlite3 / xcodebuild / docker / 等等),
不限于 git。

## 验证 SwiftUI 改动

SourceKit 单文件分析常报 "Cannot find type X in scope" 跨文件 false positive — **以 `xcodebuild ... build` 输出为准**, `BUILD SUCCEEDED` 即可忽略 SourceKit 报错。

修 UI 后必须实际重启 Panel 验证 (xcodebuild + pkill + open),不能只 build 不跑。

## Conventions

- **commit message 中文 OK**, scope 用中文 (如 `feat(mac-保健品): ...`) 或英文 (如 `feat(mac-supplements): ...`) 都可
- 主分支 `dev` (不是 `main`)
- 用户偏好「milestone 拆细 + 每完成停下等确认」, 不要一次性跑完多个未确认的 milestone
- 用户偏好简体中文回复 (含错误 message 翻译)

---

## 引用

- 详细 PRD: `/Users/kinesis/panel/PRD.md`
- 构建 todolist: `/Users/kinesis/panel/todolists/panel-construction.md`
- 历史进度: `~/.claude/projects/-Users-kinesis-panel/memory/MEMORY.md` (CC 自动 memory, 跨会话保留学到的偏好)
