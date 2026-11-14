use gpui::{Modifiers, MouseButton, TestAppContext, point, px};

use crate::test_support::{open, view};

#[gpui::test]
fn right_clicking_a_tab_opens_its_menu(cx: &mut TestAppContext) {
    let harness = open(cx, vec![view(1, &["a"]), view(2, &["b"])], 1);
    let (bar, cx) = (harness.bar, harness.cx);
    cx.simulate_mouse_down(
        point(px(100.0), px(21.0)),
        MouseButton::Right,
        Modifiers::none(),
    );
    bar.read_with(cx, |bar, _| assert!(bar.menu.is_some(), "a context menu"));
}
