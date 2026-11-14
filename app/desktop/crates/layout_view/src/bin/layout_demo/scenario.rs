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
    for (pane, beside, edge) in panes {
        let placement = Some(Placement {
            target: PaneId::new(*beside).expect("static id"),
            edge: *edge,
        });
        commands.push(Command::OpenPane {
            tab,
            instance: instance(pane),
            placement,
        });
    }
    for command in commands {
        if layout.apply(command).is_err() {
            break;
        }
    }
    layout
}
