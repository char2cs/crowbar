use std::ptr::NonNull;

use raw_window_handle::{
    AppKitWindowHandle, HandleError, HasWindowHandle, RawWindowHandle, WindowHandle,
};

/// An `NSView` that a native web view can be embedded into, presented as an `AppKit` window
/// handle.
///
/// The view must stay alive while any web view embedded through this parent exists.
#[derive(Debug, Clone, Copy)]
pub struct AppkitParent(NonNull<std::ffi::c_void>);

impl AppkitParent {
    /// The parent for the `NSView` at `view`.
    pub fn new(view: NonNull<std::ffi::c_void>) -> Self {
        Self(view)
    }
}

impl HasWindowHandle for AppkitParent {
    #[expect(
        unsafe_code,
        reason = "WindowHandle::borrow_raw is unsafe; the view outlives its web views"
    )]
    fn window_handle(&self) -> Result<WindowHandle<'_>, HandleError> {
        let handle = RawWindowHandle::AppKit(AppKitWindowHandle::new(self.0));
        // SAFETY: the NSView stays alive while any web view embedded in it exists (see the type docs).
        Ok(unsafe { WindowHandle::borrow_raw(handle) })
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn presents_the_view_as_an_appkit_window() {
        let view = NonNull::<u8>::dangling().cast();
        let handle = AppkitParent::new(view).window_handle().unwrap().as_raw();
        assert!(
            matches!(handle, RawWindowHandle::AppKit(h) if h.ns_view == view),
            "{handle:?}"
        );
    }
}
