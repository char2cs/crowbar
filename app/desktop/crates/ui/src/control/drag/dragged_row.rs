//! Drag payload of a sidebar row.

use std::sync::Arc;

/// Payload attached while a sidebar row is dragged. The sidebar listens with
/// `on_drag_move::<DraggedRow>` to reorder; the pane layout listens for the same type to open
/// the chat the row stands for.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct DraggedRow {
    /// The identifier of the row being dragged.
    pub row: Arc<str>,
    /// The chat the row opens, when it is a chat row.
    pub chat: Option<Arc<str>>,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn rows_compare_by_row_and_chat() {
        let chat = DraggedRow {
            row: Arc::from("t1"),
            chat: Some(Arc::from("t1")),
        };
        let folder = DraggedRow {
            row: Arc::from("t1"),
            chat: None,
        };
        assert_eq!(chat, chat.clone(), "same payload");
        assert_ne!(chat, folder, "a folder opens nothing");
    }
}
