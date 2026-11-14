use model::Layout;

use super::*;

#[test]
fn the_first_commands_build_a_valid_one_pane_layout() {
    let mut layout = Layout::default();
    for command in first_commands().unwrap() {
        layout.apply(command).unwrap();
    }
    layout.validate().unwrap();
    assert_eq!(layout.active_tab().map(|tab| tab.pane_ids().len()), Some(1), "one pane");
}

#[test]
fn chat_is_registered_once() {
    let mut registry = PaneRegistry::default();
    register_core_panes(&mut registry, "chat/index.html").unwrap();
    assert!(register_core_panes(&mut registry, "chat/index.html").is_err(), "second registration is refused");
    assert!(registry.get(&PaneTypeId::new("core.chat").unwrap()).is_some(), "registered");
}
