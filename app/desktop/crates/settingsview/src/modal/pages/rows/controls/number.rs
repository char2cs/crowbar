//! A stepper: minus, a typed field and plus.

use gpui::prelude::FluentBuilder as _;
use gpui::{IntoElement, ParentElement, Styled, div, px};
use settings::{Control, Nudge, Row};
use ui::{Button, IconName};

use crate::frame::Frame;
use crate::frame::element_id;
use crate::frame::spacing;
use crate::intent::Intent;

pub(crate) fn number_control(
    frame: &Frame<'_>,
    row: &Row,
) -> impl IntoElement {
    let Control::Number { range, width } = &row.control() else {
        return div();
    };
    let typed = range.parse(&frame.state.field_text(row));
    let at_min = typed.is_some_and(|value| value <= range.min());
    let at_max = typed.is_some_and(|value| value >= range.max());
    let step = |name: &str, icon: IconName, nudge: Nudge| {
        Button::icon_only(element_id(name, row.key().as_str()), icon)
            .on_click(frame.sink.send(Intent::Nudge(row.clone(), nudge)))
    };
    div()
        .flex()
        .items_center()
        .gap(spacing::TIGHT)
        .w(px(width.pixels()))
        .child(step("minus", IconName::Minus, Nudge::Down).when(at_min, Button::disabled))
        .child(
            div()
                .flex_1()
                .min_w_0()
                .children(frame.fields.get(row.key()).cloned()),
        )
        .child(step("plus", IconName::Plus, Nudge::Up).when(at_max, Button::disabled))
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;
    use settings::{Key, Nudge, Read as _, Value};

    use crate::intent::Intent;
    use crate::test_support::{inspect, open, row, run};

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
