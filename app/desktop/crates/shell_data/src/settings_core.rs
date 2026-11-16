//! Settings and record lists over the daemon and the shell's own settings file.

use std::collections::BTreeMap;
use std::sync::{Arc, Mutex, PoisonError};

use settings_ui::{Change, Error, Key, RecordCommand, Records, Snapshot, Value, Values};
use tokio::runtime::Handle;

use crate::agent_records::{self, ENABLED, TOOLS};
use crate::call::Call;
use crate::change_signal::ChangeSignal;
use crate::daemon_client::DaemonClient;
use crate::descriptor_records;
use crate::descriptor_report::DescriptorReport;
use crate::descriptors;
use crate::fleet::Fleet;
use crate::font_choices;
use crate::host_desk::HostDesk;
use crate::host_roster::HostRoster;
use crate::hub::Hub;
use crate::link_state::LinkState;
use crate::live_records::{self, KILL, REMOVE_ANYWAY, REVOKE};
use crate::loadable::Loadable;
use crate::outcome::{Outcome, OutcomeSink};
use crate::provider::Provider;
use crate::providers;
use crate::runner;
use crate::setting_file::SettingFile;
use crate::state::State;
use crate::switch::Switch;
use crate::system_fonts::SystemFonts;
use crate::typed::typed;

const PERMISSION: &str = "defaultPermissionLevel";
const PERMISSION_PATH: &str = "/v0/settings/chat/permission-level";
const PROVIDERS_PATH: &str = "/v0/settings/chat/providers";
const AGENTS: &str = "agents";
const SESSIONS: &str = "sessions";
const HOSTS: &str = "hosts";
const DESCRIPTORS: &str = "descriptors";
const UPLOAD_DESCRIPTOR: &str = "agents.uploadDescriptor";
const ADD_HOST: &str = "hosts.add";
const CLEAR_BROWSING_DATA: &str = "browser.clearData";

/// Thread-safe state behind the settings modal's backend: it holds the values, the agent list,
/// and the changes the UI thread has yet to be told about.
pub struct SettingsCore {
    file: SettingFile,
    values: Mutex<BTreeMap<String, Value>>,
    agents: Mutex<Loadable<Vec<Provider>>>,
    descriptors: Mutex<Loadable<Vec<DescriptorReport>>>,
    outbox: Mutex<Vec<Change>>,
    hub: Arc<Hub>,
    handle: Handle,
    local: DaemonClient,
    signal: ChangeSignal,
    fonts: SystemFonts,
    outcome: OutcomeSink,
    roster: Arc<HostRoster>,
}

impl std::fmt::Debug for SettingsCore {
    fn fmt(
        &self,
        formatter: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        formatter
            .debug_struct("SettingsCore")
            .finish_non_exhaustive()
    }
}

fn local_link(state: &State) -> Option<LinkState> {
    state
        .hosts
        .iter()
        .find(|slot| slot.spec.endpoint.is_none())
        .map(|slot| slot.link)
}

fn answered(state: &State) -> Option<LinkState> {
    local_link(state).filter(|link| *link != LinkState::Connecting)
}

fn live(state: &State) -> Option<LinkState> {
    local_link(state).filter(|link| *link == LinkState::Live)
}

fn locked<T>(mutex: &Mutex<T>) -> std::sync::MutexGuard<'_, T> {
    mutex.lock().unwrap_or_else(PoisonError::into_inner)
}

impl SettingsCore {
    pub(crate) fn open(
        (file, hub, handle): (SettingFile, Arc<Hub>, Handle),
        (local, signal): (DaemonClient, ChangeSignal),
        (fonts, outcome): (SystemFonts, OutcomeSink),
        (fleet, desk): (Arc<Fleet>, HostDesk),
    ) -> Result<Arc<Self>, crate::error::Error> {
        let values = file.load()?;
        let roster = HostRoster::new(
            (Arc::clone(&hub), fleet, desk),
            (Arc::clone(&outcome), Arc::clone(&signal)),
        );
        let core = Arc::new(Self {
            file,
            values: Mutex::new(values),
            agents: Mutex::new(Loadable::Loading),
            descriptors: Mutex::new(Loadable::Loading),
            outbox: Mutex::new(Vec::new()),
            hub,
            handle,
            local,
            signal,
            fonts,
            outcome,
            roster,
        });
        core.refresh();
        Ok(core)
    }

