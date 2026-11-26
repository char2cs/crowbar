//! A chat as one member of a view tab: its title and the glyph its workspace gives it.

use sidebar::{Activity, Row, RowKind, Worktree};

use crate::live::Entities;
use crate::live::WorkspaceRecord;
use crate::rows::look::{activity_of, lock_of, title_of, titling_of};
use crate::rows::workspace_view;

const DELETED: &str = "deleted";

pub(crate) fn of(
    store: &Entities,
    chat: &str,
) -> Option<Row> {
    let record = store.chats.get(chat)?;
    let mut row = Row::new(chat, RowKind::Chat, 0, title_of(&record.title));
    row.titling = titling_of(&record.title);
    row.activity = activity_of(record.working);
    let Some(ws) = owned(store, chat) else {
        return Some(row);
    };
    row.kind = RowKind::Branch;
    row.lock = lock_of(ws);
    row.worktree = Worktree::Owned;
    row.workspace = Some(workspace_view::landed(ws));
    if ws.working.is_on() {
        row.activity = Activity::Working;
    }
    Some(row)
}

fn owned<'a>(
    store: &'a Entities,
    chat: &str,
) -> Option<&'a WorkspaceRecord> {
    store
        .workspaces
        .values()
        .find(|ws| ws.owning_chat_id == chat && ws.status != DELETED)
}

#[cfg(test)]
mod tests {
    use serde_json::json;
    use sidebar::{Titling, WorkspaceStatus};

    use super::*;
    use crate::rows::look::UNTITLED_CHAT;
    use crate::test_support::entity_frame;

    fn store() -> Entities {
        let mut store = Entities::default();
        let rows = [
            (
                "workspace",
                "w",
                json!({"id": "w", "branch": "feat", "owningChatId": "owner", "status": "pr-open", "working": true}),
            ),
            (
                "workspace",
                "gone",
                json!({"id": "gone", "owningChatId": "thread", "status": "deleted"}),
            ),
            ("chat", "owner", json!({"id": "owner"})),
            ("chat", "thread", json!({"id": "thread", "title": "Perf"})),
        ];
        for (kind, id, data) in rows {
            store.apply(entity_frame(kind, id, data));
        }
        store
    }

    type Seen = (RowKind, String, Titling, Option<WorkspaceStatus>);

    fn seen(row: Row) -> Seen {
        let status = row.workspace.as_ref().map(|ws| ws.status);
        (row.kind, row.label, row.titling, status)
    }

    type Case = (
        &'static str,
        Option<(RowKind, &'static str, Titling, Option<WorkspaceStatus>)>,
    );

    const CASES: [Case; 3] = [
        (
            "thread",
            Some((RowKind::Chat, "Perf", Titling::Final, None)),
        ),
        (
            "owner",
            Some((
                RowKind::Branch,
                UNTITLED_CHAT,
                Titling::Provisional,
                Some(WorkspaceStatus::PrOpen),
            )),
        ),
        ("missing", None),
    ];

    #[test]
    fn a_chat_reads_its_title_and_the_branch_status_of_the_workspace_it_owns() {
        let store = store();
        for (chat, expected) in CASES {
            let found = of(&store, chat).map(seen);
            let expected = expected
                .map(|(kind, label, titling, status)| (kind, label.to_owned(), titling, status));
            assert_eq!(found, expected, "{chat}");
        }
        let owner = of(&store, "owner").unwrap();
        assert_eq!(owner.worktree, Worktree::Owned);
        assert_eq!(owner.activity, Activity::Working, "its workspace works");
    }
}
