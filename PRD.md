# Panel — Mac 本地个人指挥中枢

> 项目源真相文档。本文件描述 panel 的目标、模块设计、技术架构。  
> 真正的可勾选任务清单见 `todolists/panel-construction.md`。  
> 本文件和 todolist 各自服务不同目的;两者可能会有同步漂移,**以 PRD 描述的目标为准**,todolist 服务于推进过程。

## 一句话定位

单入口处理所有"看一眼+做一下"的事:今日 todo、临时事项、财务、食物保健品提醒、命令执行、记录模块、(选做) 微信 QQ 红点。

## 设计原则

- 完全本地、纯单人、无网络
- 不替代专业工具,减少切换成本
- 数据以 `.md` / SQLite 持久,可直接看 / 可手改
- 我是工程师,简洁可控优先
- 跨 Mac 关机重启 / 跨日,关键状态必须持久化

## 模块清单

### 1. Todolist (核心)

**数据源**: `~/panel/todolists/*.md`

**文件命名**:
- 普通: `主题名.md` (支持中文,UTF-8)
- 完成态: 文件名加 **前缀** ✅ (例: `✅ last30days-skill 上手指南 - checklist.md`)

**跳过标记**: `[[[skip]]]` 写在 `- [ ]` 行内任意位置。非 md 标准语法,纯 label,由用户自己加/减。

**Panel 行为**:
- 列出 `todolists/` 下 `.md` (不递归到 `micro/`)
- **锚定 / 选定 分离**:
  - **选定** (selectedName): 决定右栏渲染哪个文件,点 sidebar 列表项即可任意切换
  - **锚定** (anchor): 决定聚焦卡指向哪个 todolist,只由 toolbar **锚定按钮** (`pin` / `pin.fill`) 显式设置
  - 锚定项在左栏用**柔和橙背景**标记,即使列表失焦仍鲜艳 (永不响应失焦变灰)
  - 初始空;可锚到已完成的 (简化状态机);不跨日自动切;一天一锚,中途可换
- 右栏渲染选定文件全文 (WYSIWYG,见下方"编辑")
- **聚焦卡片**: 单拎锚定文件的"第一个非 `[[[skip]]]` 的未完成 `- [ ]`",sticky 在右栏顶部
  - `- [ ]` 可在多级嵌套,需正确解析
  - 顶部展示**父节点面包屑** (从顶层往下),每个父节点截断 60 字 (超过加 `...`)
  - 卡内 `L{n}` 链接: 点击 → 若不在锚定文件,先切回锚定 → scroll 到该任务行 + 暖橙短暂高亮
- **外部修改同步**: 5s 轮询拉后端,检测到 Typora/VS Code 等外部改了 `.md` 自动同步到右栏 buffer
- **Panel 内编辑**: WYSIWYG 即时渲染 (所见即所得,无"编辑模式"开关)
  - 实现: MilkDown (Crepe wrapper, ProseMirror-based) 嵌 WKWebView
  - 主题: classic / classic-dark,**跟随系统色彩模式**自动切换 (CSS `@media (prefers-color-scheme: dark)`)
  - 字号 +/- toolbar 按钮控制 WKWebView pageZoom,**UserDefaults 持久化**
  - 保存: Cmd+S 主动 + **5s 轮询 dirty buffer 自动**;tick 顺序保证 dirty 永远先 save 再 fetch,外部覆盖不丢
- **外部编辑器同样有效**: Typora / VS Code 都行,panel 不是唯一入口

### 2. Micro Todolist (合并入 Todolist tab)

**数据源**: `~/panel/todolists/micro/{YYYY-MM-DD}.md`

**UI 安排**: 不是独立 sidebar tab。Todolist tab 内 toolbar 一个**模式切换按钮** (`calendar` ↔ `list.bullet.rectangle`):
- **Todolist 模式** (默认): 选定文件 + 锚定文件焦点卡
- **Micro 模式**: 当日 micro 文件 + 当日第一个未完成 `- [ ]` 焦点卡 (前端 parse,无后端 anchor 概念)
- 用户在 micro 模式下点 sidebar 任一 todolist → 自动切回 Todolist 模式

