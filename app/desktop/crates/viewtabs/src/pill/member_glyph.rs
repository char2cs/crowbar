use gpui::Hsla;
use ui::{IconName, IconWeight};

/// The glyph a lone member leads with in place of its kind's: the sidebar's glyph for the chat.
#[derive(Debug, Clone, Copy, PartialEq)]
pub struct MemberGlyph {
    /// The icon drawn.
    pub icon: IconName,
    /// The weight it is drawn at.
    pub weight: IconWeight,
    /// The colour it keeps on any ground; none follows the tab's text.
    pub color: Option<Hsla>,
}
