//! The pop-up window of one owner window: the layers it shows, and following the owner.

use std::collections::BTreeMap;

use gpui::{AnyWindowHandle, Context, EntityId, Subscription, Window};
use raw_window_handle::RawWindowHandle;

use crate::floating::layers::registry::{Registry, raw_handle};
use crate::floating::portal::layer::Layer;

pub(crate) struct Owner {
    window: AnyWindowHandle,
    raw: Option<RawWindowHandle>,
    shown: BTreeMap<(usize, EntityId), Layer>,
    float: Option<AnyWindowHandle>,
    _watching: Vec<Subscription>,
}

impl Owner {
    pub(crate) fn new(
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> Self {
        let watching = vec![
            cx.observe_window_bounds(window, |this: &mut Self, _, cx| this.cover(cx)),
            cx.observe_window_activation(window, |_, _, cx| cx.notify()),
        ];
        Self {
            window: window.window_handle(),
            raw: raw_handle(window),
            shown: BTreeMap::new(),
            float: None,
            _watching: watching,
        }
    }

    pub(crate) fn place(
        &mut self,
        slot: EntityId,
        layer: Layer,
        cx: &mut Context<'_, Self>,
    ) -> bool {
        self.hide(slot, cx);
        self.shown.insert((layer.priority, slot), layer);
        self.float.is_none()
    }

    pub(crate) fn hide(
        &mut self,
        slot: EntityId,
        cx: &mut Context<'_, Self>,
    ) {
        self.shown.retain(|(_, key), _| *key != slot);
        cx.notify();
    }

    pub(crate) fn shown(&self) -> impl Iterator<Item = &Layer> {
        self.shown.values()
    }

    pub(crate) fn window(&self) -> AnyWindowHandle {
        self.window
    }

    pub(crate) fn raw(&self) -> Option<RawWindowHandle> {
        self.raw
    }

    pub(crate) fn float(&self) -> Option<AnyWindowHandle> {
        self.float
    }

    pub(crate) fn set_float(
        &mut self,
        float: AnyWindowHandle,
    ) {
        self.float = Some(float);
    }

    fn cover(
        &self,
        cx: &mut Context<'_, Self>,
    ) {
        let (Some(host), Some(owner), Some(float)) = (Registry::host(cx), self.raw, self.float)
        else {
            return;
        };
        let raw = float
            .update(cx, |_, window, _| raw_handle(window))
            .ok()
            .flatten();
        if let Some(raw) = raw {
            host.cover(raw, owner);
        }
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use crate::Float;
    use crate::test_support::floats::{toggled, windows};

    #[gpui::test]
    fn the_pop_up_window_closes_with_its_owner(cx: &mut TestAppContext) {
        let (_, mut owner) = toggled(cx, Float::Passive);
        assert_eq!(windows(&mut owner), 2);
        owner.update(|window, _| window.remove_window());
        owner.run_until_parked();
        assert!(cx.update(|cx| cx.windows().is_empty()));
    }
}
