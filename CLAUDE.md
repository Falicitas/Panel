# Panel — CC 操作约束

<!--
本文件 CC 每次新对话 / 压缩后自动加载到 context。遵循 Anthropic 官方建议:
- 目标 < 200 行
- 具体可验证指令 ("Run X" > "Test it")
- 不放短期状态 (测试期标志这种, 进 PRD 或 panel-construction.md)
- 详细背景看 PRD.md, 这里只放 CC 反复需要的硬规则
-->

项目 = Mac 本地个人指挥中枢 (SwiftUI + Go 后端 + SQLite)。详见 PRD.md。

**不确定时 STOP 问用户, 不要猜了执行**。适用于: issue 编号、该不该新开 branch、测试放单测还是集成测试、命令影响范围不明等。

---

## Git 工作流 (硬规则)

主 repo (`/Users/kinesis/panel/`) HEAD 永远在 dev, **不起 issue 分支**;
CC 在池里 (`panel-pool-N`) 推 issue 分支。每个 feat/fix/refactor/docs 推进
**走 issue 分支**, **不直接 commit 到 dev**。

```bash
# 起分支 (在池里执行, N 单调递增, 看 git branch 找下一个空号)
git checkout -b issue-N

# 在 issue-N 上做改动 + commit (一个推进可多 commit)
git add ... && git commit -m "feat(...): ..."
```

**做完 STOP — 不自己 merge 到 dev**。把 commit 留在 issue branch 等用户验证后合。

### 合 issue 到 dev 的命令格式 (硬规则)

CC 给用户的合并命令**必须**分三步 (池里没法 checkout dev, dev 被主 repo 独占):

```bash
# Step 1: 池里 snapshot todolists
git add todolists/ && \
  git commit -m "chore(todolists): snapshot" 2>/dev/null
```

```bash
# Step 2: 用户切到主 repo merge
cd /Users/kinesis/panel && \
  git merge --no-ff issue-N
```

```bash
# Step 3: 池回停车位 (fast-forward 到刚合的 dev)
cd /Users/kinesis/panel-pool-N && \
 git checkout pool-N && \
 git merge dev --ff-only
```

无 todolists/ 改动时 commit 静默失败 (`2>/dev/null`)。

❌ 不要给单步 `git merge --no-ff issue-N` (丢失 snapshot + 忘 cd 主 repo + 池停在 issue 分支)
✅ 必须给: 上面的三步

`chore(todolists): snapshot` **不是 issue 终点** — 之后仍可在 issue branch 继续加
fix/docs/test 等 commit, 合时同样三步 (snapshot 步骤幂等, 无新改动 silent skip)。

### 新 issue vs 留原 branch (硬规则)

新推进前按顺序判断:

1. 是不是修**最近几个 issue** 留下的 bug? → **留原 issue branch**, 用 cherry-pick
2. 那些 issue 都已 merge 到 dev 了? → 才开新 issue branch
3. 不确定属于哪种? → **问用户**, 不要猜

历史教训: issue-14/15 拆错过一次, issue-17/18 又拆错一次 — 默认倾向「留原 branch」。

## Worktree 并行 (硬规则)

仓库支持多 worktree 并行开发, 结构:

- `/Users/kinesis/panel/` — 主 repo, 永远在 dev, 只负责 merge
- `/Users/kinesis/panel-pool-N/` — 干活池 N (目录名), 绑专属占位分支 `pool-N`
  (分支名), 在 issue 分支上推进 + 联调

CC 用 `pwd` 判断当前位置: 含 `panel-pool-` 在池里, 等于 `/Users/kinesis/panel`
在主 repo。

### 池子分支语义

每个池绑一个**专属占位分支** `pool-N` (N 跟池号对应), 作为池子的「停车位」:

- 池 N 起来时 pool-N 跟 dev 同 commit, 但**不直接 checkout dev** (dev 被主 repo 独占)
- pool-N 分支本身**不直接 commit**, 只是停车位

完整一轮的池子状态变化:

