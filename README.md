# Panel

Personal Mac command center — SwiftUI app + Go backend + SQLite, all local on-device.

Daily ops surface for todolists, supplements, finance, and English shadowing practice. Voice keys live in macOS UserDefaults; data lives in `~/panel/data/`. Nothing leaves the machine except direct calls to OpenRouter / Groq for transcription.

See `PRD.md` for product detail and `CLAUDE.md` for build / dev conventions.

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
