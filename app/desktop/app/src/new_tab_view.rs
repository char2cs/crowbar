use gpui::{Context, EventEmitter, IntoElement, ParentElement, Render, Styled, Window, div, px};
use ui::{Button, ButtonSize, ButtonVariant, Surface, Theme, Wordmark};

use crate::new_tab_action::NewTabAction;

const WORDMARK_WIDTH: f32 = 128.0;
const PAGE_GAP: f32 = 24.0;
const ROW_GAP: f32 = 2.0;

pub(crate) struct NewTabView;

impl EventEmitter<NewTabAction> for NewTabView {}

impl Render for NewTabView {
    fn render(
        &mut self,
        _: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        let palette = Theme::of(cx).palette(Surface::Window);
        let rows = NewTabAction::ALL.map(|action| {
            Button::new(action.label(), action.label())
                .icon(action.icon())
                .variant(ButtonVariant::Ghost)
                .size(ButtonSize::Sm)
                .on_click(cx.listener(move |_, _, _, cx| cx.emit(action)))
        });
        div()
            .size_full()
            .flex()
            .flex_col()
            .items_center()
            .justify_center()
            .gap(px(PAGE_GAP))
            .bg(palette.pane_background())
            .child(Wordmark::new(px(WORDMARK_WIDTH)))
            .child(div().flex().flex_col().gap(px(ROW_GAP)).children(rows))
    }
}

#[cfg(test)]
mod tests {
    use std::cell::RefCell;
    use std::rc::Rc;

    use gpui::{AppContext as _, TestAppContext};

    use super::*;

    #[gpui::test]
    fn the_view_is_an_emitter_of_actions(cx: &mut TestAppContext) {
        let heard = Rc::new(RefCell::new(Vec::new()));
        let view = cx.new(|_| NewTabView);
        let sink = Rc::clone(&heard);
        cx.update(|cx| {
            cx.subscribe(&view, move |_, action: &NewTabAction, _| {
                sink.borrow_mut().push(*action);
            })
            .detach();
        });
        view.update(cx, |_, cx| cx.emit(NewTabAction::File));
        assert_eq!(*heard.borrow(), [NewTabAction::File]);
    }
}
