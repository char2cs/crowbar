use protocol::{PaneId, PaneState};
use serde::{Deserialize, Deserializer, Serialize};

use crate::command::Command;
use crate::error::{Error, Result};
use crate::pane_instance::PaneInstance;
use crate::placement::Placement;
use crate::tab::Tab;
use crate::tab_id::TabId;

/// All tabs of a window and which one is active.
///
/// The layout is a state machine: [`Layout::apply`] is the only way to change it, and a
/// command that fails leaves it exactly as it was. Invariants: tab ids are unique, a pane id
/// appears in at most one tab, the active tab exists, and every tab satisfies the [`Tab`]
/// invariants.
#[derive(Debug, Clone, Default, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct Layout {
    tabs: Vec<Tab>,
    active: Option<TabId>,
}

impl Layout {
    /// The tabs in tab bar order.
    #[must_use]
    pub fn tabs(&self) -> &[Tab] {
        &self.tabs
    }

    /// Looks up a tab.
    #[must_use]
    pub fn tab(&self, id: TabId) -> Option<&Tab> {
        self.tabs.iter().find(|tab| tab.id() == id)
    }

    /// The active tab.
    #[must_use]
    pub fn active_tab(&self) -> Option<&Tab> {
        self.active.and_then(|id| self.tab(id))
    }

    /// The tab holding a pane.
    #[must_use]
    pub fn tab_of(&self, pane: &PaneId) -> Option<&Tab> {
        self.tabs.iter().find(|tab| tab.pane(pane).is_some())
    }

    /// Applies a command, or changes nothing and says why not.
    ///
    /// # Errors
    /// The [`Error`] that refused the command.
    pub fn apply(&mut self, command: Command) -> Result<()> {
        let mut next = self.clone();
        next.run(command)?;
        *self = next;
        Ok(())
    }

    /// Checks every invariant.
    ///
    /// # Errors
    /// [`Error::Invalid`] naming the first broken invariant.
    pub fn validate(&self) -> Result<()> {
        let mut panes: Vec<PaneId> = Vec::new();
        for tab in &self.tabs {
            tab.validate()?;
            panes.extend(tab.pane_ids());
        }
        let panes_before = panes.len();
        panes.sort();
        panes.dedup();
        if panes.len() != panes_before {
            return Err(Error::Invalid("a pane appears twice"));
        }
        let mut ids: Vec<TabId> = self.tabs.iter().map(Tab::id).collect();
        ids.sort();
        ids.dedup();
        if ids.len() != self.tabs.len() {
            return Err(Error::Invalid("a tab id appears twice"));
        }
        let active_ok = match self.active {
            Some(id) => self.tab(id).is_some(),
            None => self.tabs.is_empty(),
        };
        if active_ok {
            Ok(())
        } else {
            Err(Error::Invalid("active tab does not match the tabs"))
        }
    }

    pub(crate) fn adopt_states(&mut self, from: &Self) {
        let states: Vec<(PaneId, PaneState)> = from.tabs.iter().flat_map(Tab::states).collect();
        for tab in &mut self.tabs {
            tab.apply_states(&states);
        }
    }

    fn run(&mut self, command: Command) -> Result<()> {
        match command {
            Command::AddTab { id, title } => self.add_tab(id, title),
            Command::CloseTab { tab } => self.close_tab(tab),
            Command::ActivateTab { tab } => self.activate(tab),
            Command::MoveTab { tab, to_index } => self.move_tab(tab, to_index),
            Command::RenameTab { tab, title } => self.rename_tab(tab, title),
            Command::OpenPane {
                tab,
                instance,
                placement,
            } => self.open_pane(tab, instance, placement),
            Command::ClosePane { pane } => self.tab_of_mut(&pane)?.close_pane(&pane).map(drop),
            Command::FocusPane { pane } => self.focus_pane(&pane),
            Command::MovePane { pane, placement } => {
                self.tab_of_mut(&pane)?.move_pane(&pane, &placement)
            },
            Command::MovePaneToTab {
                pane,
                to,
                placement,
            } => self.move_to_tab(&pane, to, placement),
            Command::SwapPanes { a, b } => self.tab_of_mut(&a)?.swap_panes(&a, &b),
            Command::ResizeSplit { tab, path, ratio } => {
                self.tab_mut(tab)?.resize_split(&path, ratio)
            },
            Command::SetPaneState { pane, state } => {
                self.tab_of_mut(&pane)?.set_pane_state(&pane, state)
            },
        }
    }

