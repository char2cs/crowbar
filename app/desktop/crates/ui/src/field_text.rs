//! The text a field shows.

use gpui::SharedString;

pub(crate) trait FieldText {
    fn content(&self) -> &str;
    fn hint(&self) -> &SharedString;
}
