# TEE attestation 实施 todolist（侧信道 + 内存清理）

## 一、侧信道

-   [ ] 全代码扫一遍 secret 相关的比较，确保用 `subtle.ConstantTimeCompare` 或 `hmac.Equal`，不用 `==` / `bytes.Equal`
    -   主要场景：拿 EKM 当 HMAC key 验客户端 token 时

就这一条。Go 标准库的 HMAC/SHA-256 本身已经是常量时间，你不用操心底层实现。其他侧信道（cache、HT、Spectre）的攻击者门槛远超你客户群体，不在威胁模型里。

## 二、内存清理

-   [ ] EKM、派生出的 HMAC/AES key 用完手动清零

    ```go
    defer func() {    for i := range ekm {        ekm[i] = 0    }}()
    ```

    Go 编译器不会优化掉这种清零，直接写就行

-   [ ] secret 全程用 `[]byte`，不要转 `string`（string 不可变，没法清零）

-   [ ] 检查日志输出，确认 secret 不会被 `fmt.Printf("%v", ...)` 之类打出去

-   [ ] 关 core dump：进程启动时 `setrlimit(RLIMIT_CORE, 0)`，或者 systemd unit 加 `LimitCORE=0`

-   [ ] 第三方库传 secret 前先看一眼它会不会自己缓存 / 打日志，不放心就少传，用完就清

## 删掉的，附理由（AI 描述的，不确定正确性）

-   ~~确认 SHA-NI 指令~~ — Go 标准库已经处理，不用查
-   ~~dudect 时序测试~~ — 威胁模型不匹配，工程量大收益低
-   ~~关 hyper-threading~~ — 同上，且 enclave 性能砍半代价太大
-   ~~评估 GC 对 cache 的影响~~ — 攻击门槛过高
-   ~~`runtime.KeepAlive` 防编译器优化~~ — Go 不做这种优化，多余
-   ~~`mlock` / `RLIMIT_MEMLOCK`~~ — TEE 内存本身不 swap，多余
-   ~~enclave 测量值固化到 CI~~ — 这是 attestation 验证流程的事，不属于"侧信道 + 内存清理"

## 三、验证

-   [ ] 跑一次 gcore dump 当前进程，`strings core.<pid> | grep <已知 secret 前缀>` 应该 grep 不到
    -   这一条同时验证了"内存清零生效"和"core dump 关闭"两件事