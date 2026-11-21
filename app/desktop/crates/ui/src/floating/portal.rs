//! The element through which a floating element reaches the top layer.

use std::rc::Rc;

use gpui::{App, ElementId, Entity, IntoElement, RenderOnce, Styled, Window, canvas, div};

use crate::floating::TopLayer;
use crate::floating::top_layer::{Build, Layer, Reach, Registry, Slot};
use crate::foundation::opaque_debug;
use crate::foundation::tokens::layer;

/// Paints what it builds in the window's [`crate::TopLayer`], above native web views, either
/// where the portal itself sits or over the whole window. A window without a top layer, such
/// as the top layer itself or a test window, paints it above the rest of the window instead.
///
/// The content is built in the window that paints it, so it captures entities and values,
/// not elements.
#[derive(IntoElement)]
pub struct Portal {
    id: ElementId,
    reach: Reach,
    priority: usize,
    build: Build,
}

impl Portal {
    /// Paints `build` at the portal's own bounds, as an anchored popup does below its trigger.
    pub fn here<E: IntoElement>(
        id: impl Into<ElementId>,
        build: impl Fn(&mut Window, &mut App) -> E + 'static,
    ) -> Self {
        Self::new(id.into(), Reach::Here(gpui::Bounds::default()), build)
    }

    /// Paints `build` at the portal's own bounds like [`Portal::here`], for the view a drag
    /// shows under the pointer. Every window draws the dragged view, so it paints only in the
    /// owner's top layer, or in place in a window without one.
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

    /// Paints `build` over the whole window like [`Portal::window`], but without a top layer it
    /// paints in place rather than deferred, so `build` may hold portals of its own: make it
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
        Self {
            id,
            reach,
            priority: layer::OVERLAY,
            build: Rc::new(move |window, cx| build(window, cx).into_any_element()),
        }
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
        let in_top_layer = window.root::<TopLayer>().flatten().is_some();
        if in_top_layer && layer.reach == Reach::Lifted {
            return div().into_any_element();
        }
        let Some(layers) = Registry::of(window, cx) else {
            return layer.inline(window, cx);
        };
        let held = Rc::clone(&layers);
        let slot: Entity<Slot> = window.use_keyed_state(self.id, cx, |_, cx| Slot::new(held, cx));
        let slot = slot.entity_id();
        canvas(
            move |bounds, window, cx| {
                let reach = match layer.reach {
                    Reach::Here(_) | Reach::Lifted => Reach::Here(bounds),
                    reach => reach,
                };
                let layer = Layer { reach, ..layer };
                layers.place((slot, layer), window.focused(cx), cx);
            },
            |_, (), _, _| {},
        )
        .absolute()
        .size_full()
        .into_any_element()
    }
}

opaque_debug!(Portal);

#[cfg(test)]
mod tests {
    use gpui::{
        InteractiveElement, MouseButton, ParentElement, Point, TestAppContext, div, point, px,
    };

    use super::*;
    use crate::test_support::probe::{open_floating, settle};

    fn floated() -> gpui::Div {
        div()
            .w(px(30.0))
            .h(px(10.0))
            .debug_selector(|| "floated".into())
    }

    fn trigger(portal: Portal) -> impl IntoElement {
        div()
            .relative()
            .w(px(40.0))
            .h(px(20.0))
            .child(div().absolute().top_full().left_0().child(portal))
    }

    #[gpui::test]
    fn a_portal_paints_in_the_top_layer_and_not_in_its_own_window(cx: &mut TestAppContext) {
        let (mut owner, mut top) =
            open_floating(cx, |_, _| trigger(Portal::here("p", |_, _| floated())));
        assert_eq!(owner.debug_bounds("floated"), None, "nothing in the owner");
        let painted = top.debug_bounds("floated").unwrap_or_default();
        let anchor = owner.debug_bounds("probe").unwrap_or_default();
        assert_eq!(painted.origin, point(anchor.left(), anchor.bottom()));
    }

    #[gpui::test]
    fn a_window_portal_covers_the_window_from_its_origin(cx: &mut TestAppContext) {
        let (_, mut top) = open_floating(cx, |_, _| trigger(Portal::window("p", |_, _| floated())));
        let painted = top.debug_bounds("floated").unwrap_or_default();
        assert_eq!(painted.origin, Point::default());
    }

    #[gpui::test]
    fn the_top_layer_takes_clicks_only_where_a_floating_element_does(cx: &mut TestAppContext) {
        let clickable =
            |_: &mut Window, _: &mut App| floated().on_mouse_down(MouseButton::Left, |_, _, _| {});
        let (_, mut top) = open_floating(cx, move |_, _| trigger(Portal::here("p", clickable)));
        let region = top.update(|window, _| window.input_region().to_vec());
        let painted = top.debug_bounds("floated").unwrap_or_default();
        assert_eq!(region, vec![painted]);
    }

    #[gpui::test]
    fn a_portal_that_stops_rendering_leaves_the_top_layer(cx: &mut TestAppContext) {
        let shown = Rc::new(std::cell::Cell::new(true));
        let showing = Rc::clone(&shown);
        let (mut owner, mut top) = open_floating(cx, move |_, _| {
            let portal = showing.get().then(|| Portal::here("p", |_, _| floated()));
            div().children(portal)
        });
        assert!(top.debug_bounds("floated").is_some());
        shown.set(false);
        owner.update(|window, _| window.refresh());
        settle(&mut owner);
        assert_eq!(top.debug_bounds("floated"), None);
    }

    #[gpui::test]
    fn the_top_layer_is_shown_only_while_a_portal_floats_something(cx: &mut TestAppContext) {
        let shown = Rc::new(std::cell::Cell::new(false));
        let showing = Rc::clone(&shown);
        let (mut owner, mut top) = open_floating(cx, move |_, _| {
            let portal = showing.get().then(|| Portal::here("p", |_, _| floated()));
            div().children(portal)
        });
        assert!(
            !top.update(|window, _| window.is_overlay_shown()),
            "idle: hidden"
        );
        shown.set(true);
        owner.update(|window, _| window.refresh());
        settle(&mut owner);
        assert!(
            top.update(|window, _| window.is_overlay_shown()),
            "floating: shown"
        );
        shown.set(false);
        owner.update(|window, _| window.refresh());
        settle(&mut owner);
        assert!(
            !top.update(|window, _| window.is_overlay_shown()),
            "idle again: hidden"
        );
    }

    #[gpui::test]
    fn without_a_top_layer_the_window_paints_the_portal_itself(cx: &mut TestAppContext) {
        let cx = crate::test_support::probe::open(cx, |_, _| {
            trigger(Portal::here("p", |_, _| floated()))
        });
        assert!(cx.debug_bounds("floated").is_some());
    }
}
