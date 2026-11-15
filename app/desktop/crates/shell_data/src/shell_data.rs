//! The shell's daemon data: one stream per host feeding the sidebar, and the commands back.

use std::sync::Arc;

use protocol::DaemonTransport;
use sidebar::{Command, RowId};
use tokio::runtime::Handle;

use crate::branch_import::BranchImport;
use crate::change_signal::ChangeSignal;
use crate::daemon_client::DaemonClient;
use crate::dial::Dialled;
use crate::error::{Error, Result};
use crate::fleet::{Fleet, relay_for, slot_for};
use crate::folder_browse::FolderBrowse;
use crate::folder_choice::{self, FolderChoice};
use crate::host_desk::HostDesk;
use crate::host_spec::HostSpec;
use crate::hub::Hub;
use crate::icon_cache::IconCache;
use crate::outcome::OutcomeSink;
use crate::pairing_session::PairingSession;
use crate::panel_loader;
use crate::setting_file::SettingFile;
use crate::settings_core::SettingsCore;
use crate::sidebar_commands::SidebarCommands;
use crate::sidebar_view::SidebarView;
use crate::state::State;
use crate::system_fonts::SystemFonts;

/// Owns the stream tasks; dropping it stops them.
pub struct ShellData {
    hub: Arc<Hub>,
    commands: SidebarCommands,
    fleet: Arc<Fleet>,
    desk: HostDesk,
}

impl std::fmt::Debug for ShellData {
    fn fmt(
        &self,
        formatter: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        formatter
            .debug_struct("ShellData")
            .field("streams", &self.fleet.len())
            .finish_non_exhaustive()
    }
}

impl ShellData {
    /// Follows `hosts` through `transport`'s relay. `signal` is called after every change and
    /// `outcome` when a command needs the shell to act.
    ///
    /// A host whose id the transport cannot address is skipped and logged.
    pub fn start(
        handle: &Handle,
        transport: &Arc<dyn DaemonTransport>,
        (hosts, desk): (Vec<HostSpec>, HostDesk),
        (signal, outcome): (ChangeSignal, OutcomeSink),
    ) -> Self {
        let dialled = hosts
            .into_iter()
            .filter_map(|spec| relay_for(transport, spec))
            .collect();
        Self::start_with(handle, transport, (dialled, desk), (signal, outcome))
    }

    pub(crate) fn start_with(
        handle: &Handle,
        transport: &Arc<dyn DaemonTransport>,
        (hosts, desk): (Vec<Dialled>, HostDesk),
        (signal, outcome): (ChangeSignal, OutcomeSink),
    ) -> Self {
        let (slots, dials): (Vec<_>, Vec<_>) = hosts
            .into_iter()
            .filter_map(|(spec, dial)| slot_for(transport, spec).map(|slot| (slot, dial)))
            .unzip();
        let followed: Vec<(String, DaemonClient)> = slots
            .iter()
            .map(|slot| (slot.spec.id.clone(), slot.client.clone()))
            .collect();
        let mut state = State::new(slots);
        state.icons = IconCache::in_dir(desk.notes.directory().join("space-icons"));
        let hub = Hub::new(state, signal);
        let fleet = Fleet::new(handle, &hub, transport);
        followed
            .into_iter()
            .zip(dials)
            .for_each(|(followed, dial)| fleet.run(followed, dial));
        let commands = SidebarCommands {
            hub: Arc::clone(&hub),
            handle: handle.clone(),
            outcome,
            fleet: Arc::clone(&fleet),
        };
        Self {
            hub,
            commands,
            fleet,
            desk,
        }
    }

    /// The sidebar's read side.
    pub fn view(&self) -> SidebarView {
        SidebarView::new(Arc::clone(&self.hub))
    }

    /// The on-disk root of the workspace the Files panel follows; `None` when none is followed
    /// or its host is another computer.
    pub fn workspace_root(&self) -> Option<std::path::PathBuf> {
        self.hub.read(crate::workspace_root::of)
    }

    /// The sidebar's write side.
    pub fn commands(&self) -> SidebarCommands {
        self.commands.clone()
    }

    /// Whether `command` is carried out here; the others are the shell's own.
    pub fn serves(command: &Command) -> bool {
        !matches!(
            command,
            Command::OpenSettings
                | Command::OpenHostSettings(_)
                | Command::OpenRow(_)
                | Command::ImportRepo
                | Command::ImportBranches(_)
                | Command::OpenFile(_)
                | Command::RevealFile { .. }
                | Command::OpenAllFiles { .. }
                | Command::OpenTerminalAt { .. }
                | Command::UploadFile { .. }
                | Command::GenerateImage { .. }
                | Command::AddFolderToWorkspace
                | Command::CreateEnvTemplate { .. }
        )
    }