    fn refresh(self: &Arc<Self>) {
        let core = Arc::clone(self);
        self.handle.spawn(async move {
            core.when_the_daemon_answers().await;
            core.read_permission().await;
            core.read_agents().await;
            core.read_descriptors().await;
        });
    }

    async fn when_the_daemon_answers(&self) {
        let reached = self.hub.until(answered).await;
        if reached == Some(LinkState::Live) {
            return;
        }
        *locked(&self.agents) = Loadable::Failed;
        *locked(&self.descriptors) = Loadable::Failed;
        self.push(Change::Records(AGENTS.to_owned()));
        self.push(Change::Records(DESCRIPTORS.to_owned()));
        self.hub.until(live).await;
    }

    async fn read_descriptors(&self) {
        let next = match descriptors::list(&self.local).await {
            Ok(all) => Loadable::Ready(all),
            Err(error) => {
                tracing::warn!(%error, "the descriptors could not be listed");
                Loadable::Failed
            },
        };
        *locked(&self.descriptors) = next;
        self.push(Change::Records(DESCRIPTORS.to_owned()));
    }

    /// Installs the YAML descriptor `text` as a new agent, then reloads the agents and the checks.
    pub fn upload_descriptor(
        self: &Arc<Self>,
        text: Vec<u8>,
    ) {
        let core = Arc::clone(self);
        self.handle.spawn(async move {
            core.install_descriptor(text).await;
        });
    }

    async fn install_descriptor(
        &self,
        text: Vec<u8>,
    ) {
        if let Err(error) = descriptors::upload(&self.local, text).await {
            (self.outcome)(Outcome::Failed(runner::reason_of(&error)));
        }
        self.read_agents().await;
        self.read_descriptors().await;
    }

    async fn read_permission(&self) {
        let answer = self.local.send(Call::get(PERMISSION_PATH)).await;
        let level = answer
            .inspect_err(|error| {
                tracing::warn!(%error, "the default permission level could not be read");
            })
            .ok()
            .and_then(|data| data.get("level")?.as_str().map(str::to_owned));
        let Some(level) = level else {
            return;
        };
        locked(&self.values).insert(PERMISSION.to_owned(), Value::Text(level.clone()));
        self.push(Change::Setting(Key::new(PERMISSION), Value::Text(level)));
    }

    async fn read_agents(&self) {
        let next = match providers::list(&self.local).await {
            Ok(all) => Loadable::Ready(all),
            Err(error) => {
                tracing::warn!(%error, "the agents could not be listed");
                Loadable::Failed
            },
        };
        *locked(&self.agents) = next;
        self.push(Change::Records(AGENTS.to_owned()));
    }

    fn push(
        &self,
        change: Change,
    ) {
        locked(&self.outbox).push(change);
        (self.signal)();
    }

    /// The changes made since the last call, for the UI thread to deliver.
    pub fn take_changes(&self) -> Vec<Change> {
        std::mem::take(&mut *locked(&self.outbox))
    }

    /// The hub changed, so the lists that read it may have.
    pub fn hub_changed(&self) {
        locked(&self.outbox).extend([
            Change::Records(SESSIONS.to_owned()),
            Change::Records(HOSTS.to_owned()),
        ]);
    }

    /// Everything the modal needs before its first paint.
    pub fn snapshot(&self) -> Snapshot {
        let mut values = Values::default();
        for (key, value) in locked(&self.values).iter() {
            values.set(Key::new(key.as_str()), value.clone());
        }
        Snapshot::new(values)
            .with_capability(CLEAR_BROWSING_DATA)
            .with_choices("themes", font_choices::themes())
            .with_choices("fonts", font_choices::fonts(&self.fonts))
            .with_choices("monoFonts", font_choices::monospace(&self.fonts))
            .with_choices("terminalFonts", font_choices::monospace(&self.fonts))
    }

