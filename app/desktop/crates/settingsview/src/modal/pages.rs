mod block_view;
mod button_block;
mod heading_view;
pub(crate) mod records;
mod rows;
mod section_view;
pub(crate) mod shortcuts;
mod title_block;

use gpui::{
    AnyElement, App, IntoElement, ParentElement, Pixels, ScrollHandle, Styled, Window, div, px,
};
use settings::Block;
use ui::ScrollArea;

use crate::frame::Frame;
use crate::frame::spacing;
use crate::modal::is_narrow;
use crate::modal::pages::block_view::block_view;

pub(crate) fn content(
    frame: &Frame<'_>,
    scroll: &ScrollHandle,
    (window, cx): (&mut Window, &mut App),
) -> impl IntoElement {
    let inner = match frame.state.page() {
        Some(page) => blocks(frame, page.blocks(), (window, cx))
            .p(page_padding(window))
            .into_any_element(),
        None => div().into_any_element(),
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
) -> gpui::Div {
    let drawn: Vec<(Pixels, AnyElement)> = blocks
        .iter()
        .enumerate()
        .filter_map(|block| spaced(frame, block, (&mut *window, &mut *cx)))
        .collect();
    let gaps: Vec<Pixels> = std::iter::once(px(0.0))
        .chain(drawn.iter().map(|(gap, _)| *gap))
        .collect();
    let views = drawn
        .into_iter()
        .zip(gaps)
        .map(|((_, view), gap)| div().mt(gap).child(view));
    div().flex().flex_col().children(views.collect::<Vec<_>>())
}

fn spaced(
    frame: &Frame<'_>,
    block: (usize, &Block),
    (window, cx): (&mut Window, &mut App),
) -> Option<(Pixels, AnyElement)> {
    let gap = match block.1 {
        Block::Heading(_) => px(0.0),
        _ => spacing::GENEROUS,
    };
    block_view(frame, block, (window, cx)).map(|view| (gap, view))
}

fn page_padding(window: &Window) -> Pixels {
    if is_narrow(window) {
        return spacing::COMFORTABLE;
    }
    spacing::ROOMY
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use crate::intent::Intent;
    use crate::test_support::{inspect, open, run};
    use settings::PageId;
    use ui::TextInputEvent;

    #[gpui::test]
    fn a_page_heading_sits_only_its_own_eight_pixels_above_the_next_block(cx: &mut TestAppContext) {
        let (_harness, cx) = open(cx, "providers");
        let heading = cx.debug_bounds("page-heading").unwrap();
        let row = cx.debug_bounds("row-text-defaultPermissionLevel").unwrap();
        let section_and_row_padding = 2.0 + 8.0;
        assert_eq!(
            f32::from(row.top() - heading.bottom()),
            8.0 + section_and_row_padding,
            "develop's heading margin replaces the page's block gap"
        );
    }

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

    #[gpui::test]
    fn another_page_keeps_the_scroll_offset_of_the_one_scroller(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx, "editor");
        let top = |cx: &mut gpui::VisualTestContext| {
            cx.debug_bounds("row-text-fontFamily")
                .map(|bounds| bounds.top())
        };
        let fresh = top(cx);
        run(&harness, Intent::Select(PageId::from("keybindings")), cx);
        let chord = cx.debug_bounds("chord-navigation.toggleConsole").unwrap();
        cx.simulate_event(gpui::ScrollEvent {
            position: chord.center(),
            delta: gpui::ScrollDelta::Pixels(gpui::point(gpui::px(0.0), gpui::px(-400.0))),
            ..Default::default()
        });
        cx.run_until_parked();
        run(&harness, Intent::Select(PageId::from("editor")), cx);
        let moved = top(cx);
        assert!(
            moved.zip(fresh).is_some_and(|(moved, fresh)| moved < fresh),
            "{moved:?} above {fresh:?}"
        );
    }
}
