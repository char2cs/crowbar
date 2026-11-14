//! Renders a single element in a test window.

use std::rc::Rc;

use gpui::{
    AnyElement, App, Bounds, Context, InteractiveElement, IntoElement, ParentElement, Pixels, Render, Styled, TestAppContext,
    VisualTestContext, Window, div,
};

use crate::motion::Motion;
use crate::theme::Theme;

type Build = Rc<dyn Fn(&mut Window, &mut App) -> AnyElement>;

/// A view that shows the element built by its closure inside a fit-content wrapper named `probe`.
pub(crate) struct Probe {
    build: Build,
}

impl Render for Probe {
    fn render(&mut self, window: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        let element = (self.build)(window, cx);
        div().size_full().flex().items_start().child(div().flex_none().debug_selector(|| "probe".into()).child(element))
    }
}

/// Opens a window showing `build()` with the default theme installed.
pub(crate) fn open<E: IntoElement>(
    cx: &mut TestAppContext,
    build: impl Fn(&mut Window, &mut App) -> E + 'static,
) -> &mut VisualTestContext {
    cx.update(|cx| {
        cx.set_global(Theme::default());
        cx.set_global(Motion::default());
        cx.bind_keys(crate::menu_actions::bindings());
        cx.bind_keys(crate::text_input_actions::bindings());
    });
    let build: Build = Rc::new(move |window, cx| build(window, cx).into_any_element());
    let (_, cx) = cx.add_window_view(move |_, _| Probe { build });
    cx.run_until_parked();
    cx
}

/// Bounds of the element built by `build`, laid out at its natural size.
pub(crate) fn measure<E: IntoElement>(
    cx: &mut TestAppContext,
    build: impl Fn(&mut Window, &mut App) -> E + 'static,
) -> Bounds<Pixels> {
    let cx = open(cx, build);
    cx.debug_bounds("probe").unwrap_or_default()
}
