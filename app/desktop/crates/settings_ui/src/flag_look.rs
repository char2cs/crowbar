//! How a record flag is drawn in its column.

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum FlagLook {
    Switch,
    Dot,
}
