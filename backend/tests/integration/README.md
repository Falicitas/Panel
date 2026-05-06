# Integration tests

跨包集成测试 — `httptest.NewServer` 起一个真后端 + temp `sqlite`,用 `http.Client` 模拟 mac client / `curl` 调 API,断言 db 状态。

跟单元测试(各 package 内 `_test.go`,如 `store/supplements_test.go`)互补:
- **单测**:单个函数 / store helper,定位 bug 用
- **集成测试**:跨 store/server/supplements 包,全链路 API → db 状态机,**user 报过的复现路径必须沉淀这里**

## 运行

```bash
cd /Users/kinesis/panel/backend
go test ./tests/integration/ -v -count=1
# 单个:
go test ./tests/integration/ -run TestSupplement_SnoozeBodySync -v
```

## 文件索引

| 文件 | 覆盖 |
|---|---|
| `helpers.go` | 测试基础设施: `newTestEnv` / `do/post/put/get` / `notifByRef` / `assertNotifBody` / `hmFromNow` 等 |
| `notifications_supplement_test.go` | supplement panel 通知 lifecycle |
| `notifications_english_test.go` | (TODO) english 通知 lifecycle |
| `notifications_meal_test.go` | (TODO) meal 通知 lifecycle |

## supplement 通知已覆盖 case

| Test | 历史 commit | 复现路径 (来自 user 报告) |
|---|---|---|
| `TestSupplement_BasicActiveTriggerNotif` | (基础) | 创建 active window → 自动 INSERT 通知,body 含"保健品 时段" |
| `TestSupplement_DeleteWindowDismissesNotif` | issue-25 `06db069` | PUT 加 active window → 通知 → 立刻 PUT `windows=[]` 删 → 通知 idx 越界 dismiss |
| `TestSupplement_SnoozeBodySync` | issue-25 `4878dae` | snooze count=1 / count=4 后 body endHM 跟 effective end sync |
| `TestSupplement_ReactivateUndismiss` | issue-25 `4c271a4` | 老 dismissed 通知 + PUT 改 window 到 active → undismiss + body sync |
| `TestSupplement_ReactivateSkipDone` | issue-25 `4c271a4` | 已勾完成的 window 不被 inner loop 自动复活(尊重用户主动 dismiss) |
| `TestSupplement_AnyModeAllDismiss` | issue-21 `dec40cb` | `completion_mode="any"` 勾任一 → 整 item 当日通知 dismiss |
| `TestSupplement_PauseSkipsActiveNotif` | (基础) | pause 后 inner loop 看 IsActive=false → 通知不复活 |

## 待补 case (后续 issue 增量)

### english (`notifications_english_test.go`)
- snooze count=1 / count=4 body sync(参照 issue-25 backend e2e)
- PUT settings windows=[] → idx 越界 dismiss(issue-24 verify)
- 阈值达 → expire 自动 dismiss
- 改 settings 让 endT 推后 → body sync 但通知保留(issue-25 revert leadStart dismiss 后)

### meal (`notifications_meal_test.go`)
- record meal → dismiss 通知;clear → undismiss
- 跨日 stale dismiss(`DismissStaleMealNotifications`)
- 完成餐 → sp window done → cancel meal → restore meal → 测试 sp 仍 done → uncheck →
  通知应正常显示(user 老 verified 路径,issue-22 cascade 修复涉及)
- meal cascade dismiss + re-set undismiss skip-done(issue-22 `0ce2423` / `30392c9`)

## 写新测试约定

1. 每个 user 报过 bug 的 issue 推进时,**fix 完同时**在对应文件加一个 `Test*` subtest,函数名带 issue ref 利于 grep
2. test func 注释里写清楚:
   - 历史 commit hash(方便 git show 看修复逻辑)
   - user 原话的复现步骤
   - 老 bug 是什么 / 修法是什么
3. 尽量用 `hmFromNow(±N)` 写相对时段 — 跑测试时 `time.Now()` 浮动,绝对 HH:MM 易过期
4. 断言用 `e.assertNotifBody` / `e.assertNotifDismissed` 这类 helper,失败信息带 ref 定位
5. test 跑完 `t.Cleanup` 自动清 db / 关 server,不污染下一 test

## 跟 CLAUDE.md 的关系

CLAUDE.md "测试沉淀" 一节是顶层入口 — 告诉 LLM "改 panel 通知 / 跨包状态机时,先看 `backend/tests/integration/README.md` 找已覆盖 case,新场景沉淀进来"。这文件是详细索引。
