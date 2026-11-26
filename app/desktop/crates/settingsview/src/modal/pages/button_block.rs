//! A button between blocks, such as Add host.

use gpui::prelude::FluentBuilder as _;
use gpui::{
    App, InteractiveElement as _, IntoElement, ParentElement, StatefulInteractiveElement as _,
    Styled, div,
};
use settings::{Action, ButtonFace, ButtonSpec};
use ui::{Button, ButtonSize, ButtonVariant, ScaledText, Surface, Theme, radius};

use crate::frame::Frame;
use crate::frame::element_id;
use crate::frame::icon_named;
use crate::frame::spacing;
use crate::intent::Intent;

pub(crate) fn button_block(
    frame: &Frame<'_>,
    spec: &ButtonSpec,
    cx: &App,
) -> Option<impl IntoElement> {
    if matches!(spec.action(), Action::ResetOverrides(_)) && !frame.state.has_custom_chords() {
        return None;
    }
    let button = match spec.face() {
        ButtonFace::Label => label_button(frame, spec, cx).into_any_element(),
        _ => outline_button(frame, spec).into_any_element(),
    };
    let label = spec.label().to_owned();
    let note = spec.note().map(|text| help_line(text, &label, cx));
    Some(
        div()
            .px(spacing::TIGHT)
            .flex()
            .flex_col()
            .items_start()
            .gap(spacing::COMFORTABLE)
            .child(
                div()
                    .debug_selector(move || format!("button-{label}"))
                    .child(button),
            )
            .children(note),
    )
}

fn outline_button(
    frame: &Frame<'_>,
    spec: &ButtonSpec,
) -> Button {
    let button = Button::new(element_id("block", spec.label()), spec.label().to_owned())
        .variant(ButtonVariant::Outline)
        .size(ButtonSize::Sm)
        .on_click(frame.sink.send(Intent::Button(spec.clone())));
    let button = match spec.icon_stem().and_then(icon_named) {
        Some(icon) => button.icon(icon),
        None => button,
    };
    button.when(!frame.state.button_enabled(spec), Button::disabled)
}

fn label_button(
    frame: &Frame<'_>,
    spec: &ButtonSpec,
    cx: &App,
) -> impl IntoElement {
    let theme = Theme::of(cx);
    let palette = theme.palette(Surface::Window);
    div()
        .id(element_id("block", spec.label()))
        .flex()
        .items_center()
        .px(spacing::ROOMY)
        .py(spacing::COMFORTABLE)
        .rounded(radius::MD)
        .border_1()
        .border_color(palette.border())
        .cursor_pointer()
        .font_family(theme.ui_family())
        .text_size(ScaledText::Sm.size(theme))
        .text_color(palette.foreground())
        .on_click(frame.sink.send(Intent::Button(spec.clone())))
        .child(spec.label().to_owned())
}

fn help_line(
    text: &str,
    label: &str,
    cx: &App,
) -> gpui::Div {
    let theme = Theme::of(cx);
    let selector = format!("button-note-{label}");
    div()
        .debug_selector(move || selector)
        .font_family(theme.ui_family())
        .text_size(ScaledText::Xs.size(theme))
        .text_color(theme.palette(Surface::Window).muted_foreground())
        .child(text.to_owned())
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;
    use settings::{Block, default_schema};

    use crate::intent::Intent;
    use crate::test_support::{open, run};

    #[gpui::test]
    fn the_add_host_button_runs_its_backend_action(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx, "hosts");
        let schema = default_schema().unwrap();
        let spec = schema
            .pages()
            .iter()
            .flat_map(|page| page.blocks().iter())
            .find_map(|block| match block {
                Block::Button(spec) if spec.label() == "Add host" => Some(spec.clone()),
                _ => None,
            })
            .unwrap();
        run(&harness, Intent::Button(spec), cx);
        assert_eq!(harness.backend.invoked(), ["hosts.add"]);
    }

    #[gpui::test]
    fn the_upload_stands_alone_as_tall_as_develops_label_with_its_help_under_it(
        cx: &mut TestAppContext
    ) {
        const LABEL_MIN_HEIGHT: f32 = 34.0;
        let (_harness, cx) = open(cx, "providers");
        let button = cx.debug_bounds("button-Upload agent descriptor").unwrap();
        let help = cx.debug_bounds("button-note-Upload agent descriptor");
        assert!(
            help.is_some_and(|help| help.top() >= button.bottom()),
            "button {button:?} help {help:?}"
        );
        assert!(button.size.height >= gpui::px(LABEL_MIN_HEIGHT), "{button:?}");
    }

}
