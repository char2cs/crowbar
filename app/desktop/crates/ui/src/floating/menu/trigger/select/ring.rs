//! The select trigger's focus-visible look: develop's `focus-visible:border-ring ring-[3px]
//! ring-ring/24`, which also drops the resting shadow.

use gpui::{BoxShadow, Div, FocusHandle, Hsla, InteractiveElement, Stateful, Styled, point, px};

use crate::foundation::Pose;
use crate::foundation::tokens::opacity;

const WIDTH: f32 = 3.0;

pub(super) fn focusable(
    trigger: Stateful<Div>,
    (focus, rung): (&FocusHandle, Pose),
    ring: Hsla,
) -> Stateful<Div> {
    let trigger = trigger.track_focus(focus);
    if rung == Pose::Off {
        return trigger;
    }
    let shadow = BoxShadow {
        color: ring.opacity(opacity::PERCENT_24),
        offset: point(px(0.0), px(0.0)),
        blur_radius: px(0.0),
        spread_radius: px(WIDTH),
        inset: false,
    };
    trigger.border_color(ring).shadow(vec![shadow])
}

#[cfg(test)]
mod tests {
    use gpui::{SharedString, TestAppContext, VisualTestContext};

    use super::*;
    use crate::floating::menu::trigger::select::Select;
    use crate::foundation::{Surface, Theme};
    use crate::test_support::menus::answering;
    use crate::test_support::probe::{open_floating, settle};

    fn options() -> Vec<SharedString> {
        vec!["System".into(), "Light".into(), "Dark".into()]
    }

    fn rings_drawn(cx: &mut VisualTestContext) -> usize {
        settle(cx);
        cx.update(|window, cx| {
            let ring = Theme::of(cx).palette(Surface::default()).ring();
            let ring = ring.opacity(opacity::PERCENT_24);
            let shadows = &window.rendered_scene().shadows;
            shadows.iter().filter(|shadow| shadow.color == ring).count()
        })
    }

    #[gpui::test]
    fn a_key_after_the_menu_rings_the_trigger_as_develop_s_focus_visible_does(
        cx: &mut TestAppContext
    ) {
        answering(cx, None);
        let mut owner = open_floating(cx, |_, _| Select::new("s", options(), Some(0)));
        owner.simulate_click(point(px(20.0), px(10.0)), gpui::Modifiers::none());
        settle(&mut owner);
        assert_eq!(rings_drawn(&mut owner), 0, "opened from a pointer press");
        owner.simulate_keystrokes("escape");
        settle(&mut owner);
        assert_eq!(rings_drawn(&mut owner), 1, "the keyboard was used");
    }
}
