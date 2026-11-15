use std::path::Path;

use model::{Command, Layout, LayoutHistory};
use pane_host::{HostEvent, IpcMessage, LayoutSnapshot, Overlay, PaneHost, VisiblePane};
use protocol::{PaneId, PaneTypeId, Params};
use webview::ParentWindow;

use crate::browser_ids::BrowserIds;
use crate::closed_panes::ClosedPanes;
use crate::error::{Error, Result};
use crate::event_commands::commands_for;
use crate::layout_store::LayoutStore;
use crate::layout_writer::LayoutWriter;
use crate::loaded_layout::LoadedLayout;
use crate::outcome::Outcome;
use crate::pane_opener::open_commands;
use crate::pane_spec::PaneSpec;
use crate::serials::Serials;
use crate::terminal_ids::TerminalIds;
use crate::titles::Titles;

pub(crate) struct ShellState {
    history: LayoutHistory,
    writer: LayoutWriter,
    notice: Option<String>,
    host: PaneHost,
    titles: Titles,
    browsers: BrowserIds,
    terminals: TerminalIds,
    serials: Serials,
    closed: ClosedPanes,
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
            browsers,
            terminals,
            serials,
            closed: ClosedPanes::default(),
        })
    }

    pub(crate) fn take_notice(&mut self) -> Option<String> {
        self.notice.take()
    }

    pub(crate) fn layout(&self) -> &Layout {
        self.history.layout()
    }

    pub(crate) fn titles(&self) -> Titles {
        self.titles.clone()
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

    pub(crate) fn apply(
        &mut self,
        commands: Vec<Command>,
    ) {
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
        if let Err(error) = self.host.set_overlay(overlay) {
            tracing::warn!(%error, "hiding web views for an overlay failed");
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
            HostEvent::OpenRequested {
                type_id, params, ..
            } => self.open_pane(type_id, params),
            HostEvent::CommandRequested { command, .. } => outcome.commands.push(command),
            HostEvent::HoverChanged { pane, hovered } => outcome.hover.push((pane, hovered)),
            HostEvent::StateChanged { .. } | HostEvent::StateDiscarded { .. } => {},
            HostEvent::ShellCall { frame, .. } => serve_frame(&frame),
            HostEvent::BrowserPage { pane, update } => self.note_page(pane, update, outcome),
            other => tracing::debug!(
                ?other,
                "a pane asked for something the shell does not serve yet"
            ),
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
            _ => {},
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
    }
}

fn serve_frame(frame: &protocol::PaneFrame) {
    let served = match frame {
        protocol::PaneFrame::OpenExternal { url } => {
            shell_platform::open_external(url.as_str()).map_err(|e| e.to_string())
        },
        protocol::PaneFrame::Reveal { path } => {
            shell_platform::reveal_in_file_manager(Path::new(path.as_str()))
                .map_err(|e| e.to_string())
        },
        other => {
            tracing::debug!(
                ?other,
                "a pane asked for something the shell does not serve yet"
            );
            return;
        },
    };
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
