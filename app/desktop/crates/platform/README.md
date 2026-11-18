# platform

Layer L0 (logic, OS layer). See `docs/plans/2026-11-18-rust-tree.md` section 3.

## Owns

Where a link may go (stay in the app, the system browser, or refused); launching the system browser
and the file manager; the native parent windows that web views are embedded into; and the switch
that makes the shell an X11 client. It is the one crate of the desktop app that holds `unsafe`
code, one minimal layer per OS: `src/linux/` and `src/macos/`.

## Public interface

- `open_external(link)`: opens a web, mail or phone link in the system's default application.
- `follow_link(link, dev_origin)`: decides a web view's navigation request (load it, hand it to the
  system browser, or refuse it).
- `open_path(path)`: opens a file or folder with the desktop's default application.
- `reveal_in_file_manager(path)`: shows a path in the file manager.
- `Error`: why a launch was refused or could not start.
- `XlibParent` (Linux): an X11 window id presented as an Xlib window handle.
- `force_x11()` (Linux): drops `WAYLAND_DISPLAY` and sets `GDK_BACKEND=x11`; `main` calls it first.
- `AppkitParent` (macOS): an `NSView` of an open window, held by a strong reference, presented as
  an `AppKit` window handle. `AppkitParent::find(address, main_thread)` looks the view up among the
  app's windows, so an address no window shows never becomes a parent.
- `classify`, `LinkTarget`, `Launch`, `Desktop`: still public today; only this crate uses them
  (they become `pub(crate)` in the narrowing step).

## Depends on

No workspace crate. External: `url` (link parsing), `raw-window-handle` (the handle types web views
are built with), `thiserror`, `tracing`; on macOS `objc2`, `objc2-app-kit` and `objc2-foundation`
(the retained `NSView` and the window lookup).

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
