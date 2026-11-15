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
#[path = "appkit_parent_tests.rs"]
mod tests;
