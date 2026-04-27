# 个人面板 Panel App 构建 Checklist

一句话说明: 从零搭建一个 Mac 本地的个人指挥中枢,覆盖 todo / 微 todo / 财务 / 食物保健品 / 记录 / bash / 状态主题 / (选做) 微信 QQ 红点;最终状态是打开一个 SwiftUI App 就能处理所有"看一眼+做一下"的事。

技术栈: SwiftUI + Go 后端 + SQLite + Markdown 文件 + (选做) wechat_chatter (OneBot) + (选做) NapCatQQ (OneBot)

前置: MBP M4、macOS SIP enabled、你是唯一用户、纯本地不联网

---

> **本文件结构说明**:
>
> - **Phase 0**: 环境准备。**完整保留**原 todolist 中你已写的笔记 (Frida 理解、insert_dylib + Mach-O / dmg-pkg-zip-AppStore 分发层等)
> - **Phase 1-8**: 新版构建主线。Panel 主体功能,不假设微信/QQ 存在
> - **Phase 9**: 微信/QQ 选做。**完整保留**原 todolist 中 Phase 1 你已写的内容 (含口播相关项),作为重启 hook 时的参考。实际 hook 通路在 2026-04-25 已经验证通,目前因 macOS TCC 暂停,详见 `~/.claude/projects/-Users-kinesis-panel/memory/project_phase1_progress.md`
> - **品味校准**: 原文件末尾的自检清单保留

---

## Phase 0 — 环境准备 (~30min)

目的:把 Frida、Xcode 命令行工具、Go 这些先装齐,一次性解决依赖问题。

- [x] 确认 Xcode Command Line Tools 已装 — `xcode-select -p`,没装就 `xcode-select --install`

