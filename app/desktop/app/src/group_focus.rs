use layout::{Command, EditorView, Layout, PaneGroup, Rect, Tab};
use protocol::PaneId;

const ANY_BOUNDS: Rect = Rect {
    x: 0.0,
    y: 0.0,
    width: 1000.0,
    height: 1000.0,
};

pub(crate) fn focused_group(tab: &Tab) -> Option<&PaneGroup> {
    tab.group_of(tab.focused()?)
}

pub(crate) fn editor_focused(layout: &Layout) -> bool {
    let Some(tab) = layout.showing("") else {
        return false;
    };
    let focused = tab.focused().and_then(|pane| tab.pane(pane));
    focused.is_some_and(|pane| !pane.type_id.is_chat())
}

pub(crate) fn close_target(tab: &Tab) -> Option<PaneId> {
    let group = focused_group(tab)?;
    let editor = group.active_tab().filter(|_| group.editor_open());
    if let Some(editor) = editor {
        return Some(editor.clone());
    }
    let chats = chats_of(tab).len();
    group.chat().filter(|_| chats > 1).cloned()
}

pub(crate) fn editor_toggle(tab: &Tab) -> Option<Command> {
    let group = focused_group(tab)?;
    let pane = tab.focused()?.clone();
    let view = EditorView::of(group).flipped();
    Some(Command::SetEditorView { pane, view })
}

pub(crate) fn listed(tab: &Tab) -> Vec<PaneId> {
    let chats = chats_of(tab);
    if !chats.is_empty() {
        return chats;
    }
    tab.leaves(ANY_BOUNDS)
        .first()
        .and_then(|(_, group)| group.shown().cloned())
        .into_iter()
        .collect()
}

fn chats_of(tab: &Tab) -> Vec<PaneId> {
    tab.leaves(ANY_BOUNDS)
        .into_iter()
        .filter_map(|(_, group)| group.chat().cloned())
        .collect()
}

#[cfg(test)]
mod tests {
    use layout::{PaneInstance, TabId};
    use protocol::{PaneState, PaneTypeId, Params};

    use super::*;

    fn instance(
        name: &str,
        kind: &str,
    ) -> PaneInstance {
        PaneInstance {
            pane_id: PaneId::new(name).unwrap(),
            type_id: PaneTypeId::new(kind).unwrap(),
            params: Params::default().with("id", name).unwrap(),
            state: PaneState::default(),
        }
    }

    type Opened = &'static [(&'static str, &'static str)];

    fn layout_of(panes: &[(&str, &str)]) -> Layout {
        let mut layout = Layout::default();
        layout
            .apply(Command::AddTab {
                id: TabId(1),
                title: String::new(),
                space: layout::SpaceId::default(),
            })
            .unwrap();
        for (name, kind) in panes {
            layout.apply(open(name, kind)).unwrap();
        }
        layout
    }

    fn open(
        name: &str,
        kind: &str,
    ) -> Command {
        Command::OpenPane {
            tab: TabId(1),
            instance: instance(name, kind),
            placement: None,
        }
    }

    struct Listing {
        name: &'static str,
        opened: Opened,
        listed: &'static [&'static str],
    }

    const LISTINGS: &[Listing] = &[
        Listing {
            name: "chats are the view tabs, editor tabs are not",
            opened: &[
                ("a", "core.chat"),
                ("b", "core.chat"),
                ("t1", "core.terminal"),
                ("t2", "core.terminal"),
            ],
            listed: &["a", "b"],
        },
        Listing {
            name: "a tab without a chat lists the shown editor tab",
            opened: &[("t1", "core.terminal"), ("t2", "core.editor")],
            listed: &["t2"],
        },
    ];

    #[test]
    fn the_top_bar_lists_chats_and_only_falls_back_to_an_editor_tab_when_there_is_none() {
        for case in LISTINGS {
            let layout = layout_of(case.opened);
            let tab = layout.showing("").unwrap();
            let names: Vec<String> = listed(tab).iter().map(ToString::to_string).collect();
            assert_eq!(names, case.listed, "{}", case.name);
        }
    }

    #[test]
    fn the_editor_area_holds_focus_only_when_an_editor_tab_is_focused() {
        const CASES: &[(Opened, bool)] = &[
            (&[], false),
            (&[("c1", "core.chat")], false),
            (&[("c1", "core.chat"), ("e1", "core.editor")], true),
            (&[("c1", "core.chat"), ("t1", "core.terminal")], true),
        ];
        assert!(!editor_focused(&Layout::default()), "no view");
        for (opened, expected) in CASES {
            let layout = layout_of(opened);
            assert_eq!(editor_focused(&layout), *expected, "{opened:?}");
        }
    }

    #[test]
    fn close_takes_the_active_editor_tab_then_a_split_chat_and_never_a_lone_chat() {
        const CASES: &[(Opened, Option<&str>)] = &[
            (&[("c1", "core.chat")], None),
            (&[("c1", "core.chat"), ("c2", "core.chat")], Some("c2")),
            (&[("c1", "core.chat"), ("e1", "core.editor")], Some("e1")),
            (
                &[
                    ("c1", "core.chat"),
                    ("e1", "core.editor"),
                    ("e2", "core.editor"),
                ],
                Some("e2"),
            ),
        ];
        for (opened, expected) in CASES {
            let layout = layout_of(opened);
            let target = close_target(layout.showing("").unwrap());
            assert_eq!(target.as_ref().map(PaneId::as_str), *expected, "{opened:?}");
        }
    }

    #[test]
    fn the_editor_toggle_flips_the_focused_group_and_needs_one() {
        const CASES: &[(Opened, Option<EditorView>)] = &[
            (&[], None),
            (&[("c1", "core.chat")], Some(EditorView::Open)),
            (
                &[("c1", "core.chat"), ("e1", "core.editor")],
                Some(EditorView::Closed),
            ),
        ];
        for (opened, expected) in CASES {
            let layout = layout_of(opened);
            let toggle = layout.showing("").and_then(editor_toggle);
            assert_eq!(toggle.and_then(|c| view_of(&c)), *expected, "{opened:?}");
        }
    }

    fn view_of(command: &Command) -> Option<EditorView> {
        let Command::SetEditorView { view, .. } = command else {
            return None;
        };
        Some(*view)
    }
}
