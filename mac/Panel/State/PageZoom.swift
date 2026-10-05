import AppKit
import SwiftUI

enum PageZoom {
    enum Action: Equatable { case zoomIn, zoomOut, reset }
    static let minimum = 0.7
    static let maximum = 1.8

    static func normalized(_ value: Double) -> Double {
        guard value.isFinite else { return 1.0 }
        return (min(maximum, max(minimum, value)) * 10).rounded() / 10
    }

    static func applying(_ action: Action, to value: Double) -> Double {
        let current = normalized(value)
        switch action {
        case .zoomIn: return normalized(current + 0.1)
        case .zoomOut: return normalized(current - 0.1)
        case .reset: return 1.0
        }
    }

    static func action(characters: String?, modifiers: NSEvent.ModifierFlags) -> Action? {
        guard modifiers.contains(.command), modifiers.intersection([.control, .option]).isEmpty else { return nil }
        switch characters {
        // Accept both the unshifted equals key and the plus key, including a numeric keypad.
        case "=", "+": return .zoomIn
        case "-" where !modifiers.contains(.shift): return .zoomOut
        case "0" where !modifiers.contains(.shift): return .reset
        default: return nil
        }
    }
}

/// Handle zoom before WKWebView or its Markdown keymap consumes the key event.
/// The monitor belongs to this view's window and is removed when the view disappears.
struct PageZoomShortcuts: NSViewRepresentable {
    let perform: (PageZoom.Action) -> Bool

    func makeCoordinator() -> Coordinator { Coordinator(perform: perform) }

    func makeNSView(context: Context) -> NSView {
        let view = NSView(frame: .zero)
        context.coordinator.start(view: view)
        return view
    }

    func updateNSView(_ view: NSView, context: Context) {
        context.coordinator.perform = perform
    }

    static func dismantleNSView(_ view: NSView, coordinator: Coordinator) {
        coordinator.stop()
    }

    final class Coordinator {
        var perform: (PageZoom.Action) -> Bool
        private var monitor: Any?

        init(perform: @escaping (PageZoom.Action) -> Bool) { self.perform = perform }

        func start(view: NSView) {
            stop()
            monitor = NSEvent.addLocalMonitorForEvents(matching: .keyDown) { [weak self, weak view] event in
                guard let self, let window = view?.window,
                      window.isKeyWindow, event.window === window,
                      let action = PageZoom.action(characters: event.charactersIgnoringModifiers, modifiers: event.modifierFlags)
                else { return event }
                return self.perform(action) ? nil : event
            }
        }

        func stop() {
            if let monitor { NSEvent.removeMonitor(monitor) }
            monitor = nil
        }

        deinit { stop() }
    }
}
