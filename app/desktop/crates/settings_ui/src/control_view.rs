//! Draws the control of a row.

use gpui::{AnyElement, App, IntoElement, ParentElement, Styled, Window, div, px};
use ui::font::MONO_FAMILY;
use ui::{Surface, Switch, Theme, UiText};

use crate::button_control::button_control;
use crate::control::Control;
use crate::element_id::element_id;
use crate::field_control::field_control;
use crate::frame::Frame;
use crate::intent::Intent;
use crate::number_control::number_control;
use crate::row::Row;
use crate::select_control::select_control;

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
        Control::Readout { mono: true } => MONO_FAMILY,
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
        .text_size(UiText::Sm.size(&theme))
        .text_color(theme.palette(Surface::Window).muted_foreground())
        .child(frame.state.field_text(row))
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use crate::catalog::default_schema;
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
