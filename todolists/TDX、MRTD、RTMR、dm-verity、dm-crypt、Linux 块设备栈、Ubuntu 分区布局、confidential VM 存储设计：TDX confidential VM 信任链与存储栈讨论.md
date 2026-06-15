## TDX、MRTD、RTMR、dm-verity、dm-crypt、Linux 块设备栈、Ubuntu 分区布局、confidential VM 存储设计：TDX confidential VM 信任链与存储栈讨论

> 涉及主题：Intel TDX attestation、Linux device mapper、Ubuntu 启动与磁盘布局、confidential VM 镜像设计

## 一、TDX measurement 链

### MRTD vs RTMR 角色

- MRTD：TD 创建时一次性 measure 的 firmware（OVMF/TDVF）镜像。一旦确定就冻结，只能证明 firmware 层。
- 4 个 RTMR：runtime measurement register，类似 TPM PCR，只能 extend 不能覆写。`RTMR_new = SHA384(RTMR_old || digest)`。RTMR 是 measurement 之一。

### RTMR 职责分工（dstack 实现）

- RTMR0：OVMF 启动后写入，记录虚拟硬件配置（vCPU 数、内存大小、设备清单、ACPI 表）。同样 CPU+RAM 配置可预先算出。
- RTMR1：OVMF 写入，记录即将加载的 Linux kernel image hash。
- RTMR2：kernel（或 EFI stub）写入，记录 kernel cmdline 和 initrd。注意 direct boot 模式下需要 EFI stub 来 measure cmdline 和 initrd，否则 RTMR2 不会跟着 cmdline 变 —— canonical/tdx issue #40 就是这个坑。
- RTMR3：留给应用层，TDX 规范不规定内容。dstack 用来装 compose hash、instance id、app id、key provider 等。Confidential Containers 一般用 measurement index 17 映射到 RTMR3。

### verifier 验证流程

1. 验签：确认 quote 是真 Intel TDX 签的（DCAP/QvE，根证书追溯到 Intel）。
2. 比 MRTD：firmware 版本。
3. 比 RTMR0/1/2：对应固定镜像，预先算出预期值并比对。
4. 比 RTMR3：通过 event log replay —— event log 是 RTMR3 extend 操作的明细，verifier 按明细从初始值重新算，跟 quote 里的 RTMR3 对比。

### dm-verity 怎么进信任链

rootfs 不直接 measure（太大）。dm-verity 把 rootfs 做成 Merkle tree，root hash 写进 kernel cmdline（典型形式 `roothash=abc123...`）。cmdline 进 RTMR2，所以 root hash 间接被 attest，rootfs 任何 block 被改都过不了 verity 校验。

整条链：Intel root key → TDX module measure OVMF 得 MRTD → OVMF measure 硬件配置进 RTMR0、measure kernel 进 RTMR1 → kernel/EFI stub measure cmdline 和 initrd 进 RTMR2 → cmdline 里的 root hash 通过 dm-verity 锁死 rootfs → initrd 起 runtime，runtime 验证 compose 和 image digest 后 extend 进 RTMR3。

## 二、dm-verity 工作机制

### 静态 vs 动态的区分

- 数据内容静态：rootfs 只读，hash tree 只读，root hash 写死在 cmdline 里，整个生命周期不变。
- 校验计算动态：每次读 block 临场算 hash，按需校验。不预先全盘 hash。

### 一次 read 的具体流程

1. 应用读 `/etc/passwd` → VFS 路由到 `/` 的 ext4 → ext4 翻译成"读 `/dev/mapper/root` 偏移 X 的 Y 字节" → 生成 BIO 提交到通用块层 → BIO 路由到 `/dev/mapper/root` 这个块设备 → verity target 接管。
2. 从 hash tree 分区读出对应路径的兄弟节点 hash 和父节点 hash。
3. 从 rootfs 分区读出实际 data block，临场算 SHA256 得 H_data。
4. 跟兄弟 hash 拼起来算父 hash 实测值，跟磁盘上声称值比对。任何一步不对 → 返回 EIO（errno=5）。
5. 一路验到 root，跟 cmdline 里的 root hash 比对。
6. 通过则把数据填回 BIO，沿原路返回。

关键点：磁盘上的 hash tree 是"待验证对象"，不是信任源。唯一信任锚点是 cmdline 里的 root hash（攻击者改不了，因为进了 RTMR2）。所有父 hash 都临场算，不直接信任磁盘存的值。

### Cache 行为