    fn open_pane(
        &mut self,
        tab: TabId,
        instance: PaneInstance,
        placement: Option<Placement>,
    ) -> Result<()> {
        if self.tab_of(&instance.pane_id).is_some() {
            return Err(Error::DuplicatePane(instance.pane_id));
        }
        self.tab_mut(tab)?.open_pane(instance, placement)?;
        self.activate(tab)
    }

    fn focus_pane(&mut self, pane: &PaneId) -> Result<()> {
        let tab = self.tab_of_mut(pane)?;
        tab.focus(pane)?;
        let id = tab.id();
        self.activate(id)
    }

    fn rename_tab(&mut self, id: TabId, title: String) -> Result<()> {
        self.tab_mut(id)?.set_title(title);
        Ok(())
    }

    fn add_tab(&mut self, id: TabId, title: String) -> Result<()> {
        if self.tab(id).is_some() {
            return Err(Error::DuplicateTab(id));
        }
        self.tabs.push(Tab::new(id, title));
        self.active = Some(id);
        Ok(())
    }

    fn close_tab(&mut self, id: TabId) -> Result<()> {
        let index = self.index_of(id)?;
        self.tabs.remove(index);
        if self.active == Some(id) {
            let heir = self.tabs.get(index).or_else(|| self.tabs.last());
            self.active = heir.map(Tab::id);
        }
        Ok(())
    }

    fn activate(&mut self, id: TabId) -> Result<()> {
        self.index_of(id)?;
        self.active = Some(id);
        Ok(())
    }

    fn move_tab(&mut self, id: TabId, to_index: usize) -> Result<()> {
        let index = self.index_of(id)?;
        if to_index >= self.tabs.len() {
            return Err(Error::IndexOutOfRange {
                index: to_index,
                len: self.tabs.len(),
            });
        }
        let tab = self.tabs.remove(index);
        self.tabs.insert(to_index, tab);
        Ok(())
    }

    fn move_to_tab(
        &mut self,
        pane: &PaneId,
        to: TabId,
        placement: Option<Placement>,
    ) -> Result<()> {
        self.index_of(to)?;
        let from = self.tab_of_mut(pane)?;
        if from.id() == to {
            return Err(Error::SamePane(pane.clone()));
        }
        let instance = from.close_pane(pane)?;
        self.tab_mut(to)?.open_pane(instance, placement)?;
        self.activate(to)
    }

    fn index_of(&self, id: TabId) -> Result<usize> {
        self.tabs
            .iter()
            .position(|tab| tab.id() == id)
            .ok_or(Error::TabNotFound(id))
    }

    fn tab_mut(&mut self, id: TabId) -> Result<&mut Tab> {
        self.tabs
            .iter_mut()
            .find(|tab| tab.id() == id)
            .ok_or(Error::TabNotFound(id))
    }

    fn tab_of_mut(&mut self, pane: &PaneId) -> Result<&mut Tab> {
        self.tabs
            .iter_mut()
            .find(|tab| tab.pane(pane).is_some())
            .ok_or_else(|| Error::PaneNotFound(pane.clone()))
    }
}

impl<'de> Deserialize<'de> for Layout {
    fn deserialize<D: Deserializer<'de>>(deserializer: D) -> std::result::Result<Self, D::Error> {
        #[derive(Deserialize)]
        struct Stored {
            tabs: Vec<Tab>,
            active: Option<TabId>,
        }
        let stored = Stored::deserialize(deserializer)?;
        let layout = Self {
            tabs: stored.tabs,
            active: stored.active,
        };
        layout.validate().map_err(serde::de::Error::custom)?;
        Ok(layout)
    }
}

#[cfg(test)]
#[path = "layout_tests.rs"]
mod tests;
