//! A select with an optional info icon.

use gpui::{IntoElement, ParentElement, SharedString, Styled, div, px};
use ui::{Icon, IconName, Select, Tooltip};

use crate::control::Control;
use crate::element_id::element_id;
use crate::frame::Frame;
use crate::intent::Intent;
use crate::row::Row;
use crate::spacing;

const INFO_ICON: f32 = 16.0;

pub(crate) fn select_control(
    frame: &Frame<'_>,
    row: &Row,
) -> impl IntoElement {
    let Control::Select {
        width,
        placeholder,
        hint,
        ..
    } = &row.control
    else {
        return div();
    };
    let labels: Vec<SharedString> = frame
        .state
        .choices(row)
        .into_iter()
        .map(|choice| SharedString::from(choice.label))
        .collect();
    let sink = frame.sink.clone();
    let target = row.clone();
    let select = Select::new(
        element_id("select", row.key.as_str()),
        labels,
        frame.state.selected_index(row),
    )
    .placeholder(placeholder.clone().unwrap_or_default())
    .width(px(width.pixels()))
    .compact()
    .on_change(move |index, window, cx| {
        sink.deliver(Intent::Choose(target.clone(), index), (window, cx));
    });
    div()
        .flex()
        .items_center()
        .gap(spacing::COMFORTABLE)
        .child(select)
        .children(hint.as_ref().map(|text| info_icon(row, text)))
}

fn info_icon(
    row: &Row,
    text: &str,
) -> impl IntoElement {
    let id = element_id("hint", row.key.as_str());
    Tooltip::new(
        id,
        text.to_owned(),
        Icon::new(IconName::Info).size(px(INFO_ICON)),
    )
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use crate::intent::Intent;
    use crate::key::Key;
    use crate::settings_read::SettingsRead as _;
    use crate::test_support::{inspect, open, row, run};
    use crate::value::Value;

    #[gpui::test]
    fn choosing_an_option_writes_its_value(cx: &mut TestAppContext) {
        let (harness, cx) = open(cx, "appearance");
        let target = row("themeMode");
        assert_eq!(
            inspect(&harness, cx, |modal| modal.state.selected_index(&target)),
            Some(0)
        );
        run(&harness, Intent::Choose(target.clone(), 2), cx);
        assert_eq!(
            harness.backend.get(&Key::new("themeMode")),
            Some(Value::Text("dark".to_owned()))
        );
        assert_eq!(
            inspect(&harness, cx, |modal| modal.state.selected_index(&target)),
            Some(2)
        );
    }
}
