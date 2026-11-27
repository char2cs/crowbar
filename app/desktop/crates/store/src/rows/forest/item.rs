use crate::rows::forest::item_kind::ItemKind;

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct Item {
    pub(crate) id: String,
    pub(crate) kind: ItemKind,
    pub(crate) order: i64,
    pub(crate) created: String,
    pub(crate) filed: String,
    pub(crate) ground: String,
}
