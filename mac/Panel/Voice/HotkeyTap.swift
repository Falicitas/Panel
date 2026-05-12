import Cocoa
import os.log

/// CGEventTap 全局 keyDown 监听 (shared 单例). 多个 controller 注册各自的 (keyCode, modifier) handler:
///   - VoiceController: F12 (录音)
///   - EnglishHotkeyController: F5/F6/F7/F8/Option+F10
///
/// callback 里按 (keyCode, modifier) 二元组查 handlers, 命中则 dispatch 到 main + 返回 nil
/// 吞掉事件 (不让其他 App 收到). tap 被 macOS 关掉 (timeout / userInput) 时自动 re-enable.
///
/// 注意: 部分 Apple 键盘上 F10/F11 等媒体键被 HID 解析成 NSSystemDefined 事件而不是
/// keyDown — 单按收不到, 必须配修饰键 (Option/Cmd/Ctrl) 让 HID 走普通 keyDown 路径.
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

    /// 关心的修饰键 mask — 屏蔽 caps-lock / numpad / function 等噪音。
    static let modifierMask: CGEventFlags = [.maskCommand, .maskAlternate, .maskControl, .maskShift]

    private static let log = OSLog(subsystem: "com.kinesis.panel", category: "voice.hotkey")

    private var eventTap: CFMachPort?
    private var runLoopSource: CFRunLoopSource?
    private var handlers: [HotkeyCombo: () -> Void] = [:]

    var isActive: Bool { eventTap != nil }

    private init() {}

    /// register/replace handler for (keyCode, modifiers) 组合. 默认 modifiers=[] = 单键.
    /// 同 combo 重复注册后者覆盖前者. 必须从 main thread 调 (handlers dict 单线程读写).
    func register(keyCode: Int64, modifiers: CGEventFlags = [], handler: @escaping () -> Void) {
        let combo = HotkeyCombo(keyCode: keyCode, modifierRaw: modifiers.intersection(Self.modifierMask).rawValue)
        handlers[combo] = handler
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
            let mods = event.flags.intersection(HotkeyTap.modifierMask)
            let combo = HotkeyCombo(keyCode: keyCode, modifierRaw: mods.rawValue)
            if let handler = me.handlers[combo] {
                os_log("hotkey keyCode=%d mods=0x%llx", log: HotkeyTap.log, type: .info, keyCode, mods.rawValue)
                DispatchQueue.main.async { handler() }
                return nil  // 吞掉, 不触发 macOS 默认行为
            }
        }
        return Unmanaged.passUnretained(event)
    }
}

/// (keyCode, modifierRaw) 二元组作为 handlers dict key. CGEventFlags 不直接 Hashable,
/// 用 rawValue UInt64 当字段。
struct HotkeyCombo: Hashable {
    let keyCode: Int64
    let modifierRaw: UInt64
}
