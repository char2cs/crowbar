use super::*;

#[test]
fn member_labels_fall_back_to_new_chat_until_a_title_arrives() {
    let titles = crate::titles::Titles::default();
    let pane = protocol::PaneId::new("p1").unwrap();
    let before = member(&titles, &pane);
    assert_eq!(
        (before.label.as_ref(), before.provisional),
        (NEW_CHAT, true),
        "unnamed"
    );
    titles.set(pane.clone(), protocol::PaneTitle::new("Fix").unwrap());
    let after = member(&titles, &pane);
    assert_eq!(
        (after.label.as_ref(), after.provisional),
        ("Fix", false),
        "named"
    );
}
