//! Every embedded icon in every embedded weight.

use gpui::{Context, IntoElement, ParentElement, Styled, div, px};
use ui::{Icon, IconName, IconWeight};

use crate::page::Page;

const WEIGHTS: [IconWeight; 4] = [IconWeight::Regular, IconWeight::Bold, IconWeight::Fill, IconWeight::Duotone];

impl Page {
    pub(crate) fn icons(&self, _cx: &Context<Self>) -> impl IntoElement {
        let cells = IconName::ALL.iter().flat_map(|&name| {
            WEIGHTS
                .into_iter()
                .filter(move |&weight| Icon::path(name, weight).is_some())
                .map(move |weight| div().size(px(24.0)).child(Icon::new(name).weight(weight).size(px(20.0))))
        });
        div().flex().flex_wrap().gap(px(6.0)).children(cells)
    }
}
