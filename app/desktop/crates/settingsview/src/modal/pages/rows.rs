//! A setting row: label, reset, description and control.

mod controls;
mod reset_button;

use gpui::{App, IntoElement, ParentElement, Styled, Window, div, px};
use settings::Row;
use ui::{HoverFill, Surface, Theme, UiText, radius};

use crate::frame::Frame;
use crate::frame::element_id;
use crate::frame::spacing;
use crate::modal::pages::rows::controls::control_view;
use crate::modal::pages::rows::reset_button::reset_button;

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
        .gap(spacing::ROOMY)
        .rounded(radius::LG)
        .px(spacing::TIGHT)
        .py(spacing::COMFORTABLE)
        .font_family(theme.ui_family())
        .text_size(text_size);
    HoverFill::new(
        element_id("row", row.key.as_str()),
        palette.muted().opacity(WASH),
    )
    .apply(base, (&mut *window, &mut *cx))
    .child(div().flex_1().min_w_0().child(label).children(description))
    .child(
        div()
            .flex_none()
            .child(control_view(frame, row, (window, cx))),
    )
}

fn label_line(
    frame: &Frame<'_>,
    row: &Row,
    cx: &App,
) -> gpui::AnyElement {
    let palette = Theme::of(cx).palette(Surface::Window);
    div()
        .flex()
        .items_center()
        .gap(spacing::SNUG)
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
mod tests {
    use gpui::TestAppContext;

    use crate::intent::Intent;
    use crate::test_support::{inspect, open, row, run};

    #[gpui::test]
    fn an_invalid_row_swaps_its_description_for_the_invalid_text(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx, "browser");
        let target = row("browserCustomSearchUrl");
        let normal = inspect(&harness, cx, |modal| {
            modal.state.description(&target).map(str::to_owned)
        });
        let typed = ui::TextInputEvent::Changed("nope".to_owned());
        run(&harness, Intent::Edit(target.clone(), typed), cx);
        let invalid = inspect(&harness, cx, |modal| {
            modal.state.description(&target).map(str::to_owned)
        });
        assert_ne!(normal, invalid);
    }
}
