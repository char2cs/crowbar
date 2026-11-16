use gpui::{Div, Hsla, ParentElement, Styled, div, linear_color_stop, linear_gradient};

use crate::look::{BASE, BRAND_GROUND, GRADIENT_ANGLE, ORANGE, ORANGE_STRENGTH, TEAL, color};
use crate::oobe_screen::OobeScreen;

fn clear(hue: Hsla) -> Hsla {
    Hsla { a: 0.0, ..hue }
}

fn layer(
    from: Hsla,
    to: Hsla,
) -> Div {
    div().absolute().size_full().bg(linear_gradient(
        GRADIENT_ANGLE,
        linear_color_stop(from, 0.0),
        linear_color_stop(to, 1.0),
    ))
}

pub(crate) fn backdrop(screen: OobeScreen) -> Div {
    if screen == OobeScreen::AddHost {
        return div().size_full().bg(color(BRAND_GROUND));
    }
    let teal = color(TEAL);
    let orange = Hsla {
        a: ORANGE_STRENGTH,
        ..color(ORANGE)
    };
    div()
        .size_full()
        .bg(color(BASE))
        .child(layer(teal, clear(teal)))
        .child(layer(clear(orange), orange))
}

#[cfg(test)]
mod tests {
    use gpui::Fill;

    use super::*;

    #[test]
    fn the_host_screen_is_the_brand_ground_and_the_others_start_from_the_base_tone() {
        let cases = [
            (OobeScreen::AddHost, BRAND_GROUND),
            (OobeScreen::Welcome, BASE),
            (OobeScreen::SystemCheck, BASE),
            (OobeScreen::AddSpace, BASE),
        ];
        for (screen, ground) in cases {
            let mut shown = backdrop(screen);
            let expected: Fill = color(ground).into();
            assert_eq!(shown.style().background, Some(expected), "{screen:?}");
        }
    }
}
