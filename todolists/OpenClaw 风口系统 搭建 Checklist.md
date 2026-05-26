# OpenClaw 风口系统 搭建 Checklist

从现有的 OpenClaw（Mac mini M2 + Telegram channel）出发，搭一套"自动采集 → 交叉分析 → 每日推送"的 pipeline。
最终状态：每天早上 9 点在 Telegram 收到一条风口信号摘要，包含新信号、升温中信号、以及跟你技能栈匹配的建议。

***

## Phase 0 — workspace 目录结构 + 依赖确认 (\~15min)

在现有 OpenClaw workspace 下建好雷达系统的文件骨架。

* [ ] 确认 OpenClaw 运行正常 — 在 Telegram 给你的 agent 发条消息，确认能收到回复

* [ ] 创建 radar 目录结构 — 在 workspace 下建：
  ```
  radar/
  ├── raw/          # 每日原始采集数据，按日期+源命名
  ├── signals/      # 被标记的信号文件，每个信号一个 md
  ├── daily/        # 每日 digest 汇总
  └── config/       # 配置文件：信号源列表、过滤规则、品味锚点名单
  ```

* [ ] 创建 `radar/config/sources.md` — 列出所有要抓的源和对应 URL/API endpoint（后续 Phase 逐个填）

* [ ] 创建 `radar/config/filters.md` — 定义噪音过滤规则，初始版本：
  * GitHub: 排除 star > 10000 的老项目、awesome-list、dotfiles、profile README

  * HN: 排除 Ask HN、政治话题、Show HN 以外的 points < 50 的帖子

  * Reddit: 排除 karma < 10 的帖子

* [ ] 创建 `radar/config/taste-anchors.md` — 列出你的"品味锚点"名单（20-30 个你认可的 builder），每人一行，格式：`名字 | Twitter handle | 关注理由`
  * 🔍 选人标准是"反复做出好东西的一线 builder"，不是粉丝多的大 V。比如某个开源项目作者、某个独立开发者

***

## Phase 1 — 第一个采集 skill：GitHub Trending (\~30min)

先跑通一个源，验证整条 pipeline 能走通。

* [ ] 🤖 创建 skill `skill-github-trending` — 让 Claude Code 帮你写，逻辑：
  * 用 browser skill 或 `curl` 抓 `https://github.com/trending?since=weekly`

  * 解析 HTML 提取：repo 全名、description、language、total stars、today stars

  * 也可以用非官方 API：`https://api.gitterapp.com/repositories?since=weekly`（更稳定）

  * 输出写到 `radar/raw/YYYY-MM-DD-github.md`，markdown table 格式

* [ ] 手动触发一次 — 在 Telegram 跟 agent 说"跑一下 github trending 采集"，确认 raw 文件生成了

* [ ] 检查输出 — 打开生成的 md 文件，看 star 增速数据是否合理，description 是否被正确提取

* [ ] 加上过滤逻辑 — 把 `config/filters.md` 里的 GitHub 规则应用上，重跑一次确认噪音被过滤掉了

***

## Phase 2 — 补齐其余采集 skills (\~1h)

逐个添加数据源，每加一个就跑一次验证。

* [ ] 🤖 `skill-hn-frontpage` — 用 HN 官方 Firebase API：
  * `https://hacker-news.firebaseio.com/v0/topstories.json` 取 top 30

  * `https://hacker-news.firebaseio.com/v0/showstories.json` 取 Show HN 前 10

  * 对每个 story 再请求 `/v0/item/{id}.json` 拿 title、url、score、time、descendants（评论数）

  * 计算 points/hour 比率用于排序

  * 输出 `radar/raw/YYYY-MM-DD-hn.md`

* [ ] 🤖 `skill-producthunt-daily` — PH 有 GraphQL API（需要 developer token）：
  * 不想申请 token 的话，用 browser skill 抓 `https://www.producthunt.com/` 当日 top 5

  * 输出 `radar/raw/YYYY-MM-DD-ph.md`

