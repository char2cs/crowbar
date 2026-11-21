//! One entry of a space's top level, a home tree or a repository block, with the rows it draws.

use sidebar::Row;

use crate::rows::forest::SiblingKey;

pub(crate) struct RootBlock {
    pub(crate) key: SiblingKey,
    pub(crate) rows: Vec<Row>,
}
