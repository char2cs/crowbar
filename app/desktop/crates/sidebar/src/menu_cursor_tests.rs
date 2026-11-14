use ui::IconName;

use super::*;
use crate::row_id::RowId;

fn entries() -> Vec<MenuEntry> {
    let rename = Intent::StartRename(RowId::from("r"));
    vec![
        MenuEntry::action("Rename", IconName::PencilSimpleLine, rename),
        MenuEntry::Separator,
        MenuEntry::Submenu {
            label: "Move to…".into(),
            icon: None,
            entries: Vec::new(),
        },
        MenuEntry::destructive("Delete", IconName::Trash, Intent::Keep(RowId::from("r"))),
    ]
}

#[test]
fn next_skips_separators_and_stops_at_the_end() {
    let list = entries();
    let mut cursor = MenuCursor::default();
    cursor.step(MenuStep::Next, &list);
    assert_eq!(cursor.highlighted(), Some(0), "first selectable");
    cursor.step(MenuStep::Next, &list);
    assert_eq!(cursor.highlighted(), Some(2), "separator skipped");
    cursor.step(MenuStep::Next, &list);
    cursor.step(MenuStep::Next, &list);
    assert_eq!(cursor.highlighted(), Some(3), "stays on the last entry");
}

#[test]
fn previous_from_nothing_lands_on_the_last_entry() {
    let list = entries();
    let mut cursor = MenuCursor::default();
    cursor.step(MenuStep::Previous, &list);
    assert_eq!(cursor.highlighted(), Some(3));
    cursor.step(MenuStep::Previous, &list);
    assert_eq!(cursor.highlighted(), Some(2));
}

#[test]
fn only_a_highlighted_submenu_entry_opens_a_submenu() {
    let list = entries();
    let mut cursor = MenuCursor::default();
    cursor.highlight(Some(0));
    assert_eq!(cursor.open_submenu(&list), None, "an action opens nothing");
    cursor.highlight(Some(2));
    assert_eq!(cursor.open_submenu(&list), Some(2));
}

#[test]
fn choosing_returns_the_intent_of_an_action_only() {
    let list = entries();
    let mut cursor = MenuCursor::default();
    cursor.highlight(Some(0));
    assert_eq!(
        cursor.chosen(&list),
        Some(Intent::StartRename(RowId::from("r")))
    );
    cursor.highlight(Some(2));
    assert_eq!(cursor.chosen(&list), None, "submenus are not chosen");
    cursor.highlight(None);
    assert_eq!(cursor.chosen(&list), None, "nothing highlighted");
}
