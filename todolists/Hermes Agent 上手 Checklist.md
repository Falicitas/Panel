# Hermes Agent 上手 Checklist

从已安装可运行的状态出发，到能自信地配置、扩展、训练 Hermes Agent。

前提：macOS，Python 3.11+，已有 \~/.hermes/ 目录和 config.yaml。

***

## Phase 0 — 确认环境可用 (\~5min)

确认 Hermes 能跑起来，排除环境问题。

* [x] 跑一次 `hermes doctor` — 看有没有红色报错，确认依赖完整

* [x] 跑一次 `hermes status` — 确认当前 profile、model、provider 信息

* [x] 在 CLI 里发一条消息 — `hermes chat`，随便问个问题，确认能收到回复

* [x] 确认 tool calling 能用 — 让 agent 执行一个 terminal 命令（比如 `ls ~`），看 tool\_call 是否正常触发

***

## Phase 0.5 一次调用会发生什么

```bash
用户在 prompt_toolkit 输入 "你好" 按回车
    │
    │ ① cli.py handle_enter()
    │   「text = "你好"，打包成 payload。
    │    根据 busy_input_mode（默认 "interrupt"）决定目标队列：
    │    - agent 空闲时 → 放入 _pending_input
    │    - agent 运行时 → 放入 _interrupt_queue（触发中断）
    │    如果 busy_input_mode="queue"：永远进 _pending_input，不中断。
    │    当前 agent 空闲 → 放入 _pending_input」
    │
    │ ② cli.py process_loop
    │   「从 _pending_input 取出，不以 / 开头 → 不是命令 → 走 chat()」
    │
    │ ③ cli.py:7412 chat("你好")
    │   │
    │   ├─ 7436: _ensure_runtime_credentials()
    │   │  「检查当前 provider 的 API key 是否有效，无效则尝试刷新。
    │   │   对 openai-codex provider 还会检查 ~/.codex/auth.json 的
  token
    │   │   是否过期，过期则写入刷新后的 token」
    │   │
    │   ├─ 7439: _resolve_turn_agent_config("你好")         ← ★ smart
  routing
    │   │  「调用 smart_model_routing.resolve_turn_route()，
    │   │   纯硬编码判断："你好" 2字符 < 160, 1词 < 28,
    │   │   无换行/代码块/URL/复杂关键词 → 全部通过 →
    │   │   返回 cheap_model 路由 (gemini-2.5-flash via openrouter)」
    │   │
    │   ├─ ~7450: route signature 比对
    │   │  「signature 是一个 tuple：(model, provider, base_url,
  api_mode, command, args)。
    │   │   它是路由结果的"指纹"。CLI 会把当前生效的 signature 存在
    │   │   _active_agent_route_signature 里。每轮对比新旧 signature，
    │   │   如果不一样（比如上轮用 qwen 这轮 smart route 到了
  gemini），
    │   │   说明 model/provider 变了，需要销毁旧 AIAgent 重建新的，
    │   │   因为 AIAgent 初始化时就绑定了 OpenAI client 和 API key。
    │   │   如果一样就复用，省掉重建开销」
    │   │
    │   ├─ ~7455: _init_agent(model="gemini-2.5-flash",
  runtime=openrouter)
    │   │  「创建新的 AIAgent 实例：初始化 OpenAI client，
    │   │   注册 51+ tools，加载 toolsets，配置 credential pool」
    │   │
    │   ├─ 7462: @ context reference 检查
    │   │  「用正则扫描消息里有没有 @diff / @staged / @file:path /
    │   │   @folder:path / @git:N / @url:https://... 这些特殊引用。
    │   │   如果有，就在发给 LLM 之前把引用展开成实际内容塞进消息里。
    │   │   比如 @diff 会跑 git diff 把结果内联，@file:main.py
  会读文件内容插入。
    │   │   展开有大小限制：硬上限是 context 的 50%，超过 25% 会警告。
    │   │   还有安全拦截：.ssh/id_rsa、.aws 等敏感路径直接拒绝。
    │   │   "你好" 里没有 @ → 跳过」
    │   │
    │   ├─ 7488: _sanitize_surrogates("你好")
    │   │  「UTF-16 代理对（surrogate）是 U+D800–U+DFFF 范围的字符，
    │   │   在 UTF-8 里是非法的。从 Word/Google Docs
  复制粘贴时偶尔会带入。
    │   │   如果不处理，后面 json.dumps() 序列化消息发给 OpenAI SDK
  时会炸。
    │   │   这个函数用正则把 lone surrogate 替换成 U+FFFD（�）。
    │   │   "你好" 是正常中文 → 正则不命中 → 原样返回，几乎零开销」
    │   │
    │   ├─ 7492: conversation_history.append({"role": "user",
  "content": "你好"})
    │   │  「把用户消息追加到本地 history 列表」
    │   │
    │   └─ 7585: agent.run_conversation("你好", history=[...])
    │      「在后台 daemon 线程中执行。
    │       注意传入的是 conversation_history[:-1]，即不含刚 append
  的那条
    │      （run_conversation 内部会自己 append user message）。
    │       以下全在 run_agent.py 里」
    │      │
    │      │ ④ run_agent.py:7527 run_conversation()
    │      │
    │      ├─ ~7570: _restore_primary_runtime()
    │      │  「如果上一轮触发过 fallback（比如 429 切了备用 key），
    │      │   这里恢复回主 runtime。首次调用时是空操作」
    │      │
    │      ├─ ~7580: _sanitize_surrogates("你好")
    │      │  「AIAgent 层再做一次代理字符清理。为什么做两遍？
    │      │   因为 run_conversation 不只被 CLI
  调用，gateway、batch_runner、
    │      │   cron scheduler 也会直接调，那些入口没经过 cli.py
  的清理。
    │      │   双重保险，确保不管从哪个入口进来都安全」
    │      │
    │      ├─ 7618: IterationBudget(max=90)
    │      │  「线程安全的迭代计数器。每次 API 调用消耗 1 次配额。
    │      │   用完后不是直接停止，而是注入一条 user message 告诉模型
    │      │   "你的迭代预算用完了，请总结当前进展"，再给一次 grace
  call。
    │      │
  这样模型能给出一个有意义的收尾，而不是在工具调用中间哑停。
    │      │   子 agent（delegate_task）有独立预算（默认 50 次），
    │      │   不共享父 agent 的配额，所以总调用次数可以超过 90。
    │      │   每轮 run_conversation 开始时重置为满额」
    │      │
    │      ├─ 7631: messages = list(history)
    │      │  「拷贝 history 避免修改调用方的列表」
    │      │
    │      ├─ 7637: _hydrate_todo_store(history)
    │      │  「从历史消息里恢复 todo 状态，
    │      │   gateway 每次新建 AIAgent 所以需要这步。CLI
  首轮为空，跳过」
    │      │
    │      ├─ 7664: messages.append({"role": "user", "content":
  "你好"})
    │      │  「用户消息加入 API 消息列表」
    │      │
    │      ├─ 7682: _cached_system_prompt is None?        ← ★ 首次构建
   system prompt
    │      │  「是 None → 调用 _build_system_prompt()，按顺序组装：
    │      │
    │      │   ① SOUL.md — 读 ~/.hermes/SOUL.md 作为 agent 人格。
    │      │     当前文件为空 → 退回硬编码的 DEFAULT_AGENT_IDENTITY。
    │      │
    │      │   ② Memory block — 从 BuiltinMemoryProvider 读 MEMORY.md
  和 USER.md。
    │      │     这是 agent 的"长期记忆"，用户通过 memory tool
  主动存的东西，
    │      │     比如"用户偏好简洁回复"、"项目用的是 PostgreSQL"。
    │      │     跟 context files 的区别：memory 是 agent 自己写的、跨
   session 持久的；
    │      │     context files 是项目目录里已经存在的文档，agent
  只读不写。
    │      │     如果配了外部 memory
  provider（Honcho/Mem0），这里还会追加它的 block。
    │      │
    │      │   ③ Skills index — 调用 build_skills_system_prompt()，
    │      │     不是把 52 个 SKILL.md 全文塞进去（那太大了），
    │      │     而是生成一份紧凑索引：每个 skill 一行（name +
  description）。
    │      │     加载路径：先查进程内 LRU 缓存（最多 8 条）→ 没有就查
    │      │     ~/.hermes/.skills_prompt_snapshot.json（磁盘缓存）→
  都没有才全量扫描文件系统。
    │      │     snapshot 缓存的验证方式：记录了每个 SKILL.md 的 mtime
   和 size，
    │      │
  任何一个文件被修改过（时间戳或大小变了），整个缓存失效，重新扫描。
    │      │     skill 还有条件过滤：比如 fallback_for_tools 会在某个
  tool 可用时隐藏该 skill，
    │      │     requires_toolsets 会在缺少某个 toolset 时隐藏。
    │      │     prompt 里现在更激进地告诉模型"当用户提到某个 skill
  的关键词时，
    │      │     必须主动加载该 skill 的完整内容"，而不只是被动等
  /skill-name 命令。
    │      │
    │      │   ④ Context files — 扫描当前工作目录及祖先目录，找
  AGENTS.md、
    │      │     .cursorrules 等项目级指令文件。这些是项目维护者写的，
    │      │     告诉 agent 项目的约定和规范（比如"用 tabs
  缩进"、"测试用 pytest"）。
    │      │     跟 memory 的区别：context files
  是人类写的项目文档，放在 git 里，
    │      │     所有协作者共享；memory 是 agent
  私有的、动态积累的记忆。
    │      │     跟 subdirectory hints 的区别：context files 在构建
  system prompt 时
    │      │     一次性扫描加载，是静态的；subdirectory hints 是 agent
   执行 tool call
    │      │     访问新目录时才懒加载的，是动态的。
    │      │
    │      │   ⑤ Subdirectory hints — 这一层在构建 system prompt
  时是空的。
    │      │     它的工作方式不同：SubdirectoryHintTracker 会监听每次
  tool call
    │      │     （read_file、terminal
  等），从参数里提取路径，检查那个目录及其
    │      │     祖先目录（最多往上 5 层）有没有 AGENTS.md /
  .cursorrules。
    │      │     如果找到了且之前没加载过，就把内容追加到 tool result
  里，
    │      │     相当于 agent
  "走到"一个新目录时才获得那个目录的上下文。
    │      │     这样做是因为 monorepo 里可能有几十个子项目各自有
  AGENTS.md，
    │      │     全部在启动时加载太浪费 token，按需加载更经济。
    │      │
    │      │   ⑥ Environment hints — build_environment_hints()
  (run_agent.py:3196-3200)
    │      │     检测执行环境：WSL（读 /proc/version 含
  microsoft/WSL）、
    │      │     Termux（$PREFIX 存在）。如果命中，注入平台特定指导
    │      │     （路径翻译、命令差异、行为适配）。
    │      │     普通 macOS/Linux → 返回空 → 跳过。
    │      │
    │      │   ⑦ 时间戳 + model/provider 信息
    │      │   ⑧ 平台 hint（CLI/Telegram/Discord 各有不同的格式建议）
    │      │
    │      │   构建完成后存入 _cached_system_prompt，整个 session
  内不再重建
    │      │  （除非触发了 context compression，会清缓存重建）」
    │      │
    │      ├─ ~7715: session_db.update_system_prompt()
    │      │  「把 system prompt 快照写入 SQLite，用于 session 恢复时
    │      │   直接加载，而不是重新构建（重建会因为 memory 变化导致
    │      │   prompt 不同，破坏 Anthropic 的 prefix cache）」
    │      │
    │      ├─ 7730: preflight context compression 检查
    │      │  「在进入主循环之前，先估算当前 messages 总 token 数
    │      │  （用 estimate_request_tokens_rough，ceiling division:
  (chars+3)//4，
    │      │   现在还包含 tool schemas 的 token）。
    │      │   如果超过了 context_compressor.threshold_tokens
    │      │  （默认是模型 context length 的 50%，但有 64K 地板：
    │      │   即使 context_length × 0.50 算出来只有 16K（32K 模型），
    │      │   也不会低于 64K。防止小 context 模型过早触发压缩），
    │      │   就提前压缩。
    │      │   压缩策略：保留最前面的 system 消息和尾部 token budget
  不动，
    │      │   中间的消息用 auxiliary model 生成摘要替换掉。
    │      │   最多压 3 轮（每轮 summarize 中间 N 条），直到低于阈值。
    │      │   为什么要 preflight？因为用户可能从大 context
  模型（Claude 1M）
    │      │   切到小 context 模型（某个 32K 的），已有 history
  可能直接爆，
    │      │   不提前压缩的话 API 调用会报 context too long 错误。
    │      │   ★ 如果本轮是 smart-routed（走了 cheap model），
    │      │   CLI 传入 skip_preflight_compression=True，跳过整个
  preflight，
    │      │   避免 cheap model 的小 context 触发不必要的压缩。
    │      │   当前是首轮只有一条 "你好" → 远未超阈值 → 跳过」
    │      │
    │      │ ─── 进入主循环 while api_call_count < max_iterations ───
    │      │
    │      ├─ 7854: 检查 _interrupt_requested
    │      │  「每次迭代开头检查。如果用户在 agent 运行时打了新字，
    │      │   CLI 的 _interrupt_queue 监听线程已调用
  agent.interrupt()，
    │      │   这里 break 出循环」
    │      │
    │      ├─ 7863: _touch_activity("starting API call #N")
    │      │  「更新 _last_activity_ts 时间戳。gateway 用这个判断
    │      │   agent 是否还活着（1800s 无活动 → 超时杀掉）。
    │      │   长时间 terminal 命令执行期间也会每 10s touch 一次，
    │      │   防止 gateway 误判超时」
    │      │
    │      ├─ 7868: IterationBudget.consume()
    │      │  「消耗一次配额。如果已用完：
    │      │   首次用完 → 注入 grace message → 设
  _budget_grace_call=True
    │      │   grace call 也用完 → break 退出循环」
    │      │
    │      ├─ ~7900: 构建 api_messages[]
    │      │  「遍历 messages 做逐条处理：
    │      │   ├─ 对 user msg 注入 memory prefetch 结果（如果有的话）
    │      │   ├─ 对 assistant msg 提取 reasoning_content 传回给 API
    │      │   ├─ 清理内部字段（reasoning, finish_reason,
  _thinking_prefill）
    │      │   └─ 对严格 API（Mistral 等）清理 Codex 专属字段」
    │      │
    │      ├─ ~7960: api_messages = [system_prompt] + api_messages
    │      │  「把 system prompt 插入消息列表最前面」
    │      │
    │      ├─ ~7970: apply_anthropic_cache_control(api_messages)
    │      │  「Anthropic 的 prompt caching 机制：在消息上打
  cache_control 标记，
    │      │   告诉 API
  "这部分内容跟上次请求一样，直接用缓存，不要重新计费"。
    │      │   Anthropic 最多允许 4 个缓存断点（breakpoint）。Hermes
  用 system_and_3 策略：
    │      │     断点 1：system prompt（每轮都一样，命中率最高）
    │      │     断点 2-4：最后 3 条非 system 消息（滚动窗口）
    │      │   实现方式：深拷贝 api_messages，在目标消息的 content
  里加
    │      │   {"cache_control": {"type": "ephemeral"}} 字段。
    │      │   效果：多轮对话中 system prompt
  只在首轮计费，后续轮次缓存命中，
    │      │   省约 75% 的 input token 费用。
    │      │   这只对 Claude 模型有效。当前走的是 gemini-2.5-flash，
    │      │   打了标记也不会出错，Gemini
  会忽略这个字段，只是没省钱效果」
    │      │
    │      ├─ 7995: _sanitize_api_messages()
    │      │  「修复 tool_call 和 tool result 的配对问题。
    │      │   LLM API 要求：assistant 消息里的每个 tool_call 都必须有
    │      │   一条对应的 tool role 消息（通过 tool_call_id
  关联），反之亦然。
    │      │   如果配对断了（常见原因：context compression
  砍掉了中间消息、
    │      │   session 恢复时数据不完整、手动编辑了消息列表），API
  会报错。
    │      │   这个函数做两件事：
    │      │     1. 删除孤立的 tool result（有 result 但找不到对应的
  tool_call）
    │      │     2. 给缺失 result 的 tool_call 补一个 stub：
    │      │        {"role":"tool","content":"[Result
  unavailable]","tool_call_id":"xxx"}
    │      │   还会过滤掉 role 不在白名单里的消息（防止脏数据进
  API）。
    │      │   每次 API 调用前都跑，不依赖 compression 是否开启」
    │      │
    │      ├─ 7997: 消息标准化（whitespace + compact JSON）   ← ★ 新增
    │      │  「对 api_messages（不是原始 messages）做两件事：
    │      │   1. 所有 content 字符串 .strip() 去首尾空白
    │      │   2. 所有 tool_calls 的 arguments 重新序列化为
    │      │      compact sorted JSON:
    │      │      json.dumps(obj, separators=(",",":"),
  sort_keys=True)
    │      │   目的：保证相同内容的消息在字节层面完全一致，
    │      │   使 local inference（llama.cpp/vLLM/Ollama）的 KV cache
  命中，
    │      │   以及 cloud provider 的 prompt cache 命中。
    │      │   只动 api_messages 副本，原始 messages 不受影响」
    │      │
    │      ├─ ~8040: _build_api_kwargs(api_messages)        ←
  组装最终请求
    │      │  「打包成 {model: "gemini-2.5-flash", messages: [...],
    │      │   tools: [51+ tool schemas], max_tokens: 4096,
    │      │   temperature: ..., ...} 字典」
    │      │
    │      ├─ ~8050: invoke_hook("pre_api_request", ...)
    │      │  「触发 plugin 的 pre_api_request hook，
    │      │   传入 model/provider/message_count/tool_count 等元数据」
    │      │
    │      └─ ~8100: _interruptible_streaming_api_call(api_kwargs)
    │         「在后台 daemon 线程里执行：                     ← ★★★
  第一次 LLM 调用
    │          client.chat.completions.create(**api_kwargs)
    │          主线程每 0.3s 检查一次 _interrupt_requested
    │         （用户在打字就中断：强制关闭 HTTP 连接 →
  InterruptedError）。
    │          对 "你好" 这条消息，实际打给的是 gemini-2.5-flash
    │          而不是主力模型 qwen/qwen3.6-plus」
```

