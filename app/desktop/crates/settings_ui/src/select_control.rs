//! A select with an optional info icon.

use gpui::{IntoElement, ParentElement, SharedString, Styled, div, px};
use ui::{Icon, IconName, Select, Tooltip};

use crate::control::Control;
use crate::dispatch::deliver;
use crate::element_id::element_id;
use crate::frame::Frame;
use crate::intent::Intent;
use crate::row::Row;

const INFO_ICON: f32 = 16.0;

pub(crate) fn select_control(frame: &Frame<'_>, row: &Row) -> impl IntoElement {
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
    let owner = frame.owner.clone();
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
        deliver(&owner, Intent::Choose(target.clone(), index), (window, cx));
    });
    div()
        .flex()
        .items_center()
        .gap(px(8.0))
        .child(select)
        .children(hint.as_ref().map(|text| info_icon(row, text)))
}

fn info_icon(row: &Row, text: &str) -> impl IntoElement {
    let id = element_id("hint", row.key.as_str());
    Tooltip::new(
        id,
        text.to_owned(),
        Icon::new(IconName::Info).size(px(INFO_ICON)),
    )
}

#[cfg(test)]
#[path = "select_control_tests.rs"]
mod tests;
