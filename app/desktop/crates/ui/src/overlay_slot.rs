//! Keeps an [`OverlayGuard`] alive for as long as an element keeps rendering.

use crate::overlay_guard::OverlayGuard;

#[derive(Debug)]
pub(crate) struct OverlaySlot {
    pub(crate) guard: OverlayGuard,
}
