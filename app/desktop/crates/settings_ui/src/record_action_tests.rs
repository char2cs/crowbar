use super::*;

#[test]
fn builders_add_label_and_confirmation() {
    let action = RecordAction::new("kill", "skull", "Kill the agent")
        .labelled("Kill")
        .confirm(Confirm::new("Kill?", "Ends it.", "Kill"));
    assert_eq!(action.label.as_deref(), Some("Kill"));
    assert!(action.confirm.is_some());
    assert_eq!(
        (action.id.as_str(), action.icon.as_str()),
        ("kill", "skull")
    );
}
