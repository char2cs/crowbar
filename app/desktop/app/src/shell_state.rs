use std::path::Path;

use model::{Command, Layout, LayoutHistory, Placement, TabId};
use pane_host::{HostEvent, IpcMessage, LayoutSnapshot, Overlay, PaneHost, VisiblePane};
use protocol::{PaneId, PaneTypeId, Params};
use webview::ParentWindow;

use crate::browser_ids::BrowserIds;
use crate::closed_panes::ClosedPanes;
use crate::core_panes::{chat_params, chat_type};
use crate::dirty_panes::DirtyPanes;
use crate::dropped_files;
use crate::error::{Error, Result};
use crate::event_commands::commands_for;
use crate::frozen_layer::FrozenLayer;
use crate::layout_store::LayoutStore;
use crate::layout_writer::LayoutWriter;
use crate::loaded_layout::LoadedLayout;
use crate::outcome::Outcome;
use crate::pane_opener::{open_at, open_commands};
use crate::pane_spec::PaneSpec;
use crate::save_report::SaveReport;
use crate::serials::Serials;
use crate::terminal_ids::TerminalIds;
use crate::titles::Titles;

pub(crate) struct ShellState {
    history: LayoutHistory,
    writer: LayoutWriter,
    notice: Option<String>,
    host: PaneHost,
    titles: Titles,
    dirty: DirtyPanes,
    open_limit: usize,
    evicted: Vec<String>,
    browsers: BrowserIds,
    terminals: TerminalIds,
    serials: Serials,
    closed: ClosedPanes,
    frozen: FrozenLayer,
}

impl std::fmt::Debug for ShellState {
    fn fmt(
        &self,
        f: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        f.debug_struct("ShellState")
            .field("panes", &self.history.layout().pane_count())
            .finish_non_exhaustive()
    }
}

impl ShellState {
    pub(crate) fn new(
        host: PaneHost,
        store: LayoutStore,
    ) -> Result<Self> {
        let LoadedLayout {
            layout,
            quarantined,
        } = store.load_or_quarantine()?;
        let browsers = BrowserIds::default();
        browsers.follow(&layout);
        let terminals = TerminalIds::default();
        terminals.follow(&layout);
        let titles = Titles::default();
        terminals.name_into(&titles);
        let serials = Serials::above(&layout);
        let writer = LayoutWriter::start(store).map_err(Error::LayoutWriter)?;
        Ok(Self {
            history: LayoutHistory::new(layout),
            writer,
            notice: quarantined.map(|path| notice_for(&path)),
            host,
            titles,
            dirty: DirtyPanes::default(),
            open_limit: crate::tab_cap::limit(None),
            evicted: Vec::new(),
            browsers,
            terminals,
            serials,
            closed: ClosedPanes::default(),
            frozen: FrozenLayer::default(),
        })
    }

    pub(crate) fn take_notice(&mut self) -> Option<String> {
        self.notice.take()
    }

    pub(crate) fn save_reports(&self) -> async_channel::Receiver<SaveReport> {
        self.writer.reports()
    }

    pub(crate) fn layout(&self) -> &Layout {
        self.history.layout()
    }

    pub(crate) fn titles(&self) -> Titles {
        self.titles.clone()
    }

    pub(crate) fn dirty(&self) -> DirtyPanes {
        self.dirty.clone()
    }

    pub(crate) fn browsers(&self) -> BrowserIds {
        self.browsers.clone()
    }

    pub(crate) fn terminals(&self) -> TerminalIds {
        self.terminals.clone()
    }

    pub(crate) fn serials(&self) -> Serials {
        self.serials.clone()
    }

    pub(crate) fn host(&self) -> &PaneHost {
        &self.host
    }

    pub(crate) fn set_keybindings(
        &mut self,
        bindings: Vec<protocol::KeyBinding>,
    ) {
        if let Err(error) = self.host.set_keybindings(bindings) {
            tracing::warn!(%error, "pushing the keybindings to the panes failed");
        }
    }

    pub(crate) fn set_open_limit(
        &mut self,
        setting: Option<f64>,
    ) {
        self.open_limit = crate::tab_cap::limit(setting);
    }

