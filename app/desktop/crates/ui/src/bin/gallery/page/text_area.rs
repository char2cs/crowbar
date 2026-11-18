//! Text areas in each size, state and fill level.

use gpui::{App, AppContext as _, Entity, IntoElement, ParentElement, Pixels, Styled, div, px};
use ui::{TextArea, TextAreaSize, spacing};

const COLUMN_WIDTH: Pixels = px(300.0);
const GROWN_MAX: Pixels = px(120.0);
const SHORT: &str = "Fix the sidebar rename";
const COMMIT: &str = "Add the native text area\n\nWraps long lines, grows with its text and scrolls past its maximum height, so commit messages stay readable.";
const LONG: &str = "One\nTwo\nThree\nFour\nFive\nSix\nSeven\nEight";

fn area(
    cx: &mut App,
    text: &str,
    build: impl FnOnce(TextArea) -> TextArea,
) -> Entity<TextArea> {
    let entity = cx.new(|cx| build(TextArea::new("Describe the change…", cx)));
    entity.update(cx, |area, cx| area.set_text(text, cx));
    entity
}

pub(crate) fn areas(cx: &mut App) -> Vec<Entity<TextArea>> {
    vec![
        area(cx, "", |area| area),
        area(cx, SHORT, |area| area.size(TextAreaSize::Small)),
        area(cx, COMMIT, |area| area),
        area(cx, SHORT, |area| area.size(TextAreaSize::Large)),
        area(cx, LONG, |area| area.max_height(GROWN_MAX)),
        area(cx, SHORT, TextArea::invalid),
        area(cx, SHORT, TextArea::disabled),
    ]
}

fn column(areas: &[Entity<TextArea>]) -> gpui::Div {
    div()
        .flex()
        .flex_col()
        .gap(spacing::STEP_3)
        .w(COLUMN_WIDTH)
        .children(areas.iter().cloned())
}

pub(crate) fn textarea(areas: &[Entity<TextArea>]) -> impl IntoElement {
    let (left, right) = areas.split_at(areas.len().div_ceil(2));
    div()
        .flex()
        .gap(spacing::STEP_6)
        .child(column(left))
        .child(column(right))
}
