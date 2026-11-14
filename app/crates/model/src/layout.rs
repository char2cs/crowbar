use std::borrow::Cow;
use std::collections::BTreeMap;

use protocol::{PaneId, PaneTypeId, Params};
use serde::{Deserialize, Deserializer, Serialize, Serializer};
use serde_json::Value;

use crate::arrangement::Arrangement;
use crate::command::Command;
use crate::error::{Error, Result};
use crate::layout_document::LayoutDocument;
use crate::pane_commands;
use crate::pane_instance::PaneInstance;
use crate::schema::{SCHEMA_VERSION, upgrade};
use crate::tab::Tab;
use crate::tab_commands;
use crate::tab_id::TabId;
use crate::undo_step::UndoStep;

/// Most tabs a layout may hold.
pub const MAX_TABS: usize = 256;

/// All tabs of a window and which one is active.
///
/// The layout is a state machine: [`Layout::apply`] is the only way to change it, and a
/// command that fails leaves it exactly as it was. Invariants: tab ids are unique, a pane id
/// appears in at most one tab, no two panes share a type and non-empty parameters, the active
/// tab exists, and every tab satisfies the [`Tab`] invariants.
#[derive(Debug, Clone, Default, PartialEq)]
pub struct Layout {
    pub(crate) tabs: Vec<Tab>,
    pub(crate) active: Option<TabId>,
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
        self.tabs.iter().find(|tab| tab.contains(pane))
    }

    /// Looks up a pane in any tab.
    #[must_use]
    pub fn pane(&self, id: &PaneId) -> Option<&PaneInstance> {
        self.tab_of(id).and_then(|tab| tab.pane(id))
    }

    /// The open pane with this type and these parameters, if any. Parameters identify a pane:
    /// a chat is open once, whichever tab it is in.
    #[must_use]
    pub fn find_pane(&self, type_id: &PaneTypeId, params: &Params) -> Option<&PaneInstance> {
        self.tabs
            .iter()
            .flat_map(Tab::instances)
            .find(|instance| is_same_thing(instance, type_id, params))
    }

    /// Number of panes in all tabs.
    #[must_use]
    pub fn pane_count(&self) -> usize {
        self.tabs.iter().map(Tab::len).sum()
    }

    /// Applies a command, or changes nothing and says why not.
    ///
    /// # Errors
    /// The [`Error`] that refused the command.
    pub fn apply(&mut self, command: Command) -> Result<()> {
        self.execute(command).map(drop)
    }

    /// Checks every invariant.
    ///
    /// # Errors
    /// [`Error::Invalid`] naming the first broken invariant.
    pub fn validate(&self) -> Result<()> {
        if self.tabs.len() > MAX_TABS {
            return Err(Error::Invalid("too many tabs"));
        }
        for tab in &self.tabs {
            tab.validate()?;
        }
        let mut panes: Vec<PaneId> = self.tabs.iter().flat_map(Tab::pane_ids).collect();
        if has_duplicates(&mut panes) {
            return Err(Error::Invalid("a pane appears twice"));
        }
        let mut ids: Vec<TabId> = self.tabs.iter().map(Tab::id).collect();
        if has_duplicates(&mut ids) {
            return Err(Error::Invalid("a tab id appears twice"));
        }
        let mut identities: Vec<(&PaneTypeId, &Params)> = self
            .tabs
            .iter()
            .flat_map(Tab::instances)
            .filter(|instance| !instance.params.is_empty())
            .map(|instance| (&instance.type_id, &instance.params))
            .collect();
        if has_duplicates(&mut identities) {
            return Err(Error::Invalid("a pane is open twice"));
        }
        self.validate_active()
    }

    fn validate_active(&self) -> Result<()> {
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

    pub(crate) fn execute(&mut self, command: Command) -> Result<Vec<PaneInstance>> {
        match command {
            Command::AddTab { id, title } => tab_commands::add(self, id, title).map(none_removed),
            Command::CloseTab { tab } => tab_commands::close(self, tab),
            Command::ActivateTab { tab } => tab_commands::activate(self, tab).map(none_removed),
            Command::MoveTab { tab, to_index } => {
                tab_commands::move_to(self, tab, to_index).map(none_removed)
            },
            Command::RenameTab { tab, title } => {
                tab_commands::rename(self, tab, title).map(none_removed)
            },
            Command::ResizeSplit {
                tab,
                path,
                bounds,
                first_extent,
            } => self
                .tab_mut(tab)
                .and_then(|t| t.resize(&path, bounds, first_extent))
                .map(none_removed),
            Command::ResetSplit { tab, path } => {
                tab_commands::equalize(self, tab, &path).map(none_removed)
            },
            pane_command => self.execute_pane(pane_command),
        }
    }

    fn execute_pane(&mut self, command: Command) -> Result<Vec<PaneInstance>> {
        match command {
            Command::OpenPane {
                tab,
                instance,
                placement,
            } => pane_commands::open(self, tab, instance, placement).map(none_removed),
            Command::ClosePane { pane } => pane_commands::close(self, &pane),
            Command::FocusPane { pane } => pane_commands::focus(self, &pane).map(none_removed),
            Command::MovePane { pane, placement } => {
                pane_commands::relocate(self, &pane, &placement).map(none_removed)
            },
            Command::MovePaneToTab {
                pane,
                to,
                placement,
            } => pane_commands::move_to_tab(self, &pane, to, placement).map(none_removed),
            Command::DetachPane {
                pane,
                new_tab,
                title,
            } => pane_commands::detach(self, &pane, new_tab, title).map(none_removed),
            Command::SwapPanes { a, b } => pane_commands::swap(self, &a, &b).map(none_removed),
            Command::SetPaneState { pane, state } => {
                pane_commands::set_state(self, &pane, state).map(none_removed)
            },
            _ => Ok(Vec::new()),
        }
    }

    pub(crate) fn arrangement(&self) -> Arrangement {
        Arrangement {
            tabs: self.tabs.iter().map(Tab::shape).collect(),
            active: self.active,
        }
    }

    pub(crate) fn swap_arrangement(
        &mut self,
        step: UndoStep,
    ) -> std::result::Result<UndoStep, UndoStep> {
        let restorable = step
            .arrangement
            .pane_ids()
            .iter()
            .all(|id| self.pane(id).is_some() || step.parked.contains_key(id));
        if !restorable {
            return Err(step);
        }
        let before = self.arrangement();
        let mut pool: BTreeMap<PaneId, PaneInstance> = step.parked;
        for tab in &mut self.tabs {
            pool.extend(tab.take_instances());
        }
        let Arrangement { tabs, active } = step.arrangement;
        self.tabs = tabs
            .into_iter()
            .map(|shape| Tab::from_shape(shape, &mut pool))
            .collect();
        self.active = active;
        Ok(UndoStep {
            arrangement: before,
            parked: pool,
        })
    }

    pub(crate) fn index_of(&self, id: TabId) -> Result<usize> {
        self.tabs
            .iter()
            .position(|tab| tab.id() == id)
            .ok_or(Error::TabNotFound(id))
    }

    pub(crate) fn index_of_pane(&self, pane: &PaneId) -> Result<usize> {
        self.tabs
            .iter()
            .position(|tab| tab.contains(pane))
            .ok_or_else(|| Error::PaneNotFound(pane.clone()))
    }

    pub(crate) fn tab_mut(&mut self, id: TabId) -> Result<&mut Tab> {
        self.tabs
            .iter_mut()
            .find(|tab| tab.id() == id)
            .ok_or(Error::TabNotFound(id))
    }

    pub(crate) fn tab_of_mut(&mut self, pane: &PaneId) -> Result<&mut Tab> {
        self.tabs
            .iter_mut()
            .find(|tab| tab.contains(pane))
            .ok_or_else(|| Error::PaneNotFound(pane.clone()))
    }

    pub(crate) fn remove_tab_at(&mut self, index: usize) -> Option<Tab> {
        if index >= self.tabs.len() {
            return None;
        }
        let removed = self.tabs.remove(index);
        if self.active == Some(removed.id()) {
            let heir = self.tabs.get(index).or_else(|| self.tabs.last());
            self.active = heir.map(Tab::id);
        }
        Some(removed)
    }
}

