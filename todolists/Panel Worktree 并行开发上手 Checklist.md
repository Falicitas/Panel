# Panel Worktree 并行开发上手 Checklist

从单 worktree 串行开发起步,跑通 2 个池的并行流程,把 worktree 约束落进 CLAUDE.md。终态:能在 2 个池里并行推 issue,主 repo 串行 merge,联调验证就在出 issue 的那个池里做。

前置:已熟悉 panel 单 worktree 流程。

***

## Phase 0 — 起池前的准备 (\~5min)

* [ ] 主 repo `git status` 干净 — `cd /Users/kinesis/panel && git status`
* [ ] 当前 HEAD 在 dev — `git branch --show-current` 应输出 `dev`
* [ ] `git worktree list` 确认现在只有主 repo 一个 worktree
* [ ] `git branch | grep issue-` 看一下未合的 issue 分支,心里有数

***

## Phase 1 — 起 2 个池 + 摸清 worktree 行为 (\~15min)

物理建立池子,顺手验证「共享 .git / 独立 working tree」的事实。

* [ ] 起池 1 — `git worktree add ../panel-pool-1 dev`
* [ ] 起池 2 — `git worktree add ../panel-pool-2 dev`
* [ ] `git worktree list` 看到 3 个 worktree(主 + 池 1 + 池 2)
* [ ] 池 1 `git checkout -b test-wt`,主 repo `git branch | grep test-wt` 应能看到(共享 `.git`)
* [ ] 主 repo `git status` 不受池 1 切分支影响(各自 HEAD 独立)
  * 🔍 worktree 不是 fork — 共享 `.git/`,working tree 独立
* [ ] 池 1 切回 dev 删测试分支 — `git checkout dev && git branch -D test-wt`

***

## Phase 2 — CLAUDE.md 加 worktree 约束 (\~20min)

把规则写进 CLAUDE.md,池里 CC 被压缩后也能 follow。

* [ ] 🤖 主 repo 起 `issue-N` 分支(N 看 git branch 取空号),把下面这块加到 CLAUDE.md(放在「数据保护」之后,「常用命令」之前):

```
## Worktree 并行约束(硬规则)

本仓库支持多 worktree 并行开发。结构:

- `/Users/kinesis/panel/` — 主 repo,永远在 dev,只负责 merge
- `/Users/kinesis/panel-pool-N/` — 干活池 N,在 issue 分支上推进 + 联调

CC 用 `pwd` 判断当前位置:含 `panel-pool-` 在池里,等于 `/Users/kinesis/panel` 在主 repo。

### 联调验证规则

任何时刻最多一个 worktree 在跑 backend(端口 8089 独占)和 mac app
(Panel.app 全局单例)。

- 多池干活推进时:全部只 build 不跑服务(`go build` / `xcodebuild`)
- 所有池 STOP 等审查时:挑一个池就地启服务联调
- 切池验证前必须 `pkill -f panel-backend && pkill -9 -f Panel.app`
  确认上一个池的服务完全停掉

### Worktree 文件共享

worktree 之间共享 `.git/` 和 `.gitignore` 规则,working tree 文件独立。

- `todolists/` — 每池独立笔记,merge 时各池带各池的 snapshot
- `data/` — 已 gitignore,各池独立 SQLite,测试用 `t.TempDir()`
- API key:存 macOS UserDefaults `com.kinesis.panel`,跟 worktree 无关

### Merge 流程(替换前面「合 issue 到 dev 的命令格式」)

CC 在池里推完 issue,给用户的命令分两步:

\`\`\`bash
# Step 1 (在当前池里执行)
git add todolists/ && \
  git commit -m "chore(todolists): snapshot" 2>/dev/null

# Step 2 (用户切到主 repo 执行)
cd /Users/kinesis/panel && \
  git merge --no-ff issue-N
\`\`\`

主 repo HEAD 永远在 dev,不在主 repo 起 issue 分支。
worktree 共享 `.git`,池子下次 `git checkout dev` 即同步最新 dev,无需 pull。

```