- [x] 装 Frida — `pip install frida-tools`,验证 `frida --version` 应该返回 17.x

    - 🔍 理解点:Frida 是动态插桩框架,通过注入一个 gadget dylib 到目标进程,让你能用 JS 脚本在运行时 hook 任意函数

    - [x] 🌟🌟🌟🌟 Frida 是什么？我装的目的？验证什么:问 AI 能把这个问题解决。走得通:大体了解且拆分成了问题;走不通:大体不了解。

    - [x] 🌟🌟🌟 Frida 最简易的例子理解？

        -   [x] 🌟🌟 JS 注入:

            以下面的代码为例,在 ls 进程中写一个 hook:

            ```bash
            cat > /tmp/hook_ls.js <<'EOF'
            // 找 libc 里的 opendir 函数 —— ls 内部用它打开目录
            const opendir = Module.getExportByName('libSystem.B.dylib', 'opendir');

            Interceptor.attach(opendir, {
              onEnter: function(args) {
                // args[0] 是 opendir 的第一个参数,也就是路径字符串
                const path = args[0].readUtf8String();
                console.log('[Frida] ls 正在打开目录: ' + path);
              }
            });
            EOF

            ```

            ```bash
            ls 进程的内存空间
            ┌─────────────────────────────┐
            │  /bin/ls 自己的代码          │
            │  libSystem.B.dylib 的代码    │  ← opendir 住在这里
            │  其他系统库                   │
            │  堆、栈                      │
            └─────────────────────────────┘
            ```

            跑 `frida -f /bin/ls -l hook_ls.js -- ~` 之后,Frida 往这个进程里加了两样东西:

            ```bash
            ls 进程的内存空间
            ┌─────────────────────────────┐
            │  /bin/ls 自己的代码          │
            │  libSystem.B.dylib 的代码    │  ← opendir 住在这里
            │  其他系统库                   │
            │  frida-agent.dylib           │  ← 新加的,Frida 的运行时
            │  └─ JS 引擎                  │
            │  └─ 你的 hook_ls.js          │
            │  堆、栈                      │
            └─────────────────────────────┘

            ```

            相当于现在在同一个进程里。Interceptor.attach(opendir, {...}) 会在进程执行前调用,相当于监听有 opendir 实例后做注入。然后 onEnter(我设置的事件)执行位置如下:

            改之前:

            ```
            opendir 的入口:
              [原始指令 1]      ← CPU 从这里开始执行
              [原始指令 2]
              [原始指令 3]
              ...
            ```

            改之后:

            ```
            opendir 的入口:
              jmp frida_trampoline   ← 被 Frida 改了
              [原始指令 2]
              [原始指令 3]
              ...

            frida_trampoline:
              保存现场
              调用你 JS 里的 onEnter
              恢复现场
              执行原始指令 1(之前被覆盖了,Frida 在别处保存了一份)
              jmp 回到 opendir 的第 2 条指令
            ```

        -   [x] 找到函数:这是 wechat_chatter 作者用 IDA/Hopper 静态分析和 Frida 动态验证(运行时调 log 确认找对了)

        -   [x] 🌟🌟 运行时调用:

            普通 `frida -f` 模式

            ```
            frida 命令活着 ──── 目标进程活着 ──── JS 活着
                 ↓
            Ctrl-C 退出 → 进程死 → JS 跟着没了
            ```

            一次性的,适合调试和抓 trace。

            gadget 模式

            ```
            FridaGadget.dylib 被永久塞进 WeChat.app
                ↓
            微信每次启动,gadget 自动加载,27042 端口开着等连
                ↓
            你的 onebot 服务连上来,load 一次 text.js
                ↓
            text.js 在微信进程里一直活着,等 RPC 调用
                ↓
            onebot 退出 → text.js 还在微信里,下次 onebot 重连能继续用
                ↓
            微信退出 → 一切归零,下次微信启动重新来
            ```

            所以 wechat_chatter 的 onebot 进程是在使用 gadget 暴露的端口的一个 http 服务,使用 Frida 协议完成对 Frida agent 的双向数据交互,然后我通过 restful http 方式访问它,间接在面板完成访问。

            ```bash
            ┌───────────────────────────┐
            │  你的面板 Go 后端           │   层 4:业务层
            └────────┬──────────────────┘
                     │ HTTP REST (OneBot 协议语义)
                     │ POST /send_private_msg
                     ▼
            ┌───────────────────────────┐
            │  wechat_chatter 的 onebot  │   层 3:协议适配层
            │  (Go 进程,独立跑)           │   把 OneBot REST 转成 Frida RPC
            └────────┬──────────────────┘
                     │ Frida Wire Protocol
                     │ (TCP 27042,二进制协议)
                     ▼
            ┌───────────────────────────┐
            │  FridaGadget.dylib         │   层 2:运行时层
            │  (住在 WeChat 进程内)       │   内含 JS 引擎 + text.js
            └────────┬──────────────────┘
                     │ 直接函数调用 / 内存访问
                     │ (同进程,零成本)
                     ▼
            ┌───────────────────────────┐
            │  WeChat 的原生代码          │   层 1:目标应用
            │  微信自己的发消息函数        │
            └───────────────────────────┘
            ```

        -   [x] 不是一次性的处理

- [x] 装 Go 1.22+ — `brew install go`,验证 `go version`

