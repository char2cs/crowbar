use protocol::PaneId;

use crate::error::{Error, Result};
use crate::layout::Layout;

pub(crate) fn pin(
    layout: &mut Layout,
    pane: &PaneId,
) -> Result<()> {
    group_of(layout, pane)?.pin(pane);
    Ok(())
}

pub(crate) fn unpin(
    layout: &mut Layout,
    pane: &PaneId,
) -> Result<()> {
    group_of(layout, pane)?.unpin(pane);
    Ok(())
}

fn group_of<'a>(
    layout: &'a mut Layout,
    pane: &PaneId,
) -> Result<&'a mut crate::group::Group> {
    layout
        .tab_of_mut(pane)?
        .tree
        .as_mut()
        .and_then(|tree| tree.group_of_mut(pane))
        .ok_or_else(|| Error::PaneNotFound(pane.clone()))
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::command::Command;
    use crate::layout_history::LayoutHistory;
    use crate::test_support::layout_fixtures::{stacked, strip};
    use crate::test_support::tab_fixtures::id;

    fn pin_cmd(name: &str) -> Command {
        Command::PinPane { pane: id(name) }
    }

    fn unpin_cmd(name: &str) -> Command {
        Command::UnpinPane { pane: id(name) }
    }

    fn applied(commands: Vec<Command>) -> Layout {
        let mut layout = stacked(&["a", "b", "c"]);
        for command in commands {
            layout.apply(command).unwrap();
        }
        layout.validate().unwrap();
        layout
    }

    #[test]
    fn pinning_and_unpinning_reorder_the_strip_and_keep_focus() {
        let cases = [
            ("pin the last", vec![pin_cmd("c")], "[c] a b"),
            ("pin two", vec![pin_cmd("c"), pin_cmd("b")], "[c] [b] a"),
            ("pin twice", vec![pin_cmd("b"), pin_cmd("b")], "[b] a c"),
            (
                "unpin",
                vec![pin_cmd("c"), pin_cmd("b"), unpin_cmd("c")],
                "[b] c a",
            ),
            ("unpin a loose one", vec![unpin_cmd("a")], "a b c"),
        ];
        for (name, commands, expected) in cases {
            let layout = applied(commands);
            assert_eq!(strip(&layout), expected, "{name}");
            let focused = layout.active_tab().and_then(|tab| tab.focused().cloned());
            assert_eq!(focused, Some(id("c")), "{name}: focus unchanged");
        }
    }

    #[test]
    fn an_unknown_pane_is_refused_without_changes() {
        for command in [pin_cmd("zz"), unpin_cmd("zz")] {
            let mut layout = stacked(&["a", "b"]);
            let before = layout.clone();
            assert_eq!(
                layout.apply(command),
                Err(Error::PaneNotFound(id("zz"))),
                "refused"
            );
            assert_eq!(layout, before, "unchanged");
        }
    }

    #[test]
    fn a_pin_is_one_undo_step_and_a_pin_that_changes_nothing_is_none() {
        let mut history = LayoutHistory::new(stacked(&["a", "b", "c"]));
        history.apply(pin_cmd("b")).unwrap();
        history.apply(pin_cmd("b")).unwrap();
        assert_eq!(strip(history.layout()), "[b] a c", "pinned");
        assert!(history.undo(), "one step");
        assert_eq!(strip(history.layout()), "a b c", "unpinned again");
        assert!(!history.undo(), "no second step");
        assert!(history.redo(), "redo");
        assert_eq!(strip(history.layout()), "[b] a c", "pinned again");
    }

    #[test]
    fn a_pinned_strip_survives_the_stored_form() {
        let mut layout = stacked(&["a", "b", "c"]);
        layout.apply(pin_cmd("c")).unwrap();
        let stored = serde_json::to_value(&layout).unwrap();
        let loaded: Layout = serde_json::from_value(stored).unwrap();
        assert_eq!(strip(&loaded), "[c] a b", "round trip");
    }
}
