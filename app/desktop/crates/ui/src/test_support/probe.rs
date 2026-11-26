//! Renders a single element in a test window.

use std::rc::Rc;

use gpui::{
    AnyElement, App, Bounds, Context, InteractiveElement, IntoElement, ParentElement, Pixels,
    Render, Styled, TestAppContext, VisualTestContext, Window, div,
};

use crate::foundation::Modality;
use crate::foundation::Motion;
use crate::foundation::Theme;

type Build = Rc<dyn Fn(&mut Window, &mut App) -> AnyElement>;

/// A view that shows the element built by its closure inside a fit-content wrapper named `probe`.
pub(crate) struct Probe {
    build: Build,
}

impl Render for Probe {
    fn render(
        &mut self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        let element = (self.build)(window, cx);
        div()
            .size_full()
            .flex()
            .items_start()
            .child(crate::hear())
            .child(
                div()
                    .flex_none()
                    .debug_selector(|| "probe".into())
                    .child(element),
            )
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
        Modality::install(cx);
        cx.bind_keys(crate::floating::menu_bindings());
        cx.bind_keys(crate::floating::popover_bindings());
        cx.bind_keys(crate::floating::dialog_bindings());
        cx.bind_keys(crate::control::text_input_bindings());
    });
    let build: Build = Rc::new(move |window, cx| build(window, cx).into_any_element());
    let (_, cx) = cx.add_window_view(move |_, _| Probe { build });
    cx.run_until_parked();
    cx
}

/// Opens a window showing `build()` with a float host installed, so its passive layers paint
/// in its pop-up window, which [`crate::testing::newest_window`] finds.
pub(crate) fn open_floating<E: IntoElement>(
    cx: &mut TestAppContext,
    build: impl Fn(&mut Window, &mut App) -> E + 'static,
) -> VisualTestContext {
    cx.update(crate::testing::open_float_windows);
    let window = open(cx, build).update(|window, _| window.window_handle());
    VisualTestContext::from_window(window, cx)
}

/// Lets a closed floating element go: the test platform draws inside an effect
/// flush, so the state of a portal that stopped rendering is released on the next one.
pub(crate) fn settle(cx: &mut VisualTestContext) {
    cx.update(|_, _| {});
    cx.run_until_parked();
}

/// Bounds of the element built by `build`, laid out at its natural size.
pub(crate) fn measure<E: IntoElement>(
    cx: &mut TestAppContext,
    build: impl Fn(&mut Window, &mut App) -> E + 'static,
) -> Bounds<Pixels> {
    let cx = open(cx, build);
    cx.debug_bounds("probe").unwrap_or_default()
}
