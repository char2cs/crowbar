//! A 0..1 value that glides toward 1 while an element is hovered and back to 0 when it is not.

use std::time::Instant;

use gpui::{App, ElementId, Entity, Window};
use ui::{CubicBezier, Glide, Motion};

const SPAN: std::time::Duration = std::time::Duration::from_millis(150);

#[derive(Debug)]
pub(crate) struct HoverGlide {
    glide: Glide,
}

impl HoverGlide {
    fn resting() -> Self {
        Self {
            glide: Glide::resting(0.0, SPAN, CubicBezier::DEFAULT),
        }
    }
}

pub(crate) fn track(
    id: impl Into<ElementId>,
    window: &mut Window,
    cx: &mut App,
) -> Entity<HoverGlide> {
    let key = (id.into(), "hover-glide");
    window.use_keyed_state(key, cx, |_, _| HoverGlide::resting())
}

pub(crate) fn listener(
    state: &Entity<HoverGlide>,
) -> impl Fn(&bool, &mut Window, &mut App) + 'static {
    let state = state.clone();
    move |hovered, _, cx| {
        let target = if *hovered { 1.0 } else { 0.0 };
        state.update(cx, |state, cx| {
            state.glide.go_to(target, Instant::now(), Motion::of(cx));
            cx.notify();
        });
    }
}

pub(crate) fn value(state: &Entity<HoverGlide>, window: &mut Window, cx: &App) -> f32 {
    let (now, motion) = (Instant::now(), Motion::of(cx));
    let glide = &state.read(cx).glide;
    if glide.is_running(now, motion) {
        window.request_animation_frame();
    }
    glide.value(now, motion)
}

#[cfg(test)]
#[path = "hover_glide_tests.rs"]
mod tests;
