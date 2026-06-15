# ～

看 `ls -l` 开头那个字母就能区分:

-   `crw-------` 的 `c` = character device(字符设备),`/dev/tdx_guest` 是这个
-   `brw-rw----` 的 `b` = block device(块设备),你那台机上只有 `nvme0n1` 和它的分区是这个

块设备是磁盘那种东西,有固定块大小、能随机寻址、上面能挂文件系统。`/dev/tdx_guest` 跟存储无关,它是个伪设备(pseudo device),背后没有真实的数据流。它的 major number 是 10,而 major 10 是内核的 misc 字符设备类——你前面那份输出里 `fuse`(10,229)、`vhost-net`(10,238)、`vhost-vsock`(10,241)、`hpet`(10,228) 全是 10,这些都是同一类:某个内核驱动注册的一个调用入口,没有专属 major 号就共享 10。`/dev/tdx_guest` 跟它们是同类,跟 `nvme0n1` 不是同类。

关于"接口":你理解得对,跟 web API 不是一回事。这里的"接口"指的是 tdx-guest 这个内核驱动暴露给用户态程序的调用入口,交互方式是 `ioctl` 系统调用,不是 HTTP。

放在一起对照就清楚了。web API 大概是这样:

```
POST https://api.example.com/get-report
{"report_data": "<64 bytes>"}
→ 200 OK, {"report": "<1024 bytes>"}
```

走网络、走 TCP/HTTP、参数要序列化成 JSON。

`/dev/tdx_guest` 这条是这样:

```c
int fd = open("/dev/tdx_guest", O_RDWR);

struct tdx_report_req req;
memcpy(req.reportdata, my_64_bytes, 64);   // 你的 nonce

ioctl(fd, TDX_CMD_GET_REPORT0, &req);       // 一次陷入内核

// 返回时内核已经把 req.tdreport 填好了(1024 字节的 TDREPORT)
close(fd);
```

没有网络,没有 HTTP,没有序列化。`open` 拿到一个文件描述符,`ioctl(fd, 命令码, 结构体指针)` 触发一次用户态到内核态的特权级切换,驱动按命令码(`TDX_CMD_GET_REPORT0`)分发,直接读你结构体里的 nonce、把 TDREPORT 写回同一块内存。整个过程在本机一颗 CPU 上完成。

所以这俩都符合"传参数、拿返回"的请求-响应形态,但一个是跨机器的网络调用,一个是本地的系统调用。"接口"在这里是"内核↔用户态的编程接口"的意思。

回到你的场景:你之前确认能用的 configfs TSM(`/sys/kernel/config/tsm/report/` 下 mkdir、写 `inblob`、读 `outblob`)是更上层、更通用的那条路,用文件读写代替了手写 ioctl。`/dev/tdx_guest` 是它底下兜底的驱动。两条都通向同一个 TDREPORT,只是 TSM 那条不用你自己拼结构体、也不绑定 TDX(SEV-SNP 也走同一套 configfs),所以一般优先用 TSM。