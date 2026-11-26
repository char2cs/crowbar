//! Where the Add a device dialog reads its code from, so tests can stand in for the daemon.

use std::sync::Arc;

use store::{DeviceOffer, OfferState};

pub(crate) trait OfferSource {
    fn state(&self) -> OfferState;
    fn renew(&self);
}

impl OfferSource for Arc<DeviceOffer> {
    fn state(&self) -> OfferState {
        DeviceOffer::state(self)
    }

    fn renew(&self) {
        DeviceOffer::renew(self);
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn needs_a_source<T: OfferSource>() -> usize {
        size_of::<T>()
    }

    #[test]
    fn a_device_offer_is_a_source() {
        assert!(needs_a_source::<Arc<DeviceOffer>>() > 0);
    }
}
