//! Every embedded icon in every embedded weight.

use gpui::{IntoElement, ParentElement, Styled, div};
use ui::{Icon, IconName, IconWeight, spacing};

const WEIGHTS: [IconWeight; 4] = [
    IconWeight::Regular,
    IconWeight::Bold,
    IconWeight::Fill,
    IconWeight::Duotone,
];

fn cell(
    name: IconName,
    weight: IconWeight,
) -> Option<impl IntoElement> {
    Icon::path(name, weight)?;
    Some(
        div()
            .size(spacing::STEP_6)
            .child(Icon::new(name).weight(weight).size(spacing::STEP_5)),
    )
}

pub(crate) fn icons() -> impl IntoElement {
    let cells = IconName::ALL.iter().flat_map(|&name| {
        WEIGHTS
            .into_iter()
            .filter_map(move |weight| cell(name, weight))
    });
    div()
        .flex()
        .flex_wrap()
        .gap(spacing::STEP_1_5)
        .children(cells)
}
