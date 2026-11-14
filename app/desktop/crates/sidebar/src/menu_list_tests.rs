use gpui::{TestAppContext, point, px};
use ui::IconName;

use super::*;
use crate::demo_data::DemoData;
use crate::row_id::RowId;
use crate::test_support::open;

#[gpui::test]
fn hovering_an_entry_highlights_it_and_clicking_applies_it(cx: &mut TestAppContext) {
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
    sidebar.update(cx, |this, cx| this.highlight(Some(0), cx));
    let highlighted = sidebar.read_with(cx, |this, _| {
        this.menu
            .as_ref()
            .and_then(|menu| menu.cursor.highlighted())
    });
    assert_eq!(highlighted, Some(0));
}
