# zeroseal MVP 构建 todolist

> 推进过程的可勾选清单。源真相是 `../PRD.md`,详细推理是 `~/.claude/plans/staged-squishing-nest.md`。
>
> 标记说明:`[用户]` = 用户在阿里云控制台 / 外部系统操作;`[CC]` = Claude Code 写代码 / 写文档;`[等待]` = 等外部事件(备案、工单回复、用户输入)。

---

## Phase 0 — Bootstrap (Day 0-3)

### 阿里云资源(用户控制台)

- [x] [用户] 阿里云控制台买 `zeroseal.cn` 域名(~¥39/年)

- [x] [用户] 提交 `zeroseal.cn` 备案(挂已有备案主体下"新增网站")→ [等待] 3-7 天

- [x] [用户] 下单 `ecs.c9i.xlarge`,华北 2 北京可用区 I,**按量付费**(等工单 1 确认再转月付) https://ecs.console.aliyun.com/server/region/cn-beijing#/

- [x] [用户] 申请独立 EIP(北京,BGP 多线,带宽 5-10 Mbps 起步)并绑定到 c9i 实例 — 不用 ECS 自带公网 IP。理由见 PRD §服务拓扑https://vpcnext.console.aliyun.com/eip/cn-beijing-btc-a01/eips

- [x] [用户] K 服务器已下单(经济型 e,北京同 VPC,公网 39.105.82.40,2C2G,Ubuntu 24.04)

- [x] [用户] RDS PG 18.0 已建(基础版 2C4G,北京同 VPC,`pgm-2ze04krx1h1wk06d.pg.rds.aliyuncs.com:5432`,白名单加 K + TD 内网)

- [x] [用户] RDS 控制台建库 `zeroseal_db` + 应用账号 `zeroseal_app`(账号密码 mac env vars `ZEROSEAL_RDS_ACCOUNT_NAME/PASSWORD`)

- [x] [用户] 在 K 服务器上准备 secrets endpoint(MVP 替代 KMS,纯内网 HTTP):
  
  - 文件 `/etc/zeroseal/secrets/zeroseal-prod.env`(权限 `640`,`root:www-data`)
  
  - K 上 nginx 监听内网 IP + 自定端口,location `/secrets/zeroseal-prod.env` 返回上述文件;**无 TLS**(K↔TD 同 VPC) 
  
  - 阿里云安全组:inbound 限定 source = TD 内网 `172.20.197.165`
  
  - TD 上 `/etc/hosts` 加 `<K 内网 IP> internal-secrets`(或直接代码里硬编码 IP)
  
  - 🌵：已让 cc 二次确认配置，以及加固了安全组。目前 sshd 有 5 次验证失败就 ban 10 分钟 ip 的规则，且只能公私钥登陆。所以 ssh 开放端口没事。
  
      >   sshd 是 OpenSSH 的服务端守护进程,跑在服务器上监听 22 端口(默认),负责 SSH 连接的认证、加密和会话管理,主配置在 `/etc/ssh/sshd_config`,推荐用 `/etc/ssh/sshd_config.d/*.conf` 写 drop-in 覆盖而不动主文件;改完先 `sshd -t` 验证语法,再 `systemctl reload sshd` 重载(保留现有连接),不要用 restart(会断当前会话),在没有 VNC/控制台兜底的机器上尤其要这样,避免改错锁死。
  
- [ ] [用户] 申请 `gw.zeroseal.cn` DV SSL 证书(阿里云 SSL 证书服务,免费版即可,等**域名+备案下来后才能签**) [[[skip]]]

- [ ] [用户] 验证 K ↔ TD 内网互通:`nc -zv <K 内网 IP> <secrets 端口>` from TD;`nc -zv pgm-2ze04krx1h1wk06d.pg.rds.aliyuncs.com 5432` from K 和 TD

- [ ] [用户] K 上 `apt purge -y aliyun-assist aegis cloudmonitor logtail`(MVP 推进期可推迟到上线前做)

### 阿里云工单(用户提交,基于 `ops/aliyun-tickets.md` 内容)

调研结果后已**删除工单 1+2**(见 `ops/aliyun-tickets.md` 顶部说明),只剩 3+4:

- [ ] [用户] 工单 4(P1):TDX 实例硬件维护性迁移对 measurement 的影响
- [ ] [用户] 工单 3(P2):远程证明服务 appraisal policy 公开化
- [ ] [等待] 工单 3+4 回复 → 更新 `ops/aliyun-tickets.md`

### 文档落盘(CC,已完成基础部分)

- [x] [CC] `.gitignore`
- [x] [CC] `PRD.md`(多轮讨论结论首次落盘)
- [x] [CC] `CLAUDE.md`(操作约束 + 引用 rules + 拓扑硬约束)
- [x] [CC] `todolists/zeroseal-construction.md`(本文件)
- [x] [CC] `ops/aliyun-tickets.md`(4 个工单完整正文)

### 仓库骨架(CC,已完成)

