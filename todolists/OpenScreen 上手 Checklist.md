# OpenScreen 上手 Checklist

从下载到录出一个带 zoom 效果的产品 demo 视频。

前置：macOS 13+ 或 Linux（Electron 应用）。

---

## Phase 0 — 安装 + 权限 (~10min)

- [ ] 从 GitHub Releases 下载最新版（v1.3.0）— macOS 选 .dmg，Linux 选 .AppImage
- [ ] macOS 用户解除 quarantine — `xattr -rd com.apple.quarantine /Applications/Openscreen.app`（先给终端 Full Disk Access）
- [ ] 授权 Screen Recording + Accessibility 权限 — System Settings > Privacy & Security
- [ ] 启动应用，确认能看到 source selector 界面

---

## Phase 1 — 录一段最简单的 demo (~10min)

- [ ] 选择录制源 — 试一下 Full Screen 和 Window 两种模式
- [ ] 录 30 秒操作 — 随便操作点什么，停止录制
- [ ] 在编辑界面预览 — 确认视频 + 音频（如果开了麦克风）正常
- [ ] 直接导出一次 — 默认设置导出 mp4，看看成品效果

---

## Phase 2 — Zoom 效果：OpenScreen 的核心功能 (~15min)

这是 OpenScreen 对标 Screen Studio 的卖点。

- [ ] 手动添加 zoom — 在 timeline 上选一个点，添加 zoom，调整 zoom 深度和持续时间
- [ ] 试自动 zoom — 录制时开启 auto zoom，录完看效果
- [ ] 调整 zoom 位置 — 拖动 zoom 区域到你想聚焦的 UI 元素上
- [ ] 用快捷键操作 — 添加/删除 zoom 和 flag 都有键盘快捷键
  - 🔍 zoom 深度分级（depth levels），理解不同级别的视觉效果差异

---

## Phase 3 — 编辑和美化 (~15min)

- [ ] 换背景 — 试 wallpaper、纯色、渐变、自定义图片四种
- [ ] 调 border radius — 给录制窗口加圆角
- [ ] 加 motion blur — 让 pan/zoom 过渡更平滑
- [ ] 添加 annotation — 文字、箭头、图片标注
- [ ] 裁剪 + 调速 — trim 掉多余部分，关键操作减速展示
- [ ] 💡 试 webcam PIP 模式 — v1.3.0 新增功能

---

## Phase 4 — 导出 (~10min)

- [ ] 选不同 aspect ratio 导出 — 16:9（YouTube）、9:16（竖屏）、1:1（社交媒体）
- [ ] 试 GIF 导出 — 适合嵌入 README 或 Slack
- [ ] 调导出分辨率 — 看看不同分辨率下文件大小差异
- [ ] 💡 试 transparent background — native aspect ratio + 0 padding

---

## 品味校准 (~5min)

- [ ] 判断：OpenScreen 能持续维护下去还是会烂尾？（MIT 协议，单人维护为主）
- [ ] 它解决的问题是真痛点吗？Screen Studio $89 买断 vs 免费 OpenScreen，值得切换吗？
- [ ] 核心设计决策：Electron + 纯前端渲染 pipeline。跟 Screen Studio 的原生方案比，这个 tradeoff 带来什么限制？
- [ ] DX 打分（1-5）：安装摩擦、UI 直觉性、导出质量、bug 密度
- [ ] 一句话总结：推荐还是劝退？
- [ ] 你的 LLM Gateway 产品如果要录 demo，OpenScreen 够用吗？

---
---