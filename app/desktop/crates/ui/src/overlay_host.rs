//! The contract between overlays and the pane host.

use gpui::{Bounds, Pixels};

use crate::overlay_id::OverlayId;

/// Implemented by the pane host so web views never paint over menus, popovers and dialogs.
///
/// Native web views sit above GPUI content, so the host hides or clips every view that intersects the
/// bounds of an open overlay. Bounds are in window coordinates and are re-sent when the overlay moves
/// or resizes. A modal dialog reports the whole window.
pub trait OverlayHost {
    /// An overlay opened or changed its bounds.
    fn overlay_changed(&self, overlay: OverlayId, bounds: Bounds<Pixels>);

    /// The overlay closed; views it hid may return.
    fn overlay_closed(&self, overlay: OverlayId);
}
