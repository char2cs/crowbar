//! Renders one element in a test window and reports its bounds.

use std::rc::Rc;

use gpui::{
    AnyElement, App, Bounds, Context, InteractiveElement, IntoElement, ParentElement, Pixels,
    Render, Styled, TestAppContext, Window, div,
};

type Build = Rc<dyn Fn(&mut Window, &mut App) -> AnyElement>;

struct Probe {
    build: Build,
}

impl Render for Probe {
    fn render(&mut self, window: &mut Window, cx: &mut Context<'_, Self>) -> impl IntoElement {
        let element = (self.build)(window, cx);
        div().size_full().flex().items_start().child(
            div()
                .flex_none()
                .w(gpui::px(294.0))
                .debug_selector(|| "probe".into())
                .child(element),
        )
    }
}

pub(crate) fn measure<E: IntoElement>(
    cx: &mut TestAppContext,
    build: impl Fn(&mut Window, &mut App) -> E + 'static,
) -> Bounds<Pixels> {
    cx.update(|cx| ui::init(cx).expect("ui installs"));
    let build: Build = Rc::new(move |window, cx| build(window, cx).into_any_element());
    let (_, cx) = cx.add_window_view(move |_, _| Probe { build });
    cx.run_until_parked();
    cx.debug_bounds("probe").unwrap_or_default()
}
