# TEE + TLSNotary 上手 Checklist

从 ZK proof 背景转过来，目标是把这两块吃透到能评估"用 TEE 跑 Notary"方案的可行性，并能跑出一个最小 PoC：本地 Prover 通过 TEE 里的 Notary 向 Anthropic API 发请求并拿到可验证的 proof。

假设前提：已有 Rust 环境，熟悉 Linux，对 TLS 协议和密码学原语（AES-GCM、ECDHE、HKDF）有概念，但没实际部署过 TEE。

---

## Phase 0 — 环境和概念对齐 (~30min)

目的是搞清楚 TEE 和 Notary 各自到底在解什么问题，避免后面 PoC 跑通了但理解是错的。

- [ ] 写出一段话回答：TLSNotary 里 Notary 的密钥到底是什么？它为什么不能单独解密流量？ — 🔍 理解点：Notary 持有的是 MPC 拆分后的 session key share，和 Prover 的 share 合起来才能解密。这个和 ZK 里的 witness 不是一回事。
- [ ] 写出一段话回答：TEE 远程证明（remote attestation）输出的是什么？验证者拿到什么就能信它？ — 🔍 理解点：attestation 是一份由硬件厂商根密钥签名的 quote，里面包含 enclave 里代码的 measurement（哈希）+ enclave 生成的公钥。验证者信任厂商根密钥 → 信任 measurement → 信任公钥 → 信任 enclave 签出来的东西。
- [ ] 对比一句话：ZK proof 和 TEE attestation 在信任模型上的差别 — 🔍 理解点：ZK 零信任假设，靠数学；TEE 信任硬件厂商 + measurement 可审计。前者成本高，后者成本低但信任锚更"实在"。
- [ ] 安装 Rust 最新稳定版、Docker、`openssl` CLI — `rustc --version` 能跑就行

---

## Phase 1 — TLSNotary 最小可运行 (~45min)

目的是先不管 TEE，把 Prover + Notary + Verifier 三方跑通一次完整的 attestation，对 tlsn 的 API 形状有手感。

- [ ] clone `tlsnotary/tlsn` 最新 release — `git clone https://github.com/tlsnotary/tlsn && cd tlsn`
- [ ] 跑通官方 `attestation` example，目标站点用默认的 `example.com` — 按 `crates/examples/attestation/` 下的 README 顺序跑 prover 和 notary，最后得到一个 `.presentation` 文件
  - 🔍 理解点：整个流程分三个阶段——notarize（Prover 和 Notary 共同完成 TLS 会话并生成 attestation）、present（Prover 从 attestation 里选择性披露字段生成 presentation）、verify（Verifier 验证 presentation）
- [ ] 跑 verify example，把上一步的 `.presentation` 喂进去，看输出 — 重点看 verifier 验证出来的"server name"和"披露的 transcript 片段"
- [ ] 改 example：把目标换成 `api.anthropic.com`，请求打 `/v1/messages`（用你自己的 API key），观察 Notary 那边有没有报错 — 🤖 这一步可以让 Claude Code 帮你改 example 的 request builder
  - 预期可能踩的坑：ECDSA 证书、TLS record size 限制、请求体太大导致 MPC 超时
- [ ] 把 Notary 从本地进程换成连接 `notary.pse.dev` — 改 config 里的 notary address，验证公开 Notary 能不能出 proof
  - 💡 如果跑不通就先用本地 Notary，不影响理解主流程

---

## Phase 2 — 选择性披露策略 (~30min)

目的是搞清楚怎么控制 proof 里披露什么、遮盖什么。你的场景里请求头有 API key 不能露，响应要能被 A 验证，这块是设计重点。

