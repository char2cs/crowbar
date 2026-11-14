//! One page entry of the rail.

use gpui::prelude::FluentBuilder;
use gpui::{
    App, ElementId, InteractiveElement, IntoElement, ParentElement, SharedString,
    StatefulInteractiveElement, Styled, Window, div, px,
};
use ui::font::HEADING_FAMILY;
use ui::{Icon, IconWeight, Surface, Theme, hover_fill, radius};

use crate::dispatch::send;
use crate::frame::Frame;
use crate::icon_stem::icon_named;
use crate::intent::Intent;
use crate::page::Page;

const HEIGHT: f32 = 32.0;
const ICON: f32 = 18.0;
const LABEL: f32 = 19.0;
const ACTIVE_TINT: f32 = 0.12;

pub(crate) fn rail_item(
    frame: &Frame<'_>,
    page: &Page,
    (window, cx): (&mut Window, &mut App),
) -> impl IntoElement {
    let palette = Theme::of(cx).palette(Surface::Window);
    let active = frame
        .state
        .page()
        .is_some_and(|selected| selected.id == page.id);
    let id = ElementId::from(SharedString::from(format!("rail-{}", page.id)));
    let color = if active {
        palette.foreground()
    } else {
        palette.muted_foreground()
    };
    let base = div()
        .flex()
        .items_center()
        .gap(px(10.0))
        .w_full()
        .h(px(HEIGHT))
        .px(px(10.0))
        .rounded(radius::XL)
        .text_color(color);
    let fill = if active {
        palette.primary().opacity(ACTIVE_TINT)
    } else {
        palette.accent()
    };
    let item = hover_fill(base, id, fill, (window, cx));
    let foreground = palette.foreground();
    item.when(active, |item| item.bg(fill))
        .when(!active, |item| {
            item.hover(move |style| style.text_color(foreground))
        })
        .cursor_pointer()
        .on_click(send(frame.owner, Intent::Select(page.id.clone())))
        .children(
            icon_named(&page.icon)
                .map(|icon| Icon::new(icon).weight(IconWeight::Duotone).size(px(ICON))),
        )
        .child(label(page))
}

fn label(page: &Page) -> impl IntoElement {
    div()
        .font_family(HEADING_FAMILY)
        .text_size(px(LABEL))
        .line_height(px(HEIGHT))
        .overflow_hidden()
        .whitespace_nowrap()
        .child(page.label.clone())
}

#[cfg(test)]
#[path = "rail_item_tests.rs"]
mod tests;
