# 个人面板 Panel App 构建 Checklist

一句话说明:从零搭建一个 Mac 本地的个人指挥中枢,整合微信/QQ 未读、todo、bash、搜索;最终状态是打开一个 SwiftUI App 就能处理所有红点。

技术栈:SwiftUI + Go 后端 + SQLite + wechat_chatter (OneBot) + NapCatQQ (OneBot)

前置:MBP M4、macOS SIP enabled、你是唯一用户、纯本地不联网

---

## Phase 0 — 环境准备 (~30min)

目的:把 Frida、Xcode 命令行工具、Go 这些先装齐,一次性解决依赖问题。

- [x] 确认 Xcode Command Line Tools 已装 — `xcode-select -p`,没装就 `xcode-select --install`

- [ ] 装 Frida — `pip install frida-tools`,验证 `frida --version` 应该返回 17.x
  - 🔍 理解点:Frida 是动态插桩框架,通过注入一个 gadget dylib 到目标进程,让你能用 JS 脚本在运行时 hook 任意函数

  - [x] 🌟🌟🌟🌟 Frida 是什么？我装的目的？验证什么：问 AI 能把这个问题解决。走得通：大体了解且拆分成了问题；走不通：大体不了解。

  - [x] 🌟🌟🌟 Frida 最简易的例子理解？

      -   [x] 🌟🌟 JS 注入：

          以下面的代码为例，在 ls 进程中写一个 hook：

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

          跑 `frida -f /bin/ls -l hook_ls.js -- ~` 之后，Frida 往这个进程里加了两样东西：

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

          相当于现在在同一个进程里。Interceptor.attach(opendir, {...}) 会在进程执行前调用，相当于监听有 opendir 实例后做注入。然后 onEnter（我设置的事件）执行位置如下：

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

      -   [x] 找到函数：这是 wechat_chatter 作者用 IDA/Hopper 静态分析和 Frida 动态验证（运行时调 log 确认找对了）

      -   [x] 🌟🌟 运行时调用：

          普通 `frida -f` 模式

          ```
          frida 命令活着 ──── 目标进程活着 ──── JS 活着
               ↓
          Ctrl-C 退出 → 进程死 → JS 跟着没了
          ```

          一次性的，适合调试和抓 trace。

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

          所以 wechat_chatter 的 onebot 进程是在使用 gadget 暴露的端口的一个 http 服务，使用 Frida 协议完成对 Frida agent 的双向数据交互，然后我通过 restful http 方式访问它，间接在面板完成访问。

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

  -   [x] insert_dylib 是什么：

      首先用 `otool -L` 能看到任何 Mach-O 二进制（iOS 的原生可执行文件，唯一）注册了哪些 dylib：

      >   **脚本**(`.sh`、`.py`、shebang 开头的)不是 Mach-O,是文本,靠解释器执行。但解释器本身(`/bin/bash`、`python`)是 Mach-O

      ```bash
      $ otool -L /bin/ls
      
      /bin/ls:
      	/usr/lib/libutil.dylib (compatibility version 1.0.0, current version 1.0.0)
      	/usr/lib/libncurses.5.4.dylib (compatibility version 5.4.0, current version 5.4.0)
      	/usr/lib/libSystem.B.dylib (compatibility version 1.0.0, current version 1351.0.0)
      ```

      这三行就是 ls 二进制里的三条 `LC_LOAD_DYLIB` 记录。ls 启动时，dyld(macOS 的动态链接器)读这张清单，挨个加载。`insert_dylib` 的作用就是往这张清单里**加一行**,指向 FridaGadget.dylib。

      >   dmg、pkg、zip、App Store 这些是打包和分发层，跟 Mach-O 是两码事，可以分两层看。
      >
      >   一层是「可执行文件本身的格式」。只有 Mach-O。你在 Mac 上能跑的原生程序都是这个。
      >
      >   另一层是「这些文件怎么到你电脑上」。这里才有 dmg、pkg、zip、App Store 的区别。
      >
      >   dmg 是磁盘映像。双击之后 macOS 把它挂载成一个虚拟磁盘，你从里面把 .app 拖到 Applications 就完事了。.app 本身只是一个有固定目录结构的文件夹（叫 bundle），里面的 Contents/MacOS/ 下放着 Mach-O 二进制。dmg 只负责把这个文件夹从网上搬到你电脑上。
      >
      >   pkg 是安装包。双击会启动 macOS 内置的 Installer.app，按脚本把文件复制到系统各处，可能还会跑 pre/post 安装脚本，也可能装 LaunchDaemon、kernel extension 之类的系统级组件。比 dmg 重，适合需要改系统状态的软件。
      >
      >   zip 就是压缩文件。解压出来是 .app bundle 或者裸二进制，剩下跟 dmg 一样，你自己拖进 Applications。
      >
      >   App Store 走苹果自己的分发系统，背后下载下来的也是 .app bundle，但会额外带一个 _MASReceipt 文件作为正版凭证（就是你上次 ls 那个）。App Store 版还受沙盒限制，权限比 dmg 版小，而且苹果能推强制更新。
      >
      >   一句话，dmg/pkg/zip/App Store 是四种「运输方式」，最终落到你硬盘上的都是同一个东西：一个包含 Mach-O 二进制的 .app bundle。运输方式决定了谁有权改它、怎么更新它；Mach-O 决定了它怎么被 dyld 加载、怎么跑起来。
      >
      >   ```
      >   分发层      dmg / pkg / zip / App Store 下载
      >      │
      >      ▼
      >   应用层      .app(目录约定)
      >      │
      >      ▼
      >   执行层      Mach-O 二进制(CPU 真正跑的东西)+ dylib / framework
      >   ```

