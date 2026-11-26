use std::collections::BTreeMap;

use crate::error::{Error, Result};
use crate::layout::tab::Tab;
use crate::layout::tab::pane_instance::PaneInstance;
use crate::layout::{Layout, MAX_TABS};
use crate::space_id::SpaceId;
use crate::tab_id::TabId;
use crate::tree::branch::Branch;

pub(crate) fn add(
    layout: &mut Layout,
    id: TabId,
    title: String,
    space: SpaceId,
) -> Result<()> {
    if layout.tab(id).is_some() {
        return Err(Error::DuplicateTab(id));
    }
    if layout.tabs.len() >= MAX_TABS {
        return Err(Error::TooManyTabs { limit: MAX_TABS });
    }
    layout.tabs.push(Tab::new(id, title, space));
    layout.show(id);
    Ok(())
}

pub(crate) fn close(
    layout: &mut Layout,
    id: TabId,
) -> Result<Vec<PaneInstance>> {
    let index = layout.index_of(id)?;
    let mut removed = layout.take_tab_at(index).ok_or(Error::TabNotFound(id))?;
    Ok(removed.take_instances().into_values().collect())
}

pub(crate) fn activate(
    layout: &mut Layout,
    id: TabId,
) -> Result<()> {
    layout.index_of(id)?;
    layout.show(id);
    Ok(())
}

pub(crate) fn adopt(
    layout: &mut Layout,
    id: TabId,
    space: SpaceId,
) -> Result<()> {
    let tab = layout.tab_mut(id)?;
    if !tab.space.is_unowned() {
        return Err(Error::Owned(id));
    }
    tab.space = space.clone();
    adopt_shown(&mut layout.shown, (id, space));
    Ok(())
}

pub(crate) fn adopt_shown(
    shown: &mut BTreeMap<SpaceId, TabId>,
    (id, space): (TabId, SpaceId),
) {
    if shown.get("") != Some(&id) {
        return;
    }
    shown.remove("");
    shown.entry(space).or_insert(id);
}

pub(crate) fn move_to(
    layout: &mut Layout,
    id: TabId,
    to_index: usize,
) -> Result<()> {
    let index = layout.index_of(id)?;
    let len = layout.tabs.len();
    (to_index < len)
        .then_some(())
        .ok_or(Error::IndexOutOfRange {
            index: to_index,
            len,
        })?;
    let tab = layout.tabs.remove(index);
    layout.tabs.insert(to_index, tab);
    Ok(())
}

pub(crate) fn rename(
    layout: &mut Layout,
    id: TabId,
    title: String,
) -> Result<()> {
    layout.tab_mut(id)?.set_title(title);
    Ok(())
}

pub(crate) fn equalize(
    layout: &mut Layout,
    id: TabId,
    path: &[Branch],
) -> Result<()> {
    layout.tab_mut(id)?.equalize(path)
}

#[cfg(test)]
mod tests {
    use crate::error::Error;
    use crate::error::Error::{DuplicateTab, IndexOutOfRange, NoSuchSplit, TabNotFound};
    use crate::layout::command::Command;
    use crate::layout::tab::Tab;
    use crate::layout::{Layout, MAX_TABS};
    use crate::tab_id::TabId;
    use crate::test_support::layout_fixtures::{add, instance, layout as build, open};
    use crate::test_support::tab_fixtures::screen;
    use crate::tree::branch::Branch;

    fn active(layout: &Layout) -> Option<TabId> {
        layout.showing("").map(Tab::id)
    }

    fn close(tab: u64) -> Command {
        Command::CloseTab { tab: TabId(tab) }
    }

    fn move_tab(
        tab: u64,
        to_index: usize,
    ) -> Command {
        let tab = TabId(tab);
        Command::MoveTab { tab, to_index }
    }

    fn rename(tab: u64) -> Command {
        let (tab, title) = (TabId(tab), "new".to_owned());
        Command::RenameTab { tab, title }
    }

    fn resize(
        tab: u64,
        first_extent: f32,
    ) -> Command {
        Command::ResizeSplit {
            tab: TabId(tab),
            path: Vec::new(),
            bounds: screen(1006.0, 400.0),
            first_extent,
        }
    }

