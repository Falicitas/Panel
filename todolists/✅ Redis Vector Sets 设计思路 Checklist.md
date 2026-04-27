# Redis Vector Sets 设计思路 Checklist

从零理解 Antirez 为什么要做 vector sets、怎么做的、和已有方案有什么区别。不需要跑 Redis，纯读代码和文档。

前置知识：知道 Redis 基本操作（GET/SET），其他从头补。

---

## Phase 0 — 补课：Redis Sorted Sets (~15min)

Vector sets 的 API 设计直接脱胎于 sorted sets，不理解后者就看不懂前者的设计意图。

- [x] 🌟🌟🌟🌟 搞清楚 sorted sets 的数据模型 — 每个元素有一个 string member 和一个 float score，按 score 排序。就像一个自动排序的 map<string, float>
- [x] 记住四个命令就够了：
  - `ZADD leaderboard 100 "alice"` — 插入元素，score 是 100
  - `ZRANGE leaderboard 0 9 WITHSCORES` — 取排名前 10
  - `ZRANGEBYSCORE leaderboard 80 100` — 取 score 在 80-100 之间的
  - `ZCARD leaderboard` — 返回元素总数
- [x] 理解 sorted sets 为什么能被玩出花 — 它就是一个"带数字标签的有序集合"，但开发者用它做了 leaderboard（score=分数）、rate limiter（score=时间戳）、delay queue（score=执行时间）、timeline（score=发布时间）。数据结构足够简单，用法由开发者定义
  - 🔍 这就是 Antirez 的设计哲学：给原语不给方案。后面 vector sets 延续的是同一个思路

---

## Phase 1 — 补课：KNN 和 HNSW (~20min)

Vector sets 底层是 HNSW 算法做近似最近邻搜索，需要知道它在干什么。

- [x] 先搞清楚 KNN（K-Nearest Neighbors）在向量搜索里的含义 — 有一堆向量存在数据库里，给一个查询向量，找出跟它最相似的 K 个。"相似"一般用 cosine similarity 衡量：两个向量方向越接近，值越接近 1

- [x] 理解暴力搜索为什么不行 — 假设有 100 万个 1536 维向量，每次查询要跟所有向量算一次 cosine similarity，太慢了。所以需要近似最近邻（ANN）算法，牺牲一点精度换速度

- [x] 🌟🌟🌟🌟 HNSW (Hierarchical Navigable Small World) 的直觉理解，不用看论文，记住三件事就行：
  - 它是一个多层的图。layer 0 包含所有节点，每往上一层节点数指数级减少（类似跳表）shau
  
  - 搜索从最顶层开始，在稀疏层快速定位大致区域，然后逐层下降到 layer 0 做精细搜索
  
  - 每个节点跟 M 个邻居建连接（M 是参数），M 越大图越稠密，搜索越准但内存越大
  
      🌵 点评：HNSW：选一个 $l$。$l$ 由指数衰退概率 $\lfloor -\ln(\operatorname{rand}()) \cdot \mathrm{mL} \rfloor$ 分配，其中 $\mathrm{mL} = \frac{1}{\ln(\mathrm{M})}$。向量 $p$ 作为节点，在 $\mathrm{layer}_{0,...,l}$ 存在，在这些层与 $\mathrm{M}$ 个邻居建立双向边。每个节点的度维持至多 $\mathrm{M}$ 个。从最高 $\mathrm{layer}$ 的 entry point（第一个在最高 $\mathrm{layer}$ 建的点）开始，有 $|S|$ 个点，$e \in S$，$T$ 是 $e$ 的邻居集合，找到一个 $t \in T$ 在 ${T, e}$ 中与 $p$ 距离最近，迭代至无更新。最后的 $t$ 作为下一级的 entry point。到了 $l$ 层及以下，开始 $\mathrm{EF}$ 束的束搜索，即 $t$ 是最优的 $\mathrm{EF}$ 个。由于 $\mathrm{EF}$ 一般大于等于 $\mathrm{M}$，所以从 $\mathrm{EF}$ 个中选 $\mathrm{M}$ 个（使得 $\mathrm{M}$ 个邻居之间尽可能远离）。
  
      特殊一点在于 $\mathrm{layer}_0$ 每个点至多 $2\mathrm M$ 条边，论文是这么写的。
  
      >   Ps：数学命名原则如下：
      >
      >   `M`、`mL`、`EF` — 算法的超参数/常数
      >
      >   `l`、`p`、`e`、`t`、`S`、`T` — 变量（每次运行会变）
      >
      >   `layer`、`rand` — 函数/结构名称，用 mathrm 和 operatorname。
      >
      >   `entry point` — 术语
  
