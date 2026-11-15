use gpui::{Context, DragMoveEvent, Window};
use model::DragSubject;
use ui::Motion;

use crate::incoming_drop::IncomingDrop;
use crate::tab_bar::TabBar;
use crate::tab_bar_event::TabBarEvent;
use crate::tab_drop::TabDrop;

impl TabBar {
    pub(crate) fn drag_over(
        &mut self,
        event: &DragMoveEvent<DragSubject>,
        _: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let hovering = self.row.contains(&event.event.position);
        let drop = hovering
            .then(|| self.drop_at(event.event.position))
            .flatten();
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
            cx.emit(TabBarEvent::Apply(commands));
        }
    }

    fn drop_at(
        &self,
        position: gpui::Point<gpui::Pixels>,
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
            DragSubject::Open(_) | DragSubject::Tab(_) => String::new(),
        }
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;
    use model::{Command, DragSubject, TabId};
    use protocol::PaneId;

    use crate::incoming_drop::IncomingDrop;
    use crate::tab_bar_event::TabBarEvent;
    use crate::tab_drop::TabDrop;
    use crate::test_support::{open, view};

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
            &[TabBarEvent::Apply(vec![expected])],
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
        let TabBarEvent::Apply(commands) = &seen[0] else {
            panic!("an apply event");
        };
        assert!(
            matches!(commands[0], Command::AddTab { id: TabId(50), .. }),
            "first free id: {commands:?}"
        );
    }

    fn instance() -> model::PaneInstance {
        model::PaneInstance {
            pane_id: PaneId::new("new").expect("id"),
            type_id: protocol::PaneTypeId::new("core.chat").expect("type"),
            params: protocol::Params::default(),
            state: protocol::PaneState::default(),
        }
    }
}
