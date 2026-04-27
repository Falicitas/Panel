# Freestyle Sandbox 上手 Checklist

从零到能在自己的 agent 工作流里创建/fork/销毁 VM。

前置：Node.js 18+ 或 Bun，需要 Freestyle API key（官网注册免费额度）。

---

## Phase 0 — 拿到 API key，跑通 hello world (~15min)

- [x] 🌟 注册 freestyle.sh 账号 — 拿到 API key：

    ```
    3oarrRczrj9xSg3vghoSKt-61aTzTNsFfPL1rVZhCyGQvCEthkatequuNcU844KcuTZ
    ```

- [x] 初始化项目 — `mkdir freestyle-lab && cd freestyle-lab && npm init -y && npm i freestyle-sandboxes`

- [x] 配置环境变量 — `export FREESTYLE_API_KEY="your-key"`，写进 `.env`

- [x] 跑通最小 demo — 创建 VM + 执行一条命令 + 拿回 stdout：
  ```ts
  import { freestyle, VmSpec } from "freestyle-sandboxes";
  import { VmBun } from "@freestyle-sh/with-bun";
  const { vm } = await freestyle.vms.create({ spec: new VmSpec({ with: { bun: new VmBun() } }) });
  const { stdout } = await vm.exec("echo hello from sandbox");
  console.log(stdout);
  ```
  验证：终端输出 `hello from sandbox`
  
- [x] 🔍 看一下 VM 创建耗时 — 官方说 <700ms，自己 `console.time` 量一下实际值

---

## Phase 1 — VM 生命周期管理 (~20min)

掌握 create → exec → hibernate → resume → destroy 全流程。

- [x] 🌟🌟 创建 persistent VM — `persistence: { type: "persistent" }`，记下返回的 `vmId`。

    结论：用不了，要冲钱。但多数场景临时够用

- [x] 在 VM 里装包跑代码 — `vm.bun.install({ deps: ["zod"] })` + `vm.bun.runCode(...)`，拿到结构化 result

- [x] hibernate VM — 调 hibernate API 后确认 VM 状态变化

- [x] resume VM — 重新唤醒，验证之前装的 zod 还在（跑同样的代码不需要重新 install）

- [x] 删除 VM — `freestyle.vms.delete(vmId)`，确认清理干净
  - 🔍 hibernate 状态下不计费，这是省钱的方式

---

## Phase 2 — Fork：sandbox 的杀手特性 (~20min)

Fork 是 Freestyle 区别于 E2B/Docker 的地方。

- [x] 🌟🌟 创建一个 VM，在里面装好 Next.js — `vm.exec("bunx create-next-app@latest my-app -y")`

- [x] fork 出 3 个副本 — `const { forks } = await vm.fork({ count: 3 })`

- [x] 在 3 个 fork 里并行执行不同任务 — 用 `Promise.all` 分别装 shadcn / drizzle / next-auth

- [x] 验证 fork 之间互不影响 — 在 fork[0] 里 `ls node_modules` 确认只有自己装的包
  - 🔍 fork 是 copy-on-write 的，不是完整复制。速度快、内存占用低。这意味着你可以给 agent 的每次尝试开一个 fork，失败了直接丢掉
  
  - 🌵 点评：提供 microVM 能力。`freestyle.vms.create()` 创建 VM，`vm.exec()` 跑 shell 命令，`vm.bun.runCode()` 跑 JS/TS 代码并拿结构化 result。
  
      fork 隔离验证：`vm.fork({ count: 3 })` 返回 `{ vmId, vm }[]`。fork 后文件系统和内存（/dev/shm）各自独立，原始 VM 不受影响。即每个 fork 实例在自己的位置写文件或者创建内存对象，不影响其他 fork 与起始的实例。

---

## Phase 3 — SSH + Identity 权限模型 (~15min)

如果你要在 agent 流程里让外部用户连接 VM，需要理解 identity 系统。

