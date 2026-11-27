//! Moving the window from the areas that stand in for its title bar.

use gpui::{
    App, Div, InteractiveElement as _, MouseButton, MouseDownEvent, Styled as _, Window, div, px,
};

const fn zooms(clicks: usize) -> bool {
    clicks >= 2
}

/// A press on a title bar area: the first moves the window, a second zooms it.
// PLATFORM: macos the app owns its title bar, so AppKit moves and zooms nothing by itself.
pub fn drag_window(
    event: &MouseDownEvent,
    window: &mut Window,
    _: &mut App,
) {
    if zooms(event.click_count) {
        window.zoom_window();
        return;
    }
    window.start_window_move();
}

/// A strip of `height` across the top of its parent that moves the window.
pub fn window_drag_strip(height: f32) -> Div {
    div()
        .debug_selector(|| "window-grip".into())
        .absolute()
        .top_0()
        .left_0()
        .right_0()
        .h(px(height))
        .on_mouse_down(MouseButton::Left, drag_window)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn one_press_moves_the_window_and_a_second_zooms_it() {
        assert!(!zooms(1));
        assert!(zooms(2) && zooms(3));
    }
}
