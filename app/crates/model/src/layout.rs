use std::borrow::Cow;
use std::collections::BTreeMap;

use protocol::{PaneId, PaneTypeId, Params};
use serde::{Deserialize, Deserializer, Serialize, Serializer};
use serde_json::Value;

use crate::arrangement::Arrangement;
use crate::command::Command;
use crate::error::{Error, Result};
use crate::layout_document::LayoutDocument;
use crate::pane_instance::PaneInstance;
use crate::schema::{SCHEMA_VERSION, upgrade};
use crate::tab::Tab;
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
    pub fn tab(
        &self,
        id: TabId,
    ) -> Option<&Tab> {
        self.tabs.iter().find(|tab| tab.id() == id)
    }

    /// The active tab.
    #[must_use]
    pub fn active_tab(&self) -> Option<&Tab> {
        self.active.and_then(|id| self.tab(id))
    }

    /// The tab holding a pane.
    #[must_use]
    pub fn tab_of(
        &self,
        pane: &PaneId,
    ) -> Option<&Tab> {
        self.tabs.iter().find(|tab| tab.contains(pane))
    }

    /// Looks up a pane in any tab.
    #[must_use]
    pub fn pane(
        &self,
        id: &PaneId,
    ) -> Option<&PaneInstance> {
        self.tab_of(id).and_then(|tab| tab.pane(id))
    }

    /// The open pane with this type and these parameters, if any. Parameters identify a pane:
    /// a chat is open once, whichever tab it is in.
    #[must_use]
    pub fn find_pane(
        &self,
        type_id: &PaneTypeId,
        params: &Params,
    ) -> Option<&PaneInstance> {
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
    pub fn apply(
        &mut self,
        command: Command,
    ) -> Result<()> {
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
        active_ok
            .then_some(())
            .ok_or(Error::Invalid("active tab does not match the tabs"))
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

    pub(crate) fn index_of(
        &self,
        id: TabId,
    ) -> Result<usize> {
        self.tabs
            .iter()
            .position(|tab| tab.id() == id)
            .ok_or(Error::TabNotFound(id))
    }

    pub(crate) fn index_of_pane(
        &self,
        pane: &PaneId,
    ) -> Result<usize> {
        self.tabs
            .iter()
            .position(|tab| tab.contains(pane))
            .ok_or_else(|| Error::PaneNotFound(pane.clone()))
    }

    pub(crate) fn tab_mut(
        &mut self,
        id: TabId,
    ) -> Result<&mut Tab> {
        self.tabs
            .iter_mut()
            .find(|tab| tab.id() == id)
            .ok_or(Error::TabNotFound(id))
    }

    pub(crate) fn tab_of_mut(
        &mut self,
        pane: &PaneId,
    ) -> Result<&mut Tab> {
        self.tabs
            .iter_mut()
            .find(|tab| tab.contains(pane))
            .ok_or_else(|| Error::PaneNotFound(pane.clone()))
    }

    pub(crate) fn take_tab_at(
        &mut self,
        index: usize,
    ) -> Option<Tab> {
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

fn is_same_thing(
    instance: &PaneInstance,
    type_id: &PaneTypeId,
    params: &Params,
) -> bool {
    !params.is_empty() && &instance.type_id == type_id && &instance.params == params
}

fn has_duplicates<T: Ord>(items: &mut [T]) -> bool {
    items.sort();
    items.windows(2).any(|pair| pair.first() == pair.get(1))
}

impl Serialize for Layout {
    fn serialize<S: Serializer>(
        &self,
        serializer: S,
    ) -> std::result::Result<S::Ok, S::Error> {
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
mod tests {
    use std::collections::BTreeMap;

    use protocol::{PaneState, PaneTypeId, Params};
    use serde_json::json;

    use super::*;
    use crate::edge::Edge;
    use crate::placement::Placement;

    fn id(name: &str) -> PaneId {
        PaneId::new(name).unwrap()
    }

    fn instance(name: &str) -> PaneInstance {
        PaneInstance {
            pane_id: id(name),
            type_id: PaneTypeId::new("core.terminal").unwrap(),
            params: Params::default(),
            state: PaneState::default(),
        }
    }

    fn chat(
        name: &str,
        chat_id: &str,
    ) -> PaneInstance {
        PaneInstance {
            type_id: PaneTypeId::new("core.chat").unwrap(),
            params: Params::default().with("chatId", chat_id).unwrap(),
            ..instance(name)
        }
    }

    fn open(
        tab: u64,
        instance: PaneInstance,
    ) -> Command {
        Command::OpenPane {
            tab: TabId(tab),
            instance,
            placement: None,
        }
    }

    fn add(tab: u64) -> Command {
        Command::AddTab {
            id: TabId(tab),
            title: format!("tab {tab}"),
        }
    }

    fn layout(commands: Vec<Command>) -> Layout {
        let mut layout = Layout::default();
        for command in commands {
            layout.apply(command).unwrap();
        }
        layout.validate().unwrap();
        layout
    }

    fn move_onto_itself(name: &str) -> Command {
        let placement = Placement {
            target: id(name),
            edge: Edge::Left,
        };
        Command::MovePane {
            pane: id(name),
            placement,
        }
    }

    fn sample() -> Layout {
        layout(vec![
            add(1),
            open(1, chat("a", "c1")),
            open(1, instance("b")),
            add(2),
            open(2, instance("c")),
        ])
    }

    #[test]
    fn queries_find_tabs_and_panes() {
        let layout = sample();
        assert_eq!(layout.pane_count(), 3, "count");
        assert_eq!(
            layout.tab_of(&id("c")).map(Tab::id),
            Some(TabId(2)),
            "tab of"
        );
        assert_eq!(
            layout.pane(&id("b")).map(|p| p.pane_id.clone()),
            Some(id("b")),
            "pane"
        );
        assert!(
            layout.pane(&id("zz")).is_none() && layout.tab(TabId(9)).is_none(),
            "missing"
        );
        let found = layout.find_pane(
            &PaneTypeId::new("core.chat").unwrap(),
            &Params::default().with("chatId", "c1").unwrap(),
        );
        assert_eq!(
            found.map(|p| p.pane_id.clone()),
            Some(id("a")),
            "by identity"
        );
        let other_type = layout.find_pane(
            &PaneTypeId::new("core.terminal").unwrap(),
            &Params::default().with("chatId", "c1").unwrap(),
        );
        assert!(other_type.is_none(), "type is part of the identity");
        let no_params = layout.find_pane(
            &PaneTypeId::new("core.terminal").unwrap(),
            &Params::default(),
        );
        assert!(no_params.is_none(), "empty parameters identify nothing");
    }

    #[test]
    fn a_refused_command_changes_nothing() {
        let mut layout = sample();
        let before = layout.clone();
        let cases = [
            ("unknown tab", open(9, instance("n"))),
            ("duplicate pane", open(1, instance("a"))),
            ("close unknown tab", Command::CloseTab { tab: TabId(9) }),
            ("close unknown pane", Command::ClosePane { pane: id("zz") }),
            ("move onto itself", move_onto_itself("a")),
        ];
        for (name, command) in cases {
            assert!(layout.apply(command).is_err(), "{name}");
            assert_eq!(layout, before, "{name}: unchanged");
        }
    }

    #[test]
    fn validate_names_broken_invariants() {
        let mut duplicated = sample();
        duplicated.tabs.push(duplicated.tabs[0].clone());
        let mut same_chat = sample();
        same_chat.tabs[1] = Tab::restored(
            crate::tab_shape::TabShape {
                id: TabId(2),
                title: "two".into(),
                tree: Some(crate::split_node::SplitNode::leaf(id("d"))),
                focused: Some(id("d")),
            },
            [(id("d"), chat("d", "c1"))].into_iter().collect(),
        );
        let cases = [
            (
                "active tab missing",
                Layout {
                    active: Some(TabId(9)),
                    ..sample()
                },
            ),
            (
                "tabs but no active",
                Layout {
                    active: None,
                    ..sample()
                },
            ),
            ("tab id and panes twice", duplicated),
            ("one chat open twice", same_chat),
        ];
        for (name, broken) in cases {
            assert!(broken.validate().is_err(), "{name}");
        }
    }

    #[test]
    fn a_layout_round_trips_through_json_with_its_version() {
        let layout = sample();
        let value = serde_json::to_value(&layout).unwrap();
        assert_eq!(value["version"], json!(SCHEMA_VERSION), "versioned");
        let back: Layout = serde_json::from_value(value).unwrap();
        assert_eq!(back, layout, "round trip");
    }

    #[test]
    fn an_unversioned_layout_from_the_first_format_is_migrated() {
        let mut value = serde_json::to_value(sample()).unwrap();
        value.as_object_mut().unwrap().remove("version");
        let migrated: Layout = serde_json::from_value(value).unwrap();
        assert_eq!(migrated, sample(), "same layout");
    }

    #[test]
    fn a_layout_from_a_newer_app_or_with_stray_data_is_refused() {
        let value = serde_json::to_value(sample()).unwrap();
        let cases = [
            ("newer", "version", json!(SCHEMA_VERSION + 1), "newer"),
            ("unknown field", "surprise", json!(true), ""),
            ("dangling active tab", "active", json!(77), "invalid layout"),
        ];
        for (name, field, bad, expected) in cases {
            let mut document = value.clone();
            document[field] = bad;
            let error = serde_json::from_value::<Layout>(document)
                .unwrap_err()
                .to_string();
            assert!(error.contains(expected), "{name}: {error}");
        }
    }

    #[test]
    fn swapping_in_an_arrangement_keeps_live_panes_and_parks_the_rest() {
        let mut layout = sample();
        let before = layout.arrangement();
        let removed = layout
            .execute(Command::ClosePane { pane: id("b") })
            .unwrap();
        let step = UndoStep {
            arrangement: before.clone(),
            parked: removed
                .into_iter()
                .map(|p| (p.pane_id.clone(), p))
                .collect(),
        };
        let after_close = layout.arrangement();
        let inverse = layout.swap_arrangement(step).unwrap();
        assert_eq!(layout.arrangement(), before, "arrangement back");
        assert_eq!(layout.pane_count(), 3, "pane back");
        assert!(
            inverse.parked.is_empty() && inverse.arrangement == after_close,
            "inverse: {inverse:?}"
        );
        layout.validate().unwrap();
    }

    #[test]
    fn an_arrangement_naming_a_vanished_pane_is_refused_untouched() {
        let mut layout = sample();
        let before = layout.clone();
        let mut arrangement = layout.arrangement();
        arrangement.tabs[0].tree = Some(crate::split_node::SplitNode::leaf(id("ghost")));
        let step = UndoStep {
            arrangement,
            parked: BTreeMap::new(),
        };
        assert!(layout.swap_arrangement(step).is_err(), "refused");
        assert_eq!(layout, before, "layout untouched");
    }
}