fn none_removed(_: ()) -> Vec<PaneInstance> {
    Vec::new()
}

fn is_same_thing(instance: &PaneInstance, type_id: &PaneTypeId, params: &Params) -> bool {
    !params.is_empty() && &instance.type_id == type_id && &instance.params == params
}

fn has_duplicates<T: Ord>(items: &mut [T]) -> bool {
    items.sort();
    items.windows(2).any(|pair| pair.first() == pair.get(1))
}

impl Serialize for Layout {
    fn serialize<S: Serializer>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error> {
        LayoutDocument {
            version: SCHEMA_VERSION,
            tabs: Cow::Borrowed(&self.tabs),
            active: self.active,
        }
        .serialize(serializer)
    }
}

impl<'de> Deserialize<'de> for Layout {
    fn deserialize<D: Deserializer<'de>>(deserializer: D) -> std::result::Result<Self, D::Error> {
        let stored = Value::deserialize(deserializer)?;
        Self::from_stored(stored).map_err(serde::de::Error::custom)
    }
}

impl Layout {
    pub(crate) fn from_stored(stored: Value) -> Result<Self> {
        let upgraded = upgrade(stored)?;
        let document: LayoutDocument<'static> = serde_json::from_value(upgraded)
            .map_err(|source| Error::Unreadable(source.to_string()))?;
        let layout = Self {
            tabs: document.tabs.into_owned(),
            active: document.active,
        };
        layout.validate()?;
        Ok(layout)
    }
}

#[cfg(test)]
#[path = "layout_tests.rs"]
mod tests;
