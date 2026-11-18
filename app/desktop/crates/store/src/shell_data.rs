//! The shell's daemon data: one stream per host feeding the sidebar, and the commands back.

use std::sync::Arc;

use protocol::DaemonTransport;
use sidebar::Command;
use tokio::runtime::Handle;

use crate::adapters::DiskFiles;
use crate::chat_terminal::ChatTerminal;
use crate::daemon_client::DaemonClient;
use crate::dial::Dialled;
use crate::fleet::Fleet;
use crate::host_desk::HostDesk;
use crate::host_dialling::{relay_for, slot_for};
use crate::host_spec::HostSpec;
use crate::hub::Hub;
use crate::icon_cache::IconCache;
use crate::panel_loader;
use crate::settings_slot::SettingsSlot;
use crate::shell_wiring::ShellWiring;
use crate::sidebar_commands::SidebarCommands;
use crate::sidebar_view::SidebarView;
use crate::state::State;

/// Owns the stream tasks; dropping it stops them.
pub struct ShellData {
    pub(crate) hub: Arc<Hub>,
    pub(crate) commands: SidebarCommands,
    pub(crate) fleet: Arc<Fleet>,
    pub(crate) desk: HostDesk,
    pub(crate) settings: SettingsSlot,
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
    /// Follows `hosts` through `transport`'s relay, wired to the shell by `wiring`.
    ///
    /// A host whose id the transport cannot address is skipped and logged.
    pub fn start(
        handle: &Handle,
        transport: &Arc<dyn DaemonTransport>,
        hosts: Vec<HostSpec>,
        wiring: ShellWiring,
    ) -> Self {
        let dialled = hosts
            .into_iter()
            .filter_map(|spec| relay_for(transport, spec))
            .collect();
        Self::start_with(handle, transport, dialled, wiring)
    }

    pub(crate) fn start_with(
        handle: &Handle,
        transport: &Arc<dyn DaemonTransport>,
        hosts: Vec<Dialled>,
        wiring: ShellWiring,
    ) -> Self {
        let ShellWiring {
            desk,
            signal,
            outcome,
        } = wiring;
        let (slots, dials): (Vec<_>, Vec<_>) = hosts
            .into_iter()
            .filter_map(|(spec, dial)| slot_for(transport, spec).map(|slot| (slot, dial)))
            .unzip();
        let followed: Vec<(String, DaemonClient)> = slots
            .iter()
            .map(|slot| (slot.spec.id.clone(), slot.client.clone()))
            .collect();
        let mut state = State::new(slots);
        let settings = SettingsSlot::default();
        state.settings = settings.clone();
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
            files: Arc::new(DiskFiles::default()),
        };
        Self {
            hub,
            commands,
            fleet,
            desk,
            settings,
        }
    }

    /// The surface, runner, terminal session and phase of `chat`; `None` when it is not loaded.
    pub fn chat_terminal(
        &self,
        chat: &str,
    ) -> Option<ChatTerminal> {
        self.hub.read(|state| ChatTerminal::of(state, chat))
    }

    /// The project `chat` belongs to; `None` when it is not loaded or has none.
    pub fn chat_project(
        &self,
        chat: &str,
    ) -> Option<String> {
        self.hub.read(|state| crate::chat_project::of(state, chat))
    }

    /// The sidebar's read side.
    pub fn view(&self) -> SidebarView {
        SidebarView::new(Arc::clone(&self.hub), self.settings.clone())
    }

    /// The on-disk root of the workspace the Files panel follows; `None` when none is followed
    /// or its host is another computer.
    pub fn workspace_root(&self) -> Option<std::path::PathBuf> {
        self.hub.read(crate::workspace_root::of)
    }

    /// The project the sidebar considers current: the selected one, else the first listed.
    pub fn active_project(&self) -> Option<String> {
        self.hub.read(|state| {
            crate::sidebar_reads::active_project(state).map(|id| id.as_str().to_owned())
        })
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
                | Command::ChooseSpaceFolder(_)
                | Command::ImportBranches(_)
                | Command::OpenFile(_)
                | Command::RevealFile { .. }
                | Command::OpenAllFiles { .. }
                | Command::OpenTerminalAt { .. }
                | Command::UploadFile { .. }
                | Command::UploadPaths { .. }
                | Command::GenerateImage { .. }
                | Command::AddFolderToWorkspace
                | Command::CreateEnvTemplate { .. }
                | Command::SetFileTreeFlag { .. }
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
    use crate::error::{Error, Result};
    use protocol::DaemonTransport;
    use sidebar::{
        ActiveSpace as _, Command, CommandSink as _, ProjectId, RowId, ScopeSync, SpaceList as _,
    };
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
        let spec = HostSpec {
            id: "local".into(),
            name: "This computer".into(),
            endpoint: None,
        };
        let dial: Arc<dyn Dial> =
            Arc::new(Once(Mutex::new(Some(Connection { incoming, outgoing }))));
        let desk = crate::test_support::desk();
        let wiring = ShellWiring {
            desk,
            signal: Arc::new(move || wake.notify_one()),
            outcome: Arc::new(move |outcome| keep.lock().unwrap().push(outcome)),
        };
        let data =
            ShellData::start_with(&Handle::current(), &transport, vec![(spec, dial)], wiring);
        Rig {
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
        assert!(!ShellData::serves(&Command::SetFileTreeFlag {
            flag: sidebar::FileTreeFlag::Hidden,
            on: false,
        }));
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