    /// The import dialog's data for the repository header or branch `row`; the dialog loads the
    /// remote branches and calls `signal` as they arrive.
    ///
    /// # Errors
    /// [`Error::Stream`] when `row` is not a repository of a followed host.
    pub fn branch_import(
        &self,
        row: &RowId,
        signal: ChangeSignal,
    ) -> Result<Arc<BranchImport>> {
        let parts = (&self.hub, &self.commands.handle, &self.commands.outcome);
        BranchImport::open(parts, row.as_str(), signal)
    }

    /// How the folder of a new repository is chosen for the open space.
    pub fn folder_choice(&self) -> FolderChoice {
        self.hub.read(folder_choice::of)
    }

    /// The remote folder picker's data for the host of the open space; the picker lists
    /// directories as they are opened and calls `signal` as the answers arrive.
    ///
    /// # Errors
    /// [`Error::Stream`] when no space is open.
    pub fn folder_browse(
        &self,
        signal: ChangeSignal,
    ) -> Result<Arc<FolderBrowse>> {
        FolderBrowse::open((&self.hub, &self.commands.handle), signal)
    }

    /// The settings the modal edits: the shell's file at `file`, and this computer's daemon.
    ///
    /// # Errors
    /// [`Error::File`] or [`Error::Json`] when the settings file exists and cannot be read, and
    /// [`Error::Stream`] when no local host is followed.
    pub fn settings(
        &self,
        file: std::path::PathBuf,
        signal: ChangeSignal,
        fonts: SystemFonts,
    ) -> Result<Arc<SettingsCore>> {
        let local = self
            .hub
            .read(State::local_client)
            .ok_or_else(|| Error::Stream("no local host is followed".to_owned()))?;
        let parts = (
            SettingFile::new(file),
            Arc::clone(&self.hub),
            self.commands.handle.clone(),
        );
        SettingsCore::open(
            parts,
            (local, signal),
            (fonts, Arc::clone(&self.commands.outcome)),
            (Arc::clone(&self.fleet), self.desk.clone()),
        )
    }

    /// A fresh attempt at pairing a host, signalling `signal` after each of its steps.
    pub fn pairing(
        &self,
        signal: ChangeSignal,
    ) -> Arc<PairingSession> {
        PairingSession::new(
            (
                Arc::clone(&self.hub),
                Arc::clone(&self.fleet),
                self.desk.clone(),
            ),
            self.commands.handle.clone(),
            signal,
        )
    }

    /// Follows the chat of the focused pane: the Files and Changes panels show its workspace.
    /// Following the chat already followed does nothing.
    pub fn focus_chat(
        &self,
        chat: Option<&str>,
    ) {
        let client = self.hub.edit(|state| {
            let followed = state.panels.chat.as_deref() == chat;
            state.panels.follow(chat.map(str::to_owned));
            chat.filter(|_| !followed)
                .and_then(|chat| state.client_of_chat(chat))
        });
        let (Some(chat), Some(client)) = (chat.map(str::to_owned), client) else {
            return;
        };
        let hub = Arc::clone(&self.hub);
        self.commands.handle.spawn(async move {
            panel_loader::load_directory(&hub, &client, (&chat, "")).await;
            panel_loader::load_changes(&hub, &client, &chat).await;
        });
    }
}

impl Drop for ShellData {
    fn drop(&mut self) {
        self.fleet.stop_all();
    }
}

#[cfg(test)]
mod tests {
    use std::sync::{Arc, Mutex};

    use crate::dial::Dial;
    use protocol::DaemonTransport;
    use sidebar::{Command, CommandSink as _, ProjectId, RowId, ScopeSync, SidebarData as _};
    use tokio::sync::{Notify, mpsc};

    use super::*;
    use crate::connection::Connection;
    use crate::dial::Opening;
    use crate::outcome::Outcome;
    use crate::test_support::{FakeDaemon, frames};

    fn refused_when_used(taken: Option<Connection>) -> Result<Connection> {
        taken.ok_or_else(|| Error::Stream("used".into()))
    }

    /// One connection, taken on the first dial.
    struct Once(Mutex<Option<Connection>>);