- [x] 记住两个调参旋钮，后面看 vector sets 会反复出现：
  - M — 每个节点的最大连接数，影响内存和搜索质量
  - EF（exploration factor）— 搜索时探索的候选节点数，EF 越大越慢但 recall 越高
  
- [x] 🔍 "recall" 在这个语境下指的是：ANN 返回的 top-K 结果里，有多少是真正的 top-K（跟暴力扫描的结果对比）。recall 0.95 意味着 100 个结果里有 95 个是对的

---

## Phase 2 — 搞清楚 vector sets 在解决什么问题 (~15min)

把 vector sets 和 Redis 已有的 Redis Query Engine (FT.SEARCH) 做向量搜索区分开。

- [x] 读 Antirez 的博客原文关于 vector sets 的段落 — 搜 "Vector sets / 向量集合" 那一节就够了。抓住一个点：他是从 sorted sets 类比出发的，score 换成 vector

- [x] 🌟🌟🌟🌟 读 Redis 官方公告 https://redis.io/blog/announcing-vector-sets-a-new-redis-data-type-for-vector-similarity/ — 只看前半部分，搞清楚 vector sets 和 Redis Query Engine 的定位区别
  - 🔍 Redis Query Engine (FT.CREATE + FT.SEARCH) 是 schema-based 的，你要先定义 index schema，适合大规模结构化查询。Vector sets 是 schema-free 的，一个 key 就是一个向量索引，没有 FT.CREATE 这一步
  
  - vector sets 是 sorted sets 中将 score 替换为向量。存的时候 VADD，查的时候 VSIM。
  
      -   量化：默认 FP 32 -> 8bit 整数，可以无量化或者二值量化（1bit）。
      
      -   支持降维：使用随机矩阵投影，JL 引理作为理论低维可容忍的失真比例（只跟向量数量和容忍的失真比例有关。），比如 100 万个向量，10% 的失真容忍，那降到 256 / 512 维度以上都是安全的，不管原维度是 1536 还是 25600。
      
          >   Johnson-Lindenstrauss 引理给了一个公式：目标维度 $k \geq \frac{C \cdot \ln(n)}{\varepsilon^2}$，其中 $n$ 是数据点的数量，$\varepsilon$ 是你能容忍的距离失真比例，$C$ 是一个常数。
      
      -   过滤：每个 member 向量可以挂 JSON 属性，查询时按条件过滤（保证查询数量（比如满足条件的 topK）与方便，而 filter 越严格，性能越差）
  
- [x] 在脑子里过一遍这个类比：`ZADD` → `VADD`，`ZRANGEBYSCORE` → `VSIM`，`ZCARD` → `VCARD` — sorted sets 用数字 score 做范围查询，vector sets 用向量做相似度查询，API 设计一脉相承

---

## Phase 3 — 过一遍完整命令集 (~20min)

知道 vector sets 能干什么、不能干什么。

- [x] 🌟🌟🌟🌟 打开 https://redis.io/docs/latest/develop/data-types/vector-sets/ — 扫一遍命令列表，不用细看参数，先建立全局印象

    验证什么：是否扫一遍没东西需要深挖；走得通：是；走不通：否

- [x] 把命令分三类记下来：
  - 写入类：`VADD`、`VREM`、`VSETATTR`
  - 查询类：`VSIM`、`VEMB`、`VGETATTR`、`VISMEMBER`、`VRANDMEMBER`、`VRANGE`
  - 元信息类：`VCARD`、`VDIM`、`VINFO`、`VLINKS`
  
