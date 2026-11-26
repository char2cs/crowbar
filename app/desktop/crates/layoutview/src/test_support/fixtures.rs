use layout::{Command, Layout, PaneInstance, Placement, TabId};
use protocol::{PaneId, PaneState, PaneTypeId, Params};

pub(crate) fn pane_id(name: &str) -> PaneId {
    PaneId::new(name).expect("test pane ids are valid")
}

pub(crate) fn chat(name: &str) -> PaneInstance {
    PaneInstance {
        pane_id: pane_id(name),
        type_id: PaneTypeId::new("core.chat").expect("valid type"),
        params: Params::default().with("chat", name).expect("valid params"),
        state: PaneState::default(),
    }
}

pub(crate) fn editor(name: &str) -> PaneInstance {
    PaneInstance {
        type_id: PaneTypeId::new("core.terminal").expect("valid type"),
        ..chat(name)
    }
}

pub(crate) fn layout_of(
    first: &str,
    others: &[(&str, &str, layout::Edge)],
) -> Layout {
    let mut layout = Layout::default();
    let tab = TabId(1);
    layout
        .apply(Command::AddTab {
            id: tab,
            title: "Tab".into(),
            space: layout::SpaceId::default(),
        })
        .expect("add tab");
    layout
        .apply(Command::OpenPane {
            tab,
            instance: chat(first),
            placement: None,
        })
        .expect("open first");
    for split in others {
        open_beside(&mut layout, tab, split);
    }
    layout
}

fn open_beside(
    layout: &mut Layout,
    tab: TabId,
    split: &(&str, &str, layout::Edge),
) {
    let (name, beside, edge) = split;
    let placement = Some(Placement {
        target: pane_id(beside),
        edge: *edge,
    });
    let command = Command::OpenPane {
        tab,
        instance: chat(name),
        placement,
    };
    layout.apply(command).expect("open pane");
}

/// Opens `name` as an editor tab of the focused group of tab 1.
pub(crate) fn open_tab(
    layout: &mut Layout,
    name: &str,
) {
    let open = Command::OpenPane {
        tab: TabId(1),
        instance: editor(name),
        placement: None,
    };
    layout.apply(open).expect("open a tab");
}

/// Tab 1 with chat `a` beside a group of the editor tabs `b` and `c`.
pub(crate) fn stacked_layout() -> Layout {
    let mut layout = layout_of("a", &[]);
    let placement = Placement {
        target: pane_id("a"),
        edge: layout::Edge::Right,
    };
    let beside = Command::OpenPane {
        tab: TabId(1),
        instance: editor("b"),
        placement: Some(placement),
    };
    layout.apply(beside).expect("b beside a");
    layout
        .apply(Command::OpenPaneInGroup {
            instance: editor("c"),
            onto: pane_id("b"),
            index: None,
        })
        .expect("stack c with b");
    layout
}

fn stack_onto_a(
    layout: &mut Layout,
    name: &str,
) {
    let open = Command::OpenPaneInGroup {
        instance: editor(name),
        onto: pane_id("a"),
        index: None,
    };
    layout.apply(open).expect("stack");
}

/// Tab 1 holding one group of the editor tabs `a`, `b` and `c`, with `shown` shown and focused.
pub(crate) fn strip_layout(shown: &str) -> Layout {
    let mut layout = Layout::default();
    layout
        .apply(Command::AddTab {
            id: TabId(1),
            title: "Tab".into(),
            space: layout::SpaceId::default(),
        })
        .expect("add tab");
    let first = Command::OpenPane {
        tab: TabId(1),
        instance: editor("a"),
        placement: None,
    };
    layout.apply(first).expect("open a");
    for name in ["b", "c"] {
        stack_onto_a(&mut layout, name);
    }
    let focus = Command::FocusPane {
        pane: pane_id(shown),
    };
    layout.apply(focus).expect("focus");
    layout
}
