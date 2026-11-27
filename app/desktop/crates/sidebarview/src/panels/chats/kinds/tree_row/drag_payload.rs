use std::sync::Arc;

use sidebar::Row;
use ui::DraggedRow;

pub(super) fn of(row: &Row) -> DraggedRow {
    let id: Arc<str> = Arc::from(row.id.as_str());
    DraggedRow {
        chat: opens_chat(row).then(|| Arc::clone(&id)),
        row: id,
        file: None,
    }
}

fn opens_chat(row: &Row) -> bool {
    matches!(row.kind, sidebar::RowKind::Chat | sidebar::RowKind::Branch)
}

#[cfg(test)]
mod tests {
    use sidebar::RowKind;

    use super::*;
    use crate::test_support::chat;

    #[test]
    fn a_row_carries_the_chat_it_opens_and_a_folder_or_header_none() {
        let mut locked = Row::new("main-owner", RowKind::Branch, 1, "main");
        locked.lock = sidebar::Lock::Locked;
        let cases = [
            (chat("Refactor"), true),
            (Row::new("b1", RowKind::Branch, 1, "Untitled chat"), true),
            (locked, true),
            (Row::new("f", RowKind::Folder, 1, "src"), false),
            (Row::new("r", RowKind::RepoHeader, 0, "checkout"), false),
        ];
        for (row, opens) in cases {
            let payload = of(&row);
            let carried = payload.chat.as_deref() == Some(payload.row.as_ref());
            assert_eq!(carried, opens, "{:?}", row.kind);
            assert_eq!(payload.chat.is_none(), !opens, "{:?}", row.kind);
        }
    }
}
