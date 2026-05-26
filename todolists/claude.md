# tee-stack — CC 操作约束

<!--
本文件 CC 每次新对话 / 压缩后自动加载到 context。遵循 Anthropic 官方建议:

- 目标 < 200 行
- 具体可验证指令 ("Run X" > "Test it")
- 不放短期状态 (测试期标志这种, 进 PRD 或 todolist)
- 详细背景看 PRD.md, 这里只放 CC 反复需要的硬规则
-->

项目 = TEE 可验证 LLM Gateway + 网页端（landing page + panel）+ 语义调教的 Openclaw (Go + Next.js(待定，我之前纯 React，需要讨论框架/定制/商业展示交付问题。我需要给用户一个很强的交付的商业landing page) + 阿里云 TDX(待定)) + 可能没讨论到的子项目。详见 PRD.md。

**不确定时 STOP 问用户, 不要猜了执行**。适用于: 跨子项目 schema 变更、measurement 是否需要更新、激活码逻辑该不该改、KMS policy 调整、命令影响范围不明等。

---

## monorepo 子项目边界 (硬规则)

```
tee-stack/
├── backend/      ← Go, 跑进 TDX enclave
├── web/          ← Next.js, 落地页 + 文档站 + verifier
├── desktop/      ← OpenClaw 行业版打包工程
├── deploy/       ← TDX 部署 + Terraform
└── scripts/      ← 跨子项目工具
```

CC 用 `pwd` 判断当前位置, 命令路径**永远从 monorepo 根开始**, 不要假设用户当前在某子目录:

```bash
# ✅ 正确
cd backend && go test ./...

# ❌ 错误 (假设用户已在 backend 目录)
go test ./...
```

子项目独立开发、独立测试、独立发版。**跨子项目改动一律拆 commit**, 一个 commit 只动一个子项目, 跨子项目用 `feat(both-...)` 或多个独立 commit。

---

## Git 工作流 (硬规则)

主 repo (`<repo_root>/`) HEAD 永远在 dev, **不起 issue 分支**;
CC 在池里 (`tee-stack-pool-N`) 推 issue 分支。每个 feat/fix/refactor/docs 推进
**走 issue 分支**, **不直接 commit 到 dev**。

```bash
git checkout -b issue-N

git add ... && git commit -m "feat(...): ..."
```

**做完 STOP — 不自己 merge 到 dev**。把 commit 留在 issue branch 等用户验证后合。

### 合 issue 到 dev 的命令格式 (硬规则)

CC 给用户的合并命令**必须**分三步 (池里没法 checkout dev, dev 被主 repo 独占):

```bash
# Step 1: 池里 snapshot todolists
git add todolists/ && \
  git commit -m "chore(todolists): snapshot" 2>/dev/null
```

```bash
# Step 2: 用户切到主 repo merge
cd <repo_root> && \
  git merge --no-ff issue-N
```

```bash
# Step 3: 池回停车位 (fast-forward 到刚合的 dev)
cd <repo_root>-pool-N && \
  git checkout pool-N && \
  git merge dev --ff-only
```

无 todolists/ 改动时 commit 静默失败 (`2>/dev/null`)。

❌ 不要给单步 `git merge --no-ff issue-N`
✅ 必须给上面的三步

### 新 issue vs 留原 branch (硬规则)

新推进前按顺序判断:

1. 是不是修**最近几个 issue** 留下的 bug? → **留原 issue branch**, 用 cherry-pick
2. 那些 issue 都已 merge 到 dev 了? → 才开新 issue branch
3. 不确定属于哪种? → **问用户**, 不要猜

历史教训: Panel 项目 issue-14/15 拆错过一次, issue-17/18 又拆错一次 — 默认倾向「留原 branch」。

## Worktree 并行 (硬规则)

仓库支持多 worktree 并行开发, 结构:

- `<repo_root>/` — 主 repo, 永远在 dev, 只负责 merge
- `<repo_root>-pool-N/` — 干活池 N, 绑专属占位分支 `pool-N`

CC 用 `pwd` 判断当前位置: 含 `-pool-` 在池里, 否则在主 repo。

### 池子分支语义

每个池绑一个**专属占位分支** `pool-N`, 作为池子的「停车位」。pool-N 分支本身**不直接 commit**, 只是停车位。

完整一轮:

1. 池 N 在 pool-N (停车位)
2. CC `git checkout -b issue-X` → 池在 issue-X
3. 推完 + snapshot todolists, STOP 等用户
4. 用户主 repo merge
5. 池回停车位: `git checkout pool-N && git merge dev --ff-only`

