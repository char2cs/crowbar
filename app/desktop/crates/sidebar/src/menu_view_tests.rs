use super::*;
use crate::intent::Intent;
use crate::test_support::prelude::*;

use crate::menu_entry::MenuEntry;
use ui::IconName;

#[gpui::test]
fn an_open_menu_renders_over_the_sidebar_and_closes_on_escape(cx: &mut TestAppContext) {
    let (_, sidebar, cx) = open(cx, DemoData::reference());
    let entries = vec![MenuEntry::action(
        "Keep",
        IconName::Check,
        Intent::Keep(RowId::from("chat")),
    )];
    sidebar.update_in(cx, |this, window, cx| {
        this.open_menu((entries, point(px(50.0), px(200.0))), window, cx);
    });
    cx.run_until_parked();
    assert!(sidebar.read_with(cx, |this, _| this.menu.is_some()));
    sidebar.update_in(cx, |this, window, cx| this.menu_key("escape", window, cx));
    cx.run_until_parked();
    assert!(sidebar.read_with(cx, |this, _| this.menu.is_none()));
}
