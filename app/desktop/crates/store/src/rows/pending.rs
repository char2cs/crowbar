//! The create operations the daemon reports, as rows to draw until their entities arrive.

use crate::live::EntityStore;
use crate::live::OperationItem;
use crate::live::OperationRecord;
use crate::live::PendingRow;

const DRAWN_KINDS: [&str; 3] = ["chat.create", "branch.create", "branch.import"];
const FAILED: &str = "failed";
const SUCCEEDED: &str = "succeeded";
const NOT_SAID: &str = "the daemon did not say why";

pub(crate) fn of(store: &EntityStore) -> Vec<PendingRow> {
    store
        .operations
        .values()
        .filter(|op| !op.dismissed && DRAWN_KINDS.contains(&op.kind.as_str()))
        .flat_map(|op| {
            op.items
                .iter()
                .filter(|item| item.target.kind == "chat")
                .filter(|item| !arrived(store, item) && !deleted_since(op, item))
                .map(|item| row(store, op, item))
        })
        .collect()
}

fn arrived(
    store: &EntityStore,
    item: &OperationItem,
) -> bool {
    store.chats.get(&item.target.id).is_some()
}

fn deleted_since(
    op: &OperationRecord,
    item: &OperationItem,
) -> bool {
    item.state == SUCCEEDED || op.state == SUCCEEDED
}

fn failure(
    op: &OperationRecord,
    item: &OperationItem,
) -> Option<String> {
    let failed = item.state == FAILED || item.error.is_some() || op.state == FAILED;
    let reason = item.error.as_ref().or(op.error.as_ref());
    failed.then(|| reason.map_or_else(|| NOT_SAID.to_owned(), |p| p.message.clone()))
}

fn row(
    store: &EntityStore,
    op: &OperationRecord,
    item: &OperationItem,
) -> PendingRow {
    let parent = item.parent_id.clone().unwrap_or_default();
    PendingRow {
        id: item.target.id.clone(),
        label: item.label.clone(),
        scope: scope_of(store, op, &parent),
        parent,
        failure: failure(op, item),
        operation: Some(op.id.clone()),
    }
}

fn scope_of(
    store: &EntityStore,
    op: &OperationRecord,
    parent: &str,
) -> String {
    let named = store
        .chats
        .get(parent)
        .map(|chat| chat.repo_id.clone())
        .or_else(|| {
            store
                .folders
                .get(parent)
                .map(|folder| folder.repo_id.clone())
        });
    named.unwrap_or_else(|| subject_scope(store, op))
}

fn subject_scope(
    store: &EntityStore,
    op: &OperationRecord,
) -> String {
    match op.subject.kind.as_str() {
        "repo" => op.subject.id.clone(),
        "workspace" => store
            .workspaces
            .get(&op.subject.id)
            .map_or_default(|ws| ws.repo_id.clone()),
        _ => String::new(),
    }
}

#[cfg(test)]
mod tests {
    use serde_json::json;

    use super::*;
    use daemonapi::Frame;

    fn store_with(op: serde_json::Value) -> EntityStore {
        let mut store = EntityStore::default();
        store.apply(Frame::Entity {
            scope: "projects".into(),
            kind: "operation".into(),
            id: "op".into(),
            v: 1,
            data: Some(op),
            gone: false,
        });
        store
    }

    fn op(
        state: &str,
        item: &serde_json::Value,
    ) -> serde_json::Value {
        json!({"id": "op", "kind": "chat.create", "state": state,
           "subject": {"kind": "workspace", "id": "w"}, "items": [item]})
    }

    fn item(state: &str) -> serde_json::Value {
        json!({"ref": {"kind": "chat", "id": "c"}, "label": "New thread", "state": state})
    }

    fn a_running_create_is_drawn_without_a_failure() {
        let rows = of(&store_with(op("running", &item("running"))));
        assert_eq!(rows.len(), 1);
        assert_eq!(rows[0].id, "c");
        assert_eq!(rows[0].failure, None);
    }

    fn a_failed_create_keeps_the_daemons_reason() {
        let mut failed = item("failed");
        failed["error"] = json!({"code": "x", "message": "no provider"});
        let rows = of(&store_with(op("failed", &failed)));
        assert_eq!(rows[0].failure.as_deref(), Some("no provider"));
    }

    fn a_finished_create_whose_entity_is_gone_is_not_drawn() {
        assert_eq!(
            of(&store_with(op("succeeded", &item("succeeded")))).len(),
            0
        );
    }

    const CASES: [fn(); 3] = [
        a_running_create_is_drawn_without_a_failure,
        a_failed_create_keeps_the_daemons_reason,
        a_finished_create_whose_entity_is_gone_is_not_drawn,
    ];

    #[test]
    fn every_create_is_drawn_by_its_state() {
        for case in CASES {
            case();
        }
    }
}
