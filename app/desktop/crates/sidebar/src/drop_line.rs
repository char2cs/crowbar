//! The 2px line that marks a between-rows drop slot.

use gpui::{Div, Hsla, Styled, div, px};

use crate::drop_mode::DropMode;

const HEIGHT: f32 = 2.0;
const INSET: f32 = 7.0;
const EDGE: f32 = 1.0;
const OFFSET: f32 = -2.0;

pub(crate) fn drop_line(
    mode: DropMode,
    color: Hsla,
) -> Option<Div> {
    let base = div()
        .absolute()
        .left(px(INSET))
        .right(px(EDGE))
        .h(px(HEIGHT))
        .rounded_full()
        .bg(color);
    match mode {
        DropMode::Before => Some(base.top(px(OFFSET))),
        DropMode::After => Some(base.bottom(px(OFFSET))),
        DropMode::Into => None,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn nesting_draws_no_line() {
        let drawn = |mode| drop_line(mode, gpui::black()).is_some();
        assert!(drawn(DropMode::Before) && drawn(DropMode::After), "slots");
        assert!(!drawn(DropMode::Into), "into");
    }
}
