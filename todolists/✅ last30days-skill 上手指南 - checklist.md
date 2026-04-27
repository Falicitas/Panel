# last30days-skill 上手 Checklist

从零配置跑通免费数据源，到读懂多源编排的设计思路。不注册任何第三方付费服务。

前置条件：Claude Code 已装，Python 3 可用，macOS。

手动读代码时统一用 marketplace 源码路径：`~/.claude/plugins/marketplaces/last30days-skill/`。这个路径不带版本号，update 不会失效。

---

## Phase 0 — 安装 + 零配置验证 (~5min)

装完就能跑。Reddit 公共 JSON、Hacker News、Polymarket 三个源不需要任何 API key。

- [x] 安装 skill — 在 Claude Code 里依次输入：
    ```
    /plugin marketplace add mvanhorn/last30days-skill
    /plugin install last30days@last30days-skill
    ```
    
- [x] 🌟🌟🌟🌟 跑诊断看数据源状态 — 在 Claude Code 里输入 `/last30days setup`，它会显示各数据源的可用状态

    -   验证什么：真实运行可行与否；走得通：可行；走不通：不行

        🌵 点评：x 和 youtube 都不通，这俩用的是 nodeJS 脚本，fetch 不读 http_proxy。总体是一个美国用没问题，没考虑好代理环境的项目。

- [x] 跑一次真实查询 — `/last30days Claude Code skills`，等 1-2 分钟看输出
  - 🔍 注意输出结构：来源标记（r/xxx、HN）、评分、时间、engagement 数据

---

## Phase 1 — 解锁免费数据源 (~10min)

把不花钱的源全部打通。X/Twitter 走浏览器 cookie，YouTube 走 yt-dlp，Bluesky 走免费 app password。

- [ ] 解锁 X/Twitter — `/last30days setup` 的 wizard 会自动从浏览器提取 cookie
  - 要求：Chrome/Firefox/Safari 里已登录 X。cookie 只在内存中使用，不落盘
- [ ] 解锁 YouTube — 终端跑 `brew install yt-dlp`，装完后 /last30days 自动识别
- [ ] 💡 解锁 Bluesky — 去 bsky.app/settings/app-passwords 创建 app password，写入配置：
    ```bash
    mkdir -p ~/.config/last30days
    cat >> ~/.config/last30days/.env << 'EOF'
    BSKY_HANDLE=你的handle.bsky.social
    BSKY_APP_PASSWORD=xxxx-xxxx-xxxx-xxxx
    EOF
    chmod 600 ~/.config/last30days/.env
    ```
- [ ] 再跑一次 `/last30days setup` 确认所有免费源已打通

---

## Phase 2 — 体验不同意图模式 (~15min)

SKILL.md 定义了五种意图（PROMPTING / RECOMMENDATIONS / NEWS / COMPARISON / GENERAL），挑两个差异大的跑一下。都在 Claude Code 里输入。

- [ ] NEWS 意图 — `/last30days latest on Chinese AI models`
  - 观察：输出侧重时间线和事件，不会生成 prompt
- [ ] COMPARISON 意图 — `/last30days DeepSeek vs Qwen`
  - 观察：三轮并行研究 + side-by-side 对比表，耗时最长
  - 🔍 对比两次输出的结构差异——同一个引擎，不同意图走完全不同的合成路径

---

## Phase 3 — 理解数据管道 (~30min)

读代码。重点是 discovery → enrich → normalize → score → dedupe → render 这条管道。以下路径前缀均为 `~/.claude/plugins/marketplaces/last30days-skill`。

- [ ] 读 SPEC.md 拿到全局视角 — `cat ~/.claude/plugins/marketplaces/last30days-skill/SPEC.md`
  - 🔍 orchestrator 怎么协调各模块，数据流向是什么
- [ ] 读评分算法 — `cat ~/.claude/plugins/marketplaces/last30days-skill/scripts/lib/score.py`
  - 🔍 公式：`0.50 × log1p(score) + 0.35 × log1p(comments) + 0.05 × (ratio×10) + 0.10 × log1p(top_comment_score)`
  - 时间衰减曲线怎么设计的，engagement 跨平台怎么归一化
- [ ] 读去重逻辑 — `cat ~/.claude/plugins/marketplaces/last30days-skill/scripts/lib/dedupe.py`
  - 🔍 近似去重用的 trigram Jaccard 还是 token Jaccard？阈值多少？
- [ ] 读 orchestrator 主流程 — `cat ~/.claude/plugins/marketplaces/last30days-skill/scripts/last30days.py`
  - 跟着 main 函数走一遍完整流程，对照 SPEC.md 的架构图验证
- [ ] 读 SKILL.md — `cat ~/.claude/plugins/marketplaces/last30days-skill/SKILL.md`
  - 🔍 五种意图的判定关键词列表在哪？"FORMAT MATCHES RESEARCH" 的反模式是什么意思？

---

## Phase 4 — CLI 参数和输出格式 (~10min)

需要手动跑脚本对比输出差异。也可以在 Claude Code 里让它跑这些命令。

- [ ] 对比三种 emit 模式的输出差异：
    ```bash
    python3 ~/.claude/plugins/marketplaces/last30days-skill/scripts/last30days.py "test topic" --mock --emit=json
    python3 ~/.claude/plugins/marketplaces/last30days-skill/scripts/last30days.py "test topic" --mock --emit=compact
    python3 ~/.claude/plugins/marketplaces/last30days-skill/scripts/last30days.py "test topic" --mock --emit=context
    ```
  - 🔍 context 模式是给 Claude Code 注入上下文用的，这个文本怎么被消费的
- [ ] 试单源模式 — `--sources=reddit` 和 `--sources=x` 分别跑同一个 topic，对比社区讨论的差异
- [ ] 看自动保存的研究文件 — `ls ~/Documents/Last30Days/`，每次 /last30days 跑完自动存 .md

---

## Phase 5 — 品味校准 (~5min)

- [ ] 这个项目会持续发展还是会停滞？19k star 的增长轨迹、commit 频率、contributor 数量分别说明什么
- [ ] 它解决的问题有没有更简单的替代方案？Claude Code 自带的 web search、Perplexity、手动搜 Reddit 分别能覆盖多少？
- [ ] 多源编排的设计（并行请求 → 统一 schema → 评分排序 → 去重）对你的 LLM API Gateway 有什么借鉴？特别是 normalize + score 这两步
- [ ] DX 打分（1-5）：安装摩擦、配置复杂度、报错信息、文档质量
- [ ] 一句话总结：推荐还是劝退？