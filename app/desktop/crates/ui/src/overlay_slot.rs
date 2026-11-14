//! Keeps an [`OverlayGuard`] alive for as long as an element keeps rendering.

use crate::overlay_guard::OverlayGuard;

/// Entity wrapper so the guard drops, and the host hears the close, when the element stops rendering.
#[derive(Debug)]
pub(crate) struct OverlaySlot {
    pub(crate) guard: OverlayGuard,
}