- 中间节点 hash：验证过的会缓存，下次读到经过同一中间节点的 block 时，验到该节点为止即可，不用一路推到 root。
- 叶子层 data hash：永远不缓存，每次读 data block 都临场算。

性能开销稳态下大约百分之几。SHA256 + AES-NI 硬件加速下基本不显眼。

### 写入行为

dm-verity 只支持读。所有写 BIO 直接返回 EIO 拒绝（device mapper 层拒绝，不是文件系统层的 ro mount）。`dd` 直接写底层物理分区也写不进去 —— 整个写路径被堵死。

### EIO 是什么

不是 Python `IOError`，是内核块设备层的 IO 错误，errno=5。传播路径：verity 检测到 hash 不匹配 → bio 标记错误 → 文件系统拿到错误 → VFS 转成 syscall 返回值 → `read()` 返回 -1，errno=EIO。`dmesg` 会有 `device-mapper: verity: <devname>: data block <N> is corrupted` 日志。

## 三、攻击场景与信任模型

### SSH 进 VM 后能做什么

前提：production confidential VM 不应开 SSH。假设开发期开了。

- 读 rootfs 任意文件：可以，verity 只防篡改不防读。
- 改 rootfs 文件：不行，device mapper 拒绝写。`vim /etc/passwd` 报 "Read-only file system"。`mount -o remount,rw /` 也失败。`dd` 直写底盘也失败。
- 改 `/var`、`/tmp` 这些可写目录：可以，但这些不被 measure，不影响 attestation。
- 改 RTMR 值：不行，只能 extend 不能覆写，且 RTMR0/1/2 在 boot 阶段后就不动了，运行时只有 RTMR3 能 extend。
- 改 cmdline：`/proc/cmdline` 是只读伪文件。即使能改，RTMR2 在 kernel 启动早期就锁定了。
- 读 TDX 加密内存：宿主机视角不行，TDX CPU 硬件级隔离。自己进程内存当然能读。

### 改 cmdline 的离线攻击场景

攻击者关 VM，作为云平台 admin 修改 VM 配置里的 cmdline（比如把 root hash 换成跟篡改后 rootfs 匹配的值），重启 VM。这次 RTMR2 跟原来不一样，attestation 失败，KBS 不释放 secret，VM 起来也是空壳。

### confidential computing 信任模型

不是"防止一切修改"，而是：任何可能被修改的东西要么不被信任（可写区域），要么修改会留下密码学证据（measured boot 链）。

防的攻击者范围：宿主机管理员、云厂商 admin、获取 VM root 的人都视为攻击者。

## 四、Linux 块设备栈

### 分区 vs 文件系统 vs device mapper

- 分区：磁盘上的物理切片（GPT 分区表条目）。比如 `/dev/nvme0n1p2`。
- 文件系统：块设备上的数据组织格式（ext4/xfs/btrfs）。决定 inode、目录、权限怎么从原始字节解析出来。
- device mapper：内核框架，把一个或多个底层块设备映射成虚拟块设备，中间做处理。位置在通用块层和 IO 调度器之间。dm 设备本身就是一等公民块设备，文件系统直接挂在上面，不知道也不需要知道底下有 dm。

三者不是一对一关系。可以叠任意多层 dm。一个文件系统可以跨多分区（LVM、Btrfs raid），一个分区也可以装多个文件系统（loop device 挂镜像）。

### confidential VM 实际只需要两个 target

- dm-verity：只读 + 完整性校验，给代码用。
- dm-crypt：透明加密，给数据用。

其他 target（dm-thin、dm-cache、dm-raid、LVM）是宿主机或传统场景的事，confidential VM 镜像里通常不用。

### dm-crypt 不复制数据

虚拟设备 `/dev/mapper/dm_crypt-0` 是底层物理分区的"实时翻译视图"，不占额外存储。磁盘上始终只有一份密文。

按需翻译：应用读 4KB → dm-crypt 从底盘读对应 4KB 密文 → 内核里解密 → 返回明文给应用。明文存 RAM 缓冲区，用完释放。从不落盘。

读和写双向工作：写明文 → dm-crypt 加密 → 落到底盘是密文。整个对应用透明。

`cryptsetup luksOpen` 几秒钟完成，因为只是从 LUKS 头读 master key、在内核注册 dm 设备，不预先解密任何数据。

### dm-verity 同样不复制

`/dev/mapper/root` 跟 `/dev/vda3`（rootfs）大小相同，但只占 `/dev/vda3` + `/dev/vda4`（hash tree）的物理空间，不是两倍。

