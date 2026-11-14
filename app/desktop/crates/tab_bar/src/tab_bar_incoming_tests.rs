use gpui::TestAppContext;
use model::{Command, DragSubject, TabId};
use protocol::PaneId;

use crate::incoming_drop::IncomingDrop;
use crate::tab_bar_event::TabBarEvent;
use crate::tab_drop::TabDrop;
use crate::test_support::{open, view};

#[gpui::test]
fn dropping_a_pane_on_a_tab_asks_to_merge_it(cx: &mut TestAppContext) {
    let harness = open(cx, vec![view(1, &["a"]), view(2, &["b", "c"])], 1);
    let (bar, cx, events) = (harness.bar, harness.cx, harness.events);
    bar.update(cx, |bar, _| {
        bar.incoming = Some(IncomingDrop::new(TabDrop::Merge(TabId(1)), 0.0));
    });
    let subject = DragSubject::Pane(PaneId::new("c").expect("id"));
    bar.update_in(cx, |bar, window, cx| bar.dropped(&subject, window, cx));
    let seen = events.borrow();
    let expected = Command::MovePaneToTab {
        pane: PaneId::new("c").expect("id"),
        to: TabId(1),
        placement: None,
    };
    assert_eq!(
        seen.as_slice(),
        &[TabBarEvent::Apply(vec![expected])],
        "merge command"
    );
    bar.read_with(cx, |bar, _| {
        assert!(bar.incoming.is_none(), "marker consumed");
    });
}

#[gpui::test]
fn dropping_a_chat_between_tabs_creates_a_tab_with_a_fresh_id(cx: &mut TestAppContext) {
    let harness = open(cx, vec![view(1, &["a"])], 1);
    let (bar, cx, events) = (harness.bar, harness.cx, harness.events);
    bar.update(cx, |bar, _| {
        bar.incoming = Some(IncomingDrop::new(TabDrop::Reorder(0), 0.0));
    });
    let subject = DragSubject::Open(instance());
    bar.update_in(cx, |bar, window, cx| bar.dropped(&subject, window, cx));
    let seen = events.borrow();
    let TabBarEvent::Apply(commands) = &seen[0] else {
        panic!("an apply event");
    };
    assert!(
        matches!(commands[0], Command::AddTab { id: TabId(50), .. }),
        "first free id: {commands:?}"
    );
}

fn instance() -> model::PaneInstance {
    model::PaneInstance {
        pane_id: PaneId::new("new").expect("id"),
        type_id: protocol::PaneTypeId::new("core.chat").expect("type"),
        params: protocol::Params::default(),
        state: protocol::PaneState::default(),
    }
}
