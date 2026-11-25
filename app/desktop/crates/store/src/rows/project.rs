//! The chats tree of one space: its home level and its repositories, each under its header,
//! sorted as one sibling list.

mod root_block;

use sidebar::{Mark, Row, RowKind, Worktree};

use crate::live::Entities;
use crate::live::IconCache;
use crate::live::PendingRow;
use crate::live::RepoRecord;
use crate::live::WorkspaceRecord;
use crate::rows::emitter::Emitter;
use crate::rows::forest::SiblingKey;
use crate::rows::look::{activity_of, nesting_for};
use crate::rows::project::root_block::RootBlock;
use crate::rows::{forest, repo_mark};
use crate::rows::{scope_items, scope_items::ScopeItems};

const REPO_DEPTH: u8 = 1;

pub(crate) fn of(
    store: &Entities,
    project: &str,
    pending: &[PendingRow],
    icons: &IconCache,
) -> Vec<Row> {
    let mut blocks = home_blocks(store, project, pending);
    let repos = store
        .repos
        .values()
        .filter(|repo| repo.project_id == project);
    blocks.extend(repos.map(|repo| repo_block(store, repo, pending, icons)));
    blocks.sort_by(|a, b| a.key.cmp(&b.key));
    blocks.into_iter().flat_map(|block| block.rows).collect()
}

fn home_blocks(
    store: &Entities,
    project: &str,
    pending: &[PendingRow],
) -> Vec<RootBlock> {
    let ScopeItems { items, owners } = scope_items::of(store, (project, ""), pending);
    let trees = forest::fold_owners(forest::build(&items), &owners);
    let emitter = Emitter {
        store,
        pending,
        owners: &owners,
        folders: Worktree::Shared,
    };
    let key_of = |id: &str| {
        let item = items.iter().find(|item| item.id == id);
        item.map_or_default(SiblingKey::of_item)
    };
    trees
        .iter()
        .map(|tree| RootBlock {
            key: key_of(&tree.id),
            rows: emitter.rows(std::slice::from_ref(tree), 0),
        })
        .collect()
}

fn repo_block(
    store: &Entities,
    repo: &RepoRecord,
    pending: &[PendingRow],
    icons: &IconCache,
) -> RootBlock {
    let body = repo_rows(store, (&repo.project_id, &repo.id), pending);
    let mut rows = vec![header(store, repo, body.len(), icons)];
    rows.extend(body);
    RootBlock {
        key: SiblingKey::of_repo(repo.order, &repo.id),
        rows,
    }
}

fn repo_rows(
    store: &Entities,
    (project, repo): (&str, &str),
    pending: &[PendingRow],
) -> Vec<Row> {
    let ScopeItems { items, owners } = scope_items::of(store, (project, repo), pending);
    let trees = forest::fold_owners(forest::build(&items), &owners);
    let emitter = Emitter {
        store,
        pending,
        owners: &owners,
        folders: Worktree::Owned,
    };
    emitter.rows(&trees, REPO_DEPTH)
}

fn header(
    store: &Entities,
    repo: &RepoRecord,
    children: usize,
    icons: &IconCache,
) -> Row {
    let checkout = scope_items::default_workspace(store, (&repo.project_id, &repo.id));
    let mut row = Row::new(
        header_id(checkout, repo),
        RowKind::RepoHeader,
        0,
        repo.name.as_str(),
    );
    row.mark = Some(picture(repo, icons));
    row.activity = activity_of(checkout.map_or_default(|ws| ws.working));
    row.nesting = nesting_for(children);
    row
}

fn picture(
    repo: &RepoRecord,
    icons: &IconCache,
) -> Mark {
    let image = icons.file_for(&repo.id, &repo.avatar_url);
    match image {
        Some(path) if repo.avatar_emoji.is_empty() => Mark::Image(path.to_owned()),
        _ => repo_mark::of(
            (&repo.name, &repo.avatar_label),
            (&repo.avatar_emoji, &repo.avatar_color),
        ),
    }
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

    fn store() -> Entities {
        store_of(&ROWS)
    }

    fn of(
        store: &Entities,
        project: &str,
        pending: &[PendingRow],
    ) -> Vec<Row> {
        super::of(store, project, pending, &IconCache::default())
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
        let mut store = Entities::default();
        for text in frames {
            store.apply(daemonapi::Frame::decode(text).unwrap());
        }
        let rows = of(&store, "p", &[]);
        assert_eq!(
            rows.iter().map(|row| row.id.as_str()).collect::<Vec<_>>(),
            ["x"]
        );
    }

    const ORDERED: [(&str, &str, &str); 4] = [
        (
            "chat",
            "late",
            r#"{"id":"late","projectId":"p","repoId":"","title":"Late","order":2}"#,
        ),
        (
            "repo",
            "r",
            r#"{"id":"r","projectId":"p","name":"crowbar","order":1}"#,
        ),
        (
            "chat",
            "first",
            r#"{"id":"first","projectId":"p","repoId":"","title":"First","order":0}"#,
        ),
        (
            "folder",
            "f",
            r#"{"id":"f","projectId":"p","name":"Notes","order":1}"#,
        ),
    ];

    fn the_home_level_and_the_repos_sort_as_one_sibling_list() {
        let rows = of(&store_of(&ORDERED), "p", &[]);
        let ids: Vec<&str> = rows.iter().map(|row| row.id.as_str()).collect();
        assert_eq!(
            ids,
            ["first", "f", "r", "late"],
            "order, then folder before repo"
        );
    }

    const PICTURED: [(&str, &str, &str); 1] = [(
        "repo",
        "r",
        r#"{"id":"r","projectId":"p","name":"crowbar","avatarUrl":"/v0/projects/p/repos/r/icon?v=1"}"#,
    )];

    fn a_repo_with_a_fetched_image_wears_it_as_its_mark() {
        let mut icons = IconCache::in_dir("/c".into());
        let url = "/v0/projects/p/repos/r/icon?v=1";
        icons.finish("r", url, Some("/c/r-1".into()));
        let rows = super::of(&store_of(&PICTURED), "p", &[], &icons);
        let marks: Vec<_> = rows.iter().map(|row| row.mark.clone()).collect();
        assert_eq!(marks, [Some(Mark::Image("/c/r-1".into()))]);
    }

    const CASES: [fn(); 5] = [
        a_repo_with_a_fetched_image_wears_it_as_its_mark,
        the_home_level_and_the_repos_sort_as_one_sibling_list,
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