## Phase 1 — 理解配置体系 (\~15min)

搞清楚 config.yaml 里每块配什么，能按需调整。

* [x] 🌟🌟🌟(🌟) 打开 `~/.hermes/config.yaml`，通读一遍 — 830 行，重点看 model、terminal、compression、agent 这几个 section

  * 🔍 配置优先级：CLI flags > ENV vars (.env) > config.yaml > defaults

* [x] 🌟🌟🌟 读完单次调用和第二次调用的树结构

  第一次调用 & 主 completion 循环前：

  * [x] 消息管理，现在有几个 input 的信息了。

    * conversation\_history：HermesCLI 实例持有，session 级别，跨多轮对话。标准 OpenAI 消息格式：

      ```ts
      {"role": "user"|"assistant"|"tool", "content": ..., ...}
      ```

    * system prompt：session 首次调用或者 context compression 后重建。MEMORY.md 这种长期静态记忆会初始构建时（BuiltinMemoryProvider）注入，之后在 session 中保持不变。

    * messages：run\_conversation 对话循环时的临时变量，由 conversation\_history 先初始赋值。

      在主循环结束后，messages 变为：原 history + 新 user msg + （主循环开始）新 assistant msg + tool calls + tool results + 最终 assistant 回复

    * api\_messages：在主循环每一轮迭代的局部变量。这里注意 messages 是干净的，同构于 conversation\_history，只有三种角色的对话信息。而 api\_messages 是在 messages 的基础上，注入 system prompt（没错，messages 里没有 system prompt），memory context（只注入到当前 user msg 尾部，属动态记忆。这有个副作用好处就是 api\_messages 前缀子数组相同，前缀 hash 没变，能索引到同个缓存 KVCache），plugin context（和 memory 一样，在当前 user msg 尾部），fet-shot examples、缓存标记等。

      * [x] strip budget warnings 是什么？主会话的单次 completion（1 turn）可能有多次 tool call。为了提醒 llm 尽早结束，主循环会注入类似下面的提醒到 messages 的每一个 `role: tool` 的调用结果后：

        ```bash
        [BUDGET WARNING: Iteration 81/90. Only 9 iteration(s) left.
           Provide your final response NOW. No more tool calls unless absolutely critical.]
        ```

        甚至最后这个带 BUDGET WARNING 标签的会被赋值到 conversation\_history，只在下一次 run\_conversation 进行剥离。

        为什么这么设计：原因是 turn 会被用户中断，或者 tool\_call 调用抛异常。中断恢复时的 history 的 BUDGET WARNING 是有意义的。另外主循环的每一轮也都会落盘，避免 hermes 进程也挂了可以 resume。messages 是一直在内存中脏写 BUDGET WARNING 的（turn 被中断称为 soft interrupt；hermes 进程被中断是 hard crash）。

      * [x] 怎么算缓存命中：Hermes 的 system\_and\_3 策略（prompt\_caching.py:41）。会在 system propmt 和最后三条非 system 信息打上缓存断点。断点前缀 hash 后匹配就行了。

      * [x] 修复孤儿如何修复？assistant toolcall 要和 tool result 一一配对。会因为压缩或者 llm completion 中断而配对缺失。没有 toolcall 的 toolresult 丢掉；没有 toolresult 的 toolcall，在对应的 assistant 后插入一个 stub toolresult，内容为 `[Result unavailable — see context summary above]`。具体执行的函数就是 `_sanitize_api_messages`。

      * [x] fallback\_for\_tools："tool 不存在 → 启用 skill"，是给缺失功能打补丁用的

      * [x] requires\_tools：tool 存在时才启动 skill。

      * [x] 本地 skills 在 system prompt 构建时就注入了下面形式的目录：

        ```bash
        <available_skills>
            apple:
              - apple-notes: Manage Apple Notes via the memo CLI on macOS (create, view, search, edit).
              - apple-reminders: ...
              - imessage: ...
            github:
              - github-pr-workflow: ...
              - github-auth: ...
            software-development:
              - tdd-workflow: ...
              - debugging: ...
            ...
          </available_skills>
        ```

        name + description 的形式。llm 觉得需要时，会 `skill_view("skill-name")` toolcall（skill\_view 被登记到了 tool registry 中），然后期望一个 toolresult，内容为 skill 完整内容。

      * [x] tools 参数是在 api\_kwargs 即 api 接口的一个参数传入的，这就是所谓的 tool registry。

  * [x] gateway 跟 CLI 调用的业务区别，以及为什么 gateway 每次要新建而 CLI 不用（这个怎么影响了 \_hydrate\_todo\_store）：实际不会，AI 当时分析是基于 gateway 挂了。然后 todo 这个是其中的一个工具 tool（\_todo\_store 是 agent 的一个属性 `self._todo_store`），toolcall 它可以查询当前 agent 的 todolist。

    > 严格来说：eviction 不止"挂了"。进程常驻情况下，/new、/branch、/model 切模型、工具集变更、credential fallback 都会主动 evict 缓存
    > agent。这些是正常操作，不是崩溃。重启只是其中一种触发源。
    >
    > ![1.00](/Users/kinesis/.hermes/HERMES_ONBOARDING_CHECKLIST.assets/image-20260410224505378.png)

  * [x] 为啥 execute\_code 的迭代会被 refund：execute\_code 让模型一次 API 调用内批处理多个工具操作，这样用来无视 /90 的惩罚。

  * [x] Subdirectory hints 没懂：Subdirectory hints 是在 agent 跑完每次 tool call（read\_file、terminal 等）之后，从 tool
    参数里解析出被访问的目录，往上爬最多 5 层找 AGENTS.md / CLAUDE.md / .cursorrules
    这类项目约定文件，第一次访问某个目录时把找到的内容追加到 tool result 尾部（不改 system prompt，因此不破坏 prompt
    cache）；每个目录只加载一次，主要用途是 monorepo 里按需加载各个子项目的规则。

  * [x] Platform hint 则是完全独立的东西：在
    \_build\_system\_prompt() 最后一步根据 self.platform（telegram / discord / whatsapp 等）从硬编码的 PLATFORM\_HINTS
    字典里查出一段静态文本追加到 system prompt 结尾，告诉模型"你在某某平台，别用 markdown / 用 MEDIA:/path 发附件"；CLI
    平台没有对应 key 所以什么都不加。

  * [x] AGENTS.md 实际更像是规范 spec，不是启用某个 agent。

  * [x] 🌟🌟🌟🌟 preflight 压缩是事件触发还是什么，在长上下文模型不会触发吧？验证什么：cc update 后，需不需要重新装载？走得通：不需要；走不通：需要

  * [x] perflight 是否可以提交一个 pr：

    * [x] 版本落后两天，不确定是否修改了

    * [x] 如果没修改，pr 如何算作是规范？我从 pr + issue 到结束，可能要做的事情

    * [x] 🌟🌟🌟🌟 开始 pr。验证什么：AI 他到底理解有没有问题；走得通：理解了；走不通：没理解

    * [x] 🌟🌟🌟🌟 恢复现场，继续推。验证什么：我可以正常问问题；走得通：可以；走不通：不行

  第一次调用 & 主 completion 循环中：

  * [x] max\_tokens 是单次最大 token 输出还是什么：是的，单次 llm 输出。A 社默认很大，Qwen 很小，Hermes 给 Qwen 设置了 65536。

  * [x] 用户打字就中断？返回 completion 状态还是有更细粒度的“现场保存&恢复”：每次主循环执行的 `_interrupt_requested` 会检查有无用户打断。没有恢复，messages 不管是中断还是正常完成，都会直接赋值给 conversation\_history 并落盘 sqlite（每个 turn 一次，并非每个 iteration），供 session 恢复时恢复 conversation\_history。

    hard crash 会丢失 1 turn 的内容。

    ```bash
     被打断的 turn 的 messages（到最后一个完整 iteration 为止）:

        user: "帮我做 X"                         ← 本轮开头
        assistant: [tool_call: search_files]      ← iteration 1 ✅ 完整
        tool: {result: "..."}
        assistant: [tool_call: read_file]         ← iteration 2 ✅ 完整
        tool: {result: "..."}
        assistant: [tool_call: write_file]        ← iteration 3 ✅ 完整
        tool: {result: "..."}
        assistant: "好的，我已经..."               ← iteration 4 ❌ 流式中断，这条丢了
                                                    (break 出 stream 循环，未 append)
                                                 下一轮：
        user: "等一下，改成 Y"                    ← 中断消息作为新 user input
    ```