    fn reset(path: Vec<Branch>) -> Command {
        let tab = TabId(1);
        Command::ResetSplit { tab, path }
    }

    fn two_panes() -> Layout {
        build(vec![add(1), open(1, instance("a")), open(1, instance("b"))])
    }

    fn widths(layout: &Layout) -> Vec<f32> {
        let slots = layout.tab(TabId(1)).unwrap().layout(screen(1006.0, 400.0));
        slots.iter().map(|slot| slot.rect.width).collect()
    }

    fn ids(layout: &Layout) -> Vec<u64> {
        layout.tabs().iter().map(|t| t.id().0).collect()
    }

    #[test]
    fn commands_naming_a_missing_tab_or_bad_target_are_refused() {
        let activate = Command::ActivateTab { tab: TabId(9) };
        let cases = [
            (add(1), DuplicateTab(TabId(1))),
            (close(9), TabNotFound(TabId(9))),
            (activate, TabNotFound(TabId(9))),
            (move_tab(3, 3), IndexOutOfRange { index: 3, len: 3 }),
            (move_tab(7, 0), TabNotFound(TabId(7))),
            (rename(8), TabNotFound(TabId(8))),
            (resize(5, 1.0), TabNotFound(TabId(5))),
            (reset(vec![Branch::First]), NoSuchSplit),
        ];
        for (command, error) in cases {
            let mut layout = two_panes();
            layout.apply(add(2)).unwrap();
            layout.apply(add(3)).unwrap();
            let before = layout.clone();
            assert_eq!(layout.apply(command), Err(error));
            assert_eq!(layout, before, "unchanged");
        }
    }

    #[test]
    fn the_number_of_tabs_is_bounded() {
        let commands = (0..MAX_TABS as u64).map(add).collect();
        let mut layout = build(commands);
        let before = layout.clone();
        let full = Err(Error::TooManyTabs { limit: MAX_TABS });
        assert_eq!(layout.apply(add(u64::MAX)), full, "full");
        assert_eq!(layout, before, "unchanged");
    }

    #[test]
    fn adding_activating_and_closing_tabs_choose_the_shown_one() {
        let mut layout = build(vec![add(1), add(2), add(3)]);
        assert_eq!(active(&layout), Some(TabId(3)), "added is shown");
        layout
            .apply(Command::ActivateTab { tab: TabId(2) })
            .unwrap();
        assert_eq!(active(&layout), Some(TabId(2)), "activated");
        layout.apply(close(2)).unwrap();
        assert_eq!(active(&layout), Some(TabId(3)), "next");
        layout.apply(close(1)).unwrap();
        assert_eq!(active(&layout), Some(TabId(3)), "inactive closed");
        layout.apply(close(3)).unwrap();
        assert!(active(&layout).is_none(), "none shown");
        assert!(layout.tabs().is_empty(), "none left");
        let mut layout = build(vec![add(1), add(2), add(3)]);
        layout.apply(close(3)).unwrap();
        assert_eq!(active(&layout), Some(TabId(2)), "previous");
    }

    #[test]
    fn closing_a_tab_removes_its_panes_and_reports_them() {
        let mut layout = two_panes();
        layout.apply(add(2)).unwrap();
        let removed = layout.execute(close(1)).unwrap();
        let mut ids: Vec<String> = removed.iter().map(|p| p.pane_id.to_string()).collect();
        ids.sort();
        assert_eq!(ids, ["a", "b"], "both panes handed back for undo");
        assert_eq!(layout.pane_count(), 0, "gone from the layout");
    }

    #[test]
    fn tabs_reorder_rename_and_resize() {
        let mut layout = two_panes();
        layout.apply(add(2)).unwrap();
        layout.apply(add(3)).unwrap();
        layout.apply(move_tab(3, 0)).unwrap();
        assert_eq!(ids(&layout), [3, 1, 2], "moved to front");
        layout.apply(rename(1)).unwrap();
        assert_eq!(layout.tab(TabId(1)).map(Tab::title), Some("new"), "title");
        layout.apply(resize(1, 300.0)).unwrap();
        assert_eq!(widths(&layout), [300.0, 700.0], "resized");
        layout.apply(reset(Vec::new())).unwrap();
        assert_eq!(widths(&layout), [500.0, 500.0], "reset");
    }
}
