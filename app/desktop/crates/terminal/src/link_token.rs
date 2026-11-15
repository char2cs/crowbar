//! A link candidate inside a line of text.

use crate::link_target::LinkTarget;

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct LinkToken {
    pub(crate) target: LinkTarget,
    pub(crate) start: usize,
    pub(crate) end: usize,
}
