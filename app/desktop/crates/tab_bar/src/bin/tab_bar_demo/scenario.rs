use model::{Command, Layout, PaneInstance, TabId};
use protocol::{PaneId, PaneState, PaneTypeId, Params};
use tab_bar::MemberView;

pub(crate) fn instance(name: &str) -> PaneInstance {
    PaneInstance {
        pane_id: PaneId::new(name).expect("static id"),
        type_id: PaneTypeId::new("core.chat").expect("static type"),
        params: Params::default().with("chat", name).expect("static params"),
        state: PaneState::default(),
    }
}

pub(crate) fn member(pane: &PaneId) -> MemberView {
    let title = match pane.as_str() {
        "p1" => "Untitled chat",
        "p2" => "Fix the sidebar drag",
        "p3" => "Release notes",
        "p4" => "Review the migration plan",
        other => other,
    };
    MemberView {
        pane: pane.clone(),
        label: title.to_owned().into(),
        provisional: pane.as_str() == "p1",
        working: pane.as_str() == "p3",
    }
}

pub(crate) fn build(name: &str) -> Layout {
    let count: u64 = match name {
        "one" => 1,
        "many" => 14,
        _ => 3,
    };
    let mut layout = Layout::default();
    for index in 1..=count {
        let title = format!("p{index}");
        let tab = TabId(index);
        let opened = layout
            .apply(Command::AddTab {
                id: tab,
                title: title.clone(),
            })
            .and_then(|()| {
                layout.apply(Command::OpenPane {
                    tab,
                    instance: instance(&title),
                    placement: None,
                })
            });
        if opened.is_err() {
            break;
        }
    }
    drop(layout.apply(Command::ActivateTab { tab: TabId(1) }));
    layout
}
