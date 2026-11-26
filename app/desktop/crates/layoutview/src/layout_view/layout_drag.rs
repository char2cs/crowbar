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
        let Some(subject) = row_subject(event.drag(cx)) else {
            return;
        };
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
            now: (self.clock)(),
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
            .map_or_else(|| self.drop.stage_event(), |tab| self.drop.row_event(tab));
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

fn row_subject(row: &DraggedRow) -> Option<DragSubject> {
    let chat = row
        .chat
        .as_deref()
        .map(|chat| DragSubject::Chat(chat.to_owned()));
    chat.or_else(|| {
        row.file
            .as_deref()
            .map(|path| DragSubject::File(path.to_owned()))
    })
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;
    use layout::{Command, Edge, Layout, Placement, SpaceId, TabId};

    use crate::LayoutEvent;
    use crate::test_support::{
        chat_row, drag_row_to, file_row, layout_of, open, open_under_row_source, pane_id,
    };

    fn two_tabs() -> Layout {
        let mut layout = layout_of("a", &[]);
        let second = Command::AddTab {
            id: TabId(2),
            title: "Two".into(),
            space: SpaceId::default(),
        };
        layout.apply(second).expect("a second tab");
        layout
            .apply(Command::ActivateTab { tab: TabId(1) })
            .expect("tab one shown");
        layout
    }

    #[gpui::test]
    fn a_view_tab_drops_its_chats_on_a_pane_but_not_on_its_own_panes(cx: &mut TestAppContext) {
        let harness = open(cx, &two_tabs());
        let (view, cx, opened) = (harness.view, harness.cx, harness.opened);
        let centre = view.read_with(cx, |view, _| view.bounds.center());
        let own = view.update(cx, |view, cx| {
            view.tab_dragged_over(TabId(1), centre, cx);
            let previewed = view.drop.preview((view.clock)(), ui::Motion::default());
            view.tab_dropped(TabId(1), centre, cx);
            previewed
        });
        assert_eq!(own, None, "no preview over its own panes");
        let cancelled = view.update(cx, |view, cx| {
            view.tab_dragged_over(TabId(2), centre, cx);
            view.tab_drag_cancelled(cx);
            view.drop.is_dragging()
        });
        assert!(!cancelled, "a cancelled drag clears its preview");
        view.update(cx, |view, cx| view.tab_dragged_over(TabId(2), centre, cx));
        let none = gpui::Modifiers::none();
        cx.simulate_mouse_up(centre, gpui::MouseButton::Left, none);
        view.update(cx, |view, cx| view.tab_dropped(TabId(2), centre, cx));
        let placement = Placement {
            target: pane_id("a"),
            edge: Edge::Right,
        };
        let expected = LayoutEvent::DropTab {
            tab: TabId(2),
            onto: TabId(1),
            placement,
        };
        assert_eq!(*opened.borrow(), [expected], "resolved after the mouse up");
    }

    #[gpui::test]
    fn a_chat_row_released_on_the_empty_stage_opens_there_and_a_file_row_does_not(
        cx: &mut TestAppContext
    ) {
        let chat = LayoutEvent::StageChat { chat: "t1".into() };
        for (row, expected) in [(chat_row(), vec![chat]), (file_row("src/a.rs"), vec![])] {
            let harness = open_under_row_source(cx, &Layout::default(), row);
            let (view, cx, opened) = (harness.view, harness.cx, harness.opened);
            let centre = view.read_with(cx, |view, _| view.bounds.center());
            drag_row_to(cx, centre);
            assert_eq!(*opened.borrow(), expected);
        }
    }
}
