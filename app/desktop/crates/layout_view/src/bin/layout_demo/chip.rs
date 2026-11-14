use gpui::{
    AppContext as _, ElementId, InteractiveElement, IntoElement, ParentElement, SharedString,
    StatefulInteractiveElement, Styled, div, px,
};
use model::DragSubject;
use ui::{DragGhost, Surface, Theme, radius, text_size};

use crate::scenario::instance;

pub(crate) fn chip(name: &'static str, cx: &gpui::App) -> impl IntoElement {
    let palette = Theme::of(cx).palette(Surface::Chrome);
    let label: SharedString = format!("Chat {name}").into();
    let ghost = label.clone();
    div()
        .id(ElementId::from(name))
        .h(px(32.0))
        .px(px(12.0))
        .flex()
        .items_center()
        .rounded(radius::LG)
        .bg(palette.sidebar_element_idle())
        .hover(move |style| style.bg(palette.sidebar_element_hover()))
        .text_size(text_size::SM)
        .text_color(palette.foreground())
        .on_drag(DragSubject::Open(instance(name)), move |_, _, _, cx| {
            cx.new(|_| DragGhost::new(ghost.clone(), 160.0))
        })
        .child(label)
}
