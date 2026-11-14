use gpui::{TestAppContext, point, px};

use super::*;
use crate::button::Button;
use crate::icon_name::IconName;
use crate::test_support::{measure, open};

#[gpui::test]
fn wrapping_does_not_change_the_trigger_size(cx: &mut TestAppContext) {
    let bare = measure(cx, |_, _| Button::icon_only("b", IconName::Gear));
    let wrapped = measure(cx, |_, _| Tooltip::new("t", "Settings", Button::icon_only("b", IconName::Gear)));
    assert_eq!(bare.size, wrapped.size);
}

#[gpui::test]
fn card_is_absent_until_the_pointer_rests(cx: &mut TestAppContext) {
    let cx = open(cx, |_, _| Tooltip::new("t", "Settings", Button::icon_only("b", IconName::Gear)).side(TooltipSide::Top));
    cx.simulate_mouse_move(point(px(10.0), px(10.0)), None, gpui::Modifiers::none());
    let before = cx.debug_bounds("probe").map(|b| b.size);
    assert_eq!(before.map(|s| f32::from(s.height)), Some(32.0));
}
