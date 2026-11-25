//! How far a single-line field scrolls its text so the caret stays in view, as a browser input does.

use gpui::{Bounds, ContentMask, Pixels, ShapedLine, point, px, size};

use crate::control::editor::text_input::caret_scroll::{CARET_WIDTH, CaretScroll};

const CLIP_LINES: f32 = 3.0;

pub(crate) fn place<S: CaretScroll>(
    input: &S,
    (visible, line): (Bounds<Pixels>, &ShapedLine),
) -> (Bounds<Pixels>, Pixels) {
    let caret = line.x_for_index(input.caret());
    let scroll = follow(input.scrolled(), (caret, line.width), visible.size.width);
    let shifted = Bounds::new(
        point(visible.left() - scroll, visible.top()),
        size(visible.size.width + scroll, visible.size.height),
    );
    (shifted, scroll)
}

pub(crate) fn clip(visible: Bounds<Pixels>) -> ContentMask<Pixels> {
    let height = visible.size.height;
    ContentMask {
        bounds: Bounds::new(
            point(visible.left(), visible.top() - height),
            size(visible.size.width, height * CLIP_LINES),
        ),
    }
}

pub(crate) fn follow(
    previous: Pixels,
    (caret, line): (Pixels, Pixels),
    visible: Pixels,
) -> Pixels {
    let reach = (line + px(CARET_WIDTH) - visible).max(px(0.0));
    let kept = previous.min(reach);
    if caret + px(CARET_WIDTH) > kept + visible {
        return caret + px(CARET_WIDTH) - visible;
    }
    kept.min(caret)
}

#[cfg(test)]
mod tests {
    use super::*;

    const VISIBLE: f32 = 80.0;
    const LINE: f32 = 200.0;

    fn at(
        previous: f32,
        caret: f32,
    ) -> f32 {
        f32::from(follow(px(previous), (px(caret), px(LINE)), px(VISIBLE)))
    }

    #[test]
    fn the_text_moves_only_as_far_as_the_caret_needs() {
        assert_eq!(at(0.0, 40.0), 0.0, "a caret in view keeps the text still");
        assert_eq!(
            at(0.0, LINE),
            LINE + CARET_WIDTH - VISIBLE,
            "the end scrolls in"
        );
        assert_eq!(at(100.0, 150.0), 100.0, "a caret still in view leaves it");
        assert_eq!(
            at(100.0, 60.0),
            60.0,
            "a caret left of the view pulls it back"
        );
        assert_eq!(at(500.0, 0.0), 0.0, "never past the start");
    }
}