    /// The stored value of `key`.
    pub fn get(
        &self,
        key: &Key,
    ) -> Option<Value> {
        locked(&self.values).get(key.as_str()).cloned()
    }

    /// Stores `value`; the permission level goes to the daemon, everything else to the file.
    ///
    /// # Errors
    /// [`Error::Unavailable`] when the settings file cannot be written.
    pub fn set(
        self: &Arc<Self>,
        key: &Key,
        value: Value,
    ) -> Result<(), Error> {
        let level = value.as_text().unwrap_or_default().to_owned();
        let announced = value.clone();
        let previous = locked(&self.values).insert(key.as_str().to_owned(), value);
        if key.as_str() == PERMISSION {
            self.send_permission(previous, level);
            return Ok(());
        }
        let saved = self.file.save(&self.local_values());
        if let Err(error) = saved {
            self.restore(key.as_str(), previous);
            return Err(Error::Unavailable(error.to_string()));
        }
        self.push(Change::Setting(key.clone(), announced));
        Ok(())
    }

    fn local_values(&self) -> BTreeMap<String, Value> {
        let mut values = locked(&self.values).clone();
        values.remove(PERMISSION);
        values
    }

    fn restore(
        &self,
        key: &str,
        previous: Option<Value>,
    ) {
        let mut values = locked(&self.values);
        match previous {
            Some(value) => values.insert(key.to_owned(), value),
            None => values.remove(key),
        };
    }

    fn send_permission(
        self: &Arc<Self>,
        previous: Option<Value>,
        level: String,
    ) {
        let core = Arc::clone(self);
        self.handle
            .spawn(async move { core.put_permission(level, previous).await });
    }

    async fn put_permission(
        &self,
        level: String,
        previous: Option<Value>,
    ) {
        let call = Call::put(PERMISSION_PATH).json(serde_json::json!({"level": level}));
        let Err(error) = self.local.send(call).await else {
            return;
        };
        tracing::warn!(%error, "the default permission level was not saved");
        self.restore(PERMISSION, previous.clone());
        if let Some(old) = previous {
            self.push(Change::Setting(Key::new(PERMISSION), old));
        }
    }

    /// Runs the backend action `id`.
    ///
    /// # Errors
    /// [`Error::Unavailable`] for an action this shell does not offer.
    pub fn invoke(
        &self,
        id: &str,
    ) -> Result<(), Error> {
        if id == UPLOAD_DESCRIPTOR {
            (self.outcome)(Outcome::PickDescriptor);
            return Ok(());
        }
        if id == ADD_HOST {
            (self.outcome)(Outcome::PairHost);
            return Ok(());
        }
        if id == CLEAR_BROWSING_DATA {
            (self.outcome)(Outcome::ClearBrowsingData);
            return Ok(());
        }
        Err(Error::Unavailable(format!("{id} is not available here")))
    }

    /// The content of the list `source`.
    pub fn records(
        &self,
        source: &str,
    ) -> Records {
        match source {
            AGENTS => locked(&self.agents).records(|all| agent_records::of(all)),
            SESSIONS => Records::Ready(self.hub.read(live_records::sessions)),
            HOSTS => Records::Ready(self.hub.read(live_records::hosts)),
            DESCRIPTORS => locked(&self.descriptors).records(|all| descriptor_records::of(all)),
            _ => Records::Failed,
        }
    }

