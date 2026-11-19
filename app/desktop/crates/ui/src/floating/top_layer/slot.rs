//! A portal's place in the top layer, released with the portal's element state.

use std::rc::Rc;

use gpui::{App, Context};

use crate::floating::top_layer::layers::Layers;

pub(crate) struct Slot {
    layers: Rc<Layers>,
}

impl Slot {
    pub(crate) fn new(
        layers: Rc<Layers>,
        cx: &mut Context<'_, Self>,
    ) -> Self {
        let slot = cx.entity_id();
        cx.on_release(move |this, cx: &mut App| this.layers.hide(slot, cx))
            .detach();
        Self { layers }
    }
}

#[cfg(test)]
mod tests {
    use gpui::{AppContext as _, IntoElement, TestAppContext, div};

    use super::*;
    use crate::floating::top_layer::layer::Layer;
    use crate::floating::top_layer::reach::Reach;

    #[gpui::test]
    fn a_released_slot_takes_its_layer_out_of_the_top_layer(cx: &mut TestAppContext) {
        let layers = Rc::new(Layers::default());
        let slot = cx.update(|cx| cx.new(|cx| Slot::new(Rc::clone(&layers), cx)));
        let layer = Layer {
            reach: Reach::Window,
            priority: 1,
            build: Rc::new(|_, _| div().into_any_element()),
        };
        cx.update(|cx| layers.place((slot.entity_id(), layer), None, cx));
        assert_eq!(layers.shown().len(), 1);
        cx.update(|_| drop(slot));
        assert!(layers.shown().is_empty());
    }
}
