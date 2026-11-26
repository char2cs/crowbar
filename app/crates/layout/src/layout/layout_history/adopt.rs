use crate::layout::layout_history::LayoutHistory;
use crate::space_id::SpaceId;
use crate::tab_id::TabId;

impl LayoutHistory {
    pub(crate) fn carry_adoption(
        &mut self,
        (tab, space): (TabId, &SpaceId),
    ) {
        for step in self.undo.iter_mut().chain(&mut self.redo) {
            step.arrangement.adopt((tab, space));
            let unowned_changed = step.spaces.contains("");
            step.spaces.extend(unowned_changed.then(|| space.clone()));
        }
    }
}

#[cfg(test)]
mod tests {
    use crate::layout::command::Command;
    use crate::layout::layout_history::LayoutHistory;
    use crate::layout::tab::Tab;
    use crate::space_id::SpaceId;
    use crate::tab_id::TabId;
    use crate::test_support::layout_fixtures::{instance, open};

    fn add(
        id: u64,
        space: &str,
    ) -> Command {
        Command::AddTab {
            id: TabId(id),
            title: String::new(),
            space: SpaceId::new(space),
        }
    }

    fn adopt(space: &str) -> Command {
        Command::AdoptSpace {
            tab: TabId(1),
            space: SpaceId::new(space),
        }
    }

    fn space_of_first(history: &LayoutHistory) -> Option<&str> {
        history.layout().tab(TabId(1)).map(Tab::space)
    }

    #[test]
    fn an_unowned_tab_is_adopted_once_and_undo_and_redo_keep_its_space() {
        let mut history = LayoutHistory::default();
        for step in [
            add(1, ""),
            open(1, instance("a")),
            add(2, "p1"),
            open(2, instance("b")),
        ] {
            history.apply(step).unwrap();
        }
        history
            .apply(Command::ActivateTab { tab: TabId(1) })
            .unwrap();
        history.apply(adopt("p2")).unwrap();
        let layout = history.layout();
        assert_eq!(space_of_first(&history), Some("p2"), "adopted");
        assert_eq!(
            layout.showing("p2").map(Tab::id),
            Some(TabId(1)),
            "p2 shows it"
        );
        assert!(
            layout.showing("").is_none(),
            "the empty space shows nothing"
        );
        assert!(history.apply(adopt("p3")).is_err(), "only once");
        assert!(history.undo() && history.undo(), "undo the second tab");
        assert_eq!(space_of_first(&history), Some("p2"), "undo keeps the space");
        assert!(history.redo(), "redo");
        assert_eq!(space_of_first(&history), Some("p2"), "redo keeps the space");
        history.layout().validate().unwrap();
    }
}
