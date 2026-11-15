//! A stepper: minus, a typed field and plus.

use gpui::prelude::FluentBuilder as _;
use gpui::{IntoElement, ParentElement, Styled, div, px};
use ui::{Button, IconName};

use crate::control::Control;
use crate::element_id::element_id;
use crate::frame::Frame;
use crate::intent::Intent;
use crate::nudge::Nudge;
use crate::row::Row;
use crate::spacing;

pub(crate) fn number_control(
    frame: &Frame<'_>,
    row: &Row,
) -> impl IntoElement {
    let Control::Number { range, width } = &row.control else {
        return div();
    };
    let typed = range.parse(&frame.state.field_text(row));
    let at_min = typed.is_some_and(|value| value <= range.min);
    let at_max = typed.is_some_and(|value| value >= range.max);
    let step = |name: &str, icon: IconName, nudge: Nudge, blocked: bool| {
        Button::icon_only(element_id(name, row.key.as_str()), icon)
            .on_click(frame.sink.send(Intent::Nudge(row.clone(), nudge)))
            .when(blocked, Button::disabled)
    };
    div()
        .flex()
        .items_center()
        .gap(spacing::TIGHT)
        .w(px(width.pixels()))
        .child(step("minus", IconName::Minus, Nudge::Down, at_min))
        .child(
            div()
                .flex_1()
                .min_w_0()
                .children(frame.fields.get(&row.key).cloned()),
        )
        .child(step("plus", IconName::Plus, Nudge::Up, at_max))
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use crate::intent::Intent;
    use crate::key::Key;
    use crate::nudge::Nudge;
    use crate::settings_backend::SettingsBackend;
    use crate::test_support::{inspect, open, row, run};
    use crate::value::Value;

    #[gpui::test]
    fn the_buttons_step_by_the_range_step_and_stop_at_its_ends(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx, "appearance");
        let target = row("uiFontSize");
        run(&harness, Intent::Nudge(target.clone(), Nudge::Up), cx);
        assert_eq!(
            harness.backend.get(&Key::new("uiFontSize")),
            Some(Value::Number(15.5))
        );
        harness
            .backend
            .publish(&Key::new("uiFontSize"), Value::Number(24.0));
        cx.run_until_parked();
        run(&harness, Intent::Nudge(target.clone(), Nudge::Up), cx);
        assert_eq!(
            harness.backend.get(&Key::new("uiFontSize")),
            Some(Value::Number(24.0))
        );
        assert_eq!(
            inspect(&harness, cx, |modal| modal.state.field_text(&target)),
            "24"
        );
    }
}
