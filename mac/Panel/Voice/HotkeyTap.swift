import Cocoa
import os.log

/// CGEventTap 全局 keyDown 监听 (shared 单例). 多个 controller 注册各自的 keyCode handler:
///   - VoiceController: F12 (录音)
///   - EnglishHotkeyController: F5 (回退 5s) / F6 (toggle play/pause)
///
/// callback 里查 handlers dict, 命中则 dispatch 到 main + 返回 nil 吞掉事件 (不让其他
/// App 收到, 避免触发 macOS 默认行为). tap 被 macOS 关掉 (timeout / userInput) 时自动 re-enable.
///
/// 需要 Input Monitoring 权限 (不是 Accessibility) — 见 Permissions.swift 注释.
final class HotkeyTap {
    static let shared = HotkeyTap()

    static let f5KeyCode: Int64 = 96
    static let f6KeyCode: Int64 = 97
    static let f7KeyCode: Int64 = 98
    static let f8KeyCode: Int64 = 100
    static let f10KeyCode: Int64 = 109
    static let f12KeyCode: Int64 = 111

    private static let log = OSLog(subsystem: "com.kinesis.panel", category: "voice.hotkey")

    private var eventTap: CFMachPort?
    private var runLoopSource: CFRunLoopSource?
    private var handlers: [Int64: () -> Void] = [:]

    var isActive: Bool { eventTap != nil }

    private init() {}

    /// register/replace handler for keyCode. 同 keyCode 重复注册后者覆盖前者.
    /// 必须从 main thread 调 (handlers dict 单线程读写).
    func register(keyCode: Int64, handler: @escaping () -> Void) {
        handlers[keyCode] = handler
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
        os_log("tap installed, 监听 keyDown (handlers=%d)", log: Self.log, type: .info, handlers.count)
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
            if let handler = me.handlers[keyCode] {
                os_log("hotkey keyCode=%d", log: HotkeyTap.log, type: .info, keyCode)
                DispatchQueue.main.async { handler() }
                return nil  // 吞掉, 不触发 macOS 默认行为
            }
        }
        return Unmanaged.passUnretained(event)
    }
}
