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
- "**锚定**" 一个为当前主 todolist
  - 初始空
  - 可锚到已完成的 todolist (简化状态机)
  - 不跨日自动切换;一天一锚定,中途可手动换
- 渲染锚定文件全文 (markdown)
- **聚焦卡片**: 单拎"第一个非 `[[[skip]]]` 的未完成 `- [ ]`",独立 card 展示
  - `- [ ]` 可在多级嵌套,需正确解析
  - card 顶部展示**父节点面包屑** (从顶层往下),每个父节点截断 20 字 (超过加 `...`)
- **文件监听**: 用户用 Typora/VS Code 改 `.md` 后保存,panel 检测变更后**重新计算**第一个未完成 `- [ ]`
- **Panel 内编辑**: 直接编辑整个 `.md` 文件,**text only,无 preview/WYSIWYG** (简单实现)

**外部编辑器同样有效**: Typora / VS Code 都行,panel 不是唯一入口。

### 2. Micro Todolist

**数据源**: `~/panel/todolists/micro/{YYYY-MM-DD}.md`

**行为**:
- 当天文件不存在则**自动新建**(空)
- 平铺所有 `- [ ]` (本来就少,1-3 条/日)
- panel 内可直接编辑 `.md` (text only,同主 todolist)

**每日复发 todo**:
- 入口: micro 模块内一个按钮 (位置直观即可) → 打开**模态框**管理
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
- 框架: SwiftUI macOS App
- 位置: `~/panel/mac/`
- 布局: Tab 或 Sidebar (UI 设计阶段定)
- 渲染: Markdown
- 编辑: text-only (无 WYSIWYG)
- 主题: 4 状态动态切换
- 全局快捷键: 后续 phase 加

### 数据持久化
- 用户可读改: Markdown 文件
- 结构化: SQLite
- 跨重启状态 (wake_time, anchored todolist, 当日 mode 等): 必须持久化

## Git 约定 (monorepo)

- conventional commits
- scope **至少二级**,三级 (具体功能) 视情况加,不强求
- 格式: `<type>(<area>-<module>[-<feature>]): ...`
- 例:
  - `feat(backend-保健品-提醒推送): ...`
  - `feat(backend-保健品): ...` (二级也可)
  - `feat(mac-todolist-聚焦卡片): ...`
  - `fix(mac-财务): ...`
- 跨层: `feat(both-记录): ...`
- 主分支: `dev`

## 文件夹布局

```
~/panel/
├── .gitignore
├── PRD.md                              ← 本文件
├── todolists/
│   ├── panel-construction.md           ← panel 自身的构建 todolist
│   └── micro/
│       └── YYYY-MM-DD.md               ← 当天临时事务
├── backend/                            ← Go 后端 (Phase 1+)
├── mac/                                ← SwiftUI macOS App (Phase 2+)
├── data/                               ← 运行时 panel.db (gitignored)
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
