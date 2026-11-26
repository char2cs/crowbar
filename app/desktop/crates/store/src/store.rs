//! The shell's daemon data: one stream per host feeding the sidebar, and the commands back.

mod branch_import;
mod chat_reads;
mod dialogs;
mod folder_browse;
mod legacy_drafts;
mod local_link;
mod pairing;
mod ports;
mod projects;
pub(crate) mod settings_core;
mod system_check;

pub use branch_import::{Availability, BranchImport, BranchListing, ImportBranch, ImportPlan};
pub use folder_browse::{BrowseEntry, FolderBrowse, FolderChoice, FolderListing, ROOT_PATH};
pub use legacy_drafts::LegacyDrafts;
pub use local_link::LocalLink;
pub use pairing::device_offer::DeviceOffer;
pub use pairing::offer::Offer;
pub use pairing::offer_state::OfferState;
pub use pairing::pairing_session::{PairingPending, PairingSession, PairingStage, PairingState};
pub use ports::Ports;
pub use projects::projects_load::ProjectsLoad;
pub use settings_core::{LegacyImport, SettingsAdapter, SystemFonts};
pub use system_check::{SystemCheck, Tool, ToolReport};

use std::sync::Arc;

use protocol::DaemonTransport;
use sidebar::Command;
use tokio::runtime::Handle;

use crate::live::DiskFiles;
use crate::live::Fleet;
use crate::live::HostDesk;
use crate::live::HostSpec;
use crate::live::Hub;
use crate::live::IconCache;
use crate::live::SettingsSlot;
use crate::live::State;
use crate::live::ViewedFiles;
use crate::live::panel_loader;
use crate::live::{Dialled, relay_for, slot_for};
use crate::sidebar_commands::SidebarCommands;
use crate::sidebar_view::SidebarView;

/// Owns the stream tasks; dropping it stops them.
pub struct Store {
    pub(crate) hub: Arc<Hub>,
    pub(crate) commands: SidebarCommands,
    pub(crate) fleet: Arc<Fleet>,
    pub(crate) desk: HostDesk,
    pub(crate) settings: SettingsSlot,
}

impl std::fmt::Debug for Store {
    fn fmt(
        &self,
        formatter: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        formatter
            .debug_struct("Store")
            .field("streams", &self.fleet.len())
            .finish_non_exhaustive()
    }
}

impl Store {
    /// Follows `hosts` through `transport`'s relay, wired to the shell by `wiring`.
    ///
    /// A host whose id the transport cannot address is skipped and logged.
    pub fn start(
        handle: &Handle,
        transport: &Arc<dyn DaemonTransport>,
        hosts: Vec<HostSpec>,
        wiring: Ports,
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
        wiring: Ports,
    ) -> Self {
        let Ports {
            desk,
            signal,
            outcome,
        } = wiring;
        let (slots, dials): (Vec<_>, Vec<_>) = hosts
            .into_iter()
            .filter_map(|(spec, dial)| slot_for(transport, spec).map(|slot| (slot, dial)))
            .unzip();
        let followed: Vec<(String, daemonapi::Client)> = slots
            .iter()
            .map(|slot| (slot.spec.id.clone(), slot.client.clone()))
            .collect();
        let mut state = State::new(slots);
        let settings = SettingsSlot::default();
        state.settings = settings.clone();
        state.icons = IconCache::in_dir(desk.notes.directory().join("space-icons"));
        state.viewed = ViewedFiles::in_dir(&desk.notes.directory());
        let hub = Hub::new(state, signal);
        let fleet = Fleet::new(handle, &hub, (transport, Arc::clone(&outcome)));
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

    /// The sidebar's read side.
    pub fn view(&self) -> SidebarView {
        SidebarView::new(Arc::clone(&self.hub), self.settings.clone())
    }

    /// The on-disk root of the workspace the Files panel follows; `None` when none is followed
    /// or its host is another computer.
    pub fn workspace_root(&self) -> Option<std::path::PathBuf> {
        self.hub.read(crate::live::workspace_root::of)
    }

    /// The project the sidebar considers current: the selected one, else the first listed.
    pub fn active_project(&self) -> Option<String> {
        self.hub
            .read(|state| crate::reads::active_project(state).map(|id| id.as_str().to_owned()))
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
                | Command::PreviewFile(_)
                | Command::OpenReview { .. }
                | Command::RevealFile { .. }
                | Command::OpenAllFiles { .. }
                | Command::OpenTerminalAt { .. }
                | Command::UploadFile { .. }
                | Command::UploadPaths { .. }
                | Command::GenerateImage { .. }
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
        self.hub
            .edit(|state| state.panels.follow(chat.map(str::to_owned)));
        self.fleet
            .steer(None, crate::live::StreamControl::FollowFiles);
        panel_loader::followed_ask::ask(&self.hub, (&self.commands.handle, &self.commands.outcome));
    }

    /// Watches the files of every chat an open tab shows, as develop holds the `files:` scope of
    /// each open workspace, so a file open in a chat that is not focused hears its changes on
    /// disk. Watching the chats already watched does nothing.
    pub fn watch_chats(
        &self,
        chats: std::collections::BTreeSet<String>,
    ) {
        if self.hub.read(|state| state.watched_chats == chats) {
            return;
        }
        self.hub.edit(|state| state.watched_chats = chats);
        self.fleet
            .steer(None, crate::live::StreamControl::FollowFiles);
    }
}

impl Drop for Store {
    fn drop(&mut self) {
        self.fleet.stop_all();
    }
}

#[cfg(test)]
mod tests {
    use std::sync::Arc;