- [x] 重点看 VADD 的参数 — `REDUCE`（随机投影降维）、`Q8/BIN/NOQUANT`（量化选项）、`CAS`（线程化插入）、`EF`（构建时探索因子）、`M`（HNSW 连接数）、`SETATTR`（JSON 属性）
  - 🔍 这些参数全部是 HNSW 算法的调参旋钮，Antirez 把它们直接暴露给用户，而不是藏在配置文件里。这就是他说的 "let the developer have a role"
  
- [x] 重点看 VSIM 的参数 — `ELE` vs `VALUES` vs `FP32`（三种查询方式）、`EF`（搜索时探索因子）、`FILTER`（属性过滤）、`TRUTH`（精确线性扫描，用来做 benchmark 对照）、`EPSILON`（提前终止阈值）
  - 🔍 `TRUTH` 参数的存在说明 Antirez 在设计时就考虑了让用户自己测 recall，而不是让你信任默认值

---

## Phase 4 — 读 HNSW 实现的设计决策 (~30min)

理解 Antirez 为什么从头写 HNSW 而不用现有库。

- [ ] 读 GitHub 上的 README https://github.com/redis/redis/blob/unstable/modules/vector-sets/README.md — 重点看 threading model 那一节
  - 🔍 VSIM 默认在后台线程跑（最多 32 并发线程），VADD 在主线程跑但 CAS 模式下候选搜索阶段可以线程化。这个设计是为了不阻塞 Redis 的单线程事件循环
- [x] 注意 VREM 的实现细节 — README 里提到它不做 tombstone/逻辑删除，而是直接回收内存。对比其他 HNSW 实现（比如 hnswlib）通常用 tombstone 标记删除
  - 🔍 直接回收内存意味着长时间运行的应用不会有内存膨胀问题，但实现复杂度更高
- [x] 看量化策略 — 默认 Q8（int8 量化），可选 BIN（二值量化，更快更省内存但 recall 下降）和 NOQUANT（无量化）。回忆一下 Antirez 博客里提到他专门做了 8-bit 量化的 benchmark
- [x] 看 REDUCE 参数的随机投影 — 对高维向量（1024+维）可以做降维，但查询时仍然用全尺寸向量。这是在插入时做一次性计算换取持续的内存节省

---

## Phase 5 — 理解 "Redis way" 的设计哲学 (~15min)

搞清楚为什么 Antirez 不做 hybrid search、不做 schema。

- [x] 对比两种使用方式：
  - Redis Query Engine：`FT.CREATE idx ON HASH SCHEMA title TEXT embedding VECTOR HNSW 6 TYPE FLOAT32 DIM 1536 DISTANCE_METRIC COSINE` → 先定义 schema，再插入数据，再查询
  - Vector sets：`VADD movies VALUES 1536 0.1 0.2 ... "The Matrix"` → 直接插入，key 本身就是索引
- [x] 想一下 sorted sets 的历史 — 当年也没人预料到 sorted sets 会被用来做 rate limiter、leaderboard、timeline、delay queue 这些花样。Antirez 赌的是给一个足够简单的原语，开发者自己会组合出用法
- [x] 注意 FILTER 的实现方式 — 用 VSETATTR 给元素挂 JSON 属性，VSIM 时用表达式过滤。这比 Redis Query Engine 的结构化 metadata filtering 轻量得多，但也意味着过滤不走索引
  - 🔍 这就是 tradeoff：简单 API vs 过滤性能。对小数据集或过滤选择性高的场景够用，大规模复杂过滤场景还是得用 Redis Query Engine
- [x] 读一下 VLINKS 命令的说明 — 它能返回一个元素在 HNSW 图每一层的邻居。这种"把内部数据结构暴露给用户"的做法很 Redis：你可以用它来 debug recall 问题，或者理解图的连通性

---

## Phase 6 — 看源码的关键部分 (~20min)

不需要读完，挑着看。

- [x] 🌟🌟🌟🌟 打开 https://github.com/redis/redis/blob/8.2.3/modules/vector-sets/vset.c — 直接搜 `VSIM_RedisCommand` 函数，看它怎么解析参数、分发到线程

    验证什么：该代码值不值得读；走得通：值得；走不通：过于复杂，或者没意义

