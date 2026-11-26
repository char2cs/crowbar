//! The folders, workspaces and chats of one repository or project home, as tree items.

use std::collections::HashMap;

use crate::live::ChatRecord;
use crate::live::Entities;
use crate::live::FolderRecord;
use crate::live::PendingRow;
use crate::live::WorkspaceRecord;
use crate::rows::forest::Item;
use crate::rows::forest::ItemKind;

#[derive(Debug, Default)]
pub(crate) struct ScopeItems {
    pub(crate) items: Vec<Item>,
    pub(crate) owners: HashMap<String, String>,
}

pub(crate) fn of(
    store: &Entities,
    (project, repo): (&str, &str),
    pending: &[PendingRow],
) -> ScopeItems {
    let default_owner =
        default_workspace(store, (project, repo)).map(|ws| ws.owning_chat_id.clone());
    let in_scope = |p: &str, r: &str| p == project && r == repo;
    let mut scope = ScopeItems::default();
    for ws in store
        .workspaces
        .values()
        .filter(|ws| listed(ws, (project, repo)))
    {
        scope.items.push(workspace_item(ws));
        scope
            .owners
            .insert(ws.id.clone(), ws.owning_chat_id.clone());
    }
    scope.items.extend(
        store
            .folders
            .values()
            .filter(|f| in_scope(&f.project_id, &f.repo_id))
            .map(folder_item),
    );
    scope.items.extend(
        store
            .chats
            .values()
            .filter(|c| in_scope(&c.project_id, &c.repo_id))
            .filter(|c| default_owner.as_deref() != Some(c.id.as_str()))
            .map(chat_item),
    );
    scope.items.extend(
        pending
            .iter()
            .filter(|row| row.scope == repo && store.chats.get(&row.id).is_none())
            .map(pending_item),
    );
    scope
}

pub(crate) fn default_workspace<'a>(
    store: &'a Entities,
    (project, repo): (&str, &str),
) -> Option<&'a WorkspaceRecord> {
    store.workspaces.values().find(|ws| {
        ws.project_id == project && ws.repo_id == repo && (repo.is_empty() || ws.is_default)
    })
}

fn listed(
    ws: &WorkspaceRecord,
    (project, repo): (&str, &str),
) -> bool {
    !repo.is_empty()
        && ws.project_id == project
        && ws.repo_id == repo
        && !ws.is_default
        && ws.status != "deleted"
}

fn workspace_item(ws: &WorkspaceRecord) -> Item {
    Item {
        id: ws.id.clone(),
        kind: ItemKind::Workspace,
        order: ws.order,
        created: ws.created_at.clone(),
        filed: ws.folder_id.clone(),
        ground: ws.parent_id.clone(),
    }
}

fn folder_item(folder: &FolderRecord) -> Item {
    Item {
        id: folder.id.clone(),
        kind: ItemKind::Folder,
        order: folder.order,
        created: folder.created_at.clone(),
        filed: folder.parent_id.clone(),
        ground: String::new(),
    }
}

fn chat_item(chat: &ChatRecord) -> Item {
    Item {
        id: chat.id.clone(),
        kind: ItemKind::Chat,
        order: chat.order,
        created: chat.created_at.clone(),
        filed: chat.parent_id.clone(),
        ground: chat.workspace_id.clone(),
    }
}

fn pending_item(row: &PendingRow) -> Item {
    Item {
        id: row.id.clone(),
        kind: ItemKind::Chat,
        order: i64::MAX,
        created: String::new(),
        filed: row.parent.clone(),
        ground: String::new(),
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::store_of;

    const ROWS: [(&str, &str, &str); 10] = [
        (
            "workspace",
            "home",
            r#"{"id":"home","projectId":"p","repoId":"","owningChatId":"home-owner"}"#,
        ),
        (
            "workspace",
            "main",
            r#"{"id":"main","projectId":"p","repoId":"r","isDefault":true,"owningChatId":"main-owner"}"#,
        ),
        (
            "workspace",
            "fork",
            r#"{"id":"fork","projectId":"p","repoId":"r","owningChatId":"fork-owner"}"#,
        ),
        (
            "workspace",
            "dead",
            r#"{"id":"dead","projectId":"p","repoId":"r","status":"deleted"}"#,
        ),
        (
            "chat",
            "home-owner",
            r#"{"id":"home-owner","projectId":"p","repoId":""}"#,
        ),
        (
            "chat",
            "main-owner",
            r#"{"id":"main-owner","projectId":"p","repoId":"r"}"#,
        ),
        (
            "chat",
            "fork-owner",
            r#"{"id":"fork-owner","projectId":"p","repoId":"r"}"#,
        ),
        (
            "chat",
            "talk",
            r#"{"id":"talk","projectId":"p","repoId":"r"}"#,
        ),
        (
            "chat",
            "hello",
            r#"{"id":"hello","projectId":"p","repoId":""}"#,
        ),
        ("folder", "f", r#"{"id":"f","projectId":"p","repoId":"r"}"#),
    ];

    fn store() -> Entities {
        store_of(&ROWS)
    }

    fn ids(scope: &ScopeItems) -> Vec<&str> {
        let mut ids: Vec<&str> = scope.items.iter().map(|item| item.id.as_str()).collect();
        ids.sort_unstable();
        ids
    }

    fn a_repo_scope_lists_branches_folders_and_chats_but_not_its_header_checkout() {
        let scope = of(&store(), ("p", "r"), &[]);
        assert_eq!(ids(&scope), ["f", "fork", "fork-owner", "talk"]);
        assert_eq!(
            scope.owners.get("fork").map(String::as_str),
            Some("fork-owner")
        );
    }

    fn the_home_scope_has_no_branches_and_hides_the_home_owner() {
        let scope = of(&store(), ("p", ""), &[]);
        assert_eq!(ids(&scope), ["hello"]);
    }

    fn a_pending_row_is_an_item_until_its_chat_exists() {
        let pending = PendingRow {
            id: "new".into(),
            label: "New thread".into(),
            scope: "r".into(),
            parent: "talk".into(),
            ..PendingRow::default()
        };
        assert!(ids(&of(&store(), ("p", "r"), std::slice::from_ref(&pending))).contains(&"new"));
        let arrived = PendingRow {
            id: "talk".into(),
            ..pending
        };
        assert_eq!(
            ids(&of(&store(), ("p", "r"), &[arrived]))
                .iter()
                .filter(|id| **id == "talk")
                .count(),
            1
        );
    }

    const CASES: [fn(); 3] = [
        a_repo_scope_lists_branches_folders_and_chats_but_not_its_header_checkout,
        the_home_scope_has_no_branches_and_hides_the_home_owner,
        a_pending_row_is_an_item_until_its_chat_exists,
    ];

    #[test]
    fn every_scope_lists_what_belongs_to_it() {
        for case in CASES {
            case();
        }
    }
}
