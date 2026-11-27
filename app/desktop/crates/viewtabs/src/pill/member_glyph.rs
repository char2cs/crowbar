use gpui::Hsla;
use ui::{IconName, IconWeight};

#[derive(Debug, Clone, Copy, PartialEq)]
pub struct MemberGlyph {
    pub icon: IconName,
    pub weight: IconWeight,
    pub color: Option<Hsla>,
}
