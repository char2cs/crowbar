use std::collections::BTreeSet;

use protocol::PaneId;

use crate::error::{Error, Result};
use crate::layout::Layout;
use crate::layout::command::pane_commands;
use crate::layout::tab::pane_instance::PaneInstance;

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
    use crate::layout::command::Command;
    use crate::layout::layout_history::LayoutHistory;
    use crate::tab_id::TabId;
    use crate::test_support::layout_fixtures::{instance, stacked, strip};
    use crate::test_support::tab_fixtures::{at, id};
    use crate::tree::edge::Edge;

    fn close(names: &[&str]) -> Command {
        let panes = names.iter().map(|name| id(name)).collect();
        Command::ClosePanes { panes }
    }

    fn shown_of(layout: &Layout) -> Option<PaneId> {
        let tab = layout.showing("")?;
        let first = tab.pane_ids().into_iter().next()?;
        tab.group_of(&first)?.active_tab().cloned()
    }

    #[test]
    fn closing_several_panes_leaves_the_rest_of_the_strip_and_a_shown_survivor() {
        let cases = [
            (vec!["b", "d"], "a c e", "e"),
            (vec!["b", "c", "d", "e"], "a", "a"),
            (vec!["a", "b", "c", "d"], "e", "e"),
            (vec![], "a b c d e", "e"),
        ];
        for (closing, expected, shown) in cases {
            let mut layout = stacked(&["a", "b", "c", "d", "e"]);
            layout.apply(close(&closing)).unwrap();
            layout.validate().unwrap();
            assert_eq!(strip(&layout), expected, "{closing:?}");
            assert_eq!(shown_of(&layout), Some(id(shown)), "{closing:?}: shown");
        }
        let mut layout = stacked(&["a", "b"]);
        layout.apply(close(&["a", "b"])).unwrap();
        assert!(layout.tabs().is_empty(), "no tab is left empty");
    }

    #[test]
    fn closing_a_whole_group_gives_its_space_to_the_neighbour_and_focus_follows() {
        let mut layout = stacked(&["a", "b"]);
        let placement = Some(at("a", Edge::Right));
        let (tab, instance) = (TabId(1), instance("z"));
        let beside = Command::OpenPane {
            tab,
            instance,
            placement,
        };
        layout.apply(beside).unwrap();
        layout.apply(Command::FocusPane { pane: id("a") }).unwrap();
        layout.apply(close(&["a", "b"])).unwrap();
        layout.validate().unwrap();
        assert_eq!(layout.pane_count(), 1, "z is left");
        let focused = layout.showing("").and_then(|tab| tab.focused().cloned());
        assert_eq!(focused, Some(id("z")), "focus moved to z");
    }

    #[test]
    fn a_bulk_close_is_one_undo_step_and_a_bad_list_is_refused() {
        let mut history = LayoutHistory::new(stacked(&["a", "b", "c", "d"]));
        history.apply(close(&["b", "c"])).unwrap();
        assert_eq!(strip(history.layout()), "a d", "closed");
        assert!(history.undo(), "one step");
        assert_eq!(strip(history.layout()), "a b c d", "restored in order");
        assert!(!history.undo(), "nothing more to undo");
        let cases = [
            (close(&["a", "zz"]), Error::PaneNotFound(id("zz"))),
            (close(&["a", "a"]), Error::DuplicatePane(id("a"))),
        ];
        for (command, error) in cases {
            let mut layout = stacked(&["a", "b"]);
            let before = layout.clone();
            assert_eq!(layout.apply(command), Err(error));
            assert_eq!(layout, before, "unchanged");
        }
    }
}
