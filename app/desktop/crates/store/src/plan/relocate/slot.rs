//! A container and an insert position among its children.

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct Slot {
    pub(crate) parent: String,
    pub(crate) index: usize,
}
