use crate::intent::Intent;
use crate::test_support::prelude::*;

#[gpui::test]
fn several_spaces_render_the_other_marks(cx: &mut TestAppContext) {
    let (_, sidebar, cx) = open(cx, DemoData::rich());
    cx.run_until_parked();
    assert!(sidebar.read_with(cx, |this, _| this.data.projects().len() > 1));
}

#[gpui::test]
fn the_add_button_starts_the_create_space_flow(cx: &mut TestAppContext) {
    let (_, sidebar, cx) = open(cx, DemoData::rich());
    sidebar.update_in(cx, |this, window, cx| {
        this.apply(Intent::StartCreateSpace, window, cx);
    });
    assert!(sidebar.read_with(cx, |this, _| this.creating_space));
}
