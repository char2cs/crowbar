use std::rc::Rc;

use gpui::{
    Context, FontWeight, InteractiveElement as _, IntoElement, ParentElement, Render, Styled,
    Window, div, px,
};
use ui::{Button, ButtonSize, ButtonVariant, Surface, Theme};

const GLYPH: &str = "⚠";
const TITLE: &str = "Could not load your projects";
const RETRY: &str = "↺ Retry";
const GLYPH_TEXT: f32 = 18.0;
const GLYPH_OPACITY: f32 = 0.5;
const TITLE_TEXT: f32 = 14.0;
const GAP: f32 = 8.0;
const PADDING: f32 = 24.0;
const RETRY_LIFT: f32 = 4.0;
const RETRY_SELECTOR: &str = "projects-retry-slot";

pub(crate) struct ProjectsFailed {
    retry: Rc<dyn Fn()>,
}

impl ProjectsFailed {
    pub(crate) fn new(retry: Rc<dyn Fn()>) -> Self {
        Self { retry }
    }
}

impl Render for ProjectsFailed {
    fn render(
        &mut self,
        _: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        let palette = Theme::of(cx).palette(Surface::Window);
        let retry = Button::new("projects-retry", RETRY)
            .variant(ButtonVariant::Outline)
            .size(ButtonSize::Sm)
            .on_click(cx.listener(|landing, _, _, _| (landing.retry)()));
        div()
            .absolute()
            .inset_0()
            .bg(palette.pane_background())
            .flex()
            .flex_col()
            .items_center()
            .justify_center()
            .gap(px(GAP))
            .p(px(PADDING))
            .child(
                div()
                    .text_size(px(GLYPH_TEXT))
                    .opacity(GLYPH_OPACITY)
                    .child(GLYPH),
            )
            .child(
                div()
                    .text_size(px(TITLE_TEXT))
                    .font_weight(FontWeight::MEDIUM)
                    .text_color(palette.foreground())
                    .child(TITLE),
            )
            .child(
                div()
                    .mt(px(RETRY_LIFT))
                    .debug_selector(|| RETRY_SELECTOR.into())
                    .child(retry),
            )
    }
}

#[cfg(test)]
mod tests {
    use std::cell::Cell;

    use gpui::{Modifiers, TestAppContext};

    use super::*;

    #[gpui::test]
    fn retry_asks_for_the_projects_again(cx: &mut TestAppContext) {
        cx.update(|cx| ui::init(cx).unwrap());
        let asked = Rc::new(Cell::new(0));
        let counted = Rc::clone(&asked);
        let retry: Rc<dyn Fn()> = Rc::new(move || counted.set(counted.get() + 1));
        let (_, window) = cx.add_window_view(|_, _| ProjectsFailed::new(retry));
        window.run_until_parked();
        let button = window.debug_bounds(RETRY_SELECTOR).unwrap();
        window.simulate_click(button.center(), Modifiers::none());
        assert_eq!(asked.get(), 1);
    }
}