    /// Applies `command` to `record` of `source`.
    ///
    /// # Errors
    /// [`Error::Unavailable`] when the list is not editable or the owner is not loaded.
    pub fn apply(
        self: &Arc<Self>,
        source: &str,
        record: &str,
        command: RecordCommand,
    ) -> Result<(), Error> {
        match (source, command) {
            (AGENTS, command) => self.edit_agents(record, command),
            (SESSIONS, RecordCommand::Run { action }) if action == KILL => self.kill(record),
            (HOSTS, RecordCommand::Run { action }) if action == REVOKE => {
                self.roster.revoke(&self.handle, record);
                Ok(())
            },
            (HOSTS, RecordCommand::Run { action }) if action == REMOVE_ANYWAY => {
                self.roster.remove_anyway(&self.handle, record);
                Ok(())
            },
            (_, RecordCommand::Refresh) => {
                self.refresh();
                Ok(())
            },
            (source, _) => Err(Error::Unavailable(format!(
                "the {source} list cannot be changed from here"
            ))),
        }
    }

    fn edit_agents(
        self: &Arc<Self>,
        record: &str,
        command: RecordCommand,
    ) -> Result<(), Error> {
        let mut agents = locked(&self.agents);
        let Loadable::Ready(all) = &mut *agents else {
            return Err(Error::Unavailable("the agents are not loaded".to_owned()));
        };
        let before = all.clone();
        let at = all
            .iter()
            .position(|provider| provider.id == record)
            .ok_or_else(|| Error::Unavailable(format!("{record} is not an agent of this host")))?;
        change_agent(all, at, command)?;
        let body = agent_records::preferences(all);
        drop(agents);
        self.send_agents(body, before);
        Ok(())
    }

    fn send_agents(
        self: &Arc<Self>,
        body: serde_json::Value,
        before: Vec<Provider>,
    ) {
        let core = Arc::clone(self);
        self.handle
            .spawn(async move { core.put_agents(body, before).await });
    }

    async fn put_agents(
        &self,
        body: serde_json::Value,
        before: Vec<Provider>,
    ) {
        let saved = self
            .local
            .send(Call::put(PROVIDERS_PATH).json(body))
            .await
            .and_then(|data| typed::<Vec<Provider>>(PROVIDERS_PATH, data));
        let next = saved.unwrap_or_else(|error| {
            tracing::warn!(%error, "the agent preferences were not saved");
            before
        });
        *locked(&self.agents) = Loadable::Ready(next);
        self.push(Change::Records(AGENTS.to_owned()));
    }

    fn kill(
        self: &Arc<Self>,
        chat: &str,
    ) -> Result<(), Error> {
        let client = self
            .hub
            .read(|state| state.client_of_chat(chat))
            .ok_or_else(|| Error::Unavailable("this session is not on a known host".to_owned()))?;
        let path = format!("/v0/chats/{chat}/kill");
        self.handle.spawn(end_session(client, path));
        Ok(())
    }
}

fn change_agent(
    all: &mut Vec<Provider>,
    at: usize,
    command: RecordCommand,
) -> Result<(), Error> {
    match command {
        RecordCommand::SetFlag { flag, on } if flag == ENABLED => {
            enable(all.get_mut(at), Some(on).into())
        },
        RecordCommand::SetFlag { flag, on } if flag == TOOLS => {
            enable_tools(all.get_mut(at), Some(on).into())
        },
        RecordCommand::SetFlag { .. } => Err(Error::Unavailable(
            "whether an agent is installed is detected by the daemon".to_owned(),
        )),
        RecordCommand::Move { to } => {
            let moved = all.remove(at);
            all.insert(to.min(all.len()), moved);
            Ok(())
        },
        _ => Err(Error::Unavailable("agents have no such action".to_owned())),
    }
}

fn enable(
    provider: Option<&mut Provider>,
    on: Switch,
) -> Result<(), Error> {
    edit_provider(provider, |held| held.enabled = on)
}

fn enable_tools(
    provider: Option<&mut Provider>,
    on: Switch,
) -> Result<(), Error> {
    edit_provider(provider, |held| held.mcp_enabled = on)
}

fn edit_provider(
    provider: Option<&mut Provider>,
    change: impl FnOnce(&mut Provider),
) -> Result<(), Error> {
    provider
        .map(change)
        .ok_or_else(|| Error::Unavailable("that agent is gone".to_owned()))
}

