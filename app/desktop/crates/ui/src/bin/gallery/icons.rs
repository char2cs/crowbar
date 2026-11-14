//! Every embedded icon in every embedded weight.

use gpui::{IntoElement, ParentElement, Styled, div, px};
use ui::{Icon, IconName, IconWeight};

const WEIGHTS: [IconWeight; 4] = [
    IconWeight::Regular,
    IconWeight::Bold,
    IconWeight::Fill,
    IconWeight::Duotone,
];

fn cell(name: IconName, weight: IconWeight) -> Option<impl IntoElement> {
    Icon::path(name, weight)?;
    Some(
        div()
            .size(px(24.0))
            .child(Icon::new(name).weight(weight).size(px(20.0))),
    )
}

pub(crate) fn icons() -> impl IntoElement {
    let cells = IconName::ALL.iter().flat_map(|&name| {
        WEIGHTS
            .into_iter()
            .filter_map(move |weight| cell(name, weight))
    });
    div().flex().flex_wrap().gap(px(6.0)).children(cells)
}
