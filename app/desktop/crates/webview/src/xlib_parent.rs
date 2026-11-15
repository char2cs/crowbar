use raw_window_handle::{
    HandleError, HasWindowHandle, RawWindowHandle, WindowHandle, XlibWindowHandle,
};

#[derive(Debug, Clone, Copy)]
pub(crate) struct XlibParent(pub(crate) u32);

impl HasWindowHandle for XlibParent {
    #[allow(
        unsafe_code,
        reason = "WindowHandle::borrow_raw is unsafe; the id is a plain value"
    )]
    fn window_handle(&self) -> Result<WindowHandle<'_>, HandleError> {
        let handle = RawWindowHandle::Xlib(XlibWindowHandle::new(self.0.into()));
        // SAFETY: an Xlib window handle holds only the window id, no borrowed resource that could dangle.
        Ok(unsafe { WindowHandle::borrow_raw(handle) })
    }
}

#[cfg(test)]
#[path = "xlib_parent_tests.rs"]
mod tests;