### 设备命名规则

- `/dev/sda`、`sdb`：SCSI/SATA。物理服务器、老台式机。
- `/dev/nvme0n1`：NVMe SSD。`nvme<controller>n<namespace>`。
- `/dev/vda`、`vdb`：VirtIO 块设备。KVM/QEMU 虚拟机（包括 TDX）。
- `/dev/xvda`：Xen 虚拟块设备。AWS EC2 老一代。
- `/dev/mapper/*`：device mapper 创建的虚拟设备。

AWS Nitro 用 NVMe 接口、GCE 用 SCSI、阿里云 ECS 和 TDX 用 VirtIO。

## 五、Ubuntu 启动顺序与 /boot 独立分区

### 鸡生蛋问题

dm-crypt 是 Linux kernel 的模块，只有 kernel 跑起来后才能解密 LUKS 分区。但 kernel 自己必须先被加载。

启动链：
1. UEFI firmware 读 EFI 分区（必须 FAT32 不加密，UEFI 规范强制）→ 跑 GRUB。
2. GRUB 读 `/boot` 分区找 kernel + initrd。GRUB 有 LUKS 模块但性能差且不支持 LUKS2 新特性（Argon2），Ubuntu 默认不让 GRUB 解密。
3. GRUB 加载 kernel + initrd 进内存，跳进 kernel。
4. kernel + initrd 启动，cryptsetup 解锁 LUKS，创建 `/dev/mapper/dm_crypt-0`。
5. 挂 root 文件系统继续启动。

所以 `/boot` 必须独立分区且不加密，否则 GRUB 读不到 kernel。

不加密 Ubuntu 把 kernel 放在 root 分区的 `/boot` 目录里就行；加密 Ubuntu 必须把 `/boot` 拆成独立分区。

### Secure Boot 和 Measured Boot

`/boot` 不加密带来 kernel 被替换的风险（恶意 kernel 偷 passphrase）。两种防御：

- Secure Boot：UEFI 验证 bootloader 签名，bootloader 验证 kernel 签名。
- Measured Boot（TPM 或 TDX RTMR）：每一步加载内容都被 measure。LUKS2 可绑定 TPM —— 只有 measurement 符合预期时 TPM 才释放 master key。

confidential VM 用后者：kernel 可以放明面上，但被 measure 进 RTMR1/RTMR2，改了 attestation 失败，KBS 不放 dm-crypt key。

## 六、Ubuntu 24.04 默认布局

### 不加密安装

```
nvme0n1                259:0    0    1T  0 disk 
├─nvme0n1p1            259:1    0    1G  0 part /boot/efi
└─nvme0n1p2            259:2    0  999G  0 part /
```

`/` 直接挂在 `nvme0n1p2`，没有 dm 设备。`/boot` 是 root 分区下的普通目录。

### 加密 + LVM 安装（默认勾选 LVM and encryption）

```
nvme0n1                                  259:0    0    1T  0 disk  
├─nvme0n1p1                              259:1    0    1G  0 part  /boot/efi
├─nvme0n1p2                              259:2    0    1G  0 part  /boot
└─nvme0n1p3                              259:3    0  998G  0 part  
  └─dm_crypt-0                           252:0    0  998G  0 crypt 
    └─ubuntu--vg-ubuntu--lv              252:1    0  998G  0 lvm   /
```

三层抽象：物理分区 → dm-crypt 解密视图 → LVM 切片 → ext4 → 挂 `/`。LVM 这层在默认配置下其实多余（一个 LV 占满 PV），加上是为了未来灵活性。设备名里的双横杠 `--` 是 LVM 转义规则。

### confidential VM 改造版

```
vda                       254:0    0  100G  0 disk  
├─vda1                    254:1    0    1G  0 part  /boot/efi
├─vda2                    254:2    0    1G  0 part  /boot
├─vda3                    254:3    0   20G  0 part  
│ └─root                  252:0    0   20G  0 verity /
├─vda4                    254:4    0  200M  0 part  
│ └─root                  252:0    0   20G  0 verity /
└─vda5                    254:5    0   77G  0 part  
  └─data                  252:1    0   77G  0 crypt  /var/lib/app-data
```

vda3 + vda4 合并成同一个 verity 虚拟设备（verity 需要数据 + hash tree 两个底层来源）。

## 七、目录可信级别对照

同一棵目录树里每个挂载点安全属性完全不同：

