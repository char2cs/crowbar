# Overlays above native web views (GPUI shell)

## Problem
Web views are native child views painting above GPUI's surface, so a GPUI menu, popover,
palette or drag ghost is covered by them. `PaneHost::freeze` therefore hides every web view
and `FrozenLayer` paints a pixel snapshot where each view was. On macOS
`impl ViewCapture for MacosPlatform {}` (`crates/webview/src/macos_platform.rs`) is the empty
default: `snapshot_all` answers `None`, so `FrozenLayer::replace` draws nothing and every
chat and browser pane reads as vanished for as long as any overlay is up. Linux captures;
macOS does not. Tauri never hides anything because its overlays are DOM.

## Options
1. **Snapshot on macOS** (`WKWebView takeSnapshot`, add `objc2-web-kit` + `block2`).
   Smallest change, keeps the model. Costs: the completion handler runs on the main thread,
   so `snapshot_all` must spin the run loop until all handlers answer (bounded deadline);
   the picture is static (a streaming chat or video freezes under a menu); capture lag on
   every overlay open.
2. **Overlay in a native child window above the web views.** Overlay content renders in a
   borderless child `NSWindow` (a second GPUI window) ordered above the main window. Web
   views stay shown and live; nothing is hidden, frozen, or captured, which matches Tauri
   ("nothing is ever hidden"). Costs: a second window to position, key/focus routing between
   the two, menus clipped to nothing outside the parent, and the shell's overlay state
   (`overlays_open` and the dialogs in `shell_view.rs::overlay`) moves into that window.

## Recommendation
Option 2 is the only one with Tauri's behaviour (live panes under overlays, no flash).
Option 1 is an acceptable interim only if it is verified in the running app, since a
snapshot that is wrong or late reads as a disappearing pane again.

## Open questions
- Does GPUI on macOS support a child window sharing the parent's event loop and theme?
- Native OS context menus (already NSMenu) need no change; only GPUI-drawn overlays move.
- Drag ghosts: a window that follows the pointer, or keep freeze for drags only.

## Verification required (not done)
Open each overlay kind over a chat, a browser page and a terminal in the running app and
confirm no pane blanks, flickers, or loses focus and scroll.