* [x] 看一下 `.env` 里配了哪些 API key — 数一下你有几个 provider 可用

* [x] 试一下 smart model routing — 在 config.yaml 里找 `smart_model_routing`，开启后发一条短消息，观察是否路由到了 cheap model

* [x] 改一下 compression 配置 — `compression.threshold` 默认 0.50，改成 0.30 试试，聊几轮后看是否触发压缩

***

## Phase 2 — 玩转 CLI 功能 (\~20min)

CLI 有 40+ slash 命令，挑几个高频的跑通。

* [x] `/history` — 看当前对话的完整记录

* [x] `/tools list` — 看当前启用了哪些 tools，数一下

* [x] `/toolsets` — 看 toolset 分组，理解 web/terminal/browser/coding 等分组逻辑

* [x] `/skills` — 进入 skill 管理界面，浏览已安装的 52 个 skill

* [x] 试一下 context reference — 发一条 `@diff 这次改了什么` 或 `@file:~/.hermes/SOUL.md 看看这个文件`

* [x] `/background` 跑个任务 — 比如 `/bg 帮我搜索一下 Hermes Agent 的 GitHub stars`，看后台执行

* [x] `/compress` — 手动触发一次上下文压缩，观察消息条数变化

* [x] `/session` 相关 — `/title 我的测试会话`，然后 `/new` 开新会话，再用 `hermes sessions browse` 找回旧的。

  🌵 点评：有个有意思的事。压缩（不管是 preflight 还是 /compress），都会开一个新 session，然后指向旧的 session。旧的 session 依旧可以查找到（`hermes sessions browse`）且没丢失。hermes 牛逼。用好 /title，这样 browse 才找得到（虽然有第一次对话会有个简略的 title）。

