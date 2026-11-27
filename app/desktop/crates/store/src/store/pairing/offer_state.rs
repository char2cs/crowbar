use crate::store::pairing::offer::Offer;

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum OfferState {
    Minting,
    Failed,
    Ready(Offer),
}
