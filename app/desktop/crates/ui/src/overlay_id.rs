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

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn exposes_the_raw_number() {
        assert_eq!(OverlayId(42).get(), 42);
    }

    #[test]
    fn ids_order_by_number() {
        assert!(OverlayId(1) < OverlayId(2));
    }
}
