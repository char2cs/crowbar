//! Wiring of [`ToggleState`] into an element's hover and into animated properties.

use std::time::Instant;

use gpui::{App, ElementId, Entity, Hsla, Window};

use crate::color_mix::mix;
use crate::cubic_bezier::CubicBezier;
use crate::motion::Motion;
use crate::pose::Pose;
use crate::toggle_state::ToggleState;
use crate::tween::Tween;

pub(crate) fn track(
    id: &ElementId,
    pose: Pose,
    window: &mut Window,
    cx: &mut App,
) -> Entity<ToggleState> {
    window.use_keyed_state((id.clone(), "toggle"), cx, |_, _| {
        ToggleState::starting(pose)
    })
}

pub(crate) fn listener(
    state: &Entity<ToggleState>,
) -> impl Fn(&bool, &mut Window, &mut App) + 'static {
    let state = state.clone();
    move |hovered, _, cx| state.update(cx, |s, cx| s.set(Pose::from(*hovered), cx))
}

pub(crate) fn position(
    state: &Entity<ToggleState>,
    tween: Tween,
    window: &mut Window,
    cx: &App,
) -> f32 {
    let at = state
        .read(cx)
        .position(tween, Motion::of(cx), Instant::now());
    if at.running {
        window.request_animation_frame();
    }
    at.value
}

pub(crate) fn color(
    state: &Entity<ToggleState>,
    (rest, hot): (Hsla, Hsla),
    window: &mut Window,
    cx: &App,
) -> Hsla {
    let tween = Tween::new(crate::timing::DEFAULT, CubicBezier::DEFAULT);
    mix(rest, hot, position(state, tween, window, cx))
}

#[cfg(test)]
#[path = "hover_tests.rs"]
mod tests;
