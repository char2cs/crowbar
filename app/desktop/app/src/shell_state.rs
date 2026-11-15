use model::{Command, Layout, LayoutHistory};
use pane_host::{HostEvent, IpcMessage, LayoutSnapshot, Overlay, PaneHost, VisiblePane};
use protocol::{PaneTypeId, Params};
use webview::ParentWindow;

use crate::error::Result;
use crate::event_commands::commands_for;
use crate::layout_store::LayoutStore;
use crate::pane_opener::open_commands;
use crate::serials::Serials;
use crate::shell_outcome::Outcome;
use crate::titles::Titles;

/// The shell without a window: the layout (the only copy), the pane host that follows it, and
/// the file the layout is kept in.
pub(crate) struct ShellState {
    history: LayoutHistory,
    store: LayoutStore,
    host: PaneHost,
    titles: Titles,
    serials: Serials,
}

impl std::fmt::Debug for ShellState {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("ShellState")
            .field("panes", &self.history.layout().pane_count())
            .finish_non_exhaustive()
    }
}

impl ShellState {
    /// Loads the saved layout and follows it with `host`. A first run opens no tab.
    pub(crate) fn new(host: PaneHost, store: LayoutStore) -> Result<Self> {
        let layout = store.load()?;
        let serials = Serials::above(&layout);
        Ok(Self {
            history: LayoutHistory::new(layout),
            store,
            host,
            titles: Titles::default(),
            serials,
        })
    }

    pub(crate) fn layout(&self) -> &Layout {
        self.history.layout()
    }

    pub(crate) fn titles(&self) -> Titles {
        self.titles.clone()
    }

    pub(crate) fn serials(&self) -> Serials {
        self.serials.clone()
    }

    pub(crate) fn host(&self) -> &PaneHost {
        &self.host
    }

    /// Applies `commands` in order; every refused one is logged and skipped, the rest go
    /// through. Saves the layout when anything changed.
    pub(crate) fn apply(&mut self, commands: Vec<Command>) {
        let before = self.history.layout().clone();
        for command in commands {
            self.history
                .apply(command)
                .unwrap_or_else(|error| tracing::warn!(%error, "the layout refused a command"));
        }
        if self.history.layout() == &before {
            return;
        }
        self.forget_titles_of_closed_panes();
        if let Err(error) = self.store.save(self.history.layout()) {
            tracing::error!(%error, "saving the layout failed");
        }
    }

    /// Opens or focuses the pane `type_id` with `params`.
    pub(crate) fn open_pane(&mut self, type_id: PaneTypeId, params: Params) {
        match open_commands(self.history.layout(), (type_id, params), &self.serials) {
            Ok(commands) => self.apply(commands),
            Err(error) => tracing::warn!(%error, "could not open the pane"),
        }
    }

    /// Reconciles the web views with the layout after it or the window changed.
    pub(crate) fn sync(&mut self, parent: ParentWindow, visible: Vec<VisiblePane>) -> Outcome {
        let snapshot = LayoutSnapshot::from_layout(self.history.layout(), parent, visible);
        match self.host.sync(&snapshot) {
            Ok(events) => self.react(events),
            Err(error) => {
                tracing::error!(%error, "syncing panes with the layout failed");
                Outcome::default()
            },
        }
    }

    pub(crate) fn handle_ipc(&mut self, message: IpcMessage) -> Outcome {
        match self.host.handle_ipc(message) {
            Ok(events) => self.react(events),
            Err(error) => {
                tracing::warn!(%error, "handling a pane message failed");
                Outcome::default()
            },
        }
    }

    pub(crate) fn set_overlay(&mut self, overlay: Overlay) {
        if let Err(error) = self.host.set_overlay(overlay) {
            tracing::warn!(%error, "hiding web views for an overlay failed");
        }
    }

    fn react(&mut self, events: Vec<HostEvent>) -> Outcome {
        self.apply(commands_for(&events));
        let mut outcome = Outcome::default();
        for event in events {
            self.note(event, &mut outcome);
        }
        outcome
    }

    fn note(&mut self, event: HostEvent, outcome: &mut Outcome) {
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
            other => tracing::debug!(
                ?other,
                "a pane asked for something the shell does not serve yet"
            ),
        }
    }

    fn forget_titles_of_closed_panes(&mut self) {
        let layout = self.history.layout();
        self.titles.retain(|pane| layout.pane(pane).is_some());
    }
}

#[cfg(test)]
#[path = "shell_state_tests.rs"]
mod tests;
