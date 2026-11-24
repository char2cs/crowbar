use gpui::{Context, DragMoveEvent, Pixels, Point, Window};
use layout::{Command, DragSubject};
use protocol::PaneId;
use ui::{DraggedRow, Motion};

use crate::event::Event;
use crate::gesture::IncomingDrop;
use crate::gesture::TabDrop;
use crate::tab_bar::TabBar;

impl TabBar {
    pub(crate) fn drag_over(
        &mut self,
        event: &DragMoveEvent<DragSubject>,
        _: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        self.hover_at(event.event.position, cx);
    }

    pub(crate) fn row_over(
        &mut self,
        event: &DragMoveEvent<DraggedRow>,
        _: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        if event.drag(cx).chat.is_some() {
            self.hover_at(event.event.position, cx);
        }
    }

    pub(crate) fn row_dropped(
        &mut self,
        row: &DraggedRow,
        _: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let incoming = self.incoming.take();
        cx.notify();
        let (Some(incoming), Some(chat)) = (incoming, row.chat.as_deref()) else {
            return;
        };
        cx.emit(Event::DropChat {
            chat: chat.to_owned(),
            at: incoming.drop,
        });
    }

    /// The commands that move the open chat in `pane` to `at`, as if its pane had been dragged
    /// there: a sidebar row of an open chat drops as its pane.
    pub fn pane_drop(
        &mut self,
        pane: &PaneId,
        at: TabDrop,
    ) -> Vec<Command> {
        let subject = DragSubject::Pane(pane.clone());
        let new_tab = self.allocate_tab(self.ghost_title(&subject));
        at.commands(&subject, new_tab, &self.tabs)
    }

    fn hover_at(
        &mut self,
        position: Point<Pixels>,
        cx: &mut Context<'_, Self>,
    ) {
        let hovering = self.row.contains(&position);
        let drop = hovering.then(|| self.drop_at(position)).flatten();
        let Some(drop) = drop else {
            self.incoming = None;
            cx.notify();
            return;
        };
        let line = self.line_x(drop);
        let (now, motion) = (self.clock.now(), Motion::of(cx));
        match &mut self.incoming {
            Some(incoming) => incoming.move_to(drop, line, (now, motion)),
            None => self.incoming = Some(IncomingDrop::new(drop, line)),
        }
        cx.notify();
    }

    pub(crate) fn dropped(
        &mut self,
        subject: &DragSubject,
        _: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(incoming) = self.incoming.take() else {
            return;
        };
        let title = self.ghost_title(subject);
        let new_tab = self.allocate_tab(title);
        let commands = incoming.drop.commands(subject, new_tab, &self.tabs);
        cx.notify();
        if !commands.is_empty() {
            cx.emit(Event::Apply(commands));
        }
    }

    fn drop_at(
        &self,
        position: Point<Pixels>,
    ) -> Option<TabDrop> {
        let ids: Vec<_> = self.tabs.iter().map(|tab| tab.id).collect();
        TabDrop::at(self.content_x(position), &self.slot_spans(), &ids)
    }

    fn line_x(
        &self,
        drop: TabDrop,
    ) -> f32 {
        let spans = self.slot_spans();
        let index = match drop {
            TabDrop::Reorder(index) => index,
            TabDrop::Merge(tab) => crate::tab_bar::position(&self.tabs, tab).unwrap_or(0),
        };
        spans.get(index).map_or_else(
            || spans.last().map_or(0.0, |span| span.end()),
            |span| span.x,
        )
    }

