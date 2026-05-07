# TEE Gateway + OpenClaw 打包产品 上手 Checklist

从技术架构到接第一批付费客户。技术底层是阿里云 TDX 上跑的 LLM API 聚合 gateway，上层打包成行业专业版 OpenClaw（如会计版 OpenClaw）做流量分发。本清单默认搭配 Claude Code 使用，大量实现细节委托给 Claude Code（🤖 标记）。

假设前提:Go + TypeScript 技术栈已就绪,阿里云账号已有,OpenClaw 基础版已能跑,合伙人 Zeni 负责行业调教。

***

## Phase 0 — 架构决策和环境准备 (\~60min)

目的是拍板关键决策,别进场再改。

* [ ] 拍板 TEE 平台 — 国内侧选阿里云 TDX(gn8v-tee 实例),国外侧暂缓,先把国内业务跑通
  * 🔍 理解点:TDX 是 VM 级机密计算,Go 二进制直接塞进去就能跑,不用改代码。SGX 要拆可信/不可信部分,门槛高

* [ ] 拍板定价策略 — ¥188 会计版 OpenClaw(含 ¥100 API 预充 + ¥88 毛利),先只做这一个 SKU,不做多档

* [ ] 拍板模型路由范围 — MVP 只支持 DeepSeek + Qwen 官方 API(国内可合规调用),Claude/GPT 放 v2

* [ ] 开通阿里云 TDX 实例 — 去控制台看 gn8v-tee 系列在哪些 region 可用,先开一台最小规格跑通

* [ ] 开通阿里云 KMS — 创建一个专用 key,后面用来管 API key 和数据库 DEK

* [ ] 📝 产出 `DECISIONS.md` 文档记录所有拍板内容 — 🤖 Claude Code 可以起草,你 review

***

## Phase 1 — Gateway 最小可运行(非 TEE) (\~3h)

目的是先把业务逻辑跑通,暂不上 TEE。

* [ ] 定义 gateway 的 OpenAI 兼容接口 — `/v1/chat/completions`,`/v1/models`,支持 SSE 流式

* [ ] 🤖 实现路由模块 — 根据请求里的 `model` 字段路由到 DeepSeek 或 Qwen 官方 API,Claude Code 生成

* [ ] 🤖 实现 API key 管理 — 从环境变量读(后面换成 KMS),Claude Code 生成

* [ ] 🤖 实现 token 计费中间件 — 请求前预扣额度,请求后按实际用量结算,Claude Code 生成

* [ ] 跑通端到端测试 — 用 `curl` 发一个请求到你的 gateway,确认能拿到 DeepSeek 的流式响应

* [ ] 🤖 PostgreSQL schema 设计 — 用户表、订单表、API key 表、token 用量表,敏感字段标记为加密。Claude Code 生成

* [ ] 接入 Alipay 支付 — 复用桃源天下的支付代码,改成"充值 API 额度"语义
  * 🔍 理解点:商业模式的钱流是"用户付 ¥188 → ¥100 进 API 余额账户 → ¥88 入公司账"。支付回调要分账

***

## Phase 2 — Gateway 塞进 TDX (\~4h)

目的是把 Phase 1 的 gateway 跑进 TEE,打通 attestation 和 KMS。

* [ ] 在 TDX 实例上装 docker,跑通 AlibabaCloud 官方 TDX hello world — 确认 attestation 能出

* [ ] 🤖 把 gateway 打成 docker 镜像,部署到 TDX 实例 — Claude Code 写 Dockerfile 和部署脚本

* [ ] 🤖 改造 secret 管理 — enclave 启动时从 KMS 拉 API key 和 DB DEK,不再读环境变量。Claude Code 改
  * 🔍 理解点:KMS 策略要绑定 measurement。代码改一次 measurement 就变,旧版本拿不到 key

* [ ] 实现 `/attestation` 接口 — 返回 enclave 的 attestation document + TLS 公钥,给客户端验证用

* [ ] 🤖 写一个独立的 verifier 工具 — 输入 attestation doc,输出"measurement=X,签名有效=yes",供客户端用。Claude Code 写

* [ ] 把 gateway measurement 公开到 GitHub — 创建 `trust-anchor` 仓库,存 expected measurement + 构建脚本 + 审计说明

* [ ] 端到端测试:客户端先验 attestation 再发请求 — 验证整套信任链能走通

***

## Phase 3 — OpenClaw 打包成专业版 (\~3h)

目的是把 OpenClaw + 你的 gateway 打包成一个消费级产品。

* [ ] 跟 Zeni 对齐会计版调教方案 — 系统提示词、知识库、预设工作流、示例任务,由 Zeni 产出

* [ ] 🤖 OpenClaw 配置注入 — 把 Zeni 的调教配置打进 OpenClaw 副本,默认指向你的 gateway endpoint,Claude Code 写打包脚本

* [ ] 实现激活码机制 — 用户付款 ¥188 后拿到一个激活码,激活码换成 API 余额 ¥100
  * 🔍 理解点:激活码和 API key 是两层。激活码一次性绑定到用户账户,API key 每次调用带上

* [ ] 🤖 打包 macOS + Windows 安装包 — Claude Code 搞 pkg / msi 的构建流程,内嵌 gateway endpoint 和验证逻辑

* [ ] 自测完整购买流程 — 从落地页下单 → 付款 → 拿激活码 → 下载安装包 → 激活 → 跑第一个会计任务