- [x] 装 insert_dylib 工具 — 用来给微信二进制插入 dylib 引用

    ```
    git clone https://github.com/Tyilo/insert_dylib
    cd insert_dylib && xcodebuild
    sudo cp build/Release/insert_dylib /usr/local/bin/
    ```

    -   [x] insert_dylib 是什么:

        首先用 `otool -L` 能看到任何 Mach-O 二进制(iOS 的原生可执行文件,唯一)注册了哪些 dylib:

        >   **脚本**(`.sh`、`.py`、shebang 开头的)不是 Mach-O,是文本,靠解释器执行。但解释器本身(`/bin/bash`、`python`)是 Mach-O

        ```bash
        $ otool -L /bin/ls
    
        /bin/ls:
            /usr/lib/libutil.dylib (compatibility version 1.0.0, current version 1.0.0)
            /usr/lib/libncurses.5.4.dylib (compatibility version 5.4.0, current version 5.4.0)
            /usr/lib/libSystem.B.dylib (compatibility version 1.0.0, current version 1351.0.0)
        ```

        这三行就是 ls 二进制里的三条 `LC_LOAD_DYLIB` 记录。ls 启动时,dyld(macOS 的动态链接器)读这张清单,挨个加载。`insert_dylib` 的作用就是往这张清单里**加一行**,指向 FridaGadget.dylib。

        >   dmg、pkg、zip、App Store 这些是打包和分发层,跟 Mach-O 是两码事,可以分两层看。
        >
        >   一层是「可执行文件本身的格式」。只有 Mach-O。你在 Mac 上能跑的原生程序都是这个。
        >
        >   另一层是「这些文件怎么到你电脑上」。这里才有 dmg、pkg、zip、App Store 的区别。
        >
        >   dmg 是磁盘映像。双击之后 macOS 把它挂载成一个虚拟磁盘,你从里面把 .app 拖到 Applications 就完事了。.app 本身只是一个有固定目录结构的文件夹(叫 bundle),里面的 Contents/MacOS/ 下放着 Mach-O 二进制。dmg 只负责把这个文件夹从网上搬到你电脑上。
        >
        >   pkg 是安装包。双击会启动 macOS 内置的 Installer.app,按脚本把文件复制到系统各处,可能还会跑 pre/post 安装脚本,也可能装 LaunchDaemon、kernel extension 之类的系统级组件。比 dmg 重,适合需要改系统状态的软件。
        >
        >   zip 就是压缩文件。解压出来是 .app bundle 或者裸二进制,剩下跟 dmg 一样,你自己拖进 Applications。
        >
        >   App Store 走苹果自己的分发系统,背后下载下来的也是 .app bundle,但会额外带一个 _MASReceipt 文件作为正版凭证(就是你上次 ls 那个)。App Store 版还受沙盒限制,权限比 dmg 版小,而且苹果能推强制更新。
        >
        >   一句话,dmg/pkg/zip/App Store 是四种「运输方式」,最终落到你硬盘上的都是同一个东西:一个包含 Mach-O 二进制的 .app bundle。运输方式决定了谁有权改它、怎么更新它;Mach-O 决定了它怎么被 dyld 加载、怎么跑起来。
        >
        >   ```
        >   分发层      dmg / pkg / zip / App Store 下载
        >     │
        >     ▼
        >   应用层      .app(目录约定)
        >     │
        >     ▼
        >   执行层      Mach-O 二进制(CPU 真正跑的东西)+ dylib / framework
        >   ```

- [x] 🌟🌟 备份当前 Mac 微信 App — `cp -R /Applications/WeChat.app ~/WeChat.app.backup`

    - 🔍 接下来要改二进制,搞砸了有 backup 能一键恢复

- [x] 准备工作目录 — `mkdir ~/panel && cd ~/panel`,后面所有代码仓库都 clone 到这里

---

## Phase 1 — Go 后端骨架 + Todolist 核心 (~半天 ~ 1 天)

目的: 后端跑起来,能 read .md / 锚定 / 提取第一未完成 / 监听文件变化。REST 服务 :8089 跑通。这是 panel 的"心脏",剩下所有功能都通过它打开。

- [ ] 初始化项目 — `cd ~/panel && mkdir backend && cd backend && go mod init panel`
- [ ] 选 SQLite 驱动 — `modernc.org/sqlite` (纯 Go,无 cgo) 或 `mattn/go-sqlite3` (cgo)
  - 💡 个人项目选 modernc 更省事,不用配 C 工具链
- [ ] 设计核心数据结构:
  ```
  Todolist  { name, path, items: []TodoItem, raw_md: string }
  TodoItem  { line_no, level, text, checked, skip, parent_chain: []string }
  Anchor    { todolist_name | nil }
  ```
- [ ] Markdown 解析器 — 解析 `- [ ]` / `- [x]`,识别嵌套层级 (按缩进),识别 `[[[skip]]]` label
  - 🔍 理解点: panel 的 todolist 不需要完整 markdown AST,只要能取出所有 list item 的 (line_no, level, checked, skip, text) + 父链
