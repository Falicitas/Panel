# TDX dm-verity Attestation 实验 Checklist（Ubuntu 24.04 版）

一句话：在阿里云 Ubuntu 24.04 TDX 实例上，手动把 Go 服务锁进 dm-verity rootfs，让 root hash 进 measurement（优先 boot-time 的 RTMR2，进不去就退到运行时 extend 进 RTMR3），最终用一台独立机器只信 Intel 芯片就能离线验证“跑的是公开那份 binary”。破坏性操作随便做，实例用完即删。

信任假设：只信 Intel TDX 芯片签名，不信阿里云、不信宿主机、不信 VM 内 root。但要清楚两条边界：

-   验证不能只看 RTMR2。dm-verity 的安全建立在 kernel 会老实执行 verity 校验之上，而 kernel 的可信来自 RTMR1。所以离线验证必须同时比对 MRTD（TDVF）、RTMR0、RTMR1、RTMR2。只验 RTMR2 这条链是断的——换一个恶意 kernel（RTMR1 变了你不看），它可以无视 dm-verity 直接跑篡改 binary，再往 RTMR2 里 extend 一个等于公开值的假 hash，因为 RTMR2 里 extend 什么内容是软件控制的。
-   公有云上有个拆不掉的上限。MRTD 覆盖的 TDVF 是阿里云 build 的，你拿不到能独立复现的 TDVF 去比对。所以纯粹“只信芯片、什么云都不信”在阿里云上达不到，实际能做到的是“信 Intel 芯片 + 信阿里云用的 firmware 是它声称的那个版本”。这条要在最终对外口径里诚实写出来。

实验要回答的变量：① 阿里云 Ubuntu 24.04 上 cmdline 到底进不进 RTMR2（决定走 boot-time 还是 runtime extend，不决定实验成败）② 改 backend → root hash 变 → measurement 变，链是否闭合 ③ 独立机器能否离线复现 root hash、复现全套预期 measurement，并用 DCAP QVL 验 quote。

工具链说明（跟阿里云老文档不同，2026 最新）：

-   产 quote：直接用内核 TSM configfs `/sys/kernel/config/tsm/report/`，不装阿里云 yum 那套，也不装 Intel SDK
-   看 RTMR / event log：用 canonical 的 `tdeventlog`
-   验 quote：用 Intel DCAP QVL 或 Go 的 `github.com/google/go-tdx-guest` 离线验签，不走 Intel Tiber、也不走阿里云远程证明服务那种在线评估（那些要么联网要么把验证信任交给第三方，违背“只信芯片”）
-   rootfs 复现：必须在一个锁死版本的 Linux 环境里做（Docker 镜像，pin 住 squashfs-tools / cryptsetup 版本），macOS 原生跑不了 mksquashfs / veritysetup，也复现不出 bit-identical 的镜像

------

## Phase 0 — 开实例 + 确认 TDX 和 TSM configfs (~15min)

目的：拿到 Ubuntu 24.04 TDX 实例，确认 quote 能从内核原生接口产出。

-   [x] 控制台开实例 — 规格 `ecs.c9i.xlarge`，可用区北京 I，镜像勾“机密虚拟机” + Ubuntu 24.04，系统盘 cloud_essd 40G+

-   [x] SSH 上去确认 TDX — `dmesg | grep -i tdx` 看到 "Guest detected" 和 "Memory Encryption Features active: Intel TDX"

    >   🌵：`lscpu | grep -i tdx_guest` 看到 flag 不足以证明 TDX 真启用，因为 lscpu 自己可能不诚实。这条指令只在“lscpu 诚实”的前提下才说明问题，所以要拿 dmesg 交叉验证。lscpu 看的是 CPU 型号、核数、缓存、虚拟化、以及 feature flags（x86 架构支持哪些扩展指令集）。
    >
    >   `tdx_guest` 只是内核给“我在 TDX guest 里”这个状态起的 flag 名。TDX 底层真正的 CPU 指令扩展叫 `TDX`（靠 SEAM / tdcall 那套实现），`tdx_guest` 是内核检测到身份后对外暴露的标志，不是指令集本身的名字。这个 flag 是 kernel 标出来的，所以 kernel 本身也得可信。
    >
    >   ```bash
    >   # uname -r：当前运行的内核版本，如 6.8.0-111-generic
    >   # 6.8.0=上游版本，-111=Ubuntu补丁号，-generic=内核flavor
    >   # 决定 apt source 拿哪个版本的源码
    >   uname -r
    >   6.8.0-111-generic
    >   
    >   # lsb_release -a：发行版信息
    >   # 决定用哪套打包工具链（22.04 和 24.04 不一样）
    >   lsb_release -a
    >   Distributor ID: Ubuntu
    >   Description:    Ubuntu 24.04.4 LTS
    >   Release:        24.04
    >   Codename:       noble
    >   ```
    >
    >   ```bash
    >   ls -l /dev/tdx_guest
    >   crw------- 1 root root 10, 261 May 28 19:59 /dev/tdx_guest  # 字符设备，major 10 是内核 misc 字符设备类
    >   ```
    >
    >   `dmesg` 一样，VM 内软件产生的文本和节点,root 能任意伪造,没有签名。
    >
    >   ```bash
    >   dmesg | grep -i tdx
    >   [    0.000000] tdx: Guest detected
    >   [    0.531825] process: using TDX aware idle routine
    >   [    0.531825] Memory Encryption Features active: Intel TDX # *
    >   [    2.452840] systemd[1]: Detected confidential virtualization tdx.
    >   [38864.527634] systemd[1]: Detected confidential virtualization tdx.
    >   ```
    >
    >   >   systemd 是 init 系统(PID 1),内核启动完成后由内核拉起,职责是初始化用户空间——挂载文件系统、启动各种服务和守护进程、管理系统直到关机。