**行为**:
- 当天文件不存在则**自动新建**(空)
- 平铺所有 `- [ ]` (本来就少,1-3 条/日)
- 编辑器同 Todolist (WYSIWYG, 5s 轮询自动保存)
- Micro **隐含锚定**为当日唯一文件,锚定按钮在 micro 模式下 disabled

**每日复发 todo**:
- 入口: 切到 Micro 模式后,toolbar 的 `repeat` 按钮 → 打开**模态框**管理
- 模态框 = 复发 todo 的**唯一**永久增删改入口
- **注入时机**: 早晨蒙版用户选 "进入普通模式" 时,把当日复发 todo 注入到当天 micro 文件顶部 (选 "外出" 则**不注入**)
- 当天 micro 文件中: **只能勾选 / 加 [[[skip]]]**, 不提供"删除"动作 (避免误删)
- 真正想永久取消某条 → 模态框删
- 默认 0 条复发 todo

### 3. 财务管理

**支出三类**:

| 类型 | 字段 | 说明 |
|---|---|---|
| **账单 (bill)** | `{name, monthly_amount, remaining_months}` | 等额还款,有止期 |
| **固定支出 (fixed)** | `{name, monthly_amount}` | 等额无止期 |
| **自由支出 (free)** | `{date, amount, note?}` | 手动逐笔录入,**月度上限可调,默认 ¥1000** |

**当前余额**:
```
balance = 初始值 + Σ(手动收入) − Σ(月末结算的所有支出)
```

**观测指标 — 债务膨胀** (布尔):

设当月预期支出 = 固定支出 + 自由支出。若收入不够覆盖,差额 `a` 需要新借(增加一笔账单):

- 新借 a 元,期限 N 月,假设年利率 12%,月利息均摊 ≈ `a × 12% / N`
- **当月收入 < 预期支出 + a × 12% / N → 债务膨胀 = true**
- 直觉: 不仅没钱花,还要为新借的钱掏"无谓利息"
- N 由用户输入;a 是模型推算的差额

### 4. 食物 & 保健品

**录入字段**: `{name, type (食物|保健品), windows: [{start, end}, ...], notes?}`

**默认**: 无任何项,按需添加。

**行为**: 过点不补 (跳过那一餐,不堆积)。

**通知**: 触发 panel **顶部通知栏**(不弹 macOS 通知中心,至少当前阶段不集成)。

**与"记录"模块联动**: 食物保健品的"当月完成情况" `<name, type, 完成天数/应完成>` 在记录板块展示;录入入口仍在本模块。

**特殊变量 `wake` + 晨起选择**:
- 早上 06:00+ panel 显示**全屏蒙版** (绝大概率 6 点前不会醒)
- 蒙版上**两个按钮**: **进入普通模式** / **今天外出**
- 点击任一 → 此刻记为 `wake_time` + 状态切到对应 (普通 或 外出)
- 选 **普通**: 同步把今日的"每日复发 todo"注入到当天 micro 文件
- 选 **外出**: 不注入复发 todo (人不在电脑前)
- 之后日间可手动按钮在 普通 ↔ 外出 切换 (但不再触发复发 todo 注入)
- windows 可引用 `wake` / `wake+1h` / `wake+2h` 等表达式
- `wake_time` 和当日状态选择**持久化** (Mac 关机不丢)
- 次日 06:00 重置触发新蒙版
- 注: 若 Mac 跨夜运行,蒙版应在 06:00 弹出 (不依赖系统唤醒事件)

### 5. 全局通知栏 + 状态/主题 (非独立模块,内嵌)

**通知栏**: Panel 固定位置一条,汇集其他模块的提醒
- 食物保健品到点
- 财务 (债务膨胀触发)
- 记录 (今日握力没录)

**4 种状态**,优先级 **睡眠 > 外出 > 专注 > 普通**:

| 状态 | 触发 | 主题色 | 行为 |
|---|---|---|---|
| **普通** | 默认 | (无主题色) | 正常 |
| **专注** | 默认 17:00–21:00 (可设) | **柔和橙** | 通知栏 1-2 条 label: "手机飞行模式 / 退出微信 QQ / 不开视频站" |
| **睡眠** | 默认 22:30+ (可设) **强制** | **淡紫** | **阻挡 panel 内通知** (未来若集成 macOS 通知中心也静音) |
| **外出** | 用户手动按钮切换,**当日有效** | **某种蓝** | 暂时只切状态;不弹食物保健品到点提醒;晚上回来补录功能留到后续 phase |