- [ ] 文件监听 — `fsnotify` 监听 `~/panel/todolists/*.md` 改动,失效缓存
- [ ] REST API:
  - `GET /api/todolists` — 列出 todolists/ 下所有 .md (排除 micro/),按"完成态 (✅前缀)" 分组
  - `GET /api/todolists/:name` — 返回单个 todolist 的完整结构 + raw_md
  - `POST /api/anchor` body `{name | null}` — 设置/清空当前锚定
  - `GET /api/anchor` — 返回当前锚定 (持久化在 SQLite)
  - `GET /api/focus` — 返回锚定 todolist 的"第一个非 skip 的未完成 - [ ]" + 父节点面包屑(每个截断 20 字)
  - `PUT /api/todolists/:name` body `{raw_md}` — 写入整个 .md (panel 内编辑用)
- [ ] `MessageSource` interface + mock 实现 — 提前为 Phase 9 留好接口
  ```
  interface MessageSource {
    Source() string  // "wechat" | "qq"
    Subscribe(ctx, ch chan Message)
    Send(target, content) error
    UnreadCount() int
  }
  ```
  - mock 实现: 返回固定 0 未读,Send 打 log
- [ ] launchd plist (放后,Phase 8 才装) — 现在直接 `go run .` 即可
- [ ] 验证: `curl http://127.0.0.1:8089/api/todolists` 返回包含 `panel-construction.md`

---

## Phase 2 — SwiftUI Panel MVP + Todolist UI (~1-2 天)

目的: macOS App 跑起来,显示 todolist 列表 + 锚定 + 渲染 + 聚焦卡片 + 编辑。

- [ ] Xcode 新建 macOS App `~/panel/mac/Panel.xcodeproj` (SwiftUI + Swift)
- [ ] `APIClient` struct — 封装 `http://127.0.0.1:8089` 调用
- [ ] 主界面 NavigationSplitView:
  - 左 sidebar: todolists 列表 (✅前缀的归到"已完成"分组),点击锚定
  - 右 detail: 渲染锚定 todolist 的 markdown + 顶部聚焦卡片
- [ ] **聚焦卡片** UI:
  - 顶部父节点面包屑 (每个 20 字截断,超过加 ...)
  - 主体: 第一个非 skip 未完成 `- [ ]` 的内容
  - 单独的卡片样式,视觉上突出
- [ ] **Markdown 渲染** — 用 `MarkdownUI` 或 `swift-markdown-ui` (建议 native 库,避免 WebView)
- [ ] **Markdown 编辑** — 一个简单的 `TextEditor` (text only,无 preview/WYSIWYG),保存时 PUT 整个 .md 给后端
- [ ] 文件外部修改的实时刷新 — APIClient 用 polling (3s) 或 SSE (后续优化)
- [ ] 冒烟测试: 在 Typora 改 `panel-construction.md` 加一个 `- [ ]`,panel 看到更新;在 panel 里编辑保存,Typora 也能看到

---

## Phase 3 — Micro Todolist + 复发 todo 模态框 (~半天)

目的: 当日临时事务管理 + 复发 todo 配置 (注入逻辑放 Phase 5,这里只准备 schema 和 UI)。

- [ ] 后端 API:
  - `GET /api/micro/today` — 当日 micro 文件,不存在则**自动新建空文件**
  - `PUT /api/micro/today` body `{raw_md}` — 写入
  - `GET/POST/PUT/DELETE /api/recurring` — 复发 todo 的 CRUD
- [ ] DB schema: `recurring_todos (id, content, created_at)`
- [ ] 前端 micro tab:
  - 当日文件渲染 + 编辑 (复用 Phase 2 的 markdown 编辑组件)
  - 一个按钮(图标即可) → 打开复发 todo **模态框**
- [ ] 模态框 UI: 列出所有复发 todo + 添加/删除按钮
  - 💡 当天 micro 文件中的复发项**不能直接删**,只能勾选 / 加 [[[skip]]]。永久删除走模态框
