//! What the overlays drawn in a window cover, so that no native view draws over them.

pub(crate) mod covering;
mod covers;
mod entry;
pub(crate) mod opening;
pub(crate) mod painting;

use std::collections::BTreeMap;

use gpui::{App, Bounds, Context, Entity, EntityId, Pixels, Window};

use crate::floating::portal::float::Float;
use crate::foundation::opaque_debug;

pub(crate) use entry::{Away, Entry};

/// The overlays open in one window that the window draws itself: dialogs, popovers and drags,
/// each with the cards it painted. It notifies whenever what they cover changes, and only then.
#[derive(Default)]
pub struct Cover {
    open: BTreeMap<EntityId, Entry>,
}

opaque_debug!(Cover);

impl Cover {
    /// The cover of `window`, made on first use.
    pub fn of(
        window: &Window,
        cx: &mut App,
    ) -> Entity<Self> {
        covers::of(window, cx)
    }

    /// Whether a dialog or a drag is open, which covers the whole window.
    pub fn whole(&self) -> bool {
        self.open
            .values()
            .any(|entry| matches!(entry.float, Float::Modal | Float::Drag))
    }

    /// Whether a drag is in progress.
    pub fn dragging(&self) -> bool {
        self.open.values().any(|entry| entry.float == Float::Drag)
    }

    /// The cards the open popovers painted, in window coordinates.
    pub fn cards(&self) -> Vec<Bounds<Pixels>> {
        self.open
            .values()
            .filter(|entry| entry.float == Float::Popover)
            .flat_map(|entry| entry.cards.iter().copied())
            .collect()
    }

    /// Runs the click-away layers open in `window`, for a press a native view took outside
    /// every card, once the caller's update is done, as a press in the window would.
    pub fn press_outside(
        window: &mut Window,
        cx: &mut App,
    ) {
        let cover = Self::of(window, cx);
        let aways: Vec<Away> = cover
            .read(cx)
            .open
            .values()
            .filter_map(|entry| entry.away.clone())
            .collect();
        window.defer(cx, move |window, cx| run(&aways, window, cx));
    }

    pub(crate) fn open(
        &mut self,
        slot: EntityId,
        (float, away): (Float, Option<Away>),
        cx: &mut Context<'_, Self>,
    ) {
        let known = self.open.get(&slot).map(|entry| entry.float);
        let entry = self.open.entry(slot).or_insert_with(|| Entry {
            float,
            cards: Vec::new(),
            away: None,
        });
        entry.float = float;
        entry.away = away;
        if known != Some(float) {
            cx.notify();
        }
    }

    pub(crate) fn painted(
        &mut self,
        slot: EntityId,
        cards: Vec<Bounds<Pixels>>,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(entry) = self
            .open
            .get_mut(&slot)
            .filter(|entry| entry.cards != cards)
        else {
            return;
        };
        entry.cards = cards;
        cx.notify();
    }

    pub(crate) fn close(
        &mut self,
        slot: EntityId,
        cx: &mut Context<'_, Self>,
    ) {
        if self.open.remove(&slot).is_some() {
            cx.notify();
        }
    }
}

fn run(
    aways: &[Away],
    window: &mut Window,
    cx: &mut App,
) {
    for away in aways {
        away(window, cx);
    }
}

#[cfg(test)]
mod tests {
    use std::cell::Cell;
    use std::rc::Rc;

    use gpui::{ParentElement as _, TestAppContext, div, px};

    use super::*;
    use crate::Portal;
    use crate::test_support::floats::{covered, floated};
    use crate::test_support::probe::{open, settle};

    fn cover_of(cx: &mut gpui::VisualTestContext) -> Entity<Cover> {
        cx.update(|window, cx| Cover::of(window, cx))
    }

    #[gpui::test]
    fn a_dialog_or_a_drag_covers_the_whole_window_and_a_popover_only_its_card(
        cx: &mut TestAppContext
    ) {
        let cases = [
            (Float::Modal, (true, false)),
            (Float::Drag, (true, true)),
            (Float::Popover, (false, false)),
        ];
        for (float, (whole, dragging)) in cases {
            let window = open(cx, move |_, _| div().child(popover(float)));
            let cover = cover_of(window);
            let read = window.update(|_, cx| (cover.read(cx).whole(), cover.read(cx).dragging()));
            assert_eq!(read, (whole, dragging), "{float:?}");
        }
    }

    fn counting_away(count: &Rc<Cell<usize>>) -> impl gpui::IntoElement {
        let count = Rc::clone(count);
        crate::floating::popup::click_away("away", move |_, _| count.set(count.get() + 1))
    }

    fn popover(float: Float) -> Portal {
        Portal::here("p", |_, _| crate::Shaped::solid(floated(), px(0.0))).float(float)
    }

    #[gpui::test]
    fn an_unseen_click_away_layer_covers_nothing_and_a_press_outside_runs_it(
        cx: &mut TestAppContext
    ) {
        let pressed = Rc::new(Cell::new(0));
        let count = Rc::clone(&pressed);
        let window = open(cx, move |_, _| div().child(counting_away(&count)));
        assert_eq!(covered(window), Vec::new(), "nothing seen");
        window.update(Cover::press_outside);
        assert_eq!(pressed.get(), 1);
    }

    #[gpui::test]
    fn a_cover_notifies_once_for_what_opens_and_not_for_each_frame(cx: &mut TestAppContext) {
        let window = open(cx, |_, _| div().child(popover(Float::Popover)));
        let cover = cover_of(window);
        let notified = Rc::new(Cell::new(0));
        let count = Rc::clone(&notified);
        let bump = move |_, _: &mut App| count.set(count.get() + 1);
        let _watch = window.update(|_, cx| cx.observe(&cover, bump));
        window.update(|window, _| window.refresh());
        settle(window);
        assert_eq!(notified.get(), 0, "a redraw of the same popover");
    }
}