1. 池 N 在 pool-N (停车位, 跟最新 dev 同 commit)
2. CC 起 issue 分支: `git checkout -b issue-X` → 池在 issue-X
3. 推完 + snapshot todolists, STOP 等用户
4. 用户主 repo merge: `cd /Users/kinesis/panel && git merge --no-ff issue-X`
5. 池回停车位: `git checkout pool-N && git merge dev --ff-only`
6. 池又回到 pool-N == 最新 dev, 准备下一轮

CC 在池里 `git branch --show-current`:

- 输出 `pool-N` → 在停车位, 准备起新 issue
- 输出 `issue-N` → 正在推进 issue
- **不会**输出 `dev` (池里永远碰不到 dev)

### 池子里的两个会话

每个池目录 (panel-pool-N) 用户已经开了两个 shell 会话:

- CC 会话 — Claude Code 进程, 跑 git 推进 / 改文件 / 跑测试
- 用户会话 — 普通 bash, 用户手动执行 build & run / merge / 任意命令

两个会话的 cwd 完全独立 — CC 自己 `cd backend && go test` 不会影响用户那个
会话的 cwd。CC 输出给用户复制粘贴的命令一律按「用户当前在池子根目录」假设,
**永远不**加 `git -C` / 多余 `cd` / 绝对路径前缀来「兜底 cwd」。

### 联调验证规则

任何时刻**最多一个 worktree** 在跑 backend (端口 8089 独占) 和 mac app (Panel.app
全局单例)。

- CC **不跑 rebuild.sh / 不启 Panel.app**, build & run 都给用户。STOP 时给启动
  指令 (从 worktree 根跑, 按本次改动层选): `./backend/rebuild.sh` /
  `./mac/rebuild.sh` / 二者 `&&` 串联
- 切池验证前用户先 `pkill -f panel-backend && pkill -9 -f Panel.app` 停上一个池

### Worktree 文件共享

worktree 之间共享 `.git/` 和 `.gitignore` 规则, working tree 文件独立。

- `todolists/` — 每池独立笔记, merge 时各池带各池的 snapshot
- `data/` — 已 gitignore, 各池独立 SQLite, 测试用 `t.TempDir()`
- API key: 存 macOS UserDefaults `com.kinesis.panel`, 跟 worktree 无关

### 冲突处理

并行 merge 串行进行。池 2 的 issue 跟最新 dev 冲突时, **池里不主动 rebase**,
在主 repo 里 merge 时就地解。池 2 之后回 pool-2 fast-forward 即可。
池子只负责分支推进, 冲突解决全部在主 repo。

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

# Icon 改完 — 编辑 mac/design/Icon-3d.svg 后跑这个一键
# (SVG → 1024 PNG → 10 size 进 Asset Catalog → rebuild + 重启)
./mac/deploy-icon.sh

# 测试
cd /Users/kinesis/panel/backend && go test ./... -race
```

## 测试沉淀 (硬规则)

**每个 feat/fix 的复现路径必须沉淀成测试代码**, 不能 bash+curl 验完就消失 —
context 压缩后 LLM 拿不到测试方法 / 容易重复踩坑。

布局:

- 单测: 紧挨实现, `<pkg>/<code>_test.go` (如 `store/supplements_test.go`)
- 集成测试: `backend/tests/integration/`

何时进集成测试 (而不是仅单测):

- 跨包状态机 (store + server + supplements/english 三包以上)
- 通知 lifecycle (dismiss/undismiss/cascade/idx 越界)
- 任何 user 在 issue 推进时 bash+curl 验过的复现路径

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

**任何超过 ~80 字符的 shell 命令必须用 `\` 续行**, 每物理行 < 80 字符。适用所有
CC 输出给用户复制的命令 (git / curl / sqlite3 / xcodebuild / docker / 等等)。

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

## 验证 SwiftUI 改动

SourceKit 单文件分析常报 "Cannot find type X in scope" 跨文件 false positive — **以 `xcodebuild ... build` 输出为准**, `BUILD SUCCEEDED` 即可忽略 SourceKit 报错。

修 UI 后必须实际重启 Panel 验证 (xcodebuild + pkill + open), 不能只 build 不跑。

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
