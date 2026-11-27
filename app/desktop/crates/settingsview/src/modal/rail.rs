mod item;

use gpui::prelude::FluentBuilder as _;
use gpui::{
    AnyElement, App, InteractiveElement as _, IntoElement, ParentElement,
    StatefulInteractiveElement as _, Styled, Window, div, px,
};

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
    let narrow = ui::app_window_width(window) < NARROW;
    div()
        .flex_none()
        .w(px(WIDTH))
        .h_full()
        .flex()
        .flex_col()
        .when(narrow, Styled::hidden)
        .gap(spacing::HAIRLINE)
        .p(spacing::COMFORTABLE)
        .id("settings-rail")
        .overflow_y_scroll()
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
        run(
            &harness,
            Intent::Search(TextInputEvent::Changed(String::new())),
            cx,
        );
        assert_eq!(pages(&harness, cx), 11);
    }

    fn rail_and_page(
        harness: &crate::test_support::Harness,
        cx: &mut gpui::VisualTestContext,
        query: &str,
    ) -> (Vec<String>, Option<String>) {
        let typed = Intent::Search(TextInputEvent::Changed(query.to_owned()));
        run(harness, typed, cx);
        inspect(harness, cx, |modal| {
            let rail = modal
                .state
                .visible_pages()
                .iter()
                .map(|page| page.label().to_owned())
                .collect();
            (rail, modal.state.page().map(|page| page.label().to_owned()))
        })
    }

    #[gpui::test]
    fn a_search_lists_only_indexed_pages_and_opens_the_first(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx, "appearance");
        let (rail, page) = rail_and_page(&harness, cx, "terminal");
        assert_eq!(rail, ["Terminal", "Agents"]);
        assert_eq!(page.as_deref(), Some("Terminal"));
        let (rail, _) = rail_and_page(&harness, cx, "a");
        for unindexed in ["Active sessions", "Keybindings", "Hosts"] {
            assert!(
                !rail.iter().any(|label| label == unindexed),
                "{unindexed} in {rail:?}"
            );
        }
    }

    #[gpui::test]
    fn the_wheel_scrolls_a_rail_taller_than_the_dialog(cx: &mut TestAppContext) {
        let (_, cx) = open(cx, "appearance");
        cx.simulate_resize(gpui::size(gpui::px(760.0), gpui::px(480.0)));
        cx.run_until_parked();
        let first = |cx: &mut gpui::VisualTestContext| cx.debug_bounds("rail-appearance").unwrap();
        let before = first(cx);
        cx.simulate_event(gpui::ScrollEvent {
            position: before.center(),
            delta: gpui::ScrollDelta::Pixels(gpui::point(gpui::px(0.0), gpui::px(-60.0))),
            ..Default::default()
        });
        cx.run_until_parked();
        assert!(
            first(cx).top() < before.top(),
            "{:?} then {:?}",
            before,
            first(cx)
        );
    }
}
