# Panel Worktree 并行开发上手 Checklist

从单 worktree 串行开发起步,跑通 2 个池的并行流程,把 worktree 约束落进 CLAUDE.md。终态:能在 2 个池里并行推 issue,主 repo 串行 merge,联调验证就在出 issue 的那个池里做。

前置:已熟悉 panel 单 worktree 流程。

***

## Phase 0 — 起池前的准备 (\~5min)

* [x] 主 repo `git status` 干净 — `cd /Users/kinesis/panel && git status`

* [x] 当前 HEAD 在 dev — `git branch --show-current` 应输出 `dev`

* [x] `git worktree list` 确认现在只有主 repo 一个 worktree

* [x] `git branch | grep issue-` 看一下未合的 issue 分支,心里有数

***

## Phase 1 — 起 2 个池 + 摸清 worktree 行为 (\~15min)

物理建立池子,每个池绑专属占位分支(`pool-N`)。

* [x] 起池 1 — `git worktree add -b pool-1 ../panel-pool-1 dev`
  * 🔍 `-b pool-1` 在 dev 这个 commit 上起新分支 pool-1。dev 已被主 repo 独占,池子用专属分支绕开冲突

* [x] 起池 2 — `git worktree add -b pool-2 ../panel-pool-2 dev`

* [x] `git worktree list` 看到 3 个 worktree(主 repo dev、池 1 pool-1、池 2 pool-2)

* [x] 池 1 `git branch --show-current` 应输出 `pool-1`

* [x] `git log pool-1 -1 --oneline` 跟 `git log dev -1 --oneline` 一致(同一个 commit)

* [x] 验证「共享 .git」— 池 1 `git checkout -b test-wt`,主 repo `git branch | grep test-wt` 能看到 ✓

* [x] 验证「独立 working tree」— 池 1 改一个文件,主 repo `git status` 看不到 ✓

* [x] 池 1 切回 pool-1 删测试分支 — `git checkout pool-1 && git branch -D test-wt`

***

## Phase 2 — CLAUDE.md 加 worktree 约束 (\~20min)

把规则写进 CLAUDE.md,池里 CC 被压缩后也能 follow。

* [x] 🌟🌟🌟🌟 🤖 主 repo 起 `issue-N` 分支(N 看 git branch 取空号),把下面这块加到 CLAUDE.md(放在「数据保护」之后,「常用命令」之前)：。验证什么：将CLAUDE.md和下面的很好的融合。走得通：融合了；走不通：没融合。第一轮：目前在联调部分说明清楚，大概下一回合能结束这个。

````
## Worktree 并行约束(硬规则)

本仓库支持多 worktree 并行开发。结构:

- `/Users/kinesis/panel/` — 主 repo,永远在 dev,只负责 merge
- `/Users/kinesis/panel-pool-N/` — 干活池 N(目录名),绑专属占位分支 `pool-N`(分支名),在 issue 分支上推进 + 联调

CC 用 `pwd` 判断当前位置:含 `panel-pool-` 在池里,等于 `/Users/kinesis/panel` 在主 repo。

### 池子分支语义

每个池绑一个**专属占位分支** `pool-N`(N 跟池号对应),作为池子的「停车位」:

- 池 N 起来时 pool-N 跟 dev 同 commit,但**不直接 checkout dev**(dev 被主 repo 独占)
- pool-N 分支本身**不直接 commit**,只是停车位

完整一轮的池子状态变化:

1. 池 N 在 pool-N(停车位,跟最新 dev 同 commit)
2. CC 起 issue 分支:`git checkout -b issue-X` → 池在 issue-X
3. 推完 + snapshot todolists,STOP 等用户
4. 用户主 repo merge:`cd /Users/kinesis/panel && git merge --no-ff issue-X`
5. 池回停车位:`git checkout pool-N && git merge dev --ff-only`
6. 池又回到 pool-N == 最新 dev,准备下一轮

CC 在池里 `git branch --show-current`:
- 输出 `pool-N` → 在停车位,准备起新 issue
- 输出 `issue-N` → 正在推进 issue
- **不会**输出 `dev`(池里永远碰不到 dev)

### 联调验证规则

任何时刻最多一个 worktree 在跑 backend(端口 8089 独占)和 mac app(Panel.app 全局单例)。

- 多池干活推进时:全部只 build 不跑服务(`go build` / `xcodebuild`)
- 所有池 STOP 等审查时:挑一个池就地启服务联调
- 切池验证前必须 `pkill -f panel-backend && pkill -9 -f Panel.app` 确认上一个池的服务完全停掉

### Worktree 文件共享

worktree 之间共享 `.git/` 和 `.gitignore` 规则,working tree 文件独立。

- `todolists/` — 每池独立笔记,merge 时各池带各池的 snapshot
- `data/` — 已 gitignore,各池独立 SQLite,测试用 `t.TempDir()`
- API key:存 macOS UserDefaults `com.kinesis.panel`,跟 worktree 无关

### Merge 流程(在池里干活时用这个,覆盖前面的单 worktree 版本)

CC 在池里推完 issue,给用户的命令分两步:

```bash
# Step 1 (在当前池里执行)
git add todolists/ && \
  git commit -m "chore(todolists): snapshot" 2>/dev/null

# Step 2 (用户切到主 repo 执行)
cd /Users/kinesis/panel && \
  git merge --no-ff issue-N
```

主 repo HEAD 永远在 dev,不在主 repo 起 issue 分支。

### 冲突处理

