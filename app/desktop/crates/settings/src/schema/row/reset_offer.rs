//! Whether a row offers to reset its value to the default.

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum ResetOffer {
    Offered,
    Withheld,
}