- [x] [CC] `backend/gateway/cmd/gateway/main.go` hello-world + go.mod
- [x] [CC] `web/panel/` Vite + React + TS 初始化(实际用 `npm create vite`,本机未装 pnpm)
- [ ] [CC] `web/landing/` 占位(简单 index.html 或 Astro,Phase 1 末再做)
- [x] [CC] `git init` + 主分支 `dev` + 首个 commit(4 个 conventional commits)

### 等待用户输入

- [ ] [等待] 用户提供另一个手机验证码 + 支付宝项目的 codemap(Phase 1 panel + Phase 2 支付复用)

**Phase 0 退出条件**:域名已购、备案已提交、c9i 实例 running、K 实例 running、RDS 已建库 + 账号、K 上 secrets endpoint 配好、2 工单已发(#3+#4)、仓库骨架可 `go build`、`pnpm dev` 跑通空页面、K↔TD↔RDS 内网三方互通已验证。

---

## Phase 1 — MVP Core (Day 4-14, 备案等待期就是黄金窗口)

### 后端 Gateway (Go, 跑在 TDX 实例) — 遵循 `~/.claude/rules/golang/`

- [ ] [CC] OpenAI 兼容:`POST /v1/chat/completions` + `GET /v1/models`,SSE 流式透传
- [ ] [CC] 上游路由:`deepseek-*` → DeepSeek 官方;`qwen-*` → DashScope;`claude-*`/`gpt-*` → 401
- [ ] [CC] API key 鉴权中间件(`Authorization: Bearer sk-zs01...`)
- [ ] [CC] 计费 reservation 模式:`SELECT FOR UPDATE` 锁余额 → 预扣 → 上游 chunk 累计 → commit;TTL 5 分钟,worker 扫表;`balance_events` append-only
- [ ] [CC] 上游故障熔断:5 分钟错误率 >30% 自动切;响应 header `X-Upstream-Fallback`
- [ ] [CC] 限流:按 API key 默认 60 req/min
- [ ] [CC] Attestation endpoints:`/api/attestation/{quote,measurement,bundle}`
- [ ] [CC] dev mode(`ZEROSEAL_DEV=1`):env 读 secret + fake quote
- [ ] [CC] EKM channel binding 协议实现(`POST /api/attestation/channel-bind`)

### DB Schema (sqlc + goose, 阿里云 RDS PG 18.0)

- [ ] [CC] migrations:`users` `api_keys` `sms_verifications` `topups` `api_calls` `balance_events` `reservations`
- [ ] [CC] sqlc 生成 Go DAO,统一 `backend/store/` 包
- [ ] [CC] advisory lock 处理并发

### Secrets 加载(MVP 走 K 内网 HTTP,见 PRD §密钥放置表)

- [ ] [CC] `secretsClient` 抽象接口:`Get(name string) (string, error)`,MVP 实现是 HTTP GET 到 K 内网 secrets endpoint,v2 换 KMS 实现
- [ ] [CC] TD 启动序列:HTTP GET `http://<K 内网 IP>:<端口>/secrets/zeroseal-prod.env` → 加载内存,不落盘
- [ ] [CC] 四条凭据:`DB_PASSWORD`(RDS 账号 zeroseal_app)/ `DEEPSEEK_API_KEY` / `QWEN_API_KEY` / `GW_TLS_KEY`(Phase 2 入)
- [ ] [CC] Secrets 拉取失败 fail-fast,不 silent fallback
- [ ] [用户] 在 K 上 `secrets/zeroseal-prod.env` 文件填入实际凭据值(权限 640,root:www-data)。DB 密码从 mac env `ZEROSEAL_RDS_ACCOUNT_PASSWORD` 安全注入(SSH 进 K 手填,不经过 CC)

### Panel (web/panel, Vite + React + TS) — 遵循 `~/.claude/rules/typescript/`

- [ ] [CC] 基础栈搭骨架(路由 + 状态管理 + API client)
- [ ] [等待] 用户提供另一项目 codemap → 集成手机验证码 + 支付宝
- [ ] [CC] 路由:`/panel/login` `/panel/dashboard` `/panel/api-keys` `/panel/topup` `/panel/usage`
- [ ] [CC] 认证:手机号 + 短信验证码 + JWT in httpOnly cookie
- [ ] [CC] API key 管理:生成(只显示一次)、撤销、列表(只前缀)
- [ ] [CC] 用量展示:查 `api_calls` 聚合
- [ ] [CC] 联调用 c9i 公网 IP + 临时 hosts(不依赖 zeroseal.cn 解析)
- [ ] [CC] 试用 `~/.claude/skills/stock-frontend-react-patterns.md` 中模式作 UI 起点
- [ ] (可选)[CC] 试用 `frontend-design` plugin

### Landing (web/landing)

- [ ] [CC] 简单静态页(Astro 或纯 HTML),价值主张 + "进入控制台"按钮
- [ ] 3D Hero 推迟 Phase 2 Polish 或 Phase 3

### 测试

- [ ] [CC] 单测紧挨实现:`gateway/router_test.go` `billing/reservation_test.go`
- [ ] [CC] 集成测试 `backend/tests/integration/`:upstream mock + 完整流程

**Phase 1 退出条件**:用 c9i 公网 IP + 临时 hosts,跑通端到端"手机注册 → 拿 API key → curl `/v1/chat/completions` → SSE 流 → 扣费 → panel 查用量"。

---

## Phase 2 — MVP Polish & Launch (Day 15-21, 依赖备案通过)

### 域名 + 证书

- [ ] [等待] 备案通过
- [ ] [用户] DNS 解析:`gw.zeroseal.cn` → c9i EIP `39.106.110.87`;`zeroseal.cn` → K 公网 `39.105.82.40`
- [ ] [用户] `gw.zeroseal.cn` 证书签发,私钥写入 K 上 `secrets/zeroseal-prod.env`(GW_TLS_KEY 字段)
- [ ] [CC] gateway 启动序列加 TLS 证书加载逻辑,listen 443 自终止
- [ ] [CC] K 上写 nginx server block:`zeroseal.cn` 路径分流 `/` → landing,`/panel` → SPA,`/api` → panel 后端,`/verify` → verifier(K 是新机,从零写,无遗留)

### 支付闭环(复用用户另一项目)

- [ ] [等待] 用户给那个支付宝项目 codemap
- [ ] [CC] 移植:支付宝 web 支付 + mobile web 跳 APP + webhook 验签 + idempotency + 订单状态机
- [ ] [CC] 充值闭环:panel 选金额 → 跳支付宝 → 回调 → `balance_events` → 余额刷新

### Verifier (web/verifier)

- [ ] [CC] WASM 验证(用 `@lit-protocol/dcap-qvl-ts` 或 `Phala-Network/dcap-qvl` 编 WASM)
- [ ] [CC] UI:`zeroseal.cn/verify` → fetch quote → WASM 验 → 显示比对结果
- [ ] [CC] 测试:valid quote + tampered quote 各跑一遍

### 生产上线 checklist

- [ ] [CC] 限流:nginx (K) + 后端 (TD) 双层
- [ ] [CC] 结构化 slog,字段白名单 + lint 禁 log 黑名单字段
- [ ] [CC] Prometheus pull,`:9090/metrics` 聚合
- [ ] [用户] SWIOTLB 监控:`dmesg | grep swiotlb` cron 告警
- [ ] [用户] K 上 nginx access log 接 SLS(monitor secrets endpoint 访问,异常 source IP 告警)
- [ ] [用户] 备份:阿里云 RDS 自动备份策略 + `balance_events` 单独导出(账本审计)

### Trust Anchor

- [ ] [用户] 建 GitHub repo `zeroseal-trust-anchor`(公开)
- [ ] [CC] 首次发布:Phase 1 末稳定 measurement + reproducible build 脚本 + README

**Phase 2 退出条件**:
- 外部用户从 `zeroseal.cn` 跑通"手机注册 → 支付宝充值 ¥1 → 拿 key → 调 `gw.zeroseal.cn/v1/chat/completions` → SSE → panel 看扣费"
- `zeroseal.cn/verify` 能跑通 attestation 验证
- **按 `ops/security-checklist.md` 全跑一遍,任意一条不过不上线**

---

## Phase 3 — 推迟项(Day 22+, 不阻塞 MVP)

- [ ] [用户] OpenClaw 澄清(自有 / 外部开源 / 源码位置 / 版本节奏)
- [ ] [CC] desktop/packer:config 注入 + gateway endpoint 注入 + 激活码界面
- [ ] [Zeni] 会计版 config(系统提示词 + 知识库 + 工作流 + 示例任务)
- [ ] [CC] OpenClaw 内置 EKM channel binding(Go/Rust SDK,默认开启)
- [ ] [CC] 字段加密回填:KMS master key + HKDF DEK + AES-256-GCM + HMAC 索引
- [ ] [CC] Claude/GPT 出海版(阿里云国际版,独立实例 + 独立信任锚 + `zeroseal.ai`)
- [ ] [CC] measurement 评估策略可公开化(看工单 3 回复)
- [ ] [设计] 3D 玻璃 Hero(landing 升级)
- [ ] [用户] PostgreSQL 迁移到 RDS(用户量上来后)

---

## 待用户提供的输入(汇总)

1. ~~A 服务器当前 nginx 配置~~ — A 已脱钩,K 是新机从零配,不需要
2. **另一项目(手机验证码 + 支付宝)codemap** — Phase 1 panel 集成 + Phase 2 支付闭环
3. **OpenClaw 是什么** — Phase 3 启动前必须澄清

## 关键路径

```
域名下单 → 备案提交 → 备案通过(3-7 天) → DNS 解析 → 证书签发 → 上线
```

**MVP 上线 ≈ Day 0 + 备案天数 + 2-3 天联调**。
