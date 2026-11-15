use std::sync::Arc;

use crate::daemon_client::DaemonClient;
use crate::entity_store::EntityStore;
use crate::frame::Frame;
use crate::host_slot::HostSlot;
use crate::host_spec::HostSpec;
use crate::state::State;

use super::{FakeDaemon, local};

const ROWS: [(&str, &str, &str); 11] = [
    ("project", "p", r#"{"id":"p","name":"Demo","order":0}"#),
    (
        "workspace",
        "hw",
        r#"{"id":"hw","projectId":"p","repoId":"","kind":"home","owningChatId":"ho"}"#,
    ),
    (
        "chat",
        "ho",
        r#"{"id":"ho","projectId":"p","repoId":"","workspaceId":"hw"}"#,
    ),
    (
        "chat",
        "hc",
        r#"{"id":"hc","projectId":"p","repoId":"","workspaceId":"hw","title":"Hello","createdAt":"2026-01-01T00:00:00Z"}"#,
    ),
    (
        "repo",
        "r",
        r#"{"id":"r","projectId":"p","name":"crowbar","avatarLabel":"C","avatarColor":"avatar-rose"}"#,
    ),
    (
        "workspace",
        "rd",
        r#"{"id":"rd","projectId":"p","repoId":"r","isDefault":true,"branch":"main","owningChatId":"rdo"}"#,
    ),
    (
        "chat",
        "rdo",
        r#"{"id":"rdo","projectId":"p","repoId":"r","workspaceId":"rd"}"#,
    ),
    (
        "workspace",
        "b",
        r#"{"id":"b","projectId":"p","repoId":"r","branch":"feat","owningChatId":"bo","createdAt":"2026-01-02T00:00:00Z"}"#,
    ),
    (
        "chat",
        "bo",
        r#"{"id":"bo","projectId":"p","repoId":"r","workspaceId":"b","title":"Idea","liveRunnerId":"run","createdAt":"2026-01-02T00:00:00Z"}"#,
    ),
    (
        "chat",
        "t",
        r#"{"id":"t","projectId":"p","repoId":"r","workspaceId":"b","parentId":"bo","title":"Thread","createdAt":"2026-01-03T00:00:00Z"}"#,
    ),
    (
        "folder",
        "f",
        r#"{"id":"f","projectId":"p","repoId":"r","name":"Stuff","createdAt":"2026-01-04T00:00:00Z"}"#,
    ),
];

fn entity((kind, id, data): (&str, &str, &str)) -> Frame {
    Frame::Entity {
        scope: "projects".into(),
        kind: kind.into(),
        id: id.into(),
        v: 1,
        data: serde_json::from_str(data).ok(),
        gone: false,
    }
}

/// The daemon's frames for one space: a home with a chat, a repository with its checkout, a
/// branch holding a thread, and a folder.
pub(crate) fn frames() -> Vec<Frame> {
    let synced = Frame::Synced {
        scope: "projects".into(),
        unread: Vec::new(),
    };
    let mut frames = vec![Frame::Ready { epoch: "e".into() }];
    frames.extend(ROWS.into_iter().map(entity));
    frames.push(synced);
    frames
}

/// An entity store holding `rows`, each `(kind, id, json)`.
pub(crate) fn store_of(rows: &[(&str, &str, &str)]) -> EntityStore {
    let mut store = EntityStore::default();
    rows.iter()
        .copied()
        .map(entity)
        .for_each(|frame| store.apply(frame));
    store
}

/// A state of one local host that has heard [`frames`], and the daemon its client talks to.
pub(crate) fn state() -> (State, Arc<FakeDaemon>) {
    let spec = HostSpec {
        id: "local".into(),
        name: "This computer".into(),
        endpoint: None,
    };
    state_on(spec)
}

/// A state of the one host `spec` that has heard [`frames`], and the daemon its client talks to.
pub(crate) fn state_on(spec: HostSpec) -> (State, Arc<FakeDaemon>) {
    let daemon = FakeDaemon::shared();
    let client = DaemonClient::new(Arc::<FakeDaemon>::clone(&daemon), local());
    let id = spec.id.clone();
    let mut state = State::new(vec![HostSlot::new(spec, client)]);
    for frame in frames() {
        state.apply(&id, frame);
    }
    (state, daemon)
}
