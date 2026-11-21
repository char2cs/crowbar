use protocol::PaneId;

use crate::command::Command;
use crate::error::{Error, Result};
use crate::ide::pane_group::PaneGroup;
use crate::ide::pin_commands::group_of;
use crate::layout::Layout;

impl PaneGroup {
    /// The editor tab opened as a preview, which develop writes in italics until it is kept.
    #[must_use]
    pub fn preview(&self) -> Option<&PaneId> {
        self.preview.as_ref()
    }
}

pub(crate) fn execute(
    layout: &mut Layout,
    command: Command,
) -> Result<()> {
    match command {
        Command::PreviewPane { pane } => preview(layout, &pane),
        Command::KeepPane { pane } => keep(layout, &pane),
        _ => Ok(()),
    }
}

fn preview(
    layout: &mut Layout,
    pane: &PaneId,
) -> Result<()> {
    let group = group_of(layout, pane)?;
    if !group.tabs.contains(pane) {
        return Err(Error::ChatIsNotATab(pane.clone()));
    }
    group.preview = Some(pane.clone());
    Ok(())
}

fn keep(
    layout: &mut Layout,
    pane: &PaneId,
) -> Result<()> {
    layout.index_of_pane(pane)?;
    let trees = layout.tabs.iter_mut().filter_map(|tab| tab.tree.as_mut());
    trees.for_each(crate::split_node::SplitNode::clear_previews);
    Ok(())
}

#[cfg(test)]
mod tests {
    use crate::command::Command;
    use crate::error::Error;
    use crate::layout::Layout;
    use crate::layout_history::LayoutHistory;
    use crate::test_support::layout_fixtures::{add, chat, instance, layout, open, stacked};
    use crate::test_support::tab_fixtures::id;

    fn preview(name: &str) -> Command {
        Command::PreviewPane { pane: id(name) }
    }

    fn keep(name: &str) -> Command {
        Command::KeepPane { pane: id(name) }
    }

    fn preview_in(
        layout: &Layout,
        name: &str,
    ) -> Option<String> {
        let tab = layout.tab_of(&id(name)).unwrap();
        let group = tab.group_of(&id(name)).unwrap();
        group.preview().map(ToString::to_string)
    }

    fn applied(
        mut layout: Layout,
        commands: Vec<Command>,
    ) -> Layout {
        for command in commands {
            layout.apply(command).unwrap();
        }
        layout
    }

    #[test]
    fn a_group_holds_one_preview_the_last_one_opened() {
        let cases = [
            ("one", vec![preview("b")], Some("b")),
            (
                "the next replaces it",
                vec![preview("b"), preview("c")],
                Some("c"),
            ),
            ("none yet", vec![], None),
        ];
        for (name, commands, expected) in cases {
            let layout = applied(stacked(&["a", "b", "c"]), commands);
            let expected = expected.map(ToOwned::to_owned);
            assert_eq!(preview_in(&layout, "a"), expected, "{name}");
        }
    }

    #[test]
    fn keeping_a_pane_ends_every_preview_in_every_tab() {
        let two_tabs = applied(stacked(&["a", "b"]), vec![add(2), open(2, instance("x"))]);
        let layout = applied(two_tabs, vec![preview("b"), preview("x"), keep("a")]);
        assert_eq!(preview_in(&layout, "b"), None, "tab 1");
        assert_eq!(preview_in(&layout, "x"), None, "tab 2");
    }

    #[test]
    fn a_closed_or_moved_away_preview_is_no_longer_the_group_s() {
        let closed = applied(
            stacked(&["a", "b", "c"]),
            vec![preview("b"), Command::ClosePane { pane: id("b") }],
        );
        assert_eq!(preview_in(&closed, "a"), None, "closed");
        closed.validate().unwrap();
        let elsewhere = applied(
            stacked(&["a", "b", "c"]),
            vec![preview("b"), add(2), open(2, instance("x"))],
        );
        let moved = Command::MoveToGroup {
            pane: id("b"),
            onto: id("x"),
            index: None,
        };
        let moved = applied(elsewhere, vec![moved]);
        assert_eq!(preview_in(&moved, "a"), None, "left behind");
        moved.validate().unwrap();
    }

    #[test]
    fn only_an_open_editor_tab_can_be_a_preview() {
        let mut layout = layout(vec![add(1), open(1, chat("a", "c1"))]);
        assert_eq!(
            layout.apply(preview("a")),
            Err(Error::ChatIsNotATab(id("a")))
        );
        assert_eq!(
            layout.apply(preview("zz")),
            Err(Error::PaneNotFound(id("zz")))
        );
        assert_eq!(layout.apply(keep("zz")), Err(Error::PaneNotFound(id("zz"))));
    }

    #[test]
    fn a_preview_is_no_undo_step_but_survives_the_stored_form() {
        let mut history = LayoutHistory::new(stacked(&["a", "b"]));
        history.apply(preview("b")).unwrap();
        assert!(!history.can_undo(), "no undo step");
        let bytes = crate::model::store(history.layout()).unwrap();
        let loaded = crate::model::load(&bytes).unwrap();
        assert_eq!(preview_in(&loaded, "b").as_deref(), Some("b"), "stored");
    }
}
