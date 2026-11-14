use raw_window_handle::{
    HandleError, HasWindowHandle, RawWindowHandle, WindowHandle, XlibWindowHandle,
};

/// An X11 window id presented as an Xlib window.
///
/// GPUI hands out an XCB handle, wry's X11 backend only accepts Xlib. X window ids are
/// server-wide, so the same id names the same window on both connections; wry opens its own
/// display through GDK and never uses a connection from this handle.
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