**设置项**: 仅暴露 "进专注时间" 和 "进睡眠时间" 两个值。
- 外出靠按钮触发
- 普通由其余三态退出后默认

### 6. 记录模块

**通用能力**: "指标录入 + 滚动统计 + 状态分类" 的小框架

**首个特化: 晨起握力**
- 输入: 每日 2 次握力 (左右手最大力,机械体重秤捏)
- 滚动: 最近 14 天 均值 M、标准差 SD
- 分类:
  - 偏离 ∈ ±1 SD → "正常,照常训"
  - −2 SD ~ −1 SD → "降强度"
  - < −2 SD → "改有氧或休息"
- Panel 展示: 今日偏离值 / 基线 M / 当前 SD

**食物保健品当月完成情况展示**: 借用本模块展示位,显示每个 `<name, type>` 的当月完成天数 / 应完成天数 (录入入口仍在食物保健品模块)。

**后续可加** (Phase 6 之后): 训练记录、其他生理指标记录,共用通用能力。

### 7. Bash 入口

**功能**: panel 内执行 shell 命令,免切 Terminal。

**实现**:
- 后端 `POST /api/exec` `{cmd}` → `{stdout, stderr, exit_code}`
- 使用 **zsh**,启动时 source `~/.zshrc` 以继承 PATH 和 alias 偏好
- 简易 REPL,命令历史存 SQLite (`bash_history` 表)

### 8. (选做,推后) 微信/QQ 红点聚合

- 原 Phase 1 hook 通路已验证,因 macOS TCC 暂停
- 后端按"可插拔 message source" 接口设计,Phase 1 起就保留 `MessageSource` 抽象,先 mock 实现
- 真接入留到这个 Phase: 微信 4.1.7.57 + `tccutil reset All com.tencent.xinWeChat` + 一次性手动批准所有权限

### 9. (取消) 搜索

不做。Panel 用熟 1-2 周后再评估。

### 10. 英文练习 (精听)

**目的**: 拉取美国英文音频源, ASR 拆成句子级 segments, 用户勾选累计时长, 任一时段窗口达
10min 即整日"完成", 全部勾完归入历史档案可重听。

**核心流程**:

- **抓取**: backend Go 6h cron (`main.go:runEnglishFetchLoop`) 串行调每个注册的 fetcher,
  每个 fetcher 解析 RSS feed → 候选 audio metadata (`AudioMeta`); 长度 ∈ [30s, 25min]
  才入库 (`backend/english/fetcher.go:RunFetchAll`); 每轮限 5 条下载 + 1s 礼貌限速
- **静音检测**: 下载后调 `ValidateAudioEnergy` (Go go-mp3, 头/中/尾各 5s PCM RMS); 三段
  全 < -50dB 判 silent → 标 `segments_status=failed` + reason; 不删 mp3 留调试 (沉淀
  VOA Akamai CDN -90dB 占位坑)
- **ASR**: mac 打开 audio 触发 Groq Whisper Large V3 直连 (词级 timestamp, 18.5s/11min,
  $0.018/集); SpeechSegmenter 拆句 (标点 / 静音 gap > 0.6s / 强切 ≥12 词 + 缩写合并)
- **窗口**: `english_settings.windows_json` 多个时段 (默认 07:30-09:00 / 21:00-22:30),
  任一窗口达 `threshold_min` 阈值整日完成; 可 snooze 30min/槽 [1, 8]
- **通知**: lead 15min 弹 banner, source="english", source_ref=`english:window:{idx}:date:{d}`,
  达阈值 → DismissNotificationByRef, 跌破 → UndismissNotificationByRef (沿用 supplements
  套路)

**跳过 / 模态框**:

- 当前音频卡右上"跳过"按钮 = 完成所有段落的等价操作; 两种触发场景走完全相同流程:
  1. 用户主动点跳过
  2. 当前 audio 所有 segment 全勾完, mac `toggleEnglishSegment` 检测到自动调起
