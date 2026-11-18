//! What the text element needs from the field it paints.

use gpui::EntityInputHandler;

use crate::control::editor::field::FieldFocus;
use crate::control::editor::field::FieldSelection;
use crate::control::editor::field::FieldText;
use crate::control::editor::field::SelectionDrag;
use crate::control::editor::text_input::text_input_layout::TextInputLayout;

pub(crate) trait TextSurface:
    EntityInputHandler + SelectionDrag + FieldText + FieldSelection + FieldFocus + TextInputLayout
{
}
