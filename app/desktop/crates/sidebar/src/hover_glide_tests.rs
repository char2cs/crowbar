use gpui::{AppContext as _, Context, IntoElement, Render, TestAppContext, Window};

use super::*;

struct Probe {
    seen: std::rc::Rc<std::cell::Cell<f32>>,
}

impl Render for Probe {
    fn render(&mut self, window: &mut Window, cx: &mut Context<'_, Self>) -> impl IntoElement {
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
