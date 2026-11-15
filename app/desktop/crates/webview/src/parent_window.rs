use raw_window_handle::{HasWindowHandle, RawWindowHandle};

use crate::error::{Error, Result};

/// The native window web views are embedded into, reduced to plain numbers so it can cross to
/// the platform thread.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ParentWindow {
    /// An X11 window id. GPUI reports it through an XCB handle; the id is the same for Xlib.
    X11(u32),
    /// The address of an `NSView`.
    AppKit(usize),
}

impl ParentWindow {
    /// Extracts the parent from a window's raw handle.
    ///
    /// # Errors
    /// [`Error::Handle`] when the window gives no handle and [`Error::UnsupportedParent`] for
    /// handle kinds that cannot be embedded into.
    pub fn from_window(window: &impl HasWindowHandle) -> Result<Self> {
        match window.window_handle()?.as_raw() {
            RawWindowHandle::Xcb(handle) => Ok(Self::X11(handle.window.get())),
            RawWindowHandle::Xlib(handle) => u32::try_from(handle.window)
                .map(Self::X11)
                .map_err(|_| Error::UnsupportedParent("xlib window id exceeds 32 bits")),
            RawWindowHandle::AppKit(handle) => Ok(Self::AppKit(handle.ns_view.as_ptr().addr())),
            _ => Err(Error::UnsupportedParent(
                "only X11 and AppKit windows can host web views",
            )),
        }
    }
}

#[cfg(test)]
mod tests {
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

    type Case = (&'static str, RawWindowHandle, ParentWindow);

    fn cases() -> Vec<Case> {
        let view = NonNull::<u8>::dangling().cast();
        vec![
            (
                "xcb",
                RawWindowHandle::Xcb(XcbWindowHandle::new(NonZeroU32::new(77).unwrap())),
                ParentWindow::X11(77),
            ),
            (
                "xlib",
                RawWindowHandle::Xlib(XlibWindowHandle::new(77)),
                ParentWindow::X11(77),
            ),
            (
                "appkit",
                RawWindowHandle::AppKit(AppKitWindowHandle::new(view)),
                ParentWindow::AppKit(view.as_ptr().addr()),
            ),
        ]
    }

    #[test]
    fn supported_windows_yield_their_parent() {
        let wrong: Vec<&str> = cases()
            .into_iter()
            .filter(|(_, handle, want)| {
                ParentWindow::from_window(&Fake(Some(*handle)))
                    .ok()
                    .as_ref()
                    != Some(want)
            })
            .map(|(name, ..)| name)
            .collect();
        assert!(wrong.is_empty(), "failed cases: {wrong:?}");
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
}
