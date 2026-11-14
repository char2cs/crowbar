use std::num::{NonZeroIsize, NonZeroU32};
use std::ptr::NonNull;

use raw_window_handle::{
    AppKitWindowHandle, HandleError, RawWindowHandle, Win32WindowHandle, WindowHandle,
    XcbWindowHandle, XlibWindowHandle,
};

use super::*;

struct Fake(Option<RawWindowHandle>);

impl HasWindowHandle for Fake {
    #[allow(
        unsafe_code,
        reason = "borrow_raw needs unsafe; the test handle is never dereferenced"
    )]
    fn window_handle(&self) -> std::result::Result<WindowHandle<'_>, HandleError> {
        let raw = self.0.ok_or(HandleError::Unavailable)?;
        // SAFETY: the raw handle is only inspected by `from_window`, never dereferenced.
        Ok(unsafe { WindowHandle::borrow_raw(raw) })
    }
}

#[test]
fn xcb_and_xlib_windows_yield_the_same_x11_id() {
    let xcb = RawWindowHandle::Xcb(XcbWindowHandle::new(NonZeroU32::new(77).unwrap()));
    let xlib = RawWindowHandle::Xlib(XlibWindowHandle::new(77));
    assert_eq!(
        ParentWindow::from_window(&Fake(Some(xcb))).unwrap(),
        ParentWindow::X11(77),
        "xcb"
    );
    assert_eq!(
        ParentWindow::from_window(&Fake(Some(xlib))).unwrap(),
        ParentWindow::X11(77),
        "xlib"
    );
}

#[test]
fn appkit_views_yield_their_address() {
    let view = NonNull::<u8>::dangling().cast();
    let handle = RawWindowHandle::AppKit(AppKitWindowHandle::new(view));
    assert_eq!(
        ParentWindow::from_window(&Fake(Some(handle))).unwrap(),
        ParentWindow::AppKit(view.as_ptr().addr()),
        "appkit"
    );
}

#[test]
fn other_handles_and_missing_handles_are_errors() {
    let win32 = RawWindowHandle::Win32(Win32WindowHandle::new(NonZeroIsize::new(1).unwrap()));
    assert!(
        matches!(
            ParentWindow::from_window(&Fake(Some(win32))),
            Err(Error::UnsupportedParent(_))
        ),
        "win32"
    );
    assert!(
        matches!(
            ParentWindow::from_window(&Fake(None)),
            Err(Error::Handle(_))
        ),
        "none"
    );
    let wide = RawWindowHandle::Xlib(XlibWindowHandle::new(u64::MAX));
    assert!(
        matches!(
            ParentWindow::from_window(&Fake(Some(wide))),
            Err(Error::UnsupportedParent(_))
        ),
        "wide xlib id"
    );
}