* \[ ] 在原「合 issue 到 dev 的命令格式」章节加一行说明:`> 注:此格式用于单 worktree 模式。worktree 并行模式见下方「Worktree 并行约束」。`
* [ ] commit 这次 CLAUDE.md 改动 — `docs(meta): add worktree 并行约束`
* \[ ] STOP 等用户验证,验过后按硬规则两步合 issue 到 dev
  * 🔍 这次还是单 worktree 模式合(主 repo 起的 issue 分支),下一个 Phase 起才走池

***

## Phase 3 — 池内单飞试水 (\~30min)

挑一个**纯 mac UI 改动**的小 issue,完整跑一遍池内流程。

* \[ ] 池 1 开 CC 会话 — 终端窗口 A:`cd /Users/kinesis/panel-pool-1 && claude`
* \[ ] 池 1 开 bash 终端 — 终端窗口 B:`cd /Users/kinesis/panel-pool-1`
* \[ ] CC 起分支 + 推进 issue,期间观察:
  * CC 应该 `pwd` 识别在池里,只 build 不主动启服务 ✓
  * bash 终端 `git status` 实时看到 CC 的改动 ✓
* \[ ] CC STOP 后,在池 1 就地启服务联调:
  * `cd backend && go build -o ~/panel/bin/panel-backend ./ && kill -TERM $(pgrep -f panel-backend)`
  * `cd ../mac && xcodebuild ... && pkill -9 -f Panel.app && open .../Panel.app`
* \[ ] 验过后两步 merge:Step 1 池 1 snapshot,Step 2 主 repo merge
* [ ] 池 1 切回 dev 确认同步 — `git checkout dev && git log -1 --oneline`

***

## Phase 4 — 真正的 2 池并行 (\~60min)

两个池同时推 issue,体会并行节奏 + 串行 merge。

* \[ ] 挑两个**互不干扰**的 issue:一个 mac UI,一个 backend pure logic
* \[ ] 池 1、池 2 各起一个 issue 分支并行推进
* \[ ] 两个 CC 会话同时跑(各自一个 iTerm 窗口 + bash)
* \[ ] 两个池都 STOP 后,挑池 1 启服务联调
  * 🔍 联调时池 2 必须停着,撞端口
* [ ] 池 1 验过 → 两步 merge → `pkill panel-backend && pkill -9 -f Panel.app`
* \[ ] 池 2 切回 dev 看到池 1 的改动,起服务联调验证池 2
* \[ ] 池 2 验过 → 两步 merge
* \[ ] 如果池 2 merge 冲突,记录冲突文件 — 这是「哪些 issue 适合并行」的判断依据

***

## Phase 5 — 日常维护 (\~10min,持续)

* [ ] 池子用完一轮 `git checkout dev`,池子复用不删
* [ ] 已 merge 的 issue 分支删掉 — 在主 repo `git branch -d issue-N`
* [ ] 💡 长期不用的池 — `git worktree remove ../panel-pool-N`
* [ ] 💡 池里 `mac/build/` 各自独立占空间,定期 `rm -rf mac/build/`

***

## 品味校准 (\~5min,跑 1\~2 周后回来填)

* \[ ] 生死判断 — worktree 并行有没有真的提速?提了多少?还是 context 切换变慢了?
* \[ ] 需求真伪 — 你真需要 4 池吗,还是 2 池就够?瓶颈是 CC 速度还是脑力切换?
* \[ ] 核心设计决策 — 「主 repo 只 merge,池子干活 + 联调」这个分工你认同吗?
* \[ ] DX 打分(1-5):池间切换摩擦、merge 冲突频率、CC 在池里的可靠性、整体心智负担
* \[ ] 一句话总结 — 推荐还是劝退别人用这套并行流程?
* \[ ] 💡 第二周再做一次校准对比,看自己直觉准不准

***

执行节奏建议:

* **Phase 0\~1 一起跑**,15 分钟搞定环境
* **Phase 2 单独抽一段时间**,规则要走 issue merge 才生效,后续 Phase 才走新流程
* **Phase 3 是关键检查点**,池里 CC 行为不对就回来迭代 CLAUDE.md
* **Phase 4 跑通算正式上线**

如果 Phase 3 发现 CC 在池里行为偏(比如自己跑去主 repo 执行命令),说明 CLAUDE.md 那段还不够硬,回来加规则。
