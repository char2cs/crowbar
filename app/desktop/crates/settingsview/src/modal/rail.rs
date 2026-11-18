//! The page list on the left.

mod item;

use gpui::prelude::FluentBuilder as _;
use gpui::{AnyElement, App, IntoElement, ParentElement, Styled, Window, div, px};

use crate::frame::Frame;
use crate::frame::spacing;
use crate::modal::empty_view::empty_view;
use crate::modal::rail::item::rail_item;

const WIDTH: f32 = 208.0;
const NARROW: f32 = 720.0;

pub(crate) fn rail(
    frame: &Frame<'_>,
    (window, cx): (&mut Window, &mut App),
) -> impl IntoElement {
    let pages = frame.state.visible_pages();
    let mut items: Vec<AnyElement> = Vec::with_capacity(pages.len());
    for page in pages {
        items.push(rail_item(frame, page, (&mut *window, &mut *cx)).into_any_element());
    }
    let empty = items.is_empty().then(|| empty_view(cx));
    let narrow = f32::from(window.viewport_size().width) < NARROW;
    div()
        .flex_none()
        .when(narrow, Styled::hidden)
        .w(px(WIDTH))
        .h_full()
        .flex()
        .flex_col()
        .gap(spacing::HAIRLINE)
        .p(spacing::COMFORTABLE)
        .overflow_hidden()
        .children(items)
        .children(empty)
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use crate::intent::Intent;
    use crate::test_support::{inspect, open, run};
    use ui::TextInputEvent;

    #[gpui::test]
    fn a_search_narrows_the_rail_and_clearing_it_brings_every_page_back(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx, "appearance");
        let pages = |harness: &crate::test_support::Harness, cx: &mut gpui::VisualTestContext| {
            inspect(harness, cx, |modal| modal.state.visible_pages().len())
        };
        run(
            &harness,
            Intent::Search(TextInputEvent::Changed("scrollback".to_owned())),
            cx,
        );
        assert_eq!(pages(&harness, cx), 1);
        run(&harness, Intent::Search(TextInputEvent::Cancelled), cx);
        assert_eq!(pages(&harness, cx), 10);
    }
}
