//! Turning placed nodes into the rows the sidebar draws.

use std::collections::HashMap;

use sidebar::{Operation, OperationPhase, Row, RowKind, Titling, Worktree};

use crate::entity_store::EntityStore;
use crate::forest_tree::ForestTree;
use crate::item_kind::ItemKind;
use crate::pending_row::PendingRow;
use crate::row_look::{activity_of, lock_of, nesting_of, title_of, titling_of};
use crate::workspace_view;

pub(crate) struct RowEmitter<'a> {
    pub(crate) store: &'a EntityStore,
    pub(crate) pending: &'a [PendingRow],
    pub(crate) owners: &'a HashMap<String, String>,
}

impl RowEmitter<'_> {
    pub(crate) fn rows(
        &self,
        trees: &[ForestTree],
        depth: u8,
    ) -> Vec<Row> {
        trees
            .iter()
            .flat_map(|tree| self.subtree(tree, depth))
            .collect()
    }

    fn subtree(
        &self,
        tree: &ForestTree,
        depth: u8,
    ) -> Vec<Row> {
        let mut row = self.row(tree, depth);
        row.nesting = nesting_of(&tree.children);
        let mut rows = vec![row];
        rows.extend(self.rows(&tree.children, depth.saturating_add(1)));
        rows
    }

    fn row(
        &self,
        tree: &ForestTree,
        depth: u8,
    ) -> Row {
        match tree.kind {
            ItemKind::Folder => self.folder(tree, depth),
            ItemKind::Workspace => self.branch(tree, depth),
            ItemKind::Chat => self.chat(tree, depth),
        }
    }

    fn folder(
        &self,
        tree: &ForestTree,
        depth: u8,
    ) -> Row {
        let name = self.store.folders.get(&tree.id).map(|f| f.name.as_str());
        Row::new(
            tree.id.as_str(),
            RowKind::Folder,
            depth,
            name.unwrap_or_default(),
        )
    }

    fn chat(
        &self,
        tree: &ForestTree,
        depth: u8,
    ) -> Row {
        let Some(chat) = self.store.chats.get(&tree.id) else {
            return self.operation(tree, depth);
        };
        let mut row = Row::new(
            tree.id.as_str(),
            RowKind::Chat,
            depth,
            title_of(&chat.title),
        );
        row.titling = titling_of(&chat.title);
        row.activity = activity_of(chat.working);
        row.pending_move = chat.pending_move.as_ref().map(|moved| moved.label.clone());
        row
    }

    fn branch(
        &self,
        tree: &ForestTree,
        depth: u8,
    ) -> Row {
        let Some(ws) = self.store.workspaces.get(&tree.id) else {
            return Row::new(tree.id.as_str(), RowKind::Branch, depth, "");
        };
        let owner = self.owners.get(&ws.id).filter(|id| !id.is_empty());
        let chat = owner.and_then(|id| self.store.chats.get(id));
        let locked = workspace_view::is_locked(ws);
        let (label, titling) = match chat {
            Some(chat) if !locked => (title_of(&chat.title), titling_of(&chat.title)),
            _ => (ws.branch.as_str(), Titling::Final),
        };
        let id = owner.map_or(ws.id.as_str(), String::as_str);
        let mut row = Row::new(id, RowKind::Branch, depth, label);
        row.titling = titling;
        row.activity = activity_of(ws.working);
        row.lock = lock_of(ws);
        row.worktree = Worktree::Owned;
        row.workspace = Some(workspace_view::landed(ws));
        row
    }

    fn operation(
        &self,
        tree: &ForestTree,
        depth: u8,
    ) -> Row {
        let pending = self.pending.iter().find(|row| row.id == tree.id);
        let label = pending.map_or("", |row| row.label.as_str());
        let mut row = Row::new(tree.id.as_str(), RowKind::Chat, depth, label);
        let phase = match pending.and_then(|row| row.failure.clone()) {
            Some(reason) => OperationPhase::Failed { reason },
            None => OperationPhase::Running,
        };
        row.operation = Some(Operation {
            label: label.to_owned(),
            phase,
        });
        row
    }
}

