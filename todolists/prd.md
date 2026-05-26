# tee-stack — TEE 可验证 LLM Gateway + OpenClaw 行业版打包

>   项目源真相文档。本文件描述 tee-stack 的目标、模块设计、技术架构。 真正的可勾选任务清单见 `todolists/tee-stack-construction.md`。 本文件和 todolist 各自服务不同目的;两者可能会有同步漂移,**以 PRD 描述的目标为准**,todolist 服务于推进过程。

## 一句话定位

底层做一个跑在 TEE 里的 LLM API Gateway（核心底层能力）。网页端有一个增值服务，将语义训练调教好的 OpenClaw 副本（本质是相比于初始 Openclaw，在长时记忆中有了调配好的文件集合。以及 API 在打包前用户接入的就是我们的 Gateway）打包成行业专业版安装包 (会计版 / 法务版 / 等等),用高毛利 SKU 走流量推广。TEE 可验证性是信任背板（可验证路由透明能力需要平衡验证成本，这个在推进时需要注意，毕竟是商业项目，不过我开启项目是大概认为这部分成本可控且能做到用户可信，最终在构建工程上再讨论修正）。用户最初始的交付流程是：1. 仅使用 LLM API Gateway：在网站注册账号->获取 API key->充值->自己调用；2. 用户购买 Agent 包：在网站注册账号->购买 Agent 包->生成用户api key->某种预打包+自动注入api key->用户日常正常使用 agent与在网页 panel 完成充值。

## 商业模式

-   ¥188 行业版 OpenClaw 安装包 (首期会计版),含 ¥100 API 预充额度 + ¥88 净毛利
-   用户付款 → 拿激活码 → 下载安装包 → 激活 → API 自动走 tee-stack gateway
-   流量来自 Xiaohongshu / 抖音 / KOL 分发, 高毛利支撑付费投放 CAC
-   B 端 BYOK 模式 v2 再做, MVP 不碰

## 设计原则

-   TEE 是基础设施, 不是卖点。卖点是"行业调教好的 AI"
-   代码可审计 = 信任基础, 业务逻辑可以包但路由逻辑必须开源
-   国内合规优先, MVP 只接 DeepSeek + Qwen 官方, Claude/GPT 放 v2 或出海版
-   单 SKU 跑通再复制, 不堆 SKU 矩阵
-   子项目独立部署独立发版, 通过 OpenAPI 契约联通

## monorepo 结构

```
tee-stack/
├── PRD.md                              ← 本文件
├── CLAUDE.md                           ← CC 操作约束
├── todolists/
│   └── tee-stack-construction.md       ← 主推进 todolist
├── backend/                            ← Go gateway + 业务后端 (跑进 TDX enclave)
│   ├── gateway/                        ←   OpenAI 兼容 API + 路由层
│   ├── billing/                        ←   激活码 / 余额 / 支付回调
│   ├── attestation/                    ←   attestation document 生成 + 公开 endpoint
│   ├── store/                          ←   PostgreSQL DAO + 字段加密层
│   └── tests/integration/              ←   跨包集成测试
├── web/                                ← 落地页 + 文档站 (Next.js / Astro)
│   ├── landing/                        ←   首页, 含 3D 玻璃 hero (静态代码动画)
│   ├── docs/                           ←   文档站 (面向开发者 + B 端审计)
│   └── verifier/                       ←   浏览器端 attestation verifier (WASM)
├── desktop/                            ← OpenClaw 行业版打包工程
│   ├── packer/                         ←   打包脚本 (注入调教配置 + gateway endpoint)
│   ├── configs/
│   │   ├── accounting/                 ←   会计版 (Zeni 产出)
│   │   ├── legal/                      ←   法务版 (v2)
│   │   └── ...
│   └── installers/                     ←   构建产物 (pkg / msi)
├── deploy/                             ← 阿里云 TDX 部署脚本 + Terraform
└── scripts/                            ← 跨子项目工具脚本 (release, attestation 发布等)
```

## 模块清单

### 1. Gateway (核心)

**位置**: `backend/gateway/`

**协议**: OpenAI 兼容 (`/v1/chat/completions`, `/v1/models`, SSE 流式)。客户端直接换 base_url 就能用, 不需要改 SDK。

**路由规则**:

| 请求 model                            | 实际路由                  |
| ------------------------------------- | ------------------------- |
| `deepseek-chat`, `deepseek-reasoner`  | DeepSeek 官方 API         |
| `qwen-max`, `qwen-plus`, `qwen-turbo` | 阿里云 DashScope          |
| `claude-*`, `gpt-*`                   | MVP 阶段 401, v2 接出海版 |

