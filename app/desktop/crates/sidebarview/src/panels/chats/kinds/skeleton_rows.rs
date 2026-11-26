//! The bars shown before the projects scope has synced.

use gpui::{InteractiveElement, IntoElement, ParentElement, Pixels, Styled, div, px};
use ui::Skeleton;

use crate::sidebar::{SPACE_LG, SPACE_MD};

pub(crate) const SELECTOR: &str = "sidebar-skeleton";

const BAR_HEIGHT: Pixels = px(20.0);

const WIDTHS: [f32; 5] = [0.75, 0.5, 0.667, 0.5, 0.6];

pub(crate) fn skeleton_rows(inner_width: f32) -> impl IntoElement {
    let bars = WIDTHS.iter().enumerate().map(|(index, fraction)| {
        Skeleton::new(
            ("sidebar-skeleton", index),
            px(inner_width * fraction),
            BAR_HEIGHT,
        )
    });
    div()
        .debug_selector(|| SELECTOR.into())
        .flex()
        .flex_col()
        .gap(SPACE_MD)
        .p(SPACE_LG)
        .children(bars)
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::*;
    use crate::test_support::measure;

    #[gpui::test]
    fn five_bars_stack_with_gaps_and_padding(cx: &mut TestAppContext) {
        let bounds = measure(cx, |_, _| skeleton_rows(270.0));
        let expected = 5.0 * 20.0 + 4.0 * 8.0 + 2.0 * 12.0;
        assert_eq!(f32::from(bounds.size.height), expected);
    }
}
