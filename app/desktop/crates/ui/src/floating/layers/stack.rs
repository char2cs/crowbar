//! The layers one float window of an owner shows, in paint order, and the window once open.

use std::collections::BTreeMap;

use gpui::{AnyWindowHandle, EntityId};

use crate::floating::portal::layer::Layer;

#[derive(Default)]
pub(crate) struct Stack {
    shown: BTreeMap<(usize, EntityId), Layer>,
    window: Option<AnyWindowHandle>,
}

impl Stack {
    pub(crate) fn place(
        &mut self,
        slot: EntityId,
        layer: Layer,
    ) -> bool {
        self.hide(slot);
        self.shown.insert((layer.priority, slot), layer);
        self.window.is_none()
    }

    pub(crate) fn hide(
        &mut self,
        slot: EntityId,
    ) {
        self.shown.retain(|(_, key), _| *key != slot);
    }

    pub(crate) fn shown(&self) -> Vec<Layer> {
        self.shown.values().cloned().collect()
    }

    pub(crate) fn window(&self) -> Option<AnyWindowHandle> {
        self.window
    }

    pub(crate) fn set_window(
        &mut self,
        window: Option<AnyWindowHandle>,
    ) {
        self.window = window;
    }
}

#[cfg(test)]
mod tests {
    use std::rc::Rc;

    use gpui::{AppContext as _, Bounds, IntoElement as _, TestAppContext, div};

    use super::*;
    use crate::floating::portal::reach::Reach;

    fn layer(priority: usize) -> Layer {
        Layer {
            reach: Reach::Here,
            priority,
            build: Rc::new(|_, _, _| div().into_any_element()),
            bounds: Bounds::default(),
        }
    }

    #[gpui::test]
    fn each_slot_shows_its_latest_layer_in_priority_order(cx: &mut TestAppContext) {
        let (low, high) = cx.update(|cx| (cx.new(|_| ()).entity_id(), cx.new(|_| ()).entity_id()));
        let mut stack = Stack::default();
        assert!(stack.place(high, layer(5)), "no window yet");
        assert!(stack.place(low, layer(9)));
        assert!(
            stack.place(low, layer(1)),
            "a later layer of the slot replaces it"
        );
        let order: Vec<usize> = stack.shown().iter().map(|layer| layer.priority).collect();
        assert_eq!(order, [1, 5]);
        stack.hide(low);
        stack.hide(high);
        assert!(stack.shown().is_empty());
    }
}
