use gpui::Hsla;

use crate::terminal_view::paint::cursor::focus_state::FocusState;
use crate::terminal_view::paint::shaping::Shaping;
use crate::terminal_view::paint::view_layout::ViewLayout;

pub(crate) struct Look<'a> {
    pub(crate) color: Hsla,
    pub(crate) glyph_color: Hsla,
    pub(crate) backdrop: Hsla,
    pub(crate) thickness: f32,
    pub(crate) focus: FocusState,
    pub(crate) shaping: &'a Shaping,
    pub(crate) layout: ViewLayout,
}