路由表是 gateway 代码里的硬编码 + config 文件, 不做动态配置。改路由 = 改代码 = measurement 变化 = 用户能查到。

**API key 管理**:

-   enclave 启动时从阿里云 KMS 拉 DeepSeek key + DashScope key
-   KMS 访问策略绑定 measurement, 改代码就拿不到 key
-   本地 dev 模式从环境变量读, 但 dev binary 和生产 binary measurement 必然不同

**计费中间件**:

-   请求前预扣 max_tokens 对应的额度 (按上游 list price 计算)
-   上游返回后按实际 usage 结算多退少补
-   余额 < 0 直接 402, 不给"先用后付"
-   余额数据加密存 PostgreSQL, 记账事件追加写, 不做就地更新 (审计需要)

**限流**:

-   按激活码维度限流 (默认 60 req/min, 单激活码可调)
-   上游故障熔断: DeepSeek 5 分钟错误率 > 30% → 自动切 Qwen, 标记 fallback header 让客户端知道

### 2. Attestation (信任基础)

**位置**: `backend/attestation/`

**接口**:

```
GET  /attestation/quote          → 返回 TDX attestation document (base64)
GET  /attestation/measurement    → 返回当前 measurement (用户对比公开值)
GET  /attestation/cert-chain     → 返回阿里云 TDX 验证证书链
```

**流程**:

-   enclave 启动时调用 TDX driver 生成一对 ephemeral keypair
-   公钥 hash 写进 attestation 的 user_data 字段
-   enclave 内 HTTPS 证书的指纹也绑进 user_data, 防止 MITM
-   attestation document 每次启动生成一次, 有效期到下次重启
-   客户端拿到 quote → 验签 → 比对 measurement → 提取公钥 → 后续信任建立

**Measurement 发布**:

-   每次 release 把 measurement 写进 `trust-anchor/` 目录 (单独 GitHub repo, 公开)
-   同时附带构建脚本 + reproducible build 指南
-   用户可以自己跑构建脚本验证 "源码 → 同样的 measurement"

### 3. Billing + 激活码

**位置**: `backend/billing/`

**激活码模型**:

```
{
  code: "OC-XXXX-XXXX-XXXX",         // 16 位字符, 用户输入
  sku: "openclaw-accounting-v1",     // 哪个行业版
  initial_credit_cny: 100.00,        // 初始 API 余额
  activated_at: timestamp,           // 激活时间, 未激活为 null
  bound_machine_id: string,          // 激活时绑定的机器 ID
  expires_at: timestamp              // 激活后 N 天过期 (默认 1 年)
}
```

**单次激活, 绑机器**: 一个激活码只能绑一台机器, 换机需要客服流程。这条规则关系到 ¥100 余额的"复购模型"假设, 必须卡死。

**支付链路**:

-   用户在落地页选 SKU → 跳转 Alipay → 回调 webhook → 生成激活码 → 邮件/短信发送
-   支付金额拆账: ¥100 进 "API 余额账户" (虚拟账本), ¥88 进公司账
-   ¥100 是公司对用户的 API 服务负债, 用户每次调用消耗, 余额到 0 后用户可以单独充值 (走另一条充值流程, MVP 阶段不做)

**退款政策**:

-   7 天无理由: 全额退, 激活码作废
-   7 天后: 不退, 但剩余余额可以转给其他用户 (MVP 不做)

### 4. Store (数据持久化)

**位置**: `backend/store/`

**PostgreSQL Schema (敏感字段加密)**:

| 表                 | 字段                                                         | 加密                       |
| ------------------ | ------------------------------------------------------------ | -------------------------- |
| `users`            | id, machine_id, created_at                                   | 否                         |
| `users`            | email, phone                                                 | HMAC 索引 + AES 密文       |
| `activation_codes` | code, sku, initial_credit_cny                                | 否 (code 本身就是凭据)     |
| `activation_codes` | bound_user_id, activated_at                                  | 否                         |
| `api_calls`        | id, user_id, model, tokens_in, tokens_out, cost_cny, created_at | 否                         |
| `api_calls`        | request_body_enc, response_body_enc                          | 加密 (可选, 默认不存 body) |
| `balance_events`   | id, user_id, delta_cny, reason, created_at                   | 否 (账本, append-only)     |
| `payment_orders`   | id, alipay_trade_no, amount_cny, status                      | 否                         |

**DEK (Data Encryption Key)**:

-   从 KMS 拉一个 master key, 派生若干 DEK (按字段类型分)
-   DEK 缓存在 enclave 内存, 不落盘
-   HMAC 用独立的 secret, 防止 HMAC 值反查

