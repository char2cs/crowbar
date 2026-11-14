//! The body of the modal: the rail and the page.

use gpui::{
    AnyElement, App, FocusHandle, InteractiveElement, IntoElement, ParentElement, ScrollHandle,
    Styled, Window, div,
};

use crate::content::content;
use crate::dispatch::deliver;
use crate::frame::Frame;
use crate::key_mode::KeyMode;
use crate::key_routing::route;
use crate::rail::rail;

pub(crate) fn body(
    frame: &Frame<'_>,
    (focus, scroll): (&FocusHandle, &ScrollHandle),
    (window, cx): (&mut Window, &mut App),
) -> AnyElement {
    let owner = frame.owner.clone();
    let capturing = frame.state.capturing().is_some();
    let focused = focus.clone();
    div()
        .id("settings-body")
        .track_focus(focus)
        .flex()
        .size_full()
        .min_w_0()
        .overflow_hidden()
        .on_key_down(move |event, window, cx| {
            let mode = match (capturing, focused.is_focused(window)) {
                (true, _) => KeyMode::Capturing,
                (false, true) => KeyMode::Rail,
                (false, false) => KeyMode::Page,
            };
            if let Some(intent) = route(&event.keystroke, mode) {
                cx.stop_propagation();
                deliver(&owner, intent, (window, cx));
            }
        })
        .child(rail(frame, (&mut *window, &mut *cx)))
        .child(content(frame, scroll, (window, cx)))
        .into_any_element()
}

#[cfg(test)]
#[path = "body_tests.rs"]
mod tests;
