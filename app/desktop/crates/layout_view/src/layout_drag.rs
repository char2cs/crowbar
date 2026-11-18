use gpui::{Context, DragMoveEvent, MouseUpEvent, Pixels, Point, Window};
use layout::DragSubject;
use protocol::PaneId;
use ui::DraggedRow;

use crate::drop_hit::DropHit;
use crate::drop_target::DropTarget;
use crate::layout_view::LayoutView;

impl LayoutView {
    pub(crate) fn drag_over(
        &mut self,
        event: &DragMoveEvent<DragSubject>,
        _: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let subject = event.drag(cx).clone();
        self.hover(&subject, event.event.position, cx);
    }

    pub(crate) fn row_over(
        &mut self,
        event: &DragMoveEvent<DraggedRow>,
        _: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(chat) = event.drag(cx).chat.as_deref() else {
            return;
        };
        let subject = DragSubject::Chat(chat.to_owned());
        self.hover(&subject, event.event.position, cx);
    }

    pub(crate) fn hover(
        &mut self,
        subject: &DragSubject,
        position: Point<Pixels>,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(tab) = &self.tab else {
            return;
        };
        if !self.bounds.contains(&position) {
            self.drop.clear();
            cx.notify();
            return;
        }
        let slots = DropTarget::slots_for(subject, tab, self.local());
        let joinable: Vec<PaneId> = slots
            .iter()
            .map(|slot| slot.pane.clone())
            .filter(|pane| self.joins_strip(subject, pane))
            .collect();
        let hit = DropHit {
            subject,
            point: self.local_point(position),
            slots: &slots,
            joinable: &joinable,
            window_bounds: self.bounds,
            now: self.clock.now(),
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
        self.release(subject, cx);
    }

    fn release(
        &mut self,
        subject: &DragSubject,
        cx: &mut Context<'_, Self>,
    ) {
        let event = self
            .tab
            .as_ref()
            .and_then(|tab| self.drop.event(subject, tab));
        self.drop.clear();
        cx.notify();
        if let Some(event) = event {
            cx.emit(event);
        }
    }

    pub(crate) fn row_released(
        &mut self,
        event: &MouseUpEvent,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let opened = self.tab.as_ref().and_then(|tab| self.drop.chat_event(tab));
        self.released(event, window, cx);
        if let Some(opened) = opened {
            cx.emit(opened);
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
}

#[cfg(test)]
mod tests {
    use gpui::{TestAppContext, point, px};
    use layout::Edge;

    use crate::test_support::{layout_of, open};

    fn dragging_after_hover(
        view: &gpui::Entity<crate::layout_view::LayoutView>,
        subject: &layout::DragSubject,
        spot: gpui::Point<gpui::Pixels>,
        cx: &mut gpui::VisualTestContext,
    ) -> bool {
        view.update(cx, |view, cx| {
            view.hover(subject, spot, cx);
            view.drop.is_dragging()
        })
    }

    #[gpui::test]
    fn hovering_inside_the_view_opens_a_drop_and_outside_it_clears_it(cx: &mut TestAppContext) {
        let layout = layout_of("a", &[("b", "a", Edge::Right)]);
        let harness = open(cx, &layout);
        let (view, cx) = (harness.view, harness.cx);
        let subject = layout::DragSubject::Chat("t1".into());
        let centre = view.read_with(cx, |view, _| view.bounds.center());
        let cases = [(centre, true), (point(px(-50.0), px(-50.0)), false)];
        for (spot, expected) in cases {
            let dragging = dragging_after_hover(&view, &subject, spot, cx);
            assert_eq!(dragging, expected, "dragging at {spot:?}");
        }
    }

    #[gpui::test]
    fn releasing_a_drag_clears_the_drop_overlay(cx: &mut TestAppContext) {
        let layout = layout_of("a", &[("b", "a", Edge::Right)]);
        let harness = open(cx, &layout);
        let (view, cx) = (harness.view, harness.cx);
        let subject = layout::DragSubject::Chat("t1".into());
        let after = view.update(cx, |view, cx| {
            view.hover(&subject, view.bounds.center(), cx);
            view.release(&subject, cx);
            view.drop.is_dragging()
        });
        assert!(!after, "no drag after the release");
    }
}
