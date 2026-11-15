use model::{Command, Edge, Layout, PaneInstance, Placement, TabId};
use protocol::{PaneId, PaneState, PaneTypeId, Params};

pub(crate) fn instance(name: &str) -> PaneInstance {
    PaneInstance {
        pane_id: PaneId::new(name).expect("static id"),
        type_id: PaneTypeId::new("core.chat").expect("static type"),
        params: Params::default().with("chat", name).expect("static params"),
        state: PaneState::default(),
    }
}

const SPLIT: &[(&str, &str, Edge)] = &[("beta", "alpha", Edge::Right)];
const THREE: &[(&str, &str, Edge)] = &[
    ("beta", "alpha", Edge::Right),
    ("gamma", "beta", Edge::Bottom),
];

fn open_beside(
    tab: TabId,
    (pane, beside, edge): &(&str, &str, Edge),
) -> Command {
    let placement = Some(Placement {
        target: PaneId::new(*beside).expect("static id"),
        edge: *edge,
    });
    Command::OpenPane {
        tab,
        instance: instance(pane),
        placement,
    }
}

pub(crate) fn build(name: &str) -> Layout {
    let panes = match name {
        "single" => &[][..],
        "three" => THREE,
        _ => SPLIT,
    };
    let mut layout = Layout::default();
    let tab = TabId(1);
    let mut commands = vec![
        Command::AddTab {
            id: tab,
            title: "Tab".into(),
        },
        Command::OpenPane {
            tab,
            instance: instance("alpha"),
            placement: None,
        },
    ];
    commands.extend(panes.iter().map(|split| open_beside(tab, split)));
    drop(
        commands
            .into_iter()
            .try_for_each(|command| layout.apply(command)),
    );
    layout
}
