mod placed_row;

use serde::Deserialize;

use crate::live::held::entities::Entities;
use crate::live::held::entities::placement_answer::placed_row::PlacedRow;

#[derive(Debug, Clone, PartialEq, Eq, Default, Deserialize)]
#[serde(default)]
pub(crate) struct PlacementAnswer {
    workspace: PlacedRow,
    shifted: Vec<PlacedRow>,
}

impl PlacementAnswer {
    pub(crate) fn apply(
        &self,
        (workspace, store): (&str, &mut Entities),
    ) {
        file_workspace(store, (workspace, &self.workspace));
        self.shifted.iter().for_each(|row| shift(store, row));
    }
}

fn file_workspace(
    store: &mut Entities,
    (id, row): (&str, &PlacedRow),
) {
    store.workspaces.patch(id, |ws| {
        ws.folder_id.clone_from(&row.parent_id);
        ws.order = row.order;
    });
}

fn shift_chat(
    store: &mut Entities,
    row: &PlacedRow,
) {
    store.chats.patch(&row.id, |held| {
        held.parent_id.clone_from(&row.parent_id);
        held.order = row.order;
    });
}

fn shift_folder(
    store: &mut Entities,
    row: &PlacedRow,
) {
    store.folders.patch(&row.id, |held| {
        held.parent_id.clone_from(&row.parent_id);
        held.order = row.order;
    });
}

fn shift(
    store: &mut Entities,
    row: &PlacedRow,
) {
    let id = row.id.as_str();
    if store.chats.get(id).is_some() {
        return shift_chat(store, row);
    }
    if store.folders.get(id).is_some() {
        return shift_folder(store, row);
    }
    file_workspace(store, (id, row));
}

#[cfg(test)]
mod tests {
    use serde_json::json;

    use super::*;
    use crate::test_support::store_of;

    fn answer() -> PlacementAnswer {
        let raw = json!({
            "workspace": {"id": "ownerchat", "parentId": "f", "order": 0},
            "shifted": [
                {"id": "t", "parentId": "f", "order": 1},
                {"id": "f2", "parentId": "", "order": 2},
            ],
        });
        serde_json::from_value(raw).unwrap()
    }

    const ROWS: [(&str, &str, &str); 3] = [
        ("workspace", "ws", r#"{"id":"ws","folderId":"","order":4}"#),
        ("chat", "t", r#"{"id":"t","parentId":"bo","order":0}"#),
        ("folder", "f2", r#"{"id":"f2","parentId":"f","order":0}"#),
    ];

    #[test]
    fn the_answer_files_the_workspace_and_moves_each_shifted_sibling() {
        let mut store = store_of(&ROWS);
        answer().apply(("ws", &mut store));
        let ws = store.workspaces.get("ws").unwrap();
        assert_eq!((ws.folder_id.as_str(), ws.order), ("f", 0), "workspace");
        let chat = store.chats.get("t").unwrap();
        assert_eq!((chat.parent_id.as_str(), chat.order), ("f", 1), "chat");
        let folder = store.folders.get("f2").unwrap();
        assert_eq!((folder.parent_id.as_str(), folder.order), ("", 2), "folder");
    }
}
