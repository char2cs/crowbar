use gpui::{
    Context, InteractiveElement as _, IntoElement, Render, StatefulInteractiveElement as _,
    Styled as _, TestAppContext, Window, div, point, px,
};

use super::*;
use crate::timing;

struct Probe {
    state: Option<Entity<ToggleState>>,
}

impl Render for Probe {
    fn render(&mut self, window: &mut Window, cx: &mut Context<'_, Self>) -> impl IntoElement {
        let id = ElementId::from("probe");
        let state = track(&id, Pose::Off, window, cx);
        self.state = Some(state.clone());
        div().id(id).size(px(40.0)).on_hover(listener(&state))
    }
}

#[gpui::test]
fn pointer_enter_starts_a_fade(cx: &mut TestAppContext) {
    let (view, cx) = cx.add_window_view(|_, _| Probe { state: None });
    cx.simulate_mouse_move(point(px(10.0), px(10.0)), None, gpui::Modifiers::none());
    let state = view.read_with(cx, |probe, _| probe.state.clone());
    let tween = Tween::new(timing::DEFAULT, CubicBezier::DEFAULT);
    let at = state.map(|state| {
        state.read_with(cx, |s, _| {
            s.position(tween, Motion::default(), Instant::now())
        })
    });
    assert_eq!(
        at.map(|at| at.running),
        Some(true),
        "a fade should be running right after entering"
    );
}
