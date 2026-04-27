# RL for LLM Agents 上手 Checklist

前提：理解 Hermes Agent 架构（含 RL codemap 章节），有 GPU 或云 GPU 访问，Python 熟练，RL 工程零基础。

---

## Phase 0 — RL 概念速通 (~2h)

不用看教科书，只看跟 LLM agent 直接相关的。

- [ ] 读 huggingface/trl 的 README + quickstart — 理解 PPO/DPO/GRPO 这三个词分别在干嘛
- [ ] 读一篇 GRPO 的博客或论文摘要 — Search-R1 和 veRL 都用这个算法，知道"不需要 critic model"就够了
- [ ] 搞清楚三个概念的关系：reward function、trajectory、policy update
  - reward function：跑完一轮 agent 操作后打分（比如"答案对了 +1，用了太多步 -0.5"）
  - trajectory：一轮完整的 agent 操作记录（user prompt → tool calls → final answer）
  - policy update：根据 reward 调整模型权重，让高分 trajectory 出现概率增大

---

## Phase 1 — trl 跑通一个最小例子 (~3h)

用 huggingface/trl 跑一个非 agent 的 RL 训练，纯粹验证环境能用。

- [ ] `pip install trl` + 选一个小模型（Qwen2.5-0.5B 或 SmolLM-135M）
- [ ] 跑 trl 的 GRPO example（`examples/scripts/grpo.py`）— 数学题 reward，几分钟跑完
- [ ] 观察 wandb/tensorboard 的 reward 曲线 — 确认 reward 在涨
- [ ] 改一下 reward function — 比如把"答案正确"改成"答案正确且回复少于 100 字"，看曲线变化
- [ ] 理解训练循环：sample → score → update 这三步在代码里分别在哪

---

## Phase 2 — Search-R1：第一个 agent RL 项目 (~1天)

训练一个会调 web search tool 的小模型。

- [ ] clone PeterGriffinJin/Search-R1，读 README
- [ ] 环境搭建：veRL + 本地搜索引擎（Elasticsearch 或他们提供的 mock）
- [ ] 用最小配置跑一次训练：小模型（1.5B-3B）、少 step、单卡
- [ ] 理解 reward 设计：搜索结果的相关性怎么评分，最终答案的正确性怎么评分
- [ ] 观察训练前后的行为差异：未训练的模型乱搜或不搜，训练后会有策略地搜
- [ ] 读懂 environment 代码：它怎么定义"一轮交互"、怎么判定结束、怎么算 reward
- [ ] 试改 reward：比如加一个"搜索次数少于 3 次给 bonus"，看模型行为变化

---

## Phase 3 — Atropos 环境入门 (~1天)

回到 Hermes 的 RL 系统，用你已有的知识跑通。

- [ ] clone NousResearch/atropos，读 README + 看 environments/ 目录结构
- [ ] 读 `environments/hermes_base_env.py` — 跟 codemap 对照，理解 HermesAgentBaseEnv 的 6 个方法
- [ ] 读 `environments/agent_loop.py` — HermesAgentLoop 怎么在 async Atropos 里跑同步 tool call
  - 128 线程池做 sync→async 桥接
- [ ] 读最短的 benchmark 环境：`benchmarks/tblite/tblite_env.py`（~150 行）
  - 理解 get_next_item → format_prompt → compute_reward 的完整链路
- [ ] 读 `tools/rl_training_tool.py` 的 8 个 tool 函数签名 — 理解训练生命周期
- [ ] 如果有 TINKER_API_KEY：用 `rl_cli.py` 跑 `rl_list_environments`，看可用环境
- [ ] 如果没有 key：读 `datagen-config-examples/web_research.yaml`，理解训练配置格式

---

## Phase 4 — 写一个自定义 Atropos 环境 (~1-2天)

这是真正的上手。挑一个简单任务，写 environment 让 agent 通过 RL 学会做。

- [ ] 选任务（建议从简单的开始）：
  - "给一个 JSON 文件，agent 用 terminal tool 提取某个字段的值" — 最简单
  - "给一个 git repo，agent 找到最近一次改了某文件的 commit" — 中等
  - "给一个报错日志，agent 定位到出错的文件和行号" — 中等
- [ ] 继承 HermesAgentBaseEnv，实现 6 个方法：
  - `setup()` — 初始化（比如创建测试文件）
  - `get_next_item()` — 返回下一道题
  - `format_prompt()` — 把题目变成 agent 能理解的 prompt
  - `compute_reward()` — 打分（0-1）
  - `evaluate()` — 验证集上的指标
  - `wandb_log()` — 自定义日志
- [ ] 先不训练，用 `batch_runner.py` 跑几条 trajectory — 看 agent 在你的环境里怎么表现
- [ ] 调整 reward function，直到 reward 能区分好坏 trajectory
- [ ] 跑一轮训练，观察 reward 曲线

---

## Phase 5 — 轨迹压缩和数据处理 (~半天)

理解训练数据怎么从原始 trajectory 变成可训练的格式。

- [ ] 读 `scripts/trajectory_compressor.py`（~1000 行）— 理解压缩流水线
  - 保护首尾 turn，summarize 中间，目标 16K-29K tokens
  - 4 workers，50 concurrent API requests，300s per-trajectory timeout
- [ ] 读 `scripts/sample_and_compress.py` — 从 HuggingFace 下载数据集，采样，计数 tokens
- [ ] 用自己 Phase 4 产生的 trajectory 跑一次压缩，看压缩前后对比

---

## Phase 6 — verl-tool 或 ReCall 对比学习 (可选，~1天)

换一个框架做类似的事，理解不同设计选择。

- [ ] 选一个：
  - TIGER-AI-Lab/verl-tool — 更工程化，适合想深入 veRL 生态的
  - Agent-RL/ReCall — 更学术，适合想理解"什么时候该调 tool"这个 decision boundary 的
- [ ] 跑通它的最小 example
- [ ] 对比它和 Atropos 的差异：environment 定义方式、reward 设计、训练循环

---

## 参考项目

| 项目 | Stars | 定位 | 难度 |
|------|-------|------|------|
| huggingface/trl | 18K | RL for LLM 标准库，PPO/DPO/GRPO | 易 |
| PeterGriffinJin/Search-R1 | 4.5K | 训练 LLM 交替 reasoning + web search | 易-中 |
| TIGER-AI-Lab/verl-tool | 950 | veRL 的 tool-use RL 训练扩展 | 中 |
| NousResearch/atropos | 1K | Hermes 的 RL 环境框架 | 中 |
| langfengQ/verl-agent | 1.8K | veRL agent 版，多步推理 + tool calling | 中 |
| Agent-RL/ReCall | 1.4K | 用 RL 训练 tool-calling 决策 | 中 |
| AgentR1/Agent-R1 | 1.4K | 端到端 agent RL 训练 | 中 |
| verl-project/verl | 20K | 大规模 RL 训练框架 | 中 |
| OpenRLHF/OpenRLHF | 9.4K | Ray 分布式 RL 训练 | 中-难 |

---

## 品味校准

- [ ] Atropos 的 environment 抽象（6 个方法）够不够用？有没有想加的接口？
- [ ] reward function 是人写的 heuristic 还是该用 LLM-as-judge？各自的坑在哪？
- [ ] 小模型（1.5B-3B）通过 RL 学 tool-use 效果如何？跟大模型 prompt engineering 比呢？
