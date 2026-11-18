//! A link found in terminal output.

use crate::hyperlink::link_target::LinkTarget;
use crate::snapshot::Highlight;

/// A link and the visible cells it covers (one highlight per row it spans).
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Link {
    /// Where it points.
    pub target: LinkTarget,
    /// The cells to underline while it is hovered.
    pub cells: Vec<Highlight>,
}
