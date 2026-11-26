//! Commands that change one group's own state: its pins, its preview and its editor view.

use protocol::PaneId;

use crate::error::{Error, Result};
use crate::ide::pane_group::PaneGroup;
use crate::ide::presentation::editor_view::EditorView;
use crate::layout::Layout;
use crate::layout::command::Command;

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

pub(crate) fn group_of<'a>(
    layout: &'a mut Layout,
    pane: &PaneId,
) -> Result<&'a mut crate::ide::pane_group::PaneGroup> {
    layout
        .tab_of_mut(pane)?
        .tree
        .as_mut()
        .and_then(|tree| tree.group_of_mut(pane))
        .ok_or_else(|| Error::PaneNotFound(pane.clone()))
}

pub(crate) fn set_editor_view(
    layout: &mut Layout,
    pane: &PaneId,
    view: EditorView,
) -> Result<()> {
    group_of(layout, pane)?.editor_open = view == EditorView::Open;
    Ok(())
}

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
    trees.for_each(crate::tree::split_node::SplitNode::clear_previews);
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::ide::presentation::editor_view::EditorView::{Closed, Open};
    use crate::ide::presentation::sidebar_side::SidebarSide;
    use crate::ide::presentation::{DEFAULT_CHAT_SHARE, Presentation};
    use crate::layout::layout_history::LayoutHistory;
    use crate::layout::stored::layout_stored::{load, store};
    use crate::tab_id::TabId;
    use crate::test_support::layout_fixtures::{add, chat, instance, layout, open, stacked, strip};
    use crate::test_support::tab_fixtures::{id, screen};

    fn pin_cmd(name: &str) -> Command {
        Command::PinPane { pane: id(name) }
    }

    fn unpin_cmd(name: &str) -> Command {
        Command::UnpinPane { pane: id(name) }
    }

    #[test]
    fn pinning_and_unpinning_reorder_the_strip_and_keep_focus() {
        let cases = [
            (vec![pin_cmd("c")], "[c] a b"),
            (vec![pin_cmd("c"), pin_cmd("b")], "[c] [b] a"),
            (vec![pin_cmd("b"), pin_cmd("b")], "[b] a c"),
            (vec![pin_cmd("c"), pin_cmd("b"), unpin_cmd("c")], "[b] c a"),
            (vec![unpin_cmd("a")], "a b c"),
        ];
        for (commands, expected) in cases {
            let mut layout = stacked(&["a", "b", "c"]);
            commands.into_iter().for_each(|c| layout.apply(c).unwrap());
            layout.validate().unwrap();
            assert_eq!(strip(&layout), expected);
            let focused = layout.showing("").and_then(|tab| tab.focused().cloned());
            assert_eq!(focused, Some(id("c")), "{expected}: focus unchanged");
        }
        for command in [pin_cmd("zz"), unpin_cmd("zz")] {
            let mut layout = stacked(&["a", "b"]);
            let before = layout.clone();
            let refused = Err(Error::PaneNotFound(id("zz")));
            assert_eq!(layout.apply(command), refused, "unknown pane");
            assert_eq!(layout, before, "unchanged");
        }
    }

    #[test]
    fn a_pin_is_one_undo_step_and_survives_the_stored_form() {
        let mut history = LayoutHistory::new(stacked(&["a", "b", "c"]));
        history.apply(pin_cmd("b")).unwrap();
        history.apply(pin_cmd("b")).unwrap();
        assert_eq!(strip(history.layout()), "[b] a c", "pinned");
        let stored = serde_json::to_value(history.layout()).unwrap();
        let loaded: Layout = serde_json::from_value(stored).unwrap();
        assert_eq!(strip(&loaded), "[b] a c", "round trip");
        assert!(history.undo(), "one step");
        assert_eq!(strip(history.layout()), "a b c", "unpinned again");
        assert!(!history.undo(), "no second step");
        assert!(history.redo(), "redo");
        assert_eq!(strip(history.layout()), "[b] a c", "pinned again");
    }

    fn preview(name: &str) -> Command {
        Command::PreviewPane { pane: id(name) }
    }

    fn preview_in(
        layout: &Layout,
        name: &str,
    ) -> Option<String> {
        let tab = layout.tab_of(&id(name)).unwrap();
        let group = tab.group_of(&id(name)).unwrap();
        group.preview().map(ToString::to_string)
    }

    fn applied(commands: Vec<Command>) -> Layout {
        let mut layout = stacked(&["a", "b", "c"]);
        let other_tab = [add(2), open(2, instance("x"))];
        for command in other_tab.into_iter().chain(commands) {
            layout.apply(command).unwrap();
        }
        layout.validate().unwrap();
        layout
    }

    #[test]
    fn a_group_holds_one_preview_until_it_is_kept_closed_or_moved() {
        let keep = Command::KeepPane { pane: id("a") };
        let close = Command::ClosePane { pane: id("b") };
        let (pane, onto) = (id("b"), id("x"));
        let moved = Command::MoveToGroup {
            pane,
            onto,
            index: None,
        };
        let cases = [
            (vec![], None),
            (vec![preview("b")], Some("b")),
            (vec![preview("b"), preview("c")], Some("c")),
            (vec![preview("b"), preview("x"), keep], None),
            (vec![preview("b"), close], None),
            (vec![preview("b"), moved], None),
        ];
        for (commands, expected) in cases {
            let named = format!("{commands:?}");
            let layout = applied(commands);
            let expected = expected.map(ToOwned::to_owned);
            assert_eq!(preview_in(&layout, "a"), expected, "{named}");
            assert_eq!(preview_in(&layout, "x"), None, "{named}: tab 2");
        }
    }

    #[test]
    fn only_an_open_editor_tab_can_be_a_preview() {
        let mut layout = layout(vec![add(1), open(1, chat("a", "c1"))]);
        let chat_refused = Err(Error::ChatIsNotATab(id("a")));
        assert_eq!(layout.apply(preview("a")), chat_refused, "chat");
        let missing = Err(Error::PaneNotFound(id("zz")));
        assert_eq!(layout.apply(preview("zz")), missing, "unknown");
        let keep = Command::KeepPane { pane: id("zz") };
        assert_eq!(layout.apply(keep), missing, "keep unknown");
    }

    #[test]
    fn a_preview_is_no_undo_step_but_survives_the_stored_form() {
        let mut history = LayoutHistory::new(stacked(&["a", "b"]));
        history.apply(preview("b")).unwrap();
        let loaded = load(&store(history.layout()).unwrap()).unwrap();
        assert_eq!(preview_in(&loaded, "b").as_deref(), Some("b"), "stored");
        assert!(!history.undo(), "no undo step");
    }

    fn set(
        pane: &str,
        view: EditorView,
    ) -> Command {
        let pane = id(pane);
        Command::SetEditorView { pane, view }
    }

    #[test]
    fn opening_the_editor_shows_it_beside_the_chat_and_closing_it_hides_it() {
        let cases: [(&[EditorView], bool); 3] =
            [(&[], false), (&[Open], true), (&[Open, Closed], false)];
        for (steps, editor) in cases {
            let mut layout = layout(vec![add(1), open(1, chat("a", "c1"))]);
            steps.iter().for_each(|view| layout.apply(set("a", *view)).unwrap());
            let group = layout.tab(TabId(1)).unwrap().group_of(&id("a")).unwrap();
            let area = screen(1200.0, 800.0);
            let shown = Presentation::of(group, area, SidebarSide::Left, DEFAULT_CHAT_SHARE);
            assert_eq!(shown.editor.is_some(), editor, "{steps:?}");
            assert!(shown.chat.is_some(), "{steps:?}: the chat stays");
            let refused = layout.apply(set("zz", Open));
            assert_eq!(refused, Err(Error::PaneNotFound(id("zz"))), "unknown");
        }
    }
}
