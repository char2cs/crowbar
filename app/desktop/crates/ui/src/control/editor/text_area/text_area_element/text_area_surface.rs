//! What the text area element needs from the field it paints.

use gpui::EntityInputHandler;

use crate::control::editor::field::FieldEditable;
use crate::control::editor::field::FieldFocus;
use crate::control::editor::field::FieldSelection;
use crate::control::editor::field::FieldText;
use crate::control::editor::field::SelectionDrag;
use crate::control::editor::text_area::text_area_element::text_area_caret::TextAreaCaret;
use crate::control::editor::text_area::text_area_element::text_area_sink::TextAreaSink;

pub(crate) trait TextAreaSurface:
    EntityInputHandler
    + SelectionDrag
    + FieldText
    + FieldSelection
    + FieldFocus
    + FieldEditable
    + TextAreaCaret
    + TextAreaSink
{
}
