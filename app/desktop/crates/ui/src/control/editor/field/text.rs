//! The text a field shows.

use gpui::SharedString;

pub(crate) trait Text {
    fn content(&self) -> &str;
    fn hint(&self) -> &SharedString;
}
