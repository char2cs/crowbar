use crate::hyperlink::link_target::LinkTarget;
use crate::snapshot::Highlight;

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Link {
    pub target: LinkTarget,
    pub cells: Vec<Highlight>,
}