    pub(crate) fn take_evicted(&mut self) -> Vec<String> {
        std::mem::take(&mut self.evicted)
    }

    fn make_room(
        &mut self,
        commands: Vec<Command>,
    ) -> Vec<Command> {
        let layout = self.history.layout();
        let victims = crate::tab_cap::evictions(layout, &commands, self.open_limit, &self.dirty);
        let names: Vec<String> = victims
            .iter()
            .map(|pane| {
                self.titles
                    .get(pane)
                    .map_or_else(|| pane.to_string(), |t| t.to_string())
            })
            .collect();
        self.evicted.extend(names);
        let closing = victims.into_iter().map(|pane| Command::ClosePane { pane });
        closing.chain(commands).collect()
    }

    pub(crate) fn apply(
        &mut self,
        commands: Vec<Command>,
    ) {
        let commands = self.make_room(commands);
        let mut applied = 0_usize;
        for command in commands {
            applied += usize::from(self.try_apply(command));
        }
        if applied == 0 {
            return;
        }
        self.forget_titles_of_closed_panes();
        self.browsers.follow(self.history.layout());
        self.terminals.follow(self.history.layout());
        self.terminals.name_into(&self.titles);
        self.writer.submit(self.history.layout().clone());
    }

    fn try_apply(
        &mut self,
        command: Command,
    ) -> bool {
        let closing = ClosedPanes::closing(self.history.layout(), &command);
        let result = self.history.apply(command);
        if let Err(error) = &result {
            tracing::warn!(%error, "the layout refused a command");
        }
        let applied = result.is_ok();
        if applied {
            self.closed.remember(closing);
        }
        applied
    }

    pub(crate) fn open_pane(
        &mut self,
        type_id: PaneTypeId,
        params: Params,
    ) {
        match open_commands(self.history.layout(), (type_id, params), &self.serials) {
            Ok(commands) => self.apply(commands),
            Err(error) => tracing::warn!(%error, "could not open the pane"),
        }
    }

    pub(crate) fn open_chat_at(
        &mut self,
        chat: &str,
        (tab, placement): (TabId, Placement),
    ) {
        let opened = chat_type()
            .and_then(|type_id| Ok((type_id, chat_params(chat)?)))
            .and_then(|pane| open_at(pane, (tab, placement), &self.serials));
        match opened {
            Ok(command) => self.apply(vec![command]),
            Err(error) => tracing::warn!(%error, "could not open the dropped chat"),
        }
    }

    pub(crate) fn open_spec(
        &mut self,
        spec: PaneSpec,
    ) {
        match spec.open_commands(self.history.layout(), &self.serials) {
            Ok(commands) => self.apply(commands),
            Err(error) => tracing::warn!(%error, "could not open the pane"),
        }
    }

    pub(crate) fn reopen_closed(&mut self) -> bool {
        let Some(spec) = self.closed.take_last() else {
            return false;
        };
        self.open_spec(spec);
        true
    }

    pub(crate) fn sync(
        &mut self,
        parent: ParentWindow,
        visible: Vec<VisiblePane>,
    ) -> Outcome {
        self.frozen.set_holes(&visible);
        let snapshot = LayoutSnapshot::from_layout(self.history.layout(), parent, visible);
        let report = self.host.sync(&snapshot);
        if let Some(error) = report.failure {
            tracing::error!(%error, "a pane could not be driven while syncing with the layout");
        }
        self.react(report.events)
    }

    pub(crate) fn handle_ipc(
        &mut self,
        message: IpcMessage,
    ) -> Outcome {
        match self.host.handle_ipc(message, self.history.layout()) {
            Ok(events) => self.react(events),
            Err(error) => {
                tracing::warn!(%error, "handling a pane message failed");
                Outcome::default()
            },
        }
    }

    pub(crate) fn set_theme(
        &mut self,
        theme: ui::Theme,
    ) {
        let tokens = match crate::pane_theme::tokens_of(theme) {
            Ok(tokens) => tokens,
            Err(error) => {
                return tracing::warn!(%error, "the theme could not be described to panes");
            },
        };
        if let Err(error) = self.host.set_theme(tokens) {
            tracing::warn!(%error, "sending the theme to panes failed");
        }
        let surface = crate::pane_theme::surface_color(theme);
        if let Err(error) = self.host.set_background(surface) {
            tracing::warn!(%error, "colouring the pane web views failed");
        }
    }