***

## Phase 3 — Tool 系统 (\~20min)

理解 tool 注册、发现、调度的完整链路。

* [x] 读 `hermes-agent/tools/registry.py` (320行) — 理解 ToolRegistry 的 register/dispatch 接口

  * 🔍 每个 tools/\*.py 文件底部都有类似的 registry.register() 调用。model\_tools.py 被 import 时触发，发现所有 tools/\*.py 下的工具，后续就没啥了，schema 注入 tools。schema 这个很重要，决定了模型为了调用工具而要生成的入参。下面的 SCHEMA 会包成 openai 的样式。

* [x] 挑一个简单的 tool 文件读一遍 — 比如 `tools/tts_tool.py`，看 schema 定义和 handler 实现

  ```python
  TTS_SCHEMA = {
      "name": "text_to_speech",
      "description": "Convert text to speech audio. Returns a MEDIA: path that the platform delivers as a voice message. On Telegram it plays as a voice bubble, on Discord/WhatsApp as an audio attachment. In CLI mode, saves to ~/voice-memos/. Voice and provider are user-configured, not model-selected.",
      "parameters": {
          "type": "object",
          "properties": {
              "text": {
                  "type": "string",
                  "description": "The text to convert to speech. Keep under 4000 characters."
              },
              "output_path": {
                  "type": "string",
                  "description": "Optional custom file path to save the audio. Defaults to ~/.hermes/audio_cache/<timestamp>.mp3"
              }
          },
          "required": ["text"]
      }
  }
  ```

