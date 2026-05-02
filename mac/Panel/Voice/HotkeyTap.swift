import Cocoa
import os.log

/// CGEventTap 监听全局 F12 keyDown (kVK_F12 = 111)。
/// callback 返回 nil 吞掉事件,不让其他 App 收到 (避免触发 macOS 默认 F12 行为)。
/// tap 被 macOS 关掉 (timeout / userInput) 时自动 re-enable。
///
/// 需要 Input Monitoring 权限 (不是 Accessibility) — 见 Permissions.swift 注释。
final class HotkeyTap {
    static let f12KeyCode: Int64 = 111
    private static let log = OSLog(subsystem: "com.kinesis.panel", category: "voice.hotkey")

    private var eventTap: CFMachPort?
    private var runLoopSource: CFRunLoopSource?
    private let onF12: () -> Void

    var isActive: Bool { eventTap != nil }

    init(onF12: @escaping () -> Void) {
        self.onF12 = onF12
    }

    func start() {
        guard eventTap == nil else { return }
        let mask: CGEventMask = (1 << CGEventType.keyDown.rawValue)
                              | (1 << CGEventType.tapDisabledByTimeout.rawValue)
                              | (1 << CGEventType.tapDisabledByUserInput.rawValue)
        let userInfo = Unmanaged.passUnretained(self).toOpaque()
        guard let tap = CGEvent.tapCreate(
            tap: .cgSessionEventTap,
            place: .headInsertEventTap,
            options: .defaultTap,
            eventsOfInterest: mask,
            callback: Self.callback,
            userInfo: userInfo
        ) else {
            os_log("tapCreate 返回 nil — 通常是 Input Monitoring 未授权 (或 ad-hoc 签名 trust 失效)", log: Self.log, type: .error)
            return
        }
        let src = CFMachPortCreateRunLoopSource(kCFAllocatorDefault, tap, 0)
        CFRunLoopAddSource(CFRunLoopGetCurrent(), src, .commonModes)
        CGEvent.tapEnable(tap: tap, enable: true)
        self.eventTap = tap
        self.runLoopSource = src
        os_log("tap installed, F12 监听中", log: Self.log, type: .info)
    }

    func stop() {
        if let tap = eventTap {
            CGEvent.tapEnable(tap: tap, enable: false)
        }
        if let src = runLoopSource {
            CFRunLoopRemoveSource(CFRunLoopGetCurrent(), src, .commonModes)
        }
        eventTap = nil
        runLoopSource = nil
    }

    private static let callback: CGEventTapCallBack = { _, type, event, info in
        guard let info else { return Unmanaged.passUnretained(event) }
        let me = Unmanaged<HotkeyTap>.fromOpaque(info).takeUnretainedValue()
        if type == .tapDisabledByTimeout || type == .tapDisabledByUserInput {
            os_log("tap disabled (rawValue=%d), re-enable", log: HotkeyTap.log, type: .info, type.rawValue)
            if let tap = me.eventTap {
                CGEvent.tapEnable(tap: tap, enable: true)
            }
            return Unmanaged.passUnretained(event)
        }
        if type == .keyDown {
            let keyCode = event.getIntegerValueField(.keyboardEventKeycode)
            if keyCode == HotkeyTap.f12KeyCode {
                os_log("F12 received", log: HotkeyTap.log, type: .info)
                DispatchQueue.main.async { me.onF12() }
                return nil  // 吞掉 F12
            }
        }
        return Unmanaged.passUnretained(event)
    }
}
