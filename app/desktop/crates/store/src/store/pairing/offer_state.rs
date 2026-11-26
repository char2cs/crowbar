//! Where minting a code for the Add a device dialog stands.

use crate::store::pairing::offer::Offer;

/// Where minting a code stands.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum OfferState {
    /// The daemon is making a code.
    Minting,
    /// The daemon could not make one.
    Failed,
    /// A code to show.
    Ready(Offer),
}
