//! The layers an owner window's portals show in its top layer.

use std::cell::{OnceCell, RefCell};
use std::collections::BTreeMap;

use gpui::{AnyWindowHandle, App, EntityId, FocusHandle, Window};

use crate::floating::top_layer::layer::Layer;

type Shown = BTreeMap<(usize, EntityId), Layer>;

#[derive(Default)]
pub(crate) struct Layers {
    window: OnceCell<AnyWindowHandle>,
    shown: RefCell<Shown>,
    mirrored: RefCell<Option<FocusHandle>>,
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
        let focus = owner_focus.filter(|focus| self.mirrored.borrow().as_ref() != Some(focus));
        if focus.is_some() {
            self.mirrored.replace(focus.clone());
        }
        self.redraw(focus, cx);
    }

    pub(crate) fn hide(
        &self,
        slot: EntityId,
        cx: &mut App,
    ) {
        self.shown.borrow_mut().retain(|(_, key), _| *key != slot);
        self.redraw(None, cx);
    }

    /// Redraws the top layer once the owner's frame is done, first focusing `focus` in it.
    fn redraw(
        &self,
        focus: Option<FocusHandle>,
        cx: &mut App,
    ) {
        let Some(window) = self.window.get().copied() else {
            return;
        };
        cx.defer(move |cx| redraw_now(window, focus.as_ref(), cx));
    }

    pub(crate) fn shown(&self) -> Vec<Layer> {
        self.shown.borrow().values().cloned().collect()
    }
}

fn redraw_now(
    window: AnyWindowHandle,
    focus: Option<&FocusHandle>,
    cx: &mut App,
) {
    if let Err(error) = window.update(cx, |_, window, _| show(focus, window)) {
        tracing::debug!(%error, "the top layer closed before it could redraw");
    }
}

fn show(
    focus: Option<&FocusHandle>,
    window: &mut Window,
) {
    if let Some(focus) = focus {
        window.focus(focus);
    }
    window.refresh();
}

#[cfg(test)]
mod tests {
    use std::rc::Rc;

    use gpui::{IntoElement, TestAppContext, div};

    use super::*;
    use crate::floating::top_layer::reach::Reach;

    fn layer(priority: usize) -> Layer {
        Layer {
            reach: Reach::Window,
            priority,
            build: Rc::new(|_, _| div().into_any_element()),
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
}