async fn end_session(
    client: DaemonClient,
    path: String,
) {
    if let Err(error) = client.send(Call::post(path)).await {
        tracing::warn!(%error, "a session could not be ended");
    }
}

#[cfg(test)]
mod tests {
    use serde_json::json;
    use tokio::sync::Notify;

    use super::*;
    use crate::test_support::{FakeDaemon, state};

    struct Rig {
        core: Arc<SettingsCore>,
        daemon: Arc<FakeDaemon>,
        changed: Arc<Notify>,
        dir: tempfile::TempDir,
        told: Arc<Mutex<Vec<Outcome>>>,
    }

    const PROVIDERS: &str = r#"[{"id":"claude","displayName":"Claude","enabled":true,"mcpEnabled":true,"connected":true},{"id":"codex","enabled":true,"mcpEnabled":true}]"#;

    fn open(daemon_setup: impl FnOnce(&FakeDaemon)) -> Rig {
        open_on(LinkState::Live, daemon_setup)
    }

    fn open_on(
        link: LinkState,
        daemon_setup: impl FnOnce(&FakeDaemon),
    ) -> Rig {
        let (state, daemon) = state();
        daemon_setup(&daemon);
        let dir = tempfile::tempdir().unwrap();
        let changed = Arc::new(Notify::new());
        let wake = Arc::clone(&changed);
        let signal: ChangeSignal = Arc::new(move || wake.notify_one());
        let told = Arc::new(Mutex::new(Vec::new()));
        let heard = Arc::clone(&told);
        let sink: OutcomeSink = Arc::new(move |outcome| locked(&heard).push(outcome));
        let hub = Hub::new(state, Arc::clone(&signal));
        hub.edit(|state| state.link("local", link));
        let local = hub.read(|state| state.hosts[0].client.clone());
        let transport: Arc<dyn protocol::DaemonTransport> = Arc::<FakeDaemon>::clone(&daemon);
        let fleet = Fleet::new(&Handle::current(), &hub, &transport);
        let desk = HostDesk {
            book: Arc::new(crate::test_support::FakeBook::default()),
            notes: Arc::new(
                crate::host_notes::HostNotes::open(dir.path().join("hosts.json")).unwrap(),
            ),
        };
        let file = SettingFile::new(dir.path().join("settings.json"));
        let core = SettingsCore::open(
            (file, hub, Handle::current()),
            (local, signal),
            (SystemFonts::default(), sink),
            (fleet, desk),
        )
        .unwrap();
        Rig {
            core,
            daemon,
            changed,
            dir,
            told,
        }
    }

    async fn collect_until(
        core: &SettingsCore,
        changed: &Notify,
        done: impl Fn(&[Change]) -> bool,
    ) -> Vec<Change> {
        let mut all = core.take_changes();
        while !done(&all) {
            changed.notified().await;
            all.extend(core.take_changes());
        }
        all
    }

    impl Rig {
        /// Collects changes until `done` holds for them.
        async fn changes_until(
            &self,
            done: impl Fn(&[Change]) -> bool,
        ) -> Vec<Change> {
            collect_until(&self.core, &self.changed, done).await
        }
    }

    fn agents_ready(changes: &[Change]) -> bool {
        changes.contains(&Change::Records("agents".into()))
    }

    async fn the_agents_and_the_permission_level_come_from_the_daemon() {
        let rig = open(|daemon| {
            daemon.ok(
                "GET",
                "/v0/settings/chat/permission-level",
                r#"{"level":"trusted"}"#,
            );
            daemon.ok("GET", "/v0/settings/chat/providers", PROVIDERS);
        });
        assert_eq!(rig.core.records("agents"), Records::Loading);
        rig.changes_until(agents_ready).await;
        assert!(matches!(rig.core.records("agents"), Records::Ready(list) if list.len() == 2));
        assert_eq!(
            rig.core.get(&Key::new("defaultPermissionLevel")),
            Some(Value::Text("trusted".into()))
        );
    }

