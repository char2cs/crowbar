//! Placement of popups below their trigger and the click-away layer behind them.

use gpui::{
    AnyElement, App, InteractiveElement, IntoElement, MouseButton, ParentElement, Styled, Window, anchored, deferred,
    div, point, px,
};

use crate::layer;

const WINDOW_MARGIN: f32 = 8.0;

/// Positions `content` below the element it is a child of, `offset` pixels away, kept inside the window.
///
/// The parent must be `relative`; the content paints above everything else.
pub(crate) fn below(offset: f32, content: impl IntoElement) -> AnyElement {
    div()
        .absolute()
        .left_0()
        .top_full()
        .mt(px(offset))
        .child(
            deferred(anchored().snap_to_window_with_margin(px(WINDOW_MARGIN)).child(content))
                .with_priority(layer::MENU),
        )
        .into_any_element()
}

/// A transparent window-sized layer that closes the popup on any press and swallows the event.
pub(crate) fn click_away(
    window: &Window,
    on_press: impl Fn(&mut Window, &mut App) + 'static,
) -> AnyElement {
    let viewport = window.viewport_size();
    let layer = div()
        .occlude()
        .w(viewport.width)
        .h(viewport.height)
        .on_mouse_down(MouseButton::Left, move |_, window, cx| on_press(window, cx));
    deferred(anchored().position(point(px(0.0), px(0.0))).child(layer))
        .with_priority(layer::MENU - 1)
        .into_any_element()
}

#[cfg(test)]
#[path = "popup_tests.rs"]
mod tests;
