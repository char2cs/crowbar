//! Everything the cursor painter needs besides the cell.

use gpui::Hsla;

use crate::focus_state::FocusState;
use crate::shaping::Shaping;
use crate::view_layout::ViewLayout;

pub(crate) struct CursorLook<'a> {
    pub(crate) color: Hsla,
    pub(crate) glyph_color: Hsla,
    pub(crate) backdrop: Hsla,
    pub(crate) thickness: f32,
    pub(crate) focus: FocusState,
    pub(crate) shaping: &'a Shaping,
    pub(crate) layout: ViewLayout,
}
