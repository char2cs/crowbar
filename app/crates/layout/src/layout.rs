mod arrangement;
pub(crate) mod command;
pub(crate) mod layout_history;
mod layout_tabs;
pub(crate) mod stored;
pub(crate) mod tab;
mod undo_step;

use std::collections::BTreeMap;

use protocol::{PaneId, PaneTypeId, Params};

use crate::error::Result;
use crate::layout::command::Command;
use crate::layout::tab::Tab;
use crate::layout::tab::pane_instance::PaneInstance;
use crate::space_id::SpaceId;
use crate::tab_id::TabId;

pub const MAX_TABS: usize = 256;

#[derive(Debug, Clone, Default, PartialEq)]
pub struct Layout {
    pub(crate) tabs: Vec<Tab>,
    pub(crate) shown: BTreeMap<SpaceId, TabId>,
}

impl Layout {
    #[must_use]
    pub fn tabs(&self) -> &[Tab] {
        &self.tabs
    }

    #[must_use]
    pub fn tab(
        &self,
        id: TabId,
    ) -> Option<&Tab> {
        self.tabs.iter().find(|tab| tab.id() == id)
    }

    #[must_use]
    pub fn showing(
        &self,
        space: &str,
    ) -> Option<&Tab> {
        let remembered = self.shown.get(space).and_then(|id| self.tab(*id));
        remembered.or_else(|| self.tabs.iter().find(|tab| tab.space() == space))
    }

    #[must_use]
    pub fn tab_of(
        &self,
        pane: &PaneId,
    ) -> Option<&Tab> {
        self.tabs.iter().find(|tab| tab.contains(pane))
    }

    #[must_use]
    pub fn pane(
        &self,
        id: &PaneId,
    ) -> Option<&PaneInstance> {
        self.tab_of(id).and_then(|tab| tab.pane(id))
    }

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

    #[must_use]
    pub fn pane_count(&self) -> usize {
        self.tabs.iter().map(Tab::len).sum()
    }

    pub fn apply(
        &mut self,
        command: Command,
    ) -> Result<()> {
        self.execute(command).map(drop)
    }
}

fn is_same_thing(
    instance: &PaneInstance,
    type_id: &PaneTypeId,
    params: &Params,
) -> bool {
    !params.is_empty() && &instance.type_id == type_id && &instance.params == params
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::layout_fixtures::{instance, move_onto_itself, open, sample};
    use crate::test_support::tab_fixtures::id;

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
}
