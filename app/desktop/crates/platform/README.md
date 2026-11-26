# platform

Layer L0 (logic, OS layer). See `docs/plans/2026-11-18-rust-tree.md` section 3.

## Owns

Where a link may go (stay in the app, the system browser, or refused); launching the system browser
and the file manager; the native parent windows that web views are embedded into; the X keyboard
focus of web views; the placing and shaping of the shell's float windows; the holes cut out of web
views; and the switch that makes the shell an X11 client. It is the one crate of the
desktop app that holds `unsafe` code, one minimal layer per OS: `src/linux/` and `src/macos/`.

## Public interface

- `open_external(link)`: opens a web, mail or phone link in the system's default application.
- `follow_link(link, dev_origin)`: decides a web view's navigation request (load it, hand it to the
  system browser, or refuse it).
- `open_path(path)`: opens a file or folder with the desktop's default application.
- `reveal_in_file_manager(path)`: shows a path in the file manager.
- `Error`: why a launch was refused or could not start.
- `XlibParent` (Linux): an X11 window id presented as an Xlib window handle.
- `force_x11()` (Linux): drops `WAYLAND_DISPLAY` and sets `GDK_BACKEND=x11`; `main` calls it first.
- `InputFocus` (Linux): a connection of its own to the X server that gives the keyboard to a window
  (`give_to`), so a web view takes keys wherever the pointer is; `next_change` waits for a followed
  window to gain or lose it.
- `FloatPlacer` (Linux): a connection of its own to the X server that keeps a float window over its
  owner (`cover`), off the taskbar (`pin`), clipped to its cards (`shape`: the input shape always,
  the bounding shape only without a compositor) and focused (`focus`). `Card` is one rounded
  rectangle of a shape, in the float window's pixels.
- `XdndProxy` (Linux): the `XdndProxy` of a window, on a thread of its own, which hands drags from
  other applications to the web view under the pointer and its answers back to the source.
- `AppkitParent` (macOS): an `NSView` of an open window, held by a strong reference, presented as
  an `AppKit` window handle. `AppkitParent::find(address, main_thread)` looks the view up among the
  app's windows, so an address no window shows never becomes a parent.
- `FloatPlacer` (macOS): keeps the pop-up window over its owner's content area (`cover`) as a
  child window at the owner's level that lets every click through, so it never becomes key.
- `PageHole` (macOS): the pane handle's hole in a web view: the page's layer masked there and a
  plain view above the page whose mouse events `AppKit` hands to the shell view under it; dropping
  it makes the page whole, `follow` lays it again after the page moved.

## Depends on

No workspace crate. External: `url` (link parsing), `raw-window-handle` (the handle types web views
are built with), `thiserror`, `tracing`; on Linux `x11rb` with its `shape` extension (the X keyboard focus and the float windows, safe Rust); on macOS `objc2`, `objc2-app-kit`, `objc2-foundation`,
`objc2-quartz-core` and `objc2-core-graphics` (the retained `NSView`, the window lookup, the pop-up
window and the hole's mask layer).

## Unsafe

Every `unsafe` block of the desktop app. Each function that holds one carries
`#[expect(unsafe_code, reason = "...")]`, each block does one operation and has a `// SAFETY:`
line. The rules gate (`unsafe-home`) rejects `unsafe` anywhere else in `app/`, except the iOS
plugin's C ABI file `app/mobile/ios-plugin/src/ffi.rs`.

| Block | File | Safe API | SAFETY argument | Test |
|---|---|---|---|---|
| `std::env::remove_var(key)` | `src/linux/env_change.rs` (`EnvChange::apply`) | `force_x11()` | The shell calls `force_x11` first in `main`, before it starts any other thread, so no thread reads the environment concurrently. | `env_change::tests::a_change_sets_and_removes_a_variable`, `x11_session::tests::the_session_drops_wayland_and_pins_gtk_to_x11` |
| `std::env::set_var(key, value)` | `src/linux/env_change.rs` (`EnvChange::apply`) | `force_x11()` | As above. | As above. |
| `WindowHandle::borrow_raw(Xlib)` | `src/linux/xlib_parent.rs` (`XlibParent::window_handle`) | `XlibParent` | An Xlib window handle holds only the window id, a plain number; it borrows no resource that could dangle. | `xlib_parent::tests::presents_the_id_as_an_xlib_window` |
| `WindowHandle::borrow_raw(AppKit)` | `src/macos/appkit_parent.rs` (`AppkitParent::window_handle`) | `AppkitParent` | The parent holds a `Retained<NSView>`, a strong reference, for as long as the handle borrows it, so the view cannot be freed under the handle. | `tests/platform.rs` (macOS, `real-infra`: `NSView` needs the main thread, so the target has no harness and its `main` runs the cases under `--ignored` or `--include-ignored`) |
| `NSWindow::addChildWindow_ordered(float, Above)` | `src/macos/float_placer.rs` (`adopt`) | `FloatPlacer::cover` | Both are open windows of the app, found by their views among `NSApplication::windows` on the main thread; `AppKit` drops the bond when either closes. | `tests/platform.rs` (macOS, `a_pop_up_window_covers_its_owner_as_a_child_that_lets_clicks_through`) |
| `CALayer::setMask(mask)` | `src/macos/page_hole.rs` (`set_mask`) | `PageHole::cut`, `Drop for PageHole` | The mask is a fresh layer of the hole's own, in no layer tree and masking no other layer, as `setMask` requires. | `tests/platform.rs` (macOS, `a_click_in_the_hole_reaches_the_shell_view_and_one_beside_it_the_page`) |
