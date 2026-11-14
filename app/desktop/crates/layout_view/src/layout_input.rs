use std::time::Instant;

use gpui::{
    Bounds, Context, DragMoveEvent, MouseDownEvent, MouseUpEvent, Pixels, Point, Window, point, px,
    size,
};
use model::{Command, DragSubject, Sash};
use protocol::PaneId;
use ui::{Motion, Overlays, SashDrag};

use crate::drop_hit::DropHit;
use crate::layout_event::LayoutEvent;
use crate::layout_view::LayoutView;
use crate::pane_handle::PaneHandle;
use crate::sash_grab::SashGrab;

impl LayoutView {
    pub(crate) fn drag_over(
        &mut self,
        event: &DragMoveEvent<DragSubject>,
        _: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let position = event.event.position;
        let Some(tab) = &self.tab else {
            return;
        };
        if !self.bounds.contains(&position) {
            self.drop.clear();
            cx.notify();
            return;
        }
        let subject = event.drag(cx).clone();
        let slots = tab.layout(self.local());
        let hit = DropHit {
            subject: &subject,
            point: self.local_point(position),
            slots: &slots,
            window_bounds: self.bounds,
        };
        self.drop.over(hit, cx);
        cx.notify();
    }

    pub(crate) fn dropped(
        &mut self,
        subject: &DragSubject,
        _: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let command = self
            .tab
            .as_ref()
            .and_then(|tab| self.drop.command(subject, tab));
        self.drop.clear();
        cx.notify();
        if let Some(command) = command {
            cx.emit(LayoutEvent::Apply(command));
        }
    }

    pub(crate) fn released(
        &mut self,
        _: &MouseUpEvent,
        _: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        self.sash = None;
        if self.drop.is_dragging() {
            self.drop.clear();
            cx.notify();
        }
    }

    pub(crate) fn grab_sash(
        &mut self,
        sash: &Sash,
        event: &MouseDownEvent,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(tab) = self.tab.clone() else {
            return;
        };
        let local = self.local_point(event.position);
        let grab = SashGrab::begin(&tab, sash, local, self.local());
        if event.click_count < 2 {
            self.sash = grab;
            return;
        }
        self.sash = None;
        if let Some(grab) = grab {
            cx.emit(LayoutEvent::Apply(grab.reset()));
        }
    }

    pub(crate) fn sash_moved(
        &mut self,
        event: &DragMoveEvent<SashDrag>,
        _: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(grab) = &self.sash else {
            return;
        };
        let command = grab.resize(self.local_point(event.event.position));
        cx.emit(LayoutEvent::Apply(command));
    }

    pub(crate) fn pane_entered(&mut self, pane: &PaneId, cx: &mut Context<'_, Self>) {
        self.hovered = Some(pane.clone());
        self.hover_changed(cx);
    }

    pub(crate) fn pane_left(&mut self, pane: &PaneId, cx: &mut Context<'_, Self>) {
        if self.hovered.as_ref() == Some(pane) {
            self.hovered = None;
        }
        self.hover_changed(cx);
    }

    fn hover_changed(&mut self, cx: &mut Context<'_, Self>) {
        self.sync_fades(cx);
        self.sync_handle_overlay(cx);
        cx.notify();
    }

    pub(crate) fn pane_pressed(&mut self, pane: &PaneId, cx: &mut Context<'_, Self>) {
        let focused = self.tab.as_ref().and_then(|tab| tab.focused());
        if focused != Some(pane) {
            cx.emit(LayoutEvent::Apply(Command::FocusPane {
                pane: pane.clone(),
            }));
        }
    }

    pub(crate) fn detach(&mut self, pane: &PaneId, cx: &mut Context<'_, Self>) {
        let (new_tab, title) = self.source.fresh_tab(pane);
        cx.emit(LayoutEvent::Apply(Command::DetachPane {
            pane: pane.clone(),
            new_tab,
            title,
        }));
    }

    pub(crate) fn close(pane: &PaneId, cx: &mut Context<'_, Self>) {
        cx.emit(LayoutEvent::Apply(Command::ClosePane {
            pane: pane.clone(),
        }));
    }

    pub(crate) fn sync_handle_overlay(&mut self, cx: &mut Context<'_, Self>) {
        let Some(bounds) = self.handle_bounds(Instant::now(), Motion::of(cx)) else {
            self.handle_overlay = None;
            return;
        };
        match &self.handle_overlay {
            Some(guard) => guard.update(bounds),
            None => self.handle_overlay = Some(Overlays::open(cx, bounds)),
        }
    }

    fn handle_bounds(&self, now: Instant, motion: Motion) -> Option<Bounds<Pixels>> {
        let pane = self.hovered.as_ref()?;
        let tab = self.tab.as_ref().filter(|tab| tab.len() > 1)?;
        if self.handles.opacity(pane, now, motion) <= 0.0 && self.hovered.is_none() {
            return None;
        }
        let slot = tab
            .layout(self.local())
            .into_iter()
            .find(|slot| &slot.pane == pane)?;
        let frame = crate::pane_frame::PaneFrame::new(slot.rect, self.local(), self.chrome);
        let (width, height) = PaneHandle::size();
        let left = frame.outer.x + (frame.outer.width - width) / 2.0;
        let origin = self.bounds.origin + point(px(left), px(frame.outer.y));
        Some(Bounds::new(origin, size(px(width), px(height))))
    }

    fn local_point(&self, position: Point<Pixels>) -> (f32, f32) {
        let local = position - self.bounds.origin;
        (f32::from(local.x), f32::from(local.y))
    }
}

#[cfg(test)]
#[path = "layout_input_tests.rs"]
mod tests;
