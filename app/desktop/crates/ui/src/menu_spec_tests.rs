use gpui::{TestAppContext, px};

use super::*;
use crate::test_support::open;

fn spec(min_width: Option<Pixels>) -> MenuSpec {
    MenuSpec {
        items: Rc::from(Vec::<MenuItem>::new()),
        min_width,
    }
}

#[gpui::test]
fn menu_is_at_least_the_requested_width(cx: &mut TestAppContext) {
    let wide = spec(Some(px(300.0)));
    let cx = open(cx, move |window, cx| wide.build(window, cx));
    let menu = cx.debug_bounds("ui-menu").unwrap_or_default();
    assert!(f32::from(menu.size.width) >= 300.0, "{menu:?}");
}

#[gpui::test]
fn default_minimum_applies_without_a_request(cx: &mut TestAppContext) {
    let plain = spec(None);
    let cx = open(cx, move |window, cx| plain.build(window, cx));
    let menu = cx.debug_bounds("ui-menu").unwrap_or_default();
    assert!(f32::from(menu.size.width) >= 128.0, "{menu:?}");
}
