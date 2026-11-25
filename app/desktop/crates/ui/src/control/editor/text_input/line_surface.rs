//! What the text element needs from the field it paints.

use gpui::EntityInputHandler;

use crate::control::editor::field::Blinking;
use crate::control::editor::field::Focus;
use crate::control::editor::field::Selection;
use crate::control::editor::field::SelectionDrag;
use crate::control::editor::field::Text;
use crate::control::editor::text_input::caret_scroll::CaretScroll;
use crate::control::editor::text_input::layout::Layout;

pub(crate) trait LineSurface:
    EntityInputHandler + SelectionDrag + Text + Selection + Focus + Layout + CaretScroll + Blinking
{
}
