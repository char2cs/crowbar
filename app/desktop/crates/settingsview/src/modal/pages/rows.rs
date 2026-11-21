//! A setting row: label, reset, description and control.

mod controls;
mod reset_button;

use gpui::prelude::FluentBuilder as _;
use gpui::{
    App, InteractiveElement as _, IntoElement, ParentElement, StatefulInteractiveElement as _,
    Styled, Window, div, px,
};
use settings::Row;
use ui::{HoverFill, ScaledText, Surface, Theme, radius};

use crate::frame::Frame;
use crate::frame::RowLayout;
use crate::frame::element_id;
use crate::frame::spacing;
use crate::intent::Intent;
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
    let text_size = ScaledText::Sm.size(&theme);
    let base = div()
        .flex()
        .items_center()
        .justify_between()
        .gap(spacing::ROOMY)
        .rounded(radius::LG)
        .px(spacing::TIGHT)
        .py(spacing::COMFORTABLE)
        .font_family(theme.ui_family())
        .text_size(text_size)
        .when(frame.rows == RowLayout::Stacked, stack);
    HoverFill::new(
        element_id("row", row.key().as_str()),
        palette.muted().opacity(WASH),
    )
    .apply(base, (&mut *window, &mut *cx))
    .on_click(frame.sink.send(Intent::Activate(row.clone())))
    .child(text_column(frame, row, cx))
    .child(
        div()
            .id(element_id("control", row.key().as_str()))
            .debug_selector({
                let key = row.key().as_str().to_owned();
                move || format!("row-control-{key}")
            })
            .flex_none()
            .when(frame.rows == RowLayout::Stacked, Styled::w_full)
            .on_click(|_, _, cx| cx.stop_propagation())
            .child(control_view(frame, row, (window, cx))),
    )
}

fn stack(mut base: gpui::Div) -> gpui::Div {
    base.style().align_items = Some(gpui::AlignItems::Stretch);
    base.flex_col().gap(spacing::COMFORTABLE)
}

fn text_column(
    frame: &Frame<'_>,
    row: &Row,
    cx: &App,
) -> impl IntoElement {
    let palette = Theme::of(cx).palette(Surface::Window);
    let description = frame.state.description(row).map(|text| {
        div()
            .line_height(px(TEXT_LINE))
            .text_color(palette.muted_foreground())
            .child(text.to_owned())
    });
    let key = row.key().as_str().to_owned();
    div()
        .flex_1()
        .min_w_0()
        .debug_selector(move || format!("row-text-{key}"))
        .child(label_line(frame, row, cx))
        .children(description)
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
                .child(row.label().to_owned()),
        )
        .child(reset_button(frame, row, cx))
        .into_any_element()
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use settings::Read as _;

    use crate::intent::Intent;
    use crate::test_support::{inspect, open, row, run};

    #[gpui::test]
    fn under_640px_a_row_puts_its_control_under_its_text_at_full_width(cx: &mut TestAppContext) {
        let (_, cx) = open(cx, "appearance");
        cx.simulate_resize(gpui::size(gpui::px(600.0), gpui::px(480.0)));
        cx.run_until_parked();
        let text = cx.debug_bounds("row-text-themeMode").unwrap();
        let control = cx.debug_bounds("row-select-themeMode").unwrap();
        assert!(control.top() >= text.bottom(), "{text:?} {control:?}");
        assert_eq!(control.size.width, text.size.width);
        let search = cx.debug_bounds("settings-search").unwrap();
        let base = (control.size.height, search.size.height);
        assert_eq!(
            base,
            (gpui::px(32.0), gpui::px(32.0)),
            "controls at base size"
        );
    }

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

    fn click_description(
        cx: &mut gpui::VisualTestContext,
        selector: &'static str,
    ) {
        let text = cx.debug_bounds(selector).unwrap();
        cx.simulate_click(text.center(), gpui::Modifiers::none());
        cx.run_until_parked();
    }

    #[gpui::test]
    fn a_click_on_the_row_text_toggles_its_switch(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx, "editor");
        click_description(cx, "row-text-wordWrap");
        assert_eq!(
            harness.backend.get(&settings::Key::new("wordWrap")),
            Some(settings::Value::Bool(true))
        );
    }

    #[gpui::test]
    fn a_click_on_a_number_row_focuses_its_field(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx, "editor");
        click_description(cx, "row-text-fontSize");
        let focused = harness.modal.update_in(cx, |modal, window, cx| {
            let input = modal.fields.get(&settings::Key::new("fontSize")).cloned();
            input.is_some_and(|input| input.focus_handle(cx).is_focused(window))
        });
        assert!(focused);
    }
}