* [x] 读 `model_tools.py` (577行) — 看 handle\_function\_call 怎么从 registry 分发

* [x] 试触发一个不常用的 tool — 比如让 agent 生成一张图片（image\_generate）或者搜索 session 历史（session\_search）

* [ ] 💡 如果有 MCP server，试一下 `/reload-mcp`，看动态 tool 加载

***

## Phase 4 — Skill 和 Plugin 机制 (\~15min)

理解 skill 怎么被发现、加载、注入到 prompt 里。

* [x] 🌟🌟🌟🌟 读一个 SKILL.md — 比如 `skills/github/github-pr-workflow/SKILL.md`，看 frontmatter 格式和 body 结构。

  🌵 点评：Prerequisites：这个阶段可以让 llm 去看哪些 skill 以达成前置条件（这个 SKILL 虽然也展开了 prerequisite，很大可能不需要再索引对应 skill）。

  接着是 SKILL 的分步骤和汇总步骤，以及可能有帮助的 hint。整个 SKILL 结束。

  > 开头的 --- 是 YAML frontmatter

* [ ] 读 `agent/skill_commands.py` 的 `scan_skill_commands()` — 理解 skill 发现流程

  🌵 点评：tool 跟 skill 的区别是，tool 一般是作者预先写好的，通过一次初始化，schema 等就在内存中。用户则可以随意添加 skill，所以还是不太同，虽然只有新 session 才会重新扫描 skill 文件夹， \_cached\_system\_prompt 没有自动监听 skill 文件夹或 MEMORY.md 变化的机制。

