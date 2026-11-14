use std::rc::Rc;

use gpui::prelude::FluentBuilder;
use gpui::{
    AnyElement, App, ElementId, InteractiveElement, IntoElement, MouseButton, ParentElement,
    RenderOnce, SharedString, StatefulInteractiveElement, Styled, Window, div, px,
};
use model::Rect;
use protocol::PaneId;
use ui::{Appearance, Surface, Theme};

use crate::insets::Insets;

use crate::pane_frame::PaneFrame;
use crate::pane_handle::PaneHandle;

const RING_WIDTH_LIGHT: f32 = 2.0;
const RING_WIDTH_DARK: f32 = 1.0;

pub(crate) type CellAction = Rc<dyn Fn(&mut Window, &mut App)>;

#[derive(IntoElement)]
pub(crate) struct PaneCell {
    pub(crate) pane: PaneId,
    pub(crate) slot: Rect,
    pub(crate) frame: PaneFrame,
    pub(crate) content: AnyElement,
    pub(crate) ring_opacity: f32,
    pub(crate) handle: Option<PaneHandle>,
    pub(crate) on_enter: CellAction,
    pub(crate) on_leave: CellAction,
    pub(crate) on_press: CellAction,
}

impl RenderOnce for PaneCell {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let palette = Theme::of(cx).palette(Surface::Window);
        let frame = self.frame;
        let (enter, leave, press) = (self.on_enter, self.on_leave, self.on_press);
        let id = ElementId::from(SharedString::from(self.pane.as_str().to_owned()));
        let (handle_left, _) = handle_origin(&frame);
        div()
            .id(id)
            .absolute()
            .left(px(self.slot.x))
            .top(px(self.slot.y))
            .w(px(self.slot.width))
            .h(px(self.slot.height))
            .on_hover(move |hovered, window, cx| {
                route([&leave, &enter], usize::from(*hovered), window, cx);
            })
            .on_mouse_down(MouseButton::Left, move |_, window, cx| press(window, cx))
            .child(
                boxed(
                    &frame,
                    self.slot,
                    palette.pane_border(),
                    palette.pane_chrome_bg(),
                )
                .child(self.content),
            )
            .child(ring(
                &frame,
                self.slot,
                (palette.pane_accent(), ring_width(cx)),
                self.ring_opacity,
            ))
            .when_some(self.handle, |cell, handle| {
                let (left, top) = (handle_left - self.slot.x, frame.outer.y - self.slot.y);
                cell.child(div().absolute().left(px(left)).top(px(top)).child(handle))
            })
    }
}

fn route(actions: [&CellAction; 2], hovered: usize, window: &mut Window, cx: &mut App) {
    if let Some(action) = actions.get(hovered) {
        action(window, cx);
    }
}

fn handle_origin(frame: &PaneFrame) -> (f32, f32) {
    let (width, _) = PaneHandle::size();
    (
        frame.outer.x + (frame.outer.width - width) / 2.0,
        frame.outer.y,
    )
}

fn boxed(frame: &PaneFrame, slot: Rect, border: gpui::Hsla, fill: gpui::Hsla) -> gpui::Div {
    framed(frame, slot)
        .overflow_hidden()
        .bg(fill)
        .border_color(border)
}

fn ring_width(cx: &App) -> f32 {
    match Theme::of(cx).appearance() {
        Appearance::Light => RING_WIDTH_LIGHT,
        Appearance::Dark => RING_WIDTH_DARK,
    }
}

fn ring(
    frame: &PaneFrame,
    slot: Rect,
    (accent, width): (gpui::Hsla, f32),
    opacity: f32,
) -> gpui::Div {
    let drawn = |side: f32| if side > 0.0 { width } else { 0.0 };
    let border = Insets {
        left: drawn(frame.border.left),
        top: drawn(frame.border.top),
        right: drawn(frame.border.right),
        bottom: drawn(frame.border.bottom),
    };
    framed(&PaneFrame { border, ..*frame }, slot)
        .border_color(accent)
        .opacity(opacity)
}

fn framed(frame: &PaneFrame, slot: Rect) -> gpui::Div {
    let (outer, border, radius) = (frame.outer, frame.border, frame.radius);
    div()
        .absolute()
        .left(px(outer.x - slot.x))
        .top(px(outer.y - slot.y))
        .w(px(outer.width))
        .h(px(outer.height))
        .border_l(px(border.left))
        .border_t(px(border.top))
        .border_r(px(border.right))
        .border_b(px(border.bottom))
        .rounded_tl(px(radius.top_left))
        .rounded_tr(px(radius.top_right))
        .rounded_bl(px(radius.bottom_left))
        .rounded_br(px(radius.bottom_right))
}

#[cfg(test)]
#[path = "pane_cell_tests.rs"]
mod tests;