    pub(crate) fn set_pane_settings(
        &mut self,
        values: protocol::SettingValues,
    ) {
        if let Err(error) = self.host.set_settings(values) {
            tracing::warn!(%error, "sending the settings to panes failed");
        }
    }

    pub(crate) fn set_overlay(
        &mut self,
        overlay: Overlay,
    ) {
        if overlay == Overlay::Active {
            return self.freeze_panes();
        }
        if let Err(error) = self.host.set_overlay(overlay) {
            tracing::warn!(%error, "showing web views after an overlay failed");
        }
        self.frozen.replace(Vec::new());
    }

    pub(crate) fn frozen_elements(&self) -> Vec<gpui::AnyElement> {
        self.frozen.elements()
    }

    fn freeze_panes(&mut self) {
        if self.host.is_overlaid() {
            return;
        }
        match self.host.freeze() {
            Ok(frozen) => self.frozen.replace(frozen),
            Err(error) => tracing::warn!(%error, "hiding web views for an overlay failed"),
        }
    }

    fn react(
        &mut self,
        events: Vec<HostEvent>,
    ) -> Outcome {
        self.apply(commands_for(&events));
        let mut outcome = Outcome::default();
        for event in events {
            self.note(event, &mut outcome);
        }
        outcome
    }

    fn note(
        &mut self,
        event: HostEvent,
        outcome: &mut Outcome,
    ) {
        match event {
            HostEvent::TitleChanged { pane, title } => {
                self.titles.set(pane, title);
                outcome.titles_changed = true;
            },
            HostEvent::DirtyChanged { pane, dirty: true } => {
                self.dirty.mark(pane);
                outcome.dirty_changed = true;
            },
            HostEvent::DirtyChanged { pane, dirty: false } => {
                self.dirty.clear(&pane);
                outcome.dirty_changed = true;
            },
            HostEvent::OpenRequested {
                type_id, params, ..
            } => self.open_pane(type_id, params),
            HostEvent::CommandRequested { command, .. } => outcome.commands.push(command),
            HostEvent::HoverChanged { pane, hovered } => outcome.hover.push((pane, hovered)),
            HostEvent::SettingRequested { key, value, .. } => outcome.settings.push((key, value)),
            HostEvent::OpenExternal { url } => {
                log_failure(shell_platform::open_external(url.as_str()).map_err(|e| e.to_string()));
            },
            HostEvent::Reveal { path } => log_failure(
                shell_platform::reveal_in_file_manager(Path::new(path.as_str()))
                    .map_err(|e| e.to_string()),
            ),
            HostEvent::FilesDropped { pane, paths }
                if dropped_files::opens_files(self.history.layout(), &pane) =>
            {
                outcome.dropped.extend(paths);
            },
            HostEvent::StateChanged { .. }
            | HostEvent::StateDiscarded { .. }
            | HostEvent::FilesDropped { .. } => {},
            HostEvent::BrowserPage { pane, update } => self.note_page(pane, update, outcome),
        }
    }

    fn note_page(
        &mut self,
        pane: PaneId,
        update: browser_nav::PageUpdate,
        outcome: &mut Outcome,
    ) {
        match update {
            browser_nav::PageUpdate::Loading { url } => outcome.pages.push((pane, url)),
            browser_nav::PageUpdate::Loaded { url } => {
                outcome.pages.push((pane.clone(), url.clone()));
                self.save_address(pane, &url);
            },
            browser_nav::PageUpdate::Title { title } => self.set_page_title(pane, title, outcome),
            pick => outcome.picks.push((pane, pick)),
        }
    }

    fn set_page_title(
        &mut self,
        pane: PaneId,
        title: String,
        outcome: &mut Outcome,
    ) {
        let Ok(title) = protocol::PaneTitle::new(title) else {
            return;
        };
        self.titles.set(pane, title);
        outcome.titles_changed = true;
    }