* [ ] 💡 加一个"查看我的 API 调用记录"功能 — 把 TEE 可验证性暴露给用户,作为信任卖点(v2 再做也行)

***

## Phase 4 — 落地页和冷启动素材 (\~4h)

目的是让目标用户(中小财税从业者)能找到你并下单。

* [ ] 域名和落地页搭建 — 一个单页面,主卖点是"会计专用 AI 助手",技术栈 Next.js 或 Astro

* [ ] 🤖 落地页文案 — 痛点(手工对账、凭证录入、税务查询)→ 解决方案(会计版 OpenClaw 演示)→ 定价 → 信任背书。Claude Code 起草

* [ ] 录一段 3 分钟演示视频 — Zeni 用会计版 OpenClaw 跑一个真实案例(如月末对账),用 OpenScreen 录屏

* [ ] 准备 Xiaohongshu / 抖音冷启动素材 — 3 条短视频 + 10 条图文,都围绕"会计效率翻倍"一个痛点

* [ ] 把"可验证 AI"作为次要卖点写进 FAQ — 不是主卖点,是建立信任的背板。目标用户不懂 TEE,别讲技术

* [ ] 💡 准备一份 PDF 白皮书 — 面向 B 端(小会计事务所)潜在客户,讲数据安全和 TEE 背书
  * 🔍 理解点:会计行业对数据隐私敏感,这是你比普通 AI 助手的差异化点,但要用会计能听懂的话讲

***

## Phase 5 — 第一批付费客户 (\~2 周观察期)

目的是验证商业假设,修正产品和营销。

* [ ] 内测 10 个种子用户 — 从 Zeni 的会计圈子找,免费送 3 个月,换反馈

* [ ] 跑 Xiaohongshu 付费流量 — 预算 ¥500/天 跑 1 周,看 CPA 和转化率

* [ ] 监测关键指标 — 落地页 UV、下单转化率、激活率、7 日留存、差评原因

* [ ] 每天收集至少 3 条用户反馈 — 主动私信问"哪里不好用",记成表格

* [ ] 🤖 每周生成一份复盘 — Claude Code 从飞书表格里拉数据,生成用户画像和产品改进优先级

* [ ] 🚨 红线指标:如果一周过了 CPA > ¥200,停流量重新打磨产品 — 这条比什么都重要

* [ ] 💡 找 1-2 个行业 KOL 合作分发 — 财税博主带货,抽成 30-50%

* [ ] 💡 准备 v2 的方向 — 如果会计版跑通,复制到法律、教培、电商客服等行业

***

## Phase 6 — 基础设施收尾 (\~4h)

目的是让产品达到能稳定服务 100-1000 用户的状态,不是"能跑就行"。

* [ ] TDX 实例监控 — CPU、内存、KMS 调用失败率、attestation 生成延迟,接 Grafana

* [ ] gateway 限流和熔断 — 按 API key 做限流,上游 DeepSeek/Qwen 出故障时熔断切换

* [ ] 备份和灾备 — PostgreSQL 每日快照,API key 余额数据重要性等同于钱

* [ ] 🤖 写一份 `OPERATIONS.md` — 常见故障的处理步骤,供未来你或运维跟进。Claude Code 起草

* [ ] 🤖 自动化部署流水线 — GitHub Actions 推镜像到阿里云 ACR,TDX 实例拉取更新并重新 attest。Claude Code 写

* [ ] 公告 measurement 更新机制 — 每次发布新版本,更新 trust-anchor 仓库,用户可以查

***

## 品味校准 (\~5min)

做完 Phase 5 第一周再来回答,答案写完三个月后回来看。

* [ ] 生死判断 — "TEE 可验证"对消费级产品用户(会计、法务这些)来说是强卖点还是没人在乎?流量推广时这个点要不要讲?

* [ ] 需求真伪 — ¥188 会计版 OpenClaw 这个定价,用户是为"AI 工具"买单还是为"会计专用"买单?如果去掉"会计专用"只卖通用 OpenClaw,转化率会掉多少?

* [ ] 核心设计决策 — 你选了"高毛利 ToC 分发 + TEE 基础设施"这条路,vs "纯 BYOK 卖给 B 端"或"出海做 OpenRouter 竞品"。回头看这个决策对吗?

* [ ] 商业模式验证 — ¥100 API 预充值用户平均消耗率多少?如果用户 30 天只用了 ¥20,剩下 ¥80 是你白赚还是要退?这影响复购模型

* [ ] 渠道打分(1-5)— Xiaohongshu 获客、抖音获客、Zeni 的圈子分发、KOL 合作、B 端直销,各自 ROI 打分

* [ ] 一句话总结 — 如果现在让你劝朋友"别做 TEE Gateway,做纯 OpenClaw 打包分发就够了"你会怎么说?反过来又怎么说?

***

## 额外提醒

* 阿里云 TDX 实例在不同 region 可用性不一样,下单前提工单问,别 assume 香港/新加坡有

* OpenClaw 基础版的 license 要确认是否允许商业化打包,别被追责

* ¥88 毛利看起来小,但前提是 ¥100 API 余额用户不会全用完。实际 ARPU 如果 < ¥100,你的真实毛利更高

* 会计行业敏感数据多,上线前准备好一份"数据处理说明"放在下载页,堵住用户质疑

* 合伙人 Zeni 负责行业调教的副本是你的 v1 产品的全部价值,这部分进度要和你的 TEE 基础设施进度同步,任何一边慢了整个产品都出不了

