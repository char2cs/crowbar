use gpui::{
    Context, EventEmitter, IntoElement, ParentElement, Pixels, Render, Size, Styled, Window, div,
    px,
};
use ui::{Button, ButtonSize, ButtonVariant, Wordmark};

use crate::panes::content::new_tab::new_tab_action::NewTabAction;

const WORDMARK_MIN: f32 = 96.0;
const WORDMARK_MAX: f32 = 148.0;
const WORDMARK_SHARE: f32 = 0.14;
const PAGE_GAP: f32 = 24.0;
const ROW_GAP: f32 = 2.0;

pub(crate) struct NewTabView {
    pub(crate) actions: [NewTabAction; NewTabAction::ALL.len()],
}

impl EventEmitter<NewTabAction> for NewTabView {}

impl Render for NewTabView {
    fn render(
        &mut self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        let wordmark = wordmark_width(window.viewport_size());
        let rows = self.actions.map(|action| {
            Button::new(action.label(), action.label())
                .icon(action.icon())
                .variant(ButtonVariant::Row)
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
            .child(Wordmark::new(wordmark))
            .child(div().flex().flex_col().gap(px(ROW_GAP)).children(rows))
    }
}

fn wordmark_width(viewport: Size<Pixels>) -> Pixels {
    let short = f32::from(viewport.width.min(viewport.height));
    px((short * WORDMARK_SHARE).clamp(WORDMARK_MIN, WORDMARK_MAX))
}

#[cfg(test)]
mod tests {
    use std::cell::RefCell;
    use std::rc::Rc;

    use gpui::{App, AppContext as _, Entity, TestAppContext};

    use super::*;

    fn listen(
        view: &Entity<NewTabView>,
        sink: Rc<RefCell<Vec<NewTabAction>>>,
        cx: &mut TestAppContext,
    ) {
        let handler = move |_, action: &NewTabAction, _: &mut App| sink.borrow_mut().push(*action);
        cx.update(|cx| cx.subscribe(view, handler).detach());
    }

    #[test]
    fn the_wordmark_is_a_seventh_of_the_window_s_short_side_within_96_and_148_px() {
        let cases = [
            ((640.0, 480.0), 96.0),
            ((1280.0, 800.0), 112.0),
            ((1920.0, 1200.0), 148.0),
        ];
        for ((width, height), expected) in cases {
            let viewport = gpui::size(px(width), px(height));
            assert_eq!(wordmark_width(viewport), px(expected), "{width}x{height}");
        }
    }

    #[gpui::test]
    fn the_view_is_an_emitter_of_actions(cx: &mut TestAppContext) {
        let heard = Rc::new(RefCell::new(Vec::new()));
        let view = cx.new(|_| NewTabView {
            actions: NewTabAction::ALL,
        });
        listen(&view, Rc::clone(&heard), cx);
        view.update(cx, |_, cx| cx.emit(NewTabAction::File));
        assert_eq!(*heard.borrow(), [NewTabAction::File]);
    }
}
