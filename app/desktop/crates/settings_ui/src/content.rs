//! The scrolling page on the right.

use gpui::{AnyElement, App, IntoElement, ParentElement, ScrollHandle, Styled, Window, div};
use ui::ScrollArea;

use crate::block::Block;
use crate::block_view::block_view;
use crate::empty_view::empty_view;
use crate::frame::Frame;
use crate::spacing;

pub(crate) fn content(
    frame: &Frame<'_>,
    scroll: &ScrollHandle,
    (window, cx): (&mut Window, &mut App),
) -> impl IntoElement {
    let inner = match frame.state.page() {
        Some(page) => blocks(frame, &page.blocks, (window, cx)).into_any_element(),
        None => empty_view(cx).into_any_element(),
    };
    div()
        .flex_1()
        .min_w_0()
        .h_full()
        .child(ScrollArea::new("settings-page", scroll, inner))
}

fn blocks(
    frame: &Frame<'_>,
    blocks: &[Block],
    (window, cx): (&mut Window, &mut App),
) -> impl IntoElement {
    let drawn: Vec<AnyElement> = blocks
        .iter()
        .enumerate()
        .filter_map(|block| block_view(frame, block, (&mut *window, &mut *cx)))
        .collect();
    div()
        .flex()
        .flex_col()
        .gap(spacing::GENEROUS)
        .p(spacing::ROOMY)
        .children(drawn)
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use crate::intent::Intent;
    use crate::test_support::{inspect, open, run};
    use ui::TextInputEvent;

    #[gpui::test]
    fn a_search_with_no_match_leaves_no_page_to_show(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx, "appearance");
        run(
            &harness,
            Intent::Search(TextInputEvent::Changed("zzzz".to_owned())),
            cx,
        );
        assert!(inspect(&harness, cx, |modal| modal
            .state
            .visible_pages()
            .is_empty()));
    }
}
