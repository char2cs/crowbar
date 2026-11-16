use std::rc::Rc;
use std::time::Instant;

use gpui::{
    AnyElement, AnyView, App, Context, Hsla, InteractiveElement, IntoElement, MouseButton,
    MouseDownEvent, ParentElement, Render, Styled, Window, canvas, div, px,
};
use model::{Axis, DragSubject, PaneSlot, Rect, Sash as ModelSash};
use protocol::PaneId;
use ui::{DraggedRow, Motion, Sash, SashDrag, Surface, Theme, Wordmark};

use crate::layout_view::LayoutView;
use crate::pane_cell::{CellAction, PaneCell};
use crate::pane_frame::PaneFrame;
use crate::pane_handle::PaneHandle;

const PREVIEW_BORDER: f32 = 2.0;
const PREVIEW_FILL: f32 = 0.14;
const PREVIEW_RING: f32 = 0.25;
const LOGO_SHARE: f32 = 0.14;
const LOGO_MIN_WIDTH: f32 = 96.0;
const LOGO_MAX_WIDTH: f32 = 148.0;

impl Render for LayoutView {
    fn render(
        &mut self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> impl IntoElement {
        let (now, motion) = (self.clock.now(), Motion::of(cx));
        let busy = self.rings.is_running(now, motion)
            || self.handles.is_running(now, motion)
            || self.drop.is_running(now, motion);
        if busy {
            window.request_animation_frame();
        }
        let view = cx.entity();
        div()
            .id("layout-view")
            .relative()
            .size_full()
            .child(
                canvas(
                    move |bounds, _, cx: &mut App| {
                        view.update(cx, |this, cx| this.measured(bounds, cx));
                    },
                    |_, (), _, _| {},
                )
                .absolute()
                .size_full(),
            )
            .children(self.logo())
            .children(self.pane_cells(window, cx))
            .children(self.sashes(cx))
            .children(self.preview(now, motion, cx))
            .on_drag_move::<DragSubject>(cx.listener(Self::drag_over))
            .on_drag_move::<DraggedRow>(cx.listener(Self::row_over))
            .on_drag_move::<SashDrag>(cx.listener(Self::sash_moved))
            .on_drop::<DragSubject>(cx.listener(Self::dropped))
            .on_mouse_up(MouseButton::Left, cx.listener(Self::row_released))
            .on_mouse_up_out(MouseButton::Left, cx.listener(Self::released))
    }
}

impl LayoutView {
    pub(crate) fn shows_logo(&self) -> bool {
        self.tab.is_none()
    }

    fn logo(&self) -> Option<AnyElement> {
        if !self.shows_logo() {
            return None;
        }
        let smaller_side = f32::from(self.bounds.size.width.min(self.bounds.size.height));
        let width = (smaller_side * LOGO_SHARE).clamp(LOGO_MIN_WIDTH, LOGO_MAX_WIDTH);
        let centred = div()
            .absolute()
            .size_full()
            .flex()
            .items_center()
            .justify_center()
            .child(Wordmark::new(px(width)));
        Some(centred.into_any_element())
    }

