//! A list of backend records with its loading, failed and empty states.

use gpui::{AnyElement, App, IntoElement, ParentElement, Styled, Window, div, px};
use ui::{Button, ButtonSize, ButtonVariant, IconName, Skeleton};

use crate::dispatch::send;
use crate::frame::Frame;
use crate::intent::Intent;
use crate::list_block::ListBlock;
use crate::note_text::note_text;
use crate::record_view::{FLAG_COLUMN, record_view};
use crate::records::Records;
use crate::title_block::title_block;

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
    div().px(px(4.0)).children(header).child(body)
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
            .gap(px(8.0))
            .child(note_text(&list.loading, cx))
            .child(Skeleton::new("list-skeleton", px(320.0), px(56.0)))
            .into_any_element(),
        Records::Failed => failed(frame, list, cx).into_any_element(),
        Records::Ready(records) if records.is_empty() => {
            note_text(&list.empty, cx).into_any_element()
        },
        Records::Ready(records) => table(frame, (list, &records), (window, cx)).into_any_element(),
    }
}

fn failed(frame: &Frame<'_>, list: &ListBlock, cx: &App) -> impl IntoElement {
    div()
        .flex()
        .flex_col()
        .items_start()
        .gap(px(8.0))
        .child(note_text(&list.failed, cx))
        .child(
            Button::new("list-retry", "Retry")
                .variant(ButtonVariant::Outline)
                .size(ButtonSize::Sm)
                .icon(IconName::ArrowCounterClockwise)
                .on_click(send(frame.owner, Intent::Retry(list.source.clone()))),
        )
}

fn table(
    frame: &Frame<'_>,
    (list, records): (&ListBlock, &[crate::record::Record]),
    (window, cx): (&mut Window, &mut App),
) -> impl IntoElement {
    let count = records.len();
    let mut rows: Vec<AnyElement> = Vec::with_capacity(count);
    for (index, record) in records.iter().enumerate() {
        let row = record_view(
            frame,
            (&list.source, record, (index, count)),
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

const MOVER_WIDTH: f32 = 76.0;

fn flag_header(records: &[crate::record::Record], cx: &App) -> impl IntoElement {
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
    let movable = records.first().is_some_and(|record| record.movable);
    let tail = if movable { MOVER_WIDTH } else { 4.0 };
    div()
        .flex()
        .justify_end()
        .gap(px(12.0))
        .pr(px(tail))
        .children(cells)
}

#[cfg(test)]
#[path = "list_view_tests.rs"]
mod tests;
