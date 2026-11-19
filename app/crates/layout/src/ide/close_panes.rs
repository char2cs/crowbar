use std::collections::BTreeSet;

use protocol::PaneId;

use crate::error::{Error, Result};
use crate::layout::Layout;
use crate::pane_commands;
use crate::pane_instance::PaneInstance;

pub(crate) fn close_many(
    layout: &mut Layout,
    panes: &[PaneId],
) -> Result<Vec<PaneInstance>> {
    check_all(layout, panes)?;
    let closed: Result<Vec<Vec<PaneInstance>>> = panes
        .iter()
        .map(|pane| pane_commands::close(layout, pane))
        .collect();
    Ok(closed?.into_iter().flatten().collect())
}

fn check_all(
    layout: &Layout,
    panes: &[PaneId],
) -> Result<()> {
    let mut seen = BTreeSet::new();
    panes.iter().try_for_each(|pane| {
        layout.index_of_pane(pane)?;
        seen.insert(pane)
            .then_some(())
            .ok_or_else(|| Error::DuplicatePane(pane.clone()))
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::command::Command;
    use crate::edge::Edge;
    use crate::layout_history::LayoutHistory;
    use crate::placement::Placement;
    use crate::tab_id::TabId;
    use crate::test_support::layout_fixtures::{instance, stacked, strip};
    use crate::test_support::tab_fixtures::id;

    fn close(names: &[&str]) -> Command {
        Command::ClosePanes {
            panes: names.iter().map(|name| id(name)).collect(),
        }
    }

    fn beside(layout: &mut Layout) {
        let placement = Placement {
            target: id("a"),
            edge: Edge::Right,
        };
        let open = Command::OpenPane {
            tab: TabId(1),
            instance: instance("z"),
            placement: Some(placement),
        };
        layout.apply(open).unwrap();
        layout.apply(Command::FocusPane { pane: id("a") }).unwrap();
    }

    fn shown_of(layout: &Layout) -> Option<PaneId> {
        let tab = layout.active_tab()?;
        let first = tab.pane_ids().into_iter().next()?;
        tab.group_of(&first)
            .and_then(|group| group.active_tab().cloned())
    }

    fn focused(layout: &Layout) -> Option<PaneId> {
        layout.active_tab().and_then(|tab| tab.focused().cloned())
    }

    #[test]
    fn closing_several_panes_leaves_the_rest_of_the_strip_and_a_shown_survivor() {
        let cases = [
            ("some", vec!["b", "d"], "a c e", "e"),
            ("all but the first", vec!["b", "c", "d", "e"], "a", "a"),
            ("all but the last", vec!["a", "b", "c", "d"], "e", "e"),
            ("none", vec![], "a b c d e", "e"),
        ];
        for (name, closing, expected, shown) in cases {
            let mut layout = stacked(&["a", "b", "c", "d", "e"]);
            layout.apply(close(&closing)).unwrap();
            layout.validate().unwrap();
            assert_eq!(strip(&layout), expected, "{name}");
            assert_eq!(shown_of(&layout), Some(id(shown)), "{name}: shown");
        }
    }

    #[test]
    fn closing_a_whole_group_gives_its_space_to_the_neighbour_and_focus_follows() {
        let mut layout = stacked(&["a", "b"]);
        beside(&mut layout);
        layout.apply(close(&["a", "b"])).unwrap();
        layout.validate().unwrap();
        assert_eq!(layout.pane_count(), 1, "z is left");
        assert_eq!(focused(&layout), Some(id("z")), "focus moved to z");
    }

    #[test]
    fn closing_every_pane_of_the_last_tab_closes_the_tab() {
        let mut layout = stacked(&["a", "b"]);
        layout.apply(close(&["a", "b"])).unwrap();
        assert!(layout.tabs().is_empty(), "no tab is left empty");
    }

    #[test]
    fn a_bad_list_is_refused_without_changes() {
        let cases = [
            (
                "unknown",
                close(&["a", "zz"]),
                Error::PaneNotFound(id("zz")),
            ),
            ("twice", close(&["a", "a"]), Error::DuplicatePane(id("a"))),
        ];
        for (name, command, error) in cases {
            let mut layout = stacked(&["a", "b"]);
            let before = layout.clone();
            assert_eq!(layout.apply(command), Err(error), "{name}");
            assert_eq!(layout, before, "{name}: unchanged");
        }
    }

    #[test]
    fn a_bulk_close_is_one_undo_step_that_brings_back_the_strip_in_order() {
        let mut history = LayoutHistory::new(stacked(&["a", "b", "c", "d"]));
        history.apply(close(&["b", "c"])).unwrap();
        assert_eq!(strip(history.layout()), "a d", "closed");
        assert!(history.undo(), "one step");
        assert_eq!(strip(history.layout()), "a b c d", "restored in order");
        assert!(!history.undo(), "nothing more to undo");
    }
}