    fn save_address(
        &mut self,
        pane: PaneId,
        url: &str,
    ) {
        let saved = serde_json::json!({ "url": url });
        if let Ok(state) = protocol::PaneState::new(saved) {
            self.apply(vec![Command::SetPaneState { pane, state }]);
        }
    }

    fn forget_titles_of_closed_panes(&mut self) {
        let layout = self.history.layout();
        self.titles.retain(|pane| layout.pane(pane).is_some());
        self.dirty.retain(|pane| layout.pane(pane).is_some());
    }
}

fn log_failure<E: std::fmt::Display>(served: std::result::Result<(), E>) {
    if let Err(error) = served {
        tracing::warn!(%error, "a pane's request could not be served");
    }
}

fn notice_for(path: &Path) -> String {
    format!(
        "The saved layout could not be read. It was kept as {} and Crowbar started fresh.",
        path.display()
    )
}

#[cfg(test)]
mod tests {
    use model::TabId;
    use protocol::{PaneFrame, PaneState, PaneTitle, encode};
    use serde_json::json;
    use std::rc::Rc;
    use webview::{Bounds, Call, MockPlatform};

    use super::*;
    use crate::core_panes::{chat_params, chat_type};
    use crate::test_support::mock_host;

    struct Rig {
        shell: ShellState,
        mock: Rc<MockPlatform>,
        dir: tempfile::TempDir,
    }

    fn rig() -> Rig {
        let mock = Rc::new(MockPlatform::default());
        let dir = tempfile::tempdir().unwrap();
        let store = LayoutStore::new(dir.path().join("layout.json"));
        let shell = ShellState::new(mock_host(&mock), store).unwrap();
        Rig { shell, mock, dir }
    }

    fn open_chat(
        shell: &mut ShellState,
        thread: &str,
    ) {
        shell.open_pane(chat_type().unwrap(), chat_params(thread).unwrap());
    }

    fn window_area() -> Bounds {
        Bounds {
            x: 294.0,
            y: 42.0,
            width: 800.0,
            height: 600.0,
        }
    }

    fn in_window(pane: PaneId) -> VisiblePane {
        VisiblePane {
            pane,
            bounds: window_area(),
        }
    }

    fn visible(shell: &ShellState) -> Vec<VisiblePane> {
        shell.layout().active_tab().map_or_else(Vec::new, |tab| {
            tab.pane_ids().into_iter().map(in_window).collect()
        })
    }

    fn synced_chat(rig: &mut Rig) -> PaneId {
        open_chat(&mut rig.shell, "t1");
        let visible = visible(&rig.shell);
        rig.shell.sync(ParentWindow::X11(9), visible);
        rig.shell.layout().active_tab().unwrap().pane_ids()[0].clone()
    }

    #[test]
    fn an_opened_chat_gets_a_web_view_in_the_window() {
        let mut rig = rig();
        let rig = &mut rig;
        synced_chat(rig);
        let calls = rig.mock.calls();
        assert!(
            matches!(
                &calls[0],
                Call::Create {
                    parent: ParentWindow::X11(9),
                    ..
                }
            ),
            "{calls:?}"
        );
        assert!(
            matches!(&calls[1], Call::Navigate { url, .. }
            if url.starts_with("crowbar://local/panes/core/chat/index.html")),
            "{calls:?}"
        );
    }

    #[test]
    fn pane_state_flows_into_the_layout() {
        let mut rig = rig();
        let rig = &mut rig;
        let pane = synced_chat(rig);
        let state = PaneState::new(json!({"draft": "hi"})).unwrap();
        let text = encode(&PaneFrame::SetState {
            state: state.clone(),
        })
        .unwrap();
        rig.shell.handle_ipc(IpcMessage {
            pane: pane.clone(),
            text,
        });
        let stored = rig.shell.layout().pane(&pane).map(|p| p.state.clone());
        assert_eq!(stored, Some(state), "the model owns the new state");
    }

