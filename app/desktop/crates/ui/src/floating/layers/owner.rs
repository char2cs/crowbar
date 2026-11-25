//! The float windows of one owner window: what each shows, and following the owner.

use std::collections::BTreeMap;

use gpui::{AnyWindowHandle, App, Context, EntityId, Subscription, Window};
use raw_window_handle::RawWindowHandle;

use crate::floating::layers::raw_handle::raw_handle;
use crate::floating::layers::registry::Registry;
use crate::floating::layers::stack::Stack;
use crate::floating::portal::float::Float;
use crate::floating::portal::layer::Layer;

pub(crate) struct Owner {
    window: AnyWindowHandle,
    raw: Option<RawWindowHandle>,
    floats: BTreeMap<Float, Stack>,
    _watching: Vec<Subscription>,
}

impl Owner {
    pub(crate) fn new(
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) -> Self {
        let watching = vec![
            cx.observe_window_bounds(window, |this: &mut Self, _, cx| this.cover(cx)),
            cx.observe_window_activation(window, Self::follow_activation),
        ];
        Self {
            window: Window::window_handle(window),
            raw: raw_handle(window),
            floats: BTreeMap::new(),
            _watching: watching,
        }
    }

    fn follow_activation(
        &mut self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        cx.notify();
        let held = self.holds_keys().filter(|_| window.is_window_active());
        let Some(float) = held else {
            return;
        };
        float
            .update(cx, |_, window, _| window.activate_window())
            .unwrap_or_else(|error| tracing::debug!(%error, "a float window was already closed"));
    }

    pub(crate) fn place(
        &mut self,
        (slot, float): (EntityId, Float),
        layer: Layer,
        cx: &mut Context<'_, Self>,
    ) -> bool {
        self.floats.values_mut().for_each(|stack| stack.hide(slot));
        let opens = self.floats.entry(float).or_default().place(slot, layer);
        cx.notify();
        opens
    }

    pub(crate) fn hide(
        &mut self,
        slot: EntityId,
        cx: &mut Context<'_, Self>,
    ) {
        self.floats.values_mut().for_each(|stack| stack.hide(slot));
        cx.notify();
    }

    pub(crate) fn shown(
        &self,
        float: Float,
    ) -> Vec<Layer> {
        self.floats.get(&float).map_or_else(Vec::new, Stack::shown)
    }

    pub(crate) fn window(&self) -> AnyWindowHandle {
        self.window
    }

    pub(crate) fn raw(&self) -> Option<RawWindowHandle> {
        self.raw
    }

    pub(crate) fn floats(&self) -> Vec<(Float, AnyWindowHandle)> {
        self.floats
            .iter()
            .filter_map(|(float, stack)| Some((*float, stack.window()?)))
            .collect()
    }

    pub(crate) fn holds_keys(&self) -> Option<AnyWindowHandle> {
        self.floats()
            .into_iter()
            .find(|(kind, _)| kind.takes_keys())
            .map(|(_, window)| window)
    }

    pub(crate) fn set_window(
        &mut self,
        float: Float,
        window: Option<AnyWindowHandle>,
    ) {
        self.floats.entry(float).or_default().set_window(window);
    }

    pub(crate) fn close_floats(
        &mut self,
        cx: &mut App,
    ) {
        self.floats()
            .into_iter()
            .filter_map(|(_, float)| {
                float
                    .update(cx, |_, window, _| window.remove_window())
                    .err()
            })
            .for_each(|error| tracing::debug!(%error, "a float window was already closed"));
    }

    fn cover(
        &self,
        cx: &mut Context<'_, Self>,
    ) {
        let (Some(host), Some(owner)) = (Registry::host(cx), self.raw) else {
            return;
        };
        let floats: Vec<_> = self
            .floats()
            .into_iter()
            .filter_map(|(kind, float)| {
                let raw = float.update(cx, |_, window, _| raw_handle(window)).ok()?;
                Some((kind, raw?))
            })
            .collect();
        for (kind, raw) in floats {
            host.cover(raw, owner, kind);
        }
    }
}

#[cfg(test)]
mod tests {
    use std::cell::Cell;
    use std::rc::Rc;

    use gpui::{ParentElement as _, TestAppContext, div};

    use crate::test_support::floats::{floated, toggled, windows};
    use crate::test_support::probe::{open_floating, settle};
    use crate::testing::newest_window;
    use crate::{Float, Portal};

    #[gpui::test]
    fn a_portal_that_turns_modal_leaves_the_passive_window(cx: &mut TestAppContext) {
        let kind = Rc::new(Cell::new(Float::Passive));
        let read = Rc::clone(&kind);
        let mut owner = open_floating(cx, move |_, _| {
            div().child(Portal::here("p", |_, _| floated()).float(read.get()))
        });
        let mut passive = newest_window(&mut owner);
        kind.set(Float::Modal);
        owner.update(|window, _| window.refresh());
        settle(&mut owner);
        assert_eq!(
            passive.debug_bounds("floated"),
            None,
            "left the passive window"
        );
        assert!(newest_window(&mut owner).debug_bounds("floated").is_some());
    }

    #[gpui::test]
    fn float_windows_close_with_their_owner(cx: &mut TestAppContext) {
        let (_, mut owner) = toggled(cx, Float::Passive);
        assert_eq!(windows(&mut owner), 2);
        owner.update(|window, _| window.remove_window());
        owner.run_until_parked();
        assert!(cx.update(|cx| cx.windows().is_empty()));
    }
}