* [x] 看 `~/.hermes/.skills_prompt_snapshot.json` 前 50 行 — 理解 skill index 缓存格式

  * 🔍 两层缓存：进程内 LRU + 磁盘 snapshot，任何 SKILL.md 改动触发失效

* [x] 试着禁用一个 skill — 在 config.yaml 的 `skills.disabled` 里加一个 skill name，重启看效果

* [x] 显式调用 skill：` /gif-search 给我一只 cat 的 gif`。

* [x] 💡 读 `hermes_cli/plugins.py` 的 PluginContext 类 — 了解 plugin 怎么注册 tool/hook/command：🌵 跳过

***

## Phase 5 — Gateway 和消息平台 (\~20min)

理解 gateway 怎么接入 Telegram 等平台。

* [x] 跑一次 `hermes gateway status` — 看当前状态

* [x] 读 config.yaml 里的 telegram/discord/slack section — 理解每个平台需要什么 credential

* [x] 🤖 如果有 Telegram bot token，配好后跑 `hermes gateway run` — 从手机发消息测试

* [x] 🌟🌟🌟(🌟) 读 `gateway/run.py` 的 `_handle_message` 方法（搜这个函数名）— 理解消息从进来到回复的完整流程
  * 🔍 gateway 有 agent cache，同一个 session 复用 AIAgent 实例，保留 prompt prefix cache。验证什么：三个具体例子能搞明白；走得通：搞明白；走不通：没明白

    * [x] 🌟🌟 过滤

    * [x] 🌟🌟 分发

    * [x] 🌟🌟 准备

    * [x] 🌟🌟 执行

