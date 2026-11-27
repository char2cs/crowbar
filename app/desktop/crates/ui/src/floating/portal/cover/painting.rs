use std::collections::HashMap;

use gpui::{App, Bounds, EntityId, Global, Pixels};

#[derive(Default)]
pub(crate) struct Painting {
    within: Vec<EntityId>,
    cards: HashMap<EntityId, Vec<Bounds<Pixels>>>,
}

impl Global for Painting {}

impl Painting {
    pub(crate) fn enter(
        slot: EntityId,
        cx: &mut App,
    ) {
        let painting = cx.default_global::<Self>();
        painting.cards.insert(slot, Vec::new());
        painting.within.push(slot);
    }

    pub(crate) fn resume(
        slot: EntityId,
        cx: &mut App,
    ) {
        cx.default_global::<Self>().within.push(slot);
    }

    pub(crate) fn leave(cx: &mut App) {
        cx.default_global::<Self>().within.pop();
    }

    pub(crate) fn current(cx: &App) -> Option<EntityId> {
        cx.try_global::<Self>()?.within.last().copied()
    }

    pub(crate) fn note(
        slot: EntityId,
        card: Bounds<Pixels>,
        cx: &mut App,
    ) {
        if let Some(cards) = cx.default_global::<Self>().cards.get_mut(&slot) {
            cards.push(card);
        }
    }

    pub(crate) fn take(
        slot: EntityId,
        cx: &mut App,
    ) -> Vec<Bounds<Pixels>> {
        cx.default_global::<Self>()
            .cards
            .remove(&slot)
            .unwrap_or_default()
    }
}

#[cfg(test)]
mod tests {
    use gpui::{AppContext as _, Entity, TestAppContext, bounds, point, px, size};

    use super::*;

    #[gpui::test]
    fn a_card_reaches_the_overlay_it_is_laid_out_in_and_is_taken_once(cx: &mut TestAppContext) {
        cx.update(|cx| {
            let slots: [Entity<()>; 2] = [cx.new(|_| ()), cx.new(|_| ())];
            let [outer, inner] = slots.map(|slot| slot.entity_id());
            let card = bounds(point(px(1.0), px(2.0)), size(px(3.0), px(4.0)));
            Painting::enter(outer, cx);
            Painting::enter(inner, cx);
            Painting::leave(cx);
            let current = Painting::current(cx);
            Painting::note(outer, card, cx);
            Painting::leave(cx);
            assert_eq!(current, Some(outer), "back in the outer overlay");
            assert_eq!(Painting::take(outer, cx), [card], "its card");
            assert_eq!(Painting::take(outer, cx), [], "taken once");
            assert_eq!(
                Painting::take(inner, cx),
                [],
                "nothing painted in the inner one"
            );
        });
    }
}
