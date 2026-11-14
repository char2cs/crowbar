use model::{Command, Layout, PaneInstance, TabId};
use protocol::{PaneId, PaneState, PaneTypeId, Params};

use super::*;

fn instance(name: &str) -> PaneInstance {
    PaneInstance {
        pane_id: PaneId::new(name).expect("id"),
        type_id: PaneTypeId::new("core.chat").expect("type"),
        params: Params::default().with("chat", name).expect("params"),
        state: PaneState::default(),
    }
}

fn member(pane: &PaneId) -> MemberView {
    MemberView {
        pane: pane.clone(),
        label: pane.as_str().to_uppercase().into(),
        provisional: false,
        working: false,
    }
}

#[test]
fn a_tab_view_lists_its_panes_in_layout_order_and_joins_their_titles() {
    let mut layout = Layout::default();
    layout
        .apply(Command::AddTab {
            id: TabId(7),
            title: "T".into(),
        })
        .expect("tab");
    for name in ["a", "b"] {
        layout
            .apply(Command::OpenPane {
                tab: TabId(7),
                instance: instance(name),
                placement: None,
            })
            .expect("pane");
    }
    let view = TabView::from_tab(layout.active_tab().expect("tab"), member);
    assert_eq!(view.id, TabId(7), "same id as the model tab");
    assert_eq!(view.members.len(), 2, "two members");
    assert_eq!(view.title(), "A | B", "titles joined");
}
