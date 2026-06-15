# initrd 从 verity 到 switch-root

先看一条 grub 配置。这是在一台实例 MockInstance 上跑的启动项，写入了启动 verity 与 verity 校验的几个值：

```
menuentry 'MockInstance verity' {
    search --no-floppy --fs-uuid --set=root d4ed479e-...
    linux /boot/vmlinuz-6.8.0-117-generic \
        systemd.verity=1 \
        roothash=6761c548...859b99 \
        systemd.verity_root_data=/dev/disk/by-id/nvme-...4e \
        systemd.verity_root_hash=/dev/disk/by-id/nvme-...4f \
        systemd.verity_root_options=panic-on-corruption \
        console=tty0 console=ttyS0,115200n8 ...
    initrd /boot/MockInstance.initrd
}
```

grub 目前看来做了三件事：加载 `vmlinuz`，加载 `MockInstance.initrd`，把 `linux` 后面那一长串字符串作为 cmdline 传给内核。

注意 cmdline 里那三个参数：`roothash`、`verity_root_data`、`verity_root_hash`。一个哈希值，两块盘。最后系统跑起来时，根文件系统是一个只读的、每读一个 block 都会被内核校验哈希的设备。从"grub 传了一行字符串"到"内核在逐块校验 rootfs"，中间发生的事全在 initrd 里。

## vmlinuz 和 initrd 是两个东西

`linux` 和 `initrd` 是两行，加载的是两个独立文件。

`vmlinuz` 是压缩过的内核镜像，grub 把它解压、跳进去执行。`MockInstance.initrd` 是另一个文件，内核启动早期会把它解压到内存里，当成临时根文件系统。

initrd 本身就是一个 cpio 归档，mkosi 用 zstd 压缩它，所以看到的是 `.cpio.zst`。内核解压出来就是一棵完整的目录树——有 `/usr/bin`、`/usr/lib`、systemd 的各种单元文件，是一套能跑起来的精简根系统。

为什么需要这么个临时根？因为真正的 rootfs 现在还挂不上。它预备存在一个 dm-verity 设备之上，而这个设备需要去建。建它需要 `veritysetup` 这个用户态工具，工具又得有个文件系统装着它才能运行。initrd 作为一个完全在内存里、不依赖任何磁盘的根，专门用来把磁盘上的 rootfs 准备好。

## 去 initrd 里看看有什么

把 initrd 解出来看一眼：

```
$ mkdir /tmp/ird && cd /tmp/ird
$ zstd -d < /boot/MockInstance.initrd | cpio -idm
$ ls
bin  dev  etc  proc  run  sys  sysroot  usr  var
```

跟一个正常 Linux 根目录长得差不多。但里面的东西少很多。挑几个跟 verity 有关的看：

```
$ ls usr/bin/ | grep -E 'veritysetup|systemctl'
systemctl
veritysetup

$ ls usr/lib/systemd/system-generators/
systemd-fstab-generator
...

$ ls usr/lib/systemd/system/ | grep verity
systemd-veritysetup@.service
```

几个关键角色都在：

- `systemd` 本体，在 initrd 里它就是 PID 1。
- `veritysetup`，建立 dm-verity 映射的用户态工具。
- `systemd-fstab-generator`，一个 generator。
- `systemd-veritysetup@.service`，一个带 `@` 的模板单元。

注意这里有个空缺：你在 initrd 里找不到一个写死的 `systemd-veritysetup@root.service`，只有那个模板 `@.service`。那个 `root` 实例是启动时凭空冒出来的。这个"凭空冒出来"是后面整件事的关键，先记着。

至于这些文件怎么进的 initrd——是 mkosi 的配置决定的，它知道你要做 verity 启动，就把 `veritysetup` 和对应的 systemd 单元一起塞了进来。这篇不展开打包细节，只看运行时它们怎么协作。

## cmdline 怎么传到 initrd 里

grub 把那一行 cmdline 交给内核。内核启动后把它原样放进 `/proc/cmdline`。任何能读这个文件的进程都能拿到完整的启动参数。

```
$ cat /proc/cmdline
... systemd.verity=1 roothash=6761c548... systemd.verity_root_data=/dev/... ...
```

这里要分清楚谁消费哪些参数。`console=`、`iommu=pt` 这种是内核自己看的。而 `systemd.verity*` 这一组，内核根本不认识，它只是照单全收放进 `/proc/cmdline`。真正读这组参数的是 initrd 里的 systemd。这是 systemd 自己约定的一套命名，前缀 `systemd.` 就是给它自己用的。

