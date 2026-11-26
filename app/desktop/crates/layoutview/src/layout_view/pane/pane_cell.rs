use std::rc::Rc;

use gpui::{
    AnyElement, App, ElementId, InteractiveElement, IntoElement, MouseButton, ParentElement,
    RenderOnce, SharedString, StatefulInteractiveElement, Styled, Window, div, px,
};
use layout::Rect;
use protocol::PaneId;
use ui::{Appearance, Palette, Surface, Theme};

use crate::geometry::insets::Insets;
use crate::layout_view::pane::hover_route::route;

use crate::geometry::pane_frame::PaneFrame;
use crate::layout_view::pane::pane_handle::PaneHandle;
use crate::layout_view::pane::ring_paint::RingPaint;

const RING_WIDTH_LIGHT: f32 = 2.0;
const RING_WIDTH_DARK: f32 = 1.0;
const DRAG_TINT: f32 = 0.08;

pub(crate) type CellAction = Rc<dyn Fn(&mut Window, &mut App)>;

#[derive(IntoElement)]
pub(crate) struct PaneCell {
    pub(crate) group: PaneId,
    pub(crate) slot: Rect,
    pub(crate) frame: PaneFrame,
    pub(crate) content: AnyElement,
    pub(crate) ring_opacity: f32,
    pub(crate) dragging: bool,
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
        let id = ElementId::from(SharedString::from(self.group.as_str().to_owned()));
        let at = handle_rect(&frame);
        let seat = (at.x - self.slot.x, at.y - self.slot.y);
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
                pane_box(&frame, self.slot, palette)
                    .child(self.content)
                    .children(self.dragging.then(|| tint(palette))),
            )
            .child(ring(
                &frame,
                self.slot,
                &RingPaint {
                    accent: palette.pane_accent(),
                    width: ring_width(cx),
                    opacity: self.ring_opacity,
                },
            ))
            .children(self.handle.map(|handle| seated(handle, seat)))
    }
}

fn seated(
    handle: PaneHandle,
    (left, top): (f32, f32),
) -> impl IntoElement {
    let (width, height) = PaneHandle::size();
    div()
        .absolute()
        .left(px(left))
        .top(px(top))
        .w(px(width))
        .h(px(height))
        .child(handle)
}

fn tint(palette: &Palette) -> gpui::Div {
    div()
        .debug_selector(|| "pane-drag-tint".into())
        .absolute()
        .inset_0()
        .bg(palette.secondary().opacity(DRAG_TINT))
}

pub(crate) fn pane_box(
    frame: &PaneFrame,
    slot: Rect,
    palette: &Palette,
) -> gpui::Div {
    boxed(frame, slot, palette.pane_border(), palette.pane_chrome_bg())
}

pub(crate) fn handle_rect(frame: &PaneFrame) -> Rect {
    let (width, height) = PaneHandle::size();
    Rect {
        x: frame.outer.x + (frame.outer.width - width) / 2.0,
        y: frame.outer.y,
        width,
        height,
    }
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
    use gpui::{TestAppContext, point, px};
    use layout::{DragSubject, Edge};

    use crate::test_support::{layout_of, open_floating};

    #[gpui::test]
    fn while_the_drop_preview_shows_the_panes_are_tinted_and_the_window_covered(
        cx: &mut TestAppContext
    ) {
        let mut floating = open_floating(cx, &layout_of("a", &[("b", "a", Edge::Right)]));
        let view = floating.view.clone();
        let over = DragSubject::Chat("t1".into());
        let window = &mut floating.window;
        assert_eq!(
            window.debug_bounds("pane-drag-tint"),
            None,
            "no drag, no tint"
        );
        view.update(window, |view, cx| {
            view.hover(&over, point(px(10.0), px(100.0)), cx);
        });
        window.run_until_parked();
        assert!(
            window.debug_bounds("drop-preview").is_some(),
            "in the window"
        );
        assert!(ui::testing::covered(window).0, "the web views are hidden");
        assert!(window.debug_bounds("pane-drag-tint").is_some(), "tinted");
        view.update(window, |view, cx| {
            view.hover(&over, point(px(-5.0), px(-5.0)), cx);
        });
        window.run_until_parked();
        assert_eq!(window.debug_bounds("drop-preview"), None, "gone outside");
        assert!(!ui::testing::covered(window).0, "the web views are back");
        assert_eq!(window.debug_bounds("pane-drag-tint"), None, "untinted");
    }
}
