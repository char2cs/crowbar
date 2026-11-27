use gpui::Entity;
use ui::TextInput;

use crate::panels::chats::parts::row_label::trailing::Trailing;

#[derive(Debug, Clone)]
pub(crate) struct RowInput {
    pub(crate) field: Entity<TextInput>,
    pub(crate) error: Option<String>,
    pub(crate) trailing: Trailing,
}