所以链路是：grub → kernel → `/proc/cmdline` → initrd 的 systemd 读它。内核在中间只是个传话的。

## initrd 里的四步

systemd 在 initrd 里当 PID 1，它要做的核心就是把 `/sysroot` 准备好。围绕 verity，分四步。

**第一步：generator 把 cmdline 翻译成一个 service。**

systemd 启动极早期会跑所有 generator。`systemd-fstab-generator` 读 `/proc/cmdline`，看到 `systemd.verity=1` 和那三个参数，于是动态生成一个单元文件——以那个 `@.service` 模板为基础，实例化出 `systemd-veritysetup@root.service`。

前面说的"凭空冒出来"就是这一步。没人手写这个 service 文件，它是 generator 在内存里根据 cmdline 现造的，造完塞进本次启动的依赖图。这是 systemd 体系里很多人不知道的机制：一堆服务不是配出来的，是启动时根据环境算出来的。

**第二步：service 调 veritysetup，建出映射设备。**

`systemd-veritysetup@root.service` 跑起来，本质是执行一条 `veritysetup open`，参数就是 cmdline 里给的三样东西：

```
veritysetup open \
    /dev/disk/by-id/nvme-...4e \   # data 盘
    root \                          # 映射名
    /dev/disk/by-id/nvme-...4f \    # hash 盘
    6761c548...859b99               # roothash
```

执行完，`/dev/mapper/root` 出现了。data 盘装的是 rootfs 的实际数据（erofs 格式），hash 盘装的是一棵覆盖 data 盘所有 block 的哈希树（Merkle tree），roothash 是这棵树的根。

**第三步：内核 dm-verity 接管，开始逐块校验。**

`/dev/mapper/root` 不是一块普通的块设备。它背后是内核的 dm-verity target。从它建立的那一刻起，每次有人读它的某个 block，内核都会顺着 Merkle tree 算一遍哈希，跟 hash 盘里存的对，再一路验到 roothash。对不上就说明数据被改过。

cmdline 里那个 `panic-on-corruption` 决定校验失败时怎么办：直接 panic，不给降级运行的机会。

这一步划清了 veritysetup 和 dm-verity 的分工。veritysetup 是用户态的，负责 setup——把盘、哈希、roothash 拼起来，告诉内核"建一个这样的设备"。建完它就没事了。dm-verity 是内核态的，负责 enforce——之后每一次读取的校验都是它在做，全程在内核里，绕不过去。

**第四步：把这个设备挂成 /sysroot。**

`/dev/mapper/root` 上的 erofs 是只读文件系统，systemd 把它挂到 initrd 里那个空的 `/sysroot` 目录。到这里，磁盘上那个受 dm-verity 保护的 rootfs，已经挂在临时根的一个子目录下了。准备工作做完了。

## 交接：switch_root

`/sysroot` 挂好之后，initrd 的活就干完了，剩下的是把控制权交出去。

systemd 执行 `switch_root`：把根从 initrd 的内存文件系统切换到 `/sysroot`，然后 exec `/sysroot` 里那个真正 rootfs 自带的 systemd。新的 systemd 接管 PID 1，老的被替换掉，initrd 占的那块内存被释放。

从这一刻起，系统跑在那个只读、逐块校验的 rootfs 上。后面所有对根文件系统的读取，都要经过 dm-verity 那一关。

## 这套东西对 TDX 有什么用

回到一开始那条 cmdline。roothash 写在里面，意味着它是 grub 传给内核的启动参数的一部分。在 TDX 这种机密计算环境里，cmdline 会被度量进 RTMR，成为远程证明信任链的一环。

这就把两件事接上了：roothash 进了度量，说明启动时声明的 rootfs 哈希是什么是可证明的；dm-verity 保证运行时这个 rootfs 一个 block 都改不了。一个管"启动时是不是这个 rootfs"，一个管"运行中有没有被人动过"。对于要向远端证明"我这台机器跑的就是这套未经篡改代码"的场景，这两个正好凑齐。

整条链路从一行 cmdline 开始，到 dm-verity 在内核里逐块校验结束。中间那个临时的、纯内存的 initrd，存在的全部理由就是把这件事搭起来，搭完就消失。