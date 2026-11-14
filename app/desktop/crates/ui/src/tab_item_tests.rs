use super::*;

#[test]
fn text_tabs_have_a_label_and_no_icon() {
    let tab = TabItem::text("Chat");
    assert_eq!(tab.label.as_ref().map(AsRef::as_ref), Some("Chat"));
    assert!(tab.icon.is_none());
    assert_eq!(tab.selected_weight, IconWeight::Regular);
}

#[test]
fn icon_tabs_fill_when_selected() {
    let tab = TabItem::icon(IconName::Check);
    assert!(tab.label.is_none());
    assert_eq!(tab.selected_weight, IconWeight::Fill);
}

#[test]
fn builders_add_an_icon_and_disable() {
    let tab = TabItem::text("Chat").with_icon(IconName::Check).disabled();
    assert_eq!(tab.icon, Some(IconName::Check));
    assert!(tab.disabled);
}
