//! Wiring of [`ToggleState`] into an element's hover and into animated properties.

use std::time::{Duration, Instant};

use gpui::{App, ElementId, Entity, Hsla, Window};

use crate::color_mix::mix;
use crate::cubic_bezier::CubicBezier;
use crate::motion::Motion;
use crate::timing;
use crate::toggle_state::{Position, ToggleState};

/// The state of `key` under `id`, created on first use and kept while the element keeps rendering.
pub(crate) fn track(id: &ElementId, key: &'static str, on: bool, window: &mut Window, cx: &mut App) -> Entity<ToggleState> {
    window.use_keyed_state((id.clone(), key), cx, |_, _| ToggleState::starting(on))
}

/// The listener to attach with `on_hover`.
pub(crate) fn listener(state: &Entity<ToggleState>) -> impl Fn(&bool, &mut Window, &mut App) + 'static {
    let state = state.clone();
    move |hovered, _, cx| state.update(cx, |s, cx| s.set(*hovered, cx))
}

/// The eased position for this frame; asks for another frame while it runs.
pub(crate) fn position(
    state: &Entity<ToggleState>,
    span: Duration,
    curve: CubicBezier,
    window: &mut Window,
    cx: &App,
) -> f32 {
    let at: Position = state.read(cx).position(span, curve, Motion::of(cx), Instant::now());
    if at.running {
        window.request_animation_frame();
    }
    at.value
}

/// The hover fade between `rest` and `hot`: 150ms on the default curve.
pub(crate) fn color(state: &Entity<ToggleState>, rest: Hsla, hot: Hsla, window: &mut Window, cx: &App) -> Hsla {
    let at = position(state, timing::DEFAULT, CubicBezier::DEFAULT, window, cx);
    mix(rest, hot, at)
}

#[cfg(test)]
#[path = "hover_tests.rs"]
mod tests;
