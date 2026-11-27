use crate::rows::forest::item_kind::ItemKind;
use crate::rows::forest::sibling_key::SiblingKey;

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct Tree {
    pub(crate) id: String,
    pub(crate) kind: ItemKind,
    pub(crate) key: SiblingKey,
    pub(crate) children: Vec<Tree>,
}