- [x] 🌟🌟 备份当前 Mac 微信 App — `cp -R /Applications/WeChat.app ~/WeChat.app.backup`

  - 🔍 接下来要改二进制,搞砸了有 backup 能一键恢复

- [x] 准备工作目录 — `mkdir ~/panel && cd ~/panel`,后面所有代码仓库都 clone 到这里

---

## Phase 1 — 微信 hook 打通 (~60min)

目的：让 Mac 微信能被外部程序 send/recv 文本消息。这一步最难，做完剩下的都是常规活。

- [x] clone wechat_chatter — `git clone https://github.com/yincongcyincong/wechat_chatter ~/panel/wechat_chatter`

- [x] 查当前微信版本 — 在微信"设置 → 关于微信"看版本号,对照 release 页面确认有对应 JS 脚本

    - 💡 如果版本太新没适配，降级微信到 release 支持的版本,或直接 watch 这个仓库

- [x] 下载对应版本的 mac 微信。验证什么：107 版本的 wx-chatter 能不能用到 106 版本的 wx 上；走得通：可以；走不通：不行。ok 了，下载了微信 4.1.7.57，wechat_chatter 的[v0.0.8](https://github.com/yincongcyincong/wechat_chatter/releases/tag/v0.0.8) 支持这个版本。
  
- [x] 下载 frida-gadget — 从 Frida release 下载 macOS universal dylib
  
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
  
- [ ] 拷贝 gadget 配置 — `cp frida-gadget/FridaGadget.config /Applications/WeChat.app/Contents/Frameworks/`

- [ ] 启动微信,验证 gadget 在监听 — 启动微信后 `lsof -i :27042`,应该看到 WeChat 进程在监听
  - 💡 如果微信启动没反应:`sudo codesign --force --deep --sign - /Applications/WeChat.app`
  
- [ ] 🌟🌟🌟🌟 确认口播出问题的地方。走得通：确认完毕；走不通：没确认完。结束：没找完。第一次确认：方向没有问题，要看，但是没看完。目前估摸着还要有个 30 ～ 60 分钟，只考虑 2 个视频即可。第二次确认：视频是看完了。但是资料有点乱，显然目前不适合呈现给别人看。验证什么：整理成 .md 会不会清楚一些？：走得通：清楚了，走不通：还不清楚。第一次确认：方向可以，不过估计要写多两次 30 分钟。第二&第三次确认：不止，起码还要有 60 分钟来弄。第四次确认：弄完了，至少是文本的编写。 
  
  -   封前 15 分钟应该开始爆破，有中文。“冲啊 美服玩家 朝伟报道”“老A呢 开下爆率啊 大半天了东西都没有”。国服破解玩家
  -   如果我的铁蹄在向前一步，这个螳臂当车的火毒，它能抵挡我这种不要钱的抢怪方式吗。完了直播间要没了。
  -   手握核弹按钮（？）
  -   第二个视频
  -   开头：你们都知道的理由：被爆破了，然后有利益方来封直播间。
  -   体验：沉浸感，挡
  -   有很多想法真的没有办法实现，这里指的是技术层面的，比如游戏样式的弹幕机
  -   被超管封了一分钟
  -   之前被封过一次，说了不该说的东西被爆了
  -   今天被爆了是公屏
  
  - [ ] - `
  - [ ] 看看小红书如何处理？验证什么：看解封经验。走得通：有：走不通：都是主播去弄
  
- [ ] 跑一次发送测试 — 先给文件传输助手发一条测试消息
  
  ```
  cd ~/panel/wechat_chatter
  frida -H 127.0.0.1:27042 -n Gadget -l ./frida/text.js
  # 在 frida REPL 里:
  triggerSendTextMessage(0x20000095, "filehelper", "hello from panel")
  ```
  - 验证:手机微信上文件传输助手应该收到这条消息
  
- [ ] 启动 onebot HTTP 服务 — 按 wechat_chatter/onebot/readme.md 编译并跑起来,默认监听 `127.0.0.1:58080`

- [ ] curl 测试 OneBot 接口 — `curl -X POST http://127.0.0.1:58080/send_private_msg -d '{"user_id":"filehelper","message":"hello from onebot"}'`
  - 验证:文件传输助手再次收到消息

---

## Phase 2 — QQ 接入 (~30min)

目的:NapCatQQ 跑起来,和微信一样暴露 OneBot 接口给后端。

- [ ] 下载 NapCatQQ Mac 版 — 从 release 找 MAC DMG(注意 Mac 版本号可能落后 Windows 几个版本,可接受)
- [ ] 安装并扫码登录 QQ — 按 WebUI 指引
- [ ] 在 NapCat WebUI 配置网络 — 开启 HTTP Server (端口自选,如 `3001`) 和正向 WebSocket Server (如 `3002`)
  - 🔍 理解点:NapCat 支持正向/反向 WS 和 HTTP,自用选正向 WS 最简单——你后端主动连 NapCat 就行,不用暴露端口
- [ ] 验证 QQ 能收发 — curl NapCat 的 `/send_private_msg` 给自己一个小号发消息
- [ ] 对比两端 OneBot 接口字段 — 记下微信和 QQ 在 `user_id`/`group_id`/消息事件结构上的细微差异
  - 🤖 让 Claude Code 读两边的 OneBot 事件样例,生成一份"字段差异对照表"存到 `~/panel/docs/onebot-diff.md`

---

## Phase 3 — Go 后端骨架 (~半天)

目的:一个进程把两边消息聚合,存 SQLite,REST 给前端用。

- [ ] 初始化项目 — `cd ~/panel && mkdir backend && cd backend && go mod init panel`
- [ ] 设计核心数据结构 — `Message { ID, Source (wx/qq), ChatID, ChatName, SenderID, SenderName, Content, Timestamp, IsRead, IsSelfSent }`
  - 🤖 直接让 Claude Code 生成 schema、migration、CRUD,你审 schema 定义就够了
- [ ] 写 OneBot client 抽象 — 一个 interface,两个 impl (WeChatClient、QQClient)
  - 🔍 理解点:OneBot 协议统一了"消息事件"结构,但具体字段有差异,抽象层要在这里归一化
- [ ] WS 客户端连 NapCatQQ — 订阅消息事件,写入 SQLite,标记 `is_read=false`
- [ ] WS 客户端(或 HTTP 轮询)连 wechat_chatter — 看项目实际暴露的事件通道
  - 💡 如果 wechat_chatter 没有 WS 推送,只能轮询 `/get_new_messages` 这种接口,加个 1-2s 间隔
- [ ] 实现 REST API 给前端用:
  - `GET /api/unread` — 返回按会话聚合的未读列表
  - `GET /api/messages?chat_id=X&limit=50` — 某会话历史
  - `POST /api/reply` — body `{source, chat_id, content}`,转发给对应 OneBot 后端
  - `POST /api/mark_read` — body `{chat_id}`
- [ ] 起服务验证 — `go run .`,curl 各接口应该都能工作
- [ ] 加个 launchd plist 开机启动 — `~/Library/LaunchAgents/panel.backend.plist`
  - 🤖 让 Claude Code 生成 plist,你检查路径对不对

---

## Phase 4 — SwiftUI 面板 MVP (~1-2 天)

目的:单窗口 App,左边未读列表,右边会话详情,底部输入框。

- [ ] Xcode 新建 macOS App 工程 — SwiftUI + Swift,叫 `Panel`
- [ ] 建一个 `APIClient` struct — 封装 `http://localhost:8080` 的调用
- [ ] 主界面 `NavigationSplitView` — 左侧 sidebar 显示未读会话列表,右侧 detail 显示选中会话
- [ ] 会话 cell 样式 — 图标区分微信/QQ,显示会话名 + 最后一条摘要 + 未读数
- [ ] 消息气泡视图 — 自己发的靠右、别人发的靠左,时间戳显示
- [ ] 输入框 — 底部 `TextField`,按 Cmd+Enter 发送,调 `/api/reply`
- [ ] 轮询或 WS 推送 — MVP 先 3s 轮询 `/api/unread`,WS 放 P2
  - 🔍 理解点:SwiftUI 的 `Timer.publish` 配合 `@StateObject` 做轮询最省心,WS 要自己管重连
- [ ] "打开即已读"行为 — 点击会话后调 `/api/mark_read`,未读红点消失
- [ ] 编译打包成 `.app` — `xcodebuild archive` 或直接 Run,拖到 Applications
- [ ] 冒烟测试 — 从外面给自己微信/QQ 发消息,面板能看到并能回

---

## Phase 5 — Todo 模块 (~2-3h)

目的:本地 todo 管理,区分"今日主 todo"和"临时事务"两类。

- [ ] 后端加 Todo 表 — `{ID, Content, Category (main/ad_hoc), Status, CreatedAt, DueAt, CompletedAt}`
- [ ] Todo CRUD 的 REST API — list/create/update/complete/delete
- [ ] 前端 Todo Tab — 顶部切换 Main/Ad-hoc,列表展示 + 加号按钮新建
- [ ] 快捷新建 — 全局快捷键 `Cmd+Shift+T` 弹出输入框快速加 todo
  - 💡 用 `NSEvent.addGlobalMonitorForEvents` 实现全局快捷键
- [ ] "打开面板即看到红点"逻辑 — 未完成的 ad_hoc todo 数 + 微信/QQ 未读数,汇总成 dock badge

---

## Phase 6 — Bash 输入 (~2h)

目的:在面板里跑命令,不用切到 Terminal。

- [ ] 后端加 `/api/exec` — body `{cmd}`,返回 `{stdout, stderr, exit_code}`
  - 🔍 理解点:`exec.Command("bash", "-c", cmd)` 就行,但要注意环境变量——直接 spawn 的 shell 没有你 Terminal 的 PATH
- [ ] 环境变量处理 — 后端启动时 source `~/.zshrc` 或者直接把常用工具路径硬编码
- [ ] 前端 Bash Tab — 输入框 + 输出区,类似一个极简 REPL
- [ ] 命令历史 — 上下方向键翻历史,存到 SQLite 的 `bash_history` 表

---

## Phase 7 — 搜索 (~3h)

目的:一个搜索框搜 todo + 消息历史 + bash 历史。

- [ ] 后端加 `/api/search?q=X` — 先用 SQLite FTS5 做全文索引,不要上来就 ES
  - 🔍 理解点:FTS5 是 SQLite 内建的全文检索虚拟表,用 `CREATE VIRTUAL TABLE ... USING fts5(content)` 建,查询性能对个人数据量完全够用
- [ ] 给 messages/todos/bash_history 各建一个 FTS5 索引表 + trigger 自动同步
- [ ] 前端顶部搜索框 — `Cmd+K` 触发,结果分组展示(Messages / Todos / Commands)
- [ ] 结果点击跳转 — 消息跳到对应会话,todo 跳到 Todo tab 高亮,bash 历史 copy 到剪贴板

---

## Phase 8 — 稳定性与兜底 (~半天)

目的:让这玩意儿能用上半年,不是一周。

- [ ] 微信 hook crash 自动重启 — 监控 WeChat 进程,异常退出后自动重启并重新 attach gadget
- [ ] SQLite 每天自动备份 — cron 或 launchd 定时任务,备份到 `~/panel/backups/YYYY-MM-DD.db`
- [ ] OneBot 连接断线重连 — backend 的 WS client 要有指数退避重连
- [ ] 日志系统 — 后端所有错误写 `~/panel/logs/backend.log`,面板顶部加个小红点提示"后端有错误"
- [ ] 微信版本升级 checklist — 写一份 `UPGRADE.md`,记录每次微信升级后要重做哪些步骤(重新 insert_dylib + 重签名 + 可能更新 JS 脚本)
  - 🔍 这是长期维护成本最高的一块,写清楚流程能节省未来每次几十分钟的查阅时间

---

## 品味校准 (~5min)

做完 Phase 1-4 MVP 之后,在继续后面之前自己填这份:

- [ ] 生死判断 — 这个自建面板一个月后你还在用吗?三个月后呢?
- [ ] 需求真伪 — "面板即一切红点"是真痛点,还是你只是想折腾?如果改用 Raycast + 微信原生客户端,能解决多少?
- [ ] 核心设计决策 — 你选了 SwiftUI + Go 双进程,而不是全 Electron 或全 Swift,这个 tradeoff 一个月后还认同吗?(纠结点:Go 后端长期跑的价值 vs Swift 单进程的简洁性)
- [ ] DX 打分(1-5)— 开发体验(Xcode vs VSCode 频繁切换)、wechat_chatter 踩坑程度、OneBot 协议清晰度、SQLite 是否够用
- [ ] 一句话总结 — 推荐给另一个有相似需求的朋友,你会怎么描述这套方案的适用边界?

