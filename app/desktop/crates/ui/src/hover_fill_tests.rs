use std::time::Instant;

use gpui::{Context, Entity, IntoElement, Render, TestAppContext, Window, div, hsla, point, px};

use super::*;
use crate::cubic_bezier::CubicBezier;
use crate::motion::Motion;
use crate::theme::Theme;
use crate::toggle_state::ToggleState;
use crate::tween::Tween;

struct Probe {
    state: Option<Entity<ToggleState>>,
}

impl Render for Probe {
    fn render(&mut self, window: &mut Window, cx: &mut Context<'_, Self>) -> impl IntoElement {
        self.state = Some(hover::track(
            &ElementId::from("fill"),
            Pose::Off,
            window,
            cx,
        ));
        hover_fill(
            div().size(px(40.0)),
            "fill",
            hsla(0.0, 0.0, 0.0, 0.5),
            (window, cx),
        )
    }
}

#[gpui::test]
fn the_pointer_entering_starts_a_fade_on_the_element_id(cx: &mut TestAppContext) {
    cx.update(|cx| {
        cx.set_global(Theme::default());
        cx.set_global(Motion::default());
    });
    let (view, cx) = cx.add_window_view(|_, _| Probe { state: None });
    cx.simulate_mouse_move(point(px(10.0), px(10.0)), None, gpui::Modifiers::none());
    let state = view.read_with(cx, |probe, _| probe.state.clone());
    let tween = Tween::new(crate::timing::DEFAULT, CubicBezier::DEFAULT);
    let running = state.map(|state| {
        state.read_with(cx, |s, _| {
            s.position(tween, Motion::default(), Instant::now()).running
        })
    });
    assert_eq!(
        running,
        Some(true),
        "a fade should run right after entering"
    );
}
