//! A button inside a row.

use gpui::IntoElement;
use gpui::prelude::FluentBuilder as _;
use settings::{ButtonSpec, Row};
use ui::{Button, ButtonSize, ButtonVariant};

use crate::frame::Frame;
use crate::frame::element_id;
use crate::frame::icon_named;
use crate::intent::Intent;

pub(crate) fn button_control(
    frame: &Frame<'_>,
    row: &Row,
    spec: &ButtonSpec,
) -> impl IntoElement {
    let button = Button::new(
        element_id("button", row.key().as_str()),
        spec.label().to_owned(),
    )
    .variant(ButtonVariant::Outline)
    .size(ButtonSize::Sm)
    .on_click(frame.sink.send(Intent::Button(spec.clone())));
    let button = match spec.icon_stem().and_then(icon_named) {
        Some(icon) => button.icon(icon),
        None => button,
    };
    button.when(!frame.state.button_enabled(spec), Button::disabled)
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;
    use settings::Control;

    use crate::test_support::{inspect, open, row};

    #[gpui::test]
    fn the_row_button_is_enabled_when_the_backend_offers_its_capability(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx, "browser");
        let clear = row("browserClearData");
        let Control::Button(spec) = clear.control() else {
            panic!("browserClearData is a button row");
        };
        assert!(inspect(&harness, cx, |modal| modal
            .state
            .button_enabled(spec)));
    }
}
