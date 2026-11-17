use gpui::{
    AppContext as _, ElementId, InteractiveElement, IntoElement, ParentElement, SharedString,
    StatefulInteractiveElement, Styled, div, px,
};
use model::{DragSubject, PaneInstance};
use ui::{DragGhost, Surface, Theme, radius, text_size};


const CHIP_HEIGHT: f32 = 32.0;
const CHIP_PADDING: f32 = 12.0;
const GHOST_WIDTH: f32 = 160.0;

pub(crate) fn chip(
    instance: &PaneInstance,
    cx: &gpui::App,
) -> impl IntoElement {
    let palette = Theme::of(cx).palette(Surface::Chrome);
    let name = instance.pane_id.as_str().to_owned();
    let label: SharedString = format!("Chat {name}").into();
    let ghost = label.clone();
    div()
        .id(ElementId::from(SharedString::from(name)))
        .h(px(CHIP_HEIGHT))
        .px(px(CHIP_PADDING))
        .flex()
        .items_center()
        .rounded(radius::LG)
        .bg(palette.sidebar_element_idle())
        .hover(move |style| style.bg(palette.sidebar_element_hover()))
        .text_size(text_size::SM)
        .text_color(palette.foreground())
        .on_drag(DragSubject::Open(instance.clone()), move |_, _, _, cx| {
            cx.new(|_| DragGhost::new(ghost.clone(), GHOST_WIDTH))
        })
        .child(label)
}
