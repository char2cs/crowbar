//! Text input and select.

use gpui::{App, Entity, IntoElement, ParentElement, Pixels, SharedString, Styled, div, px};
use ui::{Select, spacing};

use crate::page::Page;

const SELECT_WIDTH: Pixels = px(160.0);
const INPUT_WIDTH: Pixels = px(240.0);

fn choose(
    page: &Entity<Page>,
    index: usize,
    cx: &mut App,
) {
    page.update(cx, |page, cx| {
        page.edit(cx, |page| page.choice = Some(index));
    });
}

pub(crate) fn inputs(
    page: &Page,
    entity: &Entity<Page>,
) -> impl IntoElement {
    let options: Vec<SharedString> = ["System", "Light", "Dark"].map(Into::into).to_vec();
    let owner = entity.clone();
    let select = Select::new("select", options, page.choice)
        .width(SELECT_WIDTH)
        .on_change(move |index, _, cx| choose(&owner, index, cx));
    div()
        .flex()
        .items_start()
        .gap(spacing::STEP_4)
        .child(div().w(INPUT_WIDTH).child(page.input.clone()))
        .child(select)
}