- [ ] 验证: 模态框加 2 条复发 todo;触发"普通模式"注入逻辑(占位,Phase 5 才接上);手动新建一个 micro 文件(空)能正常打开

---

## Phase 4 — 财务管理 (~1 天)

目的: 三类支出录入 + 当前余额 + 债务膨胀指标。

- [ ] DB schema:
  ```
  bills          (id, name, monthly_amount, total_months, started_at, remaining_months_calc)
  fixed_expenses (id, name, monthly_amount, started_at)
  free_expenses  (id, date, amount, note)
  incomes        (id, date, amount, note)
  finance_settings (initial_balance, free_cap_monthly DEFAULT 1000, n_months_assumed, annual_rate_assumed DEFAULT 0.12)
  ```
- [ ] 后端 API:
  - `GET /api/finance/summary` — 当前余额 / 当月预期支出 / 债务膨胀布尔
  - `GET/POST/PUT/DELETE /api/finance/bills`
  - `GET/POST/PUT/DELETE /api/finance/fixed`
  - `GET/POST/PUT/DELETE /api/finance/free`
  - `GET/POST /api/finance/incomes`
  - `PUT /api/finance/settings`
- [ ] 月末结算逻辑 — 每月 1 日(或当月最后一日 EOD)把 bills/fixed 自动累计到本月总支出
- [ ] 债务膨胀计算:
  ```
  expected = sum(fixed) + sum(free 当月) + sum(bill 当月分摊)
  shortfall a = max(0, expected - 当月收入)
  N = settings.n_months_assumed (默认 12)
  rate = settings.annual_rate_assumed (0.12)
  inflated = (当月收入 < expected + a * rate / N)
  ```
- [ ] 前端财务 tab — 余额卡片 / 当月支出列表 / 三类支出管理 / 债务膨胀指示灯

---

## Phase 5 — 食物保健品 + 全局通知栏 + 4 状态主题 + Wake 蒙版 (~1-2 天)

目的: 一次性把状态切换、晨起仪式、提醒系统、主题色这一组绑在一起的功能做完。这一 Phase 是 panel 的"灵魂",做完之后日常使用就成立了。

- [ ] DB schema:
  ```
  supplements (id, name, type [食物|保健品], windows_json, notes)
  daily_state (date, mode [普通|专注|睡眠|外出], wake_time, mode_set_at)
  notifications (id, source, title, body, level [info|warn|alert], created_at, dismissed_at)
  state_settings (focus_start TIME DEFAULT '17:00', sleep_start TIME DEFAULT '22:30')
  ```
- [ ] **Wake 蒙版逻辑** (后端):
  - 后端定时器每分钟检查: 当日是否已选 mode? 当前时间 >= 06:00? → 状态 = "等待选择"
  - 前端轮询 `/api/state/today` 收到"等待选择"时显示蒙版
  - 蒙版有两个按钮 (普通 / 外出),POST `/api/state/today` 设置当日 mode,记 wake_time
- [ ] **复发 todo 注入** — 普通模式按钮被点时,把 `recurring_todos` 列表注入到当天 micro 文件顶部
- [ ] **状态切换 + 主题** (前端):
  - 4 状态轮询 (1s) `/api/state/now` → 返回当前 effective mode (按优先级 睡眠 > 外出 > 专注 > 普通 计算)
  - 主题色 binding:
    - 普通 → default
    - 专注 → 柔和橙
    - 睡眠 → 淡紫
    - 外出 → 蓝
- [ ] **专注模式 label 通知** — 专注状态时通知栏显示 1-2 条 "手机飞行模式 / 退出微信 QQ / 不开视频站"
- [ ] **睡眠模式静音** — 睡眠状态时通知栏不弹新通知 (内部仍记录到 notifications 表,只是 UI 不弹)
- [ ] **食物保健品**:
  - `GET/POST/PUT/DELETE /api/supplements`
  - 时间窗口表达式解析: 支持 `HH:MM` / `wake` / `wake+1h` 等 (`wake` 来自 daily_state.wake_time)
  - 后端定时器每分钟检查"现在落在哪个 supplement 的窗口起点附近?未完成的就生成通知"
