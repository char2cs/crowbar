use objc2::MainThreadMarker;
use objc2::rc::Retained;
use objc2_app_kit::{NSWindow, NSWindowOrderingMode};
use raw_window_handle::RawWindowHandle;

use crate::error::Error;
use crate::macos::appkit_parent::AppkitParent;

/// Keeps the pop-up window that floats above a shell window over its owner, given the raw
/// handles of stock GPUI windows.
///
/// The pop-up window covers its owner's content area exactly, so what it paints lines up with
/// the owner. It is a child window of the owner, so it moves, hides and stacks with it, and it
/// lets every click through, so it never becomes the key window.
#[derive(Debug, Clone, Copy)]
pub struct FloatPlacer {
    main_thread: MainThreadMarker,
}

impl FloatPlacer {
    /// A placer for the windows of this app.
    ///
    /// # Errors
    /// [`Error::OffMainThread`] off the main thread, where no `AppKit` window may be touched.
    pub fn connect() -> Result<Self, Error> {
        let main_thread = MainThreadMarker::new().ok_or(Error::OffMainThread)?;
        Ok(Self { main_thread })
    }

    /// Moves and sizes `float` over the content area of `owner` and keeps it above it.
    ///
    /// # Errors
    /// [`Error::UnknownWindow`] for a handle that names no open `AppKit` window.
    pub fn cover(
        &self,
        float: RawWindowHandle,
        owner: RawWindowHandle,
    ) -> Result<(), Error> {
        let float = self.window_of(float)?;
        let owner = self.window_of(owner)?;
        float.setIgnoresMouseEvents(true);
        float.setLevel(owner.level());
        let content = owner.contentRectForFrameRect(owner.frame());
        float.setFrame_display(float.frameRectForContentRect(content), true);
        if !is_child_of(&float, &owner) {
            adopt(&owner, &float);
        }
        Ok(())
    }

    fn window_of(
        &self,
        handle: RawWindowHandle,
    ) -> Result<Retained<NSWindow>, Error> {
        let RawWindowHandle::AppKit(handle) = handle else {
            return Err(Error::UnknownWindow);
        };
        AppkitParent::find(handle.ns_view.as_ptr().addr(), self.main_thread)
            .and_then(|view| view.window())
            .ok_or(Error::UnknownWindow)
    }
}

fn is_child_of(
    float: &NSWindow,
    owner: &NSWindow,
) -> bool {
    float
        .parentWindow()
        .is_some_and(|parent| std::ptr::eq(Retained::as_ptr(&parent), owner))
}

#[expect(
    unsafe_code,
    reason = "NSWindow::addChildWindow_ordered is unsafe; both windows are open app windows"
)]
fn adopt(
    owner: &NSWindow,
    float: &NSWindow,
) {
    // SAFETY: both are open windows of this app, touched on the main thread; AppKit drops the bond when either closes.
    unsafe { owner.addChildWindow_ordered(float, NSWindowOrderingMode::Above) };
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_placer_cannot_start_off_the_main_thread() {
        let started = std::thread::spawn(|| FloatPlacer::connect().map(drop))
            .join()
            .unwrap();
        assert!(matches!(started, Err(Error::OffMainThread)));
    }
}
