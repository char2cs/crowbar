use model::TabId;

use super::*;

fn chat() -> PaneTypeId {
    PaneTypeId::new("core.chat").unwrap()
}

fn thread(id: &str) -> Params {
    Params::default().with("threadId", id).unwrap()
}

fn apply(layout: &mut Layout, commands: Vec<Command>) {
    for command in commands {
        layout.apply(command).unwrap();
    }
}

#[test]
fn the_first_pane_opens_in_a_new_tab() {
    let mut layout = Layout::default();
    let serials = Serials::above(&layout);
    let commands = open_commands(&layout, (chat(), thread("a")), &serials).unwrap();
    apply(&mut layout, commands);
    assert_eq!(layout.tabs().len(), 1, "one tab");
    assert_eq!(layout.pane_count(), 1, "one pane");
}

#[test]
fn later_panes_open_beside_the_focused_one() {
    let mut layout = Layout::default();
    let serials = Serials::above(&layout);
    for name in ["a", "b"] {
        let commands = open_commands(&layout, (chat(), thread(name)), &serials).unwrap();
        apply(&mut layout, commands);
    }
    assert_eq!(
        (layout.tabs().len(), layout.pane_count()),
        (1, 2),
        "same tab, two panes"
    );
}

#[test]
fn an_open_pane_is_focused_instead_of_opened_twice() {
    let mut layout = Layout::default();
    let serials = Serials::above(&layout);
    for name in ["a", "b"] {
        let commands = open_commands(&layout, (chat(), thread(name)), &serials).unwrap();
        apply(&mut layout, commands);
    }
    let commands = open_commands(&layout, (chat(), thread("a")), &serials).unwrap();
    apply(&mut layout, commands);
    assert_eq!(layout.pane_count(), 2, "no duplicate");
    let focused = layout.active_tab().and_then(|tab| tab.focused().cloned());
    assert_eq!(
        focused.map(|id| id.to_string()),
        Some("p1".to_owned()),
        "a is focused"
    );
    assert_eq!(
        layout.active_tab().map(model::Tab::id),
        Some(TabId(1)),
        "its tab is active"
    );
}