    async fn the_lists_wait_for_the_daemon_link_instead_of_failing_on_its_startup() {
        let rig = open_on(LinkState::Connecting, |daemon| {
            daemon.ok("GET", "/v0/settings/chat/providers", PROVIDERS);
        });
        tokio::task::yield_now().await;
        assert_eq!(rig.core.records("agents"), Records::Loading);
        rig.core
            .hub
            .edit(|state| state.link("local", LinkState::Live));
        rig.changes_until(agents_ready).await;
        assert!(matches!(rig.core.records("agents"), Records::Ready(list) if list.len() == 2));
    }

    async fn a_link_that_is_down_fails_the_lists_until_it_comes_up() {
        let rig = open_on(LinkState::Down, |daemon| {
            daemon.ok("GET", "/v0/settings/chat/providers", PROVIDERS);
        });
        rig.changes_until(agents_ready).await;
        assert_eq!(rig.core.records("agents"), Records::Failed);
        rig.core
            .hub
            .edit(|state| state.link("local", LinkState::Live));
        rig.changes_until(agents_ready).await;
        assert!(matches!(rig.core.records("agents"), Records::Ready(list) if list.len() == 2));
    }

    async fn an_unreachable_daemon_fails_the_agent_list() {
        let rig = open(|_| {});
        rig.changes_until(agents_ready).await;
        assert_eq!(rig.core.records("agents"), Records::Failed);
    }

    fn announced(
        core: &SettingsCore,
        key: &str,
        text: &str,
    ) -> bool {
        core.take_changes()
            .contains(&Change::Setting(Key::new(key), Value::Text(text.into())))
    }

    async fn shell_settings_persist_to_the_file_and_the_permission_level_reverts_when_refused() {
        let rig = open(|daemon| {
            daemon.ok(
                "GET",
                "/v0/settings/chat/permission-level",
                r#"{"level":"guarded"}"#,
            );
            daemon.ok("GET", "/v0/settings/chat/providers", PROVIDERS);
        });
        rig.changes_until(agents_ready).await;
        rig.core
            .set(&Key::new("theme"), Value::Text("dark".into()))
            .unwrap();
        let saved = std::fs::read_to_string(rig.dir.path().join("settings.json")).unwrap();
        assert!(saved.contains("dark") && !saved.contains("defaultPermissionLevel"));
        assert!(
            announced(&rig.core, "theme", "dark"),
            "stored settings are announced"
        );
        rig.core
            .set(
                &Key::new("defaultPermissionLevel"),
                Value::Text("full-auto".into()),
            )
            .unwrap();
        let changes = rig
            .changes_until(|all| {
                all.contains(&Change::Setting(
                    Key::new("defaultPermissionLevel"),
                    Value::Text("guarded".into()),
                ))
            })
            .await;
        assert_ne!(changes.len(), 0);
        assert_eq!(
            rig.core.get(&Key::new("defaultPermissionLevel")),
            Some(Value::Text("guarded".into()))
        );
    }

    async fn agent_flags_and_order_are_put_as_the_whole_preference_list() {
        let rig = open(|daemon| {
            daemon.ok("GET", "/v0/settings/chat/providers", PROVIDERS);
            daemon.ok("PUT", "/v0/settings/chat/providers", PROVIDERS);
        });
        rig.changes_until(agents_ready).await;
        rig.core
            .apply(
                "agents",
                "codex",
                RecordCommand::SetFlag {
                    flag: "enabled".into(),
                    on: false,
                },
            )
            .unwrap();
        rig.core
            .apply("agents", "codex", RecordCommand::Move { to: 0 })
            .unwrap();
        rig.changes_until(|all| {
            all.iter()
                .filter(|change| agents_ready(std::slice::from_ref(change)))
                .count()
                >= 2
        })
        .await;
        let puts: Vec<_> = rig
            .daemon
            .seen()
            .into_iter()
            .filter(|call| call.method == "PUT")
            .collect();
        assert_eq!(
            puts[0].body.clone().unwrap()["providers"][1],
            json!({"id": "codex", "disabled": true, "mcpDisabled": false})
        );
    }

