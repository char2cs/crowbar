//! `AppkitParent`, `FloatPlacer` and `PageHole` against real `AppKit` views and windows.
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
    use objc2::rc::Retained;
    use objc2::{ClassType as _, MainThreadMarker};
    use objc2_app_kit::{NSApplication, NSView, NSViewController, NSWindow};
    use objc2_foundation::{NSObjectProtocol as _, NSPoint, NSRect, NSSize};
    use platform::{AppkitParent, FloatPlacer, PageHole};
    use raw_window_handle::{HasWindowHandle, RawWindowHandle};

    const SIZE: (f64, f64) = (400.0, 300.0);

    /// Runs every case on the main thread; the first failure is the error `main` exits with.
    pub(crate) fn run() -> Result<(), String> {
        let main_thread =
            MainThreadMarker::new().ok_or("the AppKit cases must run on the main thread")?;
        finds_a_nested_view_of_an_open_window_and_presents_it(main_thread)?;
        a_view_no_open_window_shows_is_not_found(main_thread)?;
        a_pop_up_window_covers_its_owner_as_a_child_that_lets_clicks_through(main_thread)?;
        a_click_in_the_hole_reaches_the_shell_view_and_one_beside_it_the_page(main_thread)
    }

    fn window_showing(
        content: &NSView,
        main_thread: MainThreadMarker,
    ) -> Retained<NSWindow> {
        let _app = NSApplication::sharedApplication(main_thread);
        content.setFrame(NSRect::new(
            NSPoint::new(0.0, 0.0),
            NSSize::new(SIZE.0, SIZE.1),
        ));
        let controller = NSViewController::new(main_thread);
        controller.setView(content);
        NSWindow::windowWithContentViewController(&controller)
    }

    fn raw_handle(
        view: &NSView,
        main_thread: MainThreadMarker,
    ) -> Result<RawWindowHandle, String> {
        let address = std::ptr::from_ref(view).addr();
        let parent = AppkitParent::find(address, main_thread).ok_or("the view is in no window")?;
        let handle = parent
            .window_handle()
            .map_err(|error| format!("the view has no window handle: {error}"))?;
        Ok(handle.as_raw())
    }

    fn a_pop_up_window_covers_its_owner_as_a_child_that_lets_clicks_through(
        main_thread: MainThreadMarker
    ) -> Result<(), String> {
        let (owner_view, float_view) = (NSView::new(main_thread), NSView::new(main_thread));
        let owner = window_showing(&owner_view, main_thread);
        let float = window_showing(&float_view, main_thread);
        let placer = FloatPlacer::connect().map_err(|error| error.to_string())?;
        let (float_raw, owner_raw) = (
            raw_handle(&float_view, main_thread)?,
            raw_handle(&owner_view, main_thread)?,
        );
        placer
            .cover(float_raw, owner_raw)
            .map_err(|error| error.to_string())?;
        let adopted = float
            .parentWindow()
            .is_some_and(|parent| Retained::as_ptr(&parent) == Retained::as_ptr(&owner));
        let covered = float.contentRectForFrameRect(float.frame())
            == owner.contentRectForFrameRect(owner.frame());
        match (adopted, covered, float.ignoresMouseEvents()) {
            (true, true, true) => Ok(()),
            seen => Err(format!(
                "want (child, covering, click-through), got {seen:?}"
            )),
        }
    }

    fn a_click_in_the_hole_reaches_the_shell_view_and_one_beside_it_the_page(
        main_thread: MainThreadMarker
    ) -> Result<(), String> {
        let shell = NSView::new(main_thread);
        let _window = window_showing(&shell, main_thread);
        let page = NSView::new(main_thread);
        page.setFrame(shell.bounds());
        shell.addSubview(&page);
        let top = shell.bounds().size.height;
        let (in_hole, beside) = (
            NSPoint::new(130.0, top - 10.0),
            NSPoint::new(300.0, top / 2.0),
        );
        let hole = NSRect::new(NSPoint::new(100.0, 0.0), NSSize::new(60.0, 20.0));
        let cut = PageHole::cut(&page, hole, 8.0).ok_or("the hole was not cut")?;
        let caught = shell.hitTest(in_hole).ok_or("nothing under the hole")?;
        let plain = caught.isMemberOfClass(NSView::class()) && !is(&caught, &page);
        let masked = page.layer().and_then(|layer| layer.mask()).is_some();
        let page_beside = shell.hitTest(beside).is_some_and(|view| is(&view, &page));
        drop(cut);
        let mended = shell.hitTest(in_hole).is_some_and(|view| is(&view, &page));
        let unmasked = page.layer().and_then(|layer| layer.mask()).is_none();
        match (plain, masked, page_beside, mended, unmasked) {
            (true, true, true, true, true) => Ok(()),
            seen => Err(format!(
                "want (plain catcher, masked, page beside, page again, unmasked), got {seen:?}"
            )),
        }
    }

    fn is(
        view: &NSView,
        other: &NSView,
    ) -> bool {
        std::ptr::eq(view, other)
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
