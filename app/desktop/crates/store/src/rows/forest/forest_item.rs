//! A node to place in the tree, with the two ids its parent may be named by.

use crate::rows::forest::item_kind::ItemKind;

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct ForestItem {
    pub(crate) id: String,
    pub(crate) kind: ItemKind,
    pub(crate) order: i64,
    pub(crate) created: String,
    pub(crate) filed: String,
    pub(crate) ground: String,
}
