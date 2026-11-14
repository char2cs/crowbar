use super::*;

#[test]
fn only_enabled_actions_are_selectable() {
    let enabled = MenuItem::Action(MenuAction::new("Open", |_, _| {}));
    let disabled = MenuItem::Action(MenuAction::new("Open", |_, _| {}).disabled(true));
    assert!(enabled.is_selectable());
    assert!(!disabled.is_selectable());
    assert!(!MenuItem::Separator.is_selectable());
    assert!(!MenuItem::Label("Group".into()).is_selectable());
}
