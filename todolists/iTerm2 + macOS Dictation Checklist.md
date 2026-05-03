# iTerm2 + macOS Dictation Checklist

按 fn fn 在 iTerm2 / Claude Code 里语音输入。

***

## Phase 0 — 开启 Dictation (\~3min)

* [x] System Settings → Keyboard → Dictation 开启，Shortcut 设为 `F12`

* [x] Language 加上需要的语言（至少 English + Chinese 一个）

***

## Phase 1 — 给 iTerm2 权限 (\~2min)

* [x] Privacy & Security → Microphone 和 Accessibility 都把 iTerm2 加上勾选
  * 🔍 Accessibility 是 Dictation 把文字注入到 iTerm2 的前置条件，少了就只有浮窗没有文字

***

## Phase 2 — 跑通 (\~3min)

* [x] iTerm2 里 fn fn，说"echo hello"，看到字出现并能 Enter 执行

* [x] 启动 `claude`，在 CC 输入框 fn fn 说一段中文 prompt，提交
  * 🔍 别拿它写代码，符号识别基本不能用，只用来说自然语言 prompt

***

## 品味校准 (\~3min)

* [ ] 实际场景里语音比打字快吗？什么场景下真的赢？

* [ ] 中英混说的识别准确率能接受吗？

* [x] 一句话：留着用，还是回去打字？

