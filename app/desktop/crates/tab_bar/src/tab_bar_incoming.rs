use std::time::Instant;

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
        let (now, motion) = (Instant::now(), Motion::of(cx));
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
        let new_tab = self.fresh_tab(title);
        let commands = incoming.drop.commands(subject, new_tab, &self.tabs);
        cx.notify();
        if !commands.is_empty() {
            cx.emit(TabBarEvent::Apply(commands));
        }
    }

    fn drop_at(&self, position: gpui::Point<gpui::Pixels>) -> Option<TabDrop> {
        let ids: Vec<_> = self.tabs.iter().map(|tab| tab.id).collect();
        TabDrop::at(self.content_x(position), &self.slot_spans(), &ids)
    }

    fn line_x(&self, drop: TabDrop) -> f32 {
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

    fn ghost_title(&self, subject: &DragSubject) -> String {
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
#[path = "tab_bar_incoming_tests.rs"]
mod tests;
