use std::ffi::c_void;
use std::ptr::NonNull;

use objc2::MainThreadMarker;
use objc2::rc::Retained;
use objc2_app_kit::{NSApplication, NSView};
use raw_window_handle::{
    AppKitWindowHandle, HandleError, HasWindowHandle, RawWindowHandle, WindowHandle,
};

/// An `NSView` of one of the app's open windows that a native web view can be embedded into,
/// presented as an `AppKit` window handle.
///
/// It holds a strong reference to the view, so the view outlives every handle borrowed from it.
#[derive(Debug, Clone)]
pub struct AppkitParent(Retained<NSView>);

impl AppkitParent {
    /// Finds the view at `address` among the views of the app's open windows.
    ///
    /// Returns `None` when no open window shows a view at that address, so a stale or made-up
    /// address never becomes a parent.
    pub fn find(
        address: usize,
        main_thread: MainThreadMarker,
    ) -> Option<Self> {
        NSApplication::sharedApplication(main_thread)
            .windows()
            .to_vec()
            .into_iter()
            .filter_map(|window| window.contentView())
            .find_map(|content| view_at(content, address))
            .map(Self)
    }
}

fn view_at(
    view: Retained<NSView>,
    address: usize,
) -> Option<Retained<NSView>> {
    if Retained::as_ptr(&view).addr() == address {
        return Some(view);
    }
    view.subviews()
        .to_vec()
        .into_iter()
        .find_map(|child| view_at(child, address))
}

impl HasWindowHandle for AppkitParent {
    #[expect(
        unsafe_code,
        reason = "WindowHandle::borrow_raw is unsafe; the parent holds the view alive"
    )]
    fn window_handle(&self) -> Result<WindowHandle<'_>, HandleError> {
        let view = NonNull::from(&*self.0).cast::<c_void>();
        let handle = RawWindowHandle::AppKit(AppKitWindowHandle::new(view));
        // SAFETY: `self` holds a strong reference to the view for as long as the handle borrows it.
        Ok(unsafe { WindowHandle::borrow_raw(handle) })
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    #[ignore = "real-infra: AppKit views exist only on the main thread (--test-threads=1)"]
    fn presents_its_view_as_an_appkit_window() {
        let main_thread = MainThreadMarker::new().unwrap();
        let parent = AppkitParent(NSView::new(main_thread));
        let view = Retained::as_ptr(&parent.0).addr();
        let handle = parent.window_handle().unwrap().as_raw();
        assert!(
            matches!(handle, RawWindowHandle::AppKit(h) if h.ns_view.as_ptr().addr() == view),
            "{handle:?}"
        );
    }

    #[test]
    #[ignore = "real-infra: AppKit views exist only on the main thread (--test-threads=1)"]
    fn a_view_no_open_window_shows_is_not_found() {
        let main_thread = MainThreadMarker::new().unwrap();
        let loose = NSView::new(main_thread);
        let address = Retained::as_ptr(&loose).addr();
        assert!(
            AppkitParent::find(address, main_thread).is_none(),
            "a view outside every window"
        );
    }
}
