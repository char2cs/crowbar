//! The UI family at each weight of its variable axis.

use gpui::{FontWeight, IntoElement, ParentElement, Styled, div};
use ui::{font, spacing};

const WEIGHTS: [f32; 5] = [300.0, 400.0, 500.0, 600.0, 700.0];

pub(crate) fn typography() -> impl IntoElement {
    let rows = WEIGHTS.map(|weight| {
        div()
            .font_weight(FontWeight(weight))
            .child(format!("Settings and sessions {weight}"))
    });
    div()
        .flex()
        .flex_col()
        .gap(spacing::STEP_1)
        .font_family(font::UI_FAMILY)
        .children(rows)
}