- 模态框 (`mac/Panel/Views/EnglishSkipSheet.swift`):
  - 顶部 topic Picker (全部 / 金融 / 科技 / 时事 / 自然科学 / 综合)
  - 主体候选卡: title + [SOURCE] + 时长 + topic chip + description preview (~120 字)
  - "再抽一个" → 把当前候选 ID 加进 `excludeIDs` 列表 + refetch (后端 PickCandidate
    跳过 excludeIDs)
  - "取消" 不归档当前 audio; "确认" → POST /skip 归档当前 + 切到 selected
- 主题分类: free-text (后端不做 enum 校验), 当前可选值 finance / tech / current_affairs
  / science / general; 加新 topic 不需 schema migrate
- **breaking 注意** (vs Phase 13): server `handleCheckEnglishSegment` 不再自动调
  `maybeArchiveAudio`; 归档**只**通过模态框确认走 POST /skip

**新源接入流程** (硬规则, 用户偏好"先听样本再纳入"):

1. CC 起 issue branch, 写新 fetcher (仿 `backend/english/npr_fetcher.go`, RSS allowlist
   涵盖整条 redirect chain + Topic 字段必填), **暂不**注册到 `main.go` fetchers 列表
2. 跑 `go test -tags=integration -v -run Test<Source>Fetcher_FetchListsItems ./english/`
   拉 RSS, log top 5 audio meta (title, duration, audio_url)
3. 跑 `go test -tags=integration -v -run Test<Source>Fetcher_DownloadOne` 下第 1 条 mp3
   到 tempdir, 输出绝对路径 + ValidateAudioEnergy 结果
4. **STOP** — 用户人工耳朵听 mp3 (`afplay <path>` 或 finder open), 验声音质量 + 主题
   对位 + 长度合理 (在 [30s, 25min] 区间)
5. 用户口头 ok → main.go fetchers 列表加 fetcher 实例 + 提 PR
6. 失败的源在 PR 加注释 "rejected: <reason>", fetcher 文件保留**不**注册 (历史教训
   沉淀, 避免下个 LLM 重复评估同一个失败源)

**已知坑**:

- VOA Akamai CDN 全静音 (mp3 看起来 30min 实际 -90dB 占位): 静音检测 fetcher 层拦
- 28min Whisper 中断 (Groq 25MB 上限): fetcher 层 `[30s, 25min]` cap 直接拒
- Description 文本可能很长 (NPR 描述 > 500 字): mac 模态框前端 truncate 到 ~120 rune +
  "…"; 真不够直观再考虑 LLM 摘要提取

**当前 active 源**:

- NPR The Indicator from Planet Money (Topic="finance", 9-11min/集, ~10MB)
- VOA Learning English (Topic="general") — 已禁用 (Akamai 静音坑), 静音检测上线后可考虑回挂

**详细 plan**: `~/.claude/plans/linked-petting-emerson.md` (Phase 14)

## 技术架构

### 后端
- 语言: Go
- 部署: 常驻进程,launchd 启动
- 端口: REST `:8089`
- 数据:
  - SQLite (`~/panel/data/panel.db`): 财务、食物保健品、记录、bash 历史、设置、wake_time 等状态
  - .md 文件: `todolists/`, `todolists/micro/`
- 单文件 SQLite,无内置权限/用户体系 (与 PG/MySQL 不同),权限 = 文件系统权限
- 可选 OneBot client (Phase 9)

### 前端
- 框架: SwiftUI macOS App (deployment target macOS 14)
- 位置: `~/panel/mac/`
- 布局: `NavigationSplitView` 三栏
  - sidebar: tab (Todolist / 财务 / 保健品 / 记录 / Bash; Micro 已并入 Todolist)
  - content: 当前 tab 的列表 (todolists 列出 / 财务支出列表 / etc.)
  - detail: 渲染区
- WYSIWYG 编辑器:
  - 库: MilkDown (Crepe wrapper, ProseMirror + remark)
  - 集成: WKWebView (NSViewRepresentable),JS↔Swift bridge 走 `postMessage` / `evaluateJavaScript`
  - 源码: `mac/editor-src/` (npm + esbuild,iife bundle)
  - 产物: `mac/Panel/Resources/editor/{index.html, bundle.js, bundle.css}` (folder reference 进 .app)
  - 透明背景 + scroll 自管,字号通过 `webView.pageZoom` 控制
- 主题:
  - WYSIWYG: classic / classic-dark,跟随系统色彩 (CSS `@media`)
  - 4 状态(普通/专注/睡眠/外出): 顶部通知栏背景色 + 锚定行底色 (柔和橙) 等局部叠加
