//! How an inline input ended.

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum EditOutcome {
    Confirm,
    Cancel,
}
