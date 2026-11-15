use model::{Command, PaneInstance, TabId};
use protocol::{PaneId, PaneState, PaneTypeId, Params};

use super::*;

#[test]
fn an_empty_layout_starts_at_one() {
    let serials = Serials::above(&Layout::default());
    assert_eq!(serials.next_tab(), TabId(1), "tab");
    assert_eq!(serials.next_pane(), "p1", "pane");
}

#[test]
fn counters_start_above_the_ids_in_use() {
    let mut layout = Layout::default();
    layout
        .apply(Command::AddTab {
            id: TabId(7),
            title: "t".into(),
        })
        .unwrap();
    layout
        .apply(Command::OpenPane {
            tab: TabId(7),
            instance: PaneInstance {
                pane_id: PaneId::new("p12").unwrap(),
                type_id: PaneTypeId::new("core.chat").unwrap(),
                params: Params::default(),
                state: PaneState::default(),
            },
            placement: None,
        })
        .unwrap();
    let serials = Serials::above(&layout);
    assert_eq!(serials.next_tab(), TabId(8), "tab");
    assert_eq!(serials.next_pane(), "p13", "pane");
    assert_eq!(serials.next_pane(), "p14", "advances");
}
