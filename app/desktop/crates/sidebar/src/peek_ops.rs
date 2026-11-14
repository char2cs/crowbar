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
    pub fn begin_peek(&mut self, cx: &mut Context<'_, Self>) {
        self.slide_peek(1.0, cx);
    }

    /// Slides the peek card out again.
    pub fn end_peek(&mut self, cx: &mut Context<'_, Self>) {
        self.slide_peek(0.0, cx);
    }

    fn slide_peek(&mut self, target: f32, cx: &mut Context<'_, Self>) {
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
#[path = "peek_ops_tests.rs"]
mod tests;