    async fn an_installed_flag_and_an_unknown_agent_cannot_be_changed() {
        let rig = open(|daemon| daemon.ok("GET", "/v0/settings/chat/providers", PROVIDERS));
        rig.changes_until(agents_ready).await;
        assert!(
            rig.core
                .apply(
                    "agents",
                    "codex",
                    RecordCommand::SetFlag {
                        flag: "connected".into(),
                        on: true
                    }
                )
                .is_err()
        );
        assert!(
            rig.core
                .apply("agents", "ghost", RecordCommand::Refresh)
                .is_err()
        );
    }

    async fn a_session_is_killed_on_its_host() {
        let rig = open(|daemon| daemon.ok("POST", "/v0/chats/bo/kill", "{}"));
        rig.core
            .apply(
                "sessions",
                "bo",
                RecordCommand::Run {
                    action: "kill".into(),
                },
            )
            .unwrap();
        while !rig
            .daemon
            .seen()
            .iter()
            .any(|call| call.path == "/v0/chats/bo/kill")
        {
            tokio::task::yield_now().await;
        }
    }

    fn unknown_sessions_are_refused_and_backend_actions_are_unavailable() {
        let rig = open(|_| {});
        assert!(
            rig.core
                .apply(
                    "sessions",
                    "ghost",
                    RecordCommand::Run {
                        action: "kill".into()
                    }
                )
                .is_err()
        );
        assert!(
            rig.core
                .apply("hosts", "local", RecordCommand::Refresh)
                .is_ok()
        );
        assert!(rig.core.invoke("hosts.bogus").is_err());
        assert!(matches!(rig.core.records("sessions"), Records::Ready(list) if list.len() == 1));
        assert!(matches!(rig.core.records("hosts"), Records::Ready(list) if list.len() == 1));
    }

    fn adding_a_host_asks_the_shell_to_open_the_pairing_dialog()
    -> std::pin::Pin<Box<dyn Future<Output = ()>>> {
        let rig = open(|_| {});
        assert!(rig.core.invoke("hosts.add").is_ok());
        assert_eq!(*locked(&rig.told), [Outcome::PairHost]);
        Box::pin(std::future::ready(()))
    }

    fn clearing_browsing_data_is_offered_and_asks_the_shell_to_clear() {
        let rig = open(|_| {});
        assert!(rig.core.invoke("browser.clearData").is_ok());
        assert_eq!(*locked(&rig.told), [Outcome::ClearBrowsingData]);
    }

    type Case = fn() -> std::pin::Pin<Box<dyn Future<Output = ()>>>;

    fn unknown_sessions_as_a_case() -> std::pin::Pin<Box<dyn Future<Output = ()>>> {
        unknown_sessions_are_refused_and_backend_actions_are_unavailable();
        Box::pin(std::future::ready(()))
    }

    const CASES: [Case; 11] = [
        || {
            clearing_browsing_data_is_offered_and_asks_the_shell_to_clear();
            Box::pin(std::future::ready(()))
        },
        adding_a_host_asks_the_shell_to_open_the_pairing_dialog,
        || Box::pin(the_agents_and_the_permission_level_come_from_the_daemon()),
        || Box::pin(the_lists_wait_for_the_daemon_link_instead_of_failing_on_its_startup()),
        || Box::pin(a_link_that_is_down_fails_the_lists_until_it_comes_up()),
        || Box::pin(an_unreachable_daemon_fails_the_agent_list()),
        || {
            Box::pin(
                shell_settings_persist_to_the_file_and_the_permission_level_reverts_when_refused(),
            )
        },
        || Box::pin(agent_flags_and_order_are_put_as_the_whole_preference_list()),
        || Box::pin(an_installed_flag_and_an_unknown_agent_cannot_be_changed()),
        || Box::pin(a_session_is_killed_on_its_host()),
        unknown_sessions_as_a_case,
    ];

    async fn run_all() {
        for case in CASES {
            case().await;
        }
    }

    #[tokio::test]
    async fn every_setting_is_served_by_its_route() {
        run_all().await;
    }
}
