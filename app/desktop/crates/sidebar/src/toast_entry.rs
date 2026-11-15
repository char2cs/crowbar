//! One toast of the sidebar overlay.

use ui::ToastKind;

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct ToastEntry {
    pub(crate) id: u64,
    pub(crate) key: Option<String>,
    pub(crate) kind: ToastKind,
    pub(crate) message: String,
    pub(crate) updates: u32,
}
