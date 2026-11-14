//! The inline input that replaces a row's label.

use gpui::Entity;
use ui::TextInput;

#[derive(Debug, Clone)]
pub(crate) struct RowInput {
    pub(crate) field: Entity<TextInput>,
    pub(crate) error: Option<String>,
}
