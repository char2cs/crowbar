//! A titled group of rows.

use gpui::{
    AnyElement, App, InteractiveElement as _, IntoElement, ParentElement, Styled, Window, div,
};
use settings::{Row, Section};

use crate::frame::Frame;
use crate::frame::spacing;
use crate::modal::pages::records::note_text;
use crate::modal::pages::rows::row_view;
use crate::modal::pages::title_block::title_block;

pub(crate) fn section_view(
    frame: &Frame<'_>,
    section: &Section,
    (window, cx): (&mut Window, &mut App),
) -> Option<AnyElement> {
    let rows: Vec<&Row> = section
        .rows()
        .iter()
        .filter(|row| frame.state.row_visible(row))
        .collect();
    if rows.is_empty() {
        return None;
    }
    let header = section
        .show_header()
        .then(|| title_block(section.title(), section.description(), cx));
    let mut body: Vec<AnyElement> = Vec::with_capacity(rows.len());
    for row in rows {
        body.push(row_view(frame, row, (&mut *window, &mut *cx)).into_any_element());
    }
    let note = section.note().map(|text| note_view(text, cx));
    let section = div()
        .px(spacing::TIGHT)
        .py(spacing::HAIRLINE)
        .children(header)
        .child(
            div()
                .flex()
                .flex_col()
                .gap(spacing::COMFORTABLE)
                .children(body)
                .children(note),
        );
    Some(section.into_any_element())
}

fn note_view(
    text: &str,
    cx: &App,
) -> AnyElement {
    div()
        .debug_selector(|| "section-note".to_owned())
        .px(spacing::TIGHT)
        .child(note_text(text, cx))
        .into_any_element()
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

    #[gpui::test]
    fn the_preset_note_runs_under_the_preset_row(cx: &mut TestAppContext) {
        let (_harness, cx) = open(cx, "keybindings");
        let label = cx.debug_bounds("row-text-keybindingPreset").unwrap();
        let note = cx.debug_bounds("section-note");
        assert!(
            note.is_some_and(|note| note.top() >= label.bottom()),
            "note {note:?} under label {label:?}"
        );
    }
}
