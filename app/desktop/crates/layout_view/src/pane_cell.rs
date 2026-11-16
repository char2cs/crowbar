use std::rc::Rc;

use gpui::prelude::FluentBuilder;
use gpui::{
    AnyElement, AnyView, App, ElementId, InteractiveElement, IntoElement, MouseButton,
    ParentElement, RenderOnce, SharedString, StatefulInteractiveElement, Styled, Window, div, px,
};
use model::Rect;
use protocol::PaneId;
use ui::{Appearance, Palette, Surface, Theme};

use crate::insets::Insets;

use crate::pane_frame::PaneFrame;
use crate::pane_handle::PaneHandle;
use crate::pane_strip::{PaneStrip, STRIP_HEIGHT};

const RING_WIDTH_LIGHT: f32 = 2.0;
const RING_WIDTH_DARK: f32 = 1.0;

type Chrome = Option<(AnyView, f32)>;

pub(crate) type CellAction = Rc<dyn Fn(&mut Window, &mut App)>;

#[derive(IntoElement)]
pub(crate) struct PaneCell {
    pub(crate) pane: PaneId,
    pub(crate) slot: Rect,
    pub(crate) frame: PaneFrame,
    pub(crate) content: AnyElement,
    pub(crate) chrome: Chrome,
    pub(crate) strip: Option<PaneStrip>,
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
            .child(body(
                &frame,
                self.slot,
                (self.chrome, self.strip, self.content),
                palette,
            ))
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

fn body(
    frame: &PaneFrame,
    slot: Rect,
    (chrome, strip, content): (Chrome, Option<PaneStrip>, AnyElement),
    palette: &Palette,
) -> impl IntoElement {
    let offset = f32::from(strip.is_some()) * STRIP_HEIGHT;
    boxed(frame, slot, palette.pane_border(), palette.pane_chrome_bg())
        .when_some(chrome, |frame, (chrome, height)| {
            frame.child(chrome_strip(chrome, (offset, height)))
        })
        .when_some(strip, ParentElement::child)
        .child(content)
}

fn chrome_strip(
    chrome: AnyView,
    (top, height): (f32, f32),
) -> gpui::Div {
    div()
        .absolute()
        .top(px(top))
        .left_0()
        .w_full()
        .h(px(height))
        .child(chrome)
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
    (accent, width): (gpui::Hsla, f32),
    opacity: f32,
) -> gpui::Div {
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
    use model::Rect;

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
