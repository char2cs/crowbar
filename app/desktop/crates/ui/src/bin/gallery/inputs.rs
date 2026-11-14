//! Text input and select.

use gpui::{Entity, IntoElement, ParentElement, SharedString, Styled, div, px};
use ui::Select;

use crate::page::Page;

pub(crate) fn inputs(page: &Page, entity: &Entity<Page>) -> impl IntoElement {
    let options: Vec<SharedString> = ["System", "Light", "Dark"].map(Into::into).to_vec();
    let owner = entity.clone();
    let select = Select::new("select", options, page.choice)
        .width(px(160.0))
        .on_change(move |index, _, cx| {
            owner.update(cx, |page, cx| {
                page.edit(cx, |page| page.choice = Some(index));
            });
        });
    div()
        .flex()
        .items_start()
        .gap(px(16.0))
        .child(div().w(px(240.0)).child(page.input.clone()))
        .child(select)
}
