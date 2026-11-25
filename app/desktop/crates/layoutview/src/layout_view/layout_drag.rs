use gpui::{Context, DragMoveEvent, MouseUpEvent, Pixels, Point, Window};
use layout::{DragSubject, TabId};
use protocol::PaneId;
use ui::DraggedRow;

use crate::layout_view::LayoutView;
use crate::layout_view::drag::drop_hit::DropHit;
use crate::layout_view::drag::drop_target::DropTarget;

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

    /// A view tab dragged out of the tab strip is at `position`, in window coordinates: preview
    /// where its chats would land.
    pub fn tab_dragged_over(
        &mut self,
        tab: TabId,
        position: Point<Pixels>,
        cx: &mut Context<'_, Self>,
    ) {
        self.hover(&DragSubject::Tab(tab), position, cx);
    }

    /// The view tab dragged out of the tab strip was released at `position`, in window
    /// coordinates: ask to move its chats to the place under it, if any. The place is resolved
    /// at the release, since this view's own mouse-up may already have cleared the preview.
    pub fn tab_dropped(
        &mut self,
        tab: TabId,
        position: Point<Pixels>,
        cx: &mut Context<'_, Self>,
    ) {
        let subject = DragSubject::Tab(tab);
        self.hover(&subject, position, cx);
        self.release(&subject, cx);
    }

    /// The view tab drag ended without a drop here: clear its preview.
    pub fn tab_drag_cancelled(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        if self.drop.is_dragging() {
            self.drop.clear();
            cx.notify();
        }
    }

    pub(crate) fn hover(
        &mut self,
        subject: &DragSubject,
        position: Point<Pixels>,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(tab) = &self.tab else {
            self.hover_stage(subject, position, cx);
            return;
        };
        let own_tab = matches!(subject, DragSubject::Tab(dragged) if *dragged == tab.id());
        if own_tab || !self.bounds.contains(&position) {
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
            now: self.clock.now(),
        };
        self.drop.over(hit, cx);
        cx.notify();
    }

    fn hover_stage(
        &mut self,
        subject: &DragSubject,
        position: Point<Pixels>,
        cx: &mut Context<'_, Self>,
    ) {
        let on_stage = matches!(subject, DragSubject::Chat(_)) && self.bounds.contains(&position);
        self.drop.clear();
        if on_stage {
            self.drop.over_stage(subject);
        }
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
        let opened = self
            .tab
            .as_ref()
            .map_or_else(|| self.drop.stage_event(), |tab| self.drop.chat_event(tab));
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

    fn two_tabs() -> layout::Layout {
        let mut layout = layout_of("a", &[]);
        let second = layout::Command::AddTab {
            id: layout::TabId(2),
            title: "Two".into(),
            space: layout::SpaceId::default(),
        };
        layout.apply(second).expect("a second tab");
        let shown = layout::Command::ActivateTab {
            tab: layout::TabId(1),
        };
        layout.apply(shown).expect("tab one shown");
        layout
    }

    #[gpui::test]
    fn a_view_tab_dropped_on_a_pane_asks_to_move_its_chats_beside_it_after_the_mouse_up(
        cx: &mut TestAppContext
    ) {
        let layout = two_tabs();
        let harness = open(cx, &layout);
        let (view, cx, opened) = (harness.view, harness.cx, harness.opened);
        let centre = view.read_with(cx, |view, _| view.bounds.center());
        view.update(cx, |view, cx| {
            view.tab_dragged_over(layout::TabId(2), centre, cx);
        });
        let none = gpui::Modifiers::none();
        cx.simulate_mouse_up(centre, gpui::MouseButton::Left, none);
        view.update(cx, |view, cx| {
            view.tab_dropped(layout::TabId(2), centre, cx);
        });
        let placement = layout::Placement {
            target: crate::test_support::pane_id("a"),
            edge: Edge::Right,
        };
        let expected = crate::LayoutEvent::DropTab {
            tab: layout::TabId(2),
            onto: layout::TabId(1),
            placement,
        };
        assert_eq!(opened.borrow().clone(), vec![expected]);
    }

    #[gpui::test]
    fn the_shown_tab_dragged_over_its_own_panes_drops_nothing(cx: &mut TestAppContext) {
        let layout = two_tabs();
        let harness = open(cx, &layout);
        let (view, cx, opened) = (harness.view, harness.cx, harness.opened);
        let previewed = view.update(cx, |view, cx| {
            let centre = view.bounds.center();
            view.tab_dragged_over(layout::TabId(1), centre, cx);
            let previewed = view.drop.preview(view.clock.now(), ui::Motion::default());
            view.tab_dropped(layout::TabId(1), centre, cx);
            previewed
        });
        assert_eq!(previewed, None, "no preview over its own panes");
        assert!(opened.borrow().is_empty(), "no drop");
    }

    #[gpui::test]
    fn a_cancelled_view_tab_drag_clears_its_preview(cx: &mut TestAppContext) {
        let layout = two_tabs();
        let harness = open(cx, &layout);
        let (view, cx) = (harness.view, harness.cx);
        let dragging = view.update(cx, |view, cx| {
            view.tab_dragged_over(layout::TabId(2), view.bounds.center(), cx);
            view.tab_drag_cancelled(cx);
            view.drop.is_dragging()
        });
        assert!(!dragging, "the drag is over");
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

    #[gpui::test]
    fn a_chat_row_released_on_the_empty_stage_asks_to_open_it_there(cx: &mut TestAppContext) {
        let layout = layout::Layout::default();
        let harness = crate::test_support::open_under_row_source(cx, &layout);
        let (view, cx, opened) = (harness.view, harness.cx, harness.opened);
        let centre = view.read_with(cx, |view, _| view.bounds.center());
        let none = gpui::Modifiers::none();
        let left = gpui::MouseButton::Left;
        cx.simulate_mouse_down(point(px(4.0), px(4.0)), left, none);
        cx.simulate_mouse_move(point(px(40.0), px(40.0)), left, none);
        cx.simulate_mouse_move(centre, left, none);
        cx.simulate_mouse_up(centre, left, none);
        let expected = crate::LayoutEvent::StageChat { chat: "t1".into() };
        assert_eq!(
            opened.borrow().clone(),
            vec![expected],
            "open t1 on the stage"
        );
    }
}