CC 在池里 `git branch --show-current`:
- 输出 `pool-N` → 在停车位, 准备起新 issue
- 输出 `issue-N` → 正在推进 issue
- **不会**输出 `dev`

### 池子里的两个会话

每个池目录用户已开两个 shell:
- CC 会话 — Claude Code 进程
- 用户会话 — 普通 bash, 用户手动 build & run / merge

两个会话 cwd 独立。CC 输出给用户的命令一律按「用户当前在池子根目录」假设, **永远不**加 `git -C` / 多余 `cd` / 绝对路径前缀。

### 联调验证规则

任何时刻**最多一个 worktree** 在跑 backend (端口 8443 独占) 和 web (端口 3000 独占)。

- CC **不跑 build & run**, 都给用户。STOP 时给启动指令:
  - backend 改动: `cd backend && go build -o ./bin/gateway ./cmd/gateway && ./bin/gateway`
  - web 改动: `cd web && pnpm dev`
  - desktop 改动: `cd desktop/packer && ./build.sh accounting` (产物在 `desktop/installers/`)
- 切池验证前用户先 `pkill -f tee-stack-gateway && pkill -f next-server`

### Worktree 文件共享

worktree 之间共享 `.git/` 和 `.gitignore`, working tree 文件独立。

- `todolists/` — 每池独立笔记, merge 时各池带各池的 snapshot
- `backend/data/` — 已 gitignore, 各池独立测试 DB
- `web/.next/`, `web/node_modules/` — 已 gitignore
- `desktop/installers/build/` — 已 gitignore (构建产物)

### 冲突处理

并行 merge 串行进行。池里**不主动 rebase**, 在主 repo merge 时就地解。

---

## TEE 相关硬规则

### Measurement 不能随便变

每次 backend 代码改动都会改 measurement, 进而影响 KMS 策略和用户信任。**CC 改 backend 代码时必须意识到这点**:

- 改业务逻辑 = measurement 变 = trust-anchor 仓库要发新版
- 用户在用旧版 measurement 验证的话, 会失败 → 必须提前 announce
- 灰度期 30 天, 期间新老 measurement 都在 KMS 白名单

不确定改动会不会触发 measurement 变化时 → **STOP 问用户**。

### 本地 dev 模式 vs 生产模式

- dev 模式: `GATEWAY_DEV=1`, 从环境变量读 secret, 不连 KMS, 不出 attestation
- 生产模式: 不带 `GATEWAY_DEV`, 启动时强制连 KMS, 必须能出 attestation

**两套 binary 必然不同**, dev binary 永远跑不进生产 enclave (代码里有 dev mode 检查)。CC 写代码时记得加 `if devMode { ... } else { kmsClient.Get(...) }` 这种分支。

### Secret 不能日志输出

API key、DEK、激活码、用户 PII 一律不能 log。CC 写日志时:

```go
// ✅
log.Info("api call", "user_id", userID, "model", model, "tokens", tokens)

// ❌
log.Info("api call", "api_key", apiKey)
log.Info("user", "email", user.Email)
```

ESLint / golangci-lint 配置里加规则禁止 log 这些字段。

---

## Conventional Commits

格式 `<type>(<sub-project>-<module>[-<feature>]): ...`:

- `feat(backend-gateway-路由): 加 qwen 路由`
- `feat(backend-attestation): 暴露 measurement endpoint`
- `feat(web-landing-hero): 3D 玻璃动画接入`
- `fix(desktop-packer): macOS 公证流程修复`
- `feat(deploy-tdx): terraform 模板初版`
- 跨子项目: `feat(both-billing): ...`

type: `feat / fix / refactor / docs / test / chore / perf / ci`。**粒度细** — 一个 logical change 一个 commit。

## 数据保护 (硬规则)

`todolists/*.md` 是变化最频繁的文件, 跟 commit 节奏不同步。

- **默认避免 `git reset --hard`**, 优先 `git revert` 或 fix-forward
- 必须 reset --hard 时先 stash:

```bash
git stash push -m "save todolists" -- todolists/
git reset --hard <commit>
git stash pop
```

- 单文件回退用 `git checkout <commit> -- <path>`
- 已 gitignore: `backend/data/`, `backend/bin/`, `web/.next/`, `web/node_modules/`, `desktop/installers/build/`, `*.db`, `.env*`

---

## 常用命令

