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
mod tests {
    use super::*;

    #[test]
    fn presents_the_id_as_an_xlib_window() {
        let handle = XlibParent(0x42).window_handle().unwrap().as_raw();
        assert!(
            matches!(handle, RawWindowHandle::Xlib(h) if h.window == 0x42),
            "{handle:?}"
        );
    }
}
