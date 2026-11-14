//! A setting row: label, reset, description and control.

use gpui::{App, IntoElement, ParentElement, Styled, Window, div, px};
use ui::font::UI_FAMILY;
use ui::{Surface, Theme, UiText, hover_fill, radius};

use crate::control_view::control_view;
use crate::element_id::element_id;
use crate::frame::Frame;
use crate::reset_button::reset_button;
use crate::row::Row;

const LABEL_LINE: f32 = 20.0;
const TEXT_LINE: f32 = 18.0;
const WASH: f32 = 0.5;

pub(crate) fn row_view(
    frame: &Frame<'_>,
    row: &Row,
    (window, cx): (&mut Window, &mut App),
) -> impl IntoElement {
    let theme = *Theme::of(cx);
    let palette = theme.palette(Surface::Window);
    let text_size = UiText::Sm.size(&theme);
    let description = frame.state.description(row).map(|text| {
        div()
            .line_height(px(TEXT_LINE))
            .text_color(palette.muted_foreground())
            .child(text.to_owned())
    });
    let label = label_line(frame, row, cx);
    let base = div()
        .flex()
        .items_center()
        .justify_between()
        .gap(px(12.0))
        .rounded(radius::LG)
        .px(px(4.0))
        .py(px(8.0))
        .font_family(UI_FAMILY)
        .text_size(text_size);
    hover_fill(
        base,
        element_id("row", row.key.as_str()),
        palette.muted().opacity(WASH),
        (&mut *window, &mut *cx),
    )
    .child(div().flex_1().min_w_0().child(label).children(description))
    .child(
        div()
            .flex_none()
            .child(control_view(frame, row, (window, cx))),
    )
}

fn label_line(frame: &Frame<'_>, row: &Row, cx: &App) -> gpui::AnyElement {
    let palette = Theme::of(cx).palette(Surface::Window);
    div()
        .flex()
        .items_center()
        .gap(px(6.0))
        .h(px(LABEL_LINE))
        .child(
            div()
                .text_color(palette.foreground())
                .child(row.label.clone()),
        )
        .child(reset_button(frame, row, cx))
        .into_any_element()
}

#[cfg(test)]
#[path = "row_view_tests.rs"]
mod tests;
