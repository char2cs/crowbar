//! Wraps an overlay's content so the cards it paints reach the overlay's entry in its cover,
//! even from deferred content: a card learns its overlay while it is laid out, which content
//! built at prepaint does then.

use gpui::{
    AnyElement, App, Bounds, Element, ElementId, Entity, EntityId, GlobalElementId,
    InspectorElementId, IntoElement, LayoutId, Pixels, WeakEntity, Window,
};

use crate::floating::portal::cover::Cover;
use crate::floating::portal::cover::opening::Opening;
use crate::floating::portal::cover::painting::Painting;
use crate::floating::portal::layer::Layer;
use crate::floating::portal::slot::keyed;
use crate::foundation::opaque_debug;

pub(crate) fn covering(
    opening: Opening,
    layer: &Layer,
    window: &mut Window,
    cx: &mut App,
) -> AnyElement {
    let Opening(id, float, away) = opening;
    let cover = Cover::of(window, cx);
    let held = cover.downgrade();
    let release = move |slot, cx: &mut App| close(&held, slot, cx);
    let key = keyed((id.clone(), "cover"), release, (window, cx));
    cover.update(cx, |cover, cx| cover.open(key, (float, away), cx));
    window.with_element_namespace(id, |window| {
        let content = layer.inline(window, cx);
        Covering::new((key, cover), content).into_any_element()
    })
}

fn close(
    cover: &WeakEntity<Cover>,
    slot: EntityId,
    cx: &mut App,
) {
    if let Err(error) = cover.update(cx, |cover, cx| cover.close(slot, cx)) {
        tracing::debug!(%error, "the window of an overlay closed first");
    }
}

struct Covering {
    slot: EntityId,
    cover: Entity<Cover>,
    child: AnyElement,
}

impl Covering {
    fn new(
        (slot, cover): (EntityId, Entity<Cover>),
        child: impl IntoElement,
    ) -> Self {
        Self {
            slot,
            cover,
            child: child.into_any_element(),
        }
    }
}

opaque_debug!(Covering);

impl Element for Covering {
    type RequestLayoutState = ();
    type PrepaintState = ();

    fn id(&self) -> Option<ElementId> {
        None
    }

    fn source_location(&self) -> Option<&'static core::panic::Location<'static>> {
        None
    }

    fn request_layout(
        &mut self,
        _: Option<&GlobalElementId>,
        _: Option<&InspectorElementId>,
        window: &mut Window,
        cx: &mut App,
    ) -> (LayoutId, Self::RequestLayoutState) {
        Painting::enter(self.slot, cx);
        let layout = self.child.request_layout(window, cx);
        Painting::leave(cx);
        let (slot, cover) = (self.slot, self.cover.clone());
        window.defer(cx, move |_, cx| {
            let cards = Painting::take(slot, cx);
            cover.update(cx, |cover, cx| cover.painted(slot, cards, cx));
        });
        (layout, ())
    }

    fn prepaint(
        &mut self,
        _: Option<&GlobalElementId>,
        _: Option<&InspectorElementId>,
        _: Bounds<Pixels>,
        (): &mut Self::RequestLayoutState,
        window: &mut Window,
        cx: &mut App,
    ) -> Self::PrepaintState {
        Painting::resume(self.slot, cx);
        self.child.prepaint(window, cx);
        Painting::leave(cx);
    }

    fn paint(
        &mut self,
        _: Option<&GlobalElementId>,
        _: Option<&InspectorElementId>,
        _: Bounds<Pixels>,
        (): &mut Self::RequestLayoutState,
        (): &mut Self::PrepaintState,
        window: &mut Window,
        cx: &mut App,
    ) {
        self.child.paint(window, cx);
    }
}

#[cfg(test)]
mod tests {
    use gpui::{ParentElement as _, Styled as _, TestAppContext, div, px};

    use crate::test_support::floats::{covered, floated};
    use crate::test_support::probe::open;
    use crate::{Float, Portal};

    #[gpui::test]
    fn a_popover_covers_where_its_card_is_painted(cx: &mut TestAppContext) {
        let popover = || {
            Portal::here("p", |_, _| crate::Shaped::solid(floated(), px(0.0))).float(Float::Popover)
        };
        let window = open(cx, move |_, _| {
            div().pl(px(20.0)).pt(px(30.0)).child(popover())
        });
        let painted = window.debug_bounds("floated");
        assert_eq!(covered(window).first().copied(), painted);
    }
}
