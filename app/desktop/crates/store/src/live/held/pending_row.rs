//! A create that has no entity yet, drawn as an operation row.

#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub(crate) struct PendingRow {
    pub(crate) id: String,
    pub(crate) label: String,
    pub(crate) scope: String,
    pub(crate) parent: String,
    pub(crate) failure: Option<String>,
    pub(crate) operation: Option<String>,
}
