//! What the text area element needs from the field it paints.

use gpui::EntityInputHandler;

use crate::field_editable::FieldEditable;
use crate::field_focus::FieldFocus;
use crate::field_selection::FieldSelection;
use crate::field_text::FieldText;
use crate::selection_drag::SelectionDrag;
use crate::text_area_caret::TextAreaCaret;
use crate::text_area_sink::TextAreaSink;

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
