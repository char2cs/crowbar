//! The element through which a floating element paints above the rest of its window.

use std::rc::Rc;

use gpui::{
    AnyElement, App, Bounds, ElementId, Entity, IntoElement, Pixels, RenderOnce, Styled, Window,
    canvas, div,
};

use crate::floating::layers::{Owners, Registry, Slot, place};
use crate::floating::portal::reach::Reach;
use crate::foundation::opaque_debug;
use crate::foundation::tokens::layer as paint_order;

pub(crate) mod float;
pub(crate) mod layer;
pub(crate) mod reach;

pub use float::Float;
use layer::{Build, Layer};

/// Paints what it builds above the rest of its window, either where the portal itself sits or
/// over the whole window. With a [`crate::FloatHost`] installed it paints in one of the
/// window's float windows ([`Float`]), so native web views cannot cover it; otherwise, and in
/// a float window itself, it paints in its own window.
///
/// The content is built in the window that paints it, so it captures entities and values,
/// not elements.
#[derive(IntoElement)]
pub struct Portal {
    id: ElementId,
    reach: Reach,
    priority: usize,
    float: Float,
    build: Build,
}

impl Portal {
    /// Paints `build` at the portal's own bounds, as an anchored popup does below its trigger.
    pub fn here<E: IntoElement>(
        id: impl Into<ElementId>,
        build: impl Fn(&mut Window, &mut App) -> E + 'static,
    ) -> Self {
        Self::new(id.into(), Reach::Here, build)
    }

    pub(crate) fn here_sized<E: IntoElement>(
        id: impl Into<ElementId>,
        build: impl Fn(Bounds<Pixels>, &mut Window, &mut App) -> E + 'static,
    ) -> Self {
        let build: Build =
            Rc::new(move |bounds, window, cx| build(bounds, window, cx).into_any_element());
        Self::built(id.into(), Reach::Here, build)
    }

    /// Paints `build` at the portal's own bounds like [`Portal::here`], for the view a drag
    /// shows under the pointer; it paints in place, not deferred.
    pub fn lifted<E: IntoElement>(
        id: impl Into<ElementId>,
        build: impl Fn(&mut Window, &mut App) -> E + 'static,
    ) -> Self {
        Self::new(id.into(), Reach::Lifted, build)
    }

    /// Paints `build` over the whole window, its origin at the window's top-left corner.
    pub fn window<E: IntoElement>(
        id: impl Into<ElementId>,
        build: impl Fn(&mut Window, &mut App) -> E + 'static,
    ) -> Self {
        Self::new(id.into(), Reach::Window, build)
    }

    /// Paints `build` over the whole window like [`Portal::window`], but in place rather than
    /// deferred, so `build` may hold portals of its own: make it
    /// the last child of the window's root, as a modal layer is.
    pub fn cover<E: IntoElement>(
        id: impl Into<ElementId>,
        build: impl Fn(&mut Window, &mut App) -> E + 'static,
    ) -> Self {
        Self::new(id.into(), Reach::Cover, build)
    }

    fn new<E: IntoElement>(
        id: ElementId,
        reach: Reach,
        build: impl Fn(&mut Window, &mut App) -> E + 'static,
    ) -> Self {
        let build: Build = Rc::new(move |_, window, cx| build(window, cx).into_any_element());
        Self::built(id, reach, build)
    }

    fn built(
        id: ElementId,
        reach: Reach,
        build: Build,
    ) -> Self {
        let float = match reach {
            Reach::Lifted => Float::Drag,
            Reach::Here | Reach::Window | Reach::Cover => Float::Passive,
        };
        Self {
            id,
            reach,
            priority: paint_order::OVERLAY,
            float,
            build,
        }
    }

    /// Sets the float window it paints in; it is [`Float::Passive`] unless lifted.
    #[must_use]
    pub fn float(
        mut self,
        float: Float,
    ) -> Self {
        self.float = float;
        self
    }

    /// Sets the paint order among floating elements (see [`crate::layer`]); larger is on top.
    #[must_use]
    pub fn priority(
        mut self,
        priority: usize,
    ) -> Self {
        self.priority = priority;
        self
    }

    fn layer(&self) -> Layer {
        Layer {
            reach: self.reach,
            priority: self.priority,
            build: Rc::clone(&self.build),
            bounds: Bounds::default(),
        }
    }
}

impl RenderOnce for Portal {
    fn render(
        self,
        window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        let layer = self.layer();
        let Some(owner) = Owners::of(window, cx) else {
            return self.unowned(&layer, window, cx);
        };
        let held = owner.clone();
        let slot: Entity<Slot> = window.use_keyed_state(self.id, cx, |_, cx| Slot::new(&held, cx));
        let key = (slot.entity_id(), self.float);
        canvas(
            move |bounds, _, cx| place(&owner, (key, Layer { bounds, ..layer }), cx),
            |_, (), _, _| {},
        )
        .absolute()
        .size_full()
        .into_any_element()
    }
}

impl Portal {
    fn unowned(
        self,
        layer: &Layer,
        window: &mut Window,
        cx: &mut App,
    ) -> AnyElement {
        let lifted_away = layer.reach == Reach::Lifted && Registry::host(cx).is_some();
        if lifted_away {
            return div().into_any_element();
        }
        window.with_element_namespace(self.id, |window| layer.inline(window, cx))
    }
}

opaque_debug!(Portal);

#[cfg(test)]
mod tests {
    use gpui::{IntoElement, ParentElement, Styled, TestAppContext, div, point, px};

    use super::*;
    use crate::test_support::floats::floated;
    use crate::test_support::probe::open_floating;
    use crate::testing::newest_window;

    fn trigger(portal: Portal) -> impl IntoElement {
        div()
            .relative()
            .w(px(40.0))
            .h(px(20.0))
            .child(div().absolute().top_full().left_0().child(portal))
    }

    #[gpui::test]
    fn a_portal_paints_in_a_float_window_over_its_owner_and_not_in_the_owner(
        cx: &mut TestAppContext
    ) {
        let mut owner = open_floating(cx, |_, _| trigger(Portal::here("p", |_, _| floated())));
        let mut top = newest_window(&mut owner);
        assert_eq!(owner.debug_bounds("floated"), None, "nothing in the owner");
        let painted = top.debug_bounds("floated").unwrap_or_default();
        let anchor = owner.debug_bounds("probe").unwrap_or_default();
        assert_eq!(painted.origin, point(anchor.left(), anchor.bottom()));
    }

    #[gpui::test]
    fn without_a_float_host_the_window_paints_the_portal_itself(cx: &mut TestAppContext) {
        let cx = crate::test_support::probe::open(cx, |_, _| {
            trigger(Portal::here("p", |_, _| floated()))
        });
        assert!(cx.debug_bounds("floated").is_some());
    }
}
