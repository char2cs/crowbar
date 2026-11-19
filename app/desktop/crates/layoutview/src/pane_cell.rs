use std::rc::Rc;

use gpui::{
    AnyElement, App, ElementId, InteractiveElement, IntoElement, MouseButton, ParentElement,
    RenderOnce, SharedString, StatefulInteractiveElement, Styled, Window, div, px,
};
use layout::Rect;
use protocol::PaneId;
use ui::{Appearance, Palette, Portal, Surface, Theme};

use crate::insets::Insets;

use crate::pane_frame::PaneFrame;
use crate::pane_handle::PaneHandle;
use crate::ring_paint::RingPaint;

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
    fn render(
        self,
        _window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        let palette = Theme::of(cx).palette(Surface::Window);
        let frame = self.frame;
        let (enter, leave, press) = (self.on_enter, self.on_leave, self.on_press);
        let id = ElementId::from(SharedString::from(self.pane.as_str().to_owned()));
        let (handle_left, handle_top) = handle_origin(&frame);
        let seat = (handle_left - self.slot.x, handle_top - self.slot.y);
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
            .child(pane_box(&frame, self.slot, palette).child(self.content))
            .child(ring(
                &frame,
                self.slot,
                &RingPaint {
                    accent: palette.pane_accent(),
                    width: ring_width(cx),
                    opacity: self.ring_opacity,
                },
            ))
            .children(self.handle.map(|handle| floated_handle(handle, seat)))
    }
}

fn floated_handle(
    handle: PaneHandle,
    (left, top): (f32, f32),
) -> impl IntoElement {
    let (width, height) = PaneHandle::size();
    let id = SharedString::from(format!("pane-handle-{}", handle.pane.as_str()));
    let floated = Portal::here(id, move |_, _| handle.clone());
    div()
        .absolute()
        .left(px(left))
        .top(px(top))
        .w(px(width))
        .h(px(height))
        .child(floated)
}

pub(crate) fn pane_box(
    frame: &PaneFrame,
    slot: Rect,
    palette: &Palette,
) -> gpui::Div {
    boxed(frame, slot, palette.pane_border(), palette.pane_chrome_bg())
}

fn route(
    actions: [&CellAction; 2],
    hovered: usize,
    window: &mut Window,
    cx: &mut App,
) {
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

fn boxed(
    frame: &PaneFrame,
    slot: Rect,
    border: gpui::Hsla,
    fill: gpui::Hsla,
) -> gpui::Div {
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
    paint: &RingPaint,
) -> gpui::Div {
    let RingPaint {
        accent,
        width,
        opacity,
    } = *paint;
    let drawn = |side: f32| width * f32::from(side > 0.0);
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

fn framed(
    frame: &PaneFrame,
    slot: Rect,
) -> gpui::Div {
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
mod tests {
    use layout::Rect;

    use super::*;
    use crate::chrome_context::ChromeContext;

    #[test]
    fn the_handle_sits_centred_on_the_top_edge_of_the_box() {
        let slot = Rect {
            x: 0.0,
            y: 0.0,
            width: 400.0,
            height: 300.0,
        };
        let frame = PaneFrame::new(slot, slot, ChromeContext::default());
        let (left, top) = handle_origin(&frame);
        let (width, _) = PaneHandle::size();
        assert_eq!(
            left,
            frame.outer.x + (frame.outer.width - width) / 2.0,
            "centred"
        );
        assert_eq!(top, frame.outer.y, "on the top edge");
    }

    #[test]
    fn the_ring_is_two_pixels_in_light_mode_and_one_in_dark() {
        assert_eq!(RING_WIDTH_LIGHT, 2.0, "light");
        assert_eq!(RING_WIDTH_DARK, 1.0, "dark");
    }
}
