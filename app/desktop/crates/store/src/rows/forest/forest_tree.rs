//! A placed node with its children.

use crate::rows::forest::item_kind::ItemKind;

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct ForestTree {
    pub(crate) id: String,
    pub(crate) kind: ItemKind,
    pub(crate) children: Vec<ForestTree>,
}
