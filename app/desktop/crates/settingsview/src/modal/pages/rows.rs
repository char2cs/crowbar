//! A setting row: label, reset, description and control.

mod controls;
mod reset_button;

use gpui::prelude::FluentBuilder as _;
use gpui::{
    App, FocusHandle, InteractiveElement as _, IntoElement, ParentElement,
    StatefulInteractiveElement as _, Styled, Window, div, px,
};
use settings::{LabelStyle, Row};
use ui::{HoverFill, ScaledText, Surface, Theme, radius};

use crate::frame::Frame;
use crate::frame::RowLayout;
use crate::frame::element_id;
use crate::frame::spacing;
use crate::intent::Intent;
use crate::modal::pages::rows::controls::control_view;
use crate::modal::pages::rows::reset_button::reset_button;

const LABEL_LINE: f32 = 20.0;
const TEXT_LEADING: f32 = 1.5;
const WASH: f32 = 0.5;
// SPEC: app/panes/src/components/ui/label.tsx: `sm:text-sm/4` outranks `ui-text-sm`; 14px at any UI size.
const LABEL_TEXT: (f32, f32) = (14.0, 16.0);
const STACKED_LABEL_TEXT: (f32, f32) = (16.0, 18.0);

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
    let wash = palette.muted().opacity(WASH);
    let within = control_focus(row, window, cx).contains_focused(window, cx);
    HoverFill::new(element_id("row", row.key().as_str()), wash)
        .apply(base, (&mut *window, &mut *cx))
        .when(within, |tinted| {
            let key = row.key().as_str().to_owned();
            tinted
                .bg(wash)
                .debug_selector(move || format!("row-within-{key}"))
        })
        .on_click(frame.sink.send(Intent::Activate(row.clone())))
        .child(text_column(frame, row, cx))
        .child(control_column(frame, row, (window, cx)))
}

fn control_column(
    frame: &Frame<'_>,
    row: &Row,
    (window, cx): (&mut Window, &mut App),
) -> impl IntoElement {
    let key = row.key().as_str().to_owned();
    div()
        .id(element_id("control", row.key().as_str()))
        .track_focus(&control_focus(row, window, cx))
        .debug_selector(move || format!("row-control-{key}"))
        .flex_none()
        .when(frame.rows == RowLayout::Stacked, Styled::w_full)
        .on_click(|_, _, cx| cx.stop_propagation())
        .child(control_view(frame, row, (window, cx)))
}

fn control_focus(
    row: &Row,
    window: &mut Window,
    cx: &mut App,
) -> FocusHandle {
    let key = element_id("within", row.key().as_str());
    let focus = window.use_keyed_state(key, cx, |_, cx| cx.focus_handle());
    focus.read(cx).clone()
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
            .line_height(gpui::relative(TEXT_LEADING))
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
    let key = row.key().as_str().to_owned();
    div()
        .flex()
        .items_center()
        .gap(spacing::SNUG)
        .h(px(LABEL_LINE))
        .child(
            label_text(frame.rows, row.label_style(), cx)
                .debug_selector(move || format!("row-label-{key}"))
                .child(row.label().to_owned()),
        )
        .child(reset_button(frame, row, cx))
        .into_any_element()
}

fn label_text(
    rows: RowLayout,
    style: LabelStyle,
    cx: &App,
) -> gpui::Div {
    let palette = Theme::of(cx).palette(Surface::Window);
    let (size, line) = match (style, rows) {
        (LabelStyle::Caption, _) => {
            return div()
                .line_height(gpui::relative(TEXT_LEADING))
                .text_color(palette.muted_foreground());
        },
        (LabelStyle::Label, RowLayout::Inline) => LABEL_TEXT,
        (LabelStyle::Label, RowLayout::Stacked) => STACKED_LABEL_TEXT,
    };
    div()
        .text_size(px(size))
        .line_height(px(line))
        .text_color(palette.foreground())
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
    fn a_label_keeps_its_14px_text_while_the_ui_font_size_grows(cx: &mut TestAppContext) {
        let (_, cx) = open(cx, "appearance");
        let before = cx.debug_bounds("row-label-themeMode").unwrap_or_default();
        let text = cx.debug_bounds("row-text-themeMode").unwrap_or_default();
        cx.update(|_, cx| ui::Theme::of(cx).with_ui_font_size(24.0).install(cx));
        cx.run_until_parked();
        let after = cx.debug_bounds("row-label-themeMode").unwrap_or_default();
        let grown = cx.debug_bounds("row-text-themeMode").unwrap_or_default();
        assert_eq!(before.size.height, gpui::px(16.0));
        assert_eq!(after.size, before.size);
        assert!(grown.size.height > text.size.height, "{text:?} {grown:?}");
    }

    #[gpui::test]
    fn the_keymap_preset_is_named_by_a_caption_that_scales_with_the_ui(cx: &mut TestAppContext) {
        let (_, cx) = open(cx, "keybindings");
        cx.update(|_, cx| ui::Theme::of(cx).with_ui_font_size(24.0).install(cx));
        cx.run_until_parked();
        let caption = cx
            .debug_bounds("row-label-keybindingPreset")
            .unwrap_or_default();
        assert_eq!(
            caption.size.height.round(),
            gpui::px(29.0),
            "12px x 1.6 on 1.5 leading"
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
    fn a_row_keeps_its_wash_while_its_select_holds_focus_after_a_pick(cx: &mut TestAppContext) {
        let (harness, mut owner, mut top) = crate::test_support::open_floating(cx, "appearance");
        harness
            .modal
            .update_in(&mut owner, |modal, window, cx| modal.focus(window, cx));
        owner.run_until_parked();
        assert!(top.debug_bounds("row-within-themeMode").is_none());
        click_description(&mut top, "row-select-themeMode");
        click_description(&mut top, "menu-row-1");
        owner.run_until_parked();
        assert!(top.debug_bounds("row-within-themeMode").is_some());
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
