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

fn xlib_parent(window: std::ffi::c_ulong) -> Result<ParentWindow> {
    u32::try_from(window)
        .map(ParentWindow::X11)
        .map_err(|error| {
            tracing::debug!(%error, "xlib window id out of range");
            Error::UnsupportedParent("xlib window id exceeds 32 bits")
        })
}

impl ParentWindow {
    /// Extracts the parent from a window's raw handle.
    ///
    /// # Errors
    /// [`Error::Handle`] when the window gives no handle and [`Error::UnsupportedParent`] for
    /// handle kinds that cannot be embedded into.
    pub fn from_window(window: &impl HasWindowHandle) -> Result<Self> {
        Self::from_raw(window.window_handle()?.as_raw())
    }

    fn from_raw(handle: RawWindowHandle) -> Result<Self> {
        match handle {
            RawWindowHandle::Xcb(handle) => Ok(Self::X11(handle.window.get())),
            RawWindowHandle::Xlib(handle) => xlib_parent(handle.window),
            RawWindowHandle::AppKit(handle) => Ok(Self::AppKit(handle.ns_view.as_ptr().addr())),
            _ => Err(Error::UnsupportedParent(
                "only X11 and AppKit windows can host web views",
            )),
        }
    }
}

#[cfg(test)]
mod tests {
    use std::num::NonZeroU32;
    use std::ptr::NonNull;

    use raw_window_handle::{
        AppKitWindowHandle, HandleError, RawWindowHandle, WaylandWindowHandle, WindowHandle,
        XcbWindowHandle, XlibWindowHandle,
    };

    use super::*;

    struct NoHandle;

    impl HasWindowHandle for NoHandle {
        fn window_handle(&self) -> std::result::Result<WindowHandle<'_>, HandleError> {
            Err(HandleError::Unavailable)
        }
    }

    type Case = (&'static str, RawWindowHandle, Option<ParentWindow>);

    fn cases() -> Vec<Case> {
        let view = NonNull::<u8>::dangling().cast();
        let surface = NonNull::<u8>::dangling().cast();
        let xcb = XcbWindowHandle::new(NonZeroU32::new(77).unwrap());
        vec![
            ("xcb", RawWindowHandle::Xcb(xcb), Some(ParentWindow::X11(77))),
            (
                "xlib",
                RawWindowHandle::Xlib(XlibWindowHandle::new(77)),
                Some(ParentWindow::X11(77)),
            ),
            (
                "appkit",
                RawWindowHandle::AppKit(AppKitWindowHandle::new(view)),
                Some(ParentWindow::AppKit(view.as_ptr().addr())),
            ),
            (
                "wayland",
                RawWindowHandle::Wayland(WaylandWindowHandle::new(surface)),
                None,
            ),
            (
                "wide xlib id",
                RawWindowHandle::Xlib(XlibWindowHandle::new(u64::MAX)),
                None,
            ),
        ]
    }

    #[test]
    fn x11_and_appkit_windows_yield_their_parent_and_other_handles_are_refused() {
        for (name, handle, want) in cases() {
            let parent = ParentWindow::from_raw(handle);
            match want {
                Some(want) => assert_eq!(parent.ok(), Some(want), "{name}"),
                None => assert!(matches!(parent, Err(Error::UnsupportedParent(_))), "{name}"),
            }
        }
        assert!(
            matches!(ParentWindow::from_window(&NoHandle), Err(Error::Handle(_))),
            "no handle"
        );
    }
}
