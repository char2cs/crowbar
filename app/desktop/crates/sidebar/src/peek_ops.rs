//! Summoning and dismissing the peek card.

use gpui::{Context, Pixels, Point, Window};
use ui::Motion;

use crate::metrics::PEEK_INSET;
use crate::panel::Panel;
use crate::peek_zone::PeekZone;
use crate::presentation::Presentation;
use crate::sidebar::Sidebar;
use crate::visibility::Visibility;

impl Sidebar {
    /// Slides the peek card in while the sidebar is closed. The shell calls this when the pointer
    /// rests on its sidebar toggle.
    pub fn begin_peek(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        self.slide_peek(1.0, cx);
    }

    /// Slides the peek card out again.
    pub fn end_peek(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        self.slide_peek(0.0, cx);
    }

    pub(crate) fn pointer_left_window(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        if self.overlay_open() || !self.peek.is_wanted() {
            return;
        }
        self.end_peek(cx);
    }

    fn overlay_open(&self) -> bool {
        self.menu.is_some() || self.tree.confirmation().is_some()
    }

    fn slide_peek(
        &mut self,
        target: f32,
        cx: &mut Context<'_, Self>,
    ) {
        if self.panel.visibility() == Visibility::Open {
            return;
        }
        self.peek.slide_to(target, Motion::of(cx));
        cx.notify();
    }

    pub(crate) fn track_peek(
        &mut self,
        at: Point<Pixels>,
        window: &Window,
        cx: &mut Context<'_, Self>,
    ) {
        if self.overlay_open() {
            return;
        }
        let container = Self::container_width(window);
        let closed = self.panel.visibility() == Visibility::Closed;
        if !closed || Panel::presentation(container) == Presentation::Overlay {
            return;
        }
        let size = window.viewport_size();
        let zone = PeekZone {
            window: (f32::from(size.width), f32::from(size.height)),
            card_width: self.panel.preferred().min(container - 2.0 * PEEK_INSET),
            side: self.panel.side(),
        };
        let (x, y) = (f32::from(at.x), f32::from(at.y));
        match (
            zone.summons(x, y) || (self.peek.is_wanted() && zone.keeps(x, y)),
            self.peek.is_wanted(),
        ) {
            (true, false) => self.begin_peek(cx),
            (false, true) => self.end_peek(cx),
            _ => {},
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::prelude::*;

    fn closed_sidebar(
        cx: &mut TestAppContext
    ) -> (gpui::Entity<Sidebar>, &mut gpui::VisualTestContext) {
        let (_, sidebar, cx) = open(cx, DemoData::reference());
        sidebar.update(cx, Sidebar::toggle);
        (sidebar, cx)
    }

    #[gpui::test]
    fn the_pointer_on_the_left_edge_summons_the_card(cx: &mut TestAppContext) {
        let (sidebar, cx) = closed_sidebar(cx);
        sidebar.update_in(cx, |this, window, cx| {
            this.track_peek(point(px(2.0), px(300.0)), window, cx);
        });
        assert!(sidebar.read_with(cx, |this, _| this.peek.is_wanted()));
    }

    #[gpui::test]
    fn leaving_the_card_dismisses_it(cx: &mut TestAppContext) {
        let (sidebar, cx) = closed_sidebar(cx);
        sidebar.update_in(cx, |this, window, cx| {
            this.track_peek(point(px(2.0), px(300.0)), window, cx);
            this.track_peek(point(px(900.0), px(300.0)), window, cx);
        });
        assert!(!sidebar.read_with(cx, |this, _| this.peek.is_wanted()));
    }

    #[gpui::test]
    fn an_open_sidebar_ignores_peeking(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoData::reference());
        sidebar.update(cx, Sidebar::begin_peek);
        assert!(!sidebar.read_with(cx, |this, _| this.peek.is_wanted()));
    }

    #[gpui::test]
    fn the_shell_can_begin_and_end_a_peek(cx: &mut TestAppContext) {
        let (sidebar, cx) = closed_sidebar(cx);
        sidebar.update(cx, Sidebar::begin_peek);
        assert!(sidebar.read_with(cx, |this, _| this.peek.is_wanted()));
        sidebar.update(cx, Sidebar::end_peek);
        assert!(!sidebar.read_with(cx, |this, _| this.peek.is_wanted()));
    }

    #[gpui::test]
    fn leaving_the_window_dismisses_the_card(cx: &mut TestAppContext) {
        let (sidebar, cx) = closed_sidebar(cx);
        sidebar.update(cx, Sidebar::begin_peek);
        sidebar.update(cx, Sidebar::pointer_left_window);
        assert!(!sidebar.read_with(cx, |this, _| this.peek.is_wanted()));
    }

    #[gpui::test]
    fn an_open_menu_keeps_the_card_when_the_pointer_wanders(cx: &mut TestAppContext) {
        let (sidebar, cx) = closed_sidebar(cx);
        sidebar.update(cx, Sidebar::begin_peek);
        sidebar.update_in(cx, |this, window, cx| {
            let entry = crate::menu_entry::MenuEntry::action(
                "Keep",
                ui::IconName::Check,
                crate::intent::Intent::Keep(RowId::from("chat")),
            );
            this.open_menu((vec![entry], point(px(10.0), px(10.0))), window, cx);
            this.pointer_left_window(cx);
        });
        assert!(sidebar.read_with(cx, |this, _| this.peek.is_wanted()));
    }
}