- [x] 🌟🌟 创建 identity + token — `freestyle.identities.create()` → `identity.permissions.vms.grant({ vmId })` → `identity.tokens.create()`

- [x] 用生成的 SSH 命令连进去 — `ssh ${vmId}+developer:${token}@vm-ssh.freestyle.sh`

- [x] 在 SSH session 里执行几条命令，确认权限隔离生效

- [x] 💡 试试 client-side SDK — `new Freestyle({ accessToken: token })`，理解 client token 只能操作已有 VM、不能创建新 VM 的设计

    🌵 点评：服务端收到用户请求后调 `freestyle.vms.create()` 创建 VM 并装好运行环境，然后通过 `freestyle.identities.create()` 创建身份，`identity.permissions.vms.grant({ vmId })` 把这个身份绑定到指定 VM，再 `identity.tokens.create()` 生成 token 返回给前端。前端拿 token 初始化 `new Freestyle({ accessToken: token })`，之后执行代码、读写文件、打开终端这些操作全部直连 VM，不再经过服务端。服务端只负责创建和销毁 VM，不参与中间的交互过程，不承担转发流量的压力。

    实际上我测试时是通过 `bunx freestyle vm ssh 9rc8mrgeie9wshsbboc7` 连接的，手动 ssh 遇到了点问题。不过机制就是这样。

---

## Phase 5 — CLI 工具 (~10min)

- [x] 安装 CLI — npm 包自带，`npx freestyle vm create --ssh` 一步创建+连入
- [x] `freestyle vm list` 看看有哪些活跃 VM。
    -   🌵 点评：他会拿 .env 或者环境变量的 API key，这样就能查到哪些活跃的 vm，然后做运维。
- [x] `freestyle vm exec <vmId> "ls -la"` 远程执行命令
- [x] `freestyle vm delete <vmId>` 清理

---

## 更底层的理解

物理机上的 Linux 内核加载 KVM 模块后具备硬件虚拟化能力，利用 CPU 的 VT-x 指令集直接在硬件层面隔离多个虚拟机的内存和执行环境。VMM（如 Firecracker）是用户态进程，通过 /dev/kvm 接口调用 KVM 来创建和管理 VM——分配 guest memory、模拟虚拟磁盘和网卡、处理 I/O。每个 VM 内部跑完整的 Linux 内核和用户空间，对外表现为一台独立的机器。VMM 能读取和操控 VM 的全部状态（CPU 寄存器、内存页表、设备状态），这是实现 snapshot 和 fork 的前提。

以 Freestyle 的 fork 为例：用户调用 fork API 后，VMM 暂停原始 VM，保存 CPU 寄存器和虚拟设备状态（几 KB），然后创建一个新的 VM 实例，guest memory 不做复制，而是让新 VM 的内存映射指向原始 VM 相同的物理页并标记为只读。两个 VM 各自恢复运行，谁先写入某一页就触发 page fault，内核此时才复制那一页（4KB）给写入方。所以 fork 耗时不随内存大小增长，只有实际被修改的页才产生复制开销。

## 品味校准 (~5min)

- [x] 判断：Freestyle 会成为 AI sandbox 的标准选择还是会被大厂吃掉？

    -   只能说起步时是个方便的服务，而且确实简单，通过 api 调用而不是通过 dashboard 选服务器配置。

- [x] 它解决的问题是真痛点吗？E2B、Modal、Daytona 已经存在，Freestyle 的差异在哪？

    另外三个没怎么了解

- [x] 核心设计决策：自建裸金属 + microVM（不用 Docker）。这个 tradeoff 你认同吗？

    了解了原理后稍微认同吧，基本上要做起来，还是得这么做。

- [x] DX 打分（1-5）：上手摩擦、文档质量、报错信息、SDK 设计

    DX 3.5

- [x] 一句话总结：推荐还是劝退？

    看场景

---