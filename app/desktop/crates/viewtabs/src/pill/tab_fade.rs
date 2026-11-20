use gpui::{Hsla, InteractiveElement, IntoElement, SharedString, Styled, div, px};
use ui::{Palette, radius};

use crate::pill::shell_look::ShellLook;

const FADE_REACH: f32 = 34.0;
const FADE_SPAN: f32 = 10.0;
const FADE_ANGLE: f32 = 90.0;

pub(crate) fn fade_ends(
    look: ShellLook,
    palette: &Palette,
) -> (Hsla, Hsla) {
    let ground = palette.chrome_bg();
    match look {
        ShellLook::Idle => (ground, ground.blend(palette.sidebar_element_hover())),
        ShellLook::Active | ShellLook::Lifted => {
            (palette.control_surface(), palette.control_surface())
        },
        ShellLook::MergeTarget => {
            let nest = ground.blend(palette.sidebar_drop_nest());
            (nest, nest)
        },
    }
}

pub(crate) fn fade(
    group: &SharedString,
    (rest, hovered): (Hsla, Hsla),
) -> impl IntoElement {
    div()
        .absolute()
        .top_0()
        .right_0()
        .h_full()
        .w(px(FADE_REACH))
        .rounded_r(radius::FULL)
        .bg(fading_into(rest))
        .group_hover(group.clone(), move |style| style.bg(fading_into(hovered)))
}

fn fading_into(fill: Hsla) -> gpui::Background {
    gpui::linear_gradient(
        FADE_ANGLE,
        gpui::linear_color_stop(fill.opacity(0.0), 0.0),
        gpui::linear_color_stop(fill, FADE_SPAN / FADE_REACH),
    )
}

#[cfg(test)]
mod tests {
    use ui::{Surface, Theme};

    use super::*;

    #[test]
    fn the_fade_under_the_close_button_ends_in_the_fill_the_tab_shows_in_each_state() {
        let theme = Theme::default();
        let palette = theme.palette(Surface::Chrome);
        let ground = palette.chrome_bg();
        let hovered = ground.blend(palette.sidebar_element_hover());
        let raised = palette.control_surface();
        let nest = ground.blend(palette.sidebar_drop_nest());
        let cases = [
            (ShellLook::Idle, (ground, hovered)),
            (ShellLook::Active, (raised, raised)),
            (ShellLook::Lifted, (raised, raised)),
            (ShellLook::MergeTarget, (nest, nest)),
        ];
        for (look, expected) in cases {
            assert_eq!(fade_ends(look, palette), expected, "{look:?}");
        }
    }

    #[test]
    fn an_idle_tab_s_fade_rests_in_the_strip_ground_so_it_shows_no_shape() {
        let palette = Theme::default().palette(Surface::Chrome);
        let (rest, _) = fade_ends(ShellLook::Idle, palette);
        assert_eq!(
            rest,
            Hsla::from(gpui::rgba(0x1212_10ff)),
            "the dark strip ground"
        );
    }
}