- [ ] 打开 Phase 1 生成的 attestation，看它的 transcript 结构 — 通常是 sent bytes（请求）和 recv bytes（响应）两段
- [ ] 用 tlsn 的 commit API 把请求拆成 ranges：method+path 披露，`Authorization` header 遮盖，body 披露 — 🔍 理解点：披露粒度是 byte range 级别，不是 HTTP 语义级别。要自己计算字节偏移，或用 tlsn 提供的 HTTP parser helper。
- [ ] 生成一个只披露部分字段的 presentation，用 verifier 验证，确认遮盖字段是 `******` 或空白 — 验证 verifier 那边只能看到披露部分
- [ ] 写一份遮盖规则草稿，列出 Claude API 请求和响应里每个字段的策略 — 🔍 理解点：A 需要能对比"自己发的请求"和"披露的请求体"，否则代理可以篡改模型参数。响应方面 `x-request-id`、status code、body 都要披露。

---

## Phase 3 — TEE 基础：先跑 AWS Nitro Enclaves (~60min)

目的是选一个 TEE 类型上手。Nitro 比 SGX 门槛低（不需要特殊硬件，EC2 就能跑），工具链也更现代，适合做 gateway 类服务。

- [ ] 创建一个支持 Nitro Enclaves 的 EC2 实例 — 实例类型选 `m5.xlarge` 或更大，创建时勾选 Enable Nitro Enclaves；region 选离你近的
- [ ] 装 `nitro-cli`，跑 AWS 官方的 hello-world enclave — 按 [Nitro Enclaves Getting Started](https://docs.aws.amazon.com/enclaves/latest/user/getting-started.html) 走一遍，最后 `nitro-cli describe-enclaves` 能看到运行中的 enclave
  - 🔍 理解点：Enclave 是一个从 parent EC2 切出来的独立 VM，没有网络、没有磁盘、没有交互 shell。和外界通信只能走 vsock。
- [ ] 写一个最小 enclave 程序：从 vsock 接收一个字符串，返回 SHA256 — 用 Rust，参考 `aws-nitro-enclaves-sdk-rust` 里的 example
- [ ] 从 parent 实例向 enclave 发请求，确认通信通畅 — 🤖 这一步 Claude Code 能直接搭好
- [ ] 调用 `nsm` (Nitro Security Module) 生成一份 attestation document — 关键 API 是 `nsm_get_attestation_doc`，返回的是一个 COSE_Sign1 结构，里面含 PCR measurement
- [ ] 在 parent 实例上验证这份 attestation document — 🔍 理解点：验证链是 attestation doc 的签名 → AWS Nitro 根证书。AWS 公开发布了根证书，离线也能验。
  - 💡 把验证代码抽出来独立成一个 crate，后面 A 端要用

---

## Phase 4 — TEE 里跑 Notary (~90min)

目的是把 Phase 1 的 notary-server 塞进 enclave 里跑起来，这是整个方案的核心组件。

- [ ] 把 `tlsnotary/tlsn` 的 `notary-server` 交叉编译成静态 Linux 二进制 — `cargo build --release --target x86_64-unknown-linux-musl`，避免 enclave 里缺 glibc
- [ ] 写 Dockerfile 打包 notary-server，然后用 `nitro-cli build-enclave` 转成 eif 镜像 — 记录下 PCR0/PCR1/PCR2 的 measurement 值，这是 A 端验证时要认的"正确代码哈希"
- [ ] 解决 enclave 的网络问题 — Enclave 本身没网络，需要在 parent 实例跑一个 vsock-proxy 把出向 TCP 流量转发到 `notary.anthropic.com` 以及 Prover — 🤖 这部分 Claude Code 能生成
  - 🔍 理解点：这里有个微妙的信任问题。parent 实例跑的 proxy 是不可信的，但它只能决定"让不让流量出去"，不能看流量内容（流量到了 enclave 内部才被处理）。
- [ ] 启动 enclave 版 notary-server，让 Phase 1 的 prover 连上来跑一次 attestation — 预期会比本地 notary 慢，记下延迟数字
- [ ] 让 notary-server 把自己的公钥放进 attestation document 里 — 🔍 理解点：这是把 TEE attestation 和 Notary 签名绑定的关键。enclave 启动时生成一对签名密钥，把公钥 hash 放进 attestation 的 user_data 字段，这样任何人拿到 Notary 签名 + attestation doc，就能确认"这个签名是这个 measurement 对应的 enclave 出的"
- [ ] 把 attestation doc 跟 notary 签名一起打包返给 Prover — 这是 presentation 最终要带的两份东西

---

## Phase 5 — 完整信任链打通 (~45min)

目的是模拟 A 端的验证流程，确认从零信任出发能一路验到 Anthropic。

- [ ] 写一个 Verifier 程序，接受 `{presentation, attestation_doc}` 作为输入 — 这就是 A 端要跑的东西
- [ ] Verifier 的验证步骤依次写清楚：
  - [ ] 用 AWS Nitro 根证书验 attestation_doc 签名
  - [ ] 比对 attestation_doc 里的 PCR measurement 和已知的"正确 notary 代码哈希"
  - [ ] 从 attestation_doc 里取出 notary 公钥
  - [ ] 用该公钥验 presentation 上的 notary 签名
  - [ ] 验 presentation 里的 TLS 证书链到 Web PKI 根，确认 server name 是 `api.anthropic.com`
  - [ ] 解析披露的 transcript，提取请求字段和响应字段
- [ ] 让 A 端 Verifier 跑完整验证流程，输出 "verified: true, server: api.anthropic.com, request_model: claude-opus-4, response_body: ..." — 🤖 Claude Code 可以直接搭出来
- [ ] 把 PCR measurement 值写进一个公开的 `expected_measurements.json`，模拟"发布"给 A 端的动作 — 🔍 理解点：这一步是信任锚。A 凭什么信这个 measurement 是"正确的 notary 代码"？靠 reproducible build + 你把源码和构建脚本公开 + 社区审计。

---

## Phase 6 — 压测和 SSE 场景验证 (~60min)

目的是把 PoC 推到你真实业务场景看能不能扛住。

- [ ] 跑一次 SSE 流式 Claude API 调用，让 MPC Notary 全程参与 — 用 `/v1/messages` 的 `stream: true`，生成一段 500 token 的回复，观察整个会话时长
- [ ] 记录几个数字：MPC 握手延迟、流式过程中的额外延迟、总带宽消耗 — 和普通 HTTPS 调用对比
- [ ] 测试多并发：同时跑 10 个 attestation 会话，看 enclave CPU 和内存占用 — 🔍 理解点：enclave 的 CPU 和内存是启动时固定分配的，这决定了你能并发多少个会话
- [ ] 💡 尝试 prover 和 notary 网络断开的情况，看 tlsn 怎么处理中途失败 — 异常处理对生产环境重要
- [ ] 💡 看看 enclave 重启后 notary 签名密钥怎么处理 — 每次重启生成新密钥意味着历史 attestation 可验证性取决于"当时的公钥是否被记录过"

---

## 品味校准 (~5min)

做完前面所有 Phase 再回来答这些，答案写完三个月后回来看。

- [ ] 生死判断 — TLSNotary 这个协议会成为 Web2→Web3 的标准数据通道，还是会被 Reclaim 或者其他方案吃掉？
- [ ] 需求真伪 — "证明某个 HTTP 响应来自某个服务器"这个需求，除了你的 LLM 代理场景，真的有多少地方在买单？还是一个找问题的解决方案？
- [ ] 核心设计决策 — TLSNotary 选了 MPC 而不是 ZK 证明 TLS session key，这个 tradeoff 你认同吗？放在你自己的场景里（不是链上验证），这个选择是对的吗？
- [ ] DX 打分（1-5）— tlsn 的 Rust API 成熟度、Nitro Enclaves 的工具链、文档完整度、报错信息质量，分别打分
- [ ] 一句话总结 — 如果你要劝朋友"别用 TLSNotary+TEE，用 Reclaim 就行"，你的一句话理由是什么？反过来劝他"值得自己搭"的理由是什么？
- [ ] 给你场景的额外问题：你作为代理跑 Notary，这个方案里"你既是被怀疑的人，又是给用户出 proof 的人"，有没有利益冲突导致 A 不买账？如果 A 要求 Notary 由第三方跑，你愿意放弃控制权吗？