* [ ] 🤖 `skill-huggingface-trending` — 抓两个页面：
  * `https://huggingface.co/models?sort=trending` 前 10

  * `https://huggingface.co/spaces?sort=trending` 前 10

  * 提取 model/space 名、作者、likes、描述

  * 输出 `radar/raw/YYYY-MM-DD-hf.md`

* [ ] 🤖 `skill-reddit-signals` — 用 Reddit JSON API（不需认证）：
  * `https://www.reddit.com/r/LocalLLaMA/hot.json?limit=10`

  * `https://www.reddit.com/r/SideProject/hot.json?limit=10`

  * `https://www.reddit.com/r/Entrepreneur/hot.json?limit=10`

  * 提取 title、score、num\_comments、url

  * 输出 `radar/raw/YYYY-MM-DD-reddit.md`

* [ ] 💡 `skill-xitter-builders`（可选，后续加）— Twitter 抓取最不稳定，三个方案：
  * A：用 Nitter 实例（如果还有存活的）抓 RSS feed

  * B：用 browser skill 打开品味锚点名单的 Twitter 主页，截取最新 5 条

  * C：先不做，用其他 5 个源跑起来再说

* [ ] 逐个手动触发每个 skill — 确认每个 raw 文件格式正确、数据合理

***

## Phase 3 — 分析 skill：交叉信号检测 (\~45min)

整个系统里最值得花时间调的部分。

* [ ] 🤖 创建 `skill-radar-analyze` — prompt 设计：
  * 读取当天所有 `radar/raw/YYYY-MM-DD-*.md` 文件

  * Step 1: 对每个条目提取 1-3 个概念标签（抽象概念，如 "character consistency for AI video""local-first database""browser agent"，不要复述标题）

  * Step 2: 交叉检测 — 同一概念在 2+ 个不同源出现 → 标记为信号

  * Step 3: 读取 `radar/signals/` 下已有的信号文件，判断是"新信号"还是"已有信号在升温"

  * Step 4: 读取你的 memory/背景信息，判断哪些信号跟你的技能栈（React/TS/Go/Python/LLM API）对得上

  * 输出两种文件（见下面两步）

* [ ] 定义 daily digest 格式 — 输出到 `radar/daily/YYYY-MM-DD-digest.md`：
  ```
  # 风口雷达 YYYY-MM-DD

  ## 🔴 新信号
  - [概念] — 出现在 [源1] + [源2]，一句话描述

  ## 🟡 升温中（连续 N 天）
  - [概念] — 强度 N/5，数据点

  ## 🟢 建议上手（匹配你的技能栈）
  - [概念] — 为什么匹配 + 建议切入点

  ## 📊 今日原始数据概览
  - GitHub: N 个项目，top 3 列出
  - HN: N 条，top 3 列出
  - ...
  ```

* [ ] 定义 signal 文件格式 — 每个新信号创建 `radar/signals/signal-[概念slug].md`：
  ```
  # Signal: [概念名]

  首次出现：YYYY-MM-DD
  当前强度：N/5
  状态：新发现 | 升温中 | 持续热门 | 降温

  ## 来源记录
  - [YYYY-MM-DD] GitHub: repo-name (★N, +N/day)
  - [YYYY-MM-DD] HN: "标题" (N points)

  ## 我的判断
  （留空，你手动填）
  ```

* [ ] 手动触发分析 skill — 确保已有当天的 raw 数据，跑一次看 digest 和 signal 文件是否合理
  * 🔍 分析 skill 的 prompt 质量决定了这套系统有没有用。调优重点在"概念标签提取"这步——提取太粗（都是"AI tool"），交叉检测就没意义；太细（精确到项目名），跨源信号就检测不到

***

## Phase 4 — Telegram 推送 + cron 调度 (\~30min)

把 pipeline 串起来，每天自动跑。

