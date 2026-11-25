//! The select trigger's focus-visible look: develop's `focus-visible:border-ring ring-[3px]
//! ring-ring/24`, which also drops the resting shadow.

use gpui::{
    BoxShadow, Div, FocusHandle, Hsla, InteractiveElement, Stateful, StyleRefinement, Styled,
    point, px,
};

use crate::foundation::tokens::opacity;

const WIDTH: f32 = 3.0;

pub(super) fn focusable(
    trigger: Stateful<Div>,
    focus: &FocusHandle,
    ring: Hsla,
) -> Stateful<Div> {
    trigger.track_focus(focus).focus_visible(ringed(ring))
}

fn ringed(ring: Hsla) -> impl FnOnce(StyleRefinement) -> StyleRefinement {
    let shadow = BoxShadow {
        color: ring.opacity(opacity::PERCENT_24),
        offset: point(px(0.0), px(0.0)),
        blur_radius: px(0.0),
        spread_radius: px(WIDTH),
        inset: false,
    };
    move |style| style.border_color(ring).shadow(vec![shadow])
}

#[cfg(test)]
mod tests {
    use gpui::{SharedString, TestAppContext, VisualTestContext};

    use super::*;
    use crate::floating::menu::trigger::select::Select;
    use crate::foundation::{Surface, Theme};
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
    fn escape_on_the_open_list_rings_the_trigger_as_develop_s_focus_visible_does(
        cx: &mut TestAppContext
    ) {
        let (mut owner, mut top) = open_floating(cx, |_, _| Select::new("s", options(), Some(0)));
        owner.simulate_click(point(px(20.0), px(10.0)), gpui::Modifiers::none());
        settle(&mut top);
        assert_eq!(rings_drawn(&mut owner), 0, "opened from a pointer press");
        top.simulate_keystrokes("escape");
        settle(&mut top);
        assert_eq!(rings_drawn(&mut owner), 1, "the list closed on a key");
    }
}
