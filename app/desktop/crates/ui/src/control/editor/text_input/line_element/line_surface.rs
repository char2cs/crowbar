use gpui::EntityInputHandler;

use crate::control::editor::field::Blinking;
use crate::control::editor::field::Focus;
use crate::control::editor::field::Selection;
use crate::control::editor::field::SelectionDrag;
use crate::control::editor::field::Text;
use crate::control::editor::text_input::line_element::caret_scroll::CaretScroll;
use crate::control::editor::text_input::line_element::layout::Layout;

pub(crate) trait LineSurface:
    EntityInputHandler + SelectionDrag + Text + Selection + Focus + Layout + CaretScroll + Blinking
{
}
