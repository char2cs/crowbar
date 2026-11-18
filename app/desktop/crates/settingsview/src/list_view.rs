//! A list of backend records with its loading, failed and empty states.

use gpui::{AnyElement, App, IntoElement, ParentElement, Styled, Window, div, px};
use settings::{ListBlock, Records};
use ui::{Button, ButtonSize, ButtonVariant, IconName, Skeleton};

use crate::frame::Frame;
use crate::intent::Intent;
use crate::note_text::note_text;
use crate::record_marks::FLAG_COLUMN;
use crate::record_ref::RecordRef;
use crate::record_view::record_view;
use crate::spacing;
use crate::title_block::title_block;

const SKELETON_WIDTH: f32 = 320.0;
const SKELETON_HEIGHT: f32 = 56.0;

pub(crate) fn list_view(
    frame: &Frame<'_>,
    list: &ListBlock,
    (window, cx): (&mut Window, &mut App),
) -> impl IntoElement {
    let header = list
        .title
        .as_deref()
        .map(|title| title_block(title, list.intro.as_deref(), cx));
    let body = list_body(frame, list, (window, cx));
    div().px(spacing::TIGHT).children(header).child(body)
}

fn list_body(
    frame: &Frame<'_>,
    list: &ListBlock,
    (window, cx): (&mut Window, &mut App),
) -> AnyElement {
    match frame.records.records(&list.source) {
        Records::Loading => div()
            .flex()
            .flex_col()
            .gap(spacing::COMFORTABLE)
            .child(note_text(&list.loading, cx))
            .child(Skeleton::new(
                "list-skeleton",
                px(SKELETON_WIDTH),
                px(SKELETON_HEIGHT),
            ))
            .into_any_element(),
        Records::Failed => failed(frame, list, cx).into_any_element(),
        Records::Ready(records) if records.is_empty() => {
            note_text(&list.empty, cx).into_any_element()
        },
        Records::Ready(records) => table(frame, (list, &records), (window, cx)).into_any_element(),
    }
}

fn failed(
    frame: &Frame<'_>,
    list: &ListBlock,
    cx: &App,
) -> impl IntoElement {
    div()
        .flex()
        .flex_col()
        .items_start()
        .gap(spacing::COMFORTABLE)
        .child(note_text(&list.failed, cx))
        .child(
            Button::new("list-retry", "Retry")
                .variant(ButtonVariant::Outline)
                .size(ButtonSize::Sm)
                .icon(IconName::ArrowCounterClockwise)
                .on_click(frame.sink.send(Intent::Retry(list.source.clone()))),
        )
}

fn table(
    frame: &Frame<'_>,
    source: (&ListBlock, &[settings::Record]),
    (window, cx): (&mut Window, &mut App),
) -> impl IntoElement {
    let (list, records) = source;
    let mut rows: Vec<AnyElement> = Vec::with_capacity(records.len());
    for (index, record) in records.iter().enumerate() {
        let row = record_view(
            frame,
            RecordRef::new(&list.source, record, index),
            (&mut *window, &mut *cx),
        );
        rows.push(row.into_any_element());
    }
    div()
        .flex()
        .flex_col()
        .child(flag_header(records, cx))
        .children(rows)
}

fn flag_header(
    records: &[settings::Record],
    cx: &App,
) -> impl IntoElement {
    let labels = records.first().map(|record| {
        record
            .flags
            .iter()
            .map(|flag| flag.label.clone())
            .collect::<Vec<_>>()
    });
    let cells: Vec<AnyElement> = labels
        .unwrap_or_default()
        .into_iter()
        .map(|label| {
            div()
                .w(px(FLAG_COLUMN))
                .flex()
                .justify_center()
                .child(note_text(&label, cx))
                .into_any_element()
        })
        .collect();
    div()
        .flex()
        .justify_end()
        .gap(spacing::ROOMY)
        .pr(spacing::TIGHT)
        .children(cells)
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;
    use settings::{AGENT_RECORDS, RecordCommand, SESSION_RECORDS};

    use crate::intent::Intent;
    use crate::test_support::{open, run};

    #[gpui::test]
    fn retry_asks_the_backend_to_refresh_the_failed_list(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx, "active-sessions");
        run(&harness, Intent::Retry(SESSION_RECORDS.to_owned()), cx);
        assert_eq!(
            harness.backend.commands(),
            [(
                SESSION_RECORDS.to_owned(),
                String::new(),
                RecordCommand::Refresh
            )]
        );
    }

    #[gpui::test]
    fn a_ready_list_and_its_sibling_states_render_on_one_page(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx, "providers");
        run(&harness, Intent::Retry(AGENT_RECORDS.to_owned()), cx);
        assert_eq!(harness.backend.commands().len(), 1);
    }
}