- [ ] **全局通知栏**:
  - `GET /api/notifications/active` — 未 dismiss 的通知
  - `POST /api/notifications/:id/dismiss`
  - 前端固定位置一条 banner,显示最高优先级的未 dismiss 通知
- [ ] 设置面板(简陋版): 只暴露 focus_start / sleep_start 两个时间字段
- [ ] 验证: 早上 6 点蒙版弹出 → 选普通 → 复发 todo 注入 → 17:00 切橙色 → 22:30 切淡紫并静音

---

## Phase 6 — 记录模块 + 晨起握力 + 食物保健品月度展示 (~半天)

目的: 通用 record framework + 第一个特化 (晨起握力) + 食物保健品的"当月完成情况"借用展示位。

- [ ] DB schema:
  ```
  records (id, module, date, key, value_num, value_text, created_at)
    -- module: 'grip_strength' | 'supplement_completion' | (后续其他)
    -- key: 'left' | 'right' | 'completed' 等
  ```
- [ ] 晨起握力:
  - `POST /api/records/grip` body `{date, left, right}` (一日内 PUT 同一 date 即覆盖)
  - `GET /api/records/grip/today` — 今日值 + 14 天 M / SD + 偏离 + 分类 (正常/降强度/有氧休息)
- [ ] 食物保健品月度完成情况:
  - 当 supplement 通知被 dismiss 时记一条 `records (supplement_completion, date, supplement_id, completed=true)`
  - `GET /api/records/supplements/month` — 返回每个 `<name, type, 完成天数/应完成天数>`
- [ ] 前端记录 tab — 晨起握力卡 + 食物保健品月度卡

---

## Phase 7 — Bash 入口 (~2-3h)

目的: panel 内执行 zsh 命令,免切 Terminal。

- [ ] 后端 `POST /api/exec` body `{cmd}` → `{stdout, stderr, exit_code, duration_ms}`
  - 实现: `exec.Command("zsh", "-i", "-c", cmd)` —— `-i` 是 interactive,触发 source `~/.zshrc`
  - 🔍 理解点: 不用 `-i` 的话 zsh 不会 source rc,你的 alias / PATH 偏好都没了
  - 💡 注意: `-i` 模式有时会有 stderr 噪音(zsh 启动 banner),需要 strip 或忽略
- [ ] DB schema: `bash_history (id, cmd, stdout, stderr, exit_code, duration_ms, created_at)`
- [ ] 前端 bash tab — 极简 REPL: 输入框 (Cmd+Enter 提交) + 输出区 + 上下方向键翻历史

---

## Phase 8 — 稳定性与兜底 (~半天)

目的: 让这玩意儿能用上半年,不是一周。

- [ ] launchd plist `~/Library/LaunchAgents/panel.backend.plist` — 开机启动后端
- [ ] SQLite 自动备份 — launchd 定时任务每天 03:00 备份到 `~/panel/backups/YYYY-MM-DD.db` (保留 30 天)
- [ ] 后端日志 — 写 `~/panel/logs/backend.log`,按天 rotate
- [ ] 错误聚合到通知栏 — 后端 panic / 文件读写失败 等关键错误生成 `level=alert` 通知,panel 顶部显示
- [ ] 健康检查 — `GET /api/health` 返回各模块状态 (DB ok / 文件监听 ok / fsnotify ok 等)

---

## Phase 9 — (选做,推后) 微信/QQ 红点聚合

> ⚠️ **本 Phase 内容是从原 todolist Phase 1 完整搬运的**(含一些和此项目无直接关系的笔记,保留原状)。
>
> 实际状态: 微信 4.1.7.57 hook 通路在 2026-04-25 已经验证通,目前因 macOS TCC 重签后所有授权失效(尤其是麦克风,语音通话弹窗风暴)而**暂停使用**,微信回滚到 4.1.9 (未 hook)。
>
> 重启时机: panel 主体 Phase 1-8 完成、用熟之后再回来做。  
> 重启关键: 装回 4.1.7.57 → 跑 `tccutil reset All com.tencent.xinWeChat` → 一次性手动批准所有权限(尤其麦克风/摄像头) → 走下面的步骤。
>
> 详细踩坑记录: `~/.claude/projects/-Users-kinesis-panel/memory/project_phase1_progress.md`

