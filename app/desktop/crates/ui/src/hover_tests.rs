use gpui::{Context, IntoElement, Render, TestAppContext, Window, div, point, px};

use super::*;

struct Probe {
    state: Option<Entity<ToggleState>>,
}

impl Render for Probe {
    fn render(&mut self, window: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        let id = ElementId::from("probe");
        let state = track(&id, "hover", false, window, cx);
        self.state = Some(state.clone());
        div().id(id).size(px(40.0)).on_hover(listener(&state))
    }
}

#[gpui::test]
fn pointer_enter_starts_a_fade(cx: &mut TestAppContext) {
    let (view, cx) = cx.add_window_view(|_, _| Probe { state: None });
    cx.simulate_mouse_move(point(px(10.0), px(10.0)), None, gpui::Modifiers::none());
    let state = view.read_with(cx, |probe, _| probe.state.clone()).expect("rendered");
    let at = state.read_with(cx, |s, _| s.position(timing::DEFAULT, CubicBezier::DEFAULT, Motion::default(), Instant::now()));
    assert!(at.running, "a fade should be running right after entering");
}