    impl Dial for Once {
        fn open(&self) -> Opening<'_> {
            let taken = self.0.lock().ok().and_then(|mut slot| slot.take());
            Box::pin(std::future::ready(refused_when_used(taken)))
        }
    }

    fn entity_texts() -> Vec<String> {
        frames()
        .into_iter()
        .filter_map(|frame| match frame {
            crate::frame::Frame::Entity { kind, id, data, .. } => Some(
                serde_json::json!({"t": "entity", "scope": "projects", "kind": kind, "id": id, "v": 1, "data": data})
                    .to_string(),
            ),
            _ => None,
        })
        .collect()
    }

    type Opened = Arc<Mutex<Vec<Outcome>>>;

    struct Rig {
        data: ShellData,
        frames_in: mpsc::Sender<String>,
        requests: mpsc::UnboundedReceiver<String>,
        changed: Arc<Notify>,
        opened: Opened,
        _dir: tempfile::TempDir,
    }

    fn start() -> Rig {
        let daemon = FakeDaemon::shared();
        daemon.ok(
            "GET",
            "/v0/settings/chat/providers",
            r#"[{"id":"claude","enabled":true,"connected":true}]"#,
        );
        daemon.ok("POST", "/v0/projects/p/home/chats", "{}");
        let transport: Arc<dyn DaemonTransport> = daemon;
        let (frames_in, incoming) = mpsc::channel(64);
        let (outgoing, requests) = mpsc::unbounded_channel();
        let changed = Arc::new(Notify::new());
        let wake = Arc::clone(&changed);
        let opened = Opened::default();
        let keep = Arc::clone(&opened);
        let sinks: (ChangeSignal, OutcomeSink) = (
            Arc::new(move || wake.notify_one()),
            Arc::new(move |outcome| keep.lock().unwrap().push(outcome)),
        );
        let spec = HostSpec {
            id: "local".into(),
            name: "This computer".into(),
            endpoint: None,
        };
        let dial: Arc<dyn Dial> =
            Arc::new(Once(Mutex::new(Some(Connection { incoming, outgoing }))));
        let (desk, dir) = crate::test_support::desk();
        let hosts = (vec![(spec, dial)], desk);
        let data = ShellData::start_with(&Handle::current(), &transport, hosts, sinks);
        Rig {
            _dir: dir,
            data,
            frames_in,
            requests,
            changed,
            opened,
        }
    }

    impl Rig {
        async fn hear_the_space(&mut self) {
            self.frames_in
                .send(r#"{"t":"ready","epoch":"e"}"#.to_owned())
                .await
                .unwrap();
            self.requests.recv().await.unwrap();
            send_entities(&self.frames_in).await;
            self.frames_in
                .send(r#"{"t":"synced","scope":"projects","unread":[]}"#.into())
                .await
                .unwrap();
            until_ready(&self.data, &self.changed).await;
        }
    }

    async fn send_entities(frames_in: &mpsc::Sender<String>) {
        for text in entity_texts() {
            frames_in.send(text).await.unwrap();
        }
    }

    async fn until_ready(
        data: &ShellData,
        changed: &Notify,
    ) {
        let view = data.view();
        while view.sync() != ScopeSync::Ready {
            changed.notified().await;
        }
    }

    async fn until_opened(
        opened: &Opened,
        changed: &Notify,
    ) {
        while opened.lock().unwrap().is_empty() {
            changed.notified().await;
        }
    }

    async fn entities_from_the_stream_reach_the_sidebar() {
        let mut rig = start();
        let view = rig.data.view();
        assert_eq!(
            view.sync(),
            ScopeSync::Pending,
            "nothing has been heard yet"
        );
        rig.hear_the_space().await;
        let rows = view.tree(&ProjectId::from("p")).rows;
        assert!(
            rows.iter()
                .any(|row| row.id.as_str() == "hc" && row.label == "Hello")
        );
    }

    async fn commands_reach_the_daemon_and_the_shells_own_are_refused() {
        let mut rig = start();
        rig.hear_the_space().await;
        let commands = rig.data.commands();
        commands
            .dispatch(Command::NewThread(ProjectId::from("p")))
            .unwrap();
        until_opened(&rig.opened, &rig.changed).await;
        assert!(matches!(
            rig.opened.lock().unwrap()[0],
            Outcome::OpenChat(_)
        ));
        assert!(
            commands
                .dispatch(Command::Lock(RowId::from("ghost")))
                .is_err()
        );
        assert!(commands.dispatch(Command::ImportRepo).is_err());
    }

    fn the_shells_own_commands_are_not_served_here() {
        assert!(!ShellData::serves(&Command::OpenSettings));
        assert!(!ShellData::serves(&Command::OpenRow(RowId::from("x"))));
        assert!(ShellData::serves(&Command::NewThread(ProjectId::from("p"))));
    }

    type Case = fn() -> std::pin::Pin<Box<dyn Future<Output = ()>>>;

    fn the_shells_own_as_a_case() -> std::pin::Pin<Box<dyn Future<Output = ()>>> {
        the_shells_own_commands_are_not_served_here();
        Box::pin(std::future::ready(()))
    }

    const CASES: [Case; 3] = [
        || Box::pin(entities_from_the_stream_reach_the_sidebar()),
        || Box::pin(commands_reach_the_daemon_and_the_shells_own_are_refused()),
        the_shells_own_as_a_case,
    ];

    async fn run_all() {
        for case in CASES {
            case().await;
        }
    }

    #[tokio::test]
    async fn every_gateway_call_reaches_its_layer() {
        run_all().await;
    }
}