**默认不存 request/response body**: 用户没勾选 "保存调用记录" 就只存元数据 (model, tokens, 时间)。勾选后才存加密 body, 用户可以自己导出。

### 5. Web — Landing + Docs

**位置**: `web/`

**子站**:

-   **landing** (`tee-stack.com/`): 单页落地页, 3D 玻璃 hero + 价值主张 + 行业版 SKU 列表 + 购买入口
-   **docs** (`tee-stack.com/docs/`): 文档站, 面向开发者讲 OpenAI 兼容 API 用法, 面向 B 端讲数据安全和 TEE attestation
-   **verifier** (`tee-stack.com/verify`): 浏览器端 attestation 验证工具, 用户粘贴 attestation document, 浏览器里跑 WASM 验签 + 比对 measurement, 输出 "verified ✓" 或具体错误

**Landing 首页 3D Hero**:

-   类 canvas 3D 立体玻璃居中跑, 静态代码行高亮动画 (类调试时的 highlight 移动)
-   代码内容是 gateway 的核心路由函数 (展示 "我们的路由逻辑就这么多, 都在这")
-   视觉传达 "你能看到我们的代码"
-   实现细节 Phase 6 跟 CC 着重讨论

**Docs 站结构**:

-   Quickstart: 5 分钟接入 (换 base_url 就行)
-   Verification: 如何验证 attestation, 包含 verifier 工具用法
-   Security: TEE 架构说明, 数据加密策略, 信任模型
-   API Reference: OpenAI 兼容部分 + 我们的扩展 (attestation header 等)

### 6. Desktop — OpenClaw 行业版打包

**位置**: `desktop/`

**核心思路**: OpenClaw 是已有产品, 我们做的是 "调教配置 + 我们的 gateway endpoint + 激活码机制" 三层注入。

**Zeni 产出 (`desktop/configs/accounting/`)**:

-   系统提示词 (会计专家人设)
-   知识库片段 (中国会计准则关键章节, 常见科目, 税法基础)
-   预设工作流 (月末结账 / 凭证录入 / 增值税申报 等)
-   示例任务 (用户 onboarding 时跑给用户看)

**packer 脚本 (`desktop/packer/`)**:

-   输入: OpenClaw 副本 + 行业 config
-   操作: 注入 config, 把 OpenClaw 的 LLM endpoint 改成 `https://api.tee-stack.com/v1`, 嵌入激活码输入界面
-   输出: macOS .pkg + Windows .msi

**激活流程**:

1.  用户启动 OpenClaw → 弹激活码输入框
2.  客户端 POST 激活码 + 机器 ID 到 `/v1/activate`
3.  后端校验激活码 + 绑定机器 ID + 返回 API key (绑定本机器, 不可迁移)
4.  OpenClaw 把 API key 存本机 keychain, 后续调用自动带上

**安装包大小目标**: < 80MB (OpenClaw 本身 + 配置 + 激活逻辑)。超了再优化。

### 7. Deploy

**位置**: `deploy/`

**目标环境**: 阿里云 TDX (gn8v-tee 系列实例)

**部署流程**:

1.  GitHub Actions 构建 backend Docker 镜像 → 推阿里云 ACR
2.  Terraform 起 TDX 实例 + RDS PostgreSQL + KMS keys + SLB
3.  TDX 实例启动时 pull 镜像 + 生成 attestation + 注册到 SLB
4.  新 measurement 自动 PR 到 `trust-anchor/` 仓库, 人工 review 后 merge 发布

**Measurement 更新策略**:

-   代码 PR 合并到 main 触发自动构建
-   构建产出新 measurement → 提 PR 到 trust-anchor 仓库
-   人工 review 通过后老 measurement 标记 deprecated (30 天后 KMS 拒绝)
-   新老 measurement 重叠期 30 天, 允许灰度

**故障切换**:

-   主区 (杭州 / 北京 TDX) 故障 → DNS 切备区
-   KMS 故障 → enclave 缓存的 key 仍可用直到重启, 触发告警

## 技术架构

### 后端

-   语言: Go
-   部署: TDX enclave 内 Docker 容器, 阿里云 gn8v-tee 实例
-   端口: REST `:443` (内嵌 TLS, 证书指纹绑 attestation), 健康检查 `:8080`
-   数据:
    -   阿里云 RDS PostgreSQL (敏感字段加密, DEK 从 KMS 拉)
    -   阿里云 KMS (master key + 上游 API key + DEK 派生 key)
    -   不用 Redis (规模没到, 用 PostgreSQL advisory lock 处理并发足够)

### 前端 (web)