- [x] 搜 `vectorSetInsert`，看插入流程：维度校验 → 量化 → HNSW 图更新

- [x] 🤖 让 Claude 帮你读 vset.c 里的 threading model 代码，重点问：主线程和后台线程之间怎么同步的？`in_use` 这个读写锁的设计意图是什么？

- [x] 💡 如果对 C 不熟也没关系，重点看函数签名和注释里的设计意图，不用逐行理解实现

---

## Phase 7 — 拆解 Antirez 对 RAG 的判断 (~15min)

Antirez 在博客里夹了一段括号话，说 RAG 不一定是向量搜索最重要的应用。这个判断值得单独想。

- [x] 先还原他的论点链条：
  - 前提 1：模型上下文窗口在快速变长（GPT-4 128K → Gemini 1M+ → 未来更长）
  - 前提 2：注意力机制可能很快变成线性复杂度（标准 Transformer 是 O(n²)）
  - 推论：如果上下文足够长且便宜，很多现在需要 RAG 检索再塞进 context 的场景，可以直接把原始数据丢进去
  - 结论：RAG 不是向量搜索的终极用例，learned embeddings 的价值比 RAG 更持久
- [x] 想一下这个论点哪里站得住：RAG 的一个痛点确实是检索精度——你不知道捞上来的 chunk 是不是最相关的，如果上下文窗口够大，直接塞全文确实能避免这个问题
- [x] 想一下这个论点哪里可能有问题：
  - 上下文长不等于便宜。就算模型能吃 1M token，推理成本跟 token 数正相关，RAG 通过只检索相关片段来省钱这个价值不会消失
  - 线性注意力到现在（2026 年）还没有在主流模型里大规模替代标准注意力。长上下文模型的 "lost in the middle" 问题仍然存在
  - 💡 可以搜一下 2025-2026 年关于线性注意力（如 Mamba、RWKV、RetNet）的最新进展，看看 Antirez 的预判是否在兑现
- [x] 理解他说的 "learned embeddings are here to stay" — 即使 RAG 被长上下文替代，embedding 在推荐系统、语义搜索、异常检测、去重、分类等场景仍然有用。向量搜索的价值不绑定在 RAG 上
  - 🔍 这也解释了为什么 vector sets 的 API 没有任何 RAG 特定的设计（没有 chunk 管理、没有 document 概念）。它就是一个通用的向量相似度原语

---

## 品味校准 (~5min)

- [x] 🌟🌟 判断：guiju 这个设计会成为 Redis 的长期标配，还是会被 Redis Query Engine 吞掉？为什么？
    -   一个组合性更强的简易数据结构能存在于更多由用户构建的系统中。如何理解 vector 是用户的事，不同的 vector 定义产生不同的用法，属于一种有意（但高明）的保鲜社区活跃度的方案。不会被吞掉。
- [x] 它解决的问题是真痛点还是伪需求？已经有 FT.SEARCH 做向量搜索了，再加一个轻量版到底有没有独立存在的价值？
    -   AI 时代下不好说。但不管怎么说，有多少场景根本不需要 FT.SEARCH 那套东西，那么 vector sets 就有多少机会。
- [x] 核心设计决策：Antirez 选择"每个 key 一个独立索引 + 无 schema"而不是"统一的查询引擎"。这个 tradeoff 你认同吗？什么场景下这个选择明显更好或更差？
    -   有开源作者愿意构建更简洁的原语，我除了鼓掌还能说啥呢。
- [x] DX 打分（1-5）：API 直觉性、命令命名一致性、参数暴露的合理程度、文档质量
    -   4 分。
- [x] 一句话总结：如果要推荐或劝退别人用 vector sets 而不是 Redis Query Engine 或独立向量数据库，你会怎么说？
    -   简洁，用法上充满想象力。
- [x] 关于 Antirez 的 RAG 判断：到 2026 年，长上下文模型有没有减少你或你周围团队对 RAG 的依赖？如果没有，是成本问题、精度问题、还是惯性？
    -   我至少没有什么巨量的资料用 rag 管理。现在更倾向于抽象成 skill。