- `/`：来自 `/dev/mapper/root`，dm-verity 保护，只读 + 完整性校验。代码、`/etc` 默认配置、`/usr/bin`、`/usr/lib` 全在这里。据说 /var 默认是在 dm-verity 下，需要配置到 dm-crypt 下。（不过 TDX 默认 Ubuntu 实例不是加密的）
- `/boot/efi`、`/boot`：普通分区，但内容被 RTMR1/RTMR2 测量。运行时不写。
- `/var/lib/app-data`：来自 `/dev/mapper/data`，dm-crypt 加密。可读写。不被 attestation 测量，但 master key 由 attestation 释放。
- `/tmp`、`/run`、`/dev/shm`：tmpfs（纯内存），重启即清。
- `/proc`、`/sys`、`/dev`：虚拟文件系统，内核临场生成，不在磁盘。

## 八、Ubuntu FHS 目录速查

### root 分区上的目录

- `/usr`：系统程序和库（`bin/`、`sbin/`、`lib/`、`share/`、`local/`），通常 5-10 GB。所有 apt 装的软件在这。运行时只读语义。
- `/etc`：配置文件。约 50 MB。
- `/var`：可变数据。子目录：
  - `/var/log/`：日志
  - `/var/lib/`：应用持久化数据（postgresql、docker、apt 元数据）
  - `/var/cache/`：可重建缓存
  - `/var/tmp/`：临时文件但重启保留
  - `/var/spool/`：任务队列
- `/home`：用户家目录
- `/root`：root 用户家目录（不在 /home 下）
- `/opt`：第三方应用（chrome、spotify）
- `/srv`：服务对外数据（FHS 设计意图，实际很多人用 /var/www）

### 符号链接到 /usr 的（usrmerge）

`/bin → /usr/bin`、`/sbin → /usr/sbin`、`/lib → /usr/lib`、`/lib64 → /usr/lib64`、`/lib32 → /usr/lib32`。Ubuntu 24.04 上这些都是符号链接，物理上在 `/usr` 下。

### 独立分区或挂载点

- `/boot`：bootloader + kernel + initrd
- `/boot/efi`：EFI 分区（FAT32）

### tmpfs（内存）

- `/tmp`：临时文件，Ubuntu 24.04 默认挂 tmpfs，重启清空
- `/run`：运行时状态（PID、socket、lock），systemd 大量用
- `/dev/shm`：进程间共享内存

### 虚拟文件系统（内核生成）

- `/proc`：进程信息和 kernel 状态（`/proc/cmdline`、`/proc/cpuinfo`、`/proc/<pid>/`）
- `/sys`：内核对象（设备、驱动、子系统状态）
- `/dev`：设备文件（devtmpfs，由内核动态生成）

### 通常空挂载点

- `/mnt`：临时手动挂载
- `/media`：自动挂载点（U 盘等）
- `/lost+found`：fsck 恢复的孤儿数据

### Ubuntu 特有

- `/snap`：snap 包
- `/var/snap`：snap 应用数据
- `/sys/fs/cgroup`：cgroup v2 统一挂载点

## 九、对 Camille LLM gateway 落地的具体建议

1. 阿里云 TDX 只保底 MRTD + RTMR0，RTMR1/2/3 怎么用是自己的事。务必启用 EFI stub 的 measure 支持（efi/libstub patch），用 `pytdxmeasure` 验证改 cmdline 后 RTMR2 跟着变。

2. RTMR3 schema 自己定。简单版：compose.yml + image digest + 配置拼成 canonical JSON，SHA384 后 extend 一次。复杂版：每个 image digest 和配置单独 extend。可参考 dstack 设计。

3. 提供类似 `dstack-mr` 的工具让用户从镜像 metadata 算出预期 MRTD/RTMR0/1/2，否则 reproducible verification 站不住。

4. BYOK 通过 KBS。KBS 拿到 quote 验证后释放 user key 给 TD。KBS 需要 attestation policy（认 RTMR3 是哪些值才放 key）。

5. 镜像内部存储分层：
   - 应用代码（Go binary）→ rootfs（dm-verity 只读）
   - 模型权重 → 选 1：rootfs（镜像大但简单）；选 2：dm-crypt 加密 volume（更新方便，但要把模型 hash 加进 attestation policy）
   - TLS 私钥、用户 API key → 启动后从 KBS 拉，放 RAM/tmpfs，永不落盘
   - 请求日志、计费数据 → dm-crypt 加密 volume
   - KV cache → tmpfs 或进程内存

6. Production 镜像绝对不开 SSH。调试通道通过 attestation 加密的 RPC 端点提供，endpoint key 由 attestation 释放。