- 状态管理: `@Observable AppState` 单例,5s polling 处理外部改动 + dirty buffer autosave
- 全局快捷键: 后续 phase 加 (当前只有 `Cmd+S` 保存)

### 数据持久化
- 用户可读改: Markdown 文件
- 结构化: SQLite
- 跨重启状态 (wake_time, anchored todolist, 当日 mode 等): 必须持久化

## Git 约定 (monorepo)

### Issue 分支工作流 (2026-05-02 起)

每个独立的 feat / fix / style / refactor / docs 推进都走"分支 → 合并",不直接在 dev 上 commit。

```bash
# 1. 起 issue 分支 (N 单调递增,从 1 开始;之前的直接 commit 不追溯)
git checkout dev
git checkout -b issue-N

# 2. 在 issue-N 上做完所有工作 (一个推进可多个 commit)
git commit -m "feat(...): ..."
# ...

# 3. 完成后回 dev,non-fast-forward + 不立即 commit
git checkout dev
git merge --no-ff --no-commit issue-N
# 等用户看完 diff 再 commit
```

### Conventional Commits

- 至少二级 scope,三级 (具体功能) 视情况加,不强求
- 格式: `<type>(<area>-<module>[-<feature>]): ...`
- 例:
  - `feat(backend-保健品-提醒推送): ...`
  - `feat(backend-保健品): ...` (二级也可)
  - `feat(mac-todolist-聚焦卡片): ...`
  - `fix(mac-财务): ...`
- 跨层: `feat(both-记录): ...`
- 主分支: `dev`

### 数据保护 / 危险操作

`todolists/` 是项目里变化最频繁的区域 — panel 自己 autosave normalize +
用户随时写笔记, 跟 commit 节奏完全不同步。**任何危险操作 (尤其
`reset --hard`) 之前必须优先保护它**, 否则一不小心就丢未提交笔记 (历
史上踩过)。

- **默认避免 `git reset --hard`**, 优先 `git revert` 或 fix-forward 撤改
- 如果必须 `reset --hard`, 先 stash `todolists/` 再 reset 再 pop:
  ```bash
  git stash push -m "save todolists" -- todolists/
  git reset --hard <commit>
  git stash pop
  ```
- 单文件回退用 path 形式 `git checkout <commit> -- <path>`, 不动其他
- `data/` `mac/build/` 已 gitignore, reset 不影响; `todolists/micro/`
  也 gitignore (每日临时文件); 真正需要保护的是 tracked
  `todolists/*.md`

## 文件夹布局

```
~/panel/
├── .gitignore
├── PRD.md                              ← 本文件
├── todolists/
│   ├── panel-construction.md           ← panel 自身的构建 todolist
│   └── micro/
│       └── YYYY-MM-DD.md               ← 当天临时事务
├── backend/                            ← Go 后端 (HTTP :8089)
├── mac/                                ← SwiftUI macOS App
│   ├── Panel/                          ←   Swift 源
│   ├── Panel/Resources/editor/         ←   MilkDown bundle 产物 (folder ref 进 .app)
│   ├── editor-src/                     ←   MilkDown 源 (npm + esbuild;node_modules ignored)
│   └── project.yml                     ←   xcodegen
├── scripts/
│   ├── com.kinesis.panel.plist         ← launchd LaunchAgent 模板
│   └── install.sh                      ← 等幂安装/重装 backend
├── data/                               ← 运行时 panel.db / panel.log / backups/ (gitignored)
├── wechat_chatter/                     ← 已有,Phase 9 选做时用
├── frida-devkit/                       ← 已有
└── frida-gadget-17.9.1-*.dylib         ← 已有
```

## 已完成 (历史)

微信 4.1.7.57 hook 通路验证完成 (现因 macOS TCC 暂停)。详见 `~/.claude/projects/-Users-kinesis-panel/memory/project_phase1_progress.md`。

## 文档同步约定

PRD 与 todolist 可能漂移。每次需求变化:
1. 优先更新 PRD (本文件)
2. 然后视情况调整 `todolists/panel-construction.md`

PRD 是项目目标的真相;todolist 是手动推进的勾选清单。两者不同步时,以 PRD 为准修正 todolist。