    fn ghost_title(
        &self,
        subject: &DragSubject,
    ) -> String {
        match subject {
            DragSubject::Pane(pane) => self
                .tabs
                .iter()
                .flat_map(|tab| &tab.members)
                .find(|member| &member.pane == pane)
                .map_or_else(String::new, |member| member.label.to_string()),
            DragSubject::Open(_) | DragSubject::Chat(_) | DragSubject::Tab(_) => String::new(),
        }
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;
    use layout::{Command, DragSubject, TabId};
    use protocol::PaneId;

    use crate::event::Event;
    use crate::gesture::IncomingDrop;
    use crate::gesture::TabDrop;
    use crate::test_support::{ROW_SOURCE_Y, open, open_under_row_source, view};

    #[gpui::test]
    fn dropping_a_pane_on_a_tab_asks_to_merge_it(cx: &mut TestAppContext) {
        let harness = open(cx, vec![view(1, &["a"]), view(2, &["b", "c"])], 1);
        let (bar, cx, events) = (harness.bar, harness.cx, harness.events);
        bar.update(cx, |bar, _| {
            bar.incoming = Some(IncomingDrop::new(TabDrop::Merge(TabId(1)), 0.0));
        });
        let subject = DragSubject::Pane(PaneId::new("c").expect("id"));
        bar.update_in(cx, |bar, window, cx| bar.dropped(&subject, window, cx));
        let seen = events.borrow();
        let expected = Command::MovePaneToTab {
            pane: PaneId::new("c").expect("id"),
            to: TabId(1),
            placement: None,
        };
        assert_eq!(
            seen.as_slice(),
            &[Event::Apply(vec![expected])],
            "merge command"
        );
        bar.read_with(cx, |bar, _| {
            assert!(bar.incoming.is_none(), "marker consumed");
        });
    }

    #[gpui::test]
    fn dropping_a_chat_between_tabs_creates_a_tab_with_a_fresh_id(cx: &mut TestAppContext) {
        let harness = open(cx, vec![view(1, &["a"])], 1);
        let (bar, cx, events) = (harness.bar, harness.cx, harness.events);
        bar.update(cx, |bar, _| {
            bar.incoming = Some(IncomingDrop::new(TabDrop::Reorder(0), 0.0));
        });
        let subject = DragSubject::Open(instance());
        bar.update_in(cx, |bar, window, cx| bar.dropped(&subject, window, cx));
        let seen = events.borrow();
        let Event::Apply(commands) = &seen[0] else {
            panic!("an apply event");
        };
        assert!(
            matches!(commands[0], Command::AddTab { id: TabId(50), .. }),
            "first free id: {commands:?}"
        );
    }

    fn chat_row_dropped_at(
        x: f32,
        cx: &mut TestAppContext,
    ) -> Vec<Event> {
        let harness = open_under_row_source(cx, vec![view(1, &["a"]), view(2, &["b"])], 1);
        let (cx, events) = (harness.cx, harness.events);
        let none = gpui::Modifiers::none();
        let (source, over) = (
            gpui::point(gpui::px(4.0), gpui::px(ROW_SOURCE_Y + 4.0)),
            gpui::point(gpui::px(x), gpui::px(21.0)),
        );
        cx.simulate_mouse_down(source, gpui::MouseButton::Left, none);
        cx.simulate_mouse_move(over, gpui::MouseButton::Left, none);
        cx.simulate_mouse_move(over, gpui::MouseButton::Left, none);
        cx.simulate_mouse_up(over, gpui::MouseButton::Left, none);
        events.borrow().clone()
    }

    #[gpui::test]
    fn a_chat_row_dropped_on_a_tab_middle_asks_to_join_that_tab(cx: &mut TestAppContext) {
        let expected = Event::DropChat {
            chat: "t9".into(),
            at: TabDrop::Merge(TabId(1)),
        };
        assert_eq!(
            chat_row_dropped_at(8.0 + 120.0, cx),
            vec![expected],
            "joins tab one"
        );
    }

    #[gpui::test]
    fn a_chat_row_dropped_on_a_tab_edge_asks_to_move_beside_it(cx: &mut TestAppContext) {
        let expected = Event::DropChat {
            chat: "t9".into(),
            at: TabDrop::Reorder(1),
        };
        assert_eq!(
            chat_row_dropped_at(8.0 + 240.0 + 20.0, cx),
            vec![expected],
            "before tab two"
        );
    }

    fn instance() -> layout::PaneInstance {
        layout::PaneInstance {
            pane_id: PaneId::new("new").expect("id"),
            type_id: protocol::PaneTypeId::new("core.chat").expect("type"),
            params: protocol::Params::default(),
            state: protocol::PaneState::default(),
        }
    }
}