-   [x] 确认 TSM（Trusted Security Module，抽象层，硬件是 intel 会用 intel 的 tdx 能力） configfs 是 quote provider — 跑：

    ```bash
    sudo mkdir -p /sys/kernel/config/tsm/report/test0
    cat /sys/kernel/config/tsm/report/test0/provider
    ```

    应输出 `tdx_guest`。这是 backend 产 quote 的接口。

    ```bash
    # inblob 能写,因为它注册了 write 回调
    head -c 64 /dev/urandom | sudo tee /sys/kernel/config/tsm/report/exp1/inblob > /dev/null
    ```

    -   有一个心智模型。configfs 所生成的存在于内存中的属性文件，实际里面都是回调函数，通过文件读写触发。

        ```c
        struct configfs_attribute {
            const char *ca_name;       // 属性名,比如 "provider"
            umode_t     ca_mode;       // 权限位,比如 0444(只读)
            ssize_t (*show)(...);      // read 回调:cat 时调它。函数指针
            ssize_t (*store)(...);     // write 回调:echo something | tee 文件 时调它。函数指针
        };
        ```

        找到 [6.8 kernel tsm.c 代码](https://github.com/torvalds/linux/blob/v6.8/drivers/virt/coco/tsm.c)（generation 是从 0 自增计数器，也充当缓存失效标记）：

        ```bash
        # 文件名 → 回调函数 → 文件系统宏 → 权限:
        generation  →  tsm_report_generation_show     →  CONFIGFS_ATTR_RO      →  0444 (r--) 
        provider    →  tsm_report_provider_show        →  CONFIGFS_ATTR_RO      →  0444 (r--)
        inblob      →  tsm_report_inblob_write         →  CONFIGFS_BIN_ATTR_WO  →  0200 (-w-)
        outblob     →  tsm_report_outblob_read         →  CONFIGFS_BIN_ATTR_RO  →  0444 (r--)
        ```

        -   `_show` 结尾 = 读回调(cat 时调),`_write`/`_read` 结尾 = 二进制属性的读写回调。普通文本属性用 `_show`,二进制属性(inblob/outblob)用 `_read`/`_write`。
        -   宏里 `RO`=只读、`WO`=只写,直接决定权限位。`BIN` 前缀=二进制属性。
        -   `provider_show` 里返回值的那行:`sysfs_emit(buf, "%s\n", provider.ops->name)` —— 你 cat 出 `tdx_guest` 就是这行。
        -   `outblob_read` 调 `tsm_report_read`,里面 `ops->report_new(...)` 才是真正触发产 quote 的点。读 outblob = 触发硬件 attestation。

        mkdir 触发的回调：

        ```
        mkdir  →  tsm_report_make_item  (经 .make_item 注册到 group_ops)
        ```

        回调函数由宏绑定到 tsm_report_attr_provider 类上：

        ```c
        #define CONFIGFS_ATTR_RO(_pfx, _name)			\
        static struct configfs_attribute _pfx##attr_##_name = {	\
        	.ca_name	= __stringify(_name),		\ // 属性文件名
        	.ca_mode	= 0444,				\
        	.ca_owner	= THIS_MODULE,			\
        	.show		= _pfx##_name##_show,		\
        }
        ```

        最后注册链：

        ```bash
        注册链:
          tsm_report_attrs[] 数组收集 attr 地址
          → tsm_report_default_type.ct_attrs 持有数组
          → make_item 里 config_item_init_type_name 绑定 type
          → configfs 遍历 ct_attrs,逐个建文件 + 接 .show/.store
        ```

        对于 configfs 来说，其在 `/sys/kernel/config/tsm/report` 的管理机制就是上面所描述的。

        最后 outblob 结构：

        ```bash
        Quote
        ├── Header
        ├── TD Report Body(MRTD/RTMR0-3/report_data/TD attributes/TEE_TCB_SVN...)
        └── Signature Data(签名区,内部又分层)
            ├── Quote Signature ← 用 Attestation Key(AK)对 Header+Body 的 ECDSA 签名
            ├── Attestation Key(AK 公钥)
            ├── QE Report ← Quoting Enclave 自己的 report,里面绑定了 AK 的 hash
            ├── QE Report Signature ← 用 PCK 私钥对 QE Report 签名
            └── Certification Data
                └── PCK Cert → Platform CA → Root CA(三张证书)
                
                
        cat /sys/kernel/config/tsm/report/exp1/outblob
        ���r3�L��
        ���%����
                 ���k�Gj)��+�ǃp�=
        ��V�r'+�lMc�Ծ(e�&��u�-�B[�Y�|�B��������so%��gMB����T$+�UJ�^�;��b�>�0嚲V�w�����R7�<�c�b&�_��Ù���Ul�@U�st���׋6"�d�S��
                                                                                                                            �'B�ZS��;�>��w�qw���M�(����[�f�aE����k�
                                     ��)s���/���v�'��!���:�l�������tiڜ~��ItN�q��SL�8�ݲ�N�疗�?�Z�M��C@�k}�i��k�Pܚ�S.M:�Gf�뱔��&9*�\^56��f�z(۠�c����+I�D����xmz�L�28��#':Sϼ�,J
        �Ia?\`)��+�X�U桯�D .V�I-r�}M�j�,�F   Q-[�މ�P�Ζ�q��壧��0;�SLlY��O�4�3������)7���j��z�)it�Ռ��
        ��k�ܞ*|o��GN4��C�|c��c@�.T���Y�mn�y����ɫ>�x�pR�T����=�@G��>�	�
                                                                         H0w:�FADIѼhE>Ά�c/;Y
        '�Y\�R�im~x��l�P��YwA����
        
        
        ^-----BEGIN CERTIFICATE-----
        MIIE8TCCBJegAwIBAgIUDUvPQI9PlnFN7vLlgM6liVTZpAswCgYIKoZIzj0EAwIw
        cDEiMCAGA1UEAwwZSW50ZWwgU0dYIFBDSyBQbGF0Zm9ybSBDQTEaMBgGA1UECgwR
        SW50ZWwgQ29ycG9yYXRpb24xFDASBgNVBAcMC1NhbnRhIENsYXJhMQswCQYDVQQI
        DAJDQTELMAkGA1UEBhMCVVMwHhcNMjYwNTA5MTYwNDQxWhcNMzMwNTA5MTYwNDQx
        WjBwMSIwIAYDVQQDDBlJbnRlbCBTR1ggUENLIENlcnRpZmljYXRlMRowGAYDVQQK
        DBFJbnRlbCBDb3Jwb3JhdGlvbjEUMBIGA1UEBwwLU2FudGEgQ2xhcmExCzAJBgNV
        BAgMAkNBMQswCQYDVQQGEwJVUzBZMBMGByqGSM49AgEGCCqGSM49AwEHA0IABKvW
        cvvnN5pgySEauIycrX/kDtWnJIkf4IERUPlGjx/CYzTbokicF+qWYsdieOLqvWtQ
        aE+000U6SxLDG7SXjP+jggMNMIIDCTAfBgNVHSMEGDAWgBSVb13NvRvh6UBJydT0
        M84BVwveVDBrBgNVHR8EZDBiMGCgXqBchlpodHRwczovL2FwaS50cnVzdGVkc2Vy
        dmljZXMuaW50ZWwuY29tL3NneC9jZXJ0aWZpY2F0aW9uL3YzL3Bja2NybD9jYT1w
        bGF0Zm9ybSZlbmNvZGluZz1kZXIwHQYDVR0OBBYEFNHm1OR07WgPkgeqcmsLhBNM
        fs5NMA4GA1UdDwEB/wQEAwIGwDAMBgNVHRMBAf8EAjAAMIICOgYJKoZIhvhNAQ0B
        BIICKzCCAicwHgYKKoZIhvhNAQ0BAQQQnqFrry8KG1c+6+8UbXTVVzCCAWQGCiqG
        SIb4TQENAQIwggFUMBAGCyqGSIb4TQENAQIBAgECMBAGCyqGSIb4TQENAQICAgEC
        MBAGCyqGSIb4TQENAQIDAgECMBAGCyqGSIb4TQENAQIEAgECMBAGCyqGSIb4TQEN
        AQIFAgEEMBEGCyqGSIb4TQENAQIGAgIA/zAQBgsqhkiG+E0BDQECBwIBADAQBgsq
        hkiG+E0BDQECCAIBAjAQBgsqhkiG+E0BDQECCQIBADAQBgsqhkiG+E0BDQECCgIB
        ADAQBgsqhkiG+E0BDQECCwIBADAQBgsqhkiG+E0BDQECDAIBADAQBgsqhkiG+E0B
        DQECDQIBADAQBgsqhkiG+E0BDQECDgIBADAQBgsqhkiG+E0BDQECDwIBADAQBgsq
        hkiG+E0BDQECEAIBADAQBgsqhkiG+E0BDQECEQIBDTAfBgsqhkiG+E0BDQECEgQQ
        AgICAgT/AAIAAAAAAAAAADAQBgoqhkiG+E0BDQEDBAIAADAUBgoqhkiG+E0BDQEE
        BAYgoG0IAAAwDwYKKoZIhvhNAQ0BBQoBATAeBgoqhkiG+E0BDQEGBBBPEUicS4dR
        oQb4i0+HvWQiMEQGCiqGSIb4TQENAQcwNjAQBgsqhkiG+E0BDQEHAQEB/zAQBgsq
        hkiG+E0BDQEHAgEB/zAQBgsqhkiG+E0BDQEHAwEB/zAKBggqhkjOPQQDAgNIADBF
        AiAg/P6rjWyBdDeoWanxCBHNgGKUnyU28sPfYezWF3au2wIhAMMRUsaKWwD0iCyS
        JIFm3gCdwDMO90/gnH29Ywk8keZe
        -----END CERTIFICATE-----
        -----BEGIN CERTIFICATE-----
        MIICljCCAj2gAwIBAgIVAJVvXc29G+HpQEnJ1PQzzgFXC95UMAoGCCqGSM49BAMC
        MGgxGjAYBgNVBAMMEUludGVsIFNHWCBSb290IENBMRowGAYDVQQKDBFJbnRlbCBD
        b3Jwb3JhdGlvbjEUMBIGA1UEBwwLU2FudGEgQ2xhcmExCzAJBgNVBAgMAkNBMQsw
        CQYDVQQGEwJVUzAeFw0xODA1MjExMDUwMTBaFw0zMzA1MjExMDUwMTBaMHAxIjAg
        BgNVBAMMGUludGVsIFNHWCBQQ0sgUGxhdGZvcm0gQ0ExGjAYBgNVBAoMEUludGVs
        IENvcnBvcmF0aW9uMRQwEgYDVQQHDAtTYW50YSBDbGFyYTELMAkGA1UECAwCQ0Ex
        CzAJBgNVBAYTAlVTMFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAENSB/7t21lXSO
        2Cuzpxw74eJB72EyDGgW5rXCtx2tVTLq6hKk6z+UiRZCnqR7psOvgqFeSxlmTlJl
        eTmi2WYz3qOBuzCBuDAfBgNVHSMEGDAWgBQiZQzWWp00ifODtJVSv1AbOScGrDBS
        BgNVHR8ESzBJMEegRaBDhkFodHRwczovL2NlcnRpZmljYXRlcy50cnVzdGVkc2Vy
        dmljZXMuaW50ZWwuY29tL0ludGVsU0dYUm9vdENBLmRlcjAdBgNVHQ4EFgQUlW9d
        zb0b4elAScnU9DPOAVcL3lQwDgYDVR0PAQH/BAQDAgEGMBIGA1UdEwEB/wQIMAYB
        Af8CAQAwCgYIKoZIzj0EAwIDRwAwRAIgXsVki0w+i6VYGW3UF/22uaXe0YJDj1Ue
        nA+TjD1ai5cCICYb1SAmD5xkfTVpvo4UoyiSYxrDWLmUR4CI9NKyfPN+
        -----END CERTIFICATE-----
        -----BEGIN CERTIFICATE-----
        MIICjzCCAjSgAwIBAgIUImUM1lqdNInzg7SVUr9QGzknBqwwCgYIKoZIzj0EAwIw
        aDEaMBgGA1UEAwwRSW50ZWwgU0dYIFJvb3QgQ0ExGjAYBgNVBAoMEUludGVsIENv
        cnBvcmF0aW9uMRQwEgYDVQQHDAtTYW50YSBDbGFyYTELMAkGA1UECAwCQ0ExCzAJ
        BgNVBAYTAlVTMB4XDTE4MDUyMTEwNDUxMFoXDTQ5MTIzMTIzNTk1OVowaDEaMBgG
        A1UEAwwRSW50ZWwgU0dYIFJvb3QgQ0ExGjAYBgNVBAoMEUludGVsIENvcnBvcmF0
        aW9uMRQwEgYDVQQHDAtTYW50YSBDbGFyYTELMAkGA1UECAwCQ0ExCzAJBgNVBAYT
        AlVTMFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEC6nEwMDIYZOj/iPWsCzaEKi7
        1OiOSLRFhWGjbnBVJfVnkY4u3IjkDYYL0MxO4mqsyYjlBalTVYxFP2sJBK5zlKOB
        uzCBuDAfBgNVHSMEGDAWgBQiZQzWWp00ifODtJVSv1AbOScGrDBSBgNVHR8ESzBJ
        MEegRaBDhkFodHRwczovL2NlcnRpZmljYXRlcy50cnVzdGVkc2VydmljZXMuaW50
        ZWwuY29tL0ludGVsU0dYUm9vdENBLmRlcjAdBgNVHQ4EFgQUImUM1lqdNInzg7SV
        Ur9QGzknBqwwDgYDVR0PAQH/BAQDAgEGMBIGA1UdEwEB/wQIMAYBAf8CAQEwCgYI
        KoZIzj0EAwIDSQAwRgIhAOW/5QkR+S9CiSDcNoowLuPRLsWGf/Yi7GSX94BgwTwg
        AiEA4J0lrHoMs+Xo5o/sX6O9QWxHRAvZUGOdRQ7cvqRXaqI=
        -----END CERTIFICATE-----
        ```

-   [x] 手动产一个基线 quote 验证接口可用 — 写 64 字节 report_data 到 inblob，读 outblob：

    ```bash
    cd /sys/kernel/config/tsm/report/test0
    head -c 64 /dev/urandom | sudo tee inblob > /dev/null # tee 输出到文件 + 保留 stdout，这里重定向 fd1 到虚空
    sudo cat outblob > /tmp/baseline-quote.bin
    ls -l /tmp/baseline-quote.bin   # 应该几 KB
    ```

-   [x] 清理 test report — `sudo rmdir /sys/kernel/config/tsm/report/test0`

------

## Phase 1 — 摸清 RTMR 怎么动（先做，决定走哪条路线）(~30min)

目的：在碰 dm-verity 前，先验阿里云 Ubuntu 24.04 上 cmdline 进不进 RTMR2。结果决定你走 boot-time measure（强）还是 runtime extend（够用），不决定实验做不做得成——runtime extend 那条退路 Ubuntu 一定能用。

>   背景：阿里云两篇文档给的 measurement 示例里，RTMR2 一个是全零、一个有非零值。说明 RTMR2 能不能被 extend 是 boot 链配置决定的，不是被 firmware 层锁死的。对你是利好，但具体到这台 c9i Ubuntu 还是得实测。
>
>   upd:实测结论：LoadOptions 在 RTMR2 事件 19、initrd 事件 20，boot-time 可以说是合格的。

-   [x] 装 canonical 的 guest 工具拿 tdeventlog — clone `https://github.com/canonical/tdx`，看 guest-tools 里的 tdeventlog（Python 脚本）

    -   ```bash
        开机 → OVMF(firmware,初始化 + 提供 UEFI)
              → GRUB(bootloader,加载 kernel/initrd/cmdline)
              → kernel
        ```

        做实验是为了确认 GRUB 是否有将 cmdline 和 initrd 作为 digest 打入 rtmr2。

        ```bash
        boot 期间(每个组件重复):
          组件(TDVF/shik/GRUB/kernel)
            ├─ 算 digest
            ├─ tdcall → 固件 extend 进 RTMR(固件被动,看不见)
            └─ 自己往内存写 event log(软件主动,明文)← tdeventlog 之后读这个
        
        attestation 时(你 cat outblob):
          tsm.c → tdcall(TDG.MR.REPORT)+ 填入 reportdata
            └─ 固件:读 RTMR 当前快照 + MRTD(早已固定)+ 你的 reportdata
                 → 拼成 TDREPORT → 后续签名成 quote
        ```

        ```bash
        # ~/tdx/tests/lib/tdx-tools/src/tdxtools，在这个目录下
        sudo tdeventlog 2>&1 | python3 eventlog_annotate.py # eventlog_summary.py 对于原本的这个 tdeventlog 指令来说，它并没有 event data 这一项。或者说，它是放在右侧的 ASCII 码阵列。所以就稍微增加了一个 py 的脚本。实际上也就是每个 log entry 补了个 event data 行。信息依旧来自 raw data  
        ```

        ```bash
        sudo tdeventlog 2>&1 | python3 eventlog_annotate.py
        # 事件 0 — log 头(EV_NO_ACTION,RTMR0)
        # `Spec ID Event03` + SHA384 声明。代表"event log 起点、用 SHA384",不 extend。认得它是开头标记即可。
        ==== TDX Event Log Entry - 0 [0xBAE1E000] ====
        RTMR              : 0
        Type              : 3 (EV_NO_ACTION)
        Length            : 65
        Algorithms Number : 1
          Algorithms[0xC] Size: 384
        RAW DATA: ----------------------------------------------
        BAE1E000  01 00 00 00 03 00 00 00 00 00 00 00 00 00 00 00  ................
        BAE1E010  00 00 00 00 00 00 00 00 00 00 00 00 21 00 00 00  ............!...
        BAE1E020  53 70 65 63 20 49 44 20 45 76 65 6E 74 30 33 00  Spec ID Event03.
        BAE1E030  00 00 00 00 00 02 00 02 01 00 00 00 0C 00 30 00  ..............0.
        BAE1E040  00                                               .
        RAW DATA: ----------------------------------------------
        
        # ...
        # **事件 13 — grubx64.efi(EV_EFI_BOOT_SERVICES_APPLICATION,RTMR1)**
        # bootloader 二进制进 RTMR1 的代表。RTMR1 = kernel/bootloader 这层的锚点,看这一条就懂 RTMR1 装什么。
        ==== TDX Event Log Entry - 13 [0xBAE1E59B] ====
        RTMR              : 1
        Type              : 0x80000003 (EV_EFI_BOOT_SERVICES_APPLICATION)
        Length            : 230
        Algorithms ID     : 12 (TPM_ALG_SHA384)
        Digest[0] : 4637fb5cd30847e5f09ae24f8a50ce1611c4d21afd0ecb69c8ec40bc82dc11bc48abda1f8044fe340bfb70b29606eb47
        Event Data        : npK
        RAW DATA: ----------------------------------------------
        BAE1E59B  02 00 00 00 03 00 00 80 01 00 00 00 0C 00 46 37  ..............F7
        BAE1E5AB  FB 5C D3 08 47 E5 F0 9A E2 4F 8A 50 CE 16 11 C4  .\..G....O.P....
        BAE1E5BB  D2 1A FD 0E CB 69 C8 EC 40 BC 82 DC 11 BC 48 AB  .....i..@.....H.
        BAE1E5CB  DA 1F 80 44 FE 34 0B FB 70 B2 96 06 EB 47 A4 00  ...D.4..p....G..
        BAE1E5DB  00 00 18 90 DA B9 00 00 00 00 08 C0 0E 00 00 00  ................
        BAE1E5EB  00 00 00 00 00 00 00 00 00 00 84 00 00 00 00 00  ................
        BAE1E5FB  00 00 02 01 0C 00 D0 41 03 0A 00 00 00 00 01 01  .......A........
        BAE1E60B  06 00 00 03 03 17 10 00 01 00 00 00 00 00 00 00  ................
        BAE1E61B  00 00 00 00 04 01 2A 00 02 00 00 00 00 10 00 00  ......*.........
        BAE1E62B  00 00 00 00 00 40 06 00 00 00 00 00 F2 F8 1F BF  .....@..........
        BAE1E63B  8D BB 61 47 A7 0E 08 E6 6E 70 4B F3 02 02 04 04  ..aG....npK.....
        BAE1E64B  34 00 5C 00 45 00 46 00 49 00 5C 00 75 00 62 00  4.\.E.F.I.\.u.b.
        BAE1E65B  75 00 6E 00 74 00 75 00 5C 00 73 00 68 00 69 00  u.n.t.u.\.s.h.i.
        BAE1E66B  6D 00 78 00 36 00 34 00 2E 00 65 00 66 00 69 00  m.x.6.4...e.f.i.
        BAE1E67B  00 00 7F FF 04 00                                ......
        RAW DATA: ----------------------------------------------
        # ...
        # **事件 19 — LOADED_IMAGE::LoadOptions(EV_EVENT_TAG,RTMR2)** ← 最重要
        # 这就是 cmdline。你整个实验的命门,改 cmdline → 这条 digest 变 → RTMR2 变。
        ==== TDX Event Log Entry - 19 [0xBAE1E887] ====
        RTMR              : 2
        Type              : 0x6 (EV_EVENT_TAG)
        Length            : 100
        Algorithms ID     : 12 (TPM_ALG_SHA384)
        Digest[0] : 4ab335a5759f45e080a3a6902d258c61ba3209b6e1adbd9d473e446f08670e8d5c4652c3409a3cf921c1889e8711d276
        Event Data        : LOADED_IMAGE::LoadOptions
        RAW DATA: ----------------------------------------------
        BAE1E887  03 00 00 00 06 00 00 00 01 00 00 00 0C 00 4A B3  ..............J.
        BAE1E897  35 A5 75 9F 45 E0 80 A3 A6 90 2D 25 8C 61 BA 32  5.u.E.....-%.a.2
        BAE1E8A7  09 B6 E1 AD BD 9D 47 3E 44 6F 08 67 0E 8D 5C 46  ......G>Do.g..\F
        BAE1E8B7  52 C3 40 9A 3C F9 21 C1 88 9E 87 11 D2 76 22 00  R.@.<.!......v".
        BAE1E8C7  00 00 ED 22 3B 8F 1A 00 00 00 4C 4F 41 44 45 44  ...";.....LOADED
        BAE1E8D7  5F 49 4D 41 47 45 3A 3A 4C 6F 61 64 4F 70 74 69  _IMAGE::LoadOpti
        BAE1E8E7  6F 6E 73 00                                      ons.
        RAW DATA: ----------------------------------------------
        # **事件 20 — Linux initrd(EV_EVENT_TAG,RTMR2)**
        # initrd 进 RTMR2。将来 verity rootfs 启动时这条闭合 initramfs 这一环。
        ==== TDX Event Log Entry - 20 [0xBAE1E8EB] ====
        RTMR              : 2
        Type              : 0x6 (EV_EVENT_TAG)
        Length            : 87
        Algorithms ID     : 12 (TPM_ALG_SHA384)
        Digest[0] : bcec46c17d8756f0ee2dcb59b2cdfcde0ed2a96f7f59542bea17aadb3fad032ecf5f39be32dff85b4c76e1f853c89395
        Event Data        : Linux initrd
        RAW DATA: ----------------------------------------------
        BAE1E8EB  03 00 00 00 06 00 00 00 01 00 00 00 0C 00 BC EC  ................
        BAE1E8FB  46 C1 7D 87 56 F0 EE 2D CB 59 B2 CD FC DE 0E D2  F.}.V..-.Y......
        BAE1E90B  A9 6F 7F 59 54 2B EA 17 AA DB 3F AD 03 2E CF 5F  .o.YT+....?...._
        BAE1E91B  39 BE 32 DF F8 5B 4C 76 E1 F8 53 C8 93 95 15 00  9.2..[Lv..S.....
        BAE1E92B  00 00 EC 22 3B 8F 0D 00 00 00 4C 69 6E 75 78 20  ...";.....Linux 
        BAE1E93B  69 6E 69 74 72 64 00                             initrd.
        RAW DATA: ----------------------------------------------
        
        # 总计 21 条 event entry（upd：这是初次启动的。后面稳定重启可复现是 15 条。这里只是展示 log entry 的结构）
        
        ==== Replayed RTMR values from event log ====
        rtmr_0 : 77fd11e6cbd9c85237a93ce063ed816226f45fd5efc399b2b4cd556cbf4055c27374149f06888d1506d78b3622ae6401
        rtmr_1 : 860553c2e50b7e08d12742fd5a53d1c40e3b993e8f84779d71771aecc1f7814dc1288a0fadae13f7a15bdf66a26145d0
        rtmr_2 : f6dfce6bc70c15dae02973f2fbbbc2022ff0bdbbe710f0769912271ce7c0219e14afbe3a8c6c91c91ba707d1f2eb1e83
        rtmr_3 : 000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000
        ```

-   [x] 看当前 cmdline — `cat /proc/cmdline` 存下来。注意这是 kernel 看到的 cmdline，GRUB 实际 measure 进 RTMR 的字节可能跟它不完全一样（GRUB 会补充/规范化），复现时以 event log 里的为准。另外，`/etc/default/grub` 才是写 cmdline 的位置。`/proc/cmdline` 是 kernel 报告的内存产物。事实上对比了两者不一样。

    ```bash
    cat /proc/cmdline
    BOOT_IMAGE=/boot/vmlinuz-6.8.0-111-generic root=UUID=d4e9ff18-70f5-4482-bac6-9f3dafdcb3c8 ro vga=792 console=tty0 console=ttyS0,115200n8 net.ifnames=0 noibrs nvme_core.io_timeout=4294967295 nvme_core.admin_timeout=4294967295 iommu=pt crashkernel=0M-2G:0M,2G-4G:256M,4G-64G:384M,64G-512G:512M,512G-:1G crash_kexec_post_notifiers=1
    ```

-   [x] 破坏性测试 A：改 cmdline 看 RTMR2 变不变 —

    ```bash
    sudo vim /etc/default/grub
    # 在 GRUB_CMDLINE_LINUX 里加一个无害参数，比如 test_marker=1
    sudo update-grub
    ```

    然后阿里云控制台关机 → 开机（不要 reboot），重新 dump RTMR

    -   🔍 RTMR2 变了 = boot 链有 measure cmdline，boot-time 路可走，继续 Phase 3/4。RTMR2 没变 = 退到 runtime extend（见下方备用分支），不是终点
    -   ⚠️ TDX 实例内 reboot 被禁（会变成关机），所有重启走控制台关机 + 开机，每次都是销毁旧 TD 建新 TD、RTMR 从零重建

-   [x] 去掉 test_marker，确认 RTMR2 回基线值 — 证明 measure 是确定性可复现的

### 实验结果

boot-time 会 measure cmdline，LoadOptions extend 进 RTMR2。

measure 确定性可复现。体现在机器重启一致性，以及不同 TD 实例的 MRTD / RTMR 一致。

首次启动（21 event）与稳定态（15 event）RTMR0/1/2 不同，故基线应取自“非首次启动”的稳定态。

稳定态 measurement 实例无关：同规格同镜像不同实例，RTMR0/1/2 相同 → 基线可全局一份,绑定到确切组件版本组合

稳定态基线(当前组件版本：阿里云 TDVF 版本、shim/grub 版本(Canonical 发布)、kernel 版本(`6.8.0-111`)、initrd 内容、cmdline)：rtmr_0=`a50078ab...` / rtmr_1=`8e609e74...` / rtmr_2=`0d6c53fe...`（无 dm-verity） / mrtd=`0d6c53fe...`。

------

## Phase 2 — 写最小 Go 服务（亲自从源码到编译）(~25min)

目的：写一个能输出自己 quote 的 HTTP 服务。它是普通业务进程，由 systemd 拉起，不碰 PID 1 的事。亲自写、亲自编，记下编译环境。

>   Go 交叉编译理论上只认 Go 版本和 GOOS/GOARCH，跟编译机的 OS 无关。但要实测。这轮直接在 tdx-test（Linux x86_64）上编，Phase 5 复现也在同版本 Linux 容器里编。

-   [x] 在 tdx-test 上确认编译环境：

    ```bash
    go version          # 记下确切版本，Phase 5 要 pin 同一个
    uname -m            # x86_64
    go env GOOS GOARCH # go env 打印 Go 的环境变量,GOOS GOARCH 是查这两个变量当前值: GOOS = 目标操作系统(你这里 linux)，GOARCH = 目标架构(你这里 amd64)
    ```

    -   🔍 Go 版本影响编出来的 binary。1.25 和 1.26 编同一份源码可能不同

-   [x] 写 `go.mod`，纯 stdlib，无外部依赖：

    ```
    module tdx-verity-exp
    
    go 1.26
    ```

-   [x] 写 `main.go`，两个端点：

    -   `GET /hello` → "hello from TD"
    -   `GET /attestation` → 走 TSM configfs 产 quote，返回 hex
    -   产 quote：`mkdir /sys/kernel/config/tsm/report/<随机名>` → 写 64 字节到 `inblob` → 读 `outblob` → `rmdir` → 返回 hex
    -   ⚠️ 每个请求用独立随机目录，用完 rmdir。并发请求共用 inblob 会让 A 拿到 B 的 quote
    -   收 `SIGTERM` 优雅关 server（systemd 停服务时发 SIGTERM）
    -   🔍 gateway 是普通进程：不挂载文件系统、不管 reap、不用永不退出——这些 systemd 管。崩了 systemd 能按 `Restart=` 拉起来，不会 kernel panic
    -   🔍 report_data 放 TLS 公钥 hash（Phase 6）是运行时做的，不编译进 binary，所以 rootfs hash 稳定

-   [x] 在 tdx-test 上静态编译：

    ```bash
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
      go build -trimpath -ldflags="-buildid=" -o gateway
    ```

    -   🔍 `-trimpath` 去路径、`-buildid=` 去 build ID，为可复现。`CGO_ENABLED=0` 纯静态，少一类环境依赖

        >   只要用到 cgo(比如某些 net、os/user 包在特定配置下会调用系统 C 库做 DNS 解析、用户查询),就会引入对 libc 的动态依赖。但 Go 默认在不少情况下已经接近静态。这里是为了让外围的 rootfs 尽量干净（外围的 dylib 得在 rootfs 中才可信。或者说，缩小 TCB 可信基）

-   [x] 确认静态 — `file gateway` 看到 "statically linked"

    ```bash
    file gateway
    gateway: ELF 64-bit LSB executable, x86-64, version 1 (SYSV), statically linked, with debug_info, not stripped
    ```

-   [x] 同机编两次验 hash 一致（`CGO_ENABLED`/`GOOS`/`GOARCH` 不固定时实验不一致）：

    ```bash
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-buildid=" -o g1
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-buildid=" -o g2
    sha256sum g1 g2
    
    # rm gateway2 g1 g2
    ```

    -   🔍 两次一致 = 同环境确定。不一致就是有没固定的变量（GOFLAGS、环境变量）

-   [x] 跑通 — `./gateway`，另开 SSH：

    ```bash
    curl -s localhost:8080/hello
    curl -s localhost:8080/attestation | head -c 32
    ```

-   [x] 记 binary sha256 — `sha256sum gateway`

    -   ⚠️ Phase 4 测试 C 拿它当对照：改 binary 重编 → 值变 → rootfs 变 → root hash 变 → RTMR2 变

        binary sha256 = `e7afff435a23449605842e593efa0bba97711253f117e3225c798b51ea9e5a1e`

    -   ⚠️ 这个 hash 绑定"在 tdx-test 上、用上面那个 go version 编"。Phase 5 在同版本 Linux 容器里编，看能不能得到同值

-   [x] 💡 跟 macOS 交叉编译版比（可选）：CC 之前 macOS `GOOS=linux` 交叉编得到 `4451a2a8...`，比这次原生编的

    ```bash
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
      go build -trimpath -ldflags="-buildid=" -o gateway-mac
    
    sha256sum gateway-mac
    # e7afff435a23449605842e593efa0bba97711253f117e3225c798b51ea9e5a1e  gateway-mac
    ```
    
    -   一致 → 交叉编译跟 host OS 无关，Phase 5 在哪编都行 ✅ 同 Go 版本 + 同源码 + 同环境变量 + 同 flag 可在 mac 复现相同 hash。
    -   不一致 → 编译机 OS 也是变量，Phase 5 锁"在 Linux 编"，记进关键变量清单

------

## Phase 3 — 造可复现的 dm-verity rootfs (~60min)

目的：用确定性构建做一个完整 rootfs（带 systemd），做成只读 + verity，生成 root hash。systemd 当 PID 1，gateway 是它拉起的 service。

>   核对一个认知：可复现取决于构建是否确定（固定包版本、固定时间戳、不带运行时状态），跟 rootfs 大小无关。所以不必削成最小，用确定性工具构建完整 rootfs 即可。手搓 squashfs 也能做，但完整 rootfs 用 mkosi 这类工具更省事，COCOON 就是用 mkosi。
>
>   >   mkosi 产出三个文件:rootfs(那棵目录树打成 erofs 镜像)、kernel(vmlinuz)、initrd。
>   >
>   >   搬过去其实就是把这三个文件拷到 TD 服务器,挂到阿里云已有的启动链上,不是装一套新系统。具体:
>   >
>   >   把 rootfs 镜像写进系统盘的一个分区(dd 或 cp 到块设备),kernel 和 initrd 放进 `/boot`。然后改阿里云那台机器的 GRUB cmdline,加上你 veritysetup 出来的 `roothash=...`,让它指向你的 rootfs 分区和 verity 设备。关机开机,GRUB 加载你的 kernel + initrd,initrd 按 roothash 挂上只读 verity rootfs,systemd 起来,把 gateway 拉起来。
>   >
>   >   启动链里 TDVF、shim、grub 还是阿里云的(你也换不了),你换掉的是它们后面那段:kernel、initrd、rootfs。这段正好对应 RTMR1/RTMR2,所以换完 measurement 会变成你这套的新值。
>   >
>   >   第一轮可以更省事,Phase 4 那个简化路线:先不真从 verity rootfs 启动,只把 root hash 塞进 cmdline、关机开机、确认 RTMR2 按预期变了,验证因果链。真把镜像写进分区、从 verity rootfs 启动留到第二轮。
>   >
>   >   至于具体怎么写盘、GRUB 怎么配,等三个产物在构建 4 出来了再展开,现在不用纠结。
>   >
>   >   >   怎么写盘：我先用 mkosi 制作了 rootfs 这个角色，以及它对应的文件。我开一个新的磁盘，将 mkosi 制作的那些文件写进这个新的块设备。然后我在 cmdline 那里将 root 指向这个块设备。那么角色还是那个角色，都是rootfs，但是里面的内容就完全不一样了，就达到了我们想要的效果：定制 rootfs 文件内容。又因为角色的先后顺序，我不需要担心挂载点 / 下的其他角色的问题，因为这些角色后于 rootfs 挂载。

-   [x] 选构建方式 — 两条，先定一条：

    -   **mkosi（推荐，COCOON 同款）**：完整 Debian/Ubuntu rootfs，自带 systemd，确定性构建。学习成本高一点，但 PID 1 和服务管理都现成。参考 COCOON 的 `reprodebian/mkosi.conf` 和 mkosi 官方文档，别自己凭空写 config。

        安装两个依赖包，虚拟环境

        ```bash
        sudo apt-get install -y python3-venv
        python3 -m venv ~/mkosi-venv
        . ~/mkosi-venv/bin/activate
        pip install git+https://github.com/systemd/mkosi.git@v26
        mkosi --version
        ```

        ```ini
        # ~/zeroseal-mkosi
        cat > mkosi.conf << 'EOF'
        
        [Distribution]
        Distribution=ubuntu
        Release=noble # ubuntu 发行版 24.04 代称
        Architecture=x86-64
        
        [Content]
        Packages=
          systemd
          bash
        
        [Output]
        Format=directory
        Output=zeroseal
        EOF
        
        . ~/mkosi-venv/bin/activate
        mkosi build
        ```

        >   mkosi build 经过的阶段,对照前面的流程,这次没有自定义脚本,只有声明式那几步:
        >
        >   1.  解析 config。
        >   2.  连 archive.ubuntu.com 拉包(日志里大量 `Setting up xxx`),把 ubuntu noble 基础系统 + systemd + bash 装进一棵新目录树 `/buildroot`。
        >   3.  跑 mkosi 内置收尾,日志里能看到的:
        >       -   补建装包时缺的用户/组(`Creating user 'systemd-network'`、`systemd-journal` 组)——tmpfiles 先跑、用户后建,这里补上。
        >       -   generate system users / volatile files / 应用 presets(`systemctl preset-all`)。
        >       -   几个 unit 报 `is masked, ignoring`(x11-common / cryptdisks / hwclock),Ubuntu 默认 mask,跳过,正常。
        >       -   hwdb 报 `No hwdb files found, skipping`,没装 udev 硬件库,正常。
        >   4.  输出 `Format=directory`,产物是目录树不是镜像文件。
        >
        >   两处跟最初设想不同,记一下:
        >
        >   -   host 工具够用,没 bootstrap tools tree,直接用了 host 的 apt(tools tree 留构建 3)。
        >   -   产物落在工作目录 `~/zeroseal-mkosi/zeroseal`,没有 `mkosi.output/` 那层。这版默认行为。

        跟复现相关的：

        ```bash
        cd ~/zeroseal-mkosi/zeroseal
        
        sudo cat etc/machine-id; echo "---"
        uninitialized
        ---
        
        ls usr/lib
        clock-epoch    mime        systemd # systemd
        environment.d  modprobe.d  sysusers.d
        init           os-release  tmpfiles.d
        kernel         pam.d       udev
        locale         pcrlock.d   x86_64-linux-gnu
        lsb            sysctl.d
        
        sudo ls -la var/log var/lib/apt/lists 2>/dev/null | head -30
        var/log:
        total 20
        drwxr-xr-x   4 root root            4096 Jun  1 16:58 .
        drwxr-xr-x  11 root root            4096 Jun  1 16:58 ..
        -rw-r--r--   1 root root            2202 Jun  1 16:58 alternatives.log
        -rw-rw----   1 root utmp               0 Jun  1 16:58 btmp
        -rw-r--r--   1 root root               0 Jun  1 16:58 faillog
        drwxr-sr-x+  2 root systemd-journal 4096 Jun  1 16:58 journal
        -rw-rw-r--   1 root utmp               0 Jun  1 16:58 lastlog
        drwx------   2 root root            4096 Jun  1 16:58 private
        lrwxrwxrwx   1 root root              39 Jun  1 16:58 README -> ../../usr/share/doc/systemd/README.logs
        -rw-rw-r--   1 root utmp               0 Jun  1 16:58 wtmp
        # var/lib/apt/lists/ —— 从 archive.ubuntu.com 下载的仓库索引(每个仓库有哪些包、什么版本、依赖关系、各自的 hash)。apt 拿它算依赖、做版本比较。apt update 刷新的就是这个目录。
        
        ```

        然而上面的 `mkosi.conf` 还不能稳定本机编译复现。下面这个才可以。

        ```ini
        [Distribution]
        Distribution=ubuntu
        Release=noble
        Architecture=x86-64
        Snapshot=20260430T230000Z   # 这一步锁 Ubuntu 仓库版本，固定在那个时刻，锁当天仓库所有 apt 包版本 + kernel 版本
        
        [Content]
        Packages=
          systemd
          bash
          linux-image-6.8.0-111-generic # kernel 包要留(mkosi 拿它造 initrd + 装 /usr/lib/modules),vmlinuz 这次也用 mkosi 的(已确认与云端 byte 一致)。
        
        [Output]
        Format=directory
        Output=zeroseal
        SourceDateEpoch=0  # SourceDateEpoch=0 归零 mtime
        MachineId=ffffffffffffffffffffffffffffffff # MachineId= 钉死机器 ID
        RemoveFiles=
          /var/log/alternatives.log
          /var/cache/ldconfig/aux-cache
        # 在不填写这个时，在服务器 build 两次有这俩的差异。cocoon 也将这俩删除
        ```

        之后将 .service 服务放入 overlay。`ExtraTrees=overlay` 告诉 mkosi:构建到某一步时,把 `overlay/` 这个目录的内容,按目录结构原样叠加(拷贝)到正在攒的 rootfs 树上。叠加按照路径对路径地铺—— `overlay/usr/local/bin/gateway` 就铺到 rootfs 的 `/usr/local/bin/gateway`,`overlay/usr/lib/systemd/system/gateway.service` 铺到 rootfs 的 `/usr/lib/systemd/system/gateway.service`。overlay 里的目录层级,就是它在 rootfs 里最终落的位置。

        ```bash
        cd ~/zeroseal-mkosi
        
        # 建叠加树目录,按 rootfs 根的布局摆
        mkdir -p overlay/usr/local/bin
        mkdir -p overlay/usr/lib/systemd/system
        mkdir -p overlay/usr/lib/systemd/system/multi-user.target.wants
        
        # 1. gateway binary 放进去
        cp ~/tdx-verity-exp/gateway overlay/usr/local/bin/gateway
        chmod 755 overlay/usr/local/bin/gateway
        
        # 2. service 文件(跟构建 2 同一个)
        cat > overlay/usr/lib/systemd/system/gateway.service << 'EOF'
        [Unit]
        Description=ZeroSeal attestation gateway
        After=systemd-tmpfiles-setup.service
        
        [Service]
        ExecStart=/usr/local/bin/gateway
        Restart=on-failure
        
        [Install]
        WantedBy=multi-user.target
        EOF
        
        # 3. enable 软链(跟构建 2 同样,手动建好放进叠加树)
        ln -sf ../gateway.service \
          overlay/usr/lib/systemd/system/multi-user.target.wants/gateway.service
        ```

        以及 toolstree。目前 mkosi 无 Toolstree 的可复现机制。实际也能理解，300+ 的编译用工具一个月内多少有一些 CVE(Common Vulnerabilities and Exposures) 要修。下面有个 bash 脚本在 4.30 到 6.3 一个月内有 15/376 apt 包进行了版本变动。

        一个可行的方案是镜像包里提供 toolstree.pinned，基于用户不信任 toolstree.pinned 但信任上游最新包的原则，由用户 diff 上游 toolstree 的增量部分来断定是否可信。然后是目前的最终版本 mkosi.conf：

        ```bash
        [Distribution]
        Distribution=ubuntu
        Release=noble
        Architecture=x86-64
        Snapshot=20260430T230000Z
        
        [Content]
        Packages=
          systemd
          bash
          linux-image-6.8.0-111-generic
        ExtraTrees=overlay # 多了二进制进的
        
        [Output]
        Format=directory
        Output=zeroseal
        SourceDateEpoch=0
        MachineId=ffffffffffffffffffffffffffffffff
        RemoveFiles=
          /var/log/alternatives.log
          /var/cache/ldconfig/aux-cache
        
        [Build]
        ToolsTree=mkosi.tools.pinned.20260602T220000 # 多了个自打包的 toolstree
        
        ```

        下面是用于评定 toolstree 在某个时间后更新了哪些包的脚本（TODO：和 diff 那个脚本合并为一个 workflow）：

        ```bash
        # 用于评定 toolstree 在某个时间后更新了哪些包
        TREE=/root/zeroseal-mkosi/mkosi.tools.pinned
        CUT=2026-04-30
        JOBS=32
        SAFE=${CUT//-/}   # 用于文件名 20260430
        command -v jq >/dev/null || sudo apt-get install -y jq
        check() {
          local line="$1" name ver json d
          name=${line%% *}; ver=${line#* }
          json=$(curl -fsSL --max-time 25 --retry 3 --retry-delay 2 -G \
            "https://api.launchpad.net/devel/ubuntu/+archive/primary" \
            --data-urlencode "ws.op=getPublishedBinaries" \
            --data-urlencode "binary_name=${name}" \
            --data-urlencode "exact_match=true" \
            --data-urlencode "order_by_date=true") || { echo "ERR  $name" >&2; return; }
          d=$(printf '%s' "$json" | jq -r --arg v "$ver" \
              '[.entries[]|select(.binary_package_version==$v)|.date_published]|min // empty')
          [ -n "$d" ] && [[ "${d:0:10}" > "$CUT" ]] && printf '%s\t%s\t%s\n' "$name" "$ver" "${d:0:10}"
        }
        export -f check; export CUT
        dpkg-query --admindir="$TREE/var/lib/dpkg" -W -f '${Package} ${Version}\n' \
        | xargs -P "$JOBS" -d '\n' -I{} bash -c 'check "$@"' _ {} \
        | sort | tee "/tmp/after_${SAFE}.txt"
        echo "---"; echo "${CUT} 之后发布: $(grep -c . /tmp/after_${SAFE}.txt) 个"
        ```

        launchpad.net 查证示例：

        ```bash
        https://launchpad.net/ubuntu/+source/curl/+publishinghistory
        # 找 8.5.0-2ubuntu10.9 / 2026-05-04(覆盖 curl + libcurl3t64-gnutls + libcurl4t64 + libcurl4-openssl-dev)
        https://launchpad.net/ubuntu/+source/xz-utils/+publishinghistory
        # 找 5.6.1+really5.4.5-1ubuntu0.3 / 2026-06-02(覆盖 liblzma5 + xz-utils)
        https://launchpad.net/ubuntu/+source/libpng1.6/+publishinghistory
        # 找 1.6.43-5ubuntu0.6 / 2026-05-07(libpng16-16t64)
        
        # 找 Status: Published（相较于 Superseded 是生效中），target: Noble
        ```

        镜像安全性，也就是镜像内的 rootfs 和 kernel 则有另一套没搭的 workflow。大体文字规则：

        >   **月度固定更新**:把镜像的 `Snapshot=` 挪到新时刻、`linux-image-6.8.0-NNN-generic` 升到 noble 最新 ABI,重建并重新部署,然后存下新 manifest 跟旧的 diff 作为可复现核对。
        >
        >   **月内紧急判断**:盯 Ubuntu Security Notices(USN RSS `https://ubuntu.com/security/notices/rss.xml`,按你**镜像**清单过滤),只要某条标着"remote + 任意代码执行/认证绕过"且命中你对外暴露的组件、或是能容器逃逸/提权的 kernel CVE(对你 CVM 最致命),就立刻走带外重建,不等月底。

-   [ ] rootfs 里要有：

    -   systemd（PID 1，构建工具会带）
    -   `/usr/local/bin/gateway`（你的 binary）
    -   一个 systemd service 文件拉起 gateway（`After=` 等基础挂载就绪，`Restart=on-failure`）
    -   产 quote 要写的 `/sys/kernel/config`（configfs）—— systemd 默认会挂，确认 unit 没禁掉
    -   🔍 运行时要写的路径（/run /tmp、Phase 6 的证书目录）走 tmpfs，不落在只读 rootfs 上。systemd 默认把 /run /tmp 挂 tmpfs，证书目录额外配

-   [ ] 确定性打包（手搓 squashfs 时用，mkosi 自己管确定性）：

    ```bash
    mksquashfs ./rootfs rootfs.img -all-root -noappend \
      -no-xattrs -mkfs-time 0 -all-time 0 -reproducible
      
      cd ~/zeroseal-mkosi
    echo "=== 新工具树里 veritysetup 在不在 ==="
    sudo find mkosi.tools.old -name 'veritysetup' 2>/dev/null
    echo "=== manifest 里有没有 cryptsetup-bin ==="
    grep -iE 'cryptsetup' mkosi.tools.manifest
    === 新工具树里 veritysetup 在不在 ===
    mkosi.tools/usr/sbin/veritysetup
    === manifest 里有没有 cryptsetup-bin ===
            "cryptsetup-bin",
            "libcryptsetup12",
    
    
    ```

    -   🔍 时间戳、文件顺序、压缩参数任一不固定 hash 就不稳。记下哪个参数影响最大
    -   ⚠️ squashfs-tools 版本也影响产出，记下版本号，Phase 5 pin 同一个

-   [ ] 生成 verity hash tree 和 root hash — `veritysetup format rootfs.img hashtree.img`

    -   `Root hash: <64 hex>` 是产出，cmdline、复现、验证都用它，存下来
    -   ⚠️ 可复现要固定并记录：`--salt=<fixed>`（默认随机，不固定每次 root hash 不同）、data block size、hash block size、hash 算法（默认 sha256）

-   [ ] 验证能挂、写被拒：

    ```bash
    sudo veritysetup open rootfs.img verity-test hashtree.img <ROOT_HASH>
    sudo mount -o ro /dev/mapper/verity-test /mnt/test
    cat /mnt/test/usr/local/bin/gateway > /dev/null   # 能读
    echo x | sudo tee /mnt/test/x                      # 应失败（只读）
    ```

-   [ ] 破坏性测试 B：改一字节看 verity 报错 — `dd` 往 rootfs.img 写一字节，重新 open + 读，`dmesg | grep verity` 看报错

    -   🔍 确认 verity 按 block 校验

-   [ ] 💡 启动验证留到第二轮：第一轮走 Phase 4 简化路线（只验 cmdline→RTMR2 因果，不真从 verity rootfs 启动）时，systemd + gateway 这套不真跑。真启动（第二轮）时才验 systemd 能不能起、gateway service 拉没拉起、configfs 挂没挂上。先把 rootfs 和 root hash 造出来

-   [ ] 💡 initrd 这层：真用 verity rootfs 当启动盘时，要么 cmdline 用 `dm-mod.create` 让内核直接建 verity 设备（免 initramfs），要么 initramfs setup。用 initramfs 它也进 measurement（RTMR2），mkosi 能把 initramfs 也做成确定性（比 Ubuntu initramfs-tools 好复现）。这也是用 mkosi 的一个好处

------

## Phase 4 — root hash 进 measurement(闭合链条)

目的:把 verity root hash 灌进 cmdline 让它进 RTMR2,验证换 binary 必然导致 measurement 变。

> 本轮 kernel/initrd/rootfs 全用 mkosi 产出(已确认 mkosi 的 vmlinuz 与云端 byte 一致,
> 所以统一用 mkosi 这套,RTMR1 不会因换 kernel 而变)。挂 verity 走 mkosi initrd 里的
> systemd-veritysetup,不用 dm-mod.create。
>
> Phase 1 已实测:cmdline 经 LOADED_IMAGE::LoadOptions 进 RTMR2,boot-time 路通。
> 这一步不再验"cmdline 进不进 RTMR2",只验"binary→root hash→cmdline→RTMR2"上游因果。

- [ ] 确认 mkosi initrd 带 verity 挂载能力
    - 🔍 用 mkosi 的 initrd 启动,挂只读 verity rootfs 靠 initrd 里的 systemd-veritysetup。
      构建出 initrd 后先确认它包含 veritysetup 相关组件(Phase 3 出 initrd 时一并查)

- [ ] 构造带 verity 的 cmdline — 用 roothash,不用 dm-mod.create
roothash=<ROOT_HASH> systemd.verity=1 root=<rootfs 分区>
    - ⚠️ 具体 cmdline 参数等 Phase 3 出了 root hash、确认 initrd 挂 verity 的方式后写准
      (systemd.verity 系列参数 vs 直接 root=/dev/mapper/... 取决于 initrd 配置)
    - ⚠️ root hash 是 dm-verity 自己的 hash(sha256 算的 block 校验树),跟 RTMR 的
      SHA384 是两套,别混

- [ ] 关机 + 开机,重新 dump RTMR — 跟 baseline 比,RTMR2 应该变了
    - 🔍 "RTMR2 因 cmdline 改而变"Phase 1 测试 A 已证。这步真正确认的是下面测试 C/D 的
      上游因果(binary→root hash→cmdline)
    - ⚠️ 换了 mkosi 的 kernel/initrd,RTMR1/RTMR2 基线相对纯云端会变(initrd 不同),
      要重新建基。比对用稳定态(非首次启动)的值
    - ⚠️ TDX 实例内 reboot 被禁,走控制台关机 + 开机

- [ ] 破坏性测试 C(主路径):改 Go binary → 重跑 mkosi → 新 root hash → 新 cmdline →
      关机开机 → RTMR2 再变
    - 🔍 证明换 binary 必然导致 root hash 变、进而 RTMR2 变。攻击者想偷换 binary 又让
      measurement 不变,做不到
    - 注:"重做 rootfs"现在指重跑 mkosi build,不是手搓

- [ ] 破坏性测试 D:改 binary 但 cmdline 塞旧 root hash → verity 挂载失败 / 起不来
    - 🔍 堵死另一条攻击路:想用旧 hash 骗 measurement,verity 这关过不了

- [ ] 启动路线:kernel byte 与云端一致、initrd/rootfs 都齐,本轮可以直接真从 verity
      rootfs 启动(原"第一轮只验因果不真启动"的简化可不要了,门槛已经低了)

------

## Phase 5 — 离线验证（reproducible 的灵魂）(~90min)

目的：证明一台独立机器（你的 Mac）不依赖服务器、不联网在线服务，从公开源码复现出全套预期 measurement，再用离线验签确认 quote。没这步“可验证”是空话。

>   **在哪台机器上做什么**：“Mac 端”指独立于服务器的验证方，不是所有步骤都在 macOS 原生跑。复现 rootfs 要在锁死版本的 Linux 环境里做（mksquashfs / veritysetup 是 Linux 工具，macOS 装的版本和行为跟 Ubuntu 对不上）。能在 macOS 原生做的只有 RTMR replay（纯 SHA384 计算）和 quote 验签（go-tdx-guest 是 Go 库，跨平台）。
>
>   **两个工具别混**：验签用 `go-tdx-guest`（验 quote 签名链 + 比对 measurement）；算 MRTD 预期值用 `gce-tcb-verifier` 的 `mrtd_from_ovmf.go`（从 TDVF 二进制算 MRTD）。是两个不同的库，干两件不同的事。

-   [ ] 准备 pinned 复现环境 — 写一个 Dockerfile，把 squashfs-tools、cryptsetup 的版本号 pin 到跟服务器完全一致，其他参与 build 的工具也固定
    -   🔍 reproducible 的“可复现”意思是：任何人用相同工具链 + 相同源码能算出相同 hash。这个“相同工具链”就是这个 pinned Docker 镜像，验证方在哪个 OS 上无所谓，能 `docker run` 就行
    -   ⚠️ **真瓶颈之一**。卡住很正常，记录每个导致 hash 不一致的变量
-   [ ] 容器里从同源码 + 同 build 步骤复现 rootfs.img — 目标 bit-for-bit 一致
-   [ ] 容器里 `veritysetup format` 算 root hash — 跟服务器的对比
    -   一致 = rootfs 复现成功。不一致 = 还有没固定的变量（squashfs-tools 版本、salt、block size、文件顺序、时间戳里挑）
-   [ ] 算全套预期 measurement，逐个比对（不能只算 RTMR2）：
    -   **MRTD**：可从运行实例 dump TDVF，用 `gce-tcb-verifier` 的 `mrtd_from_ovmf.go` 算预期 MRTD 验一致性；来源仍信阿里云（无可复现 build）。一致性可自验、来源信阿里云，与信 host/硬件同级
        -   ✅ Phase 1 实测：两台独立实例（隔 6 天、不同实例 ID）MRTD 完全一致（`e0199499...`），说明阿里云这两台用同一 TDVF 版本，MRTD 基线可全局一份。但**绑定到确切 TDVF 版本**，阿里云升级 firmware 后要重新建基
    -   **RTMR0**（TDVF 配置）：对照预期 firmware config measurement
    -   **RTMR1**（bootloader 二进制 shim/grub）：对照 Canonical 发布的 shim/grub digest
        -   ⚠️ **真瓶颈之二**：RTMR0/RTMR1 的参考值难拿。gce-tcb-verifier issue #73 实证——连 Google 都只给了 MRTD 参考值，RTMR0/1 参考值依然没有现成来源。别当顺手活，要预留时间自己从组件二进制推
        -   📌 kernel（vmlinuz）进哪个 RTMR 要从 Phase 1 的 event log 实际确认，别想当然写死成“RTMR1 = kernel”。你的 event log 里 RTMR1 装的是 shim/grub/BOOTX64 这些 EFI 程序
    -   **RTMR2**：用你复现的 root hash + 完整 cmdline 字节算
    -   🔍 RTMR replay 分两步，别合成一步：① 用 event log 里记录的 digest 按顺序做 `RTMR = SHA384(RTMR_prev || digest)` replay；② 另外验证 event log 里这个 digest == 你从公开源码算出的 component hash。不要默认 digest 就等于 SHA384(component)，取决于 event 类型
    -   🔍 cmdline 差一个空格 RTMR2 就不对，拿 event log 里被 measure 的确切字节（LoadOptions 事件的 raw data），不是肉眼看 /proc/cmdline
-   [ ] 也要验 td_info 里的非 measurement 字段（Phase 1 已拿到实测基线）：
    -   **attributes 的 debug 位 = 0**：实测两台均为 `0000001000000000`。debug=1 的 TD 内存可被外部读，measurement 全对也没用，这是必查项
    -   **xfam**：实测 `e742060000000000`，纳入预期值
    -   mrconfigid / mrowner / mrownerconfig：实测全零（你没用 TD owner 绑定），符合预期
-   [ ] 离线验 quote — 用 go-tdx-guest（Mac 原生能跑）或 DCAP QVL，验两件事：
    -   ① quote 签名链追到 Intel root CA（确认真 TDX 芯片）。需要 Intel 的 PCK 证书和 collateral，预先缓存到本地，验签本身离线。注意 TCB info 有有效期，会过期，要有更新机制
    -   ② quote 里的 MRTD / RTMR0 / RTMR1 / RTMR2 + attributes + xfam 全部 == 你算出的预期值
    -   🔍 这才是“只信 Intel 芯片”：验签用 Intel 根证书，不把 quote 发给任何在线服务（包括阿里云远程证明服务，那个会把验证信任交还给阿里云）
-   [ ] 交叉验证 event log 与 quote 自洽（Phase 1 已验过一次，生产沿用）：
    -   tdeventlog replay 出的 RTMR == tdreport / quote 里的 RTMR。一条是软件明文 replay、一条是硬件寄存器快照，两者对上才证明 event log 没被篡改。Phase 1 实测两者一致（rtmr_0/1/2 = `a50078ab` / `8e609e74` / `0d6c53fe`）
-   [ ] 闭环确认 — 签名追到 Intel + 全套 measurement 都对 = 在公有云 firmware 上限内，证明了远端跑的是公开源码 build 的 binary，没夹私货
-   [ ] ⚠️ 上线前必验（Phase 1 未覆盖的边界）：
    -   **关机时长影响**：Phase 1 的实例无关性是在连续快速重启下验的。一台 TD 关机数天再开，MOK/SBAT 状态会不会重新初始化（退回 21 条首次态）未测。生产前验一次
    -   **稳定态基线值**（当前组件版本，全局一份）：
        -   MRTD = `e0199499baacb2e4f4bc73046f25bedf674d42defbe4e854242bd6554a9d155edf7f3bff8e6202e63ed230e59ab2568a`
        -   rtmr_0 = `a50078ab426e8e044b88eca0192a0fb7fec69e31f55f7575af3e0ab865611eeb6cc29a1110d99f7562af1f0d84a3338b`
        -   rtmr_1 = `8e609e74189d7e91b22e20cc2297e407f069824cf94c7706cc391a9e701c0e085ca5a9f2b7f3cef3a91d55f5443f50e4`
        -   rtmr_2（无 dm-verity 的裸 Ubuntu 基线）= `0d6c53fe9686af16ec44f8c261897ab675acc35ab00eedacc40bf3c447cad8c5aa054d9fffa60750e87a118d99b26c1f`
        -   注：rtmr_2 这个值是当前未上 verity 的状态，Phase 4 上了 dm-verity cmdline 后会变成新值，到时记新基线

------

## Phase 6 — TLS 绑定（Mac 当 CA + report_data 绑公钥）(~45min)

目的：把 attestation 跟 TLS 连起来，做到私钥不出 TD。

-   [ ] Mac 建自签 CA — openssl 生成 CA key + cert，这是你的信任根
-   [ ] 选方案 — 两条路：
    -   方案 A（先跑通）：Mac 签好 server cert + key，scp 到服务器。够验 TLS 链路，但 key 经过 scp 不算安全
    -   方案 B（RA-TLS）：服务器在 TD 内生成 key pair，把 pubkey 的 sha256 放进 quote 的 report_data（Phase 2 已预留），Mac 验 quote（含验全套 measurement）通过后，确认这个 pubkey 来自可信 TD，再用 CA 签发证书。私钥从不离开 TD
    -   🔍 选 B 才符合“私钥不出 TD”。先用 A 跑通 TLS，再升级 B
-   [ ] 服务器起 HTTPS — Go 服务加载证书，两个端点走 TLS
-   [ ] Mac 验证 — `curl --cacert myCA.pem https://<server>/hello` 通
-   [ ] 💡 进阶完整 RA-TLS：把 quote 直接嵌进 TLS 证书的 X.509 扩展，握手时 Mac 验 measurement，attestation + 传输加密一次完成（Phala / Intel 都有现成库可参考）
-   [ ] 💡 前瞻 — 方案 B 的验证逻辑（收到 quote → 验签 + 比对 measurement → 确认 report_data 里的 pubkey → 放行）跟你将来自建 KMS 的 verifier 是同一套东西。Phase 5 写的离线验证代码，自建 KMS 时能直接复用，做成一个长期服务

------

## Phase 7 — 清理 (~5min)

-   [ ] 导出产物 — baseline-rtmr.txt、全套 measurement 基线（MRTD / RTMR0 / RTMR1 / RTMR2）、各阶段 RTMR 值、root hash、salt、mksquashfs 参数、pin 了版本的 Dockerfile、复现脚本，存本地
-   [ ] 记录“关键变量清单”（见品味校准）
-   [ ] 控制台删实例 + 删盘

------

## 品味校准 (~5min)

自己写，逼自己形成判断，三个月后回头验证。

-   [ ] 变量定位：哪几个变量决定 RTMR2 的值？哪几个决定 root hash 能否复现？（cmdline 全文、salt、block size、squashfs-tools 版本、mksquashfs 时间戳 / 顺序、Go 编译 flag……列出来）
-   [ ] 路线结果：阿里云 c9i Ubuntu 24.04 默认把 cmdline measure 进 RTMR2 了吗？进了就走 boot-time（强）；没进就退 RTMR3 runtime extend（够用但有 bootstrap 缝）。这两条你各自的安全语义说得清吗？
-   [ ] reproducible build 卡在哪？容器里复现的 root hash 和服务器一致了吗？不一致是哪个变量？这是 zeroseal “可验证”的真正瓶颈
-   [ ] firmware 上限：MRTD 这一格在公有云上验不到可复现的 TDVF，你愿不愿意接受“信芯片 + 信阿里云 firmware 是声称版本”这个边界？要彻底拆掉只能上能控 firmware 的裸金属，那个成本你算过吗？
-   [ ] 工具链判断：放弃阿里云那套 SDK、改用 TSM configfs + 离线 DCAP QVL，对 zeroseal 生产部署，你会依赖哪条工具链？
-   [ ] 一句话总结：跟潜在用户解释“我们怎么做到只信 Intel 你就能验证我们没偷看”，连同上面那条 firmware 边界，你会怎么说？