    use protocol::DaemonTransport;
    use sidebar::{
        ActiveSpace as _, Command, ProjectId, RowId, ScopeSync, Sink as _, SpaceList as _,
    };
    use tokio::sync::Notify;

    use super::*;

    use crate::live::Outcome;
    use crate::test_support::{
        AsyncCase, FakeDaemon, Peer, Script, Told, frames, pair, recorder, run_cases,
    };

    fn entity_texts() -> Vec<String> {
        frames()
        .into_iter()
        .filter_map(|frame| match frame {
            daemonapi::Frame::Entity { kind, id, data, .. } => Some(
                serde_json::json!({"t": "entity", "scope": "projects", "kind": kind, "id": id, "v": 1, "data": data})
                    .to_string(),
            ),
            _ => None,
        })
        .collect()
    }

    struct Rig {
        data: Store,
        peer: Peer,
        changed: Arc<Notify>,
        opened: Told,
        daemon: Arc<FakeDaemon>,
    }

    fn start() -> Rig {
        let daemon = FakeDaemon::shared();
        daemon.ok(
            "GET",
            "/v0/settings/chat/providers",
            r#"[{"id":"claude","enabled":true,"connected":true}]"#,
        );
        daemon.ok("POST", "/v0/projects/p/home/chats", "{}");
        let transport: Arc<dyn DaemonTransport> = Arc::<FakeDaemon>::clone(&daemon);
        let (connection, peer) = pair();
        let changed = Arc::new(Notify::new());
        let wake = Arc::clone(&changed);
        let (outcome, opened) = recorder();
        let spec = HostSpec {
            id: "local".into(),
            name: "This computer".into(),
            endpoint: None,
        };
        let wiring = Ports {
            desk: crate::test_support::desk(),
            signal: Arc::new(move || wake.notify_one()),
            outcome,
        };
        let data = Store::start_with(
            &Handle::current(),
            &transport,
            vec![(spec, Script::of(vec![connection]))],
            wiring,
        );
        Rig {
            data,
            peer,
            changed,
            opened,
            daemon,
        }
    }

    impl Rig {
        async fn hear_the_space(&mut self) {
            self.peer.ready().await;
            self.peer.snapshot(&entity_texts()).await;
            until_ready(&self.data, &self.changed).await;
        }

        async fn until(
            &self,
            ready: impl Fn(&State) -> bool,
        ) -> bool {
            let reached = reached(&self.data.hub, &self.changed, ready);
            tokio::time::timeout(STUCK, reached).await.is_ok()
        }

        async fn next_request(&mut self) -> Option<String> {
            let asked = tokio::time::timeout(STUCK, self.peer.requests.recv()).await;
            asked.ok().flatten()
        }

        async fn send(
            &self,
            frame: &str,
        ) {
            self.peer.frames.send(frame.to_owned()).await.unwrap();
        }
    }

    async fn reached(
        hub: &Hub,
        changed: &Notify,
        ready: impl Fn(&State) -> bool,
    ) {
        while !hub.read(&ready) {
            changed.notified().await;
        }
    }