-   框架: Next.js 14 (App Router) + TypeScript
-   部署: 阿里云 OSS 静态托管 + CDN, 海外用 Cloudflare Pages 备份
-   3D Hero: Three.js + GLSL shader, 静态代码动画用 Shiki + 自定义 highlight 状态机
-   Verifier: Rust 编译 WASM, 浏览器端验 TDX attestation, 不依赖后端

### Desktop 打包

-   macOS: pkgbuild + productbuild + Developer ID Application 证书签名 + notarytool 公证
-   Windows: WiX Toolset 生成 MSI + EV 代码签名证书 (后续买)
-   配置注入: 构建期把 config JSON 内嵌进 OpenClaw 资源目录

### 数据持久化

-   用户数据: PostgreSQL (字段加密)
-   attestation document: 不持久化, enclave 重启重新生成
-   激活码: PostgreSQL 明文存 (code 本身就是凭据)
-   调用日志: 默认不存 body, 元数据存 PostgreSQL

### TEE 信任模型

-   信任链: AWS/Intel/Aliyun TDX 硬件根 → measurement → enclave 公钥 → TLS 证书 → 用户请求
-   用户验证手段:
    1.  浏览器访问 `tee-stack.com/verify` → 拉 attestation → WASM 验签 → 对比公开 measurement
    2.  命令行工具 (`scripts/verify.sh`) 给开发者用
    3.  文档站 transparency 页展示当前 measurement + 历史版本
-   不依赖任何第三方信任服务, 全部自证

## Git 约定

### Issue 分支工作流 (沿用 Panel 项目规则)

每个独立的 feat / fix / style / refactor / docs 推进都走"分支 → 合并", 不直接在 dev 上 commit。

```bash
git checkout dev
git checkout -b issue-N

git commit -m "feat(...): ..."
# ...

git checkout dev
git merge --no-ff --no-commit issue-N
```

### Conventional Commits

-   至少二级 scope, 三级 (具体功能) 视情况加
-   monorepo 一级 scope = 子项目目录名
-   格式: `<type>(<sub-project>-<module>[-<feature>]): ...`
-   例:
    -   `feat(backend-gateway-路由): 加 qwen 路由`
    -   `feat(backend-attestation): 暴露 measurement endpoint`
    -   `feat(web-landing-hero): 3D 玻璃动画接入`
    -   `fix(desktop-packer): macOS 公证流程修复`
    -   `feat(deploy-tdx): terraform 模板初版`
-   跨子项目: `feat(both-billing): ...` 或拆成多个 commit
-   主分支: `dev`

### 数据保护

monorepo 里 `todolists/` 仍然是变化最频繁区域, 沿用 Panel 项目规则:

-   默认避免 `git reset --hard`, 优先 `git revert` 或 fix-forward
-   必须 reset --hard 前 stash todolists
-   已 gitignore: `backend/build/`, `web/.next/`, `web/node_modules/`, `desktop/installers/build/`, `data/`, `*.db`

## 跨子项目契约

子项目独立开发独立发版, 通过两个契约联通:

### 1. OpenAPI Schema

-   位置: `backend/openapi.yaml`
-   涵盖 gateway API + attestation API + billing API
-   web 和 desktop 都从这个 schema 生成客户端代码
-   schema 变更 = breaking change = 走 issue branch + 多端同步

### 2. Trust Anchor

-   独立 repo `tee-stack/trust-anchor` (公开)
-   内容: 当前 active measurement, 历史 measurement, 构建脚本, audit log
-   backend 发版 → 自动 PR → 人工 review → merge → web verifier 自动拉取

## 文件夹布局

详见 monorepo 结构那一节。子项目内部布局各子项目自决, 不强制对齐。

## 已完成 (历史)

无 (新建项目)。LLM Gateway 老项目的竞品分析、定价模型可作为参考。

## 文档同步约定

PRD 与 todolist 可能漂移。每次需求变化:

1.  优先更新 PRD (本文件)
2.  然后视情况调整 `todolists/tee-stack-construction.md`

PRD 是项目目标的真相; todolist 是手动推进的勾选清单。两者不同步时, 以 PRD 为准修正 todolist。

## 商业关键假设 (写在这里以便回头验证)

-   用户为"行业调教"付费, 不是为"AI 工具"付费 — 若去掉行业属性卖通用 OpenClaw, 转化率显著下降
-   ¥100 API 余额平均消耗率 < 60% — 否则毛利模型崩
-   TEE 可验证性是 "锁单理由" 而非 "购买理由" — 用户看完会计版演示决定买, 看完安全说明决定不退
-   Xiaohongshu / 抖音 CAC < ¥100 — 否则 ¥88 毛利不够覆盖获客
-   Zeni 的行业调教质量直接决定产品上限, 任何技术栈优化都救不回内容差的版本