# Panel

Personal Mac command center — SwiftUI app + Go backend + SQLite, all local on-device.

Daily ops surface for todolists, supplements, finance, and English shadowing practice. Voice keys live in macOS UserDefaults; data lives in `~/panel/data/`. Nothing leaves the machine except direct calls to OpenRouter / Groq for transcription.

See `PRD.md` for product detail and `CLAUDE.md` for build / dev conventions.

## 新机器

装好 Xcode 与 Go 1.26.1，整段执行。路径必须是 `~/panel`，后端和 launchd 都写死它：

```bash
git clone https://github.com/Falicitas/Panel.git ~/panel
cd ~/panel
./scripts/install.sh                # 后端 → 127.0.0.1:8089，launchd 常驻
./backend/rebuild-dual-notes.sh     # 双栏 → 127.0.0.1:8090
cd mac && ./rebuild.sh              # 构建并启动 Panel.app
```

首次构建从 GitHub 拉 MarkdownUI、NetworkImage、swift-cmark。

`mac/rebuild.sh` 先查钥匙串里有没有 `Developer ID Application: Tingzhang Cai
(9MA3NPTKD4)`：有就用它签名，TCC 授权（输入监控、辅助功能、麦克风）只问一次；没有就
用 ad-hoc，每次 rebuild 的 CDHash 都变，macOS 把 app 当新的，前两项可能重问一次。把
开发机的这张证书导出成 `.p12` 导入新机，就固定在第一种。

`data/` 不入库，首次启动自动建空库 `~/panel/data/panel.db`；旧数据在停掉 8089 与 8090
后直接覆盖该文件。转写与语音的 key 存在 UserDefaults，在新机设置里重填。

双栏使用独立的 `panel-dual-notes` 服务（`127.0.0.1:8090`），由 launchd 的
`com.kinesis.panel.dual-notes` 启动。其他功能仍连接 `panel-backend`（8089）。

在当前工作区根目录安装或更新双栏服务，再重建客户端：

```bash
./backend/rebuild-dual-notes.sh
./mac/rebuild.sh
```

双栏安装脚本只操作双栏的进程和启动项。登录后自动启动，退出后由 launchd
重新拉起；关闭或卸载 `com.kinesis.panel` 不影响双栏。
更新其他后端仍使用 `./backend/rebuild.sh`。

双栏沿用 `~/panel/data/panel.db` 的 `dual_notes_days` 表，保留全部记录和版本号，
无需迁移。两个服务共享数据库文件，但双栏启动只初始化自己的表。
独立日志为 `~/panel/data/dual-notes.log`；每日数据库快照存入
`~/panel/data/dual-notes-backups/`，保留最近 7 份，主后端关闭时仍会备份。

停止双栏服务（再次运行安装脚本即可恢复）：

```bash
launchctl bootout gui/$(id -u)/com.kinesis.panel.dual-notes
```

验证：`cd backend && go test ./... -race`，以及
`./mac/test-dual-notes-client.sh`。集成测试使用临时数据库和随机本地端口。

双栏支持行内公式 `$E=mc^2$` 和独立公式块（`$$` 各占一行，中间写公式）。
点击公式可修改 LaTeX 源码，移开光标后显示 KaTeX 渲染；普通美元符号写为 `\$`。
公式编辑、渲染和保存回归：在 `mac/editor-src` 运行 `npm run test:math`。


## AutoDL

侧边栏的 **AutoDL** 打开独立本机控制台 `http://127.0.0.1:8765/`，直接复用
`~/autodl-infra` 提供的实例列表、开关机、原机抢卡、独立抢新实例、本地名称和
GPU 信息。Panel 主后端（8089）与双栏服务（8090）无需修改。

AutoDL 后端需单独保持运行；尚未启动时，在终端执行：

```bash
cd ~/autodl-infra
python3 -m autodl_infra --token-file local/web-token web
```

页面顶部可重新加载或在默认浏览器打开；首次无法连接时会显示重新连接提示。
每个窗口独立保留网页；切换 Panel 侧栏保留该窗口的表单状态。关闭 Panel 不会取消抢卡任务，任务
仍由 AutoDL 后端执行。凭据留在原后端，Panel 不读 Token 或 SSH 私钥。

导航仅留在固定的本机控制台来源。macOS 的 HTTP 例外仅设置 `127.0.0.1`，
页面导航继续限制端口为 8765，不对远端网站放宽传输策略。

回归测试（独立 Swift 测试程序，不启动 Panel）：

```bash
./mac/test-autodl-page.sh
```

新文件已登记在 Xcode 工程。按工作池约定，由用户在当前工作池构建并重启验证：

```bash
./mac/rebuild.sh
```

打开侧边栏的 AutoDL，检查实例列表、缺卡时禁用开机、编辑本地名称；切到其他
页面再切回来，未提交的表单应保留。后端暂时停止时，页面应给出连接失败提示；
后端恢复后可重新连接。日常验证优先使用现有只读状态，不为验收额外租卡。
