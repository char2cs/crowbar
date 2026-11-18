//! A placed node with its children.

use crate::item_kind::ItemKind;

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct ForestTree {
    pub(crate) id: String,
    pub(crate) kind: ItemKind,
    pub(crate) children: Vec<ForestTree>,
}
