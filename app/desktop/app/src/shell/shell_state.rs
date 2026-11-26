use std::path::Path;
use std::rc::Rc;

use layout::{Command, LayoutHistory};
use std::sync::Arc;

use browseragent::Replies;
use panehost::{Keyboard, PaneHost};

use crate::error::{Error, Result};
use crate::panes::browser::browser_ids::BrowserIds;
use crate::panes::browser::browser_pages::BrowserPages;
use crate::panes::ide::closed_panes::ClosedPanes;
use crate::panes::ide::tab_cap;
use crate::panes::kinds::serials::Serials;
use crate::panes::terminals::terminal_ids::TerminalIds;
use crate::panes::tracking::dirty_panes::DirtyPanes;
use crate::panes::tracking::draft_views;
use crate::panes::tracking::titles::Titles;
use crate::shell::shell_state::active_space::ActiveSpace;
use crate::shell::shell_state::persistence::layout_store::LayoutStore;
use crate::shell::shell_state::persistence::layout_writer::LayoutWriter;
use crate::shell::shell_state::persistence::loaded_layout::LoadedLayout;

mod accessors;
pub(crate) mod active_space;
mod agent;
mod browsing;
mod chat_zoom;
pub(crate) mod host;
pub(crate) mod persistence;
mod placing;
mod renaming;

pub(crate) struct ShellState {
    history: LayoutHistory,
    space: Rc<dyn ActiveSpace>,
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
    keyboard: Keyboard,
    pages: BrowserPages,
    replies: Arc<Replies>,
    pending_loads: agent::pending_loads::PendingLoads,
    tabs_out: tokio::sync::watch::Sender<Vec<browseragent::TabSummary>>,
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
        space: Rc<dyn ActiveSpace>,
    ) -> Result<Self> {
        let LoadedLayout {
            layout,
            quarantined,
        } = store.load_or_quarantine()?;
        let layout = draft_views::without_drafts(layout);
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
            space,
            writer,
            notice: quarantined.map(|path| notice_for(&path)),
            host,
            titles,
            dirty: DirtyPanes::default(),
            open_limit: tab_cap::limit(None),
            evicted: Vec::new(),
            browsers,
            terminals,
            serials,
            closed: ClosedPanes::default(),
            keyboard: Keyboard::default(),
            pages: BrowserPages::default(),
            replies: Arc::default(),
            pending_loads: agent::pending_loads::PendingLoads::default(),
            tabs_out: tokio::sync::watch::channel(Vec::new()).0,
        })
    }

    pub(crate) fn set_open_limit(
        &mut self,
        setting: Option<f64>,
    ) {
        self.open_limit = tab_cap::limit(setting);
    }

    pub(crate) fn take_evicted(&mut self) -> Vec<String> {
        std::mem::take(&mut self.evicted)
    }

    fn make_room(
        &mut self,
        commands: Vec<Command>,
    ) -> Vec<Command> {
        let layout = self.history.layout();
        let victims = tab_cap::evictions(layout, &commands, self.open_limit, &self.dirty);
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
        if commands.is_empty() {
            return;
        }
        self.keyboard = Keyboard::Pane;
        self.run(commands);
    }

    pub(crate) fn apply_from_strip(
        &mut self,
        commands: Vec<Command>,
    ) {
        self.run(commands);
        self.keyboard = Keyboard::Shell;
    }

    pub(crate) fn keep_keyboard_in_shell(&mut self) {
        self.keyboard = Keyboard::Shell;
    }

    // PLATFORM: linux GTK ends the process on its next request to a web view whose parent window is gone.
    pub(crate) fn close_web_views(&mut self) {
        if let Err(error) = self.host.close_web_views() {
            tracing::warn!(%error, "the web views did not close before their window");
        }
    }

    fn run(
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
        self.settle();
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
}

fn notice_for(path: &Path) -> String {
    format!(
        "The saved layout could not be read. It was kept as {} and Crowbar started fresh.",
        path.display()
    )
}

#[cfg(test)]
mod tests {
    use layout::TabId;

    use protocol::PaneId;

    use super::*;
    use crate::panes::kinds::core_panes::{chat_params, chat_type};
    use crate::test_support::{ShellRig, open_chat, shell_rig};

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
            space: layout::SpaceId::default(),
        }
    }

    fn leaves_the_layout_alone_and_keeps_going(command: &Command) -> bool {
        let mut rig = shell_rig();
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

    fn handed_back(rig: &ShellRig) -> usize {
        let calls = rig.mock.calls();
        calls
            .iter()
            .filter(|call| matches!(call, webview::Call::FocusParent { .. }))
            .count()
    }

    fn sync_chat(
        rig: &mut ShellRig,
        pane: &PaneId,
    ) {
        let bounds = webview::Bounds {
            x: 0.0,
            y: 0.0,
            width: 400.0,
            height: 300.0,
        };
        let shown = panehost::VisiblePane {
            pane: pane.clone(),
            bounds,
        };
        rig.shell
            .sync(webview::ParentWindow::X11(9), (vec![shown], &()));
    }

    enum Via {
        Strip,
        ShellPress,
        ShellPressThenNothing,
        SidebarRow,
        Elsewhere,
    }

    const CASES: &[(&str, Via, usize)] = &[
        ("a strip press keeps the keys in the shell", Via::Strip, 1),
        (
            "a press on the shell's own surface keeps them too",
            Via::ShellPress,
            1,
        ),
        (
            "an empty batch after a shell press leaves them in the shell",
            Via::ShellPressThenNothing,
            1,
        ),
        (
            "a sidebar row opening its chat keeps them on the sidebar",
            Via::SidebarRow,
            1,
        ),
        (
            "any other command gives them to the page",
            Via::Elsewhere,
            0,
        ),
    ];

    fn focus_via(
        shell: &mut ShellState,
        via: &Via,
        pane: &PaneId,
    ) {
        let focus = vec![Command::FocusPane { pane: pane.clone() }];
        match via {
            Via::Strip => shell.apply_from_strip(focus),
            Via::ShellPress => {
                shell.apply(focus);
                shell.keep_keyboard_in_shell();
            },
            Via::ShellPressThenNothing => {
                shell.apply(focus);
                shell.keep_keyboard_in_shell();
                shell.apply(Vec::new());
            },
            Via::SidebarRow => {
                shell.keep_keyboard_in_shell();
                let row = (chat_type().unwrap(), chat_params("t1").unwrap());
                shell.open_from_sidebar(row);
            },
            Via::Elsewhere => shell.apply(focus),
        }
    }

    #[test]
    fn only_a_strip_or_a_shell_press_keeps_the_keyboard_from_the_page_that_becomes_active() {
        for (name, via, handed) in CASES {
            let mut rig = shell_rig();
            let pane = crate::test_support::synced_chat(&mut rig);
            let before = handed_back(&rig);
            focus_via(&mut rig.shell, via, &pane);
            sync_chat(&mut rig, &pane);
            assert_eq!(handed_back(&rig) - before, *handed, "{name}");
        }
    }

    #[test]
    fn the_layout_survives_a_restart() {
        let mut rig = shell_rig();
        open_chat(&mut rig.shell, "t1");
        let expected = rig.shell.layout().clone();
        let ShellRig {
            shell, mock, files, ..
        } = rig;
        drop(shell);
        let again = ShellState::new(
            crate::test_support::mock_host(&mock),
            files.store(),
            Rc::new(crate::test_support::FakeSpace::default()),
        )
        .unwrap();
        assert_eq!(again.layout(), &expected, "same layout after loading");
    }
}
