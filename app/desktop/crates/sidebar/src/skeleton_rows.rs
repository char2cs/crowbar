//! The bars shown before the projects scope has synced.

use gpui::{IntoElement, ParentElement, Styled, div, px};
use ui::Skeleton;

const WIDTHS: [f32; 5] = [0.75, 0.5, 0.667, 0.5, 0.6];

pub(crate) fn skeleton_rows(inner_width: f32) -> impl IntoElement {
    let bars = WIDTHS.iter().enumerate().map(|(index, fraction)| {
        Skeleton::new(
            ("sidebar-skeleton", index),
            px(inner_width * fraction),
            px(20.0),
        )
    });
    div()
        .flex()
        .flex_col()
        .gap(px(8.0))
        .p(px(12.0))
        .children(bars)
}

#[cfg(test)]
#[path = "skeleton_rows_tests.rs"]
mod tests;
