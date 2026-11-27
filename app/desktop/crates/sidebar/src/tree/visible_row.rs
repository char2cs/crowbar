use crate::shell::Expansion;
use crate::tree::deletion::Hold;

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct VisibleRow {
    pub index: usize,
    pub expansion: Expansion,
    pub removal: Option<Hold>,
}
