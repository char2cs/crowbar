//! A select with an optional info icon.

use gpui::prelude::FluentBuilder as _;
use gpui::{InteractiveElement as _, IntoElement, ParentElement, SharedString, Styled, div, px};
use settings::{Control, Row};
use ui::{Icon, IconName, Select, Tooltip};

use crate::frame::Frame;
use crate::frame::RowLayout;
use crate::frame::element_id;
use crate::frame::spacing;
use crate::intent::Intent;

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
    } = &row.control()
    else {
        return div();
    };
    let labels: Vec<SharedString> = frame
        .state
        .choices(row)
        .into_iter()
        .map(|choice| SharedString::from(choice.label().to_owned()))
        .collect();
    let sink = frame.sink.clone();
    let target = row.clone();
    let select = Select::new(
        element_id("select", row.key().as_str()),
        labels,
        frame.state.selected_index(row),
    )
    .placeholder(placeholder.clone().unwrap_or_default())
    .width(px(width.pixels()))
    .compact()
    .when(frame.rows == RowLayout::Stacked, Select::fill)
    .on_change(move |index, window, cx| {
        sink.deliver(Intent::Choose(target.clone(), index), (window, cx));
    });
    let key = row.key().as_str().to_owned();
    div()
        .debug_selector(move || format!("row-select-{key}"))
        .flex()
        .items_center()
        .gap(spacing::COMFORTABLE)
        .when(frame.rows == RowLayout::Stacked, Styled::w_full)
        .child(div().flex_1().min_w_0().child(select))
        .children(hint.as_ref().map(|text| info_icon(row, text)))
}

fn info_icon(
    row: &Row,
    text: &str,
) -> impl IntoElement {
    let id = element_id("hint", row.key().as_str());
    Tooltip::new(
        id,
        text.to_owned(),
        Icon::new(IconName::Info).size(px(INFO_ICON)),
    )
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;
    use settings::{Key, Read as _, Value};

    use crate::intent::Intent;
    use crate::test_support::{inspect, open, row, run};

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