```bash
# Backend 改完 (Go)
cd backend && \
  go build -o ./bin/gateway ./cmd/gateway && \
  ./bin/gateway

# Backend 测试
cd backend && go test ./... -race

# Backend 集成测试 (需要本地 postgres)
cd backend && \
  go test -tags=integration ./tests/integration/...

# Web 改完
cd web && pnpm dev

# Web 构建
cd web && pnpm build

# Desktop 打包 (会计版)
cd desktop/packer && \
  ./build.sh accounting

# Desktop 公证 (macOS, 需要 Developer ID + notarytool)
cd desktop/packer && \
  ./notarize.sh ../installers/accounting-v1.pkg

# 验证 attestation (开发期)
cd scripts && \
  ./verify-attestation.sh https://api.tee-stack.com/attestation/quote
```

## 测试沉淀 (硬规则)

**每个 feat/fix 的复现路径必须沉淀成测试代码**, 不能 curl 验完就消失 —
context 压缩后 LLM 拿不到测试方法 / 容易重复踩坑。

布局:

- 单测: 紧挨实现, `<pkg>/<code>_test.go` (如 `gateway/router_test.go`)
- 集成测试: `backend/tests/integration/` (需要 postgres + 上游 mock)

何时进集成测试 (而不是仅单测):

- 跨包状态机 (gateway + billing + store 三包以上)
- 上游 API 真实交互 (用 httptest server 模拟 DeepSeek / Qwen)
- attestation 生成 + 验证全链路
- 任何用户在 issue 推进时 curl 验过的复现路径

## 关键路径

- backend HTTPS `:8443` (生产 TDX 实例) / `:8080` (健康检查)
- backend dev `:8089` (本地, `GATEWAY_DEV=1`)
- web dev `:3000`
- web prod `tee-stack.com` (阿里云 OSS + CDN)
- PostgreSQL: 阿里云 RDS (生产) / 本地 docker (dev)
- KMS: 阿里云 KMS, key alias `tee-stack-master-v1`
- Trust anchor repo: `github.com/<org>/tee-stack-trust-anchor` (公开)

---

## 命令格式 (iTerm2 wrap 防护, 硬规则)

CC 输出的代码块在 iTerm2 / 大多数终端里会按窗口宽度 wrap 成多物理行 — 用户复制
粘贴时, wrap 后的尾部 token 常被当成下一行命令独立执行, 导致行为错乱。

**任何超过 ~80 字符的 shell 命令必须用 `\` 续行**, 每物理行 < 80 字符。

❌ 错误:

```bash
cd backend && go build -o ./bin/gateway ./cmd/gateway && ./bin/gateway --config ./config/dev.yaml
```

✅ 正确:

```bash
cd backend && \
  go build -o ./bin/gateway ./cmd/gateway && \
  ./bin/gateway --config ./config/dev.yaml
```

## 验证后端改动

backend 修完必须实际启服务跑请求验, 不能只 `go build` 通过:

- 改路由 → curl 一次每个支持的 model, 看流式是否完整
- 改 billing → 跑一遍激活码生成 + 激活 + 扣费 + 余额查询的集成测试
- 改 attestation → 起一次本地 mock TDX (dev mode 用 fake quote), 跑 verifier 验通过
- 改 store → 跑 migration + 集成测试, 检查加密字段读写

## 验证 web 改动

- 改 landing → `pnpm dev`, 浏览器看 3D hero 动画顺畅 + 落地页移动端响应式
- 改 docs → `pnpm build` 检查 MDX 编译无误 + 链接无 404
- 改 verifier → 用一个已知 valid 的 attestation 粘贴进去, 应该 verified ✓; 用 tampered 的应该 ✗

## 验证 desktop 改动

- 改 packer → `./build.sh accounting`, 装到本机, 跑一遍完整激活流程
- 改激活逻辑 → 必须测两种异常: 错误激活码 / 重复激活 / 过期激活码

## Conventions

- commit message 中文 OK, scope 用中文 (如 `feat(backend-gateway-路由): ...`) 或英文都可
- 主分支 `dev` (不是 `main`)
- 用户偏好「milestone 拆细 + 每完成停下等确认」, 不要一次性跑完多个未确认的 milestone
- 用户偏好简体中文回复 (含错误 message 翻译)
- 写代码注释保留中文, 但函数名、变量名一律英文

---

## 引用

- 详细 PRD: `<repo_root>/PRD.md`
- 构建 todolist: `<repo_root>/todolists/tee-stack-construction.md`
- 历史进度: `~/.claude/projects/-Users-*-tee-stack/memory/MEMORY.md`
- 参考项目 (类似规范): Panel 项目 (`/Users/kinesis/panel/CLAUDE.md`)