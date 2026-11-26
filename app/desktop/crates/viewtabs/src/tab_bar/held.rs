use gpui::{Context, Pixels, Point};
use layout::Command;
use ui::Motion;

use crate::gesture::{HeldGap, Settle};
use crate::span::Span;
use crate::strip_metrics::{ROW_HEIGHT, SHELL_MARGIN, TAB_HEIGHT, slot_width, slots};
use crate::tab_bar::TabBar;

impl TabBar {
    pub(crate) fn hold_gap(
        &mut self,
        position: Point<Pixels>,
        cx: &mut Context<'_, Self>,
    ) {
        let spans = self.slot_spans();
        if spans.is_empty() {
            return;
        }
        let (now, motion) = ((self.clock)(), Motion::of(cx));
        let (x, visible) = (self.content_x(position), self.visible_span());
        let (inside, count) = (self.row.contains(&position), spans.len());
        let pitch = slot_width(count + 1, self.strip_width());
        let gap = self
            .gap
            .get_or_insert_with(|| HeldGap::new(position, pitch, count));
        gap.pointer = position;
        gap.aim(inside.then_some(x), &spans, visible);
        gap.slide_to(now, motion);
        cx.notify();
    }

    pub(crate) fn held_offset(&self) -> Option<(f32, f32)> {
        let gap = self.gap.as_ref()?;
        gap.target()?;
        let (pointer, half) = (gap.pointer, gap.width() / 2.0);
        let x = self.content_x(pointer);
        let dx = gap.pinned_left(x, &self.slot_spans(), self.visible_span()) - (x - half);
        let dy = self.tabs_top() - (f32::from(pointer.y) - TAB_HEIGHT / 2.0);
        let pinned = 1.0 - (self.outside_tabs(pointer) / TAB_HEIGHT).min(1.0);
        Some((dx * pinned, dy * pinned))
    }

    pub(crate) fn take_held(&mut self) -> Option<(usize, f32)> {
        let (dx, _) = self.held_offset()?;
        let gap = self.gap.take()?;
        let to = gap.target()?;
        Some((to, self.content_x(gap.pointer) - gap.width() / 2.0 + dx))
    }

    pub(crate) fn settle_held(
        &mut self,
        (to, left): (usize, f32),
        commands: &[Command],
        cx: &Context<'_, Self>,
    ) {
        let Some(tab) = commands.iter().find_map(|command| match command {
            Command::MoveTab { tab, .. } => Some(*tab),
            _ => None,
        }) else {
            return;
        };
        let joined = slots(self.tabs.len() + 1, self.strip_width());
        let slot = joined.get(to).map_or(0.0, |span| span.x);
        let from = left - slot - SHELL_MARGIN;
        self.settle = Some(Settle::new(tab, from, (self.clock)(), Motion::of(cx)));
    }

    fn visible_span(&self) -> Span {
        Span {
            x: -f32::from(self.scroll.offset().x),
            width: self.strip_width(),
        }
    }

    fn tabs_top(&self) -> f32 {
        f32::from(self.row.origin.y) + (ROW_HEIGHT - TAB_HEIGHT) / 2.0
    }

    fn outside_tabs(
        &self,
        pointer: Point<Pixels>,
    ) -> f32 {
        let (left, top) = (f32::from(self.strip_left()), self.tabs_top());
        let across = outside(f32::from(pointer.x), left, left + self.strip_width());
        across.max(outside(f32::from(pointer.y), top, top + TAB_HEIGHT))
    }
}

fn outside(
    value: f32,
    low: f32,
    high: f32,
) -> f32 {
    (low - value).max(value - high).max(0.0)
}

#[cfg(test)]
mod tests {
    use gpui::{TestAppContext, point, px, size};
    use layout::DragSubject;
    use protocol::PaneId;

    use crate::test_support::{open, view};

    fn held_offset(
        at: (f32, f32),
        cx: &mut TestAppContext,
    ) -> Option<(f32, f32)> {
        let harness = open(cx, vec![view(1, &["a"]), view(2, &["b", "c"])], 1);
        let (bar, cx) = (harness.bar, harness.cx);
        cx.simulate_resize(size(px(616.0), px(400.0)));
        cx.run_until_parked();
        let subject = DragSubject::Pane(PaneId::new("c").expect("id"));
        bar.update(cx, |bar, cx| {
            bar.hover_at((point(px(at.0), px(at.1)), Some(&subject)), cx);
        });
        bar.read_with(cx, |bar, _| bar.held_offset())
    }

    #[gpui::test]
    fn a_member_held_past_the_first_tab_stays_over_it(cx: &mut TestAppContext) {
        let offset = held_offset((8.0 + 10.0, 21.0), cx);
        assert_eq!(offset, Some((2.0 + 88.0, 0.0)), "pushed right");
    }

    #[gpui::test]
    fn a_member_held_off_the_strip_follows_the_pointer_freely(cx: &mut TestAppContext) {
        assert_eq!(held_offset((300.0, 200.0), cx), None, "free");
    }
}
