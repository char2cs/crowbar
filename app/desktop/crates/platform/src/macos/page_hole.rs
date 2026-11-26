use objc2::rc::Retained;
use objc2::{MainThreadOnly as _, Message as _};
use objc2_app_kit::{NSView, NSWindowOrderingMode};
use objc2_core_graphics::CGColor;
use objc2_foundation::{NSPoint, NSRect, NSSize};
use objc2_quartz_core::{CALayer, CATransaction};

use crate::macos::appkit_parent::superview_of;

const BLACK: f64 = 0.0;
const OPAQUE: f64 = 1.0;

/// A hole cut out of a web view, where the shell view under it shows and takes the pointer, as
/// for the handle a split pane hangs from its top.
///
/// The page's layer is masked there, and a plain view above the page catches the pointer there:
/// a plain view handles no mouse event itself, so `AppKit` hands each one to its next responder,
/// the shell view. Dropping the value makes the page whole again.
#[derive(Debug)]
pub struct PageHole {
    page: Retained<NSView>,
    shell: Retained<NSView>,
    catcher: Retained<NSView>,
    mask: Retained<CALayer>,
    hole: NSRect,
    radius: f64,
}

impl PageHole {
    /// Cuts `hole`, in points from the top-left corner of `page` with y down, out of `page`. Its
    /// bottom corners round by `radius`; its top corners are lifted above the page.
    ///
    /// Returns `None` when `page` is not shown in an open window or has no layer.
    #[must_use]
    pub fn cut(
        page: &NSView,
        hole: NSRect,
        radius: f64,
    ) -> Option<Self> {
        let shell = superview_of(page)?;
        page.setWantsLayer(true);
        let layer = page.layer()?;
        let mask = CALayer::new();
        mask.setBorderColor(Some(&CGColor::new_generic_gray(BLACK, OPAQUE)));
        set_mask(&layer, Some(&mask));
        let catcher = NSView::new(page.mtm());
        shell.addSubview_positioned_relativeTo(&catcher, NSWindowOrderingMode::Above, Some(page));
        let cut = Self {
            page: page.retain(),
            shell,
            catcher,
            mask,
            hole,
            radius,
        };
        cut.follow();
        Some(cut)
    }

    /// Lays the hole over the page again, after the page moved or resized.
    pub fn follow(&self) {
        let caught = in_view(self.hole, &self.page);
        let rect = self.shell.convertRect_fromView(caught, Some(&self.page));
        self.catcher.setFrame(rect);
        let lifted = in_view(lift(self.hole, self.radius), &self.page);
        let masked = self.page.convertRectToLayer(lifted);
        let size = self.page.bounds().size;
        let reach = size.width + size.height;
        CATransaction::begin();
        CATransaction::setDisableActions(true);
        self.mask.setFrame(widen(masked, reach));
        self.mask.setCornerRadius(self.radius + reach);
        self.mask.setBorderWidth(reach);
        CATransaction::commit();
    }
}

impl Drop for PageHole {
    fn drop(&mut self) {
        self.catcher.removeFromSuperview();
        if let Some(layer) = self.page.layer() {
            set_mask(&layer, None);
        }
    }
}

#[expect(
    unsafe_code,
    reason = "CALayer::setMask is unsafe; the mask is a layer of its own"
)]
fn set_mask(
    layer: &CALayer,
    mask: Option<&CALayer>,
) {
    // SAFETY: the mask is a fresh layer in no layer tree and masks no other layer.
    unsafe { layer.setMask(mask) };
}

fn lift(
    hole: NSRect,
    radius: f64,
) -> NSRect {
    let origin = NSPoint::new(hole.origin.x, hole.origin.y - radius);
    NSRect::new(
        origin,
        NSSize::new(hole.size.width, hole.size.height + radius),
    )
}

// PLATFORM: macos a view's y grows upward unless it is flipped; the hole is given with y down.
fn in_view(
    hole: NSRect,
    page: &NSView,
) -> NSRect {
    if page.isFlipped() {
        return hole;
    }
    upright(hole, page.bounds().size.height)
}

fn upright(
    hole: NSRect,
    height: f64,
) -> NSRect {
    let y = height - hole.origin.y - hole.size.height;
    NSRect::new(NSPoint::new(hole.origin.x, y), hole.size)
}

fn widen(
    rect: NSRect,
    by: f64,
) -> NSRect {
    let origin = NSPoint::new(rect.origin.x - by, rect.origin.y - by);
    let size = NSSize::new(rect.size.width + by * 2.0, rect.size.height + by * 2.0);
    NSRect::new(origin, size)
}

#[cfg(test)]
mod tests {
    use super::*;

    fn rect(
        at: (f64, f64),
        size: (f64, f64),
    ) -> NSRect {
        NSRect::new(NSPoint::new(at.0, at.1), NSSize::new(size.0, size.1))
    }

    #[test]
    fn a_hole_from_the_top_lands_at_the_top_of_an_upright_view() {
        let hole = rect((10.0, 0.0), (60.0, 20.0));
        assert_eq!(upright(hole, 300.0), rect((10.0, 280.0), (60.0, 20.0)));
    }

    #[test]
    fn the_masked_hole_reaches_above_the_page_and_its_ring_past_every_edge() {
        let lifted = lift(rect((10.0, 0.0), (60.0, 20.0)), 8.0);
        assert_eq!(lifted, rect((10.0, -8.0), (60.0, 28.0)));
        let ring = widen(lifted, 100.0);
        assert_eq!(ring, rect((-90.0, -108.0), (260.0, 228.0)));
    }
}
