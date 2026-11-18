use layout::{Command, Edge, Layout, PaneInstance, Placement, TabId};
use protocol::{PaneId, PaneState, PaneTypeId, Params};

pub(crate) type DemoResult<T> = Result<T, Box<dyn std::error::Error>>;

const CHIP_NAMES: [&str; 3] = ["delta", "epsilon", "zeta"];
const CHAT_TYPE: &str = "core.chat";

fn instance(name: &str) -> DemoResult<PaneInstance> {
    Ok(PaneInstance {
        pane_id: PaneId::new(name)?,
        type_id: PaneTypeId::new(CHAT_TYPE)?,
        params: Params::default().with("chat", name)?,
        state: PaneState::default(),
    })
}

pub(crate) fn chips() -> DemoResult<Vec<PaneInstance>> {
    CHIP_NAMES.into_iter().map(instance).collect()
}

const SPLIT: &[(&str, &str, Edge)] = &[("beta", "alpha", Edge::Right)];
const THREE: &[(&str, &str, Edge)] = &[
    ("beta", "alpha", Edge::Right),
    ("gamma", "beta", Edge::Bottom),
];

fn open_beside(
    tab: TabId,
    (pane, beside, edge): &(&str, &str, Edge),
) -> DemoResult<Command> {
    let placement = Some(Placement {
        target: PaneId::new(*beside)?,
        edge: *edge,
    });
    Ok(Command::OpenPane {
        tab,
        instance: instance(pane)?,
        placement,
    })
}

pub(crate) fn build(name: &str) -> DemoResult<Layout> {
    let panes = match name {
        "single" => &[][..],
        "three" => THREE,
        _ => SPLIT,
    };
    let mut layout = Layout::default();
    let tab = TabId(1);
    let first = [
        Command::AddTab {
            id: tab,
            title: "Tab".into(),
        },
        Command::OpenPane {
            tab,
            instance: instance("alpha")?,
            placement: None,
        },
    ];
    let besides = panes
        .iter()
        .map(|split| open_beside(tab, split))
        .collect::<DemoResult<Vec<_>>>()?;
    first
        .into_iter()
        .chain(besides)
        .try_for_each(|command| layout.apply(command))?;
    Ok(layout)
}
