//! The layers an owner window's portals show in its top layer.

use std::cell::{OnceCell, RefCell};
use std::collections::BTreeMap;

use gpui::{AnyWindowHandle, App, EntityId, FocusHandle};

use crate::floating::top_layer::layer::Layer;
use crate::floating::top_layer::redraw::{Redraw, show_at_once};

type Shown = BTreeMap<(usize, EntityId), Layer>;

#[derive(Default)]
pub(crate) struct Layers {
    window: OnceCell<AnyWindowHandle>,
    shown: RefCell<Shown>,
}

impl Layers {
    pub(crate) fn attach(
        &self,
        window: AnyWindowHandle,
    ) {
        self.window.get_or_init(|| window);
    }

    pub(crate) fn place(
        &self,
        (slot, layer): (EntityId, Layer),
        owner_focus: Option<FocusHandle>,
        cx: &mut App,
    ) {
        let mut shown = self.shown.borrow_mut();
        shown.retain(|(_, key), _| *key != slot);
        shown.insert((layer.priority, slot), layer);
        drop(shown);
        self.redraw(owner_focus, cx);
    }

    pub(crate) fn hide(
        &self,
        slot: EntityId,
        cx: &mut App,
    ) {
        self.shown.borrow_mut().retain(|(_, key), _| *key != slot);
        self.redraw(None, cx);
    }

    fn redraw(
        &self,
        focus: Option<FocusHandle>,
        cx: &mut App,
    ) {
        let Some(window) = self.window.get().copied() else {
            return;
        };
        let floating = self.is_floating();
        if floating {
            show_at_once(window, cx);
        }
        let redraw = Redraw { focus, floating };
        cx.defer(move |cx| redraw.on(window, cx));
    }

    fn is_floating(&self) -> bool {
        !self.shown.borrow().is_empty()
    }

    pub(crate) fn take_keys(
        &self,
        cx: &mut App,
    ) -> bool {
        let Some(window) = self.window.get().copied() else {
            return false;
        };
        window
            .update(cx, |_, window, _| window.takes_keys())
            .unwrap_or_default()
    }

    pub(crate) fn shown(&self) -> Vec<Layer> {
        self.shown.borrow().values().cloned().collect()
    }
}

#[cfg(test)]
mod tests {
    use std::rc::Rc;

    use gpui::{IntoElement, TestAppContext, div};

    use super::*;
    use crate::floating::top_layer::Registry;
    use crate::floating::top_layer::reach::Reach;

    fn layer(priority: usize) -> Layer {
        Layer {
            reach: Reach::Window,
            priority,
            build: Rc::new(|_, _, _| div().into_any_element()),
        }
    }

    fn priorities(layers: &Layers) -> Vec<usize> {
        layers.shown().iter().map(|layer| layer.priority).collect()
    }

    #[gpui::test]
    fn layers_paint_lowest_priority_first_and_a_slot_shows_one_layer(cx: &mut TestAppContext) {
        let layers = Layers::default();
        let (low, high) = (EntityId::from(1_u64), EntityId::from(2_u64));
        cx.update(|cx| {
            layers.place((high, layer(9)), None, cx);
            layers.place((low, layer(3)), None, cx);
            layers.place((low, layer(5)), None, cx);
        });
        assert_eq!(priorities(&layers), vec![5, 9]);
        cx.update(|cx| layers.hide(high, cx));
        assert_eq!(priorities(&layers), vec![5]);
    }

    #[gpui::test]
    fn a_layer_placed_while_the_owner_draws_shows_the_top_layer_at_once(cx: &mut TestAppContext) {
        let (mut owner, mut top) = crate::test_support::probe::open_floating(cx, |_, _| div());
        let layers = owner.update(|window, cx| Registry::of(window, cx));
        let Some(layers) = layers else {
            panic!("the owner has a top layer");
        };
        let top = top.update(|window, _| window.window_handle());
        let shown = cx.update(|cx| {
            layers.place((EntityId::from(1_u64), layer(1)), None, cx);
            top.update(cx, |_, window, _| window.is_overlay_shown())
        });
        assert_eq!(
            shown.ok(),
            Some(true),
            "shown before the owner's frame ends"
        );
    }
}
