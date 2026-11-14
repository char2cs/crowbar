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
#[path = "parent_window_tests.rs"]
mod tests;
