use gpui::prelude::FluentBuilder as _;
use gpui::{
    AnyElement, App, FontWeight, InteractiveElement as _, IntoElement as _, ParentElement as _,
    Styled as _, div, px,
};
use ui::{Button, ButtonSize, ButtonVariant, Surface, Theme};

use super::retry::Retry;

const TRY_AGAIN: &str = "Try again";
const BUTTON_ID: &str = "document-retry";
const RETRY_SELECTOR: &str = "document-retry-slot";
const PADDING: f32 = 32.0;
const GAP: f32 = 4.0;
const BUTTON_GAP: f32 = 8.0;
const TITLE_TEXT: f32 = 14.0;
const MESSAGE_TEXT: f32 = 12.0;

pub(crate) fn panel(
    (failure, retry): (&editor::OpenFailure, Retry),
    on_retry: impl Fn(&mut App) + 'static,
    cx: &App,
) -> AnyElement {
    let palette = Theme::of(cx).palette(Surface::Window);
    let button = Button::new(BUTTON_ID, TRY_AGAIN)
        .variant(ButtonVariant::Outline)
        .size(ButtonSize::Xs)
        .when(retry == Retry::Running, Button::loading)
        .on_click(move |_, _, cx| on_retry(cx));
    let title = div()
        .text_size(px(TITLE_TEXT))
        .font_weight(FontWeight::MEDIUM)
        .text_color(palette.foreground())
        .child(failure.title().to_owned());
    let message = div()
        .text_size(px(MESSAGE_TEXT))
        .text_color(palette.muted_foreground())
        .child(failure.message().to_owned());
    div()
        .size_full()
        .flex()
        .flex_col()
        .items_center()
        .justify_center()
        .gap(px(GAP))
        .p(px(PADDING))
        .bg(palette.pane_background())
        .child(title)
        .child(message)
        .child(
            div()
                .mt(px(BUTTON_GAP))
                .debug_selector(|| RETRY_SELECTOR.into())
                .child(button),
        )
        .into_any_element()
}

#[cfg(test)]
mod tests {
    use std::cell::Cell;
    use std::rc::Rc;

    use gpui::{Context, IntoElement, Modifiers, Render, TestAppContext, Window};

    use super::*;

    struct Shown(Rc<Cell<u32>>);

    impl Render for Shown {
        fn render(
            &mut self,
            _: &mut Window,
            cx: &mut Context<'_, Self>,
        ) -> impl IntoElement {
            let asked = Rc::clone(&self.0);
            let failure = editor::OpenFailure::of("src/a.ts", 404, None);
            panel(
                (&failure, Retry::Idle),
                move |_| asked.set(asked.get() + 1),
                cx,
            )
        }
    }

    #[gpui::test]
    fn try_again_asks_for_the_document_once_more(cx: &mut TestAppContext) {
        cx.update(|cx| ui::init(cx).unwrap());
        let asked = Rc::new(Cell::new(0));
        let shown = Rc::clone(&asked);
        let (_, window) = cx.add_window_view(|_, _| Shown(shown));
        window.run_until_parked();
        let button = window.debug_bounds(RETRY_SELECTOR).unwrap();
        window.simulate_click(button.center(), Modifiers::none());
        assert_eq!(asked.get(), 1);
    }
}
