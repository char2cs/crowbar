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
    use protocol::{PaneState, PaneTypeId, Params};

    use crate::error::Error;
    use crate::layout::command::Command;
    use crate::layout::tab::Tab;
    use crate::layout::tab::pane_instance::PaneInstance;
    use crate::layout::{Layout, MAX_TABS};
    use crate::tab_id::TabId;
    use crate::tree::geometry::rect::Rect;

    fn instance(name: &str) -> PaneInstance {
        PaneInstance {
            pane_id: protocol::PaneId::new(name).unwrap(),
            type_id: PaneTypeId::new("core.chat").unwrap(),
            params: Params::default(),
            state: PaneState::default(),
        }
    }

    fn add(tab: u64) -> Command {
        Command::AddTab {
            id: TabId(tab),
            title: format!("tab {tab}"),
            space: crate::SpaceId::default(),
        }
    }

    fn open(
        tab: u64,
        name: &str,
    ) -> Command {
        Command::OpenPane {
            tab: TabId(tab),
            instance: instance(name),
            placement: None,
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

    fn active(layout: &Layout) -> Option<TabId> {
        layout.showing("").map(Tab::id)
    }

    #[test]
    fn adding_a_tab_activates_it() {
        let layout = layout(vec![add(1), add(2)]);
        assert_eq!(active(&layout), Some(TabId(2)), "active");
        assert_eq!(layout.tabs().len(), 2, "count");
    }

    fn refusal_cases() -> Vec<(&'static str, Command, Error)> {
        vec![
            ("duplicate id", add(1), Error::DuplicateTab(TabId(1))),
            (
                "close missing",
                Command::CloseTab { tab: TabId(9) },
                Error::TabNotFound(TabId(9)),
            ),
            (
                "activate missing",
                Command::ActivateTab { tab: TabId(9) },
                Error::TabNotFound(TabId(9)),
            ),
            (
                "move out of range",
                Command::MoveTab {
                    tab: TabId(3),
                    to_index: 3,
                },
                Error::IndexOutOfRange { index: 3, len: 3 },
            ),
            (
                "move missing",
                Command::MoveTab {
                    tab: TabId(7),
                    to_index: 0,
                },
                Error::TabNotFound(TabId(7)),
            ),
            (
                "rename missing",
                Command::RenameTab {
                    tab: TabId(8),
                    title: "x".into(),
                },
                Error::TabNotFound(TabId(8)),
            ),
        ]
    }

    #[test]
    fn commands_naming_a_missing_tab_or_bad_target_are_refused() {
        for (name, command, error) in refusal_cases() {
            let mut layout = layout(vec![add(1), add(2), add(3)]);
            let before = layout.clone();
            assert_eq!(layout.apply(command), Err(error), "{name}");
            assert_eq!(layout, before, "{name}: unchanged");
        }
    }

    #[test]
    fn the_number_of_tabs_is_bounded() {
        let mut layout = Layout::default();
        for tab in 0..MAX_TABS as u64 {
            layout.apply(add(tab)).unwrap();
        }
        let before = layout.clone();
        assert_eq!(
            layout.apply(add(u64::MAX)),
            Err(Error::TooManyTabs { limit: MAX_TABS }),
            "full"
        );
        assert_eq!(layout, before, "unchanged");
    }

    #[test]
    fn closing_the_active_tab_activates_its_neighbour() {
        let mut layout = layout(vec![
            add(1),
            add(2),
            add(3),
            Command::ActivateTab { tab: TabId(2) },
        ]);
        layout.apply(Command::CloseTab { tab: TabId(2) }).unwrap();
        assert_eq!(active(&layout), Some(TabId(3)), "next");
        layout.apply(Command::CloseTab { tab: TabId(3) }).unwrap();
        assert_eq!(active(&layout), Some(TabId(1)), "previous");
        layout.apply(Command::CloseTab { tab: TabId(1) }).unwrap();
        assert!(
            active(&layout).is_none() && layout.tabs().is_empty(),
            "none left"
        );
        layout.validate().unwrap();
    }

    #[test]
    fn closing_an_inactive_tab_keeps_the_active_one() {
        let mut layout = layout(vec![add(1), add(2)]);
        layout.apply(Command::CloseTab { tab: TabId(1) }).unwrap();
        assert_eq!(active(&layout), Some(TabId(2)), "active kept");
    }

    #[test]
    fn closing_a_tab_removes_its_panes_and_reports_them() {
        let mut layout = layout(vec![add(1), open(1, "a"), open(1, "b"), add(2)]);
        let removed = layout.execute(Command::CloseTab { tab: TabId(1) }).unwrap();
        let mut ids: Vec<String> = removed.iter().map(|p| p.pane_id.to_string()).collect();
        ids.sort();
        assert_eq!(ids, ["a", "b"], "both panes handed back for undo");
        assert_eq!(layout.pane_count(), 0, "gone from the layout");
        assert_eq!(
            layout.apply(Command::CloseTab { tab: TabId(1) }),
            Err(Error::TabNotFound(TabId(1))),
            "again"
        );
    }

    #[test]
    fn activating_needs_an_existing_tab() {
        let mut layout = layout(vec![add(1), add(2)]);
        layout
            .apply(Command::ActivateTab { tab: TabId(1) })
            .unwrap();
        assert_eq!(active(&layout), Some(TabId(1)), "switched");
    }

    #[test]
    fn tabs_reorder_within_range() {
        let mut layout = layout(vec![add(1), add(2), add(3)]);
        layout
            .apply(Command::MoveTab {
                tab: TabId(3),
                to_index: 0,
            })
            .unwrap();
        let ids: Vec<u64> = layout.tabs().iter().map(|t| t.id().0).collect();
        assert_eq!(ids, [3, 1, 2], "moved to front");
    }

    #[test]
    fn rename_changes_only_the_title() {
        let mut layout = layout(vec![add(1)]);
        layout
            .apply(Command::RenameTab {
                tab: TabId(1),
                title: "new".into(),
            })
            .unwrap();
        assert_eq!(layout.tab(TabId(1)).map(Tab::title), Some("new"), "title");
    }

    const BOUNDS: Rect = Rect {
        x: 0.0,
        y: 0.0,
        width: 1006.0,
        height: 400.0,
    };

    fn widths(layout: &Layout) -> Vec<f32> {
        layout.tab(TabId(1)).map_or_else(Vec::new, |tab| {
            tab.layout(BOUNDS).iter().map(|s| s.rect.width).collect()
        })
    }

    #[test]
    fn resize_and_reset_address_a_split_by_path() {
        let mut layout = layout(vec![add(1), open(1, "a"), open(1, "b")]);
        let bounds = BOUNDS;
        layout
            .apply(Command::ResizeSplit {
                tab: TabId(1),
                path: Vec::new(),
                bounds,
                first_extent: 300.0,
            })
            .unwrap();
        assert_eq!(widths(&layout), [300.0, 700.0], "resized");
        layout
            .apply(Command::ResetSplit {
                tab: TabId(1),
                path: Vec::new(),
            })
            .unwrap();
        assert_eq!(widths(&layout), [500.0, 500.0], "reset");
    }

    #[test]
    fn resize_and_reset_refuse_a_missing_tab_or_path() {
        let cases = [
            (
                "tab",
                Command::ResizeSplit {
                    tab: TabId(5),
                    path: Vec::new(),
                    bounds: BOUNDS,
                    first_extent: 1.0,
                },
                Error::TabNotFound(TabId(5)),
            ),
            (
                "path",
                Command::ResetSplit {
                    tab: TabId(1),
                    path: vec![crate::tree::branch::Branch::First],
                },
                Error::NoSuchSplit,
            ),
        ];
        for (name, command, error) in cases {
            let mut layout = layout(vec![add(1), open(1, "a"), open(1, "b")]);
            assert_eq!(layout.apply(command), Err(error), "{name}");
        }
    }
}
