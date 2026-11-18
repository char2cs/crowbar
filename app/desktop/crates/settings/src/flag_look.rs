//! How a record flag is drawn in its column.

/// The way a [`crate::RecordFlag`] is drawn: a switch the user toggles or a read-only dot.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum FlagLook {
    /// A switch the user can toggle.
    Switch,
    /// A small status dot the user cannot toggle.
    Dot,
}