    async fn until_ready(
        data: &Store,
        changed: &Notify,
    ) {
        let view = data.view();
        while view.sync() != ScopeSync::Ready {
            changed.notified().await;
        }
    }

    async fn until_opened(
        opened: &Told,
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

    async fn commands_reach_the_daemon_and_the_shells_own_are_not_served_here() {
        let mut rig = start();
        rig.hear_the_space().await;
        let commands = rig.data.commands();
        commands
            .dispatch(Command::NewChat(ProjectId::from("p")))
            .unwrap();
        until_opened(&rig.opened, &rig.changed).await;
        assert!(matches!(
            rig.opened.lock().unwrap()[0],
            Outcome::OpenChat(_)
        ));
        let own = [
            Command::OpenSettings,
            Command::OpenRow(RowId::from("x")),
            Command::SetFileTreeFlag {
                flag: sidebar::FileTreeFlag::Hidden,
                on: false,
            },
        ];
        assert!(own.iter().all(|command| !Store::serves(command)));
        assert!(Store::serves(&Command::NewThread(ProjectId::from("p"))));
    }

    const CASES: [AsyncCase; 2] = [
        || Box::pin(entities_from_the_stream_reach_the_sidebar()),
        || Box::pin(commands_reach_the_daemon_and_the_shells_own_are_not_served_here()),
    ];

    const ONE_FILE: &str = r#"[{"name":"a.txt","path":"a.txt","type":"file"}]"#;
    const TWO_FILES: &str = r#"[{"name":"a.txt","path":"a.txt","type":"file"},{"name":"b.txt","path":"b.txt","type":"file"}]"#;
    const B_CREATED: &str = r#"{"t":"signal","scope":"files:hw","kind":"file_change","data":{"type":"created","path":"b.txt","wsId":"hw"}}"#;
    const FILES_SYNCED: &str = r#"{"t":"synced","scope":"files:hw"}"#;
    const ROOT: &str = "/v0/chats/hc/files/tree?path=.";

    fn root_names(state: &State) -> Vec<String> {
        let root = state.panels.listings.get("").into_iter().flatten();
        root.map(|entry| entry.name.clone()).collect()
    }

    const STUCK: std::time::Duration = std::time::Duration::from_secs(5);

    fn b_listed_on_feature(state: &State) -> bool {
        root_names(state) == ["a.txt", "b.txt"] && state.panels.changes.branch == "feature"
    }

    #[tokio::test(start_paused = true)]
    async fn the_focused_and_open_chats_files_scopes_are_held_and_focused_changes_relist_the_tree()
    {
        let mut rig = start();
        rig.hear_the_space().await;
        rig.daemon.ok("GET", ROOT, ONE_FILE);
        rig.data.focus_chat(Some("hc"));
        let held = Some(r#"{"op":"sub","scope":"files:hw"}"#.to_owned());
        assert_eq!(rig.next_request().await, held);
        assert!(rig.until(|state| root_names(state) == ["a.txt"]).await);
        rig.daemon.ok("GET", ROOT, TWO_FILES);
        let status = r#"{"branch":"feature","files":[]}"#;
        rig.daemon.ok("GET", "/v0/chats/hc/git/status", status);
        rig.send(B_CREATED).await;
        assert!(
            rig.until(b_listed_on_feature).await,
            "a created file re-lists its folder and git"
        );
        rig.daemon.ok("GET", ROOT, ONE_FILE);
        rig.send(FILES_SYNCED).await;
        assert!(
            rig.until(|state| root_names(state) == ["a.txt"]).await,
            "a snapshot re-lists the root"
        );
        rig.data.focus_chat(None);
        let released = Some(r#"{"op":"unsub","scope":"files:hw"}"#.to_owned());
        assert_eq!(rig.next_request().await, released);
        rig.data.watch_chats(["bo".to_owned()].into());
        let watched = Some(r#"{"op":"sub","scope":"files:b"}"#.to_owned());
        assert_eq!(
            rig.next_request().await,
            watched,
            "an open chat's workspace is held unfocused"
        );
    }

    #[tokio::test]
    async fn every_gateway_call_reaches_its_layer() {
        run_cases(&CASES).await;
    }
}
