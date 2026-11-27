use std::sync::Arc;

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct DraggedRow {
    pub row: Arc<str>,
    pub chat: Option<Arc<str>>,
    pub file: Option<Arc<str>>,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn rows_compare_by_row_and_chat() {
        let chat = DraggedRow {
            row: Arc::from("t1"),
            chat: Some(Arc::from("t1")),
            file: None,
        };
        let folder = DraggedRow {
            row: Arc::from("t1"),
            chat: None,
            file: None,
        };
        assert_eq!(chat, chat.clone(), "same payload");
        assert_ne!(chat, folder, "a folder opens nothing");
    }
}
