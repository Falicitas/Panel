# HN 热帖消化 Checklist

三篇帖子的动手 checklist：Git 代码考古、Railway 去 Next.js 迁移、卡尔曼滤波。按兴趣挑着做，不用全过。

---

## Part A — Git Commands Before Reading Code (~25min)

原文：https://piechowski.io/post/git-commands-before-reading-code/

目标：把这套 git 考古命令在你自己的项目上跑一遍，形成接手新 codebase 的肌肉记忆。

### Phase 0 — 选一个够大的 repo (~2min)

- [ ] 找一个有至少一年提交历史的 repo — 可以用你自己的桃源天下或 LLM API Gateway，也可以用开源大项目（TanStack Router、Gin 之类的）
- [ ] `cd` 进去，确认 `git log --oneline -5` 能正常输出

### Phase 1 — 五个命令逐个跑 (~15min)

- [ ] Churn hotspot — `git log --format=format: --name-only --since="1 year ago" | sort | uniq -c | sort -nr | head -20`
  - 🔍 高频改动文件未必有问题，但如果高频改动 + 没人愿意 own = 拖慢整个团队的文件
- [ ] Bus factor — `git shortlog -sn --no-merges`
  - 再加 `--since="6 months ago"` 对比。如果 all-time 第一名最近半年消失了，说明维护者已经换人
  - 🔍 squash-merge 的 repo 会把 authorship 压缩到 merge 的人头上，这个命令的结果会失真
- [ ] Bug cluster — `git log -i -E --grep="fix|bug|broken" --name-only --format='' | sort | uniq -c | sort -nr | head -20`
  - 拿这个结果和 churn hotspot 交叉比对。两个列表都出现的文件 = 反复修反复坏
- [ ] Commit velocity — `git log --format='%ad' --date=format:'%Y-%m' | sort | uniq -c`
  - 看形状：稳定节奏是健康的，突然减半通常是有人离职，持续下滑 6-12 个月 = 项目在失去动量
- [ ] Firefighting frequency — `git log --oneline --since="1 year ago" | grep -iE 'revert|hotfix|emergency|rollback'`
  - 一年几次正常。每两周一次 = 部署流程有问题。零结果可能是团队稳定，也可能是 commit message 太随意

### Phase 2 — 整合判断 (~8min)

- [ ] 把 churn top 5 和 bug top 5 做个交集 — 写下来，这些是接手后第一批要读的文件
- [ ] 看 commit velocity 的图形，用一句话描述这个 repo 的节奏（比如"稳定双周发版"或"去年 Q3 后明显放缓"）
- [ ] 🤖 可以让 Claude Code 帮你跑这些命令并汇总，用 `git log` 输出 pipe 给它分析
- [ ] 💡 把这五个命令存成一个 shell script 或 alias，下次接手新 repo 直接跑

### 品味校准 (~5min)

- [ ] 这套方法论会火还是会死？作者定位是 codebase audit 咨询，这是个好切入点吗？
- [ ] 它解决的是真痛点还是伪需求？你接手过的项目里，这些信号是不是真的有用？
- [ ] 跟你现有的 repo 理解方式比，增量在哪？有没有这套命令覆盖不到的维度（比如架构层面）？
- [ ] DX 打分（1-5）：命令好不好记、输出好不好读、对不同规模 repo 的适用性
- [ ] 一句话总结：推荐还是劝退？

---

## Part B — Railway 去 Next.js 迁移 (~30min)

原文：https://blog.railway.com/p/moving-railways-frontend-off-nextjs

目标：理解 Railway 的迁移决策和具体收益，评估你自己的 Next.js 项目（如果有）是否也应该迁移。

### Phase 0 — 读原文，拆解决策链 (~10min)

- [ ] 读完全文，列出 Railway 迁出 Next.js 的具体原因（不要泛泛说"慢"，把每个点写下来）
  - 构建时间 10+ 分钟，其中 6 分钟是 Next.js 自己的 "finalizing page optimization"
  - 产品形态是 client-heavy（dashboard、canvas、WebSocket），server-first 的 primitives 用不上
  - 还在 Pages Router，迁 App Router 意味着走向 server-first，跟产品方向相反
  - Layout 都是 hack 出来的 workaround
- [ ] 列出迁移目标栈：Vite + TanStack Router
- [ ] 记下关键数据：迁移用了 2 个 PR，zero downtime，构建降到 2 分钟以内

### Phase 1 — 对照你自己的场景 (~10min)

- [ ] 你的 LLM API Gateway 前端（如果有 dashboard）和桃源天下，用的是什么构建工具？构建时间是多少？
- [ ] 列出你项目里 Next.js 的 server-side feature 使用情况（SSR、API Routes、Middleware）—— 如果大部分是 client-side，那情况跟 Railway 类似
- [ ] TanStack Router 的 type-safe routing 对你有没有吸引力？你现在的 routing 方案有什么痛点？
- [ ] 💡 如果你的项目纯 SPA + API 分离，Vite + TanStack Router 值得试。如果你依赖 SSR/ISR，留在 Next.js 可能合理

