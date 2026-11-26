//! The pane handle's hole in a `WKWebView`: its layer masked there and the pointer handed to
//! the shell view.

use objc2_foundation::{NSPoint, NSRect, NSSize};
use platform::PageHole;
use wry::WebViewExtMacOS as _;

use crate::backend::native::macos::appkit_platform::AppkitPlatform;
use crate::backend::view::view_cutout::ViewCutout;
use crate::error::{Error, Result};
use crate::view::Bounds;
use crate::view::Id;

const CUT_RADIUS: f64 = 8.0;

impl ViewCutout for AppkitPlatform {
    fn set_cutout(
        &self,
        id: Id,
        hole: Option<Bounds>,
    ) -> Result<()> {
        let view = self
            .views
            .borrow()
            .get(&id)
            .cloned()
            .ok_or(Error::UnknownWebView(id))?;
        self.holes.borrow_mut().remove(&id);
        let Some(hole) = hole else {
            return Ok(());
        };
        let cut = PageHole::cut(&view.webview(), ns_rect(hole), CUT_RADIUS).ok_or(
            Error::UnsupportedParent("the web view is in no open window"),
        )?;
        self.holes.borrow_mut().insert(id, cut);
        Ok(())
    }
}

fn ns_rect(hole: Bounds) -> NSRect {
    NSRect::new(
        NSPoint::new(hole.x, hole.y),
        NSSize::new(hole.width, hole.height),
    )
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_hole_keeps_its_place_and_size_in_points() {
        let hole = Bounds {
            x: 100.0,
            y: 0.0,
            width: 69.0,
            height: 20.0,
        };
        let want = NSRect::new(NSPoint::new(100.0, 0.0), NSSize::new(69.0, 20.0));
        assert_eq!(ns_rect(hole), want);
    }
}