* [ ] 创建 `skill-radar-push` — 读取当天 digest 文件，格式化为 Telegram 消息（控制在一屏以内），发送
  * 消息模板：

    ```
    🦞 风口雷达 YYYY-MM-DD

    🔴 新: [概念A]（GitHub+HN）| [概念B]（HF+Reddit）
    🟡 升温: [概念C] 4/5 连续3天 | [概念D] 3/5
    🟢 建议上手: [概念E] → 你可以用 [技术] 做 [什么]

    回复"详情 [概念]"看完整信号 | 回复"todolist [概念]"生成上手清单
    ```

  * 🔍 最后两个交互命令是重点——在 Telegram 里直接触发深入了解或生成 todolist，"发现 → 判断 → 上手"一条线走完

* [ ] 创建 `skill-radar-detail` — 回复"详情 XX"时，读取对应 signal 文件，返回完整信息

* [ ] 创建 `skill-radar-todolist` — 回复"todolist XX"时，用 learning-todolist skill 的格式，针对该项目/技术生成上手 checklist

* [ ] 配置 HEARTBEAT.md 或 cron — 在 HEARTBEAT.md 里添加：
  ```
  - [ ] 每天 07:00 执行所有 radar 采集 skills（github、hn、ph、hf、reddit）
  - [ ] 每天 08:30 执行 radar-analyze
  - [ ] 每天 09:00 执行 radar-push 推送到 Telegram
  ```
  * heartbeat 精度不够（默认 30min 间隔）的话，改用 OpenClaw 的 cron tool 精确调度

* [ ] 跑一个完整周期 — 手动触发采集 → 等一会 → 触发分析 → 触发推送，确认端到端 OK

***

## Phase 5 — 降噪调优 + 成本控制 (\~20min)

跑几天后回来做。

* [ ] 回顾前 3-5 天的 digest — 标记哪些信号有价值、哪些是噪音，找噪音的共同特征

* [ ] 更新过滤规则 `config/filters.md` — 根据实际噪音追加规则，比如：
  * 过滤反复出现但没用的关键词（如 "awesome-list"、"tutorial"、"boilerplate"）

  * 调整各源的 threshold（HN 的 points 下限、GitHub 的 star 增速下限）

* [ ] 成本审计 — 查过去几天 LLM API 的消耗，主要看分析 skill 的 token 用量
  * 超预期的话，把采集层的初步格式化交给便宜模型（Haiku 或 local Ollama），只有交叉分析用 Sonnet/Opus

  * 或者缩减每个源的采集数量（Reddit 从 10 条降到 5 条之类的）

* [ ] 💡 加上周度回顾 — 每周日额外跑一个 `skill-radar-weekly`，汇总本周所有信号的生命周期变化，输出周报到 `radar/weekly/YYYY-WNN.md`

***

## Phase 6 — 闭环：信号 → 判断 → 验证 (\~15min)

建立反馈回路。

* [ ] 在每个 signal 文件的"我的判断"区域写你的看法 — 每次看到新信号时花 30 秒写一句：会火还是会死？为什么？

* [ ] 设置月度 review 提醒 — 日历上标每月 1 号，回看上月所有 signal 文件，对比你的判断和实际情况
  * 在 signal 文件里追加：`[YYYY-MM-DD 回顾] 判断正确/错误，原因：...`

* [ ] 💡 建判断准确率 log — 在 `radar/` 下创建 `accuracy-log.md`，每月记录：判断了 N 个信号，正确 M 个，准确率 M/N，主要错在什么方向上
  * 🔍 这个 log 就是品味提升的量化指标。三个月后能看到预测准确率在升还是停，以及你在哪类判断上系统性犯错（比如总是高估 infra 项目、低估 C 端应用）

***

## 品味校准 (\~5min)

* [ ] 这套"自动化信号捕捉"系统，能比手动刷资讯提前多久发现风口？如果只是提前一两天，维护成本值不值？

* [ ] 最大风险是什么？信号太多导致麻木？还是过滤太狠漏掉机会？

* [ ] 自动化程度 vs 信号质量怎么权衡。全自动的分析肯定不如你自己看，但你没时间每天花两小时刷各种源。平衡点在哪？

* [ ] 三个月后这套系统的理想状态是什么？一句话

