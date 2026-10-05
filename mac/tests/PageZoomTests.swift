import AppKit

@main
struct PageZoomTests {
    static func main() {
        // Common keyboard layouts put plus on Shift+Equals; keypad plus has no Shift.
        expect(PageZoom.action(characters: "=", modifiers: .command) == .zoomIn, "Command+Equals")
        expect(PageZoom.action(characters: "+", modifiers: [.command, .shift]) == .zoomIn, "Command+Shift+Plus")
        expect(PageZoom.action(characters: "+", modifiers: [.command, .numericPad]) == .zoomIn, "Keypad plus")
        expect(PageZoom.action(characters: "-", modifiers: .command) == .zoomOut, "Command+Minus")
        expect(PageZoom.action(characters: "0", modifiers: .command) == .reset, "Command+Zero")
        for key in ["=", "+", "-", "0"] {
            expect(PageZoom.action(characters: key, modifiers: []) == nil, "Plain typing must pass through")
            expect(PageZoom.action(characters: key, modifiers: .control) == nil, "Control is not Command")
            expect(PageZoom.action(characters: key, modifiers: [.command, .option]) == nil, "Keep Option shortcuts")
            expect(PageZoom.action(characters: key, modifiers: [.command, .control]) == nil, "Keep Control shortcuts")
        }
        expect(PageZoom.action(characters: "0", modifiers: [.command, .shift]) == nil, "Keep shifted zero")
        expect(PageZoom.action(characters: "-", modifiers: [.command, .shift]) == nil, "Keep shifted minus")
        expect(PageZoom.action(characters: "f", modifiers: .command) == nil, "Keep find shortcut")
        expect(PageZoom.action(characters: nil, modifiers: .command) == nil, "Non-character key")

        var zoom = 1.0
        for _ in 0..<30 { zoom = PageZoom.applying(.zoomIn, to: zoom) }
        expect(zoom == 1.8, "Repeated zoom in stops exactly at upper bound")
        for _ in 0..<30 { zoom = PageZoom.applying(.zoomOut, to: zoom) }
        expect(zoom == 0.7, "Repeated zoom out stops exactly at lower bound")
        expect(PageZoom.applying(.reset, to: zoom) == 1.0, "Reset returns to 100 percent")
        for value in [0.7, 1.0, 1.5, 1.7] {
            expect(PageZoom.applying(.zoomOut, to: PageZoom.applying(.zoomIn, to: value)) == value,
                   "Zoom step round-trip must not accumulate rounding errors")
        }
        expect(PageZoom.normalized(.nan) == 1.0, "Invalid saved zoom falls back")
        expect(PageZoom.normalized(.infinity) == 1.0, "Infinite saved zoom falls back")
        expect(PageZoom.normalized(-10) == 0.7, "Low saved zoom is safe")
        expect(PageZoom.normalized(100) == 1.8, "High saved zoom is safe")
        expect(PageZoom.normalized(0.8999999999) == 0.9, "Old floating point values normalize")
        print("Page zoom shortcut and boundary checks passed")
    }

    private static func expect(_ condition: @autoclosure () -> Bool, _ message: String) {
        precondition(condition(), message)
    }
}
