use std::sync::{Arc, Mutex, PoisonError};

use settings::{Change, Key, Value};
use tokio::runtime::Handle;
use tokio::sync::Notify;

use super::{FakeDaemon, FakeFiles, state};
use crate::live::ChangeSignal;
use crate::live::Fleet;
use crate::live::Hub;
use crate::live::LinkState;
use crate::live::{Outcome, OutcomeSink};
use crate::store::SystemFonts;
use crate::store::settings_core::SettingsCore;
use crate::store::settings_core::json_file::JsonFile;
use crate::store::settings_core::parts::Parts;

pub(crate) const SETTINGS_PATH: &str = "/state/settings.json";

pub(crate) const PROVIDERS: &str = r#"[{"id":"claude","displayName":"Claude","enabled":true,"mcpEnabled":true,"connected":true},{"id":"codex","enabled":true,"mcpEnabled":true}]"#;

pub(crate) type Route = (&'static str, &'static str, &'static str);

pub(crate) const AGENTS: Route = ("GET", "/v0/settings/chat/providers", PROVIDERS);
pub(crate) const AGENTS_PUT: Route = ("PUT", "/v0/settings/chat/providers", PROVIDERS);
pub(crate) const LEVEL_GUARDED: Route = (
    "GET",
    "/v0/settings/chat/permission-level",
    r#"{"level":"guarded"}"#,
);
pub(crate) const LEVEL_TRUSTED: Route = (
    "GET",
    "/v0/settings/chat/permission-level",
    r#"{"level":"trusted"}"#,
);
pub(crate) const KILL_PATH: &str = "/v0/chats/bo/kill";
pub(crate) const KILL: Route = ("POST", KILL_PATH, "{}");

pub(crate) struct SettingsRig {
    pub(crate) core: Arc<SettingsCore>,
    pub(crate) daemon: Arc<FakeDaemon>,
    pub(crate) files: Arc<FakeFiles>,
    pub(crate) told: Told,
    changed: Arc<Notify>,
}

impl SettingsRig {
    pub(crate) fn open(routes: &[Route]) -> Self {
        Self::open_on(LinkState::Live, routes)
    }

    pub(crate) fn open_on(
        link: LinkState,
        routes: &[Route],
    ) -> Self {
        let (state, daemon) = state();
        for (method, path, data) in routes {
            daemon.ok(method, path, data);
        }
        let files: Arc<FakeFiles> = Arc::default();
        let (signal, changed) = listener();
        let (outcome, told) = recorder();
        let hub = Hub::new(state, Arc::clone(&signal));
        hub.edit(|state| state.link("local", link));
        let transport: Arc<dyn protocol::DaemonTransport> = Arc::<FakeDaemon>::clone(&daemon);
        let wiring = Parts {
            file: JsonFile::new(SETTINGS_PATH.into(), Arc::<FakeFiles>::clone(&files)),
            local: hub.read(|state| state.hosts[0].client.clone()),
            fleet: Fleet::new(&Handle::current(), &hub, (&transport, Arc::new(|_| {}))),
            desk: super::desk(),
            hub,
            handle: Handle::current(),
            signal,
            fonts: SystemFonts::default(),
            outcome,
        };
        let core = SettingsCore::open(wiring).unwrap();
        Self {
            core,
            daemon,
            files,
            told,
            changed,
        }
    }

    pub(crate) async fn changes_until(
        &self,
        done: impl Fn(&[Change]) -> bool,
    ) -> Vec<Change> {
        let mut all = self.core.take_changes();
        while !done(&all) {
            self.changed.notified().await;
            all.extend(self.core.take_changes());
        }
        all
    }

    pub(crate) async fn called(
        &self,
        path: &str,
    ) {
        while !self.daemon.seen().iter().any(|call| call.path == path) {
            tokio::task::yield_now().await;
        }
    }

    pub(crate) async fn agents_ready(&self) {
        self.changes_until(agents_ready).await;
    }

    pub(crate) fn announced(
        &self,
        key: &str,
        text: &str,
    ) -> bool {
        self.core
            .take_changes()
            .contains(&Change::Setting(Key::new(key), Value::Text(text.into())))
    }
}

fn listener() -> (ChangeSignal, Arc<Notify>) {
    let changed = Arc::new(Notify::new());
    let wake = Arc::clone(&changed);
    (Arc::new(move || wake.notify_one()), changed)
}

type Told = Arc<Mutex<Vec<Outcome>>>;

fn recorder() -> (OutcomeSink, Told) {
    let told = Arc::new(Mutex::new(Vec::new()));
    let heard = Arc::clone(&told);
    let sink: OutcomeSink = Arc::new(move |said| {
        heard
            .lock()
            .unwrap_or_else(PoisonError::into_inner)
            .push(said);
    });
    (sink, told)
}

pub(crate) fn agents_ready(changes: &[Change]) -> bool {
    changes.contains(&Change::Records("agents".into()))
}
