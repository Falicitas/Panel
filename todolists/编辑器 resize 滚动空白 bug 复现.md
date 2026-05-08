# 编辑器 resize 滚动空白 bug — 复现记录

WKWebView 容器被 SwiftUI 加宽后, Milkdown 编辑器内的 scroller 没及时
clamp `scrollTop` → 视口落在 doc 末尾之外, 出现一大片空白。修复在
`mac/editor-src/src/main.ts` 的 `setupResizeClamp()`。

## 触发条件 (任一)

- ⌃⌘S 隐藏 sidebar (任意 tab 走 detail column)
- 主窗口拖拽变宽
- 进入/退出全屏
- Stage Manager / 外接显示器切换

共同点: detail column 宽度变大 → CSS reflow → 文档总高度变小 →
旧 `scrollTop` 落到新 `scrollHeight - clientHeight` 之外。

## 复现步骤 (回归测试)

1. 启动 Panel, 切到 todolist tab, 打开一份内容较长的笔记 (滚动占两屏以上)
2. **保持** sidebar 显示 (⌃⌘S 看到中间列存在), 此时 detail 列较窄, doc 渲染高度大
3. 鼠标点到文档**最底部**, cursor 落在末行
4. 按 ⌃⌘S 隐藏 sidebar
5. **预期**: 视口仍能看到 cursor 那一行 (或文档末尾, 因为 cursor 在末尾时 max scrollTop 正好让 doc 末贴视口底)
6. **bug 现象 (修复前)**: 视口里全部是空白, 必须滚轮往上拨才能看到内容
7. 鼠标随便滚一下, 内容会被「拽」回来 — 这是浏览器 / ProseMirror 内部 ResizeObserver 事后触发的 layout 修正

## 根因

实际是两层叠加:

### 第 1 层: scrollTop 不会自动 clamp (基础原因)

- `NavigationSplitView` 列宽变化不会让 SwiftUI 重新调用
  `MilkDownEditor.updateNSView` (没有 binding 变化), JS 拿不到 Swift 侧通知
- WKWebView 自己接收到了 SwiftUI 的 frame resize, 浏览器 reflow, 但
  `scrollTop` 不会主动 clamp — 只有用户后续滚动操作时浏览器才 clamp
- ProseMirror 内部有 ResizeObserver 但只关心自己的 viewport tracking, 不动
  外层 scroller 的 `scrollTop`

### 第 2 层: HTML.scrollHeight 比内容真实底偏大 (关键原因)

诊断走过两版假设才挖到真根因:

- **v1 假设**: 真 scroller 在 `.milkdown` / `.ProseMirror` 候选里 → clamp 不
  生效, 长文档仍空白
- **v2 假设**: 真 scroller 在 `#editor` 子树某个 `scrollTop>0` 元素 → clamp
  仍不生效。dump 一遍才发现:

```
HTML  sh: 29865  ch: 1260   st: 28605    ← 真 scroller, 已 clamp 到 max
.milkdown  sh: 29865  ch: 25966          ← 实际渲染高度只有 25966
```

- 真 scroller 是 `documentElement` (HTML); `body` / `#editor` 都 `height:100%`
  锁死 viewport 大小, `.milkdown` 溢出顶大 HTML 让它接收滚动
- 但 `documentElement.scrollHeight` 会被 ProseMirror 末尾的 phantom 元素
  (`overflow:visible` 子元素 / caret 占位) 撑大, 实测比 `.milkdown` 实际
  渲染底多 3899px
- 所以 v2 的 clamp **生效了** (把 scrollTop 压到 28605), 但 28605 这个 max
  本身就是错的 — 视口落在 `[28605, 29865]` 这段没真实内容的空区

## 修复要点

`main.ts` IIFE 末尾 `setupResizeClamp()`:

- `window.addEventListener('resize')` + `ResizeObserver(document.body)` 双路
  监听 (单走 window resize 在 sidebar collapse 场景下不一定 fire)
- 触发后 `requestAnimationFrame` 一帧 + `setTimeout(80ms)` 双 clamp, 兜底
  ProseMirror / Crepe 内部异步 layout
- `documentElement` 的 max 用 `.milkdown.getBoundingClientRect().bottom +
  scrollTop - innerHeight` 算 (实际内容底页面坐标减视口高度), **不能** 用
  `scrollHeight - clientHeight` (会偏大撑进空区)
- 其他 scroller (`body` / `#editor` / `#editor` 子树里 `scrollTop>0` 的) 仍走
  `scrollHeight - clientHeight` clamp, 实测都是 no-op, 留作 Crepe 升级换
  内部 scroller 时的兜底
- **不动 cursor / selection** — cursor 在末尾时 max 位置正好让 `.milkdown`
  底贴视口底, 自然可见; cursor 不在末尾时也不该被强行拉到视口里抢用户视线

## 不处理的边界

- cursor 在文档**中间**且用户已滚到 cursor 视口外位置 → 这种情况 cursor 本来
  也不在用户的视线焦点里, sidebar 隐藏后 doc 变矮, 整个 doc 可能都进视口, 没
  bug; 不进的话 cursor 仍在视口外, 但这不是 resize 引入的 — 用户原本就这么看
- find bar 打开时的 active mark — `findSetActive` 自己已经 `scrollIntoView`
  到 mark 中心, resize 后 mark 可能不在视口中心了, 但仍可见 (clamp 不会反向
  把它推出视口)

## 关联代码

- `mac/editor-src/src/main.ts` — `setupResizeClamp` / `clampEditorScrollers`
- `mac/Panel/Resources/editor/bundle.js` — esbuild 产物, 改 `main.ts` 后跑
  `cd mac/editor-src && node build.mjs` 重 build
- `mac/Panel/Views/MilkDownEditor.swift` — Swift 侧未改, 桥接保持原状
