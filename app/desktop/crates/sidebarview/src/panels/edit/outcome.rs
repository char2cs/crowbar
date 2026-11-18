//! How an inline input ended.

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum Outcome {
    Confirm,
    Cancel,
}
