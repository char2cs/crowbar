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

pub(crate) fn layout_of(first: &str, others: &[(&str, &str, model::Edge)]) -> Layout {
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
    for (name, beside, edge) in others {
        let placement = Some(Placement {
            target: pane_id(beside),
            edge: *edge,
        });
        layout
            .apply(Command::OpenPane {
                tab,
                instance: chat(name),
                placement,
            })
            .expect("open pane");
    }
    layout
}
