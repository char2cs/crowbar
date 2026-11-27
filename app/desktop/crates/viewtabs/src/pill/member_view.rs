use gpui::SharedString;
use protocol::PaneId;

use crate::pill::member_glyph::MemberGlyph;
use crate::pill::member_kind::MemberKind;

#[derive(Debug, Clone, PartialEq)]
pub struct MemberView {
    pub pane: PaneId,
    pub label: SharedString,
    pub provisional: bool,
    pub working: bool,
    pub kind: MemberKind,
    pub glyph: Option<MemberGlyph>,
}
