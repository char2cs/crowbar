//! A titled group of rows.

use gpui::{AnyElement, App, IntoElement, ParentElement, Styled, Window, div};
use settings::{Row, Section};

use crate::frame::Frame;
use crate::row_view::row_view;
use crate::spacing;
use crate::title_block::title_block;

pub(crate) fn section_view(
    frame: &Frame<'_>,
    section: &Section,
    (window, cx): (&mut Window, &mut App),
) -> Option<AnyElement> {
    let rows: Vec<&Row> = section
        .rows
        .iter()
        .filter(|row| frame.state.row_visible(row))
        .collect();
    if rows.is_empty() {
        return None;
    }
    let header = section
        .show_header
        .then(|| title_block(&section.title, section.description.as_deref(), cx));
    let mut body: Vec<AnyElement> = Vec::with_capacity(rows.len());
    for row in rows {
        body.push(row_view(frame, row, (&mut *window, &mut *cx)).into_any_element());
    }
    let section = div()
        .px(spacing::TIGHT)
        .py(spacing::HAIRLINE)
        .children(header)
        .child(
            div()
                .flex()
                .flex_col()
                .gap(spacing::COMFORTABLE)
                .children(body),
        );
    Some(section.into_any_element())
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use crate::intent::Intent;
    use crate::test_support::{inspect, open, row, run};
    use ui::TextInputEvent;

    #[gpui::test]
    fn a_search_hides_the_rows_that_do_not_match_and_keeps_those_that_do(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx, "appearance");
        run(
            &harness,
            Intent::Search(TextInputEvent::Changed("font size".to_owned())),
            cx,
        );
        let shown = |key: &str,
                     harness: &crate::test_support::Harness,
                     cx: &mut gpui::VisualTestContext| {
            let target = row(key);
            inspect(harness, cx, |modal| modal.state.row_visible(&target))
        };
        assert!(shown("uiFontSize", &harness, cx));
        assert!(!shown("themeMode", &harness, cx));
    }
}
