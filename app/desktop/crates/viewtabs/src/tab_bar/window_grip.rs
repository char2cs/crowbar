use gpui::{InteractiveElement, IntoElement, MouseButton, Styled, div};

use crate::pill::TabView;
use crate::strip_metrics::ROW_HEIGHT;

const NO_ROW: f32 = 0.0;

// PLATFORM: linux hands the move to the window manager through _NET_WM_MOVERESIZE; macos is unverified.
pub(crate) fn window_grip() -> impl IntoElement {
    div()
        .id("tab-strip-grip")
        .flex_1()
        .min_w_0()
        .h_full()
        .on_mouse_down(MouseButton::Left, |_, window, _| window.start_window_move())
}

pub(crate) fn row_height(tabs: &[TabView]) -> f32 {
    if tabs.is_empty() {
        return NO_ROW;
    }
    ROW_HEIGHT
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_strip_takes_no_room_until_a_view_exists() {
        assert_eq!(row_height(&[]), NO_ROW, "no view");
        let one = TabView {
            id: layout::TabId(1),
            members: Vec::new(),
        };
        assert_eq!(row_height(&[one]), ROW_HEIGHT, "one view");
    }
}
