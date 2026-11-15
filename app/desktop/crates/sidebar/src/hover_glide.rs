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
    state: &Entity<HoverGlide>
) -> impl Fn(&bool, &mut Window, &mut App) + 'static {
    let state = state.clone();
    move |hovered, _, cx| glide_to(&state, f32::from(u8::from(*hovered)), cx)
}

fn glide_to(
    state: &Entity<HoverGlide>,
    target: f32,
    cx: &mut App,
) {
    state.update(cx, |state, cx| {
        state.glide.go_to(target, Instant::now(), Motion::of(cx));
        cx.notify();
    });
}

pub(crate) fn value(
    state: &Entity<HoverGlide>,
    window: &mut Window,
    cx: &App,
) -> f32 {
    let (now, motion) = (Instant::now(), Motion::of(cx));
    let glide = &state.read(cx).glide;
    if glide.is_running(now, motion) {
        window.request_animation_frame();
    }
    glide.value(now, motion)
}

#[cfg(test)]
mod tests {
    use gpui::{AppContext as _, Context, IntoElement, Render, TestAppContext, Window};

    use super::*;

    struct Probe {
        seen: std::rc::Rc<std::cell::Cell<f32>>,
    }

    impl Render for Probe {
        fn render(
            &mut self,
            window: &mut Window,
            cx: &mut Context<'_, Self>,
        ) -> impl IntoElement {
            let state = track("probe", window, cx);
            self.seen.set(value(&state, window, cx));
            gpui::div()
        }
    }

    #[gpui::test]
    fn an_unhovered_element_rests_at_zero(cx: &mut TestAppContext) {
        cx.update(|cx| ui::init(cx).expect("ui installs"));
        let seen = std::rc::Rc::new(std::cell::Cell::new(1.0));
        let shared = std::rc::Rc::clone(&seen);
        let (_, _cx) = cx.add_window_view(|_, _| Probe { seen: shared });
        assert_eq!(seen.get(), 0.0);
    }

    #[gpui::test]
    fn hovering_starts_a_glide_toward_one(cx: &mut TestAppContext) {
        cx.update(|cx| ui::init(cx).expect("ui installs"));
        let (_, cx) = cx.add_window_view(|_, _| gpui::Empty);
        let state = cx.update(|_, cx| cx.new(|_| HoverGlide::resting()));
        cx.update(|window, cx| listener(&state)(&true, window, cx));
        let running = cx.update(|_, cx| {
            let motion = Motion::of(cx);
            state.read(cx).glide.is_running(Instant::now(), motion)
        });
        assert!(running);
    }
}
