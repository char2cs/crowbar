//! The chats tree of one space: its home level, then each repository under its header.

use sidebar::{Row, RowKind};

use crate::entity_store::EntityStore;
use crate::pending_row::PendingRow;
use crate::repo_record::RepoRecord;
use crate::row_emitter::RowEmitter;
use crate::row_look::{activity_of, nesting_for};
use crate::scope_items::{self, ScopeItems};
use crate::workspace_record::WorkspaceRecord;
use crate::{forest, repo_mark};

pub(crate) fn of(
    store: &EntityStore,
    project: &str,
    pending: &[PendingRow],
) -> Vec<Row> {
    let mut rows = scope_rows(store, (project, ""), pending);
    let mut repos: Vec<&RepoRecord> = store
        .repos
        .values()
        .filter(|repo| repo.project_id == project)
        .collect();
    repos.sort_by(|a, b| (a.order, &a.id).cmp(&(b.order, &b.id)));
    for repo in repos {
        let body = scope_rows(store, (project, &repo.id), pending);
        rows.push(header(store, repo, body.len()));
        rows.extend(body);
    }
    rows
}

fn scope_rows(
    store: &EntityStore,
    (project, repo): (&str, &str),
    pending: &[PendingRow],
) -> Vec<Row> {
    let depth = u8::from(!repo.is_empty());
    let ScopeItems { items, owners } = scope_items::of(store, (project, repo), pending);
    let trees = forest::fold_owners(forest::build(&items), &owners);
    let emitter = RowEmitter {
        store,
        pending,
        owners: &owners,
    };
    emitter.rows(&trees, depth)
}

fn header(
    store: &EntityStore,
    repo: &RepoRecord,
    children: usize,
) -> Row {
    let checkout = scope_items::default_workspace(store, (&repo.project_id, &repo.id));
    let mut row = Row::new(
        header_id(checkout, repo),
        RowKind::RepoHeader,
        0,
        repo.name.as_str(),
    );
    row.mark = Some(repo_mark::of(
        (&repo.name, &repo.avatar_label),
        (&repo.avatar_emoji, &repo.avatar_color),
    ));
    row.activity = activity_of(checkout.map_or_default(|ws| ws.working));
    row.nesting = nesting_for(children);
    row
}

fn header_id(
    checkout: Option<&WorkspaceRecord>,
    repo: &RepoRecord,
) -> String {
    let named = checkout.map(|ws| [&ws.owning_chat_id, &ws.id]);
    let first = named.and_then(|ids| ids.into_iter().find(|id| !id.is_empty()));
    first.unwrap_or(&repo.id).clone()
}

#[cfg(test)]
mod tests {
    use sidebar::Nesting;

    use super::*;
    use crate::test_support::store_of;

    const ROWS: [(&str, &str, &str); 10] = [
        (
            "workspace",
            "home",
            r#"{"id":"home","projectId":"p","repoId":"","owningChatId":"home-owner"}"#,
        ),
        (
            "chat",
            "home-owner",
            r#"{"id":"home-owner","projectId":"p","repoId":""}"#,
        ),
        (
            "chat",
            "hello",
            r#"{"id":"hello","projectId":"p","repoId":"","title":"Hello"}"#,
        ),
        (
            "repo",
            "r",
            r#"{"id":"r","projectId":"p","name":"crowbar","avatarLabel":"C","avatarColor":"avatar-rose"}"#,
        ),
        (
            "workspace",
            "main",
            r#"{"id":"main","projectId":"p","repoId":"r","isDefault":true,"owningChatId":"main-owner"}"#,
        ),
        (
            "chat",
            "main-owner",
            r#"{"id":"main-owner","projectId":"p","repoId":"r"}"#,
        ),
        (
            "workspace",
            "lock",
            r#"{"id":"lock","projectId":"p","repoId":"r","branch":"main","status":"locked","owningChatId":"lock-owner"}"#,
        ),
        (
            "chat",
            "lock-owner",
            r#"{"id":"lock-owner","projectId":"p","repoId":"r","workspaceId":"lock"}"#,
        ),
        (
            "chat",
            "thread",
            r#"{"id":"thread","projectId":"p","repoId":"r","workspaceId":"lock","parentId":"lock-owner","title":"Idea"}"#,
        ),
        (
            "chat",
            "other",
            r#"{"id":"other","projectId":"q","repoId":"","title":"elsewhere"}"#,
        ),
    ];

    fn store() -> EntityStore {
        store_of(&ROWS)
    }

    type Line<'a> = (&'a str, RowKind, u8, &'a str);

    fn a_space_lists_its_home_chats_then_each_repo_under_a_header() {
        let rows = of(&store(), "p", &[]);
        let summary: Vec<Line<'_>> = rows
            .iter()
            .map(|row| (row.id.as_str(), row.kind, row.depth, row.label.as_str()))
            .collect();
        assert_eq!(
            summary,
            [
                ("hello", RowKind::Chat, 0, "Hello"),
                ("main-owner", RowKind::RepoHeader, 0, "crowbar"),
                ("lock-owner", RowKind::Branch, 1, "main"),
                ("thread", RowKind::Chat, 2, "Idea"),
            ]
        );
        assert_eq!(rows[1].nesting, Nesting::Parent);
        assert!(rows[1].mark.is_some());
    }

    fn another_space_has_its_own_rows() {
        let rows = of(&store(), "q", &[]);
        assert_eq!(
            rows.iter().map(|row| row.id.as_str()).collect::<Vec<_>>(),
            ["other"]
        );
    }

    fn the_shapes_the_daemon_sends_for_a_fresh_space_list_its_extra_home_chat() {
        let frames = [
            r#"{"t":"entity","scope":"projects","kind":"workspace","id":"w","v":43,"data":{"id":"w","repoId":"","projectId":"p","kind":"home","branch":"","status":"new","working":false,"isDefault":false,"owningChatId":"o","folderId":"","order":0,"createdAt":"2026-11-15T02:12:37.227758984Z"}}"#,
            r#"{"t":"entity","scope":"projects","kind":"chat","id":"o","v":43,"data":{"id":"o","workspaceId":"w","title":"","type":"chat","liveRunnerId":"","working":false,"parentId":"","order":0,"createdAt":"2026-11-15T02:12:37.288453053Z","version":1794708732871309,"phase":"dormant","projectId":"p","repoId":""}}"#,
            r#"{"t":"entity","scope":"projects","kind":"chat","id":"x","v":43,"data":{"id":"x","workspaceId":"w","title":"","type":"chat","parentId":"","order":1,"createdAt":"2026-11-15T02:13:36.045653199Z","projectId":"p","repoId":""}}"#,
        ];
        let mut store = EntityStore::default();
        for text in frames {
            store.apply(daemonapi::frame::decode(text).unwrap());
        }
        let rows = of(&store, "p", &[]);
        assert_eq!(
            rows.iter().map(|row| row.id.as_str()).collect::<Vec<_>>(),
            ["x"]
        );
    }

    const CASES: [fn(); 3] = [
        a_space_lists_its_home_chats_then_each_repo_under_a_header,
        another_space_has_its_own_rows,
        the_shapes_the_daemon_sends_for_a_fresh_space_list_its_extra_home_chat,
    ];

    #[test]
    fn every_space_lists_its_own_rows() {
        for case in CASES {
            case();
        }
    }
}
