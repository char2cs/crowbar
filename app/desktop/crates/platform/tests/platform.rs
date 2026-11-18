//! `AppkitParent` against real `AppKit` views and windows.
//!
//! `AppKit` views exist only on the main thread and libtest runs every test on a spawned one, so
//! this target has no harness and `main` runs the cases. Like a `real-infra` test it runs only
//! under `--ignored` or `--include-ignored`, and it is empty off macOS.

#[cfg(target_os = "macos")]
fn main() -> Result<(), String> {
    if std::env::args().any(|arg| arg == "--ignored" || arg == "--include-ignored") {
        return appkit::run();
    }
    Ok(())
}

#[cfg(not(target_os = "macos"))]
fn main() {}

#[cfg(target_os = "macos")]
mod appkit {
    use objc2::MainThreadMarker;
    use objc2::rc::Retained;
    use objc2_app_kit::{NSApplication, NSView, NSViewController, NSWindow};
    use platform::AppkitParent;
    use raw_window_handle::{HasWindowHandle, RawWindowHandle};

    /// Runs every case on the main thread; the first failure is the error `main` exits with.
    pub(crate) fn run() -> Result<(), String> {
        let main_thread =
            MainThreadMarker::new().ok_or("the AppKit cases must run on the main thread")?;
        finds_a_nested_view_of_an_open_window_and_presents_it(main_thread)?;
        a_view_no_open_window_shows_is_not_found(main_thread)
    }

    fn finds_a_nested_view_of_an_open_window_and_presents_it(
        main_thread: MainThreadMarker
    ) -> Result<(), String> {
        let _app = NSApplication::sharedApplication(main_thread);
        let content = NSView::new(main_thread);
        let nested = NSView::new(main_thread);
        content.addSubview(&nested);
        let controller = NSViewController::new(main_thread);
        controller.setView(&content);
        let _window = NSWindow::windowWithContentViewController(&controller);
        let address = Retained::as_ptr(&nested).addr();

        let parent = AppkitParent::find(address, main_thread)
            .ok_or("the nested view of an open window was not found")?;
        let handle = parent
            .window_handle()
            .map_err(|error| format!("the parent has no window handle: {error}"))?
            .as_raw();
        match handle {
            RawWindowHandle::AppKit(appkit) if appkit.ns_view.as_ptr().addr() == address => Ok(()),
            other => Err(format!(
                "expected the nested view as an AppKit handle, got {other:?}"
            )),
        }
    }

    fn a_view_no_open_window_shows_is_not_found(
        main_thread: MainThreadMarker
    ) -> Result<(), String> {
        let loose = NSView::new(main_thread);
        let address = Retained::as_ptr(&loose).addr();
        match AppkitParent::find(address, main_thread) {
            None => Ok(()),
            Some(parent) => Err(format!("a view outside every window was found: {parent:?}")),
        }
    }
}
