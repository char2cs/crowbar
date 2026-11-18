//! Vertical scroll offset of a text area.

use gpui::Pixels;

use crate::control::editor::text_area::wrapped_element::viewport::Viewport;

pub(crate) fn clamp(
    offset: Pixels,
    view: &Viewport,
) -> Pixels {
    let furthest = (view.content - view.height).max(Pixels::ZERO);
    offset.clamp(Pixels::ZERO, furthest)
}

pub(crate) fn reveal(
    offset: Pixels,
    view: &Viewport,
    caret: (Pixels, Pixels),
) -> Pixels {
    let (top, bottom) = caret;
    if top < offset {
        return clamp(top, view);
    }
    if bottom > offset + view.height {
        return clamp(bottom - view.height, view);
    }
    clamp(offset, view)
}

#[cfg(test)]
mod tests {
    use gpui::px;

    use super::*;

    const VIEW: Viewport = Viewport {
        height: px(60.0),
        content: px(200.0),
    };

    fn scrolling(content: f32) -> Viewport {
        Viewport {
            content: px(content),
            ..VIEW
        }
    }

    #[test]
    fn the_offset_stays_within_the_content() {
        const CASES: &[(f32, f32, f32)] = &[
            (-5.0, 200.0, 0.0),
            (50.0, 200.0, 50.0),
            (500.0, 200.0, 140.0),
            (30.0, 40.0, 0.0),
        ];
        for &(offset, content, want) in CASES {
            assert_eq!(
                clamp(px(offset), &scrolling(content)),
                px(want),
                "{offset} in {content}"
            );
        }
    }

    #[test]
    fn the_caret_is_scrolled_into_view_from_either_side() {
        type Case = (f32, (f32, f32), f32);
        const CASES: &[Case] = &[
            (50.0, (20.0, 40.0), 20.0),
            (50.0, (140.0, 160.0), 100.0),
            (50.0, (60.0, 80.0), 50.0),
            (0.0, (0.0, 20.0), 0.0),
        ];
        for &(offset, (top, bottom), want) in CASES {
            let got = reveal(px(offset), &VIEW, (px(top), px(bottom)));
            assert_eq!(got, px(want), "{offset} {top}..{bottom}");
        }
    }
}