* [x] 💡 试一下 hook 机制 — 在 `~/.hermes/hooks/` 下创建一个简单的 hook 目录，写一个 HOOK.yaml + handler.py

***

## Phase 6 — Agent 内核 (\~30min)

这是 Hermes 的心脏，值得花时间。

* [x] 读 `run_agent.py` 的 AIAgent.__init__ — 看初始化参数（搜 `class AIAgent`）

* [x] 读 `run_agent.py` 的 run\_conversation 方法 — 跟一遍主循环：prompt build → API call → tool dispatch → loop
  * 🔍 同步循环，不是 async。每轮：build\_system\_prompt → resolve\_provider → API → check tool\_calls

* [x] 读 `agent/prompt_builder.py` 的 `build_system_prompt()` — 看 6 层 prompt 组装顺序

* [x] 读 `agent/context_compressor.py` — 理解压缩策略（保护首尾 turn，summarize 中间）

* [ ] 读 `agent/credential_pool.py` 的 PooledCredential — 理解多 key 切换逻辑
  * 🔍 4 种策略：fill\_first、round\_robin、random、least\_used。429 → 1h 冷却

* [ ] 💡 读 `agent/redact.py` — 看 30+ 种 secret 脱敏 pattern

***

## Phase 7 — 动手改一个东西 (\~30min)

