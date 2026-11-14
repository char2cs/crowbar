//! Identity of one open overlay.

/// Identifies an overlay between its `overlay_opened` and `overlay_closed` notifications.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, PartialOrd, Ord)]
pub struct OverlayId(pub(crate) u64);

impl OverlayId {
    /// The raw number, stable while the overlay is open.
    pub fn get(self) -> u64 {
        self.0
    }
}
