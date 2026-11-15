use std::ptr::NonNull;

use raw_window_handle::{
    AppKitWindowHandle, HandleError, HasWindowHandle, RawWindowHandle, WindowHandle,
};

#[derive(Debug, Clone, Copy)]
pub(crate) struct AppkitParent(pub(crate) NonNull<std::ffi::c_void>);

impl HasWindowHandle for AppkitParent {
    #[allow(
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
        let handle = AppkitParent(view).window_handle().unwrap().as_raw();
        assert!(
            matches!(handle, RawWindowHandle::AppKit(h) if h.ns_view == view),
            "{handle:?}"
        );
    }
}