读了这么多，改点东西才能真正理解。

* [ ] 写一个自定义 SOUL.md — 给 agent 一个你想要的人格，重启 CLI 看效果

* [ ] 写一个简单的 tool — 在 `tools/` 下新建一个 .py，注册一个能被 agent 调用的工具
  * 参考 `tools/tts_tool.py` 的结构：定义 schema → 写 handler → registry.register()

* [ ] 或者写一个自定义 skill — 在 `~/.hermes/skills/` 下新建一个目录，写 SKILL.md

* [ ] 跑一轮验证 — 确认你加的 tool/skill 能被 agent 发现和使用

***

## 品味校准 (\~5min)

* [ ] 判断：Hermes Agent 在 AI agent 赛道里会火还是会死？跟 Claude Code、Cursor Agent、OpenAI Codex 比，它的生态位在哪？

* [ ] 它解决的问题是真痛点还是伪需求？"一个 agent 跑在所有平台上"这个需求有多大？

* [ ] 核心设计决策：同步 agent loop（不用 async）、OpenAI-compatible format 一统 18+ provider、skill 作为 user message 注入而非 system prompt。这些 tradeoff 你认同吗？

* [ ] DX 打分（1-5）：上手摩擦、config.yaml 的可读性、报错信息质量、文档覆盖度

* [ ] 一句话总结：如果要推荐或劝退别人用 Hermes Agent，你会怎么说？

