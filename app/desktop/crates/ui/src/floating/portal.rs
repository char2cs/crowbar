//! The element through which a floating element paints above the rest of its window.

use std::rc::Rc;

use gpui::{App, Bounds, ElementId, IntoElement, Pixels, RenderOnce, Window};

use crate::floating::portal::layer::{Build, Layer};
use crate::floating::portal::reach::Reach;
use crate::foundation::opaque_debug;
use crate::foundation::tokens::layer as paint_order;

mod layer;
mod reach;

/// Paints what it builds above the rest of its window, either where the portal itself sits or
/// over the whole window. It paints in the window itself, so native web views still cover it.
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
        Self {
            id,
            reach,
            priority: paint_order::OVERLAY,
            build,
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
        window.with_element_namespace(self.id, |window| layer.inline(window, cx))
    }
}

opaque_debug!(Portal);

#[cfg(test)]
mod tests {
    use gpui::{InteractiveElement, ParentElement, Point, Styled, TestAppContext, div, point, px};

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
    fn without_a_top_layer_the_window_paints_the_portal_itself(cx: &mut TestAppContext) {
        let cx = crate::test_support::probe::open(cx, |_, _| {
            trigger(Portal::here("p", |_, _| floated()))
        });
        assert!(cx.debug_bounds("floated").is_some());
    }
}
