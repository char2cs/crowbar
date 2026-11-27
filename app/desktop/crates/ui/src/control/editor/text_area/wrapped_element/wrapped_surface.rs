use gpui::EntityInputHandler;

use crate::control::editor::field::Blinking;
use crate::control::editor::field::Editable;
use crate::control::editor::field::Focus;
use crate::control::editor::field::Selection;
use crate::control::editor::field::SelectionDrag;
use crate::control::editor::field::Text;
use crate::control::editor::text_area::wrapped_element::caret::Caret;
use crate::control::editor::text_area::wrapped_element::sink::Sink;

pub(crate) trait WrappedSurface:
    EntityInputHandler + SelectionDrag + Text + Selection + Focus + Editable + Caret + Sink + Blinking
{
}
