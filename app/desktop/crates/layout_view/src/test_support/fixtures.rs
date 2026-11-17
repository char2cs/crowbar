use model::{Command, Layout, PaneInstance, Placement, TabId};
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

pub(crate) fn layout_of(
    first: &str,
    others: &[(&str, &str, model::Edge)],
) -> Layout {
    let mut layout = Layout::default();
    let tab = TabId(1);
    layout
        .apply(Command::AddTab {
            id: tab,
            title: "Tab".into(),
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
    split: &(&str, &str, model::Edge),
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

/// Tab 1 with chat `a` beside a group of `b` and `c`.
pub(crate) fn stacked_layout() -> Layout {
    let mut layout = layout_of("a", &[("b", "a", model::Edge::Right)]);
    layout
        .apply(Command::OpenPaneInGroup {
            instance: chat("c"),
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
        instance: chat(name),
        onto: pane_id("a"),
        index: None,
    };
    layout.apply(open).expect("stack");
}

/// Tab 1 holding one group that stacks `a`, `b` and `c`, with `shown` shown and focused.
pub(crate) fn strip_layout(shown: &str) -> Layout {
    let mut layout = layout_of("a", &[]);
    for name in ["b", "c"] {
        stack_onto_a(&mut layout, name);
    }
    let focus = Command::FocusPane {
        pane: pane_id(shown),
    };
    layout.apply(focus).expect("focus");
    layout
}