并行 merge 串行进行。池 2 的 issue 跟最新 dev 冲突时,**池里不主动 rebase**,
在主 repo 里 merge 时就地解。池 2 之后回 pool-2 fast-forward 即可。
池子只负责分支推进,冲突解决全部在主 repo。

````

* [x] 在原「合 issue 到 dev 的命令格式」章节加一行说明:`> 注:此格式用于单 worktree 模式。worktree 并行模式见下方「Worktree 并行约束」。`

* [x] commit 这次 CLAUDE.md 改动 — `docs(meta): add worktree 并行约束`

* [x] STOP 等用户验证,验过后按硬规则两步合 issue 到 dev
  * 🔍 这次还是单 worktree 模式合(主 repo 起的 issue 分支),下一个 Phase 起才走池

***

## Phase 3 — 池内单飞试水 (\~30min)

挑一个**纯 mac UI 改动**的小 issue,完整跑一遍池内流程。

* [x] 池 1 开 CC 会话 — 终端窗口 A:`cd /Users/kinesis/panel-pool-1 && claude`

* [x] 池 1 开 bash 终端 — 终端窗口 B:`cd /Users/kinesis/panel-pool-1`

* [x] 池 1 当前在 pool-1,起 issue 分支 — CC 跑 `git checkout -b issue-N`
  * 🔍 此时 pool-1 == dev,从 pool-1 起 issue 等于从 dev 起

* [x] CC 推进 issue,期间观察:
  * CC `pwd` 识别在池里,只 build 不主动启服务 ✓

  * bash 终端 `git status` 实时看到 CC 改动 ✓

* [x] CC STOP 后,池 1 就地启服务联调:

```
cd backend && \
  go build -o ~/panel/bin/panel-backend ./ && \
  kill -TERM $(pgrep -f panel-backend)

cd ../mac && \
  xcodebuild -scheme Panel -configuration Debug \
    -derivedDataPath build build && \
  pkill -9 -f Panel.app && \
  open build/Build/Products/Debug/Panel.app

```

* [x] 验过后两步 merge:Step 1 池 1 snapshot,Step 2 主 repo merge

* [x] **池 1 回停车位** — `git checkout pool-1 && git merge dev --ff-only`
  * 🔍 切回 pool-1 而不是 dev(dev 被主 repo 独占),然后 fast-forward 让 pool-1 跟上刚 merge 的最新 dev

* [x] `git log -1 --oneline` 应看到刚才 merge 的 commit

***

## Phase 4 — 真正的 2 池并行 (\~60min)

两个池同时推 issue,体会并行节奏 + 串行 merge。**池子只负责分支推进,冲突全部在主 repo 解。**

* [x] 挑两个**互不干扰**的 issue:一个 mac UI,一个 backend pure logic

* [x] 池 1 在 pool-1 上起 issue 分支,池 2 在 pool-2 上起 issue 分支

* [x] 两个 CC 会话同时跑

* [x] 两个池都 STOP 后,挑池 1 启服务联调

* [x] 池 1 验过 → 两步 merge → `pkill -f panel-backend && pkill -9 -f Panel.app`

* [x] **池 1 回停车位** — `git checkout pool-1 && git merge dev --ff-only`

* [x] 池 2 启服务联调(此时池 2 还在它的 issue 分支上,基于老 dev)

* [x] 池 2 验过 → 主 repo merge:

  * 无冲突 → 直接合上

  * 有冲突 → 在主 repo 就地解,池 2 不动

* [x] **池 2 回停车位** — `git checkout pool-2 && git merge dev --ff-only`

* [x] 冲突文件记一下,作为「哪些 issue 适合并行」的判断依据

***

## Phase 5 — 日常维护 (\~10min,持续)

* [x] 池子用完一轮回停车位 — `git checkout pool-N && git merge dev --ff-only`

* [x] 已 merge 的 issue 分支删掉 — 在主 repo `git branch -d issue-N`

* [x] **不要删 pool-N 分支** — 删了池子下次起新 issue 没起点

* [x] 💡 长期不用的池整体回收 — `git worktree remove ../panel-pool-N`,然后 `git branch -D pool-N`(占位分支也清掉)

* [x] 💡 池里 `mac/build/` 各自独立占空间,定期 `rm -rf mac/build/`

***

## 品味校准 (\~5min,跑 1\~2 周后回来填)

* [x] 生死判断 — worktree 并行有没有真的提速?提了多少?还是 context 切换变慢了?

  真提速了吧。爽推

* [ ] 需求真伪 — 你真需要 4 池吗,还是 2 池就够?瓶颈是 CC 速度还是脑力切换?

* [ ] 核心设计决策 — 「主 repo 只 merge,池子干活 + 联调」这个分工你认同吗?

* [ ] DX 打分(1-5):池间切换摩擦、merge 冲突频率、CC 在池里的可靠性、整体心智负担

* [ ] 一句话总结 — 推荐还是劝退别人用这套并行流程?

* [ ] 💡 第二周再做一次校准对比,看自己直觉准不准

***

执行节奏建议:

* **Phase 0\~1 一起跑**,15 分钟搞定环境

* **Phase 2 单独抽一段时间**,规则要走 issue merge 才生效,后续 Phase 才走新流程

* **Phase 3 是关键检查点**,池里 CC 行为不对就回来迭代 CLAUDE.md

* **Phase 4 跑通算正式上线**

如果 Phase 3 发现 CC 在池里行为偏(比如自己跑去主 repo 执行命令),说明 CLAUDE.md 那段还不够硬,回来加规则。
