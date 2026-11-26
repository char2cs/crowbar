use daemonapi::Frame;
use std::sync::{Arc, Mutex, OnceLock, PoisonError, Weak};

use protocol::HostId;

use crate::live::Entities;
use crate::live::HostSlot;
use crate::live::HostSpec;
use crate::live::Hub;
use crate::live::State;
use crate::live::{Outcome, OutcomeSink};

use super::FakeDaemon;

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
    entity_frame(kind, id, serde_json::from_str(data).unwrap_or_default())
}

/// A first version of the projects-scope entity `kind`/`id` holding `data`.
pub(crate) fn entity_frame(
    kind: &str,
    id: &str,
    data: serde_json::Value,
) -> Frame {
    Frame::Entity {
        scope: "projects".into(),
        kind: kind.into(),
        id: id.into(),
        v: 1,
        data: Some(data),
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
pub(crate) fn store_of(rows: &[(&str, &str, &str)]) -> Entities {
    let mut store = Entities::default();
    rows.iter()
        .copied()
        .map(entity)
        .for_each(|frame| store.apply(frame));
    store
}

/// A state of one local host that has heard [`frames`], and the daemon its client talks to.
/// A change signal nobody listens to.
pub(crate) fn quiet() -> crate::live::ChangeSignal {
    Arc::new(|| ())
}

/// What a watched hub noted, one entry per change.
pub(crate) type Noted<T> = Arc<Mutex<Vec<T>>>;

/// A hub over `state` that notes `read` of the state on every change it signals.
pub(crate) fn watched<T: Send + 'static>(
    state: State,
    read: impl Fn(&State) -> T + Send + Sync + 'static,
) -> (Arc<Hub>, Noted<T>) {
    let hub_of = Arc::new(OnceLock::<Weak<Hub>>::new());
    let seen = Arc::new(Mutex::new(Vec::new()));
    let (reader, heard) = (Arc::clone(&hub_of), Arc::clone(&seen));
    let signal: crate::live::ChangeSignal = Arc::new(move || {
        let hub = reader.get().and_then(Weak::upgrade);
        let noted = hub.map(|hub| hub.read(&read));
        heard.lock().unwrap().extend(noted);
    });
    let hub = Hub::new(state, signal);
    hub_of.set(Arc::downgrade(&hub)).ok();
    (hub, seen)
}

/// What an outcome sink was told, in order.
pub(crate) type Told = Arc<Mutex<Vec<Outcome>>>;

/// An outcome sink that records what it is told.
pub(crate) fn recorder() -> (OutcomeSink, Told) {
    let told = Told::default();
    let heard = Arc::clone(&told);
    let sink: OutcomeSink = Arc::new(move |said| {
        heard
            .lock()
            .unwrap_or_else(PoisonError::into_inner)
            .push(said);
    });
    (sink, told)
}

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
    let client = daemonapi::Client::new(Arc::<FakeDaemon>::clone(&daemon), local());
    let id = spec.id.clone();
    let mut state = State::new(vec![HostSlot::new(spec, client)]);
    for frame in frames() {
        state.apply(&id, frame);
    }
    (state, daemon)
}

pub(crate) fn local() -> HostId {
    HostId::new("local").unwrap_or_else(|error| panic!("local is a valid host id: {error}"))
}

/// One async test case.
pub(crate) type AsyncCase = fn() -> std::pin::Pin<Box<dyn Future<Output = ()>>>;

/// Runs each case in turn.
pub(crate) async fn run_cases(cases: &[AsyncCase]) {
    for case in cases {
        case().await;
    }
}
