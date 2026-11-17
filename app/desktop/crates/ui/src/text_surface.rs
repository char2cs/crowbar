//! What the text element needs from the field it paints.

use gpui::EntityInputHandler;

use crate::field_focus::FieldFocus;
use crate::field_selection::FieldSelection;
use crate::field_text::FieldText;
use crate::selection_drag::SelectionDrag;
use crate::text_input_layout::TextInputLayout;

pub(crate) trait TextSurface:
    EntityInputHandler + SelectionDrag + FieldText + FieldSelection + FieldFocus + TextInputLayout
{
}
