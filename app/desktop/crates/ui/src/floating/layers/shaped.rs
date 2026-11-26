//! An element that tells where a card is: to the pop-up window it paints in, which shows it and
//! lets the pointer reach it there, or to the cover of the overlay it belongs to.

use gpui::{
    AnyElement, App, Bounds, DevicePixels, Element, ElementId, EntityId, GlobalElementId,
    InspectorElementId, IntoElement, LayoutId, Pixels, Window,
};

use crate::floating::layers::float_host::float_region::FloatRegion;
use crate::floating::layers::float_host::part::Part;
use crate::floating::layers::registry::Registry;
use crate::floating::portal::cover::painting::Painting;
use crate::foundation::opaque_debug;

/// Wraps a floating card, laid out and painted as the card itself. In the pop-up window it
/// marks the card's bounds as shown, as taking the pointer, or both; in an overlay drawn in the
/// window, a seen card covers the native views under it.
pub struct Shaped {
    child: AnyElement,
    radius: Pixels,
    part: Part,
    overlay: Option<EntityId>,
}

impl Shaped {
    /// A card that is seen and takes the pointer, such as a menu or a toast.
    pub fn solid(
        child: impl IntoElement,
        radius: Pixels,
    ) -> Self {
        Self::new(child, radius, Part::Solid)
    }

    /// A card that is seen but lets the pointer through, such as a tooltip or a drag ghost.
    pub fn seen(
        child: impl IntoElement,
        radius: Pixels,
    ) -> Self {
        Self::new(child, radius, Part::Seen)
    }

    /// An unseen area that takes the pointer, such as the layer a press outside a menu closes
    /// it through.
    pub fn catch(child: impl IntoElement) -> Self {
        Self::new(child, Pixels::ZERO, Part::Catch)
    }

    fn new(
        child: impl IntoElement,
        radius: Pixels,
        part: Part,
    ) -> Self {
        Self {
            child: child.into_any_element(),
            radius,
            part,
            overlay: None,
        }
    }
}

opaque_debug!(Shaped);

impl Element for Shaped {
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
        self.overlay = Painting::current(cx);
        (self.child.request_layout(window, cx), ())
    }

    fn prepaint(
        &mut self,
        _: Option<&GlobalElementId>,
        _: Option<&InspectorElementId>,
        bounds: Bounds<Pixels>,
        (): &mut Self::RequestLayoutState,
        window: &mut Window,
        cx: &mut App,
    ) -> Self::PrepaintState {
        let scale = window.scale_factor();
        let region = FloatRegion {
            bounds: bounds.to_device_pixels(scale),
            radius: DevicePixels::from(self.radius.scale(scale)),
        };
        Registry::note(window.window_handle().window_id(), (self.part, region), cx);
        if let Some(overlay) = self.overlay.filter(|_| self.part != Part::Catch) {
            Painting::note(overlay, bounds, cx);
        }
        self.child.prepaint(window, cx);
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
    use gpui::{InteractiveElement as _, Styled as _, TestAppContext, div, px};

    use super::*;
    use crate::test_support::probe::measure;

    #[gpui::test]
    fn a_shaped_card_is_laid_out_as_the_card_itself(cx: &mut TestAppContext) {
        let card = || {
            div()
                .w(px(30.0))
                .h(px(12.0))
                .debug_selector(|| "card".into())
        };
        let plain = measure(cx, move |_, _| card());
        let shaped = measure(cx, move |_, _| Shaped::solid(card(), px(4.0)));
        assert_eq!(shaped.size, plain.size);
    }
}