#[cfg(test)]
mod tests {
    use std::collections::HashMap;

    use serde_json::json;

    use sidebar::{Activity, Nesting};

    use super::*;
    use daemonapi::Frame;

    use crate::forest_tree::ForestTree;
    use crate::row_look::UNTITLED_CHAT;

    fn store() -> EntityStore {
        let mut store = EntityStore::default();
        let rows = [
            (
                "workspace",
                "w",
                json!({"id": "w", "branch": "feat", "working": true, "status": "pr-open", "added": 3, "deleted": 1}),
            ),
            ("chat", "owner", json!({"id": "owner", "title": "Fix it"})),
            (
                "chat",
                "blank",
                json!({"id": "blank", "pendingMove": {"label": "feat"}}),
            ),
            ("folder", "f", json!({"id": "f", "name": "Stuff"})),
        ];
        for (kind, id, data) in rows {
            put(&mut store, (kind, id), data);
        }
        store
    }

    fn put(
        store: &mut EntityStore,
        (kind, id): (&str, &str),
        data: serde_json::Value,
    ) {
        store.apply(Frame::Entity {
            scope: "projects".into(),
            kind: kind.into(),
            id: id.into(),
            v: 1,
            data: Some(data),
            gone: false,
        });
    }

    fn node(
        id: &str,
        kind: ItemKind,
        children: Vec<ForestTree>,
    ) -> ForestTree {
        ForestTree {
            id: id.into(),
            kind,
            children,
        }
    }

    type Line<'a> = (&'a str, RowKind, u8, &'a str);

    #[test]
    fn rows_carry_kind_label_depth_state_and_nesting() {
        let store = store();
        let owners = HashMap::from([("w".to_owned(), "owner".to_owned())]);
        let emitter = RowEmitter {
            store: &store,
            pending: &[],
            owners: &owners,
        };
        let trees = [node(
            "f",
            ItemKind::Folder,
            vec![node(
                "w",
                ItemKind::Workspace,
                vec![node("blank", ItemKind::Chat, vec![])],
            )],
        )];
        let rows = emitter.rows(&trees, 0);
        let summary: Vec<Line<'_>> = rows
            .iter()
            .map(|row| (row.id.as_str(), row.kind, row.depth, row.label.as_str()))
            .collect();
        assert_eq!(
            summary,
            [
                ("f", RowKind::Folder, 0, "Stuff"),
                ("owner", RowKind::Branch, 1, "Fix it"),
                ("blank", RowKind::Chat, 2, UNTITLED_CHAT),
            ]
        );
        assert_eq!(rows[0].nesting, Nesting::Parent);
        assert_eq!(rows[1].activity, Activity::Working);
        assert_eq!(rows[2].titling, Titling::Provisional);
        assert_eq!(rows[2].pending_move.as_deref(), Some("feat"));
        let landed = rows[1].workspace.as_ref().unwrap();
        assert_eq!(
            (landed.added, landed.deleted, landed.branch.as_str()),
            (3, 1, "feat")
        );
    }

    #[test]
    fn an_unknown_chat_is_an_operation_row_with_its_failure() {
        let store = EntityStore::default();
        let owners = HashMap::new();
        let pending = [PendingRow {
            id: "c".into(),
            label: "New thread".into(),
            scope: String::new(),
            parent: String::new(),
            failure: Some("no provider".into()),
            operation: None,
        }];
        let emitter = RowEmitter {
            store: &store,
            pending: &pending,
            owners: &owners,
        };
        let rows = emitter.rows(&[node("c", ItemKind::Chat, vec![])], 0);
        assert_eq!(
            rows[0].operation.as_ref().map(|op| op.phase.clone()),
            Some(OperationPhase::Failed {
                reason: "no provider".into()
            })
        );
    }
}
