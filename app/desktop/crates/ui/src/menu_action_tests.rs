use super::*;

fn action() -> MenuAction {
    MenuAction::new("Rename", |_, _| {})
}

#[test]
fn a_new_action_is_plain() {
    let action = action();
    assert_eq!(action.label.as_ref(), "Rename");
    assert!(!action.checked && !action.disabled);
    assert_eq!(action.tone, MenuTone::Normal);
    assert!(action.icon.is_none() && action.shortcut.is_none());
}

#[test]
fn builders_set_their_field() {
    let action = action()
        .icon(IconName::Check)
        .shortcut("F2")
        .checked()
        .disabled()
        .destructive();
    assert_eq!(action.icon, Some(IconName::Check));
    assert_eq!(action.shortcut.as_ref().map(AsRef::as_ref), Some("F2"));
    assert!(action.checked && action.disabled);
    assert_eq!(action.tone, MenuTone::Destructive);
}
