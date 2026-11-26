//! A list of backend records with its loading, failed and empty states.

mod drag_record;
mod hover;
mod listed_record;
mod marks;
mod note_text;
mod sortable;

pub(crate) use note_text::note_text;
mod tone_color;
mod view;

use gpui::prelude::FluentBuilder as _;
use gpui::{
    AnyElement, App, FontWeight, InteractiveElement as _, IntoElement, ParentElement, Styled,
    Window, div, px,
};
use settings::{ListBlock, Records};
use ui::{Button, ButtonSize, ButtonVariant, ScaledText, Surface, Theme};

use crate::frame::Frame;
use crate::frame::spacing;
use crate::intent::Intent;
use crate::modal::pages::records::listed_record::ListedRecord;
use crate::modal::pages::records::marks::FLAG_COLUMN;
use crate::modal::pages::records::view::record_view;

pub(crate) fn list_view(
    frame: &Frame<'_>,
    list: &ListBlock,
    (window, cx): (&mut Window, &mut App),
) -> impl IntoElement {
    let header = list
        .title()
        .map(|title| list_title(list.source(), title, cx));
    let body = list_body(frame, list, (window, cx));
    div()
        .px(spacing::TIGHT)
        .flex()
        .flex_col()
        .gap(spacing::COMFORTABLE)
        .when(header.is_some(), |list| list.pt(spacing::ROOMY))
        .children(header)
        .child(body)
}

fn list_title(
    source: &str,
    title: &str,
    cx: &App,
) -> gpui::Div {
    let theme = Theme::of(cx);
    let selector = format!("list-title-{source}");
    div()
        .debug_selector(move || selector)
        .font_family(theme.ui_family())
        .text_size(ScaledText::Sm.size(theme))
        .font_weight(FontWeight::MEDIUM)
        .text_color(theme.palette(Surface::Window).foreground())
        .child(title.to_owned())
}

fn list_body(
    frame: &Frame<'_>,
    list: &ListBlock,
    (window, cx): (&mut Window, &mut App),
) -> AnyElement {
    match frame.records.records(list.source()) {
        Records::Loading => note_text(list.loading(), cx).into_any_element(),
        Records::Failed => failed(frame, list, cx).into_any_element(),
        Records::Ready(records) if records.is_empty() => {
            note_text(list.empty(), cx).into_any_element()
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
        .items_center()
        .gap(spacing::COMFORTABLE)
        .child(note_text(list.failed(), cx))
        .child(
            Button::new("list-retry", "Retry")
                .variant(ButtonVariant::Outline)
                .size(ButtonSize::Xs)
                .on_click(frame.sink.send(Intent::Retry(list.source().to_owned()))),
        )
}

fn table(
    frame: &Frame<'_>,
    source: (&ListBlock, &[settings::Record]),
    (window, cx): (&mut Window, &mut App),
) -> impl IntoElement {
    let (list, records) = source;
    let mut rows: Vec<AnyElement> = Vec::with_capacity(records.len());
    let order = crate::lift::arranged(frame.lift, list.source(), records);
    for (index, record) in order.into_iter().enumerate() {
        let row = record_view(
            frame,
            ListedRecord::new(list.source(), record, index),
            (&mut *window, &mut *cx),
        );
        rows.push(row.into_any_element());
    }
    let intro = list.intro_text().map(|text| {
        let selector = format!("list-intro-{}", list.source());
        div()
            .debug_selector(move || selector)
            .pb(spacing::TIGHT)
            .child(note_text(text, cx))
    });
    div()
        .flex()
        .flex_col()
        .children(intro)
        .child(flag_header(records, cx).debug_selector({
            let selector = format!("list-columns-{}", list.source());
            move || selector
        }))
        .children(rows)
}

fn flag_header(
    records: &[settings::Record],
    cx: &App,
) -> gpui::Div {
    let labels = records.first().map(|record| {
        record
            .flags()
            .iter()
            .map(settings::RecordFlag::label)
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
                .child(note_text(label, cx))
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
    fn the_agents_list_opens_with_its_hint_and_descriptor_checks_has_a_small_label(
        cx: &mut TestAppContext
    ) {
        const LABEL_MAX_HEIGHT: f32 = 24.0;
        let (_harness, cx) = open(cx, "providers");
        let hint = cx.debug_bounds("list-intro-agents");
        let header = cx.debug_bounds("list-columns-agents").unwrap();
        assert!(
            hint.is_some_and(|hint| hint.bottom() <= header.top()),
            "hint {hint:?} over the columns {header:?}"
        );
        let label = cx.debug_bounds("list-title-descriptors");
        assert!(
            label.is_some_and(|label| label.size.height < gpui::px(LABEL_MAX_HEIGHT)),
            "{label:?}"
        );
    }

}
