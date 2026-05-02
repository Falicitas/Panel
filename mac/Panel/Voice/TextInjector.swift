import Cocoa

/// CGEventKeyboardSetUnicodeString 注入文字到当前光标处。
/// 5 字符一块,块间 8ms 间隔避免 iTerm2 截断。
/// 需要 Accessibility 权限 — 缺时事件被系统丢弃 (silent fail)。
enum TextInjector {
    private static let chunkSize = 5
    private static let chunkIntervalMicroSec: UInt32 = 8000

    static func inject(_ text: String) {
        guard !text.isEmpty else { return }
        for chunk in chunked(text, size: chunkSize) {
            postChunk(chunk)
            usleep(chunkIntervalMicroSec)
        }
    }

    private static func postChunk(_ chunk: String) {
        let utf16 = Array(chunk.utf16)
        guard let down = CGEvent(keyboardEventSource: nil, virtualKey: 0, keyDown: true) else { return }
        utf16.withUnsafeBufferPointer { buf in
            down.keyboardSetUnicodeString(stringLength: buf.count, unicodeString: buf.baseAddress)
        }
        down.post(tap: .cgSessionEventTap)
        // SetUnicodeString 路径下 keyUp 不必要 (系统按 unicode 字符串单事件处理)
    }

    private static func chunked(_ s: String, size: Int) -> [String] {
        var result: [String] = []
        var i = s.startIndex
        while i < s.endIndex {
            let j = s.index(i, offsetBy: size, limitedBy: s.endIndex) ?? s.endIndex
            result.append(String(s[i..<j]))
            i = j
        }
        return result
    }
}