    type Refused = (&'static str, Command);

    fn refused() -> Vec<Refused> {
        vec![
            (
                "closing an unknown tab",
                Command::CloseTab { tab: TabId(99) },
            ),
            (
                "closing an unknown pane",
                Command::ClosePane {
                    pane: PaneId::new("nope").unwrap(),
                },
            ),
        ]
    }

    fn add_tab() -> Command {
        Command::AddTab {
            id: TabId(1),
            title: "ok".into(),
        }
    }

    fn leaves_the_layout_alone_and_keeps_going(command: &Command) -> bool {
        let mut rig = rig();
        let before = rig.shell.layout().clone();
        rig.shell.apply(vec![command.clone()]);
        let unchanged = rig.shell.layout() == &before;
        rig.shell.apply(vec![add_tab()]);
        unchanged && rig.shell.layout().tabs().len() == 1
    }

    #[test]
    fn refused_commands_change_nothing_and_do_not_stop_the_rest() {
        let wrong: Vec<&str> = refused()
            .iter()
            .filter(|(_, command)| !leaves_the_layout_alone_and_keeps_going(command))
            .map(|(name, _)| *name)
            .collect();
        assert!(wrong.is_empty(), "failed cases: {wrong:?}");
    }

    #[test]
    fn a_page_title_is_kept_until_its_pane_closes() {
        let mut rig = rig();
        let rig = &mut rig;
        let pane = synced_chat(rig);
        let text = encode(&PaneFrame::SetTitle {
            title: PaneTitle::new("Fix the build").unwrap(),
        })
        .unwrap();
        let outcome = rig.shell.handle_ipc(IpcMessage {
            pane: pane.clone(),
            text,
        });
        assert!(outcome.titles_changed, "the tab strip must refresh");
        assert_eq!(
            rig.shell.titles().get(&pane).map(|t| t.to_string()),
            Some("Fix the build".into()),
            "kept"
        );
        rig.shell
            .apply(vec![Command::ClosePane { pane: pane.clone() }]);
        assert!(
            rig.shell.titles().get(&pane).is_none(),
            "forgotten with the pane"
        );
    }

    #[test]
    fn a_terminal_opened_from_a_folder_is_titled_by_it() {
        let mut rig = rig();
        let spec = PaneSpec::terminal("t1", None, Some("/work/src")).unwrap();
        rig.shell.open_spec(spec);
        let pane = rig.shell.layout().tabs()[0].pane_ids()[0].clone();
        assert_eq!(
            rig.shell.titles().get(&pane).map(|t| t.to_string()),
            Some("src".into())
        );
    }

    #[test]
    fn a_chat_dropped_on_a_pane_opens_beside_it_and_an_open_one_moves() {
        let mut rig = rig();
        open_chat(&mut rig.shell, "t1");
        open_chat(&mut rig.shell, "t2");
        let first = rig.shell.layout().tabs()[0].pane_ids()[0].clone();
        let tab = rig.shell.layout().tabs()[0].id();
        let placement = Placement {
            target: first,
            edge: model::Edge::Left,
        };
        rig.shell.open_chat_at("t3", (tab, placement.clone()));
        assert_eq!(rig.shell.layout().pane_count(), 3, "t3 opened");
        rig.shell.open_chat_at("t2", (tab, placement));
        assert_eq!(rig.shell.layout().pane_count(), 3, "t2 moved, not copied");
    }

    #[test]
    fn the_layout_survives_a_restart() {
        let mut rig = rig();
        open_chat(&mut rig.shell, "t1");
        let expected = rig.shell.layout().clone();
        let Rig { shell, mock, dir } = rig;
        drop(shell);
        let again = ShellState::new(
            mock_host(&mock),
            LayoutStore::new(dir.path().join("layout.json")),
        )
        .unwrap();
        assert_eq!(again.layout(), &expected, "same layout after loading");
    }

    #[test]
    fn a_corrupt_layout_file_starts_fresh_and_leaves_a_notice() {
        let mock = Rc::new(MockPlatform::default());
        let dir = tempfile::tempdir().unwrap();
        std::fs::write(dir.path().join("layout.json"), "garbage").unwrap();
        let mut shell = ShellState::new(
            mock_host(&mock),
            LayoutStore::new(dir.path().join("layout.json")),
        )
        .unwrap();
        assert_eq!(shell.layout().pane_count(), 0, "fresh layout");
        let notice = shell.take_notice().unwrap();
        assert!(notice.contains("layout.json.bad-"), "{notice}");
        assert!(shell.take_notice().is_none(), "shown once");
    }
}