### Phase 2 — 动手摸一下 Vite + TanStack Router (~10min)

- [ ] `npm create vite@latest test-tanstack -- --template react-ts`，`cd test-tanstack && npm i`
- [ ] `npm i @tanstack/react-router`，跟着 TanStack Router 的 quickstart 跑通 file-based routing
- [ ] 对比 `npm run build` 的时间和 Next.js 项目的 `next build` 时间
- [ ] 🔍 TanStack Router 的 route params 和 search params 是 type-inferred 的，不需要手动定义类型。这个 DX 体验跟 Next.js 的 `useParams()` 比差别挺大

### 品味校准 (~5min)

- [ ] Railway 的迁移决策你认同吗？还是说他们只是选了一条阻力最小的路？
- [ ] "去 Next.js" 会成为趋势吗？还是说大部分团队会继续留在 Next.js 生态里？
- [ ] TanStack Router 跟 React Router v7 比，谁更适合你的场景？
- [ ] DX 打分（1-5）：TanStack Router 上手摩擦、文档质量、类型推导体验、生态成熟度
- [ ] 一句话总结：你会在下一个项目里用 Vite + TanStack Router 吗？

---

## Part C — 卡尔曼滤波（雷达追踪例子） (~40min)

原文：https://kalmanfilter.net/

目标：通过雷达追踪的例子理解卡尔曼滤波的 predict-update 循环，能手写一个 1D 的实现。跟你之前的 OpenClaw 信号检测有交集。

### Phase 0 — 建立直觉 (~10min)

- [ ] 打开 https://kalmanfilter.net/，从首页的雷达追踪例子开始读
- [ ] 搞清楚三个概念的物理含义：
  - measurement noise — 雷达每次测量都有误差（10 个雷达测同一架飞机会得到 10 个不同的距离值）
  - process noise — 运动模型不完美（假设匀速，但有风）
  - state estimate — 综合测量和预测得到的最优估计
- [ ] 🔍 卡尔曼滤波的核心 idea：两个不确定的信息（预测 + 测量）合并成一个更确定的估计。数学上就是两个高斯分布相乘

### Phase 1 — 跟着数值例子走一遍 (~15min)

- [ ] 跟着网站的 Kalman Filter in One Dimension 部分，手算（或者用 Python）走一遍 predict-update 循环
  - predict: x_predicted = F * x_previous, P_predicted = F * P * F^T + Q
  - update: K = P_predicted * H^T * (H * P_predicted * H^T + R)^-1, x = x_predicted + K * (z - H * x_predicted)
- [ ] 关注 Kalman Gain K 的行为：当 measurement noise 很大时 K 趋向 0（更信预测），当 process noise 很大时 K 趋向 1（更信测量）
- [ ] 💡 还有个经典教程 https://www.bzarg.com/p/how-a-kalman-filter-works-in-pictures/ 图画得好，可以交叉看

### Phase 2 — 写一个 1D 卡尔曼滤波 (~15min)

- [ ] 用 Python 实现一个 1D 卡尔曼滤波，tracking 一个匀速运动的目标
  - state = [position, velocity]，measurement = position only
  - 用 numpy 就够了，不需要 filterpy 之类的库
- [ ] 生成一组带噪声的位置测量数据，跑滤波器，画三条线：真实轨迹、带噪测量、滤波估计
- [ ] 调 Q 和 R 的比值，观察滤波器响应速度 vs 平滑度的 tradeoff
  - 🔍 Q/R 大 = 响应快但抖动大，Q/R 小 = 平滑但跟踪滞后。这个 tradeoff 在量化信号处理里也一样
- [ ] 🤖 可以让 Claude Code 帮生成测试数据和画图代码，但 predict/update 逻辑自己写

### Phase 3 — 连接到你的场景 (~5min)

- [ ] 想一下你的 OpenClaw 风口雷达系统里，趋势信号检测有没有可以用卡尔曼滤波的地方
  - 比如：对某个指标的趋势做 state estimation，measurement 是原始数据，process model 是趋势延续假设
- [ ] 💡 如果你在做股票相关的信号，卡尔曼滤波可以用来做 adaptive moving average（比 EMA 更灵活，因为 gain 是动态调整的）

### 品味校准 (~5min)

- [ ] kalmanfilter.net 这个教程站做得怎么样？跟 bzarg 那篇比，各有什么优势？
- [ ] 卡尔曼滤波在你的实际工作中有多大用武之地？是值得深入还是知道个大概就够？
- [ ] 跟你之前学的 HNSW、random projection 这些比，卡尔曼滤波的数学门槛在哪个位置？
- [ ] DX 打分（1-5）：教程清晰度、数学推导的可跟随性、代码实现的难度、实际应用距离
- [ ] 一句话总结：推荐还是劝退？