    fn pane_cells(
        &self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> Vec<AnyElement> {
        let Some(tab) = &self.tab else {
            return Vec::new();
        };
        let local = self.local();
        tab.layout(local)
            .into_iter()
            .map(|slot| self.pane_cell(slot, window, cx))
            .collect()
    }

    fn pane_cell(
        &self,
        slot: PaneSlot,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> AnyElement {
        let (now, motion) = (self.clock.now(), Motion::of(cx));
        let frame = PaneFrame::new(slot.rect, self.local(), self.chrome);
        let content = self.source.content(&slot.pane, window, cx);
        let chrome = self.chrome_for(&slot.pane, window, cx);
        let slot_pane = slot.pane.clone();
        PaneCell {
            handle: self.handle_for(&slot.pane, (now, motion), cx),
            ring_opacity: self.rings.opacity(&slot.pane, now, motion),
            on_enter: Self::enter_action(&slot.pane, cx),
            on_leave: Self::leave_action(&slot.pane, cx),
            on_press: Self::press_action(&slot.pane, cx),
            pane: slot.pane,
            slot: slot.rect,
            frame,
            content,
            chrome,
            strip: self.strip_of(&slot_pane, cx),
        }
        .into_any_element()
    }

    fn chrome_for(
        &self,
        pane: &PaneId,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> Option<(AnyView, f32)> {
        let height = self.source.chrome_height(pane);
        if height <= 0.0 {
            return None;
        }
        self.source
            .chrome(pane, window, cx)
            .map(|view| (view, height))
    }

    fn handle_for(
        &self,
        pane: &PaneId,
        (now, motion): (Instant, Motion),
        cx: &mut Context<'_, Self>,
    ) -> Option<PaneHandle> {
        let opacity = self.handles.opacity(pane, now, motion);
        let crowded = self.tab.as_ref().is_some_and(|tab| tab.group_count() > 1);
        if !crowded || opacity <= 0.0 {
            return None;
        }
        Some(PaneHandle {
            pane: pane.clone(),
            label: self.source.label(pane),
            opacity,
            on_detach: Self::detach_action(pane, cx),
            on_close: Self::close_action(pane, cx),
        })
    }

    fn sashes(
        &self,
        cx: &mut Context<'_, Self>,
    ) -> Vec<AnyElement> {
        let Some(tab) = &self.tab else {
            return Vec::new();
        };
        tab.sashes(self.local())
            .into_iter()
            .enumerate()
            .map(|(index, sash)| Self::sash_element(index, &sash, cx))
            .collect()
    }

    fn sash_element(
        index: usize,
        sash: &ModelSash,
        cx: &mut Context<'_, Self>,
    ) -> AnyElement {
        let rect = sash.rect;
        let axis = ui_axis(sash.axis);
        let grabbed = sash.clone();
        div()
            .id(("sash", index))
            .absolute()
            .left(px(rect.x))
            .top(px(rect.y))
            .w(px(rect.width))
            .h(px(rect.height))
            .on_mouse_down(
                MouseButton::Left,
                cx.listener(move |this, event: &MouseDownEvent, _, cx| {
                    this.grab_sash(&grabbed, event, cx);
                }),
            )
            .child(Sash::new(("sash-bar", index), axis).on(Surface::Window))
            .into_any_element()
    }

    fn preview(
        &self,
        now: Instant,
        motion: Motion,
        cx: &App,
    ) -> Option<AnyElement> {
        let rect = self.drop.preview(now, motion)?;
        let palette = Theme::of(cx).palette(Surface::Window);
        Some(preview_box(rect, palette.secondary()).into_any_element())
    }

    fn enter_action(
        pane: &PaneId,
        cx: &mut Context<'_, Self>,
    ) -> CellAction {
        let (view, pane) = (cx.entity(), pane.clone());
        Rc::new(move |_, cx| view.update(cx, |this, cx| this.pane_entered(&pane, cx)))
    }

    fn leave_action(
        pane: &PaneId,
        cx: &mut Context<'_, Self>,
    ) -> CellAction {
        let (view, pane) = (cx.entity(), pane.clone());
        Rc::new(move |_, cx| view.update(cx, |this, cx| this.pane_left(&pane, cx)))
    }

    fn press_action(
        pane: &PaneId,
        cx: &mut Context<'_, Self>,
    ) -> CellAction {
        let (view, pane) = (cx.entity(), pane.clone());
        Rc::new(move |_, cx| view.update(cx, |this, cx| this.pane_pressed(&pane, cx)))
    }

    fn detach_action(
        pane: &PaneId,
        cx: &mut Context<'_, Self>,
    ) -> crate::pane_handle::HandleAction {
        let (view, pane) = (cx.entity(), pane.clone());
        Rc::new(move |_, cx| view.update(cx, |this, cx| this.detach(&pane, cx)))
    }

    fn close_action(
        pane: &PaneId,
        cx: &mut Context<'_, Self>,
    ) -> crate::pane_handle::HandleAction {
        let (view, pane) = (cx.entity(), pane.clone());
        Rc::new(move |_, cx| view.update(cx, |_, cx| Self::close(&pane, cx)))
    }
}

fn ui_axis(axis: Axis) -> ui::Axis {
    match axis {
        Axis::Horizontal => ui::Axis::Horizontal,
        Axis::Vertical => ui::Axis::Vertical,
    }
}

fn preview_box(
    rect: Rect,
    accent: Hsla,
) -> gpui::Div {
    div()
        .absolute()
        .left(px(rect.x))
        .top(px(rect.y))
        .w(px(rect.width))
        .h(px(rect.height))
        .rounded(ui::radius::LG)
        .border(px(PREVIEW_BORDER))
        .border_color(accent)
        .bg(accent.opacity(PREVIEW_FILL))
        .shadow(vec![gpui::BoxShadow {
            color: accent.opacity(PREVIEW_RING),
            offset: gpui::point(px(0.0), px(0.0)),
            blur_radius: px(0.0),
            spread_radius: px(1.0),
        }])
}

#[cfg(test)]
mod tests {
    use model::Axis;

    use super::*;

    #[test]
    fn model_axes_map_to_the_matching_sash_axes() {
        assert_eq!(
            ui_axis(Axis::Horizontal),
            ui::Axis::Horizontal,
            "side by side"
        );
        assert_eq!(ui_axis(Axis::Vertical), ui::Axis::Vertical, "stacked");
    }
}
