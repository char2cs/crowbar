use gpui::{Entity, Subscription};
use ui::TextInput;

use sidebar::RowId;

#[derive(Debug)]
pub(crate) struct Field {
    pub(crate) row: RowId,
    pub(crate) input: Entity<TextInput>,
    pub(crate) error: Option<String>,
    pub(crate) _subscriptions: Vec<Subscription>,
}
