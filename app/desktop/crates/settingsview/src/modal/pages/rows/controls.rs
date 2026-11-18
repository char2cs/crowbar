//! Draws the control of a row.

mod button;
mod field;
mod number;
mod select;

use gpui::{AnyElement, App, IntoElement, ParentElement, Styled, Window, div, px};
use settings::{Control, Row};
use ui::{ScaledText, Surface, Switch, Theme};

use crate::frame::Frame;
use crate::frame::element_id;
use crate::intent::Intent;
use crate::modal::pages::rows::controls::button::button_control;
use crate::modal::pages::rows::controls::field::field_control;
use crate::modal::pages::rows::controls::number::number_control;
use crate::modal::pages::rows::controls::select::select_control;

const READOUT_WIDTH: f32 = 320.0;

pub(crate) fn control_view(
    frame: &Frame<'_>,
    row: &Row,
    (window, cx): (&mut Window, &mut App),
) -> AnyElement {
    match &row.control {
        Control::Switch { .. } => switch_control(frame, row).into_any_element(),
        Control::Select { .. } => select_control(frame, row).into_any_element(),
        Control::Number { .. } => number_control(frame, row).into_any_element(),
        Control::Text { .. } | Control::TextList { .. } => {
            field_control(frame, row).into_any_element()
        },
        Control::Button(spec) => button_control(frame, row, spec).into_any_element(),
        Control::Readout { .. } => readout_control(frame, row, (window, cx)).into_any_element(),
    }
}

fn switch_control(
    frame: &Frame<'_>,
    row: &Row,
) -> impl IntoElement {
    let sink = frame.sink.clone();
    let target = row.clone();
    Switch::new(
        element_id("switch", row.key.as_str()),
        frame.state.switch_on(row),
    )
    .on_change(move |_, window, cx| {
        sink.deliver(Intent::Toggle(target.clone()), (window, cx));
    })
}

fn readout_family(
    control: &Control,
    theme: &Theme,
) -> &'static str {
    match control {
        Control::Readout { mono: true } => theme.mono_family(),
        _ => theme.ui_family(),
    }
}

fn readout_control(
    frame: &Frame<'_>,
    row: &Row,
    (_, cx): (&mut Window, &mut App),
) -> impl IntoElement {
    let theme = *Theme::of(cx);
    div()
        .max_w(px(READOUT_WIDTH))
        .font_family(readout_family(&row.control, &theme))
        .text_size(ScaledText::Sm.size(&theme))
        .text_color(theme.palette(Surface::Window).muted_foreground())
        .child(frame.state.field_text(row))
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;
    use settings::default_schema;

    use crate::intent::Intent;
    use crate::test_support::{inspect, open, run};

    #[gpui::test]
    fn every_control_kind_of_the_catalog_renders_without_a_notice(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx, "appearance");
        let ids: Vec<_> = default_schema()
            .unwrap()
            .pages()
            .iter()
            .map(|page| page.id.clone())
            .collect();
        for id in ids {
            run(&harness, Intent::Select(id), cx);
        }
        assert!(inspect(&harness, cx, |modal| modal.notice.is_none()));
    }
}