目的: 让 Mac 微信能被外部程序 send/recv 文本消息。这一步最难,做完剩下的都是常规活。

- [x] clone wechat_chatter — `git clone https://github.com/yincongcyincong/wechat_chatter ~/panel/wechat_chatter`

- [x] 查当前微信版本 — 在微信"设置 → 关于微信"看版本号,对照 release 页面确认有对应 JS 脚本

    - 💡 如果版本太新没适配,降级微信到 release 支持的版本,或直接 watch 这个仓库

- [x] 下载对应版本的 mac 微信。验证什么:107 版本的 wx-chatter 能不能用到 106 版本的 wx 上;走得通:可以;走不通:不行。ok 了,下载了微信 4.1.7.57,wechat_chatter 的[v0.0.8](https://github.com/yincongcyincong/wechat_chatter/releases/tag/v0.0.8) 支持这个版本。

- [ ] 下载 frida-gadget — 从 Frida release 下载 macOS universal dylib

    ```
    cd ~/panel
    wget https://github.com/frida/frida/releases/download/17.5.2/frida-gadget-17.5.2-macos-universal.dylib.xz
    xz -d frida-gadget-17.5.2-macos-universal.dylib.xz
    ```

- [ ] 注入 gadget 到微信 — 按顺序执行:

    ```
    cp frida-gadget-17.5.2-macos-universal.dylib /Applications/WeChat.app/Contents/Frameworks/FridaGadget.dylib
    chmod +x /Applications/WeChat.app/Contents/Frameworks/FridaGadget.dylib
    cd /Applications/WeChat.app/Contents/MacOS/
    /usr/local/bin/insert_dylib --inplace --strip-codesig "@executable_path/../Frameworks/FridaGadget.dylib" WeChat
    ```

    - 🔍 理解点:`insert_dylib` 往 Mach-O 二进制的 Load Commands 里加一条 LC_LOAD_DYLIB,让 WeChat 启动时自动加载 FridaGadget,这样你就能外部 attach 进去了
    -

- [ ] 跑 sign.sh 重签名 — `cd ~/panel/wechat_chatter && ./frida-gadget/sign.sh`

    - 🔍 理解点:改完二进制原签名失效,macOS 会拒绝启动。ad-hoc 重签名用自签名证书绕过这个检查,但 app 就不再能走 App Store 分发了——对自用无所谓
    - ⚠️ **注意 (2026-04-25 实测)**: Sequoia 下 `sign.sh` 自带的 `--entitlements` 和 `-o runtime` 都会让 launchd 拒绝启动 (POSIX 153)。**正确命令**: `codesign --deep --force --sign - --timestamp=none /path/to/WeChat.app`,无 entitlements 无 runtime。

- [ ] 拷贝 gadget 配置 — `cp frida-gadget/FridaGadget.config /Applications/WeChat.app/Contents/Frameworks/`

- [ ] 启动微信,验证 gadget 在监听 — 启动微信后 `lsof -i :27042`,应该看到 WeChat 进程在监听

    - 💡 如果微信启动没反应:`sudo codesign --force --deep --sign - /Applications/WeChat.app`

- [ ] 🌟🌟🌟🌟 确认口播出问题的地方。走得通:确认完毕;走不通:没确认完。结束:没找完。第一次确认:方向没有问题,要看,但是没看完。目前估摸着还要有个 30 ～ 60 分钟,只考虑 2 个视频即可。第二次确认:视频是看完了。但是资料有点乱,显然目前不适合呈现给别人看。验证什么:整理成 .md 会不会清楚一些?:走得通:清楚了,走不通:还不清楚。第一次确认:方向可以,不过估计要写多两次 30 分钟。第二&第三次确认:不止,起码还要有 60 分钟来弄。第四次确认:弄完了,至少是文本的编写。

  -   封前 15 分钟应该开始爆破,有中文。"冲啊 美服玩家 朝伟报道""老A呢 开下爆率啊 大半天了东西都没有"。国服破解玩家
  -   如果我的铁蹄在向前一步,这个螳臂当车的火毒,它能抵挡我这种不要钱的抢怪方式吗。完了直播间要没了。
  -   手握核弹按钮(?)
  -   第二个视频
  -   开头:你们都知道的理由:被爆破了,然后有利益方来封直播间。
  -   体验:沉浸感,挡
  -   有很多想法真的没有办法实现,这里指的是技术层面的,比如游戏样式的弹幕机
  -   被超管封了一分钟
  -   之前被封过一次,说了不该说的东西被爆了
  -   今天被爆了是公屏

  - [ ] - `
  - [ ] 看看小红书如何处理?验证什么:看解封经验。走得通:有:走不通:都是主播去弄

- [ ] 跑一次发送测试 — 先给文件传输助手发一条测试消息

  ```
  cd ~/panel/wechat_chatter
  frida -H 127.0.0.1:27042 -n Gadget -l ./frida/text.js
  # 在 frida REPL 里:
  triggerSendTextMessage(0x20000095, "filehelper", "hello from panel")
  ```
  - 验证:手机微信上文件传输助手应该收到这条消息
  - ⚠️ **注意**: 实测发现 frida CLI REPL 与 Frida 17.x gadget listen mode 不兼容,直接跑 onebot 验证更可靠。详见 memory。

- [ ] 启动 onebot HTTP 服务 — 按 wechat_chatter/onebot/readme.md 编译并跑起来,默认监听 `127.0.0.1:58080`
  - ⚠️ 编译需要 frida-core-devkit,见 memory 中的 CGO 环境变量配置

- [ ] curl 测试 OneBot 接口 — `curl -X POST http://127.0.0.1:58080/send_private_msg -d '{"user_id":"filehelper","message":"hello from onebot"}'`
  - 验证:文件传输助手再次收到消息
  - ⚠️ 首次调用会失败,因为 `triggerX1Payload` 要先被 WeChat 内部 MMStartTask 调用 hook 一次。等 log 出 `[+] 捕获到 StartTask 调用` 再 curl

### Phase 9 接入到 Panel 的步骤 (在 hook 通路通了之后)

- [ ] 后端写一个 OneBot 适配器 (`MessageSource` 接口的 wechat 实现)
  - WS / HTTP polling 拉 onebot 进来的消息
  - Send 时 POST 到 onebot
- [ ] QQ 用 NapCatQQ 同样模式 (qq 实现)
- [ ] 前端加一个"消息" tab,展示未读列表
- [ ] 设置: 启用/禁用 wechat / qq

---

## (取消) 搜索

不做。Panel 用熟 1-2 周后再评估是否需要。原 Phase 7 内容废弃。

---

## 品味校准 (~5min)

做完 Phase 1-5 MVP 之后,在继续后面之前自己填这份:

- [ ] 生死判断 — 这个自建面板一个月后你还在用吗?三个月后呢?
- [ ] 需求真伪 — "面板即一切红点"是真痛点,还是你只是想折腾?如果改用 Raycast + 微信原生客户端,能解决多少?
- [ ] 核心设计决策 — 你选了 SwiftUI + Go 双进程,而不是全 Electron 或全 Swift,这个 tradeoff 一个月后还认同吗?(纠结点:Go 后端长期跑的价值 vs Swift 单进程的简洁性)
- [ ] DX 打分(1-5)— 开发体验(Xcode vs VSCode 频繁切换)、wechat_chatter 踩坑程度、OneBot 协议清晰度、SQLite 是否够用
- [ ] 一句话总结 — 推荐给另一个有相似需求的朋友,你会怎么描述这套方案的适用边界?
